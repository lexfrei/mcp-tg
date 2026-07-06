// Package main provides the entry point for the mcp-tg MCP server.
package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/log/logzap"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/sync/errgroup"

	"github.com/lexfrei/mcp-tg/internal/completions"
	"github.com/lexfrei/mcp-tg/internal/config"
	mcpmw "github.com/lexfrei/mcp-tg/internal/middleware"
	"github.com/lexfrei/mcp-tg/internal/prompts"
	"github.com/lexfrei/mcp-tg/internal/resources"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName        = "mcp-tg"
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
	keepAliveInterval = 30 * time.Second
)

var (
	version  = "dev"
	revision = "unknown"
)

func main() {
	if loginRequested(os.Args) {
		if loginErr := runLogin(); loginErr != nil {
			log.Printf("login error: %v", loginErr)
			os.Exit(1)
		}

		return
	}

	err := run()
	if err != nil {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		return errors.Wrap(cfgErr, "invalid configuration")
	}

	mkdirErr := os.MkdirAll(filepath.Dir(cfg.SessionFile), 0o700)
	if mkdirErr != nil {
		return errors.Wrap(mkdirErr, "creating session directory")
	}

	ensureSessionPerms(cfg.SessionFile)

	logger := newLogger()
	health := mcpmw.NewSessionHealth()

	tgClient := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.SessionFile},
		Logger:         logzap.New(newGotdLogger()),
		Middlewares: []telegram.Middleware{
			newFloodWaitMiddleware(),
			newConnReinitMiddleware(cfg.AppID),
			newAuthRevokedMiddleware(health, logger),
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(ctx, cancel)

	return errors.Wrap(tgClient.Run(ctx, func(ctx context.Context) error {
		return startServer(ctx, cancel, tgClient, cfg, health)
	}), "telegram client stopped")
}

func startServer(
	ctx context.Context, cancel context.CancelFunc, tgClient *telegram.Client,
	cfg *config.Config, health *mcpmw.SessionHealth,
) error {
	wrapper := tgclient.NewWrapper(tgClient.API())

	if cfg.HTTPOnly {
		return startHeadless(ctx, tgClient, wrapper, cfg, health)
	}

	return startStdio(ctx, cancel, tgClient, wrapper, cfg, health)
}

// startStdio runs the server with stdio as the primary transport (the default
// one-process-per-client mode) plus an optional additional HTTP transport.
// Auth elicitation is routed through the stdio session, so this mode can
// complete an interactive login.
func startStdio(
	ctx context.Context, cancel context.CancelFunc, tgClient *telegram.Client,
	wrapper tgclient.Client, cfg *config.Config, health *mcpmw.SessionHealth,
) error {
	initDone := make(chan struct{})
	authDone := make(chan struct{})

	server := buildServer(wrapper, cfg.DownloadDir, authDone, health, func() { close(initDone) })

	stdioSession, err := server.Connect(ctx, &mcp.StdioTransport{}, nil)
	if err != nil {
		return errors.Wrap(err, "connecting stdio transport")
	}

	select {
	case <-initDone:
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), "waiting for client initialization")
	}

	authErr := authenticate(ctx, tgClient, cfg, stdioSession)
	if authErr != nil {
		return authErr
	}

	// Arm revocation tracking only now: the IfNecessary probe above answers
	// AUTH_KEY_UNREGISTERED for a fresh or revoked session, and that expected
	// pre-login 401 must not be mistaken for a revocation of this new session.
	health.Arm()
	close(authDone)

	return waitForTransports(ctx, cancel, server, stdioSession, cfg)
}

// startHeadless runs the server with HTTP as the only transport and no stdio
// peer. One process and one Telegram connection serve many concurrent MCP
// clients — the shared-daemon mode.
//
// Auth cannot elicit interactively here (there is no client session to prompt),
// so it relies on a valid persisted session file or env-var credentials. Log in
// once in stdio mode to create the session, then run headless.
func startHeadless(
	ctx context.Context, tgClient *telegram.Client, wrapper tgclient.Client,
	cfg *config.Config, health *mcpmw.SessionHealth,
) error {
	authDone := make(chan struct{})
	server := newHeadlessServer(wrapper, cfg.DownloadDir, authDone, health)

	authErr := authenticate(ctx, tgClient, cfg, nil)
	if authErr != nil {
		return authErr
	}

	// See startStdio: arm only after the initial auth succeeds so the startup
	// probe's expected AUTH_KEY_UNREGISTERED does not trip the revocation guard.
	health.Arm()
	close(authDone)

	log.Printf("starting in HTTP-only headless mode (shared daemon)")

	return runHTTPServer(ctx, server, cfg.HTTPAddr())
}

// newHeadlessServer builds the MCP server for headless HTTP-only mode. It is a
// named seam so the daemon and its tests construct the server identically — in
// particular the nil init hook is owned here, not duplicated per call site.
//
// onInit must stay nil: headless HTTP serves many clients, and a hook that
// closes a shared channel would panic on the second client's initialize (close
// of a closed channel). Wiring such a hook here trips the multi-client test.
func newHeadlessServer(
	client tgclient.Client, downloadDir string, authDone chan struct{}, health *mcpmw.SessionHealth,
) *mcp.Server {
	return buildServer(client, downloadDir, authDone, health, nil)
}

// buildServer constructs the MCP server with all tools, resources, prompts, and
// middleware. onInit, when non-nil, runs after a client completes the MCP
// initialize handshake; pass nil when no single client owns the lifecycle.
func buildServer(
	client tgclient.Client, downloadDir string, authDone chan struct{},
	health *mcpmw.SessionHealth, onInit func(),
) *mcp.Server {
	opts := newServerOptions(client)
	if onInit != nil {
		opts.InitializedHandler = func(_ context.Context, _ *mcp.InitializedRequest) {
			onInit()
		}
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: version + "+" + revision,
		},
		opts,
	)

	boolFields := tools.BoolFieldRegistry{}
	registerTools(server, client, boolFields, downloadDir)
	resources.Register(server, client)
	prompts.Register(server, client)
	server.AddReceivingMiddleware(receivingMiddlewares(opts.Logger, boolFields, authDone, health)...)

	return server
}

// receivingMiddlewares returns the server middleware chain; the first entry is
// the outermost wrapper. Logging must wrap the auth guard so calls rejected
// before reaching a handler (every tool call made while authentication is
// still pending) show up in the request log too.
func receivingMiddlewares(
	logger *slog.Logger, boolFields tools.BoolFieldRegistry, authDone chan struct{},
	health *mcpmw.SessionHealth,
) []mcp.Middleware {
	return []mcp.Middleware{
		mcpmw.NewLogging(logger),
		mcpmw.NewBoolCoercer(boolFields),
		mcpmw.NewAuthGuard(authDone, []string{tools.ServerVersionToolName}),
		mcpmw.NewSessionGuard(health, []string{tools.ServerVersionToolName}),
	}
}

// authenticate runs the Telegram auth flow. When session is non-nil the
// authenticator may elicit missing credentials through it; otherwise it is
// limited to env vars and the persisted session file.
func authenticate(
	ctx context.Context, tgClient *telegram.Client, cfg *config.Config, clientSession *mcp.ServerSession,
) error {
	authenticator := tgclient.NewAuthenticator(cfg.Phone, cfg.Password, cfg.AuthCode)
	if clientSession != nil {
		authenticator.SetSession(clientSession)
	}

	flow := auth.NewFlow(authenticator, auth.SendCodeOptions{})

	return errors.Wrap(tgClient.Auth().IfNecessary(ctx, flow), "authentication failed")
}

func waitForTransports(
	ctx context.Context,
	cancel context.CancelFunc,
	server *mcp.Server,
	stdioSession *mcp.ServerSession,
	cfg *config.Config,
) error {
	group, groupCtx := errgroup.WithContext(ctx)
	httpEnabled := cfg.HTTPEnabled()

	group.Go(func() error {
		waitErr := stdioSession.Wait()
		if waitErr != nil && groupCtx.Err() == nil {
			return errors.Wrap(waitErr, "stdio session ended")
		}

		if !httpEnabled {
			cancel()
		}

		return nil
	})

	if httpEnabled {
		group.Go(func() error {
			return runHTTPServer(groupCtx, server, cfg.HTTPAddr())
		})
	}

	return group.Wait() //nolint:wrapcheck // errors are already wrapped inside group goroutines.
}

const sessionFilePerms = 0o600

// ensureSessionPerms sets restrictive permissions on the session file
// if it already exists. gotd/td creates it with default umask (often 0644),
// but it contains MTProto auth keys and should not be world-readable.
func ensureSessionPerms(path string) {
	_ = os.Chmod(path, sessionFilePerms)
}

func setupSignalHandler(ctx context.Context, cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}

		signal.Stop(sigChan)
	}()
}

// newLogger builds the structured logger used for MCP request logging and the
// invoker middlewares. Text handler to stderr, which launchd routes into the
// daemon log.
func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// newGotdLogger builds the zap logger handed to gotd so MTProto connection,
// migration, and auth-key lifecycle events land in the daemon log. Without it
// gotd defaults to a nop logger and an incident leaves no client-side trace —
// exactly what made the last AUTH_KEY_UNREGISTERED hard to explain.
//
// It uses the console encoder with ISO8601 timestamps so gotd lines read as
// plain text alongside the slog output on the same stderr stream, rather than
// JSON amid key=value. Falls back to a nop logger if zap construction fails,
// never blocking startup.
func newGotdLogger() *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}

	return logger
}

func newServerOptions(client tgclient.Client) *mcp.ServerOptions {
	logger := newLogger()

	return &mcp.ServerOptions{
		Instructions: "MCP server for Telegram Client API (MTProto, user account, not bot). " +
			"All tools accepting 'peer' support: @username, bare username, " +
			"https://t.me/username, t.me/+invite_hash, or numeric bot-API style ID " +
			"(positive=user, negative=chat, -100xxx=channel). Prefer @username over numeric IDs. " +
			"Tools with 'limit' accept pagination: use offsetId or offsetDate from previous results. " +
			"Read-only tools are safe to call freely. Write/destructive tools modify Telegram state.",
		Logger:            logger,
		KeepAlive:         keepAliveInterval,
		CompletionHandler: completions.NewHandler(client),
		RootsListChangedHandler: func(_ context.Context, _ *mcp.RootsListChangedRequest) {
			logger.Info("client roots list changed")
		},
	}
}

func registerTools(server *mcp.Server, client tgclient.Client, registry tools.BoolFieldRegistry, downloadDir string) {
	tools.AddTool(server, registry, tools.ServerVersionTool(),
		tools.NewServerVersionHandler(version, revision, runtime.Version()))
	tools.AddTool(server, registry, tools.ProfileGetTool(), tools.NewProfileGetHandler(client))
	tools.AddTool(server, registry, tools.DialogsListTool(), tools.NewDialogsListHandler(client))
	tools.AddTool(server, registry, tools.DialogsSearchTool(), tools.NewDialogsSearchHandler(client))
	tools.AddTool(server, registry, tools.DialogsGetInfoTool(), tools.NewDialogsGetInfoHandler(client))
	tools.AddTool(server, registry, tools.MessagesListTool(), tools.NewMessagesListHandler(client))
	tools.AddTool(server, registry, tools.MessagesGetTool(), tools.NewMessagesGetHandler(client))
	tools.AddTool(server, registry, tools.MessagesContextTool(), tools.NewMessagesContextHandler(client))
	tools.AddTool(server, registry, tools.MessagesSearchTool(), tools.NewMessagesSearchHandler(client))
	tools.AddTool(server, registry, tools.MessagesSendTool(), tools.NewMessagesSendHandler(client))
	tools.AddTool(server, registry, tools.MessagesEditTool(), tools.NewMessagesEditHandler(client))
	tools.AddTool(server, registry, tools.MessagesDeleteTool(), tools.NewMessagesDeleteHandler(client))
	tools.AddTool(server, registry, tools.MessagesForwardTool(), tools.NewMessagesForwardHandler(client))
	tools.AddTool(server, registry, tools.MessagesPinTool(), tools.NewMessagesPinHandler(client))
	tools.AddTool(server, registry, tools.MessagesReactTool(), tools.NewMessagesReactHandler(client))
	tools.AddTool(server, registry, tools.MessagesMarkReadTool(), tools.NewMessagesMarkReadHandler(client))

	// Phase 2: Contacts, Users, Groups, Chat management tools.
	tools.AddTool(server, registry, tools.ContactsGetTool(), tools.NewContactsGetHandler(client))
	tools.AddTool(server, registry, tools.ContactsSearchTool(), tools.NewContactsSearchHandler(client))
	tools.AddTool(server, registry, tools.UsersGetTool(), tools.NewUsersGetHandler(client))
	tools.AddTool(server, registry, tools.UsersPhotosTool(), tools.NewUsersPhotosHandler(client))
	tools.AddTool(server, registry, tools.UsersBlockTool(), tools.NewUsersBlockHandler(client))
	tools.AddTool(server, registry, tools.UsersCommonChatsTool(), tools.NewUsersCommonChatsHandler(client))
	tools.AddTool(server, registry, tools.GroupsListTool(), tools.NewGroupsListHandler(client))
	tools.AddTool(server, registry, tools.GroupsInfoTool(), tools.NewGroupsInfoHandler(client))
	tools.AddTool(server, registry, tools.GroupsJoinTool(), tools.NewGroupsJoinHandler(client))
	tools.AddTool(server, registry, tools.GroupsLeaveTool(), tools.NewGroupsLeaveHandler(client))
	tools.AddTool(server, registry, tools.GroupsRenameTool(), tools.NewGroupsRenameHandler(client))
	tools.AddTool(server, registry, tools.GroupsMembersAddTool(), tools.NewGroupsMembersAddHandler(client))
	tools.AddTool(server, registry, tools.GroupsMembersRemoveTool(), tools.NewGroupsMembersRemoveHandler(client))
	tools.AddTool(server, registry, tools.GroupsInviteLinkGetTool(), tools.NewGroupsInviteLinkGetHandler(client))
	tools.AddTool(server, registry, tools.GroupsInviteLinkRevokeTool(), tools.NewGroupsInviteLinkRevokeHandler(client))
	tools.AddTool(server, registry, tools.ChatsAdminsTool(), tools.NewChatsAdminsHandler(client))
	tools.AddTool(server, registry, tools.ChatsPermissionsTool(), tools.NewChatsPermissionsHandler(client))

	// Phase 3: Media, Files, Chat Management, Profile tools.
	tools.AddTool(server, registry, tools.MessagesSendFileTool(), tools.NewMessagesSendFileHandler(client))
	tools.AddTool(server, registry, tools.MediaDownloadTool(), tools.NewMediaDownloadHandler(client, downloadDir))
	tools.AddTool(server, registry, tools.MediaUploadTool(), tools.NewMediaUploadHandler(client))
	tools.AddTool(server, registry, tools.MediaSendAlbumTool(), tools.NewMediaSendAlbumHandler(client))
	tools.AddTool(server, registry, tools.ChatsCreateTool(), tools.NewChatsCreateHandler(client))
	tools.AddTool(server, registry, tools.ChatsArchiveTool(), tools.NewChatsArchiveHandler(client))
	tools.AddTool(server, registry, tools.ChatsMuteTool(), tools.NewChatsMuteHandler(client))
	tools.AddTool(server, registry, tools.ChatsDeleteTool(), tools.NewChatsDeleteHandler(client))
	tools.AddTool(server, registry, tools.ChatsSetPhotoTool(), tools.NewChatsSetPhotoHandler(client))
	tools.AddTool(server, registry, tools.ChatsSetDescriptionTool(), tools.NewChatsSetDescriptionHandler(client))
	tools.AddTool(server, registry, tools.ProfileSetNameTool(), tools.NewProfileSetNameHandler(client))
	tools.AddTool(server, registry, tools.ProfileSetBioTool(), tools.NewProfileSetBioHandler(client))
	tools.AddTool(server, registry, tools.ProfileSetPhotoTool(), tools.NewProfileSetPhotoHandler(client))

	// Phase 4: Topics, Stickers, Drafts, Folders, Status tools.
	tools.AddTool(server, registry, tools.TopicsListTool(), tools.NewTopicsListHandler(client))
	tools.AddTool(server, registry, tools.TopicsSearchTool(), tools.NewTopicsSearchHandler(client))
	tools.AddTool(server, registry, tools.StickersSearchTool(), tools.NewStickersSearchHandler(client))
	tools.AddTool(server, registry, tools.StickersGetSetTool(), tools.NewStickersGetSetHandler(client))
	tools.AddTool(server, registry, tools.StickersSendTool(), tools.NewStickersSendHandler(client))
	tools.AddTool(server, registry, tools.DraftsSetTool(), tools.NewDraftsSetHandler(client))
	tools.AddTool(server, registry, tools.DraftsClearTool(), tools.NewDraftsClearHandler(client))
	tools.AddTool(server, registry, tools.FoldersListTool(), tools.NewFoldersListHandler(client))
	tools.AddTool(server, registry, tools.FoldersCreateTool(), tools.NewFoldersCreateHandler(client))
	tools.AddTool(server, registry, tools.FoldersEditTool(), tools.NewFoldersEditHandler(client))
	tools.AddTool(server, registry, tools.FoldersDeleteTool(), tools.NewFoldersDeleteHandler(client))
	tools.AddTool(server, registry, tools.TypingSendTool(), tools.NewTypingSendHandler(client))
	tools.AddTool(server, registry, tools.OnlineStatusSetTool(), tools.NewOnlineStatusSetHandler(client))

	// Phase 5: Extended coverage tools.
	tools.AddTool(server, registry, tools.MessagesGetScheduledTool(), tools.NewMessagesGetScheduledHandler(client))
	tools.AddTool(server, registry, tools.MessagesSearchGlobalTool(), tools.NewMessagesSearchGlobalHandler(client))
	tools.AddTool(server, registry, tools.ContactsListBlockedTool(), tools.NewContactsListBlockedHandler(client))
	tools.AddTool(server, registry, tools.MessagesGetReactionsTool(), tools.NewMessagesGetReactionsHandler(client))
	tools.AddTool(server, registry, tools.GroupsMembersListTool(), tools.NewGroupsMembersListHandler(client))
	tools.AddTool(server, registry, tools.ContactsGetStatusesTool(), tools.NewContactsGetStatusesHandler(client))
	tools.AddTool(server, registry, tools.DialogsPinTool(), tools.NewDialogsPinHandler(client))
	tools.AddTool(server, registry, tools.DialogsMarkUnreadTool(), tools.NewDialogsMarkUnreadHandler(client))
	tools.AddTool(server, registry, tools.GroupsSlowmodeTool(), tools.NewGroupsSlowmodeHandler(client))
	tools.AddTool(server, registry, tools.TopicsCreateTool(), tools.NewTopicsCreateHandler(client))
	tools.AddTool(server, registry, tools.TopicsEditTool(), tools.NewTopicsEditHandler(client))
	tools.AddTool(server, registry, tools.ContactsAddTool(), tools.NewContactsAddHandler(client))
	tools.AddTool(server, registry, tools.GroupsAdminSetTool(), tools.NewGroupsAdminSetHandler(client))
	tools.AddTool(server, registry, tools.ContactsDeleteTool(), tools.NewContactsDeleteHandler(client))
	tools.AddTool(server, registry, tools.MessagesDeleteHistoryTool(), tools.NewMessagesDeleteHistoryHandler(client))
	tools.AddTool(server, registry, tools.MessagesClearAllDraftsTool(), tools.NewMessagesClearAllDraftsHandler(client))
}

// newHTTPHandler builds the HTTP handler chain for the MCP server with
// explicit cross-origin protection.
//
// MCP SDK v1.6 stopped enabling cross-origin protection by default when
// StreamableHTTPOptions is nil. Wrap the handler explicitly so a browser
// page on the same host cannot drive the HTTP transport via CSRF. DNS
// rebinding protection stays on by default in the SDK itself.
func newHTTPHandler(server *mcp.Server) http.Handler {
	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server {
			return server
		},
		nil,
	)

	return http.NewCrossOriginProtection().Handler(handler)
}

func runHTTPServer(ctx context.Context, server *mcp.Server, addr string) error {
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           newHTTPHandler(server),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       keepAliveInterval * 2,
	}

	//nolint:gosec // G118: ctx is already cancelled when goroutine runs, must use fresh context for graceful shutdown.
	go func() {
		<-ctx.Done()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		shutdownErr := httpServer.Shutdown(shutdownCtx) //nolint:contextcheck // ctx is cancelled, need fresh context for graceful shutdown.
		if shutdownErr != nil {
			log.Printf("HTTP server shutdown error: %v", shutdownErr)
		}
	}()

	listener, listenErr := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if listenErr != nil {
		return errors.Wrapf(listenErr, "HTTP port %s unavailable", addr)
	}

	log.Printf("HTTP server listening on %s", addr)

	serveErr := httpServer.Serve(listener)
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}

	return errors.Wrap(serveErr, "HTTP serve failed")
}

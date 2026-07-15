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
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
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
	if versionRequested(os.Args) {
		runVersion()

		return
	}

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
	level, levelErr := resolveLogLevel(os.Args, os.Getenv("MCP_LOG_LEVEL"))
	if levelErr != nil {
		return levelErr
	}

	logger := newLogger(level)
	logStartupVersion(logger)

	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		return errors.Wrap(cfgErr, "invalid configuration")
	}

	if dirErr := ensureFileStorageDir(cfg, cfg.InsecureStorage); dirErr != nil {
		return dirErr
	}

	health := mcpmw.NewSessionHealth()

	storage, storageErr := newSessionStorage(cfg, cfg.InsecureStorage)
	if storageErr != nil {
		return storageErr
	}

	device := mcpDevice()

	transcriptionBroker := tgclient.NewTranscriptionBroker()
	subscriptionBroker := tgclient.NewSubscriptionBroker()
	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnTranscribedAudio(transcriptionBroker.HandleUpdate)
	dispatcher.OnNewMessage(subscriptionBroker.HandleNewMessage)
	dispatcher.OnNewChannelMessage(subscriptionBroker.HandleNewChannelMessage)

	tgClient := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: storage,
		Logger:         logzap.New(newGotdLogger(level)),
		Device:         device,
		UpdateHandler:  dispatcher,
		Middlewares: []telegram.Middleware{
			newFloodWaitMiddleware(logger),
			newConnReinitMiddleware(cfg.AppID, &device),
			newAuthRevokedMiddleware(health, logger),
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(ctx, cancel)

	return revokedExitError(tgClient.Run(ctx, func(ctx context.Context) error {
		return startServer(ctx, cancel, tgClient, transcriptionBroker, subscriptionBroker, cfg, health, logger)
	}))
}

func startServer(
	ctx context.Context, cancel context.CancelFunc, tgClient *telegram.Client,
	transcriptionBroker *tgclient.TranscriptionBroker, subscriptionBroker *tgclient.SubscriptionBroker,
	cfg *config.Config, health *mcpmw.SessionHealth, logger *slog.Logger,
) error {
	wrapper := tgclient.NewWrapperWithTranscriptionBroker(tgClient.API(), transcriptionBroker)

	if cfg.HTTPOnly {
		return startHeadless(ctx, tgClient, wrapper, subscriptionBroker, cfg, health, logger)
	}

	return startStdio(ctx, cancel, tgClient, wrapper, subscriptionBroker, cfg, health, logger)
}

// startStdio runs the server with stdio as the primary transport (the default
// one-process-per-client mode) plus an optional additional HTTP transport.
// Auth elicitation is routed through the stdio session, so this mode can
// complete an interactive login.
func startStdio(
	ctx context.Context, cancel context.CancelFunc, tgClient *telegram.Client,
	wrapper tgclient.Client, subscriptionBroker *tgclient.SubscriptionBroker,
	cfg *config.Config, health *mcpmw.SessionHealth, logger *slog.Logger,
) error {
	initDone := make(chan struct{})
	authDone := make(chan struct{})

	server := buildServer(
		wrapper, cfg.DownloadDir, subscriptionBroker, authDone, health, func() { close(initDone) }, logger,
	)

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

	return waitForTransports(ctx, cancel, server, stdioSession, cfg, logger)
}

// startHeadless runs the server with HTTP as the only transport and no stdio
// peer. One process and one Telegram connection serve many concurrent MCP
// clients — the shared-daemon mode.
//
// Auth cannot elicit interactively here (there is no client session to prompt),
// so it relies on a session persisted by an earlier `mcp-tg login` — in the OS
// keychain by default, or a plaintext file under --insecure-storage. If none is
// valid it fails fast via headlessLoginRequired, pointing at `mcp-tg login`.
func startHeadless(
	ctx context.Context, tgClient *telegram.Client, wrapper tgclient.Client,
	subscriptionBroker *tgclient.SubscriptionBroker, cfg *config.Config, health *mcpmw.SessionHealth,
	logger *slog.Logger,
) error {
	authDone := make(chan struct{})
	server := newHeadlessServer(wrapper, cfg.DownloadDir, subscriptionBroker, authDone, health, logger)

	authErr := authenticate(ctx, tgClient, cfg, nil)
	if authErr != nil {
		if loginWouldFix(authErr) {
			return headlessLoginRequired(authErr)
		}

		// A transient failure (network, 5xx, DC migration) that re-login cannot
		// fix — surface it as-is instead of the misleading login-required message.
		return authErr
	}

	// See startStdio: arm only after the initial auth succeeds so the startup
	// probe's expected AUTH_KEY_UNREGISTERED does not trip the revocation guard.
	health.Arm()
	close(authDone)

	logger.Info("starting in HTTP-only headless mode (shared daemon)")

	return runHTTPServer(ctx, server, cfg.HTTPAddr(), logger)
}

// newHeadlessServer builds the MCP server for headless HTTP-only mode. It is a
// named seam so the daemon and its tests construct the server identically — in
// particular the nil init hook is owned here, not duplicated per call site.
//
// onInit must stay nil: headless HTTP serves many clients, and a hook that
// closes a shared channel would panic on the second client's initialize (close
// of a closed channel). Wiring such a hook here trips the multi-client test.
func newHeadlessServer(
	client tgclient.Client, downloadDir string, broker *tgclient.SubscriptionBroker,
	authDone chan struct{}, health *mcpmw.SessionHealth, logger *slog.Logger,
) *mcp.Server {
	return buildServer(client, downloadDir, broker, authDone, health, nil, logger)
}

// buildServer constructs the MCP server with all tools, resources, prompts, and
// middleware. onInit, when non-nil, runs after a client completes the MCP
// initialize handshake; pass nil when no single client owns the lifecycle.
func buildServer(
	client tgclient.Client, downloadDir string, broker *tgclient.SubscriptionBroker,
	authDone chan struct{}, health *mcpmw.SessionHealth, onInit func(), logger *slog.Logger,
) *mcp.Server {
	opts := newServerOptions(client, broker, logger)
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

	// Wire the notifier now that the server exists: the update dispatcher was
	// registered before Run, but ResourceUpdated needs the built server.
	broker.SetNotifier(newResourceUpdater(server))

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

// headlessLoginRequired turns a headless startup auth failure into an
// actionable message. The underlying gotd error is typically "TELEGRAM_PHONE is
// required", which misleads toward setting an env var; the real fix is an
// interactive login the headless daemon cannot perform itself.
func headlessLoginRequired(cause error) error {
	return errors.Wrap(cause,
		"no valid Telegram session and the headless daemon cannot log in by itself — "+
			"run `mcp-tg login` in a terminal (outside any MCP client), then restart the daemon")
}

// loginWouldFix reports whether a headless startup auth failure is one that
// `mcp-tg login` can actually resolve — a missing session (the authenticator
// asks for credentials none of which are configured) or a revoked auth key from
// the known-fixable set. It deliberately does not treat every 401 as fixable:
// terminal account states (USER_DEACTIVATED / _BAN) are also 401 but re-login
// cannot fix them, so — like authRevokedCodes — they must surface unchanged
// rather than as misleading "run mcp-tg login" guidance. Transient failures
// (network, 5xx, DC migration) likewise surface as-is.
func loginWouldFix(err error) bool {
	if _, revoked := revokedCode(err); revoked {
		return true
	}

	return errors.Is(err, tgclient.ErrPhoneRequired) ||
		errors.Is(err, tgclient.ErrPasswordRequired) ||
		errors.Is(err, tgclient.ErrNoAuthCode) ||
		errors.Is(err, tgclient.ErrElicitDeclined)
}

// revokedExitError wraps the error tgClient.Run returns when the connection ends.
// Some revoked-session codes (notably AUTH_KEY_DUPLICATED) are classified by gotd
// as permanent *connection* errors, so they tear down the client instead of
// surfacing through the invoker middleware — the daemon exits here rather than
// staying up and fast-failing tool calls. Point the operator at `mcp-tg login`
// with the same guidance as the invoker path; otherwise fall back to the generic
// stop message. A nil error (clean shutdown) stays nil.
func revokedExitError(err error) error {
	if code, ok := revokedCode(err); ok {
		return errors.Wrapf(err,
			"Telegram session revoked (%s) — the daemon cannot recover on its own; "+
				"run `mcp-tg login` in a terminal, then restart the daemon", code)
	}

	return errors.Wrap(err, "telegram client stopped")
}

func waitForTransports(
	ctx context.Context,
	cancel context.CancelFunc,
	server *mcp.Server,
	stdioSession *mcp.ServerSession,
	cfg *config.Config,
	logger *slog.Logger,
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
			return runHTTPServer(groupCtx, server, cfg.HTTPAddr(), logger)
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

// ensureFileStorageDir prepares the session-file directory, but only for
// insecure/file storage. Keychain mode writes no file — TELEGRAM_SESSION_FILE is
// just the keychain account key there — so it must not create or require any
// filesystem path.
func ensureFileStorageDir(cfg *config.Config, insecure bool) error {
	if !insecure {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(cfg.SessionFile), 0o700); err != nil {
		return errors.Wrap(err, "creating session directory")
	}

	ensureSessionPerms(cfg.SessionFile)

	return nil
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
// invoker middlewares, at the level resolved from --log-level / MCP_LOG_LEVEL.
// Text handler to stderr, which launchd routes into the daemon log. One logger
// is threaded through the whole server so the configured level applies
// everywhere, rather than a second logger silently pinning info.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// zapLevel maps a slog.Level onto the nearest zapcore.Level so the gotd zap
// logger honours the same --log-level / MCP_LOG_LEVEL as the slog logger.
func zapLevel(level slog.Level) zapcore.Level {
	switch {
	case level <= slog.LevelDebug:
		return zapcore.DebugLevel
	case level <= slog.LevelInfo:
		return zapcore.InfoLevel
	case level <= slog.LevelWarn:
		return zapcore.WarnLevel
	default:
		return zapcore.ErrorLevel
	}
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
//
// It honours the same resolved level as the slog logger, so --log-level debug
// turns on the full gotd connection trace and warn/error quiet gotd's info
// chatter — otherwise the level would apply to the slog side only.
func newGotdLogger(level slog.Level) *zap.Logger {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.Level = zap.NewAtomicLevelAt(zapLevel(level))

	// This is a diagnostic logger whose whole point is to preserve the context of
	// a connection/auth incident. Production sampling would thin out repeated
	// lines (e.g. "Restarting connection") during exactly the reconnect storm we
	// want fully logged, so disable it.
	cfg.Sampling = nil

	logger, err := cfg.Build()
	if err != nil {
		return zap.NewNop()
	}

	return logger
}

// mcpDevice is the client identity sent to Telegram in initConnection and shown
// in the account's Devices list. Without it gotd fills defaults that read as
// "go1.26.4" (the Go version, as device model) and gotd's own version — this
// names the client mcp-tg instead. SetDefaults only fills empty fields, so the
// values set here are preserved and the language codes are filled in.
func mcpDevice() telegram.DeviceConfig {
	device := telegram.DeviceConfig{
		DeviceModel:   serverName,
		SystemVersion: runtime.GOOS + "/" + runtime.GOARCH,
		AppVersion:    version + "+" + shortRevision(revision),
	}
	device.SetDefaults()

	return device
}

// shortRevisionLen bounds the git SHA shown in the Devices list; a release
// revision is a full 40-char SHA, which is noise in that UI.
const shortRevisionLen = 8

func shortRevision(revision string) string {
	if len(revision) > shortRevisionLen {
		return revision[:shortRevisionLen]
	}

	return revision
}

func newServerOptions(
	client tgclient.Client, broker *tgclient.SubscriptionBroker, logger *slog.Logger,
) *mcp.ServerOptions {
	return &mcp.ServerOptions{
		Instructions: "MCP server for Telegram Client API (MTProto, user account, not bot). " +
			"All tools accepting 'peer' support: @username, bare username, " +
			"https://t.me/username, t.me/+invite_hash, or numeric bot-API style ID " +
			"(positive=user, negative=chat, -100xxx=channel). Prefer @username over numeric IDs. " +
			"Tools with 'limit' accept pagination: use offsetId or offsetDate from previous results; " +
			"tg_messages_search_global pages through a compound cursor — copy the result's " +
			"nextRate/nextOffsetId/nextOffsetPeer back as offsetRate/offsetId/offsetPeer verbatim. " +
			"Text tools require parseMode ('plain' or 'commonmark'); results report entitiesParsed, " +
			"the formatting-entity count of the sent message (auto-detected links and hashtags excluded) — " +
			"0 after a commonmark send whose text CONTAINED formatting means the markdown did not parse. " +
			"Read-only tools are safe to call freely. Write/destructive tools modify Telegram state.",
		Logger:             logger,
		KeepAlive:          keepAliveInterval,
		CompletionHandler:  completions.NewHandler(client),
		SubscribeHandler:   newSubscribeHandler(client, broker),
		UnsubscribeHandler: newUnsubscribeHandler(broker),
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
	tools.AddTool(server, registry, tools.MessagesTranscribeAudioTool(), tools.NewMessagesTranscribeAudioHandler(client))

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
	tools.AddTool(server, registry, tools.ChatsGetSendAsTool(), tools.NewChatsGetSendAsHandler(client))
	tools.AddTool(server, registry, tools.ChatsSetSendAsTool(), tools.NewChatsSetSendAsHandler(client))
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

func runHTTPServer(ctx context.Context, server *mcp.Server, addr string, logger *slog.Logger) error {
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
			logger.Error("HTTP server shutdown failed", "error", shutdownErr)
		}
	}()

	listener, listenErr := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if listenErr != nil {
		return errors.Wrapf(listenErr, "HTTP port %s unavailable", addr)
	}

	logger.Info("HTTP server listening", "addr", addr)

	serveErr := httpServer.Serve(listener)
	if errors.Is(serveErr, http.ErrServerClosed) {
		return nil
	}

	return errors.Wrap(serveErr, "HTTP serve failed")
}

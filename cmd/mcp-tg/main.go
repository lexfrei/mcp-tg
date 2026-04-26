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
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
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

	tgClient := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.SessionFile},
		Middlewares:    []telegram.Middleware{newFloodWaitMiddleware()},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandler(ctx, cancel)

	return errors.Wrap(tgClient.Run(ctx, func(ctx context.Context) error {
		return startServer(ctx, cancel, tgClient, cfg)
	}), "telegram client stopped")
}

func startServer(
	ctx context.Context, cancel context.CancelFunc, tgClient *telegram.Client, cfg *config.Config,
) error {
	wrapper := tgclient.NewWrapper(tgClient.API())

	initDone := make(chan struct{})
	authDone := make(chan struct{})
	opts := newServerOptions(wrapper)
	opts.InitializedHandler = func(_ context.Context, _ *mcp.InitializedRequest) {
		close(initDone)
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: version + "+" + revision,
		},
		opts,
	)

	boolFields := tools.BoolFieldRegistry{}
	registerTools(server, wrapper, boolFields, cfg.DownloadDir)
	resources.Register(server, wrapper)
	prompts.Register(server, wrapper)
	server.AddReceivingMiddleware(
		mcpmw.NewBoolCoercer(boolFields),
		mcpmw.NewAuthGuard(authDone),
		mcpmw.NewLogging(opts.Logger),
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

	authenticator := tgclient.NewAuthenticator(cfg.Phone, cfg.Password, cfg.AuthCode)
	authenticator.SetSession(stdioSession)

	flow := auth.NewFlow(authenticator, auth.SendCodeOptions{})

	authErr := tgClient.Auth().IfNecessary(ctx, flow)
	if authErr != nil {
		return errors.Wrap(authErr, "authentication failed")
	}

	close(authDone)

	return waitForTransports(ctx, cancel, server, stdioSession, cfg)
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

func newServerOptions(client tgclient.Client) *mcp.ServerOptions {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

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

func runHTTPServer(ctx context.Context, server *mcp.Server, addr string) error {
	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server {
			return server
		},
		nil,
	)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
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

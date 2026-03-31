package main

import (
	"context"
	"log"
	"log/slog"
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

	"github.com/lexfrei/mcp-tg/internal/config"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	serverName        = "mcp-tg"
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
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

	sessionDir := filepath.Dir(cfg.SessionFile)

	mkdirErr := os.MkdirAll(sessionDir, 0o700)
	if mkdirErr != nil {
		return errors.Wrap(mkdirErr, "creating session directory")
	}

	tgClient := telegram.NewClient(cfg.AppID, cfg.AppHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.SessionFile},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	return errors.Wrap(tgClient.Run(ctx, func(ctx context.Context) error {
		authenticator := tgclient.NewEnvCodeAuthenticator(cfg.Phone, cfg.Password, cfg.AuthCode)
		flow := auth.NewFlow(authenticator, auth.SendCodeOptions{})

		authErr := tgClient.Auth().IfNecessary(ctx, flow)
		if authErr != nil {
			return errors.Wrap(authErr, "authentication failed")
		}

		wrapper := tgclient.NewWrapper(tgClient.API())

		serverOpts := newServerOptions()
		server := mcp.NewServer(
			&mcp.Implementation{
				Name:    serverName,
				Version: version + "+" + revision,
			},
			serverOpts,
		)

		registerTools(server, wrapper)

		return runTransports(ctx, cancel, server, cfg)
	}), "telegram client stopped")
}

func newServerOptions() *mcp.ServerOptions {
	return &mcp.ServerOptions{
		Instructions: "MCP server for Telegram Client API. " +
			"Provides tools to manage messages, dialogs, contacts, groups, " +
			"channels, stickers, folders, and user profile. " +
			"Uses MTProto protocol via user account (not bot). " +
			"Requires TELEGRAM_APP_ID and TELEGRAM_APP_HASH from my.telegram.org.",
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}
}

func registerTools(server *mcp.Server, client tgclient.Client) {
	mcp.AddTool(server, tools.ProfileGetTool(), tools.NewProfileGetHandler(client))
	mcp.AddTool(server, tools.DialogsListTool(), tools.NewDialogsListHandler(client))
	mcp.AddTool(server, tools.DialogsSearchTool(), tools.NewDialogsSearchHandler(client))
	mcp.AddTool(server, tools.DialogsGetInfoTool(), tools.NewDialogsGetInfoHandler(client))
	mcp.AddTool(server, tools.MessagesListTool(), tools.NewMessagesListHandler(client))
	mcp.AddTool(server, tools.MessagesGetTool(), tools.NewMessagesGetHandler(client))
	mcp.AddTool(server, tools.MessagesContextTool(), tools.NewMessagesContextHandler(client))
	mcp.AddTool(server, tools.MessagesSearchTool(), tools.NewMessagesSearchHandler(client))
	mcp.AddTool(server, tools.MessagesSendTool(), tools.NewMessagesSendHandler(client))
	mcp.AddTool(server, tools.MessagesEditTool(), tools.NewMessagesEditHandler(client))
	mcp.AddTool(server, tools.MessagesDeleteTool(), tools.NewMessagesDeleteHandler(client))
	mcp.AddTool(server, tools.MessagesForwardTool(), tools.NewMessagesForwardHandler(client))
	mcp.AddTool(server, tools.MessagesPinTool(), tools.NewMessagesPinHandler(client))
	mcp.AddTool(server, tools.MessagesReactTool(), tools.NewMessagesReactHandler(client))
	mcp.AddTool(server, tools.MessagesMarkReadTool(), tools.NewMessagesMarkReadHandler(client))

	// Phase 2: Contacts, Users, Groups, Chat management tools.
	mcp.AddTool(server, tools.ContactsGetTool(), tools.NewContactsGetHandler(client))
	mcp.AddTool(server, tools.ContactsSearchTool(), tools.NewContactsSearchHandler(client))
	mcp.AddTool(server, tools.UsersGetTool(), tools.NewUsersGetHandler(client))
	mcp.AddTool(server, tools.UsersPhotosTool(), tools.NewUsersPhotosHandler(client))
	mcp.AddTool(server, tools.UsersBlockTool(), tools.NewUsersBlockHandler(client))
	mcp.AddTool(server, tools.UsersCommonChatsTool(), tools.NewUsersCommonChatsHandler(client))
	mcp.AddTool(server, tools.GroupsListTool(), tools.NewGroupsListHandler(client))
	mcp.AddTool(server, tools.GroupsInfoTool(), tools.NewGroupsInfoHandler(client))
	mcp.AddTool(server, tools.GroupsJoinTool(), tools.NewGroupsJoinHandler(client))
	mcp.AddTool(server, tools.GroupsLeaveTool(), tools.NewGroupsLeaveHandler(client))
	mcp.AddTool(server, tools.GroupsRenameTool(), tools.NewGroupsRenameHandler(client))
	mcp.AddTool(server, tools.GroupsMembersAddTool(), tools.NewGroupsMembersAddHandler(client))
	mcp.AddTool(server, tools.GroupsMembersRemoveTool(), tools.NewGroupsMembersRemoveHandler(client))
	mcp.AddTool(server, tools.GroupsInviteLinkGetTool(), tools.NewGroupsInviteLinkGetHandler(client))
	mcp.AddTool(server, tools.GroupsInviteLinkRevokeTool(), tools.NewGroupsInviteLinkRevokeHandler(client))
	mcp.AddTool(server, tools.ChatsAdminsTool(), tools.NewChatsAdminsHandler(client))
	mcp.AddTool(server, tools.ChatsPermissionsTool(), tools.NewChatsPermissionsHandler(client))

	// Phase 3: Media, Files, Chat Management, Profile tools.
	mcp.AddTool(server, tools.MessagesSendFileTool(), tools.NewMessagesSendFileHandler(client))
	mcp.AddTool(server, tools.MediaDownloadTool(), tools.NewMediaDownloadHandler(client))
	mcp.AddTool(server, tools.MediaUploadTool(), tools.NewMediaUploadHandler(client))
	mcp.AddTool(server, tools.MediaSendAlbumTool(), tools.NewMediaSendAlbumHandler(client))
	mcp.AddTool(server, tools.ChatsCreateTool(), tools.NewChatsCreateHandler(client))
	mcp.AddTool(server, tools.ChatsArchiveTool(), tools.NewChatsArchiveHandler(client))
	mcp.AddTool(server, tools.ChatsMuteTool(), tools.NewChatsMuteHandler(client))
	mcp.AddTool(server, tools.ChatsDeleteTool(), tools.NewChatsDeleteHandler(client))
	mcp.AddTool(server, tools.ChatsSetPhotoTool(), tools.NewChatsSetPhotoHandler(client))
	mcp.AddTool(server, tools.ChatsSetDescriptionTool(), tools.NewChatsSetDescriptionHandler(client))
	mcp.AddTool(server, tools.ProfileSetNameTool(), tools.NewProfileSetNameHandler(client))
	mcp.AddTool(server, tools.ProfileSetBioTool(), tools.NewProfileSetBioHandler(client))
	mcp.AddTool(server, tools.ProfileSetPhotoTool(), tools.NewProfileSetPhotoHandler(client))
}

func runTransports(ctx context.Context, cancel context.CancelFunc, server *mcp.Server, cfg *config.Config) error {
	group, groupCtx := errgroup.WithContext(ctx)
	httpEnabled := cfg.HTTPEnabled()

	group.Go(func() error {
		runErr := server.Run(groupCtx, &mcp.StdioTransport{})
		if runErr != nil && groupCtx.Err() == nil {
			return errors.Wrap(runErr, "stdio server failed")
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

	log.Printf("HTTP server listening on %s", addr)

	listenErr := httpServer.ListenAndServe()
	if errors.Is(listenErr, http.ErrServerClosed) {
		return nil
	}

	return errors.Wrap(listenErr, "HTTP listen failed")
}

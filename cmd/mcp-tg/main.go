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
		// Order is outermost first, and the nesting decides whose budget
		// multiplies whose. With FLOOD_WAIT outermost a server-named delay is
		// slept once per flood attempt, with the fast 500 cycle nested inside
		// it; reversed, that delay would be slept inside every server-error
		// attempt instead. A 420 passes straight through the server-error
		// middleware either way, so the order is about the sleeping, not about
		// honouring the delay the server asked for.
		Middlewares: []telegram.Middleware{
			newFloodWaitMiddleware(logger),
			newServerErrorMiddleware(logger, serverErrorBaseDelay),
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
// The login prompts through the stdio session, so this mode can complete an
// interactive login: by elicitation on clients before 2026-07-28, and by input
// requests returned from the tool call on later ones.
func startStdio(
	ctx context.Context, cancel context.CancelFunc, tgClient *telegram.Client,
	wrapper tgclient.Client, subscriptionBroker *tgclient.SubscriptionBroker,
	cfg *config.Config, health *mcpmw.SessionHealth, logger *slog.Logger,
) error {
	authDone := make(chan struct{})
	login := newLogin(tgClient, cfg, health, authDone, logger)

	server := buildServer(
		wrapper, cfg.DownloadDir, cfg.FileRoots, subscriptionBroker, authDone, health, login, logger,
	)

	stdioSession, err := server.Connect(ctx, &mcp.StdioTransport{}, nil)
	if err != nil {
		return errors.Wrap(err, "connecting stdio transport")
	}

	// Only now: a saved session needs no client, but reaching Telegram can take
	// a while — auth.sendCode is a favourite of FLOOD_WAIT, and the retry
	// middleware sleeps whatever delay the server names. Connecting first keeps
	// the client answered and the tool list readable throughout, instead of
	// leaving its initialize unanswered with nothing to report.
	if err := settleLogin(ctx, login, logger); err != nil {
		return err
	}

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
	login := newLogin(tgClient, cfg, health, authDone, logger)
	server := newHeadlessServer(wrapper, cfg.DownloadDir, cfg.FileRoots, subscriptionBroker, authDone, health, logger)

	authErr := login.Probe(ctx)
	if authErr == nil && !login.Done() {
		authErr = login.RunUnattended(ctx)
	}

	if authErr != nil {
		if loginWouldFix(authErr) {
			return headlessLoginRequired(authErr)
		}

		// A transient failure (network, 5xx, DC migration) that re-login cannot
		// fix — surface it as-is instead of the misleading login-required message.
		return errors.Wrap(authErr, "logging in to Telegram")
	}

	logger.Info("starting in HTTP-only headless mode (shared daemon)")

	return runHTTPServer(ctx, server, cfg.HTTPAddr(), logger)
}

// newHeadlessServer builds the MCP server for headless HTTP-only mode. It is a
// named seam so the daemon and its tests construct the server identically.
func newHeadlessServer(
	client tgclient.Client, downloadDir string, fileRoots []string, broker *tgclient.SubscriptionBroker,
	authDone chan struct{}, health *mcpmw.SessionHealth, logger *slog.Logger,
) *mcp.Server {
	return buildServer(client, downloadDir, fileRoots, broker, authDone, health, nil, logger)
}

// buildServer constructs the MCP server with all tools, resources, prompts, and
// middleware. login, when non-nil, is offered to every Telegram tool so the
// first call that needs an account can log in; pass nil when there is no
// Telegram client to log into.
func buildServer(
	client tgclient.Client, downloadDir string, fileRoots []string, broker *tgclient.SubscriptionBroker,
	authDone chan struct{}, health *mcpmw.SessionHealth, login *tgclient.Login, logger *slog.Logger,
) *mcp.Server {
	opts := newServerOptions(client, broker, logger)

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
	registerTools(server, client, boolFields, tools.NewLoginGate(login), downloadDir, fileRoots)
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
		mcpmw.NewAuthGuard(authDone),
		mcpmw.NewSessionGuard(health, []string{tools.ServerVersionToolName}),
	}
}

// newLogin builds the login flow. Arming revocation tracking is deferred to
// the moment the account is authorized: the session probe answers
// AUTH_KEY_UNREGISTERED for a fresh or revoked session, and that expected
// pre-login 401 must not be mistaken for a revocation.
func newLogin(
	tgClient *telegram.Client, cfg *config.Config, health *mcpmw.SessionHealth,
	authDone chan struct{}, logger *slog.Logger,
) *tgclient.Login {
	credentials := tgclient.Credentials{Phone: cfg.Phone, Code: cfg.AuthCode, Password: cfg.Password}

	return tgclient.NewLogin(tgClient.Auth(), credentials, logger, func() {
		health.Arm()
		close(authDone)
	})
}

// settleLogin takes the login as far as it goes without a human. Missing
// input is not a failure here — a client can supply it on the first tool call
// — but anything else is, since it says nothing about whether a login would
// help.
func settleLogin(ctx context.Context, login *tgclient.Login, logger *slog.Logger) error {
	err := login.Probe(ctx)
	if err == nil && !login.Done() {
		err = login.RunUnattended(ctx)
	}

	switch {
	case err == nil:
		return nil
	case errors.Is(err, tgclient.ErrLoginInputRequired):
		logger.Info("no Telegram session — the first tool call will prompt for the login, " +
			"or run `mcp-tg login` in a terminal")

		return nil
	default:
		return errors.Wrap(err, "logging in to Telegram")
	}
}

// headlessLoginRequired turns a headless startup login failure into an
// actionable message. The cause names the credential that is missing, which
// reads like an invitation to set one more environment variable; for a daemon
// with no session the real fix is the interactive login it cannot perform
// itself.
func headlessLoginRequired(cause error) error {
	return errors.Wrap(cause,
		"no valid Telegram session and the headless daemon cannot log in by itself — "+
			"run `mcp-tg login` in a terminal (outside any MCP client), then restart the daemon")
}

// loginWouldFix reports whether a headless startup auth failure is one that
// `mcp-tg login` can actually resolve — a missing session (the login reached a
// step nothing configured could answer) or a revoked auth key from
// the known-fixable set. It deliberately does not treat every 401 as fixable:
// terminal account states (USER_DEACTIVATED / _BAN) are also 401 but re-login
// cannot fix them, so — like authRevokedCodes — they must surface unchanged
// rather than as misleading "run mcp-tg login" guidance. Transient failures
// (network, 5xx, DC migration) likewise surface as-is.
func loginWouldFix(err error) bool {
	if _, revoked := revokedCode(err); revoked {
		return true
	}

	return errors.Is(err, tgclient.ErrLoginInputRequired)
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
	}
}

func registerTools(
	server *mcp.Server, client tgclient.Client, registry tools.BoolFieldRegistry, gate *tools.LoginGate,
	downloadDir string, fileRoots []string,
) {
	tools.AddTool(server, registry, gate, tools.ServerVersionTool(),
		tools.NewServerVersionHandler(version, revision, runtime.Version()))
	tools.AddTool(server, registry, gate, tools.ProfileGetTool(), tools.NewProfileGetHandler(client))
	tools.AddTool(server, registry, gate, tools.DialogsListTool(), tools.NewDialogsListHandler(client))
	tools.AddTool(server, registry, gate, tools.DialogsSearchTool(), tools.NewDialogsSearchHandler(client))
	tools.AddTool(server, registry, gate, tools.DialogsGetInfoTool(), tools.NewDialogsGetInfoHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesListTool(), tools.NewMessagesListHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesGetTool(), tools.NewMessagesGetHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesContextTool(), tools.NewMessagesContextHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesSearchTool(), tools.NewMessagesSearchHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesSendTool(), tools.NewMessagesSendHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesEditTool(), tools.NewMessagesEditHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesDeleteTool(), tools.NewMessagesDeleteHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesForwardTool(), tools.NewMessagesForwardHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesPinTool(), tools.NewMessagesPinHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesReactTool(), tools.NewMessagesReactHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesMarkReadTool(), tools.NewMessagesMarkReadHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesTranscribeAudioTool(), tools.NewMessagesTranscribeAudioHandler(client))

	// Phase 2: Contacts, Users, Groups, Chat management tools.
	tools.AddTool(server, registry, gate, tools.ContactsGetTool(), tools.NewContactsGetHandler(client))
	tools.AddTool(server, registry, gate, tools.ContactsSearchTool(), tools.NewContactsSearchHandler(client))
	tools.AddTool(server, registry, gate, tools.UsersGetTool(), tools.NewUsersGetHandler(client))
	tools.AddTool(server, registry, gate, tools.UsersPhotosTool(), tools.NewUsersPhotosHandler(client))
	tools.AddTool(server, registry, gate, tools.UsersBlockTool(), tools.NewUsersBlockHandler(client))
	tools.AddTool(server, registry, gate, tools.UsersCommonChatsTool(), tools.NewUsersCommonChatsHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsListTool(), tools.NewGroupsListHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsInfoTool(), tools.NewGroupsInfoHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsJoinTool(), tools.NewGroupsJoinHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsLeaveTool(), tools.NewGroupsLeaveHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsRenameTool(), tools.NewGroupsRenameHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsMembersAddTool(), tools.NewGroupsMembersAddHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsMembersRemoveTool(), tools.NewGroupsMembersRemoveHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsInviteLinkGetTool(), tools.NewGroupsInviteLinkGetHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsInviteLinkRevokeTool(), tools.NewGroupsInviteLinkRevokeHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsAdminsTool(), tools.NewChatsAdminsHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsPermissionsTool(), tools.NewChatsPermissionsHandler(client))

	// Phase 3: Media, Files, Chat Management, Profile tools.
	tools.AddTool(server, registry, gate, tools.MessagesSendFileTool(), tools.NewMessagesSendFileHandler(client, fileRoots))
	tools.AddTool(server, registry, gate, tools.MediaDownloadTool(), tools.NewMediaDownloadHandler(client, downloadDir, fileRoots))
	tools.AddTool(server, registry, gate, tools.MediaUploadTool(), tools.NewMediaUploadHandler(client, fileRoots))
	tools.AddTool(server, registry, gate, tools.MediaSendAlbumTool(), tools.NewMediaSendAlbumHandler(client, fileRoots))
	tools.AddTool(server, registry, gate, tools.ChatsCreateTool(), tools.NewChatsCreateHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsArchiveTool(), tools.NewChatsArchiveHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsMuteTool(), tools.NewChatsMuteHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsDeleteTool(), tools.NewChatsDeleteHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsSetPhotoTool(), tools.NewChatsSetPhotoHandler(client, fileRoots))
	tools.AddTool(server, registry, gate, tools.ChatsSetDescriptionTool(), tools.NewChatsSetDescriptionHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsGetSendAsTool(), tools.NewChatsGetSendAsHandler(client))
	tools.AddTool(server, registry, gate, tools.ChatsSetSendAsTool(), tools.NewChatsSetSendAsHandler(client))
	tools.AddTool(server, registry, gate, tools.ProfileSetNameTool(), tools.NewProfileSetNameHandler(client))
	tools.AddTool(server, registry, gate, tools.ProfileSetBioTool(), tools.NewProfileSetBioHandler(client))
	tools.AddTool(server, registry, gate, tools.ProfileSetPhotoTool(), tools.NewProfileSetPhotoHandler(client, fileRoots))

	// Phase 4: Topics, Stickers, Drafts, Folders, Status tools.
	tools.AddTool(server, registry, gate, tools.TopicsListTool(), tools.NewTopicsListHandler(client))
	tools.AddTool(server, registry, gate, tools.TopicsSearchTool(), tools.NewTopicsSearchHandler(client))
	tools.AddTool(server, registry, gate, tools.StickersSearchTool(), tools.NewStickersSearchHandler(client))
	tools.AddTool(server, registry, gate, tools.StickersGetSetTool(), tools.NewStickersGetSetHandler(client))
	tools.AddTool(server, registry, gate, tools.StickersSendTool(), tools.NewStickersSendHandler(client))
	tools.AddTool(server, registry, gate, tools.DraftsSetTool(), tools.NewDraftsSetHandler(client))
	tools.AddTool(server, registry, gate, tools.DraftsClearTool(), tools.NewDraftsClearHandler(client))
	tools.AddTool(server, registry, gate, tools.FoldersListTool(), tools.NewFoldersListHandler(client))
	tools.AddTool(server, registry, gate, tools.FoldersCreateTool(), tools.NewFoldersCreateHandler(client))
	tools.AddTool(server, registry, gate, tools.FoldersEditTool(), tools.NewFoldersEditHandler(client))
	tools.AddTool(server, registry, gate, tools.FoldersDeleteTool(), tools.NewFoldersDeleteHandler(client))
	tools.AddTool(server, registry, gate, tools.TypingSendTool(), tools.NewTypingSendHandler(client))
	tools.AddTool(server, registry, gate, tools.OnlineStatusSetTool(), tools.NewOnlineStatusSetHandler(client))

	// Phase 5: Extended coverage tools.
	tools.AddTool(server, registry, gate, tools.MessagesGetScheduledTool(), tools.NewMessagesGetScheduledHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesSearchGlobalTool(), tools.NewMessagesSearchGlobalHandler(client))
	tools.AddTool(server, registry, gate, tools.ContactsListBlockedTool(), tools.NewContactsListBlockedHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesGetReactionsTool(), tools.NewMessagesGetReactionsHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsMembersListTool(), tools.NewGroupsMembersListHandler(client))
	tools.AddTool(server, registry, gate, tools.ContactsGetStatusesTool(), tools.NewContactsGetStatusesHandler(client))
	tools.AddTool(server, registry, gate, tools.DialogsPinTool(), tools.NewDialogsPinHandler(client))
	tools.AddTool(server, registry, gate, tools.DialogsMarkUnreadTool(), tools.NewDialogsMarkUnreadHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsSlowmodeTool(), tools.NewGroupsSlowmodeHandler(client))
	tools.AddTool(server, registry, gate, tools.TopicsCreateTool(), tools.NewTopicsCreateHandler(client))
	tools.AddTool(server, registry, gate, tools.TopicsEditTool(), tools.NewTopicsEditHandler(client))
	tools.AddTool(server, registry, gate, tools.ContactsAddTool(), tools.NewContactsAddHandler(client))
	tools.AddTool(server, registry, gate, tools.GroupsAdminSetTool(), tools.NewGroupsAdminSetHandler(client))
	tools.AddTool(server, registry, gate, tools.ContactsDeleteTool(), tools.NewContactsDeleteHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesDeleteHistoryTool(), tools.NewMessagesDeleteHistoryHandler(client))
	tools.AddTool(server, registry, gate, tools.MessagesClearAllDraftsTool(), tools.NewMessagesClearAllDraftsHandler(client))
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

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

	return tgClient.Run(ctx, func(ctx context.Context) error {
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
	})
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
	_ = server
	_ = client
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

	return group.Wait()
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

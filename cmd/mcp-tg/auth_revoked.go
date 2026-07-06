package main

import (
	"context"
	"log/slog"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/lexfrei/mcp-tg/internal/middleware"
)

// codeAuthKeyUnregistered is the canonical revoked-session error Telegram
// returns once an MTProto auth key is no longer registered on the server.
const codeAuthKeyUnregistered = "AUTH_KEY_UNREGISTERED"

// authRevokedCodes are the MTProto errors that mean the session's auth key is
// gone for good: the daemon cannot recover in headless mode, only a fresh
// interactive login restores service. Kept broad on purpose — every code here
// warrants the same "re-login required" operator signal.
var authRevokedCodes = []string{
	codeAuthKeyUnregistered,
	"AUTH_KEY_INVALID",
	"AUTH_KEY_PERM_EMPTY",
	"SESSION_REVOKED",
	"SESSION_EXPIRED",
	"USER_DEACTIVATED",
	"USER_DEACTIVATED_BAN",
}

// newAuthRevokedMiddleware watches every Telegram API call for a revoked-session
// error and records it in health. On the first occurrence it logs one ERROR so
// the operator sees a single clear signal instead of a stream of raw
// AUTH_KEY_UNREGISTERED failures, and the MCP session guard can then fast-fail
// tool calls with an explanation. The original error is passed through
// unchanged, mirroring the other invoker middlewares (flood_wait, conn_reinit).
func newAuthRevokedMiddleware(health *middleware.SessionHealth, logger *slog.Logger) telegram.MiddlewareFunc {
	return func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			err := next.Invoke(ctx, input, output)
			if err == nil {
				return nil
			}

			if code, ok := revokedCode(err); ok && health.MarkRevoked(code) {
				logger.Error("Telegram session revoked — re-login required (run: mcp-tg login)", "code", code)
			}

			return err //nolint:wrapcheck // pass-through: middleware must return the original API error.
		}
	}
}

// revokedCode reports the first authRevokedCodes entry the error matches.
func revokedCode(err error) (string, bool) {
	for _, code := range authRevokedCodes {
		if tgerr.Is(err, code) {
			return code, true
		}
	}

	return "", false
}

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
// gone but a fresh interactive login restores service — exactly the cases where
// the "re-login required" signal is correct. Account-level terminal states
// (USER_DEACTIVATED / USER_DEACTIVATED_BAN) are deliberately excluded: re-login
// cannot fix a deactivated or banned account, so pointing the operator at
// `mcp-tg login` there would mislead — those surface as their raw error instead.
//
// The list drives two detection paths, because gotd surfaces these codes in two
// different ways: the invoker middleware (newAuthRevokedMiddleware) catches the
// ones returned as an RPC error on a live connection (AUTH_KEY_UNREGISTERED),
// and revokedExitError catches the ones gotd classifies as a permanent
// *connection* error that ends tgClient.Run (AUTH_KEY_DUPLICATED and friends —
// see gotd telegram.Client.isPermanentError). A given deployment may hit either.
var authRevokedCodes = []string{
	codeAuthKeyUnregistered,
	"AUTH_KEY_INVALID",
	// same key from two places; gotd makes this a permanent connection error, so revokedExitError catches it (not the invoker)
	"AUTH_KEY_DUPLICATED",
	"AUTH_KEY_PERM_EMPTY",
	"SESSION_REVOKED",
	"SESSION_EXPIRED",
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

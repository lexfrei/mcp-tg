package middleware

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrSessionRevoked is returned for tool/resource/prompt calls once the Telegram
// session's auth key has been revoked server-side (AUTH_KEY_UNREGISTERED and
// friends). In headless mode the daemon cannot recover on its own: it must be
// re-logged-in in stdio mode. Surfacing one explicit error beats forwarding a
// raw MTProto 401 from every tool.
var ErrSessionRevoked = errors.New(
	"Telegram session is no longer authorized (auth key revoked) — " +
		"re-login with `mcp-tg login`; see README → Recovery",
)

// NewSessionGuard returns a middleware that fast-fails tool/resource/prompt
// calls with ErrSessionRevoked once health reports the session revoked, instead
// of forwarding to a handler that would emit a raw AUTH_KEY_UNREGISTERED per
// call. It shares requiresAuth/isBypassed with NewAuthGuard: protocol methods
// and *_list calls always pass through, and bypassTools (server-meta tools that
// never touch Telegram, e.g. build version) stay reachable so an operator can
// still probe the daemon while it is locked out.
func NewSessionGuard(health *SessionHealth, bypassTools []string) mcp.Middleware {
	bypass := make(map[string]struct{}, len(bypassTools))
	for _, name := range bypassTools {
		bypass[name] = struct{}{}
	}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if health.Revoked() && requiresAuth(method) && !isBypassed(method, req, bypass) {
				return nil, ErrSessionRevoked
			}

			return next(ctx, method, req)
		}
	}
}

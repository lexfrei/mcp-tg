package middleware

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNotAuthenticated is returned when a tool is called before authentication completes.
var ErrNotAuthenticated = errors.New("server is still authenticating with Telegram, please retry shortly")

// NewAuthGuard returns a middleware that blocks tool/resource calls until
// the provided channel is closed (signaling authentication is complete).
// Protocol methods (initialize, ping, etc.) are always allowed through.
func NewAuthGuard(authDone <-chan struct{}) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if requiresAuth(method) {
				select {
				case <-authDone:
				default:
					return nil, ErrNotAuthenticated
				}
			}

			return next(ctx, method, req)
		}
	}
}

func requiresAuth(method string) bool {
	return strings.HasPrefix(method, "tools/") ||
		strings.HasPrefix(method, "resources/") ||
		strings.HasPrefix(method, "prompts/")
}

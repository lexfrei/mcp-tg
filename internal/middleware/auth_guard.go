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
//
// bypassTools is an allowlist of tool names that may be invoked even before
// authentication completes — for tools that read only server-internal state
// (build metadata, etc.) and never touch the Telegram API.
func NewAuthGuard(authDone <-chan struct{}, bypassTools []string) mcp.Middleware {
	bypass := make(map[string]struct{}, len(bypassTools))
	for _, name := range bypassTools {
		bypass[name] = struct{}{}
	}

	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if requiresAuth(method) && !isBypassed(method, req, bypass) {
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
	// Allow listing tools/resources/prompts before auth — clients need
	// the catalog at connect time, and these don't call Telegram API.
	if strings.HasSuffix(method, "/list") {
		return false
	}

	return strings.HasPrefix(method, "tools/") ||
		strings.HasPrefix(method, "resources/") ||
		strings.HasPrefix(method, "prompts/")
}

// isBypassed reports whether a tools/call request targets a tool in the
// allowlist. Returns false for any non-tools/call method, malformed
// requests, or tools not in the allowlist.
func isBypassed(method string, req mcp.Request, bypass map[string]struct{}) bool {
	if method != methodCallTool || len(bypass) == 0 || req == nil {
		return false
	}

	call, ok := req.(*mcp.CallToolRequest)
	if !ok || call == nil || call.Params == nil {
		return false
	}

	_, exempt := bypass[call.Params.Name]

	return exempt
}

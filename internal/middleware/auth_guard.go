package middleware

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrNotAuthenticated is returned for a resource or prompt requested before
// the Telegram login completes. It names the way out, because nothing else
// will: a tool call logs in, and a terminal login always works.
var ErrNotAuthenticated = errors.New(
	"not logged in to Telegram yet — call any Telegram tool to log in " +
		"(the client will prompt for the phone and code), or run `mcp-tg login` in a terminal")

// NewAuthGuard returns a middleware that holds back resource reads and prompt
// expansions until the provided channel is closed, signalling that the
// Telegram login finished. Protocol methods (initialize, ping, etc.) are
// always allowed through.
//
// Tool calls are NOT held back: the login itself runs inside a tool call, so
// blocking them here would leave no way to log in. Tools that need an account
// go through the login gate instead, which prompts and then runs them.
func NewAuthGuard(authDone <-chan struct{}) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != methodCallTool && requiresAuth(method) {
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

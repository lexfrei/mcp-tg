// Package middleware provides MCP middleware for the mcp-tg server.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewLogging returns a receiving middleware that logs all incoming MCP requests.
func NewLogging(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()

			result, err := next(ctx, method, req)

			duration := time.Since(start)

			if err != nil {
				logger.ErrorContext(ctx, "MCP request failed",
					slog.String("method", method),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
			} else {
				logger.InfoContext(ctx, "MCP request handled",
					slog.String("method", method),
					slog.Duration("duration", duration),
				)
			}

			return result, err
		}
	}
}

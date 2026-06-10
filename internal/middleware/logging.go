// Package middleware provides MCP middleware for the mcp-tg server.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewLogging returns a receiving middleware that logs all incoming MCP
// requests. Tool handler errors arrive as CallToolResult{IsError: true} with a
// nil method-handler error (the SDK wraps them per the MCP spec), so the
// middleware inspects the result too — otherwise failed tool calls are
// indistinguishable from successful ones in the log.
func NewLogging(logger *slog.Logger) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()

			result, err := next(ctx, method, req)

			attrs := []slog.Attr{
				slog.String("method", method),
				slog.Duration("duration", time.Since(start)),
			}
			if name := toolName(req); name != "" {
				attrs = append(attrs, slog.String("tool", name))
			}

			switch {
			case err != nil:
				attrs = append(attrs, slog.String("error", err.Error()))
				logger.LogAttrs(ctx, slog.LevelError, "MCP request failed", attrs...)
			case isErrorResult(result):
				attrs = append(attrs, slog.String("error", resultErrorText(result)))
				logger.LogAttrs(ctx, slog.LevelError, "MCP tool call failed", attrs...)
			default:
				logger.LogAttrs(ctx, slog.LevelInfo, "MCP request handled", attrs...)
			}

			return result, err
		}
	}
}

func toolName(req mcp.Request) string {
	call, ok := req.(*mcp.CallToolRequest)
	if !ok || call.Params == nil {
		return ""
	}

	return call.Params.Name
}

func isErrorResult(result mcp.Result) bool {
	res, ok := result.(*mcp.CallToolResult)

	return ok && res.IsError
}

func resultErrorText(result mcp.Result) string {
	res, ok := result.(*mcp.CallToolResult)
	if !ok {
		return ""
	}

	for _, content := range res.Content {
		if text, isText := content.(*mcp.TextContent); isText {
			return text.Text
		}
	}

	return "tool returned IsError without text content"
}

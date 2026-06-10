package main

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-tg/internal/tools"
)

// chainMiddlewares replicates the SDK's AddReceivingMiddleware semantics:
// the first middleware in the list is the outermost wrapper.
func chainMiddlewares(mws []mcp.Middleware, handler mcp.MethodHandler) mcp.MethodHandler {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}

	return handler
}

// Auth guard rejections happen before the tool handler runs; the logging
// middleware must wrap the guard (be listed first) or those failures never
// reach the request log.
func TestReceivingMiddlewares_LogsAuthGuardRejections(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	authDone := make(chan struct{}) // never closed: auth still pending

	handler := chainMiddlewares(
		receivingMiddlewares(logger, tools.BoolFieldRegistry{}, authDone),
		func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		},
	)

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tg_messages_send"}}

	_, err := handler(context.Background(), "tools/call", req)
	if err == nil {
		t.Fatal("expected auth guard to reject the call while auth is pending")
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("MCP request failed")) {
		t.Errorf("expected rejected call to be logged, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("tool=tg_messages_send")) {
		t.Errorf("expected tool name in log, got: %s", output)
	}
}

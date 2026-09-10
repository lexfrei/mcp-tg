package main

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-tg/internal/middleware"
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

// Auth guard rejections happen before the handler runs; the logging
// middleware must wrap the guard (be listed first) or those failures never
// reach the request log. A resource read is the subject because tool calls
// deliberately pass the guard now — they are how the login happens.
func TestReceivingMiddlewares_LogsAuthGuardRejections(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	authDone := make(chan struct{}) // never closed: login still pending

	handler := chainMiddlewares(
		receivingMiddlewares(logger, tools.BoolFieldRegistry{}, authDone, middleware.NewSessionHealth()),
		func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			return &mcp.ReadResourceResult{}, nil
		},
	)

	req := &mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{URI: "tg://chat/@example/messages"}}

	_, err := handler(context.Background(), "resources/read", req)
	if err == nil {
		t.Fatal("expected the auth guard to reject the read while the login is pending")
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("MCP request failed")) {
		t.Errorf("expected the rejected read to be logged, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("method=resources/read")) {
		t.Errorf("expected the method in the log, got: %s", output)
	}
}

// A tool call while the login is pending must reach the handler: the login
// gate lives there, and the guard blocking it would leave no way in.
func TestReceivingMiddlewares_PassToolCallsWhileLoginPending(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	authDone := make(chan struct{}) // never closed

	reached := false
	handler := chainMiddlewares(
		receivingMiddlewares(logger, tools.BoolFieldRegistry{}, authDone, middleware.NewSessionHealth()),
		func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			reached = true

			return &mcp.CallToolResult{}, nil
		},
	)

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tg_messages_send"}}

	if _, err := handler(context.Background(), "tools/call", req); err != nil {
		t.Fatalf("tool call must pass the guard while the login is pending: %v", err)
	}

	if !reached {
		t.Error("the handler must have run")
	}
}

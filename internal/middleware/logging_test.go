package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNewLogging_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	noopResult := &mcp.CallToolResult{}
	handler := NewLogging(logger)(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return noopResult, nil
	})

	_, err := handler(context.Background(), "tools/call", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected log output, got empty")
	}

	if !bytes.Contains(buf.Bytes(), []byte("MCP request handled")) {
		t.Errorf("expected 'MCP request handled' in log, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("tools/call")) {
		t.Errorf("expected method name in log, got: %s", output)
	}
}

func TestNewLogging_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	testErr := errors.New("test failure")

	handler := NewLogging(logger)(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, testErr
	})

	_, err := handler(context.Background(), "tools/call", nil)
	if err == nil {
		t.Fatal("expected error to propagate")
	}

	if !bytes.Contains(buf.Bytes(), []byte("MCP request failed")) {
		t.Errorf("expected 'MCP request failed' in log, got: %s", buf.String())
	}
}

func TestNewLogging_ToolName(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	handler := NewLogging(logger)(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tg_messages_send"}}

	_, err := handler(context.Background(), "tools/call", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("tool=tg_messages_send")) {
		t.Errorf("expected tool name in log, got: %s", buf.String())
	}
}

// Tool handler errors come back as CallToolResult{IsError: true} with a nil
// method-handler error (the SDK wraps them per the MCP spec), so the
// middleware must inspect the result to surface them.
func TestNewLogging_ToolResultError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	failedResult := &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: "rpc error code 400: CONNECTION_LAYER_INVALID"}},
	}
	handler := NewLogging(logger)(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return failedResult, nil
	})

	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tg_users_get"}}

	_, err := handler(context.Background(), "tools/call", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("MCP tool call failed")) {
		t.Errorf("expected 'MCP tool call failed' in log, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("level=ERROR")) {
		t.Errorf("expected ERROR level in log, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("tool=tg_users_get")) {
		t.Errorf("expected tool name in log, got: %s", output)
	}

	if !bytes.Contains(buf.Bytes(), []byte("CONNECTION_LAYER_INVALID")) {
		t.Errorf("expected error text in log, got: %s", output)
	}
}

func TestNewLogging_ToolResultErrorWithoutText(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	handler := NewLogging(logger)(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{IsError: true}, nil
	})

	_, err := handler(context.Background(), "tools/call", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte("tool returned IsError without text content")) {
		t.Errorf("expected placeholder error text in log, got: %s", buf.String())
	}
}

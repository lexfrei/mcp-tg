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

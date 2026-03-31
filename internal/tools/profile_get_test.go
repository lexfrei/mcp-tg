package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProfileGetTool_Definition(t *testing.T) {
	tool := ProfileGetTool()
	if tool.Name == "" {
		t.Error("tool name must not be empty")
	}

	if tool.Description == "" {
		t.Error("tool description must not be empty")
	}
}

func TestProfileGetHandler_Success(t *testing.T) {
	mock := &mockClient{
		user: &telegram.User{
			ID:        12345,
			FirstName: "Pavel",
			LastName:  "Durov",
			Username:  "durov",
			Bio:       "CEO of Telegram",
		},
	}
	handler := NewProfileGetHandler(mock)

	result, structured, err := handler(context.Background(), nil, ProfileGetParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != nil && result.IsError {
		t.Error("result.IsError = true, want false")
	}

	if structured.UserID != 12345 {
		t.Errorf("UserID = %d, want 12345", structured.UserID)
	}

	if structured.Username != "durov" {
		t.Errorf("Username = %q, want %q", structured.Username, "durov")
	}
}

func TestProfileGetHandler_Error(t *testing.T) {
	mock := &mockClient{
		err: errors.New("connection failed"),
	}

	handler := NewProfileGetHandler(mock)
	result, _, err := handler(context.Background(), nil, ProfileGetParams{})

	if err == nil {
		t.Fatal("expected error")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true on error")
	}
}

// Verify handler signature matches MCP SDK expectations.
var _ mcp.ToolHandlerFor[ProfileGetParams, ProfileGetResult] = NewProfileGetHandler(nil)

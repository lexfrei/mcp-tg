package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestDialogsListTool_Definition(t *testing.T) {
	tool := DialogsListTool()
	if tool.Name == "" {
		t.Error("tool name must not be empty")
	}
}

func TestDialogsListHandler_Success(t *testing.T) {
	mock := &mockClient{
		dialogs: []telegram.Dialog{
			{Title: "Chat 1", IsGroup: true},
			{Title: "Chat 2", IsChannel: true},
		},
	}
	handler := NewDialogsListHandler(mock)

	_, structured, err := handler(context.Background(), nil, DialogsListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Count != 2 {
		t.Errorf("Count = %d, want 2", structured.Count)
	}
}

func TestDialogsListHandler_Error(t *testing.T) {
	mock := &mockClient{err: errors.New("fail")}
	handler := NewDialogsListHandler(mock)

	result, _, err := handler(context.Background(), nil, DialogsListParams{})
	if err == nil {
		t.Fatal("expected error")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

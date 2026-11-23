package tools

import (
	"context"
	"strings"
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

// The wrapper pages past the server's 100, so the cap bounds the round-trips
// one call makes. Both tools that page dialogs apply it.
func TestDialogsListHandlers_RejectALimitAboveTheCap(t *testing.T) {
	limit := maxDialogsLimit + 1

	_, _, err := NewDialogsListHandler(&mockClient{})(context.Background(), nil, DialogsListParams{Limit: &limit})
	if !errors.Is(err, ErrLimitTooLarge) {
		t.Errorf("tg_dialogs_list: err = %v, want ErrLimitTooLarge", err)
	}

	_, _, err = NewGroupsListHandler(&mockClient{})(context.Background(), nil, GroupsListParams{Limit: &limit})
	if !errors.Is(err, ErrLimitTooLarge) {
		t.Errorf("tg_groups_list: err = %v, want ErrLimitTooLarge", err)
	}
}

func TestDialogsListHandler_AcceptsTheCap(t *testing.T) {
	limit := maxDialogsLimit

	_, _, err := NewDialogsListHandler(&mockClient{})(context.Background(), nil, DialogsListParams{Limit: &limit})
	if err != nil {
		t.Errorf("limit %d: unexpected error %v", limit, err)
	}
}

// lastMessageDate is the cursor a caller pages with, so it must reach the item.
func TestDialogsListHandler_ReportsLastMessageDate(t *testing.T) {
	mock := &mockClient{dialogs: []telegram.Dialog{{Title: "Chat", LastMessageDate: 1700000000}}}

	_, structured, err := NewDialogsListHandler(mock)(context.Background(), nil, DialogsListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := structured.Dialogs[0].LastMessageDate; got != 1700000000 {
		t.Errorf("LastMessageDate = %d, want 1700000000", got)
	}
}

// A page that ends on a pinned chat yields an old cursor, so the description
// warns about it next to the paging instructions.
func TestDialogsListTool_WarnsAboutPinnedCursor(t *testing.T) {
	if desc := DialogsListTool().Description; !strings.Contains(desc, "Pinned dialogs come first") {
		t.Errorf("description does not warn about paging from a pinned dialog: %q", desc)
	}
}

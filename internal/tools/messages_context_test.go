package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// messages_context must fetch its window in a single server page: the
// window is symmetric by construction (OffsetID = messageId + radius),
// and letting the wrapper auto-paginate would extend it only on the
// older side and turn a large radius into many round-trips. This pins
// SinglePage so a refactor that drops it fails instead of silently
// breaking the "before and after" contract.
func TestMessagesContextHandler_UsesSinglePage(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: []telegram.Message{{ID: 5, Date: 1000, Text: "hi", Type: "text"}},
	}

	_, _, err := NewMessagesContextHandler(mock)(
		context.Background(), nil, MessagesContextParams{Peer: "@x", MessageID: 5},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.getHistoryOpts) == 0 {
		t.Fatal("expected a GetHistory call")
	}

	if !mock.getHistoryOpts[0].SinglePage {
		t.Error("messages_context must set SinglePage to keep the context window symmetric (one RPC)")
	}
}

func TestFormatContextMessages_TargetMarkerOnEveryLine(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 99, Date: 1700000000, Text: "earlier context"},
		{
			ID: 100, Date: 1700000001,
			FromID: 1, FromName: "Alice",
			Text: "first line of body\nsecond line of body\nthird",
		},
		{ID: 101, Date: 1700000002, Text: "later context"},
	}

	got := formatContextMessages(msgs, 100)

	// Every line of the target block (header, from:, text:, body lines)
	// must carry the "> " marker so the target stays anchored visually
	// once the body is more than a couple of lines.
	wantLines := []string{
		"> [100] 2023-11-14T22:13:21Z",
		"> from: Alice [user:1]",
		"> type: text",
		"> text:",
		"> first line of body",
		"> second line of body",
		"> third",
	}

	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Errorf("target block missing prefixed line %q in:\n%s", line, got)
		}
	}
}

func TestFormatContextMessages_NonTargetBlocksUnmarked(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 99, Date: 1700000000, Text: "earlier"},
		{ID: 100, Date: 1700000001, Text: "target"},
	}

	got := formatContextMessages(msgs, 100)

	// Non-target block must NOT be prefixed.
	if !strings.Contains(got, "[99] 2023-11-14T22:13:20Z\ntype: text\ntext:\nearlier") {
		t.Errorf("non-target block was altered, got:\n%s", got)
	}

	// "> earlier" should never appear (non-target body never gets a marker).
	if strings.Contains(got, "> earlier") {
		t.Errorf("non-target body got prefixed with marker, got:\n%s", got)
	}
}

func TestFormatContextMessages_EmptyInput(t *testing.T) {
	got := formatContextMessages(nil, 0)

	if got != "" {
		t.Errorf("formatContextMessages(nil) = %q, want empty string", got)
	}
}

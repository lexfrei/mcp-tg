package resources

import (
	"context"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testPeer = "durov"

// chatMessagesMock wraps NoopClient and overrides only the two
// methods chatMessagesHandler needs, so we don't have to re-implement
// the full Client interface for this single test.
type chatMessagesMock struct {
	testutil.NoopClient

	resolved telegram.InputPeer
	messages []telegram.Message
}

func (m *chatMessagesMock) ResolvePeer(_ context.Context, _ string) (telegram.InputPeer, error) {
	return m.resolved, nil
}

func (m *chatMessagesMock) GetHistory(
	_ context.Context, _ telegram.InputPeer, _ telegram.HistoryOpts,
) ([]telegram.Message, int, error) {
	return m.messages, len(m.messages), nil
}

// TestChatMessagesTemplateMIMETypeMatchesHandler pins the agreement
// between the template-declared MIMEType (used by MCP clients to
// decide how to render the resource) and the actual MIMEType the
// handler emits. A mismatch would silently mis-render the resource —
// the template previously claimed 'application/json' while the
// handler returned 'text/plain'.
func TestChatMessagesTemplateMIMETypeMatchesHandler(t *testing.T) {
	template := chatMessagesTemplate()

	mock := &chatMessagesMock{
		resolved: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: []telegram.Message{{ID: 1, Date: 1700000000, Text: "hi"}},
	}
	handler := chatMessagesHandler(mock)

	result, err := handler(context.Background(), &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "tg://chat/durov/messages"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("got %d content blocks, want 1", len(result.Contents))
	}

	if template.MIMEType != result.Contents[0].MIMEType {
		t.Errorf("template MIMEType=%q vs handler MIMEType=%q — must agree so MCP clients render the resource correctly",
			template.MIMEType, result.Contents[0].MIMEType)
	}
}

func TestChatMessagesHandler_UsesUnifiedMultiLineFormat(t *testing.T) {
	mock := &chatMessagesMock{
		resolved: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: []telegram.Message{
			{
				ID: 10, Date: 1700000000,
				FromID: 5, FromType: telegram.PeerUser,
				FromName: "Alice", FromUsername: "alice",
				Text: "hello",
			},
			{ID: 11, Date: 1700000001, Text: "second"},
		},
	}

	handler := chatMessagesHandler(mock)

	result, err := handler(context.Background(), &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{URI: "tg://chat/durov/messages"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Contents) != 1 {
		t.Fatalf("got %d content blocks, want 1", len(result.Contents))
	}

	out := result.Contents[0].Text

	// Sender line via the shared peer-ref shape.
	if !strings.Contains(out, "from: Alice [@alice]") {
		t.Errorf("resource output missing unified 'from:' line, got:\n%s", out)
	}

	// Block separator from the shared formatter.
	if !strings.Contains(out, "\n---\n") {
		t.Errorf("resource output missing '---' separator, got:\n%s", out)
	}

	// Both messages survived.
	if !strings.Contains(out, "[10]") || !strings.Contains(out, "[11]") {
		t.Errorf("resource output missing one of the messages, got:\n%s", out)
	}
}

func TestExtractPeer_ChatInfo(t *testing.T) {
	got := extractPeer("tg://chat/" + testPeer)
	if got != testPeer {
		t.Errorf("extractPeer(chat info) = %q, want %q", got, testPeer)
	}
}

func TestExtractPeer_ChatMessages(t *testing.T) {
	got := extractPeer("tg://chat/" + testPeer + "/messages")
	if got != testPeer {
		t.Errorf("extractPeer(chat messages) = %q, want %q", got, testPeer)
	}
}

func TestChatMessagesPeer_RequiresMessagesSuffix(t *testing.T) {
	// A chat-messages URI yields its peer.
	if got := ChatMessagesPeer("tg://chat/" + testPeer + "/messages"); got != testPeer {
		t.Errorf("ChatMessagesPeer(messages) = %q, want %q", got, testPeer)
	}

	// A bare chat-info URI (no /messages) must NOT match — the subscribe
	// handler relies on this to ignore the info resource.
	if got := ChatMessagesPeer("tg://chat/" + testPeer); got != "" {
		t.Errorf("ChatMessagesPeer(chat info) = %q, want empty", got)
	}

	// Wrong scheme yields empty.
	if got := ChatMessagesPeer("other://chat/" + testPeer + "/messages"); got != "" {
		t.Errorf("ChatMessagesPeer(wrong scheme) = %q, want empty", got)
	}

	// Numeric peer round-trips verbatim (the live-smoke shape).
	if got := ChatMessagesPeer("tg://chat/777000/messages"); got != "777000" {
		t.Errorf("ChatMessagesPeer(numeric) = %q, want %q", got, "777000")
	}
}

func TestExtractPeer_WrongScheme(t *testing.T) {
	got := extractPeer("other://chat/someone")
	if got != "" {
		t.Errorf("extractPeer(wrong scheme) = %q, want empty", got)
	}
}

func TestExtractPeer_Empty(t *testing.T) {
	got := extractPeer("")
	if got != "" {
		t.Errorf("extractPeer(empty) = %q, want empty", got)
	}
}

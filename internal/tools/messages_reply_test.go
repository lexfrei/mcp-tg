package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testAliceName = "Alice"

func messagesWithReply() []telegram.Message {
	return []telegram.Message{
		{ID: 26150, Date: 1700000000, Text: "setup", FromName: testAliceName},
		{
			ID:       26154,
			Date:     1700000001,
			Text:     "punchline",
			FromName: "Bob",
			ReplyTo:  &telegram.ReplyToInfo{MessageID: 26150},
		},
	}
}

func TestMessagesListHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
		total:    2,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{Peer: "@chat"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil {
		t.Fatal("expected ReplyTo on second message")
	}

	if res.Messages[1].ReplyTo.MessageID != 26150 {
		t.Errorf("ReplyTo.MessageID = %d, want 26150", res.Messages[1].ReplyTo.MessageID)
	}

	if !strings.Contains(res.Output, "↩26150") {
		t.Errorf("output missing reply marker ↩26150: %q", res.Output)
	}

	// Without ResolveReplies, parent text should not be fetched again
	// (it's already in the batch), but ReplyToMessage should also not
	// be populated since the flag is off.
	if res.Messages[1].ReplyToMessage != nil {
		t.Errorf("ReplyToMessage = %+v, want nil when resolveReplies is off", res.Messages[1].ReplyToMessage)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0", mock.getMessagesCalls)
	}
}

func TestMessagesListHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    "punchline",
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
		parentMessages: []telegram.Message{
			{ID: 26150, Text: "setup from earlier", FromName: testAliceName},
		},
		total: 1,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:           "@chat",
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != "setup from earlier" {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, "setup from earlier")
	}

	if res.Messages[0].ReplyToMessage.FromName != testAliceName {
		t.Errorf("ReplyToMessage.FromName = %q, want %q",
			res.Messages[0].ReplyToMessage.FromName, testAliceName)
	}
}

func TestMessagesContextHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesContextHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesContextParams{
		Peer:      "@chat",
		MessageID: 26154,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil || res.Messages[1].ReplyTo.MessageID != 26150 {
		t.Errorf("ReplyTo not propagated: %+v", res.Messages[1].ReplyTo)
	}

	if !strings.Contains(res.Output, "↩26150") {
		t.Errorf("output missing reply marker: %q", res.Output)
	}
}

func TestMessagesGetHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesGetHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesGetParams{
		Peer: "@chat",
		IDs:  []int{26150, 26154},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want populated")
	}

	if !strings.Contains(res.Output, "↩26150") {
		t.Errorf("output missing reply marker: %q", res.Output)
	}
}

func TestMessagesSearchHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(context.Background(),
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		MessagesSearchParams{Peer: "@chat", Query: "punchline"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[1].ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want populated")
	}

	if !strings.Contains(res.Output, "↩26150") {
		t.Errorf("output missing reply marker: %q", res.Output)
	}
}

func TestMessagesSearchGlobalHandler_ReplyTo_Propagated(t *testing.T) {
	mock := &mockClient{
		messages: []telegram.Message{
			{
				ID:      26154,
				Text:    "punchline",
				ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
			},
		},
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSearchGlobalParams{
		Query: "punchline",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Messages[0].ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want propagated in global search")
	}

	if res.Messages[0].ReplyTo.MessageID != 26150 {
		t.Errorf("ReplyTo.MessageID = %d, want 26150", res.Messages[0].ReplyTo.MessageID)
	}
}

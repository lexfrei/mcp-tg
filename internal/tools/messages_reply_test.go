package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	testAliceName      = "Alice"
	testSetupFromEarly = "setup from earlier"
	testSetupWord      = "setup"
)

func messagesWithReply() []telegram.Message {
	return []telegram.Message{
		{ID: 26150, Date: 1700000000, Text: testSetupWord, FromName: testAliceName},
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
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
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

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}

	if res.Messages[0].ReplyToMessage.FromName != testAliceName {
		t.Errorf("ReplyToMessage.FromName = %q, want %q",
			res.Messages[0].ReplyToMessage.FromName, testAliceName)
	}
}

func TestMessagesListHandler_ResolveReplies_ParentInBatchNoFetch(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
		total:    2,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:           "@chat",
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 when parent already in batch",
			mock.getMessagesCalls)
	}

	if res.Messages[1].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated from batch when resolveReplies is on")
	}

	if res.Messages[1].ReplyToMessage.Text != testSetupWord {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[1].ReplyToMessage.Text, testSetupWord)
	}
}

func TestMessagesContextHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
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
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
	}
	handler := NewMessagesContextHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesContextParams{
		Peer:           "@chat",
		MessageID:      26154,
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

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}
}

func TestMessagesGetHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
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
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
		},
	}
	handler := NewMessagesGetHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesGetParams{
		Peer:           "@chat",
		IDs:            []int{26154},
		ResolveReplies: &resolveOn,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// messages_get fetches the requested IDs (26154) via GetMessages,
	// then the resolver makes a second GetMessages call for the
	// missing parent (26150). Any change that folds the two into a
	// single call must update this assertion deliberately.
	const wantGetMessagesCalls = 2
	if mock.getMessagesCalls != wantGetMessagesCalls {
		t.Errorf("GetMessages calls = %d, want %d (primary fetch + resolver lookup)",
			mock.getMessagesCalls, wantGetMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
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

func TestMessagesSearchHandler_ResolveReplies_FetchesMissingParent(t *testing.T) {
	resolveOn := true
	primary := []telegram.Message{
		{
			ID:      26154,
			Text:    "punchline",
			ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
		},
	}
	fetched := []telegram.Message{
		{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
	}
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: primary,
		getMessagesFn: func(_ []int) []telegram.Message {
			return fetched
		},
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(
		context.Background(),
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		MessagesSearchParams{
			Peer:           "@chat",
			Query:          "punchline",
			ResolveReplies: &resolveOn,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 1 {
		t.Errorf("GetMessages called %d times, want 1", mock.getMessagesCalls)
	}

	if res.Messages[0].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated via fetch")
	}

	if res.Messages[0].ReplyToMessage.Text != testSetupFromEarly {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[0].ReplyToMessage.Text, testSetupFromEarly)
	}
}

func TestMessagesSearchHandler_ResolveReplies_ParentInBatchNoFetch(t *testing.T) {
	resolveOn := true
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(
		context.Background(),
		&mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}},
		MessagesSearchParams{
			Peer:           "@chat",
			Query:          "punchline",
			ResolveReplies: &resolveOn,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.getMessagesCalls != 0 {
		t.Errorf("GetMessages called %d times, want 0 when parent already in batch",
			mock.getMessagesCalls)
	}

	if res.Messages[1].ReplyToMessage == nil {
		t.Fatal("ReplyToMessage = nil, want populated from batch when resolveReplies is on")
	}

	if res.Messages[1].ReplyToMessage.Text != testSetupWord {
		t.Errorf("ReplyToMessage.Text = %q, want %q",
			res.Messages[1].ReplyToMessage.Text, testSetupWord)
	}
}

func TestMessagesListHandler_ResolveReplies_OutputUnchanged(t *testing.T) {
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
			{ID: 26150, Text: testSetupFromEarly, FromName: testAliceName},
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

	// Output only carries the ↩<parentId> marker; resolved parent text
	// lives in the JSON replyToMessage field. Keep both behaviours
	// pinned so future changes touch them deliberately.
	if strings.Contains(res.Output, testSetupFromEarly) {
		t.Errorf("Output must not embed resolved parent text, got %q", res.Output)
	}

	if !strings.Contains(res.Output, "↩26150") {
		t.Errorf("Output missing reply marker ↩26150: %q", res.Output)
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

	// Global search intentionally returns only a summary line as
	// output; individual ↩ markers are not emitted. Pin that, so a
	// future change to output format has to touch this test.
	if strings.Contains(res.Output, "↩") {
		t.Errorf("Output must not contain reply markers for global search, got %q", res.Output)
	}
}

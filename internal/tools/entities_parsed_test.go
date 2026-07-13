package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func twoEntityMessage() *telegram.Message {
	return &telegram.Message{
		ID: 7,
		Entities: []telegram.Entity{
			{Type: "bold", Offset: 0, Length: 4},
			{Type: "code", Offset: 5, Length: 3},
		},
	}
}

func TestMessagesSendHandler_EntitiesParsedFromEcho(t *testing.T) {
	mock := &mockClient{
		peer:    telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		message: twoEntityMessage(),
	}
	handler := NewMessagesSendHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "**bold** `c`", ParseMode: "commonmark",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.EntitiesParsed != 2 {
		t.Errorf("EntitiesParsed = %d, want 2", res.EntitiesParsed)
	}
}

// TestMessagesSendHandler_EntitiesParsedZeroSerialized pins that a
// zero count still lands in the JSON — 0 is the caller's signal that
// nothing parsed, so omitempty would defeat the whole field.
func TestMessagesSendHandler_EntitiesParsedZeroSerialized(t *testing.T) {
	mock := &mockClient{
		peer:    telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		message: &telegram.Message{ID: 7},
	}
	handler := NewMessagesSendHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "hello", ParseMode: "plain",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(raw), `"entitiesParsed":0`) {
		t.Errorf("zero count must serialize, got: %s", raw)
	}
}

func TestMessagesEditHandler_EntitiesParsedFromEcho(t *testing.T) {
	mock := &mockClient{
		peer:    telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		message: twoEntityMessage(),
	}
	handler := NewMessagesEditHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesEditParams{
		Peer: "@chat", MessageID: 7, Text: "**bold** `c`", ParseMode: "commonmark",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.EntitiesParsed != 2 {
		t.Errorf("EntitiesParsed = %d, want 2", res.EntitiesParsed)
	}
}

// TestMediaSendAlbumHandler_EntitiesParsedSumsAllMessages pins the sum
// across the album: server update order is not a contract, so the
// count must not depend on which returned message carries the caption.
func TestMediaSendAlbumHandler_EntitiesParsedSumsAllMessages(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: []telegram.Message{
			{ID: 1},
			{ID: 2, Entities: []telegram.Entity{{Type: "bold"}, {Type: "italic"}, {Type: "code"}}},
		},
	}
	handler := NewMediaSendAlbumHandler(mock)

	_, res, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer: "@chat", Paths: []string{"/tmp/a", "/tmp/b"}, ParseMode: "plain",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.EntitiesParsed != 3 {
		t.Errorf("EntitiesParsed = %d, want the sum 3", res.EntitiesParsed)
	}
}

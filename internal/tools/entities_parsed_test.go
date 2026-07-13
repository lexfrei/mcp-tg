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

func TestMessagesSendFileHandler_EntitiesParsedFromEcho(t *testing.T) {
	mock := &mockClient{
		peer:    telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		message: twoEntityMessage(),
	}
	handler := NewMessagesSendFileHandler(mock)

	_, res, err := handler(context.Background(), emptyToolRequest(), MessagesSendFileParams{
		Peer: "@chat", Path: "/tmp/f", ParseMode: "plain",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.EntitiesParsed != 2 {
		t.Errorf("EntitiesParsed = %d, want 2", res.EntitiesParsed)
	}
}

// TestEntitiesParsed_ZeroSerializesInEveryResult pins the no-omitempty
// contract across all four result shapes — 0 is the signal, so a
// silently omitted field would defeat it on any of them.
func TestEntitiesParsed_ZeroSerializesInEveryResult(t *testing.T) {
	results := map[string]any{
		"send":     MessagesSendResult{},
		"edit":     MessagesEditResult{},
		"sendFile": MessagesSendFileResult{},
		"album":    MediaSendAlbumResult{},
	}

	for name, result := range results {
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("%s: marshal: %v", name, err)
		}

		if !strings.Contains(string(raw), `"entitiesParsed":0`) {
			t.Errorf("%s: zero entitiesParsed must serialize, got: %s", name, raw)
		}
	}
}

// TestMessagesSendHandler_AutoDetectedEntitiesNotCounted pins the bug
// that made the signal lie: a plain send containing a bare link and a
// hashtag comes back with server-added entities, and counting them
// would tell the caller their markdown parsed when none was requested.
func TestMessagesSendHandler_AutoDetectedEntitiesNotCounted(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		message: &telegram.Message{
			ID: 7,
			Entities: []telegram.Entity{
				{Type: telegram.EntityTypeURL, Offset: 4, Length: 19},
				{Type: telegram.EntityTypeHashtag, Offset: 28, Length: 4},
			},
		},
	}
	handler := NewMessagesSendHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "see https://example.com and #tag", ParseMode: "plain",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.EntitiesParsed != 0 {
		t.Errorf("EntitiesParsed = %d, want 0 — the server detected those, no parseMode did", res.EntitiesParsed)
	}
}

// TestMessagesSendHandler_MixedEntitiesCountFormattingOnly pins the
// other half: real formatting still counts, alongside auto-detected
// entities in the same message.
func TestMessagesSendHandler_MixedEntitiesCountFormattingOnly(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		message: &telegram.Message{
			ID: 7,
			Entities: []telegram.Entity{
				{Type: telegram.EntityTypeURL},
				{Type: telegram.EntityTypeBold},
				{Type: telegram.EntityTypeTextURL},
			},
		},
	}
	handler := NewMessagesSendHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "**b** [t](u) https://x.y", ParseMode: "commonmark",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.EntitiesParsed != 2 {
		t.Errorf("EntitiesParsed = %d, want 2 (bold + text_url, not the bare url)", res.EntitiesParsed)
	}
}

// TestMediaSendAlbumHandler_ScheduledAlbumIsNotEmpty pins the symptom
// the scheduled-echo fix addresses: a scheduled album used to come back
// with count 0 and entitiesParsed 0 even though every item was queued.
func TestMediaSendAlbumHandler_ScheduledAlbumIsNotEmpty(t *testing.T) {
	when := 1900000000
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: []telegram.Message{
			{ID: 1, Entities: []telegram.Entity{{Type: telegram.EntityTypeBold}}},
			{ID: 2},
		},
	}
	handler := NewMediaSendAlbumHandler(mock)

	_, res, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer: "@chat", Paths: []string{"/tmp/a", "/tmp/b"},
		ParseMode: "commonmark", ScheduleDate: &when,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 2 || res.EntitiesParsed != 1 {
		t.Errorf("count=%d entitiesParsed=%d, want 2/1 — the scheduled echo was dropped", res.Count, res.EntitiesParsed)
	}
}

// TestMediaSendAlbumHandler_UnreadableEchoReportsNothing pins the one
// path deliberately left unrepaired: an unreadable album echo yields no
// messages, and reporting entities beside count 0 would be incoherent.
// The anomaly must surface as count 0, not as a plausible-looking zero
// entity count on a phantom album.
func TestMediaSendAlbumHandler_UnreadableEchoReportsNothing(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: nil, // the wrapper could not read the envelope
	}
	handler := NewMediaSendAlbumHandler(mock)

	_, res, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer: "@chat", Paths: []string{"/tmp/a"}, ParseMode: "commonmark",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 0 || res.EntitiesParsed != 0 {
		t.Errorf("count=%d entitiesParsed=%d, want 0/0 — no entities may be invented for an album that echoed nothing",
			res.Count, res.EntitiesParsed)
	}
}

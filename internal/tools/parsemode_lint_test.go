package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestMessagesSendHandler_PlainLintCatchesMarkdown(t *testing.T) {
	handler := NewMessagesSendHandler(&mockClient{})

	result, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "this is **bold** move", ParseMode: "plain",
	})
	if !errors.Is(err, ErrPlainLooksLikeMarkdown) {
		t.Errorf("err = %v, want ErrPlainLooksLikeMarkdown", err)
	}

	if result == nil || !result.IsError {
		t.Error("result must be marked IsError")
	}
}

func TestMessagesSendHandler_AllowRawMarkdownOverridesLint(t *testing.T) {
	allow := true
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesSendHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "literal `backticks` on purpose", ParseMode: "plain",
		AllowRawMarkdown: &allow,
	})
	if err != nil {
		t.Fatalf("allowRawMarkdown must bypass the lint, got: %v", err)
	}
}

func TestMessagesSendHandler_CommonmarkSkipsLint(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesSendHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@chat", Text: "real **bold** here", ParseMode: "commonmark",
	})
	if err != nil {
		t.Fatalf("commonmark text must not be linted, got: %v", err)
	}
}

func TestMessagesEditHandler_PlainLintCatchesMarkdown(t *testing.T) {
	handler := NewMessagesEditHandler(&mockClient{})

	msgID := 5
	_, _, err := handler(context.Background(), nil, MessagesEditParams{
		Peer: "@chat", MessageID: msgID, Text: "see [docs](https://example.com)", ParseMode: "plain",
	})
	if !errors.Is(err, ErrPlainLooksLikeMarkdown) {
		t.Errorf("err = %v, want ErrPlainLooksLikeMarkdown", err)
	}
}

func TestMessagesSendFileHandler_PlainLintOnCaption(t *testing.T) {
	caption := "run ```go build``` first"
	handler := NewMessagesSendFileHandler(&mockClient{})

	_, _, err := handler(context.Background(), emptyToolRequest(), MessagesSendFileParams{
		Peer: "@chat", Path: "/tmp/f", Caption: &caption, ParseMode: "plain",
	})
	if !errors.Is(err, ErrPlainLooksLikeMarkdown) {
		t.Errorf("err = %v, want ErrPlainLooksLikeMarkdown", err)
	}
}

func TestMediaSendAlbumHandler_PlainLintOnCaption(t *testing.T) {
	caption := "the ||spoiler|| inside"
	handler := NewMediaSendAlbumHandler(&mockClient{})

	_, _, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer: "@chat", Paths: []string{"/tmp/a"}, Caption: &caption, ParseMode: "plain",
	})
	if !errors.Is(err, ErrPlainLooksLikeMarkdown) {
		t.Errorf("err = %v, want ErrPlainLooksLikeMarkdown", err)
	}
}

func TestMediaSendAlbumHandler_EmptyCaptionSkipsLint(t *testing.T) {
	// No caption means nothing to lint — plain mode must not error on
	// the empty string.
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMediaSendAlbumHandler(mock)

	_, _, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer: "@chat", Paths: []string{"/tmp/a", "/tmp/b"}, ParseMode: "plain",
	})
	if errors.Is(err, ErrPlainLooksLikeMarkdown) {
		t.Errorf("empty caption must not trip the lint, got: %v", err)
	}
}

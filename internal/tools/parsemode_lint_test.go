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

// TestValidatePlainText_Matrix covers the (mode x allowRaw x text) grid
// at the single point where the rule lives, instead of relying on the
// per-tool handler tests to cover it by accident.
func TestValidatePlainText_Matrix(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		allowRaw bool
		text     string
		want     error
	}{
		{"plain markdown rejected", telegram.ParseModePlain, false, "a **b** c", ErrPlainLooksLikeMarkdown},
		{"plain prose passes", telegram.ParseModePlain, false, "a plain sentence", nil},
		{"plain empty passes", telegram.ParseModePlain, false, "", nil},
		{"plain override passes", telegram.ParseModePlain, true, "a **b** c", nil},
		{"commonmark markdown passes", telegram.ParseModeCommonMark, false, "a **b** c", nil},
		{
			"commonmark override rejected",
			telegram.ParseModeCommonMark, true, "a **b** c",
			ErrAllowRawMarkdownWithoutPlain,
		},
	}

	for _, tc := range cases {
		got := validatePlainText(tc.mode, tc.allowRaw, tc.text)
		if !errors.Is(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

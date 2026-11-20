package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mediaPeer() telegram.InputPeer {
	return telegram.InputPeer{Type: telegram.PeerUser, ID: 5, AccessHash: 1}
}

func TestMediaSendVoice(t *testing.T) {
	mock := &mockClient{peer: mediaPeer(), message: &telegram.Message{ID: 3}}

	_, result, err := NewMediaSendVoiceHandler(mock, nil)(context.Background(), nil, MediaSendVoiceParams{
		Peer: "@ann", Path: "/tmp/note.mp4", VideoNote: new(true), Duration: 9,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.MessageID != 3 || mock.lastVoicePath != "/tmp/note.mp4" || !mock.lastVoiceOpts.VideoNote ||
		mock.lastVoiceOpts.Duration != 9 {
		t.Errorf("result = %+v, opts = %+v", result, mock.lastVoiceOpts)
	}
}

// A path outside TELEGRAM_FILE_ROOTS must be refused before anything is
// uploaded, like every other tool that reads a local file.
func TestMediaSendVoice_RespectsFileRoots(t *testing.T) {
	mock := &mockClient{peer: mediaPeer()}

	_, _, err := NewMediaSendVoiceHandler(mock, []string{t.TempDir()})(context.Background(), nil,
		MediaSendVoiceParams{Peer: "@ann", Path: "/etc/passwd"})
	if err == nil {
		t.Fatal("a path outside the roots was accepted")
	}

	if mock.lastVoicePath != "" {
		t.Error("the file was sent despite the refusal")
	}
}

func TestMediaGetInfo(t *testing.T) {
	mock := &mockClient{peer: mediaPeer(), mediaInfo: &telegram.MediaInfo{
		Type: "video", MimeType: "video/mp4", Size: 2048, Width: 640, Height: 360, Duration: 12,
	}}

	_, result, err := NewMediaGetInfoHandler(mock)(context.Background(), nil, MediaGetInfoParams{Peer: "@ann", MessageID: 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "video, video/mp4, 2048 bytes, 640x360, 12s" {
		t.Errorf("output = %q", result.Output)
	}
}

// The image must reach the client as image content, not only as a number of
// bytes in the structured result.
func TestMediaView_ReturnsImageContent(t *testing.T) {
	mock := &mockClient{peer: mediaPeer(), viewed: &telegram.ViewedImage{Data: []byte{0xFF, 0xD8}, MimeType: "image/jpeg"}}

	res, result, err := NewMediaViewHandler(mock)(context.Background(), nil, MediaViewParams{Peer: "@ann", MessageID: 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastViewLimit != viewMaxBytes || result.Size != 2 {
		t.Errorf("limit = %d, result = %+v", mock.lastViewLimit, result)
	}

	if res == nil || len(res.Content) != 2 {
		t.Fatalf("content = %+v, want text and image", res)
	}

	image, ok := res.Content[1].(*mcp.ImageContent)
	if !ok || image.MIMEType != "image/jpeg" || len(image.Data) != 2 {
		t.Errorf("image content = %+v", res.Content[1])
	}
}

func TestInlineQueryAndSend(t *testing.T) {
	bot := telegram.InputPeer{Type: telegram.PeerUser, ID: 99, AccessHash: 2}
	mock := &mockClient{
		resolvePeerFn: func(identifier string) (telegram.InputPeer, error) {
			if identifier == "@gif" {
				return bot, nil
			}

			return mediaPeer(), nil
		},
		inlineResults: &telegram.InlineResults{QueryID: "9007199254740993", Results: []telegram.InlineResult{
			{ID: "r1", Type: "gif", Title: "cat"},
		}},
		message: &telegram.Message{ID: 11},
	}

	_, query, err := NewInlineQueryHandler(mock)(context.Background(), nil, InlineQueryParams{Bot: "@gif", Query: "cat", Peer: "@ann"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if mock.lastInlineBot.ID != 99 || mock.lastInlineQuery != "cat" || !strings.Contains(query.Output, "[r1] gif — cat") {
		t.Errorf("bot = %+v, query = %q, output = %q", mock.lastInlineBot, mock.lastInlineQuery, query.Output)
	}

	// The query ID needs all 64 bits; passing it through a float would
	// round this one to ...992.
	_, sent, err := NewInlineSendHandler(mock)(context.Background(), nil,
		InlineSendParams{Peer: "@ann", QueryID: query.QueryID, ResultID: "r1"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	if mock.lastQueryID != 9007199254740993 || mock.lastResultID != "r1" || sent.MessageID != 11 {
		t.Errorf("queryId = %d, resultId = %q, result = %+v", mock.lastQueryID, mock.lastResultID, sent)
	}
}

func TestInlineSend_Validation(t *testing.T) {
	for want, params := range map[error]InlineSendParams{
		ErrPeerRequired:     {QueryID: "1", ResultID: "r"},
		ErrQueryIDRequired:  {Peer: "@x", ResultID: "r"},
		ErrResultIDRequired: {Peer: "@x", QueryID: "1"},
	} {
		_, _, err := NewInlineSendHandler(&mockClient{})(context.Background(), nil, params)
		if !errors.Is(err, want) {
			t.Errorf("params %+v: err = %v, want %v", params, err, want)
		}
	}

	_, _, err := NewInlineSendHandler(&mockClient{})(context.Background(), nil,
		InlineSendParams{Peer: "@x", QueryID: "abc", ResultID: "r"})
	if !errors.Is(err, ErrQueryIDRequired) {
		t.Errorf("non-numeric queryId: err = %v", err)
	}
}

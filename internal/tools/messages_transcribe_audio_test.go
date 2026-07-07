package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

var errPlainPremiumText = errors.New("plain text PREMIUM_ACCOUNT_REQUIRED")

func TestMessagesTranscribeAudioHandler_ReturnsCompletedText(t *testing.T) {
	waitSeconds := 1
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
		transcription: &telegram.Transcription{
			Status:          telegram.TranscriptionStatusCompleted,
			MessageID:       42,
			Type:            telegram.MessageTypeVoice,
			TranscriptionID: 77,
			Text:            "hello from voice",
		},
	}
	handler := NewMessagesTranscribeAudioHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesTranscribeAudioParams{
		Peer:        "@chat",
		MessageID:   42,
		WaitSeconds: &waitSeconds,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != telegram.TranscriptionStatusCompleted {
		t.Errorf("Status = %q, want completed", res.Status)
	}

	if res.Text != "hello from voice" {
		t.Errorf("Text = %q, want voice text", res.Text)
	}

	if res.Type != telegram.MessageTypeVoice {
		t.Errorf("Type = %q, want voice", res.Type)
	}

	if mock.lastTranscribeID != 42 {
		t.Errorf("lastTranscribeID = %d, want 42", mock.lastTranscribeID)
	}

	if mock.lastWait != time.Second {
		t.Errorf("lastWait = %s, want 1s", mock.lastWait)
	}

	if !strings.Contains(res.Output, "transcription:\nhello from voice") {
		t.Errorf("Output missing transcription text, got:\n%s", res.Output)
	}
}

func TestMessagesTranscribeAudioHandler_ReturnsVideoNoteType(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
		transcription: &telegram.Transcription{
			Status:          telegram.TranscriptionStatusCompleted,
			MessageID:       51555,
			Type:            telegram.MessageTypeVideoNote,
			TranscriptionID: 77,
			Text:            "hello from video note",
		},
	}
	handler := NewMessagesTranscribeAudioHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesTranscribeAudioParams{
		Peer:      "@chat",
		MessageID: 51555,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Type != telegram.MessageTypeVideoNote {
		t.Errorf("Type = %q, want video_note", res.Type)
	}
	if !strings.Contains(res.Output, "type: video_note") {
		t.Errorf("Output missing video_note type, got:\n%s", res.Output)
	}
}

func TestMessagesTranscribeAudioHandler_ZeroMessageID(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesTranscribeAudioHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesTranscribeAudioParams{Peer: "@chat"})
	if err == nil {
		t.Fatal("expected validation error for zero message ID")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesTranscribeAudioHandler_RejectsInvalidWaitSeconds(t *testing.T) {
	tests := []struct {
		name        string
		waitSeconds int
	}{
		{name: "negative", waitSeconds: -1},
		{name: "above maximum", waitSeconds: 121},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockClient{}
			handler := NewMessagesTranscribeAudioHandler(mock)

			result, _, err := handler(context.Background(), nil, MessagesTranscribeAudioParams{
				Peer:        "@chat",
				MessageID:   42,
				WaitSeconds: &tt.waitSeconds,
			})
			if err == nil {
				t.Fatal("expected validation error for invalid waitSeconds")
			}

			if result == nil || !result.IsError {
				t.Error("result.IsError should be true")
			}
		})
	}
}

func TestMessagesTranscribeAudioHandler_DoesNotMapPlainStringErrors(t *testing.T) {
	mock := &mockClient{
		peer:          telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
		transcribeErr: errPlainPremiumText,
	}
	handler := NewMessagesTranscribeAudioHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesTranscribeAudioParams{
		Peer:      "@chat",
		MessageID: 42,
	})
	if err == nil {
		t.Fatal("expected plain string error to remain a tool error")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesTranscribeAudioHandler_MapsTelegramStatuses(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "premium required",
			err:  tgerr.New(403, tg.ErrPremiumAccountRequired),
			want: telegram.TranscriptionStatusPremiumRequired,
		},
		{
			name: "not transcribable",
			err:  tgerr.New(400, tg.ErrMsgVoiceMissing),
			want: telegram.TranscriptionStatusNotTranscribable,
		},
		{
			name: "failed",
			err:  tgerr.New(400, tg.ErrTranscriptionFailed),
			want: telegram.TranscriptionStatusFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockClient{
				peer:          telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
				transcribeErr: tt.err,
			}
			handler := NewMessagesTranscribeAudioHandler(mock)

			result, res, err := handler(context.Background(), nil, MessagesTranscribeAudioParams{
				Peer:      "@chat",
				MessageID: 42,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != nil && result.IsError {
				t.Errorf("%s should be returned as a structured status, not an MCP tool error", tt.want)
			}

			if res.Status != tt.want {
				t.Errorf("Status = %q, want %q", res.Status, tt.want)
			}
		})
	}
}

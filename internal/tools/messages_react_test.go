package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
)

func TestMessagesReactTool_Definition(t *testing.T) {
	tool := MessagesReactTool()
	if tool.Name == "" {
		t.Error("tool name must not be empty")
	}
}

func TestMessagesReactHandler_SingleEmoji(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emoji:     "👍",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := mock.lastReactionOpts.Emojis; len(got) != 1 || got[0] != "👍" {
		t.Errorf("Emojis = %v, want [👍]", got)
	}

	if mock.lastReactionOpts.Remove {
		t.Error("Remove should be false")
	}

	if !strings.Contains(structured.Output, "Added") {
		t.Errorf("Output = %q, want it to mention Added", structured.Output)
	}
}

func TestMessagesReactHandler_MultipleEmojis(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	_, structured, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emojis:    []string{"👍", "custom:5210952531676504517"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := mock.lastReactionOpts.Emojis
	if len(got) != 2 || got[0] != "👍" || got[1] != "custom:5210952531676504517" {
		t.Errorf("Emojis = %v, want [👍 custom:5210952531676504517]", got)
	}

	if !strings.Contains(structured.Output, "reactions") {
		t.Errorf("Output = %q, want plural 'reactions' for multiple", structured.Output)
	}
}

func TestMessagesReactHandler_InvalidCustomEmoji(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emojis:    []string{"custom:not-a-number"},
	})
	if err == nil {
		t.Fatal("expected error for malformed custom emoji id")
	}

	if result == nil || !result.IsError {
		t.Error("expected IsError result")
	}

	// Malformed input must be rejected before any send is attempted.
	if mock.lastReactionOpts.Emojis != nil {
		t.Error("SendReaction should not be called for invalid input")
	}
}

// The plural Emojis field wins over the singular Emoji when both are set.
func TestMessagesReactHandler_EmojisOverridesEmoji(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emoji:     "👎",
		Emojis:    []string{"❤"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := mock.lastReactionOpts.Emojis; len(got) != 1 || got[0] != "❤" {
		t.Errorf("Emojis = %v, want [❤]", got)
	}
}

func TestMessagesReactHandler_Big(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)
	big := true

	_, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emoji:     "👍",
		Big:       &big,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.lastReactionOpts.Big {
		t.Error("Big should be true")
	}
}

func TestMessagesReactHandler_Remove(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)
	remove := true

	_, structured, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Remove:    &remove,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.lastReactionOpts.Remove {
		t.Error("Remove should be true")
	}

	if !strings.Contains(structured.Output, "Removed") {
		t.Errorf("Output = %q, want it to mention Removed", structured.Output)
	}
}

func TestMessagesReactHandler_MissingEmoji(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
	})
	if err == nil {
		t.Fatal("expected error when no emoji and not removing")
	}

	if result == nil || !result.IsError {
		t.Error("expected IsError result")
	}
}

func TestMessagesReactHandler_EmptyEmojiInList(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emojis:    []string{""},
	})
	if err == nil {
		t.Fatal("expected error for empty emoji in list")
	}

	if result == nil || !result.IsError {
		t.Error("expected IsError result")
	}
}

func TestMessagesReactHandler_EmptyEmojiAmongValid(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emojis:    []string{"👍", ""},
	})
	if err == nil {
		t.Fatal("expected error when a blank emoji is mixed with valid ones")
	}
}

func TestMessagesReactHandler_MissingPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesReactParams{
		MessageID: 42,
		Emoji:     "👍",
	})
	if err == nil {
		t.Fatal("expected error when peer is missing")
	}
}

func TestMessagesReactHandler_MissingMessageID(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesReactHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:  "@user",
		Emoji: "👍",
	})
	if err == nil {
		t.Fatal("expected error when message ID is missing")
	}
}

func TestMessagesReactHandler_ClientError(t *testing.T) {
	mock := &mockClient{err: errors.New("fail")}
	handler := NewMessagesReactHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesReactParams{
		Peer:      "@user",
		MessageID: 42,
		Emoji:     "👍",
	})
	if err == nil {
		t.Fatal("expected error from client")
	}
}

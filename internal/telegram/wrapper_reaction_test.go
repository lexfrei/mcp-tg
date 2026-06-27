package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestParseReaction_StandardEmoji(t *testing.T) {
	got, err := parseReaction("👍")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	emoji, ok := got.(*tg.ReactionEmoji)
	if !ok {
		t.Fatalf("got %T, want *tg.ReactionEmoji", got)
	}

	if emoji.Emoticon != "👍" {
		t.Errorf("Emoticon = %q, want 👍", emoji.Emoticon)
	}
}

func TestParseReaction_CustomEmoji(t *testing.T) {
	got, err := parseReaction("custom:5210952531676504517")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	custom, ok := got.(*tg.ReactionCustomEmoji)
	if !ok {
		t.Fatalf("got %T, want *tg.ReactionCustomEmoji", got)
	}

	if custom.DocumentID != 5210952531676504517 {
		t.Errorf("DocumentID = %d, want 5210952531676504517", custom.DocumentID)
	}
}

func TestParseReaction_CustomEmojiInvalidID(t *testing.T) {
	_, err := parseReaction("custom:not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric custom emoji id")
	}
}

func TestParseReaction_CustomEmojiEmptyID(t *testing.T) {
	_, err := parseReaction("custom:")
	if err == nil {
		t.Fatal("expected error for empty custom emoji id")
	}
}

func TestValidateReactionString(t *testing.T) {
	standardErr := ValidateReactionString("👍")
	if standardErr != nil {
		t.Errorf("standard emoji: unexpected error: %v", standardErr)
	}

	customErr := ValidateReactionString("custom:5210952531676504517")
	if customErr != nil {
		t.Errorf("custom emoji: unexpected error: %v", customErr)
	}

	malformedErr := ValidateReactionString("custom:nope")
	if malformedErr == nil {
		t.Error("malformed custom id: expected error")
	}
}

// A custom-emoji reaction read back via reactionEmoji must produce the exact
// string parseReaction accepts, so a reaction can be round-tripped read → send.
func TestReaction_RoundTrip(t *testing.T) {
	const encoded = "custom:5210952531676504517"

	parsed, err := parseReaction(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := reactionEmoji(parsed); got != encoded {
		t.Errorf("round-trip = %q, want %q", got, encoded)
	}
}

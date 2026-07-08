package tools

import (
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestFormatContextMessages_TargetMarkerOnEveryLine(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 99, Date: 1700000000, Text: "earlier context"},
		{
			ID: 100, Date: 1700000001,
			FromID: 1, FromName: "Alice",
			Text: "first line of body\nsecond line of body\nthird",
		},
		{ID: 101, Date: 1700000002, Text: "later context"},
	}

	got := formatContextMessages(msgs, 100)

	// Every line of the target block (header, from:, text:, body lines)
	// must carry the "> " marker so the target stays anchored visually
	// once the body is more than a couple of lines.
	wantLines := []string{
		"> [100] 2023-11-14T22:13:21Z",
		"> from: Alice [user:1]",
		"> type: text",
		"> text:",
		"> first line of body",
		"> second line of body",
		"> third",
	}

	for _, line := range wantLines {
		if !strings.Contains(got, line) {
			t.Errorf("target block missing prefixed line %q in:\n%s", line, got)
		}
	}
}

func TestFormatContextMessages_NonTargetBlocksUnmarked(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 99, Date: 1700000000, Text: "earlier"},
		{ID: 100, Date: 1700000001, Text: "target"},
	}

	got := formatContextMessages(msgs, 100)

	// Non-target block must NOT be prefixed.
	if !strings.Contains(got, "[99] 2023-11-14T22:13:20Z\ntype: text\ntext:\nearlier") {
		t.Errorf("non-target block was altered, got:\n%s", got)
	}

	// "> earlier" should never appear (non-target body never gets a marker).
	if strings.Contains(got, "> earlier") {
		t.Errorf("non-target body got prefixed with marker, got:\n%s", got)
	}
}

func TestFormatContextMessages_EmptyInput(t *testing.T) {
	got := formatContextMessages(nil, 0)

	if got != "" {
		t.Errorf("formatContextMessages(nil) = %q, want empty string", got)
	}
}

package tools

import (
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestFormatTimestamp(t *testing.T) {
	got := formatTimestamp(1700000000)
	want := "2023-11-14T22:13:20Z"

	if got != want {
		t.Errorf("formatTimestamp(1700000000) = %q, want %q", got, want)
	}
}

func TestFormatTimestamp_Zero(t *testing.T) {
	got := formatTimestamp(0)
	want := unknownValue

	if got != want {
		t.Errorf("formatTimestamp(0) = %q, want %q", got, want)
	}
}

func TestFormatMessage_TextOnly(t *testing.T) {
	msg := &telegram.Message{
		ID:   42,
		Date: 1700000000,
		Text: "hello",
	}

	got := formatMessage(msg)
	want := "[42] 2023-11-14T22:13:20Z hello"

	if got != want {
		t.Errorf("formatMessage() = %q, want %q", got, want)
	}
}

func TestFormatMessage_WithMedia(t *testing.T) {
	msg := &telegram.Message{
		ID:        42,
		Date:      1700000000,
		Text:      "caption",
		MediaType: "photo",
	}

	got := formatMessage(msg)
	want := "[42] 2023-11-14T22:13:20Z [photo] caption"

	if got != want {
		t.Errorf("formatMessage() = %q, want %q", got, want)
	}
}

func TestFormatMessage_LongText(t *testing.T) {
	long := strings.Repeat("a", 120)
	msg := &telegram.Message{ID: 1, Date: 1700000000, Text: long}
	got := formatMessage(msg)

	if len(got) > 200 {
		t.Errorf("formatMessage should truncate long text, got len=%d", len(got))
	}
}

func TestFormatDialog_User(t *testing.T) {
	dlg := &telegram.Dialog{Title: "Pavel Durov"}
	got := formatDialog(dlg)
	want := "[user] Pavel Durov"

	if got != want {
		t.Errorf("formatDialog() = %q, want %q", got, want)
	}
}

func TestFormatDialog_Channel_WithUnread(t *testing.T) {
	dlg := &telegram.Dialog{
		Title:       "News",
		IsChannel:   true,
		UnreadCount: 5,
	}
	got := formatDialog(dlg)
	want := "[channel] News (5 unread)"

	if got != want {
		t.Errorf("formatDialog() = %q, want %q", got, want)
	}
}

func TestFormatDialog_Group(t *testing.T) {
	dlg := &telegram.Dialog{Title: "Devs", IsGroup: true}
	got := formatDialog(dlg)
	want := "[group] Devs"

	if got != want {
		t.Errorf("formatDialog() = %q, want %q", got, want)
	}
}

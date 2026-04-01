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

func TestFormatTimestamp_Negative(t *testing.T) {
	got := formatTimestamp(-1)

	if got == "" {
		t.Error("formatTimestamp(-1) should return non-empty string")
	}
}

func TestFormatMessage_Nil(t *testing.T) {
	got := formatMessage(nil)

	if got != unknownValue {
		t.Errorf("formatMessage(nil) = %q, want %q", got, unknownValue)
	}
}

func TestFormatDialog_Nil(t *testing.T) {
	got := formatDialog(nil)

	if got != unknownValue {
		t.Errorf("formatDialog(nil) = %q, want %q", got, unknownValue)
	}
}

func TestFormatMessage_EmptyText(t *testing.T) {
	msg := &telegram.Message{ID: 1, Date: 1700000000}
	got := formatMessage(msg)

	if got == unknownValue {
		t.Error("formatMessage(empty text) should not return unknown")
	}

	if !strings.Contains(got, "[1]") {
		t.Errorf("formatMessage() = %q, should contain message ID", got)
	}
}

func TestFormatDialog_ZeroUnread(t *testing.T) {
	dlg := &telegram.Dialog{Title: "Test", UnreadCount: 0}
	got := formatDialog(dlg)

	if strings.Contains(got, "unread") {
		t.Errorf("formatDialog() = %q, should not mention unread when count is 0", got)
	}
}

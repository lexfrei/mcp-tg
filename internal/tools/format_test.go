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
	msg := &telegram.Message{ID: 42, Date: 1700000000, Text: "hello"}
	got := formatMessage(msg)
	want := "[42] 2023-11-14T22:13:20Z\ntext:\nhello"

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_SenderHiddenButNamed(t *testing.T) {
	msg := &telegram.Message{
		ID: 7, Date: 1700000000,
		FromName: "Anonymized Author",
		Text:     "body",
	}

	got := formatMessage(msg)
	wantLine := "from: Anonymized Author [hidden]"

	if !strings.Contains(got, wantLine) {
		t.Errorf("formatMessage should emit a [hidden] sender line when only FromName is set, got:\n%s",
			got)
	}
}

func TestFormatMessage_ChannelOnBehalfOfSender(t *testing.T) {
	msg := &telegram.Message{
		ID:       7,
		Date:     1700000000,
		FromID:   5005003001,
		FromType: telegram.PeerChannel,
		FromName: "Cozystack Blog",
		Text:     "official post",
	}

	got := formatMessage(msg)
	wantLine := "from: Cozystack Blog [channel:5005003001]"

	if !strings.Contains(got, wantLine) {
		t.Errorf("formatMessage() must label channel-on-behalf-of sender with channel:, got:\n%s", got)
	}
}

func TestFormatMessage_WithSenderUsername(t *testing.T) {
	msg := &telegram.Message{
		ID:           7,
		Date:         1700000000,
		FromID:       123,
		FromName:     "Alice",
		FromUsername: "alice",
		Text:         "hi",
	}

	got := formatMessage(msg)
	want := "[7] 2023-11-14T22:13:20Z\nfrom: Alice [@alice]\ntext:\nhi"

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_WithReply(t *testing.T) {
	msg := &telegram.Message{
		ID:      26154,
		Date:    1700000000,
		Text:    "punchline",
		ReplyTo: &telegram.ReplyToInfo{MessageID: 26150},
	}

	got := formatMessage(msg)
	want := "[26154] 2023-11-14T22:13:20Z\nreply to: 26150\ntext:\npunchline"

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_WithReplyAndMedia(t *testing.T) {
	msg := &telegram.Message{
		ID:        100,
		Date:      1700000000,
		Text:      "caption",
		MediaType: "photo",
		ReplyTo:   &telegram.ReplyToInfo{MessageID: 99},
	}

	got := formatMessage(msg)
	want := "[100] 2023-11-14T22:13:20Z\nreply to: 99\nmedia: photo\ntext:\ncaption"

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_WithMedia(t *testing.T) {
	msg := &telegram.Message{ID: 42, Date: 1700000000, Text: "caption", MediaType: "photo"}
	got := formatMessage(msg)
	want := "[42] 2023-11-14T22:13:20Z\nmedia: photo\ntext:\ncaption"

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_ForwardedFromUser(t *testing.T) {
	msg := &telegram.Message{
		ID: 7, Date: 1700000200,
		FromID: 1, FromName: "Forwarder", FromUsername: "forw",
		Text: "fwd body",
		Forward: &telegram.ForwardInfo{
			Date: 1700000000,
			From: &telegram.PeerRef{
				Peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 99},
				Name:     "Origin",
				Username: "orig",
			},
		},
	}

	got := formatMessage(msg)
	want := strings.Join([]string{
		"[7] 2023-11-14T22:16:40Z",
		"from: Forwarder [@forw]",
		"forwarded from: Origin [@orig] at 2023-11-14T22:13:20Z",
		"text:",
		"fwd body",
	}, "\n")

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_ForwardedFromChannel_WithPostAuthor(t *testing.T) {
	msg := &telegram.Message{
		ID: 7, Date: 1700000200,
		FromID: 1, FromName: "Forwarder",
		Text: "body",
		Forward: &telegram.ForwardInfo{
			Date:        1700000000,
			ChannelPost: 4567,
			PostAuthor:  "Kvaps",
			From: &telegram.PeerRef{
				Peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 500},
				Name:     "Cozystack Blog",
				Username: "cozystack_blog",
			},
		},
	}

	got := formatMessage(msg)
	want := strings.Join([]string{
		"[7] 2023-11-14T22:16:40Z",
		"from: Forwarder [user:1]",
		`forwarded from channel: Cozystack Blog [@cozystack_blog] #4567 by "Kvaps" at 2023-11-14T22:13:20Z`,
		"text:",
		"body",
	}, "\n")

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessage_ForwardedAnonymousChannelPost_KeepsPostID(t *testing.T) {
	msg := &telegram.Message{
		ID: 7, Date: 1700000200,
		FromID: 1, FromName: "Forwarder",
		Text: "body",
		Forward: &telegram.ForwardInfo{
			Date:        1700000000,
			ChannelPost: 4567,
		},
	}

	got := formatMessage(msg)
	wantLine := "forwarded from channel: [hidden] #4567 at 2023-11-14T22:13:20Z"

	if !strings.Contains(got, wantLine) {
		t.Errorf("formatMessage() must keep the channel post number even when From is nil, missing %q in:\n%s",
			wantLine, got)
	}
}

func TestFormatMessage_ForwardedHiddenPrivacy(t *testing.T) {
	msg := &telegram.Message{
		ID: 7, Date: 1700000200,
		FromID: 1, FromName: "Forwarder",
		Text: "body",
		Forward: &telegram.ForwardInfo{
			Date:     1700000000,
			FromName: "Kaidxen",
		},
	}

	got := formatMessage(msg)
	wantLine := "forwarded from: Kaidxen [hidden] at 2023-11-14T22:13:20Z"

	if !strings.Contains(got, wantLine) {
		t.Errorf("formatMessage() missing %q in:\n%s", wantLine, got)
	}
}

func TestFormatMessage_QuoteWithEmbeddedNewlineStaysOnOneLine(t *testing.T) {
	msg := &telegram.Message{
		ID: 10, Date: 1700000000,
		Text: "reply body",
		ReplyTo: &telegram.ReplyToInfo{
			MessageID: 5,
			QuoteText: "line 1\nline 2\nline 3",
		},
	}

	got := formatMessage(msg)

	if strings.Contains(got, "line 1\nline 2") {
		t.Errorf("quote: line must collapse embedded newlines so each line stays one key:value pair, got:\n%s",
			got)
	}

	wantLine := "quote: «line 1 line 2 line 3»"
	if !strings.Contains(got, wantLine) {
		t.Errorf("missing collapsed quote line %q in:\n%s", wantLine, got)
	}
}

func TestFormatMessage_CrossChatReplyWithQuote(t *testing.T) {
	otherPeer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 999}
	msg := &telegram.Message{
		ID: 10, Date: 1700000000,
		Text: "my reaction",
		ReplyTo: &telegram.ReplyToInfo{
			MessageID:    698,
			FromPeerID:   &otherPeer,
			FromName:     "Other Author",
			FromUsername: "otherauthor",
			QuoteText:    "key fragment",
		},
	}

	got := formatMessage(msg)
	want := strings.Join([]string{
		"[10] 2023-11-14T22:13:20Z",
		"reply to: 698 in Other Author [@otherauthor]",
		"quote: «key fragment»",
		"text:",
		"my reaction",
	}, "\n")

	if got != want {
		t.Errorf("formatMessage() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMessages_BlankLineBetweenBlocks(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 1, Date: 1700000000, Text: "first"},
		{ID: 2, Date: 1700000000, Text: "second"},
	}

	got := formatMessages(msgs)

	if !strings.Contains(got, "first\n\n[2]") {
		t.Errorf("formatMessages should separate blocks with a blank line, got:\n%s", got)
	}
}

func TestFormatMessage_LongText(t *testing.T) {
	long := strings.Repeat("a", 5000)
	msg := &telegram.Message{ID: 1, Date: 1700000000, Text: long}
	got := formatMessage(msg)

	if !strings.HasSuffix(got, long) {
		t.Errorf("formatMessage should preserve long text body verbatim, got len=%d", len(got))
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

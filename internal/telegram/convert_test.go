package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

const testMessageText = "hello world"

func TestConvertUser(t *testing.T) {
	raw := &tg.User{
		ID:        12345,
		FirstName: "Pavel",
		LastName:  "Durov",
		Username:  "durov",
		Phone:     "+1234567890",
		Bot:       false,
	}

	got := ConvertUser(raw)

	if got.ID != 12345 {
		t.Errorf("ID = %d, want 12345", got.ID)
	}

	if got.FirstName != "Pavel" {
		t.Errorf("FirstName = %q, want %q", got.FirstName, "Pavel")
	}

	if got.LastName != "Durov" {
		t.Errorf("LastName = %q, want %q", got.LastName, "Durov")
	}

	if got.Username != "durov" {
		t.Errorf("Username = %q, want %q", got.Username, "durov")
	}

	if got.Bot {
		t.Error("Bot = true, want false")
	}
}

func TestConvertUser_Nil(t *testing.T) {
	got := ConvertUser(nil)

	if got.ID != 0 {
		t.Errorf("ID = %d, want 0 for nil user", got.ID)
	}
}

func TestConvertMessage(t *testing.T) {
	raw := &tg.Message{
		ID:      42,
		Date:    1700000000,
		Message: testMessageText,
	}

	got := ConvertMessage(raw)

	if got.ID != 42 {
		t.Errorf("ID = %d, want 42", got.ID)
	}

	if got.Text != testMessageText {
		t.Errorf("Text = %q, want %q", got.Text, testMessageText)
	}

	if got.Date != 1700000000 {
		t.Errorf("Date = %d, want 1700000000", got.Date)
	}
}

func TestConvertMessage_WithPeerID(t *testing.T) {
	raw := &tg.Message{
		ID:     99,
		Date:   1700000000,
		PeerID: &tg.PeerChannel{ChannelID: 555},
	}

	got := ConvertMessage(raw)

	if got.PeerID.Type != PeerChannel {
		t.Errorf("PeerID.Type = %d, want PeerChannel", got.PeerID.Type)
	}

	if got.PeerID.ID != 555 {
		t.Errorf("PeerID.ID = %d, want 555", got.PeerID.ID)
	}
}

func TestConvertMessage_Nil(t *testing.T) {
	got := ConvertMessage(nil)

	if got.ID != 0 {
		t.Errorf("ID = %d, want 0 for nil message", got.ID)
	}
}

func TestConvertMessage_WithTopicID(t *testing.T) {
	raw := &tg.Message{
		ID:   1,
		Date: 1700000000,
	}
	raw.ReplyTo = &tg.MessageReplyHeader{}
	raw.ReplyTo.(*tg.MessageReplyHeader).SetReplyToTopID(42)
	raw.ReplyTo.(*tg.MessageReplyHeader).ForumTopic = true

	got := ConvertMessage(raw)

	if got.TopicID != 42 {
		t.Errorf("TopicID = %d, want 42", got.TopicID)
	}
}

func TestConvertMessage_WithoutTopic(t *testing.T) {
	raw := &tg.Message{
		ID:   2,
		Date: 1700000000,
	}

	got := ConvertMessage(raw)

	if got.TopicID != 0 {
		t.Errorf("TopicID = %d, want 0 for non-topic message", got.TopicID)
	}
}

func TestMessageMediaType(t *testing.T) {
	tests := []struct {
		name  string
		media tg.MessageMediaClass
		want  string
	}{
		{name: "photo", media: &tg.MessageMediaPhoto{}, want: "photo"},
		{name: "document", media: &tg.MessageMediaDocument{}, want: "document"},
		{name: "geo", media: &tg.MessageMediaGeo{}, want: "geo"},
		{name: "contact", media: &tg.MessageMediaContact{}, want: "contact"},
		{name: "venue", media: &tg.MessageMediaVenue{}, want: "venue"},
		{name: "webpage", media: &tg.MessageMediaWebPage{}, want: "webpage"},
		{name: "poll", media: &tg.MessageMediaPoll{}, want: "poll"},
		{name: "nil", media: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MessageMediaType(tt.media)
			if got != tt.want {
				t.Errorf("MessageMediaType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInputPeerToTG_User(t *testing.T) {
	peer := InputPeer{Type: PeerUser, ID: 123, AccessHash: 456}
	got := InputPeerToTG(peer)

	tgPeer, ok := got.(*tg.InputPeerUser)
	if !ok {
		t.Fatalf("expected *tg.InputPeerUser, got %T", got)
	}

	if tgPeer.UserID != 123 {
		t.Errorf("UserID = %d, want 123", tgPeer.UserID)
	}

	if tgPeer.AccessHash != 456 {
		t.Errorf("AccessHash = %d, want 456", tgPeer.AccessHash)
	}
}

func TestInputPeerToTG_Chat(t *testing.T) {
	peer := InputPeer{Type: PeerChat, ID: 789}
	got := InputPeerToTG(peer)

	tgPeer, ok := got.(*tg.InputPeerChat)
	if !ok {
		t.Fatalf("expected *tg.InputPeerChat, got %T", got)
	}

	if tgPeer.ChatID != 789 {
		t.Errorf("ChatID = %d, want 789", tgPeer.ChatID)
	}
}

func TestInputPeerToTG_Channel(t *testing.T) {
	peer := InputPeer{Type: PeerChannel, ID: 111, AccessHash: 222}
	got := InputPeerToTG(peer)

	tgPeer, ok := got.(*tg.InputPeerChannel)
	if !ok {
		t.Fatalf("expected *tg.InputPeerChannel, got %T", got)
	}

	if tgPeer.ChannelID != 111 {
		t.Errorf("ChannelID = %d, want 111", tgPeer.ChannelID)
	}

	if tgPeer.AccessHash != 222 {
		t.Errorf("AccessHash = %d, want 222", tgPeer.AccessHash)
	}
}

func TestExtractPeerID_User(t *testing.T) {
	got := extractPeerID(&tg.PeerUser{UserID: 1})

	if got.Type != PeerUser {
		t.Errorf("Type = %d, want PeerUser", got.Type)
	}

	if got.ID != 1 {
		t.Errorf("ID = %d, want 1", got.ID)
	}
}

func TestExtractPeerID_Chat(t *testing.T) {
	got := extractPeerID(&tg.PeerChat{ChatID: 2})

	if got.Type != PeerChat {
		t.Errorf("Type = %d, want PeerChat", got.Type)
	}

	if got.ID != 2 {
		t.Errorf("ID = %d, want 2", got.ID)
	}
}

func TestExtractPeerID_Channel(t *testing.T) {
	got := extractPeerID(&tg.PeerChannel{ChannelID: 3})

	if got.Type != PeerChannel {
		t.Errorf("Type = %d, want PeerChannel", got.Type)
	}

	if got.ID != 3 {
		t.Errorf("ID = %d, want 3", got.ID)
	}
}

func TestExtractPeerID_Nil(t *testing.T) {
	got := extractPeerID(nil)

	if got != (InputPeer{}) {
		t.Errorf("extractPeerID(nil) = %+v, want zero InputPeer", got)
	}
}

func TestExtractFromID_User(t *testing.T) {
	got := extractFromID(&tg.PeerUser{UserID: 10})

	if got != 10 {
		t.Errorf("extractFromID(PeerUser) = %d, want 10", got)
	}
}

func TestExtractFromID_Nil(t *testing.T) {
	got := extractFromID(nil)

	if got != 0 {
		t.Errorf("extractFromID(nil) = %d, want 0", got)
	}
}

func TestExtractReplyTo_Header(t *testing.T) {
	hdr := &tg.MessageReplyHeader{ReplyToMsgID: 77}
	got := extractReplyTo(hdr)

	if got == nil {
		t.Fatal("extractReplyTo(header) = nil, want non-nil")
	}

	if got.MessageID != 77 {
		t.Errorf("MessageID = %d, want 77", got.MessageID)
	}

	if got.TopID != 0 {
		t.Errorf("TopID = %d, want 0", got.TopID)
	}

	if got.QuoteText != "" {
		t.Errorf("QuoteText = %q, want empty", got.QuoteText)
	}

	if got.FromPeerID != nil {
		t.Errorf("FromPeerID = %+v, want nil", got.FromPeerID)
	}
}

func TestExtractReplyTo_Nil(t *testing.T) {
	got := extractReplyTo(nil)

	if got != nil {
		t.Errorf("extractReplyTo(nil) = %+v, want nil", got)
	}
}

func TestExtractReplyTo_ForumTopicNotReply(t *testing.T) {
	raw := &tg.Message{ID: 1, Date: 100}
	raw.ReplyTo = &tg.MessageReplyHeader{
		ForumTopic:   true,
		ReplyToMsgID: 99,
	}

	got := ConvertMessage(raw)

	if got.ReplyTo != nil {
		t.Errorf("ReplyTo = %+v, want nil", got.ReplyTo)
	}

	if got.TopicID != 99 {
		t.Errorf("TopicID = %d, want 99", got.TopicID)
	}
}

func TestExtractReplyTo_ZeroMsgID(t *testing.T) {
	hdr := &tg.MessageReplyHeader{ReplyToMsgID: 0}

	got := extractReplyTo(hdr)

	if got != nil {
		t.Errorf("extractReplyTo(zero id) = %+v, want nil", got)
	}
}

func TestExtractReplyTo_WithTopID(t *testing.T) {
	hdr := &tg.MessageReplyHeader{ReplyToMsgID: 50}
	hdr.SetReplyToTopID(10)

	got := extractReplyTo(hdr)

	if got == nil {
		t.Fatal("extractReplyTo = nil, want non-nil")
	}

	if got.MessageID != 50 {
		t.Errorf("MessageID = %d, want 50", got.MessageID)
	}

	if got.TopID != 10 {
		t.Errorf("TopID = %d, want 10", got.TopID)
	}
}

func TestExtractReplyTo_WithQuoteText(t *testing.T) {
	hdr := &tg.MessageReplyHeader{ReplyToMsgID: 50}
	hdr.SetQuoteText("the quoted part")

	got := extractReplyTo(hdr)

	if got == nil {
		t.Fatal("extractReplyTo = nil, want non-nil")
	}

	if got.QuoteText != "the quoted part" {
		t.Errorf("QuoteText = %q, want %q", got.QuoteText, "the quoted part")
	}
}

func TestExtractReplyTo_CrossChat(t *testing.T) {
	hdr := &tg.MessageReplyHeader{ReplyToMsgID: 50}
	hdr.SetReplyToPeerID(&tg.PeerChannel{ChannelID: 999})

	got := extractReplyTo(hdr)

	if got == nil {
		t.Fatal("extractReplyTo = nil, want non-nil")
	}

	if got.FromPeerID == nil {
		t.Fatal("FromPeerID = nil, want non-nil")
	}

	if got.FromPeerID.Type != PeerChannel {
		t.Errorf("FromPeerID.Type = %d, want PeerChannel", got.FromPeerID.Type)
	}

	if got.FromPeerID.ID != 999 {
		t.Errorf("FromPeerID.ID = %d, want 999", got.FromPeerID.ID)
	}
}

func TestConvertMessage_ForumTopicRootSelfReference(t *testing.T) {
	// Edge case: a message in a forum topic whose ReplyToMsgID equals
	// its ReplyToTopID. Current behaviour emits a ReplyToInfo pointing
	// at the topic root — the header carries that target so we surface
	// it and let the caller decide how to interpret. Documented here
	// so any future semantic change is deliberate.
	raw := &tg.Message{ID: 3, Date: 100}
	hdr := &tg.MessageReplyHeader{
		ForumTopic:   true,
		ReplyToMsgID: 42,
	}
	hdr.SetReplyToTopID(42)
	raw.ReplyTo = hdr

	got := ConvertMessage(raw)

	if got.ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want ReplyToInfo{42,42}")
	}

	if got.ReplyTo.MessageID != 42 || got.ReplyTo.TopID != 42 {
		t.Errorf("ReplyTo = %+v, want MessageID=42 TopID=42", got.ReplyTo)
	}

	if got.TopicID != 42 {
		t.Errorf("TopicID = %d, want 42", got.TopicID)
	}
}

func TestConvertMessage_ReplyInsideTopicKept(t *testing.T) {
	raw := &tg.Message{ID: 3, Date: 100}
	hdr := &tg.MessageReplyHeader{ReplyToMsgID: 200}
	hdr.SetReplyToTopID(42)
	raw.ReplyTo = hdr

	got := ConvertMessage(raw)

	if got.ReplyTo == nil {
		t.Fatal("ReplyTo = nil, want non-nil reply inside topic")
	}

	if got.ReplyTo.MessageID != 200 {
		t.Errorf("ReplyTo.MessageID = %d, want 200", got.ReplyTo.MessageID)
	}

	if got.ReplyTo.TopID != 42 {
		t.Errorf("ReplyTo.TopID = %d, want 42", got.ReplyTo.TopID)
	}

	if got.TopicID != 42 {
		t.Errorf("TopicID = %d, want 42", got.TopicID)
	}
}

func TestConvertMessage_TopicFallbackToMsgID(t *testing.T) {
	raw := &tg.Message{ID: 1, Date: 100}
	raw.ReplyTo = &tg.MessageReplyHeader{
		ForumTopic:   true,
		ReplyToMsgID: 99,
	}

	got := ConvertMessage(raw)

	if got.TopicID != 99 {
		t.Errorf("TopicID = %d, want 99 (fallback to ReplyToMsgID)", got.TopicID)
	}
}

func TestConvertMessage_GeneralTopic(t *testing.T) {
	raw := &tg.Message{ID: 1, Date: 100}
	hdr := &tg.MessageReplyHeader{
		ForumTopic:   false,
		ReplyToMsgID: 50,
	}
	hdr.SetReplyToTopID(1)
	raw.ReplyTo = hdr

	got := ConvertMessage(raw)

	if got.TopicID != 1 {
		t.Errorf("TopicID = %d, want 1 (General topic)", got.TopicID)
	}
}

package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

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
		Message: "hello world",
	}

	got := ConvertMessage(raw)

	if got.ID != 42 {
		t.Errorf("ID = %d, want 42", got.ID)
	}

	if got.Text != "hello world" {
		t.Errorf("Text = %q, want %q", got.Text, "hello world")
	}

	if got.Date != 1700000000 {
		t.Errorf("Date = %d, want 1700000000", got.Date)
	}
}

func TestConvertMessage_Nil(t *testing.T) {
	got := ConvertMessage(nil)

	if got.ID != 0 {
		t.Errorf("ID = %d, want 0 for nil message", got.ID)
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

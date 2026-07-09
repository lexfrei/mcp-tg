package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

// A channel can react to a message when it is the chat's default send-as
// identity. Its name lives in Chats, not Users, and its kind must survive
// into the domain type — otherwise the reactor renders as [user:N] and
// the id is read against the wrong id space.
func TestExtractReactionUsers_ChannelReactor(t *testing.T) {
	list := &tg.MessagesMessageReactionsList{
		Reactions: []tg.MessagePeerReaction{
			{PeerID: &tg.PeerChannel{ChannelID: ownChannelID}, Reaction: &tg.ReactionEmoji{Emoticon: "👍"}},
			{PeerID: &tg.PeerUser{UserID: selfUserID}, Reaction: &tg.ReactionEmoji{Emoticon: "🔥"}},
		},
		Chats: []tg.ChatClass{
			&tg.Channel{
				ID: ownChannelID, AccessHash: ownChannelHash,
				Title: "My Channel", Username: "mychan", Photo: &tg.ChatPhotoEmpty{},
			},
		},
		Users: []tg.UserClass{
			&tg.User{ID: selfUserID, AccessHash: selfUserHash, FirstName: "Alice", Username: "alice"},
		},
	}

	got := extractReactionUsers(list)

	if len(got) != 2 {
		t.Fatalf("got %d reactions, want 2", len(got))
	}

	channel := got[0]
	if channel.PeerType != PeerChannel {
		t.Errorf("channel reactor PeerType = %v, want PeerChannel", channel.PeerType)
	}

	if channel.Name != "My Channel" || channel.Username != "mychan" {
		t.Errorf("channel reactor = %+v, want My Channel @mychan", channel)
	}

	user := got[1]
	if user.PeerType != PeerUser {
		t.Errorf("user reactor PeerType = %v, want PeerUser", user.PeerType)
	}

	if user.Name != "Alice" || user.Username != "alice" {
		t.Errorf("user reactor = %+v, want Alice @alice", user)
	}
}

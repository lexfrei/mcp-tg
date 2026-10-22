package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

func TestDraftsFromUpdates(t *testing.T) {
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateDraftMessage{
				Peer: &tg.PeerUser{UserID: 5},
				Draft: &tg.DraftMessage{
					Message: "see you",
					Date:    100,
					ReplyTo: &tg.InputReplyToMessage{ReplyToMsgID: 7},
				},
			},
			&tg.UpdateDraftMessage{
				Peer:     &tg.PeerChannel{ChannelID: 9},
				TopMsgID: 3,
				Draft:    &tg.DraftMessage{Message: "in topic", Date: 200},
			},
			// A cleared draft arrives as draftMessageEmpty and is not a draft.
			&tg.UpdateDraftMessage{Peer: &tg.PeerUser{UserID: 6}, Draft: &tg.DraftMessageEmpty{}},
			&tg.UpdateNewMessage{},
		},
		Users: []tg.UserClass{&tg.User{ID: 5, FirstName: "Ann", Username: "ann"}},
		Chats: []tg.ChatClass{&tg.Channel{ID: 9, Title: "Team"}},
	}

	drafts := draftsFromUpdates(updates)
	if len(drafts) != 2 {
		t.Fatalf("got %d drafts, want 2: %+v", len(drafts), drafts)
	}

	first := drafts[0]
	if first.Peer.ID != 5 || first.Username != "ann" || first.Text != "see you" || first.ReplyTo != 7 {
		t.Errorf("first draft = %+v", first)
	}

	second := drafts[1]
	if second.Peer.Type != PeerChannel || second.Name != "Team" || second.TopicID != 3 {
		t.Errorf("second draft = %+v", second)
	}
}

// Telegram exports message links only for channels and supergroups; asking
// for one elsewhere must fail before a round trip, with the reason.
func TestGetMessageLink_RejectsNonChannels(t *testing.T) {
	wrapper := &Wrapper{}

	for _, peer := range []InputPeer{{Type: PeerUser, ID: 1}, {Type: PeerChat, ID: 2}} {
		_, err := wrapper.GetMessageLink(context.Background(), peer, 1, false)
		if !errors.Is(err, ErrMessageLinkUnsupported) {
			t.Errorf("peer %+v: err = %v, want ErrMessageLinkUnsupported", peer, err)
		}
	}
}

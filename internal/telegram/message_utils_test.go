package telegram

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
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
				Draft: &tg.DraftMessage{
					Message: "in topic", Date: 200,
					Media: &tg.InputMediaPhoto{ID: &tg.InputPhoto{ID: 1}},
				},
			},
			// A cleared draft arrives as draftMessageEmpty and is not a draft.
			&tg.UpdateDraftMessage{Peer: &tg.PeerUser{UserID: 6}, Draft: &tg.DraftMessageEmpty{}},
			&tg.UpdateNewMessage{},
		},
		Users: []tg.UserClass{&tg.User{ID: 5, FirstName: "Ann", Username: "ann"}},
		Chats: []tg.ChatClass{&tg.Channel{ID: 9, Title: "Team"}},
	}

	drafts := draftsFromUpdates(updates.Updates, updates.Users, updates.Chats)
	if len(drafts) != 2 {
		t.Fatalf("got %d drafts, want 2: %+v", len(drafts), drafts)
	}

	first := drafts[0]
	if first.Peer.ID != 5 || first.Username != "ann" || first.Text != "see you" || first.ReplyTo != 7 ||
		first.HasMedia {
		t.Errorf("first draft = %+v", first)
	}

	second := drafts[1]
	if second.Peer.Type != PeerChannel || second.Name != "Team" || second.TopicID != 3 ||
		!second.HasMedia {
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

// unpinInvoker answers unpinAllMessages with the scripted offsets in turn,
// and the last one for every request after them.
type unpinInvoker struct {
	offsets  []int
	requests []*tg.MessagesUnpinAllMessagesRequest
}

func (u *unpinInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesUnpinAllMessagesRequest)
	if !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	u.requests = append(u.requests, req)
	offset := u.offsets[min(len(u.requests), len(u.offsets))-1]

	return encodeAndDecode(&tg.MessagesAffectedHistory{Offset: offset}, output)
}

func TestUnpinAllMessages_RepeatsUntilTheOffsetIsZero(t *testing.T) {
	invoker := &unpinInvoker{offsets: []int{5, 3, 0}}

	err := newWrapperWithInvoker(invoker).UnpinAllMessages(t.Context(), InputPeer{Type: PeerChannel, ID: 1, AccessHash: 2}, 7)
	if err != nil {
		t.Fatalf("UnpinAllMessages: %v", err)
	}

	if len(invoker.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(invoker.requests))
	}

	if top, set := invoker.requests[0].GetTopMsgID(); !set || top != 7 {
		t.Errorf("top_msg_id = %d (set %v), want 7", top, set)
	}
}

func TestUnpinAllMessages_StopsAtTheBound(t *testing.T) {
	invoker := &unpinInvoker{offsets: []int{1}}

	err := newWrapperWithInvoker(invoker).UnpinAllMessages(t.Context(), InputPeer{Type: PeerChannel, ID: 1, AccessHash: 2}, 0)
	if !errors.Is(err, ErrUnpinIncomplete) || len(invoker.requests) != maxUnpinRounds {
		t.Errorf("err = %v after %d requests, want ErrUnpinIncomplete after %d", err, len(invoker.requests), maxUnpinRounds)
	}
}

// draftsInvoker answers getAllDrafts with a fixed envelope.
type draftsInvoker struct{ reply tg.UpdatesClass }

func (d *draftsInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetAllDraftsRequest); !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	return encodeAndDecode(d.reply, output)
}

// updatesCombined is a valid reply; reading it as "no drafts" would be a
// silent wrong answer.
func TestGetAllDrafts_ReadsTheCombinedEnvelope(t *testing.T) {
	reply := &tg.UpdatesCombined{Updates: []tg.UpdateClass{&tg.UpdateDraftMessage{
		Peer:  &tg.PeerUser{UserID: 5},
		Draft: &tg.DraftMessage{Message: "later", Date: 1},
	}}}

	drafts, err := newWrapperWithInvoker(&draftsInvoker{reply: reply}).GetAllDrafts(t.Context())
	if err != nil || len(drafts) != 1 || drafts[0].Text != "later" {
		t.Errorf("drafts = %+v, err = %v; want the one draft", drafts, err)
	}
}

func TestGetAllDrafts_UnknownShapeIsAnError(t *testing.T) {
	_, err := newWrapperWithInvoker(&draftsInvoker{reply: &tg.UpdatesTooLong{}}).GetAllDrafts(t.Context())
	if !errors.Is(err, ErrUnexpectedDraftsReply) {
		t.Errorf("err = %v, want ErrUnexpectedDraftsReply", err)
	}
}

package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// pagedDialogsInvoker serves a dialog list of total users, newest first,
// in pages no larger than the request limit, the way the server caps them.
// Each page repeats the first user, like a pinned dialog, to exercise the
// de-duplication.
type pagedDialogsInvoker struct {
	mu       sync.Mutex
	total    int
	requests []*tg.MessagesGetDialogsRequest
}

func (p *pagedDialogsInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		return errUnexpectedRequest
	}

	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()

	// Dialog i has top message i+1 dated 1000-i, so the cursor's offset
	// id tells where the previous page ended.
	start := 0
	if req.OffsetID > 0 {
		start = req.OffsetID
	}

	end := min(start+min(req.Limit, warmDialogsPageLimit), p.total)

	resp := &tg.MessagesDialogsSlice{Count: p.total}

	if start > 0 {
		resp.Dialogs = append(resp.Dialogs, &tg.Dialog{Peer: &tg.PeerUser{UserID: 1}, TopMessage: 1})
	}

	for idx := start; idx < end; idx++ {
		userID := int64(idx + 1)
		resp.Dialogs = append(resp.Dialogs, &tg.Dialog{Peer: &tg.PeerUser{UserID: userID}, TopMessage: idx + 1})
		resp.Messages = append(resp.Messages, &tg.Message{ID: idx + 1, Date: 1000 - idx, PeerID: &tg.PeerUser{UserID: userID}})
		resp.Users = append(resp.Users, userWithHash(userID, userID*7))
	}

	return encodeAndDecode(resp, output)
}

func TestGetDialogs_PagesPastServerCap(t *testing.T) {
	invoker := &pagedDialogsInvoker{total: 250}
	wrap := newWrapperWithInvoker(invoker)

	dialogs, err := wrap.GetDialogs(t.Context(), DialogOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 250 {
		t.Fatalf("got %d dialogs, want all 250", len(dialogs))
	}

	for idx, dlg := range dialogs {
		if dlg.Peer.ID != int64(idx+1) {
			t.Fatalf("dialog %d is user %d, want %d (order or de-duplication broken)", idx, dlg.Peer.ID, idx+1)
		}
	}

	if got := dialogs[249].LastMessageDate; got != 1000-249 {
		t.Errorf("last dialog LastMessageDate = %d, want %d", got, 1000-249)
	}

	for _, req := range invoker.requests {
		if req.Limit > warmDialogsPageLimit {
			t.Errorf("request limit %d exceeds the server page cap", req.Limit)
		}
	}

	second := invoker.requests[1]
	if second.OffsetID != 100 || second.OffsetDate != 1000-99 {
		t.Errorf("second page cursor = id %d date %d, want id 100 date %d", second.OffsetID, second.OffsetDate, 1000-99)
	}

	if peer, ok := second.OffsetPeer.(*tg.InputPeerUser); !ok || peer.UserID != 100 || peer.AccessHash != 700 {
		t.Errorf("second page offset peer = %#v, want user 100 with its access hash", second.OffsetPeer)
	}
}

func TestGetDialogs_StopsAtLimit(t *testing.T) {
	invoker := &pagedDialogsInvoker{total: 250}
	wrap := newWrapperWithInvoker(invoker)

	dialogs, err := wrap.GetDialogs(t.Context(), DialogOpts{Limit: 150})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 150 {
		t.Fatalf("got %d dialogs, want 150", len(dialogs))
	}

	if len(invoker.requests) != 2 || invoker.requests[1].Limit != 50 {
		t.Errorf("requests = %d, second limit = %d; want 2 requests, the second asking for the remaining 50",
			len(invoker.requests), invoker.requests[len(invoker.requests)-1].Limit)
	}
}

func TestFindMessageDate_MatchesTheChatAndServiceMessages(t *testing.T) {
	messages := []tg.MessageClass{
		// Same ID in another chat: must not be taken for the dialog's own.
		&tg.Message{ID: 5, Date: 111, PeerID: &tg.PeerUser{UserID: 9}},
		// The group's newest message is a service one ("X joined").
		&tg.MessageService{ID: 5, Date: 222, PeerID: &tg.PeerChat{ChatID: 3}, Action: &tg.MessageActionChatJoinedByLink{}},
	}

	if got := findMessageDate(messages, &tg.PeerChat{ChatID: 3}, 5); got != 222 {
		t.Errorf("date = %d, want 222 (the group's own service message)", got)
	}
}

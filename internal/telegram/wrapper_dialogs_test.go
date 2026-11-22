package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/cockroachdb/errors"
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

	// An archive request gets dialogs the server files under the archive,
	// the way the real one does; without the folder they would all be
	// filtered out and the walk would never reach a second page.
	_, archive := req.GetFolderID()

	dialog := func(userID int64, top int) *tg.Dialog {
		dlg := &tg.Dialog{Peer: &tg.PeerUser{UserID: userID}, TopMessage: top}
		if archive {
			dlg.SetFolderID(warmArchiveFolderID)
		}

		return dlg
	}

	if start > 0 {
		resp.Dialogs = append(resp.Dialogs, dialog(1, 1))
	}

	for idx := start; idx < end; idx++ {
		userID := int64(idx + 1)
		resp.Dialogs = append(resp.Dialogs, dialog(userID, idx+1))
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

// The collections are sized before the first request, so a huge limit must
// not size them: limit 1e18 used to panic in makeslice.
func TestGetDialogs_HugeLimitDoesNotPresize(t *testing.T) {
	invoker := &pagedDialogsInvoker{total: 3}
	wrap := newWrapperWithInvoker(invoker)

	dialogs, err := wrap.GetDialogs(t.Context(), DialogOpts{Limit: 1_000_000_000_000_000_000})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 3 {
		t.Errorf("got %d dialogs, want 3", len(dialogs))
	}
}

// A page shorter than requested means the list ended: no extra request.
func TestGetDialogs_ShortPageEndsTheWalk(t *testing.T) {
	invoker := &pagedDialogsInvoker{total: 250}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.GetDialogs(t.Context(), DialogOpts{Limit: 1000})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	// 100 + 100 + 50: the third page is short, so there is no fourth.
	if len(invoker.requests) != 3 {
		t.Errorf("requests = %d, want 3", len(invoker.requests))
	}
}

// scriptedDialogsInvoker answers each getDialogs with the next scripted reply.
type scriptedDialogsInvoker struct {
	replies  []func() (tg.MessagesDialogsClass, error)
	requests int
}

func (s *scriptedDialogsInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok || s.requests >= len(s.replies) {
		return errUnexpectedRequest
	}

	reply, err := s.replies[s.requests]()
	s.requests++

	if err != nil {
		return err
	}

	return encodeAndDecode(reply, output)
}

func dialogsPage(from, count int) *tg.MessagesDialogsSlice {
	page := &tg.MessagesDialogsSlice{Count: 1000}

	for idx := from; idx < from+count; idx++ {
		userID := int64(idx + 1)
		page.Dialogs = append(page.Dialogs, &tg.Dialog{Peer: &tg.PeerUser{UserID: userID}, TopMessage: idx + 1})
		page.Messages = append(page.Messages, &tg.Message{ID: idx + 1, Date: 1000 - idx, PeerID: &tg.PeerUser{UserID: userID}})
		page.Users = append(page.Users, userWithHash(userID, userID*7))
	}

	return page
}

// A failure on a later page fails the call: a partial list would read as the
// whole one.
func TestGetDialogs_LaterPageErrorFails(t *testing.T) {
	errPage := errors.New("page two failed")
	invoker := &scriptedDialogsInvoker{replies: []func() (tg.MessagesDialogsClass, error){
		func() (tg.MessagesDialogsClass, error) { return dialogsPage(0, 100), nil },
		func() (tg.MessagesDialogsClass, error) { return nil, errPage },
	}}

	dialogs, err := newWrapperWithInvoker(invoker).GetDialogs(t.Context(), DialogOpts{Limit: 300})
	if !errors.Is(err, errPage) || dialogs != nil {
		t.Errorf("dialogs = %d, err = %v; want nil and the page error", len(dialogs), err)
	}
}

// A complete messages.dialogs reply is the whole list: one request.
func TestGetDialogs_CompleteReplyEndsAfterOneCall(t *testing.T) {
	invoker := &scriptedDialogsInvoker{replies: []func() (tg.MessagesDialogsClass, error){
		func() (tg.MessagesDialogsClass, error) {
			page := dialogsPage(0, 100)

			return &tg.MessagesDialogs{Dialogs: page.Dialogs, Messages: page.Messages, Users: page.Users}, nil
		},
	}}

	dialogs, err := newWrapperWithInvoker(invoker).GetDialogs(t.Context(), DialogOpts{Limit: 300})
	if err != nil || len(dialogs) != 100 || invoker.requests != 1 {
		t.Errorf("dialogs = %d, requests = %d, err = %v; want 100 from one request", len(dialogs), invoker.requests, err)
	}
}

func TestGetDialogs_ArchivedAsksForTheArchiveFolder(t *testing.T) {
	invoker := &pagedDialogsInvoker{total: 150}
	wrap := newWrapperWithInvoker(invoker)

	dialogs, err := wrap.GetDialogs(t.Context(), DialogOpts{Limit: 150, Archived: true})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 150 || len(invoker.requests) < 2 {
		t.Fatalf("dialogs = %d over %d request(s), want 150 over more than one page", len(dialogs), len(invoker.requests))
	}

	for idx, req := range invoker.requests {
		if folderID, set := req.GetFolderID(); !set || folderID != warmArchiveFolderID {
			t.Errorf("request %d folder = %d (set %v), want the archive on every page", idx, folderID, set)
		}
	}
}

// mixedArchiveInvoker answers an archive request the way the server was
// seen to: the main list's pinned dialog first, without a folder, then the
// archived ones.
type mixedArchiveInvoker struct{}

func (mixedArchiveInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	archived := &tg.Dialog{Peer: &tg.PeerUser{UserID: 2}, TopMessage: 2}
	archived.SetFolderID(warmArchiveFolderID)

	resp := &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Pinned: true, Peer: &tg.PeerUser{UserID: 1}, TopMessage: 1},
			archived,
		},
		Users: []tg.UserClass{userWithHash(1, 10), userWithHash(2, 20)},
	}

	return encodeAndDecode(resp, output)
}

func TestGetDialogs_ArchivedDropsTheMainListsPinned(t *testing.T) {
	wrap := newWrapperWithInvoker(mixedArchiveInvoker{})

	dialogs, err := wrap.GetDialogs(t.Context(), DialogOpts{Archived: true})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 1 || dialogs[0].Peer.ID != 2 || !dialogs[0].Archived {
		t.Errorf("dialogs = %+v, want only the archived user 2", dialogs)
	}
}

// A page that held only the main list's pinned dialogs filters to nothing,
// and must not end the walk: the archived dialogs come on the next page.
func TestGetDialogs_ArchivedReadsPastAPageOfPinnedOnly(t *testing.T) {
	pinned := dialogsPage(0, 1)

	archived := dialogsPage(1, 1)
	for _, dlg := range archived.Dialogs {
		dlg.(*tg.Dialog).SetFolderID(warmArchiveFolderID)
	}

	invoker := &scriptedDialogsInvoker{replies: []func() (tg.MessagesDialogsClass, error){
		func() (tg.MessagesDialogsClass, error) { return pinned, nil },
		func() (tg.MessagesDialogsClass, error) { return archived, nil },
	}}

	dialogs, err := newWrapperWithInvoker(invoker).GetDialogs(t.Context(), DialogOpts{Limit: 1, Archived: true})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 1 || dialogs[0].Peer.ID != 2 {
		t.Errorf("dialogs = %+v, want the archived user 2 from the second page", dialogs)
	}
}

// A cursor that does not move would ask for the same page forever: here a
// full page of the main list's pinned dialog, filtered out of the archive,
// comes back on every request.
func TestGetDialogs_StopsWhenTheCursorDoesNotAdvance(t *testing.T) {
	same := func() (tg.MessagesDialogsClass, error) { return dialogsPage(0, 1), nil }
	invoker := &scriptedDialogsInvoker{replies: []func() (tg.MessagesDialogsClass, error){same, same, same}}

	dialogs, err := newWrapperWithInvoker(invoker).GetDialogs(t.Context(), DialogOpts{Limit: 1, Archived: true})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 0 || invoker.requests != 2 {
		t.Errorf("dialogs = %d, requests = %d; want none and a stop after the repeated page", len(dialogs), invoker.requests)
	}
}

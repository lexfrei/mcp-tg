package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// dialogServer answers messages.getDialogs from a list of user dialogs,
// newest first, the way the server does: a MessagesDialogsSlice whose
// dialogs carry only the peer and the top message ID, with the message
// dates in the Messages array next to them. The PeerAllowlist under test
// wraps a real Wrapper over it, so every cursor field the walk uses has to
// come out of extractDialogs.
type dialogServer struct {
	// total is the number of dialogs; endless serves full pages forever.
	total   int
	endless bool
	// perDate dialogs share each timestamp, so a date-only cursor would
	// skip the rest of a group.
	perDate int
	// inclusive repeats the cursor's own dialog at the top of the next
	// page, as a server treating the boundary as non-strict would.
	inclusive bool
	// stuck ignores the cursor and serves the first page every time.
	stuck bool

	requests []*tg.MessagesGetDialogsRequest
}

func dialogUserID(idx int) int64 { return int64(1000 + idx) }

func (server *dialogServer) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		return errUnexpectedRequest
	}

	server.requests = append(server.requests, req)

	return encodeAndDecode(server.page(server.start(req), req.Limit), output)
}

func (server *dialogServer) date(idx int) int { return 100000 - idx/max(server.perDate, 1) }

// start is the index the page begins at: after the cursor's dialog, found
// by its peer; or, with a date alone, at the first dialog older than it.
func (server *dialogServer) start(req *tg.MessagesGetDialogsRequest) int {
	if server.stuck || req.OffsetDate == 0 {
		return 0
	}

	if user, ok := req.OffsetPeer.(*tg.InputPeerUser); ok {
		idx := int(user.UserID - dialogUserID(0))
		if server.inclusive {
			return idx
		}

		return idx + 1
	}

	idx := 0
	for server.date(idx) >= req.OffsetDate {
		idx++
	}

	return idx
}

func (server *dialogServer) page(start, limit int) *tg.MessagesDialogsSlice {
	end := start + limit
	if !server.endless {
		end = min(end, server.total)
	}

	slice := &tg.MessagesDialogsSlice{Count: server.total}

	for idx := start; idx < end; idx++ {
		userID := dialogUserID(idx)
		slice.Dialogs = append(slice.Dialogs, &tg.Dialog{Peer: &tg.PeerUser{UserID: userID}, TopMessage: idx + 1})
		slice.Messages = append(slice.Messages, &tg.Message{
			ID: idx + 1, PeerID: &tg.PeerUser{UserID: userID}, Date: server.date(idx),
		})
		slice.Users = append(slice.Users, &tg.User{ID: userID, AccessHash: userID * 7})
	}

	return slice
}

func allowlistOverServer(server *dialogServer, entries ...string) *PeerAllowlist {
	return NewPeerAllowlist(newWrapperWithInvoker(server), entries)
}

func dialogIDs(dialogs []Dialog) []int64 {
	ids := make([]int64, 0, len(dialogs))
	for _, dialog := range dialogs {
		ids = append(ids, dialog.Peer.ID)
	}

	return ids
}

// The dates of the top messages live next to the dialogs, not in them:
// extractDialogs must join them, matching the chat as well as the ID.
func TestExtractDialogs_TakesTheTopMessageDate(t *testing.T) {
	dialogs := extractDialogs(&tg.MessagesDialogsSlice{
		Dialogs: []tg.DialogClass{&tg.Dialog{Peer: &tg.PeerUser{UserID: 1}, TopMessage: 5}},
		Messages: []tg.MessageClass{
			&tg.Message{ID: 5, PeerID: &tg.PeerChannel{ChannelID: 1}, Date: 111},
			&tg.Message{ID: 5, PeerID: &tg.PeerUser{UserID: 1}, Date: 222},
		},
	})

	if len(dialogs) != 1 || dialogs[0].LastMessageDate != 222 || dialogs[0].TopMessageID != 5 {
		t.Errorf("dialogs = %+v, want the user's message 5 dated 222", dialogs)
	}
}

// An allowed chat past the first source page is found, paging by the full
// cursor: dialogs 99 to 101 share a date, and a date-only cursor would skip
// 100 and 101.
func TestPeerAllowlist_DialogsPageThroughTheWrapper(t *testing.T) {
	server := &dialogServer{total: 250, perDate: 3}
	list := allowlistOverServer(server, "1100", "1149")

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 100})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if ids := dialogIDs(dialogs); len(ids) != 2 || ids[0] != 1100 || ids[1] != 1149 {
		t.Fatalf("dialogs = %v, want 1100 and 1149", ids)
	}

	if len(server.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(server.requests))
	}

	second := server.requests[1]
	peer, _ := second.OffsetPeer.(*tg.InputPeerUser)

	if second.OffsetDate != server.date(99) || second.OffsetID != 100 || peer == nil ||
		peer.UserID != 1099 || peer.AccessHash != 1099*7 {
		t.Errorf("second request cursor = (%d, %d, %v), want dialog 99's (%d, 100, user 1099)",
			second.OffsetDate, second.OffsetID, second.OffsetPeer, server.date(99))
	}
}

// Once every entry has turned up, the walk stops, though the page is short
// of the limit and the dialog list goes on.
func TestPeerAllowlist_DialogsStopOnceEveryEntryTurnedUp(t *testing.T) {
	server := &dialogServer{total: 250}
	list := allowlistOverServer(server, "1005")

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 100})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 1 || len(server.requests) != 1 {
		t.Errorf("dialogs = %v over %d request(s), want 1005 from one", dialogIDs(dialogs), len(server.requests))
	}
}

// Once the page holds limit allowed dialogs, nothing more is read.
func TestPeerAllowlist_DialogsStopAtTheLimit(t *testing.T) {
	server := &dialogServer{total: 250}
	list := allowlistOverServer(server, "1000", "1001", "1200")

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 2})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 2 || len(server.requests) != 1 {
		t.Errorf("dialogs = %v over %d request(s), want 2 from one", dialogIDs(dialogs), len(server.requests))
	}
}

// An entry that never turns up costs a bounded walk: 50 pages, 5000 dialogs.
func TestPeerAllowlist_DialogsStopAtThePageCap(t *testing.T) {
	server := &dialogServer{endless: true}
	list := allowlistOverServer(server, "1")

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 100})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(dialogs) != 0 || len(server.requests) != allowlistDialogsMaxPages {
		t.Errorf("dialogs = %v over %d request(s), want none over %d",
			dialogIDs(dialogs), len(server.requests), allowlistDialogsMaxPages)
	}
}

// A cursor that does not move would read the same page forever.
func TestPeerAllowlist_DialogsStopOnAStuckCursor(t *testing.T) {
	server := &dialogServer{total: 250, stuck: true}
	list := allowlistOverServer(server, "1")

	_, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 100})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if len(server.requests) != 2 {
		t.Errorf("requests = %d, want 2: the repeat shows the cursor is stuck", len(server.requests))
	}
}

// A server that repeats the boundary dialog must not list it twice.
func TestPeerAllowlist_DialogsDedupeOverlappingPages(t *testing.T) {
	server := &dialogServer{total: 250, inclusive: true}
	list := allowlistOverServer(server, "1099", "1150")

	dialogs, err := list.GetDialogs(context.Background(), DialogOpts{Limit: 100})
	if err != nil {
		t.Fatalf("GetDialogs: %v", err)
	}

	if ids := dialogIDs(dialogs); len(ids) != 2 || ids[0] != 1099 || ids[1] != 1150 {
		t.Errorf("dialogs = %v, want 1099 and 1150 once each", ids)
	}

	if len(server.requests) != 2 {
		t.Errorf("requests = %d, want 2", len(server.requests))
	}
}

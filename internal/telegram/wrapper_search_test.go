package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// searchInvoker captures the outgoing search RPCs so tests can assert
// which conditional fields the wrapper set, and answers each with a
// canned response.
type searchInvoker struct {
	mu     sync.Mutex
	search *tg.MessagesSearchRequest
	global *tg.MessagesSearchGlobalRequest
	resp   tg.MessagesMessagesClass
}

func (s *searchInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch req := input.(type) {
	case *tg.MessagesSearchRequest:
		s.search = req

		return encodeResp(s.resp, output)
	case *tg.MessagesSearchGlobalRequest:
		s.global = req

		return encodeResp(s.resp, output)
	default:
		return errUnexpectedRequest
	}
}

func newSearchWrapper(inv *searchInvoker) *Wrapper {
	return &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}
}

func searchSliceResponse() *tg.MessagesMessagesSlice {
	return &tg.MessagesMessagesSlice{
		Count: 42,
		Messages: []tg.MessageClass{
			&tg.Message{ID: 10, Message: "hit", PeerID: &tg.PeerUser{UserID: 1}},
		},
	}
}

func searchChatPeer() InputPeer {
	return InputPeer{Type: PeerChannel, ID: 111, AccessHash: 222}
}

func TestSearchMessages_DefaultsLeaveConditionalFieldsUnset(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}

	_, _, err := newSearchWrapper(inv).SearchMessages(context.Background(), searchChatPeer(), "q", SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := inv.search.Filter.(*tg.InputMessagesFilterEmpty); !ok {
		t.Errorf("Filter = %T, want *tg.InputMessagesFilterEmpty", inv.search.Filter)
	}

	if _, ok := inv.search.GetTopMsgID(); ok {
		t.Error("TopMsgID must stay unset without a TopicID")
	}

	if _, ok := inv.search.GetFromID(); ok {
		t.Error("FromID must stay unset without a sender filter")
	}

	if inv.search.Limit != defaultLimit {
		t.Errorf("Limit = %d, want default %d", inv.search.Limit, defaultLimit)
	}
}

func TestSearchMessages_SetsTopicAndSender(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}
	sender := InputPeer{Type: PeerUser, ID: 9, AccessHash: 8}

	_, _, err := newSearchWrapper(inv).SearchMessages(
		context.Background(), searchChatPeer(), "q", SearchOpts{TopicID: 33, FromID: &sender},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	topicID, ok := inv.search.GetTopMsgID()
	if !ok || topicID != 33 {
		t.Errorf("TopMsgID = %d (set=%v), want 33 set", topicID, ok)
	}

	fromID, ok := inv.search.GetFromID()
	if !ok {
		t.Fatal("FromID must be set")
	}

	fromUser, ok := fromID.(*tg.InputPeerUser)
	if !ok || fromUser.UserID != 9 || fromUser.AccessHash != 8 {
		t.Errorf("FromID = %#v, want InputPeerUser{9, 8}", fromID)
	}
}

func TestSearchMessages_SetsFilterAndDates(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}

	_, _, err := newSearchWrapper(inv).SearchMessages(
		context.Background(), searchChatPeer(), "q",
		SearchOpts{Filter: SearchFilterPhotos, MinDate: 100, MaxDate: 200},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := inv.search.Filter.(*tg.InputMessagesFilterPhotos); !ok {
		t.Errorf("Filter = %T, want *tg.InputMessagesFilterPhotos", inv.search.Filter)
	}

	if inv.search.MinDate != 100 || inv.search.MaxDate != 200 {
		t.Errorf("MinDate/MaxDate = %d/%d, want 100/200", inv.search.MinDate, inv.search.MaxDate)
	}
}

func TestSearchMessages_ReturnsServerTotal(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}

	msgs, total, err := newSearchWrapper(inv).SearchMessages(
		context.Background(), searchChatPeer(), "q", SearchOpts{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}

	if len(msgs) != 1 {
		t.Errorf("len(msgs) = %d, want 1", len(msgs))
	}
}

func TestSearchMessages_UnknownFilterFailsBeforeRPC(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}

	_, _, err := newSearchWrapper(inv).SearchMessages(
		context.Background(), searchChatPeer(), "q", SearchOpts{Filter: "bogus"},
	)
	if err == nil {
		t.Fatal("expected an error for an unknown filter")
	}

	if inv.search != nil {
		t.Error("the RPC must not fire when the filter name is invalid")
	}
}

func TestSearchGlobal_FirstPageDefaults(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}

	_, err := newSearchWrapper(inv).SearchGlobal(context.Background(), "q", &SearchGlobalOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := inv.global.OffsetPeer.(*tg.InputPeerEmpty); !ok {
		t.Errorf("OffsetPeer = %T, want *tg.InputPeerEmpty", inv.global.OffsetPeer)
	}

	if inv.global.UsersOnly || inv.global.GroupsOnly || inv.global.BroadcastsOnly {
		t.Error("scope flags must stay clear without a scope")
	}

	if inv.global.Limit != defaultLimit {
		t.Errorf("Limit = %d, want default %d", inv.global.Limit, defaultLimit)
	}
}

func TestSearchGlobal_ThreadsCursorAndScope(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}
	cursor := InputPeer{Type: PeerChannel, ID: 555, AccessHash: 666}

	_, err := newSearchWrapper(inv).SearchGlobal(context.Background(), "q", &SearchGlobalOpts{
		Scope:      SearchScopeChannels,
		OffsetRate: 7,
		OffsetID:   10,
		OffsetPeer: &cursor,
		MinDate:    100,
		MaxDate:    200,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !inv.global.BroadcastsOnly || inv.global.UsersOnly || inv.global.GroupsOnly {
		t.Error("channels scope must set only BroadcastsOnly")
	}

	if inv.global.OffsetRate != 7 || inv.global.OffsetID != 10 {
		t.Errorf("OffsetRate/OffsetID = %d/%d, want 7/10", inv.global.OffsetRate, inv.global.OffsetID)
	}

	channel, ok := inv.global.OffsetPeer.(*tg.InputPeerChannel)
	if !ok || channel.ChannelID != 555 {
		t.Errorf("OffsetPeer = %#v, want InputPeerChannel{555}", inv.global.OffsetPeer)
	}

	if inv.global.MinDate != 100 || inv.global.MaxDate != 200 {
		t.Errorf("MinDate/MaxDate = %d/%d, want 100/200", inv.global.MinDate, inv.global.MaxDate)
	}
}

func TestSearchGlobal_ExtractsNextRateAndSeedsCache(t *testing.T) {
	resp := searchSliceResponse()
	resp.SetNextRate(99)
	// Photo is a required field on channel#, not a conditional one,
	// so a nil there fails encoding rather than decoding as absent.
	resp.Chats = []tg.ChatClass{&tg.Channel{ID: 555, AccessHash: 666, Photo: &tg.ChatPhotoEmpty{}}}
	inv := &searchInvoker{resp: resp}
	wrapper := newSearchWrapper(inv)

	page, err := wrapper.SearchGlobal(context.Background(), "q", &SearchGlobalOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if page.NextRate != 99 {
		t.Errorf("NextRate = %d, want 99", page.NextRate)
	}

	if page.Total != 42 {
		t.Errorf("Total = %d, want 42", page.Total)
	}

	cached, ok := wrapper.cache.Lookup(PeerChannel, 555)
	if !ok || cached.AccessHash != 666 {
		t.Errorf("cache.Lookup(channel 555) = %+v (found=%v), want the reply's access hash", cached, ok)
	}
}

// TestSearchGlobal_ScopeMapsToExactlyOneFlag pins the wire mapping of
// every scope value: a copy-paste swap of the three comparisons would
// pass the rest of the suite, since handler tests stop at the opts
// string and only the channels case asserted the TL flags.
func TestSearchGlobal_ScopeMapsToExactlyOneFlag(t *testing.T) {
	cases := []struct {
		scope                     string
		users, groups, broadcasts bool
	}{
		{scope: SearchScopeUsers, users: true},
		{scope: SearchScopeGroups, groups: true},
		{scope: SearchScopeChannels, broadcasts: true},
	}

	for _, tc := range cases {
		inv := &searchInvoker{resp: searchSliceResponse()}

		_, err := newSearchWrapper(inv).SearchGlobal(context.Background(), "q", &SearchGlobalOpts{Scope: tc.scope})
		if err != nil {
			t.Fatalf("scope %q: unexpected error: %v", tc.scope, err)
		}

		got := [3]bool{inv.global.UsersOnly, inv.global.GroupsOnly, inv.global.BroadcastsOnly}
		want := [3]bool{tc.users, tc.groups, tc.broadcasts}

		if got != want {
			t.Errorf("scope %q: flags [users groups broadcasts] = %v, want %v", tc.scope, got, want)
		}
	}
}

// TestSearchGlobal_NextRateFallsBackToLastMessageDate pins the
// documented cursor contract: when messages.messagesSlice carries no
// next_rate, the caller must continue with the date of the last
// returned message. Returning 0 there would break pagination for
// exactly those pages.
func TestSearchGlobal_NextRateFallsBackToLastMessageDate(t *testing.T) {
	resp := &tg.MessagesMessagesSlice{
		Count: 42,
		Messages: []tg.MessageClass{
			&tg.Message{ID: 11, Date: 2000, Message: "newer", PeerID: &tg.PeerUser{UserID: 1}},
			&tg.Message{ID: 10, Date: 1000, Message: "older", PeerID: &tg.PeerUser{UserID: 1}},
		},
	}
	inv := &searchInvoker{resp: resp}

	page, err := newSearchWrapper(inv).SearchGlobal(context.Background(), "q", &SearchGlobalOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if page.NextRate != 1000 {
		t.Errorf("NextRate = %d, want the last message's date 1000", page.NextRate)
	}
}

// TestSearchGlobal_ChannelMessagesSeedsCache pins the symmetry with
// extractMessages: the schema reserves messages.channelMessages for
// peer-scoped requests, but if the server ever answers a global search
// with it, the peers must still land in the cache and the total must
// still be extracted.
func TestSearchGlobal_ChannelMessagesSeedsCache(t *testing.T) {
	inv := &searchInvoker{resp: &tg.MessagesChannelMessages{
		Count: 7,
		Messages: []tg.MessageClass{
			&tg.Message{ID: 10, Message: "hit", PeerID: &tg.PeerChannel{ChannelID: 555}},
		},
		Chats: []tg.ChatClass{&tg.Channel{ID: 555, AccessHash: 666, Photo: &tg.ChatPhotoEmpty{}}},
	}}
	wrapper := newSearchWrapper(inv)

	page, err := wrapper.SearchGlobal(context.Background(), "q", &SearchGlobalOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if page.Total != 7 {
		t.Errorf("Total = %d, want 7", page.Total)
	}

	if _, ok := wrapper.cache.Lookup(PeerChannel, 555); !ok {
		t.Error("the reply's channel must be cached")
	}
}

func TestSearchGlobal_UnknownFilterFailsBeforeRPC(t *testing.T) {
	inv := &searchInvoker{resp: searchSliceResponse()}

	_, err := newSearchWrapper(inv).SearchGlobal(
		context.Background(), "q", &SearchGlobalOpts{Filter: "bogus"},
	)
	if err == nil {
		t.Fatal("expected an error for an unknown filter")
	}

	if inv.global != nil {
		t.Error("the RPC must not fire when the filter name is invalid")
	}
}

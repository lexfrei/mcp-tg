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

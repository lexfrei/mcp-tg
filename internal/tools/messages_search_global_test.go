package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestMessagesSearchGlobalHandler_ThreadsOptsThrough(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 555, AccessHash: 666}}
	handler := NewMessagesSearchGlobalHandler(mock)

	minDate, maxDate, limit, offsetRate, offsetID := 100, 200, 5, 7, 10
	_, _, err := handler(context.Background(), nil, MessagesSearchGlobalParams{
		Query:      "q",
		Filter:     telegram.SearchFilterPhotos,
		MinDate:    &minDate,
		MaxDate:    &maxDate,
		Scope:      telegram.SearchScopeChannels,
		Limit:      &limit,
		OffsetRate: &offsetRate,
		OffsetID:   &offsetID,
		OffsetPeer: "-100555",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opts := mock.lastSearchGlobalOpts
	if opts.MinDate != 100 || opts.MaxDate != 200 || opts.Limit != 5 {
		t.Errorf("opts = %+v, want dates 100/200 and limit 5", opts)
	}

	if opts.Filter != telegram.SearchFilterPhotos || opts.Scope != telegram.SearchScopeChannels {
		t.Errorf("filter/scope = %q/%q, want photos/channels", opts.Filter, opts.Scope)
	}

	if opts.OffsetRate != 7 || opts.OffsetID != 10 {
		t.Errorf("cursor = %d/%d, want 7/10", opts.OffsetRate, opts.OffsetID)
	}

	if opts.OffsetPeer == nil || opts.OffsetPeer.ID != 555 {
		t.Errorf("OffsetPeer = %+v, want the resolved cursor peer", opts.OffsetPeer)
	}
}

func TestMessagesSearchGlobalHandler_FirstPageResolvesNoPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesSearchGlobalParams{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.resolvedQueries) != 0 {
		t.Errorf("resolvedQueries = %v, want none on the first page", mock.resolvedQueries)
	}

	if mock.lastSearchGlobalOpts.OffsetPeer != nil {
		t.Errorf("OffsetPeer = %+v, want nil", mock.lastSearchGlobalOpts.OffsetPeer)
	}
}

func TestMessagesSearchGlobalHandler_NextRateAndTotalPropagated(t *testing.T) {
	mock := &mockClient{messages: messagesWithReply(), total: 42, nextRate: 99}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSearchGlobalParams{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.NextRate != 99 {
		t.Errorf("NextRate = %d, want 99", res.NextRate)
	}

	if res.Total != 42 {
		t.Errorf("Total = %d, want 42", res.Total)
	}
}

// TestMessagesSearchGlobalHandler_ReadyMadeCursor pins that the result
// carries the full next-page cursor in directly reusable form: the JSON
// messages[].peerId is a structured object, so callers would otherwise
// have to convert it to the bot-style string offsetPeer expects.
func TestMessagesSearchGlobalHandler_ReadyMadeCursor(t *testing.T) {
	msgs := []telegram.Message{
		{ID: 20, Date: 2000, Text: "newer", PeerID: telegram.InputPeer{Type: telegram.PeerUser, ID: 5}},
		{ID: 10, Date: 1000, Text: "older", PeerID: telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}},
	}
	mock := &mockClient{messages: msgs, total: 42, nextRate: 99}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSearchGlobalParams{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.NextOffsetID != 10 {
		t.Errorf("NextOffsetID = %d, want the last message's id 10", res.NextOffsetID)
	}

	if res.NextOffsetPeer != "-1000000000555" {
		t.Errorf("NextOffsetPeer = %q, want the bot-style channel ID", res.NextOffsetPeer)
	}
}

// TestMessagesSearchGlobalHandler_OffsetPeerResolveFailureNamesTheParam
// pins that a failing cursor-peer resolution blames the offsetPeer
// parameter, since the tool has no other peer argument to confuse it
// with but the error would otherwise read as a generic resolve failure.
func TestMessagesSearchGlobalHandler_OffsetPeerResolveFailureNamesTheParam(t *testing.T) {
	mock := &mockClient{
		resolvePeerFn: func(string) (telegram.InputPeer, error) {
			return telegram.InputPeer{}, errors.New("PEER_ID_INVALID")
		},
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	res, _, err := handler(context.Background(), nil,
		MessagesSearchGlobalParams{Query: "q", OffsetPeer: "-100999"})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve the offsetPeer peer") {
		t.Errorf("err = %v, want it to name the offsetPeer parameter", err)
	}

	if res == nil || !res.IsError {
		t.Error("result must be marked IsError")
	}
}

// TestMessagesSearchGlobalHandler_UnresolvedOffsetPeerRejected pins the
// cold-cache path: after a daemon restart the previous page's numeric
// peer resolves with a zero access hash, and sending it on would fail
// with a server error naming neither the parameter nor the remedy.
func TestMessagesSearchGlobalHandler_UnresolvedOffsetPeerRejected(t *testing.T) {
	mock := &mockClient{
		resolvePeerFn: func(string) (telegram.InputPeer, error) {
			return telegram.InputPeer{Type: telegram.PeerChannel, ID: 555}, nil
		},
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, _, err := handler(context.Background(), nil,
		MessagesSearchGlobalParams{Query: "q", OffsetPeer: "-1000000000555"})
	if !errors.Is(err, ErrOffsetPeerUnresolved) {
		t.Errorf("err = %v, want ErrOffsetPeerUnresolved", err)
	}
}

// TestMessagesSearchGlobalHandler_HasMoreFollowsCursor pins that
// hasMore is derived from the cursor, not from page saturation: a
// complete result carries no nextRate, and a full final page must not
// advertise a next page that does not exist.
func TestMessagesSearchGlobalHandler_HasMoreFollowsCursor(t *testing.T) {
	limit := 2
	mock := &mockClient{messages: messagesWithReply(), total: 2, nextRate: 0}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(context.Background(), nil,
		MessagesSearchGlobalParams{Query: "q", Limit: &limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.HasMore {
		t.Error("a full page without a cursor must report hasMore=false")
	}

	mock.nextRate = 99

	_, res, err = handler(context.Background(), nil,
		MessagesSearchGlobalParams{Query: "q", Limit: &limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.HasMore {
		t.Error("a page with a cursor must report hasMore=true")
	}
}

func TestMessagesSearchGlobalHandler_EmptyPageHasNoCursor(t *testing.T) {
	handler := NewMessagesSearchGlobalHandler(&mockClient{})

	_, res, err := handler(context.Background(), nil, MessagesSearchGlobalParams{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.NextOffsetID != 0 || res.NextOffsetPeer != "" {
		t.Errorf("empty page must carry no cursor, got id=%d peer=%q", res.NextOffsetID, res.NextOffsetPeer)
	}
}

func TestMessagesSearchGlobalHandler_EmptyQueryWithFilterAllowed(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, _, err := handler(context.Background(), nil,
		MessagesSearchGlobalParams{Filter: telegram.SearchFilterPhotos})
	if err != nil {
		t.Fatalf("empty query with a filter must be a valid 'all photos' search, got: %v", err)
	}
}

func TestMessagesSearchGlobalHandler_EmptyQueryWithoutFilterRejected(t *testing.T) {
	handler := NewMessagesSearchGlobalHandler(&mockClient{})

	_, _, err := handler(context.Background(), nil, MessagesSearchGlobalParams{})
	if !errors.Is(err, ErrQueryOrFilterRequired) {
		t.Errorf("err = %v, want ErrQueryOrFilterRequired", err)
	}
}

func TestMessagesSearchGlobalHandler_UnknownScope(t *testing.T) {
	handler := NewMessagesSearchGlobalHandler(&mockClient{})

	_, _, err := handler(context.Background(), nil,
		MessagesSearchGlobalParams{Query: "q", Scope: "everything"})
	if !errors.Is(err, ErrUnknownSearchScope) {
		t.Errorf("err = %v, want ErrUnknownSearchScope", err)
	}
}

func TestMessagesSearchGlobalHandler_UnknownFilter(t *testing.T) {
	handler := NewMessagesSearchGlobalHandler(&mockClient{})

	_, _, err := handler(context.Background(), nil,
		MessagesSearchGlobalParams{Query: "q", Filter: "bogus"})
	if !errors.Is(err, ErrUnknownMessageFilter) {
		t.Errorf("err = %v, want ErrUnknownMessageFilter", err)
	}
}

func TestMessagesSearchGlobalHandler_InvertedDateRange(t *testing.T) {
	handler := NewMessagesSearchGlobalHandler(&mockClient{})

	minDate, maxDate := 200, 100
	_, _, err := handler(context.Background(), nil, MessagesSearchGlobalParams{
		Query: "q", MinDate: &minDate, MaxDate: &maxDate,
	})
	if !errors.Is(err, ErrInvalidDateRange) {
		t.Errorf("err = %v, want ErrInvalidDateRange", err)
	}
}

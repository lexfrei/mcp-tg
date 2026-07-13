package tools

import (
	"context"
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

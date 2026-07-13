package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func searchRequest() *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}}
}

func TestMessagesSearchHandler_ThreadsOptsThrough(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1, AccessHash: 2}}
	handler := NewMessagesSearchHandler(mock)

	topicID, minDate, maxDate, limit, offsetID := 33, 100, 200, 5, 777
	_, _, err := handler(context.Background(), searchRequest(), MessagesSearchParams{
		Peer:     testChatPeer,
		Query:    "q",
		TopicID:  &topicID,
		From:     "@sender",
		Filter:   telegram.SearchFilterPhotos,
		MinDate:  &minDate,
		MaxDate:  &maxDate,
		Limit:    &limit,
		OffsetID: &offsetID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opts := mock.lastSearchOpts
	if opts.TopicID != 33 || opts.MinDate != 100 || opts.MaxDate != 200 || opts.Limit != 5 || opts.OffsetID != 777 {
		t.Errorf("opts = %+v, want topic 33, dates 100/200, limit 5, offset 777", opts)
	}

	if opts.Filter != telegram.SearchFilterPhotos {
		t.Errorf("opts.Filter = %q, want %q", opts.Filter, telegram.SearchFilterPhotos)
	}

	if opts.FromID == nil || opts.FromID.ID != 1 {
		t.Errorf("opts.FromID = %+v, want the resolved sender", opts.FromID)
	}

	if len(mock.resolvedQueries) != 2 || mock.resolvedQueries[1] != "@sender" {
		t.Errorf("resolvedQueries = %v, want [peer, @sender]", mock.resolvedQueries)
	}
}

func TestMessagesSearchHandler_NoOptionalParamsLeaveOptsZero(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1}}
	handler := NewMessagesSearchHandler(mock)

	_, _, err := handler(context.Background(), searchRequest(),
		MessagesSearchParams{Peer: testChatPeer, Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	opts := mock.lastSearchOpts
	if opts.TopicID != 0 || opts.FromID != nil || opts.Filter != "" || opts.MinDate != 0 || opts.MaxDate != 0 {
		t.Errorf("opts = %+v, want zero conditional fields", opts)
	}

	if len(mock.resolvedQueries) != 1 {
		t.Errorf("resolvedQueries = %v, want only the chat peer", mock.resolvedQueries)
	}
}

func TestMessagesSearchHandler_UnknownFilter(t *testing.T) {
	handler := NewMessagesSearchHandler(&mockClient{})

	res, _, err := handler(context.Background(), searchRequest(),
		MessagesSearchParams{Peer: testChatPeer, Query: "q", Filter: "bogus"})
	if !errors.Is(err, ErrUnknownMessageFilter) {
		t.Errorf("err = %v, want ErrUnknownMessageFilter", err)
	}

	if res == nil || !res.IsError {
		t.Error("result must be marked IsError")
	}
}

func TestMessagesSearchHandler_InvertedDateRange(t *testing.T) {
	handler := NewMessagesSearchHandler(&mockClient{})

	minDate, maxDate := 200, 100
	_, _, err := handler(context.Background(), searchRequest(), MessagesSearchParams{
		Peer: testChatPeer, Query: "q", MinDate: &minDate, MaxDate: &maxDate,
	})
	if !errors.Is(err, ErrInvalidDateRange) {
		t.Errorf("err = %v, want ErrInvalidDateRange", err)
	}
}

// TestMessagesSearchHandler_FromResolveFailureNamesTheParam pins the
// whole point of resolveOptionalPeer's paramName argument: a failing
// sender resolution must blame the from parameter, not the chat peer.
func TestMessagesSearchHandler_FromResolveFailureNamesTheParam(t *testing.T) {
	mock := &mockClient{
		resolvePeerFn: func(identifier string) (telegram.InputPeer, error) {
			if identifier == "@ghost" {
				return telegram.InputPeer{}, errors.New("USERNAME_NOT_OCCUPIED")
			}

			return telegram.InputPeer{Type: telegram.PeerChannel, ID: 1}, nil
		},
	}
	handler := NewMessagesSearchHandler(mock)

	res, _, err := handler(context.Background(), searchRequest(),
		MessagesSearchParams{Peer: testChatPeer, Query: "q", From: "@ghost"})
	if err == nil || !strings.Contains(err.Error(), "failed to resolve the from peer") {
		t.Errorf("err = %v, want it to name the from parameter", err)
	}

	if res == nil || !res.IsError {
		t.Error("result must be marked IsError")
	}
}

func TestMessagesSearchHandler_TotalPropagated(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		messages: messagesWithReply(),
		total:    42,
	}
	handler := NewMessagesSearchHandler(mock)

	_, res, err := handler(context.Background(), searchRequest(),
		MessagesSearchParams{Peer: testChatPeer, Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Total != 42 {
		t.Errorf("Total = %d, want 42", res.Total)
	}

	if res.Count != 2 {
		t.Errorf("Count = %d, want 2", res.Count)
	}
}

package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestMessagesListHandler_RejectsLimitAboveCap(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesListHandler(mock)

	over := maxMessagesListLimit + 1

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", Limit: &over})
	if err == nil {
		t.Fatal("expected an error for a limit above the cap")
	}

	if !errors.Is(err, ErrLimitTooLarge) {
		t.Errorf("error = %v, want ErrLimitTooLarge", err)
	}

	if mock.getHistoryCalls != 0 {
		t.Errorf("getHistoryCalls = %d, want 0 (rejected before any RPC)", mock.getHistoryCalls)
	}
}

// The non-type path must surface the wrapper paginator's hasMore, not
// derive it from len-vs-limit: when the scan cap stops the walk with a
// short batch, len < limit would wrongly report "no more" and strand a
// caller. Here the pager signals more despite returning a single message.
func TestMessagesListHandler_HasMoreReflectsPagerSignal(t *testing.T) {
	limit := 100
	mock := &mockClient{
		peer:           telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		total:          5000,
		historyHasMore: true,
		messages:       []telegram.Message{{ID: 5, Type: "text", Text: "hi", Date: 1000}},
	}

	_, res, err := NewMessagesListHandler(mock)(context.Background(), nil, MessagesListParams{Peer: "@x", Limit: &limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.HasMore {
		t.Errorf("HasMore = false, want true (pager signalled more despite a short batch of %d)", res.Count)
	}

	// And the inverse: a false pager signal yields HasMore false even
	// when the batch happened to fill the requested limit.
	mockDone := &mockClient{
		peer:           telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		total:          1,
		historyHasMore: false,
		messages:       []telegram.Message{{ID: 5, Type: "text", Text: "hi", Date: 1000}},
	}

	_, resDone, err := NewMessagesListHandler(mockDone)(context.Background(), nil, MessagesListParams{Peer: "@x", Limit: &limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resDone.HasMore {
		t.Error("HasMore = true, want false (pager signalled no more)")
	}
}

func TestMessagesListHandler_RejectsNegativeLimit(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesListHandler(mock)

	neg := -1

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", Limit: &neg})
	if err == nil {
		t.Fatal("expected an error for a negative limit")
	}

	if !errors.Is(err, ErrNegativeLimit) {
		t.Errorf("error = %v, want ErrNegativeLimit", err)
	}

	if mock.getHistoryCalls != 0 {
		t.Errorf("getHistoryCalls = %d, want 0 (rejected before any RPC)", mock.getHistoryCalls)
	}
}

func TestMessagesListHandler_AllowsLimitAtCap(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesListHandler(mock)

	atCap := maxMessagesListLimit

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", Limit: &atCap})
	if err != nil {
		t.Fatalf("unexpected error at the cap: %v", err)
	}
}

// The type-filter path drives its own outer pagination loop, so each RPC
// it issues must be bounded to a single server page — otherwise it would
// double-page against GetHistory's internal pager.
func TestMessagesListHandler_TypeFilterBoundsPerRPCToOnePage(t *testing.T) {
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		total:    5,
		messages: []telegram.Message{{ID: 5, Type: "text", Text: "hi", Date: 1000}},
	}
	handler := NewMessagesListHandler(mock)

	limit := 250

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", Type: "text", Limit: &limit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.getHistoryOpts) == 0 {
		t.Fatal("expected at least one GetHistory call")
	}

	if got := mock.getHistoryOpts[0].Limit; got != telegram.DefaultLimit {
		t.Errorf("per-RPC Limit = %d, want %d (one server page)", got, telegram.DefaultLimit)
	}
}

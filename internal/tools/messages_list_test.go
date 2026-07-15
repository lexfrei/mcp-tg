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

func TestMessagesListHandler_ThreadsDateRangeIntoHistoryOpts(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesListHandler(mock)

	from := 1000
	to := 2000

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", FromDate: &from, ToDate: &to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.getHistoryOpts) == 0 {
		t.Fatal("expected a GetHistory call")
	}

	opts := mock.getHistoryOpts[0]
	if opts.OffsetDate != to {
		t.Errorf("OffsetDate = %d, want %d (toDate anchors the newest message)", opts.OffsetDate, to)
	}

	if opts.MinDate != from {
		t.Errorf("MinDate = %d, want %d (fromDate is the client-side floor)", opts.MinDate, from)
	}
}

func TestMessagesListHandler_RejectsInvertedDateRange(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesListHandler(mock)

	from := 2000
	to := 1000

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", FromDate: &from, ToDate: &to})
	if err == nil {
		t.Fatal("expected an error for fromDate > toDate")
	}

	if !errors.Is(err, ErrInvalidDateRange) {
		t.Errorf("error = %v, want ErrInvalidDateRange", err)
	}

	if mock.getHistoryCalls != 0 {
		t.Errorf("getHistoryCalls = %d, want 0 (rejected before any RPC)", mock.getHistoryCalls)
	}
}

func TestMessagesListHandler_RejectsNegativeDate(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}}
	handler := NewMessagesListHandler(mock)

	from := -1

	_, _, err := handler(context.Background(), nil, MessagesListParams{Peer: "@x", FromDate: &from})
	if err == nil {
		t.Fatal("expected an error for a negative date")
	}

	if !errors.Is(err, ErrNegativeDate) {
		t.Errorf("error = %v, want ErrNegativeDate", err)
	}
}

// The type-filter path applies toDate only to its first RPC (offset_date
// anchors the first page) while the fromDate floor rides every page.
func TestMessagesListHandler_TypeFilterThreadsDateRange(t *testing.T) {
	from := 500
	to := 3000
	mock := &mockClient{
		peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		total:    1,
		messages: []telegram.Message{{ID: 5, Type: "text", Text: "hi", Date: 1000}},
	}
	handler := NewMessagesListHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesListParams{
		Peer: "@x", Type: "text", FromDate: &from, ToDate: &to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first := mock.getHistoryOpts[0]
	if first.OffsetDate != to {
		t.Errorf("first-page OffsetDate = %d, want %d", first.OffsetDate, to)
	}

	if first.MinDate != from {
		t.Errorf("first-page MinDate = %d, want %d", first.MinDate, from)
	}
}

// Across multiple type-filter pages, offset_date must anchor only the
// first RPC; later pages advance by offset_id and carry no offset_date,
// while the fromDate floor rides every page.
func TestMessagesListHandler_TypeFilterOffsetDateFirstPageOnly(t *testing.T) {
	from := 500
	to := 3000
	mock := &mockClient{
		getHistoryFn: func(_ telegram.InputPeer, opts telegram.HistoryOpts) ([]telegram.Message, int, error) {
			switch opts.OffsetID {
			case 0:
				return typeFilterPage(1000, telegram.DefaultLimit, "photo"), 104, nil
			case 901:
				return []telegram.Message{
					{ID: 900, Date: 998, Type: "voice"},
					{ID: 899, Date: 997, Type: "voice"},
				}, 104, nil
			default:
				return nil, 104, nil
			}
		},
	}
	handler := NewMessagesListHandler(mock)

	limit := 2

	_, _, err := handler(context.Background(), nil, MessagesListParams{
		Peer: "@x", Type: "voice", Limit: &limit, FromDate: &from, ToDate: &to,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.getHistoryOpts) < 2 {
		t.Fatalf("expected at least two GetHistory calls, got %d", len(mock.getHistoryOpts))
	}

	if got := mock.getHistoryOpts[0].OffsetDate; got != to {
		t.Errorf("first-page OffsetDate = %d, want %d", got, to)
	}

	if got := mock.getHistoryOpts[1].OffsetDate; got != 0 {
		t.Errorf("second-page OffsetDate = %d, want 0 (offset_date anchors the first page only)", got)
	}

	if got := mock.getHistoryOpts[1].MinDate; got != from {
		t.Errorf("second-page MinDate = %d, want %d (fromDate floor rides every page)", got, from)
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

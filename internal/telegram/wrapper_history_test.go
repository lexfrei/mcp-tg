package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// Static sentinels for the error-path tests — err113 forbids dynamic
// errors.New inside a function body.
var (
	errTestFetchBoom = errors.New("fetch boom")
	errTestRPCBoom   = errors.New("rpc boom")
)

// histCall records one fetch invocation so tests can assert how the
// pager advanced its cursor.
type histCall struct {
	offsetID   int
	offsetDate int
	limit      int
}

// histPage builds a newest-first page of n messages numbered downward
// from startID, all stamped with date.
func histPage(startID, n, date int) []Message {
	msgs := make([]Message, n)
	for i := range n {
		msgs[i] = Message{ID: startID - i, Date: date}
	}

	return msgs
}

// fakeFetch answers each call with the next page in pages (truncated to
// the requested limit) and records the call. Every message is visible,
// so RawCount equals the returned length and NextOffset is the page's
// smallest ID.
func fakeFetch(pages [][]Message, total int, calls *[]histCall) historyPageFunc {
	idx := 0

	return func(_ context.Context, offsetID, offsetDate, limit int) (historyPageData, error) {
		*calls = append(*calls, histCall{offsetID: offsetID, offsetDate: offsetDate, limit: limit})

		if idx >= len(pages) {
			return historyPageData{Total: total}, nil
		}

		page := pages[idx]
		idx++

		if len(page) > limit {
			page = page[:limit]
		}

		return historyPageData{
			Messages:   page,
			Total:      total,
			RawCount:   len(page),
			NextOffset: minMessageID(page),
		}, nil
	}
}

// fakeFetchData answers each call with the next pre-built page, so a test
// can set RawCount independently of the visible message count.
func fakeFetchData(pages []historyPageData, calls *[]histCall) historyPageFunc {
	idx := 0

	return func(_ context.Context, offsetID, offsetDate, limit int) (historyPageData, error) {
		*calls = append(*calls, histCall{offsetID: offsetID, offsetDate: offsetDate, limit: limit})

		if idx >= len(pages) {
			return historyPageData{}, nil
		}

		page := pages[idx]
		idx++

		return page, nil
	}
}

func minMessageID(page []Message) int {
	next := 0

	for i := range page {
		id := page[i].ID
		if id <= 0 {
			continue
		}

		if next == 0 || id < next {
			next = id
		}
	}

	return next
}

func TestPageHistory_SinglePageUnderLimit(t *testing.T) {
	var calls []histCall

	msgs, total, _, err := pageHistory(
		context.Background(),
		HistoryOpts{Limit: 100},
		fakeFetch([][]Message{histPage(30, 30, 1000)}, 30, &calls),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 30 {
		t.Errorf("len(msgs) = %d, want 30", len(msgs))
	}

	if total != 30 {
		t.Errorf("total = %d, want 30", total)
	}

	if len(calls) != 1 {
		t.Errorf("fetch calls = %d, want 1 (short page stops paging)", len(calls))
	}
}

func TestPageHistory_MergesAcrossPagesAndTrimsToLimit(t *testing.T) {
	var calls []histCall

	pages := [][]Message{
		histPage(300, 100, 1000),
		histPage(200, 100, 1000),
		histPage(100, 100, 1000),
	}

	msgs, total, _, err := pageHistory(context.Background(), HistoryOpts{Limit: 250}, fakeFetch(pages, 999, &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 250 {
		t.Fatalf("len(msgs) = %d, want 250 (merged then trimmed)", len(msgs))
	}

	if total != 999 {
		t.Errorf("total = %d, want 999 (from the first page)", total)
	}

	// Page 3 is requested with the remaining 50, not a full 100.
	wantLimits := []int{100, 100, 50}
	for i, c := range calls {
		if c.limit != wantLimits[i] {
			t.Errorf("call %d limit = %d, want %d", i, c.limit, wantLimits[i])
		}
	}

	// The merged slice is contiguous newest-first: 300..51.
	if msgs[0].ID != 300 || msgs[len(msgs)-1].ID != 51 {
		t.Errorf("merged range = [%d..%d], want [300..51]", msgs[0].ID, msgs[len(msgs)-1].ID)
	}
}

func TestPageHistory_OffsetDateOnFirstPageOnly(t *testing.T) {
	var calls []histCall

	pages := [][]Message{histPage(300, 100, 1000), histPage(200, 100, 1000)}

	//nolint:dogsled // only err and the recorded calls matter to this test.
	_, _, _, err := pageHistory(context.Background(), HistoryOpts{Limit: 200, OffsetDate: 555}, fakeFetch(pages, 999, &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("fetch calls = %d, want 2", len(calls))
	}

	if calls[0].offsetDate != 555 {
		t.Errorf("first call offsetDate = %d, want 555", calls[0].offsetDate)
	}

	if calls[1].offsetDate != 0 {
		t.Errorf("second call offsetDate = %d, want 0 (offset_date anchors the first page only)", calls[1].offsetDate)
	}

	// The second page starts past the oldest ID of the first page.
	if calls[1].offsetID != 201 {
		t.Errorf("second call offsetID = %d, want 201 (advanced past the first page)", calls[1].offsetID)
	}
}

func TestPageHistory_MinDateFloorStopsAndDropsOlder(t *testing.T) {
	var calls []histCall

	// One page: first three at/above the floor, the rest below it.
	page := []Message{
		{ID: 10, Date: 2000},
		{ID: 9, Date: 1500},
		{ID: 8, Date: 1000},
		{ID: 7, Date: 500},
		{ID: 6, Date: 400},
	}

	msgs, _, hasMore, err := pageHistory(
		context.Background(),
		HistoryOpts{Limit: 100, MinDate: 1000},
		fakeFetch([][]Message{page}, 5, &calls),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3 (messages older than the floor dropped)", len(msgs))
	}

	if msgs[len(msgs)-1].Date != 1000 {
		t.Errorf("oldest kept date = %d, want 1000 (floor is inclusive)", msgs[len(msgs)-1].Date)
	}

	if len(calls) != 1 {
		t.Errorf("fetch calls = %d, want 1 (floor stops paging)", len(calls))
	}

	// A floor stop is terminal for the query's range — no more within it.
	if hasMore {
		t.Error("hasMore = true, want false (the MinDate floor ended the range)")
	}
}

func TestPageHistory_EmptyPageStops(t *testing.T) {
	var calls []histCall

	msgs, total, _, err := pageHistory(
		context.Background(),
		HistoryOpts{Limit: 250},
		fakeFetch([][]Message{{}}, 0, &calls),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 0 {
		t.Errorf("len(msgs) = %d, want 0", len(msgs))
	}

	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}

	if len(calls) != 1 {
		t.Errorf("fetch calls = %d, want 1 (empty page stops paging)", len(calls))
	}
}

func TestPageHistory_ZeroLimitDefaultsToOnePage(t *testing.T) {
	var calls []histCall

	msgs, _, _, err := pageHistory(
		context.Background(),
		HistoryOpts{},
		fakeFetch([][]Message{histPage(100, 100, 1000)}, 500, &calls),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != DefaultLimit {
		t.Errorf("len(msgs) = %d, want %d (zero limit defaults to one page)", len(msgs), DefaultLimit)
	}

	if len(calls) != 1 {
		t.Errorf("fetch calls = %d, want 1", len(calls))
	}

	if calls[0].limit != DefaultLimit {
		t.Errorf("first call limit = %d, want %d", calls[0].limit, DefaultLimit)
	}
}

func TestPageHistory_HasMoreOnExactLimitFill(t *testing.T) {
	// A full page fills the limit exactly with a live cursor: older
	// history remains, so hasMore MUST be true. Conflating "limit
	// reached" with the MinDate floor used to invert this and drop data.
	var calls []histCall

	msgs, _, hasMore, err := pageHistory(
		context.Background(),
		HistoryOpts{Limit: 100},
		fakeFetch([][]Message{histPage(1000, 100, 1000)}, 5000, &calls),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 100 {
		t.Fatalf("len(msgs) = %d, want 100 (limit filled by one full page)", len(msgs))
	}

	if len(calls) != 1 {
		t.Errorf("fetch calls = %d, want 1 (the limit fill stops the walk without an extra fetch)", len(calls))
	}

	if !hasMore {
		t.Error("hasMore = false, want true (a full page filled the limit with a live cursor)")
	}
}

func TestPageHistory_HasMoreFalseOnShortPage(t *testing.T) {
	var calls []histCall

	msgs, _, hasMore, err := pageHistory(
		context.Background(),
		HistoryOpts{Limit: 100},
		fakeFetch([][]Message{histPage(1000, 30, 1000)}, 30, &calls),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 30 {
		t.Fatalf("len(msgs) = %d, want 30", len(msgs))
	}

	if hasMore {
		t.Error("hasMore = true, want false (a short page means the start of history was reached)")
	}
}

func TestPageHistory_ServiceMessagesDoNotStopPagingEarly(t *testing.T) {
	var calls []histCall

	// A full raw page whose visible messages are far fewer (service
	// messages dropped). RawCount == want, so the walk must continue.
	pages := []historyPageData{
		{Messages: histPage(1000, 40, 1000), Total: 500, RawCount: 100, NextOffset: 901},
		{Messages: histPage(900, 50, 1000), Total: 500, RawCount: 50, NextOffset: 851},
	}

	msgs, total, _, err := pageHistory(context.Background(), HistoryOpts{Limit: 100}, fakeFetchData(pages, &calls))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("fetch calls = %d, want 2 (a full raw page must not stop the walk just because its visible count is low)",
			len(calls))
	}

	if len(msgs) != 90 {
		t.Errorf("len(msgs) = %d, want 90 (40 + 50 visible)", len(msgs))
	}

	if total != 500 {
		t.Errorf("total = %d, want 500", total)
	}

	// The second call advances by the raw NextOffset, not the visible min.
	if calls[1].offsetID != 901 {
		t.Errorf("second call offsetID = %d, want 901 (raw cursor, past dropped service messages)", calls[1].offsetID)
	}
}

func TestPageHistory_CapsLimitAtMaxPagedHistory(t *testing.T) {
	var calls []histCall

	// Every page is a full server page, so without a cap the walk would
	// run far past maxPagedHistory.
	startID := 1_000_000
	fetch := func(_ context.Context, offsetID, offsetDate, limit int) (historyPageData, error) {
		calls = append(calls, histCall{offsetID: offsetID, offsetDate: offsetDate, limit: limit})

		page := histPage(startID, maxHistoryPage, 1000)
		startID -= maxHistoryPage

		return historyPageData{
			Messages:   page,
			Total:      9_999,
			RawCount:   maxHistoryPage,
			NextOffset: minMessageID(page),
		}, nil
	}

	msgs, _, _, err := pageHistory(context.Background(), HistoryOpts{Limit: 5000}, fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != maxPagedHistory {
		t.Errorf("len(msgs) = %d, want %d (limit capped at maxPagedHistory)", len(msgs), maxPagedHistory)
	}

	if len(calls) != maxPagedHistory/maxHistoryPage {
		t.Errorf("fetch calls = %d, want %d (bounded round-trips)", len(calls), maxPagedHistory/maxHistoryPage)
	}
}

func TestPageHistory_ServiceHeavyHistoryIsBoundedByScanCap(t *testing.T) {
	var calls []histCall

	// Every page is a FULL raw page (RawCount 100) but only a few
	// messages are visible — the rest are service messages. merged grows
	// slowly, so without the raw-scan cap the loop would page ~200 times
	// to reach a limit of 1000. The cap must stop it far sooner.
	const visiblePerPage = 5

	startID := 1_000_000
	fetch := func(_ context.Context, offsetID, offsetDate, limit int) (historyPageData, error) {
		calls = append(calls, histCall{offsetID: offsetID, offsetDate: offsetDate, limit: limit})

		page := histPage(startID, visiblePerPage, 1000)
		startID -= maxHistoryPage

		return historyPageData{
			Messages:   page,
			Total:      50_000,
			RawCount:   maxHistoryPage, // full raw page, mostly service messages
			NextOffset: minMessageID(page),
		}, nil
	}

	msgs, _, hasMore, err := pageHistory(context.Background(), HistoryOpts{Limit: maxPagedHistory}, fetch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantCalls := maxHistoryScan / maxHistoryPage
	if len(calls) != wantCalls {
		t.Errorf("fetch calls = %d, want %d (raw-scan cap bounds round-trips independent of visible density)",
			len(calls), wantCalls)
	}

	// merged never reached the limit — the scan cap, not the limit,
	// ended the walk.
	if len(msgs) >= maxPagedHistory {
		t.Errorf("len(msgs) = %d, want < %d (walk ended on the scan cap, not the limit)", len(msgs), maxPagedHistory)
	}

	// The walk stopped on the scan cap while older history remains, so
	// hasMore must stay true — a len-vs-limit compare would misreport
	// this as "no more" and strand a caller that pages on it.
	if !hasMore {
		t.Error("hasMore = false, want true (scan-cap stop with older history still available)")
	}

	if len(msgs) != wantCalls*visiblePerPage {
		t.Errorf("len(msgs) = %d, want %d", len(msgs), wantCalls*visiblePerPage)
	}
}

func TestPageHistory_FetchErrorAbortsTheWalk(t *testing.T) {
	wantErr := errTestFetchBoom
	call := 0
	fetch := func(_ context.Context, _, _, _ int) (historyPageData, error) {
		call++
		if call == 1 {
			return historyPageData{Messages: histPage(300, 100, 1000), Total: 999, RawCount: 100, NextOffset: 201}, nil
		}

		return historyPageData{}, wantErr
	}

	msgs, total, _, err := pageHistory(context.Background(), HistoryOpts{Limit: 250}, fetch)
	if err == nil {
		t.Fatal("expected the fetch error to abort the walk")
	}

	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}

	if msgs != nil || total != 0 {
		t.Errorf("on error want (nil, 0), got (%v, %d)", msgs, total)
	}

	if call != 2 {
		t.Errorf("fetch calls = %d, want 2 (aborts on the erroring page, not before or after)", call)
	}
}

func TestHistoryPageResult_WrapsFetchError(t *testing.T) {
	base := errTestRPCBoom

	_, err := historyPageResult(nil, base, InputPeer{}, "getting history")
	if err == nil {
		t.Fatal("expected a wrapped error")
	}

	if !errors.Is(err, base) {
		t.Errorf("error = %v, want it to wrap %v", err, base)
	}

	if !strings.Contains(err.Error(), "getting history") {
		t.Errorf("error %q missing the %q context", err.Error(), "getting history")
	}
}

// historyInvoker answers MessagesGetHistory with a per-request canned
// page keyed by the request's OffsetID, and records the requests so a
// test can assert the wrapper wired offset_date onto the wire.
type historyInvoker struct {
	byOffset map[int]*tg.MessagesMessagesSlice
	requests []*tg.MessagesGetHistoryRequest
}

func (h *historyInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetHistoryRequest)
	if !ok {
		return errUnexpectedRequest
	}

	h.requests = append(h.requests, req)

	resp, ok := h.byOffset[req.OffsetID]
	if !ok {
		resp = &tg.MessagesMessagesSlice{Count: 0}
	}

	return encodeResp(resp, output)
}

func tgHistoryPage(count, startID, n int) *tg.MessagesMessagesSlice {
	msgs := make([]tg.MessageClass, n)
	for i := range n {
		msgs[i] = &tg.Message{ID: startID - i, Message: "m", PeerID: &tg.PeerUser{UserID: 1}, Date: 1000}
	}

	return &tg.MessagesMessagesSlice{Count: count, Messages: msgs}
}

func TestGetHistory_PagesAndWiresOffsetDate(t *testing.T) {
	inv := &historyInvoker{byOffset: map[int]*tg.MessagesMessagesSlice{
		0:   tgHistoryPage(999, 300, 100),
		201: tgHistoryPage(999, 200, 30), // short page ends paging
	}}

	wrapper := &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}

	msgs, total, _, err := wrapper.GetHistory(
		context.Background(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 2},
		HistoryOpts{Limit: 250, OffsetDate: 777},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 130 {
		t.Errorf("len(msgs) = %d, want 130 (100 + short 30)", len(msgs))
	}

	if total != 999 {
		t.Errorf("total = %d, want 999", total)
	}

	if len(inv.requests) != 2 {
		t.Fatalf("history requests = %d, want 2", len(inv.requests))
	}

	if inv.requests[0].OffsetDate != 777 {
		t.Errorf("first request OffsetDate = %d, want 777", inv.requests[0].OffsetDate)
	}

	if inv.requests[1].OffsetDate != 0 {
		t.Errorf("second request OffsetDate = %d, want 0 (offset_date only anchors the first page)",
			inv.requests[1].OffsetDate)
	}
}

func TestGetHistory_ExactLimitFillReportsHasMore(t *testing.T) {
	// One full server page fills limit=100 while older history exists at
	// the next offset: exactly one RPC, and hasMore must be true so a
	// caller keeps paging instead of stopping and losing the rest.
	inv := &historyInvoker{byOffset: map[int]*tg.MessagesMessagesSlice{
		0:   tgHistoryPage(500, 300, 100),
		201: tgHistoryPage(500, 200, 100),
	}}

	wrapper := &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}

	msgs, _, hasMore, err := wrapper.GetHistory(
		context.Background(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 2},
		HistoryOpts{Limit: 100},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 100 {
		t.Fatalf("len(msgs) = %d, want 100", len(msgs))
	}

	if len(inv.requests) != 1 {
		t.Errorf("history requests = %d, want 1 (the limit fill needs no extra fetch)", len(inv.requests))
	}

	if !hasMore {
		t.Error("hasMore = false, want true (full page filled the limit, older history available)")
	}
}

func TestGetHistory_SinglePageStopsAfterOneRPC(t *testing.T) {
	// A full first page (100) with older history available at offset 201:
	// without SinglePage the pager would continue. SinglePage must stop
	// it after exactly one RPC, so a fixed window anchored at OffsetID is
	// not extended past its bound.
	inv := &historyInvoker{byOffset: map[int]*tg.MessagesMessagesSlice{
		0:   tgHistoryPage(999, 300, 100),
		201: tgHistoryPage(999, 200, 100),
	}}

	wrapper := &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}

	msgs, _, _, err := wrapper.GetHistory(
		context.Background(),
		InputPeer{Type: PeerUser, ID: 1, AccessHash: 2},
		HistoryOpts{Limit: 250, SinglePage: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(inv.requests) != 1 {
		t.Fatalf("history requests = %d, want 1 (SinglePage fetches exactly one page)", len(inv.requests))
	}

	if len(msgs) != 100 {
		t.Errorf("len(msgs) = %d, want 100 (the single fetched page)", len(msgs))
	}
}

// repliesInvoker is the historyInvoker analogue for MessagesGetReplies,
// so the GetTopicMessages delegation to the shared pager is pinned too.
type repliesInvoker struct {
	byOffset map[int]*tg.MessagesMessagesSlice
	requests []*tg.MessagesGetRepliesRequest
}

func (h *repliesInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetRepliesRequest)
	if !ok {
		return errUnexpectedRequest
	}

	h.requests = append(h.requests, req)

	resp, ok := h.byOffset[req.OffsetID]
	if !ok {
		resp = &tg.MessagesMessagesSlice{Count: 0}
	}

	return encodeResp(resp, output)
}

func TestGetTopicMessages_PagesAndWiresOffsetDate(t *testing.T) {
	inv := &repliesInvoker{byOffset: map[int]*tg.MessagesMessagesSlice{
		0:   tgHistoryPage(999, 300, 100),
		201: tgHistoryPage(999, 200, 30), // short page ends paging
	}}

	wrapper := &Wrapper{api: tg.NewClient(inv), cache: NewPeerCache()}

	const topicID = 42

	msgs, total, _, err := wrapper.GetTopicMessages(
		context.Background(),
		InputPeer{Type: PeerChannel, ID: 1, AccessHash: 2},
		topicID,
		HistoryOpts{Limit: 250, OffsetDate: 777},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(msgs) != 130 {
		t.Errorf("len(msgs) = %d, want 130 (100 + short 30)", len(msgs))
	}

	if total != 999 {
		t.Errorf("total = %d, want 999", total)
	}

	if len(inv.requests) != 2 {
		t.Fatalf("replies requests = %d, want 2", len(inv.requests))
	}

	if inv.requests[0].MsgID != topicID {
		t.Errorf("MsgID = %d, want %d (topic root)", inv.requests[0].MsgID, topicID)
	}

	if inv.requests[0].OffsetDate != 777 {
		t.Errorf("first request OffsetDate = %d, want 777", inv.requests[0].OffsetDate)
	}

	if inv.requests[1].OffsetDate != 0 {
		t.Errorf("second request OffsetDate = %d, want 0 (offset_date only anchors the first page)",
			inv.requests[1].OffsetDate)
	}
}

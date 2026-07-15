package telegram

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

var (
	errUnexpectedRequest = errors.New("unexpected request")
	errTestBoom          = errors.New("boom")
	errChannelInvalidFix = errors.New("CHANNEL_INVALID")
)

type fakeInvoker struct {
	calls    atomic.Int32
	response tg.MessagesDialogsClass
	err      error
}

func (f *fakeInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	f.calls.Add(1)

	if f.err != nil {
		return f.err
	}

	return encodeAndDecode(f.response, output)
}

func encodeAndDecode(resp tg.MessagesDialogsClass, output bin.Decoder) error {
	var buf bin.Buffer

	err := resp.Encode(&buf)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	err = output.Decode(&buf)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}

func newWrapperWithInvoker(invoker tg.Invoker) *Wrapper {
	return &Wrapper{
		api:   tg.NewClient(invoker),
		cache: NewPeerCache(),
	}
}

func TestWarmDialogsCache_PopulatesChannelAccessHash(t *testing.T) {
	channelID := int64(3282239618)
	channelHash := int64(0xDEADBEEF)

	invoker := &fakeInvoker{
		response: &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: channelID}, TopMessage: 1},
			},
			Chats: []tg.ChatClass{channelWithHash(channelID, channelHash, "Test")},
		},
	}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	cached, hit := wrap.cache.Lookup(PeerChannel, channelID)
	if !hit {
		t.Fatalf("expected cache hit for channel %d after warm", channelID)
	}

	if cached.AccessHash != channelHash {
		t.Errorf("cached AccessHash = %x, want %x", cached.AccessHash, channelHash)
	}

	// One complete page per folder: the main list and the archive.
	if invoker.calls.Load() != 2 {
		t.Errorf("expected 2 API calls (main + archive), got %d", invoker.calls.Load())
	}
}

func TestWarmDialogsCache_ThrottledOnSecondCall(t *testing.T) {
	invoker := &fakeInvoker{response: &tg.MessagesDialogs{}}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())
	_, _ = wrap.warmDialogsCache(t.Context())

	// First warm scans both folders (2 calls); the second is throttled.
	if got := invoker.calls.Load(); got != 2 {
		t.Errorf("expected second call throttled (2 total), got %d", got)
	}
}

func TestWarmDialogsCache_AllowsRetryAfterFirstPageError(t *testing.T) {
	invoker := &fakeInvoker{err: errTestBoom}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	// A failed warm never stamps completion, so the throttle stays open.
	if wrap.warmedAt.Load() != 0 {
		t.Errorf("expected warmedAt unset after a failed warm, got %d", wrap.warmedAt.Load())
	}

	_, _ = wrap.warmDialogsCache(t.Context())

	// Each warm attempts both folders (main + archive), and the second
	// warm is not throttled — 2 folders × 2 attempts.
	if got := invoker.calls.Load(); got != 4 {
		t.Errorf("expected retry after failure, got %d calls", got)
	}
}

func TestWarmDialogsCache_PaginatesMainUntilComplete(t *testing.T) {
	invoker := &folderScriptedInvoker{
		main: []tg.MessagesDialogsClass{
			warmSlice(100),
			warmSlice(200),
			&tg.MessagesDialogs{}, // complete → stop
		},
	}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	if got := invoker.mainCalls.Load(); got != 3 {
		t.Errorf("expected 3 main pages (2 slices + terminal), got %d", got)
	}

	if _, hit := wrap.cache.Lookup(PeerChannel, 100); !hit {
		t.Error("expected page 1 channel cached")
	}

	if _, hit := wrap.cache.Lookup(PeerChannel, 200); !hit {
		t.Error("expected page 2 channel cached")
	}

	if got := invoker.archCalls.Load(); got != 1 {
		t.Errorf("expected archive queried once after main completes, got %d", got)
	}
}

func TestWarmDialogsCache_PaginatesBeyondFivePages(t *testing.T) {
	// The target channel sits on page 6 of the main list, past the old
	// five-page window. A complete result on page 6 stops pagination.
	const targetID = int64(105)

	invoker := &folderScriptedInvoker{
		main: []tg.MessagesDialogsClass{
			warmSlice(100), warmSlice(101), warmSlice(102), warmSlice(103), warmSlice(104),
			&tg.MessagesDialogs{
				Dialogs: []tg.DialogClass{
					&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: targetID}, TopMessage: 1},
				},
				Chats: []tg.ChatClass{channelWithHash(targetID, 0xF00D, "Target")},
			},
		},
	}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	if _, hit := wrap.cache.Lookup(PeerChannel, targetID); !hit {
		t.Fatalf("expected channel %d on page 6 to be cached after warm", targetID)
	}

	if got := invoker.mainCalls.Load(); got != 6 {
		t.Errorf("expected 6 main-list pages fetched, got %d", got)
	}
}

func TestWarmDialogsCache_SeedsArchivedChannels(t *testing.T) {
	const archivedID = int64(900)

	invoker := &folderScriptedInvoker{
		main: []tg.MessagesDialogsClass{
			&tg.MessagesDialogs{
				Dialogs: []tg.DialogClass{
					&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 100}, TopMessage: 1},
				},
				Chats: []tg.ChatClass{channelWithHash(100, 0xAAAA, "Main")},
			},
		},
		archive: []tg.MessagesDialogsClass{
			&tg.MessagesDialogs{
				Dialogs: []tg.DialogClass{
					&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: archivedID}, TopMessage: 1},
				},
				Chats: []tg.ChatClass{channelWithHash(archivedID, 0xBEEF, "Archived")},
			},
		},
	}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	if _, hit := wrap.cache.Lookup(PeerChannel, archivedID); !hit {
		t.Fatalf("expected archived channel %d to be cached after warm", archivedID)
	}

	if got := invoker.archCalls.Load(); got != 1 {
		t.Errorf("expected the archive folder to be queried once, got %d", got)
	}
}

func TestWarmDialogsCache_ArchiveFirstPageErrorAllowsRetry(t *testing.T) {
	// The main list completes but the archive's first page fails. The
	// warm must not latch the throttle: an archived channel would
	// otherwise be reported uncached for the whole window even though the
	// archive was never scanned.
	invoker := &archiveFailInvoker{}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	if wrap.warmedAt.Load() != 0 {
		t.Errorf("expected warmedAt reset after archive first-page error, got %d", wrap.warmedAt.Load())
	}

	_, _ = wrap.warmDialogsCache(t.Context())

	// Two full attempts: each scans the main list and then hits the
	// failing archive first page (2 requests per attempt).
	if got := invoker.calls.Load(); got != 4 {
		t.Errorf("expected the warm to retry after an archive failure, got %d calls", got)
	}
}

func TestWarmDialogsCache_SafetyBoundStopsUnboundedSlices(t *testing.T) {
	// A folder that answers with a slice on every page never signals
	// completion; the safety bound must stop the crawl.
	total := warmDialogsMaxPages + 10
	main := make([]tg.MessagesDialogsClass, 0, total)

	for i := range total {
		main = append(main, warmSlice(int64(1000+i)))
	}

	invoker := &folderScriptedInvoker{main: main}
	wrap := newWrapperWithInvoker(invoker)

	_, _ = wrap.warmDialogsCache(t.Context())

	if got := invoker.mainCalls.Load(); got != int32(warmDialogsMaxPages) {
		t.Errorf("expected main warm capped at %d pages, got %d", warmDialogsMaxPages, got)
	}
}

func TestResolvePeer_ColdMissWarmsCacheAndReturnsHash(t *testing.T) {
	channelID := int64(3282239618)
	channelHash := int64(0xCAFEBABE)

	invoker := &warmingInvoker{channelID: channelID, channelHash: channelHash}
	wrap := newWrapperWithInvoker(invoker)

	identifier := "-100" + int64ToString(channelID)

	peer, err := wrap.ResolvePeer(t.Context(), identifier)
	if err != nil {
		t.Fatalf("ResolvePeer: %v", err)
	}

	if peer.AccessHash != channelHash {
		t.Errorf("resolved AccessHash = %x, want %x", peer.AccessHash, channelHash)
	}

	if peer.Type != PeerChannel || peer.ID != channelID {
		t.Errorf("resolved peer = %+v, want channel %d", peer, channelID)
	}
}

func TestResolvePeer_ColdMissUnresolvedChannelReturnsClearError(t *testing.T) {
	// The warm returns no dialogs, so the numeric channel stays
	// unresolved and must surface a clear, actionable error instead of a
	// hash-0 peer that later fails as an opaque CHANNEL_INVALID.
	wrap := newWrapperWithInvoker(coldChannelInvoker{})

	channelID := int64(3282239618)

	_, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if !errors.Is(err, ErrChannelNotCached) {
		t.Fatalf("expected ErrChannelNotCached, got %v", err)
	}
}

func TestResolvePeer_WarmFailureSurfacesRetryableError(t *testing.T) {
	// The dialog warm fails outright (its first page errors), so a
	// numeric channel miss is inconclusive — ResolvePeer must surface the
	// retryable failure, not the terminal ErrChannelNotCached that tells
	// the caller to go open a channel which may well be in their dialogs.
	wrap := newWrapperWithInvoker(warmFailInvoker{})

	channelID := int64(3282239618)

	_, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if err == nil {
		t.Fatal("expected an error from a failed warm")
	}

	if errors.Is(err, ErrChannelNotCached) {
		t.Errorf("expected a retryable warm error, got terminal ErrChannelNotCached: %v", err)
	}
}

// warmFailInvoker rejects the peer probe and fails every dialog fetch, so
// the warm never completes.
type warmFailInvoker struct{}

func (warmFailInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetPeerDialogsRequest:
		return errChannelInvalidFix
	case *tg.MessagesGetDialogsRequest:
		return errTestBoom
	default:
		return errUnexpectedRequest
	}
}

func TestResolvePeer_ChannelLaterPageWarmFailureIsRetryable(t *testing.T) {
	// The target channel sits beyond a main-list page that fails mid-scan.
	// The warm never completed, so a miss is inconclusive and must surface
	// as retryable — not the terminal ErrChannelNotCached, which would
	// also latch the throttle against a retry.
	wrap := newWrapperWithInvoker(&laterPageFailInvoker{})

	// Large enough that -100<id> lands below the channel threshold.
	channelID := int64(3282239618)

	_, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if err == nil {
		t.Fatal("expected an error from an incomplete warm")
	}

	if errors.Is(err, ErrChannelNotCached) {
		t.Errorf("expected a retryable error for an incomplete warm, got terminal: %v", err)
	}
}

// laterPageFailInvoker serves one good main-list page and then fails the
// next, so the warm caches some peers but never completes its scan.
type laterPageFailInvoker struct{ mainCalls atomic.Int32 }

func (l *laterPageFailInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		if _, isProbe := input.(*tg.MessagesGetPeerDialogsRequest); isProbe {
			return errChannelInvalidFix
		}

		return errUnexpectedRequest
	}

	if folderID, set := req.GetFolderID(); set && folderID == warmArchiveFolderID {
		return encodeAndDecode(&tg.MessagesDialogs{}, output)
	}

	if l.mainCalls.Add(1) == 1 {
		return encodeAndDecode(warmSlice(100), output)
	}

	return errTestBoom
}

func TestWarmDialogsCache_ScansArchiveEvenWhenMainHitsCap(t *testing.T) {
	// The main list never exhausts (hits the cap), but the target channel
	// is archived. The archive pass must still run so the channel resolves
	// instead of failing with a scan-limit error.
	archivedID := int64(3282239618)
	archivedHash := int64(0xABCDEF)

	wrap := newWrapperWithInvoker(&mainCapArchiveHitInvoker{
		archivedID:   archivedID,
		archivedHash: archivedHash,
	})

	peer, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(archivedID))
	if err != nil {
		t.Fatalf("expected the archived channel to resolve despite the main cap, got: %v", err)
	}

	if peer.AccessHash != archivedHash {
		t.Errorf("resolved AccessHash = %x, want %x", peer.AccessHash, archivedHash)
	}
}

// mainCapArchiveHitInvoker never exhausts the main list (forcing the cap)
// but serves the target channel as a complete archive page.
type mainCapArchiveHitInvoker struct {
	archivedID   int64
	archivedHash int64
	mainPage     atomic.Int64
}

func (m *mainCapArchiveHitInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		if _, isProbe := input.(*tg.MessagesGetPeerDialogsRequest); isProbe {
			return errChannelInvalidFix
		}

		return errUnexpectedRequest
	}

	if folderID, set := req.GetFolderID(); set && folderID == warmArchiveFolderID {
		resp := &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: m.archivedID}, TopMessage: 1},
			},
			Chats: []tg.ChatClass{channelWithHash(m.archivedID, m.archivedHash, "Archived")},
		}

		return encodeAndDecode(resp, output)
	}

	return encodeAndDecode(warmSlice(4000+m.mainPage.Add(1)), output)
}

func TestResolvePeer_ChannelBeyondScanLimitIsNotTerminal(t *testing.T) {
	// The main list never exhausts (every page is a slice), so the warm
	// hits its safety cap without proving the channel absent. The miss
	// must surface ErrDialogScanLimit, not the terminal ErrChannelNotCached.
	wrap := newWrapperWithInvoker(&unboundedSliceInvoker{})

	channelID := int64(3282239618)

	_, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if err == nil {
		t.Fatal("expected an error from a cap-limited warm")
	}

	if errors.Is(err, ErrChannelNotCached) {
		t.Errorf("a cap-limited warm must not be terminal, got: %v", err)
	}

	if !errors.Is(err, ErrDialogScanLimit) {
		t.Errorf("expected ErrDialogScanLimit, got: %v", err)
	}
}

// unboundedSliceInvoker never returns a complete result, so the folder
// scan runs to the safety cap without exhausting.
type unboundedSliceInvoker struct{ page atomic.Int64 }

func (u *unboundedSliceInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetPeerDialogsRequest); ok {
		return errChannelInvalidFix
	}

	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	return encodeAndDecode(warmSlice(2000+u.page.Add(1)), output)
}

func TestResolvePeer_UserInDialogsResolvesViaWarm(t *testing.T) {
	// A numeric user ID with a DM dialog resolves via the warm: the scan
	// caches the user's access hash, so the post-warm lookup hits. This is
	// the user-side happy path (the channel one is covered separately).
	userID := int64(424242)
	userHash := int64(0x5151ABCD)

	wrap := newWrapperWithInvoker(&warmUserInvoker{userID: userID, userHash: userHash})

	peer, err := wrap.ResolvePeer(t.Context(), int64ToString(userID))
	if err != nil {
		t.Fatalf("ResolvePeer: %v", err)
	}

	if peer.Type != PeerUser || peer.ID != userID || peer.AccessHash != userHash {
		t.Errorf("resolved peer = %+v, want user %d hash %x", peer, userID, userHash)
	}
}

func TestResolvePeer_StaleRetryReScansToTerminal(t *testing.T) {
	// The full retry contract: a throttled miss returns ErrChannelWarmStale
	// and resets the throttle, so an immediate retry re-scans and — the
	// channel still absent — returns the terminal ErrChannelNotCached.
	wrap := newWrapperWithInvoker(coldChannelInvoker{})

	_, _ = wrap.warmDialogsCache(t.Context()) // latch the throttle

	ident := "-100" + int64ToString(3282239618)

	_, first := wrap.ResolvePeer(t.Context(), ident)
	if !errors.Is(first, ErrChannelWarmStale) {
		t.Fatalf("first (throttled) resolve: want ErrChannelWarmStale, got %v", first)
	}

	_, second := wrap.ResolvePeer(t.Context(), ident)
	if !errors.Is(second, ErrChannelNotCached) {
		t.Fatalf("retry should re-scan to a terminal verdict, got %v", second)
	}
}

// warmUserInvoker rejects the peer probe and serves the target user as a
// single complete dialog page, so the warm caches its access hash.
type warmUserInvoker struct {
	userID   int64
	userHash int64
}

func (w *warmUserInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetPeerDialogsRequest:
		return errChannelInvalidFix
	case *tg.MessagesGetDialogsRequest:
		resp := &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerUser{UserID: w.userID}, TopMessage: 1},
			},
			Users: []tg.UserClass{userWithHash(w.userID, w.userHash)},
		}

		return encodeAndDecode(resp, output)
	default:
		return errUnexpectedRequest
	}
}

func userWithHash(userID, accessHash int64) *tg.User {
	usr := &tg.User{ID: userID}
	usr.SetAccessHash(accessHash)

	return usr
}

func TestResolvePeer_ThrottledChannelMissIsRetryableAndResetsThrottle(t *testing.T) {
	// A completed warm latches the throttle. A later cold channel resolve
	// within the window skips the warm, so its miss rests on a possibly
	// stale scan — it must be retryable (ErrChannelWarmStale), NOT the
	// terminal ErrChannelNotCached, and must reset the throttle so a retry
	// re-scans (catching e.g. a channel joined on another session since).
	wrap := newWrapperWithInvoker(coldChannelInvoker{})

	// First warm completes on an empty dialog set and latches the throttle.
	ran, err := wrap.warmDialogsCache(t.Context())
	if !ran || err != nil {
		t.Fatalf("expected the first warm to run and complete, got ran=%v err=%v", ran, err)
	}

	if wrap.warmedAt.Load() == 0 {
		t.Fatal("expected a completed warm to latch the throttle")
	}

	channelID := int64(3282239618)

	_, err = wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if !errors.Is(err, ErrChannelWarmStale) {
		t.Fatalf("expected ErrChannelWarmStale for a throttled miss, got: %v", err)
	}

	if errors.Is(err, ErrChannelNotCached) {
		t.Errorf("a throttled miss must not be the terminal ErrChannelNotCached: %v", err)
	}

	if wrap.warmedAt.Load() != 0 {
		t.Errorf("a throttled channel miss must reset the throttle for retry, got %d", wrap.warmedAt.Load())
	}
}

func TestResolvePeer_UserWarmFailureKeepsHashZeroFallback(t *testing.T) {
	// A numeric user whose warm fails must still fall back to the hash-0
	// peer — only channels get a terminal verdict; the tools layer labels
	// unresolved users per parameter (from / offsetPeer).
	wrap := newWrapperWithInvoker(warmFailInvoker{})

	userID := int64(777001)

	peer, err := wrap.ResolvePeer(t.Context(), int64ToString(userID))
	if err != nil {
		t.Fatalf("expected hash-0 fallback for a user, got error: %v", err)
	}

	if peer.Type != PeerUser || peer.ID != userID || peer.AccessHash != 0 {
		t.Errorf("expected hash-0 user peer %d, got %+v", userID, peer)
	}
}

func TestResolvePeer_ConcurrentColdMissShareWarm(t *testing.T) {
	// Two cold resolutions race. The first wins the singleflight and
	// blocks inside the warm; the second must queue on it and observe the
	// warmed cache, not a premature miss reported as ErrChannelNotCached.
	channelID := int64(3282239618)
	channelHash := int64(0xCAFEF00D)

	invoker := &blockingWarmInvoker{
		channelID:   channelID,
		channelHash: channelHash,
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	wrap := newWrapperWithInvoker(invoker)
	ident := "-100" + int64ToString(channelID)

	type result struct {
		peer InputPeer
		err  error
	}

	results := make(chan result, 2)
	resolve := func() { p, e := wrap.ResolvePeer(t.Context(), ident); results <- result{p, e} }

	go resolve()
	<-invoker.entered // the warm is now in flight with warmedAt still unset
	go resolve()
	close(invoker.release)

	for range 2 {
		got := <-results
		if got.err != nil {
			t.Errorf("concurrent resolve returned error: %v", got.err)
		}

		if got.peer.AccessHash != channelHash {
			t.Errorf("resolved AccessHash = %x, want %x", got.peer.AccessHash, channelHash)
		}
	}
}

// blockingWarmInvoker holds the first dialog fetch open until released, so
// a second concurrent resolver is forced to share the in-flight warm
// through the singleflight instead of observing a premature miss.
type blockingWarmInvoker struct {
	channelID   int64
	channelHash int64
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
}

func (b *blockingWarmInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		if _, isProbe := input.(*tg.MessagesGetPeerDialogsRequest); isProbe {
			return errChannelInvalidFix
		}

		return errUnexpectedRequest
	}

	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})

	if folderID, set := req.GetFolderID(); set && folderID == warmArchiveFolderID {
		return encodeAndDecode(&tg.MessagesDialogs{}, output)
	}

	resp := &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: b.channelID}, TopMessage: 1},
		},
		Chats: []tg.ChatClass{channelWithHash(b.channelID, b.channelHash, "Test")},
	}

	return encodeAndDecode(resp, output)
}

// warmSlice builds one paginated main-list page holding a single channel
// dialog with a non-zero access hash, enough for the cursor to advance.
func warmSlice(id int64) *tg.MessagesDialogsSlice {
	return &tg.MessagesDialogsSlice{
		Count: 999,
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: id}, TopMessage: int(id)},
		},
		Messages: []tg.MessageClass{
			&tg.Message{ID: int(id), Date: 1700000000 + int(id), PeerID: &tg.PeerChannel{ChannelID: id}},
		},
		Chats: []tg.ChatClass{channelWithHash(id, id*7+1, "Ch")},
	}
}

func channelWithHash(channelID, accessHash int64, title string) *tg.Channel {
	ch := &tg.Channel{
		ID:    channelID,
		Title: title,
		Photo: &tg.ChatPhotoEmpty{},
	}
	ch.SetAccessHash(accessHash)

	return ch
}

// folderScriptedInvoker serves scripted messages.getDialogs pages per
// folder (main list vs. archive), so a test can place a channel beyond
// the old five-page window of the main list or only in the archive and
// assert the warm reaches it. A folder whose script is exhausted answers
// with a complete *tg.MessagesDialogs, which stops that folder's crawl.
type folderScriptedInvoker struct {
	calls     atomic.Int32
	mainCalls atomic.Int32
	archCalls atomic.Int32
	main      []tg.MessagesDialogsClass
	archive   []tg.MessagesDialogsClass
}

func (s *folderScriptedInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		return errUnexpectedRequest
	}

	s.calls.Add(1)

	pages := s.main
	counter := &s.mainCalls

	if folderID, set := req.GetFolderID(); set && folderID == warmArchiveFolderID {
		pages = s.archive
		counter = &s.archCalls
	}

	idx := int(counter.Add(1)) - 1
	if idx >= len(pages) {
		return encodeAndDecode(&tg.MessagesDialogs{}, output)
	}

	return encodeAndDecode(pages[idx], output)
}

// archiveFailInvoker completes the main-list warm but fails the archive
// folder's first page, exercising the throttle-reset path.
type archiveFailInvoker struct{ calls atomic.Int32 }

func (a *archiveFailInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		return errUnexpectedRequest
	}

	a.calls.Add(1)

	if folderID, set := req.GetFolderID(); set && folderID == warmArchiveFolderID {
		return errTestBoom
	}

	return encodeAndDecode(&tg.MessagesDialogs{}, output)
}

// warmingInvoker rejects the peer-specific GetPeerDialogs probe (the
// hash-0 channel is CHANNEL_INVALID) and answers the full-dialog warm
// with the target channel, mirroring the real cold-miss recovery path.
type warmingInvoker struct {
	channelID   int64
	channelHash int64
}

func (w *warmingInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetPeerDialogsRequest:
		return errChannelInvalidFix
	case *tg.MessagesGetDialogsRequest:
		resp := &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: w.channelID}, TopMessage: 1},
			},
			Chats: []tg.ChatClass{channelWithHash(w.channelID, w.channelHash, "Test")},
		}

		return encodeAndDecode(resp, output)
	default:
		return errUnexpectedRequest
	}
}

// coldChannelInvoker rejects the peer probe and returns empty dialog
// pages for both folders, so the warm caches nothing and the numeric
// channel stays unresolved.
type coldChannelInvoker struct{}

func (coldChannelInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetPeerDialogsRequest:
		return errChannelInvalidFix
	case *tg.MessagesGetDialogsRequest:
		return encodeAndDecode(&tg.MessagesDialogs{}, output)
	default:
		return errUnexpectedRequest
	}
}

func int64ToString(value int64) string {
	const base = 10

	if value == 0 {
		return "0"
	}

	buf := make([]byte, 0, 20)

	for value > 0 {
		buf = append(buf, byte('0'+value%base))
		value /= base
	}

	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}

func TestPeerCacheHoldsAFullWarmWithHeadroom(t *testing.T) {
	// A single warm can insert warmDialogsMaxPages * warmDialogsPageLimit
	// peers per folder across both folders. PeerCache rotates generations
	// on overflow (it does not clear), so a warm's freshly-cached peers
	// survive — but only if a single warm triggers AT MOST one rotation.
	// That holds only while a full warm stays under the per-generation
	// limit, so the limit must exceed a full warm with headroom to keep
	// rotations infrequent.
	warmMax := warmDialogsMaxPages * warmDialogsPageLimit * 2

	if maxCacheEntries <= warmMax {
		t.Fatalf("maxCacheEntries %d must exceed a full warm of %d peers", maxCacheEntries, warmMax)
	}

	if maxCacheEntries < warmMax*2 {
		t.Errorf("maxCacheEntries %d leaves little headroom over a full warm of %d", maxCacheEntries, warmMax)
	}
}

func TestWarmDialogsThrottleIsPositive(t *testing.T) {
	if warmDialogsThrottle <= 0 {
		t.Fatal("warmDialogsThrottle must be positive")
	}

	if warmDialogsThrottle < time.Second {
		t.Errorf("throttle %v is suspiciously short", warmDialogsThrottle)
	}
}

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
	"github.com/gotd/td/tgerr"
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

func encodeAndDecode(resp bin.Encoder, output bin.Decoder) error {
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

	_ = wrap.warmDialogsCache(t.Context())

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

	_ = wrap.warmDialogsCache(t.Context())
	_ = wrap.warmDialogsCache(t.Context())

	// First warm scans both folders (2 calls); the second is throttled.
	if got := invoker.calls.Load(); got != 2 {
		t.Errorf("expected second call throttled (2 total), got %d", got)
	}
}

func TestWarmDialogsCache_AllowsRetryAfterFirstPageError(t *testing.T) {
	invoker := &fakeInvoker{err: errTestBoom}
	wrap := newWrapperWithInvoker(invoker)

	_ = wrap.warmDialogsCache(t.Context())

	// A failed warm never stamps completion, so the throttle stays open.
	if wrap.warmedAt.Load() != 0 {
		t.Errorf("expected warmedAt unset after a failed warm, got %d", wrap.warmedAt.Load())
	}

	_ = wrap.warmDialogsCache(t.Context())

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

	_ = wrap.warmDialogsCache(t.Context())

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

	_ = wrap.warmDialogsCache(t.Context())

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

	_ = wrap.warmDialogsCache(t.Context())

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

	_ = wrap.warmDialogsCache(t.Context())

	if wrap.warmedAt.Load() != 0 {
		t.Errorf("expected warmedAt reset after archive first-page error, got %d", wrap.warmedAt.Load())
	}

	_ = wrap.warmDialogsCache(t.Context())

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

	_ = wrap.warmDialogsCache(t.Context())

	if got := invoker.mainCalls.Load(); got != int32(warmDialogsMaxPages) {
		t.Errorf("expected main warm capped at %d pages, got %d", warmDialogsMaxPages, got)
	}
}

func TestResolvePeer_ColdChannelAsksTheServerForItsHash(t *testing.T) {
	// A numeric channel the cache does not know is resolved by asking
	// Telegram directly — one channels.getChannels round trip, no dialog
	// scan. The invoker fails any messages.getDialogs, so a warm would
	// fail the test rather than quietly rescue it.
	channelID := int64(3282239618)
	channelHash := int64(0xCAFEBABE)

	invoker := &getChannelsInvoker{channelID: channelID, channelHash: channelHash}
	wrap := newWrapperWithInvoker(invoker)

	peer, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if err != nil {
		t.Fatalf("ResolvePeer: %v", err)
	}

	if peer.Type != PeerChannel || peer.ID != channelID || peer.AccessHash != channelHash {
		t.Errorf("resolved peer = %+v, want channel %d hash %x", peer, channelID, channelHash)
	}

	if got := invoker.calls.Load(); got != 1 {
		t.Errorf("expected exactly 1 channels.getChannels call, got %d", got)
	}
}

func TestResolvePeer_UnreachableChannelFallsBackWithoutACacheError(t *testing.T) {
	// The server refuses to hand out the hash (the account cannot address
	// this channel at all). That is not a cache problem, so ResolvePeer
	// must not invent a cache-shaped error telling the caller to go warm
	// something: it returns the hash-0 peer and lets the request that
	// follows fail with the server's own CHANNEL_INVALID.
	wrap := newWrapperWithInvoker(refusingChannelInvoker{})

	channelID := int64(3282239618)

	peer, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(channelID))
	if err != nil {
		t.Fatalf("expected a hash-0 fallback, got error: %v", err)
	}

	if peer.Type != PeerChannel || peer.ID != channelID || peer.AccessHash != 0 {
		t.Errorf("expected hash-0 channel peer %d, got %+v", channelID, peer)
	}
}

func TestResolvePeer_FloodWaitIsNotMistakenForARefusal(t *testing.T) {
	// A FLOOD_WAIT that outlived its retries says nothing about the peer.
	// Swallowing it into the hash-0 fallback would hide the one actionable
	// fact (how long to wait) behind a second request that floods the same
	// way, so it must propagate from both the channel and the user path.
	cases := map[string]struct {
		invoker tg.Invoker
		peer    string
	}{
		"channel lookup": {floodingInvoker{request: floodChannels}, "-100" + int64ToString(3282239618)},
		"dialog warm":    {floodingInvoker{request: floodDialogs}, int64ToString(424242)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wrap := newWrapperWithInvoker(tc.invoker)

			_, err := wrap.ResolvePeer(t.Context(), tc.peer)
			if err == nil {
				t.Fatal("expected the FLOOD_WAIT to propagate, got a silent fallback")
			}

			if _, isFlood := tgerr.AsFloodWait(err); !isFlood {
				t.Errorf("expected a FLOOD_WAIT error, got: %v", err)
			}
		})
	}
}

func TestResolvePeer_DeadContextPropagates(t *testing.T) {
	// A cancelled context is not a verdict on the peer either. Reporting a
	// hash-0 fallback would send the caller into a second request that
	// cannot run.
	wrap := newWrapperWithInvoker(refusingChannelInvoker{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := wrap.ResolvePeer(ctx, "-100"+int64ToString(3282239618))
	if err == nil {
		t.Fatal("expected the cancelled context to propagate, got a silent fallback")
	}
}

func TestResolvePeer_ConcurrentColdChannelsShareOneRequest(t *testing.T) {
	// Two cold resolutions of the SAME channel race. Each would otherwise
	// issue its own channels.getChannels; the singleflight must collapse
	// them, as the dialog warm this path replaced already did.
	channelID := int64(3282239618)
	channelHash := int64(0xCAFEF00D)

	invoker := &blockingChannelInvoker{
		getChannelsInvoker: getChannelsInvoker{channelID: channelID, channelHash: channelHash},
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	wrap := newWrapperWithInvoker(invoker)
	ident := "-100" + int64ToString(channelID)

	hashes := make(chan int64, 2)
	resolve := func() {
		peer, err := wrap.ResolvePeer(t.Context(), ident)
		if err != nil {
			t.Errorf("concurrent resolve: %v", err)
		}

		hashes <- peer.AccessHash
	}

	go resolve()
	<-invoker.entered // the lookup is in flight with the cache still empty
	go resolve()
	close(invoker.release)

	for range 2 {
		if got := <-hashes; got != channelHash {
			t.Errorf("resolved AccessHash = %x, want %x", got, channelHash)
		}
	}

	if got := invoker.calls.Load(); got != 1 {
		t.Errorf("expected the two racing resolves to share 1 request, got %d", got)
	}
}

func TestShared_RetriesALeadersCancellationOnItsOwnContext(t *testing.T) {
	// singleflight hands the LEADER's error to every waiter. When that
	// error is the leader's own cancellation, a waiter whose request is
	// still alive must not take it as an answer: reported as-is it would
	// leave a reachable peer unresolved, which is the false verdict this
	// resolver exists to stop giving. It retries on its own context.
	wrap := newWrapperWithInvoker(refusingChannelInvoker{})

	var calls int

	err := wrap.shared(t.Context(), "channel:1", func(context.Context) error {
		calls++
		if calls == 1 {
			return context.Canceled // as if inherited from another caller
		}

		return nil
	})
	if err != nil {
		t.Fatalf("shared: %v", err)
	}

	if calls != 2 {
		t.Errorf("expected the inherited cancellation to be retried, got %d call(s)", calls)
	}
}

func TestResolvePeer_UnknownUserDoesNotRescanOnEveryCall(t *testing.T) {
	// A scan that RAN and still missed is the best answer the dialog list
	// can give. Unlatching the throttle there would make every repeat of
	// the same unknown ID pay for another full scan — two attempts at ~14 s
	// each on a large account, for an answer that will not change.
	invoker := &fakeInvoker{response: &tg.MessagesDialogs{}}
	wrap := newWrapperWithInvoker(invoker)

	ident := int64ToString(777001)

	for range 2 {
		peer, err := wrap.ResolvePeer(t.Context(), ident)
		if err != nil {
			t.Fatalf("ResolvePeer: %v", err)
		}

		if peer.AccessHash != 0 {
			t.Fatalf("expected an unresolved user, got %+v", peer)
		}
	}

	// One warm, both folders. The second resolve rides the throttle.
	if got := invoker.calls.Load(); got != 2 {
		t.Errorf("expected a single warm across both resolves (2 pages), got %d", got)
	}
}

func TestFetchChannelByID_SkipsARequestThatRacedItsWay(t *testing.T) {
	// A caller can miss the cache, queue behind a lookup for the same
	// channel, and reach the request only after that lookup filled the
	// cache. Rechecking inside the shared call keeps it from repeating a
	// request whose answer is already in hand — and from surfacing that
	// request's FLOOD_WAIT for a peer that is no longer cold.
	channelID := int64(3282239618)
	channelHash := int64(0xCAFEF00D)

	invoker := &getChannelsInvoker{channelID: channelID, channelHash: channelHash}
	wrap := newWrapperWithInvoker(invoker)
	wrap.cache.Store(InputPeer{Type: PeerChannel, ID: channelID, AccessHash: channelHash})

	err := wrap.fetchChannelByID(t.Context(), channelID)
	if err != nil {
		t.Fatalf("fetchChannelByID: %v", err)
	}

	if got := invoker.calls.Load(); got != 0 {
		t.Errorf("expected no request for an already-cached channel, got %d", got)
	}
}

func TestResolvePeer_ExhaustedRetryReportsTheCancellation(t *testing.T) {
	// The retry above is bounded at one. When it is cancelled too, the
	// caller must hear about it: a cancellation says nothing about the
	// peer, so answering with the hash-0 fallback would blame a reachable
	// channel for someone else's dead request.
	invoker := &cancellingInvoker{}
	wrap := newWrapperWithInvoker(invoker)

	_, err := wrap.ResolvePeer(t.Context(), "-100"+int64ToString(3282239618))
	if err == nil {
		t.Fatal("expected the cancellation to propagate, got a silent fallback")
	}

	if got := invoker.calls.Load(); got != 2 {
		t.Errorf("expected the lookup and one bounded retry, got %d call(s)", got)
	}
}

// cancellingInvoker answers every lookup with a cancellation the caller's
// own context did not produce.
type cancellingInvoker struct{ calls atomic.Int32 }

func (c *cancellingInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	if _, ok := input.(*tg.ChannelsGetChannelsRequest); !ok {
		return errUnexpectedRequest
	}

	c.calls.Add(1)

	return context.Canceled
}

func TestShared_ReportsItsOwnCancellation(t *testing.T) {
	// The retry above keys on the CALLER's context still being alive, so a
	// caller whose own context is dead gets the cancellation reported
	// rather than retried in a loop.
	wrap := newWrapperWithInvoker(refusingChannelInvoker{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := wrap.shared(ctx, "channel:2", func(context.Context) error {
		return context.Canceled
	})
	if err == nil {
		t.Fatal("expected the caller's own cancellation to surface")
	}
}

func TestResolvePeer_AbandonedWaiterStopsWaiting(t *testing.T) {
	// The leader holds a slow lookup open. A second caller whose own
	// request is already cancelled must not block on it — singleflight's
	// plain Do would, and the leader here is the slowest call in the
	// resolver (a full dialog warm can run for seconds).
	invoker := &blockingChannelInvoker{
		getChannelsInvoker: getChannelsInvoker{channelID: 3282239618, channelHash: 0xCAFEF00D},
		entered:            make(chan struct{}),
		release:            make(chan struct{}),
	}
	wrap := newWrapperWithInvoker(invoker)
	ident := "-100" + int64ToString(3282239618)

	go func() { _, _ = wrap.ResolvePeer(t.Context(), ident) }()
	<-invoker.entered // the leader is in flight and will not return yet

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	returned := make(chan error, 1)

	go func() {
		_, err := wrap.ResolvePeer(ctx, ident)
		returned <- err
	}()

	select {
	case err := <-returned:
		if err == nil {
			t.Error("expected the abandoned waiter to report its cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Error("the abandoned waiter blocked on the leader's call")
	}

	close(invoker.release)
}

// blockingChannelInvoker holds the first channels.getChannels open until
// released, forcing a second concurrent resolver to either share the
// in-flight request or issue its own — which the call count then catches.
type blockingChannelInvoker struct {
	getChannelsInvoker

	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingChannelInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})

	err := ctx.Err()
	if err != nil {
		return fmt.Errorf("blocked call: %w", err)
	}

	return b.getChannelsInvoker.Invoke(ctx, input, output)
}

// floodingInvoker answers one request kind with an exhausted FLOOD_WAIT.
type floodingInvoker struct{ request floodTarget }

type floodTarget int

const (
	floodChannels floodTarget = iota
	floodDialogs
)

func (f floodingInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	var match bool

	switch f.request {
	case floodChannels:
		_, match = input.(*tg.ChannelsGetChannelsRequest)
	case floodDialogs:
		_, match = input.(*tg.MessagesGetDialogsRequest)
	}

	if !match {
		return errUnexpectedRequest
	}

	return tgerr.New(420, "FLOOD_WAIT_30")
}

// warmFailInvoker fails every dialog fetch, so the warm never completes.
type warmFailInvoker struct{}

func (warmFailInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	return errTestBoom
}

// getChannelsInvoker answers channels.getChannels with the target channel
// and rejects everything else, so a test can prove the channel path never
// falls back to a dialog scan.
type getChannelsInvoker struct {
	channelID   int64
	channelHash int64
	calls       atomic.Int32
}

func (g *getChannelsInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.ChannelsGetChannelsRequest); !ok {
		return errUnexpectedRequest
	}

	g.calls.Add(1)

	resp := &tg.MessagesChats{
		Chats: []tg.ChatClass{channelWithHash(g.channelID, g.channelHash, "Test")},
	}

	return encodeAndDecode(resp, output)
}

// refusingChannelInvoker answers channels.getChannels the way the server
// does for a channel this account may not address.
type refusingChannelInvoker struct{}

func (refusingChannelInvoker) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	if _, ok := input.(*tg.ChannelsGetChannelsRequest); !ok {
		return errUnexpectedRequest
	}

	return errChannelInvalidFix
}

func TestResolvePeer_ArchivedUserResolvesEvenWhenMainHitsCap(t *testing.T) {
	// The main list never exhausts (hits the cap), but the target user's
	// dialog is archived. The archive pass must still run, otherwise the
	// user stays unresolved on an account with a long main list.
	archivedID := int64(424242)
	archivedHash := int64(0xABCDEF)

	wrap := newWrapperWithInvoker(&mainCapArchiveHitInvoker{
		archivedID:   archivedID,
		archivedHash: archivedHash,
	})

	peer, err := wrap.ResolvePeer(t.Context(), int64ToString(archivedID))
	if err != nil {
		t.Fatalf("expected the archived user to resolve despite the main cap, got: %v", err)
	}

	if peer.AccessHash != archivedHash {
		t.Errorf("resolved AccessHash = %x, want %x", peer.AccessHash, archivedHash)
	}
}

// mainCapArchiveHitInvoker never exhausts the main list (forcing the cap)
// but serves the target user as a complete archive page.
type mainCapArchiveHitInvoker struct {
	archivedID   int64
	archivedHash int64
	mainPage     atomic.Int64
}

func (m *mainCapArchiveHitInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
		return errUnexpectedRequest
	}

	if folderID, set := req.GetFolderID(); set && folderID == warmArchiveFolderID {
		resp := &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerUser{UserID: m.archivedID}, TopMessage: 1},
			},
			Users: []tg.UserClass{userWithHash(m.archivedID, m.archivedHash)},
		}

		return encodeAndDecode(resp, output)
	}

	return encodeAndDecode(warmSlice(4000+m.mainPage.Add(1)), output)
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

// warmUserInvoker serves the target user as a single complete dialog page,
// so the warm caches its access hash.
type warmUserInvoker struct {
	userID   int64
	userHash int64
}

func (w *warmUserInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	resp := &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerUser{UserID: w.userID}, TopMessage: 1},
		},
		Users: []tg.UserClass{userWithHash(w.userID, w.userHash)},
	}

	return encodeAndDecode(resp, output)
}

func userWithHash(userID, accessHash int64) *tg.User {
	usr := &tg.User{ID: userID}
	usr.SetAccessHash(accessHash)

	return usr
}

func TestResolvePeer_ThrottledUserMissRescansOnRetry(t *testing.T) {
	// A completed warm latches the throttle, so a later cold user resolve
	// within the window skips the scan and misses against a cache up to a
	// minute stale — a user first messaged on another device since would
	// stay unresolvable for the whole window. The miss must unlatch the
	// throttle so the immediate retry rescans and finds them.
	invoker := &appearingUserInvoker{userID: 424242, userHash: 0x5151ABCD}
	wrap := newWrapperWithInvoker(invoker)

	// The first warm completes on an empty dialog set and latches the throttle.
	_ = wrap.warmDialogsCache(t.Context())

	if wrap.warmedAt.Load() == 0 {
		t.Fatal("expected a completed warm to latch the throttle")
	}

	// The user now exists, but this resolve is throttled out of rescanning.
	peer, err := wrap.ResolvePeer(t.Context(), int64ToString(invoker.userID))
	if err != nil {
		t.Fatalf("a throttled miss must fall back, not error: %v", err)
	}

	if peer.AccessHash != 0 {
		t.Fatalf("expected the throttled resolve to miss, got hash %x", peer.AccessHash)
	}

	if wrap.warmedAt.Load() != 0 {
		t.Fatalf("a throttled miss must unlatch the throttle, got %d", wrap.warmedAt.Load())
	}

	retry, err := wrap.ResolvePeer(t.Context(), int64ToString(invoker.userID))
	if err != nil {
		t.Fatalf("retry: %v", err)
	}

	if retry.AccessHash != invoker.userHash {
		t.Errorf("retry should rescan and resolve, got %+v", retry)
	}
}

// appearingUserInvoker serves empty dialog pages until the target user is
// "discovered", then includes them — the shape of a user first messaged on
// another device after the last warm.
type appearingUserInvoker struct {
	userID   int64
	userHash int64
	warms    atomic.Int32
}

func (a *appearingUserInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	// The first warm scans both folders and finds nothing.
	if a.warms.Add(1) <= 2 {
		return encodeAndDecode(&tg.MessagesDialogs{}, output)
	}

	resp := &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			&tg.Dialog{Peer: &tg.PeerUser{UserID: a.userID}, TopMessage: 1},
		},
		Users: []tg.UserClass{userWithHash(a.userID, a.userHash)},
	}

	return encodeAndDecode(resp, output)
}

func TestResolvePeer_UserWarmFailureKeepsHashZeroFallback(t *testing.T) {
	// A numeric user whose warm fails must still fall back to the hash-0
	// peer rather than surfacing the warm's failure: the tools layer labels
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
	// Two cold user resolutions race. The first wins the singleflight and
	// blocks inside the warm; the second must queue on it and observe the
	// warmed cache rather than a premature miss.
	userID := int64(424242)
	userHash := int64(0xCAFEF00D)

	invoker := &blockingWarmInvoker{
		userID:   userID,
		userHash: userHash,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
	wrap := newWrapperWithInvoker(invoker)
	ident := int64ToString(userID)

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

		if got.peer.AccessHash != userHash {
			t.Errorf("resolved AccessHash = %x, want %x", got.peer.AccessHash, userHash)
		}
	}
}

// blockingWarmInvoker holds the first dialog fetch open until released, so
// a second concurrent resolver is forced to share the in-flight warm
// through the singleflight instead of observing a premature miss.
type blockingWarmInvoker struct {
	userID   int64
	userHash int64
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (b *blockingWarmInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	req, ok := input.(*tg.MessagesGetDialogsRequest)
	if !ok {
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
			&tg.Dialog{Peer: &tg.PeerUser{UserID: b.userID}, TopMessage: 1},
		},
		Users: []tg.UserClass{userWithHash(b.userID, b.userHash)},
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

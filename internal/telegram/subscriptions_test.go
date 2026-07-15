package telegram

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gotd/td/tg"
)

// errPush is the sentinel a failingUpdater returns, to drive the broker's
// push-error aggregation path.
var errPush = errors.New("push failed")

// failingUpdater records every URI it is asked to push and returns errPush for
// each, so a test can prove dispatch attempts all watching URIs and surfaces the
// error rather than stopping at the first.
type failingUpdater struct {
	mu   sync.Mutex
	uris []string
}

func (f *failingUpdater) ResourceUpdated(_ context.Context, uri string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.uris = append(f.uris, uri)

	return errPush
}

func (f *failingUpdater) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, len(f.uris))
	copy(out, f.uris)

	return out
}

// Test session tokens. The broker treats a session as an opaque comparable
// value; strings stand in for the daemon's *mcp.ServerSession.
const (
	sessA = "session-a"
	sessB = "session-b"
)

// fakeUpdater records the URIs the broker pushes, so a test can assert
// which resource a new message triggered a notification for.
type fakeUpdater struct {
	mu   sync.Mutex
	uris []string
}

func (f *fakeUpdater) ResourceUpdated(_ context.Context, uri string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.uris = append(f.uris, uri)

	return nil
}

func (f *fakeUpdater) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, len(f.uris))
	copy(out, f.uris)

	return out
}

func newMessageUpdate(peer tg.PeerClass) *tg.UpdateNewMessage {
	return &tg.UpdateNewMessage{Message: &tg.Message{PeerID: peer}}
}

func newChannelMessageUpdate(peer tg.PeerClass) *tg.UpdateNewChannelMessage {
	return &tg.UpdateNewChannelMessage{Message: &tg.Message{PeerID: peer}}
}

// containsAll reports whether got contains every want value (order-independent),
// used because the broker snapshots watching URIs from a map with no fixed order.
func containsAll(got []string, want ...string) bool {
	set := make(map[string]struct{}, len(got))
	for _, g := range got {
		set[g] = struct{}{}
	}

	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}

	return true
}

// fireMessage delivers a DM/basic-group update to the broker and fails the test
// on a handler error, keeping the assertion sites focused on the notification.
func fireMessage(t *testing.T, broker *SubscriptionBroker, update *tg.UpdateNewMessage) {
	t.Helper()

	err := broker.HandleNewMessage(context.Background(), tg.Entities{}, update)
	if err != nil {
		t.Fatalf("HandleNewMessage: %v", err)
	}
}

// fireChannel delivers a channel update to the broker and fails on a handler
// error.
func fireChannel(t *testing.T, broker *SubscriptionBroker, update *tg.UpdateNewChannelMessage) {
	t.Helper()

	err := broker.HandleNewChannelMessage(context.Background(), tg.Entities{}, update)
	if err != nil {
		t.Fatalf("HandleNewChannelMessage: %v", err)
	}
}

// TestSubscriptionBroker_WatchedPeerNotifiesExactURI pins that a message from a
// watched peer pushes a notification carrying the EXACT URI the caller
// subscribed with — the SDK's subscription registry keys on the literal URI, so
// a derived-but-different spelling would fan out to zero sessions.
func TestSubscriptionBroker_WatchedPeerNotifiesExactURI(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/42/messages"
	broker.Watch(sessA, InputPeer{Type: PeerUser, ID: 42}, uri)

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	got := updater.snapshot()
	if len(got) != 1 || got[0] != uri {
		t.Fatalf("notifications = %v, want exactly [%q]", got, uri)
	}
}

// TestSubscriptionBroker_MultipleURIsPerPeerAllNotified pins that when the same
// chat is subscribed under two different literal URIs (e.g. an @username spelling
// and a numeric one), a message notifies BOTH — the SDK fans out by the literal
// URI, so storing only one would starve every subscriber that used the other.
func TestSubscriptionBroker_MultipleURIsPerPeerAllNotified(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const (
		uriName    = "tg://chat/@alice/messages"
		uriNumeric = "tg://chat/42/messages"
	)

	peer := InputPeer{Type: PeerUser, ID: 42}
	broker.Watch(sessA, peer, uriName)
	broker.Watch(sessB, peer, uriNumeric)

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	got := updater.snapshot()
	if len(got) != 2 || !containsAll(got, uriName, uriNumeric) {
		t.Fatalf("notifications = %v, want both %q and %q", got, uriName, uriNumeric)
	}

	// Dropping one spelling must not starve the other.
	broker.Unwatch(sessA, uriName)
	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	got = updater.snapshot()
	if len(got) != 3 || got[2] != uriNumeric {
		t.Fatalf("after dropping %q, third notification = %v, want %q", uriName, got, uriNumeric)
	}
}

// TestSubscriptionBroker_UnwatchedPeerIsSilent pins that a message from a peer
// nobody subscribed to fires nothing, even while another peer is watched.
func TestSubscriptionBroker_UnwatchedPeerIsSilent(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	broker.Watch(sessA, InputPeer{Type: PeerUser, ID: 42}, "tg://chat/42/messages")

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 99}))

	if got := updater.snapshot(); len(got) != 0 {
		t.Fatalf("notifications = %v, want none from an unwatched peer", got)
	}
}

// TestSubscriptionBroker_SameIDDifferentTypeNoCrossFire pins that peer identity
// includes the TYPE: a watch on channel:1234 must not fire for a message from
// user:1234. The key carries peerType, so the two are distinct chats.
func TestSubscriptionBroker_SameIDDifferentTypeNoCrossFire(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	broker.Watch(sessA, InputPeer{Type: PeerChannel, ID: 1234}, "tg://chat/-1001234/messages")

	// A user with the same numeric id must not match the channel watch.
	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 1234}))

	if got := updater.snapshot(); len(got) != 0 {
		t.Fatalf("a user:1234 message fired a channel:1234 watch: %v", got)
	}

	// The channel itself does fire.
	fireChannel(t, broker, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: 1234}))

	if got := updater.snapshot(); len(got) != 1 {
		t.Fatalf("channel:1234 message did not fire its own watch: %v", got)
	}
}

// TestSubscriptionBroker_ChannelMessagePath pins that channel updates route
// through the same peer-identity match as DM/group updates.
func TestSubscriptionBroker_ChannelMessagePath(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/-1001234/messages"
	broker.Watch(sessA, InputPeer{Type: PeerChannel, ID: 1234}, uri)

	fireChannel(t, broker, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: 1234}))

	got := updater.snapshot()
	if len(got) != 1 || got[0] != uri {
		t.Fatalf("notifications = %v, want exactly [%q]", got, uri)
	}
}

// TestSubscriptionBroker_TwoSessionsNeedTwoUnwatches pins that a URI shared by
// two sessions stays watched until BOTH unsubscribe — the subscriber set, not a
// single flag, gates the watch.
func TestSubscriptionBroker_TwoSessionsNeedTwoUnwatches(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/42/messages"
	peer := InputPeer{Type: PeerUser, ID: 42}

	broker.Watch(sessA, peer, uri)
	broker.Watch(sessB, peer, uri)
	broker.Unwatch(sessA, uri)

	// One session remains — still watched.
	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	if got := updater.snapshot(); len(got) != 1 {
		t.Fatalf("after one of two unwatches: notifications = %v, want one", got)
	}

	broker.Unwatch(sessB, uri)

	// Last session gone — now silent.
	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	if got := updater.snapshot(); len(got) != 1 {
		t.Fatalf("after both unwatches: notifications = %v, want still one (no new)", got)
	}
}

// TestSubscriptionBroker_DuplicateWatchIsIdempotent pins that the same session
// subscribing to the same URI twice counts once, so a single unwatch drops it —
// the SDK calls SubscribeHandler even for a duplicate, and the broker must not
// double-count.
func TestSubscriptionBroker_DuplicateWatchIsIdempotent(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/42/messages"
	peer := InputPeer{Type: PeerUser, ID: 42}

	broker.Watch(sessA, peer, uri)
	broker.Watch(sessA, peer, uri) // duplicate from the same session
	broker.Unwatch(sessA, uri)

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	if got := updater.snapshot(); len(got) != 0 {
		t.Fatalf("a duplicate watch survived one unwatch: %v", got)
	}
}

// TestSubscriptionBroker_UnmatchedUnwatchCannotDropAnothersWatch pins that an
// unsubscribe from a session that never subscribed to a URI does not drop the
// watch held by a different session. The SDK invokes UnsubscribeHandler before
// checking its own registry, so a spurious/cross-session unsubscribe reaches the
// broker and must be a no-op.
func TestSubscriptionBroker_UnmatchedUnwatchCannotDropAnothersWatch(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/42/messages"
	peer := InputPeer{Type: PeerUser, ID: 42}

	broker.Watch(sessA, peer, uri)

	// sessB never subscribed to this URI; its unsubscribe must not affect sessA.
	broker.Unwatch(sessB, uri)
	broker.Unwatch(sessB, uri) // and a repeat is still a no-op

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	if got := updater.snapshot(); len(got) != 1 || got[0] != uri {
		t.Fatalf("a cross-session unwatch dropped an active watch: %v", got)
	}
}

// TestSubscriptionBroker_UnwatchUnknownIsSafe pins that an Unwatch with no
// matching Watch (the disconnect-drift edge, or a never-watched URI) neither
// panics nor removes anything.
func TestSubscriptionBroker_UnwatchUnknownIsSafe(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/42/messages"

	broker.Unwatch(sessA, uri) // never watched
	broker.Watch(sessA, InputPeer{Type: PeerUser, ID: 42}, uri)
	broker.Unwatch(sessA, uri)
	broker.Unwatch(sessA, uri) // extra unwatch must be a no-op

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	if got := updater.snapshot(); len(got) != 0 {
		t.Fatalf("notifications = %v, want none after balanced watch/unwatch", got)
	}
}

// TestSubscriptionBroker_URIReassignedToNewPeerEvictsOld pins the single-owner
// invariant: when a URI is left stale under one peer (drift) and then watched for
// a different peer — the same @username reassigned to another chat — the old
// mapping is evicted, so a message in the OLD chat does not fire the URI now
// aimed at the NEW subscribers.
func TestSubscriptionBroker_URIReassignedToNewPeerEvictsOld(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/@handle/messages"

	oldPeer := InputPeer{Type: PeerUser, ID: 1}
	newPeer := InputPeer{Type: PeerChannel, ID: 2}

	broker.Watch(sessA, oldPeer, uri) // subscribed while @handle == oldPeer, never unwatched
	broker.Watch(sessA, newPeer, uri) // @handle reassigned; the stale oldPeer mapping must be evicted

	// A message in the OLD chat must NOT fire the URI now aimed at newPeer.
	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 1}))

	if got := updater.snapshot(); len(got) != 0 {
		t.Fatalf("old-peer message fired a notification for a reassigned URI: %v", got)
	}

	// A message in the NEW chat fires it exactly once.
	fireChannel(t, broker, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: 2}))

	if got := updater.snapshot(); len(got) != 1 || got[0] != uri {
		t.Fatalf("new-peer message = %v, want exactly [%q]", got, uri)
	}
}

// TestSubscriptionBroker_ReassignmentPreservesSubscribers pins that moving a
// URI's trigger to a new peer carries its subscriber set over: an earlier
// subscriber unwatching must not remove the watch while a later subscriber is
// still active.
func TestSubscriptionBroker_ReassignmentPreservesSubscribers(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/@handle/messages"

	oldPeer := InputPeer{Type: PeerUser, ID: 1}
	newPeer := InputPeer{Type: PeerChannel, ID: 2}

	broker.Watch(sessA, oldPeer, uri) // session A, while @handle == oldPeer
	broker.Watch(sessB, newPeer, uri) // session B, after @handle was reassigned to newPeer

	// A unsubscribes. B is still subscribed, so the watch must survive.
	broker.Unwatch(sessA, uri)

	fireChannel(t, broker, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: 2}))

	if got := updater.snapshot(); len(got) != 1 || got[0] != uri {
		t.Fatalf("after one of two unsubscribes, new-peer message = %v, want exactly [%q]", got, uri)
	}

	// B unsubscribes too — now the watch is gone.
	broker.Unwatch(sessB, uri)

	fireChannel(t, broker, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: 2}))

	if got := updater.snapshot(); len(got) != 1 {
		t.Fatalf("after both unsubscribes: notifications = %v, want still one (no new)", got)
	}
}

// TestSubscriptionBroker_WatchIgnoresZeroPeer pins the defensive guard: watching
// the ID-0 absent sentinel registers nothing, so a caller that passes an
// unresolved peer cannot create a watch that could never match a real message.
func TestSubscriptionBroker_WatchIgnoresZeroPeer(t *testing.T) {
	broker := NewSubscriptionBroker()

	broker.Watch(sessA, InputPeer{}, "tg://chat/0/messages")

	broker.mu.Lock()
	subs := len(broker.subscribers)
	owners := len(broker.owner)
	peers := len(broker.peerURIs)
	broker.mu.Unlock()

	if subs != 0 || owners != 0 || peers != 0 {
		t.Fatalf("Watch of a zero peer registered state: subscribers=%d owner=%d peerURIs=%d, want all 0",
			subs, owners, peers)
	}
}

// TestSubscriptionBroker_NoPeerIsSilent pins that a message carrying no
// resolvable peer (MessageEmpty, or a self-send echo whose upconverted
// UpdateNewMessage has no PeerID) is dropped without a notification.
func TestSubscriptionBroker_NoPeerIsSilent(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	broker.Watch(sessA, InputPeer{Type: PeerUser, ID: 42}, "tg://chat/42/messages")

	// MessageEmpty: AsNotEmpty reports ok=false.
	fireMessage(t, broker, &tg.UpdateNewMessage{Message: &tg.MessageEmpty{}})

	// Message with a nil PeerID: extractPeerID yields ID 0.
	fireMessage(t, broker, newMessageUpdate(nil))

	if got := updater.snapshot(); len(got) != 0 {
		t.Fatalf("notifications = %v, want none for peerless updates", got)
	}
}

// TestSubscriptionBroker_NoNotifierIsSafe pins that a matching update before a
// notifier is wired (or a nil broker) neither panics nor errors.
func TestSubscriptionBroker_NoNotifierIsSafe(t *testing.T) {
	broker := NewSubscriptionBroker()
	broker.Watch(sessA, InputPeer{Type: PeerUser, ID: 42}, "tg://chat/42/messages")

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	var nilBroker *SubscriptionBroker

	fireMessage(t, nilBroker, newMessageUpdate(&tg.PeerUser{UserID: 42}))
}

// TestSubscriptionBroker_DispatchAttemptsAllURIsAndReturnsError pins the dispatch
// contract: when a peer triggers two URIs and the notifier fails, BOTH are
// attempted (one bad push does not skip the rest) and the first error surfaces
// through the handler.
func TestSubscriptionBroker_DispatchAttemptsAllURIsAndReturnsError(t *testing.T) {
	updater := &failingUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const (
		uriA = "tg://chat/@alice/messages"
		uriB = "tg://chat/42/messages"
	)

	peer := InputPeer{Type: PeerUser, ID: 42}
	broker.Watch(sessA, peer, uriA)
	broker.Watch(sessB, peer, uriB)

	err := broker.HandleNewMessage(context.Background(), tg.Entities{}, newMessageUpdate(&tg.PeerUser{UserID: 42}))
	if !errors.Is(err, errPush) {
		t.Fatalf("dispatch error = %v, want it to wrap errPush", err)
	}

	if got := updater.snapshot(); len(got) != 2 || !containsAll(got, uriA, uriB) {
		t.Fatalf("attempted URIs = %v, want both %q and %q despite the first failing", got, uriA, uriB)
	}
}

// TestSubscriptionBroker_AbandonedSessionLingers pins the documented drift: a
// session that is never Unwatched (the abnormal-disconnect edge, since the SDK
// does not call UnsubscribeHandler on disconnect) keeps its entry — the URI is
// never pruned and keeps firing, and owner/peerURIs stay populated. This is the
// leak the docs warn about; it is cleared only by a process restart.
func TestSubscriptionBroker_AbandonedSessionLingers(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const uri = "tg://chat/42/messages"
	peer := InputPeer{Type: PeerUser, ID: 42}

	broker.Watch(sessA, peer, uri) // subscribed, then the session vanishes without Unwatch

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))
	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	// The watch keeps firing — nothing pruned it.
	if got := updater.snapshot(); len(got) != 2 {
		t.Fatalf("abandoned watch stopped firing: notifications = %v, want 2", got)
	}

	// And the entry is still present in every index (the leak, until restart).
	broker.mu.Lock()
	_, sub := broker.subscribers[uri]
	_, own := broker.owner[uri]
	_, pu := broker.peerURIs[subscriptionKey{peerType: peer.Type, peerID: peer.ID}]
	broker.mu.Unlock()

	if !sub || !own || !pu {
		t.Fatalf("abandoned session was pruned: subscribers=%v owner=%v peerURIs=%v, want all present", sub, own, pu)
	}
}

// TestSubscriptionBroker_ConcurrentAccess drives Watch, Unwatch and both handler
// paths from many goroutines so the race detector can prove the broker's locking
// is sound.
func TestSubscriptionBroker_ConcurrentAccess(t *testing.T) {
	updater := &fakeUpdater{}
	broker := NewSubscriptionBroker()
	broker.SetNotifier(updater)

	const workers = 32

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func(n int) {
			defer wg.Done()

			session := n
			peer := InputPeer{Type: PeerUser, ID: int64(n%4 + 1)}
			uri := "tg://chat/" + string(rune('a'+n%4)) + "/messages"

			broker.Watch(session, peer, uri)
			_ = broker.HandleNewMessage(context.Background(), tg.Entities{}, newMessageUpdate(&tg.PeerUser{UserID: peer.ID}))
			_ = broker.HandleNewChannelMessage(
				context.Background(), tg.Entities{}, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: peer.ID}),
			)
			broker.Unwatch(session, uri)
		}(i)
	}

	wg.Wait()
}

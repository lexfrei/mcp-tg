package telegram

import (
	"context"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
)

// ResourceUpdater is the minimal slice of the MCP server the subscription
// broker needs to push a resource-updated notification. Keeping it local means
// internal/telegram carries no dependency on the MCP SDK; the daemon wires an
// adapter over *mcp.Server.
type ResourceUpdater interface {
	ResourceUpdated(ctx context.Context, uri string) error
}

// subscriptionKey identifies a watched chat by peer identity. Access hash is
// deliberately excluded: an incoming update's peer and the subscribed peer are
// the same chat iff their (type, id) match, regardless of which access hash
// each was resolved through.
type subscriptionKey struct {
	peerType PeerType
	peerID   int64
}

// SubscriptionBroker maps incoming Telegram messages to the MCP resource URIs
// that watch their chat, and pushes a resources/updated notification for each
// match. It is process-global shared state — the headless daemon serves many
// MCP sessions from one update goroutine — so every method is safe for
// concurrent use.
//
// Bookkeeping mirrors the MCP SDK, which gates delivery by the LITERAL URI a
// client subscribed with (not by peer) and tracks a SET of sessions per URI. So
// the broker tracks the same: the set of subscribing sessions per URI, which
// peer currently triggers each URI, and a peer→URIs reverse index for message
// lookup. Keying by URI (not peer) keeps it correct when several URI spellings
// resolve to one chat and when a URI's @username is reassigned to a different
// chat. Keying subscribers by SESSION (not a plain count) makes Watch and
// Unwatch idempotent per session: the SDK calls the handlers before checking its
// own registry, so a duplicate subscribe or an unmatched unsubscribe from
// another session must not skew a shared watch.
//
// Subscription drift is a known limitation, cleared only by a process restart:
// the MCP SDK removes a disconnected session from its OWN subscription registry
// but does not call the UnsubscribeHandler, so the broker's session set is not
// pruned on an abnormal disconnect (a crash, or network loss past the KeepAlive
// window). Delivery stays correct — the SDK's registry is authoritative, so a
// dead session receives nothing — but two costs accrue over the daemon's
// lifetime, and neither is bounded by the set of distinct chats: the retained
// (URI, session) entries grow with cumulative reconnect churn, since each
// reconnect mints a fresh *ServerSession; and because a lingering dead session
// keeps subscribers[uri] non-empty, that URI's owner/peerURIs are never cleaned,
// so every later message in the chat costs a peer lookup and one
// subscriber_count=0 log line. There is no clean fix: ServerOptions exposes no
// session-closed callback, the headless path never holds the per-session handle,
// and a prune-on-zero-live-subscribers scheme would race Watch (which runs
// before the SDK registers the subscription).
type SubscriptionBroker struct {
	mu sync.Mutex
	// subscribers is the set of subscribing sessions per exact URI, mirroring the
	// SDK's own per-URI session registry. A URI is watched while its set is
	// non-empty. Session identity is an opaque comparable token (the daemon
	// passes *mcp.ServerSession); the broker only ever compares it.
	subscribers map[string]map[any]struct{}
	// owner records which peer's messages currently trigger each URI. The latest
	// resolution wins, so a URI whose @username was reassigned to another chat
	// fires for the new peer only.
	owner map[string]subscriptionKey
	// peerURIs is the reverse index — the set of URIs a given peer triggers —
	// used to find the watchers when a message arrives.
	peerURIs map[subscriptionKey]map[string]struct{}
	notifier ResourceUpdater
}

// NewSubscriptionBroker creates an empty broker with no notifier wired yet.
func NewSubscriptionBroker() *SubscriptionBroker {
	return &SubscriptionBroker{
		subscribers: make(map[string]map[any]struct{}),
		owner:       make(map[string]subscriptionKey),
		peerURIs:    make(map[subscriptionKey]map[string]struct{}),
	}
}

// SetNotifier wires the sink that receives resource-updated URIs. The server is
// built after the update dispatcher is registered, so the notifier arrives after
// construction rather than in NewSubscriptionBroker.
func (b *SubscriptionBroker) SetNotifier(notifier ResourceUpdater) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.notifier = notifier
}

// Watch registers one session's subscription to a chat under a specific exact
// URI. A peer may be watched under several distinct URIs at once, and each is
// notified, because the SDK fans out by the literal subscribed string. Repeated
// Watches by the same session under the same URI are idempotent. A peer with no
// ID (the absent sentinel) is ignored.
//
// If the URI currently triggers a different peer — a stale mapping left by drift
// now that the same @username resolves elsewhere — the trigger is moved to the
// new peer so old-peer messages no longer fire it; the URI's subscriber set is
// preserved across the move.
func (b *SubscriptionBroker) Watch(session any, peer InputPeer, uri string) {
	if peer.ID == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	key := subscriptionKey{peerType: peer.Type, peerID: peer.ID}

	if prev, ok := b.owner[uri]; ok && prev != key {
		b.detach(prev, uri)
	}

	b.attach(key, uri)
	b.owner[uri] = key

	subs := b.subscribers[uri]
	if subs == nil {
		subs = make(map[any]struct{})
		b.subscribers[uri] = subs
	}

	subs[session] = struct{}{}
}

// Unwatch drops one session's subscription to a URI, removing the watch when its
// last subscriber leaves. It is keyed by URI alone — the SDK tracks sessions per
// URI, so unsubscribe needs no peer and never resolves one. An Unwatch from a
// session that never subscribed to this URI (an unmatched unsubscribe, or the
// disconnect-drift edge) is a no-op, so one client cannot drop another's watch.
func (b *SubscriptionBroker) Unwatch(session any, uri string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, ok := b.subscribers[uri]
	if !ok {
		return
	}

	if _, ok := subs[session]; !ok {
		return
	}

	delete(subs, session)

	if len(subs) > 0 {
		return
	}

	delete(b.subscribers, uri)

	if key, ok := b.owner[uri]; ok {
		b.detach(key, uri)
		delete(b.owner, uri)
	}
}

// HandleNewMessage satisfies tg.NewMessageHandler: it routes a DM/basic-group
// update to any watching resource. updateShortMessage/updateShortChatMessage
// upconvert to this update WITH a proper PeerID.
func (b *SubscriptionBroker) HandleNewMessage(
	ctx context.Context, _ tg.Entities, update *tg.UpdateNewMessage,
) error {
	if b == nil || update == nil {
		return nil
	}

	return b.dispatch(ctx, update.Message)
}

// HandleNewChannelMessage satisfies tg.NewChannelMessageHandler: it routes a
// channel/supergroup update to any watching resource.
func (b *SubscriptionBroker) HandleNewChannelMessage(
	ctx context.Context, _ tg.Entities, update *tg.UpdateNewChannelMessage,
) error {
	if b == nil || update == nil {
		return nil
	}

	return b.dispatch(ctx, update.Message)
}

// attach adds a URI to a peer's trigger set. The caller must hold b.mu.
func (b *SubscriptionBroker) attach(key subscriptionKey, uri string) {
	uris := b.peerURIs[key]
	if uris == nil {
		uris = make(map[string]struct{})
		b.peerURIs[key] = uris
	}

	uris[uri] = struct{}{}
}

// detach removes a URI from a peer's trigger set, dropping the peer entry when
// its last URI goes. The caller must hold b.mu.
func (b *SubscriptionBroker) detach(key subscriptionKey, uri string) {
	uris := b.peerURIs[key]
	delete(uris, uri)

	if len(uris) == 0 {
		delete(b.peerURIs, key)
	}
}

// dispatch resolves the message's peer to the URIs watching it and calls the
// notifier for each, OUTSIDE the lock so concurrent Watch/Unwatch calls are not
// blocked. This does NOT protect the caller's goroutine: dispatch runs on the
// caller's stack (in the daemon, gotd's single update-read loop), so the notifier
// itself must be non-blocking or a slow client would stall update processing for
// everyone — the daemon's resourceUpdater hands each push to a goroutine for
// exactly that reason. Every watching URI is notified because the SDK fans out by
// the literal subscribed string. The first push error is returned after all URIs
// are attempted, so one bad URI does not skip the rest (inert in the daemon,
// where the async notifier always returns nil).
func (b *SubscriptionBroker) dispatch(ctx context.Context, msg tg.MessageClass) error {
	uris, notifier := b.lookup(msg)
	if notifier == nil {
		return nil
	}

	var firstErr error

	for _, uri := range uris {
		err := notifier.ResourceUpdated(ctx, uri)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return errors.Wrap(firstErr, "pushing resource update")
}

// lookup snapshots the watching URIs and the notifier for a message's peer under
// the lock. It returns a nil notifier when the message carries no resolvable
// peer, no resource watches that peer, or no notifier is wired yet.
func (b *SubscriptionBroker) lookup(msg tg.MessageClass) ([]string, ResourceUpdater) {
	peer := messagePeer(msg)
	if peer.ID == 0 {
		return nil, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	uris, ok := b.peerURIs[subscriptionKey{peerType: peer.Type, peerID: peer.ID}]
	if !ok || len(uris) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(uris))
	for uri := range uris {
		out = append(out, uri)
	}

	return out, b.notifier
}

// messagePeer extracts the destination peer of a message update. A MessageEmpty
// (AsNotEmpty reports ok=false) or a message with a nil/unknown PeerID yields the
// absent-sentinel {ID: 0} — the self-send echo shape, which carries no peer.
func messagePeer(msg tg.MessageClass) InputPeer {
	if msg == nil {
		return InputPeer{}
	}

	notEmpty, ok := msg.AsNotEmpty()
	if !ok {
		return InputPeer{}
	}

	return extractPeerID(notEmpty.GetPeerID())
}

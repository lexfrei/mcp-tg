package main

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-tg/internal/resources"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
)

// errPeerResolvedToNothing is returned when a subscribed URI resolves to the
// ID-0 sentinel — a numeric 0 or any peer the client has never seen — which the
// broker would silently refuse to watch.
var errPeerResolvedToNothing = errors.New("subscribed peer resolved to no chat")

// resourceUpdater adapts *mcp.Server to the telegram.ResourceUpdater interface,
// letting the subscription broker push resources/updated notifications without
// the internal/telegram package importing the MCP SDK.
//
// It hands each push to a goroutine and returns immediately. This is load-bearing
// for the headless shared daemon: the broker calls ResourceUpdated from gotd's
// SINGLE update-read goroutine, and Server.ResourceUpdated writes to every
// subscribed session synchronously (a 10s SDK timeout each), so a slow or
// half-open client would otherwise stall update processing — and the
// co-registered transcription handler — for every client. The push runs under a
// detached context because the read loop's ctx is cancelled on Telegram
// reconnect, and MCP delivery is independent of the Telegram connection. The push
// is stored as a func so a test can substitute a blocking one and assert the
// caller is not blocked.
type resourceUpdater struct {
	push func(context.Context, string) error
}

func newResourceUpdater(server *mcp.Server) resourceUpdater {
	return resourceUpdater{
		push: func(ctx context.Context, uri string) error {
			return server.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: uri})
		},
	}
}

func (r resourceUpdater) ResourceUpdated(_ context.Context, uri string) error {
	// Detached context is deliberate: the caller's ctx is the gotd read-loop ctx,
	// cancelled on Telegram reconnect, and MCP delivery is independent of the
	// Telegram connection. See the type doc for why the push is off the caller's
	// goroutine.
	go func() { //nolint:gosec,contextcheck // G118/contextcheck: detached on purpose (see above).
		_ = r.push(context.Background(), uri)
	}()

	return nil
}

// resolveWatchedPeer resolves the peer of a tg://chat/<peer>/messages URI and
// hands it to apply. A non-messages URI (e.g. the bare chat-info resource) is a
// no-op — apply is not called — since only the messages resource streams
// updates. A peer that fails to resolve returns the error so the RPC fails,
// telling the client the URI is unusable rather than silently going dark.
//
// A resolution that yields the ID-0 sentinel is treated as a failure too:
// resolveByID returns {PeerChat, 0} with a nil error for a numeric 0, and the
// broker's Watch drops any ID-0 peer — so without this guard the subscribe would
// report success yet never push, the exact silent-dark outcome above.
func resolveWatchedPeer(
	ctx context.Context, client tgclient.Client, uri string, apply func(tgclient.InputPeer),
) error {
	peerStr := resources.ChatMessagesPeer(uri)
	if peerStr == "" {
		return nil
	}

	peer, err := client.ResolvePeer(ctx, peerStr)
	if err != nil {
		return errors.Wrap(err, "resolving watched peer")
	}

	if peer.ID == 0 {
		return errors.Wrapf(errPeerResolvedToNothing, "peer %q", peerStr)
	}

	apply(peer)

	return nil
}

// newSubscribeHandler returns the MCP SubscribeHandler: it registers a broker
// watch on the subscribed chat so later messages there fire a resources/updated
// notification for the exact URI.
func newSubscribeHandler(
	client tgclient.Client, broker *tgclient.SubscriptionBroker,
) func(context.Context, *mcp.SubscribeRequest) error {
	return func(ctx context.Context, req *mcp.SubscribeRequest) error {
		return resolveWatchedPeer(ctx, client, req.Params.URI, func(peer tgclient.InputPeer) {
			broker.Watch(req.Session, peer, req.Params.URI)
		})
	}
}

// newUnsubscribeHandler drops this session's watch on a URI. It needs no client
// and resolves no peer: the SDK tracks sessions per literal URI, so the broker
// drops the watch by that same (session, URI). This is deliberately asymmetric
// with subscribe — resolving here would add a needless round-trip and, worse, a
// resolution failure (the username was changed, a transient blip) could return
// an error before the SDK removes the subscription, leaving the client stuck
// receiving updates. An unsubscribe for a URI this session never watched (e.g.
// the bare chat-info resource) is a harmless no-op.
func newUnsubscribeHandler(
	broker *tgclient.SubscriptionBroker,
) func(context.Context, *mcp.UnsubscribeRequest) error {
	return func(_ context.Context, req *mcp.UnsubscribeRequest) error {
		broker.Unwatch(req.Session, req.Params.URI)

		return nil
	}
}

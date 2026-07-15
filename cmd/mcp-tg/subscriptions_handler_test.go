package main

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
)

// failingResolver resolves every peer with an error, simulating a changed
// username or a transient network failure at (un)subscribe time.
type failingResolver struct {
	testutil.NoopClient
}

func (failingResolver) ResolvePeer(_ context.Context, _ string) (tgclient.InputPeer, error) {
	return tgclient.InputPeer{}, errors.New("resolve failed")
}

// zeroResolver resolves every peer to the ID-0 sentinel with a nil error — what
// resolveByID returns for a numeric 0 or a never-seen peer.
type zeroResolver struct {
	testutil.NoopClient
}

func (zeroResolver) ResolvePeer(_ context.Context, _ string) (tgclient.InputPeer, error) {
	return tgclient.InputPeer{}, nil
}

// TestSubscribeHandler_FailsWhenPeerUnresolvable pins that subscribe surfaces a
// resolution failure, so the client learns the URI is unusable rather than
// silently receiving no updates.
func TestSubscribeHandler_FailsWhenPeerUnresolvable(t *testing.T) {
	broker := tgclient.NewSubscriptionBroker()
	handler := newSubscribeHandler(failingResolver{}, broker)

	err := handler(context.Background(), &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: "tg://chat/@x/messages"},
	})
	if err == nil {
		t.Fatal("subscribe must fail when the peer cannot be resolved")
	}
}

// TestUnsubscribeHandler_DropsWatchWithoutResolving pins the asymmetry with
// subscribe: unsubscribe resolves no peer at all (it drops the watch by URI), so
// it always returns nil and can never be blocked by a resolution failure that
// would leave the client stuck receiving updates for a URI it asked to stop.
func TestUnsubscribeHandler_DropsWatchWithoutResolving(t *testing.T) {
	broker := tgclient.NewSubscriptionBroker()
	handler := newUnsubscribeHandler(broker)

	err := handler(context.Background(), &mcp.UnsubscribeRequest{
		Params: &mcp.UnsubscribeParams{URI: "tg://chat/@x/messages"},
	})
	if err != nil {
		t.Fatalf("unsubscribe must always succeed, got: %v", err)
	}
}

// TestSubscribeHandler_FailsWhenPeerResolvesToZero pins that a resolution
// yielding the ID-0 sentinel (nil error) fails the subscribe — otherwise the
// broker's Watch would silently drop the ID-0 peer and the client would report
// success yet never receive updates.
func TestSubscribeHandler_FailsWhenPeerResolvesToZero(t *testing.T) {
	broker := tgclient.NewSubscriptionBroker()
	handler := newSubscribeHandler(zeroResolver{}, broker)

	err := handler(context.Background(), &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: "tg://chat/0/messages"},
	})
	if !errors.Is(err, errPeerResolvedToNothing) {
		t.Fatalf("subscribe error = %v, want it to wrap errPeerResolvedToNothing", err)
	}
}

// TestSubscribeHandler_IgnoresNonMessagesURI pins that subscribing to the bare
// chat-info resource is a no-op success — it is never resolved or watched, so a
// failing resolver is irrelevant.
func TestSubscribeHandler_IgnoresNonMessagesURI(t *testing.T) {
	broker := tgclient.NewSubscriptionBroker()
	handler := newSubscribeHandler(failingResolver{}, broker)

	err := handler(context.Background(), &mcp.SubscribeRequest{
		Params: &mcp.SubscribeParams{URI: "tg://chat/@x"},
	})
	if err != nil {
		t.Fatalf("subscribe to a non-messages URI must be a no-op success, got: %v", err)
	}
}

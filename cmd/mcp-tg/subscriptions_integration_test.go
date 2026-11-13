package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-tg/internal/middleware"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
)

// resolvingClient overrides NoopClient.ResolvePeer to map any identifier to a
// fixed peer, so the subscribe handler registers a real watch (NoopClient
// resolves everything to the ID-0 absent sentinel, which Watch ignores).
type resolvingClient struct {
	testutil.NoopClient

	peer tgclient.InputPeer
}

func (c resolvingClient) ResolvePeer(_ context.Context, _ string) (tgclient.InputPeer, error) {
	return c.peer, nil
}

// TestResourceUpdater_PushIsNonBlocking pins that the daemon's notifier hands the
// push to a goroutine and returns immediately, so a backpressured client's
// transport write cannot stall the gotd update-read loop that drives the broker.
// If ResourceUpdated ever became synchronous again, the blocking push below would
// hang the call and this test would time out.
func TestResourceUpdater_PushIsNonBlocking(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	updater := resourceUpdater{
		push: func(_ context.Context, _ string) error {
			close(started)
			<-release // simulate a slow / half-open client

			return nil
		},
	}

	done := make(chan error, 1)
	go func() { done <- updater.ResourceUpdated(context.Background(), "tg://chat/1/messages") }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ResourceUpdated returned an error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ResourceUpdated blocked the caller while the push was in flight")
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the detached push never ran")
	}
}

// TestSubscription_EndToEnd drives the full subscribe → new-message → notify
// path over the in-memory transport, exactly the production seam buildServer.
// It pins two contracts at once: the server advertises Resources.Subscribe once
// both handlers are wired, and a message in a subscribed chat delivers a
// resources/updated notification carrying that chat's exact URI.
func TestSubscription_EndToEnd(t *testing.T) {
	ctx := context.Background()

	const (
		selfID = int64(777000)
		uri    = "tg://chat/777000/messages"
	)

	authDone := make(chan struct{})
	close(authDone) // auth completed so the guard lets calls through

	health := middleware.NewSessionHealth()
	health.Arm()

	broker := tgclient.NewSubscriptionBroker()
	client := resolvingClient{peer: tgclient.InputPeer{Type: tgclient.PeerUser, ID: selfID}}

	server := buildServer(client, "/tmp/mcp-tg/downloads", nil, broker, authDone, health, nil, discardLogger())

	ct, st := mcp.NewInMemoryTransports()

	ss, connErr := server.Connect(ctx, st, nil)
	if connErr != nil {
		t.Fatalf("server connect: %v", connErr)
	}

	defer ss.Wait()

	updates := make(chan string, 1)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			select {
			case updates <- req.Params.URI:
			default:
			}
		},
	})

	cs, clientErr := mcpClient.Connect(ctx, ct, nil)
	if clientErr != nil {
		t.Fatalf("client connect: %v", clientErr)
	}

	defer func() { _ = cs.Close() }()

	// The subscribe capability must be advertised — it is what makes an MCP
	// client offer subscribe/unsubscribe at all.
	caps := cs.InitializeResult().Capabilities
	if caps.Resources == nil || !caps.Resources.Subscribe {
		t.Fatalf("server did not advertise Resources.Subscribe, capabilities: %+v", caps.Resources)
	}

	subErr := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: uri})
	if subErr != nil {
		t.Fatalf("subscribe: %v", subErr)
	}

	// A new message in the subscribed chat, delivered as a pushed update
	// carrying the peer (the from-another-device shape, not the self-send echo).
	//
	// The message is fired repeatedly because Subscribe returning does not mean
	// the server has registered the watch. Under protocol 2026-07-28 the legacy
	// resources/subscribe RPC is gone: Subscribe opens a long-lived
	// subscriptions/listen stream carrying the URI and returns as soon as it is
	// sent, while the server registers the subscription when it handles that
	// stream. Firing once raced that gap and the notification was simply
	// addressed to nobody. A sleep would hide the race rather than wait for it.
	fire := func() {
		if err := broker.HandleNewMessage(
			ctx, tg.Entities{}, &tg.UpdateNewMessage{Message: &tg.Message{PeerID: &tg.PeerUser{UserID: selfID}}},
		); err != nil {
			t.Errorf("HandleNewMessage: %v", err)
		}
	}

	deadline := time.After(5 * time.Second)
	retry := time.NewTicker(50 * time.Millisecond)

	defer retry.Stop()

	fire()

	for {
		select {
		case got := <-updates:
			if got != uri {
				t.Fatalf("resources/updated URI = %q, want %q", got, uri)
			}

			return
		case <-retry.C:
			fire()
		case <-deadline:
			t.Fatal("timed out waiting for resources/updated notification")
		}
	}
}

// sdkEventHandler forwards the messages of the SDK's own log records about one
// URI: the SDK logs them right after it updates its subscription registry, and
// exposes no other hook at that point. The message text is the SDK's own log
// text; if it changes, waitForSDKEvent fails rather than passing.
type sdkEventHandler struct {
	uri    string
	events chan<- string
}

func (h sdkEventHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h sdkEventHandler) Handle(_ context.Context, rec slog.Record) error {
	rec.Attrs(func(attr slog.Attr) bool {
		if attr.Key != "uri" || attr.Value.String() != h.uri {
			return true
		}

		select {
		case h.events <- rec.Message:
		default:
		}

		return false
	})

	return nil
}

func (h sdkEventHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h sdkEventHandler) WithGroup(string) slog.Handler { return h }

func waitForSDKEvent(t *testing.T, events <-chan string, want string) {
	t.Helper()

	deadline := time.After(5 * time.Second)

	for {
		select {
		case got := <-events:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("server never logged %q", want)
		}
	}
}

// TestSubscription_UnsubscribeStopsUpdates pins that once the server has
// processed an unsubscribe, a new message in the same chat no longer delivers a
// notification — the broker drops the watch and the SDK drops the session from
// its registry.
//
// Under protocol 2026-07-28 ClientSession.Unsubscribe only cancels the
// subscriptions/listen stream on the client; it does not wait for the server to
// process the cancellation, so a message fired right after it can still reach the client.
// The test therefore waits for the server's own "resource unsubscribed" event
// instead of treating Unsubscribe's return as that point. Subscribe does not
// wait for the server either, hence the "resource subscribed" wait.
func TestSubscription_UnsubscribeStopsUpdates(t *testing.T) {
	ctx := context.Background()

	const (
		selfID = int64(777000)
		uri    = "tg://chat/777000/messages"
	)

	authDone := make(chan struct{})
	close(authDone)

	health := middleware.NewSessionHealth()
	health.Arm()

	broker := tgclient.NewSubscriptionBroker()
	client := resolvingClient{peer: tgclient.InputPeer{Type: tgclient.PeerUser, ID: selfID}}

	events := make(chan string, 16)
	logger := slog.New(sdkEventHandler{uri: uri, events: events})

	server := buildServer(client, "/tmp/mcp-tg/downloads", nil, broker, authDone, health, nil, logger)

	ct, st := mcp.NewInMemoryTransports()

	ss, connErr := server.Connect(ctx, st, nil)
	if connErr != nil {
		t.Fatalf("server connect: %v", connErr)
	}

	defer ss.Wait()

	updates := make(chan string, 1)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			select {
			case updates <- req.Params.URI:
			default:
			}
		},
	})

	cs, clientErr := mcpClient.Connect(ctx, ct, nil)
	if clientErr != nil {
		t.Fatalf("client connect: %v", clientErr)
	}

	defer func() { _ = cs.Close() }()

	fire := func() {
		err := broker.HandleNewMessage(
			ctx, tg.Entities{}, &tg.UpdateNewMessage{Message: &tg.Message{PeerID: &tg.PeerUser{UserID: selfID}}},
		)
		if err != nil {
			t.Fatalf("HandleNewMessage: %v", err)
		}
	}

	if subErr := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); subErr != nil {
		t.Fatalf("subscribe: %v", subErr)
	}

	waitForSDKEvent(t, events, "resource subscribed")

	// Without a delivered update first, the silence asserted below would also
	// hold for a subscription that never worked.
	fire()

	select {
	case <-updates:
	case <-time.After(5 * time.Second):
		t.Fatal("no update while subscribed")
	}

	if unsubErr := cs.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri}); unsubErr != nil {
		t.Fatalf("unsubscribe: %v", unsubErr)
	}

	waitForSDKEvent(t, events, "resource unsubscribed")

	fire()

	select {
	case got := <-updates:
		t.Fatalf("received an update after unsubscribe: %q", got)
	case <-time.After(300 * time.Millisecond):
		// No notification — correct.
	}
}

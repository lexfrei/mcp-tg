package main

import (
	"context"
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

	server := buildServer(client, "/tmp/mcp-tg/downloads", broker, authDone, health, nil)

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
	fireErr := broker.HandleNewMessage(
		ctx, tg.Entities{}, &tg.UpdateNewMessage{Message: &tg.Message{PeerID: &tg.PeerUser{UserID: selfID}}},
	)
	if fireErr != nil {
		t.Fatalf("HandleNewMessage: %v", fireErr)
	}

	select {
	case got := <-updates:
		if got != uri {
			t.Fatalf("resources/updated URI = %q, want %q", got, uri)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resources/updated notification")
	}
}

// TestSubscription_UnsubscribeStopsUpdates pins that after an unsubscribe a new
// message in the same chat no longer delivers a notification — the broker drops
// the watch and the SDK drops the session from its registry.
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

	server := buildServer(client, "/tmp/mcp-tg/downloads", broker, authDone, health, nil)

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

	if subErr := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); subErr != nil {
		t.Fatalf("subscribe: %v", subErr)
	}

	if unsubErr := cs.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri}); unsubErr != nil {
		t.Fatalf("unsubscribe: %v", unsubErr)
	}

	fireErr := broker.HandleNewMessage(
		ctx, tg.Entities{}, &tg.UpdateNewMessage{Message: &tg.Message{PeerID: &tg.PeerUser{UserID: selfID}}},
	)
	if fireErr != nil {
		t.Fatalf("HandleNewMessage: %v", fireErr)
	}

	select {
	case got := <-updates:
		t.Fatalf("received an update after unsubscribe: %q", got)
	case <-time.After(300 * time.Millisecond):
		// No notification — correct.
	}
}

package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func signalled(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

// A listener is woken by its own peer's messages only, and needs no resource
// subscription or notifier to be woken.
func TestSubscriptionBroker_ListenWakesOnItsPeerOnly(t *testing.T) {
	broker := NewSubscriptionBroker()

	listener := broker.Listen(InputPeer{Type: PeerChannel, ID: 7})
	defer listener.Stop()

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 7}))

	if signalled(listener.Signal) {
		t.Fatal("a user with the same ID woke a channel listener")
	}

	fireChannel(t, broker, newChannelMessageUpdate(&tg.PeerChannel{ChannelID: 7}))

	if !signalled(listener.Signal) {
		t.Fatal("the channel's own message did not wake its listener")
	}
}

// A burst of messages must not block the update loop on a listener that has
// not drained its signal yet: one pending signal already means "look again".
func TestSubscriptionBroker_ListenNeverBlocksTheUpdateLoop(t *testing.T) {
	broker := NewSubscriptionBroker()

	listener := broker.Listen(InputPeer{Type: PeerUser, ID: 42})
	defer listener.Stop()

	for range 5 {
		fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))
	}

	if !signalled(listener.Signal) {
		t.Fatal("no signal after a burst")
	}

	if signalled(listener.Signal) {
		t.Fatal("more than one pending signal; the buffer should coalesce a burst")
	}
}

func TestSubscriptionBroker_StoppedListenerIsForgotten(t *testing.T) {
	broker := NewSubscriptionBroker()

	listener := broker.Listen(InputPeer{Type: PeerUser, ID: 42})
	listener.Stop()

	fireMessage(t, broker, newMessageUpdate(&tg.PeerUser{UserID: 42}))

	if signalled(listener.Signal) {
		t.Fatal("a stopped listener was woken")
	}

	if len(broker.listeners) != 0 {
		t.Fatalf("listeners = %v, want empty after stop", broker.listeners)
	}
}

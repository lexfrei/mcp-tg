package tools

import (
	"context"
	"sync"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

const (
	waitSelfID   = 1
	waitFriendID = 2
	waitOtherID  = 3
)

// fakeListener hands out one signal channel and lets the test fire it.
type fakeListener struct {
	signal  chan struct{}
	stopped bool
}

func newFakeListener() *fakeListener {
	return &fakeListener{signal: make(chan struct{}, 1)}
}

func (listener *fakeListener) Listen(telegram.InputPeer) telegram.Listener {
	return telegram.Listener{Signal: listener.signal, Stop: func() { listener.stopped = true }}
}

// historyScript serves the chat history newest first, and lets the test
// append messages while a wait is in progress.
type historyScript struct {
	mu    sync.Mutex
	msgs  []telegram.Message
	reads chan struct{} // receives once per read when non-nil
}

func (script *historyScript) add(msgs ...telegram.Message) {
	script.mu.Lock()
	defer script.mu.Unlock()

	script.msgs = append(msgs, script.msgs...)
}

func (script *historyScript) read(_ telegram.InputPeer, opts telegram.HistoryOpts) ([]telegram.Message, int, error) {
	script.mu.Lock()
	defer script.mu.Unlock()

	out := script.msgs
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}

	if script.reads != nil {
		script.reads <- struct{}{}
	}

	return append([]telegram.Message(nil), out...), len(script.msgs), nil
}

func waitMessage(id int, from int64) telegram.Message {
	return telegram.Message{ID: id, FromID: from, FromType: telegram.PeerUser, Text: "msg"}
}

func newWaitMock(script *historyScript) *mockClient {
	return &mockClient{
		peer:         telegram.InputPeer{Type: telegram.PeerUser, ID: waitFriendID, AccessHash: 1},
		user:         &telegram.User{ID: waitSelfID},
		getHistoryFn: script.read,
	}
}

func TestMessagesWait_ReturnsMessagesAlreadyNewerThanAfter(t *testing.T) {
	script := &historyScript{}
	script.add(waitMessage(10, waitFriendID), waitMessage(11, waitFriendID), waitMessage(12, waitFriendID))

	handler := NewMessagesWaitHandler(newWaitMock(script), newFakeListener())

	_, result, err := handler(context.Background(), nil, MessagesWaitParams{Peer: "@friend", AfterMessageID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TimedOut || result.Count != 2 {
		t.Fatalf("result = %+v, want two messages", result)
	}

	if result.Messages[0].ID != 11 || result.Messages[1].ID != 12 || result.LastMessageID != 12 {
		t.Errorf("messages = %d, %d (last %d), want 11, 12 oldest first", result.Messages[0].ID,
			result.Messages[1].ID, result.LastMessageID)
	}
}

// Without afterMessageId only messages arriving during the call count: the
// one already there when it starts is the baseline, not the answer.
func TestMessagesWait_DefaultBaselineIsTheNewestMessage(t *testing.T) {
	script := &historyScript{reads: make(chan struct{}, 4)}
	script.add(waitMessage(10, waitFriendID))

	listener := newFakeListener()
	handler := NewMessagesWaitHandler(newWaitMock(script), listener)

	done := make(chan MessagesWaitResult, 1)

	go func() {
		_, result, err := handler(context.Background(), nil, MessagesWaitParams{Peer: "@friend", TimeoutSeconds: 5})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		done <- result
	}()

	<-script.reads // the baseline
	<-script.reads // the first look, which finds nothing

	script.add(waitMessage(11, waitFriendID))
	listener.signal <- struct{}{}

	result := <-done
	if result.Count != 1 || result.Messages[0].ID != 11 {
		t.Fatalf("result = %+v, want only message 11", result)
	}

	if !listener.stopped {
		t.Error("the listener was not released")
	}
}

func TestMessagesWait_SkipsOwnMessagesUnlessAsked(t *testing.T) {
	script := &historyScript{}
	script.add(waitMessage(11, waitSelfID), waitMessage(12, waitFriendID))

	handler := NewMessagesWaitHandler(newWaitMock(script), newFakeListener())

	_, result, err := handler(context.Background(), nil, MessagesWaitParams{Peer: "@friend", AfterMessageID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || result.Messages[0].ID != 12 {
		t.Errorf("default wait returned %+v, want only the friend's message 12", result.Messages)
	}

	_, result, err = handler(context.Background(), nil,
		MessagesWaitParams{Peer: "@friend", AfterMessageID: 10, IncludeOwn: new(true)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 2 {
		t.Errorf("includeOwn wait returned %d messages, want 2", result.Count)
	}
}

func TestMessagesWait_FromFilter(t *testing.T) {
	script := &historyScript{}
	script.add(waitMessage(11, waitOtherID), waitMessage(12, waitFriendID))

	mock := newWaitMock(script)
	handler := NewMessagesWaitHandler(mock, newFakeListener())

	_, result, err := handler(context.Background(), nil,
		MessagesWaitParams{Peer: "@friend", From: "@friend", AfterMessageID: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Count != 1 || result.Messages[0].ID != 12 {
		t.Errorf("returned %+v, want only message 12 from the sender", result.Messages)
	}
}

func TestMessagesWait_TimesOut(t *testing.T) {
	script := &historyScript{}
	script.add(waitMessage(10, waitFriendID))

	handler := NewMessagesWaitHandler(newWaitMock(script), newFakeListener())

	_, result, err := handler(context.Background(), nil, MessagesWaitParams{Peer: "@friend", TimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.TimedOut || result.Count != 0 || result.LastMessageID != 10 {
		t.Errorf("result = %+v, want a timeout that reports the baseline 10", result)
	}
}

func TestMessagesWait_RejectsTimeoutOutOfRange(t *testing.T) {
	handler := NewMessagesWaitHandler(&mockClient{}, newFakeListener())

	for _, seconds := range []int{-1, maxWaitTimeout + 1} {
		result, _, err := handler(context.Background(), nil, MessagesWaitParams{Peer: "@friend", TimeoutSeconds: seconds})
		if !errors.Is(err, ErrWaitTimeoutRange) || result == nil || !result.IsError {
			t.Errorf("timeoutSeconds=%d: err = %v, want ErrWaitTimeoutRange", seconds, err)
		}
	}
}

func TestMessagesWait_EmptyPeer(t *testing.T) {
	handler := NewMessagesWaitHandler(&mockClient{}, newFakeListener())

	_, _, err := handler(context.Background(), nil, MessagesWaitParams{})
	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}
}

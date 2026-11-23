package tools

import (
	"context"
	"sync"
	"testing"
	"time"

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

// A chat whose newest message is a service one reads as empty at Limit 1,
// since the history drops service messages. The baseline must come from the
// same 100-message read the wait makes, or that read hands back old messages
// as new.
func TestMessagesWait_BaselineSurvivesAServiceNewestMessage(t *testing.T) {
	var old []telegram.Message
	for id := 10; id >= 5; id-- {
		old = append(old, waitMessage(id, waitFriendID))
	}

	mock := newWaitMock(&historyScript{})
	mock.getHistoryFn = func(_ telegram.InputPeer, opts telegram.HistoryOpts) ([]telegram.Message, int, error) {
		if opts.Limit == 1 {
			return nil, 0, nil
		}

		return old, len(old), nil
	}

	_, result, err := NewMessagesWaitHandler(mock, newFakeListener())(
		context.Background(), nil, MessagesWaitParams{Peer: "@friend", TimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.TimedOut || result.Count != 0 || result.LastMessageID != 10 {
		t.Errorf("result = %+v, want a timeout at baseline 10, not old messages returned as new", result)
	}
}

// When more than a page arrived since afterMessageId, the result says so
// instead of silently skipping the older ones.
func TestMessagesWait_ReportsTruncationPastOnePage(t *testing.T) {
	script := &historyScript{}
	for id := 2; id <= 151; id++ {
		script.add(waitMessage(id, waitFriendID))
	}

	mock := newWaitMock(script)
	mock.historyHasMore = true

	_, result, err := NewMessagesWaitHandler(mock, newFakeListener())(
		context.Background(), nil, MessagesWaitParams{Peer: "@friend", AfterMessageID: 1, TimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Truncated || result.Count != 100 || result.Messages[0].ID != 52 || result.LastMessageID != 151 {
		t.Errorf("truncated = %v, count = %d, lastMessageId = %d; want truncated, 100 messages 52..151",
			result.Truncated, result.Count, result.LastMessageID)
	}
}

func TestMessagesWait_NoTruncationWhenThePageCoversTheGap(t *testing.T) {
	script := &historyScript{}
	script.add(waitMessage(11, waitFriendID), waitMessage(10, waitFriendID))

	mock := newWaitMock(script)
	mock.historyHasMore = true

	_, result, err := NewMessagesWaitHandler(mock, newFakeListener())(
		context.Background(), nil, MessagesWaitParams{Peer: "@friend", AfterMessageID: 10, TimeoutSeconds: 1})
	if err != nil || result.Truncated || result.Count != 1 {
		t.Errorf("result = %+v, err = %v; want message 11, not truncated", result, err)
	}
}

// A stream of wake-ups must not turn into a read per message.
func TestWaiter_SignalsInARowCostBoundedReads(t *testing.T) {
	var (
		mu    sync.Mutex
		reads int
	)

	mock := newWaitMock(&historyScript{})
	mock.getHistoryFn = func(telegram.InputPeer, telegram.HistoryOpts) ([]telegram.Message, int, error) {
		mu.Lock()
		reads++
		mu.Unlock()

		return nil, 0, nil
	}

	signal := make(chan struct{}, 1)
	stop := make(chan struct{})

	go func() {
		for {
			select {
			case <-stop:
				return
			case signal <- struct{}{}:
			default:
			}

			time.Sleep(time.Millisecond)
		}
	}()

	wait := waiter{
		client: mock, peer: mock.peer, filter: &waitFilter{}, signal: signal,
		timeout: 500 * time.Millisecond, poll: time.Hour, minGap: 100 * time.Millisecond,
	}

	result, err := wait.run(context.Background(), 0)

	close(stop)

	if err != nil || !result.TimedOut {
		t.Fatalf("result = %+v, err = %v; want a timeout", result, err)
	}

	mu.Lock()
	defer mu.Unlock()

	// 500ms at one read per 100ms, plus the first read and the one the
	// deadline cuts short.
	if reads > 7 {
		t.Errorf("reads = %d over 500ms of constant signals, want at most 7", reads)
	}
}

func TestMessagesWait_AcceptsTheTimeoutBounds(t *testing.T) {
	for _, seconds := range []int{1, maxWaitTimeout} {
		got, err := waitTimeout(seconds)
		if err != nil || got != time.Duration(seconds)*time.Second {
			t.Errorf("timeoutSeconds=%d: got %v, %v", seconds, got, err)
		}
	}
}

func TestMessagesWait_PropagatesClientErrors(t *testing.T) {
	errBoom := errors.New("boom")

	for name, mock := range map[string]*mockClient{
		"ResolvePeer": func() *mockClient {
			mock := newWaitMock(&historyScript{})
			mock.resolvePeerFn = func(string) (telegram.InputPeer, error) { return telegram.InputPeer{}, errBoom }

			return mock
		}(),
		"GetSelf": func() *mockClient {
			mock := newWaitMock(&historyScript{})
			mock.err = errBoom

			return mock
		}(),
		"GetHistory": func() *mockClient {
			mock := newWaitMock(&historyScript{})
			mock.getHistoryFn = func(telegram.InputPeer, telegram.HistoryOpts) ([]telegram.Message, int, error) {
				return nil, 0, errBoom
			}

			return mock
		}(),
	} {
		result, _, err := NewMessagesWaitHandler(mock, newFakeListener())(
			context.Background(), nil, MessagesWaitParams{Peer: "@friend", TimeoutSeconds: 1})
		if !errors.Is(err, errBoom) || result == nil || !result.IsError {
			t.Errorf("%s failing: err = %v, want it passed through as a tool error", name, err)
		}
	}
}

func TestMessagesWait_StopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	started := time.Now()

	result, _, err := NewMessagesWaitHandler(newWaitMock(&historyScript{}), newFakeListener())(
		ctx, nil, MessagesWaitParams{Peer: "@friend", TimeoutSeconds: 60})
	if !errors.Is(err, context.Canceled) || result == nil || !result.IsError {
		t.Errorf("err = %v, want context.Canceled as a tool error", err)
	}

	if time.Since(started) > 5*time.Second {
		t.Error("the wait did not stop on cancellation")
	}
}

package telegram

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

var (
	errUnexpectedRequest = errors.New("unexpected request")
	errTestBoom          = errors.New("boom")
	errNoScriptedPages   = errors.New("no more scripted pages")
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

func encodeAndDecode(resp tg.MessagesDialogsClass, output bin.Decoder) error {
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

	wrap.warmDialogsCache(t.Context())

	cached, hit := wrap.cache.Lookup(PeerChannel, channelID)
	if !hit {
		t.Fatalf("expected cache hit for channel %d after warm", channelID)
	}

	if cached.AccessHash != channelHash {
		t.Errorf("cached AccessHash = %x, want %x", cached.AccessHash, channelHash)
	}

	if invoker.calls.Load() != 1 {
		t.Errorf("expected 1 API call, got %d", invoker.calls.Load())
	}
}

func TestWarmDialogsCache_ThrottledOnSecondCall(t *testing.T) {
	invoker := &fakeInvoker{response: &tg.MessagesDialogs{}}
	wrap := newWrapperWithInvoker(invoker)

	wrap.warmDialogsCache(t.Context())
	wrap.warmDialogsCache(t.Context())

	if got := invoker.calls.Load(); got != 1 {
		t.Errorf("expected second call throttled (1 total), got %d", got)
	}
}

func TestWarmDialogsCache_AllowsRetryAfterFirstPageError(t *testing.T) {
	invoker := &fakeInvoker{err: errTestBoom}
	wrap := newWrapperWithInvoker(invoker)

	wrap.warmDialogsCache(t.Context())

	if wrap.warmedAt.Load() != 0 {
		t.Errorf("expected warmedAt reset after first-page error, got %d", wrap.warmedAt.Load())
	}

	wrap.warmDialogsCache(t.Context())

	if got := invoker.calls.Load(); got != 2 {
		t.Errorf("expected retry after error, got %d calls", got)
	}
}

func TestWarmDialogsCache_PaginatesUntilSliceExhausted(t *testing.T) {
	pages := buildWarmPages()
	invoker := &scriptedInvoker{pages: pages}
	wrap := newWrapperWithInvoker(invoker)

	wrap.warmDialogsCache(t.Context())

	if got := invoker.calls.Load(); got != 3 {
		t.Errorf("expected 3 paginated calls, got %d", got)
	}

	if _, hit := wrap.cache.Lookup(PeerChannel, 100); !hit {
		t.Errorf("expected page 1 channel cached")
	}

	if _, hit := wrap.cache.Lookup(PeerChannel, 200); !hit {
		t.Errorf("expected page 2 channel cached")
	}
}

func buildWarmPages() []tg.MessagesDialogsClass {
	return []tg.MessagesDialogsClass{
		&tg.MessagesDialogsSlice{
			Count: 2,
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 100}, TopMessage: 10},
			},
			Messages: []tg.MessageClass{
				&tg.Message{ID: 10, Date: 1700000000, PeerID: &tg.PeerChannel{ChannelID: 100}},
			},
			Chats: []tg.ChatClass{channelWithHash(100, 0xAAAA, "One")},
		},
		&tg.MessagesDialogsSlice{
			Count: 2,
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: 200}, TopMessage: 20},
			},
			Messages: []tg.MessageClass{
				&tg.Message{ID: 20, Date: 1700000100, PeerID: &tg.PeerChannel{ChannelID: 200}},
			},
			Chats: []tg.ChatClass{channelWithHash(200, 0xBBBB, "Two")},
		},
		&tg.MessagesDialogs{},
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

type scriptedInvoker struct {
	calls atomic.Int32
	pages []tg.MessagesDialogsClass
}

func (s *scriptedInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.MessagesGetDialogsRequest); !ok {
		return errUnexpectedRequest
	}

	idx := int(s.calls.Add(1)) - 1
	if idx >= len(s.pages) {
		return errNoScriptedPages
	}

	return encodeAndDecode(s.pages[idx], output)
}

func TestResolvePeer_ColdMissWarmsCacheAndReturnsHash(t *testing.T) {
	channelID := int64(3282239618)
	channelHash := int64(0xCAFEBABE)

	invoker := &warmingInvoker{channelID: channelID, channelHash: channelHash}
	wrap := newWrapperWithInvoker(invoker)

	identifier := "-100" + int64ToString(channelID)

	peer, err := wrap.ResolvePeer(t.Context(), identifier)
	if err != nil {
		t.Fatalf("ResolvePeer: %v", err)
	}

	if peer.AccessHash != channelHash {
		t.Errorf("resolved AccessHash = %x, want %x", peer.AccessHash, channelHash)
	}

	if peer.Type != PeerChannel || peer.ID != channelID {
		t.Errorf("resolved peer = %+v, want channel %d", peer, channelID)
	}
}

type warmingInvoker struct {
	channelID   int64
	channelHash int64
}

func (w *warmingInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	switch input.(type) {
	case *tg.MessagesGetPeerDialogsRequest:
		return errChannelInvalidFix
	case *tg.MessagesGetDialogsRequest:
		resp := &tg.MessagesDialogs{
			Dialogs: []tg.DialogClass{
				&tg.Dialog{Peer: &tg.PeerChannel{ChannelID: w.channelID}, TopMessage: 1},
			},
			Chats: []tg.ChatClass{channelWithHash(w.channelID, w.channelHash, "Test")},
		}

		return encodeAndDecode(resp, output)
	default:
		return errUnexpectedRequest
	}
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

func TestWarmDialogsThrottleIsPositive(t *testing.T) {
	if warmDialogsThrottle <= 0 {
		t.Fatal("warmDialogsThrottle must be positive")
	}

	if warmDialogsThrottle < time.Second {
		t.Errorf("throttle %v is suspiciously short", warmDialogsThrottle)
	}
}

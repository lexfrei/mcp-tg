package telegram

import (
	"context"
	"sync"
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// The identity every send-as test posts under. Access hash is non-zero
// because a channel InputPeer without one is rejected before it reaches
// the wrapper.
const (
	sendAsChannelID   = 777
	sendAsChannelHash = 888
)

func sendAsIdentity() InputPeer {
	return InputPeer{Type: PeerChannel, ID: sendAsChannelID, AccessHash: sendAsChannelHash}
}

// sendAsInvoker captures the outgoing request of every send RPC that
// carries the conditional send_as field, so a test can assert whether
// the flag bit was set and what identity it names.
type sendAsInvoker struct {
	mu          sync.Mutex
	sendMsg     *tg.MessagesSendMessageRequest
	sendMedia   *tg.MessagesSendMediaRequest
	multiMedia  *tg.MessagesSendMultiMediaRequest
	forward     *tg.MessagesForwardMessagesRequest
	createTopic *tg.MessagesCreateForumTopicRequest
}

func (s *sendAsInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch req := input.(type) {
	case *tg.UploadSaveFilePartRequest:
		return encodeResp(&tg.BoolTrue{}, output)
	case *tg.MessagesUploadMediaRequest:
		return encodeResp(uploadMediaResponse(req.Media), output)
	case *tg.MessagesSendMessageRequest:
		s.sendMsg = req

		return encodeResp(&tg.Updates{}, output)
	case *tg.MessagesSendMediaRequest:
		s.sendMedia = req

		return encodeResp(&tg.Updates{}, output)
	case *tg.MessagesSendMultiMediaRequest:
		s.multiMedia = req

		return encodeResp(&tg.Updates{}, output)
	case *tg.MessagesForwardMessagesRequest:
		s.forward = req

		return encodeResp(&tg.Updates{}, output)
	case *tg.MessagesCreateForumTopicRequest:
		s.createTopic = req

		return encodeResp(&tg.Updates{}, output)
	default:
		return errUnexpectedRequest
	}
}

func newSendAsWrapper(inv *sendAsInvoker) *Wrapper {
	api := tg.NewClient(inv)

	return &Wrapper{api: api, up: uploader.NewUploader(api), cache: NewPeerCache()}
}

// assertSendAsIdentity fails unless got is the channel InputPeer that
// sendAsIdentity names.
func assertSendAsIdentity(t *testing.T, got tg.InputPeerClass) {
	t.Helper()

	channel, ok := got.(*tg.InputPeerChannel)
	if !ok {
		t.Fatalf("send_as is %T, want *tg.InputPeerChannel", got)
	}

	if channel.ChannelID != sendAsChannelID || channel.AccessHash != sendAsChannelHash {
		t.Errorf("send_as = {id:%d hash:%d}, want {id:%d hash:%d}",
			channel.ChannelID, channel.AccessHash, sendAsChannelID, sendAsChannelHash)
	}
}

func targetPeer() InputPeer {
	return InputPeer{Type: PeerChannel, ID: 1, AccessHash: 2}
}

func TestSendMessage_SetsSendAs(t *testing.T) {
	inv := &sendAsInvoker{}
	identity := sendAsIdentity()

	_, err := newSendAsWrapper(inv).SendMessage(
		t.Context(), targetPeer(), "hi", SendOpts{SendAs: &identity},
	)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if inv.sendMsg == nil {
		t.Fatal("messages.sendMessage was never invoked")
	}

	got, ok := inv.sendMsg.GetSendAs()
	if !ok {
		t.Fatal("send_as flag is not set on the request")
	}

	assertSendAsIdentity(t, got)
}

// Leaving the flag clear is deliberate, not an omission: the server then
// applies the chat's saved default, the way the official clients do.
// Setting inputPeerSelf here would make SetDefaultSendAs useless for
// messages while it still governed reactions.
//
// Verified against a live account: with a channel saved as the chat
// default, a send with no send_as posts as that channel.
func TestSendMessage_OmitsSendAsWhenNil(t *testing.T) {
	inv := &sendAsInvoker{}

	_, err := newSendAsWrapper(inv).SendMessage(t.Context(), targetPeer(), "hi", SendOpts{})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	if _, ok := inv.sendMsg.GetSendAs(); ok {
		t.Error("send_as flag is set even though SendOpts.SendAs is nil")
	}
}

func TestSendFile_SetsSendAs(t *testing.T) {
	inv := &sendAsInvoker{}
	identity := sendAsIdentity()
	path := writeTempFile(t, "doc.bin", 16)

	_, err := newSendAsWrapper(inv).SendFile(
		t.Context(), targetPeer(), path, "caption", SendOpts{SendAs: &identity},
	)
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	if inv.sendMedia == nil {
		t.Fatal("messages.sendMedia was never invoked")
	}

	got, ok := inv.sendMedia.GetSendAs()
	if !ok {
		t.Fatal("send_as flag is not set on the request")
	}

	assertSendAsIdentity(t, got)
}

func TestSendFile_OmitsSendAsWhenNil(t *testing.T) {
	inv := &sendAsInvoker{}
	path := writeTempFile(t, "doc.bin", 16)

	_, err := newSendAsWrapper(inv).SendFile(t.Context(), targetPeer(), path, "caption", SendOpts{})
	if err != nil {
		t.Fatalf("SendFile: %v", err)
	}

	if _, ok := inv.sendMedia.GetSendAs(); ok {
		t.Error("send_as flag is set even though SendOpts.SendAs is nil")
	}
}

func TestSendAlbum_SetsSendAs(t *testing.T) {
	inv := &sendAsInvoker{}
	identity := sendAsIdentity()
	paths := []string{writeTempFile(t, "a.png", 8), writeTempFile(t, "b.png", 8)}

	_, err := newSendAsWrapper(inv).SendAlbum(
		t.Context(), targetPeer(), paths, "caption", SendOpts{SendAs: &identity},
	)
	if err != nil {
		t.Fatalf("SendAlbum: %v", err)
	}

	if inv.multiMedia == nil {
		t.Fatal("messages.sendMultiMedia was never invoked")
	}

	got, ok := inv.multiMedia.GetSendAs()
	if !ok {
		t.Fatal("send_as flag is not set on the request")
	}

	assertSendAsIdentity(t, got)
}

func TestSendAlbum_OmitsSendAsWhenNil(t *testing.T) {
	inv := &sendAsInvoker{}
	paths := []string{writeTempFile(t, "a.png", 8)}

	_, err := newSendAsWrapper(inv).SendAlbum(t.Context(), targetPeer(), paths, "", SendOpts{})
	if err != nil {
		t.Fatalf("SendAlbum: %v", err)
	}

	if _, ok := inv.multiMedia.GetSendAs(); ok {
		t.Error("send_as flag is set even though SendOpts.SendAs is nil")
	}
}

func TestForwardMessages_SetsSendAs(t *testing.T) {
	inv := &sendAsInvoker{}
	identity := sendAsIdentity()

	_, err := newSendAsWrapper(inv).ForwardMessages(
		t.Context(), targetPeer(), targetPeer(), []int{5}, &identity,
	)
	if err != nil {
		t.Fatalf("ForwardMessages: %v", err)
	}

	if inv.forward == nil {
		t.Fatal("messages.forwardMessages was never invoked")
	}

	got, ok := inv.forward.GetSendAs()
	if !ok {
		t.Fatal("send_as flag is not set on the request")
	}

	assertSendAsIdentity(t, got)
}

func TestForwardMessages_OmitsSendAsWhenNil(t *testing.T) {
	inv := &sendAsInvoker{}

	_, err := newSendAsWrapper(inv).ForwardMessages(t.Context(), targetPeer(), targetPeer(), []int{5}, nil)
	if err != nil {
		t.Fatalf("ForwardMessages: %v", err)
	}

	if _, ok := inv.forward.GetSendAs(); ok {
		t.Error("send_as flag is set even though sendAs is nil")
	}
}

// Sticker sends live in sticker_cache_test.go: they need a cached
// document before the request can be built at all.

func TestCreateForumTopic_SetsSendAs(t *testing.T) {
	inv := &sendAsInvoker{}
	identity := sendAsIdentity()

	_, err := newSendAsWrapper(inv).CreateForumTopic(t.Context(), targetPeer(), "topic", &identity)
	if err != nil {
		t.Fatalf("CreateForumTopic: %v", err)
	}

	if inv.createTopic == nil {
		t.Fatal("messages.createForumTopic was never invoked")
	}

	got, ok := inv.createTopic.GetSendAs()
	if !ok {
		t.Fatal("send_as flag is not set on the request")
	}

	assertSendAsIdentity(t, got)
}

func TestCreateForumTopic_OmitsSendAsWhenNil(t *testing.T) {
	inv := &sendAsInvoker{}

	_, err := newSendAsWrapper(inv).CreateForumTopic(t.Context(), targetPeer(), "topic", nil)
	if err != nil {
		t.Fatalf("CreateForumTopic: %v", err)
	}

	if _, ok := inv.createTopic.GetSendAs(); ok {
		t.Error("send_as flag is set even though sendAs is nil")
	}
}

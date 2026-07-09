package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

const sendAsRef = "@mychan"

func sendAsChannel() telegram.InputPeer {
	return telegram.InputPeer{Type: telegram.PeerChannel, ID: 20, AccessHash: 21}
}

func destPeer() telegram.InputPeer {
	return telegram.InputPeer{Type: telegram.PeerChannel, ID: 1, AccessHash: 2}
}

// sendAsMock resolves the send-as reference to a different peer than the
// destination, so a test can prove the two never get crossed.
func sendAsMock() *mockClient {
	return &mockClient{
		peer:     destPeer(),
		message:  &telegram.Message{ID: 1},
		messages: []telegram.Message{{ID: 1}},
		topic:    &telegram.ForumTopic{ID: 1, Title: "t"},
		resolvePeerFn: func(identifier string) (telegram.InputPeer, error) {
			if identifier == sendAsRef {
				return sendAsChannel(), nil
			}

			return destPeer(), nil
		},
	}
}

func assertSendAsChannel(t *testing.T, got *telegram.InputPeer) {
	t.Helper()

	if got == nil {
		t.Fatal("send-as identity was not passed to the client")
	}

	if *got != sendAsChannel() {
		t.Errorf("send-as identity = %+v, want %+v", *got, sendAsChannel())
	}
}

func TestMessagesSendHandler_PassesSendAs(t *testing.T) {
	mock := sendAsMock()
	handler := NewMessagesSendHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer: "@group", Text: "hi", SendAs: new(sendAsRef),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSendOpts.SendAs)

	if mock.lastPeer != destPeer() {
		t.Errorf("destination peer = %+v, want %+v", mock.lastPeer, destPeer())
	}
}

func TestMessagesSendHandler_OmitsSendAsWhenAbsent(t *testing.T) {
	mock := sendAsMock()
	handler := NewMessagesSendHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesSendParams{Peer: "@group", Text: "hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastSendOpts.SendAs != nil {
		t.Errorf("send-as identity = %+v, want nil", *mock.lastSendOpts.SendAs)
	}
}

func TestMessagesSendFileHandler_PassesSendAs(t *testing.T) {
	mock := sendAsMock()
	handler := NewMessagesSendFileHandler(mock)

	_, _, err := handler(context.Background(), emptyToolRequest(), MessagesSendFileParams{
		Peer: "@group", Path: "/tmp/x", SendAs: new(sendAsRef),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSendOpts.SendAs)
}

func TestMediaSendAlbumHandler_PassesSendAs(t *testing.T) {
	mock := sendAsMock()
	handler := NewMediaSendAlbumHandler(mock)

	_, _, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer: "@group", Paths: []string{"/tmp/a"}, SendAs: new(sendAsRef),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSendOpts.SendAs)
}

func TestMessagesForwardHandler_PassesSendAs(t *testing.T) {
	mock := sendAsMock()
	handler := NewMessagesForwardHandler(mock)

	_, _, err := handler(context.Background(), nil, MessagesForwardParams{
		FromPeer: "@src", ToPeer: "@group", IDs: []int{7}, SendAs: new(sendAsRef),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSendAs)
}

func TestStickersSendHandler_PassesSendAs(t *testing.T) {
	mock := sendAsMock()
	handler := NewStickersSendHandler(mock)

	_, _, err := handler(context.Background(), nil, StickersSendParams{
		Peer: "@group", StickerFileID: 42, SendAs: new(sendAsRef),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSendAs)
}

func TestTopicsCreateHandler_PassesSendAs(t *testing.T) {
	mock := sendAsMock()
	handler := NewTopicsCreateHandler(mock)

	_, _, err := handler(context.Background(), nil, TopicsCreateParams{
		Peer: "@group", Title: "topic", SendAs: new(sendAsRef),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSendAsChannel(t, mock.lastSendAs)
}

func TestResolveSendAs_EmptyMeansSelf(t *testing.T) {
	got, err := resolveSendAs(context.Background(), sendAsMock(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != nil {
		t.Errorf("identity = %+v, want nil for an empty reference", *got)
	}
}

// A numeric reference to a private channel resolves without error but
// without an access hash. Sending that on would produce a server-side
// PEER_ID_INVALID that reads as a problem with the destination.
func TestResolveSendAs_RejectsChannelWithoutAccessHash(t *testing.T) {
	mock := &mockClient{
		resolvePeerFn: func(string) (telegram.InputPeer, error) {
			return telegram.InputPeer{Type: telegram.PeerChannel, ID: 99}, nil
		},
	}

	_, err := resolveSendAs(context.Background(), mock, "-10099")
	if !errors.Is(err, ErrSendAsUnresolved) {
		t.Fatalf("error = %v, want ErrSendAsUnresolved", err)
	}
}

func TestMessagesSendHandler_RejectsUnresolvableSendAs(t *testing.T) {
	mock := sendAsMock()
	mock.resolvePeerFn = func(identifier string) (telegram.InputPeer, error) {
		if identifier == sendAsRef {
			return telegram.InputPeer{Type: telegram.PeerChannel, ID: 99}, nil
		}

		return destPeer(), nil
	}

	result, _, err := NewMessagesSendHandler(mock)(context.Background(), nil, MessagesSendParams{
		Peer: "@group", Text: "hi", SendAs: new(sendAsRef),
	})
	if !errors.Is(err, ErrSendAsUnresolved) {
		t.Fatalf("error = %v, want ErrSendAsUnresolved", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}

	if mock.message != nil && mock.lastSendOpts.SendAs != nil {
		t.Error("the message was sent despite an unresolvable identity")
	}
}

func TestExplainMTProtoCode_SendAsPeerInvalid(t *testing.T) {
	got := explainMTProtoCode("rpc error code 400: SEND_AS_PEER_INVALID")
	if !strings.Contains(got, "tg_chats_get_send_as") {
		t.Errorf("explanation %q must point at tg_chats_get_send_as", got)
	}
}

// A channel reacts whenever it is the chat's default send-as identity.
// Rendering it as [user:N] points the reader at the wrong id space.
func TestMessagesGetReactionsHandler_ChannelReactor(t *testing.T) {
	mock := &mockClient{
		peer: destPeer(),
		reactions: []telegram.ReactionUser{
			{UserID: 20, PeerType: telegram.PeerChannel, Name: "My Channel", Username: "mychan", Emoji: "👍"},
			{UserID: 10, PeerType: telegram.PeerUser, Name: "Alice", Emoji: "🔥"},
		},
	}

	_, structured, err := NewMessagesGetReactionsHandler(mock)(
		context.Background(), nil, MessagesGetReactionsParams{Peer: "@group", MessageID: 1},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Reactions[0].Type != peerChannel {
		t.Errorf("channel reactor type = %q, want %q", structured.Reactions[0].Type, peerChannel)
	}

	if structured.Reactions[1].Type != peerUser {
		t.Errorf("user reactor type = %q, want %q", structured.Reactions[1].Type, peerUser)
	}

	if !strings.Contains(structured.Output, "My Channel [@mychan]") {
		t.Errorf("output %q must render the channel reactor by name", structured.Output)
	}

	if !strings.Contains(structured.Output, "Alice [user:10]") {
		t.Errorf("output %q must label the user reactor as a user", structured.Output)
	}
}

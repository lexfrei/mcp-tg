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
		Peer: "@group", StickerFileID: "42", SendAs: new(sendAsRef),
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

// Telegram rejects a disallowed send-as identity with the same codes it
// uses for chat-level permission failures, so the raw message blames the
// destination. Verified against a live account: a channel the account
// does not administrate yields CHAT_ADMIN_REQUIRED, a foreign user
// yields CHAT_WRITE_FORBIDDEN. Neither mentions send_as.
func TestSendErr_BlamesTheIdentityWhenOneWasRequested(t *testing.T) {
	identity := sendAsChannel()

	for _, code := range []string{"CHAT_ADMIN_REQUIRED", "CHAT_WRITE_FORBIDDEN", "SEND_AS_PEER_INVALID"} {
		err := sendErr("failed to send message", errors.New("rpc error code 400: "+code), &identity)
		if !strings.Contains(err.Error(), "sendAs") {
			t.Errorf("%s: error %q must name sendAs as a suspect", code, err)
		}

		if !strings.Contains(err.Error(), toolGetSendAs) {
			t.Errorf("%s: error %q must point at %s", code, err, toolGetSendAs)
		}
	}
}

func TestSendErr_StaysQuietWithoutAnIdentity(t *testing.T) {
	err := sendErr("failed to send message", errors.New("rpc error code 400: CHAT_ADMIN_REQUIRED"), nil)
	if strings.Contains(err.Error(), "sendAs") {
		t.Errorf("error %q must not mention sendAs when none was requested", err)
	}
}

// An unrelated failure keeps its own explanation even with an identity set.
func TestSendErr_LeavesUnrelatedFailuresAlone(t *testing.T) {
	identity := sendAsChannel()

	err := sendErr("failed to send message", errors.New("rpc error code 420: SLOWMODE_WAIT_30"), &identity)
	if strings.Contains(err.Error(), "sendAs") {
		t.Errorf("error %q must not blame sendAs for a slow-mode wait", err)
	}
}

func TestExplainMTProtoCode_ChatAdminRequired(t *testing.T) {
	got := explainMTProtoCode("rpc error code 400: CHAT_ADMIN_REQUIRED")
	if got == "" {
		t.Error("CHAT_ADMIN_REQUIRED must have a human-readable explanation")
	}
}

// The MCP SDK unmarshals tool arguments into map[string]any, applies
// schema defaults and re-marshals (go-sdk v1.6.1, mcp/tool.go). Any JSON
// number therefore round-trips through float64, whose mantissa holds 53
// bits. A sticker document id needs 63, so passing it as a JSON number
// silently corrupts it: 5181593617004757506 arrives as 5181593617004758000.
// The id is a decimal string for that reason and must stay one.
func TestStickersSendHandler_ParsesFileIDBeyondFloat64Precision(t *testing.T) {
	const exact int64 = 5181593617004757506

	mock := sendAsMock()

	_, _, err := NewStickersSendHandler(mock)(context.Background(), nil, StickersSendParams{
		Peer: "@group", StickerFileID: "5181593617004757506",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastStickerID != exact {
		t.Errorf("sticker id = %d, want %d (lost %d)", mock.lastStickerID, exact, exact-mock.lastStickerID)
	}
}

func TestStickersSendHandler_RejectsNonNumericFileID(t *testing.T) {
	result, _, err := NewStickersSendHandler(sendAsMock())(context.Background(), nil, StickersSendParams{
		Peer: "@group", StickerFileID: "not-a-number",
	})
	if !errors.Is(err, ErrInvalidStickerFileID) {
		t.Fatalf("error = %v, want ErrInvalidStickerFileID", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestStickersSendHandler_RejectsEmptyFileID(t *testing.T) {
	_, _, err := NewStickersSendHandler(sendAsMock())(context.Background(), nil, StickersSendParams{
		Peer: "@group",
	})
	if !errors.Is(err, ErrStickerFileIDRequired) {
		t.Fatalf("error = %v, want ErrStickerFileIDRequired", err)
	}
}

// The blame hint is only useful if it survives the handler. Drive the
// tool end to end with an RPC error Telegram actually answers when it
// refuses an identity, and check the caller is pointed at the parameter
// rather than at the destination chat.
func TestMessagesSendHandler_SurfacesTheIdentityHintOnRejection(t *testing.T) {
	mock := sendAsMock()
	mock.err = errors.New("rpc error code 400: CHAT_ADMIN_REQUIRED")

	result, _, err := NewMessagesSendHandler(mock)(context.Background(), nil, MessagesSendParams{
		Peer: "@group", Text: "hi", SendAs: new(sendAsRef),
	})
	if err == nil {
		t.Fatal("expected the send to fail")
	}

	if !strings.Contains(err.Error(), "sendAs") {
		t.Errorf("error %q must name sendAs as a suspect", err)
	}

	if !strings.Contains(err.Error(), toolGetSendAs) {
		t.Errorf("error %q must point at %s", err, toolGetSendAs)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

// The same failure without an identity must not invent one.
func TestMessagesSendHandler_NoIdentityHintWhenNoneRequested(t *testing.T) {
	mock := sendAsMock()
	mock.err = errors.New("rpc error code 400: CHAT_ADMIN_REQUIRED")

	_, _, err := NewMessagesSendHandler(mock)(context.Background(), nil, MessagesSendParams{
		Peer: "@group", Text: "hi",
	})
	if err == nil {
		t.Fatal("expected the send to fail")
	}

	if strings.Contains(err.Error(), "sendAs") {
		t.Errorf("error %q must not mention sendAs when none was requested", err)
	}
}

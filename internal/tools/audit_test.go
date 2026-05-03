package tools

// Tests pinning the v0.11.0 audit findings on the tools layer. Each
// TestAudit_* corresponds to one finding from the UX audit shipped in
// v0.12.0. See README.md "Known Limitations" for the deliberately-deferred
// cases (none on the tools layer in this PR — all tools-layer findings are
// fixed).

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// When peer resolution fails because the user passed a numeric ID for a
// peer the account doesn't have an access hash for, the failure surfaces
// as PEER_ID_INVALID from MTProto. The wrapped error must hint at
// @username so the caller has a concrete next step. (ErrPeerNotFound
// itself stays neutral — it's reached on different code paths where the
// @username hint would be wrong.)
func TestAudit_PeerNotFoundHintsUsername(t *testing.T) {
	got := explainMTProtoCode("rpc error code 400: PEER_ID_INVALID")
	if got == "" {
		t.Fatal("PEER_ID_INVALID has no human-readable explanation")
	}

	if !strings.Contains(got, "@username") {
		t.Errorf("PEER_ID_INVALID explanation = %q, want hint about @username", got)
	}
}

// Sending to a topicId on a non-forum chat must fail fast with a clear
// message, not a cryptic MTProto error after the round-trip.
func TestAudit_TopicIDOnNonForumChat(t *testing.T) {
	mock := &mockClient{
		peer:  telegram.InputPeer{Type: telegram.PeerChannel, ID: 1, AccessHash: 1},
		group: &telegram.GroupInfo{IsForum: false, Title: "Plain channel"},
	}
	handler := NewMessagesSendHandler(mock)

	topicID := 7

	result, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer:    "@plain",
		Text:    "hello",
		TopicID: &topicID,
	})
	if err == nil {
		t.Fatal("expected error for topicId on non-forum chat")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError must be true")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "forum") {
		t.Errorf("error %q must mention 'forum' so the user understands the constraint", err)
	}
}

// topicId on a DM (PeerUser) must short-circuit without calling
// GetGroupInfo at all — that wrapper falls through to MessagesGetFullChat
// with a user ID, which produces a nonsense MTProto error that buries the
// actual constraint. mockClient.groupInfoCalls makes the no-call check
// observable.
func TestAudit_TopicIDOnUserPeerSkipsGroupInfo(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewMessagesSendHandler(mock)

	topicID := 7

	_, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer:    "@user",
		Text:    "hello",
		TopicID: &topicID,
	})
	if err == nil {
		t.Fatal("expected error for topicId on a user peer")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "forum") {
		t.Errorf("error %q must mention 'forum'", err)
	}

	if mock.groupInfoCalls != 0 {
		t.Errorf("GetGroupInfo called %d times for a PeerUser; expected 0", mock.groupInfoCalls)
	}
}

// errReplyMessageIDInvalid is a fixture mimicking gotd/td's tgerr.Error
// surface for REPLY_MESSAGE_ID_INVALID. Static so the err113 linter
// doesn't object to dynamic test errors.
var errReplyMessageIDInvalid = errors.New("rpc error code 400: REPLY_MESSAGE_ID_INVALID")

// MTProto reply errors (REPLY_MESSAGE_ID_INVALID etc.) must be rewrapped in
// a human-readable form rather than passed through verbatim. The wrapper
// is exposed as wrapTelegramError; verify it produces useful text.
func TestAudit_ReplyToInvalidWrapped(t *testing.T) {
	wrapped := wrapTelegramError(errReplyMessageIDInvalid)

	if wrapped == nil {
		t.Fatal("wrapTelegramError returned nil for known MTProto error")
	}

	got := wrapped.Error()
	if !strings.Contains(strings.ToLower(got), "reply") {
		t.Errorf("wrapped %q must mention 'reply' (the user's input field)", got)
	}

	if strings.Contains(got, "REPLY_MESSAGE_ID_INVALID") && !strings.Contains(got, "does not exist") {
		t.Errorf("wrapped %q should explain the failure, not just echo the raw code", got)
	}
}

// Paginated read tools must expose a hasMore signal so callers don't have
// to compare returned count with the limit they requested. dialogs_list is
// the simplest of the three to verify.
func TestAudit_PaginationHasMore(t *testing.T) {
	const limit = 2

	full := make([]telegram.Dialog, limit)
	for i := range full {
		full[i] = telegram.Dialog{Title: "d"}
	}

	mock := &mockClient{dialogs: full}
	handler := NewDialogsListHandler(mock)

	limitVal := limit

	_, structured, err := handler(context.Background(), nil, DialogsListParams{Limit: &limitVal})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !structured.HasMore {
		t.Errorf("HasMore = false, want true (returned %d items at limit %d)", structured.Count, limit)
	}

	mockShort := &mockClient{dialogs: []telegram.Dialog{{Title: "only"}}}
	handler2 := NewDialogsListHandler(mockShort)

	_, structured2, err := handler2(context.Background(), nil, DialogsListParams{Limit: &limitVal})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured2.HasMore {
		t.Errorf("HasMore = true, want false (returned %d items at limit %d)", structured2.Count, limit)
	}
}

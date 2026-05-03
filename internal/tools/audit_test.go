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
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// hasMorePage's fallback path: when the caller passes no limit (or 0),
// the helper must compare against telegram.DefaultLimit. Direct unit
// test rather than indirect via a tool, so the contract is locked in
// without depending on any handler shape.
func TestAudit_HasMorePage_DefaultLimitFallback(t *testing.T) {
	cases := []struct {
		count, requested int
		want             bool
	}{
		{count: 100, requested: 0, want: true},  // saturates default
		{count: 99, requested: 0, want: false},  // one short of default
		{count: 100, requested: -1, want: true}, // negative also falls back
		{count: 5, requested: 5, want: true},    // exact-fit at custom limit
		{count: 4, requested: 5, want: false},   // short of custom limit
	}

	for _, tc := range cases {
		got := hasMorePage(tc.count, tc.requested)
		if got != tc.want {
			t.Errorf("hasMorePage(%d, %d) = %v, want %v", tc.count, tc.requested, got, tc.want)
		}
	}
}

// groups_list filters from a full dialog page; HasMore must reflect the
// underlying dialog page (some non-group dialogs may have been skipped),
// not the filtered groups slice. Pin the design so a future "fix" to
// `hasMorePage(len(groups), limit)` can't pass review.
func TestAudit_GroupsListHasMoreUsesUnderlyingDialogPage(t *testing.T) {
	const limit = 4

	mock := &mockClient{
		dialogs: []telegram.Dialog{
			{Title: "user", IsGroup: false},
			{Title: "group1", IsGroup: true},
			{Title: "user2", IsGroup: false},
			{Title: "group2", IsGroup: true},
		},
	}
	handler := NewGroupsListHandler(mock)

	limitVal := limit

	_, structured, err := handler(context.Background(), nil, GroupsListParams{Limit: &limitVal})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Count != 2 {
		t.Errorf("filtered Count = %d, want 2", structured.Count)
	}

	if !structured.HasMore {
		t.Errorf("HasMore = false; the underlying dialog page saturated limit %d, more groups may follow", limit)
	}
}

// emptyToolRequest builds a minimal CallToolRequest sufficient to dive
// into a handler's body without nil-deref panics on req.Params or
// req.Session lookups (both nil-safe in the production code).
func emptyToolRequest() *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{}}
}

// topicId pre-flight must fire on every send tool, not just messages_send.
// Drift would happen if someone inlined the validation differently in
// send_file or media_send_album.
func TestAudit_TopicIDValidatedOnSendFile(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewMessagesSendFileHandler(mock)

	topicID := 7

	_, _, err := handler(context.Background(), emptyToolRequest(), MessagesSendFileParams{
		Peer:    "@user",
		Path:    "/tmp/x",
		TopicID: &topicID,
	})
	if err == nil {
		t.Fatal("expected error for topicId on a user peer in send_file")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "forum") {
		t.Errorf("send_file error %q must mention 'forum'", err)
	}
}

func TestAudit_TopicIDValidatedOnMediaAlbum(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewMediaSendAlbumHandler(mock)

	topicID := 7

	_, _, err := handler(context.Background(), emptyToolRequest(), MediaSendAlbumParams{
		Peer:    "@user",
		Paths:   []string{"/tmp/a", "/tmp/b"},
		TopicID: &topicID,
	})
	if err == nil {
		t.Fatal("expected error for topicId on a user peer in media_album")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "forum") {
		t.Errorf("media_album error %q must mention 'forum'", err)
	}
}

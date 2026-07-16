package tools

import (
	"context"
	"reflect"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestGroupsMembersListHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewGroupsMembersListHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsMembersListParams{},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsMembersListHandler_BasicGroup(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChat, ID: 123},
		err:  errors.New("listing members is only supported for channels and supergroups"),
	}
	handler := NewGroupsMembersListHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsMembersListParams{Peer: "-123"},
	)
	if err == nil {
		t.Fatal("expected error for basic group")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsSlowmodeHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewGroupsSlowmodeHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsSlowmodeParams{},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsSlowmodeHandler_BasicGroup(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChat, ID: 456},
		err:  errors.New("slowmode is only supported for channels and supergroups"),
	}
	handler := NewGroupsSlowmodeHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsSlowmodeParams{Peer: "-456", Seconds: 30},
	)
	if err == nil {
		t.Fatal("expected error for basic group")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsAdminSetHandler_EmptyGroup(t *testing.T) {
	mock := &mockClient{}
	handler := NewGroupsAdminSetHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsAdminSetParams{User: "@alice"},
	)
	if err == nil {
		t.Fatal("expected error for empty group")
	}

	if !errors.Is(err, ErrGroupRequired) {
		t.Errorf("err = %v, want ErrGroupRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsAdminSetHandler_EmptyUser(t *testing.T) {
	mock := &mockClient{}
	handler := NewGroupsAdminSetHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsAdminSetParams{Group: "@mygroup"},
	)
	if err == nil {
		t.Fatal("expected error for empty user")
	}

	if !errors.Is(err, ErrUserRequired) {
		t.Errorf("err = %v, want ErrUserRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsAdminSetHandler_BasicGroup(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChat, ID: 789},
		err: errors.New(
			"setting admins is only supported for channels and supergroups",
		),
	}
	handler := NewGroupsAdminSetHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsAdminSetParams{Group: "-789", User: "@alice"},
	)
	if err == nil {
		t.Fatal("expected error for basic group")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestGroupsAdminSetTool_Annotation(t *testing.T) {
	tool := GroupsAdminSetTool()
	if tool.Annotations == nil {
		t.Fatal("annotations must not be nil")
	}

	if !tool.Annotations.IdempotentHint {
		t.Error("GroupsAdminSet should be idempotent")
	}
}

func TestTopicsEditHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewTopicsEditHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		TopicsEditParams{TopicID: 1, Title: "test"},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestTopicsEditHandler_EmptyTitle(t *testing.T) {
	mock := &mockClient{}
	handler := NewTopicsEditHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		TopicsEditParams{Peer: "@group", TopicID: 1},
	)
	if err == nil {
		t.Fatal("expected error for empty title")
	}

	if !errors.Is(err, ErrTitleRequired) {
		t.Errorf("err = %v, want ErrTitleRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestTopicsCreateHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewTopicsCreateHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		TopicsCreateParams{Title: "test"},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestTopicsCreateHandler_EmptyTitle(t *testing.T) {
	mock := &mockClient{}
	handler := NewTopicsCreateHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		TopicsCreateParams{Peer: "@group"},
	)
	if err == nil {
		t.Fatal("expected error for empty title")
	}

	if !errors.Is(err, ErrTitleRequired) {
		t.Errorf("err = %v, want ErrTitleRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesSendHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesSendHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		MessagesSendParams{ParseMode: "plain", Text: "hello"},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesSendFileHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesSendFileHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		MessagesSendFileParams{ParseMode: "plain", Path: "/tmp/f"},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesSendFileHandler_EmptyPath(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesSendFileHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		MessagesSendFileParams{ParseMode: "plain", Peer: "@user"},
	)
	if err == nil {
		t.Fatal("expected error for empty path")
	}

	if !errors.Is(err, ErrPathRequired) {
		t.Errorf("err = %v, want ErrPathRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMediaSendAlbumHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewMediaSendAlbumHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		MediaSendAlbumParams{ParseMode: "plain", Paths: []string{"/f"}},
	)
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if !errors.Is(err, ErrPeerRequired) {
		t.Errorf("err = %v, want ErrPeerRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMediaSendAlbumHandler_EmptyPaths(t *testing.T) {
	mock := &mockClient{}
	handler := NewMediaSendAlbumHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		MediaSendAlbumParams{ParseMode: "plain", Peer: "@user"},
	)
	if err == nil {
		t.Fatal("expected error for empty paths")
	}

	if !errors.Is(err, ErrPathsRequired) {
		t.Errorf("err = %v, want ErrPathsRequired", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

// --- Issue 1: TopicsEdit annotation test ---

func TestTopicsEditTool_Annotation(t *testing.T) {
	tool := TopicsEditTool()
	if tool.Annotations == nil {
		t.Fatal("annotations must not be nil")
	}

	if !tool.Annotations.IdempotentHint {
		t.Error("TopicsEdit should be idempotent")
	}
}

// --- Issue 2: GroupsSlowmode invalid seconds ---

func TestGroupsSlowmodeHandler_InvalidSeconds(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
	}
	handler := NewGroupsSlowmodeHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		GroupsSlowmodeParams{Peer: "@group", Seconds: 15},
	)
	if err == nil {
		t.Fatal("expected error for invalid slowmode seconds")
	}

	if !errors.Is(err, ErrInvalidSlowmode) {
		t.Errorf("err = %v, want ErrInvalidSlowmode", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

// --- Issue 4: TopicsCreate nil topic ---

func TestTopicsCreateHandler_NilTopic(t *testing.T) {
	mock := &mockClient{
		peer:  telegram.InputPeer{Type: telegram.PeerChannel, ID: 1},
		topic: nil,
	}
	handler := NewTopicsCreateHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		TopicsCreateParams{Peer: "@group", Title: "test"},
	)
	if err == nil {
		t.Fatal("expected error for nil topic response")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

// --- Happy-path and error-propagation tests for all 16 tools ---

func TestMessagesGetScheduledHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		messages: []telegram.Message{
			{ID: 10, Text: "scheduled"},
		},
	}
	handler := NewMessagesGetScheduledHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		MessagesGetScheduledParams{Peer: "@alice"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("count = %d, want 1", res.Count)
	}
}

func TestMessagesGetScheduledHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewMessagesGetScheduledHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		MessagesGetScheduledParams{Peer: "@alice"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestMessagesSearchGlobalHandler_Happy(t *testing.T) {
	mock := &mockClient{
		messages: []telegram.Message{
			{ID: 20, Text: "found"},
		},
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		MessagesSearchGlobalParams{Query: "hello"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("count = %d, want 1", res.Count)
	}
}

// TestMessagesSearchGlobalHandler_PeerIDPropagated pins the
// PeerID-per-message contract that docs/guides/messages.md documents as the only
// way callers of global search learn which chat a result came from.
// A regression in messageToItem dropping msg.PeerID would silently
// break cross-peer attribution; this test catches it.
func TestMessagesSearchGlobalHandler_PeerIDPropagated(t *testing.T) {
	mock := &mockClient{
		messages: []telegram.Message{
			{
				ID:     20,
				PeerID: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1006503122},
				Text:   "from channel",
			},
			{
				ID:     21,
				PeerID: telegram.InputPeer{Type: telegram.PeerUser, ID: 42},
				Text:   "from user DM",
			},
		},
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		MessagesSearchGlobalParams{Query: "hello"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(res.Messages))
	}

	if res.Messages[0].PeerID.Type != telegram.PeerChannel || res.Messages[0].PeerID.ID != 1006503122 {
		t.Errorf("messages[0].PeerID = %+v, want channel:1006503122", res.Messages[0].PeerID)
	}

	if res.Messages[1].PeerID.Type != telegram.PeerUser || res.Messages[1].PeerID.ID != 42 {
		t.Errorf("messages[1].PeerID = %+v, want user:42", res.Messages[1].PeerID)
	}
}

// TestMessagesSearchGlobalResult_NoParticipantsField pins that the
// MessagesSearchGlobalResult JSON shape has no Participants field.
// Global search spans arbitrary peers with unreliable access-hash
// resolution; documenting it here so the documented claim stays honest.
func TestMessagesSearchGlobalResult_NoParticipantsField(t *testing.T) {
	val := reflect.ValueOf(MessagesSearchGlobalResult{})
	if _, found := val.Type().FieldByName("Participants"); found {
		t.Error("MessagesSearchGlobalResult must not expose Participants — see docs/guides/messages.md's tg_messages_search_global caveat")
	}
}

func TestMessagesSearchGlobalHandler_Error(t *testing.T) {
	mock := &mockClient{
		err: errors.New("api failure"),
	}
	handler := NewMessagesSearchGlobalHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		MessagesSearchGlobalParams{Query: "hello"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestContactsListBlockedHandler_Happy(t *testing.T) {
	mock := &mockClient{
		users: []telegram.User{
			{ID: 30, FirstName: "Bob"},
		},
	}
	handler := NewContactsListBlockedHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		ContactsListBlockedParams{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("count = %d, want 1", res.Count)
	}
}

func TestContactsListBlockedHandler_Error(t *testing.T) {
	mock := &mockClient{
		err: errors.New("api failure"),
	}
	handler := NewContactsListBlockedHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		ContactsListBlockedParams{},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestMessagesGetReactionsHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		reactions: []telegram.ReactionUser{
			{UserID: 40, Emoji: "👍"},
		},
	}
	handler := NewMessagesGetReactionsHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		MessagesGetReactionsParams{
			Peer: "@chat", MessageID: 5,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("count = %d, want 1", res.Count)
	}
}

func TestMessagesGetReactionsHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewMessagesGetReactionsHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		MessagesGetReactionsParams{
			Peer: "@chat", MessageID: 5,
		},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestGroupsMembersListHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		users: []telegram.User{
			{ID: 50, FirstName: "Eve"},
		},
	}
	handler := NewGroupsMembersListHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		GroupsMembersListParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("count = %d, want 1", res.Count)
	}
}

func TestGroupsMembersListHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		err: errors.New("api failure"),
	}
	handler := NewGroupsMembersListHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		GroupsMembersListParams{Peer: "@group"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestContactsGetStatusesHandler_Happy(t *testing.T) {
	mock := &mockClient{
		statuses: []telegram.ContactStatus{
			{UserID: 60, Status: "online"},
		},
	}
	handler := NewContactsGetStatusesHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		ContactsGetStatusesParams{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("count = %d, want 1", res.Count)
	}
}

func TestContactsGetStatusesHandler_Error(t *testing.T) {
	mock := &mockClient{
		err: errors.New("api failure"),
	}
	handler := NewContactsGetStatusesHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		ContactsGetStatusesParams{},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestDialogsPinHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewDialogsPinHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		DialogsPinParams{Peer: "@alice", Pinned: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestDialogsPinHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewDialogsPinHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		DialogsPinParams{Peer: "@alice", Pinned: true},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestDialogsMarkUnreadHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewDialogsMarkUnreadHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		DialogsMarkUnreadParams{Peer: "@alice", Unread: true},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestDialogsMarkUnreadHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewDialogsMarkUnreadHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		DialogsMarkUnreadParams{Peer: "@alice", Unread: true},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestGroupsSlowmodeHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
	}
	handler := NewGroupsSlowmodeHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		GroupsSlowmodeParams{Peer: "@group", Seconds: 30},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestGroupsSlowmodeHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		err: errors.New("api failure"),
	}
	handler := NewGroupsSlowmodeHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		GroupsSlowmodeParams{Peer: "@group", Seconds: 30},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestTopicsCreateHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		topic: &telegram.ForumTopic{ID: 99, Title: "New"},
	}
	handler := NewTopicsCreateHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		TopicsCreateParams{Peer: "@group", Title: "New"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TopicID != 99 {
		t.Errorf("topicID = %d, want 99", res.TopicID)
	}
}

func TestTopicsCreateHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		err: errors.New("api failure"),
	}
	handler := NewTopicsCreateHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		TopicsCreateParams{Peer: "@group", Title: "New"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestTopicsEditHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
	}
	handler := NewTopicsEditHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		TopicsEditParams{
			Peer: "@group", TopicID: 1, Title: "Edited",
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestTopicsEditHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		err: errors.New("api failure"),
	}
	handler := NewTopicsEditHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		TopicsEditParams{
			Peer: "@group", TopicID: 1, Title: "Edited",
		},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestContactsAddHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewContactsAddHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		ContactsAddParams{Peer: "@alice", FirstName: "Alice"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestContactsAddHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewContactsAddHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		ContactsAddParams{Peer: "@alice", FirstName: "Alice"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestGroupsAdminSetHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
	}
	handler := NewGroupsAdminSetHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		GroupsAdminSetParams{Group: "@group", User: "@alice"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestGroupsAdminSetHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 1,
		},
		err: errors.New("api failure"),
	}
	handler := NewGroupsAdminSetHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		GroupsAdminSetParams{Group: "@group", User: "@alice"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestContactsDeleteHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewContactsDeleteHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		ContactsDeleteParams{Peer: "@alice"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Deleted {
		t.Error("expected deleted = true")
	}
}

func TestContactsDeleteHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewContactsDeleteHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		ContactsDeleteParams{Peer: "@alice"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestMessagesDeleteHistoryHandler_Happy(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewMessagesDeleteHistoryHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		MessagesDeleteHistoryParams{Peer: "@alice"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Output == "" {
		t.Error("expected non-empty output")
	}
}

func TestMessagesDeleteHistoryHandler_Error(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
		err:  errors.New("api failure"),
	}
	handler := NewMessagesDeleteHistoryHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		MessagesDeleteHistoryParams{Peer: "@alice"},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestMessagesClearAllDraftsHandler_Happy(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesClearAllDraftsHandler(mock)

	_, res, err := handler(
		context.Background(), nil,
		MessagesClearAllDraftsParams{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Cleared {
		t.Error("expected cleared = true")
	}
}

func TestMessagesClearAllDraftsHandler_Error(t *testing.T) {
	mock := &mockClient{
		err: errors.New("api failure"),
	}
	handler := NewMessagesClearAllDraftsHandler(mock)

	_, _, err := handler(
		context.Background(), nil,
		MessagesClearAllDraftsParams{},
	)
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

// --- Issue 1-3: DeleteHistory channel guard ---

func TestDeleteHistoryHandler_ChannelRejected(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 123,
		},
		err: errors.New(
			"delete history is only supported for users and basic groups",
		),
	}
	handler := NewMessagesDeleteHistoryHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		MessagesDeleteHistoryParams{Peer: "@channel"},
	)
	if err == nil {
		t.Fatal("expected error for channel peer")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

// --- Issue 4: Contacts add/delete channel guard ---

func TestContactsAddHandler_ChannelRejected(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 123,
		},
	}
	handler := NewContactsAddHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		ContactsAddParams{Peer: "@channel", FirstName: "Test"},
	)
	if err == nil {
		t.Fatal("expected error for channel peer")
	}

	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestContactsDeleteHandler_ChannelRejected(t *testing.T) {
	mock := &mockClient{
		peer: telegram.InputPeer{
			Type: telegram.PeerChannel, ID: 123,
		},
	}
	handler := NewContactsDeleteHandler(mock)

	result, _, err := handler(
		context.Background(), nil,
		ContactsDeleteParams{Peer: "@channel"},
	)
	if err == nil {
		t.Fatal("expected error for channel peer")
	}

	if !errors.Is(err, ErrValidation) {
		t.Errorf("err = %v, want ErrValidation", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestTopicsEditHandler_ZeroTopicID(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1}}
	handler := NewTopicsEditHandler(mock)

	result, _, err := handler(context.Background(), nil, TopicsEditParams{
		Peer:    "@group",
		TopicID: 0,
		Title:   "test",
	})

	if err == nil {
		t.Fatal("expected error for zero topic ID")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesListHandler_Happy(t *testing.T) {
	mock := &mockClient{
		messages: []telegram.Message{{ID: 1, Date: 100, Text: "hello"}},
		total:    1,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{Peer: "@chat"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}
}

func TestMessagesListHandler_WithTopicID(t *testing.T) {
	topicID := 42
	mock := &mockClient{
		messages: []telegram.Message{{ID: 1}},
		total:    1,
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:    "@chat",
		TopicID: &topicID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}

	if mock.lastTopicID != 42 {
		t.Errorf("lastTopicID = %d, want 42", mock.lastTopicID)
	}
}

// typeFilterPage builds a newest-first page of n messages numbered
// downward from startID, all of the given type.
func typeFilterPage(startID, n int, msgType string) []telegram.Message {
	msgs := make([]telegram.Message, n)
	for i := range n {
		msgs[i] = telegram.Message{ID: startID - i, Date: 1000, Type: msgType}
	}

	return msgs
}

func TestMessagesListHandler_TypeFilterPaginatesUntilLimit(t *testing.T) {
	limit := 2
	// The first page is a full server page of non-matching messages, so
	// the outer loop learns more history exists and pages on; the second
	// page carries the matches. A short page means end-of-history now
	// that each RPC is bounded to one server page.
	mock := &mockClient{
		getHistoryFn: func(_ telegram.InputPeer, opts telegram.HistoryOpts) ([]telegram.Message, int, error) {
			switch opts.OffsetID {
			case 0:
				return typeFilterPage(1000, telegram.DefaultLimit, "photo"), 104, nil
			case 901:
				return []telegram.Message{
					{ID: 900, Date: 998, Type: "voice"},
					{ID: 899, Date: 997, Type: "voice"},
				}, 104, nil
			default:
				return nil, 104, nil
			}
		},
	}
	handler := NewMessagesListHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesListParams{
		Peer:  "@chat",
		Limit: &limit,
		Type:  "voice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2", res.Count)
	}

	for _, msg := range res.Messages {
		if msg.Type != "voice" {
			t.Errorf("returned message type = %q, want voice", msg.Type)
		}
	}

	if mock.getHistoryCalls != 2 {
		t.Fatalf("GetHistory calls = %d, want 2", mock.getHistoryCalls)
	}

	if got := mock.getHistoryOpts[1].OffsetID; got != 901 {
		t.Errorf("second page OffsetID = %d, want 901", got)
	}
}

func TestMessagesListHandler_RejectsUnknownTypeFilter(t *testing.T) {
	mock := &mockClient{peer: telegram.InputPeer{Type: telegram.PeerChannel, ID: 1}}
	handler := NewMessagesListHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesListParams{
		Peer: "@chat",
		Type: "unknown_kind",
	})
	if err == nil {
		t.Fatal("expected validation error for unknown type")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesListHandler_EmptyPeer(t *testing.T) {
	mock := &mockClient{}
	handler := NewMessagesListHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesListParams{})
	if err == nil {
		t.Fatal("expected error for empty peer")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesSendHandler_WithTopicID(t *testing.T) {
	topicID := 42
	mock := &mockClient{
		message: &telegram.Message{ID: 1},
		peer:    telegram.InputPeer{Type: telegram.PeerChannel, ID: 1, AccessHash: 1},
		group:   &telegram.GroupInfo{IsForum: true, IsSupergroup: true, Title: "Forum"},
	}
	handler := NewMessagesSendHandler(mock)

	_, res, err := handler(context.Background(), nil, MessagesSendParams{
		ParseMode: "plain",

		Peer:    "@chat",
		Text:    "hello",
		TopicID: &topicID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.MessageID != 1 {
		t.Errorf("MessageID = %d, want 1", res.MessageID)
	}

	if mock.lastSendOpts.TopicID != topicID {
		t.Errorf("TopicID = %d, want %d", mock.lastSendOpts.TopicID, topicID)
	}
}

const testBadParseMode = "bogus"

func TestMessagesEditHandler_InvalidParseMode(t *testing.T) {
	badMode := testBadParseMode
	mock := &mockClient{
		peer: telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
	}
	handler := NewMessagesEditHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesEditParams{
		Peer:      "@chat",
		MessageID: 1,
		Text:      "hi",
		ParseMode: badMode,
	})
	if err == nil {
		t.Fatal("expected error for invalid parseMode")
	}

	if !errors.Is(err, ErrUnknownParseMode) {
		t.Errorf("err = %v, want ErrUnknownParseMode", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

func TestMessagesSendHandler_InvalidParseMode(t *testing.T) {
	badMode := testBadParseMode
	mock := &mockClient{}
	handler := NewMessagesSendHandler(mock)

	result, _, err := handler(context.Background(), nil, MessagesSendParams{
		Peer:      "@chat",
		Text:      "hi",
		ParseMode: badMode,
	})
	if err == nil {
		t.Fatal("expected error for invalid parseMode")
	}

	if !errors.Is(err, ErrUnknownParseMode) {
		t.Errorf("err = %v, want ErrUnknownParseMode", err)
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

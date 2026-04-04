package tools

import (
	"context"
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
		MessagesSendParams{Text: "hello"},
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
		MessagesSendFileParams{Path: "/tmp/f"},
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
		MessagesSendFileParams{Peer: "@user"},
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
		MediaSendAlbumParams{Paths: []string{"/f"}},
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
		MediaSendAlbumParams{Peer: "@user"},
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

package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
)

const testInviteLink = "https://t.me/+primary"

func TestGroupsInviteLinkGetTool_StaysReadOnly(t *testing.T) {
	tool := GroupsInviteLinkGetTool()
	if tool.Name != "tg_groups_invite_link_get" {
		t.Errorf("name = %q, want tg_groups_invite_link_get", tool.Name)
	}

	if !tool.Annotations.ReadOnlyHint {
		t.Error("reading the invite link must be annotated read-only")
	}
}

func TestGroupsInviteLinkGetHandler_ReturnsTheLink(t *testing.T) {
	mock := &mockClient{peer: destPeer(), link: testInviteLink}

	_, structured, err := NewGroupsInviteLinkGetHandler(mock)(
		context.Background(), nil, GroupsInviteLinkGetParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Link != testInviteLink {
		t.Errorf("Link = %q, want %q", structured.Link, testInviteLink)
	}

	if structured.Output != "Invite link: "+testInviteLink {
		t.Errorf("Output = %q", structured.Output)
	}
}

func TestGroupsInviteLinkGetHandler_EmptyPeer(t *testing.T) {
	_, _, err := NewGroupsInviteLinkGetHandler(&mockClient{})(
		context.Background(), nil, GroupsInviteLinkGetParams{},
	)
	if !errors.Is(err, ErrPeerRequired) {
		t.Fatalf("error = %v, want ErrPeerRequired", err)
	}
}

func TestGroupsInviteLinkRevokeHandler_EmptyLink(t *testing.T) {
	_, _, err := NewGroupsInviteLinkRevokeHandler(&mockClient{peer: destPeer()})(
		context.Background(), nil, GroupsInviteLinkRevokeParams{Peer: "@group"},
	)
	if !errors.Is(err, ErrLinkRequired) {
		t.Fatalf("error = %v, want ErrLinkRequired", err)
	}
}

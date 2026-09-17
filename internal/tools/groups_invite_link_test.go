package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
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

func TestGroupsInviteLinkCreateTool_IsAnnotatedWrite(t *testing.T) {
	tool := GroupsInviteLinkCreateTool()
	if tool.Name != "tg_groups_invite_link_create" {
		t.Errorf("name = %q, want tg_groups_invite_link_create", tool.Name)
	}

	if tool.Annotations.ReadOnlyHint {
		t.Error("creating a link is not a read")
	}

	if tool.Annotations.IdempotentHint {
		t.Error("every call creates another link, so it is not idempotent")
	}
}

func TestGroupsInviteLinkCreateHandler_PassesTheOptions(t *testing.T) {
	mock := &mockClient{
		peer:          destPeer(),
		createdInvite: &telegram.InviteLink{Link: testInviteLink, Title: "Conference", UsageLimit: 50},
	}

	title, expire, limit, needed := "Conference", 1800000000, 50, true

	_, structured, err := NewGroupsInviteLinkCreateHandler(mock)(
		context.Background(), nil, GroupsInviteLinkCreateParams{
			Peer: "@group", Title: &title, ExpireDate: &expire,
			UsageLimit: &limit, RequestNeeded: &needed,
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := telegram.InviteLinkOpts{
		Title: title, ExpireDate: expire, UsageLimit: limit, RequestNeeded: needed,
	}
	if mock.lastInviteOpts != want {
		t.Errorf("opts = %+v, want %+v", mock.lastInviteOpts, want)
	}

	if structured.Invite.Link != testInviteLink {
		t.Errorf("Invite.Link = %q, want %q", structured.Invite.Link, testInviteLink)
	}

	if structured.Output != "Created invite link: "+testInviteLink {
		t.Errorf("Output = %q", structured.Output)
	}
}

func TestGroupsInviteLinkCreateHandler_RejectsBadInput(t *testing.T) {
	negativeLimit, negativeDate := -1, -1

	for name, testCase := range map[string]struct {
		params GroupsInviteLinkCreateParams
		want   error
	}{
		"no peer":         {GroupsInviteLinkCreateParams{}, ErrPeerRequired},
		"negative limit":  {GroupsInviteLinkCreateParams{Peer: "@g", UsageLimit: &negativeLimit}, ErrNegativeLimit},
		"negative expiry": {GroupsInviteLinkCreateParams{Peer: "@g", ExpireDate: &negativeDate}, ErrNegativeDate},
	} {
		t.Run(name, func(t *testing.T) {
			mock := &mockClient{peer: destPeer()}

			_, _, err := NewGroupsInviteLinkCreateHandler(mock)(context.Background(), nil, testCase.params)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}

			if mock.lastInviteOpts != (telegram.InviteLinkOpts{}) {
				t.Error("bad input must be rejected before the client is called")
			}
		})
	}
}

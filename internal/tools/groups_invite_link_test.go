package tools

import (
	"context"
	"strings"
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

func TestGroupsInviteLinkListTool_IsReadOnly(t *testing.T) {
	tool := GroupsInviteLinkListTool()
	if tool.Name != "tg_groups_invite_link_list" {
		t.Errorf("name = %q, want tg_groups_invite_link_list", tool.Name)
	}

	if !tool.Annotations.ReadOnlyHint {
		t.Error("listing links must be annotated read-only")
	}
}

// The server's total and the number of rows returned are different facts, and
// there is no cursor — so reporting the row count twice would hide the ceiling.
func TestGroupsInviteLinkListHandler_ReportsTheServerTotal(t *testing.T) {
	mock := &mockClient{
		peer:        destPeer(),
		inviteLinks: []telegram.InviteLink{{Link: testInviteLink, Usage: 4}},
		inviteTotal: 7,
	}

	_, structured, err := NewGroupsInviteLinkListHandler(mock)(
		context.Background(), nil, GroupsInviteLinkListParams{Peer: "@group"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Count != 1 {
		t.Errorf("Count = %d, want 1", structured.Count)
	}

	if structured.Total != 7 {
		t.Errorf("Total = %d, want the server's 7", structured.Total)
	}

	if !strings.Contains(structured.Output, "1 of 7 invite link(s)") {
		t.Errorf("Output = %q", structured.Output)
	}
}

func TestGroupsInviteLinkListHandler_PassesRevokedAndLimit(t *testing.T) {
	mock := &mockClient{peer: destPeer()}
	revoked, limit := true, 5

	_, _, err := NewGroupsInviteLinkListHandler(mock)(
		context.Background(), nil,
		GroupsInviteLinkListParams{Peer: "@group", Revoked: &revoked, Limit: &limit},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.lastInviteRevoked {
		t.Error("revoked must reach the client")
	}

	if mock.lastInviteLimit != limit {
		t.Errorf("limit = %d, want %d", mock.lastInviteLimit, limit)
	}
}

func TestGroupsInviteLinkListHandler_RejectsBadInput(t *testing.T) {
	negative := -1

	for name, testCase := range map[string]struct {
		params GroupsInviteLinkListParams
		want   error
	}{
		"no peer":        {GroupsInviteLinkListParams{}, ErrPeerRequired},
		"negative limit": {GroupsInviteLinkListParams{Peer: "@g", Limit: &negative}, ErrNegativeLimit},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := NewGroupsInviteLinkListHandler(&mockClient{peer: destPeer()})(
				context.Background(), nil, testCase.params,
			)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
		})
	}
}

func TestFormatInviteLinks_RendersTheFactsThatMatter(t *testing.T) {
	links := []telegram.InviteLink{
		{Link: "https://t.me/+A", Title: "Conference", Usage: 12, UsageLimit: 50, ExpireDate: 1800000000},
		{Link: "https://t.me/+B", Permanent: true, Usage: 4, RequestNeeded: true, Requested: 2},
	}

	got := formatInviteLinks(links, 7)

	for _, want := range []string{
		"2 of 7 invite link(s)",
		`https://t.me/+A — "Conference", 12/50 used, expires 2027-01-15T08:00:00Z`,
		"https://t.me/+B — permanent, 4 used, approval required, 2 pending",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestFormatInviteLinks_EmptySaysSo(t *testing.T) {
	if got := formatInviteLinks(nil, 0); got != "No invite links found" {
		t.Errorf("output = %q", got)
	}
}

func TestGroupsInviteLinkRevokeHandler_SurfacesTheNewPrimaryLink(t *testing.T) {
	const replacement = "https://t.me/+replacement"

	mock := &mockClient{peer: destPeer(), replacementLink: replacement}

	_, structured, err := NewGroupsInviteLinkRevokeHandler(mock)(
		context.Background(), nil,
		GroupsInviteLinkRevokeParams{Peer: "@group", Link: testInviteLink},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.NewLink != replacement {
		t.Errorf("NewLink = %q, want %q", structured.NewLink, replacement)
	}

	if !strings.Contains(structured.Output, replacement) {
		t.Errorf("Output = %q, want it to name the replacement", structured.Output)
	}
}

func TestGroupsInviteLinkRevokeHandler_NoReplacementLeavesTheFieldEmpty(t *testing.T) {
	mock := &mockClient{peer: destPeer()}

	_, structured, err := NewGroupsInviteLinkRevokeHandler(mock)(
		context.Background(), nil,
		GroupsInviteLinkRevokeParams{Peer: "@group", Link: testInviteLink},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.NewLink != "" {
		t.Errorf("NewLink = %q, want empty", structured.NewLink)
	}

	if strings.Contains(structured.Output, "replaced") {
		t.Errorf("Output = %q, must not claim a replacement", structured.Output)
	}
}

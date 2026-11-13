package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// alreadyDoneCall runs one tool handler against a client whose write fails
// with err, and returns what the caller would see.
type alreadyDoneCall func(client telegram.Client) (*mcp.CallToolResult, string, error)

func alreadyDoneCalls() map[string]alreadyDoneCall {
	ctx := context.Background()

	return map[string]alreadyDoneCall{
		"tg_groups_join": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewGroupsJoinHandler(c)(ctx, nil, GroupsJoinParams{Peer: "@grp"})

			return res, out.Output, err
		},
		"tg_groups_leave": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewGroupsLeaveHandler(c)(ctx, nil, GroupsLeaveParams{Peer: "@grp"})

			return res, out.Output, err
		},
		"tg_groups_members_add": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewGroupsMembersAddHandler(c)(ctx, nil, GroupsMembersAddParams{Group: "@grp", User: "@usr"})

			return res, out.Output, err
		},
		"tg_groups_rename": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewGroupsRenameHandler(c)(ctx, nil, GroupsRenameParams{Peer: "@grp", Title: "Same"})

			return res, out.Output, err
		},
		"tg_groups_slowmode": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewGroupsSlowmodeHandler(c)(ctx, nil, GroupsSlowmodeParams{Peer: "@grp", Seconds: 10})

			return res, out.Output, err
		},
		"tg_chats_set_description": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewChatsSetDescriptionHandler(c)(ctx, nil, ChatsSetDescriptionParams{Peer: "@grp", About: "x"})

			return res, out.Output, err
		},
		"tg_chats_set_permissions": func(c telegram.Client) (*mcp.CallToolResult, string, error) {
			res, out, err := NewChatsPermissionsHandler(c)(ctx, nil, ChatsPermissionsParams{Peer: "@grp"})

			return res, out.Output, err
		},
	}
}

func clientFailingWith(code string) *mockClient {
	peer := telegram.InputPeer{Type: telegram.PeerChannel, ID: 1, AccessHash: 2}

	return &mockClient{
		resolvePeerFn: func(string) (telegram.InputPeer, error) { return peer, nil },
		err:           errors.Wrap(tgerr.New(400, code), "wrapper context"),
	}
}

// A refusal stating that the requested state already holds is the tool's
// postcondition, whether the caller repeated the call or a resend below the
// tool repeated it after the first copy applied.
func TestAlreadyDone_RefusalStatingThePostconditionIsSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tool, code, want string
	}{
		{"tg_groups_join", "USER_ALREADY_PARTICIPANT", "Already a member of @grp"},
		{"tg_groups_leave", "USER_NOT_PARTICIPANT", "Not a member of @grp"},
		{"tg_groups_members_add", "USER_ALREADY_PARTICIPANT", "@usr is already a member of @grp"},
		{"tg_groups_rename", "CHAT_NOT_MODIFIED", `Group @grp already has the title "Same"`},
		{"tg_groups_slowmode", "CHAT_NOT_MODIFIED", "Slowmode is already 10s for @grp"},
		{"tg_chats_set_description", "CHAT_ABOUT_NOT_MODIFIED", "Description for @grp already matches"},
		{"tg_chats_set_description", "CHAT_NOT_MODIFIED", "Description for @grp already matches"},
		{"tg_chats_set_permissions", "CHAT_NOT_MODIFIED", "Chat permissions for @grp already match"},
	}

	calls := alreadyDoneCalls()

	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.code, func(t *testing.T) {
			t.Parallel()

			res, out, err := calls[tt.tool](clientFailingWith(tt.code))
			if err != nil {
				t.Fatalf("got error %v, want success", err)
			}

			if res != nil && res.IsError {
				t.Fatal("result marked IsError")
			}

			if out != tt.want {
				t.Errorf("output = %q, want %q", out, tt.want)
			}
		})
	}
}

// Each tool accepts only the codes that state ITS postcondition: another
// tool's "already" code, or an unrelated refusal, still fails.
func TestAlreadyDone_OtherRefusalsStillFail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tool, code string
	}{
		{"tg_groups_join", "USER_NOT_PARTICIPANT"},
		{"tg_groups_join", "INVITE_REQUEST_SENT"},
		{"tg_groups_leave", "USER_ALREADY_PARTICIPANT"},
		{"tg_groups_members_add", "USER_NOT_PARTICIPANT"},
		{"tg_groups_rename", "CHAT_ABOUT_NOT_MODIFIED"},
		{"tg_groups_slowmode", "USER_ALREADY_PARTICIPANT"},
		{"tg_chats_set_description", "CHAT_ADMIN_REQUIRED"},
		{"tg_chats_set_permissions", "CHAT_ABOUT_NOT_MODIFIED"},
	}

	calls := alreadyDoneCalls()

	for _, tt := range tests {
		t.Run(tt.tool+"/"+tt.code, func(t *testing.T) {
			t.Parallel()

			res, _, err := calls[tt.tool](clientFailingWith(tt.code))
			if err == nil || res == nil || !res.IsError {
				t.Fatalf("got res=%v err=%v, want an error result", res, err)
			}
		})
	}
}

// The wrapper answers an unchanged edit exactly like an applied one, so the
// edit reply must be a statement about the message that is true either way.
func TestAlreadyDone_EditReplyDescribesTheState(t *testing.T) {
	t.Parallel()

	mock := &mockClient{
		resolvePeerFn: func(string) (telegram.InputPeer, error) {
			return telegram.InputPeer{Type: telegram.PeerUser, ID: 1, AccessHash: 2}, nil
		},
		message: &telegram.Message{ID: 42, Entities: []telegram.Entity{{Type: "bold", Length: 1}}},
	}

	_, out, err := NewMessagesEditHandler(mock)(context.Background(), nil, MessagesEditParams{
		Peer: "@usr", MessageID: 42, Text: "**x**", ParseMode: "commonmark",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	if out.Output != "Message 42 has the requested text" {
		t.Errorf("output = %q, want a statement of the state", out.Output)
	}

	if out.EntitiesParsed != 1 {
		t.Errorf("entitiesParsed = %d, want 1", out.EntitiesParsed)
	}
}

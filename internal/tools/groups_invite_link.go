package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsInviteLinkGetParams defines the parameters for the tg_groups_invite_link_get tool.
type GroupsInviteLinkGetParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// GroupsInviteLinkGetResult is the output of the tg_groups_invite_link_get tool.
type GroupsInviteLinkGetResult struct {
	Link   string `json:"link"`
	Output string `json:"output"`
}

// NewGroupsInviteLinkGetHandler creates a handler for the tg_groups_invite_link_get tool.
func NewGroupsInviteLinkGetHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsInviteLinkGetParams, GroupsInviteLinkGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteLinkGetParams,
	) (*mcp.CallToolResult, GroupsInviteLinkGetResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkGetResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkGetResult{},
				telegramErr("failed to resolve peer", err)
		}

		link, err := client.GetInviteLink(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkGetResult{},
				telegramErr("failed to get invite link", err)
		}

		return nil, GroupsInviteLinkGetResult{
			Link:   link,
			Output: "Invite link: " + link,
		}, nil
	}
}

// GroupsInviteLinkGetTool returns the MCP tool definition for tg_groups_invite_link_get.
func GroupsInviteLinkGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_invite_link_get",
		Description: "Get the invite link for a Telegram group",
		Annotations: readOnlyAnnotations(),
	}
}

// GroupsInviteLinkRevokeParams defines the parameters for the tg_groups_invite_link_revoke tool.
type GroupsInviteLinkRevokeParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
	Link string `json:"link" jsonschema:"Invite link to revoke"`
}

// GroupsInviteLinkRevokeResult is the output of the tg_groups_invite_link_revoke tool.
type GroupsInviteLinkRevokeResult struct {
	Peer   string `json:"peer"`
	Output string `json:"output"`
}

// NewGroupsInviteLinkRevokeHandler creates a handler for the tg_groups_invite_link_revoke tool.
func NewGroupsInviteLinkRevokeHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsInviteLinkRevokeParams, GroupsInviteLinkRevokeResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteLinkRevokeParams,
	) (*mcp.CallToolResult, GroupsInviteLinkRevokeResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Link == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				validationErr(ErrLinkRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.RevokeInviteLink(ctx, peer, params.Link)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				telegramErr("failed to revoke invite link", err)
		}

		return nil, GroupsInviteLinkRevokeResult{
			Peer:   params.Peer,
			Output: "Revoked invite link for " + params.Peer,
		}, nil
	}
}

// GroupsInviteLinkRevokeTool returns the MCP tool definition for tg_groups_invite_link_revoke.
func GroupsInviteLinkRevokeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_invite_link_revoke",
		Description: "Revoke an invite link for a Telegram group",
		Annotations: idempotentAnnotations(),
	}
}

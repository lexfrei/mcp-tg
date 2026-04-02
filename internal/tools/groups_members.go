package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsMembersAddParams defines the parameters for the tg_groups_members_add tool.
type GroupsMembersAddParams struct {
	Group string `json:"group" jsonschema:"@username, t.me/ link, or numeric ID"`
	User  string `json:"user"  jsonschema:"User ID or @username to add"`
}

// GroupsMembersAddResult is the output of the tg_groups_members_add tool.
type GroupsMembersAddResult struct {
	Group  string `json:"group"`
	User   string `json:"user"`
	Output string `json:"output"`
}

// NewGroupsMembersAddHandler creates a handler for the tg_groups_members_add tool.
func NewGroupsMembersAddHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsMembersAddParams, GroupsMembersAddResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsMembersAddParams,
	) (*mcp.CallToolResult, GroupsMembersAddResult, error) {
		if params.Group == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersAddResult{},
				validationErr(ErrGroupRequired)
		}

		if params.User == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersAddResult{},
				validationErr(ErrUserRequired)
		}

		group, err := client.ResolvePeer(ctx, params.Group)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersAddResult{},
				telegramErr("failed to resolve group", err)
		}

		user, err := client.ResolvePeer(ctx, params.User)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersAddResult{},
				telegramErr("failed to resolve user", err)
		}

		err = client.AddGroupMember(ctx, group, user)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersAddResult{},
				telegramErr("failed to add member to group", err)
		}

		return nil, GroupsMembersAddResult{
			Group:  params.Group,
			User:   params.User,
			Output: fmt.Sprintf("Added %s to %s", params.User, params.Group),
		}, nil
	}
}

// GroupsMembersAddTool returns the MCP tool definition for tg_groups_members_add.
func GroupsMembersAddTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_members_add",
		Description: "Add a user to a Telegram group",
		Annotations: idempotentAnnotations(),
	}
}

// GroupsMembersRemoveParams defines the parameters for the tg_groups_members_remove tool.
type GroupsMembersRemoveParams struct {
	Group string `json:"group" jsonschema:"@username, t.me/ link, or numeric ID"`
	User  string `json:"user"  jsonschema:"User ID or @username to remove"`
}

// GroupsMembersRemoveResult is the output of the tg_groups_members_remove tool.
type GroupsMembersRemoveResult struct {
	Output string `json:"output"`
}

// NewGroupsMembersRemoveHandler creates a handler for the tg_groups_members_remove tool.
func NewGroupsMembersRemoveHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsMembersRemoveParams, GroupsMembersRemoveResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsMembersRemoveParams,
	) (*mcp.CallToolResult, GroupsMembersRemoveResult, error) {
		if params.Group == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersRemoveResult{},
				validationErr(ErrGroupRequired)
		}

		if params.User == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersRemoveResult{},
				validationErr(ErrUserRequired)
		}

		groupPeer, err := client.ResolvePeer(ctx, params.Group)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersRemoveResult{},
				telegramErr("failed to resolve group peer", err)
		}

		userPeer, err := client.ResolvePeer(ctx, params.User)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersRemoveResult{},
				telegramErr("failed to resolve user peer", err)
		}

		err = client.RemoveGroupMember(ctx, groupPeer, userPeer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsMembersRemoveResult{},
				telegramErr("failed to remove member from group", err)
		}

		return nil, GroupsMembersRemoveResult{
			Output: fmt.Sprintf("Removed %s from %s", params.User, params.Group),
		}, nil
	}
}

// GroupsMembersRemoveTool returns the MCP tool definition for tg_groups_members_remove.
func GroupsMembersRemoveTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_members_remove",
		Description: "Remove a user from a Telegram group",
		Annotations: destructiveAnnotations(),
	}
}

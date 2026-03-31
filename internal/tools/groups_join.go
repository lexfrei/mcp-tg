package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsJoinParams defines the parameters for the tg_groups_join tool.
type GroupsJoinParams struct {
	Peer string `json:"peer" jsonschema:"Group ID, @username, or invite link"`
}

// GroupsJoinResult is the output of the tg_groups_join tool.
type GroupsJoinResult struct {
	Output string `json:"output"`
}

// NewGroupsJoinHandler creates a handler for the tg_groups_join tool.
func NewGroupsJoinHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsJoinParams, GroupsJoinResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsJoinParams,
	) (*mcp.CallToolResult, GroupsJoinResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsJoinResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsJoinResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.JoinGroup(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsJoinResult{},
				telegramErr("failed to join group", err)
		}

		return nil, GroupsJoinResult{
			Output: "Joined group " + params.Peer,
		}, nil
	}
}

// GroupsJoinTool returns the MCP tool definition for tg_groups_join.
func GroupsJoinTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_join",
		Description: "Join a Telegram group by ID, @username, or invite link",
		Annotations: idempotentAnnotations(),
	}
}

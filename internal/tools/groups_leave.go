package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsLeaveParams defines the parameters for the tg_groups_leave tool.
type GroupsLeaveParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// GroupsLeaveResult is the output of the tg_groups_leave tool.
type GroupsLeaveResult struct {
	Peer    string `json:"peer"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
}

// NewGroupsLeaveHandler creates a handler for the tg_groups_leave tool.
func NewGroupsLeaveHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsLeaveParams, GroupsLeaveResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsLeaveParams,
	) (*mcp.CallToolResult, GroupsLeaveResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsLeaveResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsLeaveResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.LeaveGroup(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsLeaveResult{},
				telegramErr("failed to leave group", err)
		}

		return nil, GroupsLeaveResult{
			Peer:    params.Peer,
			Success: true,
			Output:  "Left group " + params.Peer,
		}, nil
	}
}

// GroupsLeaveTool returns the MCP tool definition for tg_groups_leave.
func GroupsLeaveTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_leave",
		Description: "Leave a Telegram group or supergroup",
		Annotations: destructiveAnnotations(),
	}
}

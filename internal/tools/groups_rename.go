package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsRenameParams defines the parameters for the tg_groups_rename tool.
type GroupsRenameParams struct {
	Peer  string `json:"peer"  jsonschema:"Group ID or @username"`
	Title string `json:"title" jsonschema:"New group title"`
}

// GroupsRenameResult is the output of the tg_groups_rename tool.
type GroupsRenameResult struct {
	Peer     string `json:"peer"`
	OldTitle string `json:"oldTitle"`
	NewTitle string `json:"newTitle"`
	Output   string `json:"output"`
}

// NewGroupsRenameHandler creates a handler for the tg_groups_rename tool.
func NewGroupsRenameHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsRenameParams, GroupsRenameResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsRenameParams,
	) (*mcp.CallToolResult, GroupsRenameResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsRenameResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Title == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsRenameResult{},
				validationErr(ErrTitleRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsRenameResult{},
				telegramErr("failed to resolve group peer", err)
		}

		// Fetch old title for the response before renaming.
		oldTitle := params.Peer

		info, infoErr := client.GetGroupInfo(ctx, peer)
		if infoErr == nil && info != nil {
			oldTitle = info.Title
		}

		err = client.RenameGroup(ctx, peer, params.Title)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsRenameResult{},
				telegramErr("failed to rename group", err)
		}

		return nil, GroupsRenameResult{
			Peer:     params.Peer,
			OldTitle: oldTitle,
			NewTitle: params.Title,
			Output:   fmt.Sprintf("Renamed group %q to %q", oldTitle, params.Title),
		}, nil
	}
}

// GroupsRenameTool returns the MCP tool definition for tg_groups_rename.
func GroupsRenameTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_rename",
		Description: "Rename a Telegram group",
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsListParams defines the parameters for the tg_groups_list tool.
type GroupsListParams struct {
	Limit *int `json:"limit,omitempty" jsonschema:"Maximum number of groups to return (default 100)"`
}

// GroupsListResult is the output of the tg_groups_list tool.
type GroupsListResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewGroupsListHandler creates a handler for the tg_groups_list tool.
func NewGroupsListHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsListParams, GroupsListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsListParams,
	) (*mcp.CallToolResult, GroupsListResult, error) {
		opts := telegram.DialogOpts{Limit: deref(params.Limit)}

		dialogs, err := client.GetDialogs(ctx, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsListResult{},
				telegramErr("failed to list dialogs", err)
		}

		var buf strings.Builder

		count := 0

		for _, dlg := range dialogs {
			if !dlg.IsGroup {
				continue
			}

			fmt.Fprintf(&buf, "%s (peer: %s)\n", dlg.Title, formatPeer(dlg.Peer))

			count++
		}

		return nil, GroupsListResult{
			Count:  count,
			Output: buf.String(),
		}, nil
	}
}

// GroupsListTool returns the MCP tool definition for tg_groups_list.
func GroupsListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_list",
		Description: "List all Telegram groups from the dialog list",
		Annotations: readOnlyAnnotations(),
	}
}

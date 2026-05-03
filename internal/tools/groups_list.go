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
	Count   int          `json:"count"`
	HasMore bool         `json:"hasMore"`
	Groups  []DialogItem `json:"groups"`
	Output  string       `json:"output"`
}

// NewGroupsListHandler creates a handler for the tg_groups_list tool.
func NewGroupsListHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsListParams, GroupsListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsListParams,
	) (*mcp.CallToolResult, GroupsListResult, error) {
		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsListResult{},
				validationErr(limitErr)
		}

		dialogs, err := client.GetDialogs(ctx, telegram.DialogOpts{Limit: limit})
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsListResult{},
				telegramErr("failed to list dialogs", err)
		}

		var (
			buf    strings.Builder
			groups []DialogItem
		)

		for idx := range dialogs {
			if !dialogs[idx].IsGroup {
				continue
			}

			fmt.Fprintf(&buf, "%s (peer: %s)\n", dialogs[idx].Title, formatPeer(dialogs[idx].Peer))

			groups = append(groups, dialogToItem(&dialogs[idx]))
		}

		return nil, GroupsListResult{
			Count: len(groups),
			// hasMore is computed from the underlying dialog page size,
			// not the filtered groups count: a full dialog page may
			// contain more groups beyond it.
			HasMore: hasMorePage(len(dialogs), limit),
			Groups:  groups,
			Output:  buf.String(),
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

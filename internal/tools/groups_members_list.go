package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsMembersListParams defines parameters for tg_groups_members_list.
type GroupsMembersListParams struct {
	Peer   string  `json:"peer"             jsonschema:"@username, t.me/ link, or numeric ID"`
	Filter *string `json:"filter,omitempty" jsonschema:"recent, admins, banned, bots, or search query"`
	Limit  *int    `json:"limit,omitempty"  jsonschema:"Maximum results (default 100)"`
}

// GroupsMembersListResult is the output of tg_groups_members_list.
type GroupsMembersListResult struct {
	Count   int        `json:"count"`
	HasMore bool       `json:"hasMore"`
	Users   []UserItem `json:"users"`
	Output  string     `json:"output"`
}

// NewGroupsMembersListHandler creates a handler for tg_groups_members_list.
func NewGroupsMembersListHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsMembersListParams, GroupsMembersListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsMembersListParams,
	) (*mcp.CallToolResult, GroupsMembersListResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				GroupsMembersListResult{},
				validationErr(ErrPeerRequired)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true},
				GroupsMembersListResult{},
				validationErr(limitErr)
		}

		return fetchGroupMembers(ctx, client, params, limit)
	}
}

func fetchGroupMembers(
	ctx context.Context,
	client telegram.Client,
	params GroupsMembersListParams,
	limit int,
) (*mcp.CallToolResult, GroupsMembersListResult, error) {
	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			GroupsMembersListResult{},
			telegramErr("failed to resolve peer", err)
	}

	users, err := client.GetGroupMembers(
		ctx, peer, deref(params.Filter), limit,
	)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			GroupsMembersListResult{},
			telegramErr("failed to get group members", err)
	}

	var buf strings.Builder

	for idx := range users {
		fmt.Fprintf(
			&buf, "[%d] %s\n",
			users[idx].ID, formatUserName(&users[idx]),
		)
	}

	return nil, GroupsMembersListResult{
		Count:   len(users),
		HasMore: hasMorePage(len(users), limit),
		Users:   usersToItems(users),
		Output:  buf.String(),
	}, nil
}

// GroupsMembersListTool returns the tool definition.
func GroupsMembersListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_members_list",
		Description: "List members of a Telegram group or channel",
		Annotations: readOnlyAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsInfoParams defines the parameters for the tg_groups_info tool.
type GroupsInfoParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// GroupsInfoResult is the output of the tg_groups_info tool.
type GroupsInfoResult struct {
	Title        string `json:"title"`
	Username     string `json:"username"`
	About        string `json:"about"`
	MembersCount int    `json:"membersCount"`
	IsChannel    bool   `json:"isChannel"`
	IsSupergroup bool   `json:"isSupergroup"`
	IsForum      bool   `json:"isForum"`
	Output       string `json:"output"`
}

// NewGroupsInfoHandler creates a handler for the tg_groups_info tool.
func NewGroupsInfoHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsInfoParams, GroupsInfoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInfoParams,
	) (*mcp.CallToolResult, GroupsInfoResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInfoResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInfoResult{},
				telegramErr("failed to resolve peer", err)
		}

		info, err := client.GetGroupInfo(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInfoResult{},
				telegramErr("failed to get group info", err)
		}

		return nil, GroupsInfoResult{
			Title:        info.Title,
			Username:     info.Username,
			About:        info.About,
			MembersCount: info.MembersCount,
			IsChannel:    info.IsChannel,
			IsSupergroup: info.IsSupergroup,
			IsForum:      info.IsForum,
			Output: fmt.Sprintf(
				"%s (@%s) — %d members: %s",
				info.Title, info.Username, info.MembersCount, info.About,
			),
		}, nil
	}
}

// GroupsInfoTool returns the MCP tool definition for tg_groups_info.
func GroupsInfoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_info",
		Description: "Get detailed information about a Telegram group",
		Annotations: readOnlyAnnotations(),
	}
}

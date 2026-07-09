package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsInfoParams defines the parameters for the tg_groups_info tool.
type GroupsInfoParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// GroupsInfoResult is the output of the tg_groups_info tool.
//
// DefaultSendAs is the identity this account posts under here unless a
// send tool names another. It is absent when the account posts as itself.
type GroupsInfoResult struct {
	Title         string      `json:"title"`
	Username      string      `json:"username"`
	About         string      `json:"about"`
	MembersCount  int         `json:"membersCount"`
	IsChannel     bool        `json:"isChannel"`
	IsSupergroup  bool        `json:"isSupergroup"`
	IsForum       bool        `json:"isForum"`
	DefaultSendAs *SendAsItem `json:"defaultSendAs,omitempty"`
	Output        string      `json:"output"`
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
			Title:         info.Title,
			Username:      info.Username,
			About:         info.About,
			MembersCount:  info.MembersCount,
			IsChannel:     info.IsChannel,
			IsSupergroup:  info.IsSupergroup,
			IsForum:       info.IsForum,
			DefaultSendAs: defaultSendAsItem(info.DefaultSendAs),
			Output:        formatGroupInfo(info),
		}, nil
	}
}

func formatGroupInfo(info *telegram.GroupInfo) string {
	var out strings.Builder

	out.WriteString(formatPeerRef(info.Title, info.Username, info.Peer))

	if info.MembersCount > 0 {
		fmt.Fprintf(&out, " — %d members", info.MembersCount)
	}

	if info.About != "" {
		out.WriteString(": ")
		out.WriteString(info.About)
	}

	if info.DefaultSendAs != nil {
		identity := *info.DefaultSendAs
		fmt.Fprintf(&out, "\nposts as: %s",
			formatPeerRef(identity.Name, identity.Username, identity.Peer))
	}

	return out.String()
}

// defaultSendAsItem renders the group's default identity in the same
// shape tg_chats_get_send_as lists, so its peer string can be handed
// straight back as a sendAs argument.
func defaultSendAsItem(option *telegram.SendAsOption) *SendAsItem {
	if option == nil {
		return nil
	}

	item := sendAsItems([]telegram.SendAsOption{*option})[0]

	return &item
}

// GroupsInfoTool returns the MCP tool definition for tg_groups_info.
func GroupsInfoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_info",
		Description: "Get detailed information about a Telegram group",
		Annotations: readOnlyAnnotations(),
	}
}

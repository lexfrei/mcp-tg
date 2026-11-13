package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxAdminLogLimit = 100

// ErrInviteLinkRequired is returned when an invite tool gets no link.
var ErrInviteLinkRequired = errors.New("link is required")

// GroupsBanParams defines the parameters for the tg_groups_ban tool.
type GroupsBanParams struct {
	Group     string `json:"group"               jsonschema:"Channel or supergroup: @username, t.me/ link, or numeric ID"`
	User      string `json:"user"                jsonschema:"User to ban or unban: @username or numeric ID"`
	UntilDate int    `json:"untilDate,omitempty" jsonschema:"Unix time the ban ends (default: forever)"`
	Unban     *bool  `json:"unban,omitempty"     jsonschema:"Lift the ban instead (default false)"`
}

// GroupsBanResult is the output of the tg_groups_ban tool.
type GroupsBanResult struct {
	Output string `json:"output"`
}

// NewGroupsBanHandler creates a handler for the tg_groups_ban tool.
func NewGroupsBanHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsBanParams, GroupsBanResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsBanParams,
	) (*mcp.CallToolResult, GroupsBanResult, error) {
		target, err := resolveGroupMember(ctx, client, params.Group, params.User)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsBanResult{}, err
		}

		unban := deref(params.Unban)

		err = client.BanMember(ctx, target.group, target.user, params.UntilDate, unban)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsBanResult{}, telegramErr("failed to change the ban", err)
		}

		output := "Banned " + params.User
		if unban {
			output = "Unbanned " + params.User
		} else if params.UntilDate > 0 {
			output += " until " + formatTimestamp(params.UntilDate)
		}

		return nil, GroupsBanResult{Output: output}, nil
	}
}

// GroupsBanTool returns the MCP tool definition for tg_groups_ban.
func GroupsBanTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_groups_ban",
		Description: "Ban a user from a Telegram channel or supergroup (removing them and blocking rejoining), " +
			"or lift the ban",
		Annotations: destructiveAnnotations(),
	}
}

// groupMember is a resolved group + user pair.
type groupMember struct {
	group telegram.InputPeer
	user  telegram.InputPeer
}

func resolveGroupMember(ctx context.Context, client telegram.Client, groupRef, userRef string) (groupMember, error) {
	if groupRef == "" {
		return groupMember{}, validationErr(ErrPeerRequired)
	}

	if userRef == "" {
		return groupMember{}, validationErr(ErrUserRequired)
	}

	group, err := client.ResolvePeer(ctx, groupRef)
	if err != nil {
		return groupMember{}, telegramErr("failed to resolve group", err)
	}

	user, err := client.ResolvePeer(ctx, userRef)
	if err != nil {
		return groupMember{}, telegramErr("failed to resolve user", err)
	}

	return groupMember{group: group, user: user}, nil
}

// GroupsInviteCheckParams defines the parameters for the tg_groups_invite_check tool.
type GroupsInviteCheckParams struct {
	Link string `json:"link" jsonschema:"Invite link: t.me/+hash, t.me/joinchat/hash, or the hash"`
}

// GroupsInviteCheckResult is the output of the tg_groups_invite_check tool.
type GroupsInviteCheckResult struct {
	telegram.InviteInfo

	Output string `json:"output"`
}

// NewGroupsInviteCheckHandler creates a handler for the tg_groups_invite_check tool.
func NewGroupsInviteCheckHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsInviteCheckParams, GroupsInviteCheckResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteCheckParams,
	) (*mcp.CallToolResult, GroupsInviteCheckResult, error) {
		if params.Link == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteCheckResult{}, validationErr(ErrInviteLinkRequired)
		}

		info, err := client.CheckInvite(ctx, params.Link)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteCheckResult{},
				telegramErr("failed to check the invite link", err)
		}

		return nil, GroupsInviteCheckResult{InviteInfo: *info, Output: formatInviteInfo(info)}, nil
	}
}

// GroupsInviteCheckTool returns the MCP tool definition for tg_groups_invite_check.
func GroupsInviteCheckTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_invite_check",
		Description: "Preview the chat behind a Telegram invite link without joining it",
		Annotations: readOnlyAnnotations(),
	}
}

func formatInviteInfo(info *telegram.InviteInfo) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%s (%s)", info.Title, info.Type)

	// Telegram leaves the count out of the reply for a chat already joined.
	if info.ParticipantsCount > 0 {
		fmt.Fprintf(&buf, ", %d member(s)", info.ParticipantsCount)
	}

	if info.AlreadyJoined {
		buf.WriteString(", already joined")
	}

	if info.RequestNeeded {
		buf.WriteString(", joining needs admin approval")
	}

	if info.About != "" {
		buf.WriteString("\n" + info.About)
	}

	return buf.String()
}

// GroupsJoinInviteParams defines the parameters for the tg_groups_join_invite tool.
type GroupsJoinInviteParams struct {
	Link string `json:"link" jsonschema:"Invite link: t.me/+hash, t.me/joinchat/hash, or the hash"`
}

// GroupsJoinInviteResult is the output of the tg_groups_join_invite tool.
type GroupsJoinInviteResult struct {
	Chat   telegram.PeerInfo `json:"chat"`
	Output string            `json:"output"`
}

// NewGroupsJoinInviteHandler creates a handler for the tg_groups_join_invite tool.
func NewGroupsJoinInviteHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsJoinInviteParams, GroupsJoinInviteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsJoinInviteParams,
	) (*mcp.CallToolResult, GroupsJoinInviteResult, error) {
		if params.Link == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsJoinInviteResult{}, validationErr(ErrInviteLinkRequired)
		}

		chat, err := client.JoinByInvite(ctx, params.Link)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsJoinInviteResult{},
				telegramErr("failed to join by invite link", err)
		}

		return nil, GroupsJoinInviteResult{
			Chat:   *chat,
			Output: "Joined " + formatPeerRef(chat.Title, chat.Username, chat.Peer),
		}, nil
	}
}

// GroupsJoinInviteTool returns the MCP tool definition for tg_groups_join_invite.
func GroupsJoinInviteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_join_invite",
		Description: "Join a Telegram group or channel by its private invite link",
		Annotations: idempotentAnnotations(),
	}
}

// GroupsUpgradeParams defines the parameters for the tg_groups_upgrade tool.
type GroupsUpgradeParams struct {
	Peer string `json:"peer" jsonschema:"Basic group: @username, t.me/ link, or numeric ID"`
}

// GroupsUpgradeResult is the output of the tg_groups_upgrade tool.
type GroupsUpgradeResult struct {
	Supergroup telegram.PeerInfo `json:"supergroup"`
	Output     string            `json:"output"`
}

// NewGroupsUpgradeHandler creates a handler for the tg_groups_upgrade tool.
func NewGroupsUpgradeHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsUpgradeParams, GroupsUpgradeResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsUpgradeParams,
	) (*mcp.CallToolResult, GroupsUpgradeResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsUpgradeResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsUpgradeResult{}, telegramErr("failed to resolve peer", err)
		}

		channel, err := client.UpgradeToSupergroup(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsUpgradeResult{},
				telegramErr("failed to upgrade the group", err)
		}

		return nil, GroupsUpgradeResult{
			Supergroup: *channel,
			Output:     "Upgraded to supergroup " + formatPeerRef(channel.Title, channel.Username, channel.Peer),
		}, nil
	}
}

// GroupsUpgradeTool returns the MCP tool definition for tg_groups_upgrade.
func GroupsUpgradeTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_groups_upgrade",
		Description: "Upgrade a basic Telegram group to a supergroup. The supergroup gets a new ID; " +
			"the old group stops accepting messages",
		Annotations: writeAnnotations(),
	}
}

// GroupsSetForumParams defines the parameters for the tg_groups_set_forum tool.
type GroupsSetForumParams struct {
	Peer    string `json:"peer"    jsonschema:"Supergroup: @username, t.me/ link, or numeric ID"`
	Enabled bool   `json:"enabled" jsonschema:"true to turn forum topics on, false to turn them off"`
}

// GroupsSetForumResult is the output of the tg_groups_set_forum tool.
type GroupsSetForumResult struct {
	Output string `json:"output"`
}

// NewGroupsSetForumHandler creates a handler for the tg_groups_set_forum tool.
func NewGroupsSetForumHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsSetForumParams, GroupsSetForumResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsSetForumParams,
	) (*mcp.CallToolResult, GroupsSetForumResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsSetForumResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsSetForumResult{}, telegramErr("failed to resolve peer", err)
		}

		err = client.SetForum(ctx, peer, params.Enabled)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsSetForumResult{},
				telegramErr("failed to change forum mode", err)
		}

		output := "Forum topics turned off"
		if params.Enabled {
			output = "Forum topics turned on"
		}

		return nil, GroupsSetForumResult{Output: output}, nil
	}
}

// GroupsSetForumTool returns the MCP tool definition for tg_groups_set_forum.
func GroupsSetForumTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_set_forum",
		Description: "Turn forum topics on or off in a Telegram supergroup",
		Annotations: idempotentAnnotations(),
	}
}

// GroupsAdminLogParams defines the parameters for the tg_groups_admin_log tool.
type GroupsAdminLogParams struct {
	Peer  string `json:"peer"            jsonschema:"Channel or supergroup: @username, t.me/ link, or numeric ID"`
	Query string `json:"query,omitempty" jsonschema:"Only events matching this text"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum events to return, up to 100 (default 50)"`
	MaxID int64  `json:"maxId,omitempty" jsonschema:"Only events older than this event ID, for paging"`
}

// GroupsAdminLogResult is the output of the tg_groups_admin_log tool.
type GroupsAdminLogResult struct {
	Count  int                      `json:"count"`
	Events []telegram.AdminLogEvent `json:"events"`
	Output string                   `json:"output"`
}

// NewGroupsAdminLogHandler creates a handler for the tg_groups_admin_log tool.
func NewGroupsAdminLogHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsAdminLogParams, GroupsAdminLogResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsAdminLogParams,
	) (*mcp.CallToolResult, GroupsAdminLogResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsAdminLogResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsAdminLogResult{}, telegramErr("failed to resolve peer", err)
		}

		limit := params.Limit
		if limit <= 0 || limit > maxAdminLogLimit {
			limit = maxAdminLogLimit / 2
		}

		events, err := client.GetAdminLog(ctx, peer, telegram.AdminLogOpts{Query: params.Query, Limit: limit, MaxID: params.MaxID})
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsAdminLogResult{},
				telegramErr("failed to read the admin log", err)
		}

		return nil, GroupsAdminLogResult{Count: len(events), Events: events, Output: formatAdminLog(events)}, nil
	}
}

// GroupsAdminLogTool returns the MCP tool definition for tg_groups_admin_log.
func GroupsAdminLogTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_admin_log",
		Description: "Read the recent admin actions (bans, edits, deletions, settings) in a Telegram channel or supergroup",
		Annotations: readOnlyAnnotations(),
	}
}

func formatAdminLog(events []telegram.AdminLogEvent) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%d event(s)", len(events))

	for _, event := range events {
		fmt.Fprintf(&buf, "\n[%d] %s %s: %s", event.ID, formatTimestamp(event.Date),
			formatPeerRef("", "", telegram.InputPeer{Type: telegram.PeerUser, ID: event.UserID}), event.Action)

		if event.Detail != "" {
			buf.WriteString(" — " + event.Detail)
		}
	}

	return buf.String()
}

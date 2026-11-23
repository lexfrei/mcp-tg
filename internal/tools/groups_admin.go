package tools

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxAdminLogLimit = 100

// ErrAdminLogLimit is returned when tg_groups_admin_log is asked for more
// events than Telegram returns in one page.
var ErrAdminLogLimit = errors.New("limit must not exceed 100")

// ErrAdminLogMaxID is returned when maxId is not a decimal event ID.
var ErrAdminLogMaxID = errors.New("maxId must be an event ID as a decimal string")

// ErrUntilDateOutOfRange is returned when tg_groups_ban's untilDate does not
// fit the 32-bit field Telegram encodes it in, where it would wrap.
var ErrUntilDateOutOfRange = errors.New("untilDate must be a Unix time between 0 and 2147483647")

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
		// until_date is a 32-bit int on the wire: a larger value would wrap
		// into a different (possibly past) date.
		if params.UntilDate < 0 || params.UntilDate > math.MaxInt32 {
			return &mcp.CallToolResult{IsError: true}, GroupsBanResult{}, validationErr(ErrUntilDateOutOfRange)
		}

		target, err := resolveGroupMember(ctx, client, params.Group, params.User)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsBanResult{}, err
		}

		unban := deref(params.Unban)

		err = client.BanMember(ctx, target.group, target.user, params.UntilDate, unban)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsBanResult{}, groupErr("failed to change the ban", err)
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
	Chat        telegram.PeerInfo `json:"chat,omitzero"`
	RequestSent bool              `json:"requestSent,omitempty"`
	Output      string            `json:"output"`
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
		if alreadyDone(err, "USER_ALREADY_PARTICIPANT") {
			return nil, alreadyInvited(ctx, client, params.Link), nil
		}

		// gotd documents INVITE_REQUEST_SENT as "you have successfully
		// requested to join": the request is filed, and a retry would file a
		// second one, so it is a result, not a failure.
		if alreadyDone(err, "INVITE_REQUEST_SENT") {
			return nil, GroupsJoinInviteResult{
				RequestSent: true,
				Output:      "A join request for " + params.Link + " is pending; an admin of the chat has to approve it",
			}, nil
		}

		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsJoinInviteResult{},
				groupErr("failed to join by invite link", err)
		}

		return nil, GroupsJoinInviteResult{
			Chat:   *chat,
			Output: "Joined " + formatPeerRef(chat.Title, chat.Username, chat.Peer),
		}, nil
	}
}

// alreadyInvited answers a join of a chat the account is already in. The
// invite is read back to name the chat; when that read fails too, the reply
// still states the membership, naming the link instead.
func alreadyInvited(ctx context.Context, client telegram.Client, link string) GroupsJoinInviteResult {
	info, err := client.CheckInvite(ctx, link)
	if err != nil || info == nil {
		return GroupsJoinInviteResult{Output: "Already a member of the chat behind " + link}
	}

	chat := telegram.PeerInfo{Peer: info.Peer, Title: info.Title, Type: info.Type}

	return GroupsJoinInviteResult{
		Chat:   chat,
		Output: "Already a member of " + formatPeerRef(chat.Title, chat.Username, chat.Peer),
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
				groupErr("failed to upgrade the group", err)
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
		if err != nil && !alreadyDone(err, "CHAT_NOT_MODIFIED") {
			return &mcp.CallToolResult{IsError: true}, GroupsSetForumResult{},
				groupErr("failed to change forum mode", err)
		}

		// Worded as the state, which holds whether this call changed it or a
		// repeat found it already so (CHAT_NOT_MODIFIED).
		output := "Forum topics are off in " + params.Peer
		if params.Enabled {
			output = "Forum topics are on in " + params.Peer
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
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum events to return, 1-100 (default 50)"`
	MaxID string `json:"maxId,omitempty" jsonschema:"Only events older than this event ID (a decimal string), for paging"`
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

		opts, err := adminLogOpts(&params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsAdminLogResult{}, validationErr(err)
		}

		events, err := client.GetAdminLog(ctx, peer, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsAdminLogResult{},
				groupErr("failed to read the admin log", err)
		}

		return nil, GroupsAdminLogResult{Count: len(events), Events: events, Output: formatAdminLog(events)}, nil
	}
}

// adminLogOpts validates the paging parameters. maxId is a decimal string,
// not a number: event IDs are 64-bit, and go-sdk round-trips JSON numbers
// through float64, which would round one above 2^53 and make a page skip or
// repeat events.
func adminLogOpts(params *GroupsAdminLogParams) (telegram.AdminLogOpts, error) {
	opts := telegram.AdminLogOpts{Query: params.Query, Limit: params.Limit}

	switch {
	case params.Limit < 0:
		return opts, ErrNegativeLimit
	case params.Limit > maxAdminLogLimit:
		return opts, ErrAdminLogLimit
	case params.Limit == 0:
		opts.Limit = maxAdminLogLimit / 2
	}

	if params.MaxID != "" {
		maxID, err := strconv.ParseInt(params.MaxID, 10, 64)
		if err != nil || maxID < 0 {
			return opts, ErrAdminLogMaxID
		}

		opts.MaxID = maxID
	}

	return opts, nil
}

// groupErr reports a failed group operation. The wrapper's channel-only and
// basic-group-only refusals are about the input, raised before any request,
// so they are validation errors, not Telegram failures. ErrNoChatInReply is
// not a failure of the operation: the upgrade or join went through and only
// the reply left the chat out, so it gets no "failed to" prefix — that text
// is what has to stop a retry of an upgrade that cannot be repeated.
func groupErr(msg string, err error) error {
	if errors.Is(err, telegram.ErrChannelOnly) || errors.Is(err, telegram.ErrBasicGroupOnly) ||
		errors.Is(err, telegram.ErrNotAGroup) {
		return validationErr(err)
	}

	if errors.Is(err, telegram.ErrNoChatInReply) {
		//nolint:wrapcheck // Mark adds a sentinel category; the wrapper's text already says what happened.
		return errors.Mark(err, ErrTelegram)
	}

	return telegramErr(msg, err)
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
		fmt.Fprintf(&buf, "\n[%s] %s %s: %s", event.ID, formatTimestamp(event.Date),
			formatPeerRef("", "", telegram.InputPeer{Type: telegram.PeerUser, ID: event.UserID}), event.Action)

		if event.Detail != "" {
			buf.WriteString(" — " + event.Detail)
		}
	}

	return buf.String()
}

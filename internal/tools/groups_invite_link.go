package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsInviteLinkGetParams defines the parameters for the tg_groups_invite_link_get tool.
type GroupsInviteLinkGetParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// GroupsInviteLinkGetResult is the output of the tg_groups_invite_link_get tool.
type GroupsInviteLinkGetResult struct {
	Link   string `json:"link"`
	Output string `json:"output"`
}

// NewGroupsInviteLinkGetHandler creates a handler for the tg_groups_invite_link_get tool.
func NewGroupsInviteLinkGetHandler(client telegram.Client) mcp.ToolHandlerFor[GroupsInviteLinkGetParams, GroupsInviteLinkGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteLinkGetParams,
	) (*mcp.CallToolResult, GroupsInviteLinkGetResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkGetResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkGetResult{},
				telegramErr("failed to resolve peer", err)
		}

		link, err := client.GetInviteLink(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkGetResult{},
				telegramErr("failed to get invite link", err)
		}

		return nil, GroupsInviteLinkGetResult{
			Link:   link,
			Output: "Invite link: " + link,
		}, nil
	}
}

// GroupsInviteLinkGetTool returns the MCP tool definition for tg_groups_invite_link_get.
func GroupsInviteLinkGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_groups_invite_link_get",
		Description: "Get a chat's primary invite link. Reads the link Telegram already " +
			"holds for the chat; it never creates one, and it is visible only to " +
			"administrators with the right to invite users",
		Annotations: readOnlyAnnotations(),
	}
}

// GroupsInviteLinkRevokeParams defines the parameters for the tg_groups_invite_link_revoke tool.
type GroupsInviteLinkRevokeParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
	Link string `json:"link" jsonschema:"Invite link to revoke"`
}

// GroupsInviteLinkRevokeResult is the output of the tg_groups_invite_link_revoke
// tool. NewLink is set only when the revoked link was the chat's primary one:
// Telegram mints a replacement on the spot, and its absence from the reply
// would leave the caller believing the chat has no link at all.
//
// Only the call that performed the replacement is told about it, so a caller
// that retries a revoke whose answer it lost gets an empty NewLink for a chat
// that does have a new primary link. tg_groups_invite_link_get is the
// authoritative read for that.
type GroupsInviteLinkRevokeResult struct {
	Peer    string `json:"peer"`
	NewLink string `json:"newLink,omitempty"`
	Output  string `json:"output"`
}

// NewGroupsInviteLinkRevokeHandler creates a handler for the tg_groups_invite_link_revoke tool.
func NewGroupsInviteLinkRevokeHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsInviteLinkRevokeParams, GroupsInviteLinkRevokeResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteLinkRevokeParams,
	) (*mcp.CallToolResult, GroupsInviteLinkRevokeResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Link == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				validationErr(ErrLinkRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				telegramErr("failed to resolve peer", err)
		}

		replacement, err := client.RevokeInviteLink(ctx, peer, params.Link)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkRevokeResult{},
				telegramErr("failed to revoke invite link", err)
		}

		output := "Revoked invite link for " + params.Peer
		if replacement != "" {
			output += "; Telegram replaced the chat's primary link with " + replacement
		}

		return nil, GroupsInviteLinkRevokeResult{
			Peer:    params.Peer,
			NewLink: replacement,
			Output:  output,
		}, nil
	}
}

// GroupsInviteLinkRevokeTool returns the MCP tool definition for tg_groups_invite_link_revoke.
func GroupsInviteLinkRevokeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_invite_link_revoke",
		Description: "Revoke an invite link for a Telegram group",
		Annotations: idempotentAnnotations(),
	}
}

// GroupsInviteLinkCreateParams defines the parameters for the tg_groups_invite_link_create tool.
type GroupsInviteLinkCreateParams struct {
	Peer       string  `json:"peer"                 jsonschema:"@username, t.me/ link, or numeric ID"`
	Title      *string `json:"title,omitempty"      jsonschema:"Label shown only to administrators; people joining never see it"`
	ExpireDate *int    `json:"expireDate,omitempty" jsonschema:"Unix timestamp when the link stops working; omit for no expiry"`
	UsageLimit *int    `json:"usageLimit,omitempty" jsonschema:"How many people may join through this link; omit for no limit"`
	//nolint:lll // the Bot API caveat is the whole reason this parameter needs a description.
	RequestNeeded *bool `json:"requestNeeded,omitempty" jsonschema:"Joiners must be approved by an administrator. Telegram's Bot API documents that usageLimit cannot be combined with this; the MTProto documentation does not, so the server decides — read the echoed usageLimit, or the error, to see what it did"`
}

// GroupsInviteLinkCreateResult is the output of the tg_groups_invite_link_create tool.
type GroupsInviteLinkCreateResult struct {
	Invite InviteLinkItem `json:"invite"`
	Output string         `json:"output"`
}

// NewGroupsInviteLinkCreateHandler creates a handler for the tg_groups_invite_link_create tool.
func NewGroupsInviteLinkCreateHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsInviteLinkCreateParams, GroupsInviteLinkCreateResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteLinkCreateParams,
	) (*mcp.CallToolResult, GroupsInviteLinkCreateResult, error) {
		err := validateInviteLinkCreate(params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkCreateResult{}, validationErr(err)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkCreateResult{},
				telegramErr("failed to resolve peer", err)
		}

		invite, err := client.CreateInviteLink(ctx, peer, telegram.InviteLinkOpts{
			Title:         deref(params.Title),
			ExpireDate:    deref(params.ExpireDate),
			UsageLimit:    deref(params.UsageLimit),
			RequestNeeded: deref(params.RequestNeeded),
		})
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkCreateResult{},
				telegramErr("failed to create invite link", err)
		}

		return nil, GroupsInviteLinkCreateResult{
			Invite: *invite,
			Output: "Created invite link: " + invite.Link,
		}, nil
	}
}

func validateInviteLinkCreate(params GroupsInviteLinkCreateParams) error {
	if params.Peer == "" {
		return ErrPeerRequired
	}

	err := validateLimit(deref(params.UsageLimit))
	if err != nil {
		return err
	}

	return validateDateRange(0, deref(params.ExpireDate))
}

// GroupsInviteLinkCreateTool returns the MCP tool definition for tg_groups_invite_link_create.
func GroupsInviteLinkCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_groups_invite_link_create",
		Description: "Create an ADDITIONAL invite link for a chat. It does not touch the chat's " +
			"primary link, which tg_groups_invite_link_get reads",
		Annotations: writeAnnotations(),
	}
}

// GroupsInviteLinkListParams defines the parameters for the tg_groups_invite_link_list tool.
type GroupsInviteLinkListParams struct {
	Peer    string `json:"peer"              jsonschema:"@username, t.me/ link, or numeric ID"`
	Revoked *bool  `json:"revoked,omitempty" jsonschema:"List revoked links instead of active ones"`
	Limit   *int   `json:"limit,omitempty"   jsonschema:"Maximum number of links to return (default 100)"`
}

// GroupsInviteLinkListResult is the output of the tg_groups_invite_link_list tool.
//
// Total is the server's own count across all pages, which is how a caller
// learns the page did not cover everything. Telegram does offer a cursor
// (offset_date plus offset_link, which travel together); this tool does not
// expose it, because links are created by hand rather than accumulated, so
// raising Limit is the knob until somebody reports a chat that needs paging.
type GroupsInviteLinkListResult struct {
	Count  int              `json:"count"`
	Total  int              `json:"total"`
	Links  []InviteLinkItem `json:"links"`
	Output string           `json:"output"`
}

// NewGroupsInviteLinkListHandler creates a handler for the tg_groups_invite_link_list tool.
func NewGroupsInviteLinkListHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsInviteLinkListParams, GroupsInviteLinkListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsInviteLinkListParams,
	) (*mcp.CallToolResult, GroupsInviteLinkListResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkListResult{},
				validationErr(ErrPeerRequired)
		}

		err := validateLimit(deref(params.Limit))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkListResult{}, validationErr(err)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkListResult{},
				telegramErr("failed to resolve peer", err)
		}

		links, total, err := client.ListInviteLinks(ctx, peer, deref(params.Revoked), deref(params.Limit))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, GroupsInviteLinkListResult{},
				telegramErr("failed to list invite links", err)
		}

		return nil, GroupsInviteLinkListResult{
			Count:  len(links),
			Total:  total,
			Links:  links,
			Output: formatInviteLinks(links, total),
		}, nil
	}
}

// GroupsInviteLinkListTool returns the MCP tool definition for tg_groups_invite_link_list.
func GroupsInviteLinkListTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_groups_invite_link_list",
		Description: "List the invite links this account created in a chat. Telegram scopes the " +
			"listing to one administrator, so links created by anyone else do not appear; the " +
			"chat's primary link shows up only if this account is what created it",
		Annotations: readOnlyAnnotations(),
	}
}

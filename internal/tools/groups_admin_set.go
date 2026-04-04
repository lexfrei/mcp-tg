package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsAdminSetParams defines parameters for tg_groups_admin_set.
type GroupsAdminSetParams struct {
	Group        string  `json:"group"                    jsonschema:"@username, t.me/ link, or numeric ID"`
	User         string  `json:"user"                     jsonschema:"User ID or @username"`
	Rank         *string `json:"rank,omitempty"           jsonschema:"Admin title/rank"`
	ChangeInfo   *bool   `json:"changeInfo,omitempty"     jsonschema:"Allow changing group info"`
	PostMessages *bool   `json:"postMessages,omitempty"   jsonschema:"Allow posting in channels"`
	EditMessages *bool   `json:"editMessages,omitempty"   jsonschema:"Allow editing others messages"`
	DeleteMsgs   *bool   `json:"deleteMessages,omitempty" jsonschema:"Allow deleting messages"`
	BanUsers     *bool   `json:"banUsers,omitempty"       jsonschema:"Allow banning users"`
	InviteUsers  *bool   `json:"inviteUsers,omitempty"    jsonschema:"Allow inviting users"`
	PinMessages  *bool   `json:"pinMessages,omitempty"    jsonschema:"Allow pinning messages"`
	ManageCall   *bool   `json:"manageCall,omitempty"     jsonschema:"Allow managing calls"`
	AddAdmins    *bool   `json:"addAdmins,omitempty"      jsonschema:"Allow adding admins"`
	ManageTopics *bool   `json:"manageTopics,omitempty"   jsonschema:"Allow managing topics"`
}

// GroupsAdminSetResult is the output of tg_groups_admin_set.
type GroupsAdminSetResult struct {
	Output string `json:"output"`
}

// NewGroupsAdminSetHandler creates a handler for tg_groups_admin_set.
func NewGroupsAdminSetHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsAdminSetParams, GroupsAdminSetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsAdminSetParams,
	) (*mcp.CallToolResult, GroupsAdminSetResult, error) {
		if params.Group == "" {
			return &mcp.CallToolResult{IsError: true},
				GroupsAdminSetResult{},
				validationErr(ErrGroupRequired)
		}

		if params.User == "" {
			return &mcp.CallToolResult{IsError: true},
				GroupsAdminSetResult{},
				validationErr(ErrUserRequired)
		}

		return doSetAdmin(ctx, client, &params)
	}
}

func doSetAdmin(
	ctx context.Context,
	client telegram.Client,
	params *GroupsAdminSetParams,
) (*mcp.CallToolResult, GroupsAdminSetResult, error) {
	groupPeer, err := client.ResolvePeer(ctx, params.Group)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			GroupsAdminSetResult{},
			telegramErr("failed to resolve group", err)
	}

	userPeer, err := client.ResolvePeer(ctx, params.User)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			GroupsAdminSetResult{},
			telegramErr("failed to resolve user", err)
	}

	rights := buildAdminRights(params)

	err = client.SetAdmin(
		ctx, groupPeer, userPeer, rights, deref(params.Rank),
	)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			GroupsAdminSetResult{},
			telegramErr("failed to set admin rights", err)
	}

	return nil, GroupsAdminSetResult{
		Output: fmt.Sprintf(
			"Set admin rights for %s in %s",
			params.User, params.Group,
		),
	}, nil
}

func buildAdminRights(
	params *GroupsAdminSetParams,
) telegram.AdminRights {
	return telegram.AdminRights{
		ChangeInfo:   deref(params.ChangeInfo),
		PostMessages: deref(params.PostMessages),
		EditMessages: deref(params.EditMessages),
		DeleteMsgs:   deref(params.DeleteMsgs),
		BanUsers:     deref(params.BanUsers),
		InviteUsers:  deref(params.InviteUsers),
		PinMessages:  deref(params.PinMessages),
		ManageCall:   deref(params.ManageCall),
		AddAdmins:    deref(params.AddAdmins),
		ManageTopics: deref(params.ManageTopics),
	}
}

// GroupsAdminSetTool returns the tool definition for tg_groups_admin_set.
func GroupsAdminSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_admin_set",
		Description: "Set administrator rights for a user in a Telegram group",
		Annotations: destructiveAnnotations(),
	}
}

package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatsPermissionsParams defines the parameters for the tg_chats_set_permissions tool.
type ChatsPermissionsParams struct {
	Peer         string `json:"peer"                   jsonschema:"@username, t.me/ link, or numeric ID"`
	SendMessages *bool  `json:"sendMessages,omitempty" jsonschema:"Allow sending text messages"`
	SendMedia    *bool  `json:"sendMedia,omitempty"    jsonschema:"Allow sending media (photos, videos, etc.)"`
	SendStickers *bool  `json:"sendStickers,omitempty" jsonschema:"Allow sending stickers"`
	SendGifs     *bool  `json:"sendGifs,omitempty"     jsonschema:"Allow sending GIFs"`
	SendPolls    *bool  `json:"sendPolls,omitempty"    jsonschema:"Allow sending polls"`
	AddMembers   *bool  `json:"addMembers,omitempty"   jsonschema:"Allow adding new members"`
	PinMessages  *bool  `json:"pinMessages,omitempty"  jsonschema:"Allow pinning messages"`
	ChangeInfo   *bool  `json:"changeInfo,omitempty"   jsonschema:"Allow changing chat info"`
}

// ChatsPermissionsResult is the output of the tg_chats_set_permissions tool.
type ChatsPermissionsResult struct {
	Output string `json:"output"`
}

// NewChatsPermissionsHandler creates a handler for the tg_chats_set_permissions tool.
func NewChatsPermissionsHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ChatsPermissionsParams, ChatsPermissionsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsPermissionsParams,
	) (*mcp.CallToolResult, ChatsPermissionsResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsPermissionsResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsPermissionsResult{},
				telegramErr("failed to resolve peer", err)
		}

		perms := buildPermissions(&params)

		err = client.SetChatPermissions(ctx, peer, perms)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsPermissionsResult{},
				telegramErr("failed to set chat permissions", err)
		}

		return nil, ChatsPermissionsResult{
			Output: "Updated chat permissions for " + params.Peer,
		}, nil
	}
}

// boolPtrOrTrue returns the value of a bool pointer or true if nil.
func boolPtrOrTrue(ptr *bool) bool {
	if ptr == nil {
		return true
	}

	return *ptr
}

// buildPermissions constructs ChatPermissions from optional param bools,
// defaulting each to true when not explicitly provided.
func buildPermissions(params *ChatsPermissionsParams) telegram.ChatPermissions {
	return telegram.ChatPermissions{
		SendMessages: boolPtrOrTrue(params.SendMessages),
		SendMedia:    boolPtrOrTrue(params.SendMedia),
		SendStickers: boolPtrOrTrue(params.SendStickers),
		SendGifs:     boolPtrOrTrue(params.SendGifs),
		SendPolls:    boolPtrOrTrue(params.SendPolls),
		AddMembers:   boolPtrOrTrue(params.AddMembers),
		PinMessages:  boolPtrOrTrue(params.PinMessages),
		ChangeInfo:   boolPtrOrTrue(params.ChangeInfo),
	}
}

// ChatsPermissionsTool returns the MCP tool definition for tg_chats_set_permissions.
func ChatsPermissionsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_set_permissions",
		Description: "Set default chat permissions for a Telegram group or channel",
		Annotations: idempotentAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatsAdminsParams defines the parameters for the tg_chats_get_admins tool.
type ChatsAdminsParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// ChatsAdminsResult is the output of the tg_chats_get_admins tool.
type ChatsAdminsResult struct {
	Count  int        `json:"count"`
	Admins []UserItem `json:"admins"`
	Output string     `json:"output"`
}

// NewChatsAdminsHandler creates a handler for the tg_chats_get_admins tool.
func NewChatsAdminsHandler(client telegram.Client) mcp.ToolHandlerFor[ChatsAdminsParams, ChatsAdminsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsAdminsParams,
	) (*mcp.CallToolResult, ChatsAdminsResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsAdminsResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsAdminsResult{},
				telegramErr("failed to resolve peer", err)
		}

		admins, err := client.GetChatAdmins(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsAdminsResult{},
				telegramErr("failed to get chat admins", err)
		}

		var buf strings.Builder

		for idx := range admins {
			fmt.Fprintf(&buf, "[%d] %s\n", admins[idx].ID, formatUserName(&admins[idx]))
		}

		return nil, ChatsAdminsResult{
			Count:  len(admins),
			Admins: usersToItems(admins),
			Output: buf.String(),
		}, nil
	}
}

// ChatsAdminsTool returns the MCP tool definition for tg_chats_get_admins.
func ChatsAdminsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_get_admins",
		Description: "Get the list of administrators in a Telegram channel or supergroup",
		Annotations: readOnlyAnnotations(),
	}
}

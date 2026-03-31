package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UsersCommonChatsParams defines the parameters for the tg_users_get_common_chats tool.
type UsersCommonChatsParams struct {
	Peer string `json:"peer" jsonschema:"User ID or @username"`
}

// UsersCommonChatsResult is the output of the tg_users_get_common_chats tool.
type UsersCommonChatsResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewUsersCommonChatsHandler creates a handler for the tg_users_get_common_chats tool.
func NewUsersCommonChatsHandler(client telegram.Client) mcp.ToolHandlerFor[UsersCommonChatsParams, UsersCommonChatsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params UsersCommonChatsParams,
	) (*mcp.CallToolResult, UsersCommonChatsResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, UsersCommonChatsResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersCommonChatsResult{},
				telegramErr("failed to resolve peer", err)
		}

		chats, err := client.GetCommonChats(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersCommonChatsResult{},
				telegramErr("failed to get common chats", err)
		}

		var buf strings.Builder

		for _, chat := range chats {
			fmt.Fprintf(&buf, "%s (@%s) [%s]\n", chat.Title, chat.Username, chat.Type)
		}

		return nil, UsersCommonChatsResult{
			Count:  len(chats),
			Output: buf.String(),
		}, nil
	}
}

// UsersCommonChatsTool returns the MCP tool definition for tg_users_get_common_chats.
func UsersCommonChatsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_users_get_common_chats",
		Description: "Get chats in common with a Telegram user",
	}
}

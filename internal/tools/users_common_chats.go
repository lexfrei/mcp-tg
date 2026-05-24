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
//
// Chats carries one PeerRefItem per shared chat with {id, type, name,
// username}, same shape every other peer-listing tool exposes, so a
// caller doesn't need to regex-parse Output.
type UsersCommonChatsResult struct {
	Count  int           `json:"count"`
	Chats  []PeerRefItem `json:"chats"`
	Output string        `json:"output"`
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

		var (
			buf   strings.Builder
			items = make([]PeerRefItem, len(chats))
		)

		for idx, chat := range chats {
			items[idx] = PeerRefItem{
				ID:       chat.Peer.ID,
				Type:     participantTypeLabel(chat.Peer.Type),
				Name:     chat.Title,
				Username: chat.Username,
			}
			fmt.Fprintf(&buf, "%s\n", formatPeerRef(chat.Title, chat.Username, chat.Peer))
		}

		return nil, UsersCommonChatsResult{
			Count:  len(chats),
			Chats:  items,
			Output: buf.String(),
		}, nil
	}
}

// UsersCommonChatsTool returns the MCP tool definition for tg_users_get_common_chats.
func UsersCommonChatsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_users_get_common_chats",
		Description: "Get chats in common with a Telegram user",
		Annotations: readOnlyAnnotations(),
	}
}

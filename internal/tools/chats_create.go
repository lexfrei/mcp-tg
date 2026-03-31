package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatsCreateParams defines the parameters for the tg_chats_create tool.
type ChatsCreateParams struct {
	Title     string   `json:"title"               jsonschema:"Title for the new chat or channel"`
	IsChannel *bool    `json:"isChannel,omitempty" jsonschema:"Create a channel instead of a group"`
	UserPeers []string `json:"userPeers,omitempty" jsonschema:"User IDs or @usernames to invite"`
}

// ChatsCreateResult is the output of the tg_chats_create tool.
type ChatsCreateResult struct {
	Title  string `json:"title"`
	Type   string `json:"type"`
	Output string `json:"output"`
}

// NewChatsCreateHandler creates a handler for the tg_chats_create tool.
func NewChatsCreateHandler(client telegram.Client) mcp.ToolHandlerFor[ChatsCreateParams, ChatsCreateResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsCreateParams,
	) (*mcp.CallToolResult, ChatsCreateResult, error) {
		if params.Title == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsCreateResult{},
				validationErr(ErrTitleRequired)
		}

		users, err := resolveUserPeers(ctx, client, params.UserPeers)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsCreateResult{},
				telegramErr("failed to resolve user peers", err)
		}

		isChannel := deref(params.IsChannel)

		info, err := client.CreateChat(ctx, params.Title, users, isChannel)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsCreateResult{},
				telegramErr("failed to create chat", err)
		}

		chatType := "group"
		if isChannel {
			chatType = "channel"
		}

		title := params.Title
		if info != nil && info.Title != "" {
			title = info.Title
		}

		return nil, ChatsCreateResult{
			Title:  title,
			Type:   chatType,
			Output: fmt.Sprintf("Created %s %q with %d invited user(s)", chatType, title, len(users)),
		}, nil
	}
}

// resolveUserPeers resolves a list of user identifier strings to InputPeer slices.
func resolveUserPeers(ctx context.Context, client telegram.Client, identifiers []string) ([]telegram.InputPeer, error) {
	peers := make([]telegram.InputPeer, 0, len(identifiers))

	for _, ident := range identifiers {
		peer, err := client.ResolvePeer(ctx, ident)
		if err != nil {
			return nil, fmt.Errorf("resolving %q: %w", ident, err)
		}

		peers = append(peers, peer)
	}

	return peers, nil
}

// ChatsCreateTool returns the MCP tool definition for tg_chats_create.
func ChatsCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_create",
		Description: "Create a new Telegram group or channel",
		Annotations: idempotentAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesGetReactionsParams defines parameters for tg_messages_get_reactions.
type MessagesGetReactionsParams struct {
	Peer      string `json:"peer"            jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId"       jsonschema:"Message ID to get reactions for"`
	Limit     *int   `json:"limit,omitempty" jsonschema:"Maximum results (default 100)"`
}

// ReactionUserItem is a structured reaction entry for JSON results.
//
// Name and Username mirror the shape used by every other peer-bearing
// JSON entry (sender, forward-author, participant) so a consumer can
// render a reactor with the same "Display Name [@username]" identifier.
//
// Type says which id space UserID belongs to: a channel reacts whenever
// it is the chat's default send-as identity, and a channel id collides
// with an unrelated user id.
type ReactionUserItem struct {
	UserID   int64  `json:"userId"`
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Emoji    string `json:"emoji"`
}

// MessagesGetReactionsResult is the output of tg_messages_get_reactions.
type MessagesGetReactionsResult struct {
	Count     int                `json:"count"`
	HasMore   bool               `json:"hasMore"`
	Reactions []ReactionUserItem `json:"reactions"`
	Output    string             `json:"output"`
}

// NewMessagesGetReactionsHandler creates a handler for tg_messages_get_reactions.
func NewMessagesGetReactionsHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesGetReactionsParams, MessagesGetReactionsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesGetReactionsParams,
	) (*mcp.CallToolResult, MessagesGetReactionsResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				MessagesGetReactionsResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true},
				MessagesGetReactionsResult{},
				validationErr(ErrMessageIDRequired)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesGetReactionsResult{},
				validationErr(limitErr)
		}

		return fetchReactions(ctx, client, params, limit)
	}
}

func fetchReactions(
	ctx context.Context,
	client telegram.Client,
	params MessagesGetReactionsParams,
	limit int,
) (*mcp.CallToolResult, MessagesGetReactionsResult, error) {
	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			MessagesGetReactionsResult{},
			telegramErr("failed to resolve peer", err)
	}

	reactions, err := client.GetReactions(
		ctx, peer, params.MessageID, limit,
	)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			MessagesGetReactionsResult{},
			telegramErr("failed to get reactions", err)
	}

	return nil, MessagesGetReactionsResult{
		Count:     len(reactions),
		HasMore:   hasMorePage(len(reactions), limit),
		Reactions: reactionUsersToItems(reactions),
		Output:    formatReactionUsers(reactions),
	}, nil
}

func reactionUsersToItems(
	reactions []telegram.ReactionUser,
) []ReactionUserItem {
	items := make([]ReactionUserItem, len(reactions))

	for idx := range reactions {
		items[idx] = ReactionUserItem{
			UserID:   reactions[idx].UserID,
			Type:     participantTypeLabel(reactions[idx].PeerType),
			Name:     reactions[idx].Name,
			Username: reactions[idx].Username,
			Emoji:    reactions[idx].Emoji,
		}
	}

	return items
}

func formatReactionUsers(reactions []telegram.ReactionUser) string {
	var buf strings.Builder

	for idx := range reactions {
		ref := formatPeerRef(
			reactions[idx].Name,
			reactions[idx].Username,
			telegram.InputPeer{Type: reactions[idx].PeerType, ID: reactions[idx].UserID},
		)
		fmt.Fprintf(&buf, "%s %s\n", reactions[idx].Emoji, ref)
	}

	return buf.String()
}

// MessagesGetReactionsTool returns the tool definition.
func MessagesGetReactionsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_get_reactions",
		Description: "Get users who reacted to a Telegram message",
		Annotations: readOnlyAnnotations(),
	}
}

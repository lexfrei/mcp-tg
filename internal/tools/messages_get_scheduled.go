package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesGetScheduledParams defines parameters for tg_messages_get_scheduled.
type MessagesGetScheduledParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// MessagesGetScheduledResult is the output of tg_messages_get_scheduled.
type MessagesGetScheduledResult struct {
	Count    int           `json:"count"`
	Messages []MessageItem `json:"messages"`
	Output   string        `json:"output"`
}

// NewMessagesGetScheduledHandler creates a handler for tg_messages_get_scheduled.
func NewMessagesGetScheduledHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesGetScheduledParams, MessagesGetScheduledResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesGetScheduledParams,
	) (*mcp.CallToolResult, MessagesGetScheduledResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				MessagesGetScheduledResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesGetScheduledResult{},
				telegramErr("failed to resolve peer", err)
		}

		msgs, err := client.GetScheduledMessages(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesGetScheduledResult{},
				telegramErr("failed to get scheduled messages", err)
		}

		return nil, MessagesGetScheduledResult{
			Count:    len(msgs),
			Messages: messagesToItems(msgs),
			Output:   fmt.Sprintf("%d scheduled message(s)", len(msgs)),
		}, nil
	}
}

// MessagesGetScheduledTool returns the tool definition for tg_messages_get_scheduled.
func MessagesGetScheduledTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_get_scheduled",
		Description: "List scheduled messages for a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

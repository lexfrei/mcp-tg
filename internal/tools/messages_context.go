package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultContextRadius = 10

// MessagesContextParams defines the parameters for the tg_messages_context tool.
type MessagesContextParams struct {
	Peer      string `json:"peer"             jsonschema:"Chat ID or @username"`
	MessageID int    `json:"messageId"        jsonschema:"Message ID to get context around"`
	Radius    *int   `json:"radius,omitempty" jsonschema:"Number of messages before and after (default 10)"`
}

// MessagesContextResult is the output of the tg_messages_context tool.
type MessagesContextResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewMessagesContextHandler creates a handler for the tg_messages_context tool.
func NewMessagesContextHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesContextParams, MessagesContextResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesContextParams,
	) (*mcp.CallToolResult, MessagesContextResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				telegramErr("failed to resolve peer", err)
		}

		radius := deref(params.Radius)
		if radius <= 0 {
			radius = defaultContextRadius
		}

		opts := telegram.HistoryOpts{
			Limit:    radius*2 + 1,
			OffsetID: params.MessageID + radius,
		}

		msgs, _, err := client.GetHistory(ctx, peer, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				telegramErr("failed to get message context", err)
		}

		var buf strings.Builder

		for idx := range msgs {
			marker := "  "
			if msgs[idx].ID == params.MessageID {
				marker = "> "
			}

			fmt.Fprintf(&buf, "%s%s\n", marker, formatMessage(&msgs[idx]))
		}

		return nil, MessagesContextResult{
			Count:  len(msgs),
			Output: buf.String(),
		}, nil
	}
}

// MessagesContextTool returns the MCP tool definition for tg_messages_context.
func MessagesContextTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_context",
		Description: "Get messages around a specific message in a Telegram chat",
	}
}

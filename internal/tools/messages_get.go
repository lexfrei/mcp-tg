package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesGetParams defines the parameters for the tg_messages_get tool.
type MessagesGetParams struct {
	Peer string `json:"peer" jsonschema:"Chat ID or @username"`
	IDs  []int  `json:"ids"  jsonschema:"Message IDs to retrieve"`
}

// MessagesGetResult is the output of the tg_messages_get tool.
type MessagesGetResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewMessagesGetHandler creates a handler for the tg_messages_get tool.
func NewMessagesGetHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesGetParams, MessagesGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesGetParams,
	) (*mcp.CallToolResult, MessagesGetResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesGetResult{},
				validationErr(ErrPeerRequired)
		}

		if len(params.IDs) == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesGetResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetResult{},
				telegramErr("failed to resolve peer", err)
		}

		msgs, err := client.GetMessages(ctx, peer, params.IDs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetResult{},
				telegramErr("failed to get messages", err)
		}

		var buf strings.Builder

		for idx := range msgs {
			fmt.Fprintln(&buf, formatMessage(&msgs[idx]))
		}

		return nil, MessagesGetResult{
			Count:  len(msgs),
			Output: buf.String(),
		}, nil
	}
}

// MessagesGetTool returns the MCP tool definition for tg_messages_get.
func MessagesGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_get",
		Description: "Get specific messages by their IDs from a Telegram chat",
	}
}

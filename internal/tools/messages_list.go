package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesListParams defines the parameters for the tg_messages_list tool.
type MessagesListParams struct {
	Peer     string `json:"peer"               jsonschema:"@username, t.me/ link, or numeric ID"`
	Limit    *int   `json:"limit,omitempty"    jsonschema:"Max messages to return (default 100)"`
	OffsetID *int   `json:"offsetId,omitempty" jsonschema:"Message ID to start from"`
}

// MessagesListResult is the output of the tg_messages_list tool.
type MessagesListResult struct {
	Count  int    `json:"count"`
	Total  int    `json:"total"`
	Output string `json:"output"`
}

// NewMessagesListHandler creates a handler for the tg_messages_list tool.
func NewMessagesListHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesListParams, MessagesListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesListParams,
	) (*mcp.CallToolResult, MessagesListResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				telegramErr("failed to resolve peer", err)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				validationErr(limitErr)
		}

		opts := telegram.HistoryOpts{
			Limit:    limit,
			OffsetID: deref(params.OffsetID),
		}

		msgs, total, err := client.GetHistory(ctx, peer, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				telegramErr("failed to get messages", err)
		}

		var buf strings.Builder

		for idx := range msgs {
			fmt.Fprintln(&buf, formatMessage(&msgs[idx]))
		}

		return nil, MessagesListResult{
			Count:  len(msgs),
			Total:  total,
			Output: buf.String(),
		}, nil
	}
}

// MessagesListTool returns the MCP tool definition for tg_messages_list.
func MessagesListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_list",
		Description: "List messages in a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

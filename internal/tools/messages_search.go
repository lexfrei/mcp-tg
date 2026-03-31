package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesSearchParams defines the parameters for the tg_messages_search tool.
type MessagesSearchParams struct {
	Peer  string `json:"peer"            jsonschema:"Chat ID or @username"`
	Query string `json:"query"           jsonschema:"Search query"`
	Limit *int   `json:"limit,omitempty" jsonschema:"Max results (default 100)"`
}

// MessagesSearchResult is the output of the tg_messages_search tool.
type MessagesSearchResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewMessagesSearchHandler creates a handler for the tg_messages_search tool.
func NewMessagesSearchHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesSearchParams, MessagesSearchResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesSearchParams,
	) (*mcp.CallToolResult, MessagesSearchResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				validationErr(ErrQueryRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				telegramErr("failed to resolve peer", err)
		}

		opts := telegram.SearchOpts{Limit: deref(params.Limit)}

		msgs, err := client.SearchMessages(ctx, peer, params.Query, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				telegramErr("failed to search messages", err)
		}

		var buf strings.Builder

		for idx := range msgs {
			fmt.Fprintln(&buf, formatMessage(&msgs[idx]))
		}

		return nil, MessagesSearchResult{
			Count:  len(msgs),
			Output: buf.String(),
		}, nil
	}
}

// MessagesSearchTool returns the MCP tool definition for tg_messages_search.
func MessagesSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_search",
		Description: "Search for messages in a Telegram chat",
	}
}

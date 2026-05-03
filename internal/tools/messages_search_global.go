package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesSearchGlobalParams defines parameters for tg_messages_search_global.
//
// Note: resolveReplies is intentionally not offered here. Global search
// returns messages from arbitrary peers, each needing its own access
// hash to fetch the parent; a single batched lookup is not possible.
// Structural replyTo metadata is still populated.
type MessagesSearchGlobalParams struct {
	Query string `json:"query"           jsonschema:"Search query"`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum results (default 100)"`
}

// MessagesSearchGlobalResult is the output of tg_messages_search_global.
type MessagesSearchGlobalResult struct {
	Count    int           `json:"count"`
	HasMore  bool          `json:"hasMore"`
	Messages []MessageItem `json:"messages"`
	Output   string        `json:"output"`
}

// NewMessagesSearchGlobalHandler creates a handler for tg_messages_search_global.
func NewMessagesSearchGlobalHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesSearchGlobalParams, MessagesSearchGlobalResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesSearchGlobalParams,
	) (*mcp.CallToolResult, MessagesSearchGlobalResult, error) {
		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true},
				MessagesSearchGlobalResult{},
				validationErr(ErrQueryRequired)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesSearchGlobalResult{},
				validationErr(limitErr)
		}

		msgs, err := client.SearchGlobal(ctx, params.Query, limit)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesSearchGlobalResult{},
				telegramErr("failed to search global", err)
		}

		return nil, MessagesSearchGlobalResult{
			Count:    len(msgs),
			HasMore:  hasMorePage(len(msgs), limit),
			Messages: messagesToItems(msgs),
			Output:   fmt.Sprintf("Found %d message(s)", len(msgs)),
		}, nil
	}
}

// MessagesSearchGlobalTool returns the tool definition.
func MessagesSearchGlobalTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_search_global",
		Description: "Search messages across all Telegram chats",
		Annotations: readOnlyAnnotations(),
	}
}

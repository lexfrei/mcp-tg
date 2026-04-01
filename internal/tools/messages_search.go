package tools

import (
	"context"

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
		req *mcp.CallToolRequest,
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

		token := req.Params.GetProgressToken()
		notifyProgress(ctx, req.Session, token, 0, 1, "Resolving peer")

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				telegramErr("failed to resolve peer", err)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				validationErr(limitErr)
		}

		notifyProgress(ctx, req.Session, token, 0, 1, "Searching messages")

		result, err := executeSearch(ctx, client, peer, params.Query, limit)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{}, err
		}

		return nil, result, nil
	}
}

func executeSearch(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, query string, limit int,
) (MessagesSearchResult, error) {
	msgs, err := client.SearchMessages(ctx, peer, query, telegram.SearchOpts{Limit: limit})
	if err != nil {
		return MessagesSearchResult{}, telegramErr("failed to search messages", err)
	}

	return MessagesSearchResult{
		Count:  len(msgs),
		Output: formatMessages(msgs),
	}, nil
}

// MessagesSearchTool returns the MCP tool definition for tg_messages_search.
func MessagesSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_search",
		Description: "Search for messages in a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

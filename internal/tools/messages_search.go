package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesSearchParams defines the parameters for the tg_messages_search tool.
type MessagesSearchParams struct {
	Peer           string `json:"peer"                     jsonschema:"@username, t.me/ link, or numeric ID"`
	Query          string `json:"query"                    jsonschema:"Search query"`
	Limit          *int   `json:"limit,omitempty"          jsonschema:"Max results (default 100)"`
	OffsetID       *int   `json:"offsetId,omitempty"       jsonschema:"Message ID to start search from (for pagination)"`
	ResolveReplies *bool  `json:"resolveReplies,omitempty" jsonschema:"Fetch parent message text for replies (default false, extra API call)"`
}

// MessagesSearchResult is the output of the tg_messages_search tool.
type MessagesSearchResult struct {
	Count        int               `json:"count"`
	HasMore      bool              `json:"hasMore"`
	Participants []ParticipantItem `json:"participants,omitempty"`
	Messages     []MessageItem     `json:"messages"`
	Output       string            `json:"output"`
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

		result, msgs, err := executeSearch(ctx, client, peer, params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{}, err
		}

		if deref(params.ResolveReplies) {
			resolveReplyParents(ctx, client, peer, result.Messages, msgs)
		}

		return nil, result, nil
	}
}

func executeSearch(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, params MessagesSearchParams,
) (MessagesSearchResult, []telegram.Message, error) {
	opts := telegram.SearchOpts{
		Limit:    deref(params.Limit),
		OffsetID: deref(params.OffsetID),
	}

	msgs, err := client.SearchMessages(ctx, peer, params.Query, opts)
	if err != nil {
		return MessagesSearchResult{}, nil, telegramErr("failed to search messages", err)
	}

	return MessagesSearchResult{
		Count:        len(msgs),
		HasMore:      hasMorePage(len(msgs), deref(params.Limit)),
		Participants: participantsFromMessages(msgs),
		Messages:     messagesToItems(msgs),
		Output:       formatMessages(msgs),
	}, msgs, nil
}

// MessagesSearchTool returns the MCP tool definition for tg_messages_search.
func MessagesSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_search",
		Description: "Search for messages in a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

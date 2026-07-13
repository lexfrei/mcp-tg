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
	TopicID        *int   `json:"topicId,omitempty"        jsonschema:"Forum topic ID to search within"`
	From           string `json:"from,omitempty"           jsonschema:"Only messages from this sender (@username, t.me/ link, or numeric ID)"`
	Filter         string `json:"filter,omitempty"         jsonschema:"Server-side kind filter (photos, video, document, url, voice, ...)"`
	MinDate        *int   `json:"minDate,omitempty"        jsonschema:"Only messages sent after this unix timestamp"`
	MaxDate        *int   `json:"maxDate,omitempty"        jsonschema:"Only messages sent before this unix timestamp"`
	Limit          *int   `json:"limit,omitempty"          jsonschema:"Max results (default 100)"`
	OffsetID       *int   `json:"offsetId,omitempty"       jsonschema:"Message ID to start search from (for pagination)"`
	ResolveReplies *bool  `json:"resolveReplies,omitempty" jsonschema:"Fetch parent message text for replies (default false, extra API call)"`
}

// MessagesSearchResult is the output of the tg_messages_search tool.
type MessagesSearchResult struct {
	Count        int               `json:"count"`
	Total        int               `json:"total"`
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
		validErr := validateSearchParams(&params)
		if validErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				validationErr(validErr)
		}

		token := req.Params.GetProgressToken()
		notifyProgress(ctx, req.Session, token, 0, 1, "Resolving peer")

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{},
				telegramErr("failed to resolve peer", err)
		}

		opts, err := searchOptsFromParams(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{}, err
		}

		notifyProgress(ctx, req.Session, token, 0, 1, "Searching messages")

		result, msgs, err := executeSearch(ctx, client, peer, params.Query, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSearchResult{}, err
		}

		if deref(params.ResolveReplies) {
			resolveReplyParents(ctx, client, peer, result.Messages, msgs)
		}

		return nil, result, nil
	}
}

// validateSearchParams runs every request-shape check that needs no
// network round-trip, so a malformed call fails before any RPC.
func validateSearchParams(params *MessagesSearchParams) error {
	if params.Peer == "" {
		return ErrPeerRequired
	}

	if params.Query == "" {
		return ErrQueryRequired
	}

	limitErr := validateLimit(deref(params.Limit))
	if limitErr != nil {
		return limitErr
	}

	if params.Filter != "" && !telegram.IsSearchFilter(params.Filter) {
		return ErrUnknownMessageFilter
	}

	return validateDateRange(deref(params.MinDate), deref(params.MaxDate))
}

// searchOptsFromParams threads the tool parameters into SearchOpts,
// resolving the optional sender filter into a concrete peer.
func searchOptsFromParams(
	ctx context.Context, client telegram.Client, params *MessagesSearchParams,
) (telegram.SearchOpts, error) {
	fromPeer, err := resolveOptionalPeer(ctx, client, params.From, "from")
	if err != nil {
		return telegram.SearchOpts{}, err
	}

	return telegram.SearchOpts{
		Limit:    deref(params.Limit),
		OffsetID: deref(params.OffsetID),
		TopicID:  deref(params.TopicID),
		FromID:   fromPeer,
		Filter:   params.Filter,
		MinDate:  deref(params.MinDate),
		MaxDate:  deref(params.MaxDate),
	}, nil
}

func executeSearch(
	ctx context.Context, client telegram.Client,
	peer telegram.InputPeer, query string, opts telegram.SearchOpts,
) (MessagesSearchResult, []telegram.Message, error) {
	msgs, total, err := client.SearchMessages(ctx, peer, query, opts)
	if err != nil {
		return MessagesSearchResult{}, nil, telegramErr("failed to search messages", err)
	}

	return MessagesSearchResult{
		Count:        len(msgs),
		Total:        total,
		HasMore:      hasMorePage(len(msgs), opts.Limit),
		Participants: participantsFromMessages(msgs),
		Messages:     messagesToItems(msgs),
		Output:       formatMessages(msgs),
	}, msgs, nil
}

// MessagesSearchTool returns the MCP tool definition for tg_messages_search.
func MessagesSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_search",
		Description: "Search for messages in a Telegram chat, optionally scoped to a forum topic, " +
			"a sender, a media kind, or a date range",
		Annotations: readOnlyAnnotations(),
	}
}

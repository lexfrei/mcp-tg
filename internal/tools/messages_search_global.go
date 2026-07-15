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
//
// offsetRate/offsetId/offsetPeer form one compound pagination cursor:
// to fetch the next page, copy the previous result's nextRate,
// nextOffsetId and nextOffsetPeer into them verbatim.
type MessagesSearchGlobalParams struct {
	Query      string `json:"query,omitempty"      jsonschema:"Search query (optional when filter is set)"`
	Filter     string `json:"filter,omitempty"     jsonschema:"Server-side kind filter (photos, video, document, url, voice, ...)"`
	MinDate    *int   `json:"minDate,omitempty"    jsonschema:"Only messages sent after this unix timestamp"`
	MaxDate    *int   `json:"maxDate,omitempty"    jsonschema:"Only messages sent before this unix timestamp"`
	Scope      string `json:"scope,omitempty"      jsonschema:"Restrict to one dialog kind: users, groups, or channels"`
	Limit      *int   `json:"limit,omitempty"      jsonschema:"Maximum results (default 100)"`
	OffsetRate *int   `json:"offsetRate,omitempty" jsonschema:"Pagination cursor: nextRate from the previous page"`
	OffsetID   *int   `json:"offsetId,omitempty"   jsonschema:"Pagination cursor: nextOffsetId from the previous page"`
	OffsetPeer string `json:"offsetPeer,omitempty" jsonschema:"Pagination cursor: nextOffsetPeer from the previous page"`
	Format     string `json:"format,omitempty"     jsonschema:"Output shape: full (default), json (messages only), text (output only)"`
}

// MessagesSearchGlobalResult is the output of tg_messages_search_global.
//
// nextRate/nextOffsetId/nextOffsetPeer are the ready-made cursor for
// the next page: pass them back as offsetRate/offsetId/offsetPeer.
// They exist because messages[].peerId is a structured object, not the
// bot-style string offsetPeer expects.
type MessagesSearchGlobalResult struct {
	Count          int           `json:"count"`
	Total          int           `json:"total"`
	HasMore        bool          `json:"hasMore"`
	NextRate       int           `json:"nextRate,omitempty"`
	NextOffsetID   int           `json:"nextOffsetId,omitempty"`
	NextOffsetPeer string        `json:"nextOffsetPeer,omitempty"`
	Messages       []MessageItem `json:"messages,omitempty"`
	Output         string        `json:"output,omitempty"`
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
		validErr := validateSearchGlobalParams(&params)
		if validErr != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesSearchGlobalResult{},
				validationErr(validErr)
		}

		opts, err := searchGlobalOptsFromParams(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesSearchGlobalResult{}, err
		}

		page, err := client.SearchGlobal(ctx, params.Query, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesSearchGlobalResult{},
				telegramErr("failed to search global", err)
		}

		result := MessagesSearchGlobalResult{
			Count:    len(page.Messages),
			Total:    page.Total,
			Messages: messagesToItems(page.Messages),
			Output:   fmt.Sprintf("Found %d of %d message(s)", len(page.Messages), page.Total),
		}

		// hasMore and the cursor are one atomic signal: the three
		// cursor fields travel together (partial cursors fail this
		// tool's own validation), and advertising a next page the
		// caller cannot address would strand the pagination. A page
		// that yields no complete cursor — no next_rate, no messages,
		// or a privacy-hidden last peer — is therefore terminal.
		if last, ok := nextPageCursor(page); ok {
			result.HasMore = true
			result.NextRate = page.NextRate
			result.NextOffsetID = last.ID
			result.NextOffsetPeer = formatPeer(last.PeerID)
		}

		result.Messages = messagesForFormat(params.Format, result.Messages)
		result.Output = outputForFormat(params.Format, result.Output)

		return nil, result, nil
	}
}

// validateSearchGlobalParams runs every request-shape check that needs
// no network round-trip, so a malformed call fails before any RPC.
func validateSearchGlobalParams(params *MessagesSearchGlobalParams) error {
	if params.Query == "" && params.Filter == "" {
		return ErrQueryOrFilterRequired
	}

	limitErr := validateLimit(deref(params.Limit))
	if limitErr != nil {
		return limitErr
	}

	if params.Filter != "" && !telegram.IsSearchFilter(params.Filter) {
		return ErrUnknownMessageFilter
	}

	if params.Scope != "" && !telegram.IsSearchScope(params.Scope) {
		return ErrUnknownSearchScope
	}

	cursorErr := validateGlobalCursor(params)
	if cursorErr != nil {
		return cursorErr
	}

	return validateDateRange(deref(params.MinDate), deref(params.MaxDate))
}

// nextPageCursor returns the message anchoring the next-page cursor,
// and whether a complete cursor can be built at all.
func nextPageCursor(page telegram.SearchGlobalPage) (telegram.Message, bool) {
	if page.NextRate <= 0 || len(page.Messages) == 0 {
		return telegram.Message{}, false
	}

	last := page.Messages[len(page.Messages)-1]

	return last, last.PeerID.ID != 0
}

// validateGlobalCursor rejects a partial pagination cursor: the three
// fields travel together, and the server silently accepts a subset
// while returning a skewed page instead of an error.
func validateGlobalCursor(params *MessagesSearchGlobalParams) error {
	hasRate := deref(params.OffsetRate) != 0
	hasID := deref(params.OffsetID) != 0
	hasPeer := params.OffsetPeer != ""

	if (hasRate || hasID || hasPeer) && (!hasRate || !hasID || !hasPeer) {
		return ErrPartialCursor
	}

	return nil
}

// searchGlobalOptsFromParams threads the tool parameters into
// SearchGlobalOpts, resolving the cursor's peer reference when a
// continuation is requested.
func searchGlobalOptsFromParams(
	ctx context.Context, client telegram.Client, params *MessagesSearchGlobalParams,
) (*telegram.SearchGlobalOpts, error) {
	offsetPeer, err := resolveOptionalPeer(ctx, client, params.OffsetPeer, "offsetPeer")
	if err != nil {
		return nil, err
	}

	// A resolved cursor peer without an access hash cannot go on the
	// wire (basic groups excepted — they have none by design). Typical
	// after a restart cleared the cache the previous page had seeded.
	if offsetPeer != nil && offsetPeer.Type != telegram.PeerChat && offsetPeer.AccessHash == 0 {
		return nil, validationErr(ErrOffsetPeerUnresolved)
	}

	return &telegram.SearchGlobalOpts{
		Limit:      deref(params.Limit),
		Filter:     params.Filter,
		MinDate:    deref(params.MinDate),
		MaxDate:    deref(params.MaxDate),
		Scope:      params.Scope,
		OffsetRate: deref(params.OffsetRate),
		OffsetID:   deref(params.OffsetID),
		OffsetPeer: offsetPeer,
	}, nil
}

// MessagesSearchGlobalTool returns the tool definition.
func MessagesSearchGlobalTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_search_global",
		Description: "Search messages across all Telegram chats, optionally filtered by kind, " +
			"date range, or dialog scope",
		InputSchema: inputSchemaWithEnum[MessagesSearchGlobalParams]("format", formatEnum()),
		Annotations: readOnlyAnnotations(),
	}
}

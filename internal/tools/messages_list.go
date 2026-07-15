package tools

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesListParams defines the parameters for the tg_messages_list tool.
type MessagesListParams struct {
	Peer           string `json:"peer"                     jsonschema:"@username, t.me/ link, or numeric ID"`
	TopicID        *int   `json:"topicId,omitempty"        jsonschema:"Forum topic ID to filter messages"`
	Limit          *int   `json:"limit,omitempty"          jsonschema:"Max messages to return (default 100)"`
	OffsetID       *int   `json:"offsetId,omitempty"       jsonschema:"Message ID to start from"`
	Type           string `json:"type,omitempty"           jsonschema:"Optional message type filter"`
	ResolveReplies *bool  `json:"resolveReplies,omitempty" jsonschema:"Fetch parent message text for replies (default false, extra API call)"`
}

// MessagesListResult is the output of the tg_messages_list tool.
type MessagesListResult struct {
	Count        int               `json:"count"`
	Total        int               `json:"total"`
	HasMore      bool              `json:"hasMore"`
	Participants []ParticipantItem `json:"participants,omitempty"`
	Messages     []MessageItem     `json:"messages"`
	Output       string            `json:"output"`
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

		limitErr := validateMessagesListLimit(deref(params.Limit))
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				validationErr(limitErr)
		}

		if params.Type != "" && !telegram.IsMessageType(params.Type) {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				validationErr(ErrUnknownMessageType)
		}

		result, msgs, err := fetchMessages(ctx, client, peer, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{}, err
		}

		if deref(params.ResolveReplies) {
			resolveReplyParents(ctx, client, peer, result.Messages, msgs)
		}

		return nil, result, nil
	}
}

func fetchMessages(
	ctx context.Context, client telegram.Client,
	peer telegram.InputPeer, params *MessagesListParams,
) (MessagesListResult, []telegram.Message, error) {
	if params.Type != "" {
		return fetchMessagesByType(ctx, client, peer, params)
	}

	return fetchMessagePage(ctx, client, peer, params, telegram.HistoryOpts{
		Limit:    deref(params.Limit),
		OffsetID: deref(params.OffsetID),
	})
}

func fetchMessagePage(
	ctx context.Context, client telegram.Client,
	peer telegram.InputPeer, params *MessagesListParams, opts telegram.HistoryOpts,
) (MessagesListResult, []telegram.Message, error) {
	topicID := deref(params.TopicID)

	var (
		msgs    []telegram.Message
		total   int
		hasMore bool
		err     error
	)

	if topicID > 0 {
		msgs, total, hasMore, err = client.GetTopicMessages(ctx, peer, topicID, opts)
	} else {
		msgs, total, hasMore, err = client.GetHistory(ctx, peer, opts)
	}

	if err != nil {
		return MessagesListResult{}, nil, telegramErr("failed to get messages", err)
	}

	return MessagesListResult{
		Count: len(msgs),
		Total: total,
		// The wrapper's paginator reports this directly: it stays true
		// when the scan cap stops the walk with the limit unfilled, which
		// a len-vs-limit compare would misreport as "no more".
		HasMore:      hasMore,
		Participants: participantsFromMessages(msgs),
		Messages:     messagesToItems(msgs),
		Output:       formatMessages(msgs),
	}, msgs, nil
}

const maxTypeFilterScan = 5000

// maxMessagesListLimit caps how many messages a single tg_messages_list
// call fetches. Requests above one server page are paged internally, so
// the cap bounds the number of round-trips one call can trigger.
const maxMessagesListLimit = 1000

// validateMessagesListLimit rejects a negative limit and one above the
// auto-pagination cap.
func validateMessagesListLimit(limit int) error {
	if limit < 0 {
		return ErrNegativeLimit
	}

	if limit > maxMessagesListLimit {
		return ErrLimitTooLarge
	}

	return nil
}

func fetchMessagesByType(
	ctx context.Context, client telegram.Client,
	peer telegram.InputPeer, params *MessagesListParams,
) (MessagesListResult, []telegram.Message, error) {
	effectiveLimit := messageListEffectiveLimit(deref(params.Limit))
	state := newMessageTypeFilterState(deref(params.OffsetID))

	for state.shouldFetch(effectiveLimit) {
		page, err := fetchMessageTypeFilterPage(ctx, client, peer, params, state.offsetID)
		if err != nil {
			return MessagesListResult{}, nil, telegramErr("failed to get messages", err)
		}

		state.applyPage(page, params.Type, effectiveLimit)

		if !page.hasNext(state.offsetID) {
			break
		}

		state.offsetID = page.nextOffsetID
	}

	state.applyScanLimit(effectiveLimit)

	return MessagesListResult{
		Count:        len(state.filtered),
		Total:        state.total,
		HasMore:      state.hasMore,
		Participants: participantsFromMessages(state.filtered),
		Messages:     messagesToItems(state.filtered),
		Output:       formatMessages(state.filtered),
	}, state.filtered, nil
}

type messageTypeFilterState struct {
	filtered []telegram.Message
	total    int
	scanned  int
	hasMore  bool
	offsetID int
}

func newMessageTypeFilterState(offsetID int) messageTypeFilterState {
	return messageTypeFilterState{offsetID: offsetID}
}

func (state *messageTypeFilterState) shouldFetch(effectiveLimit int) bool {
	return len(state.filtered) < effectiveLimit && state.scanned < maxTypeFilterScan
}

func (state *messageTypeFilterState) applyPage(page messageTypeFilterPage, messageType string, effectiveLimit int) {
	if state.total == 0 {
		state.total = page.total
	}

	if len(page.messages) == 0 {
		state.hasMore = false

		return
	}

	state.scanned += len(page.messages)
	state.hasMore = page.hasMore

	for idx := range page.messages {
		msg := &page.messages[idx]
		if messageTypeOrText(msg.Type) != messageType {
			continue
		}

		state.filtered = append(state.filtered, *msg)
		if len(state.filtered) >= effectiveLimit {
			return
		}
	}
}

func (state *messageTypeFilterState) applyScanLimit(effectiveLimit int) {
	if state.scanned >= maxTypeFilterScan && len(state.filtered) < effectiveLimit {
		state.hasMore = true
	}
}

type messageTypeFilterPage struct {
	messages     []telegram.Message
	total        int
	hasMore      bool
	nextOffsetID int
}

func fetchMessageTypeFilterPage(
	ctx context.Context,
	client telegram.Client,
	peer telegram.InputPeer,
	params *MessagesListParams,
	offsetID int,
) (messageTypeFilterPage, error) {
	// The outer loop drives its own pagination, so each RPC fetches a
	// single server page; a larger Limit would double-page against
	// GetHistory's internal pager. effectiveLimit stays the accumulation
	// target the outer loop counts filtered matches against.
	opts := telegram.HistoryOpts{
		Limit:    telegram.DefaultLimit,
		OffsetID: offsetID,
	}

	msgs, total, err := getMessageHistoryPage(ctx, client, peer, deref(params.TopicID), opts)
	if err != nil {
		return messageTypeFilterPage{}, err
	}

	return messageTypeFilterPage{
		messages:     msgs,
		total:        total,
		hasMore:      hasMorePage(len(msgs), opts.Limit),
		nextOffsetID: nextHistoryOffsetID(msgs),
	}, nil
}

func (p messageTypeFilterPage) hasNext(currentOffsetID int) bool {
	return p.hasMore && p.nextOffsetID != 0 && p.nextOffsetID != currentOffsetID
}

func getMessageHistoryPage(
	ctx context.Context, client telegram.Client,
	peer telegram.InputPeer, topicID int, opts telegram.HistoryOpts,
) ([]telegram.Message, int, error) {
	// The type-filter path drives its own outer scan loop and derives
	// hasMore from maxTypeFilterScan (applyScanLimit), so the wrapper's
	// per-call hasMore is not used here.
	if topicID > 0 {
		msgs, total, _, err := client.GetTopicMessages(ctx, peer, topicID, opts)
		if err != nil {
			return nil, 0, errors.Wrap(err, "getting topic messages")
		}

		return msgs, total, nil
	}

	msgs, total, _, err := client.GetHistory(ctx, peer, opts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting message history")
	}

	return msgs, total, nil
}

func messageListEffectiveLimit(requestedLimit int) int {
	if requestedLimit <= 0 {
		return telegram.DefaultLimit
	}

	return requestedLimit
}

func nextHistoryOffsetID(msgs []telegram.Message) int {
	next := 0

	for idx := range msgs {
		msg := &msgs[idx]
		if msg.ID <= 0 {
			continue
		}

		if next == 0 || msg.ID < next {
			next = msg.ID
		}
	}

	return next
}

// MessagesListTool returns the MCP tool definition for tg_messages_list.
func MessagesListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_list",
		Description: "List messages in a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

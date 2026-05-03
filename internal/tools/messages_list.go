package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesListParams defines the parameters for the tg_messages_list tool.
type MessagesListParams struct {
	Peer           string `json:"peer"                     jsonschema:"@username, t.me/ link, or numeric ID"`
	TopicID        *int   `json:"topicId,omitempty"        jsonschema:"Forum topic ID to filter messages"`
	Limit          *int   `json:"limit,omitempty"          jsonschema:"Max messages to return (default 100)"`
	OffsetID       *int   `json:"offsetId,omitempty"       jsonschema:"Message ID to start from"`
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

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesListResult{},
				validationErr(limitErr)
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
	topicID := deref(params.TopicID)
	opts := telegram.HistoryOpts{
		Limit:    deref(params.Limit),
		OffsetID: deref(params.OffsetID),
	}

	var (
		msgs  []telegram.Message
		total int
		err   error
	)

	if topicID > 0 {
		msgs, total, err = client.GetTopicMessages(ctx, peer, topicID, opts)
	} else {
		msgs, total, err = client.GetHistory(ctx, peer, opts)
	}

	if err != nil {
		return MessagesListResult{}, nil, telegramErr("failed to get messages", err)
	}

	return MessagesListResult{
		Count:        len(msgs),
		Total:        total,
		HasMore:      hasMorePage(len(msgs), deref(params.Limit)),
		Participants: participantsFromMessages(msgs),
		Messages:     messagesToItems(msgs),
		Output:       formatMessages(msgs),
	}, msgs, nil
}

// MessagesListTool returns the MCP tool definition for tg_messages_list.
func MessagesListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_list",
		Description: "List messages in a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

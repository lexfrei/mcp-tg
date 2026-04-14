package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultContextRadius = 10

// MessagesContextParams defines the parameters for the tg_messages_context tool.
type MessagesContextParams struct {
	Peer           string `json:"peer"                     jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID      int    `json:"messageId"                jsonschema:"Message ID to get context around"`
	Radius         *int   `json:"radius,omitempty"         jsonschema:"Number of messages before and after (default 10)"`
	ResolveReplies *bool  `json:"resolveReplies,omitempty" jsonschema:"Fetch parent message text for replies (default false, extra API call)"`
}

// MessagesContextResult is the output of the tg_messages_context tool.
type MessagesContextResult struct {
	Count        int               `json:"count"`
	Participants []ParticipantItem `json:"participants,omitempty"`
	Messages     []MessageItem     `json:"messages"`
	Output       string            `json:"output"`
}

// NewMessagesContextHandler creates a handler for the tg_messages_context tool.
func NewMessagesContextHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesContextParams, MessagesContextResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesContextParams,
	) (*mcp.CallToolResult, MessagesContextResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				telegramErr("failed to resolve peer", err)
		}

		msgs, err := fetchContext(ctx, client, peer, params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesContextResult{},
				telegramErr("failed to get message context", err)
		}

		items := messagesToItems(msgs)

		if deref(params.ResolveReplies) {
			resolveReplyParents(ctx, client, peer, items, msgs)
		}

		return nil, MessagesContextResult{
			Count:        len(msgs),
			Participants: participantsFromMessages(msgs),
			Messages:     items,
			Output:       formatContextMessages(msgs, params.MessageID),
		}, nil
	}
}

func fetchContext(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, params MessagesContextParams,
) ([]telegram.Message, error) {
	radius := deref(params.Radius)
	if radius <= 0 {
		radius = defaultContextRadius
	}

	opts := telegram.HistoryOpts{
		Limit:    radius*2 + 1,
		OffsetID: params.MessageID + radius,
	}

	msgs, _, err := client.GetHistory(ctx, peer, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting history")
	}

	return msgs, nil
}

func formatContextMessages(msgs []telegram.Message, targetID int) string {
	var buf strings.Builder

	for idx := range msgs {
		marker := "  "
		if msgs[idx].ID == targetID {
			marker = "> "
		}

		fmt.Fprintf(&buf, "%s%s\n", marker, formatMessage(&msgs[idx]))
	}

	return buf.String()
}

// MessagesContextTool returns the MCP tool definition for tg_messages_context.
func MessagesContextTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_context",
		Description: "Get messages around a specific message in a Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

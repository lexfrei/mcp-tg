package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesUnpinAllParams defines the parameters for the tg_messages_unpin_all tool.
type MessagesUnpinAllParams struct {
	Peer    string `json:"peer"              jsonschema:"@username, t.me/ link, or numeric ID"`
	TopicID *int   `json:"topicId,omitempty" jsonschema:"Unpin only inside this forum topic"`
}

// MessagesUnpinAllResult is the output of the tg_messages_unpin_all tool.
type MessagesUnpinAllResult struct {
	Output string `json:"output"`
}

// NewMessagesUnpinAllHandler creates a handler for the tg_messages_unpin_all tool.
func NewMessagesUnpinAllHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesUnpinAllParams, MessagesUnpinAllResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesUnpinAllParams,
	) (*mcp.CallToolResult, MessagesUnpinAllResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesUnpinAllResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesUnpinAllResult{},
				telegramErr("failed to resolve peer", err)
		}

		topicID := deref(params.TopicID)

		err = validateTopicID(ctx, client, peer, topicID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesUnpinAllResult{}, err
		}

		err = client.UnpinAllMessages(ctx, peer, topicID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesUnpinAllResult{},
				telegramErr("failed to unpin messages", err)
		}

		output := "Unpinned all messages"
		if topicID > 0 {
			output = "Unpinned all messages in the topic"
		}

		return nil, MessagesUnpinAllResult{Output: output}, nil
	}
}

// MessagesUnpinAllTool returns the MCP tool definition for tg_messages_unpin_all.
func MessagesUnpinAllTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_unpin_all",
		Description: "Unpin every pinned message in a Telegram chat, or in one forum topic",
		Annotations: idempotentAnnotations(),
	}
}

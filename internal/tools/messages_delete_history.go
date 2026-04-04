package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesDeleteHistoryParams defines parameters for tg_messages_delete_history.
type MessagesDeleteHistoryParams struct {
	Peer   string `json:"peer"             jsonschema:"@username, t.me/ link, or numeric ID"`
	Revoke *bool  `json:"revoke,omitempty" jsonschema:"Delete for both sides (default false)"`
}

// MessagesDeleteHistoryResult is the output of tg_messages_delete_history.
type MessagesDeleteHistoryResult struct {
	Output string `json:"output"`
}

// NewMessagesDeleteHistoryHandler creates a handler for tg_messages_delete_history.
func NewMessagesDeleteHistoryHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesDeleteHistoryParams, MessagesDeleteHistoryResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesDeleteHistoryParams,
	) (*mcp.CallToolResult, MessagesDeleteHistoryResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				MessagesDeleteHistoryResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesDeleteHistoryResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.DeleteHistory(ctx, peer, deref(params.Revoke))
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesDeleteHistoryResult{},
				telegramErr("failed to delete history", err)
		}

		return nil, MessagesDeleteHistoryResult{
			Output: "Deleted history for " + params.Peer,
		}, nil
	}
}

// MessagesDeleteHistoryTool returns the tool definition.
func MessagesDeleteHistoryTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_delete_history",
		Description: "Delete all message history in a Telegram chat",
		Annotations: destructiveAnnotations(),
	}
}

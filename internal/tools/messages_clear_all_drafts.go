package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesClearAllDraftsParams defines parameters for tg_messages_clear_all_drafts.
type MessagesClearAllDraftsParams struct{}

// MessagesClearAllDraftsResult is the output of tg_messages_clear_all_drafts.
type MessagesClearAllDraftsResult struct {
	Cleared bool   `json:"cleared"`
	Output  string `json:"output"`
}

// NewMessagesClearAllDraftsHandler creates a handler for tg_messages_clear_all_drafts.
func NewMessagesClearAllDraftsHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesClearAllDraftsParams, MessagesClearAllDraftsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ MessagesClearAllDraftsParams,
	) (*mcp.CallToolResult, MessagesClearAllDraftsResult, error) {
		err := client.ClearAllDrafts(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				MessagesClearAllDraftsResult{},
				telegramErr("failed to clear all drafts", err)
		}

		return nil, MessagesClearAllDraftsResult{
			Cleared: true,
			Output:  "Cleared all drafts",
		}, nil
	}
}

// MessagesClearAllDraftsTool returns the tool definition.
func MessagesClearAllDraftsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_clear_all_drafts",
		Description: "Clear all message drafts across all Telegram chats",
		Annotations: destructiveAnnotations(),
	}
}

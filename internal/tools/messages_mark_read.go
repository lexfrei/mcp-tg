package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesMarkReadParams defines the parameters for the tg_messages_mark_read tool.
type MessagesMarkReadParams struct {
	Peer  string `json:"peer"  jsonschema:"Chat ID or @username"`
	MaxID int    `json:"maxId" jsonschema:"Mark all messages up to this ID as read"`
}

// MessagesMarkReadResult is the output of the tg_messages_mark_read tool.
type MessagesMarkReadResult struct {
	Output string `json:"output"`
}

// NewMessagesMarkReadHandler creates a handler for the tg_messages_mark_read tool.
func NewMessagesMarkReadHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesMarkReadParams, MessagesMarkReadResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesMarkReadParams,
	) (*mcp.CallToolResult, MessagesMarkReadResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesMarkReadResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MaxID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesMarkReadResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesMarkReadResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.MarkRead(ctx, peer, params.MaxID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesMarkReadResult{},
				telegramErr("failed to mark messages as read", err)
		}

		return nil, MessagesMarkReadResult{
			Output: fmt.Sprintf("Marked messages as read up to ID %d", params.MaxID),
		}, nil
	}
}

// MessagesMarkReadTool returns the MCP tool definition for tg_messages_mark_read.
func MessagesMarkReadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_mark_read",
		Description: "Mark messages as read in a Telegram chat",
	}
}

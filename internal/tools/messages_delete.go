package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesDeleteParams defines the parameters for the tg_messages_delete tool.
type MessagesDeleteParams struct {
	Peer   string `json:"peer"             jsonschema:"Chat ID or @username"`
	IDs    []int  `json:"ids"              jsonschema:"Message IDs to delete"`
	Revoke *bool  `json:"revoke,omitempty" jsonschema:"Delete for everyone (default true)"`
}

// MessagesDeleteResult is the output of the tg_messages_delete tool.
type MessagesDeleteResult struct {
	Deleted int    `json:"deleted"`
	Output  string `json:"output"`
}

// NewMessagesDeleteHandler creates a handler for the tg_messages_delete tool.
func NewMessagesDeleteHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesDeleteParams, MessagesDeleteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesDeleteParams,
	) (*mcp.CallToolResult, MessagesDeleteResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				validationErr(ErrPeerRequired)
		}

		if len(params.IDs) == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				telegramErr("failed to resolve peer", err)
		}

		revoke := true
		if params.Revoke != nil {
			revoke = *params.Revoke
		}

		err = client.DeleteMessages(ctx, peer, params.IDs, revoke)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				telegramErr("failed to delete messages", err)
		}

		return nil, MessagesDeleteResult{
			Deleted: len(params.IDs),
			Output:  fmt.Sprintf("Deleted %d message(s)", len(params.IDs)),
		}, nil
	}
}

// MessagesDeleteTool returns the MCP tool definition for tg_messages_delete.
func MessagesDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_delete",
		Description: "Delete messages from a Telegram chat",
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesForwardParams defines the parameters for the tg_messages_forward tool.
type MessagesForwardParams struct {
	FromPeer string `json:"fromPeer" jsonschema:"Source chat ID or @username"`
	ToPeer   string `json:"toPeer"   jsonschema:"Destination chat ID or @username"`
	IDs      []int  `json:"ids"      jsonschema:"Message IDs to forward"`
}

// MessagesForwardResult is the output of the tg_messages_forward tool.
type MessagesForwardResult struct {
	Forwarded int    `json:"forwarded"`
	Output    string `json:"output"`
}

// NewMessagesForwardHandler creates a handler for the tg_messages_forward tool.
func NewMessagesForwardHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesForwardParams, MessagesForwardResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesForwardParams,
	) (*mcp.CallToolResult, MessagesForwardResult, error) {
		if params.FromPeer == "" || params.ToPeer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesForwardResult{},
				validationErr(ErrPeerRequired)
		}

		if len(params.IDs) == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesForwardResult{},
				validationErr(ErrMessageIDRequired)
		}

		from, err := client.ResolvePeer(ctx, params.FromPeer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesForwardResult{},
				telegramErr("failed to resolve source peer", err)
		}

		dest, err := client.ResolvePeer(ctx, params.ToPeer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesForwardResult{},
				telegramErr("failed to resolve destination peer", err)
		}

		msgs, err := client.ForwardMessages(ctx, from, dest, params.IDs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesForwardResult{},
				telegramErr("failed to forward messages", err)
		}

		return nil, MessagesForwardResult{
			Forwarded: len(msgs),
			Output:    fmt.Sprintf("Forwarded %d message(s)", len(msgs)),
		}, nil
	}
}

// MessagesForwardTool returns the MCP tool definition for tg_messages_forward.
func MessagesForwardTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_forward",
		Description: "Forward messages from one Telegram chat to another",
	}
}

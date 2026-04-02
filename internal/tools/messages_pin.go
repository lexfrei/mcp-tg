package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesPinParams defines the parameters for the tg_messages_pin tool.
type MessagesPinParams struct {
	Peer      string `json:"peer"            jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId"       jsonschema:"ID of the message to pin/unpin"`
	Unpin     *bool  `json:"unpin,omitempty" jsonschema:"Set to true to unpin instead of pin"`
}

// MessagesPinResult is the output of the tg_messages_pin tool.
type MessagesPinResult struct {
	Output string `json:"output"`
}

// NewMessagesPinHandler creates a handler for the tg_messages_pin tool.
func NewMessagesPinHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesPinParams, MessagesPinResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesPinParams,
	) (*mcp.CallToolResult, MessagesPinResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesPinResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesPinResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesPinResult{},
				telegramErr("failed to resolve peer", err)
		}

		unpin := deref(params.Unpin)

		err = client.PinMessage(ctx, peer, params.MessageID, unpin)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesPinResult{},
				telegramErr("failed to pin/unpin message", err)
		}

		action := "Pinned"
		if unpin {
			action = "Unpinned"
		}

		return nil, MessagesPinResult{
			Output: fmt.Sprintf("%s message %d", action, params.MessageID),
		}, nil
	}
}

// MessagesPinTool returns the MCP tool definition for tg_messages_pin.
func MessagesPinTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_pin",
		Description: "Pin or unpin a message in a Telegram chat",
		Annotations: idempotentAnnotations(),
	}
}

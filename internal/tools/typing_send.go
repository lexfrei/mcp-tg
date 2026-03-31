package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultTypingAction = "typing"

// TypingSendParams defines the parameters for the tg_typing_send tool.
type TypingSendParams struct {
	Peer   string  `json:"peer"             jsonschema:"Chat ID or @username"`
	Action *string `json:"action,omitempty" jsonschema:"Typing action type (default typing)"`
}

// TypingSendResult is the output of the tg_typing_send tool.
type TypingSendResult struct {
	Output string `json:"output"`
}

// NewTypingSendHandler creates a handler for the tg_typing_send tool.
func NewTypingSendHandler(client telegram.Client) mcp.ToolHandlerFor[TypingSendParams, TypingSendResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TypingSendParams,
	) (*mcp.CallToolResult, TypingSendResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, TypingSendResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TypingSendResult{},
				telegramErr("failed to resolve peer", err)
		}

		action := deref(params.Action)
		if action == "" {
			action = defaultTypingAction
		}

		err = client.SendTyping(ctx, peer, action)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TypingSendResult{},
				telegramErr("failed to send typing action", err)
		}

		return nil, TypingSendResult{
			Output: "Sent " + action + " action to " + params.Peer,
		}, nil
	}
}

// TypingSendTool returns the MCP tool definition for tg_typing_send.
func TypingSendTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_typing_send",
		Description: "Send a typing indicator to a Telegram chat",
	}
}

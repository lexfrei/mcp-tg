package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesReactParams defines the parameters for the tg_messages_react tool.
type MessagesReactParams struct {
	Peer      string `json:"peer"             jsonschema:"Chat ID or @username"`
	MessageID int    `json:"messageId"        jsonschema:"ID of the message to react to"`
	Emoji     string `json:"emoji"            jsonschema:"Reaction emoji (e.g. 👍)"`
	Remove    *bool  `json:"remove,omitempty" jsonschema:"Remove reaction instead of adding"`
}

// MessagesReactResult is the output of the tg_messages_react tool.
type MessagesReactResult struct {
	Output string `json:"output"`
}

// NewMessagesReactHandler creates a handler for the tg_messages_react tool.
func NewMessagesReactHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesReactParams, MessagesReactResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesReactParams,
	) (*mcp.CallToolResult, MessagesReactResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				telegramErr("failed to resolve peer", err)
		}

		remove := deref(params.Remove)

		err = client.SendReaction(ctx, peer, params.MessageID, params.Emoji, remove)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				telegramErr("failed to send reaction", err)
		}

		action := "Added"
		if remove {
			action = "Removed"
		}

		return nil, MessagesReactResult{
			Output: fmt.Sprintf("%s reaction %s on message %d", action, params.Emoji, params.MessageID),
		}, nil
	}
}

// MessagesReactTool returns the MCP tool definition for tg_messages_react.
func MessagesReactTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_react",
		Description: "Add or remove a reaction on a Telegram message",
	}
}

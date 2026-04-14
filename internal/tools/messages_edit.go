package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesEditParams defines the parameters for the tg_messages_edit tool.
type MessagesEditParams struct {
	Peer      string  `json:"peer"                jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int     `json:"messageId"           jsonschema:"ID of the message to edit"`
	Text      string  `json:"text"                jsonschema:"New message text"`
	ParseMode *string `json:"parseMode,omitempty" jsonschema:"'' plain; 'commonmark' (**bold**, [x](url)); 'markdown' alias"`
}

// MessagesEditResult is the output of the tg_messages_edit tool.
type MessagesEditResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewMessagesEditHandler creates a handler for the tg_messages_edit tool.
func NewMessagesEditHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesEditParams, MessagesEditResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesEditParams,
	) (*mcp.CallToolResult, MessagesEditResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				validationErr(ErrMessageIDRequired)
		}

		if params.Text == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				validationErr(ErrTextRequired)
		}

		pmErr := validateParseMode(deref(params.ParseMode))
		if pmErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				validationErr(pmErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				telegramErr("failed to resolve peer", err)
		}

		msg, err := client.EditMessage(ctx, peer, params.MessageID, params.Text, deref(params.ParseMode))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				telegramErr("failed to edit message", err)
		}

		msgID := params.MessageID
		if msg != nil {
			msgID = msg.ID
		}

		return nil, MessagesEditResult{
			MessageID: msgID,
			Output:    fmt.Sprintf("Message %d edited", msgID),
		}, nil
	}
}

// MessagesEditTool returns the MCP tool definition for tg_messages_edit.
func MessagesEditTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_edit",
		Description: "Edit an existing message in a Telegram chat (supports markdown formatting)",
		Annotations: idempotentAnnotations(),
	}
}

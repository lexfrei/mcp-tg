package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesSendParams defines the parameters for the tg_messages_send tool.
type MessagesSendParams struct {
	Peer         string  `json:"peer"                   jsonschema:"@username, t.me/ link, or numeric ID"`
	Text         string  `json:"text"                   jsonschema:"Message text to send"`
	ReplyTo      *int    `json:"replyTo,omitempty"      jsonschema:"Message ID to reply to"`
	ParseMode    *string `json:"parseMode,omitempty"    jsonschema:"Text format: 'markdown' for rich text, empty for plain"`
	Silent       *bool   `json:"silent,omitempty"       jsonschema:"Send without notification sound"`
	ScheduleDate *int    `json:"scheduleDate,omitempty" jsonschema:"Unix timestamp to schedule message for later delivery"`
}

// MessagesSendResult is the output of the tg_messages_send tool.
type MessagesSendResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewMessagesSendHandler creates a handler for the tg_messages_send tool.
func NewMessagesSendHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesSendParams, MessagesSendResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesSendParams,
	) (*mcp.CallToolResult, MessagesSendResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Text == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				validationErr(ErrTextRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				telegramErr("failed to resolve peer", err)
		}

		opts := telegram.SendOpts{
			ReplyTo:      deref(params.ReplyTo),
			ParseMode:    deref(params.ParseMode),
			Silent:       deref(params.Silent),
			ScheduleDate: deref(params.ScheduleDate),
		}

		msg, err := client.SendMessage(ctx, peer, params.Text, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				telegramErr("failed to send message", err)
		}

		msgID := 0
		if msg != nil {
			msgID = msg.ID
		}

		return nil, MessagesSendResult{
			MessageID: msgID,
			Output:    fmt.Sprintf("Message sent (ID: %d)", msgID),
		}, nil
	}
}

// MessagesSendTool returns the MCP tool definition for tg_messages_send.
func MessagesSendTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_send",
		Description: "Send a text message to a Telegram chat",
		Annotations: writeAnnotations(),
	}
}

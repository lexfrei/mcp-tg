package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesEditParams defines the parameters for the tg_messages_edit tool.
type MessagesEditParams struct {
	Peer      string `json:"peer"      jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of the message to edit"`
	Text      string `json:"text"      jsonschema:"New message text"`
	ParseMode string `json:"parseMode" jsonschema:"'plain' (no formatting) or 'commonmark' (CommonMark subset, see README)"`

	// AllowRawMarkdown skips the plain-mode markdown lint.
	AllowRawMarkdown *bool `json:"allowRawMarkdown,omitempty" jsonschema:"Send markdown-looking characters literally in plain mode"`
}

// MessagesEditResult is the output of the tg_messages_edit tool.
//
// EntitiesParsed mirrors MessagesSendResult: present even at 0.
type MessagesEditResult struct {
	MessageID      int    `json:"messageId"`
	EntitiesParsed int    `json:"entitiesParsed"`
	Output         string `json:"output"`
}

// NewMessagesEditHandler creates a handler for the tg_messages_edit tool.
func NewMessagesEditHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesEditParams, MessagesEditResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesEditParams,
	) (*mcp.CallToolResult, MessagesEditResult, error) {
		validErr := validateEditParams(&params)
		if validErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				validationErr(validErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				telegramErr("failed to resolve peer", err)
		}

		msg, err := client.EditMessage(ctx, peer, params.MessageID, params.Text, normalizeParseMode(params.ParseMode))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesEditResult{},
				telegramErr("failed to edit message", err)
		}

		// Telegram does not renumber an edited message, so the ID the
		// caller passed IS the message's ID. Deriving it from the echo
		// could only ever replace a known-good value with a guess.
		msgID := params.MessageID

		return nil, MessagesEditResult{
			MessageID:      msgID,
			EntitiesParsed: entityCount(msg),
			Output:         fmt.Sprintf("Message %d edited", msgID),
		}, nil
	}
}

// validateEditParams runs every request-shape check that needs no
// network round-trip, so a malformed call fails before any RPC.
func validateEditParams(params *MessagesEditParams) error {
	if params.Peer == "" {
		return ErrPeerRequired
	}

	if params.MessageID == 0 {
		return ErrMessageIDRequired
	}

	if params.Text == "" {
		return ErrTextRequired
	}

	pmErr := validateParseMode(params.ParseMode)
	if pmErr != nil {
		return pmErr
	}

	return validatePlainText(normalizeParseMode(params.ParseMode), deref(params.AllowRawMarkdown), params.Text)
}

// MessagesEditTool returns the MCP tool definition for tg_messages_edit.
func MessagesEditTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_edit",
		Description: "Edit an existing message in a Telegram chat " +
			"(parseMode is required: 'plain' or 'commonmark')",
		InputSchema: inputSchemaWithEnum[MessagesEditParams]("parseMode", parseModeEnum()),
		Annotations: idempotentAnnotations(),
	}
}

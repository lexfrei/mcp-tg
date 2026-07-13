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
	TopicID      *int    `json:"topicId,omitempty"      jsonschema:"Forum topic ID to send into"`
	ReplyTo      *int    `json:"replyTo,omitempty"      jsonschema:"Message ID to reply to"`
	ParseMode    string  `json:"parseMode"              jsonschema:"'plain' (no formatting) or 'commonmark' (CommonMark subset, see README)"`
	Silent       *bool   `json:"silent,omitempty"       jsonschema:"Send without notification sound"`
	NoWebpage    *bool   `json:"noWebpage,omitempty"    jsonschema:"Disable link preview generation"`
	ScheduleDate *int    `json:"scheduleDate,omitempty" jsonschema:"Unix timestamp to schedule message for later delivery"`
	SendAs       *string `json:"sendAs,omitempty"       jsonschema:"Post as this channel; see tg_chats_get_send_as. Omit for the chat default"`

	// AllowRawMarkdown skips the plain-mode markdown lint.
	AllowRawMarkdown *bool `json:"allowRawMarkdown,omitempty" jsonschema:"Send markdown-looking characters literally in plain mode"`
}

// MessagesSendResult is the output of the tg_messages_send tool.
//
// EntitiesParsed is the number of formatting entities the server
// accepted — deliberately serialized even at 0, since 0 after a
// commonmark send is the caller's signal that nothing parsed.
type MessagesSendResult struct {
	MessageID      int    `json:"messageId"`
	EntitiesParsed int    `json:"entitiesParsed"`
	Output         string `json:"output"`
}

// NewMessagesSendHandler creates a handler for the tg_messages_send tool.
func NewMessagesSendHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesSendParams, MessagesSendResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesSendParams,
	) (*mcp.CallToolResult, MessagesSendResult, error) {
		vErr := validateSendParams(&params)
		if vErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{}, validationErr(vErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				telegramErr("failed to resolve peer", err)
		}

		topicErr := validateTopicID(ctx, client, peer, deref(params.TopicID))
		if topicErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				validationErr(topicErr)
		}

		opts := sendOptsFrom(&params)

		opts.SendAs, err = resolveSendAs(ctx, client, deref(params.SendAs))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{}, err
		}

		msg, err := client.SendMessage(ctx, peer, params.Text, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendResult{},
				sendErr("failed to send message", err, opts.SendAs)
		}

		// echoOrSubmitted guarantees a non-nil message on the wrapper
		// path; the guard still covers direct callers and mocks.
		msgID := 0
		if msg != nil {
			msgID = msg.ID
		}

		return nil, MessagesSendResult{
			MessageID:      msgID,
			EntitiesParsed: entityCount(msg),
			Output:         fmt.Sprintf("Message sent (ID: %d)", msgID),
		}, nil
	}
}

func validateSendParams(params *MessagesSendParams) error {
	if params.Peer == "" {
		return ErrPeerRequired
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

func sendOptsFrom(params *MessagesSendParams) telegram.SendOpts {
	return telegram.SendOpts{
		ReplyTo:      deref(params.ReplyTo),
		TopicID:      deref(params.TopicID),
		ParseMode:    normalizeParseMode(params.ParseMode),
		Silent:       deref(params.Silent),
		NoWebpage:    deref(params.NoWebpage),
		ScheduleDate: deref(params.ScheduleDate),
	}
}

// MessagesSendTool returns the MCP tool definition for tg_messages_send.
func MessagesSendTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_send",
		Description: "Send a text message to a Telegram chat (silent mode, scheduling; " +
			"parseMode is required: 'plain' or 'commonmark')",
		InputSchema: inputSchemaWithEnum[MessagesSendParams]("parseMode", parseModeEnum()),
		Annotations: writeAnnotations(),
	}
}

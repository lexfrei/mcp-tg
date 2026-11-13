package tools

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrContactPhoneRequired is returned when a contact card has no phone number.
var ErrContactPhoneRequired = errors.New("phone is required")

// ErrContactNameRequired is returned when a contact card has no first name.
var ErrContactNameRequired = errors.New("firstName is required")

// MessagesSendContactParams defines the parameters for the tg_messages_send_contact tool.
type MessagesSendContactParams struct {
	Peer      string  `json:"peer"               jsonschema:"@username, t.me/ link, or numeric ID"`
	Phone     string  `json:"phone"              jsonschema:"Phone number of the contact, international format"`
	FirstName string  `json:"firstName"          jsonschema:"First name of the contact"`
	LastName  string  `json:"lastName,omitempty" jsonschema:"Last name of the contact"`
	TopicID   *int    `json:"topicId,omitempty"  jsonschema:"Forum topic ID to send into"`
	ReplyTo   *int    `json:"replyTo,omitempty"  jsonschema:"Message ID to reply to"`
	Silent    *bool   `json:"silent,omitempty"   jsonschema:"Send without notification sound"`
	SendAs    *string `json:"sendAs,omitempty"   jsonschema:"Post as this channel; see tg_chats_get_send_as. Omit for the chat default"`
}

// MessagesSendContactResult is the output of the tg_messages_send_contact tool.
type MessagesSendContactResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewMessagesSendContactHandler creates a handler for the tg_messages_send_contact tool.
func NewMessagesSendContactHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesSendContactParams, MessagesSendContactResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesSendContactParams,
	) (*mcp.CallToolResult, MessagesSendContactResult, error) {
		err := validateSendContactParams(&params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendContactResult{}, validationErr(err)
		}

		peer, opts, err := resolveContactSend(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendContactResult{}, err
		}

		contact := telegram.Contact{Phone: params.Phone, FirstName: params.FirstName, LastName: params.LastName}

		msg, err := client.SendContact(ctx, peer, contact, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendContactResult{},
				sendErr("failed to send contact", err, opts.SendAs)
		}

		result := MessagesSendContactResult{Output: "Contact sent"}
		if msg != nil {
			result.MessageID = msg.ID
			result.Output = fmt.Sprintf("Contact sent as message %d", msg.ID)
		}

		return nil, result, nil
	}
}

func validateSendContactParams(params *MessagesSendContactParams) error {
	switch {
	case params.Peer == "":
		return ErrPeerRequired
	case params.Phone == "":
		return ErrContactPhoneRequired
	case params.FirstName == "":
		return ErrContactNameRequired
	default:
		return nil
	}
}

func resolveContactSend(
	ctx context.Context, client telegram.Client, params *MessagesSendContactParams,
) (telegram.InputPeer, telegram.SendOpts, error) {
	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return telegram.InputPeer{}, telegram.SendOpts{}, telegramErr("failed to resolve peer", err)
	}

	opts := telegram.SendOpts{
		TopicID: deref(params.TopicID),
		ReplyTo: deref(params.ReplyTo),
		Silent:  deref(params.Silent),
	}

	err = validateTopicID(ctx, client, peer, opts.TopicID)
	if err != nil {
		return telegram.InputPeer{}, telegram.SendOpts{}, err
	}

	opts.SendAs, err = resolveSendAs(ctx, client, deref(params.SendAs))
	if err != nil {
		return telegram.InputPeer{}, telegram.SendOpts{}, err
	}

	return peer, opts, nil
}

// MessagesSendContactTool returns the MCP tool definition for tg_messages_send_contact.
func MessagesSendContactTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_send_contact",
		Description: "Send a contact card (phone number and name) to a Telegram chat",
		Annotations: writeAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesGetButtonsParams defines the parameters for the tg_messages_get_buttons tool.
type MessagesGetButtonsParams struct {
	Peer      string `json:"peer"      jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of the message whose buttons to list"`
}

// MessagesGetButtonsResult is the output of the tg_messages_get_buttons tool.
type MessagesGetButtonsResult struct {
	Count   int                      `json:"count"`
	Buttons []telegram.MessageButton `json:"buttons"`
	Output  string                   `json:"output"`
}

// NewMessagesGetButtonsHandler creates a handler for the tg_messages_get_buttons tool.
func NewMessagesGetButtonsHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesGetButtonsParams, MessagesGetButtonsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesGetButtonsParams,
	) (*mcp.CallToolResult, MessagesGetButtonsResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetButtonsResult{}, err
		}

		buttons, err := client.GetMessageButtons(ctx, peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetButtonsResult{},
				telegramErr("failed to read the message's buttons", err)
		}

		return nil, MessagesGetButtonsResult{
			Count:   len(buttons),
			Buttons: buttons,
			Output:  formatButtons(buttons),
		}, nil
	}
}

// MessagesGetButtonsTool returns the MCP tool definition for tg_messages_get_buttons.
func MessagesGetButtonsTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_get_buttons",
		Description: "List the buttons of a Telegram message (a bot's inline keyboard or reply keyboard), " +
			"with the row and column tg_messages_press_button takes",
		Annotations: readOnlyAnnotations(),
	}
}

// MessagesPressButtonParams defines the parameters for the tg_messages_press_button tool.
type MessagesPressButtonParams struct {
	Peer      string `json:"peer"      jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of the message carrying the button"`
	Row       int    `json:"row"       jsonschema:"Zero-based row of the button, from tg_messages_get_buttons"`
	Column    int    `json:"column"    jsonschema:"Zero-based column of the button, from tg_messages_get_buttons"`
}

// MessagesPressButtonResult is the output of the tg_messages_press_button tool.
type MessagesPressButtonResult struct {
	telegram.ButtonPress

	Output string `json:"output"`
}

// NewMessagesPressButtonHandler creates a handler for the tg_messages_press_button tool.
func NewMessagesPressButtonHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesPressButtonParams, MessagesPressButtonResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesPressButtonParams,
	) (*mcp.CallToolResult, MessagesPressButtonResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesPressButtonResult{}, err
		}

		press, err := client.PressButton(ctx, peer, params.MessageID, params.Row, params.Column)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesPressButtonResult{},
				telegramErr("failed to press the button", err)
		}

		return nil, MessagesPressButtonResult{ButtonPress: *press, Output: formatButtonPress(press)}, nil
	}
}

// MessagesPressButtonTool returns the MCP tool definition for tg_messages_press_button.
func MessagesPressButtonTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_press_button",
		Description: "Press a button of a Telegram message. A callback button returns the bot's answer; " +
			"a link button returns its URL without opening it; a reply-keyboard button sends its text",
		Annotations: writeAnnotations(),
	}
}

func formatButtons(buttons []telegram.MessageButton) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%d button(s)", len(buttons))

	for _, btn := range buttons {
		fmt.Fprintf(&buf, "\n[%d,%d] %s (%s)", btn.Row, btn.Column, btn.Text, btn.Kind)

		switch {
		case btn.URL != "":
			fmt.Fprintf(&buf, " %s", btn.URL)
		case btn.CopyText != "":
			fmt.Fprintf(&buf, " copies %q", btn.CopyText)
		}
	}

	return buf.String()
}

func formatButtonPress(press *telegram.ButtonPress) string {
	switch {
	case press.SentMessageID != 0:
		return fmt.Sprintf("Sent the button's text as message %d", press.SentMessageID)
	case press.Message != "":
		return "Bot answered: " + press.Message
	case press.URL != "":
		return "Button links to " + press.URL
	case press.CopyText != "":
		return "Button copies: " + press.CopyText
	default:
		return "Pressed; the bot sent no answer text"
	}
}

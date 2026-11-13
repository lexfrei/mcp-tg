package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesGetLinkParams defines the parameters for the tg_messages_get_link tool.
type MessagesGetLinkParams struct {
	Peer      string `json:"peer"             jsonschema:"Channel or supergroup: @username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId"        jsonschema:"ID of the message to link to"`
	Thread    *bool  `json:"thread,omitempty" jsonschema:"Link to the message inside its comment thread or forum topic"`
}

// MessagesGetLinkResult is the output of the tg_messages_get_link tool.
type MessagesGetLinkResult struct {
	Link   string `json:"link"`
	Output string `json:"output"`
}

// NewMessagesGetLinkHandler creates a handler for the tg_messages_get_link tool.
func NewMessagesGetLinkHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesGetLinkParams, MessagesGetLinkResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesGetLinkParams,
	) (*mcp.CallToolResult, MessagesGetLinkResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetLinkResult{}, err
		}

		link, err := client.GetMessageLink(ctx, peer, params.MessageID, deref(params.Thread))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetLinkResult{},
				telegramErr("failed to get the message link", err)
		}

		return nil, MessagesGetLinkResult{Link: link, Output: link}, nil
	}
}

// MessagesGetLinkTool returns the MCP tool definition for tg_messages_get_link.
func MessagesGetLinkTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_get_link",
		Description: "Get the t.me link of a message in a channel or supergroup",
		Annotations: readOnlyAnnotations(),
	}
}

// MessagesGetReadByParams defines the parameters for the tg_messages_get_read_by tool.
type MessagesGetReadByParams struct {
	Peer      string `json:"peer"      jsonschema:"Group: @username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of one of your own messages"`
}

// MessagesGetReadByResult is the output of the tg_messages_get_read_by tool.
type MessagesGetReadByResult struct {
	Count  int                    `json:"count"`
	Users  []telegram.ReadReceipt `json:"users"`
	Output string                 `json:"output"`
}

// NewMessagesGetReadByHandler creates a handler for the tg_messages_get_read_by tool.
func NewMessagesGetReadByHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesGetReadByParams, MessagesGetReadByResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesGetReadByParams,
	) (*mcp.CallToolResult, MessagesGetReadByResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetReadByResult{}, err
		}

		receipts, err := client.GetMessageReadBy(ctx, peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesGetReadByResult{},
				telegramErr("failed to get read receipts", err)
		}

		return nil, MessagesGetReadByResult{
			Count:  len(receipts),
			Users:  receipts,
			Output: formatReadReceipts(params.MessageID, receipts),
		}, nil
	}
}

// MessagesGetReadByTool returns the MCP tool definition for tg_messages_get_read_by.
func MessagesGetReadByTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_get_read_by",
		Description: "List who read one of your own messages in a group, and when. " +
			"Telegram answers only in small groups and only for recent messages",
		Annotations: readOnlyAnnotations(),
	}
}

func formatReadReceipts(msgID int, receipts []telegram.ReadReceipt) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Message %d read by %d member(s)", msgID, len(receipts))

	for _, receipt := range receipts {
		fmt.Fprintf(&buf, "\n%s at %s",
			formatPeerRef("", "", telegram.InputPeer{Type: telegram.PeerUser, ID: receipt.UserID}),
			formatTimestamp(receipt.Date))
	}

	return buf.String()
}

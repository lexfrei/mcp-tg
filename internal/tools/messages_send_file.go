package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesSendFileParams defines the parameters for the tg_messages_send_file tool.
type MessagesSendFileParams struct {
	Peer    string  `json:"peer"              jsonschema:"Chat ID or @username"`
	Path    string  `json:"path"              jsonschema:"Local file path to send"`
	Caption *string `json:"caption,omitempty" jsonschema:"Optional caption for the file"`
}

// MessagesSendFileResult is the output of the tg_messages_send_file tool.
type MessagesSendFileResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewMessagesSendFileHandler creates a handler for the tg_messages_send_file tool.
func NewMessagesSendFileHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesSendFileParams, MessagesSendFileResult] {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		params MessagesSendFileParams,
	) (*mcp.CallToolResult, MessagesSendFileResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Path == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				validationErr(ErrPathRequired)
		}

		rootErr := validatePathAgainstRoots(ctx, req.Session, params.Path)
		if rootErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				validationErr(rootErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				telegramErr("failed to resolve peer", err)
		}

		caption := deref(params.Caption)

		msg, err := client.SendFile(ctx, peer, params.Path, caption)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				telegramErr("failed to send file", err)
		}

		msgID := 0
		if msg != nil {
			msgID = msg.ID
		}

		return nil, MessagesSendFileResult{
			MessageID: msgID,
			Output:    fmt.Sprintf("File sent to %s (message ID: %d)", params.Peer, msgID),
		}, nil
	}
}

// MessagesSendFileTool returns the MCP tool definition for tg_messages_send_file.
func MessagesSendFileTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_send_file",
		Description: "Send a file to a Telegram chat",
		Annotations: idempotentAnnotations(),
	}
}

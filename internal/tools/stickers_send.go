package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StickersSendParams defines the parameters for the tg_stickers_send tool.
type StickersSendParams struct {
	Peer          string  `json:"peer"             jsonschema:"@username, t.me/ link, or numeric ID"`
	StickerFileID int64   `json:"stickerFileId"    jsonschema:"File ID of the sticker to send"`
	SendAs        *string `json:"sendAs,omitempty" jsonschema:"Post as this channel; see tg_chats_get_send_as. Omit to post as yourself"`
}

// StickersSendResult is the output of the tg_stickers_send tool.
type StickersSendResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewStickersSendHandler creates a handler for the tg_stickers_send tool.
func NewStickersSendHandler(client telegram.Client) mcp.ToolHandlerFor[StickersSendParams, StickersSendResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params StickersSendParams,
	) (*mcp.CallToolResult, StickersSendResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, StickersSendResult{},
				validationErr(ErrPeerRequired)
		}

		if params.StickerFileID == 0 {
			return &mcp.CallToolResult{IsError: true}, StickersSendResult{},
				validationErr(ErrStickerFileIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, StickersSendResult{},
				telegramErr("failed to resolve peer", err)
		}

		sendAs, err := resolveSendAs(ctx, client, deref(params.SendAs))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, StickersSendResult{}, err
		}

		msg, err := client.SendSticker(ctx, peer, params.StickerFileID, sendAs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, StickersSendResult{},
				sendErr("failed to send sticker", err, sendAs)
		}

		msgID := 0
		if msg != nil {
			msgID = msg.ID
		}

		return nil, StickersSendResult{
			MessageID: msgID,
			Output:    fmt.Sprintf("Sticker sent (message ID: %d)", msgID),
		}, nil
	}
}

// StickersSendTool returns the MCP tool definition for tg_stickers_send.
func StickersSendTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_stickers_send",
		Description: "Send a sticker to a Telegram chat",
		Annotations: writeAnnotations(),
	}
}

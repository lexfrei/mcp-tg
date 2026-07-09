package tools

import (
	"context"
	"fmt"
	"strconv"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StickersSendParams defines the parameters for the tg_stickers_send tool.
//
// StickerFileID is a decimal string, not a number. The MCP SDK routes
// tool arguments through map[string]any to apply schema defaults, so a
// JSON number is parsed as float64 and re-marshalled. A sticker document
// id needs 63 bits and a float64 mantissa holds 53, so the number form
// silently arrives corrupted.
type StickersSendParams struct {
	Peer          string  `json:"peer"             jsonschema:"@username, t.me/ link, or numeric ID"`
	StickerFileID string  `json:"stickerFileId"    jsonschema:"Sticker file ID as a decimal string, from tg_stickers_get_set"`
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

		fileID, idErr := parseStickerFileID(params.StickerFileID)
		if idErr != nil {
			return &mcp.CallToolResult{IsError: true}, StickersSendResult{}, validationErr(idErr)
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

		msg, err := client.SendSticker(ctx, peer, fileID, sendAs)
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

// parseStickerFileID converts the decimal-string sticker id into the
// int64 the wrapper needs, rejecting anything that is not one.
func parseStickerFileID(fileID string) (int64, error) {
	if fileID == "" {
		return 0, ErrStickerFileIDRequired
	}

	parsed, err := strconv.ParseInt(fileID, 10, 64)
	if err != nil {
		return 0, ErrInvalidStickerFileID
	}

	return parsed, nil
}

// StickersSendTool returns the MCP tool definition for tg_stickers_send.
func StickersSendTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_stickers_send",
		Description: "Send a sticker to a Telegram chat. Call tg_stickers_get_set for the sticker's set first: " +
			"a sticker is addressed by an id plus an access hash and a file reference, and only the set carries those.",
		Annotations: writeAnnotations(),
	}
}

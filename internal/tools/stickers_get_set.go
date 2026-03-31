package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StickersGetSetParams defines the parameters for the tg_stickers_get_set tool.
type StickersGetSetParams struct {
	Name string `json:"name" jsonschema:"Short name of the sticker set"`
}

// StickersGetSetResult is the output of the tg_stickers_get_set tool.
type StickersGetSetResult struct {
	Title  string `json:"title"`
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewStickersGetSetHandler creates a handler for the tg_stickers_get_set tool.
func NewStickersGetSetHandler(client telegram.Client) mcp.ToolHandlerFor[StickersGetSetParams, StickersGetSetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params StickersGetSetParams,
	) (*mcp.CallToolResult, StickersGetSetResult, error) {
		if params.Name == "" {
			return &mcp.CallToolResult{IsError: true}, StickersGetSetResult{},
				validationErr(ErrNameRequired)
		}

		full, err := client.GetStickerSet(ctx, params.Name)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, StickersGetSetResult{},
				telegramErr("failed to get sticker set", err)
		}

		var buf strings.Builder

		fmt.Fprintf(&buf, "Set: %s (%s) — %d stickers\n", full.Title, full.Name, len(full.Stickers))

		for _, stk := range full.Stickers {
			fmt.Fprintf(&buf, "  %s fileID:%d (emoji: %s)\n", full.Name, stk.FileID, stk.Emoji)
		}

		return nil, StickersGetSetResult{
			Title:  full.Title,
			Count:  len(full.Stickers),
			Output: buf.String(),
		}, nil
	}
}

// StickersGetSetTool returns the MCP tool definition for tg_stickers_get_set.
func StickersGetSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_stickers_get_set",
		Description: "Get details and sticker list for a Telegram sticker set",
		Annotations: readOnlyAnnotations(),
	}
}

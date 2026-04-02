package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// StickersSearchParams defines the parameters for the tg_stickers_search tool.
type StickersSearchParams struct {
	Query string `json:"query" jsonschema:"Search query for sticker sets"`
}

// StickersSearchResult is the output of the tg_stickers_search tool.
type StickersSearchResult struct {
	Count  int              `json:"count"`
	Sets   []StickerSetItem `json:"sets"`
	Output string           `json:"output"`
}

// NewStickersSearchHandler creates a handler for the tg_stickers_search tool.
func NewStickersSearchHandler(client telegram.Client) mcp.ToolHandlerFor[StickersSearchParams, StickersSearchResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params StickersSearchParams,
	) (*mcp.CallToolResult, StickersSearchResult, error) {
		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true}, StickersSearchResult{},
				validationErr(ErrQueryRequired)
		}

		sets, err := client.SearchStickerSets(ctx, params.Query)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, StickersSearchResult{},
				telegramErr("failed to search sticker sets", err)
		}

		var buf strings.Builder

		for _, set := range sets {
			fmt.Fprintf(&buf, "[%s] %s (%d stickers)\n", set.Name, set.Title, set.Count)
		}

		return nil, StickersSearchResult{
			Count:  len(sets),
			Sets:   stickerSetsToItems(sets),
			Output: buf.String(),
		}, nil
	}
}

// StickersSearchTool returns the MCP tool definition for tg_stickers_search.
func StickersSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_stickers_search",
		Description: "Search for Telegram sticker sets by keyword",
		Annotations: readOnlyAnnotations(),
	}
}

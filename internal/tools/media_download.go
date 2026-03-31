package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MediaDownloadParams defines the parameters for the tg_media_download tool.
type MediaDownloadParams struct {
	Peer      string  `json:"peer"                jsonschema:"Chat ID or @username"`
	MessageID int     `json:"messageId"           jsonschema:"Message ID containing the media"`
	OutputDir *string `json:"outputDir,omitempty" jsonschema:"Directory to save the downloaded file"`
}

// MediaDownloadResult is the output of the tg_media_download tool.
type MediaDownloadResult struct {
	FilePath string `json:"filePath"`
	Output   string `json:"output"`
}

// NewMediaDownloadHandler creates a handler for the tg_media_download tool.
func NewMediaDownloadHandler(client telegram.Client) mcp.ToolHandlerFor[MediaDownloadParams, MediaDownloadResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MediaDownloadParams,
	) (*mcp.CallToolResult, MediaDownloadResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MediaDownloadResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MediaDownloadResult{},
				validationErr(ErrMessageIDRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaDownloadResult{},
				telegramErr("failed to resolve peer", err)
		}

		msgs, err := client.GetMessages(ctx, peer, []int{params.MessageID})
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaDownloadResult{},
				telegramErr("failed to get message", err)
		}

		if len(msgs) == 0 {
			return &mcp.CallToolResult{IsError: true}, MediaDownloadResult{},
				telegramErr("failed to find message", ErrMessageNotFound)
		}

		outDir := deref(params.OutputDir)
		if outDir == "" {
			outDir = "."
		}

		filePath, err := client.DownloadMedia(ctx, &msgs[0], outDir)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaDownloadResult{},
				telegramErr("failed to download media", err)
		}

		return nil, MediaDownloadResult{
			FilePath: filePath,
			Output:   "Downloaded media to " + filePath,
		}, nil
	}
}

// MediaDownloadTool returns the MCP tool definition for tg_media_download.
func MediaDownloadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_media_download",
		Description: "Download media from a Telegram message to a local file",
		Annotations: readOnlyAnnotations(),
	}
}

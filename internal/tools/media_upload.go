package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MediaUploadParams defines the parameters for the tg_media_upload tool.
type MediaUploadParams struct {
	Path string `json:"path" jsonschema:"Local file path to upload"`
}

// MediaUploadResult is the output of the tg_media_upload tool.
type MediaUploadResult struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Output string `json:"output"`
}

// NewMediaUploadHandler creates a handler for the tg_media_upload tool.
func NewMediaUploadHandler(client telegram.Client) mcp.ToolHandlerFor[MediaUploadParams, MediaUploadResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MediaUploadParams,
	) (*mcp.CallToolResult, MediaUploadResult, error) {
		if params.Path == "" {
			return &mcp.CallToolResult{IsError: true}, MediaUploadResult{},
				validationErr(ErrPathRequired)
		}

		uploaded, err := client.UploadFile(ctx, params.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaUploadResult{},
				telegramErr("failed to upload file", err)
		}

		return nil, MediaUploadResult{
			Name:   uploaded.Name,
			Size:   uploaded.Size,
			Output: fmt.Sprintf("Uploaded %s (%d bytes)", uploaded.Name, uploaded.Size),
		}, nil
	}
}

// MediaUploadTool returns the MCP tool definition for tg_media_upload.
func MediaUploadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_media_upload",
		Description: "Upload a local file to Telegram and return its metadata",
		Annotations: idempotentAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MediaSendAlbumParams defines the parameters for the tg_media_send_album tool.
type MediaSendAlbumParams struct {
	Peer    string   `json:"peer"              jsonschema:"Chat ID or @username"`
	Paths   []string `json:"paths"             jsonschema:"Local file paths to send as album"`
	Caption *string  `json:"caption,omitempty" jsonschema:"Optional caption for the album"`
}

// MediaSendAlbumResult is the output of the tg_media_send_album tool.
type MediaSendAlbumResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewMediaSendAlbumHandler creates a handler for the tg_media_send_album tool.
func NewMediaSendAlbumHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MediaSendAlbumParams, MediaSendAlbumResult] {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		params MediaSendAlbumParams,
	) (*mcp.CallToolResult, MediaSendAlbumResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{},
				validationErr(ErrPeerRequired)
		}

		if len(params.Paths) == 0 {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{},
				validationErr(ErrPathsRequired)
		}

		msgs, err := sendAlbum(ctx, client, req, params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{}, err
		}

		return nil, MediaSendAlbumResult{
			Count:  len(msgs),
			Output: fmt.Sprintf("Sent album with %d file(s) to %s", len(msgs), params.Peer),
		}, nil
	}
}

func sendAlbum(
	ctx context.Context, client telegram.Client, req *mcp.CallToolRequest, params MediaSendAlbumParams,
) ([]telegram.Message, error) {
	token := req.Params.GetProgressToken()
	total := float64(len(params.Paths))

	for idx, filePath := range params.Paths {
		notifyProgress(ctx, req.Session, token, float64(idx), total, "Validating file paths")

		rootErr := validatePathAgainstRoots(ctx, req.Session, filePath)
		if rootErr != nil {
			return nil, validationErr(rootErr)
		}
	}

	notifyProgress(ctx, req.Session, token, total/2, total, "Resolving peer")

	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return nil, telegramErr("failed to resolve peer", err)
	}

	notifyProgress(ctx, req.Session, token, total, total, "Uploading files")

	msgs, err := client.SendAlbum(ctx, peer, params.Paths, deref(params.Caption))
	if err != nil {
		return nil, telegramErr("failed to send album", err)
	}

	return msgs, nil
}

// MediaSendAlbumTool returns the MCP tool definition for tg_media_send_album.
func MediaSendAlbumTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_media_send_album",
		Description: "Send multiple files as an album to a Telegram chat",
		Annotations: writeAnnotations(),
	}
}

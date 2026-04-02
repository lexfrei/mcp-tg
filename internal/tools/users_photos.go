package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UsersPhotosParams defines the parameters for the tg_users_get_photos tool.
type UsersPhotosParams struct {
	Peer  string `json:"peer"            jsonschema:"User ID or @username"`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum number of photos to return (default 10)"`
}

// UsersPhotosResult is the output of the tg_users_get_photos tool.
type UsersPhotosResult struct {
	Count  int         `json:"count"`
	Photos []PhotoItem `json:"photos"`
	Output string      `json:"output"`
}

// NewUsersPhotosHandler creates a handler for the tg_users_get_photos tool.
func NewUsersPhotosHandler(client telegram.Client) mcp.ToolHandlerFor[UsersPhotosParams, UsersPhotosResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params UsersPhotosParams,
	) (*mcp.CallToolResult, UsersPhotosResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, UsersPhotosResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersPhotosResult{},
				telegramErr("failed to resolve peer", err)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, UsersPhotosResult{},
				validationErr(limitErr)
		}

		photos, err := client.GetUserPhotos(ctx, peer, limit)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersPhotosResult{},
				telegramErr("failed to get user photos", err)
		}

		var buf strings.Builder

		for _, photo := range photos {
			fmt.Fprintf(&buf, "Photo ID: %d, Date: %s\n", photo.ID, formatTimestamp(photo.Date))
		}

		return nil, UsersPhotosResult{
			Count:  len(photos),
			Photos: photosToItems(photos),
			Output: buf.String(),
		}, nil
	}
}

// UsersPhotosTool returns the MCP tool definition for tg_users_get_photos.
func UsersPhotosTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_users_get_photos",
		Description: "Get profile photos of a Telegram user",
		Annotations: readOnlyAnnotations(),
	}
}

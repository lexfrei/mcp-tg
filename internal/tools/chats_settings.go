package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- tg_chats_set_photo ---

// ChatsSetPhotoParams defines the parameters for the tg_chats_set_photo tool.
type ChatsSetPhotoParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
	Path string `json:"path" jsonschema:"Local file path of the new photo"`
}

// ChatsSetPhotoResult is the output of the tg_chats_set_photo tool.
type ChatsSetPhotoResult struct {
	Peer   string `json:"peer"`
	Path   string `json:"path"`
	Output string `json:"output"`
}

// NewChatsSetPhotoHandler creates a handler for the tg_chats_set_photo tool.
func NewChatsSetPhotoHandler(client telegram.Client) mcp.ToolHandlerFor[ChatsSetPhotoParams, ChatsSetPhotoResult] {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		params ChatsSetPhotoParams,
	) (*mcp.CallToolResult, ChatsSetPhotoResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsSetPhotoResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Path == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsSetPhotoResult{},
				validationErr(ErrPathRequired)
		}

		rootErr := validatePathAgainstRoots(ctx, req.Session, params.Path)
		if rootErr != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetPhotoResult{},
				validationErr(rootErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetPhotoResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.SetChatPhoto(ctx, peer, params.Path)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetPhotoResult{},
				telegramErr("failed to set chat photo", err)
		}

		return nil, ChatsSetPhotoResult{
			Peer:   params.Peer,
			Path:   params.Path,
			Output: "Updated chat photo for " + params.Peer,
		}, nil
	}
}

// ChatsSetPhotoTool returns the MCP tool definition for tg_chats_set_photo.
func ChatsSetPhotoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_set_photo",
		Description: "Set the photo of a Telegram chat, group, or channel",
		Annotations: idempotentAnnotations(),
	}
}

// --- tg_chats_set_description ---

// ChatsSetDescriptionParams defines the parameters for the tg_chats_set_description tool.
type ChatsSetDescriptionParams struct {
	Peer  string `json:"peer"  jsonschema:"@username, t.me/ link, or numeric ID"`
	About string `json:"about" jsonschema:"New description text for the chat"`
}

// ChatsSetDescriptionResult is the output of the tg_chats_set_description tool.
type ChatsSetDescriptionResult struct {
	Peer   string `json:"peer"`
	About  string `json:"about"`
	Output string `json:"output"`
}

// NewChatsSetDescriptionHandler creates a handler for the tg_chats_set_description tool.
func NewChatsSetDescriptionHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ChatsSetDescriptionParams, ChatsSetDescriptionResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsSetDescriptionParams,
	) (*mcp.CallToolResult, ChatsSetDescriptionResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsSetDescriptionResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetDescriptionResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.SetChatAbout(ctx, peer, params.About)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetDescriptionResult{},
				telegramErr("failed to set chat description", err)
		}

		return nil, ChatsSetDescriptionResult{
			Peer:   params.Peer,
			About:  params.About,
			Output: "Updated description for " + params.Peer,
		}, nil
	}
}

// ChatsSetDescriptionTool returns the MCP tool definition for tg_chats_set_description.
func ChatsSetDescriptionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_set_description",
		Description: "Set the description (about text) of a Telegram chat, group, or channel",
		Annotations: idempotentAnnotations(),
	}
}

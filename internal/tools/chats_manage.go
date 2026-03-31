package tools

import (
	"context"
	"fmt"
	"strconv"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- tg_chats_archive ---

// ChatsArchiveParams defines the parameters for the tg_chats_archive tool.
type ChatsArchiveParams struct {
	Peer    string `json:"peer"    jsonschema:"Chat ID or @username"`
	Archive bool   `json:"archive" jsonschema:"True to archive, false to unarchive"`
}

// ChatsArchiveResult is the output of the tg_chats_archive tool.
type ChatsArchiveResult struct {
	Peer     string `json:"peer"`
	Archived bool   `json:"archived"`
	Output   string `json:"output"`
}

// NewChatsArchiveHandler creates a handler for the tg_chats_archive tool.
func NewChatsArchiveHandler(client telegram.Client) mcp.ToolHandlerFor[ChatsArchiveParams, ChatsArchiveResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsArchiveParams,
	) (*mcp.CallToolResult, ChatsArchiveResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsArchiveResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsArchiveResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.ArchiveChat(ctx, peer, params.Archive)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsArchiveResult{},
				telegramErr("failed to archive/unarchive chat", err)
		}

		action := "Archived"
		if !params.Archive {
			action = "Unarchived"
		}

		return nil, ChatsArchiveResult{
			Peer:     params.Peer,
			Archived: params.Archive,
			Output:   action + " chat " + params.Peer,
		}, nil
	}
}

// ChatsArchiveTool returns the MCP tool definition for tg_chats_archive.
func ChatsArchiveTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_archive",
		Description: "Archive or unarchive a Telegram chat",
	}
}

// --- tg_chats_mute ---

// ChatsMuteParams defines the parameters for the tg_chats_mute tool.
type ChatsMuteParams struct {
	Peer      string `json:"peer"      jsonschema:"Chat ID or @username"`
	MuteUntil int    `json:"muteUntil" jsonschema:"Unix timestamp until which to mute (0 to unmute)"`
}

// ChatsMuteResult is the output of the tg_chats_mute tool.
type ChatsMuteResult struct {
	Peer      string `json:"peer"`
	MuteUntil int    `json:"muteUntil"`
	Output    string `json:"output"`
}

// NewChatsMuteHandler creates a handler for the tg_chats_mute tool.
func NewChatsMuteHandler(client telegram.Client) mcp.ToolHandlerFor[ChatsMuteParams, ChatsMuteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsMuteParams,
	) (*mcp.CallToolResult, ChatsMuteResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsMuteResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsMuteResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.MuteChat(ctx, peer, params.MuteUntil)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsMuteResult{},
				telegramErr("failed to mute/unmute chat", err)
		}

		action := "Muted chat " + params.Peer + " until " + strconv.Itoa(params.MuteUntil)
		if params.MuteUntil == 0 {
			action = "Unmuted chat " + params.Peer
		}

		return nil, ChatsMuteResult{
			Peer:      params.Peer,
			MuteUntil: params.MuteUntil,
			Output:    action,
		}, nil
	}
}

// ChatsMuteTool returns the MCP tool definition for tg_chats_mute.
func ChatsMuteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_mute",
		Description: "Mute or unmute notifications for a Telegram chat",
	}
}

// --- tg_chats_delete ---

// ChatsDeleteParams defines the parameters for the tg_chats_delete tool.
type ChatsDeleteParams struct {
	Peer string `json:"peer" jsonschema:"Chat ID or @username to delete"`
}

// ChatsDeleteResult is the output of the tg_chats_delete tool.
type ChatsDeleteResult struct {
	Output string `json:"output"`
}

// NewChatsDeleteHandler creates a handler for the tg_chats_delete tool.
func NewChatsDeleteHandler(client telegram.Client) mcp.ToolHandlerFor[ChatsDeleteParams, ChatsDeleteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsDeleteParams,
	) (*mcp.CallToolResult, ChatsDeleteResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsDeleteResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsDeleteResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.DeleteChat(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsDeleteResult{},
				telegramErr("failed to delete chat", err)
		}

		return nil, ChatsDeleteResult{
			Output: fmt.Sprintf("Deleted chat %s and cleared history", params.Peer),
		}, nil
	}
}

// ChatsDeleteTool returns the MCP tool definition for tg_chats_delete.
func ChatsDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_delete",
		Description: "Delete a Telegram chat and clear its history",
	}
}

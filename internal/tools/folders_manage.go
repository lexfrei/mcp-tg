package tools

import (
	"context"
	"fmt"
	"strconv"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- tg_folders_create ---

// FoldersCreateParams defines the parameters for the tg_folders_create tool.
type FoldersCreateParams struct {
	Title string   `json:"title"           jsonschema:"Title for the new folder"`
	Peers []string `json:"peers,omitempty" jsonschema:"Chat IDs or @usernames to include"`
}

// FoldersCreateResult is the output of the tg_folders_create tool.
type FoldersCreateResult struct {
	FolderID int    `json:"folderId"`
	Title    string `json:"title"`
	Output   string `json:"output"`
}

// NewFoldersCreateHandler creates a handler for the tg_folders_create tool.
func NewFoldersCreateHandler(client telegram.Client) mcp.ToolHandlerFor[FoldersCreateParams, FoldersCreateResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params FoldersCreateParams,
	) (*mcp.CallToolResult, FoldersCreateResult, error) {
		if params.Title == "" {
			return &mcp.CallToolResult{IsError: true}, FoldersCreateResult{},
				validationErr(ErrTitleRequired)
		}

		peers, err := resolveUserPeers(ctx, client, params.Peers)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersCreateResult{},
				telegramErr("failed to resolve peers", err)
		}

		folder, err := client.CreateFolder(ctx, params.Title, peers)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersCreateResult{},
				telegramErr("failed to create folder", err)
		}

		folderID := 0
		title := params.Title

		if folder != nil {
			folderID = folder.ID
			title = folder.Title
		}

		return nil, FoldersCreateResult{
			FolderID: folderID,
			Title:    title,
			Output:   fmt.Sprintf("Created folder %q (ID: %d) with %d peer(s)", title, folderID, len(peers)),
		}, nil
	}
}

// FoldersCreateTool returns the MCP tool definition for tg_folders_create.
func FoldersCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_folders_create",
		Description: "Create a new Telegram chat folder",
	}
}

// --- tg_folders_edit ---

// FoldersEditParams defines the parameters for the tg_folders_edit tool.
type FoldersEditParams struct {
	FolderID int      `json:"folderId"        jsonschema:"ID of the folder to edit"`
	Title    string   `json:"title"           jsonschema:"New title for the folder"`
	Peers    []string `json:"peers,omitempty" jsonschema:"Chat IDs or @usernames to include"`
}

// FoldersEditResult is the output of the tg_folders_edit tool.
type FoldersEditResult struct {
	FolderID int    `json:"folderId"`
	Title    string `json:"title"`
	Output   string `json:"output"`
}

// NewFoldersEditHandler creates a handler for the tg_folders_edit tool.
func NewFoldersEditHandler(client telegram.Client) mcp.ToolHandlerFor[FoldersEditParams, FoldersEditResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params FoldersEditParams,
	) (*mcp.CallToolResult, FoldersEditResult, error) {
		if params.FolderID == 0 {
			return &mcp.CallToolResult{IsError: true}, FoldersEditResult{},
				validationErr(ErrFolderIDRequired)
		}

		if params.Title == "" {
			return &mcp.CallToolResult{IsError: true}, FoldersEditResult{},
				validationErr(ErrTitleRequired)
		}

		peers, err := resolveUserPeers(ctx, client, params.Peers)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersEditResult{},
				telegramErr("failed to resolve peers", err)
		}

		err = client.EditFolder(ctx, params.FolderID, params.Title, peers)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersEditResult{},
				telegramErr("failed to edit folder", err)
		}

		return nil, FoldersEditResult{
			FolderID: params.FolderID,
			Title:    params.Title,
			Output: "Updated folder " + strconv.Itoa(params.FolderID) +
				" to " + strconv.Quote(params.Title),
		}, nil
	}
}

// FoldersEditTool returns the MCP tool definition for tg_folders_edit.
func FoldersEditTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_folders_edit",
		Description: "Edit a Telegram chat folder's title and included peers",
	}
}

// --- tg_folders_delete ---

// FoldersDeleteParams defines the parameters for the tg_folders_delete tool.
type FoldersDeleteParams struct {
	FolderID int `json:"folderId" jsonschema:"ID of the folder to delete"`
}

// FoldersDeleteResult is the output of the tg_folders_delete tool.
type FoldersDeleteResult struct {
	FolderID int    `json:"folderId"`
	Output   string `json:"output"`
}

// NewFoldersDeleteHandler creates a handler for the tg_folders_delete tool.
func NewFoldersDeleteHandler(client telegram.Client) mcp.ToolHandlerFor[FoldersDeleteParams, FoldersDeleteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params FoldersDeleteParams,
	) (*mcp.CallToolResult, FoldersDeleteResult, error) {
		if params.FolderID == 0 {
			return &mcp.CallToolResult{IsError: true}, FoldersDeleteResult{},
				validationErr(ErrFolderIDRequired)
		}

		err := client.DeleteFolder(ctx, params.FolderID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersDeleteResult{},
				telegramErr("failed to delete folder", err)
		}

		return nil, FoldersDeleteResult{
			FolderID: params.FolderID,
			Output:   "Deleted folder " + strconv.Itoa(params.FolderID),
		}, nil
	}
}

// FoldersDeleteTool returns the MCP tool definition for tg_folders_delete.
func FoldersDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_folders_delete",
		Description: "Delete a Telegram chat folder",
	}
}

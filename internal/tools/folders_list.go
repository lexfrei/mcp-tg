package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FoldersListParams defines the parameters for the tg_folders_list tool.
type FoldersListParams struct{}

// FoldersListResult is the output of the tg_folders_list tool.
type FoldersListResult struct {
	Count   int          `json:"count"`
	Folders []FolderItem `json:"folders"`
	Output  string       `json:"output"`
}

// NewFoldersListHandler creates a handler for the tg_folders_list tool.
func NewFoldersListHandler(client telegram.Client) mcp.ToolHandlerFor[FoldersListParams, FoldersListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ FoldersListParams,
	) (*mcp.CallToolResult, FoldersListResult, error) {
		folders, err := client.GetFolders(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FoldersListResult{},
				telegramErr("failed to list folders", err)
		}

		var buf strings.Builder

		for idx := range folders {
			fmt.Fprintf(&buf, "%s\n", formatFolder(&folders[idx]))
		}

		return nil, FoldersListResult{
			Count:   len(folders),
			Folders: foldersToItems(folders),
			Output:  buf.String(),
		}, nil
	}
}

// FoldersListTool returns the MCP tool definition for tg_folders_list.
func FoldersListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_folders_list",
		Description: "List all Telegram chat folders",
		Annotations: readOnlyAnnotations(),
	}
}

// formatFolder renders one folder line: its chat count, then the
// chat-kind rules that add or hide chats beyond the listed ones.
func formatFolder(folder *telegram.Folder) string {
	line := fmt.Sprintf("[%d] %s (%d peers)", folder.ID, folder.Title, len(folder.Peers)+len(folder.PinnedPeers))

	if len(folder.Includes) > 0 {
		line += " +" + strings.Join(folder.Includes, ",")
	}

	if len(folder.Excludes) > 0 {
		line += " -" + strings.Join(folder.Excludes, ",")
	}

	if folder.Shared {
		line += " shared"
	}

	return line
}

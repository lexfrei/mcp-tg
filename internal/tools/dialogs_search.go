package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DialogsSearchParams defines the parameters for the tg_dialogs_search tool.
type DialogsSearchParams struct {
	Query string `json:"query" jsonschema:"Search query"`
}

// DialogsSearchResult is the output of the tg_dialogs_search tool.
type DialogsSearchResult struct {
	Count   int          `json:"count"`
	Dialogs []DialogItem `json:"dialogs"`
	Output  string       `json:"output"`
}

// NewDialogsSearchHandler creates a handler for the tg_dialogs_search tool.
func NewDialogsSearchHandler(client telegram.Client) mcp.ToolHandlerFor[DialogsSearchParams, DialogsSearchResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DialogsSearchParams,
	) (*mcp.CallToolResult, DialogsSearchResult, error) {
		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true}, DialogsSearchResult{},
				validationErr(ErrQueryRequired)
		}

		dialogs, err := client.SearchDialogs(ctx, params.Query)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DialogsSearchResult{},
				telegramErr("failed to search dialogs", err)
		}

		var buf strings.Builder

		for _, dlg := range dialogs {
			fmt.Fprintf(&buf, "%s (peer: %s)\n", formatDialog(&dlg), formatPeer(dlg.Peer))
		}

		return nil, DialogsSearchResult{
			Count:   len(dialogs),
			Dialogs: dialogsToItems(dialogs),
			Output:  buf.String(),
		}, nil
	}
}

// DialogsSearchTool returns the MCP tool definition for tg_dialogs_search.
func DialogsSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_dialogs_search",
		Description: "Search Telegram dialogs by query",
		Annotations: readOnlyAnnotations(),
	}
}

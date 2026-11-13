package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DraftsListParams defines the parameters for the tg_drafts_list tool.
type DraftsListParams struct{}

// DraftsListResult is the output of the tg_drafts_list tool.
type DraftsListResult struct {
	Count  int              `json:"count"`
	Drafts []telegram.Draft `json:"drafts"`
	Output string           `json:"output"`
}

// NewDraftsListHandler creates a handler for the tg_drafts_list tool.
func NewDraftsListHandler(client telegram.Client) mcp.ToolHandlerFor[DraftsListParams, DraftsListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ DraftsListParams,
	) (*mcp.CallToolResult, DraftsListResult, error) {
		drafts, err := client.GetAllDrafts(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DraftsListResult{}, telegramErr("failed to list drafts", err)
		}

		return nil, DraftsListResult{Count: len(drafts), Drafts: drafts, Output: formatDrafts(drafts)}, nil
	}
}

// DraftsListTool returns the MCP tool definition for tg_drafts_list.
func DraftsListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_drafts_list",
		Description: "List the unsent drafts saved in every Telegram chat",
		Annotations: readOnlyAnnotations(),
	}
}

func formatDrafts(drafts []telegram.Draft) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%d draft(s)", len(drafts))

	for idx := range drafts {
		draft := &drafts[idx]

		fmt.Fprintf(&buf, "\n---\nchat: %s\ndate: %s", formatPeerRef(draft.Name, draft.Username, draft.Peer),
			formatTimestamp(draft.Date))

		if draft.TopicID > 0 {
			fmt.Fprintf(&buf, "\ntopic: %d", draft.TopicID)
		}

		if draft.ReplyTo > 0 {
			fmt.Fprintf(&buf, "\nreply to: %d", draft.ReplyTo)
		}

		buf.WriteString("\ntext: " + draft.Text)
	}

	return buf.String()
}

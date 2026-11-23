package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DialogsListParams defines the parameters for the tg_dialogs_list tool.
type DialogsListParams struct {
	Limit      *int `json:"limit,omitempty"      jsonschema:"Maximum number of dialogs to return (default 100, at most 1000, paged past 100)"`
	OffsetDate *int `json:"offsetDate,omitempty" jsonschema:"Unix timestamp for pagination: the last dialog's lastMessageDate."`
}

// DialogsListResult is the output of the tg_dialogs_list tool.
type DialogsListResult struct {
	Count   int          `json:"count"`
	HasMore bool         `json:"hasMore"`
	Dialogs []DialogItem `json:"dialogs"`
	Output  string       `json:"output"`
}

// NewDialogsListHandler creates a handler for the tg_dialogs_list tool.
func NewDialogsListHandler(client telegram.Client) mcp.ToolHandlerFor[DialogsListParams, DialogsListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DialogsListParams,
	) (*mcp.CallToolResult, DialogsListResult, error) {
		limit := deref(params.Limit)

		limitErr := validateDialogsLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, DialogsListResult{},
				validationErr(limitErr)
		}

		opts := telegram.DialogOpts{
			Limit:      limit,
			OffsetDate: deref(params.OffsetDate),
		}

		dialogs, err := client.GetDialogs(ctx, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DialogsListResult{},
				telegramErr("failed to list dialogs", err)
		}

		items := make([]DialogItem, len(dialogs))

		var buf strings.Builder

		for idx, dlg := range dialogs {
			items[idx] = dialogToItem(&dlg)
			fmt.Fprintf(&buf, "%s\n", formatDialog(&dlg))
		}

		return nil, DialogsListResult{
			Count:   len(dialogs),
			HasMore: hasMorePage(len(dialogs), limit),
			Dialogs: items,
			Output:  buf.String(),
		}, nil
	}
}

// maxDialogsLimit caps the limit of the tools that page the dialog list. The
// wrapper pages past the server's 100, so the cap bounds the round-trips one
// call makes, the same way maxMessagesListLimit does.
const maxDialogsLimit = 1000

// validateDialogsLimit rejects a negative limit and one above the cap.
func validateDialogsLimit(limit int) error {
	err := validateLimit(limit)
	if err != nil {
		return err
	}

	if limit > maxDialogsLimit {
		return ErrLimitTooLarge
	}

	return nil
}

// DialogsListTool returns the MCP tool definition for tg_dialogs_list.
func DialogsListTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_dialogs_list",
		Description: "List all Telegram dialogs (chats, groups, channels). " +
			"Page with offsetDate set to the last dialog's lastMessageDate; the cursor is a timestamp alone, " +
			"so dialogs whose last message falls in that same second can be skipped or repeated between pages. " +
			"Pinned dialogs come first whatever their date: when limit is not above the number of pinned chats, " +
			"the last item can be an old pinned chat, and paging from it skips every newer unpinned dialog.",
		Annotations: readOnlyAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DialogsMarkUnreadParams defines parameters for tg_dialogs_mark_unread.
type DialogsMarkUnreadParams struct {
	Peer   string `json:"peer"   jsonschema:"@username, t.me/ link, or numeric ID"`
	Unread bool   `json:"unread" jsonschema:"true to mark unread, false to mark read"`
}

// DialogsMarkUnreadResult is the output of tg_dialogs_mark_unread.
type DialogsMarkUnreadResult struct {
	Output string `json:"output"`
}

// NewDialogsMarkUnreadHandler creates a handler for tg_dialogs_mark_unread.
func NewDialogsMarkUnreadHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[DialogsMarkUnreadParams, DialogsMarkUnreadResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DialogsMarkUnreadParams,
	) (*mcp.CallToolResult, DialogsMarkUnreadResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				DialogsMarkUnreadResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				DialogsMarkUnreadResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.MarkDialogUnread(ctx, peer, params.Unread)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				DialogsMarkUnreadResult{},
				telegramErr("failed to mark dialog unread", err)
		}

		action := "read"
		if params.Unread {
			action = "unread"
		}

		return nil, DialogsMarkUnreadResult{
			Output: fmt.Sprintf("Marked %s as %s", params.Peer, action),
		}, nil
	}
}

// DialogsMarkUnreadTool returns the tool definition for tg_dialogs_mark_unread.
func DialogsMarkUnreadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_dialogs_mark_unread",
		Description: "Mark a Telegram dialog as unread or read",
		Annotations: idempotentAnnotations(),
	}
}

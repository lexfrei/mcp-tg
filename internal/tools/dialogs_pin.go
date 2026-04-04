package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DialogsPinParams defines parameters for tg_dialogs_pin.
type DialogsPinParams struct {
	Peer   string `json:"peer"   jsonschema:"@username, t.me/ link, or numeric ID"`
	Pinned bool   `json:"pinned" jsonschema:"true to pin, false to unpin"`
}

// DialogsPinResult is the output of tg_dialogs_pin.
type DialogsPinResult struct {
	Output string `json:"output"`
}

// NewDialogsPinHandler creates a handler for tg_dialogs_pin.
func NewDialogsPinHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[DialogsPinParams, DialogsPinResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DialogsPinParams,
	) (*mcp.CallToolResult, DialogsPinResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				DialogsPinResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				DialogsPinResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.PinDialog(ctx, peer, params.Pinned)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				DialogsPinResult{},
				telegramErr("failed to toggle dialog pin", err)
		}

		action := actionUnpinned
		if params.Pinned {
			action = actionPinned
		}

		return nil, DialogsPinResult{
			Output: fmt.Sprintf("%s dialog %s", action, params.Peer),
		}, nil
	}
}

// DialogsPinTool returns the tool definition for tg_dialogs_pin.
func DialogsPinTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_dialogs_pin",
		Description: "Pin or unpin a Telegram dialog",
		Annotations: idempotentAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// GroupsSlowmodeParams defines parameters for tg_groups_slowmode.
type GroupsSlowmodeParams struct {
	Peer    string `json:"peer"    jsonschema:"@username, t.me/ link, or numeric ID"`
	Seconds int    `json:"seconds" jsonschema:"Slowmode delay in seconds (0 to disable)"`
}

// GroupsSlowmodeResult is the output of tg_groups_slowmode.
type GroupsSlowmodeResult struct {
	Output string `json:"output"`
}

// NewGroupsSlowmodeHandler creates a handler for tg_groups_slowmode.
func NewGroupsSlowmodeHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[GroupsSlowmodeParams, GroupsSlowmodeResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params GroupsSlowmodeParams,
	) (*mcp.CallToolResult, GroupsSlowmodeResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				GroupsSlowmodeResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				GroupsSlowmodeResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.SetSlowMode(ctx, peer, params.Seconds)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				GroupsSlowmodeResult{},
				telegramErr("failed to set slow mode", err)
		}

		return nil, GroupsSlowmodeResult{
			Output: fmt.Sprintf(
				"Slowmode set to %ds for %s", params.Seconds, params.Peer,
			),
		}, nil
	}
}

// GroupsSlowmodeTool returns the tool definition for tg_groups_slowmode.
func GroupsSlowmodeTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_groups_slowmode",
		Description: "Set slowmode delay for a Telegram group or channel",
		Annotations: idempotentAnnotations(),
	}
}

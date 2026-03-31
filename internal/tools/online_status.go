package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OnlineStatusSetParams defines the parameters for the tg_online_status_set tool.
type OnlineStatusSetParams struct {
	Online bool `json:"online" jsonschema:"True to appear online, false to go offline"`
}

// OnlineStatusSetResult is the output of the tg_online_status_set tool.
type OnlineStatusSetResult struct {
	Online bool   `json:"online"`
	Output string `json:"output"`
}

// NewOnlineStatusSetHandler creates a handler for the tg_online_status_set tool.
func NewOnlineStatusSetHandler(client telegram.Client) mcp.ToolHandlerFor[OnlineStatusSetParams, OnlineStatusSetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params OnlineStatusSetParams,
	) (*mcp.CallToolResult, OnlineStatusSetResult, error) {
		err := client.SetOnlineStatus(ctx, params.Online)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, OnlineStatusSetResult{},
				telegramErr("failed to set online status", err)
		}

		status := "offline"
		if params.Online {
			status = "online"
		}

		return nil, OnlineStatusSetResult{
			Online: params.Online,
			Output: "Status set to " + status,
		}, nil
	}
}

// OnlineStatusSetTool returns the MCP tool definition for tg_online_status_set.
func OnlineStatusSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_online_status_set",
		Description: "Set the authenticated user's online or offline status",
		Annotations: idempotentAnnotations(),
	}
}

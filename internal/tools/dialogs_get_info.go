package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DialogsGetInfoParams defines the parameters for the tg_dialogs_get_info tool.
type DialogsGetInfoParams struct {
	Peer string `json:"peer" jsonschema:"Chat ID or @username"`
}

// DialogsGetInfoResult is the output of the tg_dialogs_get_info tool.
type DialogsGetInfoResult struct {
	Title    string `json:"title"`
	Username string `json:"username"`
	About    string `json:"about"`
	Type     string `json:"type"`
	Output   string `json:"output"`
}

// NewDialogsGetInfoHandler creates a handler for the tg_dialogs_get_info tool.
func NewDialogsGetInfoHandler(client telegram.Client) mcp.ToolHandlerFor[DialogsGetInfoParams, DialogsGetInfoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DialogsGetInfoParams,
	) (*mcp.CallToolResult, DialogsGetInfoResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, DialogsGetInfoResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DialogsGetInfoResult{},
				telegramErr("failed to resolve peer", err)
		}

		info, err := client.GetPeerInfo(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DialogsGetInfoResult{},
				telegramErr("failed to get peer info", err)
		}

		return nil, DialogsGetInfoResult{
			Title:    info.Title,
			Username: info.Username,
			About:    info.About,
			Type:     info.Type,
			Output:   fmt.Sprintf("%s (@%s) [%s]: %s", info.Title, info.Username, info.Type, info.About),
		}, nil
	}
}

// DialogsGetInfoTool returns the MCP tool definition for tg_dialogs_get_info.
func DialogsGetInfoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_dialogs_get_info",
		Description: "Get metadata about a Telegram chat, group, or channel",
		Annotations: readOnlyAnnotations(),
	}
}

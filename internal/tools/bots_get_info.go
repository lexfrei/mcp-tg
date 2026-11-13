package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// BotsGetInfoParams defines the parameters for the tg_bots_get_info tool.
type BotsGetInfoParams struct {
	Peer string `json:"peer" jsonschema:"The bot: @username, t.me/ link, or numeric ID"`
}

// BotsGetInfoResult is the output of the tg_bots_get_info tool.
type BotsGetInfoResult struct {
	telegram.BotInfo

	Output string `json:"output"`
}

// NewBotsGetInfoHandler creates a handler for the tg_bots_get_info tool.
func NewBotsGetInfoHandler(client telegram.Client) mcp.ToolHandlerFor[BotsGetInfoParams, BotsGetInfoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params BotsGetInfoParams,
	) (*mcp.CallToolResult, BotsGetInfoResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, BotsGetInfoResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, BotsGetInfoResult{},
				telegramErr("failed to resolve peer", err)
		}

		info, err := client.GetBotInfo(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, BotsGetInfoResult{},
				telegramErr("failed to get bot info", err)
		}

		return nil, BotsGetInfoResult{BotInfo: *info, Output: formatBotInfo(info)}, nil
	}
}

// BotsGetInfoTool returns the MCP tool definition for tg_bots_get_info.
func BotsGetInfoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_bots_get_info",
		Description: "Get a Telegram bot's description and command menu",
		Annotations: readOnlyAnnotations(),
	}
}

func formatBotInfo(info *telegram.BotInfo) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "Bot %d", info.UserID)

	if info.Description != "" {
		buf.WriteString(": " + info.Description)
	}

	fmt.Fprintf(&buf, "\n%d command(s)", len(info.Commands))

	for _, cmd := range info.Commands {
		fmt.Fprintf(&buf, "\n/%s — %s", cmd.Command, cmd.Description)
	}

	return buf.String()
}

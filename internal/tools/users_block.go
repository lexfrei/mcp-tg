package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UsersBlockParams defines the parameters for the tg_users_block tool.
type UsersBlockParams struct {
	Peer  string `json:"peer"  jsonschema:"User ID or @username"`
	Block bool   `json:"block" jsonschema:"True to block, false to unblock"`
}

// UsersBlockResult is the output of the tg_users_block tool.
type UsersBlockResult struct {
	Output string `json:"output"`
}

// NewUsersBlockHandler creates a handler for the tg_users_block tool.
func NewUsersBlockHandler(client telegram.Client) mcp.ToolHandlerFor[UsersBlockParams, UsersBlockResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params UsersBlockParams,
	) (*mcp.CallToolResult, UsersBlockResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, UsersBlockResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersBlockResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.BlockUser(ctx, peer, params.Block)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersBlockResult{},
				telegramErr("failed to block/unblock user", err)
		}

		action := "Blocked"
		if !params.Block {
			action = "Unblocked"
		}

		return nil, UsersBlockResult{
			Output: fmt.Sprintf("%s user %s", action, params.Peer),
		}, nil
	}
}

// UsersBlockTool returns the MCP tool definition for tg_users_block.
func UsersBlockTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_users_block",
		Description: "Block or unblock a Telegram user",
		Annotations: destructiveAnnotations(),
	}
}

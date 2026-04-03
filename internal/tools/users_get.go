package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// UsersGetParams defines the parameters for the tg_users_get tool.
type UsersGetParams struct {
	Peer string `json:"peer" jsonschema:"@username (preferred) or numeric user ID"`
}

// UsersGetResult is the output of the tg_users_get tool.
type UsersGetResult struct {
	UserID    int64  `json:"userId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Username  string `json:"username"`
	Bot       bool   `json:"bot"`
	Bio       string `json:"bio"`
	Online    bool   `json:"online"`
	Output    string `json:"output"`
}

// NewUsersGetHandler creates a handler for the tg_users_get tool.
func NewUsersGetHandler(client telegram.Client) mcp.ToolHandlerFor[UsersGetParams, UsersGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params UsersGetParams,
	) (*mcp.CallToolResult, UsersGetResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, UsersGetResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersGetResult{},
				telegramErr("failed to resolve peer", err)
		}

		user, err := client.GetUser(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, UsersGetResult{},
				telegramErr("failed to get user", err)
		}

		return nil, UsersGetResult{
			UserID:    user.ID,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Username:  user.Username,
			Bot:       user.Bot,
			Bio:       user.Bio,
			Online:    user.Online,
			Output:    fmt.Sprintf("User: %s (ID: %d)", formatUserName(user), user.ID),
		}, nil
	}
}

// UsersGetTool returns the MCP tool definition for tg_users_get.
func UsersGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_users_get",
		Description: "Get detailed information about a Telegram user",
		Annotations: readOnlyAnnotations(),
	}
}

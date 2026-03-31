package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ProfileGetParams defines the parameters for the tg_profile_get tool.
type ProfileGetParams struct{}

// ProfileGetResult is the output of the tg_profile_get tool.
type ProfileGetResult struct {
	UserID    int64  `json:"userId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Username  string `json:"username"`
	Phone     string `json:"phone"`
	Bio       string `json:"bio"`
	Output    string `json:"output"`
}

// NewProfileGetHandler creates a handler for the tg_profile_get tool.
func NewProfileGetHandler(client telegram.Client) mcp.ToolHandlerFor[ProfileGetParams, ProfileGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ ProfileGetParams,
	) (*mcp.CallToolResult, ProfileGetResult, error) {
		user, err := client.GetSelf(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ProfileGetResult{},
				telegramErr("failed to get profile", err)
		}

		return nil, ProfileGetResult{
			UserID:    user.ID,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Username:  user.Username,
			Phone:     user.Phone,
			Bio:       user.Bio,
			Output:    formatUserName(user),
		}, nil
	}
}

// ProfileGetTool returns the MCP tool definition for tg_profile_get.
func ProfileGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_profile_get",
		Description: "Get the authenticated user's Telegram profile information",
	}
}

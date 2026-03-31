package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContactsGetParams defines the parameters for the tg_contacts_get tool.
type ContactsGetParams struct {
	Peer string `json:"peer" jsonschema:"Chat ID or @username of the contact"`
}

// ContactsGetResult is the output of the tg_contacts_get tool.
type ContactsGetResult struct {
	UserID    int64  `json:"userId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Username  string `json:"username"`
	Phone     string `json:"phone"`
	Bio       string `json:"bio"`
	Output    string `json:"output"`
}

// NewContactsGetHandler creates a handler for the tg_contacts_get tool.
func NewContactsGetHandler(client telegram.Client) mcp.ToolHandlerFor[ContactsGetParams, ContactsGetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ContactsGetParams,
	) (*mcp.CallToolResult, ContactsGetResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ContactsGetResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ContactsGetResult{},
				telegramErr("failed to resolve peer", err)
		}

		user, err := client.GetContact(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ContactsGetResult{},
				telegramErr("failed to get contact", err)
		}

		return nil, ContactsGetResult{
			UserID:    user.ID,
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Username:  user.Username,
			Phone:     user.Phone,
			Bio:       user.Bio,
			Output:    fmt.Sprintf("Contact: %s (ID: %d)", formatUserName(user), user.ID),
		}, nil
	}
}

// ContactsGetTool returns the MCP tool definition for tg_contacts_get.
func ContactsGetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_get",
		Description: "Get information about a Telegram contact",
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContactsListBlockedParams defines parameters for tg_contacts_list_blocked.
type ContactsListBlockedParams struct {
	Limit *int `json:"limit,omitempty" jsonschema:"Maximum results (default 100)"`
}

// ContactsListBlockedResult is the output of tg_contacts_list_blocked.
type ContactsListBlockedResult struct {
	Count  int        `json:"count"`
	Users  []UserItem `json:"users"`
	Output string     `json:"output"`
}

// NewContactsListBlockedHandler creates a handler for tg_contacts_list_blocked.
func NewContactsListBlockedHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ContactsListBlockedParams, ContactsListBlockedResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ContactsListBlockedParams,
	) (*mcp.CallToolResult, ContactsListBlockedResult, error) {
		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true},
				ContactsListBlockedResult{},
				validationErr(limitErr)
		}

		users, err := client.GetBlockedContacts(ctx, limit)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				ContactsListBlockedResult{},
				telegramErr("failed to get blocked contacts", err)
		}

		var buf strings.Builder

		for idx := range users {
			fmt.Fprintf(
				&buf, "[%d] %s\n",
				users[idx].ID, formatUserName(&users[idx]),
			)
		}

		return nil, ContactsListBlockedResult{
			Count:  len(users),
			Users:  usersToItems(users),
			Output: buf.String(),
		}, nil
	}
}

// ContactsListBlockedTool returns the tool definition.
func ContactsListBlockedTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_list_blocked",
		Description: "List blocked Telegram users",
		Annotations: readOnlyAnnotations(),
	}
}

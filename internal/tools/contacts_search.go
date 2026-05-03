package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContactsSearchParams defines the parameters for the tg_contacts_search tool.
type ContactsSearchParams struct {
	Query string `json:"query"           jsonschema:"Search query for contacts"`
	Limit *int   `json:"limit,omitempty" jsonschema:"Maximum number of results (default 50)"`
}

// ContactsSearchResult is the output of the tg_contacts_search tool.
type ContactsSearchResult struct {
	Count   int        `json:"count"`
	HasMore bool       `json:"hasMore"`
	Users   []UserItem `json:"users"`
	Output  string     `json:"output"`
}

// NewContactsSearchHandler creates a handler for the tg_contacts_search tool.
func NewContactsSearchHandler(client telegram.Client) mcp.ToolHandlerFor[ContactsSearchParams, ContactsSearchResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ContactsSearchParams,
	) (*mcp.CallToolResult, ContactsSearchResult, error) {
		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true}, ContactsSearchResult{},
				validationErr(ErrQueryRequired)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, ContactsSearchResult{},
				validationErr(limitErr)
		}

		users, err := client.SearchContacts(ctx, params.Query, limit)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ContactsSearchResult{},
				telegramErr("failed to search contacts", err)
		}

		var buf strings.Builder

		for idx := range users {
			fmt.Fprintf(&buf, "[%d] %s\n", users[idx].ID, formatUserName(&users[idx]))
		}

		return nil, ContactsSearchResult{
			Count:   len(users),
			HasMore: hasMorePage(len(users), limit),
			Users:   usersToItems(users),
			Output:  buf.String(),
		}, nil
	}
}

// ContactsSearchTool returns the MCP tool definition for tg_contacts_search.
func ContactsSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_search",
		Description: "Search Telegram contacts by query",
		Annotations: readOnlyAnnotations(),
	}
}

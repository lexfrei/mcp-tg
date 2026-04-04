package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContactsGetStatusesParams defines parameters for tg_contacts_get_statuses.
type ContactsGetStatusesParams struct{}

// ContactStatusItem is a structured contact status entry for JSON results.
type ContactStatusItem struct {
	UserID   int64  `json:"userId"`
	Status   string `json:"status"`
	LastSeen int    `json:"lastSeen,omitempty"`
}

// ContactsGetStatusesResult is the output of tg_contacts_get_statuses.
type ContactsGetStatusesResult struct {
	Count    int                 `json:"count"`
	Statuses []ContactStatusItem `json:"statuses"`
	Output   string              `json:"output"`
}

// NewContactsGetStatusesHandler creates a handler for tg_contacts_get_statuses.
func NewContactsGetStatusesHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ContactsGetStatusesParams, ContactsGetStatusesResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		_ ContactsGetStatusesParams,
	) (*mcp.CallToolResult, ContactsGetStatusesResult, error) {
		statuses, err := client.GetContactStatuses(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				ContactsGetStatusesResult{},
				telegramErr("failed to get contact statuses", err)
		}

		items := contactStatusesToItems(statuses)

		var buf strings.Builder

		for idx := range statuses {
			fmt.Fprintf(
				&buf, "[%d] %s\n",
				statuses[idx].UserID, statuses[idx].Status,
			)
		}

		return nil, ContactsGetStatusesResult{
			Count:    len(statuses),
			Statuses: items,
			Output:   buf.String(),
		}, nil
	}
}

func contactStatusesToItems(
	statuses []telegram.ContactStatus,
) []ContactStatusItem {
	items := make([]ContactStatusItem, len(statuses))

	for idx := range statuses {
		items[idx] = ContactStatusItem{
			UserID:   statuses[idx].UserID,
			Status:   statuses[idx].Status,
			LastSeen: statuses[idx].LastSeen,
		}
	}

	return items
}

// ContactsGetStatusesTool returns the tool definition.
func ContactsGetStatusesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_get_statuses",
		Description: "Get online statuses of Telegram contacts",
		Annotations: readOnlyAnnotations(),
	}
}

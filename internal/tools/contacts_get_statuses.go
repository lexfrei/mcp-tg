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
//
// Name and Username mirror the shape every other peer-bearing JSON
// entry uses. They stay empty today because ContactsGetStatuses
// (MTProto) does not return a parallel Users[] array — a follow-up
// can wire a batched UsersGetUsers lookup to populate them without
// touching the schema again.
type ContactStatusItem struct {
	UserID   int64  `json:"userId"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
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
			ref := formatPeerRef(
				statuses[idx].Name,
				statuses[idx].Username,
				telegram.InputPeer{Type: telegram.PeerUser, ID: statuses[idx].UserID},
			)
			fmt.Fprintf(&buf, "%s %s\n", ref, statuses[idx].Status)
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
			Name:     statuses[idx].Name,
			Username: statuses[idx].Username,
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

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContactsAddParams defines parameters for tg_contacts_add.
type ContactsAddParams struct {
	Peer      string  `json:"peer"               jsonschema:"@username, t.me/ link, or numeric ID"`
	FirstName string  `json:"firstName"          jsonschema:"Contact first name"`
	LastName  *string `json:"lastName,omitempty" jsonschema:"Contact last name"`
	Phone     *string `json:"phone,omitempty"    jsonschema:"Contact phone number"`
}

// ContactsAddResult is the output of tg_contacts_add.
type ContactsAddResult struct {
	Output string `json:"output"`
}

// NewContactsAddHandler creates a handler for tg_contacts_add.
func NewContactsAddHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ContactsAddParams, ContactsAddResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ContactsAddParams,
	) (*mcp.CallToolResult, ContactsAddResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				ContactsAddResult{},
				validationErr(ErrPeerRequired)
		}

		if params.FirstName == "" {
			return &mcp.CallToolResult{IsError: true},
				ContactsAddResult{},
				validationErr(ErrFirstNameRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				ContactsAddResult{},
				telegramErr("failed to resolve peer", err)
		}

		if peer.Type != telegram.PeerUser {
			return &mcp.CallToolResult{IsError: true},
				ContactsAddResult{},
				validationErr(ErrUserPeerRequired)
		}

		err = client.AddContact(
			ctx, peer, params.FirstName,
			deref(params.LastName), deref(params.Phone),
		)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				ContactsAddResult{},
				telegramErr("failed to add contact", err)
		}

		return nil, ContactsAddResult{
			Output: fmt.Sprintf("Added %s as contact", params.Peer),
		}, nil
	}
}

// ContactsAddTool returns the tool definition for tg_contacts_add.
func ContactsAddTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_add",
		Description: "Add a user to Telegram contacts",
		Annotations: writeAnnotations(),
	}
}

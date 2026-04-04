package tools

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContactsDeleteParams defines parameters for tg_contacts_delete.
type ContactsDeleteParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// ContactsDeleteResult is the output of tg_contacts_delete.
type ContactsDeleteResult struct {
	Peer    string `json:"peer"`
	Deleted bool   `json:"deleted"`
	Output  string `json:"output"`
}

// NewContactsDeleteHandler creates a handler for tg_contacts_delete.
func NewContactsDeleteHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ContactsDeleteParams, ContactsDeleteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ContactsDeleteParams,
	) (*mcp.CallToolResult, ContactsDeleteResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				ContactsDeleteResult{},
				validationErr(ErrPeerRequired)
		}

		return executeDeleteContact(ctx, client, params.Peer)
	}
}

func executeDeleteContact(
	ctx context.Context,
	client telegram.Client,
	peerID string,
) (*mcp.CallToolResult, ContactsDeleteResult, error) {
	peer, err := client.ResolvePeer(ctx, peerID)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			ContactsDeleteResult{},
			telegramErr("failed to resolve peer", err)
	}

	if peer.Type != telegram.PeerUser {
		return &mcp.CallToolResult{IsError: true},
			ContactsDeleteResult{},
			validationErr(errors.New(
				"contacts operations require a user peer, not a group or channel",
			))
	}

	err = client.DeleteContact(ctx, peer)
	if err != nil {
		return &mcp.CallToolResult{IsError: true},
			ContactsDeleteResult{},
			telegramErr("failed to delete contact", err)
	}

	return nil, ContactsDeleteResult{
		Peer:    peerID,
		Deleted: true,
		Output:  "Deleted contact " + peerID,
	}, nil
}

// ContactsDeleteTool returns the tool definition for tg_contacts_delete.
func ContactsDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_contacts_delete",
		Description: "Remove a user from Telegram contacts",
		Annotations: destructiveAnnotations(),
	}
}

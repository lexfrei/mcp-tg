package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesDeleteParams defines the parameters for the tg_messages_delete tool.
type MessagesDeleteParams struct {
	Peer      string `json:"peer"                jsonschema:"@username, t.me/ link, or numeric ID"`
	IDs       []int  `json:"ids"                 jsonschema:"Message IDs to delete"`
	Revoke    *bool  `json:"revoke,omitempty"    jsonschema:"Delete for everyone (default true); published messages only"`
	Scheduled *bool  `json:"scheduled,omitempty" jsonschema:"Delete from the scheduled queue instead of the published history"`
}

// MessagesDeleteResult is the output of the tg_messages_delete tool.
type MessagesDeleteResult struct {
	Deleted int    `json:"deleted"`
	Output  string `json:"output"`
}

// NewMessagesDeleteHandler creates a handler for the tg_messages_delete tool.
//
// Scheduled messages have their own ID namespace, which overlaps with the
// published history: message 22 in the queue and message 22 in the channel
// are different messages. The scheduled flag therefore picks the RPC, not
// just the wording of the result — sending a scheduled ID to the history
// RPC deletes whatever is published under that number.
func NewMessagesDeleteHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesDeleteParams, MessagesDeleteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesDeleteParams,
	) (*mcp.CallToolResult, MessagesDeleteResult, error) {
		validErr := validateDeleteParams(&params)
		if validErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				validationErr(validErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				telegramErr("failed to resolve peer", err)
		}

		scheduled := deref(params.Scheduled)

		if scheduled {
			err = client.DeleteScheduledMessages(ctx, peer, params.IDs)
		} else {
			err = client.DeleteMessages(ctx, peer, params.IDs, revokeOrDefault(params.Revoke))
		}

		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				telegramErr("failed to delete messages", err)
		}

		return nil, MessagesDeleteResult{
			Deleted: len(params.IDs),
			Output:  fmt.Sprintf("Deleted %d %s", len(params.IDs), deletedKind(scheduled)),
		}, nil
	}
}

// validateDeleteParams runs the request-shape checks that need no network
// round-trip, so a malformed call fails before any RPC.
func validateDeleteParams(params *MessagesDeleteParams) error {
	if params.Peer == "" {
		return ErrPeerRequired
	}

	if len(params.IDs) == 0 {
		return ErrMessageIDRequired
	}

	return validateIDCount(params.IDs)
}

// revokeOrDefault reports whether the deletion is for everyone, defaulting
// to true when the caller says nothing.
func revokeOrDefault(revoke *bool) bool {
	if revoke == nil {
		return true
	}

	return *revoke
}

// deletedKind names the queue in the result line, so a caller reading only
// the output text can tell which of the two deletions ran.
func deletedKind(scheduled bool) string {
	if scheduled {
		return "scheduled message(s)"
	}

	return "message(s)"
}

// MessagesDeleteTool returns the MCP tool definition for tg_messages_delete.
func MessagesDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_delete",
		Description: "Delete messages from a Telegram chat. Targets the published history; " +
			"pass scheduled=true to delete from the scheduled queue instead " +
			"(the two are separate ID namespaces that overlap)",
		Annotations: destructiveAnnotations(),
	}
}

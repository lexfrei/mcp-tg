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
	// Deleted is the server's own affected-count for the scheduled queue:
	// an id already gone from the queue contributes nothing to it, so a
	// caller can tell a genuine removal from a no-op without a follow-up
	// read. It is omitted for an ordinary history delete: Telegram gives
	// no equivalent per-request signal there — messages.affectedMessages
	// carries a PtsCount, but that field belongs to the update-sequence
	// mechanism, not to a count of what this call actually removed (see
	// Wrapper.DeleteMessages), so a caller who needs confirmation for a
	// history delete has to read the messages back rather than trust a
	// number this tool cannot honestly provide.
	Deleted *int   `json:"deleted,omitempty"`
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
			affected, deleteErr := client.DeleteScheduledMessages(ctx, peer, params.IDs)
			if deleteErr != nil {
				return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
					telegramErr("failed to delete messages", deleteErr)
			}

			return nil, MessagesDeleteResult{
				Deleted: &affected,
				Output:  fmt.Sprintf("Deleted %d scheduled message(s)", affected),
			}, nil
		}

		deleteErr := client.DeleteMessages(ctx, peer, params.IDs, revokeOrDefault(params.Revoke))
		if deleteErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesDeleteResult{},
				telegramErr("failed to delete messages", deleteErr)
		}

		return nil, MessagesDeleteResult{
			Output: fmt.Sprintf("Requested deletion of %d message(s)", len(params.IDs)),
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

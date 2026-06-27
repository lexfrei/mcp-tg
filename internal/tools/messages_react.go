package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesReactParams defines the parameters for the tg_messages_react tool.
type MessagesReactParams struct {
	Peer      string   `json:"peer"             jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int      `json:"messageId"        jsonschema:"ID of the message to react to"`
	Emoji     string   `json:"emoji,omitempty"  jsonschema:"Single reaction: emoji or custom:<document_id>; use 'emojis' for multiple"`
	Emojis    []string `json:"emojis,omitempty" jsonschema:"Set multiple reactions at once; each emoji or custom:<document_id> (Premium)"`
	Big       *bool    `json:"big,omitempty"    jsonschema:"Play the large animated reaction"`
	Remove    *bool    `json:"remove,omitempty" jsonschema:"Remove all reactions instead of adding"`
}

// MessagesReactResult is the output of the tg_messages_react tool.
type MessagesReactResult struct {
	Output string `json:"output"`
}

// NewMessagesReactHandler creates a handler for the tg_messages_react tool.
func NewMessagesReactHandler(client telegram.Client) mcp.ToolHandlerFor[MessagesReactParams, MessagesReactResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesReactParams,
	) (*mcp.CallToolResult, MessagesReactResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				validationErr(ErrPeerRequired)
		}

		if params.MessageID == 0 {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				validationErr(ErrMessageIDRequired)
		}

		remove := deref(params.Remove)
		emojis := reactionEmojiList(params.Emoji, params.Emojis)

		if !remove {
			reactErr := validateReactions(emojis)
			if reactErr != nil {
				return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
					validationErr(reactErr)
			}
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.SendReaction(ctx, peer, params.MessageID, telegram.ReactionOpts{
			Emojis: emojis,
			Big:    deref(params.Big),
			Remove: remove,
		})
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesReactResult{},
				telegramErr("failed to send reaction", err)
		}

		return nil, MessagesReactResult{
			Output: reactionOutput(remove, emojis, params.MessageID),
		}, nil
	}
}

// reactionEmojiList prefers the multi-reaction list and falls back to the
// single emoji for backward compatibility.
func reactionEmojiList(single string, multi []string) []string {
	if len(multi) > 0 {
		return multi
	}

	if single != "" {
		return []string{single}
	}

	return nil
}

// validateReactions ensures the list is non-empty and every entry is a
// well-formed reaction (a standard emoji or "custom:<document_id>"). Catching
// malformed input here keeps the failure classified as a validation error
// rather than surfacing later as a failed send RPC.
func validateReactions(emojis []string) error {
	if len(emojis) == 0 {
		return ErrEmojiRequired
	}

	for _, emoji := range emojis {
		if emoji == "" {
			return ErrEmojiRequired
		}

		err := telegram.ValidateReactionString(emoji)
		if err != nil {
			return errors.Wrapf(err, "invalid reaction %q", emoji)
		}
	}

	return nil
}

func reactionOutput(remove bool, emojis []string, msgID int) string {
	if remove {
		return fmt.Sprintf("Removed reactions on message %d", msgID)
	}

	noun := "reaction"
	if len(emojis) > 1 {
		noun = "reactions"
	}

	return fmt.Sprintf("Added %s %s on message %d", noun, strings.Join(emojis, " "), msgID)
}

// MessagesReactTool returns the MCP tool definition for tg_messages_react.
func MessagesReactTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_messages_react",
		Description: "Add or remove reactions on a Telegram message. Supports standard and " +
			"custom (premium) emoji, several reactions at once, and the big animated reaction.",
		Annotations: idempotentAnnotations(),
	}
}

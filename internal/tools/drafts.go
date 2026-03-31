package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- tg_drafts_set ---

// DraftsSetParams defines the parameters for the tg_drafts_set tool.
type DraftsSetParams struct {
	Peer    string `json:"peer"              jsonschema:"Chat ID or @username"`
	Text    string `json:"text"              jsonschema:"Draft message text"`
	ReplyTo *int   `json:"replyTo,omitempty" jsonschema:"Message ID to reply to"`
}

// DraftsSetResult is the output of the tg_drafts_set tool.
type DraftsSetResult struct {
	Peer   string `json:"peer"`
	Output string `json:"output"`
}

// NewDraftsSetHandler creates a handler for the tg_drafts_set tool.
func NewDraftsSetHandler(client telegram.Client) mcp.ToolHandlerFor[DraftsSetParams, DraftsSetResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DraftsSetParams,
	) (*mcp.CallToolResult, DraftsSetResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, DraftsSetResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Text == "" {
			return &mcp.CallToolResult{IsError: true}, DraftsSetResult{},
				validationErr(ErrTextRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DraftsSetResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.SetDraft(ctx, peer, params.Text, deref(params.ReplyTo))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DraftsSetResult{},
				telegramErr("failed to set draft", err)
		}

		return nil, DraftsSetResult{
			Peer:   params.Peer,
			Output: "Draft set in " + params.Peer,
		}, nil
	}
}

// DraftsSetTool returns the MCP tool definition for tg_drafts_set.
func DraftsSetTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_drafts_set",
		Description: "Set a draft message in a Telegram chat",
		Annotations: idempotentAnnotations(),
	}
}

// --- tg_drafts_clear ---

// DraftsClearParams defines the parameters for the tg_drafts_clear tool.
type DraftsClearParams struct {
	Peer string `json:"peer" jsonschema:"Chat ID or @username"`
}

// DraftsClearResult is the output of the tg_drafts_clear tool.
type DraftsClearResult struct {
	Peer   string `json:"peer"`
	Output string `json:"output"`
}

// NewDraftsClearHandler creates a handler for the tg_drafts_clear tool.
func NewDraftsClearHandler(client telegram.Client) mcp.ToolHandlerFor[DraftsClearParams, DraftsClearResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DraftsClearParams,
	) (*mcp.CallToolResult, DraftsClearResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, DraftsClearResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DraftsClearResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.ClearDraft(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DraftsClearResult{},
				telegramErr("failed to clear draft", err)
		}

		return nil, DraftsClearResult{
			Peer:   params.Peer,
			Output: "Cleared draft in " + params.Peer,
		}, nil
	}
}

// DraftsClearTool returns the MCP tool definition for tg_drafts_clear.
func DraftsClearTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_drafts_clear",
		Description: "Clear the draft message in a Telegram chat",
		Annotations: idempotentAnnotations(),
	}
}

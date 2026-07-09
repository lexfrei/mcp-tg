package tools

import (
	"context"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatsSetSendAsParams defines the parameters for the tg_chats_set_send_as tool.
type ChatsSetSendAsParams struct {
	Peer   string  `json:"peer"             jsonschema:"Supergroup or channel: @username, t.me/ link, or numeric ID"`
	SendAs *string `json:"sendAs,omitempty" jsonschema:"Identity to post as by default; omit to reset to your own account"`
}

// ChatsSetSendAsResult is the output of the tg_chats_set_send_as tool.
type ChatsSetSendAsResult struct {
	Output string `json:"output"`
}

// NewChatsSetSendAsHandler creates a handler for the tg_chats_set_send_as tool.
func NewChatsSetSendAsHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ChatsSetSendAsParams, ChatsSetSendAsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsSetSendAsParams,
	) (*mcp.CallToolResult, ChatsSetSendAsResult, error) {
		peer, err := resolveSendAsChat(ctx, client, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetSendAsResult{}, err
		}

		// An omitted identity is not a missing argument here: it is the
		// only way to hand the chat back to the account itself.
		sendAs, err := resolveSendAs(ctx, client, deref(params.SendAs))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetSendAsResult{}, err
		}

		err = client.SetDefaultSendAs(ctx, peer, sendAs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsSetSendAsResult{},
				telegramErr("failed to set the default send-as identity", err)
		}

		return nil, ChatsSetSendAsResult{Output: sendAsSetOutput(deref(params.SendAs))}, nil
	}
}

// sendAsSetOutput names the identity by the reference the caller gave.
// saveDefaultSendAs returns no display name, and re-reading one would
// cost a round trip to tell the caller something they just typed.
func sendAsSetOutput(ref string) string {
	if ref == "" {
		return "This chat now posts as yourself"
	}

	return "This chat now posts as " + collapseLineBreaks(ref)
}

// ChatsSetSendAsTool returns the MCP tool definition for tg_chats_set_send_as.
func ChatsSetSendAsTool() *mcp.Tool {
	return &mcp.Tool{
		Name: toolSetSendAs,
		Description: "Set the identity this account posts under by default in a supergroup. " +
			"The default also governs reactions and poll votes, which cannot name an identity per call. " +
			"This is account-wide server state: it is visible in every Telegram client, survives restarts, " +
			"and affects other callers of this server. Prefer the per-call sendAs argument where it suffices. " +
			"Omit sendAs to reset the chat to your own account.",
		Annotations: idempotentAnnotations(),
	}
}

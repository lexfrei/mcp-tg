package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatsGetSendAsParams defines the parameters for the tg_chats_get_send_as tool.
type ChatsGetSendAsParams struct {
	Peer string `json:"peer" jsonschema:"Supergroup or channel: @username, t.me/ link, or numeric ID"`
}

// SendAsItem is one identity the account may post under in a chat.
//
// Peer is the bot-API form of the identity, ready to be handed straight
// back as the sendAs argument of a send tool. The embedded fields keep
// the {id, type, name, username} shape every other peer in the tool
// surface uses.
type SendAsItem struct {
	ParticipantItem

	Peer            string `json:"peer"`
	PremiumRequired bool   `json:"premiumRequired,omitempty"`
}

// Tool names, referenced from each other's descriptions.
const (
	toolGetSendAs = "tg_chats_get_send_as"
	toolSetSendAs = "tg_chats_set_send_as"
)

// ChatsGetSendAsResult is the output of the tg_chats_get_send_as tool.
type ChatsGetSendAsResult struct {
	Count      int          `json:"count"`
	Identities []SendAsItem `json:"identities"`
	Output     string       `json:"output"`
}

// NewChatsGetSendAsHandler creates a handler for the tg_chats_get_send_as tool.
func NewChatsGetSendAsHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ChatsGetSendAsParams, ChatsGetSendAsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsGetSendAsParams,
	) (*mcp.CallToolResult, ChatsGetSendAsResult, error) {
		peer, err := resolveSendAsChat(ctx, client, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsGetSendAsResult{}, err
		}

		options, err := client.GetSendAs(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsGetSendAsResult{},
				telegramErr("failed to list send-as identities", err)
		}

		return nil, ChatsGetSendAsResult{
			Count:      len(options),
			Identities: sendAsItems(options),
			Output:     formatSendAsOptions(options),
		}, nil
	}
}

// resolveSendAsChat resolves a chat that can carry send-as identities.
// Users and legacy basic groups cannot, and the server says so with a
// CHANNEL_INVALID that explains nothing, so they fail before the call.
func resolveSendAsChat(
	ctx context.Context, client telegram.Client, ref string,
) (telegram.InputPeer, error) {
	if ref == "" {
		return telegram.InputPeer{}, validationErr(ErrPeerRequired)
	}

	peer, err := client.ResolvePeer(ctx, ref)
	if err != nil {
		return telegram.InputPeer{}, telegramErr("failed to resolve peer", err)
	}

	if peer.Type != telegram.PeerChannel {
		return telegram.InputPeer{}, validationErr(telegram.ErrSendAsUnsupportedPeer)
	}

	return peer, nil
}

func sendAsItems(options []telegram.SendAsOption) []SendAsItem {
	items := make([]SendAsItem, len(options))

	for idx, opt := range options {
		items[idx] = SendAsItem{
			ParticipantItem: ParticipantItem{
				ID:       opt.Peer.ID,
				Type:     participantTypeLabel(opt.Peer.Type),
				Name:     opt.Name,
				Username: opt.Username,
			},
			Peer:            formatPeer(opt.Peer),
			PremiumRequired: opt.PremiumRequired,
		}
	}

	return items
}

func formatSendAsOptions(options []telegram.SendAsOption) string {
	var out strings.Builder

	fmt.Fprintf(&out, "%d send-as identity(ies):\n", len(options))

	for _, opt := range options {
		out.WriteString(formatPeerRef(opt.Name, opt.Username, opt.Peer))

		if opt.PremiumRequired {
			out.WriteString(" — Telegram Premium required")
		}

		out.WriteString("\n")
	}

	return out.String()
}

// ChatsGetSendAsTool returns the MCP tool definition for tg_chats_get_send_as.
func ChatsGetSendAsTool() *mcp.Tool {
	return &mcp.Tool{
		Name: toolGetSendAs,
		Description: "List the identities this account may post under in a supergroup or channel. " +
			"Pass one back as the sendAs argument of a send tool, or to " + toolSetSendAs + ".",
		Annotations: readOnlyAnnotations(),
	}
}

// Package prompts provides MCP prompt templates for common Telegram operations.
package prompts

import (
	"context"
	"fmt"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultContextMessages = 10

// Register adds all Telegram prompts to the MCP server.
func Register(server *mcp.Server, client telegram.Client) {
	server.AddPrompt(replyPrompt(), replyHandler(client))
	server.AddPrompt(summarizePrompt(), summarizeHandler(client))
	server.AddPrompt(searchAndReplyPrompt(), searchAndReplyHandler(client))
}

func replyPrompt() *mcp.Prompt {
	return &mcp.Prompt{
		Name:        "reply_to_message",
		Description: "Get context around a message to compose a reply",
		Arguments: []*mcp.PromptArgument{
			{Name: "peer", Description: "Chat ID or @username", Required: true},
			{Name: "messageId", Description: "Message ID to reply to", Required: true},
		},
	}
}

func summarizePrompt() *mcp.Prompt {
	return &mcp.Prompt{
		Name:        "summarize_chat",
		Description: "Get recent messages from a chat for summarization",
		Arguments: []*mcp.PromptArgument{
			{Name: "peer", Description: "Chat ID or @username", Required: true},
		},
	}
}

func searchAndReplyPrompt() *mcp.Prompt {
	return &mcp.Prompt{
		Name:        "search_and_reply",
		Description: "Search for messages and prepare to reply",
		Arguments: []*mcp.PromptArgument{
			{Name: "peer", Description: "Chat ID or @username", Required: true},
			{Name: "query", Description: "Search query", Required: true},
		},
	}
}

func replyHandler(client telegram.Client) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		peerStr := req.Params.Arguments["peer"]
		msgIDStr := req.Params.Arguments["messageId"]

		if peerStr == "" || msgIDStr == "" {
			return nil, errors.New("peer and messageId are required")
		}

		peer, err := client.ResolvePeer(ctx, peerStr)
		if err != nil {
			return nil, errors.Wrap(err, "resolving peer")
		}

		msgs, _, err := client.GetHistory(ctx, peer, telegram.HistoryOpts{
			Limit: defaultContextMessages,
		})
		if err != nil {
			return nil, errors.Wrap(err, "getting message context")
		}

		var buf strings.Builder

		fmt.Fprintf(&buf, "Recent messages in %s:\n\n", peerStr)

		for idx := range msgs {
			fmt.Fprintf(&buf, "[%d] %s\n", msgs[idx].ID, msgs[idx].Text)
		}

		fmt.Fprintf(&buf, "\nPlease compose a reply to message %s.", msgIDStr)

		return &mcp.GetPromptResult{
			Description: "Reply to message " + msgIDStr + " in " + peerStr,
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: buf.String()}},
			},
		}, nil
	}
}

func summarizeHandler(client telegram.Client) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		peerStr := req.Params.Arguments["peer"]
		if peerStr == "" {
			return nil, errors.New("peer is required")
		}

		peer, err := client.ResolvePeer(ctx, peerStr)
		if err != nil {
			return nil, errors.Wrap(err, "resolving peer")
		}

		msgs, _, err := client.GetHistory(ctx, peer, telegram.HistoryOpts{Limit: 100})
		if err != nil {
			return nil, errors.Wrap(err, "getting messages")
		}

		var buf strings.Builder

		fmt.Fprintf(&buf, "Recent messages in %s:\n\n", peerStr)

		for idx := range msgs {
			fmt.Fprintf(&buf, "[%d] %s\n", msgs[idx].ID, msgs[idx].Text)
		}

		buf.WriteString("\nPlease summarize this conversation.")

		return &mcp.GetPromptResult{
			Description: "Summarize conversation in " + peerStr,
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: buf.String()}},
			},
		}, nil
	}
}

func searchAndReplyHandler(client telegram.Client) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		peerStr := req.Params.Arguments["peer"]
		query := req.Params.Arguments["query"]

		if peerStr == "" || query == "" {
			return nil, errors.New("peer and query are required")
		}

		peer, err := client.ResolvePeer(ctx, peerStr)
		if err != nil {
			return nil, errors.Wrap(err, "resolving peer")
		}

		msgs, err := client.SearchMessages(ctx, peer, query, telegram.SearchOpts{Limit: defaultContextMessages})
		if err != nil {
			return nil, errors.Wrap(err, "searching messages")
		}

		var buf strings.Builder

		fmt.Fprintf(&buf, "Search results for %q in %s:\n\n", query, peerStr)

		for idx := range msgs {
			fmt.Fprintf(&buf, "[%d] %s\n", msgs[idx].ID, msgs[idx].Text)
		}

		buf.WriteString("\nPlease compose a reply based on these search results.")

		return &mcp.GetPromptResult{
			Description: "Search and reply in " + peerStr,
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: buf.String()}},
			},
		}, nil
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TopicsSearchParams defines the parameters for the tg_topics_search tool.
type TopicsSearchParams struct {
	Peer  string `json:"peer"  jsonschema:"Forum supergroup ID or @username"`
	Query string `json:"query" jsonschema:"Search query to filter topics by title"`
}

// TopicsSearchResult is the output of the tg_topics_search tool.
type TopicsSearchResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewTopicsSearchHandler creates a handler for the tg_topics_search tool.
func NewTopicsSearchHandler(client telegram.Client) mcp.ToolHandlerFor[TopicsSearchParams, TopicsSearchResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TopicsSearchParams,
	) (*mcp.CallToolResult, TopicsSearchResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, TopicsSearchResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true}, TopicsSearchResult{},
				validationErr(ErrQueryRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsSearchResult{},
				telegramErr("failed to resolve peer", err)
		}

		opts := telegram.TopicOpts{Query: params.Query}

		topics, err := client.GetForumTopics(ctx, peer, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsSearchResult{},
				telegramErr("failed to search topics", err)
		}

		var buf strings.Builder

		for idx, topic := range topics {
			fmt.Fprintf(&buf, "%d. [%d] %s (icon: %s)\n",
				idx+1, topic.ID, topic.Title, topic.Icon)
		}

		return nil, TopicsSearchResult{
			Count:  len(topics),
			Output: buf.String(),
		}, nil
	}
}

// TopicsSearchTool returns the MCP tool definition for tg_topics_search.
func TopicsSearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_topics_search",
		Description: "Search forum topics by title in a Telegram supergroup",
		Annotations: readOnlyAnnotations(),
	}
}

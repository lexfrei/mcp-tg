package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TopicsListParams defines the parameters for the tg_topics_list tool.
type TopicsListParams struct {
	Peer  string  `json:"peer"            jsonschema:"Forum supergroup ID or @username"`
	Limit *int    `json:"limit,omitempty" jsonschema:"Maximum number of topics to return"`
	Query *string `json:"query,omitempty" jsonschema:"Optional text to filter topics by title"`
}

// TopicsListResult is the output of the tg_topics_list tool.
type TopicsListResult struct {
	Count  int    `json:"count"`
	Output string `json:"output"`
}

// NewTopicsListHandler creates a handler for the tg_topics_list tool.
func NewTopicsListHandler(client telegram.Client) mcp.ToolHandlerFor[TopicsListParams, TopicsListResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TopicsListParams,
	) (*mcp.CallToolResult, TopicsListResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, TopicsListResult{},
				validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsListResult{},
				telegramErr("failed to resolve peer", err)
		}

		limit := deref(params.Limit)

		limitErr := validateLimit(limit)
		if limitErr != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsListResult{},
				validationErr(limitErr)
		}

		opts := telegram.TopicOpts{
			Limit: limit,
			Query: deref(params.Query),
		}

		topics, err := client.GetForumTopics(ctx, peer, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsListResult{},
				telegramErr("failed to list topics", err)
		}

		var buf strings.Builder

		for _, topic := range topics {
			fmt.Fprintf(&buf, "[%d] %s (icon: %s, date: %s)\n",
				topic.ID, topic.Title, topic.Icon, formatTimestamp(topic.Date))
		}

		return nil, TopicsListResult{
			Count:  len(topics),
			Output: buf.String(),
		}, nil
	}
}

// TopicsListTool returns the MCP tool definition for tg_topics_list.
func TopicsListTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_topics_list",
		Description: "List forum topics in a Telegram supergroup",
		Annotations: readOnlyAnnotations(),
	}
}

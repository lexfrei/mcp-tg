package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TopicsEditParams defines parameters for tg_topics_edit.
type TopicsEditParams struct {
	Peer    string `json:"peer"    jsonschema:"@username, t.me/ link, or numeric ID"`
	TopicID int    `json:"topicId" jsonschema:"Topic ID to edit"`
	Title   string `json:"title"   jsonschema:"New topic title"`
}

// TopicsEditResult is the output of tg_topics_edit.
type TopicsEditResult struct {
	Output string `json:"output"`
}

// NewTopicsEditHandler creates a handler for tg_topics_edit.
func NewTopicsEditHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[TopicsEditParams, TopicsEditResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TopicsEditParams,
	) (*mcp.CallToolResult, TopicsEditResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true},
				TopicsEditResult{},
				validationErr(ErrPeerRequired)
		}

		if params.TopicID == 0 {
			return &mcp.CallToolResult{IsError: true},
				TopicsEditResult{},
				validationErr(ErrTopicIDRequired)
		}

		if params.Title == "" {
			return &mcp.CallToolResult{IsError: true},
				TopicsEditResult{},
				validationErr(ErrTitleRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				TopicsEditResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.EditForumTopic(
			ctx, peer, params.TopicID, params.Title,
		)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				TopicsEditResult{},
				telegramErr("failed to edit forum topic", err)
		}

		return nil, TopicsEditResult{
			Output: fmt.Sprintf(
				"Edited topic %d in %s", params.TopicID, params.Peer,
			),
		}, nil
	}
}

// TopicsEditTool returns the tool definition for tg_topics_edit.
func TopicsEditTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_topics_edit",
		Description: "Edit a forum topic title in a Telegram supergroup",
		Annotations: writeAnnotations(),
	}
}

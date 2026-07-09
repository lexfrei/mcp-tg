package tools

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TopicsCreateParams defines parameters for tg_topics_create.
type TopicsCreateParams struct {
	Peer   string  `json:"peer"             jsonschema:"@username, t.me/ link, or numeric ID"`
	Title  string  `json:"title"            jsonschema:"Topic title"`
	SendAs *string `json:"sendAs,omitempty" jsonschema:"Create as this channel; see tg_chats_get_send_as. Omit to create as yourself"`
}

// TopicsCreateResult is the output of tg_topics_create.
type TopicsCreateResult struct {
	TopicID int    `json:"topicId"`
	Title   string `json:"title"`
	Output  string `json:"output"`
}

// NewTopicsCreateHandler creates a handler for tg_topics_create.
func NewTopicsCreateHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[TopicsCreateParams, TopicsCreateResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TopicsCreateParams,
	) (*mcp.CallToolResult, TopicsCreateResult, error) {
		vErr := validateTopicsCreateParams(&params)
		if vErr != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsCreateResult{}, validationErr(vErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				TopicsCreateResult{},
				telegramErr("failed to resolve peer", err)
		}

		sendAs, err := resolveSendAs(ctx, client, deref(params.SendAs))
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsCreateResult{}, err
		}

		topic, err := client.CreateForumTopic(ctx, peer, params.Title, sendAs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true},
				TopicsCreateResult{},
				telegramErr("failed to create forum topic", err)
		}

		if topic == nil {
			return &mcp.CallToolResult{IsError: true},
				TopicsCreateResult{},
				telegramErr("topic created but could not extract info from response",
					errors.New("nil topic in response"))
		}

		return nil, TopicsCreateResult{
			TopicID: topic.ID,
			Title:   topic.Title,
			Output: fmt.Sprintf(
				"Created topic %q (ID: %d)", topic.Title, topic.ID,
			),
		}, nil
	}
}

func validateTopicsCreateParams(params *TopicsCreateParams) error {
	if params.Peer == "" {
		return ErrPeerRequired
	}

	if params.Title == "" {
		return ErrTitleRequired
	}

	return nil
}

// TopicsCreateTool returns the tool definition for tg_topics_create.
func TopicsCreateTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_topics_create",
		Description: "Create a forum topic in a Telegram supergroup",
		Annotations: writeAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ChatsDeletePhotoParams defines the parameters for the tg_chats_delete_photo tool.
type ChatsDeletePhotoParams struct {
	Peer string `json:"peer" jsonschema:"@username, t.me/ link, or numeric ID"`
}

// ChatsDeletePhotoResult is the output of the tg_chats_delete_photo tool.
type ChatsDeletePhotoResult struct {
	Output string `json:"output"`
}

// NewChatsDeletePhotoHandler creates a handler for the tg_chats_delete_photo tool.
func NewChatsDeletePhotoHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[ChatsDeletePhotoParams, ChatsDeletePhotoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params ChatsDeletePhotoParams,
	) (*mcp.CallToolResult, ChatsDeletePhotoResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, ChatsDeletePhotoResult{}, validationErr(ErrPeerRequired)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsDeletePhotoResult{},
				telegramErr("failed to resolve peer", err)
		}

		err = client.DeleteChatPhoto(ctx, peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, ChatsDeletePhotoResult{},
				telegramErr("failed to delete the chat photo", err)
		}

		return nil, ChatsDeletePhotoResult{Output: "Chat photo removed"}, nil
	}
}

// ChatsDeletePhotoTool returns the MCP tool definition for tg_chats_delete_photo.
func ChatsDeletePhotoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_chats_delete_photo",
		Description: "Remove the photo of a Telegram group or channel",
		Annotations: destructiveAnnotations(),
	}
}

// TopicsDeleteParams defines the parameters for the tg_topics_delete tool.
type TopicsDeleteParams struct {
	Peer    string `json:"peer"    jsonschema:"Forum supergroup: @username, t.me/ link, or numeric ID"`
	TopicID int    `json:"topicId" jsonschema:"ID of the topic to delete"`
}

// TopicsDeleteResult is the output of the tg_topics_delete tool.
type TopicsDeleteResult struct {
	Output string `json:"output"`
}

// NewTopicsDeleteHandler creates a handler for the tg_topics_delete tool.
func NewTopicsDeleteHandler(client telegram.Client) mcp.ToolHandlerFor[TopicsDeleteParams, TopicsDeleteResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TopicsDeleteParams,
	) (*mcp.CallToolResult, TopicsDeleteResult, error) {
		err := deleteTopic(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TopicsDeleteResult{}, err
		}

		return nil, TopicsDeleteResult{Output: fmt.Sprintf("Deleted topic %d and its messages", params.TopicID)}, nil
	}
}

func deleteTopic(ctx context.Context, client telegram.Client, params *TopicsDeleteParams) error {
	if params.Peer == "" {
		return validationErr(ErrPeerRequired)
	}

	if params.TopicID <= 0 {
		return validationErr(ErrTopicIDRequired)
	}

	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return telegramErr("failed to resolve peer", err)
	}

	err = client.DeleteForumTopic(ctx, peer, params.TopicID)
	if err != nil {
		return telegramErr("failed to delete the topic", err)
	}

	return nil
}

// TopicsDeleteTool returns the MCP tool definition for tg_topics_delete.
func TopicsDeleteTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_topics_delete",
		Description: "Delete a forum topic in a Telegram supergroup together with all its messages",
		Annotations: destructiveAnnotations(),
	}
}

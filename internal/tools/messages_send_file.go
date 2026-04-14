package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MessagesSendFileParams defines the parameters for the tg_messages_send_file tool.
type MessagesSendFileParams struct {
	Peer         string  `json:"peer"                   jsonschema:"@username, t.me/ link, or numeric ID"`
	Path         string  `json:"path"                   jsonschema:"Local file path to send"`
	Caption      *string `json:"caption,omitempty"      jsonschema:"Optional caption for the file"`
	TopicID      *int    `json:"topicId,omitempty"      jsonschema:"Forum topic ID to send into"`
	ParseMode    *string `json:"parseMode,omitempty"    jsonschema:"Caption formatting: '' plain; 'commonmark' or 'markdown' alias"`
	Silent       *bool   `json:"silent,omitempty"       jsonschema:"Send without notification sound"`
	ScheduleDate *int    `json:"scheduleDate,omitempty" jsonschema:"Unix timestamp for scheduled delivery"`
}

// MessagesSendFileResult is the output of the tg_messages_send_file tool.
type MessagesSendFileResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewMessagesSendFileHandler creates a handler for the tg_messages_send_file tool.
func NewMessagesSendFileHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesSendFileParams, MessagesSendFileResult] {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		params MessagesSendFileParams,
	) (*mcp.CallToolResult, MessagesSendFileResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				validationErr(ErrPeerRequired)
		}

		if params.Path == "" {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				validationErr(ErrPathRequired)
		}

		pmErr := validateParseMode(deref(params.ParseMode))
		if pmErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{},
				validationErr(pmErr)
		}

		msg, err := uploadAndSendFile(ctx, client, req, params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesSendFileResult{}, err
		}

		msgID := 0
		if msg != nil {
			msgID = msg.ID
		}

		return nil, MessagesSendFileResult{
			MessageID: msgID,
			Output:    fmt.Sprintf("File sent to %s (message ID: %d)", params.Peer, msgID),
		}, nil
	}
}

func uploadAndSendFile(
	ctx context.Context, client telegram.Client, req *mcp.CallToolRequest, params MessagesSendFileParams,
) (*telegram.Message, error) {
	rootErr := validatePathAgainstRoots(ctx, req.Session, params.Path)
	if rootErr != nil {
		return nil, validationErr(rootErr)
	}

	token := req.Params.GetProgressToken()
	notifyProgress(ctx, req.Session, token, 0, 1, "Resolving peer")

	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return nil, telegramErr("failed to resolve peer", err)
	}

	notifyProgress(ctx, req.Session, token, 0, 1, "Uploading file")

	opts := telegram.SendOpts{
		TopicID:      deref(params.TopicID),
		ParseMode:    normalizeParseMode(deref(params.ParseMode)),
		Silent:       deref(params.Silent),
		ScheduleDate: deref(params.ScheduleDate),
	}

	msg, err := client.SendFile(ctx, peer, params.Path, deref(params.Caption), opts)
	if err != nil {
		return nil, telegramErr("failed to send file", err)
	}

	return msg, nil
}

// MessagesSendFileTool returns the MCP tool definition for tg_messages_send_file.
func MessagesSendFileTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_send_file",
		Description: "Send a file to a Telegram chat",
		Annotations: writeAnnotations(),
	}
}

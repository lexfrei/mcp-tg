package tools

import (
	"context"
	"fmt"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MediaSendAlbumParams defines the parameters for the tg_media_send_album tool.
type MediaSendAlbumParams struct {
	Peer         string   `json:"peer"                   jsonschema:"@username, t.me/ link, or numeric ID"`
	Paths        []string `json:"paths"                  jsonschema:"Local file paths to send as album"`
	Caption      *string  `json:"caption,omitempty"      jsonschema:"Optional caption for the album"`
	TopicID      *int     `json:"topicId,omitempty"      jsonschema:"Forum topic ID to send into"`
	ParseMode    string   `json:"parseMode"              jsonschema:"Caption: 'plain' (no formatting) or 'commonmark' subset"`
	Silent       *bool    `json:"silent,omitempty"       jsonschema:"Send without notification sound"`
	ScheduleDate *int     `json:"scheduleDate,omitempty" jsonschema:"Unix timestamp for scheduled delivery"`
	SendAs       *string  `json:"sendAs,omitempty"       jsonschema:"Post as this channel; see tg_chats_get_send_as. Omit for chat default"`

	// AllowRawMarkdown skips the plain-mode markdown lint.
	AllowRawMarkdown *bool `json:"allowRawMarkdown,omitempty" jsonschema:"Send markdown-looking caption characters literally in plain mode"`
}

// MediaSendAlbumResult is the output of the tg_media_send_album tool.
//
// EntitiesParsed sums the caption entities across the album's echoed
// messages; present even at 0.
type MediaSendAlbumResult struct {
	Count          int    `json:"count"`
	EntitiesParsed int    `json:"entitiesParsed"`
	Output         string `json:"output"`
}

// NewMediaSendAlbumHandler creates a handler for the tg_media_send_album tool.
func NewMediaSendAlbumHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MediaSendAlbumParams, MediaSendAlbumResult] {
	return func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		params MediaSendAlbumParams,
	) (*mcp.CallToolResult, MediaSendAlbumResult, error) {
		if params.Peer == "" {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{},
				validationErr(ErrPeerRequired)
		}

		if len(params.Paths) == 0 {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{},
				validationErr(ErrPathsRequired)
		}

		pmErr := validateParseMode(params.ParseMode)
		if pmErr != nil {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{},
				validationErr(pmErr)
		}

		lintErr := validatePlainText(
			normalizeParseMode(params.ParseMode), deref(params.AllowRawMarkdown), deref(params.Caption))
		if lintErr != nil {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{},
				validationErr(lintErr)
		}

		msgs, err := sendAlbum(ctx, client, req, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaSendAlbumResult{}, err
		}

		return nil, MediaSendAlbumResult{
			Count:          len(msgs),
			EntitiesParsed: entityCountAll(msgs),
			Output:         fmt.Sprintf("Sent album with %d file(s) to %s", len(msgs), params.Peer),
		}, nil
	}
}

func sendAlbum(
	ctx context.Context, client telegram.Client, req *mcp.CallToolRequest, params *MediaSendAlbumParams,
) ([]telegram.Message, error) {
	token := req.Params.GetProgressToken()

	for _, filePath := range params.Paths {
		rootErr := validatePathAgainstRoots(ctx, req.Session, filePath)
		if rootErr != nil {
			return nil, validationErr(rootErr)
		}
	}

	notifyProgress(ctx, req.Session, token, 0, 1, "Resolving peer")

	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return nil, telegramErr("failed to resolve peer", err)
	}

	topicErr := validateTopicID(ctx, client, peer, deref(params.TopicID))
	if topicErr != nil {
		return nil, validationErr(topicErr)
	}

	sendAs, err := resolveSendAs(ctx, client, deref(params.SendAs))
	if err != nil {
		return nil, err
	}

	fwd := newProgressForwarder(req.Session, token, "Uploading album")

	opts := telegram.SendOpts{
		TopicID:      deref(params.TopicID),
		ParseMode:    normalizeParseMode(params.ParseMode),
		Silent:       deref(params.Silent),
		ScheduleDate: deref(params.ScheduleDate),
		SendAs:       sendAs,
		Progress:     fwd.callback(),
	}

	msgs, err := client.SendAlbum(ctx, peer, params.Paths, deref(params.Caption), opts)
	if err != nil {
		return nil, sendErr("failed to send album", err, opts.SendAs)
	}

	fwd.done(ctx, "Album sent")

	return msgs, nil
}

// MediaSendAlbumTool returns the MCP tool definition for tg_media_send_album.
func MediaSendAlbumTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_media_send_album",
		Description: "Send multiple files as an album to a Telegram chat. " +
			"The optional caption is attached to the FIRST file only — Telegram " +
			"renders one caption per album, not per item. " +
			"(caption parseMode is required: 'plain' or 'commonmark')",
		InputSchema: inputSchemaWithEnum[MediaSendAlbumParams]("parseMode", parseModeEnum()),
		Annotations: writeAnnotations(),
	}
}

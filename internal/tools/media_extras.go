package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// viewMaxBytes caps what tg_media_view will load into a tool result. The
// image travels base64-encoded inside the response, so it is kept to a size
// a model context takes without strain.
const viewMaxBytes = 5 << 20

// outputSent is the output of a send whose echo carried no message.
const outputSent = "Sent"

// ErrBotRequired is returned when an inline query names no bot.
var ErrBotRequired = errors.New("bot is required")

// ErrQueryIDRequired is returned when an inline send has no query ID.
var ErrQueryIDRequired = errors.New("queryId is required: pass the one tg_inline_query returned")

// ErrResultIDRequired is returned when an inline send has no result ID.
var ErrResultIDRequired = errors.New("resultId is required")

// MediaSendVoiceParams defines the parameters for the tg_media_send_voice tool.
type MediaSendVoiceParams struct {
	Peer      string  `json:"peer"                jsonschema:"@username, t.me/ link, or numeric ID"`
	Path      string  `json:"path"                jsonschema:"Local audio file (OGG/Opus plays inline), or a video file for a video note"`
	VideoNote *bool   `json:"videoNote,omitempty" jsonschema:"Send a round video note instead of a voice message (default false)"`
	Duration  int     `json:"duration,omitempty"  jsonschema:"Length in seconds, shown in the player (default 0)"`
	TopicID   *int    `json:"topicId,omitempty"   jsonschema:"Forum topic ID to send into"`
	ReplyTo   *int    `json:"replyTo,omitempty"   jsonschema:"Message ID to reply to"`
	Silent    *bool   `json:"silent,omitempty"    jsonschema:"Send without notification sound"`
	SendAs    *string `json:"sendAs,omitempty"    jsonschema:"Post as this channel; see tg_chats_get_send_as. Omit for the chat default"`
}

// MediaSendVoiceResult is the output of the tg_media_send_voice tool.
type MediaSendVoiceResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewMediaSendVoiceHandler creates a handler for the tg_media_send_voice tool.
func NewMediaSendVoiceHandler(
	client telegram.Client, fileRoots []string,
) mcp.ToolHandlerFor[MediaSendVoiceParams, MediaSendVoiceResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MediaSendVoiceParams,
	) (*mcp.CallToolResult, MediaSendVoiceResult, error) {
		peer, opts, err := prepareVoiceSend(ctx, client, &params, fileRoots)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaSendVoiceResult{}, err
		}

		msg, err := client.SendVoice(ctx, peer, params.Path, &opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaSendVoiceResult{},
				sendErr("failed to send voice message", err, opts.Send.SendAs)
		}

		result := MediaSendVoiceResult{Output: outputSent}
		if msg != nil {
			result.MessageID = msg.ID
			result.Output = fmt.Sprintf("Sent as message %d", msg.ID)
		}

		return nil, result, nil
	}
}

func prepareVoiceSend(
	ctx context.Context, client telegram.Client, params *MediaSendVoiceParams, fileRoots []string,
) (telegram.InputPeer, telegram.VoiceOpts, error) {
	if params.Peer == "" {
		return telegram.InputPeer{}, telegram.VoiceOpts{}, validationErr(ErrPeerRequired)
	}

	if params.Path == "" {
		return telegram.InputPeer{}, telegram.VoiceOpts{}, validationErr(ErrPathRequired)
	}

	err := validatePathAgainstRoots(fileRoots, params.Path)
	if err != nil {
		return telegram.InputPeer{}, telegram.VoiceOpts{}, validationErr(err)
	}

	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return telegram.InputPeer{}, telegram.VoiceOpts{}, telegramErr("failed to resolve peer", err)
	}

	send, err := resolveSimpleSendOpts(ctx, client, peer, params.TopicID, params.ReplyTo, params.Silent, params.SendAs)
	if err != nil {
		return telegram.InputPeer{}, telegram.VoiceOpts{}, err
	}

	return peer, telegram.VoiceOpts{VideoNote: deref(params.VideoNote), Duration: params.Duration, Send: send}, nil
}

// resolveSimpleSendOpts builds SendOpts for the media sends that take topic,
// reply, silent and sendAs but no text.
func resolveSimpleSendOpts(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer,
	topicID, replyTo *int, silent *bool, sendAs *string,
) (telegram.SendOpts, error) {
	opts := telegram.SendOpts{TopicID: deref(topicID), ReplyTo: deref(replyTo), Silent: deref(silent)}

	err := validateTopicID(ctx, client, peer, opts.TopicID)
	if err != nil {
		return telegram.SendOpts{}, err
	}

	opts.SendAs, err = resolveSendAs(ctx, client, deref(sendAs))
	if err != nil {
		return telegram.SendOpts{}, err
	}

	return opts, nil
}

// MediaSendVoiceTool returns the MCP tool definition for tg_media_send_voice.
func MediaSendVoiceTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_media_send_voice",
		Description: "Send a local audio file as a Telegram voice message, or a video file as a round video note",
		Annotations: writeAnnotations(),
	}
}

// MediaGetInfoParams defines the parameters for the tg_media_get_info tool.
type MediaGetInfoParams struct {
	Peer      string `json:"peer"      jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of the message carrying the media"`
}

// MediaGetInfoResult is the output of the tg_media_get_info tool.
type MediaGetInfoResult struct {
	telegram.MediaInfo

	Output string `json:"output"`
}

// NewMediaGetInfoHandler creates a handler for the tg_media_get_info tool.
func NewMediaGetInfoHandler(client telegram.Client) mcp.ToolHandlerFor[MediaGetInfoParams, MediaGetInfoResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MediaGetInfoParams,
	) (*mcp.CallToolResult, MediaGetInfoResult, error) {
		info, err := readMediaInfo(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaGetInfoResult{}, err
		}

		return nil, MediaGetInfoResult{MediaInfo: *info, Output: formatMediaInfo(info)}, nil
	}
}

func readMediaInfo(ctx context.Context, client telegram.Client, params *MediaGetInfoParams) (*telegram.MediaInfo, error) {
	peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
	if err != nil {
		return nil, err
	}

	info, err := client.GetMediaInfo(ctx, peer, params.MessageID)
	if err != nil {
		return nil, telegramErr("failed to read the media", err)
	}

	return info, nil
}

// MediaGetInfoTool returns the MCP tool definition for tg_media_get_info.
func MediaGetInfoTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_media_get_info",
		Description: "Describe a message's media (type, MIME type, size, file name, duration, dimensions) without downloading it",
		Annotations: readOnlyAnnotations(),
	}
}

func formatMediaInfo(info *telegram.MediaInfo) string {
	parts := []string{info.Type}

	if info.MimeType != "" {
		parts = append(parts, info.MimeType)
	}

	if info.FileName != "" {
		parts = append(parts, info.FileName)
	}

	if info.Size > 0 {
		parts = append(parts, strconv.FormatInt(info.Size, 10)+" bytes")
	}

	if info.Width > 0 && info.Height > 0 {
		parts = append(parts, fmt.Sprintf("%dx%d", info.Width, info.Height))
	}

	if info.Duration > 0 {
		parts = append(parts, fmt.Sprintf("%.0fs", info.Duration))
	}

	return strings.Join(parts, ", ")
}

// MediaViewParams defines the parameters for the tg_media_view tool.
type MediaViewParams struct {
	Peer      string `json:"peer"      jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID int    `json:"messageId" jsonschema:"ID of the message carrying the photo or image"`
}

// MediaViewResult is the structured part of the tg_media_view result; the
// image itself travels as image content beside it.
type MediaViewResult struct {
	MimeType string `json:"mimeType"`
	Size     int    `json:"size"`
	Output   string `json:"output"`
}

// NewMediaViewHandler creates a handler for the tg_media_view tool.
func NewMediaViewHandler(client telegram.Client) mcp.ToolHandlerFor[MediaViewParams, MediaViewResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MediaViewParams,
	) (*mcp.CallToolResult, MediaViewResult, error) {
		peer, err := resolveMessageTarget(ctx, client, params.Peer, params.MessageID)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaViewResult{}, err
		}

		image, err := client.ViewMedia(ctx, peer, params.MessageID, viewMaxBytes)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MediaViewResult{}, telegramErr("failed to load the image", err)
		}

		result := MediaViewResult{
			MimeType: image.MimeType,
			Size:     len(image.Data),
			Output:   fmt.Sprintf("Image from message %d (%s, %d bytes)", params.MessageID, image.MimeType, len(image.Data)),
		}

		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.TextContent{Text: result.Output},
			&mcp.ImageContent{Data: image.Data, MIMEType: image.MimeType},
		}}, result, nil
	}
}

// MediaViewTool returns the MCP tool definition for tg_media_view.
func MediaViewTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_media_view",
		Description: "Look at a photo, image or video/document thumbnail from a Telegram message: " +
			"the image is returned in the tool result, nothing is saved to disk",
		Annotations: readOnlyAnnotations(),
	}
}

// InlineQueryParams defines the parameters for the tg_inline_query tool.
type InlineQueryParams struct {
	Bot    string `json:"bot"              jsonschema:"Inline bot, e.g. @gif, @vid, @wiki, @pic"`
	Query  string `json:"query"            jsonschema:"What to ask the bot"`
	Peer   string `json:"peer"             jsonschema:"Chat the results are for; send them to this same chat"`
	Offset string `json:"offset,omitempty" jsonschema:"nextOffset from a previous call, for the next page"`
}

// InlineQueryResult is the output of the tg_inline_query tool.
type InlineQueryResult struct {
	telegram.InlineResults

	Output string `json:"output"`
}

// NewInlineQueryHandler creates a handler for the tg_inline_query tool.
func NewInlineQueryHandler(client telegram.Client) mcp.ToolHandlerFor[InlineQueryParams, InlineQueryResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params InlineQueryParams,
	) (*mcp.CallToolResult, InlineQueryResult, error) {
		bot, peer, err := resolveInlineTarget(ctx, client, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, InlineQueryResult{}, err
		}

		results, err := client.InlineQuery(ctx, bot, peer, params.Query, params.Offset)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, InlineQueryResult{}, telegramErr("inline query failed", err)
		}

		return nil, InlineQueryResult{InlineResults: *results, Output: formatInlineResults(results)}, nil
	}
}

// resolveInlineTarget resolves the bot asked and the chat the results are for.
//
//nolint:gocritic // unnamedResult: bot then chat, as in the parameters.
func resolveInlineTarget(
	ctx context.Context, client telegram.Client, params *InlineQueryParams,
) (telegram.InputPeer, telegram.InputPeer, error) {
	if params.Bot == "" {
		return telegram.InputPeer{}, telegram.InputPeer{}, validationErr(ErrBotRequired)
	}

	if params.Peer == "" {
		return telegram.InputPeer{}, telegram.InputPeer{}, validationErr(ErrPeerRequired)
	}

	bot, err := client.ResolvePeer(ctx, params.Bot)
	if err != nil {
		return telegram.InputPeer{}, telegram.InputPeer{}, telegramErr("failed to resolve the bot", err)
	}

	peer, err := client.ResolvePeer(ctx, params.Peer)
	if err != nil {
		return telegram.InputPeer{}, telegram.InputPeer{}, telegramErr("failed to resolve peer", err)
	}

	return bot, peer, nil
}

// InlineQueryTool returns the MCP tool definition for tg_inline_query.
func InlineQueryTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "tg_inline_query",
		Description: "Ask a Telegram inline bot (@gif, @vid, @wiki, @pic, ...) for results to send to a chat; " +
			"send one with tg_inline_send",
		Annotations: readOnlyAnnotations(),
	}
}

func formatInlineResults(results *telegram.InlineResults) string {
	var buf strings.Builder

	fmt.Fprintf(&buf, "%d result(s), queryId %s", len(results.Results), results.QueryID)

	if results.NextOffset != "" {
		fmt.Fprintf(&buf, ", nextOffset %q", results.NextOffset)
	}

	for _, result := range results.Results {
		fmt.Fprintf(&buf, "\n[%s] %s", result.ID, result.Type)

		for _, part := range []string{result.Title, result.Description, result.URL} {
			if part != "" {
				buf.WriteString(" — " + part)
			}
		}
	}

	return buf.String()
}

// InlineSendParams defines the parameters for the tg_inline_send tool.
type InlineSendParams struct {
	Peer     string  `json:"peer"              jsonschema:"The chat the tg_inline_query was made for"`
	QueryID  string  `json:"queryId"           jsonschema:"queryId returned by tg_inline_query, as given"`
	ResultID string  `json:"resultId"          jsonschema:"id of the result to send"`
	TopicID  *int    `json:"topicId,omitempty" jsonschema:"Forum topic ID to send into"`
	ReplyTo  *int    `json:"replyTo,omitempty" jsonschema:"Message ID to reply to"`
	Silent   *bool   `json:"silent,omitempty"  jsonschema:"Send without notification sound"`
	SendAs   *string `json:"sendAs,omitempty"  jsonschema:"Post as this channel; see tg_chats_get_send_as. Omit for the chat default"`
}

// InlineSendResult is the output of the tg_inline_send tool.
type InlineSendResult struct {
	MessageID int    `json:"messageId"`
	Output    string `json:"output"`
}

// NewInlineSendHandler creates a handler for the tg_inline_send tool.
func NewInlineSendHandler(client telegram.Client) mcp.ToolHandlerFor[InlineSendParams, InlineSendResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params InlineSendParams,
	) (*mcp.CallToolResult, InlineSendResult, error) {
		queryID, err := validateInlineSend(&params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, InlineSendResult{}, validationErr(err)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, InlineSendResult{}, telegramErr("failed to resolve peer", err)
		}

		opts, err := resolveSimpleSendOpts(ctx, client, peer, params.TopicID, params.ReplyTo, params.Silent, params.SendAs)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, InlineSendResult{}, err
		}

		msg, err := client.SendInlineResult(ctx, peer, queryID, params.ResultID, opts)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, InlineSendResult{},
				sendErr("failed to send the inline result", err, opts.SendAs)
		}

		result := InlineSendResult{Output: outputSent}
		if msg != nil {
			result.MessageID = msg.ID
			result.Output = fmt.Sprintf("Sent as message %d", msg.ID)
		}

		return nil, result, nil
	}
}

func validateInlineSend(params *InlineSendParams) (int64, error) {
	switch {
	case params.Peer == "":
		return 0, ErrPeerRequired
	case params.QueryID == "":
		return 0, ErrQueryIDRequired
	case params.ResultID == "":
		return 0, ErrResultIDRequired
	}

	queryID, err := strconv.ParseInt(params.QueryID, 10, 64)
	if err != nil {
		return 0, errors.Wrap(ErrQueryIDRequired, "queryId is not a decimal number")
	}

	return queryID, nil
}

// InlineSendTool returns the MCP tool definition for tg_inline_send.
func InlineSendTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_inline_send",
		Description: "Send one result of a tg_inline_query (a GIF, video, article, ...) to the chat it was made for",
		Annotations: writeAnnotations(),
	}
}

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultTranscribeWaitSeconds = 30
	maxTranscribeWaitSeconds     = 120
)

// MessagesTranscribeAudioParams defines parameters for tg_messages_transcribe_audio.
type MessagesTranscribeAudioParams struct {
	Peer        string `json:"peer"                  jsonschema:"@username, t.me/ link, or numeric ID"`
	MessageID   int    `json:"messageId"             jsonschema:"Voice message or video note ID to transcribe"`
	WaitSeconds *int   `json:"waitSeconds,omitempty" jsonschema:"Wait for updateTranscribedAudio, 0-120s (default 30)"`
}

// MessagesTranscribeAudioResult is the output of tg_messages_transcribe_audio.
type MessagesTranscribeAudioResult struct {
	Status                string `json:"status"`
	MessageID             int    `json:"messageId"`
	Type                  string `json:"type,omitempty"`
	Pending               bool   `json:"pending,omitempty"`
	TranscriptionID       int64  `json:"transcriptionId,omitempty"`
	Text                  string `json:"text,omitempty"`
	TrialRemainsNum       int    `json:"trialRemainsNum,omitempty"`
	TrialRemainsUntilDate int    `json:"trialRemainsUntilDate,omitempty"`
	Output                string `json:"output"`
}

// NewMessagesTranscribeAudioHandler creates a handler for tg_messages_transcribe_audio.
func NewMessagesTranscribeAudioHandler(
	client telegram.Client,
) mcp.ToolHandlerFor[MessagesTranscribeAudioParams, MessagesTranscribeAudioResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params MessagesTranscribeAudioParams,
	) (*mcp.CallToolResult, MessagesTranscribeAudioResult, error) {
		wait, waitErr := validateTranscriptionRequest(params)
		if waitErr != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesTranscribeAudioResult{},
				validationErr(waitErr)
		}

		peer, err := client.ResolvePeer(ctx, params.Peer)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, MessagesTranscribeAudioResult{},
				telegramErr("failed to resolve peer", err)
		}

		return transcribeAudio(ctx, client, peer, params.MessageID, wait)
	}
}

func validateTranscriptionRequest(params MessagesTranscribeAudioParams) (time.Duration, error) {
	if params.Peer == "" {
		return 0, ErrPeerRequired
	}

	if params.MessageID == 0 {
		return 0, ErrMessageIDRequired
	}

	return transcribeWait(params.WaitSeconds)
}

func transcribeWait(waitSeconds *int) (time.Duration, error) {
	wait := defaultTranscribeWaitSeconds
	if waitSeconds != nil {
		wait = *waitSeconds
	}

	if wait < 0 || wait > maxTranscribeWaitSeconds {
		return 0, ErrInvalidWaitSeconds
	}

	return time.Duration(wait) * time.Second, nil
}

func transcribeAudio(
	ctx context.Context,
	client telegram.Client,
	peer telegram.InputPeer,
	messageID int,
	wait time.Duration,
) (*mcp.CallToolResult, MessagesTranscribeAudioResult, error) {
	transcription, err := client.TranscribeAudio(ctx, peer, messageID, wait)
	if err != nil {
		if status, ok := transcriptionStatusFromError(err); ok {
			return nil, transcriptionResult(&telegram.Transcription{
				Status:    status,
				MessageID: messageID,
			}), nil
		}

		return &mcp.CallToolResult{IsError: true}, MessagesTranscribeAudioResult{},
			telegramErr("failed to transcribe audio", err)
	}

	if transcription == nil {
		return &mcp.CallToolResult{IsError: true}, MessagesTranscribeAudioResult{},
			telegramErr("failed to transcribe audio", ErrEmptyTranscriptionResult)
	}

	if transcription.MessageID == 0 {
		transcription.MessageID = messageID
	}

	return nil, transcriptionResult(transcription), nil
}

func transcriptionStatusFromError(err error) (string, bool) {
	switch {
	case tg.IsPremiumAccountRequired(err):
		return telegram.TranscriptionStatusPremiumRequired, true
	case tg.IsMsgVoiceMissing(err):
		return telegram.TranscriptionStatusNotTranscribable, true
	case tg.IsTranscriptionFailed(err):
		return telegram.TranscriptionStatusFailed, true
	default:
		return "", false
	}
}

func transcriptionResult(transcription *telegram.Transcription) MessagesTranscribeAudioResult {
	res := MessagesTranscribeAudioResult{
		Status:                transcription.Status,
		MessageID:             transcription.MessageID,
		Type:                  transcription.Type,
		Pending:               transcription.Pending,
		TranscriptionID:       transcription.TranscriptionID,
		Text:                  transcription.Text,
		TrialRemainsNum:       transcription.TrialRemainsNum,
		TrialRemainsUntilDate: transcription.TrialRemainsUntilDate,
	}
	res.Output = formatTranscription(transcription)

	return res
}

func formatTranscription(transcription *telegram.Transcription) string {
	lines := []string{
		"status: " + transcription.Status,
		fmt.Sprintf("message: %d", transcription.MessageID),
	}

	if transcription.Type != "" {
		lines = append(lines, "type: "+transcription.Type)
	}

	if transcription.TranscriptionID != 0 {
		lines = append(lines, fmt.Sprintf("transcription_id: %d", transcription.TranscriptionID))
	}

	if transcription.TrialRemainsNum != 0 {
		lines = append(lines, fmt.Sprintf("trial_remains_num: %d", transcription.TrialRemainsNum))
	}

	if transcription.TrialRemainsUntilDate != 0 {
		lines = append(lines, fmt.Sprintf("trial_remains_until_date: %d", transcription.TrialRemainsUntilDate))
	}

	if transcription.Text != "" {
		lines = append(lines, "transcription:\n"+transcription.Text)
	}

	return strings.Join(lines, "\n")
}

// MessagesTranscribeAudioTool returns the MCP tool definition.
func MessagesTranscribeAudioTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "tg_messages_transcribe_audio",
		Description: "Transcribe a Telegram voice message or video note by message ID using Telegram API",
		Annotations: writeAnnotations(),
	}
}

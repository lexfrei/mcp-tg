// Package tools provides MCP tool handlers for Telegram operations.
package tools

import (
	"strings"

	"github.com/cockroachdb/errors"
)

// ErrValidation indicates invalid parameters provided by the caller.
var ErrValidation = errors.New("validation error")

// ErrTelegram indicates a failure communicating with the Telegram API.
var ErrTelegram = errors.New("telegram request error")

// ErrPeerRequired is returned when a peer parameter is missing.
var ErrPeerRequired = errors.New("peer is required")

// ErrMessageIDRequired is returned when a message ID parameter is missing.
var ErrMessageIDRequired = errors.New("message ID is required")

// ErrTextRequired is returned when a text parameter is missing.
var ErrTextRequired = errors.New("text is required")

// ErrQueryRequired is returned when a search query parameter is missing.
var ErrQueryRequired = errors.New("query is required")

// ErrNegativeLimit is returned when a numeric limit parameter is negative.
var ErrNegativeLimit = errors.New("numeric limits must not be negative")

// ErrTitleRequired is returned when a title parameter is missing.
var ErrTitleRequired = errors.New("title is required")

// ErrGroupRequired is returned when a group parameter is missing.
var ErrGroupRequired = errors.New("group is required")

// ErrUserRequired is returned when a user parameter is missing.
var ErrUserRequired = errors.New("user is required")

// ErrLinkRequired is returned when a link parameter is missing.
var ErrLinkRequired = errors.New("link is required")

// ErrPathRequired is returned when a file path parameter is missing.
var ErrPathRequired = errors.New("path is required")

// ErrPathsRequired is returned when a paths list parameter is missing.
var ErrPathsRequired = errors.New("paths list is required")

// ErrFirstNameRequired is returned when a first name parameter is missing.
var ErrFirstNameRequired = errors.New("first name is required")

// ErrMessageNotFound is returned when a message cannot be found.
var ErrMessageNotFound = errors.New("message not found")

// ErrNameRequired is returned when a name parameter is missing.
var ErrNameRequired = errors.New("name is required")

// ErrFolderIDRequired is returned when a folder ID parameter is missing.
var ErrFolderIDRequired = errors.New("folder ID is required")

// ErrStickerFileIDRequired is returned when a sticker file ID parameter is missing.
var ErrStickerFileIDRequired = errors.New("sticker file ID is required")

// ErrEmojiRequired is returned when an emoji parameter is missing.
var ErrEmojiRequired = errors.New("emoji is required")

// ErrTopicIDRequired is returned when a topic ID parameter is missing.
var ErrTopicIDRequired = errors.New("topic ID is required")

// ErrTopicIDOnNonForum is returned when topicId is supplied for a chat
// that is not a forum-enabled supergroup.
var ErrTopicIDOnNonForum = errors.New(
	"topicId is only valid for forum-enabled supergroups; this chat is not a forum",
)

// ErrTooManyIDs is returned when too many message IDs are provided.
var ErrTooManyIDs = errors.New("too many IDs (max 100)")

// ErrUserPeerRequired is returned when a user peer is needed but another type was provided.
var ErrUserPeerRequired = errors.New("this operation requires a user peer, not a group or channel")

// ErrInvalidSlowmode is returned when seconds is not an allowed Telegram slowmode value.
var ErrInvalidSlowmode = errors.New(
	"invalid slowmode seconds; allowed: 0,10,30,60,300,900,3600,21600,43200",
)

// ErrUnknownParseMode is returned when parseMode is a value the wrapper
// does not recognise.
var ErrUnknownParseMode = errors.New(
	"unknown parseMode; allowed: '' (plain), 'commonmark', 'markdown' (alias for commonmark), 'html', 'markdownv2'",
)

// ErrParseModeNotImplemented is returned when parseMode is a valid value
// whose implementation is not yet available.
var ErrParseModeNotImplemented = errors.New(
	"parseMode not yet implemented; use 'commonmark' (supports **bold**, *italic*, `code`, [text](url), ```pre```, > quote)",
)

// validationErr marks an error as a validation error.
func validationErr(err error) error {
	//nolint:wrapcheck // Mark adds a sentinel category, the caller already provides context.
	return errors.Mark(err, ErrValidation)
}

// telegramErr wraps a message and underlying error as a Telegram request
// error. Well-known MTProto error codes (REPLY_MESSAGE_ID_INVALID etc.)
// pick up an extra human-readable layer via wrapTelegramError so callers
// see why the request failed without parsing the raw rpc-error string.
func telegramErr(msg string, err error) error {
	//nolint:wrapcheck // Mark adds a sentinel category on top of Wrap which provides context.
	return errors.Mark(errors.Wrap(wrapTelegramError(err), msg), ErrTelegram)
}

// explainMTProtoCode returns a short human-readable explanation for a
// well-known MTProto error code, or empty string if the code is not in
// our translation table. Match is on substring of err.Error() because
// gotd/td prefixes the code with "rpc error code N:" or similar.
//
//nolint:cyclop,gocyclo // long flat switch is the clearest way to express the lookup table
func explainMTProtoCode(raw string) string {
	switch {
	case strings.Contains(raw, "REPLY_MESSAGE_ID_INVALID"):
		return "the reply target message does not exist in this chat"
	case strings.Contains(raw, "MESSAGE_ID_INVALID"):
		return "the referenced message does not exist or is no longer accessible"
	case strings.Contains(raw, "TOPIC_ID_INVALID"):
		return "the forum topic does not exist or has been deleted"
	case strings.Contains(raw, "TOPIC_DELETED"):
		return "the forum topic has been deleted"
	case strings.Contains(raw, "PEER_ID_INVALID"):
		return "the peer is invalid; resolve via @username if you used a numeric ID"
	case strings.Contains(raw, "USER_BANNED_IN_CHANNEL"):
		return "this account is banned in the target channel"
	case strings.Contains(raw, "CHAT_WRITE_FORBIDDEN"):
		return "this account is not permitted to write in the target chat"
	case strings.Contains(raw, "MESSAGE_TOO_LONG"):
		return "the message text exceeds the server's length limit"
	case strings.Contains(raw, "MEDIA_CAPTION_TOO_LONG"):
		return "the caption exceeds the server's length limit"
	case strings.Contains(raw, "SLOWMODE_WAIT"):
		return "the chat has slow mode enabled and this account must wait before sending"
	default:
		return ""
	}
}

// wrapTelegramError translates well-known MTProto error codes into
// human-readable forms while leaving everything else untouched. The
// original error is preserved as the cause so callers can still match
// on the underlying type or code.
func wrapTelegramError(err error) error {
	if err == nil {
		return nil
	}

	if explanation := explainMTProtoCode(err.Error()); explanation != "" {
		return errors.Wrap(err, explanation)
	}

	return err
}

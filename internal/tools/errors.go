// Package tools provides MCP tool handlers for Telegram operations.
package tools

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
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

// ErrInvalidStickerFileID is returned when a sticker file ID is not a
// decimal integer. It is a string rather than a number because the MCP
// SDK round-trips tool arguments through float64, which cannot hold the
// 63 bits a sticker document id needs.
var ErrInvalidStickerFileID = errors.New(
	"sticker file ID must be a decimal integer string, as printed by tg_stickers_get_set",
)

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

// ErrSendAsUnresolved is returned when a sendAs reference names an
// identity whose access hash is unknown — usually a channel, since your
// own account always resolves. A bare numeric ID resolves to an access
// hash of zero without erroring, and sending that on yields a server-side
// PEER_ID_INVALID that reads as a problem with the destination instead.
var ErrSendAsUnresolved = errors.New(
	"sendAs identity has no known access hash; pass @username, " +
		"or call tg_chats_get_send_as on the destination first",
)

// ErrInvalidSlowmode is returned when seconds is not an allowed Telegram slowmode value.
var ErrInvalidSlowmode = errors.New(
	"invalid slowmode seconds; allowed: 0,10,30,60,300,900,3600,21600,43200",
)

// ErrUnknownParseMode is returned when parseMode is a value the wrapper
// does not recognise.
var ErrUnknownParseMode = errors.New(
	"unknown parseMode; allowed: '' (plain), 'commonmark', 'markdown' (alias for commonmark), 'html', 'markdownv2'",
)

// ErrUnknownMessageType is returned when a messages_list type filter is
// not one of the message types emitted in MessageItem.type.
var ErrUnknownMessageType = errors.New(
	"unknown message type; allowed: text, photo, voice, video_note, video, audio, sticker, animation, document, " +
		"contact, location, venue, poll, webpage, game, invoice, unsupported",
)

// ErrUnknownMessageFilter is returned when a search filter is not one
// of the server-side filter names accepted by telegram.IsSearchFilter.
// The list is built from telegram.SearchFilters so a new filter name
// cannot silently drift out of the error text.
var ErrUnknownMessageFilter = errors.New(
	"unknown filter; allowed: " + strings.Join(telegram.SearchFilters(), ", "),
)

// ErrInvalidDateRange is returned when a minDate/maxDate window is
// inverted.
var ErrInvalidDateRange = errors.New("minDate must not exceed maxDate")

// ErrUnknownSearchScope is returned when a global search scope is not
// one of the dialog kinds Telegram can restrict a search to.
var ErrUnknownSearchScope = errors.New("unknown scope; allowed: users, groups, channels")

// ErrQueryOrFilterRequired is returned when a global search names
// neither a text query nor a kind filter. Either alone is a valid
// search — a bare filter means "all messages of this kind" — but both
// empty would ask the server to enumerate everything.
var ErrQueryOrFilterRequired = errors.New("query or filter is required")

// ErrSearchCriteriaRequired is the per-chat variant: alongside query
// and filter, a bare sender filter ("all messages from this member")
// is also a valid search.
var ErrSearchCriteriaRequired = errors.New("query, filter or from is required")

// ErrFromUnresolved is returned when the sender filter resolves without
// an access hash — a numeric ID the client has never seen resolves with
// hash 0 and a nil error, and sending it on would fail with a server
// error naming neither the parameter nor the remedy.
var ErrFromUnresolved = errors.New(
	"from resolved without an access hash; pass @username, or look the peer up via tg_dialogs_list first",
)

// ErrOffsetPeerUnresolved is returned when the pagination cursor's peer
// resolves without an access hash — typical after a restart cleared the
// peer cache that the previous page had seeded. Sending it on would
// fail with a server error naming neither the parameter nor the fix.
var ErrOffsetPeerUnresolved = errors.New(
	"offsetPeer resolved without an access hash; re-run the first page to seed the peer cache",
)

// ErrInvalidWaitSeconds is returned when a waitSeconds value is outside
// the supported range for a bounded MCP request.
var ErrInvalidWaitSeconds = errors.New("waitSeconds must be between 0 and 120")

// ErrEmptyTranscriptionResult is returned when the transcription client returns no result and no error.
var ErrEmptyTranscriptionResult = errors.New("empty transcription result")

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

// sendErr wraps a failed send, naming the send-as identity as a suspect
// when one was requested.
//
// Telegram does not report a disallowed identity distinctly: posting as a
// channel the account does not administrate answers CHAT_ADMIN_REQUIRED,
// and naming a foreign user answers CHAT_WRITE_FORBIDDEN. Both read as
// "you may not write here", which is false — the destination was fine and
// the identity was not. SEND_AS_PEER_INVALID exists in the schema but the
// server rarely reaches for it.
func sendErr(msg string, err error, sendAs *telegram.InputPeer) error {
	if sendAs != nil && rejectsIdentity(err) {
		return telegramErr(msg+"; the sendAs identity may be the cause, call "+
			toolGetSendAs+" on the destination for the allowed ones", err)
	}

	return telegramErr(msg, err)
}

// rejectsIdentity reports whether an RPC error is one of the codes
// Telegram answers with when it refuses a send-as identity.
func rejectsIdentity(err error) bool {
	raw := err.Error()

	return strings.Contains(raw, "CHAT_ADMIN_REQUIRED") ||
		strings.Contains(raw, "CHAT_WRITE_FORBIDDEN") ||
		strings.Contains(raw, "SEND_AS_PEER_INVALID")
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
	case strings.Contains(raw, "CHAT_ADMIN_REQUIRED"):
		return "this action needs administrator rights this account does not have"
	case strings.Contains(raw, "MESSAGE_TOO_LONG"):
		return "the message text exceeds the server's length limit"
	case strings.Contains(raw, "MEDIA_CAPTION_TOO_LONG"):
		return "the caption exceeds the server's length limit"
	case strings.Contains(raw, "SLOWMODE_WAIT"):
		return "the chat has slow mode enabled and this account must wait before sending"
	case strings.Contains(raw, "SEND_AS_PEER_INVALID"):
		return "this account cannot post as the requested identity here; " +
			"call tg_chats_get_send_as on the destination for the allowed ones"
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

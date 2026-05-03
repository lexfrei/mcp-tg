package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// validateTopicID rejects topicId values that the chat cannot accept.
// topicID == 0 means "no topic", which is always fine. For non-zero
// values, the chat must be a forum-enabled supergroup; otherwise the
// MTProto layer returns a cryptic error after the round-trip and the
// user has no obvious way to relate it to their input. This pre-flight
// fails fast with a clear message before any send is attempted.
//
// PeerUser (DMs) and PeerChat (legacy basic groups) cannot be forums,
// so we short-circuit without a round-trip. Calling GetGroupInfo on a
// PeerUser would hit MessagesGetFullChat with a user ID and produce a
// nonsense error that buries the actual constraint.
//
// Existence of the topic itself is NOT verified here. ChannelsGetForumTopics
// is more expensive and the failure mode (TOPIC_ID_INVALID) is already
// fielded by wrapTelegramError downstream.
func validateTopicID(
	ctx context.Context, client telegram.Client, peer telegram.InputPeer, topicID int,
) error {
	if topicID == 0 {
		return nil
	}

	if peer.Type != telegram.PeerChannel {
		return ErrTopicIDOnNonForum
	}

	info, err := client.GetGroupInfo(ctx, peer)
	if err != nil {
		return errors.Wrap(err, "fetching group info to validate topicId")
	}

	if info == nil || !info.IsForum {
		return ErrTopicIDOnNonForum
	}

	return nil
}

// normalizeParseMode lowercases the input so callers can pass
// "Markdown", "COMMONMARK" etc. without getting a validation error.
func normalizeParseMode(mode string) string {
	return strings.ToLower(mode)
}

// deref returns the value of a pointer or a zero value if nil.
func deref[T any](ptr *T) T {
	if ptr == nil {
		var zero T

		return zero
	}

	return *ptr
}

// validateLimit returns an error if limit is negative.
func validateLimit(limit int) error {
	if limit < 0 {
		return ErrNegativeLimit
	}

	return nil
}

// hasMorePage returns true when the returned count saturates the page,
// signalling the caller that another page may be available. The
// requestedLimit may be zero (caller did not specify), in which case the
// server-default page size is assumed via telegram.DefaultLimit.
func hasMorePage(count, requestedLimit int) bool {
	effective := requestedLimit
	if effective <= 0 {
		effective = telegram.DefaultLimit
	}

	return count >= effective
}

const maxIDsPerRequest = 100

// validateIDCount returns an error if too many IDs are provided.
func validateIDCount(ids []int) error {
	if len(ids) > maxIDsPerRequest {
		return ErrTooManyIDs
	}

	return nil
}

// formatPeer returns a bot-API style numeric ID that can be passed back
// as a peer parameter to other tools. Positive = user, negative = chat,
// -100xxx = channel.
func formatPeer(peer telegram.InputPeer) string {
	switch peer.Type {
	case telegram.PeerUser:
		return strconv.FormatInt(peer.ID, 10)
	case telegram.PeerChat:
		return strconv.FormatInt(-peer.ID, 10)
	case telegram.PeerChannel:
		return strconv.FormatInt(-telegram.ChannelIDOffset-peer.ID, 10)
	default:
		return strconv.FormatInt(peer.ID, 10)
	}
}

// Telegram-accepted slowmode delay values in seconds.
const (
	slowmodeOff = 0
	slowmode10s = 10
	slowmode30s = 30
	slowmode1m  = 60
	slowmode5m  = 300
	slowmode15m = 900
	slowmode1h  = 3600
	slowmode6h  = 21600
	slowmode12h = 43200
)

// validSlowmode reports whether sec is a Telegram-accepted slowmode value.
func validSlowmode(sec int) bool {
	switch sec {
	case slowmodeOff, slowmode10s, slowmode30s, slowmode1m,
		slowmode5m, slowmode15m, slowmode1h, slowmode6h,
		slowmode12h:
		return true
	default:
		return false
	}
}

// validateParseMode rejects unknown parseMode values and flags modes
// that are recognised but not yet implemented. Empty string means
// "plain text" and is always accepted. Comparison is case-insensitive
// so callers can pass "Markdown" or "COMMONMARK" without error.
func validateParseMode(mode string) error {
	switch normalizeParseMode(mode) {
	case "", telegram.ParseModeMarkdown, telegram.ParseModeCommonMark:
		return nil
	case telegram.ParseModeHTML, telegram.ParseModeMarkdownV2:
		return ErrParseModeNotImplemented
	default:
		return ErrUnknownParseMode
	}
}

// truncateText returns text shortened to at most maxRunes runes,
// appending an ellipsis when truncation happened. Operates on runes
// to avoid splitting multi-byte sequences (Cyrillic, emoji, etc.).
func truncateText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}

	runes := []rune(text)

	return string(runes[:maxRunes]) + "…"
}

// formatUserName builds a display name from first/last name and username.
func formatUserName(user *telegram.User) string {
	if user == nil {
		return unknownValue
	}

	name := strings.TrimSpace(user.FirstName + " " + user.LastName)

	if user.Username != "" {
		return fmt.Sprintf("%s (@%s)", name, user.Username)
	}

	return name
}

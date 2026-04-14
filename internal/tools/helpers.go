package tools

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

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

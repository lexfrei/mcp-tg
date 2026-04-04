package tools

import (
	"fmt"
	"strconv"
	"strings"

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

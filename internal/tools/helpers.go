package tools

import (
	"fmt"
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

// formatPeer returns a human-readable string for an InputPeer.
func formatPeer(peer telegram.InputPeer) string {
	switch peer.Type {
	case telegram.PeerUser:
		return fmt.Sprintf("user:%d", peer.ID)
	case telegram.PeerChat:
		return fmt.Sprintf("chat:%d", peer.ID)
	case telegram.PeerChannel:
		return fmt.Sprintf("channel:%d", peer.ID)
	default:
		return fmt.Sprintf("unknown:%d", peer.ID)
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

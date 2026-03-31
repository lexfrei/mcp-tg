package tools

import (
	"fmt"
	"time"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

const (
	unknownValue = "unknown"
	peerUser     = "user"
	peerChannel  = "channel"
	peerGroup    = "group"
	maxTextLen   = 100
)

// formatTimestamp converts a Unix timestamp to a human-readable string.
func formatTimestamp(unix int) string {
	if unix == 0 {
		return unknownValue
	}

	return time.Unix(int64(unix), 0).UTC().Format(time.RFC3339)
}

// formatMessage returns a single-line summary of a message.
func formatMessage(msg *telegram.Message) string {
	if msg == nil {
		return unknownValue
	}

	text := msg.Text
	if len(text) > maxTextLen {
		text = text[:maxTextLen] + "..."
	}

	timestamp := formatTimestamp(msg.Date)

	if msg.MediaType != "" {
		return fmt.Sprintf("[%d] %s [%s] %s", msg.ID, timestamp, msg.MediaType, text)
	}

	return fmt.Sprintf("[%d] %s %s", msg.ID, timestamp, text)
}

// formatDialog returns a single-line summary of a dialog.
func formatDialog(dlg *telegram.Dialog) string {
	if dlg == nil {
		return unknownValue
	}

	peerType := peerUser
	if dlg.IsChannel {
		peerType = peerChannel
	} else if dlg.IsGroup {
		peerType = peerGroup
	}

	unread := ""
	if dlg.UnreadCount > 0 {
		unread = fmt.Sprintf(" (%d unread)", dlg.UnreadCount)
	}

	return fmt.Sprintf("[%s] %s%s", peerType, dlg.Title, unread)
}

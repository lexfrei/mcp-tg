package tools

import (
	"fmt"
	"time"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

// formatTimestamp converts a Unix timestamp to a human-readable string.
func formatTimestamp(unix int) string {
	if unix == 0 {
		return "unknown"
	}

	return time.Unix(int64(unix), 0).UTC().Format(time.RFC3339)
}

// formatMessage returns a single-line summary of a message.
func formatMessage(msg *telegram.Message) string {
	text := msg.Text
	if len(text) > 100 {
		text = text[:100] + "..."
	}

	timestamp := formatTimestamp(msg.Date)

	if msg.MediaType != "" {
		return fmt.Sprintf("[%d] %s [%s] %s", msg.ID, timestamp, msg.MediaType, text)
	}

	return fmt.Sprintf("[%d] %s %s", msg.ID, timestamp, text)
}

// formatDialog returns a single-line summary of a dialog.
func formatDialog(dlg *telegram.Dialog) string {
	peerType := "user"
	if dlg.IsChannel {
		peerType = "channel"
	} else if dlg.IsGroup {
		peerType = "group"
	}

	unread := ""
	if dlg.UnreadCount > 0 {
		unread = fmt.Sprintf(" (%d unread)", dlg.UnreadCount)
	}

	return fmt.Sprintf("[%s] %s%s", peerType, dlg.Title, unread)
}

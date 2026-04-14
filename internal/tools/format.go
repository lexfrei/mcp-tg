package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/lexfrei/mcp-tg/internal/telegram"
)

const (
	unknownValue   = "unknown"
	peerUser       = "user"
	peerChannel    = "channel"
	peerGroup      = "group"
	actionPinned   = "Pinned"
	actionUnpinned = "Unpinned"
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
	timestamp := formatTimestamp(msg.Date)
	sender := formatSender(msg)
	header := formatMessageHeader(msg)

	if msg.MediaType != "" {
		return fmt.Sprintf("%s %s %s[%s] %s", header, timestamp, sender, msg.MediaType, text)
	}

	return fmt.Sprintf("%s %s %s%s", header, timestamp, sender, text)
}

// formatMessageHeader returns the "[ID]" or "[ID ↩parentID]" prefix.
// ReplyTo is non-nil only when extractReplyTo produced a valid parent
// ID, so we don't re-check MessageID here.
func formatMessageHeader(msg *telegram.Message) string {
	if msg.ReplyTo != nil {
		return fmt.Sprintf("[%d ↩%d]", msg.ID, msg.ReplyTo.MessageID)
	}

	return fmt.Sprintf("[%d]", msg.ID)
}

func formatSender(msg *telegram.Message) string {
	if msg.FromName != "" {
		return msg.FromName + ": "
	}

	if msg.FromID != 0 {
		return fmt.Sprintf("user:%d: ", msg.FromID)
	}

	return ""
}

// formatMessages formats a slice of messages as newline-separated lines.
func formatMessages(msgs []telegram.Message) string {
	var buf strings.Builder

	for idx := range msgs {
		fmt.Fprintln(&buf, formatMessage(&msgs[idx]))
	}

	return buf.String()
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

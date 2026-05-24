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

// formatMessage renders a single message as a multi-line block. Layout:
//
//	[<id>] <ISO-timestamp>
//	from: <Name [@username]>
//	forwarded from: <Name [hidden]> at <ts>           (when Forward.From is a user OR privacy-hidden)
//	forwarded from channel: <Title [@username]> #<post> by "<author>" at <ts>
//	reply to: <parentId>                              (same-chat reply)
//	reply to: <parentId> in <Name [@username]>        (cross-chat reply)
//	quote: «<QuoteText>»
//	media: <type>
//	text:
//	<msg.Text>
//
// Lines are emitted only when their underlying field is populated. The
// caller (formatMessages, formatContextMessages, etc.) is responsible
// for the blank line that separates one block from the next.
func formatMessage(msg *telegram.Message) string {
	if msg == nil {
		return unknownValue
	}

	var buf strings.Builder

	fmt.Fprintf(&buf, "[%d] %s\n", msg.ID, formatTimestamp(msg.Date))
	writeSenderLine(&buf, msg)
	writeForwardLine(&buf, msg.Forward)
	writeReplyLines(&buf, msg.ReplyTo)

	if msg.MediaType != "" {
		fmt.Fprintf(&buf, "media: %s\n", msg.MediaType)
	}

	if msg.Text != "" {
		buf.WriteString("text:\n")
		buf.WriteString(msg.Text)
	}

	return strings.TrimRight(buf.String(), "\n")
}

func writeSenderLine(buf *strings.Builder, msg *telegram.Message) {
	ref := formatPeerRef(msg.FromName, msg.FromUsername, telegram.InputPeer{
		Type: msg.FromType,
		ID:   msg.FromID,
	})

	if ref == peerRefHidden {
		return
	}

	fmt.Fprintf(buf, "from: %s\n", ref)
}

func writeForwardLine(buf *strings.Builder, fwd *telegram.ForwardInfo) {
	if fwd == nil {
		return
	}

	ref := forwardSourceRef(fwd)

	// Channel-shaped if we know the source is a channel OR if we have
	// a ChannelPost number (anonymous channel posts can arrive with
	// From == nil but ChannelPost != 0; dropping that to the generic
	// user-forward branch would lose the post number entirely).
	if isChannelForward(fwd) {
		writeChannelForwardLine(buf, fwd, ref)

		return
	}

	if fwd.Date != 0 {
		fmt.Fprintf(buf, "forwarded from: %s at %s\n", ref, formatTimestamp(fwd.Date))

		return
	}

	fmt.Fprintf(buf, "forwarded from: %s\n", ref)
}

// isChannelForward picks the channel-shaped output line for any forward
// that carries a non-zero ChannelPost — even when From is nil (the
// anonymous channel-post case where the source identity is stripped
// but the post number survives) or, in well-formed MTProto, never set
// to a user with a ChannelPost. The ChannelPost number is the
// actionable bit for reconstructing a deep-link back to the original;
// preserving it is worth more than re-deriving the peer kind from
// From when the two could in theory disagree.
func isChannelForward(fwd *telegram.ForwardInfo) bool {
	if fwd.From != nil && fwd.From.Peer.Type == telegram.PeerChannel {
		return true
	}

	return fwd.ChannelPost != 0
}

func writeChannelForwardLine(buf *strings.Builder, fwd *telegram.ForwardInfo, ref string) {
	buf.WriteString("forwarded from channel: ")
	buf.WriteString(ref)

	if fwd.ChannelPost != 0 {
		fmt.Fprintf(buf, " #%d", fwd.ChannelPost)
	}

	if fwd.PostAuthor != "" {
		fmt.Fprintf(buf, " by %q", fwd.PostAuthor)
	}

	if fwd.Date != 0 {
		fmt.Fprintf(buf, " at %s", formatTimestamp(fwd.Date))
	}

	buf.WriteString("\n")
}

func forwardSourceRef(fwd *telegram.ForwardInfo) string {
	if fwd.From != nil {
		return formatPeerRef(fwd.From.Name, fwd.From.Username, fwd.From.Peer)
	}

	return formatPeerRef(fwd.FromName, "", telegram.InputPeer{})
}

func writeReplyLines(buf *strings.Builder, reply *telegram.ReplyToInfo) {
	if reply == nil || reply.MessageID == 0 {
		return
	}

	if reply.FromPeerID != nil {
		ref := formatPeerRef(reply.FromName, reply.FromUsername, *reply.FromPeerID)
		fmt.Fprintf(buf, "reply to: %d in %s\n", reply.MessageID, ref)
	} else {
		fmt.Fprintf(buf, "reply to: %d\n", reply.MessageID)
	}

	if reply.QuoteText != "" {
		fmt.Fprintf(buf, "quote: «%s»\n", collapseLineBreaks(reply.QuoteText))
	}
}

// collapseLineBreaks replaces embedded LF, CR, and CRLF in a single
// string with a space, so multi-line content can be rendered on one
// key:value line of the multi-line output format without masquerading
// as a top-level metadata key or text body. CRLF is replaced first so
// no stray '\r' remains after a Windows-origin paste. The original
// string is unaffected — full multi-line content stays available in
// the JSON field that backs the rendered line.
func collapseLineBreaks(s string) string {
	return strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
}

// formatMessages joins message blocks separated by a blank line.
func formatMessages(msgs []telegram.Message) string {
	if len(msgs) == 0 {
		return ""
	}

	blocks := make([]string, 0, len(msgs))
	for idx := range msgs {
		blocks = append(blocks, formatMessage(&msgs[idx]))
	}

	return strings.Join(blocks, "\n\n") + "\n"
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

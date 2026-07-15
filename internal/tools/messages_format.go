package tools

// Output shapes for the message-reading tools. "full" (the default when
// the caller omits format) returns both the structured messages and the
// human-readable output; "json" drops the output; "text" drops the
// structured messages.
const (
	formatFull = "full"
	formatJSON = "json"
	formatText = "text"
)

// formatEnum locks a read tool's format schema property to the three
// output shapes, so the protocol rejects anything else before a handler
// runs. Mirrors parseModeEnum.
func formatEnum() []any {
	return []any{formatFull, formatJSON, formatText}
}

// messagesForFormat returns the structured messages unless the caller
// asked for the text-only shape. The result structs tag Messages
// omitempty, so a nil return disappears from the JSON entirely.
func messagesForFormat(format string, msgs []MessageItem) []MessageItem {
	if format == formatText {
		return nil
	}

	return msgs
}

// outputForFormat returns the human-readable output unless the caller
// asked for the json-only shape. The result structs tag Output
// omitempty, so an empty return disappears from the JSON entirely.
func outputForFormat(format, output string) string {
	if format == formatJSON {
		return ""
	}

	return output
}

// shouldResolveReplies reports whether reply-parent enrichment is worth
// its extra GetMessages round-trip: the caller asked for it AND the
// structured messages it enriches will actually be returned. In text
// format the messages are dropped, so the enrichment would be wasted.
func shouldResolveReplies(resolve *bool, format string) bool {
	return deref(resolve) && format != formatText
}

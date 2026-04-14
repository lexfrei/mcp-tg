package telegram

import "strings"

// inlineMarker defines a Markdown inline marker pattern.
type inlineMarker struct {
	open  string
	close string
	kind  string
}

// getInlineMarkers returns markers in priority order (longest first).
func getInlineMarkers() []inlineMarker {
	return []inlineMarker{
		{"**", "**", "bold"},
		{"__", "__", "underline"},
		{"~~", "~~", "strike"},
		{"||", "||", "spoiler"},
		{"`", "`", entityKindCode},
		{"*", "*", "italic"},
		{"_", "_", "italic"},
	}
}

// entityKindCode is the entity kind for inline code and code blocks.
const entityKindCode = "code"

// extractInlineEntities parses inline Markdown markers from text.
func extractInlineEntities(
	text string, existing []rawEntity,
) (string, []rawEntity) {
	entities := append([]rawEntity{}, existing...)
	plain, ents := parseInline(text)
	entities = append(entities, ents...)

	return plain, entities
}

// parseInline processes text character by character for inline markers.
func parseInline(text string) (string, []rawEntity) {
	var result strings.Builder

	var entities []rawEntity

	pos := 0
	for pos < len(text) {
		if text[pos] == '\\' && pos+1 < len(text) {
			result.WriteByte('\\')
			result.WriteByte(text[pos+1])
			pos += 2

			continue
		}

		if handled, newPos := tryLink(text, pos, &result, &entities); handled {
			pos = newPos

			continue
		}

		handled, newPos := tryMarker(text, pos, &result, &entities)
		if handled {
			pos = newPos

			continue
		}

		result.WriteByte(text[pos])

		pos++
	}

	return result.String(), entities
}

// tryLink attempts to parse a [text](url) link at position.
//
//nolint:gocritic // unnamedResult conflicts with nonamedreturns linter.
func tryLink(
	text string, pos int,
	result *strings.Builder, entities *[]rawEntity,
) (bool, int) {
	if text[pos] != '[' {
		return false, pos
	}

	closeBracket := strings.Index(text[pos:], "](")
	if closeBracket == -1 {
		return false, pos
	}

	closeBracket += pos
	closeParen := strings.IndexByte(text[closeBracket+2:], ')')

	if closeParen == -1 {
		return false, pos
	}

	closeParen += closeBracket + 2
	linkText := text[pos+1 : closeBracket]
	linkURL := text[closeBracket+2 : closeParen]
	start := utf16Len(result.String())

	result.WriteString(linkText)
	*entities = append(*entities, rawEntity{
		start: start, length: utf16Len(linkText),
		kind: "text_url", extra: linkURL,
	})

	return true, closeParen + 1
}

// isAlphanumeric reports whether the byte is an ASCII letter or digit.
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// needsWordBoundary reports whether the marker requires word boundaries
// (CommonMark rule: _ only triggers at word boundaries, * works anywhere).
func needsWordBoundary(marker inlineMarker) bool {
	return marker.open == "_"
}

// tryMarker attempts to match an inline marker at position.
//
//nolint:gocritic // unnamedResult conflicts with nonamedreturns linter.
func tryMarker(
	text string, pos int,
	result *strings.Builder, entities *[]rawEntity,
) (bool, int) {
	for _, marker := range getInlineMarkers() {
		if !strings.HasPrefix(text[pos:], marker.open) {
			continue
		}

		if needsWordBoundary(marker) &&
			pos > 0 && isAlphanumeric(text[pos-1]) {
			continue
		}

		endPos := matchMarker(text, pos, marker, result, entities)
		if endPos > pos {
			return true, endPos
		}
	}

	return false, pos
}

// matchMarker finds closing marker and builds the entity.
func matchMarker(
	text string, pos int, marker inlineMarker,
	result *strings.Builder, entities *[]rawEntity,
) int {
	after := text[pos+len(marker.open):]
	closeIdx := findClose(after, marker.close)

	if closeIdx == -1 {
		return pos
	}

	// Word-boundary check for closing marker: _ must not be
	// followed by an alphanumeric character.
	if needsWordBoundary(marker) {
		endPos := pos + len(marker.open) + closeIdx + len(marker.close)
		if endPos < len(text) && isAlphanumeric(text[endPos]) {
			return pos
		}
	}

	inner := after[:closeIdx]
	start := utf16Len(result.String())

	writeMarkerContent(inner, marker, start, result, entities)

	length := utf16Len(result.String()) - start
	*entities = append(*entities, rawEntity{
		start: start, length: length, kind: marker.kind,
	})

	endPos := pos + len(marker.open) + closeIdx + len(marker.close)

	// Stray-marker guard: a doubled-char marker (** __ ~~ ||) followed
	// immediately by the same character would otherwise let the lone
	// char open a new marker (e.g. "**x***" → bold "x" + italic "…"
	// eating the rest of the text). Consume that trailing char as a
	// literal instead so the run stays balanced.
	if isDoubledCharMarker(marker) && endPos < len(text) && text[endPos] == marker.open[0] {
		result.WriteByte(text[endPos])

		endPos++
	}

	return endPos
}

// isDoubledCharMarker reports whether the marker is a 2-byte token
// built from the same byte repeated, like "**" or "__".
func isDoubledCharMarker(marker inlineMarker) bool {
	return len(marker.open) == 2 && marker.open[0] == marker.open[1]
}

// writeMarkerContent writes inner content, parsing nested markers if needed.
func writeMarkerContent(
	inner string, marker inlineMarker, start int,
	result *strings.Builder, entities *[]rawEntity,
) {
	if marker.kind == entityKindCode {
		result.WriteString(inner)

		return
	}

	innerPlain, innerEnts := parseInline(inner)

	shiftEntities(innerEnts, start)
	*entities = append(*entities, innerEnts...)

	result.WriteString(innerPlain)
}

// findClose finds the closing marker, skipping escaped characters.
func findClose(text, marker string) int {
	pos := 0
	for pos < len(text) {
		if text[pos] == '\\' && pos+1 < len(text) {
			pos += 2

			continue
		}

		if strings.HasPrefix(text[pos:], marker) {
			return pos
		}

		pos++
	}

	return -1
}

// shiftEntities adds an offset to all entity start positions.
func shiftEntities(entities []rawEntity, offset int) {
	for idx := range entities {
		entities[idx].start += offset
	}
}

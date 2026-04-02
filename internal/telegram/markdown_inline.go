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

	inner := after[:closeIdx]
	start := utf16Len(result.String())

	writeMarkerContent(inner, marker, start, result, entities)

	length := utf16Len(result.String()) - start
	*entities = append(*entities, rawEntity{
		start: start, length: length, kind: marker.kind,
	})

	return pos + len(marker.open) + closeIdx + len(marker.close)
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

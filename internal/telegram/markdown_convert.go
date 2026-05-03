package telegram

import (
	"strings"

	"github.com/gotd/td/tg"
)

// removeEscapes strips backslash escapes and adjusts entity offsets. Per
// CommonMark §6.1, backslash escapes are inert inside code spans — the
// inner text is taken verbatim, so we skip escape processing for any
// position inside an inline-code entity.
func removeEscapes(
	text string, entities []rawEntity,
) (string, []rawEntity) {
	var result strings.Builder

	mapping := buildCharMapping(text, &result, codeEntityRanges(entities))

	adjusted := adjustEntities(entities, mapping)

	return result.String(), adjusted
}

// codeEntityRanges returns the [start, start+length) ranges of inline code
// entities. Pre entities are out of scope here: their bodies live in the
// `blocks` slice during this pass and are spliced back only by
// substituteCodeBlocks afterwards.
func codeEntityRanges(entities []rawEntity) []stripRange {
	var ranges []stripRange

	for _, ent := range entities {
		if ent.kind == EntityTypeCode {
			ranges = append(ranges, stripRange{start: ent.start, length: ent.length})
		}
	}

	return ranges
}

func isInsideRanges(pos int, ranges []stripRange) bool {
	for _, r := range ranges {
		if pos >= r.start && pos < r.start+r.length {
			return true
		}
	}

	return false
}

// buildCharMapping processes escapes and returns old-to-new offset map.
// Positions inside skipRanges are treated as literal — backslashes there
// stay in the output rather than escaping the next character.
func buildCharMapping(
	text string, result *strings.Builder, skipRanges []stripRange,
) []int {
	runes := []rune(text)
	mapping := make([]int, len(runes)+1)
	newPos := 0

	for idx := 0; idx < len(runes); idx++ {
		mapping[idx] = newPos

		if runes[idx] == '\\' && idx+1 < len(runes) && !isInsideRanges(idx, skipRanges) {
			result.WriteRune(runes[idx+1])

			newPos++
			idx++

			mapping[idx] = newPos

			continue
		}

		result.WriteRune(runes[idx])

		newPos++
	}

	mapping[len(runes)] = newPos

	return mapping
}

// adjustEntities recalculates entity offsets after escape removal.
func adjustEntities(
	entities []rawEntity, mapping []int,
) []rawEntity {
	if len(entities) == 0 {
		return entities
	}

	adjusted := make([]rawEntity, len(entities))
	copy(adjusted, entities)

	for idx := range adjusted {
		adjusted[idx] = remapEntity(adjusted[idx], mapping)
	}

	return adjusted
}

// remapEntity converts a single entity's start and length using the mapping.
// Both ends of the entity range are translated so that an escape sequence
// inside the entity (e.g. blockquote covering "hello \\* world") shrinks the
// length along with the cleaned text.
//
// mapping is rune-based; entity offsets are UTF-16 based. For escape removal
// we only remove ASCII backslashes, so as long as every preceding code unit
// is in the BMP, UTF-16 offset == rune offset and the rune-indexed mapping
// can be addressed with the entity's UTF-16 start. Supplementary-plane
// characters before an entity remain a known limitation.
func remapEntity(ent rawEntity, mapping []int) rawEntity {
	end := ent.start + ent.length

	if ent.start < len(mapping) {
		newStart := mapping[ent.start]

		if end < len(mapping) {
			ent.length = mapping[end] - newStart
		}

		ent.start = newStart
	}

	return ent
}

// toTelegramEntities converts rawEntity slice to Telegram entity types.
func toTelegramEntities(
	entities []rawEntity,
) []tg.MessageEntityClass {
	if len(entities) == 0 {
		return nil
	}

	result := make([]tg.MessageEntityClass, 0, len(entities))

	for _, ent := range entities {
		tgEnt := convertEntity(ent)
		if tgEnt != nil {
			result = append(result, tgEnt)
		}
	}

	return result
}

// convertEntity maps a rawEntity to the corresponding tg entity type.
//
//nolint:cyclop // switch over entity kinds is inherently branchy but straightforward.
func convertEntity(ent rawEntity) tg.MessageEntityClass {
	switch ent.kind {
	case EntityTypeBold:
		return &tg.MessageEntityBold{
			Offset: ent.start, Length: ent.length,
		}
	case EntityTypeItalic:
		return &tg.MessageEntityItalic{
			Offset: ent.start, Length: ent.length,
		}
	case EntityTypeCode:
		return &tg.MessageEntityCode{
			Offset: ent.start, Length: ent.length,
		}
	case EntityTypePre:
		return &tg.MessageEntityPre{
			Offset: ent.start, Length: ent.length,
			Language: ent.extra,
		}
	case EntityTypeStrike:
		return &tg.MessageEntityStrike{
			Offset: ent.start, Length: ent.length,
		}
	case EntityTypeUnderline:
		return &tg.MessageEntityUnderline{
			Offset: ent.start, Length: ent.length,
		}
	case EntityTypeSpoiler:
		return &tg.MessageEntitySpoiler{
			Offset: ent.start, Length: ent.length,
		}
	case EntityTypeTextURL:
		return &tg.MessageEntityTextURL{
			Offset: ent.start, Length: ent.length,
			URL: ent.extra,
		}
	case EntityTypeBlockquote:
		return &tg.MessageEntityBlockquote{
			Offset: ent.start, Length: ent.length,
		}
	default:
		return nil
	}
}

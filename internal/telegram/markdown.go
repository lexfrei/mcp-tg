package telegram

import (
	"strings"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

// rawEntity holds parsed entity data before conversion to Telegram types.
type rawEntity struct {
	start  int
	length int
	kind   string
	extra  string
}

// codeBlock holds the body and language of a fenced block pulled out
// during extractCodeBlocks for later re-insertion by substituteCodeBlocks.
type codeBlock struct {
	body string
	lang string
}

// preHolder is the internal placeholder rune that stands in for a fenced
// code block while the other markdown passes run. U+FDD0 is a permanent
// Unicode noncharacter ("not intended for interchange") so it will never
// appear in legitimate user input; sanitizePlaceholders strips it anyway.
const preHolder = '\uFDD0'

// ParseMarkdown converts Markdown text to plain text and Telegram entities.
//
// Pipeline: fenced blocks are replaced with a single-rune placeholder first.
// Inline markers and escapes are then resolved on the placeholdered text so
// every entity offset they emit lives in the cleaned UTF-16 space.
// extractBlockquotes runs next: at this point inline markers are gone, so a
// blockquote starting after some `…` runs no longer absorbs the consumed
// backticks into its offset. It also rebases every existing entity onto the
// post-strip space — entities that follow a "> " prefix have their start
// shifted left by 2 UTF-16 units, and entities whose range straddles a "> "
// prefix (e.g. **bold spanning across\n> a quote\nline**) have their length
// shrunk by the same amount per stripped prefix. Finally, substituteCodeBlocks
// splices the saved fenced bodies back in and emits pre entities with offsets
// taken from the final builder state.
func ParseMarkdown(text string) (string, []tg.MessageEntityClass) {
	text = sanitizePlaceholders(text)

	plain, blocks := extractCodeBlocks(text)

	var entities []rawEntity

	plain, entities = extractInlineEntities(plain, entities)
	plain, entities = removeEscapes(plain, entities)
	plain, entities = extractBlockquotes(plain, entities)
	plain, entities = substituteCodeBlocks(plain, entities, blocks)

	return plain, toTelegramEntities(entities)
}

// sanitizePlaceholders removes any stray preHolder runes from user input so
// that substituteCodeBlocks cannot confuse them with real fenced blocks.
func sanitizePlaceholders(text string) string {
	if !strings.ContainsRune(text, preHolder) {
		return text
	}

	return strings.ReplaceAll(text, string(preHolder), "")
}

// utf16Len returns the number of UTF-16 code units in a string.
func utf16Len(str string) int {
	return len(utf16.Encode([]rune(str)))
}

// extractCodeBlocks finds fenced code blocks and replaces each with a single
// placeholder rune. Body and language are stored in order for substitution
// later, after all other passes finish mutating the text.
func extractCodeBlocks(text string) (string, []codeBlock) {
	var result strings.Builder

	var blocks []codeBlock

	for {
		idx := strings.Index(text, "```")
		if idx == -1 {
			break
		}

		closeIdx, lang, body := findCodeBlockEnd(text, idx)
		if closeIdx == -1 {
			break
		}

		result.WriteString(text[:idx])
		result.WriteRune(preHolder)

		blocks = append(blocks, codeBlock{body: body, lang: lang})

		text = text[closeIdx:]
	}

	result.WriteString(text)

	return result.String(), blocks
}

// substituteCodeBlocks walks text, replacing each preHolder rune with the
// corresponding saved body. It emits one rawEntity per block with its offset
// taken from the final builder state, and shifts every previously-parsed
// entity whose start lies past the placeholder by utf16Len(body) - 1 so the
// blockquote/inline/bold entities continue to point at the right slice.
func substituteCodeBlocks(
	text string, entities []rawEntity, blocks []codeBlock,
) (string, []rawEntity) {
	if len(blocks) == 0 {
		return text, entities
	}

	var result strings.Builder

	out := append([]rawEntity{}, entities...)
	blockIdx := 0

	for _, runeValue := range text {
		if runeValue != preHolder {
			result.WriteRune(runeValue)

			continue
		}

		start := utf16Len(result.String())
		body := blocks[blockIdx].body
		bodyLen := utf16Len(body)

		result.WriteString(body)

		out = append(out, rawEntity{
			start:  start,
			length: bodyLen,
			kind:   "pre",
			extra:  blocks[blockIdx].lang,
		})

		shiftAfter(out, start, bodyLen-1)

		blockIdx++
	}

	return result.String(), out
}

// shiftAfter adds delta to the start field of every entity whose start is
// strictly greater than threshold. Equal-start entities are not shifted:
// they belong at the placeholder position itself (the pre entity we just
// emitted); nothing else should legitimately share that coordinate.
func shiftAfter(entities []rawEntity, threshold, delta int) {
	if delta == 0 {
		return
	}

	for idx := range entities {
		if entities[idx].start > threshold {
			entities[idx].start += delta
		}
	}
}

// fenceLen is the length of the ``` fence marker.
const fenceLen = 3

// findCodeBlockEnd locates closing ``` and extracts language and body.
//
//nolint:gocritic // unnamedResult conflicts with nonamedreturns linter.
func findCodeBlockEnd(
	text string, idx int,
) (int, string, string) {
	after := text[idx+fenceLen:]
	closePos := strings.Index(after, "```")

	if closePos == -1 {
		return -1, "", ""
	}

	inner := after[:closePos]
	lang, body := splitCodeBlock(inner)

	return idx + fenceLen + closePos + fenceLen, lang, body
}

// splitCodeBlock separates language hint from code body.
//
//nolint:gocritic // unnamedResult conflicts with nonamedreturns linter.
func splitCodeBlock(inner string) (string, string) {
	before, after, found := strings.Cut(inner, "\n")
	if !found {
		return "", inner
	}

	lang := strings.TrimSpace(before)
	body := strings.TrimSuffix(after, "\n")

	return lang, body
}

// blockquoteLine represents a parsed line with optional blockquote prefix.
type blockquoteLine struct {
	text    string
	isQuote bool
}

// quotePrefixLen is the UTF-16 length of the "> " marker stripped from each
// blockquote line.
const quotePrefixLen = 2

// extractBlockquotes processes lines starting with "> ". Existing entities
// (inline markers, code, etc.) come in already remapped onto the cleaned text;
// this pass strips the "> " prefixes, emits blockquote entities at offsets
// taken from the cleaned-text builder, and shifts every prior entity left by
// 2 UTF-16 units for each "> " prefix that lay before its old start.
func extractBlockquotes(
	text string, existing []rawEntity,
) (string, []rawEntity) {
	lines := strings.Split(text, "\n")
	parsed := parseQuoteLines(lines)

	return buildBlockquoteResult(parsed, existing)
}

// parseQuoteLines classifies each line as quoted or plain.
func parseQuoteLines(lines []string) []blockquoteLine {
	parsed := make([]blockquoteLine, len(lines))

	for idx, line := range lines {
		if strings.HasPrefix(line, "> ") {
			parsed[idx] = blockquoteLine{
				text: line[2:], isQuote: true,
			}
		} else {
			parsed[idx] = blockquoteLine{
				text: line, isQuote: false,
			}
		}
	}

	return parsed
}

// buildBlockquoteResult assembles plain text, emits new blockquote entities,
// and rebases existing entities to account for stripped "> " prefixes.
// Empty "> " lines are intentionally not emitted as blockquote entities:
// Telegram MTProto rejects entities with length=0.
func buildBlockquoteResult(
	parsed []blockquoteLine, existing []rawEntity,
) (string, []rawEntity) {
	var result strings.Builder

	stripPositions := make([]int, 0, len(parsed))

	newEntities := make([]rawEntity, 0, len(parsed))
	oldOffset := 0

	for idx, line := range parsed {
		if idx > 0 {
			result.WriteByte('\n')

			oldOffset++
		}

		if line.isQuote {
			stripPositions = append(stripPositions, oldOffset)
			innerStart := utf16Len(result.String())

			result.WriteString(line.text)

			lineLen := utf16Len(line.text)
			if lineLen > 0 {
				newEntities = append(newEntities, rawEntity{
					start:  innerStart,
					length: lineLen,
					kind:   "blockquote",
				})
			}

			oldOffset += quotePrefixLen + lineLen
		} else {
			result.WriteString(line.text)

			oldOffset += utf16Len(line.text)
		}
	}

	adjusted := shiftForStrippedQuotes(existing, stripPositions)

	return result.String(), append(adjusted, newEntities...)
}

// shiftForStrippedQuotes returns existing entities rebased onto the
// post-strip UTF-16 space. For each entity:
//   - every "> " prefix that lay strictly before its old start moves the
//     start left by quotePrefixLen;
//   - every "> " prefix that lay strictly inside its old range
//     [start, start+length) shrinks the length by quotePrefixLen.
//
// stripPositions are UTF-16 offsets in the pre-strip text where each "> "
// starts. The inline parser does not emit entities that begin or end inside
// a "> " prefix, so partial-overlap cases are not handled here.
func shiftForStrippedQuotes(
	existing []rawEntity, stripPositions []int,
) []rawEntity {
	if len(existing) == 0 || len(stripPositions) == 0 {
		return append([]rawEntity{}, existing...)
	}

	out := make([]rawEntity, len(existing))
	copy(out, existing)

	for idx := range out {
		out[idx] = remapForStrips(out[idx], stripPositions)
	}

	return out
}

// remapForStrips applies the start/length adjustment for one entity.
func remapForStrips(ent rawEntity, stripPositions []int) rawEntity {
	startShift := 0
	lengthShift := 0
	end := ent.start + ent.length

	for _, sp := range stripPositions {
		switch {
		case sp+quotePrefixLen <= ent.start:
			startShift += quotePrefixLen
		case ent.start <= sp && sp+quotePrefixLen <= end:
			lengthShift += quotePrefixLen
		}
	}

	ent.start -= startShift
	ent.length -= lengthShift

	return ent
}

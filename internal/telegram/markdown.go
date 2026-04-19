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
// Pipeline: fenced blocks are replaced with a single-rune placeholder first
// so that the blockquote, inline-marker, and escape passes operate on a
// text where the only mutable content is the marker they own. After those
// passes, substituteCodeBlocks splices the saved bodies back in and emits
// the pre entities with offsets taken from the final builder state, so no
// earlier pass can ever corrupt a pre offset by deleting marker bytes.
func ParseMarkdown(text string) (string, []tg.MessageEntityClass) {
	text = sanitizePlaceholders(text)

	plain, blocks := extractCodeBlocks(text)

	var entities []rawEntity

	plain, entities = extractBlockquotes(plain, entities)
	plain, entities = extractInlineEntities(plain, entities)
	plain, entities = removeEscapes(plain, entities)
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

// extractBlockquotes processes lines starting with "> ".
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

// buildBlockquoteResult assembles plain text and blockquote entities.
func buildBlockquoteResult(
	parsed []blockquoteLine, existing []rawEntity,
) (string, []rawEntity) {
	var result strings.Builder

	entities := append([]rawEntity{}, existing...)

	for idx, line := range parsed {
		if idx > 0 {
			result.WriteByte('\n')
		}

		if line.isQuote {
			start := utf16Len(result.String())
			result.WriteString(line.text)
			entities = append(entities, rawEntity{
				start: start, length: utf16Len(line.text),
				kind: "blockquote",
			})
		} else {
			result.WriteString(line.text)
		}
	}

	return result.String(), entities
}

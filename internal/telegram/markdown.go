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

// ParseMarkdown converts Markdown text to plain text and Telegram entities.
func ParseMarkdown(text string) (string, []tg.MessageEntityClass) {
	plain, entities := extractCodeBlocks(text)
	plain, entities = extractBlockquotes(plain, entities)
	plain, entities = extractInlineEntities(plain, entities)
	plain, entities = removeEscapes(plain, entities)

	return plain, toTelegramEntities(entities)
}

// utf16Len returns the number of UTF-16 code units in a string.
func utf16Len(str string) int {
	return len(utf16.Encode([]rune(str)))
}

// extractCodeBlocks finds fenced code blocks and replaces them with placeholders.
func extractCodeBlocks(text string) (string, []rawEntity) {
	var result strings.Builder

	var entities []rawEntity

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

		start := utf16Len(result.String())

		result.WriteString(body)
		entities = append(entities, rawEntity{
			start: start, length: utf16Len(body),
			kind: "pre", extra: lang,
		})

		text = text[closeIdx:]
	}

	result.WriteString(text)

	return result.String(), entities
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

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
// Inline markers are resolved next, so every inline-entity offset lives in
// the cleaned UTF-16 space (no backticks, asterisks, etc., absorbed into the
// count). extractBlockquotes then runs against that text — crucially before
// removeEscapes, so a backslash-escaped "\> literal" line still begins with
// "\" rather than ">", and is not promoted to a blockquote. extractBlockquotes
// also rebases every existing entity onto the post-strip space: entities that
// follow a "> " prefix have their start shifted left by 2 UTF-16 units, and
// entities whose range straddles a "> " prefix (e.g. **bold spanning across
// a quoted line**) have their length shrunk by the same amount per stripped
// prefix. removeEscapes then strips the backslashes and remaps offsets, and
// substituteCodeBlocks splices the saved fenced bodies back in, emitting pre
// entities with offsets taken from the final builder state.
func ParseMarkdown(text string) (string, []tg.MessageEntityClass) {
	text = sanitizePlaceholders(text)

	plain, blocks := extractCodeBlocks(text)

	var entities []rawEntity

	plain, entities = extractInlineEntities(plain, entities)
	plain, entities = extractBlockquotes(plain, entities)
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
// stripLen records the UTF-16 length of the consumed prefix marker — 2 for
// the canonical "> " form, 1 for a bare ">" continuation line (CommonMark
// §5.1), and 0 for non-quote lines.
type blockquoteLine struct {
	text     string
	isQuote  bool
	stripLen int
}

// stripRange describes a single removed quote-prefix span in the pre-strip
// UTF-16 coordinate system: [start, start+length).
type stripRange struct {
	start  int
	length int
}

// extractBlockquotes processes ">"-prefixed lines. Existing entities come in
// already remapped onto the cleaned text from the inline pass; this pass
// strips the prefixes, merges consecutive quoted lines into one entity per
// run (per CommonMark §5.1: a bare ">" continues the same blockquote), and
// shifts prior entities to account for the removed UTF-16 units.
func extractBlockquotes(
	text string, existing []rawEntity,
) (string, []rawEntity) {
	lines := strings.Split(text, "\n")
	parsed := parseQuoteLines(lines)

	return buildBlockquoteResult(parsed, existing)
}

// parseQuoteLines classifies each line. Recognised quote forms:
//   - "> X..."  → quote line with content X, stripLen=2
//   - ">"        → bare continuation line (empty content), stripLen=1
//
// Anything else is plain text. The escaped form "\>" is not handled here —
// the leading backslash prevents the prefix match and the escape is removed
// in a later pass.
func parseQuoteLines(lines []string) []blockquoteLine {
	parsed := make([]blockquoteLine, len(lines))

	for idx, line := range lines {
		switch {
		case strings.HasPrefix(line, "> "):
			parsed[idx] = blockquoteLine{
				text: line[2:], isQuote: true, stripLen: 2,
			}
		case line == ">":
			parsed[idx] = blockquoteLine{
				text: "", isQuote: true, stripLen: 1,
			}
		default:
			parsed[idx] = blockquoteLine{
				text: line, isQuote: false,
			}
		}
	}

	return parsed
}

// blockquoteState carries the running data while we walk the parsed lines
// in buildBlockquoteResult. runStart is the cleaned-text UTF-16 offset where
// the current quoted run began, or -1 when no run is open.
type blockquoteState struct {
	out         strings.Builder
	stripRanges []stripRange
	entities    []rawEntity
	oldOffset   int
	runStart    int
}

// closeRun emits one blockquote entity for the open run (if any) covering
// [runStart, end), then resets runStart. Zero-length spans are dropped:
// Telegram MTProto rejects length=0 entities.
func (state *blockquoteState) closeRun(end int) {
	if state.runStart < 0 {
		return
	}

	if end > state.runStart {
		state.entities = append(state.entities, rawEntity{
			start: state.runStart, length: end - state.runStart, kind: EntityTypeBlockquote,
		})
	}

	state.runStart = -1
}

// appendInterLineNewline writes the '\n' that joins the previous line to the
// current one, after first closing any open run that won't continue.
func (state *blockquoteState) appendInterLineNewline(currentIsQuote bool) {
	if !currentIsQuote && state.runStart >= 0 {
		state.closeRun(utf16Len(state.out.String()))
	}

	state.out.WriteByte('\n')
	state.oldOffset++
}

// appendLine writes one parsed line's content and tracks strip ranges. For
// quote lines it opens a run if none is open and records the stripped prefix.
func (state *blockquoteState) appendLine(line blockquoteLine) {
	if line.isQuote {
		if state.runStart < 0 {
			state.runStart = utf16Len(state.out.String())
		}

		state.stripRanges = append(state.stripRanges, stripRange{
			start: state.oldOffset, length: line.stripLen,
		})
	}

	state.out.WriteString(line.text)
	state.oldOffset += line.stripLen + utf16Len(line.text)
}

// buildBlockquoteResult assembles plain text, emits one blockquote entity
// per consecutive run of quoted lines (so "> A\n>\n> B" is ONE entity, not
// two), and rebases existing entities to account for stripped prefixes.
// Zero-length runs (a single bare ">" on its own with no quoted neighbours,
// or "> " producing only an empty content line) emit no entity — Telegram
// MTProto rejects length=0 entities.
func buildBlockquoteResult(
	parsed []blockquoteLine, existing []rawEntity,
) (string, []rawEntity) {
	state := blockquoteState{
		stripRanges: make([]stripRange, 0, len(parsed)),
		entities:    make([]rawEntity, 0),
		runStart:    -1,
	}

	for idx, line := range parsed {
		if idx > 0 {
			state.appendInterLineNewline(line.isQuote)
		}

		state.appendLine(line)
	}

	state.closeRun(utf16Len(state.out.String()))

	adjusted := shiftForStrippedQuotes(existing, state.stripRanges)

	return state.out.String(), append(adjusted, state.entities...)
}

// shiftForStrippedQuotes returns existing entities rebased onto the
// post-strip UTF-16 space. For each strip range and each entity range:
//   - units of the strip range lying before the entity move start left;
//   - units of the strip range lying inside the entity shrink length.
//
// Strip ranges have variable length: 2 for a canonical "> " prefix, 1 for a
// bare ">" continuation line. The same arithmetic handles both, plus
// partial-overlap cases (e.g. bold ending exactly on the ">" of a "> ").
func shiftForStrippedQuotes(
	existing []rawEntity, stripRanges []stripRange,
) []rawEntity {
	if len(existing) == 0 || len(stripRanges) == 0 {
		return append([]rawEntity{}, existing...)
	}

	out := make([]rawEntity, len(existing))
	copy(out, existing)

	for idx := range out {
		out[idx] = remapForStrips(out[idx], stripRanges)
	}

	return out
}

// remapForStrips applies the start/length adjustment for one entity.
func remapForStrips(ent rawEntity, stripRanges []stripRange) rawEntity {
	startShift := 0
	lengthShift := 0
	end := ent.start + ent.length

	for _, sr := range stripRanges {
		spEnd := sr.start + sr.length
		startShift += rangeOverlap(sr.start, spEnd, 0, ent.start)
		lengthShift += rangeOverlap(sr.start, spEnd, ent.start, end)
	}

	ent.start -= startShift
	ent.length -= lengthShift

	return ent
}

// rangeOverlap returns the length of [aStart,aEnd) ∩ [bStart,bEnd). Returns
// 0 when either input is empty or the ranges do not overlap.
func rangeOverlap(aStart, aEnd, bStart, bEnd int) int {
	lower := max(aStart, bStart)
	upper := min(aEnd, bEnd)

	if upper <= lower {
		return 0
	}

	return upper - lower
}

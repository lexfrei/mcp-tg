package telegram

import (
	"regexp"
	"strings"
	"sync"
)

// markdownHints compiles conservative approximations of the constructs
// ParseMarkdown would transform. Precision over recall: single
// *italic* / _italic_ emphasis and > quotes are deliberately excluded
// as too false-positive-prone in ordinary prose (multiplication,
// snake_case, quoted replies). Doubled markers require
// non-whitespace-flanked content, AND the opening marker must start a
// word (line start or whitespace before it) — CommonMark's left-flanking
// rule in miniature. Without that second half the lint fired on code:
// "(a||b)", "2**3**2" and the Go generic "Foo[T](x)" all look like
// spoilers, bold and a link when scanned naively, and code is the most
// common thing anyone pastes into Telegram. Note that __init__ IS a
// true positive
// by design — the parser really would underline "init" in commonmark
// mode — and allowRawMarkdown is the documented escape.
//
// 4-space indented code blocks are also excluded even though the
// parser supports them: indentation is how people paste logs and
// stack traces, which would make the lint fire on ordinary plain
// sends far too often.
//
//nolint:gochecknoglobals // compile the hint set once, not on every plain-mode send.
var markdownHints = sync.OnceValue(func() []*regexp.Regexp {
	// (?m)(?:^|\s) — the opening marker must start a word.
	return []*regexp.Regexp{
		regexp.MustCompile("`[^`\n]+`"),                            // inline code
		regexp.MustCompile(`(?m)(?:^|\s)\*\*\S(?:[^*\n]*\S)?\*\*`), // bold
		regexp.MustCompile(`(?m)(?:^|\s)__\S(?:[^_\n]*\S)?__`),     // underline
		regexp.MustCompile(`(?m)(?:^|\s)~~\S(?:[^~\n]*\S)?~~`),     // strikethrough
		regexp.MustCompile(`(?m)(?:^|\s)\|\|\S(?:[^|\n]*\S)?\|\|`), // spoiler
		regexp.MustCompile(`(?m)(?:^|\s)\[[^\]\n]+\]\([^)\s]+\)`),  // [text](url)
		regexp.MustCompile(`<https?://[^>\s]+>`),                   // <autolink>
	}
})

// LooksLikeMarkdown reports whether text contains constructs the
// CommonMark parser would transform, so a plain-mode send can be
// flagged before the formatting silently ships as literal characters.
// Fences are matched by bare substring — the same aggressiveness
// ParseMarkdown's own fence scanner has.
func LooksLikeMarkdown(text string) bool {
	if strings.Contains(text, "```") || strings.Contains(text, "~~~") {
		return true
	}

	for _, hint := range markdownHints() {
		if hint.MatchString(text) {
			return true
		}
	}

	return false
}

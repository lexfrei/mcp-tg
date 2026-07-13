package telegram

import (
	"regexp"
	"strings"
)

// markdownHints compiles conservative approximations of the constructs
// ParseMarkdown would transform. Precision over recall: single
// *italic* / _italic_ emphasis and > quotes are deliberately excluded
// as too false-positive-prone in ordinary prose (multiplication,
// snake_case, quoted replies). Doubled markers require
// non-whitespace-flanked content so "a || b || c" (shell OR) and
// "x ** y ** z" don't trigger. Note that __init__ IS a true positive
// by design — the parser really would underline "init" in commonmark
// mode — and allowRawMarkdown is the documented escape.
func markdownHints() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile("`[^`\n]+`"),                // inline code
		regexp.MustCompile(`\*\*\S(?:[^*\n]*\S)?\*\*`), // bold
		regexp.MustCompile(`__\S(?:[^_\n]*\S)?__`),     // underline
		regexp.MustCompile(`~~\S(?:[^~\n]*\S)?~~`),     // strikethrough
		regexp.MustCompile(`\|\|\S(?:[^|\n]*\S)?\|\|`), // spoiler
		regexp.MustCompile(`\[[^\]\n]+\]\([^)\s]+\)`),  // [text](url)
	}
}

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

package telegram

// Tests pinning the v0.11.0 audit findings against CommonMark spec.
// Each TestAudit_* corresponds to one finding from the markdown audit.
//
// Five tests pin the spec deviations being fixed in v0.12.0:
//
//   - TestAudit_BlockquoteNoSpace
//   - TestAudit_TildeFencedCode
//   - TestAudit_BackslashInCodeSpan
//   - TestAudit_IndentedCodeBlock
//   - TestAudit_AngleBracketAutolink
//
// Six tests pin existing CORRECT behaviour to guard against regressions:
//
//   - TestAudit_LazyContinuationStable
//   - TestAudit_WordInternalUnderscoreStable
//   - TestAudit_ATXHeaderPassthroughStable
//   - TestAudit_SetextHeaderPassthroughStable
//   - TestAudit_BackslashHardBreakStable
//   - (one more if extracted later)
//
// Three audit findings are deliberately NOT fixed in this PR. They are
// captured below as commented-out test blocks describing the expected
// post-fix behaviour. Uncomment and unblock when the matching limitation
// is addressed. See README.md "Known Limitations".

import (
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

// --- Active tests pinning fixes shipped in this PR ---

// CommonMark §5.1: the space after `>` is optional. `>foo` must produce a
// blockquote with content `foo`.
func TestAudit_BlockquoteNoSpace(t *testing.T) {
	text, entities := ParseMarkdown(">foo")

	if text != "foo" {
		t.Fatalf("plain = %q, want %q", text, "foo")
	}

	bqs := findAllBlockquotes(entities)
	if len(bqs) != 1 {
		t.Fatalf("expected 1 blockquote, got %d", len(bqs))
	}

	assertEntitySlice(t, text, bqs[0].Offset, bqs[0].Length, "foo", "blockquote")
}

// CommonMark §4.5: tilde fences (~~~) are equivalent to backtick fences.
// Currently the parser only knows about ```; ~~~ gets misparsed as a
// strikethrough plus a stray tilde.
func TestAudit_TildeFencedCode(t *testing.T) {
	text, entities := ParseMarkdown("~~~go\nx\n~~~")

	if strings.Contains(text, "~") {
		t.Errorf("plain must not retain literal tildes: %q", text)
	}

	pres := findAllPre(entities)
	if len(pres) != 1 {
		t.Fatalf("expected 1 pre entity, got %d", len(pres))
	}

	if pres[0].Language != "go" {
		t.Errorf("Language = %q, want %q", pres[0].Language, "go")
	}

	assertPreBody(t, text, pres[0], "x")
}

// CommonMark §6.1: backslash escapes inside code spans are NOT processed.
// “ `\*` “ must render as the two literal characters `\` and `*`, not `*`.
func TestAudit_BackslashInCodeSpan(t *testing.T) {
	text, entities := ParseMarkdown("`\\*asterisk`")

	const wantPlain = `\*asterisk`
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	code := findCode(entities)
	if code == nil {
		t.Fatalf("code entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, code.Offset, code.Length, wantPlain, "code")
}

// CommonMark §4.4: an indented code block (4 leading spaces, after blank
// line) is a literal code block with no language hint.
func TestAudit_IndentedCodeBlock(t *testing.T) {
	text, entities := ParseMarkdown("paragraph\n\n    line1\n    line2\n\nafter")

	if strings.Contains(text, "    ") {
		t.Errorf("plain text must not retain 4-space indents inside code block: %q", text)
	}

	pres := findAllPre(entities)
	if len(pres) != 1 {
		t.Fatalf("expected 1 pre entity, got %d in %+v", len(pres), entities)
	}

	if pres[0].Language != "" {
		t.Errorf("indented code block has no language; got %q", pres[0].Language)
	}

	assertPreBody(t, text, pres[0], "line1\nline2")
}

// CommonMark §6.3: an autolink in <...> form expands to a link entity. We
// emit it as text_url with URL == display text.
func TestAudit_AngleBracketAutolink(t *testing.T) {
	text, entities := ParseMarkdown("<https://example.com>")

	const wantPlain = "https://example.com"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	var link *tg.MessageEntityTextURL

	for _, ent := range entities {
		if l, ok := ent.(*tg.MessageEntityTextURL); ok {
			link = l
		}
	}

	if link == nil {
		t.Fatalf("text_url entity missing in %+v", entities)
	}

	if link.URL != wantPlain {
		t.Errorf("URL = %q, want %q", link.URL, wantPlain)
	}

	assertEntitySlice(t, text, link.Offset, link.Length, wantPlain, "autolink")
}

// --- Stable behaviour pins (no fix, just guard against regression) ---

// Lazy continuation: a non-`>` line directly after a `>`-line is treated as
// part of the blockquote's paragraph. Currently works by accident because
// the parser does NOT enforce CommonMark's "no blank line between" rule —
// pin the working case so refactors don't regress it.
func TestAudit_LazyContinuationStable(t *testing.T) {
	text, entities := ParseMarkdown("> start\ncontinuation")

	if !strings.Contains(text, "start") || !strings.Contains(text, "continuation") {
		t.Errorf("expected both lines in plain: %q", text)
	}

	bqs := findAllBlockquotes(entities)
	if len(bqs) == 0 {
		t.Errorf("expected at least one blockquote entity")
	}
}

// CommonMark §6.4: word-internal underscores must NOT trigger emphasis.
// `foo_bar_baz` should remain plain text. The needsWordBoundary check on
// `_` markers handles this; pin to prevent regression.
func TestAudit_WordInternalUnderscoreStable(t *testing.T) {
	text, entities := ParseMarkdown("foo_bar_baz")

	if text != "foo_bar_baz" {
		t.Errorf("plain = %q, want %q", text, "foo_bar_baz")
	}

	for _, ent := range entities {
		if _, ok := ent.(*tg.MessageEntityItalic); ok {
			t.Errorf("unexpected italic entity inside snake_case: %+v", ent)
		}
	}
}

// ATX headers (`# H`) have no Telegram entity equivalent; the literal `#`
// must pass through unchanged.
func TestAudit_ATXHeaderPassthroughStable(t *testing.T) {
	text, entities := ParseMarkdown("# Heading")

	if text != "# Heading" {
		t.Errorf("plain = %q, want %q (ATX header passthrough)", text, "# Heading")
	}

	if len(entities) != 0 {
		t.Errorf("expected 0 entities for header, got %d: %+v", len(entities), entities)
	}
}

// Setext headers (underline-style) likewise pass through unchanged.
func TestAudit_SetextHeaderPassthroughStable(t *testing.T) {
	text, entities := ParseMarkdown("Heading\n=======")

	if text != "Heading\n=======" {
		t.Errorf("plain = %q, want %q (setext header passthrough)", text, "Heading\n=======")
	}

	if len(entities) != 0 {
		t.Errorf("expected 0 entities for setext header, got %d", len(entities))
	}
}

// CommonMark §6.7 hard line break with backslash: a `\` immediately before
// `\n` triggers a hard break. Telegram has no break entity but the `\n`
// alone renders correctly. Current parser strips the backslash via the
// generic escape pass, which gives the right output by accident — pin
// that the trailing `\` is consumed and the newline survives.
func TestAudit_BackslashHardBreakStable(t *testing.T) {
	text, _ := ParseMarkdown("line1\\\nline2")

	if text != "line1\nline2" {
		t.Errorf("plain = %q, want %q (backslash consumed, newline kept)", text, "line1\nline2")
	}
}

// --- Known Limitations: not fixed in this PR ---
//
// The blocks below describe the expected post-fix behaviour for each known
// limitation. Uncomment and unblock when the corresponding limitation is
// addressed. See README.md "Known Limitations" for the rationale.

/*
// CommonMark §5.1: nested blockquotes. Inner `>` should produce a nested
// blockquote (or, in a flatter rendering, be stripped and treated as the
// outer blockquote's content with a depth marker). Currently the inner `>`
// is left as a literal character in the output.
func TestAudit_NestedBlockquoteKnownLimitation(t *testing.T) {
	text, entities := ParseMarkdown("> > nested")

	if strings.Contains(text, ">") {
		t.Errorf("plain must not contain literal '>' for nested blockquote: %q", text)
	}

	bqs := findAllBlockquotes(entities)
	if len(bqs) != 2 {
		t.Errorf("expected 2 nested blockquote entities, got %d", len(bqs))
	}
}
*/

/*
// CommonMark §6.4: nested emphasis. `**bold *italic***` should produce a
// bold entity over the whole span and an italic entity over `italic`.
// Current parser drops the inner italic and leaves literal asterisks in
// the output. Requires a delimiter-run rewrite of the inline parser.
func TestAudit_NestedEmphasisKnownLimitation(t *testing.T) {
	text, entities := ParseMarkdown("**bold *italic***")

	if strings.Contains(text, "*") {
		t.Errorf("plain must not contain literal '*': %q", text)
	}

	var (
		bold   *tg.MessageEntityBold
		italic *tg.MessageEntityItalic
	)

	for _, ent := range entities {
		switch e := ent.(type) {
		case *tg.MessageEntityBold:
			bold = e
		case *tg.MessageEntityItalic:
			italic = e
		}
	}

	if bold == nil || italic == nil {
		t.Errorf("expected both bold and italic entities, got %+v", entities)
	}
}
*/

/*
// CommonMark §6.7 hard line break via two trailing spaces. Two spaces
// before `\n` should signal a hard break — but Telegram has no break
// entity, so the practical fix would be to strip the trailing spaces
// while keeping the newline. Currently they are preserved verbatim.
// Decision deferred: stripping spaces is data corruption when the user
// did not intend a hard break. See Known Limitations.
func TestAudit_TwoSpaceHardBreakKnownLimitation(t *testing.T) {
	text, _ := ParseMarkdown("line1  \nline2")

	if text != "line1\nline2" {
		t.Errorf("plain = %q, want %q (trailing spaces stripped)", text, "line1\nline2")
	}
}
*/

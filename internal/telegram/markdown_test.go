package telegram

import (
	"testing"
	"unicode/utf16"

	"github.com/gotd/td/tg"
)

// preSlice returns the UTF-16 substring of plain at [offset, offset+length).
// Entity offsets are UTF-16 code units, and the invariant for a pre entity is
// that this slice must equal the original fenced body byte-for-byte.
func preSlice(plain string, offset, length int) string {
	units := utf16.Encode([]rune(plain))
	if offset < 0 || offset+length > len(units) {
		return ""
	}

	return string(utf16.Decode(units[offset : offset+length]))
}

// findPre returns the first MessageEntityPre in the slice, or nil.
func findPre(ents []tg.MessageEntityClass) *tg.MessageEntityPre {
	for _, ent := range ents {
		if pre, ok := ent.(*tg.MessageEntityPre); ok {
			return pre
		}
	}

	return nil
}

// findAllPre returns every MessageEntityPre in the slice, in order.
func findAllPre(ents []tg.MessageEntityClass) []*tg.MessageEntityPre {
	var out []*tg.MessageEntityPre

	for _, ent := range ents {
		if pre, ok := ent.(*tg.MessageEntityPre); ok {
			out = append(out, pre)
		}
	}

	return out
}

// assertPreBody fails the test unless the pre entity points at exactly body in plain.
func assertPreBody(t *testing.T, plain string, pre *tg.MessageEntityPre, body string) {
	t.Helper()

	if pre == nil {
		t.Fatalf("no pre entity found in %q", plain)
	}

	got := preSlice(plain, pre.Offset, pre.Length)
	if got != body {
		t.Fatalf(
			"pre slice mismatch: offset=%d length=%d\n got=%q\nwant=%q\nplain=%q",
			pre.Offset, pre.Length, got, body, plain,
		)
	}
}

func TestParseMarkdown_TripleAsteriskKeepsStrayLiteral(t *testing.T) {
	// Regression: pattern "**word***" previously closed bold, then
	// the leftover '*' opened italic that ran across the rest of the
	// message and ate another '*' at the far end. Expected behaviour:
	// bold "word" + literal '*'.
	text, entities := ParseMarkdown("«**зассал***». дальше **PR:**")

	const wantPlain = "«зассал*». дальше PR:"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	if len(entities) != 2 {
		t.Fatalf("want 2 bold entities, got %d (%+v)", len(entities), entities)
	}

	for idx, ent := range entities {
		if _, ok := ent.(*tg.MessageEntityBold); !ok {
			t.Errorf("entity[%d] = %T, want Bold", idx, ent)
		}
	}
}

func TestParseMarkdown_Bold(t *testing.T) {
	text, entities := ParseMarkdown("hello **world**")
	if text != testMessageText {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	bold, ok := entities[0].(*tg.MessageEntityBold)
	if !ok {
		t.Fatalf("expected Bold, got %T", entities[0])
	}

	if bold.Offset != 6 || bold.Length != 5 {
		t.Fatalf("offset=%d length=%d", bold.Offset, bold.Length)
	}
}

func TestParseMarkdown_Italic(t *testing.T) {
	text, entities := ParseMarkdown("hello *world*")
	if text != testMessageText {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	ent, ok := entities[0].(*tg.MessageEntityItalic)
	if !ok {
		t.Fatalf("expected Italic, got %T", entities[0])
	}

	if ent.Offset != 6 || ent.Length != 5 {
		t.Fatalf("offset=%d length=%d", ent.Offset, ent.Length)
	}
}

func TestParseMarkdown_Code(t *testing.T) {
	text, entities := ParseMarkdown("hello `world`")
	if text != testMessageText {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	ent, ok := entities[0].(*tg.MessageEntityCode)
	if !ok {
		t.Fatalf("expected Code, got %T", entities[0])
	}

	if ent.Offset != 6 || ent.Length != 5 {
		t.Fatalf("offset=%d length=%d", ent.Offset, ent.Length)
	}
}

func TestParseMarkdown_Pre(t *testing.T) {
	text, entities := ParseMarkdown("```go\nfmt.Println()\n```")
	if text != "fmt.Println()" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	ent, ok := entities[0].(*tg.MessageEntityPre)
	if !ok {
		t.Fatalf("expected Pre, got %T", entities[0])
	}

	if ent.Offset != 0 || ent.Length != 13 {
		t.Fatalf("offset=%d length=%d", ent.Offset, ent.Length)
	}

	if ent.Language != testLangGo {
		t.Fatalf("language=%q", ent.Language)
	}
}

func TestParseMarkdown_Strike(t *testing.T) {
	text, entities := ParseMarkdown("~~deleted~~")
	if text != "deleted" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	_, ok := entities[0].(*tg.MessageEntityStrike)
	if !ok {
		t.Fatalf("expected Strike, got %T", entities[0])
	}
}

func TestParseMarkdown_Underline(t *testing.T) {
	text, entities := ParseMarkdown("__important__")
	if text != "important" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	_, ok := entities[0].(*tg.MessageEntityUnderline)
	if !ok {
		t.Fatalf("expected Underline, got %T", entities[0])
	}
}

func TestParseMarkdown_Spoiler(t *testing.T) {
	text, entities := ParseMarkdown("||secret||")
	if text != "secret" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	_, ok := entities[0].(*tg.MessageEntitySpoiler)
	if !ok {
		t.Fatalf("expected Spoiler, got %T", entities[0])
	}
}

func TestParseMarkdown_Link(t *testing.T) {
	text, entities := ParseMarkdown("[click](https://example.com)")
	if text != "click" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	ent, ok := entities[0].(*tg.MessageEntityTextURL)
	if !ok {
		t.Fatalf("expected TextURL, got %T", entities[0])
	}

	if ent.URL != testExampleURL {
		t.Fatalf("url=%q", ent.URL)
	}

	if ent.Offset != 0 || ent.Length != 5 {
		t.Fatalf("offset=%d length=%d", ent.Offset, ent.Length)
	}
}

func TestParseMarkdown_Blockquote(t *testing.T) {
	text, entities := ParseMarkdown("> quoted text")
	if text != "quoted text" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	_, ok := entities[0].(*tg.MessageEntityBlockquote)
	if !ok {
		t.Fatalf("expected Blockquote, got %T", entities[0])
	}
}

func TestParseMarkdown_Emoji(t *testing.T) {
	text, entities := ParseMarkdown("\U0001f389 **bold**")
	if text != "\U0001f389 bold" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	bold, ok := entities[0].(*tg.MessageEntityBold)
	if !ok {
		t.Fatalf("expected Bold, got %T", entities[0])
	}

	// emoji U+1F389 = 2 UTF-16 units, space = 1
	if bold.Offset != 3 || bold.Length != 4 {
		t.Fatalf("offset=%d length=%d", bold.Offset, bold.Length)
	}
}

func TestParseMarkdown_PlainText(t *testing.T) {
	text, entities := ParseMarkdown("no formatting")
	if text != "no formatting" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(entities))
	}
}

func TestParseMarkdown_Mixed(t *testing.T) {
	text, entities := ParseMarkdown("**bold** and *italic*")
	if text != "bold and italic" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 2 {
		t.Fatalf("expected 2 entities, got %d", len(entities))
	}

	if _, ok := entities[0].(*tg.MessageEntityBold); !ok {
		t.Fatalf("expected Bold first, got %T", entities[0])
	}

	if _, ok := entities[1].(*tg.MessageEntityItalic); !ok {
		t.Fatalf("expected Italic second, got %T", entities[1])
	}
}

func TestParseMarkdown_EscapedMarker(t *testing.T) {
	text, entities := ParseMarkdown(`not \*italic\*`)
	if text != "not *italic*" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 0 {
		t.Fatalf("expected 0 entities, got %d", len(entities))
	}
}

func TestParseMarkdown_CodeNoNesting(t *testing.T) {
	text, entities := ParseMarkdown("`**not bold**`")
	if text != "**not bold**" {
		t.Fatalf("unexpected text: %q", text)
	}

	if len(entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(entities))
	}

	_, ok := entities[0].(*tg.MessageEntityCode)
	if !ok {
		t.Fatalf("expected Code, got %T", entities[0])
	}
}

func TestParseMarkdown_UnderscoreWordBoundary(t *testing.T) {
	t.Run("mid-word underscore is literal", func(t *testing.T) {
		text, entities := ParseMarkdown("pull_request_target")
		if text != "pull_request_target" {
			t.Fatalf("expected literal text, got %q", text)
		}
		if len(entities) != 0 {
			t.Fatalf("expected 0 entities, got %d", len(entities))
		}
	})

	t.Run("underscore italic at word boundary", func(t *testing.T) {
		text, entities := ParseMarkdown("hello _world_")
		if text != "hello world" {
			t.Fatalf("unexpected text: %q", text)
		}
		if len(entities) != 1 {
			t.Fatalf("expected 1 entity, got %d", len(entities))
		}
		if _, ok := entities[0].(*tg.MessageEntityItalic); !ok {
			t.Fatalf("expected Italic, got %T", entities[0])
		}
	})

	t.Run("underscore after punctuation", func(t *testing.T) {
		text, entities := ParseMarkdown("see: _important_")
		if text != "see: important" {
			t.Fatalf("unexpected text: %q", text)
		}
		if len(entities) != 1 {
			t.Fatalf("expected 1 entity, got %d", len(entities))
		}
	})

	t.Run("multiple underscores in identifier", func(t *testing.T) {
		text, entities := ParseMarkdown("my_var_name is good")
		if text != "my_var_name is good" {
			t.Fatalf("expected literal, got %q", text)
		}
		if len(entities) != 0 {
			t.Fatalf("expected 0 entities, got %d", len(entities))
		}
	})

	t.Run("underscore in filename", func(t *testing.T) {
		text, entities := ParseMarkdown("file prt_exfil_test.go here")
		if text != "file prt_exfil_test.go here" {
			t.Fatalf("expected literal, got %q", text)
		}
		if len(entities) != 0 {
			t.Fatalf("expected 0 entities, got %d", len(entities))
		}
	})

	t.Run("underscore italic does not trigger before alpha", func(t *testing.T) {
		text, entities := ParseMarkdown("a_b_c")
		if text != "a_b_c" {
			t.Fatalf("expected literal, got %q", text)
		}
		if len(entities) != 0 {
			t.Fatalf("expected 0 entities, got %d", len(entities))
		}
	})
}

func TestParseMarkdown_EdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"unclosed bold", "**bold no close"},
		{"unclosed italic", "*italic no close"},
		{"empty bold", "****"},
		{"empty strike", "~~~~"},
		{"empty spoiler", "||||"},
		{"across newlines", "**bold\nstill bold**"},
		{"unclosed code block", "```go\nfmt.Println()"},
		{"url with query", "[t](https://e.com/?a=1&b=2)"},
		{"empty input", ""},
		{"only spaces", "   "},
		{"single asterisk", "*"},
		{"single backtick", "`"},
	}

	for _, tCase := range cases {
		t.Run(tCase.name, func(t *testing.T) {
			text, entities := ParseMarkdown(tCase.input)
			_ = text
			_ = entities
		})
	}
}

func TestParseMarkdown_InlineCodeBeforePre(t *testing.T) {
	text, entities := ParseMarkdown("`a` `b`\n```\ncode\n```")

	const wantPlain = "a b\ncode"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "code")
}

func TestParseMarkdown_BoldBeforePre(t *testing.T) {
	text, entities := ParseMarkdown("**x** **y**\n```\ncode\n```")

	const wantPlain = "x y\ncode"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "code")
}

func TestParseMarkdown_ItalicBeforePre(t *testing.T) {
	text, entities := ParseMarkdown("*i*\n```\nz\n```")

	const wantPlain = "i\nz"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "z")
}

func TestParseMarkdown_LinkBeforePre(t *testing.T) {
	text, entities := ParseMarkdown("[a](http://x)\n```\nc\n```")

	const wantPlain = "a\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "c")
}

func TestParseMarkdown_StrikeSpoilerUnderlineBeforePre(t *testing.T) {
	cases := []struct {
		name  string
		input string
		plain string
	}{
		{"strike", "~~s~~\n```\nc\n```", "s\nc"},
		{"spoiler", "||s||\n```\nc\n```", "s\nc"},
		{"underline", "__s__\n```\nc\n```", "s\nc"},
	}

	for _, tCase := range cases {
		t.Run(tCase.name, func(t *testing.T) {
			text, entities := ParseMarkdown(tCase.input)
			if text != tCase.plain {
				t.Fatalf("plain = %q, want %q", text, tCase.plain)
			}

			assertPreBody(t, text, findPre(entities), "c")
		})
	}
}

func TestParseMarkdown_BlockquoteBeforePre(t *testing.T) {
	text, entities := ParseMarkdown("> q\n```\nc\n```")

	const wantPlain = "q\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "c")
}

func TestParseMarkdown_MultiplePreBlocks(t *testing.T) {
	text, entities := ParseMarkdown("`x`\n```go\nfirst\n```\n`y`\n```py\nsecond\n```")

	const wantPlain = "x\nfirst\ny\nsecond"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	pres := findAllPre(entities)
	if len(pres) != 2 {
		t.Fatalf("want 2 pre entities, got %d", len(pres))
	}

	assertPreBody(t, text, pres[0], "first")
	assertPreBody(t, text, pres[1], "second")

	if pres[0].Language != "go" {
		t.Errorf("pre[0] language = %q, want %q", pres[0].Language, "go")
	}

	if pres[1].Language != "py" {
		t.Errorf("pre[1] language = %q, want %q", pres[1].Language, "py")
	}
}

func TestParseMarkdown_PreThenInline(t *testing.T) {
	text, entities := ParseMarkdown("```\nc\n```\n**b**")

	const wantPlain = "c\nb"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "c")

	// Bold must land on "b" at the right spot after substitution.
	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if ebold, ok := ent.(*tg.MessageEntityBold); ok {
			bold = ebold
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing")
	}

	got := preSlice(text, bold.Offset, bold.Length)
	if got != "b" {
		t.Fatalf("bold slice = %q, want %q", got, "b")
	}
}

func TestParseMarkdown_InlineSurroundingTwoPres(t *testing.T) {
	text, entities := ParseMarkdown("`a`\n```\nP1\n```\n`b`\n```\nP2\n```\n`c`")

	const wantPlain = "a\nP1\nb\nP2\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	pres := findAllPre(entities)
	if len(pres) != 2 {
		t.Fatalf("want 2 pre entities, got %d", len(pres))
	}

	assertPreBody(t, text, pres[0], "P1")
	assertPreBody(t, text, pres[1], "P2")
}

func TestParseMarkdown_EscapedBeforePre(t *testing.T) {
	text, entities := ParseMarkdown(`\*x\*` + "\n```\nc\n```")

	const wantPlain = "*x*\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "c")
}

func TestParseMarkdown_EmojiBeforePre(t *testing.T) {
	text, entities := ParseMarkdown("\U0001F389 **b**\n```\nc\n```")

	const wantPlain = "\U0001F389 b\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	assertPreBody(t, text, findPre(entities), "c")
}

func TestParseMarkdown_UserTextContainsPlaceholderRune(t *testing.T) {
	// U+FDD0 is the internal placeholder. If it appears in user input it
	// must not be treated as a pre block and must not corrupt offsets.
	text, entities := ParseMarkdown("hello \uFDD0 **world**")

	// Placeholder rune must be stripped or preserved, but never emit a pre.
	for _, ent := range entities {
		if _, ok := ent.(*tg.MessageEntityPre); ok {
			t.Fatalf("unexpected pre entity from placeholder in user input")
		}
	}

	// Bold must still point at "world".
	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if ebold, ok := ent.(*tg.MessageEntityBold); ok {
			bold = ebold
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing")
	}

	got := preSlice(text, bold.Offset, bold.Length)
	if got != "world" {
		t.Fatalf("bold slice = %q, want %q", got, "world")
	}
}

func TestParseMarkdown_RegressionPost12(t *testing.T) {
	// Reproduction of @claudedreams/12: four inline-code entities before a
	// fenced pre block. Previously pre.Offset was computed in the pre-inline
	// text coordinate space and drifted by 2 UTF-16 units per inline code.
	input := "prefix `/etc/rancher/k3s/config.yaml` mid `server: https://...` " +
		"more `--server=...` tail `k3s server --cluster-reset`:\n\n" +
		"```text\ncannot perform cluster-reset\n```\n\nafter"

	text, entities := ParseMarkdown(input)

	const body = "cannot perform cluster-reset"

	pre := findPre(entities)
	assertPreBody(t, text, pre, body)

	if pre.Language != "text" {
		t.Errorf("Language = %q, want %q", pre.Language, "text")
	}
}

// findBlockquote returns the first MessageEntityBlockquote in the slice, or nil.
func findBlockquote(ents []tg.MessageEntityClass) *tg.MessageEntityBlockquote {
	for _, ent := range ents {
		if bq, ok := ent.(*tg.MessageEntityBlockquote); ok {
			return bq
		}
	}

	return nil
}

// findAllBlockquotes returns every MessageEntityBlockquote in the slice, in order.
func findAllBlockquotes(ents []tg.MessageEntityClass) []*tg.MessageEntityBlockquote {
	var out []*tg.MessageEntityBlockquote

	for _, ent := range ents {
		if bq, ok := ent.(*tg.MessageEntityBlockquote); ok {
			out = append(out, bq)
		}
	}

	return out
}

// findCode returns the first MessageEntityCode, or nil.
func findCode(ents []tg.MessageEntityClass) *tg.MessageEntityCode {
	for _, ent := range ents {
		if code, ok := ent.(*tg.MessageEntityCode); ok {
			return code
		}
	}

	return nil
}

// assertEntitySlice fails the test if plain[offset:offset+length] (UTF-16) != want.
func assertEntitySlice(t *testing.T, plain string, offset, length int, want, label string) {
	t.Helper()

	got := preSlice(plain, offset, length)
	if got != want {
		t.Fatalf(
			"%s slice mismatch: offset=%d length=%d\n got=%q\nwant=%q\nplain=%q",
			label, offset, length, got, want, plain,
		)
	}
}

func TestParseMarkdown_BlockquoteAfterInlineCode(t *testing.T) {
	// Regression: inline-code entities consumed before a blockquote previously
	// shifted the blockquote.Offset by 2 UTF-16 units per backtick pair.
	text, entities := ParseMarkdown("`code`\n\n> quoted")

	const wantPlain = "code\n\nquoted"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	code := findCode(entities)
	if code == nil {
		t.Fatalf("code entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, code.Offset, code.Length, "code", "code")

	bq := findBlockquote(entities)
	if bq == nil {
		t.Fatalf("blockquote entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bq.Offset, bq.Length, "quoted", "blockquote")
}

func TestParseMarkdown_BlockquoteAfterMultipleInlineMarkers(t *testing.T) {
	// Combined inline markers (code + bold + italic) before a blockquote.
	text, entities := ParseMarkdown("`a` **b** *c*\n\n> q")

	const wantPlain = "a b c\n\nq"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	bq := findBlockquote(entities)
	if bq == nil {
		t.Fatalf("blockquote entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bq.Offset, bq.Length, "q", "blockquote")

	code := findCode(entities)
	if code == nil {
		t.Fatalf("code entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, code.Offset, code.Length, "a", "code")

	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if b, ok := ent.(*tg.MessageEntityBold); ok {
			bold = b
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bold.Offset, bold.Length, "b", "bold")

	var italic *tg.MessageEntityItalic

	for _, ent := range entities {
		if i, ok := ent.(*tg.MessageEntityItalic); ok {
			italic = i
		}
	}

	if italic == nil {
		t.Fatalf("italic entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, italic.Offset, italic.Length, "c", "italic")
}

func TestParseMarkdown_BlockquoteAfterInlineCode_Cyrillic(t *testing.T) {
	// Reproduction from the @claudedreams/26 bug report: cyrillic prose with
	// three inline-code spans, then two blockquotes. Previously each
	// blockquote.Offset was shifted by 2 * (number of consumed backtick pairs
	// before it) — exactly +6 in this input. Verifies UTF-16 offsets land on
	// the right characters in the cleaned plain text.
	input := "Дополнение про того же `authelia/chartrepo`. На feature-request " +
		"issue #364 («опубликуйте чарт в OCI registry, у меня " +
		"`charts.authelia.com` отдаёт ~300 B/s, ArgoCD renders таймаутятся»), " +
		"мейнтейнер закрыл моё предложение `gh actions`-workflow двумя " +
		"аргументами:\n\n" +
		"> We don't use GHA for security critical jobs.\n\n" +
		"и про их собственный планируемый внутренний PR:\n\n" +
		"> it's uploading to GitHub so it probably won't solve your " +
		"underlying issue since it's the same CDN."

	text, entities := ParseMarkdown(input)

	bqs := findAllBlockquotes(entities)
	if len(bqs) != 2 {
		t.Fatalf("want 2 blockquote entities, got %d", len(bqs))
	}

	const (
		firstQuote  = "We don't use GHA for security critical jobs."
		secondQuote = "it's uploading to GitHub so it probably won't solve your " +
			"underlying issue since it's the same CDN."
	)

	assertEntitySlice(t, text, bqs[0].Offset, bqs[0].Length, firstQuote, "blockquote[0]")
	assertEntitySlice(t, text, bqs[1].Offset, bqs[1].Length, secondQuote, "blockquote[1]")
}

func TestParseMarkdown_InlineCodeInsideBlockquote(t *testing.T) {
	// Inline code inside a blockquote line: parse order must still emit a
	// code entity that points at the right UTF-16 slice in the cleaned text,
	// and the blockquote must cover the full line.
	text, entities := ParseMarkdown("> text `code`")

	const wantPlain = "text code"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	bq := findBlockquote(entities)
	if bq == nil {
		t.Fatalf("blockquote entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bq.Offset, bq.Length, "text code", "blockquote")

	code := findCode(entities)
	if code == nil {
		t.Fatalf("code entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, code.Offset, code.Length, "code", "code")
}

func TestParseMarkdown_BlockquoteThenInlineCode(t *testing.T) {
	// Regression for the inverse order: blockquote first, then inline code.
	// The fix must not break this case (inline code computed in cleaned space
	// after the blockquote line is stripped).
	text, entities := ParseMarkdown("> q\n\n`code`")

	const wantPlain = "q\n\ncode"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	bq := findBlockquote(entities)
	if bq == nil {
		t.Fatalf("blockquote entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bq.Offset, bq.Length, "q", "blockquote")

	code := findCode(entities)
	if code == nil {
		t.Fatalf("code entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, code.Offset, code.Length, "code", "code")
}

func TestParseMarkdown_BoldCrossingBlockquote(t *testing.T) {
	// A bold span whose range straddles a "> " line: previously the inline
	// parser captured the "> " inside the bold length, and stripping the
	// prefix in extractBlockquotes left the length stale (offset+length past
	// end of plain).
	text, entities := ParseMarkdown("**a\n> b\nc**")

	const wantPlain = "a\nb\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if b, ok := ent.(*tg.MessageEntityBold); ok {
			bold = b
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bold.Offset, bold.Length, "a\nb\nc", "bold")
}

func TestParseMarkdown_LinkCrossingBlockquote(t *testing.T) {
	// text_url whose visible-text spans across a quoted line.
	text, entities := ParseMarkdown("[link\n> q\ntext](http://e.com)")

	const wantPlain = "link\nq\ntext"
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

	assertEntitySlice(t, text, link.Offset, link.Length, "link\nq\ntext", "text_url")

	if link.URL != "http://e.com" {
		t.Errorf("URL = %q, want %q", link.URL, "http://e.com")
	}
}

func TestParseMarkdown_BoldCrossingMultipleBlockquotes(t *testing.T) {
	// Bold spans across two consecutive quoted lines: length must shrink by
	// 2 UTF-16 units per stripped "> " prefix that lies inside the bold.
	text, entities := ParseMarkdown("**before\n> q1\n> q2\nafter**")

	const wantPlain = "before\nq1\nq2\nafter"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if b, ok := ent.(*tg.MessageEntityBold); ok {
			bold = b
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bold.Offset, bold.Length, wantPlain, "bold")
}

func TestParseMarkdown_BlockquoteEmptyLineDropped(t *testing.T) {
	// A bare "> " line (with empty content) must not produce a zero-length
	// blockquote entity — Telegram MTProto rejects entities with length=0.
	_, entities := ParseMarkdown("> \nplain")

	for _, ent := range entities {
		bq, ok := ent.(*tg.MessageEntityBlockquote)
		if !ok {
			continue
		}

		if bq.Length == 0 {
			t.Fatalf("blockquote with length=0 emitted: %+v", bq)
		}
	}
}

func TestParseMarkdown_EscapedQuoteMarker(t *testing.T) {
	// "\> literal" must render as plain "> literal" with no blockquote: the
	// backslash escapes the quote-marker so the line is literal text.
	text, entities := ParseMarkdown(`\> literal`)

	const wantPlain = "> literal"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	if findBlockquote(entities) != nil {
		t.Fatalf("unexpected blockquote entity in %+v", entities)
	}
}

func TestParseMarkdown_EscapedQuoteMarkerOnSecondLine(t *testing.T) {
	// Same escape rule on a non-first line: "\> literal" on line two must
	// not turn into a blockquote.
	text, entities := ParseMarkdown("hello\n\\> literal")

	const wantPlain = "hello\n> literal"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	if findBlockquote(entities) != nil {
		t.Fatalf("unexpected blockquote entity in %+v", entities)
	}
}

func TestParseMarkdown_BlockquoteWithEscapeInside(t *testing.T) {
	// An escape sequence (\*) inside a blockquote: removeEscapes strips the
	// backslash; blockquote.Length must shrink to match the cleaned text.
	text, entities := ParseMarkdown(`> hello \* world`)

	const wantPlain = "hello * world"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	bq := findBlockquote(entities)
	if bq == nil {
		t.Fatalf("blockquote entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bq.Offset, bq.Length, wantPlain, "blockquote")
}

func TestParseMarkdown_BoldWithEscapeAcrossBlockquote(t *testing.T) {
	// Bold span containing both an escape (\\) and a stripped "> " prefix:
	// length must reflect both removals so offset+length stays within plain.
	text, entities := ParseMarkdown("**a\\\n> b\nc**")

	const wantPlain = "a\nb\nc"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if b, ok := ent.(*tg.MessageEntityBold); ok {
			bold = b
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bold.Offset, bold.Length, wantPlain, "bold")
}

func TestParseMarkdown_BoldEndingAtBlockquoteMarker(t *testing.T) {
	// Partial-overlap: bold ends inside a "> " prefix. The ">" was visible
	// inside the original bold; after strip it is gone, so bold must shrink
	// to the kept prefix instead of expanding past the cut.
	text, entities := ParseMarkdown("**a\n>** b")

	const wantPlain = "a\nb"
	if text != wantPlain {
		t.Fatalf("plain = %q, want %q", text, wantPlain)
	}

	var bold *tg.MessageEntityBold

	for _, ent := range entities {
		if b, ok := ent.(*tg.MessageEntityBold); ok {
			bold = b
		}
	}

	if bold == nil {
		t.Fatalf("bold entity missing in %+v", entities)
	}

	assertEntitySlice(t, text, bold.Offset, bold.Length, "a\n", "bold")
}

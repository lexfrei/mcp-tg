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

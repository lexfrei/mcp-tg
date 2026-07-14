package telegram

import "testing"

func TestLooksLikeMarkdown_Positives(t *testing.T) {
	cases := map[string]string{
		"backtick fence":   "look:\n```go\nfunc main() {}\n```",
		"tilde fence":      "~~~\nraw\n~~~",
		"inline code":      "run `go build` first",
		"bold":             "this is **important** stuff",
		"underline":        "dunder __init__ method",
		"strike":           "that idea is ~~dead~~ alive",
		"spoiler":          "the killer is ||the butler||",
		"link":             "see [the docs](https://example.com/page)",
		"fence mid-line":   "inline ```code``` fence",
		"bold single rune": "a **b** c",
		"autolink":         "visit <https://example.com/page> now",
		"bold at start":    "**bold** opens the line",
		"link after space": "read [the docs](https://example.com) now",
	}

	for name, text := range cases {
		if !LooksLikeMarkdown(text) {
			t.Errorf("%s: LooksLikeMarkdown(%q) = false, want true", name, text)
		}
	}
}

func TestLooksLikeMarkdown_Negatives(t *testing.T) {
	cases := map[string]string{
		"plain text":         "just a regular sentence",
		"empty":              "",
		"shell or":           "a || b || c",
		"spaced asterisks":   "x ** y ** z",
		"multiplication":     "5 * 3 * 2 = 30",
		"comparison":         "1 > 2 is false",
		"snake case":         "variable snake_case here",
		"arrow":              "then -> do this",
		"single italic star": "an *emphasis* attempt",
		"single underscore":  "an _emphasis_ attempt",
		"quote line":         "> quoted reply text",
		"bare brackets":      "see [1] and [2] for references",
		"bare url":           "https://example.com/page",
		"lone backtick":      "the ` character",
		"double tilde gap":   "waves ~~ everywhere ~~ here",
		"html-ish tag":       "the <b>tag</b> stays",
		"comparison angle":   "if x < y then y > x",
		"indented log":       "stack:\n    at main.go:10\n    at run.go:5",
		"c boolean or":       "if (a||b) && (c||d) return",
		"python power":       "python: 2**3**2 is 512",
		"go generic call":    "call Foo[T](x) with the type param",
		"index then call":    "see arr[0](fn) there",
		"dunder mid-word":    "the a__b__c identifier",
	}

	for name, text := range cases {
		if LooksLikeMarkdown(text) {
			t.Errorf("%s: LooksLikeMarkdown(%q) = true, want false", name, text)
		}
	}
}

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
	}

	for name, text := range cases {
		if LooksLikeMarkdown(text) {
			t.Errorf("%s: LooksLikeMarkdown(%q) = true, want false", name, text)
		}
	}
}

package tools

import (
	"testing"
)

type fixtureParams struct {
	Peer      string  `json:"peer"`
	Text      string  `json:"text"`
	Silent    *bool   `json:"silent,omitempty"`
	NoWebpage *bool   `json:"noWebpage,omitempty"`
	Pinned    bool    `json:"pinned"`
	Skipped   string  `json:"skipped"`
	Pointer   *string `json:"pointerField"`
	Hidden    bool    `json:"-"`
	NoTag     bool
}

func TestBoolJSONFields_FixtureParams(t *testing.T) {
	got := boolJSONFields[fixtureParams]()

	want := map[string]struct{}{
		"silent":    {},
		"noWebpage": {},
		"pinned":    {},
		"NoTag":     {},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d fields (%v), want %d (%v)", len(got), got, len(want), want)
	}

	for name := range want {
		_, ok := got[name]
		if !ok {
			t.Errorf("missing field %q in %v", name, got)
		}
	}
}

func TestBoolJSONFields_NoBoolFields(t *testing.T) {
	type noBools struct {
		A string `json:"a"`
		B int    `json:"b"`
	}

	got := boolJSONFields[noBools]()
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestBoolJSONFields_NotAStruct(t *testing.T) {
	got := boolJSONFields[int]()
	if got != nil {
		t.Fatalf("got %v, want nil for non-struct", got)
	}
}

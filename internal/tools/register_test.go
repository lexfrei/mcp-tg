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

// TestInputSchemaWithEnum_PatchesOnlyTheNamedProperty pins that the
// helper adds the enum without disturbing the rest of the inferred
// schema — required fields and sibling properties stay exactly as
// mcp.AddTool would have inferred them.
func TestInputSchemaWithEnum_PatchesOnlyTheNamedProperty(t *testing.T) {
	schema := inputSchemaWithEnum[MessagesSendParams]("parseMode", parseModeEnum())

	if got := schema.Properties["parseMode"].Enum; len(got) != 2 || got[0] != "plain" || got[1] != "commonmark" {
		t.Errorf("parseMode enum = %v, want [plain commonmark]", got)
	}

	if schema.Properties["peer"].Enum != nil {
		t.Error("sibling property must not grow an enum")
	}

	found := false
	for _, name := range schema.Required {
		if name == "parseMode" {
			found = true
		}
	}

	if !found {
		t.Errorf("parseMode missing from required %v", schema.Required)
	}
}

func TestInputSchemaWithEnum_UnknownPropertyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for an unknown property name")
		}
	}()

	inputSchemaWithEnum[MessagesSendParams]("noSuchField", parseModeEnum())
}

package main

import (
	"encoding/json"
	"slices"
	"testing"
)

// parseModeTools are the four text tools whose parseMode is a required
// protocol-level enum. The pin runs against the wire representation an
// MCP client actually receives, so it survives SDK upgrades and schema
// refactors alike.
func parseModeTools() []string {
	return []string{"tg_messages_send", "tg_messages_edit", "tg_messages_send_file", "tg_media_send_album"}
}

type wireSchema struct {
	Required   []string `json:"required"`
	Properties map[string]struct {
		Enum []string `json:"enum"`
	} `json:"properties"`
}

func TestParseModeSchema_RequiredEnumOnTheWire(t *testing.T) {
	registered := listRegisteredTools(t)

	want := parseModeTools()
	found := 0

	for _, tool := range registered {
		if !slices.Contains(want, tool.Name) {
			continue
		}

		found++

		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal InputSchema: %v", tool.Name, err)
		}

		var schema wireSchema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal InputSchema: %v", tool.Name, err)
		}

		if !slices.Contains(schema.Required, "parseMode") {
			t.Errorf("%s: parseMode missing from required %v", tool.Name, schema.Required)
		}

		enum := schema.Properties["parseMode"].Enum
		if !slices.Equal(enum, []string{"plain", "commonmark"}) {
			t.Errorf("%s: parseMode enum = %v, want [plain commonmark]", tool.Name, enum)
		}
	}

	if found != len(want) {
		t.Errorf("found %d of the %d parseMode tools", found, len(want))
	}
}

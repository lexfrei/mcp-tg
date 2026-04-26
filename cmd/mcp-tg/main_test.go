package main

import (
	"testing"

	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/lexfrei/mcp-tg/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRegisterTools(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-tg",
			Version: "test",
		},
		nil,
	)

	client := testutil.NoopClient{}
	registry := tools.BoolFieldRegistry{}
	registerTools(server, client, registry, "/tmp/mcp-tg/downloads")

	// Sample several tools spread across registration phases. If any of
	// these is missing, someone registered the tool via mcp.AddTool instead
	// of tools.AddTool — silently disabling bool-coercion for its params.
	cases := map[string][]string{
		"tg_messages_send":      {"silent", "noWebpage"},
		"tg_messages_send_file": {"silent"},
		"tg_dialogs_pin":        {"pinned"},
		"tg_groups_admin_set":   {"banUsers", "addAdmins"},
		"tg_chats_create":       {"isChannel"},
	}

	for name, expected := range cases {
		got, ok := registry[name]
		if !ok {
			t.Errorf("%s missing from bool registry — likely registered via mcp.AddTool instead of tools.AddTool", name)

			continue
		}

		for _, field := range expected {
			_, has := got[field]
			if !has {
				t.Errorf("expected %q in %s bool fields, got %v", field, name, got)
			}
		}
	}
}

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

	// tg_messages_send must register both bool params for coercion.
	got, ok := registry["tg_messages_send"]
	if !ok {
		t.Fatalf("tg_messages_send missing from bool registry: %v", registry)
	}

	for _, name := range []string{"silent", "noWebpage"} {
		_, has := got[name]
		if !has {
			t.Errorf("expected %q in tg_messages_send bool fields, got %v", name, got)
		}
	}
}

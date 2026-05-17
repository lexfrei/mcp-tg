package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestNewHTTPHandler_RejectsCrossOriginPOST pins the cross-origin protection
// applied to the streamable HTTP handler. MCP SDK v1.6 dropped the default
// protection when StreamableHTTPOptions is nil, so this test fails fast if a
// future refactor accidentally removes the explicit wrapping again.
func TestNewHTTPHandler_RejectsCrossOriginPOST(t *testing.T) {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "mcp-tg", Version: "test"},
		nil,
	)

	testServer := httptest.NewServer(newHTTPHandler(server))
	defer testServer.Close()

	req, reqErr := http.NewRequestWithContext(
		t.Context(), http.MethodPost, testServer.URL, strings.NewReader(`{}`),
	)
	if reqErr != nil {
		t.Fatalf("build request: %v", reqErr)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	resp, doErr := http.DefaultClient.Do(req)
	if doErr != nil {
		t.Fatalf("do request: %v", doErr)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site POST: status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

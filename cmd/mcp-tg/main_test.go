package main

import (
	"context"
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

// TestHeadlessServer_ServesMultipleClients pins that the headless daemon's
// server — built through the exact production seam newHeadlessServer, not a
// local nil literal — accepts more than one client without panicking. Each
// client.Connect drives a full initialize handshake; the trailing ListTools
// forces the server to process the initialized notification before the next
// client connects. If a future change wired an InitializedHandler that closes a
// shared channel into newHeadlessServer, the second handshake would panic with
// "close of closed channel" and fail this test. It also confirms the full
// toolset survives the build and the pre-auth allowlisted tool
// (tg_server_version) is reachable.
func TestHeadlessServer_ServesMultipleClients(t *testing.T) {
	ctx := context.Background()

	authDone := make(chan struct{})
	close(authDone) // simulate completed auth so the guard lets calls through

	server := newHeadlessServer(testutil.NoopClient{}, "/tmp/mcp-tg/downloads", authDone)

	for i := range 2 {
		ct, st := mcp.NewInMemoryTransports()

		ss, connErr := server.Connect(ctx, st, nil)
		if connErr != nil {
			t.Fatalf("server connect #%d: %v", i, connErr)
		}

		client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)

		cs, clientErr := client.Connect(ctx, ct, nil)
		if clientErr != nil {
			t.Fatalf("client connect #%d: %v", i, clientErr)
		}

		res, listErr := cs.ListTools(ctx, nil)
		if listErr != nil {
			t.Fatalf("list tools #%d: %v", i, listErr)
		}

		assertToolPresent(t, res, tools.ServerVersionToolName)
		assertToolPresent(t, res, tools.MessagesTranscribeAudioTool().Name)

		_ = cs.Close()
		ss.Wait()
	}
}

func assertToolPresent(t *testing.T, res *mcp.ListToolsResult, name string) {
	t.Helper()

	for _, tool := range res.Tools {
		if tool.Name == name {
			return
		}
	}

	t.Errorf("tool %q missing from headless server (got %d tools)", name, len(res.Tools))
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

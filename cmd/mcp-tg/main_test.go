package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tgerr"
	"github.com/lexfrei/mcp-tg/internal/config"
	"github.com/lexfrei/mcp-tg/internal/middleware"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
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

	server := newHeadlessServer(testutil.NoopClient{}, "/tmp/mcp-tg/downloads", authDone, middleware.NewSessionHealth())

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

		_ = cs.Close()
		ss.Wait()
	}
}

// TestHeadlessServer_RevokedSessionBlocksToolsOverMCP drives the full receiving
// middleware chain through the real mcp.Server over the in-memory transport.
// With the session marked revoked, a Telegram-touching tool call must come back
// as the explicit "no longer authorized" error rather than a raw 401, while the
// allowlisted server-meta tool (tg_server_version) still answers so a client can
// confirm the daemon is alive.
func TestHeadlessServer_RevokedSessionBlocksToolsOverMCP(t *testing.T) {
	ctx := context.Background()

	authDone := make(chan struct{})
	close(authDone) // auth completed

	health := middleware.NewSessionHealth()
	health.Arm()
	health.MarkRevoked("AUTH_KEY_UNREGISTERED")

	server := buildServer(testutil.NoopClient{}, "/tmp/mcp-tg/downloads", authDone, health, nil)

	ct, st := mcp.NewInMemoryTransports()

	ss, connErr := server.Connect(ctx, st, nil)
	if connErr != nil {
		t.Fatalf("server connect: %v", connErr)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)

	cs, clientErr := client.Connect(ctx, ct, nil)
	if clientErr != nil {
		t.Fatalf("client connect: %v", clientErr)
	}

	_, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: "tg_dialogs_list"})
	if callErr == nil || !strings.Contains(callErr.Error(), "no longer authorized") {
		t.Fatalf("revoked tool call must return the explicit auth error, got: %v", callErr)
	}

	res, verErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: tools.ServerVersionToolName})
	if verErr != nil {
		t.Fatalf("server-version tool must stay reachable when revoked, got: %v", verErr)
	}

	if res.IsError {
		t.Fatal("server-version tool returned an error result while revoked")
	}

	_ = cs.Close()
	ss.Wait()
}

// TestHeadlessLoginRequired_ActionableMessage pins that a headless startup auth
// failure is reported with the interactive-login fix, not the misleading raw
// "TELEGRAM_PHONE is required" that gotd surfaces.
func TestHeadlessLoginRequired_ActionableMessage(t *testing.T) {
	cause := errors.New("auth flow: get phone: TELEGRAM_PHONE is required for authentication")

	err := headlessLoginRequired(cause)

	if !errors.Is(err, cause) {
		t.Error("wrapped error must preserve the cause for errors.Is")
	}

	msg := err.Error()
	if !strings.Contains(msg, "mcp-tg login") {
		t.Errorf("message must point to `mcp-tg login`, got: %s", msg)
	}

	if !strings.Contains(msg, "terminal") {
		t.Errorf("message must say it needs a terminal, got: %s", msg)
	}
}

// TestLoginWouldFix pins which headless startup auth failures map to the
// "run mcp-tg login" guidance (missing session / unauthorized) versus which
// surface unchanged (transient network / server errors re-login cannot fix).
func TestLoginWouldFix(t *testing.T) {
	fixable := []error{
		tgclient.ErrPhoneRequired,
		tgclient.ErrPasswordRequired,
		tgclient.ErrNoAuthCode,
		tgclient.ErrElicitDeclined,
		errors.Wrap(tgerr.New(401, codeAuthKeyUnregistered), "authentication failed"),
		tgerr.New(401, "SOME_OTHER_401"), // any 401 UNAUTHORIZED means log in
	}
	for _, err := range fixable {
		if !loginWouldFix(err) {
			t.Errorf("loginWouldFix(%v) = false, want true", err)
		}
	}

	unfixable := []error{
		errors.New("dial tcp: i/o timeout"),
		tgerr.New(500, "INTERNAL"),
		errors.Wrap(tgerr.New(303, "NETWORK_MIGRATE_2"), "authentication failed"),
	}
	for _, err := range unfixable {
		if loginWouldFix(err) {
			t.Errorf("loginWouldFix(%v) = true, want false", err)
		}
	}
}

// TestRevokedExitError covers the connection-death path: a revoked-session code
// that gotd surfaces as a permanent connection error (ending tgClient.Run) must
// be turned into the same actionable `mcp-tg login` guidance the invoker path
// gives, while other stops keep the generic message and nil stays nil.
func TestRevokedExitError(t *testing.T) {
	if got := revokedExitError(nil); got != nil {
		t.Errorf("clean shutdown must stay nil, got: %v", got)
	}

	revoked := tgerr.New(401, "AUTH_KEY_DUPLICATED")
	err := revokedExitError(revoked)

	if !errors.Is(err, revoked) {
		t.Error("wrapped error must preserve the cause for errors.Is")
	}

	msg := err.Error()
	if !strings.Contains(msg, "mcp-tg login") {
		t.Errorf("revoked exit must point to `mcp-tg login`, got: %s", msg)
	}

	if !strings.Contains(msg, "AUTH_KEY_DUPLICATED") {
		t.Errorf("revoked exit must name the code, got: %s", msg)
	}

	generic := errors.New("context canceled")
	genericErr := revokedExitError(generic)

	if !errors.Is(genericErr, generic) {
		t.Error("generic stop must preserve the cause for errors.Is")
	}

	if gotMsg := genericErr.Error(); !strings.Contains(gotMsg, "telegram client stopped") {
		t.Errorf("non-revoked stop must use the generic message, got: %s", gotMsg)
	}
}

// TestEnsureFileStorageDir pins that the session-file directory is created only
// for insecure/file storage — keychain mode must not touch the filesystem.
func TestEnsureFileStorageDir(t *testing.T) {
	base := t.TempDir()
	cfg := &config.Config{SessionFile: filepath.Join(base, "sub", "session.json")}

	if err := ensureFileStorageDir(cfg, false); err != nil {
		t.Fatalf("secure mode returned an error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(base, "sub")); !os.IsNotExist(err) {
		t.Error("secure (keychain) mode must not create the session directory")
	}

	if err := ensureFileStorageDir(cfg, true); err != nil {
		t.Fatalf("insecure mode: %v", err)
	}

	if _, err := os.Stat(filepath.Join(base, "sub")); err != nil {
		t.Errorf("insecure (file) mode must create the session directory: %v", err)
	}
}

func TestShortRevision(t *testing.T) {
	if got := shortRevision("abcdef"); got != "abcdef" {
		t.Errorf("short revision = %q, want it unchanged", got)
	}

	if got := shortRevision("0123456789abcdef0123456789abcdef01234567"); got != "01234567" {
		t.Errorf("long revision = %q, want the first %d chars", got, shortRevisionLen)
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

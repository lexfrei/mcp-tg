package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-tg/internal/middleware"
)

func invokeWithAuthRevoked(
	t *testing.T, next *recordingInvoker, health *middleware.SessionHealth, logger *slog.Logger,
) error {
	t.Helper()

	mw := newAuthRevokedMiddleware(health, logger)
	handler := mw(next)

	return handler(context.Background(), &tg.ContactsResolveUsernameRequest{Username: "example"}, nil)
}

func TestAuthRevoked_PassthroughSuccessStaysHealthy(t *testing.T) {
	health := middleware.NewSessionHealth()
	next := &recordingInvoker{}

	err := invokeWithAuthRevoked(t, next, health, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if health.Revoked() {
		t.Error("session must stay healthy after a successful call")
	}
}

func TestAuthRevoked_MarksRevokedAndPassesErrorThrough(t *testing.T) {
	health := middleware.NewSessionHealth()
	health.Arm()
	revokeErr := tgerr.New(401, codeAuthKeyUnregistered)
	next := &recordingInvoker{errs: []error{revokeErr}}

	var buf bytes.Buffer

	err := invokeWithAuthRevoked(t, next, health, slog.New(slog.NewTextHandler(&buf, nil)))
	if !errors.Is(err, revokeErr) {
		t.Fatalf("expected original error passed through, got: %v", err)
	}

	if !health.Revoked() {
		t.Error("session must be marked revoked after AUTH_KEY_UNREGISTERED")
	}

	if code := health.Code(); code != codeAuthKeyUnregistered {
		t.Errorf("Code() = %q, want AUTH_KEY_UNREGISTERED", code)
	}

	if !strings.Contains(buf.String(), "re-login required") {
		t.Errorf("expected a clear ERROR log, got: %s", buf.String())
	}
}

func TestAuthRevoked_OtherErrorStaysHealthy(t *testing.T) {
	health := middleware.NewSessionHealth()
	health.Arm()
	otherErr := tgerr.New(420, "FLOOD_WAIT_30")
	next := &recordingInvoker{errs: []error{otherErr}}

	err := invokeWithAuthRevoked(t, next, health, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if !errors.Is(err, otherErr) {
		t.Fatalf("expected original error, got: %v", err)
	}

	if health.Revoked() {
		t.Error("a non-auth error must not revoke the session")
	}
}

func TestAuthRevoked_LogsOnlyOnce(t *testing.T) {
	health := middleware.NewSessionHealth()
	health.Arm()
	next := &recordingInvoker{errs: []error{
		tgerr.New(401, codeAuthKeyUnregistered),
		tgerr.New(401, codeAuthKeyUnregistered),
	}}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	for range 2 {
		_ = invokeWithAuthRevoked(t, next, health, logger)
	}

	if got := strings.Count(buf.String(), "re-login required"); got != 1 {
		t.Errorf("expected exactly one revoke log line, got %d\n%s", got, buf.String())
	}
}

func TestRevokedCode(t *testing.T) {
	// Every entry of the actual list must match — iterate the list itself so the
	// test tracks it and no code literal is duplicated.
	for _, code := range authRevokedCodes {
		got, ok := revokedCode(tgerr.New(401, code))
		if !ok || got != code {
			t.Errorf("revokedCode(%s) = %q, %v; want %q, true", code, got, ok, code)
		}
	}

	// Not revoked-session codes: a flood wait, and account-level bans that
	// re-login cannot fix (deliberately excluded from the list).
	for _, code := range []string{"FLOOD_WAIT_42", "USER_DEACTIVATED", "USER_DEACTIVATED_BAN"} {
		if _, ok := revokedCode(tgerr.New(400, code)); ok {
			t.Errorf("revokedCode(%s) should not match", code)
		}
	}
}

// TestAuthRevoked_PreAuthProbeDoesNotPoisonSession pins the interaction between
// the startup auth flow and the guard. Auth().IfNecessary probes users.getUsers
// on self before login, which answers AUTH_KEY_UNREGISTERED for a not-yet-
// authorized or revoked session. That expected pre-login 401 must NOT poison the
// session: after a successful login the guard must let tools through. Only a 401
// seen after authentication (Arm) is a real revocation.
func TestAuthRevoked_PreAuthProbeDoesNotPoisonSession(t *testing.T) {
	health := middleware.NewSessionHealth()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	// Startup IfNecessary probe returns 401 before any login has happened.
	probe := &recordingInvoker{errs: []error{tgerr.New(401, codeAuthKeyUnregistered)}}
	_ = invokeWithAuthRevoked(t, probe, health, logger)

	if health.Revoked() {
		t.Fatal("pre-auth probe 401 must not revoke the session")
	}

	// Authentication has now succeeded; the daemon arms revocation tracking.
	health.Arm()

	errReached := errors.New("handler reached")
	reach := func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, errReached
	}
	guard := middleware.NewSessionGuard(health, nil)(reach)
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "tg_dialogs_list"}}

	if _, err := guard(context.Background(), "tools/call", req); !errors.Is(err, errReached) {
		t.Fatalf("after successful login a tool must reach the handler, got: %v", err)
	}

	// A revocation seen after authentication is real: flip the flag, block tools.
	postAuth := &recordingInvoker{errs: []error{tgerr.New(401, codeAuthKeyUnregistered)}}
	_ = invokeWithAuthRevoked(t, postAuth, health, logger)

	if !health.Revoked() {
		t.Fatal("post-auth 401 must revoke the session")
	}

	if _, err := guard(context.Background(), "tools/call", req); !errors.Is(err, middleware.ErrSessionRevoked) {
		t.Fatalf("after revocation the guard must block with ErrSessionRevoked, got: %v", err)
	}
}

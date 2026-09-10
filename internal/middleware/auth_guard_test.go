package middleware

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errNoop = errors.New("noop")

const (
	bypassToolName    = "tg_server_version"
	nonBypassToolName = "tg_messages_send"
)

func noopResult(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
	return &mcp.CallToolResult{}, errNoop
}

func TestAuthGuard_BlocksResourcesBeforeAuth(t *testing.T) {
	authDone := make(chan struct{})
	handler := NewAuthGuard(authDone)(noopResult)

	_, err := handler(context.Background(), "resources/read", nil)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("got error %v, want ErrNotAuthenticated", err)
	}
}

func TestAuthGuard_AllowsAfterAuth(t *testing.T) {
	authDone := make(chan struct{})
	close(authDone)

	handler := NewAuthGuard(authDone)(noopResult)

	_, err := handler(context.Background(), "resources/read", nil)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (handler invoked)", err)
	}
}

func TestAuthGuard_AllowsProtocolMethods(t *testing.T) {
	authDone := make(chan struct{}) // never closed

	handler := NewAuthGuard(authDone)(noopResult)

	_, err := handler(context.Background(), "initialize", nil)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (handler invoked)", err)
	}
}

// Every tool call reaches the handler while the login is pending, including
// one that needs an account. Blocking here would be blocking the only path
// that can log in: the login runs inside a tool call, and the login gate is
// what decides whether the tool itself gets to run.
func TestAuthGuard_ToolCallsPassWhilePending(t *testing.T) {
	authDone := make(chan struct{}) // never closed

	handler := NewAuthGuard(authDone)(noopResult)

	for _, name := range []string{bypassToolName, nonBypassToolName} {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: name},
		}

		_, err := handler(context.Background(), "tools/call", req)
		if !errors.Is(err, errNoop) {
			t.Errorf("%s: got error %v, want errNoop (the call must reach the handler)", name, err)
		}
	}
}

func TestRequiresAuth(t *testing.T) {
	tests := []struct {
		method string
		want   bool
	}{
		{"tools/call", true},
		{"tools/list", false},
		{"resources/read", true},
		{"resources/list", false},
		{"prompts/get", true},
		{"prompts/list", false},
		{"initialize", false},
		{"ping", false},
		{"notifications/initialized", false},
	}

	for _, tst := range tests {
		got := requiresAuth(tst.method)
		if got != tst.want {
			t.Errorf("requiresAuth(%q) = %v, want %v", tst.method, got, tst.want)
		}
	}
}

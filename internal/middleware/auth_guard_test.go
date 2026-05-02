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

func TestAuthGuard_BlocksBeforeAuth(t *testing.T) {
	authDone := make(chan struct{})
	handler := NewAuthGuard(authDone, nil)(noopResult)

	_, err := handler(context.Background(), "tools/call", nil)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("got error %v, want ErrNotAuthenticated", err)
	}
}

func TestAuthGuard_AllowsAfterAuth(t *testing.T) {
	authDone := make(chan struct{})
	close(authDone)

	handler := NewAuthGuard(authDone, nil)(noopResult)

	_, err := handler(context.Background(), "tools/call", nil)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (handler invoked)", err)
	}
}

func TestAuthGuard_AllowsProtocolMethods(t *testing.T) {
	authDone := make(chan struct{}) // never closed

	handler := NewAuthGuard(authDone, nil)(noopResult)

	_, err := handler(context.Background(), "initialize", nil)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (handler invoked)", err)
	}
}

// Tools listed in the bypass slice must reach the handler before auth.
// This lets read-only server-meta tools (build version, etc.) work during
// auth troubleshooting, when blocking them is exactly the wrong UX.
func TestAuthGuard_BypassedToolReachesHandlerBeforeAuth(t *testing.T) {
	authDone := make(chan struct{}) // never closed

	handler := NewAuthGuard(authDone, []string{bypassToolName})(noopResult)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: bypassToolName},
	}

	_, err := handler(context.Background(), "tools/call", req)
	if !errors.Is(err, errNoop) {
		t.Errorf("got error %v, want errNoop (bypassed tool must reach handler)", err)
	}
}

// Non-bypassed tools are still blocked before auth even when other tools
// are bypassed.
func TestAuthGuard_NonBypassedToolStillBlocked(t *testing.T) {
	authDone := make(chan struct{})

	handler := NewAuthGuard(authDone, []string{bypassToolName})(noopResult)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{Name: nonBypassToolName},
	}

	_, err := handler(context.Background(), "tools/call", req)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("got error %v, want ErrNotAuthenticated", err)
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

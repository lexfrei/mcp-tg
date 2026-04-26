package middleware_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/middleware"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fakeBoolParams mirrors the shape of MessagesSendParams: nullable booleans
// declared as *bool, plus a string field that should never be coerced.
type fakeBoolParams struct {
	Peer      string `json:"peer"`
	ParseMode string `json:"parseMode,omitempty"`
	Silent    *bool  `json:"silent,omitempty"`
	NoWebpage *bool  `json:"noWebpage,omitempty"`
}

type fakeResult struct {
	Got string `json:"got"`
}

// TestE2E_BoolCoercer_StringTrueAccepted verifies that with the middleware
// in place, a tool call carrying `silent: "true"` (string) reaches the
// handler with the parsed bool value, rather than being rejected by the SDK
// validator with `type: true has type "string"`.
//
// Without the middleware, the same payload triggers exactly the validation
// error reported in the bug report against tg_messages_send.
func TestE2E_BoolCoercer_StringTrueAccepted(t *testing.T) {
	const toolName = "fake_send"

	var sawSilent *bool

	handler := func(_ context.Context, _ *mcp.CallToolRequest, in fakeBoolParams) (*mcp.CallToolResult, fakeResult, error) {
		sawSilent = in.Silent

		return nil, fakeResult{Got: in.Peer}, nil
	}

	registry := middleware.BoolFieldRegistry{
		toolName: {"silent": {}, "noWebpage": {}},
	}

	cs, _, cleanup := newTestSession(t, registry, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: toolName, Description: "x"}, handler)
	})
	defer cleanup()

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: json.RawMessage(`{"peer":"@x","silent":"true","noWebpage":"false"}`),
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if res.IsError {
		text := ""
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}

		t.Fatalf("expected success after coercion, got IsError=true: %s", text)
	}

	if sawSilent == nil || !*sawSilent {
		t.Fatalf("handler saw silent=%v, want *true", sawSilent)
	}
}

// TestE2E_BoolCoercer_OffStillRejectsString verifies the pre-fix behaviour:
// when the middleware is NOT installed, the SDK validator rejects the same
// payload with the documented error. This pins the bug repro and locks in
// what the fix actually changes.
func TestE2E_BoolCoercer_OffStillRejectsString(t *testing.T) {
	const toolName = "fake_send"

	handler := func(_ context.Context, _ *mcp.CallToolRequest, _ fakeBoolParams) (*mcp.CallToolResult, fakeResult, error) {
		return nil, fakeResult{Got: "ok"}, nil
	}

	cs, _, cleanup := newTestSession(t, nil, func(server *mcp.Server) {
		mcp.AddTool(server, &mcp.Tool{Name: toolName, Description: "x"}, handler)
	})
	defer cleanup()

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      toolName,
		Arguments: json.RawMessage(`{"peer":"@x","silent":"true"}`),
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if !res.IsError {
		t.Fatalf("expected validation error without middleware, got success")
	}

	text := ""

	if len(res.Content) > 0 {
		tc, ok := res.Content[0].(*mcp.TextContent)
		if ok {
			text = tc.Text
		}
	}

	if !strings.Contains(text, "silent") || !strings.Contains(text, "boolean") {
		t.Errorf("expected error to mention silent + boolean, got: %s", text)
	}
}

// newTestSession spins up an in-memory client/server pair, optionally
// installing the bool-coercion middleware, and applies the provided server
// configuration callback before connecting.
func newTestSession(
	t *testing.T,
	registry middleware.BoolFieldRegistry,
	configure func(*mcp.Server),
) (*mcp.ClientSession, *mcp.ServerSession, func()) {
	t.Helper()

	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	configure(server)

	if registry != nil {
		server.AddReceivingMiddleware(middleware.NewBoolCoercer(registry))
	}

	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	return cs, ss, func() {
		_ = cs.Close()
		ss.Wait()
	}
}

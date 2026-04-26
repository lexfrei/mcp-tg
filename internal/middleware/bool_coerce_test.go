package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	peerLiteral       = "@x"
	argsSilentStrTrue = `{"silent":"true"}`
)

func TestCoerceBoolArgs_StringTrueBecomesBool(t *testing.T) {
	raw := json.RawMessage(`{"silent":"true","peer":"` + peerLiteral + `","text":"hi"}`)
	fields := map[string]struct{}{"silent": {}, "noWebpage": {}}

	out, changed := coerceBoolArgs(raw, fields)
	if !changed {
		t.Fatalf("expected coercion, got changed=false; out=%s", out)
	}

	var parsed map[string]any

	err := json.Unmarshal(out, &parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	silent, ok := parsed["silent"].(bool)
	if !ok || !silent {
		t.Fatalf("silent = %v (%T), want bool true", parsed["silent"], parsed["silent"])
	}

	if parsed["peer"] != peerLiteral {
		t.Errorf("peer mutated: %v", parsed["peer"])
	}
}

func TestCoerceBoolArgs_StringFalseBecomesBool(t *testing.T) {
	raw := json.RawMessage(`{"noWebpage":"false"}`)
	fields := map[string]struct{}{"noWebpage": {}}

	out, changed := coerceBoolArgs(raw, fields)
	if !changed {
		t.Fatalf("expected coercion; out=%s", out)
	}

	var parsed map[string]any

	_ = json.Unmarshal(out, &parsed)

	v, ok := parsed["noWebpage"].(bool)
	if !ok || v {
		t.Fatalf("noWebpage = %v, want bool false", parsed["noWebpage"])
	}
}

func TestCoerceBoolArgs_RealBoolUntouched(t *testing.T) {
	raw := json.RawMessage(`{"silent":true}`)
	fields := map[string]struct{}{"silent": {}}

	out, changed := coerceBoolArgs(raw, fields)
	if changed {
		t.Fatalf("real bool should not be 'changed'; out=%s", out)
	}
}

func TestCoerceBoolArgs_StringNotInBoolFieldsUntouched(t *testing.T) {
	// "parseMode" is *string in tg_messages_send. Even if a caller sends the
	// literal string "true", it must NOT be coerced — it would corrupt the
	// string field.
	raw := json.RawMessage(`{"parseMode":"true","silent":"true"}`)
	fields := map[string]struct{}{"silent": {}} // parseMode NOT listed

	out, changed := coerceBoolArgs(raw, fields)
	if !changed {
		t.Fatalf("expected coercion of silent; out=%s", out)
	}

	var parsed map[string]any

	_ = json.Unmarshal(out, &parsed)

	if parsed["parseMode"] != "true" {
		t.Errorf("parseMode mutated to %v (%T), want string \"true\"",
			parsed["parseMode"], parsed["parseMode"])
	}

	if parsed["silent"] != true {
		t.Errorf("silent = %v, want bool true", parsed["silent"])
	}
}

func TestCoerceBoolArgs_NullUntouched(t *testing.T) {
	raw := json.RawMessage(`{"silent":null}`)
	fields := map[string]struct{}{"silent": {}}

	_, changed := coerceBoolArgs(raw, fields)
	if changed {
		t.Fatalf("null must stay null")
	}
}

func TestCoerceBoolArgs_GarbledStringUntouched(t *testing.T) {
	// Anything other than the literal "true"/"false" must be left alone, so
	// callers still get a real type-validation error from the SDK.
	raw := json.RawMessage(`{"silent":"yes"}`)
	fields := map[string]struct{}{"silent": {}}

	_, changed := coerceBoolArgs(raw, fields)
	if changed {
		t.Fatalf("non-canonical bool string must not be coerced")
	}
}

func TestCoerceBoolArgs_EmptyArgs(t *testing.T) {
	_, changed := coerceBoolArgs(nil, map[string]struct{}{"silent": {}})
	if changed {
		t.Fatalf("nil args: expected no change")
	}

	_, changed = coerceBoolArgs(json.RawMessage(`{}`), map[string]struct{}{"silent": {}})
	if changed {
		t.Fatalf("empty object: expected no change")
	}
}

// TestNewBoolCoercer_PassThroughForUnknownTool ensures that for tools not in
// the registry the middleware does not touch arguments.
func TestNewBoolCoercer_PassThroughForUnknownTool(t *testing.T) {
	registry := map[string]map[string]struct{}{
		"tg_messages_send": {"silent": {}},
	}

	captured := json.RawMessage{}

	next := func(_ context.Context, _ string, req mcp.Request) (mcp.Result, error) {
		ctReq, ok := req.(*mcp.CallToolRequest)
		if ok && ctReq.Params != nil {
			captured = ctReq.Params.Arguments
		}

		return &mcp.CallToolResult{}, nil
	}

	mw := NewBoolCoercer(registry)
	handler := mw(next)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "some_other_tool",
			Arguments: json.RawMessage(argsSilentStrTrue),
		},
	}

	_, _ = handler(context.Background(), "tools/call", req)

	if string(captured) != argsSilentStrTrue {
		t.Fatalf("args mutated for unknown tool: %s", captured)
	}
}

// TestNewBoolCoercer_CoercesKnownTool ensures the registered tool gets its
// string-typed bool params coerced to JSON booleans before reaching the next
// handler in the chain (the SDK validator).
func TestNewBoolCoercer_CoercesKnownTool(t *testing.T) {
	registry := map[string]map[string]struct{}{
		"tg_messages_send": {"silent": {}, "noWebpage": {}},
	}

	var captured json.RawMessage

	next := func(_ context.Context, _ string, req mcp.Request) (mcp.Result, error) {
		ctReq := req.(*mcp.CallToolRequest)
		captured = ctReq.Params.Arguments

		return &mcp.CallToolResult{}, nil
	}

	mw := NewBoolCoercer(registry)
	handler := mw(next)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "tg_messages_send",
			Arguments: json.RawMessage(`{"peer":"@x","text":"hi","silent":"true","noWebpage":"false"}`),
		},
	}

	_, _ = handler(context.Background(), "tools/call", req)

	var parsed map[string]any

	err := json.Unmarshal(captured, &parsed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["silent"] != true {
		t.Errorf("silent = %v (%T), want true", parsed["silent"], parsed["silent"])
	}

	if parsed["noWebpage"] != false {
		t.Errorf("noWebpage = %v (%T), want false", parsed["noWebpage"], parsed["noWebpage"])
	}

	if parsed["peer"] != "@x" || parsed["text"] != "hi" {
		t.Errorf("non-bool fields mutated: %v", parsed)
	}
}

// TestNewBoolCoercer_NonCallToolUntouched ensures non-tool methods (e.g.
// tools/list) are passed through without inspection — even if the request
// happened to carry a CallToolParamsRaw lookalike.
func TestNewBoolCoercer_NonCallToolUntouched(t *testing.T) {
	registry := map[string]map[string]struct{}{
		"tg_messages_send": {"silent": {}},
	}

	called := false

	var passedThrough json.RawMessage

	next := func(_ context.Context, _ string, req mcp.Request) (mcp.Result, error) {
		called = true
		ctReq, ok := req.(*mcp.CallToolRequest)
		if ok && ctReq.Params != nil {
			passedThrough = ctReq.Params.Arguments
		}

		return &mcp.CallToolResult{}, nil
	}

	mw := NewBoolCoercer(registry)
	handler := mw(next)

	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "tg_messages_send",
			Arguments: json.RawMessage(`{"silent":"true"}`),
		},
	}

	// Method != "tools/call" — coercion must NOT run.
	_, _ = handler(context.Background(), "tools/list", req)

	if !called {
		t.Fatalf("middleware must call next for non-tools/call methods")
	}

	if string(passedThrough) != `{"silent":"true"}` {
		t.Fatalf("args mutated for non-call method: %s", passedThrough)
	}
}

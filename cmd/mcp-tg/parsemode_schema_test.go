package main

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/middleware"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// parseModeTools are the four text tools whose parseMode is a required
// protocol-level enum. The pin runs against the wire representation an
// MCP client actually receives, so it survives SDK upgrades and schema
// refactors alike.
func parseModeTools() []string {
	return []string{"tg_messages_send", "tg_messages_edit", "tg_messages_send_file", "tg_media_send_album"}
}

type wireSchema struct {
	Required   []string `json:"required"`
	Properties map[string]struct {
		Enum []string `json:"enum"`
	} `json:"properties"`
}

func TestParseModeSchema_RequiredEnumOnTheWire(t *testing.T) {
	registered := listRegisteredTools(t)

	want := parseModeTools()
	found := 0

	for _, tool := range registered {
		if !slices.Contains(want, tool.Name) {
			continue
		}

		found++

		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal InputSchema: %v", tool.Name, err)
		}

		var schema wireSchema
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("%s: unmarshal InputSchema: %v", tool.Name, err)
		}

		if !slices.Contains(schema.Required, "parseMode") {
			t.Errorf("%s: parseMode missing from required %v", tool.Name, schema.Required)
		}

		enum := schema.Properties["parseMode"].Enum
		if !slices.Equal(enum, []string{"plain", "commonmark"}) {
			t.Errorf("%s: parseMode enum = %v, want [plain commonmark]", tool.Name, enum)
		}
	}

	if found != len(want) {
		t.Errorf("found %d of the %d parseMode tools", found, len(want))
	}
}

// callSendTool drives a real in-memory MCP session so the SDK's own
// schema validation runs — the behavioral counterpart to the shape pin
// above, independent of SDK internals across upgrades.
func callSendTool(t *testing.T, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	return callParseModeTool(t, "tg_messages_send", args)
}

func callParseModeTool(t *testing.T, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	ctx := context.Background()

	authDone := make(chan struct{})
	close(authDone)

	server := newHeadlessServer(testutil.NoopClient{}, "/tmp/mcp-tg/downloads", authDone, middleware.NewSessionHealth())

	ct, st := mcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "schema-e2e", Version: "0"}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool transport error: %v", err)
	}

	return res
}

func callToolErrorText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()

	if !res.IsError {
		t.Fatal("expected an IsError result")
	}

	for _, content := range res.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			return text.Text
		}
	}

	t.Fatal("no text content in error result")

	return ""
}

func TestParseModeSchema_RejectsBeforeHandler(t *testing.T) {
	missing := callSendTool(t, map[string]any{"peer": "@x", "text": "hi"})
	if text := callToolErrorText(t, missing); !strings.Contains(text, "parseMode") {
		t.Errorf("missing-mode error does not name parseMode: %s", text)
	}

	alias := callSendTool(t, map[string]any{"peer": "@x", "text": "hi", "parseMode": "markdown"})
	if text := callToolErrorText(t, alias); !strings.Contains(text, "enum") {
		t.Errorf("alias error is not the schema enum rejection: %s", text)
	}

	upper := callSendTool(t, map[string]any{"peer": "@x", "text": "hi", "parseMode": "Plain"})
	if text := callToolErrorText(t, upper); !strings.Contains(text, "enum") {
		t.Errorf("wrong-case error is not the schema enum rejection: %s", text)
	}

	valid := callSendTool(t, map[string]any{"peer": "@x", "text": "hi", "parseMode": "plain"})
	if valid.IsError {
		t.Errorf("a valid plain call must pass schema validation, got: %+v", valid.Content)
	}
}

// TestParseModeSchema_RejectsBeforeHandlerOnEveryTool extends the
// behavioral pin to all four text tools: they share one helper, but the
// contract is what a client experiences, not what the helper promises.
func TestParseModeSchema_RejectsBeforeHandlerOnEveryTool(t *testing.T) {
	baseArgs := map[string]map[string]any{
		"tg_messages_send":      {"peer": "@x", "text": "hi"},
		"tg_messages_edit":      {"peer": "@x", "messageId": 1, "text": "hi"},
		"tg_messages_send_file": {"peer": "@x", "path": "/tmp/f"},
		"tg_media_send_album":   {"peer": "@x", "paths": []any{"/tmp/a"}},
	}

	for _, tool := range parseModeTools() {
		args := baseArgs[tool]

		missing := callParseModeTool(t, tool, args)
		if text := callToolErrorText(t, missing); !strings.Contains(text, "parseMode") {
			t.Errorf("%s: missing-mode error does not name parseMode: %s", tool, text)
		}

		withAlias := map[string]any{"parseMode": "markdown"}
		for k, v := range args {
			withAlias[k] = v
		}

		alias := callParseModeTool(t, tool, withAlias)
		if text := callToolErrorText(t, alias); !strings.Contains(text, "enum") {
			t.Errorf("%s: alias error is not the schema enum rejection: %s", tool, text)
		}
	}
}

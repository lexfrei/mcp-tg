package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMessagesForFormatAndOutputForFormat(t *testing.T) {
	msgs := []MessageItem{{ID: 1, Text: "hi"}}
	output := "hi\n"

	cases := []struct {
		format     string
		wantMsgs   bool
		wantOutput bool
	}{
		{formatFull, true, true},
		{"", true, true},
		{formatJSON, true, false},
		{formatText, false, true},
	}

	for _, c := range cases {
		gotMsgs := messagesForFormat(c.format, msgs)
		gotOutput := outputForFormat(c.format, output)

		if (gotMsgs != nil) != c.wantMsgs {
			t.Errorf("format %q: messages present = %v, want %v", c.format, gotMsgs != nil, c.wantMsgs)
		}

		if (gotOutput != "") != c.wantOutput {
			t.Errorf("format %q: output present = %v, want %v", c.format, gotOutput != "", c.wantOutput)
		}
	}
}

func TestMessagesListResult_JSONOmitsTrimmedFields(t *testing.T) {
	// json shape: no human-readable output key.
	jsonShape := MessagesListResult{Count: 1, Messages: []MessageItem{{ID: 1}}}

	raw, err := json.Marshal(jsonShape)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(raw), `"output"`) {
		t.Errorf("json-format result must omit the output key, got:\n%s", raw)
	}

	// text shape: no structured messages key.
	textShape := MessagesListResult{Count: 1, Output: "hi\n"}

	raw, err = json.Marshal(textShape)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(raw), `"messages"`) {
		t.Errorf("text-format result must omit the messages key, got:\n%s", raw)
	}
}

func TestMessagesListHandler_FormatTrimsResultShape(t *testing.T) {
	newMock := func() *mockClient {
		return &mockClient{
			peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
			total:    1,
			messages: []telegram.Message{{ID: 5, Type: "text", Text: "hi", Date: 1000}},
		}
	}

	// full — both present.
	_, full, err := NewMessagesListHandler(newMock())(
		context.Background(), nil, MessagesListParams{Peer: "@x", Format: formatFull})
	if err != nil {
		t.Fatalf("full: unexpected error: %v", err)
	}

	if full.Messages == nil || full.Output == "" {
		t.Errorf("full must keep both messages and output, got messages=%v output=%q", full.Messages, full.Output)
	}

	// json — output cleared, messages kept.
	_, jsonRes, err := NewMessagesListHandler(newMock())(
		context.Background(), nil, MessagesListParams{Peer: "@x", Format: formatJSON})
	if err != nil {
		t.Fatalf("json: unexpected error: %v", err)
	}

	if jsonRes.Messages == nil || jsonRes.Output != "" {
		t.Errorf("json must keep messages and drop output, got messages=%v output=%q", jsonRes.Messages, jsonRes.Output)
	}

	// text — messages cleared, output kept.
	_, textRes, err := NewMessagesListHandler(newMock())(
		context.Background(), nil, MessagesListParams{Peer: "@x", Format: formatText})
	if err != nil {
		t.Fatalf("text: unexpected error: %v", err)
	}

	if textRes.Messages != nil || textRes.Output == "" {
		t.Errorf("text must drop messages and keep output, got messages=%v output=%q", textRes.Messages, textRes.Output)
	}
}

// assertFormatShape checks that a trimmed read result carries only the
// fields the format promises: json keeps messages and drops output, text
// drops messages and keeps output.
func assertFormatShape(t *testing.T, format string, msgs []MessageItem, output string) {
	t.Helper()

	switch format {
	case formatJSON:
		if msgs == nil || output != "" {
			t.Errorf("json: want messages kept and output dropped, got messages=%v output=%q", msgs, output)
		}
	case formatText:
		if msgs != nil || output == "" {
			t.Errorf("text: want messages dropped and output kept, got messages=%v output=%q", msgs, output)
		}
	}
}

// The get/context/search handlers apply the same shared format helpers as
// messages_list; this pins the trim end-to-end on each of the three.
func TestReadHandlers_FormatTrimsResultShape(t *testing.T) {
	newMock := func() *mockClient {
		return &mockClient{
			peer:     telegram.InputPeer{Type: telegram.PeerUser, ID: 1},
			total:    1,
			messages: []telegram.Message{{ID: 5, Type: "text", Text: "hi", Date: 1000}},
		}
	}

	for _, format := range []string{formatJSON, formatText} {
		_, getRes, err := NewMessagesGetHandler(newMock())(
			context.Background(), nil, MessagesGetParams{Peer: "@x", IDs: []int{5}, Format: format})
		if err != nil {
			t.Fatalf("get %s: %v", format, err)
		}

		assertFormatShape(t, format, getRes.Messages, getRes.Output)

		_, ctxRes, err := NewMessagesContextHandler(newMock())(
			context.Background(), nil, MessagesContextParams{Peer: "@x", MessageID: 5, Format: format})
		if err != nil {
			t.Fatalf("context %s: %v", format, err)
		}

		assertFormatShape(t, format, ctxRes.Messages, ctxRes.Output)

		_, searchRes, err := NewMessagesSearchHandler(newMock())(
			context.Background(), searchRequest(), MessagesSearchParams{Peer: "@x", Query: "hi", Format: format})
		if err != nil {
			t.Fatalf("search %s: %v", format, err)
		}

		assertFormatShape(t, format, searchRes.Messages, searchRes.Output)
	}
}

// search_global carries a distinct result type (with cursor fields), so
// its format trim is integration-tested separately from messages_list.
func TestMessagesSearchGlobalHandler_FormatTrimsResultShape(t *testing.T) {
	newMock := func() *mockClient {
		return &mockClient{
			total: 1,
			messages: []telegram.Message{
				{ID: 5, PeerID: telegram.InputPeer{Type: telegram.PeerUser, ID: 1}, Type: "text", Text: "hi", Date: 1000},
			},
		}
	}

	// json — the summary output is cleared, messages kept.
	_, jsonRes, err := NewMessagesSearchGlobalHandler(newMock())(
		context.Background(), nil, MessagesSearchGlobalParams{Query: "hi", Format: formatJSON})
	if err != nil {
		t.Fatalf("json: unexpected error: %v", err)
	}

	if jsonRes.Messages == nil || jsonRes.Output != "" {
		t.Errorf("json must keep messages and drop output, got messages=%v output=%q", jsonRes.Messages, jsonRes.Output)
	}

	// text — the messages are cleared, the summary output kept.
	_, textRes, err := NewMessagesSearchGlobalHandler(newMock())(
		context.Background(), nil, MessagesSearchGlobalParams{Query: "hi", Format: formatText})
	if err != nil {
		t.Fatalf("text: unexpected error: %v", err)
	}

	if textRes.Messages != nil || textRes.Output == "" {
		t.Errorf("text must drop messages and keep the summary output, got messages=%v output=%q",
			textRes.Messages, textRes.Output)
	}
}

// formatSchemaTools are the five read tools whose format is an optional
// protocol-level enum. The pin runs against the wire representation.
func formatSchemaTools() []*mcp.Tool {
	return []*mcp.Tool{
		MessagesListTool(),
		MessagesGetTool(),
		MessagesContextTool(),
		MessagesSearchTool(),
		MessagesSearchGlobalTool(),
	}
}

func TestFormatSchema_OptionalEnumOnEveryReadTool(t *testing.T) {
	type wireSchema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}

	for _, tool := range formatSchemaTools() {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("%s: marshal InputSchema: %v", tool.Name, err)
		}

		var schema wireSchema

		err = json.Unmarshal(raw, &schema)
		if err != nil {
			t.Fatalf("%s: unmarshal InputSchema: %v", tool.Name, err)
		}

		enum := schema.Properties["format"].Enum
		if !slices.Equal(enum, []string{"full", "json", "text"}) {
			t.Errorf("%s: format enum = %v, want [full json text]", tool.Name, enum)
		}

		// format is optional — a caller that omits it gets the full shape.
		if slices.Contains(schema.Required, "format") {
			t.Errorf("%s: format must not be required", tool.Name)
		}
	}
}

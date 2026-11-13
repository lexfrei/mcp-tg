package tools

import (
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolPolicy_ZeroValueAdmitsEverything(t *testing.T) {
	var policy ToolPolicy

	for _, tool := range []*mcp.Tool{MessagesListTool(), MessagesSendTool(), MessagesDeleteTool(), FoldersListTool()} {
		if !policy.Admits(tool) {
			t.Errorf("zero policy rejected %s", tool.Name)
		}
	}
}

func TestToolPolicy_ReadOnlyKeepsOnlyReadOnlyTools(t *testing.T) {
	policy := NewToolPolicy(true, nil, nil, false)

	if !policy.Admits(MessagesListTool()) {
		t.Error("read-only policy rejected tg_messages_list")
	}

	for _, tool := range []*mcp.Tool{MessagesSendTool(), MessagesDeleteTool(), MessagesPinTool()} {
		if policy.Admits(tool) {
			t.Errorf("read-only policy admitted %s", tool.Name)
		}
	}
}

func TestToolPolicy_OnlyAndDisabled(t *testing.T) {
	policy := NewToolPolicy(false, []string{"tg_messages_list", "tg_messages_send"}, []string{"tg_messages_send"}, false)

	if !policy.Admits(MessagesListTool()) {
		t.Error("rejected a tool on the TELEGRAM_TOOLS list")
	}

	if policy.Admits(MessagesSendTool()) {
		t.Error("admitted a tool that is both listed and disabled; disabling must win")
	}

	if policy.Admits(MessagesGetTool()) {
		t.Error("admitted a tool missing from the TELEGRAM_TOOLS list")
	}
}

func TestToolPolicy_PeerScopedHidesAccountWideTools(t *testing.T) {
	policy := NewToolPolicy(false, nil, nil, true)

	for _, tool := range []*mcp.Tool{MessagesSearchGlobalTool(), MessagesClearAllDraftsTool(), FoldersListTool()} {
		if policy.Admits(tool) {
			t.Errorf("peer-scoped policy admitted account-wide %s", tool.Name)
		}
	}

	if !policy.Admits(MessagesListTool()) {
		t.Error("peer-scoped policy rejected tg_messages_list, which takes a peer")
	}
}

// The build-version tool never touches Telegram and is how an operator
// identifies the binary, so no policy may hide it.
func TestToolPolicy_ServerVersionAlwaysAdmitted(t *testing.T) {
	policy := NewToolPolicy(true, []string{"tg_messages_list"}, []string{ServerVersionToolName}, true)

	if !policy.Admits(ServerVersionTool()) {
		t.Error("policy hid the server version tool")
	}
}

func TestRegistry_SkipsRejectedToolsAndReportsUnknownNames(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	registry := NewRegistry(NewToolPolicy(true, nil, []string{"tg_messages_list", "tg_no_such_tool"}, false))

	AddTool(server, registry, nil, MessagesListTool(), NewMessagesListHandler(&mockClient{}))
	AddTool(server, registry, nil, MessagesSendTool(), NewMessagesSendHandler(&mockClient{}))
	AddTool(server, registry, nil, MessagesGetTool(), NewMessagesGetHandler(&mockClient{}))

	if _, ok := registry.BoolFields["tg_messages_send"]; ok {
		t.Error("a rejected tool still recorded its bool fields")
	}

	if got := registry.UnknownTools(); !slices.Equal(got, []string{"tg_no_such_tool"}) {
		t.Errorf("UnknownTools = %q, want only tg_no_such_tool", got)
	}
}

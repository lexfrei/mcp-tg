package main

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/lexfrei/mcp-tg/internal/config"
	"github.com/lexfrei/mcp-tg/internal/middleware"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/lexfrei/mcp-tg/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func listToolsUnder(t *testing.T, access *config.Access) []*mcp.Tool {
	t.Helper()

	ctx := context.Background()

	authDone := make(chan struct{})
	close(authDone)

	server, err := newHeadlessServer(
		testutil.NoopClient{}, "/tmp/mcp-tg/downloads", nil,
		tgclient.NewSubscriptionBroker(), authDone, middleware.NewSessionHealth(), toolPolicy(access), discardLogger(),
	)
	if err != nil {
		t.Fatalf("build server: %v", err)
	}

	ct, st := mcp.NewInMemoryTransports()

	_, err = server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "access", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	return res.Tools
}

// Read-only mode must leave exactly the read-only half of the census: a
// tool missing its annotation would otherwise slip in as writable or drop
// out as unreadable without anything failing.
func TestReadOnlyMode_ListsOnlyReadOnlyTools(t *testing.T) {
	listed := listToolsUnder(t, &config.Access{ReadOnly: true})

	if len(listed) != wantReadOnlyTools {
		t.Errorf("read-only server lists %d tools, want the %d read-only ones", len(listed), wantReadOnlyTools)
	}

	for _, tool := range listed {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("read-only server lists %s, which is not read-only", tool.Name)
		}
	}
}

func TestToolsAllowlist_ListsOnlyTheNamedTools(t *testing.T) {
	listed := listToolsUnder(t, &config.Access{Tools: []string{"tg_messages_list", "tg_messages_send"}})

	names := make(map[string]bool, len(listed))
	for _, tool := range listed {
		names[tool.Name] = true
	}

	want := []string{"tg_messages_list", "tg_messages_send", tools.ServerVersionToolName}
	if len(names) != len(want) {
		t.Errorf("listed %d tools, want %d: %v", len(names), len(want), names)
	}

	for _, name := range want {
		if !names[name] {
			t.Errorf("%s is missing", name)
		}
	}
}

func TestAllowedPeers_HideAccountWideTools(t *testing.T) {
	listed := listToolsUnder(t, &config.Access{AllowedPeers: []string{"@friend"}})

	for _, tool := range listed {
		switch tool.Name {
		case "tg_messages_search_global", "tg_messages_clear_all_drafts", "tg_folders_list":
			t.Errorf("allowlisted server lists account-wide %s", tool.Name)
		}
	}
}

// A misspelt name in TELEGRAM_DISABLED_TOOLS would leave the tool it meant
// to disable enabled; startup must refuse it instead.
func TestUnknownToolName_FailsStartup(t *testing.T) {
	authDone := make(chan struct{})

	_, err := newHeadlessServer(
		testutil.NoopClient{}, "/tmp/mcp-tg/downloads", nil,
		tgclient.NewSubscriptionBroker(), authDone, middleware.NewSessionHealth(),
		toolPolicy(&config.Access{DisabledTools: []string{"tg_messages_delet"}}), discardLogger(),
	)
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("err = %v, want ErrUnknownTool", err)
	}

	// Live-tested wording: the sentence first, then the offending name.
	want := "unknown tool name in TELEGRAM_TOOLS or TELEGRAM_DISABLED_TOOLS: tg_messages_delet"
	if err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

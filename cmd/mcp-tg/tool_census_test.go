package main

import (
	"context"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/middleware"
	tgclient "github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The tool census documented in docs/tools.md (the published tool
// reference) and CLAUDE.md. These numbers drifted once already: the
// annotation counts were off by one for a whole release, and the next
// edit inherited the error instead of catching it.
//
// Two tests hang off these constants. This file pins them against the
// registered server; TestDocsAnnotationTable_MatchesTheCensus
// (docs_contract_test.go) pins docs/tools.md against them in turn, so the
// published page cannot drift from the server either.
//
// When a tool is added, update these numbers AND the two documents.
const (
	wantToolsTotal       = 78
	wantReadOnlyTools    = 31
	wantIdempotentTools  = 28
	wantWriteTools       = 10
	wantDestructiveTools = 9
)

func listRegisteredTools(t *testing.T) []*mcp.Tool {
	t.Helper()

	ctx := context.Background()

	authDone := make(chan struct{})
	close(authDone)

	server := newHeadlessServer(
		testutil.NoopClient{}, "/tmp/mcp-tg/downloads",
		tgclient.NewSubscriptionBroker(), authDone, middleware.NewSessionHealth(), discardLogger(),
	)

	ct, st := mcp.NewInMemoryTransports()

	_, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "census", Version: "0"}, nil)

	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	return res.Tools
}

// classifyTool maps a tool onto the four annotation buckets the docs
// count. The buckets are mutually exclusive by construction: read-only
// sets ReadOnlyHint, destructive sets DestructiveHint, idempotent sets
// IdempotentHint, and write sets none of the three.
func classifyTool(tool *mcp.Tool) string {
	ann := tool.Annotations
	if ann == nil {
		return "unannotated"
	}

	switch {
	case ann.ReadOnlyHint:
		return "readOnly"
	case ann.DestructiveHint != nil && *ann.DestructiveHint:
		return "destructive"
	case ann.IdempotentHint:
		return "idempotent"
	default:
		return "write"
	}
}

func TestToolCensus_MatchesTheDocumentedCounts(t *testing.T) {
	registered := listRegisteredTools(t)

	if len(registered) != wantToolsTotal {
		t.Errorf("registered %d tools, docs claim %d", len(registered), wantToolsTotal)
	}

	counts := make(map[string]int, 4)
	for _, tool := range registered {
		counts[classifyTool(tool)]++
	}

	if unannotated := counts["unannotated"]; unannotated != 0 {
		t.Errorf("%d tools carry no annotations", unannotated)
	}

	for bucket, want := range map[string]int{
		"readOnly":    wantReadOnlyTools,
		"idempotent":  wantIdempotentTools,
		"write":       wantWriteTools,
		"destructive": wantDestructiveTools,
	} {
		if counts[bucket] != want {
			t.Errorf("%s tools = %d, docs claim %d", bucket, counts[bucket], want)
		}
	}

	// A bucket total that misses the tool total means a tool was counted
	// twice or not at all — exactly how the documented numbers drifted.
	sum := wantReadOnlyTools + wantIdempotentTools + wantWriteTools + wantDestructiveTools
	if sum != wantToolsTotal {
		t.Errorf("documented buckets sum to %d, but the documented total is %d", sum, wantToolsTotal)
	}
}

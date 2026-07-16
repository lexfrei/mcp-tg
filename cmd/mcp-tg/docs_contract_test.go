package main

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
)

// The documentation site is built from docs/ and published to
// mcp-tg.lexfrei.dev. The pages below make claims the code can contradict,
// so each one is pinned here. README.md keeps only the claims it makes
// itself.
const (
	docsToolsPage    = "../../docs/tools.md"
	docsSearchPage   = "../../docs/search.md"
	docsMessagesPage = "../../docs/messages.md"
	readmePage       = "../../README.md"
)

var (
	docsToolBullet = regexp.MustCompile("^- `(tg_[a-z0-9_]+)`")
	docsToolsTitle = regexp.MustCompile(`^## Tools \((\d+)\)`)
)

// readDocsPage reads a documentation page, failing the test with the path
// so a moved page names itself instead of surfacing as a bare ENOENT.
func readDocsPage(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(raw)
}

// docsToolSection parses the tool page's "## Tools" section and returns the
// count claimed in the heading plus every tool name documented as a
// bullet. Bullets outside the section (e.g. the annotation table notes)
// must not count — that is exactly how the documented numbers drifted
// by five without anyone noticing.
func docsToolSection(t *testing.T) (int, []string) {
	t.Helper()

	file, err := os.Open(docsToolsPage)
	if err != nil {
		t.Fatalf("open %s: %v", docsToolsPage, err)
	}
	defer file.Close()

	var (
		claimed int
		names   []string
		inTools bool
	)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "## ") {
			match := docsToolsTitle.FindStringSubmatch(line)
			inTools = match != nil

			if match != nil {
				claimed, _ = strconv.Atoi(match[1])
			}

			continue
		}

		if !inTools {
			continue
		}

		if match := docsToolBullet.FindStringSubmatch(line); match != nil {
			names = append(names, match[1])
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", docsToolsPage, err)
	}

	return claimed, names
}

// TestDocsToolList_MatchesRegisteredTools pins the documented tool list
// against the registered server: every registered tool must be
// documented, no documented tool may be stale, and the heading count
// must match reality.
func TestDocsToolList_MatchesRegisteredTools(t *testing.T) {
	registered := listRegisteredTools(t)

	registeredNames := make(map[string]bool, len(registered))
	for _, tool := range registered {
		registeredNames[tool.Name] = true
	}

	claimed, documented := docsToolSection(t)

	if claimed != len(registered) {
		t.Errorf("%s heading claims %d tools, server registers %d", docsToolsPage, claimed, len(registered))
	}

	documentedNames := make(map[string]bool, len(documented))
	for _, name := range documented {
		if documentedNames[name] {
			t.Errorf("%s documents %s twice", docsToolsPage, name)
		}

		documentedNames[name] = true

		if !registeredNames[name] {
			t.Errorf("%s documents %s, but no such tool is registered", docsToolsPage, name)
		}
	}

	for name := range registeredNames {
		if !documentedNames[name] {
			t.Errorf("registered tool %s is missing from the %s tool list", name, docsToolsPage)
		}
	}
}

var docsAnnotationRow = regexp.MustCompile(`(?m)^\| (read-only|idempotent|write|destructive) \| (\d+) \|`)

// TestDocsAnnotationTable_MatchesTheCensus pins the per-bucket counts the
// tool page publishes against the census constants, which
// TestToolCensus_MatchesTheDocumentedCounts in turn pins against the
// registered server. Without this hop the page is free to drift: the
// heading count is checked above, but the four bucket numbers were not
// checked anywhere, which is how they sat one short for a whole release
// while they lived in prose.
func TestDocsAnnotationTable_MatchesTheCensus(t *testing.T) {
	body := readDocsPage(t, docsToolsPage)

	want := map[string]int{
		"read-only":   wantReadOnlyTools,
		"idempotent":  wantIdempotentTools,
		"write":       wantWriteTools,
		"destructive": wantDestructiveTools,
	}

	documented := make(map[string]int, len(want))

	for _, row := range docsAnnotationRow.FindAllStringSubmatch(body, -1) {
		count, err := strconv.Atoi(row[2])
		if err != nil {
			t.Fatalf("%s annotation row %q carries no count: %v", docsToolsPage, row[1], err)
		}

		documented[row[1]] = count
	}

	for bucket, count := range want {
		got, ok := documented[bucket]
		if !ok {
			t.Errorf("%s no longer documents the %s bucket count", docsToolsPage, bucket)

			continue
		}

		if got != count {
			t.Errorf("%s claims %d %s tools, the census says %d", docsToolsPage, got, bucket, count)
		}
	}
}

// A tool total stated as "78 tools" or "the full 78-tool reference". The
// tool page's own "## Tools (N)" heading is pinned by
// TestDocsToolList_MatchesRegisteredTools; this catches the totals the
// two landing surfaces restate.
var docsToolCountClaim = regexp.MustCompile(`(\d+)[ -]tools?\b`)

// The pages that advertise the tool total rather than documenting a
// subset of it. Only these two are scanned: a guide is free to write
// "the 6 tools that take sendAs", which is a different number by design,
// and prose like that on a landing page is what this test wants to catch
// anyway.
var docsToolCountPages = []string{readmePage, "../../docs/index.md"}

// TestDocsToolCount_MatchesTheCensus pins the tool total everywhere the
// landing pages restate it. The heading on the tool page is pinned
// against the server, but the split left the same number in four more
// places — a README blurb, its docs link, the index blurb and the
// protocol table — none of which any test read. Adding a tool would have
// left them all quietly wrong.
func TestDocsToolCount_MatchesTheCensus(t *testing.T) {
	for _, page := range docsToolCountPages {
		body := readDocsPage(t, page)

		claims := docsToolCountClaim.FindAllStringSubmatch(body, -1)
		if len(claims) == 0 {
			t.Errorf("%s no longer states the tool total — it is the first number a reader sees", page)
		}

		for _, claim := range claims {
			claimed, err := strconv.Atoi(claim[1])
			if err != nil {
				t.Fatalf("%s: unparseable tool count %q: %v", page, claim[0], err)
			}

			if claimed != wantToolsTotal {
				t.Errorf("%s claims %q, the census says %d tools", page, claim[0], wantToolsTotal)
			}
		}
	}
}

var docsFilterValues = regexp.MustCompile(`Values: ([^.]+)\.`)

// TestDocsFilterValues_MatchSearchFilters pins the documented filter
// value list against telegram.SearchFilters, the single source of
// truth — the same drift-by-hand failure mode as the tool census.
func TestDocsFilterValues_MatchSearchFilters(t *testing.T) {
	body := readDocsPage(t, docsSearchPage)

	match := docsFilterValues.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("%s no longer contains a 'Values: ...' filter list", docsSearchPage)
	}

	documented := regexp.MustCompile("`([a-z_]+)`").FindAllStringSubmatch(match[1], -1)

	names := make([]string, 0, len(documented))
	for _, m := range documented {
		names = append(names, m[1])
	}

	slices.Sort(names)

	if want := telegram.SearchFilters(); !slices.Equal(names, want) {
		t.Errorf("%s documents filter values %v, code accepts %v", docsSearchPage, names, want)
	}
}

// TestServerInstructions_MentionTheCompoundCursor pins the MCP server
// instructions — the first documentation an MCP client reads — to the
// global search cursor contract. They described plain offsetId
// pagination while the tool already demanded the full compound cursor,
// steering clients straight into ErrPartialCursor.
func TestServerInstructions_MentionTheCompoundCursor(t *testing.T) {
	opts := newServerOptions(testutil.NoopClient{}, telegram.NewSubscriptionBroker(), discardLogger())

	for _, field := range []string{"offsetRate", "nextRate", "nextOffsetId", "nextOffsetPeer"} {
		if !strings.Contains(opts.Instructions, field) {
			t.Errorf("server instructions no longer mention the cursor field %s", field)
		}
	}
}

// TestServerInstructions_MentionRequiredParseMode pins the parseMode
// contract in the first documentation an MCP client reads.
func TestServerInstructions_MentionRequiredParseMode(t *testing.T) {
	opts := newServerOptions(testutil.NoopClient{}, telegram.NewSubscriptionBroker(), discardLogger())

	for _, needle := range []string{"parseMode", "entitiesParsed", "CONTAINED formatting"} {
		if !strings.Contains(opts.Instructions, needle) {
			t.Errorf("server instructions no longer mention %s", needle)
		}
	}
}

// TestDocsParseMode_MatchesTheContract pins the message page to the
// shipped contract: both enum values named, the retired alias absent.
func TestDocsParseMode_MatchesTheContract(t *testing.T) {
	body := readDocsPage(t, docsMessagesPage)

	for _, needle := range []string{"'plain'", "'commonmark'", "allowRawMarkdown", "entitiesParsed", "<https://autolink>"} {
		if !strings.Contains(body, needle) {
			t.Errorf("%s no longer mentions %s", docsMessagesPage, needle)
		}
	}

	if strings.Contains(body, "legacy alias for") {
		t.Errorf("%s still documents the retired 'markdown' alias as usable", docsMessagesPage)
	}

	if !strings.Contains(body, "**Breaking change") {
		t.Errorf("%s no longer carries the parse-mode migration note", docsMessagesPage)
	}

	// The word-opening rule applies to doubled markers and links only —
	// backticks, fences and autolink brackets trigger anywhere. A page
	// that claims otherwise sends people to debug a lint that is working
	// as designed.
	if !strings.Contains(body, "trigger wherever they appear") {
		t.Errorf("%s no longer scopes the lint's word-opening rule correctly", docsMessagesPage)
	}
}

// TestReadmeMajorVersion_MatchesTheModulePath pins the promise the README
// makes about versioning against what the module can actually carry: a
// vN>=2 tag requires a matching /vN suffix in go.mod, or `go install
// ...@vN` fails while `@latest` silently keeps resolving to v1.
func TestReadmeMajorVersion_MatchesTheModulePath(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	readme := readDocsPage(t, readmePage)

	hasSuffix := regexp.MustCompile(`(?m)^module .+/v[2-9]\d*$`).Match(gomod)
	promisesMajor := regexp.MustCompile(`v[2-9]\d*\.0\.0`).MatchString(readme)

	if promisesMajor && !hasSuffix {
		t.Error("README promises a major version the module path cannot carry — add the /vN suffix to go.mod or drop the promise")
	}
}

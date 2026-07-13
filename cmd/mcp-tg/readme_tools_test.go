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

var (
	readmeToolBullet = regexp.MustCompile("^- `(tg_[a-z0-9_]+)`")
	readmeToolsTitle = regexp.MustCompile(`^## Tools \((\d+)\)`)
)

// readmeToolSection parses README's "## Tools" section and returns the
// count claimed in the heading plus every tool name documented as a
// bullet. Bullets outside the section (e.g. the peer-identifier notes)
// must not count — that is exactly how the documented numbers drifted
// by five without anyone noticing.
func readmeToolSection(t *testing.T) (int, []string) {
	t.Helper()

	file, err := os.Open("../../README.md")
	if err != nil {
		t.Fatalf("open README: %v", err)
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
			match := readmeToolsTitle.FindStringSubmatch(line)
			inTools = match != nil

			if match != nil {
				claimed, _ = strconv.Atoi(match[1])
			}

			continue
		}

		if !inTools {
			continue
		}

		if match := readmeToolBullet.FindStringSubmatch(line); match != nil {
			names = append(names, match[1])
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan README: %v", err)
	}

	return claimed, names
}

// TestReadmeToolList_MatchesRegisteredTools pins the README tool list
// against the registered server: every registered tool must be
// documented, no documented tool may be stale, and the heading count
// must match reality.
func TestReadmeToolList_MatchesRegisteredTools(t *testing.T) {
	registered := listRegisteredTools(t)

	registeredNames := make(map[string]bool, len(registered))
	for _, tool := range registered {
		registeredNames[tool.Name] = true
	}

	claimed, documented := readmeToolSection(t)

	if claimed != len(registered) {
		t.Errorf("README heading claims %d tools, server registers %d", claimed, len(registered))
	}

	documentedNames := make(map[string]bool, len(documented))
	for _, name := range documented {
		if documentedNames[name] {
			t.Errorf("README documents %s twice", name)
		}

		documentedNames[name] = true

		if !registeredNames[name] {
			t.Errorf("README documents %s, but no such tool is registered", name)
		}
	}

	for name := range registeredNames {
		if !documentedNames[name] {
			t.Errorf("registered tool %s is missing from the README tool list", name)
		}
	}
}

var readmeFilterValues = regexp.MustCompile(`Values: ([^.]+)\.`)

// TestReadmeFilterValues_MatchSearchFilters pins the documented filter
// value list against telegram.SearchFilters, the single source of
// truth — the same drift-by-hand failure mode as the tool census.
func TestReadmeFilterValues_MatchSearchFilters(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	match := readmeFilterValues.FindStringSubmatch(string(raw))
	if match == nil {
		t.Fatal("README no longer contains a 'Values: ...' filter list")
	}

	documented := regexp.MustCompile("`([a-z_]+)`").FindAllStringSubmatch(match[1], -1)

	names := make([]string, 0, len(documented))
	for _, m := range documented {
		names = append(names, m[1])
	}

	slices.Sort(names)

	if want := telegram.SearchFilters(); !slices.Equal(names, want) {
		t.Errorf("README documents filter values %v, code accepts %v", names, want)
	}
}

// TestServerInstructions_MentionTheCompoundCursor pins the MCP server
// instructions — the first documentation an MCP client reads — to the
// global search cursor contract. They described plain offsetId
// pagination while the tool already demanded the full compound cursor,
// steering clients straight into ErrPartialCursor.
func TestServerInstructions_MentionTheCompoundCursor(t *testing.T) {
	opts := newServerOptions(testutil.NoopClient{})

	for _, field := range []string{"offsetRate", "nextRate", "nextOffsetId", "nextOffsetPeer"} {
		if !strings.Contains(opts.Instructions, field) {
			t.Errorf("server instructions no longer mention the cursor field %s", field)
		}
	}
}

// TestServerInstructions_MentionRequiredParseMode pins the parseMode
// contract in the first documentation an MCP client reads.
func TestServerInstructions_MentionRequiredParseMode(t *testing.T) {
	opts := newServerOptions(testutil.NoopClient{})

	for _, needle := range []string{"parseMode", "entitiesParsed", "CONTAINED formatting"} {
		if !strings.Contains(opts.Instructions, needle) {
			t.Errorf("server instructions no longer mention %s", needle)
		}
	}
}

// TestReadmeParseMode_MatchesTheContract pins the README section to the
// shipped contract: both enum values named, the retired alias absent.
func TestReadmeParseMode_MatchesTheContract(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	body := string(raw)

	for _, needle := range []string{"'plain'", "'commonmark'", "allowRawMarkdown", "entitiesParsed", "<https://autolink>"} {
		if !strings.Contains(body, needle) {
			t.Errorf("README no longer mentions %s", needle)
		}
	}

	if strings.Contains(body, "'markdown' alias") {
		t.Error("README still documents the retired 'markdown' alias")
	}

	if !strings.Contains(body, "Breaking change in this release.") {
		t.Error("README no longer carries the parse-mode migration note")
	}
}

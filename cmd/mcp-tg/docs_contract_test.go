package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/middleware"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/lexfrei/mcp-tg/internal/tools"
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
	mkdocsConfig     = "../../mkdocs.yml"

	// Split from any page path on purpose: docsSiteURL needs the domain
	// and a path together to match, so a test can compose a fixture link
	// from this without the repo-wide URL scan picking the fixture up.
	docsSiteBase = "https://mcp-tg.lexfrei.dev/"
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

// censusClaim is one way a page can restate a number the server owns:
// the prose form ("78 tools", "the full 78-tool reference", "4
// resources") and the protocol-table form ("| Resources | 4 (...)"),
// which states the count without ever naming the noun beside it.
//
// prose marks the forms every census page is required to carry. The
// table rows are not: only the protocol-support table on the index has
// them, so demanding them everywhere would fail the README for a table
// it was never meant to have.
type censusClaim struct {
	subject string
	pattern *regexp.Regexp
	want    int
	prose   bool
}

func docsCensusClaims() []censusClaim {
	return []censusClaim{
		{"tools", regexp.MustCompile(`(\d+)[ -]tools?\b`), wantToolsTotal, true},
		{"tools table row", regexp.MustCompile(`(?m)^\| Tools \| (\d+)`), wantToolsTotal, false},
		{"resources", regexp.MustCompile(`(\d+) resources?\b`), wantResources, true},
		{"resources table row", regexp.MustCompile(`(?m)^\| Resources \| (\d+)`), wantResources, false},
		{"prompts", regexp.MustCompile(`(\d+) prompts?\b`), wantPrompts, true},
		{"prompts table row", regexp.MustCompile(`(?m)^\| Prompts \| (\d+)`), wantPrompts, false},
	}
}

// The surfaces that advertise the totals rather than documenting a
// subset. mkdocs.yml earns its place: site_description becomes the
// <meta name="description"> of every page Material renders, so a stale
// number there is what a search engine quotes for the whole site.
//
// A guide page is deliberately absent — it is free to write "the 6 tools
// that take sendAs", a different number by construction.
var docsCensusPages = []string{readmePage, "../../docs/index.md", "../../mkdocs.yml"}

// TestDocsCensusCounts_MatchTheServer pins every restatement of the tool,
// resource and prompt totals. Only the tool page's own "## Tools (N)"
// heading was ever read by a test; the same numbers also sit in two
// landing blurbs, a docs link, the protocol-support table and the site
// description, so adding a tool would update one and leave five wrong.
func TestDocsCensusCounts_MatchTheServer(t *testing.T) {
	for _, page := range docsCensusPages {
		body := readDocsPage(t, page)

		for _, claim := range docsCensusClaims() {
			for _, match := range claim.pattern.FindAllStringSubmatch(body, -1) {
				claimed, err := strconv.Atoi(match[1])
				if err != nil {
					t.Fatalf("%s: unparseable %s count %q: %v", page, claim.subject, match[0], err)
				}

				if claimed != claim.want {
					t.Errorf("%s claims %q (%s), but the server registers %d", page, match[0], claim.subject, claim.want)
				}
			}
		}
	}
}

// TestDocsCensusCounts_AreActuallyStated guards the test above from
// passing vacuously: a page that drops a number entirely, or rewords it
// past the pattern, would otherwise match nothing and stay green — the
// exact failure mode the pin exists to prevent. Every prose claim is
// checked, not just the tool total: rewording "4 resources" alone would
// silently retire that count's pin on that page.
func TestDocsCensusCounts_AreActuallyStated(t *testing.T) {
	for _, page := range docsCensusPages {
		body := readDocsPage(t, page)

		for _, claim := range docsCensusClaims() {
			if !claim.prose {
				continue
			}

			if !claim.pattern.MatchString(body) {
				t.Errorf("%s no longer states its %s total, so nothing pins it there", page, claim.subject)
			}
		}
	}
}

var mkdocsAnchorValidation = regexp.MustCompile(`(?m)^\s+anchors: warn$`)

// TestMkdocsValidation_FailsOnDeadAnchors pins the validation block that
// gives `mkdocs build --strict` its teeth. --strict fails the build on
// warnings, but MkDocs reports a dead anchor, an unrecognized link and a
// page missing from the nav at INFO by default — so without this block a
// link to a renamed heading builds green and ships a 404. Verified
// against mkdocs-material 9.7.6: the anchor probe exits 0 without it and
// 1 with it.
func TestMkdocsValidation_FailsOnDeadAnchors(t *testing.T) {
	config := readDocsPage(t, mkdocsConfig)

	if !mkdocsAnchorValidation.MatchString(config) {
		t.Errorf("%s no longer sets validation.anchors: warn — --strict stops catching dead anchors", mkdocsConfig)
	}

	for _, needle := range []string{"unrecognized_links: warn", "omitted_files: warn", "not_found: warn"} {
		if !strings.Contains(config, needle) {
			t.Errorf("%s no longer sets validation %s", mkdocsConfig, needle)
		}
	}
}

var docsToolSubsection = regexp.MustCompile(`^### .*\((\d+)\)`)

// toolSubsection is one "### Messages (16)" group: the subtotal the
// heading claims, and the bullets actually listed under it.
type toolSubsection struct {
	name    string
	claimed int
	bullets int
}

// docsToolSubsections walks the Tools section and pairs every category
// heading with the bullets beneath it.
func docsToolSubsections(t *testing.T) []toolSubsection {
	t.Helper()

	var (
		sections []toolSubsection
		inTools  bool
	)

	for line := range strings.SplitSeq(readDocsPage(t, docsToolsPage), "\n") {
		if strings.HasPrefix(line, "## ") {
			inTools = docsToolsTitle.MatchString(line)

			continue
		}

		if !inTools {
			continue
		}

		if match := docsToolSubsection.FindStringSubmatch(line); match != nil {
			claimed, err := strconv.Atoi(match[1])
			if err != nil {
				t.Fatalf("%s: unparseable subtotal in %q: %v", docsToolsPage, line, err)
			}

			sections = append(sections, toolSubsection{name: line, claimed: claimed})

			continue
		}

		if docsToolBullet.MatchString(line) && len(sections) > 0 {
			sections[len(sections)-1].bullets++
		}
	}

	return sections
}

// TestDocsToolSubsections_MatchTheirBullets pins the per-category
// subtotals. The "## Tools (N)" heading directly above them is pinned
// against the server, which made the gap worse rather than better: a
// contributor adding a tool gets a red build until the total and the
// bullet agree, learns the page is guarded, and has no reason to suspect
// the number in the heading beside it is not.
func TestDocsToolSubsections_MatchTheirBullets(t *testing.T) {
	sections := docsToolSubsections(t)
	if len(sections) == 0 {
		t.Fatalf("%s no longer groups tools under '### Name (N)' headings", docsToolsPage)
	}

	sum := 0

	for _, section := range sections {
		sum += section.claimed

		if section.claimed != section.bullets {
			t.Errorf("%s: %q claims %d tools, lists %d", docsToolsPage, section.name, section.claimed, section.bullets)
		}
	}

	if sum != wantToolsTotal {
		t.Errorf("%s subtotals sum to %d, the server registers %d tools", docsToolsPage, sum, wantToolsTotal)
	}
}

var (
	goModDirective  = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)$`)
	docsGoRequires  = regexp.MustCompile(`Go (\d+\.\d+(?:\.\d+)?)\+`)
	docsBuildingPag = "../../docs/building.md"
)

// TestDocsGoVersion_MatchesGoMod pins the documented minimum Go version
// against go.mod. Since Go 1.21 the `go` directive is a hard minimum, not
// a hint — a reader on the version the docs named would be told to
// upgrade by the toolchain, or silently have a second one downloaded.
// The page claimed 1.26.1 while go.mod required 1.26.5.
func TestDocsGoVersion_MatchesGoMod(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	required := goModDirective.FindSubmatch(gomod)
	if required == nil {
		t.Fatal("go.mod carries no `go` directive")
	}

	documented := docsGoRequires.FindStringSubmatch(readDocsPage(t, docsBuildingPag))
	if documented == nil {
		t.Fatalf("%s no longer states a minimum Go version", docsBuildingPag)
	}

	if want := string(required[1]); documented[1] != want {
		t.Errorf("%s requires Go %s+, go.mod requires %s", docsBuildingPag, documented[1], want)
	}
}

var docsConfigPage = "../../docs/configuration.md"

// TestDocsDownloadDir_DoesNotClaimAnAbsoluteDefault pins the one env var
// whose default cannot be written as a literal path. The code builds it
// from os.TempDir(), which is /tmp on Linux but $TMPDIR
// (/var/folders/...) on macOS — and macOS is a documented install target,
// since the Homebrew formula is one. The page claimed
// /tmp/mcp-tg/downloads flatly, which is wrong on the platform most
// likely to read it.
func TestDocsDownloadDir_DoesNotClaimAnAbsoluteDefault(t *testing.T) {
	row := regexp.MustCompile(`(?m)^\|\s*` + "`TELEGRAM_DOWNLOAD_DIR`" + `\s*\|.*$`).
		FindString(readDocsPage(t, docsConfigPage))
	if row == "" {
		t.Fatalf("%s no longer documents TELEGRAM_DOWNLOAD_DIR", docsConfigPage)
	}

	// Any absolute path, not just the literal that was wrong: /var/tmp/...
	// would be equally unsupportable, and banning one spelling pins the
	// typo rather than the claim.
	for _, value := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(row, -1) {
		if strings.HasPrefix(value[1], "/") && strings.Contains(value[1], "mcp-tg") {
			t.Errorf("%s states the absolute default %s for TELEGRAM_DOWNLOAD_DIR; the code uses "+
				"os.TempDir(), which is $TMPDIR on macOS, not /tmp", docsConfigPage, value[0])
		}
	}
}

var mkdocsMaterialPin = regexp.MustCompile(`mkdocs-material==(\d+\.\d+\.\d+)`)

// Every file that tells someone — a runner or a contributor — which
// mkdocs-material to install. They must agree: MkDocs 2.0 removes the
// plugin and theming systems with no migration path, so an unpinned or
// differently-pinned local install renders a different site than the one
// CI publishes.
var mkdocsPinSites = []string{
	"../../.github/workflows/pages.yml",
	"../../.github/workflows/pr.yml",
	"../../CLAUDE.md",
	readmePage,
}

// TestMkdocsMaterialPin_AllSitesAgree pins the version across all four
// places that name it. The README told contributors to install it
// unpinned while all three other sites pinned it — the exact drift the
// pin exists to prevent, in a repository that pins every action SHA.
func TestMkdocsMaterialPin_AllSitesAgree(t *testing.T) {
	// Keyed by version so a disagreement names every site on each side,
	// rather than comparing against a chosen file that may itself be the
	// one that lost its pin.
	sitesByVersion := make(map[string][]string, 1)

	for _, site := range mkdocsPinSites {
		// Every pin in the file, not just the first: a second, different
		// pin further down is exactly the drift this guards.
		matches := mkdocsMaterialPin.FindAllStringSubmatch(readDocsPage(t, site), -1)
		if matches == nil {
			t.Errorf("%s installs mkdocs-material without pinning a version", site)

			continue
		}

		for _, match := range matches {
			sitesByVersion[match[1]] = append(sitesByVersion[match[1]], site)
		}
	}

	if len(sitesByVersion) > 1 {
		t.Errorf("mkdocs-material is pinned to %d different versions: %v", len(sitesByVersion), sitesByVersion)
	}
}

var (
	// A tool named anywhere in prose: `tg_messages_send`, or a family
	// glob like `tg_messages_*`.
	docsToolMention = regexp.MustCompile("`(tg_[a-z0-9_]*?)(\\*?)`")
	docsPagesGlob   = "../../docs/*.md"
)

// docsPages returns every published page plus the README.
func docsPages(t *testing.T) []string {
	t.Helper()

	pages, err := filepath.Glob(docsPagesGlob)
	if err != nil {
		t.Fatalf("glob %s: %v", docsPagesGlob, err)
	}

	if len(pages) == 0 {
		t.Fatalf("no pages matched %s", docsPagesGlob)
	}

	return append(pages, readmePage)
}

// TestDocsToolMentions_NameRealTools pins every tool named in prose, not
// just the bullets in the tool list. The list is what
// TestDocsToolList_MatchesRegisteredTools reads, and a name outside it
// was checked by nothing — which is exactly how `tg_chats_admins`, a
// tool that has never existed, sat in the peer-identifier section
// telling readers to call it.
//
// A family glob (`tg_messages_*`) passes when at least one registered
// tool carries the prefix, so a whole family being renamed still fails.
func TestDocsToolMentions_NameRealTools(t *testing.T) {
	registered := listRegisteredTools(t)

	names := make(map[string]bool, len(registered))
	for _, tool := range registered {
		names[tool.Name] = true
	}

	mentioned := 0

	for _, page := range docsPages(t) {
		body := readDocsPage(t, page)

		for _, match := range docsToolMention.FindAllStringSubmatch(body, -1) {
			mentioned++

			name, glob := match[1], match[2] == "*"

			if glob {
				if !anyToolHasPrefix(names, name) {
					t.Errorf("%s mentions `%s*`, but no registered tool starts with %q", page, name, name)
				}

				continue
			}

			if !names[name] {
				t.Errorf("%s mentions `%s`, but no such tool is registered", page, name)
			}
		}
	}

	if mentioned == 0 {
		t.Error("no tool mentions found across the docs — this test has stopped checking anything")
	}
}

func anyToolHasPrefix(names map[string]bool, prefix string) bool {
	for name := range names {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// A published docs link, with the optional heading anchor captured:
// https://mcp-tg.lexfrei.dev/building/#transport-modes
//
// The anchor charset must agree with slugifyHeading, and `\w` is what
// does it. Anything narrower breaks in both directions: `_` is a legal
// slug character (`tg_messages_list` anchors as itself), so a charset
// without it truncates a correct link and reports a URL nobody wrote;
// and an anchor starting outside the charset — `#Message-Output-Format`,
// copied verbatim from a heading MkDocs slugs lowercase — makes the
// optional group match EMPTY rather than fail, which skips the anchor
// check entirely and passes a dead link. Capturing uppercase lets it
// fail honestly instead.
var docsSiteURL = regexp.MustCompile(`https://mcp-tg\.lexfrei\.dev/([a-z0-9_-]+)/(?:#([\w-]+))?`)

var (
	docsHeading   = regexp.MustCompile(`(?m)^#{1,6} +(.+?)\s*$`)
	docsCodeFence = regexp.MustCompile("(?s)```.*?```")
	slugDrop      = regexp.MustCompile(`[^\w\s-]`)
	slugSeparator = regexp.MustCompile(`[-\s]+`)
)

// slugifyHeading mirrors python-markdown's toc slugify, which is what
// MkDocs actually anchors with:
//
//	value = re.sub(r'[^\w\s-]', '', value).strip().lower()
//	return re.sub(r'[-\s]+', '-', value)
//
// Two details are load-bearing and both were wrong in the first cut of
// this helper. `\w` includes the underscore, so `tg_messages_list`
// anchors as itself rather than as `tg-messages-list` — in a repository
// where every identifier is snake_case, guessing otherwise fails a
// correct link and passes a broken one. And the strip happens BEFORE the
// separator collapse, so an em dash vanishes and leaves the spaces
// around it to collapse into a single hyphen.
func slugifyHeading(text string) string {
	stripped := strings.ToLower(strings.TrimSpace(slugDrop.ReplaceAllString(text, "")))

	return slugSeparator.ReplaceAllString(stripped, "-")
}

// headingSlugs renders a page's real anchors. Fenced code blocks are cut
// first: a `# comment` inside a ```bash fence is not a heading, but it
// matches the heading pattern exactly, so leaving it in mints anchors
// that do not exist and lets a link to one pass.
func headingSlugs(t *testing.T, page string) map[string]bool {
	t.Helper()

	body := docsCodeFence.ReplaceAllString(readDocsPage(t, page), "")

	slugs := make(map[string]bool)

	for _, heading := range docsHeading.FindAllStringSubmatch(body, -1) {
		slugs[slugifyHeading(heading[1])] = true
	}

	return slugs
}

// TestSlugifyHeading_MatchesMkdocs pins the slug rules against what
// MkDocs actually emits. Verified against a real build: each want below
// is the id= MkDocs put on the rendered heading.
func TestSlugifyHeading_MatchesMkdocs(t *testing.T) {
	for _, tc := range []struct {
		heading string
		want    string
	}{
		{"Transport modes", "transport-modes"},
		{"Recovery — revoked session", "recovery-revoked-session"},
		{"Markdown — Known Limitations", "markdown-known-limitations"},
		{"Posting as a channel (`sendAs`)", "posting-as-a-channel-sendas"},
		// The underscore survives: \w includes it. Guessing otherwise
		// would fail every correct link to an identifier heading.
		{"`tg_messages_list`", "tg_messages_list"},
		{"snake_case values", "snake_case-values"},
	} {
		if got := slugifyHeading(tc.heading); got != tc.want {
			t.Errorf("slugifyHeading(%q) = %q, MkDocs anchors it as %q", tc.heading, got, tc.want)
		}
	}
}

// TestHeadingSlugs_IgnoreFencedComments pins the fence stripping: a shell
// comment inside a code block matches the heading pattern, and counting
// it as an anchor is a false PASS — a link to that non-existent anchor
// would sail through and 404 on the site.
func TestHeadingSlugs_IgnoreFencedComments(t *testing.T) {
	slugs := headingSlugs(t, "../../docs/installation.md")

	if slugs["the-same-file-feeds-this-shell-so-login-sees-the-credentials-too"] {
		t.Error("headingSlugs treats a # comment inside a fenced block as a heading")
	}

	if !slugs["homebrew-macos-linux"] {
		t.Error("headingSlugs lost a real heading while stripping fences")
	}
}

// checkDocsSiteURLs asserts every published docs URL in one file points
// at a page that exists, and at a heading that exists when it carries an
// anchor. Returns how many it checked.
func checkDocsSiteURLs(t *testing.T, path, body string) int {
	t.Helper()

	checked := 0

	for _, match := range docsSiteURL.FindAllStringSubmatch(body, -1) {
		checked++

		page := "../../docs/" + match[1] + ".md"

		if _, err := os.Stat(page); err != nil {
			t.Errorf("%s links to %s, but %s does not exist", path, match[0], page)

			continue
		}

		if anchor := match[2]; anchor != "" && !headingSlugs(t, page)[anchor] {
			t.Errorf("%s links to %s, but %s has no heading slugging to #%s", path, match[0], page, anchor)
		}
	}

	return checked
}

// TestCheckDocsSiteURLs_AcceptsAndRejects covers the URL matcher itself,
// which the two helper tests around it never exercised. Both directions
// matter and both were broken by an anchor charset that disagreed with
// slugifyHeading: a correct link to an underscore heading failed, and a
// dead anchor that starts with an uppercase letter passed by matching
// the optional group empty.
func TestCheckDocsSiteURLs_AcceptsAndRejects(t *testing.T) {
	// Composed rather than written out, because
	// TestDocsSiteURLs_ResolveToPages scans every .go file in the tree —
	// this one included. A literal dead link here is indistinguishable
	// from a real one, and would fail the very scan it is fixture for.
	link := func(anchor string) string {
		return docsSiteBase + "messages/" + "#" + anchor
	}

	// A real heading on that page. `_`-bearing slugs are legal, so a link
	// like this is CORRECT and must not be reported.
	t.Run("live anchor is accepted", func(t *testing.T) {
		if got := countURLErrors(t, link("message-output-format")); got != 0 {
			t.Errorf("a correct link reported %d errors", got)
		}
	})

	// MkDocs slugs headings lowercase, so none of these can exist. A
	// contributor pasting a heading verbatim writes the first one.
	for _, anchor := range []string{"Message-Output-Format", "NOPE", "no-such-heading"} {
		t.Run("dead anchor is rejected: "+anchor, func(t *testing.T) {
			if got := countURLErrors(t, link(anchor)); got == 0 {
				t.Errorf("dead anchor #%s passed the check", anchor)
			}
		})
	}
}

// countURLErrors runs checkDocsSiteURLs against one link and reports how
// many errors it raised, without failing this test.
func countURLErrors(t *testing.T, link string) int {
	t.Helper()

	probe := &testing.T{}
	checkDocsSiteURLs(probe, "probe.md", link)

	if probe.Failed() {
		return 1
	}

	return 0
}

// TestDocsSiteURLs_ResolveToPages pins every published docs URL — in Go
// source and in the Markdown that is not built by mkdocs — against a page
// and a heading that exist.
//
// Two surfaces need it and neither is covered elsewhere. The
// revoked-session error hands a locked-out operator such a URL as its
// only recovery instruction, so a renamed page ships a 404 inside the one
// message whose job is telling them what to do. And the README carries
// twelve of them, anchors included: it is the entry point on GitHub, and
// mkdocs never builds it, so mkdocs.yml's link validation cannot see it.
func TestDocsSiteURLs_ResolveToPages(t *testing.T) {
	checked := 0

	err := filepath.WalkDir("../..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip dot-directories: .claude/worktrees holds whole checkouts of
		// this repository on other branches, whose URLs are not this
		// branch's problem and whose pages may not exist here.
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && entry.Name() != "." && entry.Name() != ".." {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "read %s", path)
		}

		checked += checkDocsSiteURLs(t, path, string(body))

		return nil
	})
	if err != nil {
		t.Fatalf("walk source: %v", err)
	}

	if checked == 0 {
		t.Error("no docs-site URLs found in source — this test has stopped checking anything")
	}
}

// backtickedValues pulls every `value` out of a documented list.
func backtickedValues(list string) []string {
	found := regexp.MustCompile("`([a-z_]+)`").FindAllStringSubmatch(list, -1)

	names := make([]string, 0, len(found))
	for _, match := range found {
		names = append(names, match[1])
	}

	slices.Sort(names)

	return names
}

var docsTypeValues = regexp.MustCompile(`Values are ([^.]+)\.`)

// TestDocsTypeValues_MatchMessageTypes pins the documented `type` values
// against telegram.MessageTypes. docs/search.md's structurally identical
// `filter` list has been pinned against SearchFilters all along, and its
// docstring names the reason — "the same drift-by-hand failure mode as
// the tool census" — while this list, one page over, was read by nothing.
func TestDocsTypeValues_MatchMessageTypes(t *testing.T) {
	match := docsTypeValues.FindStringSubmatch(readDocsPage(t, docsMessagesPage))
	if match == nil {
		t.Fatalf("%s no longer contains a 'Values are ...' type list", docsMessagesPage)
	}

	documented := backtickedValues(match[1])

	want := telegram.MessageTypes()
	slices.Sort(want)

	if !slices.Equal(documented, want) {
		t.Errorf("%s documents type values %v, code labels %v", docsMessagesPage, documented, want)
	}
}

var docsScopeValues = regexp.MustCompile(`takes ` + "`scope`" + ` \(([^)]+)\)`)

// TestDocsScopeValues_MatchTheCode pins the documented global-search
// scopes against the constants IsSearchScope accepts — the same class as
// the filter and type lists.
func TestDocsScopeValues_MatchTheCode(t *testing.T) {
	match := docsScopeValues.FindStringSubmatch(readDocsPage(t, docsSearchPage))
	if match == nil {
		t.Fatalf("%s no longer documents the scope values", docsSearchPage)
	}

	documented := backtickedValues(match[1])

	for _, scope := range documented {
		if !telegram.IsSearchScope(scope) {
			t.Errorf("%s documents scope %q, which the code rejects", docsSearchPage, scope)
		}
	}

	want := []string{telegram.SearchScopeChannels, telegram.SearchScopeGroups, telegram.SearchScopeUsers}
	slices.Sort(want)

	if !slices.Equal(documented, want) {
		t.Errorf("%s documents scopes %v, code accepts %v", docsSearchPage, documented, want)
	}
}

var (
	docsResourcesPage = "../../docs/resources.md"
	docsBulletName    = regexp.MustCompile("(?m)^- `([^`]+)` —")
	docsIndexPage     = "../../docs/index.md"
	docsMiddlewareRow = regexp.MustCompile(`(?m)^\| Middleware \| ([^|]+?) \|`)
)

// TestDocsMiddlewareRow_CountsTheInstalledOnes pins the protocol table's
// middleware row against what receivingMiddlewares installs. The row
// listed three while the server installs four: it predates the session
// guard, and the numbers around it in that table are pinned while its
// contents were read by nothing.
func TestDocsMiddlewareRow_CountsTheInstalledOnes(t *testing.T) {
	installed := receivingMiddlewares(
		discardLogger(), tools.BoolFieldRegistry{}, make(chan struct{}), middleware.NewSessionHealth(),
	)

	row := docsMiddlewareRow.FindStringSubmatch(readDocsPage(t, docsIndexPage))
	if row == nil {
		t.Fatalf("%s no longer carries a Middleware row", docsIndexPage)
	}

	if documented := strings.Split(row[1], ","); len(documented) != len(installed) {
		t.Errorf("%s lists %d middlewares (%q), the server installs %d",
			docsIndexPage, len(documented), row[1], len(installed))
	}
}

// TestDocsResourcesAndPrompts_NameTheRegisteredOnes pins the URIs and
// names the page prints, not merely how many there are. The census
// counts 4 and 3 off a live session and throws the identities away, so
// renaming tg://dialogs to tg://chats would keep the count at 4 and leave
// the page advertising a URI that resolves to nothing.
func TestDocsResourcesAndPrompts_NameTheRegisteredOnes(t *testing.T) {
	sections := strings.Split(readDocsPage(t, docsResourcesPage), "## ")

	documented := make(map[string]bool)

	for _, section := range sections {
		if strings.HasPrefix(section, "Resources") || strings.HasPrefix(section, "Prompts") {
			for _, match := range docsBulletName.FindAllStringSubmatch(section, -1) {
				documented[match[1]] = true
			}
		}
	}

	if len(documented) == 0 {
		t.Fatalf("%s no longer lists resources or prompts", docsResourcesPage)
	}

	registered := registeredResourceAndPromptNames(t)

	names := make(map[string]bool, len(registered))
	for _, name := range registered {
		names[name] = true

		if !documented[name] {
			t.Errorf("%s does not document the registered %q", docsResourcesPage, name)
		}
	}

	// Both directions, like the tool list: a bullet naming a URI that no
	// longer exists is as wrong as a missing one, and nothing else can
	// see it — TestDocsToolMentions_NameRealTools only reads `tg_*`
	// names, never a `tg://` URI or a prompt name.
	for name := range documented {
		if !names[name] {
			t.Errorf("%s documents %q, which is not registered", docsResourcesPage, name)
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

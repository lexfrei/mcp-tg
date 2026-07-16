package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/middleware"
	"github.com/lexfrei/mcp-tg/internal/telegram"
	"github.com/lexfrei/mcp-tg/internal/testutil"
	"github.com/lexfrei/mcp-tg/internal/tools"
	"golang.org/x/text/unicode/norm"
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
	goModDirective   = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)$`)
	docsGoRequires   = regexp.MustCompile(`Go (\d+\.\d+(?:\.\d+)?)\+`)
	docsBuildingPage = "../../docs/building.md"
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

	documented := docsGoRequires.FindStringSubmatch(readDocsPage(t, docsBuildingPage))
	if documented == nil {
		t.Fatalf("%s no longer states a minimum Go version", docsBuildingPage)
	}

	if want := string(required[1]); documented[1] != want {
		t.Errorf("%s requires Go %s+, go.mod requires %s", docsBuildingPage, documented[1], want)
	}
}

var docsConfigPage = "../../docs/configuration.md"

// TestDocsDownloadDir_DoesNotClaimAnAbsoluteDefault pins the one env var
// whose default cannot be written as a literal path. The code builds it
// from os.TempDir(), which returns $TMPDIR whenever it is set — on EVERY
// Unix, not just macOS — and falls back to /tmp only when it is not.
// macOS launchd sets it; a plain container does not (this project's own
// image is FROM scratch with two ENVs, neither of them TMPDIR), and
// neither do Linux CI runners. Windows has neither path. The page
// claimed /tmp/mcp-tg/downloads flatly, which is wrong wherever TMPDIR
// is set — but note this test pins the SHAPE of the claim, not its
// truth: it rejects an absolute path and cannot tell whether the prose
// replacing it is true of the world. That part is on the reader.
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
				"os.TempDir(), which honours $TMPDIR on every Unix", docsConfigPage, value[0])
		}
	}
}

const prWorkflow = "../../.github/workflows/pr.yml"

// requiredPRJobs are the jobs master's branch protection requires. A
// check that is not one of these does not block a merge, so a gate
// placed outside them is advisory no matter what its comment claims.
// Hand-maintained by necessity: a test cannot read the setting.
var requiredPRJobs = []string{"test"}

// requiredPRSteps are the docs gates that must run inside one of those
// jobs. Both check something no other test can see — the site build
// covers relative links and anchors inside docs/, which
// TestDocsSiteURLs_ResolveToPages cannot (it reads absolute URLs only);
// the Markdown lint covers everything its config has always described
// and nothing ever enforced.
var requiredPRSteps = []string{"mkdocs build --strict", "markdownlint-cli2@"}

// TestDocsGates_RunInARequiredJob pins each docs gate inside a job whose
// check is required. The site build began as a standalone `docs` job,
// which is not in master's required contexts (Lint, Test, Build (amd64),
// Build (arm64)) and which nothing in the graph depends on — so a PR
// breaking a relative link merged green, pages.yml then failed on
// master, and the site quietly stopped updating with the last good build
// still served.
//
// Every gate here needs the same pin, not just the first one: the lint
// was added beside the build with the same comment claiming the same
// protection, and could have been moved back out to an advisory job with
// nothing going red.
func TestDocsGates_RunInARequiredJob(t *testing.T) {
	body := readDocsPage(t, prWorkflow)

	for _, step := range requiredPRSteps {
		if !stepRunsInARequiredJob(t, body, step) {
			t.Errorf("no required job in pr.yml runs %q (required jobs: %v) — "+
				"a failure would not block a merge", step, requiredPRJobs)
		}
	}
}

func stepRunsInARequiredJob(t *testing.T, workflow, step string) bool {
	t.Helper()

	for _, job := range requiredPRJobs {
		if strings.Contains(workflowJob(t, workflow, job), step) {
			return true
		}
	}

	return false
}

// workflowJob returns one job's block from a workflow, by slicing from
// its two-space-indented key to the next one. Comment lines are dropped:
// a step search over raw YAML would otherwise match a comment that
// merely quotes the command, so deleting the step while leaving prose
// about it would pass vacuously.
func workflowJob(t *testing.T, workflow, job string) string {
	t.Helper()

	start := strings.Index(workflow, "\n  "+job+":\n")
	if start < 0 {
		t.Fatalf("pr.yml has no `%s` job", job)
	}

	rest := workflow[start+1:]

	if next := regexp.MustCompile(`(?m)^  [a-z][a-z0-9-]*:$`).FindStringIndex(rest[1:]); next != nil {
		rest = rest[:next[0]+1]
	}

	var steps []string

	for line := range strings.SplitSeq(rest, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			steps = append(steps, line)
		}
	}

	return strings.Join(steps, "\n")
}

var markdownlintPin = regexp.MustCompile(`markdownlint-cli2@(\d+\.\d+\.\d+)`)

// Every file that names a markdownlint-cli2 version. `npx markdownlint-cli2`
// without one resolves to whatever is latest at run time, and markdownlint
// adds rules in minor releases — so an unpinned invocation in a REQUIRED
// job turns red on an unrelated PR the day a new rule ships.
var markdownlintPinSites = []string{"../../.github/workflows/pr.yml", "../../CLAUDE.md"}

// TestMarkdownlintPin_AllSitesAgree pins the linter version across the
// workflow that gates on it and the doc that tells contributors how to
// run it, so the two cannot be different engines.
func TestMarkdownlintPin_AllSitesAgree(t *testing.T) {
	sitesByVersion := make(map[string][]string, 1)

	for _, site := range markdownlintPinSites {
		body := readDocsPage(t, site)

		if strings.Contains(body, "markdownlint-cli2 ") {
			t.Errorf("%s runs markdownlint-cli2 without pinning a version", site)
		}

		matches := markdownlintPin.FindAllStringSubmatch(body, -1)
		if matches == nil {
			t.Errorf("%s names no markdownlint-cli2 version", site)

			continue
		}

		for _, match := range matches {
			sitesByVersion[match[1]] = append(sitesByVersion[match[1]], site)
		}
	}

	if len(sitesByVersion) > 1 {
		t.Errorf("markdownlint-cli2 is pinned to %d different versions: %v", len(sitesByVersion), sitesByVersion)
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
	docsDir         = "../../docs"
)

// docsPages returns every published page plus the README. It walks rather
// than globs: a page in a subdirectory (docs/guides/foo.md) is a legal
// nav entry, and a glob would drop it silently — the scan would still
// find mentions elsewhere, so the vacuity guard would not notice.
func docsPages(t *testing.T) []string {
	t.Helper()

	var pages []string

	err := filepath.WalkDir(docsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() && strings.HasSuffix(path, ".md") {
			pages = append(pages, path)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", docsDir, err)
	}

	if len(pages) == 0 {
		t.Fatalf("no pages found under %s", docsDir)
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
// The page group takes `\w`, not a lowercase-only charset, for the same
// reason the anchor group does: a link with a capital (/Installation/)
// would otherwise match nothing and go unchecked, rather than failing
// honestly against a page that does not exist.
//
// The page group spans nested segments, because docsPages walks docs/
// recursively and a page at docs/guides/foo.md is a legal nav entry
// serving /guides/foo/. Matching one segment would read that as the page
// "guides" and fail a correct link against a docs/guides.md that never
// existed — the two halves of this test must agree on what a page is.
//
// The trailing slash is OPTIONAL, and that matters: requiring it made a
// slash-less link match nothing at all, so it was never checked. With
// use_directory_urls a page is served as <page>/index.html, so
// /ghost-page redirects to /ghost-page/ and 404s when the page does not
// exist — a dead link that the scan silently skipped. The segment still
// needs at least one character, so a bare domain link (the README's own
// `[mcp-tg.lexfrei.dev](https://mcp-tg.lexfrei.dev)`) stays unmatched.
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
var docsSiteURL = regexp.MustCompile(`https://mcp-tg\.lexfrei\.dev/((?:[\w-]+/)*[\w-]+)/?(?:#([\w-]+))?`)

var (
	docsHeading   = regexp.MustCompile(`(?m)^#{1,6} +(.+?)\s*$`)
	docsCodeFence = regexp.MustCompile("(?s)```.*?```")
	slugDrop      = regexp.MustCompile(`[^\w\s-]`)
	slugSeparator = regexp.MustCompile(`[-\s]+`)
)

// slugifyHeading mirrors python-markdown's toc slugify, which is what
// MkDocs actually anchors with — all three of its steps:
//
//	value = unicodedata.normalize('NFKD', value)
//	value = value.encode('ascii', 'ignore').decode('ascii')
//	value = re.sub(r'[^\w\s-]', '', value).strip().lower()
//	return re.sub(r'[-\s]+', '-', value)
//
// Each step is load-bearing and each was wrong at some point here.
//
// The NFKD fold runs FIRST and turns an accented letter into its ASCII
// base: `Café` slugs as `cafe`, not `caf`. Go's `\w` is ASCII-only, so
// dropping this step deletes the letter instead of folding it — which
// fails a correct link and passes a dead one, in both directions.
//
// `\w` keeps the underscore, so `tg_messages_list` anchors as itself
// rather than as `tg-messages-list`. In a repository where every
// identifier is snake_case, guessing otherwise is the same bug again.
//
// The strip happens BEFORE the separator collapse, so an em dash
// vanishes and leaves the spaces around it to collapse into one hyphen.
// That one is why `Recovery — revoked session` works.
func slugifyHeading(text string) string {
	folded := foldToASCII(text)
	stripped := strings.ToLower(strings.TrimSpace(slugDrop.ReplaceAllString(folded, "")))

	return slugSeparator.ReplaceAllString(stripped, "-")
}

// foldToASCII is python's NFKD + ascii-ignore: decompose, then drop
// everything non-ASCII, which leaves the base letter of an accented rune
// behind and removes runes that have no ASCII base at all.
func foldToASCII(text string) string {
	var out strings.Builder

	for _, r := range norm.NFKD.String(text) {
		if r < utf8.RuneSelf {
			out.WriteRune(r)
		}
	}

	return out.String()
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
		// An accented letter FOLDS to its ASCII base rather than being
		// dropped — python normalizes NFKD and encodes ascii/ignore
		// before the substitutions. Both verified against a real build.
		{"Café configuration", "cafe-configuration"},
		{"Naïve resolver", "naive-resolver"},
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

// skippedWalkDirs are the directories the URL walk must not descend
// into: other branches' checkouts and build output.
var skippedWalkDirs = map[string]bool{".git": true, ".claude": true, "site": true, "node_modules": true}

// TestDocsTransport_NamesTheImplementedOne pins the docs against the HTTP
// handler the server actually builds. SSE and Streamable HTTP are two
// distinct MCP transports in the SDK, not two names for one: HTTP+SSE is
// the superseded 2024-11-05 transport, and a client told to use it GETs
// an endpoint event that StreamableHTTPHandler answers with a rejection.
//
// The pages advertised "HTTP/SSE" in the protocol-support table — the
// exact place an integrator reads a transport off — while every worked
// example says `--transport http`. Whoever believed the table got a
// connect failure that names nothing.
func TestDocsTransport_NamesTheImplementedOne(t *testing.T) {
	if serverUsesHandler(t, "NewSSEHandler") {
		return // If the server ever serves SSE, the docs may say so.
	}

	for _, page := range docsPages(t) {
		if strings.Contains(readDocsPage(t, page), "SSE") {
			t.Errorf("%s advertises SSE, but the server builds a Streamable HTTP handler and "+
				"never calls mcp.NewSSEHandler — they are different transports", page)
		}
	}
}

// serverUsesHandler reports whether any non-test Go file in the tree
// calls the named SDK constructor.
func serverUsesHandler(t *testing.T, constructor string) bool {
	t.Helper()

	found := false

	err := filepath.WalkDir("../..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if skippedWalkDirs[entry.Name()] {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "read %s", path)
		}

		if strings.Contains(string(body), "mcp."+constructor+"(") {
			found = true
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk source: %v", err)
	}

	return found
}

// existingDocsPages is the set of real page paths, spelled as they are on
// disk. Walked once: the URL scan asks per link.
var existingDocsPages = func() func(*testing.T) map[string]bool {
	var pages map[string]bool

	return func(t *testing.T) map[string]bool {
		t.Helper()

		if pages != nil {
			return pages
		}

		pages = make(map[string]bool)

		err := filepath.WalkDir(docsDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.IsDir() && strings.HasSuffix(path, ".md") {
				pages[path] = true
			}

			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", docsDir, err)
		}

		return pages
	}
}()

// checkDocsSiteURLs asserts every published docs URL in one file points
// at a page that exists, and at a heading that exists when it carries an
// anchor. Returns how many it checked.
func checkDocsSiteURLs(t *testing.T, path, body string) int {
	t.Helper()

	checked := 0

	for _, match := range docsSiteURL.FindAllStringSubmatch(body, -1) {
		checked++

		page := "../../docs/" + match[1] + ".md"

		// Membership in the real page set, NOT os.Stat: macOS is
		// case-insensitive, so os.Stat("docs/Installation.md") happily
		// resolves docs/installation.md and a link that 404s on the
		// (case-sensitive) site passes on a maintainer's laptop while
		// failing on Linux CI. Comparing names keeps both honest.
		if !existingDocsPages(t)[page] {
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

	// No trailing slash. use_directory_urls serves a page as
	// <page>/index.html, so this redirects to /no-such-page/ and 404s —
	// it must be checked, not skipped for lacking the slash.
	t.Run("slashless dead page is rejected", func(t *testing.T) {
		if got := countURLErrors(t, docsSiteBase+"no-such-page"); got == 0 {
			t.Error("a slash-less link to a missing page passed the check")
		}
	})

	// The same shape, alive, must still pass.
	t.Run("slashless live page is accepted", func(t *testing.T) {
		if got := countURLErrors(t, docsSiteBase+"messages"); got != 0 {
			t.Errorf("a correct slash-less link reported %d errors", got)
		}
	})
}

// countURLErrors reports whether checkDocsSiteURLs rejected one link: 1
// if it raised any error, 0 if it accepted it. Not a count — the probe
// only records that it failed — but the callers only ask "did it bite?".
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

		// .claude/worktrees holds whole checkouts of this repository on
		// other branches, whose URLs are not this branch's problem and
		// whose pages may not exist here; site/ is build output. Skipping
		// every dot-directory would be simpler but would also skip
		// .github, whose issue and PR templates may link to the docs.
		if entry.IsDir() {
			if skippedWalkDirs[entry.Name()] {
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

// The prose label the protocol table uses for each middleware, keyed by
// the constructor that installs it.
var middlewareLabels = map[string]string{
	"NewAuthGuard":    "auth guard",
	"NewSessionGuard": "session guard",
	"NewLogging":      "request logging",
	"NewBoolCoercer":  "bool coercion",
}

// installedMiddlewareNames recovers the constructor name behind each
// installed middleware. Go names a closure after the function that
// returned it, so `NewSessionGuard`'s middleware reports
// "...receivingMiddlewares.NewSessionGuard.func4" — which is what makes
// the identities checkable rather than only the count.
func installedMiddlewareNames(t *testing.T) []string {
	t.Helper()

	installed := receivingMiddlewares(
		discardLogger(), tools.BoolFieldRegistry{}, make(chan struct{}), middleware.NewSessionHealth(),
	)

	names := make([]string, 0, len(installed))

	for _, mw := range installed {
		full := runtime.FuncForPC(reflect.ValueOf(mw).Pointer()).Name()

		match := regexp.MustCompile(`\.(New[A-Za-z]+)\.func\d+$`).FindStringSubmatch(full)
		if match == nil {
			t.Fatalf("cannot recover a constructor name from %q", full)
		}

		names = append(names, match[1])
	}

	return names
}

// TestDocsMiddlewareRow_NamesTheInstalledOnes pins the protocol table's
// middleware row against what receivingMiddlewares installs — by name,
// not merely by count. The row listed three while the server installs
// four: it predates the session guard, and nothing read it even though
// the numbers beside it in that table are pinned.
//
// Counting alone would still miss a rename, which is the same hole the
// resource and prompt check closes one test over.
func TestDocsMiddlewareRow_NamesTheInstalledOnes(t *testing.T) {
	row := docsMiddlewareRow.FindStringSubmatch(readDocsPage(t, docsIndexPage))
	if row == nil {
		t.Fatalf("%s no longer carries a Middleware row", docsIndexPage)
	}

	documented := strings.ToLower(row[1])
	installed := installedMiddlewareNames(t)

	for _, name := range installed {
		label, known := middlewareLabels[name]
		if !known {
			t.Errorf("the server installs %s, which the docs have no label for — "+
				"add one here and to the %s Middleware row", name, docsIndexPage)

			continue
		}

		if !strings.Contains(documented, label) {
			t.Errorf("%s does not list %q, installed by %s", docsIndexPage, label, name)
		}
	}

	if listed := strings.Split(row[1], ","); len(listed) != len(installed) {
		t.Errorf("%s lists %d middlewares (%q), the server installs %d",
			docsIndexPage, len(listed), row[1], len(installed))
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

// TestDocsMajorVersion_MatchesTheModulePath pins any promise the docs
// make about versioning against what the module can actually carry: a
// vN>=2 tag requires a matching /vN suffix in go.mod, or `go install
// ...@vN` fails while `@latest` silently keeps resolving to v1.
//
// It reads every published page, not just the README. The versioning
// rationale it guards moved to docs/messages.md with the parse-mode
// break, so a README-only check now watches a page that makes no such
// claim — and a future v2.0.0 promise would be written where the
// versioning discussion actually lives, out of its sight. Pin the claim
// where it lives, which is this branch's whole thesis.
func TestDocsMajorVersion_MatchesTheModulePath(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	hasSuffix := regexp.MustCompile(`(?m)^module .+/v[2-9]\d*$`).Match(gomod)
	promise := regexp.MustCompile(`v[2-9]\d*\.0\.0`)

	for _, page := range docsPages(t) {
		if promise.MatchString(readDocsPage(t, page)) && !hasSuffix {
			t.Errorf("%s promises a major version the module path cannot carry — "+
				"add the /vN suffix to go.mod or drop the promise", page)
		}
	}
}

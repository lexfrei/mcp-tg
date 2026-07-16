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
	docsSearchPage   = "../../docs/guides/search.md"
	docsMessagesPage = "../../docs/guides/messages.md"
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

var docsConfigPage = "../../docs/getting-started/configuration.md"

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

const (
	docsValidateWorkflow = "../../.github/workflows/docs-validate.yaml"
	docsDeployWorkflow   = "../../.github/workflows/docs.yaml"
)

// requiredDocsSteps are the gates the documentation PR check must run.
// Both see what no Go test can: the site build covers relative links and
// anchors inside docs/ (TestDocsSiteURLs_ResolveToPages reads absolute URLs
// only), and the Markdown lint covers what its config has always described.
var requiredDocsSteps = []string{"mkdocs build --strict", "markdownlint-cli2-action"}

// TestDocsGates_RunOnPullRequests pins both gates into the documentation
// validation workflow, and pins the paths that must trigger it: a filter
// that misses a file the build reads means the gate does not run for the
// change that breaks it.
//
// Note the limit of what a file can pin: whether this check is REQUIRED to
// merge is a branch-protection setting, not a file in the repository.
func TestDocsGates_RunOnPullRequests(t *testing.T) {
	body := readDocsPage(t, docsValidateWorkflow)

	for _, step := range requiredDocsSteps {
		if !strings.Contains(body, step) {
			t.Errorf("%s no longer runs %q — a broken docs build would reach master unseen",
				docsValidateWorkflow, step)
		}
	}

	for _, path := range []string{"docs/**", "mkdocs.yml", "requirements-docs.txt"} {
		if !strings.Contains(body, path) {
			t.Errorf("%s does not trigger on %q, so a change there skips the gate", docsValidateWorkflow, path)
		}
	}
}

var mkdocsQuietFlag = regexp.MustCompile(`mkdocs build[^\n]*\s(--quiet|-q)\b`)

// TestMkdocsBuild_IsNotQuiet pins the site build against --quiet, which
// silently disables --strict: --strict fails on the WARNING COUNT, and
// --quiet suppresses the warnings before they are counted. Verified on
// 9.7.6, same dead anchor either way:
//
//	mkdocs build --strict           -> exit 1
//	mkdocs build --strict --quiet   -> exit 0
//
// The temptation is concrete rather than theoretical: mkdocs-material
// prints a large red MkDocs 2.0 banner on every build, which is exactly
// the noise somebody reaches for --quiet to silence — and the gate would
// go on reporting success while checking nothing.
//
// TestDocsGates_RunOnPullRequests cannot see this: `mkdocs build --strict
// --quiet` still contains the string it looks for.
func TestMkdocsBuild_IsNotQuiet(t *testing.T) {
	for _, workflow := range []string{docsValidateWorkflow, docsDeployWorkflow} {
		if match := mkdocsQuietFlag.FindString(readDocsPage(t, workflow)); match != "" {
			t.Errorf("%s runs %q — --quiet suppresses the warnings --strict counts, "+
				"so the build reports success while catching nothing", workflow, match)
		}
	}
}

var actionUses = regexp.MustCompile(`(?m)^\s*uses:\s*([^\s@]+)@([^\s]+)(\s*#\s*(\S+))?`)

// TestDocsWorkflows_PinActionsBySHA pins every action the docs workflows use
// to a full commit SHA carrying a version comment, as the rest of this
// repository does. These workflows publish the site and run on master with
// `pages: write` and `id-token: write`, so a moving tag would let a retagged
// action execute unreviewed code with those permissions.
func TestDocsWorkflows_PinActionsBySHA(t *testing.T) {
	sha := regexp.MustCompile(`^[0-9a-f]{40}$`)
	checked := 0

	for _, workflow := range []string{docsValidateWorkflow, docsDeployWorkflow} {
		for _, use := range actionUses.FindAllStringSubmatch(readDocsPage(t, workflow), -1) {
			checked++

			action, ref, comment := use[1], use[2], use[4]

			if !sha.MatchString(ref) {
				t.Errorf("%s pins %s@%s, which is a moving ref — use a full commit SHA", workflow, action, ref)
			}

			if comment == "" {
				t.Errorf("%s pins %s by SHA with no version comment — nobody can tell what it is", workflow, action)
			}
		}
	}

	if checked == 0 {
		t.Error("no actions found in the docs workflows — this test has stopped checking anything")
	}
}

var (
	mkdocsMaterialPin = regexp.MustCompile(`mkdocs-material==(\d+\.\d+\.\d+)`)
	pipInstallDirect  = regexp.MustCompile(`pip install\s+(?:--upgrade\s+)?mkdocs`)
)

const requirementsDocs = "../../requirements-docs.txt"

// The docs that tell a human how to build the site. They must defer to
// requirements-docs.txt rather than restate a version: a second copy is a
// second thing to bump, which is the drift this whole file exists to stop.
var pipInstallSites = []string{"../../CLAUDE.md", readmePage}

// TestMkdocsMaterialPin_LivesInRequirements pins the version to exactly one
// place. MkDocs 2.0 removes the plugin system and the theming system with no
// migration path, so an unpinned install breaks this site on release day —
// and a version restated in a second file breaks it the day the two disagree,
// which is worse, because the build stays green while a contributor renders a
// different site than CI publishes.
func TestMkdocsMaterialPin_LivesInRequirements(t *testing.T) {
	requirements := readDocsPage(t, requirementsDocs)

	pinned := mkdocsMaterialPin.FindStringSubmatch(requirements)
	if pinned == nil {
		t.Fatalf("%s does not pin mkdocs-material to an exact version", requirementsDocs)
	}

	// Every dependency there, not just Material: an unpinned plugin drifts
	// the same way.
	for line := range strings.SplitSeq(requirements, "\n") {
		dep := strings.TrimSpace(line)
		if dep == "" || strings.HasPrefix(dep, "#") {
			continue
		}

		if !strings.Contains(dep, "==") {
			t.Errorf("%s lists %q without an exact pin", requirementsDocs, dep)
		}
	}

	for _, site := range pipInstallSites {
		body := readDocsPage(t, site)

		if match := pipInstallDirect.FindString(body); match != "" {
			t.Errorf("%s runs %q instead of `pip install --requirement %s` — "+
				"a second copy of the version is a second thing to bump",
				site, match, filepath.Base(requirementsDocs))
		}

		if restated := mkdocsMaterialPin.FindStringSubmatch(body); restated != nil && restated[1] != pinned[1] {
			t.Errorf("%s names mkdocs-material %s, %s pins %s",
				site, restated[1], requirementsDocs, pinned[1])
		}
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
	slugs := headingSlugs(t, "../../docs/getting-started/installation.md")

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
		// The word, not the substring: ADDRESSES is not a transport.
		if regexp.MustCompile(`\bSSE\b`).MatchString(readDocsPage(t, page)) {
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

		// use_directory_urls serves /a/b/ from either docs/a/b.md or
		// docs/a/b/index.md — a section landing is the latter, so both
		// spellings must be tried before calling a link dead.
		page := "../../docs/" + match[1] + ".md"
		index := "../../docs/" + match[1] + "/index.md"

		// Membership in the real page set, NOT os.Stat: macOS is
		// case-insensitive, so os.Stat("docs/Installation.md") happily
		// resolves docs/installation.md and a link that 404s on the
		// (case-sensitive) site passes on a maintainer's laptop while
		// failing on Linux CI. Comparing names keeps both honest.
		pages := existingDocsPages(t)

		switch {
		case pages[page]:
		case pages[index]:
			page = index
		default:
			t.Errorf("%s links to %s, but neither %s nor %s exists", path, match[0], page, index)

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
		return docsSiteBase + "guides/messages/" + "#" + anchor
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
		if got := countURLErrors(t, docsSiteBase+"guides/messages"); got != 0 {
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
	docsResourcesPage = "../../docs/guides/resources.md"
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

	// A break note must name its version. The site is not versioned — it
	// always serves master — so "in this release" means whatever the
	// reader loads it in. The README at least had `git show v1.0:README.md`
	// as an escape; a page has none. The output-format note proves the
	// decay: it said "this release" for six releases running.
	if drift := regexp.MustCompile(`\*\*Breaking change[^*]*\*\*`).FindAllString(body, -1); drift != nil {
		for _, note := range drift {
			if !regexp.MustCompile(`v\d+\.\d+\.\d+`).MatchString(note) {
				t.Errorf("%s carries %q, which names no version — on an unversioned site "+
					"'this release' means whenever the page is read", docsMessagesPage, note)
			}
		}
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

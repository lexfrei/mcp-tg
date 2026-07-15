package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lexfrei/mcp-tg/internal/tools"
)

func TestVersionRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"bare binary", []string{"mcp-tg"}, false},
		{"version flag", []string{"mcp-tg", "--version"}, true},
		{"version among others", []string{"mcp-tg", "login", "--version"}, true},
		{"login only", []string{"mcp-tg", "login"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionRequested(tc.args); got != tc.want {
				t.Errorf("versionRequested(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

// TestVersionLine_MatchesServerVersionTool pins the cross-package contract: the
// `--version` CLI line and the tg_server_version tool's Output must be identical
// for the same build strings. Both now delegate to tools.FormatVersionLine, so
// this cannot drift — the test fails loudly if a future change re-splits them.
func TestVersionLine_MatchesServerVersionTool(t *testing.T) {
	_, result, err := tools.NewServerVersionHandler(version, revision, runtime.Version())(
		context.Background(), nil, tools.ServerVersionParams{})
	if err != nil {
		t.Fatalf("server version handler: %v", err)
	}

	if got := versionLine(); got != result.Output {
		t.Errorf("--version line %q must equal tg_server_version Output %q", got, result.Output)
	}
}

func TestLogStartupVersion_LogsBuildAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logStartupVersion(logger)

	out := buf.String()
	if !strings.Contains(out, "starting mcp-tg") {
		t.Errorf("expected the startup line, got: %s", out)
	}

	if !strings.Contains(out, "level=INFO") {
		t.Errorf("startup line must be INFO level, got: %s", out)
	}

	if !strings.Contains(out, "version=") {
		t.Errorf("startup line must name the version, got: %s", out)
	}
}

// versionHelperEnv gates the subprocess re-entry: when set the child runs
// main() so the real --version dispatch path (os.Args scan → runVersion → exit
// 0) is exercised end to end, not just the helper in isolation.
const versionHelperEnv = "TEST_VERSION_HELPER"

// TestVersionFlag_PrintsBuildAndExitsZero drives the whole `mcp-tg --version`
// path through main() via the subprocess re-entry pattern. The `--` separator
// terminates the test binary's own flag parsing so `--version` lands in os.Args
// as a positional the dispatch scan still sees. Credentials are cleared so a
// regressed dispatch (falling through to run()) would exit non-zero on config —
// making a green result meaningful rather than vacuous.
func TestVersionFlag_PrintsBuildAndExitsZero(t *testing.T) {
	if os.Getenv(versionHelperEnv) == "1" {
		main()

		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=TestVersionFlag_PrintsBuildAndExitsZero", "--", "--version")
	cmd.Env = append(os.Environ(),
		versionHelperEnv+"=1",
		"TELEGRAM_APP_ID=",
		"TELEGRAM_APP_HASH=",
	)

	// Capture the streams separately so the assertion pins the build line to
	// STDOUT specifically — a switch of runVersion to stderr must fail this test,
	// which a merged CombinedOutput would silently tolerate.
	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if ctx.Err() != nil {
		t.Fatalf("--version did not terminate: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}

	if runErr != nil {
		t.Fatalf("--version must exit 0, got err=%v stdout=%s stderr=%s", runErr, stdout.String(), stderr.String())
	}

	if !strings.Contains(stdout.String(), "mcp-tg ") {
		t.Errorf("--version must print the build line to stdout, got stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

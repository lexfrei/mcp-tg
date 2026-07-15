package main

import (
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/lexfrei/mcp-tg/internal/tools"
)

// versionFlag prints build metadata and exits without starting the server.
const versionFlag = "--version"

// versionRequested reports whether `--version` appears on the command line. A
// plain os.Args scan, mirroring loginRequested — no flag package, so the check
// stays valid before any other argument parsing runs.
func versionRequested(args []string) bool {
	return slices.Contains(args, versionFlag)
}

// versionLine is the one-line build identity printed by --version. It delegates
// to tools.FormatVersionLine — the same function the tg_server_version tool
// uses — so the CLI check and the running server are byte-identical by
// construction, not by two copies of the format that could silently drift.
func versionLine() string {
	return tools.FormatVersionLine(version, revision)
}

// runVersion prints the build metadata to stdout; main returns and exits 0.
func runVersion() {
	fmt.Fprintln(os.Stdout, versionLine())
}

// logStartupVersion emits one INFO line naming the running build so stderr
// always records which binary is serving. The on-disk file can be rebuilt
// while an older process keeps running the previous code — this line is how a
// post-mortem tells the two apart.
func logStartupVersion(logger *slog.Logger) {
	logger.Info("starting mcp-tg", "version", version, "revision", shortRevision(revision))
}

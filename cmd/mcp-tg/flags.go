package main

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

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

// logLevelFlag selects the stderr log verbosity; it overrides MCP_LOG_LEVEL.
const logLevelFlag = "--log-level"

// Level names accepted by --log-level and MCP_LOG_LEVEL.
const (
	logLevelDebug   = "debug"
	logLevelInfo    = "info"
	logLevelWarn    = "warn"
	logLevelWarning = "warning"
	logLevelError   = "error"
)

// ErrInvalidLogLevel is returned when --log-level or MCP_LOG_LEVEL is not one
// of the recognised slog levels.
var ErrInvalidLogLevel = errors.New("log level must be one of: debug, info, warn, error")

// resolveLogLevel picks the slog level from the --log-level flag, then the
// MCP_LOG_LEVEL env value, then defaults to info. The flag wins over the env so
// a one-off invocation can override a daemon's configured default.
func resolveLogLevel(args []string, env string) (slog.Level, error) {
	if raw, ok := flagValue(args, logLevelFlag); ok {
		return parseLogLevel(raw)
	}

	if env != "" {
		return parseLogLevel(env)
	}

	return slog.LevelInfo, nil
}

// parseLogLevel maps a level name (case-insensitive) to a slog.Level. An
// unrecognised name is a hard error rather than a silent fallback — a typo in
// the level should fail loudly, not quietly serve at info.
func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case logLevelDebug:
		return slog.LevelDebug, nil
	case logLevelInfo:
		return slog.LevelInfo, nil
	case logLevelWarn, logLevelWarning:
		return slog.LevelWarn, nil
	case logLevelError:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, ErrInvalidLogLevel
	}
}

// flagValue returns the value of a `--flag value` or `--flag=value` pair, and
// whether the flag was present. A plain os.Args scan, matching the file's other
// flag helpers — no flag package, so it stays valid before subcommand dispatch.
// A present-but-dangling flag (last argument, or `--flag=` with no value) is
// reported present with an empty value, so the caller rejects the malformed CLI
// loudly instead of silently falling back to the env or the default.
func flagValue(args []string, flag string) (string, bool) {
	prefix := flag + "="
	for i, arg := range args {
		if val, found := strings.CutPrefix(arg, prefix); found {
			return val, true
		}

		if arg == flag {
			if i+1 < len(args) {
				return args[i+1], true
			}

			return "", true
		}
	}

	return "", false
}

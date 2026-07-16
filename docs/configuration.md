# Configuration

## Environment variables

| Variable | Description | Default | Required |
| --- | --- | --- | --- |
| `TELEGRAM_APP_ID` | API app_id from my.telegram.org | — | Yes |
| `TELEGRAM_APP_HASH` | API app_hash from my.telegram.org | — | Yes |
| `TELEGRAM_PHONE` | Phone number (E.164 format) | — | No (prompted via elicitation) |
| `TELEGRAM_PASSWORD` | 2FA password | — | No (prompted via elicitation) |
| `TELEGRAM_SESSION_FILE` | Session location: keychain account key by default, file path with insecure storage | `~/.mcp-tg/session.json` | No |
| `TELEGRAM_SESSION_INSECURE` | Store the session in a plaintext file instead of the OS keychain | `false` | No |
| `TELEGRAM_AUTH_CODE` | One-time auth code | — | No (prompted via elicitation) |
| `TELEGRAM_DOWNLOAD_DIR` | Media download directory | `mcp-tg/downloads` under the OS temp dir (see below) | No |
| `MCP_HTTP_PORT` | HTTP/SSE transport port | disabled | No |
| `MCP_HTTP_HOST` | HTTP bind address | `127.0.0.1` | No |
| `MCP_HTTP_ONLY` | Run as a headless HTTP-only daemon (no stdio transport) | `false` | No (requires `MCP_HTTP_PORT`) |
| `MCP_LOG_LEVEL` | stderr log verbosity: `debug`, `info`, `warn`, `error` (the `--log-level` flag overrides it) | `info` | No |

The download directory's default has no single spelling: the code derives it from Go's `os.TempDir()`, which returns `$TMPDIR` when that is set and falls back to `/tmp` on Unix otherwise. macOS sets `$TMPDIR` for every login session, so the default there lands under `/var/folders/...`; a plain container and Linux CI runners do not set it, so the container image resolves the default to `/tmp/mcp-tg/downloads`. Set `TELEGRAM_DOWNLOAD_DIR` explicitly if you need to know where files land without checking the environment first.

## Command-line flags

- `--version` — print the build metadata (`mcp-tg <version> (<short-sha>)`) to stdout and exit, without starting the server or touching Telegram. Use it to confirm which build a binary is, independent of any running process. The same metadata is available at runtime through the `tg_server_version` tool.
- `--log-level <level>` — set the stderr log verbosity to `debug`, `info` (default), `warn`, or `error`. Overrides `MCP_LOG_LEVEL`. Both the flag and the env var accept `warning` as an alias for `warn`. FLOOD_WAIT retries log at `warn`, transport and tool-call failures at `error`, so a default `info` daemon already records the events that matter for a post-mortem; drop to `debug` for the full gotd connection and per-request trace. This governs the running server only — the `login` subcommand reads its credentials from the TTY and does not honour `--log-level`.
- `--insecure-storage` — `mcp-tg login` only: write the session to a plaintext file instead of the OS keychain. `TELEGRAM_SESSION_INSECURE=true` does the same for both `login` and the server, so setting it alone covers both sides; the flag is the per-invocation form. Login and server must agree — see [Session storage](authentication.md#session-storage).

The server also logs one INFO line at startup naming the build (`starting mcp-tg version=… revision=…`), so a daemon at the default `info` (or `debug`) records which binary is serving — handy when the on-disk file was rebuilt while an older process keeps running the previous code. At `warn`/`error` that INFO line is filtered like any other record; `--version` and the `tg_server_version` tool report the build at any log level.

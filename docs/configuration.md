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
| `TELEGRAM_DOWNLOAD_DIR` | Media download directory | `/tmp/mcp-tg/downloads` | No |
| `MCP_HTTP_PORT` | HTTP/SSE transport port | disabled | No |
| `MCP_HTTP_HOST` | HTTP bind address | `127.0.0.1` | No |
| `MCP_HTTP_ONLY` | Run as a headless HTTP-only daemon (no stdio transport) | `false` | No (requires `MCP_HTTP_PORT`) |
| `MCP_LOG_LEVEL` | stderr log verbosity: `debug`, `info`, `warn`, `error` (the `--log-level` flag overrides it) | `info` | No |

## Command-line flags

- `--version` — print the build metadata (`mcp-tg <version> (<short-sha>)`) to stdout and exit, without starting the server or touching Telegram. Use it to confirm which build a binary is, independent of any running process. The same metadata is available at runtime through the `tg_server_version` tool.
- `--log-level <level>` — set the stderr log verbosity to `debug`, `info` (default), `warn`, or `error`. Overrides `MCP_LOG_LEVEL`. FLOOD_WAIT retries log at `warn`, transport and tool-call failures at `error`, so a default `info` daemon already records the events that matter for a post-mortem; drop to `debug` for the full gotd connection and per-request trace. This governs the running server only — the `login` subcommand reads its credentials from the TTY and does not honour `--log-level`.

The server also logs one INFO line at startup naming the build (`starting mcp-tg version=… revision=…`), so a daemon at the default `info` (or `debug`) records which binary is serving — handy when the on-disk file was rebuilt while an older process keeps running the previous code. At `warn`/`error` that INFO line is filtered like any other record; `--version` and the `tg_server_version` tool report the build at any log level.

# Configuration

## Environment variables

| Variable | Description | Default | Required |
| --- | --- | --- | --- |
| `TELEGRAM_APP_ID` | API app_id from my.telegram.org | — | Yes |
| `TELEGRAM_APP_HASH` | API app_hash from my.telegram.org | — | Yes |
| `TELEGRAM_PHONE` | Phone number (E.164 format) | — | No (prompted in the client on the first tool call) |
| `TELEGRAM_PASSWORD` | 2FA password | — | No (prompted in the client on the first tool call) |
| `TELEGRAM_SESSION_FILE` | Session location: keychain account key by default, file path with insecure storage | `~/.mcp-tg/session.json` | No |
| `TELEGRAM_SESSION_INSECURE` | Store the session in a plaintext file instead of the OS keychain | `false` | No |
| `TELEGRAM_AUTH_CODE` | One-time auth code | — | No (prompted in the client on the first tool call) |
| `TELEGRAM_DOWNLOAD_DIR` | Media download directory | `mcp-tg/downloads` under the OS temp dir (see below) | No |
| `TELEGRAM_FILE_ROOTS` | Directory allowlist for tools that read or write local files | unset (no restriction) | No |
| `TELEGRAM_ALLOWED_PEERS` | Comma-separated list of the only peers the server may address (see [Access control](#access-control)) | unset (no restriction) | No |
| `TELEGRAM_READ_ONLY` | Register only the read-only tools | `false` | No |
| `TELEGRAM_TOOLS` | Comma-separated list of the only tools to register | unset (all tools) | No |
| `TELEGRAM_DISABLED_TOOLS` | Comma-separated list of tools never to register | unset | No |
| `MCP_HTTP_PORT` | Streamable HTTP transport port | disabled | No |
| `MCP_HTTP_HOST` | HTTP bind address | `127.0.0.1` | No |
| `MCP_HTTP_ONLY` | Run as a headless HTTP-only daemon (no stdio transport) | `false` | No (requires `MCP_HTTP_PORT`) |
| `MCP_LOG_LEVEL` | stderr log verbosity: `debug`, `info`, `warn`, `error` (the `--log-level` flag overrides it) | `info` | No |

The download directory's default has no single spelling: the code derives it from Go's `os.TempDir()`, which returns `$TMPDIR` when that is set and falls back to `/tmp` on Unix otherwise. macOS sets `$TMPDIR` for every login session, so the default there lands under `/var/folders/...`; a plain container and Linux CI runners do not set it, so the container image resolves the default to `/tmp/mcp-tg/downloads`. Set `TELEGRAM_DOWNLOAD_DIR` explicitly if you need to know where files land without checking the environment first.

`TELEGRAM_FILE_ROOTS` is a list of absolute directories joined with the OS path-list separator (`:` on Unix, `;` on Windows), the `PATH` convention. When set, every file path a tool reads (uploads, photos) or writes (downloads) must sit under one of the listed directories; a path outside them is rejected before any network call. Symlinks are resolved on both sides before the check, so a link under a root cannot smuggle in the tree it points to, and a root that is itself a symlink (macOS `/tmp`) admits its resolved tree. A path spelled with a `..` component is rejected outright: mixed with symlinks it cannot be judged safely, and every in-root path has a `..`-free spelling. A relative entry fails configuration loading outright, because a daemon's working directory is an accident of how it was launched; so does a set value that parses to no directories at all (unexpanded shell templating), since the operator asked for a boundary and running without one would fail open. When unset, paths are not restricted. This replaced the MCP roots capability, which the protocol deprecated (SEP-2577) and which only ever protected sessions whose client declared roots; the allowlist holds for every client of a shared daemon. If you set it, make sure `TELEGRAM_DOWNLOAD_DIR` points inside one of the roots, or downloads will be rejected.

## Access control

The server acts as your own account, so by default an agent can read and write every chat you can. Four variables narrow that down. They combine: a tool is offered only if every one of them allows it.

`TELEGRAM_ALLOWED_PEERS` is a comma-separated list of peers in any form a `peer` argument accepts — `@username`, a `t.me/` link, a bot-API style numeric ID. With it set, every peer the server addresses must be on the list: the chat a tool reads or writes, and equally a `from` sender filter, a `sendAs` identity, a user added to a group, a forward's destination. Resources, prompts, subscriptions and autocompletion go through the same check. Listings that return peers you never named (`tg_dialogs_list`, `tg_dialogs_search`, `tg_groups_list`, `tg_users_get_common_chats` and the `tg://dialogs` resource) show only the listed ones. The tools that can only act on the whole account are not offered at all: `tg_messages_search_global`, `tg_messages_clear_all_drafts` and the four `tg_folders_*` tools — a folder edit replaces the folder's whole chat list, including chats the list hides. A numeric entry is checked without a network call; any other entry is resolved on the first tool call after login, and one that fails to resolve fails that call rather than being dropped, so a typo cannot quietly shrink the list. To include Saved Messages, list your own numeric user ID.

`TELEGRAM_READ_ONLY=true` registers only the tools annotated read-only (the read-only row of the [annotation table](../tools.md#annotations)). Nothing that sends, edits, deletes or changes settings is offered.

`TELEGRAM_TOOLS` registers only the tools it names; `TELEGRAM_DISABLED_TOOLS` never registers the tools it names, and wins when a tool is on both lists. A name that matches no tool fails startup instead of being ignored — a misspelt entry in `TELEGRAM_DISABLED_TOOLS` would otherwise leave the tool it meant to disable enabled. `tg_server_version` is offered whatever these lists say: it never touches Telegram and is how you check which build is answering.

A tool that is not offered is not registered at all, so a client can neither list nor call it. A list that is set but holds no entries (only commas and blanks) fails configuration loading, for the same reason as `TELEGRAM_FILE_ROOTS`.

## Command-line flags

- `--version` — print the build metadata (`mcp-tg <version> (<short-sha>)`) to stdout and exit, without starting the server or touching Telegram. Use it to confirm which build a binary is, independent of any running process. The same metadata is available at runtime through the `tg_server_version` tool.
- `--log-level <level>` — set the stderr log verbosity to `debug`, `info` (default), `warn`, or `error`. Overrides `MCP_LOG_LEVEL`. Both the flag and the env var accept `warning` as an alias for `warn`. FLOOD_WAIT and Telegram server-error retries log at `warn`, transport and tool-call failures at `error`, so a default `info` daemon already records the events that matter for a post-mortem; drop to `debug` for the full gotd connection and per-request trace. This governs the running server only — the `login` subcommand reads its credentials from the TTY and does not honour `--log-level`.
- `--insecure-storage` — `mcp-tg login` only: write the session to a plaintext file instead of the OS keychain. `TELEGRAM_SESSION_INSECURE=true` does the same for both `login` and the server, so setting it alone covers both sides; the flag is the per-invocation form. Login and server must agree — see [Session storage](authentication.md#session-storage).

The server also logs one INFO line at startup naming the build (`starting mcp-tg version=… revision=…`), so a daemon at the default `info` (or `debug`) records which binary is serving — handy when the on-disk file was rebuilt while an older process keeps running the previous code. At `warn`/`error` that INFO line is filtered like any other record; `--version` and the `tg_server_version` tool report the build at any log level.

# mcp-tg

MCP server for Telegram Client API (MTProto). Provides 78 tools, 4 resources, 3 prompts, and argument completions for comprehensive Telegram account management.

Uses [gotd/td](https://github.com/gotd/td) for MTProto protocol — this is a **user account** client, not a bot.

📖 **Full documentation: [mcp-tg.lexfrei.dev](https://mcp-tg.lexfrei.dev)**

## Install

Homebrew (macOS, Linux) — also installs a `brew services` unit for the shared HTTP daemon:

```bash
brew install lexfrei/tap/mcp-tg
```

Container:

```bash
docker pull ghcr.io/lexfrei/mcp-tg:latest
```

Binary: `darwin`, `linux` and `windows` archives for `amd64` and `arm64` are on the [releases page](https://github.com/lexfrei/mcp-tg/releases), each covered by a keyless cosign signature over `checksums.txt`.

Details for every path: [Installation](https://mcp-tg.lexfrei.dev/getting-started/installation/).

## Quickstart

Get an app id and hash from [my.telegram.org](https://my.telegram.org) — the public builds carry no credentials — then register the server:

```bash
claude mcp add mcp-tg --env TELEGRAM_APP_ID=12345 --env TELEGRAM_APP_HASH=your_app_hash -- mcp-tg
```

On the first tool call the server asks for the phone and login code right in the client, and the session lands in the OS keychain. To keep the 2FA password out of the MCP client, run `mcp-tg login` in a terminal once instead — see [Authentication](https://mcp-tg.lexfrei.dev/getting-started/authentication/).

Running many agent sessions or several MCP clients? A server process per session adds up — run one shared HTTP daemon instead: `brew services start mcp-tg`, then `claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user`. See [Transport modes](https://mcp-tg.lexfrei.dev/building/#transport-modes).

Or skip the steps entirely: tell your agent to read [Agent Setup](https://mcp-tg.lexfrei.dev/getting-started/agent-setup/) and it does the rest.

## Documentation

Everything lives at **[mcp-tg.lexfrei.dev](https://mcp-tg.lexfrei.dev)**:

- [Tools](https://mcp-tg.lexfrei.dev/tools/) — the full 78-tool reference
- [Configuration](https://mcp-tg.lexfrei.dev/getting-started/configuration/) — environment variables and flags
- [Authentication](https://mcp-tg.lexfrei.dev/getting-started/authentication/) — login, session storage, revoked-session recovery
- [Messages](https://mcp-tg.lexfrei.dev/guides/messages/) — output format, `parseMode`, markdown limitations
- [Peers](https://mcp-tg.lexfrei.dev/guides/peers/) — identifier format and resolution
- [Search](https://mcp-tg.lexfrei.dev/guides/search/) — server-side filters and pagination
- [Resources and prompts](https://mcp-tg.lexfrei.dev/guides/resources/) — including chat subscriptions
- [Posting as a channel](https://mcp-tg.lexfrei.dev/guides/send-as/) and [Reactions](https://mcp-tg.lexfrei.dev/guides/reactions/)
- [Building](https://mcp-tg.lexfrei.dev/building/) — requirements, building from source, transport modes

## Contributing

The site is built from `docs/` with [MkDocs Material](https://squidfunk.github.io/mkdocs-material/) and deploys on every push to `master`. Dependencies are locked in `requirements-docs.txt` (pinned and hashed; edit `requirements-docs.in` and recompile). To preview locally:

```bash
pip install --requirement requirements-docs.txt
mkdocs serve
```

Parts of the docs are pinned against the code by `cmd/mcp-tg/docs_contract_test.go` — the tool list, the annotation census, the search filter values and the `parseMode` contract fail the test suite when they drift from what the server actually registers. Keep the heading and bullet shapes when editing those pages.

## License

BSD 3-Clause License

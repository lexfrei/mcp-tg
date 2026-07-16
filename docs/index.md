# mcp-tg

MCP server for Telegram Client API (MTProto). Provides 78 tools, 4 resources, 3 prompts, and argument completions for comprehensive Telegram account management.

Uses [gotd/td](https://github.com/gotd/td) for MTProto protocol — this is a **user account** client, not a bot.

## Start here

- [Installation](installation.md) — Homebrew, container, or a release binary
- [Authentication](authentication.md) — the `mcp-tg login` flow and where the session is stored
- [Configuration](configuration.md) — environment variables and command-line flags
- [Tools](tools.md) — the full tool reference

## MCP Protocol Support

| Feature | Status |
| --- | --- |
| Tools | 78 tools with annotations (read-only / idempotent / write / destructive) |
| Resources | 4 (dialogs, profile, chat info, chat messages) |
| Prompts | 3 (reply, summarize, search and reply) |
| Completions | Peer argument autocompletion from dialogs |
| Elicitation | Auth flow (phone, code, 2FA password) |
| Progress | File uploads, media albums, message search |
| Roots | File path validation for uploads/downloads |
| Subscriptions | `resources/updated` on new messages in a subscribed chat |
| Transports | stdio + HTTP/SSE |
| KeepAlive | 30s ping interval |
| Middleware | Auth guard, session guard, request logging, bool coercion |

## Telegram Protocol Features

- **Peer cache** — resolved peers with access hashes are cached in memory, so numeric ID lookups reuse valid hashes instead of failing
- **Invite links** — `t.me/+hash` and `t.me/joinchat/hash` are resolved via `messages.checkChatInvite`
- **FLOOD_WAIT retry** — automatic sleep and retry (up to 3 times) when Telegram rate-limits the client
- **Connection re-init** — when the server forgets a long-lived connection's `initConnection` state and answers `CONNECTION_LAYER_INVALID` / `CONNECTION_NOT_INITED`, the request is retried once wrapped in `initConnection`, recovering the connection in place
- **Auth guard** — tool calls are blocked with a clear error until Telegram authentication completes
- **Pagination** — `offsetDate` for dialog listing, `offsetId` for message search and history; `tg_messages_list` can additionally filter by message `type`; `tg_messages_search_global` pages through a compound cursor (`offsetRate` + `offsetId` + `offsetPeer`)

## Guides

- [Messages](messages.md) — output format, `parseMode`, markdown limitations
- [Peers](peers.md) — identifier shape and resolution
- [Search](search.md) — server-side filters and cursor pagination
- [Resources and prompts](resources.md) — including chat subscriptions
- [Posting as a channel](send-as.md) — the `sendAs` identity
- [Reactions](reactions.md) — standard and custom-emoji encoding
- [Building](building.md) — requirements, building from source, transport modes

## License

BSD 3-Clause License.

# mcp-tg

MCP server for Telegram Client API (MTProto). Provides 78 tools, 4 resources, 3 prompts, and argument completions for comprehensive Telegram account management.

Uses [gotd/td](https://github.com/gotd/td) for MTProto protocol — this is a **user account** client, not a bot.

## Start here

- [Installation](getting-started/installation.md) — Homebrew, container, or a release binary
- [Authentication](getting-started/authentication.md) — the `mcp-tg login` flow and where the session is stored
- [Configuration](getting-started/configuration.md) — environment variables and command-line flags
- [Tools](tools.md) — the full tool reference

## MCP Protocol Support

| Feature | Status |
| --- | --- |
| Tools | 78 tools with annotations (read-only / idempotent / write / destructive) |
| Resources | 4 (dialogs, profile, chat info, chat messages) |
| Prompts | 3 (reply, summarize, search and reply) |
| Completions | Peer argument autocompletion from dialogs |
| Elicitation | Login prompts (phone, code, 2FA password), by elicitation or input requests depending on the client's protocol revision |
| Progress | File uploads, media albums, message search |
| Subscriptions | `resources/updated` on new messages in a subscribed chat |
| Transports | stdio + Streamable HTTP |
| KeepAlive | 30s ping interval |
| Middleware | Auth guard, session guard, request logging, bool coercion |

## Telegram Protocol Features

- **Peer cache** — resolved peers with access hashes are cached in memory, so numeric ID lookups reuse valid hashes instead of failing
- **Invite links** — `t.me/+hash` and `t.me/joinchat/hash` are resolved via `messages.checkChatInvite`
- **FLOOD_WAIT retry** — when Telegram rate-limits the client, the call sleeps for the server-specified delay and is retried, up to 3 attempts in total (so two retries); each retry logs one WARN carrying `retryAfter`
- **Server-error retry** — a 500-class internal failure on Telegram's side (`INTERDC_X_CALL_ERROR`, `RPC_CALL_FAIL`, `WORKER_BUSY_TOO_LONG_RETRY`) is resent with an exponential backoff, up to 4 attempts in total; each retry logs one WARN carrying the raw error. A failure that survives all four surfaces as a readable retry hint rather than a bare rpc code. Some things are deliberately not resent: `RANDOM_ID_DUPLICATE`, the one 500 that means the opposite (Telegram refusing a send it already accepted, reported as such so a retry does not send the message twice), and requests that create something the account keeps without a token the server could deduplicate against — creating a chat or a folder, setting a profile photo, exporting an invite link. Those come back asking you to check whether the call took effect, rather than to repeat it
- **Connection re-init** — when the server forgets a long-lived connection's `initConnection` state and answers `CONNECTION_LAYER_INVALID` / `CONNECTION_NOT_INITED`, the request is retried once wrapped in `initConnection`, recovering the connection in place
- **Auth guard** — resource reads and prompts are blocked with a clear error until the Telegram login completes; tool calls are not, since the login runs inside the first one that needs an account
- **Pagination** — `offsetDate` for dialog listing, `offsetId` for message search and history; `tg_messages_list` can additionally filter by message `type`; `tg_messages_search_global` pages through a compound cursor (`offsetRate` + `offsetId` + `offsetPeer`)

## Guides

- [Messages](guides/messages.md) — output format, `parseMode`, markdown limitations
- [Peers](guides/peers.md) — identifier shape and resolution
- [Search](guides/search.md) — server-side filters and cursor pagination
- [Resources and prompts](guides/resources.md) — including chat subscriptions
- [Posting as a channel](guides/send-as.md) — the `sendAs` identity
- [Reactions](guides/reactions.md) — standard and custom-emoji encoding
- [Building](building.md) — requirements, building from source, transport modes

## License

BSD 3-Clause License.

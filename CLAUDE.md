# CLAUDE.md — Development Guide for mcp-tg

## What is this

MCP server for Telegram Client API (MTProto, not Bot API). Uses gotd/td for protocol, exposes 75 tools + resources + prompts via MCP.

## Build & Test

```bash
go build ./cmd/mcp-tg
go test -race ./...
golangci-lint run
```

## Architecture

```text
cmd/mcp-tg/main.go          Entry point: telegram client → MCP server → transports
internal/config/             Env var loading and validation
internal/telegram/           Telegram abstraction layer
  types.go                   Domain types (Message, User, Dialog, etc.)
  client.go                  Client interface (13 sub-interfaces)
  wrapper.go                 gotd/td implementation of Client
  wrapper_helpers.go         Helper functions for wrapper (extractors, converters)
  convert.go                 tg types → domain types conversion
  auth.go                    Auth flow with MCP elicitation support
  resolve.go                 Peer resolution (@username, numeric ID, t.me/ URLs, invite links)
  peer_cache.go              Thread-safe cache for peer access hashes
internal/tools/              MCP tool handlers (75 tools)
  annotations.go             Tool annotation helpers (readOnly, idempotent, write, destructive)
  errors.go                  Error sentinels
  helpers.go                 Shared helpers (deref, formatPeer, formatUserName)
  format.go                  Output formatting (timestamps, messages, dialogs)
  roots.go                   File path validation against client roots
  progress.go                Progress notification helper
  result_types.go            Structured JSON result types (DialogItem, MessageItem, etc.)
  register.go                tools.AddTool wrapper that records bool fields into the coercer registry
  mock_test.go               Mock telegram.Client for tests
cmd/mcp-tg/flood_wait.go    FLOOD_WAIT auto-retry middleware for gotd/td
internal/telegram/
  markdown.go                Markdown → Telegram entities parser (entry point)
  markdown_inline.go         Inline marker parsing (bold, italic, code, links, etc.)
  markdown_convert.go        rawEntity → tg.MessageEntityClass conversion + escape removal
internal/resources/          MCP resources (4 resources)
internal/prompts/            MCP prompts (3 prompts)
internal/completions/        Argument autocompletion
internal/middleware/         Auth guard, request logging, and bool-coercion middleware
internal/testutil/           NoopClient for registration tests
```

## Key Patterns

### Adding a new tool

1. Create `internal/tools/xxx.go` with:
   - `XxxParams` struct (jsonschema tags: `jsonschema:"Description"`)
   - `XxxResult` struct
   - `NewXxxHandler(client telegram.Client) mcp.ToolHandlerFor[XxxParams, XxxResult]`
   - `XxxTool() *mcp.Tool` with appropriate `Annotations`
2. Create `internal/tools/xxx_test.go` (TDD: tests first)
3. Register in `cmd/mcp-tg/main.go` `registerTools()` via `tools.AddTool(server, registry, ...)` — NOT `mcp.AddTool`. The wrapper reflects over the Params struct, records every `bool`/`*bool` field into the registry consumed by the bool-coercion middleware, and then delegates to the SDK. Bypassing it silently disables coercion for the new tool's bool params.
4. Add mock method in `internal/tools/mock_test.go`
5. Add noop method in `internal/testutil/noop_client.go`

### Tool annotations

- `readOnlyAnnotations()` — tools that only read data (29 tools)
- `idempotentAnnotations()` — tools that modify state but are safe to retry (27 tools)
- `writeAnnotations()` — tools that create new entities, not idempotent (9 tools)
- `destructiveAnnotations()` — tools that delete/remove things (9 tools)

### Peer resolution

All tools accept `peer` as string. Supported formats:
- `@username`
- `username` (bare)
- `https://t.me/username`
- `https://t.me/+invite_hash` (if already joined)
- Numeric ID (bot-API style: positive=user, negative=chat, -100xxx=channel)

Peers resolved by username get cached with valid access hashes. Numeric IDs reuse cached hashes when available. Prefer @username.

### Channels vs Groups

Both are handled transparently. Wrapper checks `peer.Type == PeerChannel` and uses the appropriate API methods (e.g., `ChannelsGetMessages` vs `MessagesGetMessages`). No special flags needed except `isChannel` in `chats_create`.

### Auth flow

Cascade: env var → MCP elicitation → error. No stdin fallback (stdin = MCP protocol).

Auth guard middleware blocks tool/resource/prompt calls until auth completes.

Session persistence: volume mount `-v ~/.mcp-tg:/home/nobody/.mcp-tg`.

### Forum topics

Tools that send messages (`messages_send`, `messages_send_file`, `media_send_album`) accept `topicId` to target a specific forum topic. `messages_list` accepts `topicId` to filter messages by topic (uses `MessagesGetReplies` API instead of `MessagesGetHistory`). Message output includes `topicId` extracted from `MessageReplyHeader.ReplyToTopID`.

### Reply metadata

Reading tools (`messages_list`, `messages_context`, `messages_get`, `messages_search`, `messages_search_global`) expose reply-header fields on each `MessageItem`:

- Structured `replyTo` object (`messageId`, `topId`, `quoteText`, `fromPeerId`) when the message replies to another. Omitted otherwise.
- Text `output` prefixes replying messages with `[<id> ↩<parentId>]` so humans can see thread links without parsing JSON. This applies to `messages_list`, `messages_context`, `messages_get`, and `messages_search`. `messages_search_global` returns only a summary line (`Found N message(s)`) — its `output` does not format individual messages, so callers must read the JSON `replyTo` field for global-search replies.

The first four tools also accept an optional `resolveReplies` parameter (default `false`). When `true`, parent messages that aren't in the returned batch are fetched in a single batched `GetMessages` call and attached as `replyToMessage: { fromName, text }` (text truncated to 200 runes). Cross-chat replies (`replyTo.fromPeerId` points elsewhere) are skipped since we lack the foreign peer's access hash. `messages_search_global` does not offer `resolveReplies` for the same reason — its results span arbitrary peers, a batched lookup is not feasible.

`resolveReplies` enriches only the JSON `replyToMessage` field. The text `output` is built once from the fetched batch and keeps just the `↩<parentId>` marker regardless of the flag — callers that need resolved parent text should read the JSON structure.

### Message entities (formatting on read)

`MessageItem` carries an optional `entities` array with the message's formatting spans as read from MTProto `Message.Entities`. Each entry has `type` (Bot API naming: `bold`, `italic`, `code`, `pre`, `text_url`, `url`, `mention`, `hashtag`, etc.), `offset` and `length` in UTF-16 code units, plus optional `url`/`language`/`userId` for types that carry metadata. Plain messages omit the field entirely.

### Parse mode (formatting on write)

Tools that send or edit text (`messages_send`, `messages_edit`, `messages_send_file`, `media_send_album`) accept `parseMode`:

- `""` (empty / omitted) — plain text, no formatting.
- `"commonmark"` — CommonMark subset: `**bold**`, `*italic*`, `` `code` ``, ` ```pre``` `, `~~~pre~~~`, 4-space indented code blocks, `[text](url)`, `<https://autolink>`, `> quote`, `>quote` (no space ok), `~~strike~~`, `__underline__`, `||spoiler||`. Parsed into `tg.MessageEntity` on the server side.
- `"markdown"` — legacy alias for `commonmark`.
- `"html"` / `"markdownv2"` — recognised but not yet implemented; return a clear error.
- Anything else — rejected with the list of allowed values.

Known CommonMark gaps documented in README's "Markdown — Known Limitations": nested blockquotes (`> > x`), nested emphasis (`**a *b***`), hard line breaks via two trailing spaces or trailing `\`. Each has a commented-out test in `internal/telegram/markdown_audit_test.go`.

### Telegram protocol details

- **RandomID**: All send operations (message, file, album, forward, sticker) generate crypto-random IDs for deduplication
- **FLOOD_WAIT**: gotd/td middleware auto-retries up to 3 times with server-specified delay
- **Peer cache**: Access hashes from username resolution and dialog listing are cached in memory

## Linter

Strict config in `.golangci.yml`:
- funlen: 50 lines / 40 statements
- gocyclo/cyclop: 10
- dupl: 100
- lll: 140 characters
- All linters enabled except: depguard, exhaustruct, ireturn

## Dependencies

- `github.com/gotd/td` — Telegram MTProto client
- `github.com/cockroachdb/errors` — Error wrapping
- `github.com/modelcontextprotocol/go-sdk` — MCP protocol SDK
- `golang.org/x/sync` — errgroup for concurrent transports

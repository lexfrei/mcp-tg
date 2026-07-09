# CLAUDE.md — Development Guide for mcp-tg

## What is this

MCP server for Telegram Client API (MTProto, not Bot API). Uses gotd/td for protocol, exposes 78 tools + resources + prompts via MCP.

## Build & Test

```bash
go build ./cmd/mcp-tg
go test -race ./...
golangci-lint run
```

## Architecture

```text
cmd/mcp-tg/main.go          Entry point: `login` subcommand dispatch, else telegram client → MCP server → transports; mcpDevice/headlessLoginRequired helpers
cmd/mcp-tg/login.go         `mcp-tg login` — interactive TTY login (writes the session, keychain or file), credential-safe, no MCP surface
cmd/mcp-tg/storage.go       Session backend factory: OS keychain (lexfrei/keychain) by default, plaintext file on --insecure-storage/TELEGRAM_SESSION_INSECURE
cmd/mcp-tg/flood_wait.go    FLOOD_WAIT auto-retry middleware for gotd/td
cmd/mcp-tg/conn_reinit.go   CONNECTION_LAYER_INVALID re-init middleware for gotd/td (takes the mcpDevice DeviceConfig)
cmd/mcp-tg/auth_revoked.go  AUTH_KEY_UNREGISTERED (revoked session) detection middleware for gotd/td
internal/config/             Env var loading and validation
internal/telegram/           Telegram abstraction layer
  types.go                   Domain types (Message, User, Dialog, etc.)
  client.go                  Client interface (14 sub-interfaces)
  wrapper.go                 gotd/td implementation of Client
  wrapper_helpers.go         Helper functions for wrapper (extractors, converters)
  convert.go                 tg types → domain types conversion
  auth.go                    Auth flow with MCP elicitation support
  resolve.go                 Peer resolution (@username, numeric ID, t.me/ URLs, invite links)
  peer_cache.go              Thread-safe cache for peer access hashes
  markdown.go                Markdown → Telegram entities parser (entry point)
  markdown_inline.go         Inline marker parsing (bold, italic, code, links, etc.)
  markdown_convert.go        rawEntity → tg.MessageEntityClass conversion + escape removal
  send_as.go                 send_as identity: GetSendAs, SetDefaultSendAs, peer-cache seeding
internal/tools/              MCP tool handlers (78 tools)
  annotations.go             Tool annotation helpers (readOnly, idempotent, write, destructive)
  errors.go                  Error sentinels
  helpers.go                 Shared helpers (deref, formatPeer, formatPeerRef, formatUserName, peerLabel)
  format.go                  Output formatting (timestamps, messages, dialogs)
  roots.go                   File path validation against client roots
  progress.go                Progress notification helper
  result_types.go            Structured JSON result types (DialogItem, MessageItem, etc.)
  register.go                tools.AddTool wrapper that records bool fields into the coercer registry
  mock_test.go               Mock telegram.Client for tests
internal/resources/          MCP resources (4 resources)
internal/prompts/            MCP prompts (3 prompts)
internal/completions/        Argument autocompletion
internal/middleware/         Auth guard, session guard (revoked-session fast-fail) + SessionHealth state, request logging, bool-coercion
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

- `readOnlyAnnotations()` — tools that only read data (31 tools)
- `idempotentAnnotations()` — tools that modify state but are safe to retry (28 tools)
- `writeAnnotations()` — tools that create new entities, not idempotent (10 tools)
- `destructiveAnnotations()` — tools that delete/remove things (9 tools)

The four counts must sum to the tool total. `TestToolCensus_MatchesTheDocumentedCounts` (`cmd/mcp-tg/tool_census_test.go`) pins them against the registered server, so a stale number fails CI instead of surviving into the next PR — which is how `readOnly` sat one short for a whole release.

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

Two entry points. The server auth uses a cascade: env var → MCP elicitation → error (no stdin fallback — stdin is the MCP protocol). The `mcp-tg login` subcommand (`login.go`) is a separate interactive TTY login (phone/code/2FA read from the terminal, no MCP surface) — the only way to log in a headless daemon, which cannot elicit. On a headless startup with no valid session the auth error is rewritten (`headlessLoginRequired`) to point at `mcp-tg login` instead of gotd's misleading "TELEGRAM_PHONE is required".

Auth guard middleware blocks tool/resource/prompt calls until auth completes. After auth, a revoked session (`AUTH_KEY_UNREGISTERED` and friends, detected by `auth_revoked.go`) trips `SessionHealth`, and `NewSessionGuard` fast-fails tool calls with `ErrSessionRevoked` (explicit: logged out, run `mcp-tg login`, not fixable from the MCP client).

### Session storage (`storage.go`)

Secure by default: the session lives in the OS keychain via `github.com/lexfrei/keychain` behind gotd's `session.Storage`. macOS uses `WithSecurityCLI` (stable `apple-tool` partition → rebuild-stable for an unsigned daemon). Plaintext file only on explicit opt-in: `--insecure-storage` (login) or `TELEGRAM_SESSION_INSECURE=true` (server); both must match. An unreachable keychain (container / headless Linux) errors with guidance instead of silently writing plaintext. `TELEGRAM_SESSION_FILE` is the keychain account key in secure mode, the file path in insecure mode.

### Client identity

`mcpDevice()` sets `telegram.Options.Device` so the account's Devices list names the client `mcp-tg` (not gotd's default "go1.26.4" device model). The same `DeviceConfig` is passed to `newConnReinitMiddleware` so a connection re-init advertises identical parameters. App-name (from api_id) and Telegram's platform icon are not ours to set.

### Transport modes

`startServer` (`cmd/mcp-tg/main.go`) dispatches on `cfg.HTTPOnly` (`MCP_HTTP_ONLY`):

- Default (`startStdio`): stdio is the primary transport, one process per client, plus an optional additional HTTP transport when `MCP_HTTP_PORT` is set. Auth elicitation runs through the stdio session, so this mode can do an interactive login.
- Headless (`startHeadless`, requires `MCP_HTTP_ONLY=true` + `MCP_HTTP_PORT`): HTTP is the only transport, no stdio peer. One process and one Telegram connection serve many clients — the shared-daemon mode. Auth cannot elicit (no client session to prompt), so it depends on a valid persisted session file or env-var credentials.

Both paths build the server through `buildServer`. The headless path passes `onInit=nil` deliberately: the stdio path's `InitializedHandler` closes a shared channel, which would panic on the second HTTP client's `initialize` (close of a closed channel) since headless HTTP serves many clients.

### Forum topics

Tools that send messages (`messages_send`, `messages_send_file`, `media_send_album`) accept `topicId` to target a specific forum topic. `messages_list` accepts `topicId` to filter messages by topic (uses `MessagesGetReplies` API instead of `MessagesGetHistory`). Message output includes `topicId` extracted from `MessageReplyHeader.ReplyToTopID`.

### Reply metadata

Reading tools (`messages_list`, `messages_context`, `messages_get`, `messages_search`, `messages_search_global`) expose reply-header fields on each `MessageItem`:

- Structured `replyTo` object (`messageId`, `topId`, `quoteText`, `fromPeerId`, plus advisory `fromName`/`fromUsername` for cross-chat replies) when the message replies to another. Omitted otherwise.
- Text `output` emits a `reply to: <parentId>` line (or `reply to: <parentId> in <peer-ref>` for cross-chat replies, followed by a `quote: «...»` line if `quoteText` is present). This applies to `messages_list`, `messages_context`, `messages_get`, and `messages_search`. `messages_search_global` returns only a summary line (`Found N message(s)`) — its `output` does not format individual messages, so callers must read the JSON `replyTo` field for global-search replies. See README's "Message Output Format" for the full block layout.

The first four tools also accept an optional `resolveReplies` parameter (default `false`). When `true`, parent messages that aren't in the returned batch are fetched in a single batched `GetMessages` call and attached as `replyToMessage: { fromName, fromUsername, text }` (text truncated to 200 runes). Cross-chat replies (`replyTo.fromPeerId` points elsewhere) are skipped since we lack the foreign peer's access hash. `messages_search_global` does not offer `resolveReplies` for the same reason — its results span arbitrary peers, a batched lookup is not feasible.

`resolveReplies` enriches only the JSON `replyToMessage` field. The text `output` is built once from the fetched batch and keeps just the `reply to: <parentId>` line regardless of the flag — callers that need resolved parent text should read the JSON structure.

### Forwards and peer identifiers

Reading tools populate `MessageItem.forward` from `tg.MessageFwdHeader` so callers can distinguish a forwarder from the original author:

- `forward.from` — resolved `PeerRef` (peer+name+username) of the original sender; nil when the original author has forward-privacy enabled.
- `forward.fromName` — display name leaked through when `from` is nil (privacy-hidden forward).
- `forward.date`, `forward.channelPost`, `forward.postAuthor` — original timestamp, ID of the source channel post, and the channel's signed-author byline.

**Single helper, single shape, everywhere.** Every peer rendered to the user — sender, forward origin, cross-chat reply target, dialog, group, channel, user, contact, reactor — goes through `formatPeerRef(name, username, peer)` and produces the same string: `Display Name [@username]` / `[user:N]` / `[channel:N]` / `[group:N]` / `[hidden]` / `[unknown:N]`. The kind label (`user` / `group` / `channel` / `unknown`) matches the JSON `type` field on `ParticipantItem`, `MessageItem.fromType`, and every other peer-bearing JSON entry. `group:` covers `PeerChat` only; supergroups label as `channel:` (gotd folds them into `PeerChannel`).

`formatUserName(*telegram.User)` and `formatDialog(*telegram.Dialog)` are thin adapters that route their inputs into `formatPeerRef`, so a renamed `peer*` constant in `format.go` propagates to every surface. Resource `tg://chat/{peer}/messages` uses the exported `tools.FormatMessageList` so it shares the same multi-line block layout the read tools emit.

Username resolution for `messages_*` piggybacks on the `Users[]`/`Chats[]` arrays MTProto already returns in `MessagesMessagesClass`, so no extra API round-trip. `tg_dialogs_*` and `tg_groups_info` get the username directly from the wrapper-returned `Dialog`/`GroupInfo`. `ContactStatus` has the field slot but it is currently unpopulated — `ContactsGetStatuses` does not return `Users[]` alongside statuses, so consumers see `[user:N]` rather than `Name [@user]` for those entries until a follow-up adds a batched lookup.

### Multi-line text output

Each message in `output` is a block of `key: value` lines (`from:`, `forwarded from:`, `reply to:`, `quote:`, always-present `type:`) followed by the body under `text:`. Blocks are separated by a literal `---` line so a message body containing its own blank lines (Telegram bodies routinely have paragraph breaks) stays unambiguous. Long bodies are emitted verbatim — no truncation in the human-readable string. The single-line `[ID ↩parent] ts sender: text` form was removed; do NOT reintroduce it or grep for `↩`. Use `MessageItem.type` / `type:` for message kind; do NOT reintroduce `mediaType` / `media:`.

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

### Send-as identity (posting as a channel)

MTProto's `send_as` field appears on exactly five requests, which map onto six tools: `messages.sendMessage`, `messages.sendMedia` (backs both `tg_messages_send_file` and `tg_stickers_send`), `messages.sendMultiMedia`, `messages.forwardMessages`, `messages.createForumTopic`. All six take an optional `sendAs` string.

Wiring: `SendOpts.SendAs *InputPeer` for the three methods that already had an options struct; a trailing `sendAs *InputPeer` argument for `ForwardMessages` / `CreateForumTopic` / `SendSticker`. `nil` means "post as the account itself" everywhere. `applySendAs(sendAs, req.SetSendAs)` (`wrapper_helpers.go`) sets the conditional field for all five request types via the method value — do NOT inline the `if sendAs != nil` check per call site, `dupl` will flag it.

`resolveSendAs` (`tools/helpers.go`) resolves the string and rejects a `PeerChannel` whose `AccessHash` is 0. That state is reachable and silent: `resolveByID` returns it with a nil error for any numeric ID the client has never seen, and the resulting server error names neither the parameter nor the fix. `tg_chats_get_send_as` seeds the peer cache from the `Chats`/`Users` of its own reply, which is what makes numeric IDs work afterwards.

`tg_chats_set_send_as` wraps `messages.saveDefaultSendAs`. Omitting `sendAs` there is not a missing argument — it resets the chat to the account (`&tg.InputPeerSelf{}` in the wrapper, since the domain `InputPeer` cannot express self). The default is account-wide server state and the only way to react or vote in a poll as a channel; `messages.sendReaction` and `messages.saveDraft` have no `send_as` field at all. `tg_groups_info` reports the current default from `ChannelFull.DefaultSendAs` at no extra RPC cost.

Both new tools reject non-`PeerChannel` peers before the round trip (`telegram.ErrSendAsUnsupportedPeer`); the server's `CHANNEL_INVALID` explains nothing.

Verified against a live account: a rejected identity comes back as `CHAT_ADMIN_REQUIRED` (channel you don't administrate) or `CHAT_WRITE_FORBIDDEN` (foreign user), NOT `SEND_AS_PEER_INVALID` — that code exists in the schema but the server rarely reaches for it. Both read as a chat-permission problem, so `sendErr` (`tools/errors.go`) names `sendAs` as a suspect whenever one was supplied. Do not "simplify" the six send tools back to plain `telegramErr`.

Also verified: a channel that is the chat default reacts as itself, and `messages.getMessageReactionsList` returns its title in `Chats`, not `Users` — `extractReactionUsers` reads both and carries `ReactionUser.PeerType`.

### Sticker documents (`sticker_cache.go`)

`inputDocument` needs id + access hash + file reference. Only the id is public and stable; the other two arrive with `messages.getStickerSet` and cannot be derived. `GetStickerSet` therefore seeds `StickerCache`, and `SendSticker` looks the document up there, failing with `ErrStickerNotCached` before the RPC when it is absent. Sending a bare id (which is what the code did until this cache existed) answers `MEDIA_EMPTY` on every call. Same shape as the send-as peer cache: read the listing tool once, then the id works.

**`stickerFileId` is a string, and must stay one.** `mcp/tool.go` in go-sdk v1.6.1 unmarshals tool arguments into `map[string]any`, calls `ApplyDefaults`, then re-marshals — so every JSON number passes through `float64`. `internal/json` wraps `segmentio/encoding/json` without `UseNumber`, so there is no escape hatch. A sticker document id needs 63 bits; the mantissa holds 53. Verified end-to-end: `5181593617004757506` reached the wrapper as `5181593617004758000`. Any future tool parameter above 2^53 must be a decimal string for the same reason — this is not paranoia, `stickerFileId` was silently broken by it. Telegram user IDs are well under the limit and are safe as numbers.

### Telegram protocol details

- **RandomID**: All send operations (message, file, album, forward, sticker) generate crypto-random IDs for deduplication
- **FLOOD_WAIT**: gotd/td middleware auto-retries up to 3 times with server-specified delay
- **Peer cache**: Access hashes from username resolution, dialog listing, and `channels.getSendAs` are cached in memory

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
- `github.com/lexfrei/keychain` — cgo-free OS secret store (macOS Keychain / Linux Secret Service / Windows Credential Manager) for the default session backend; pulls `purego` + `godbus/dbus` indirectly
- `golang.org/x/sync` — errgroup for concurrent transports
- `golang.org/x/term` — no-echo TTY password read in `mcp-tg login`

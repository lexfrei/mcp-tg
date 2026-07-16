# CLAUDE.md — Development Guide for mcp-tg

## What is this

MCP server for Telegram Client API (MTProto, not Bot API). Uses gotd/td for protocol, exposes 78 tools + resources + prompts via MCP.

## Build & Test

```bash
go build ./cmd/mcp-tg
go test -race ./...
golangci-lint run
```

## Documentation

User-facing documentation lives in `docs/`, is built with MkDocs Material (`mkdocs.yml`) and publishes to <https://mcp-tg.lexfrei.dev> on every push to master (`.github/workflows/pages.yml`; `pr.yml`'s `docs` job runs the same `mkdocs build --strict` as a gate). `docs/CNAME` carries the custom domain into the built site. README.md is a landing page only — install, quickstart, links — and anything longer belongs on a docs page.

```bash
pip install mkdocs-material==9.7.6   # pinned: MkDocs 2.0 drops the plugin system with no migration path
mkdocs serve
```

Documentation that makes a claim the code can contradict is pinned by `cmd/mcp-tg/docs_contract_test.go`. Every number a page states about the server is derived from the server, never from another page:

- `docs/tools.md` — the `## Tools (N)` heading and the `` - `tg_xxx` `` bullets are parsed and compared against the registered server (`TestDocsToolList_MatchesRegisteredTools`); each `### Name (N)` subtotal must equal its own bullet count and they must sum to the total (`TestDocsToolSubsections_MatchTheirBullets`); the annotation table is compared against the census constants (`TestDocsAnnotationTable_MatchesTheCensus`). Keep all three shapes exactly when editing.
- `README.md`, `docs/index.md`, `mkdocs.yml` — every restatement of the tool / resource / prompt totals, in prose (`78 tools`, `78-tool`, `4 resources`) and in the protocol-table rows (`TestDocsCensusCounts_MatchTheServer`, with `TestDocsCensusCounts_AreActuallyStated` guarding against a page that drops the numbers and passes vacuously). `mkdocs.yml`'s `site_description` counts: Material renders it into the `<meta name="description">` of every page.
- `docs/search.md` — the `Values: ...` list is compared against `telegram.SearchFilters` (`TestDocsFilterValues_MatchSearchFilters`).
- `docs/messages.md` — the parse-mode contract: both enum values, `allowRawMarkdown`, `entitiesParsed`, the autolink example, the migration note, and the absence of the retired `markdown` alias (`TestDocsParseMode_MatchesTheContract`).
- `docs/building.md` — the minimum Go version against go.mod's `go` directive, which is a hard minimum since Go 1.21, not a hint (`TestDocsGoVersion_MatchesGoMod`).
- `README.md` — the major-version promise (`TestReadmeMajorVersion_MatchesTheModulePath`).
- The mkdocs-material version must agree across `pages.yml`, `pr.yml`, `CLAUDE.md` and `README.md` (`TestMkdocsMaterialPin_AllSitesAgree`) — an unpinned local install renders a different site than CI publishes.
- Every `https://mcp-tg.lexfrei.dev/<page>/` URL hardcoded in Go source must resolve to a file under `docs/` (`TestDocsSiteURLs_ResolveToPages`). The revoked-session error hands an operator such a URL as its only recovery instruction.

A page that moves must take its pin with it: each test names the path it read, so a relocated page fails loudly rather than silently stopping to check anything.

## Architecture

```text
cmd/mcp-tg/main.go          Entry point: `--version`/`login` dispatch (via flags.go), else telegram client → MCP server → transports; mcpDevice/headlessLoginRequired helpers
cmd/mcp-tg/login.go         `mcp-tg login` — interactive TTY login (writes the session, keychain or file), credential-safe, no MCP surface
cmd/mcp-tg/storage.go       Session backend factory: OS keychain (lexfrei/keychain) by default, plaintext file on --insecure-storage/TELEGRAM_SESSION_INSECURE
cmd/mcp-tg/flood_wait.go    FLOOD_WAIT auto-retry middleware for gotd/td (WARN log per retry; surfaces as tools.ErrFloodWait after exhaustion)
cmd/mcp-tg/conn_reinit.go   CONNECTION_LAYER_INVALID re-init middleware for gotd/td (takes the mcpDevice DeviceConfig)
cmd/mcp-tg/auth_revoked.go  AUTH_KEY_UNREGISTERED (revoked session) detection middleware for gotd/td
cmd/mcp-tg/subscriptions.go resourceUpdater adapter + Subscribe/Unsubscribe handlers wiring the SubscriptionBroker to the MCP server
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
  markdown_lint.go           LooksLikeMarkdown — plain-mode lint approximating what the parser would transform
  send_as.go                 send_as identity: GetSendAs, SetDefaultSendAs, peer-cache seeding
  subscriptions.go           SubscriptionBroker: routes new-message updates to watching resource URIs (ResourceUpdater sink)
internal/tools/              MCP tool handlers (78 tools)
  annotations.go             Tool annotation helpers (readOnly, idempotent, write, destructive)
  errors.go                  Error sentinels
  helpers.go                 Shared helpers (deref, formatPeer, formatPeerRef, formatUserName, peerLabel)
  format.go                  Output formatting (timestamps, messages, dialogs)
  roots.go                   File path validation against client roots
  progress.go                Progress notification helper
  result_types.go            Structured JSON result types (DialogItem, MessageItem, etc.)
  register.go                tools.AddTool wrapper (records bool fields into the coercer registry) + inputSchemaWithEnum for enum-constrained input schemas
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

The four counts must sum to the tool total. `TestToolCensus_MatchesTheDocumentedCounts` (`cmd/mcp-tg/tool_census_test.go`) pins them against the registered server, so a stale number fails CI instead of surviving into the next PR — which is how `readOnly` sat one short for a whole release. `TestDocsAnnotationTable_MatchesTheCensus` then pins the published table in `docs/tools.md` against those same constants, so the site cannot drift from the server either.

### Peer resolution

All tools accept `peer` as string. Supported formats:

- `@username`
- `username` (bare)
- `https://t.me/username`
- `https://t.me/+invite_hash` (if already joined)
- Numeric ID (bot-API style: positive=user, negative=chat, -100xxx=channel)

Peers resolved by username get cached with valid access hashes. Numeric IDs reuse cached hashes when available. Prefer @username.

Cold numeric miss (`ResolvePeer` / `wrapper.go`): a numeric ID with no cached hash first tries `resolveViaDialogs` (a single `messages.getPeerDialogs` — useless for a channel, whose input peer already needs the missing hash), then `warmDialogsCache`. The warm (`paginateWarmDialogs` → `warmFolder`) paginates the FULL dialog list, both the main folder and the archive (`folder_id=1`), up to `warmDialogsMaxPages` per folder as a safety bound; folder 0 is left implicit so the main request stays byte-identical to a plain listing. gotd omits `folder_id` on the wire when it is 0, so the archive pass MUST set `folder_id=1` explicitly. The whole warm is throttled to once per `warmDialogsThrottle`, and a `singleflight` (`warmSF`, same pattern as `serverConfig`'s `cfgSF`) collapses racing callers onto ONE warm — a caller that arrives mid-warm WAITS for it and then sees the populated cache rather than a premature miss. The completion stamp (`warmedAt`) is written only AFTER `warmFolder` scans a folder to EXHAUSTION (a complete `*tg.MessagesDialogs` or a page with no cursor). Every other exit is an error, because the terminal "not cached" verdict may only be drawn from a fully-scanned folder: a page fetch that fails propagates, AND hitting `warmDialogsMaxPages` with the folder still not exhausted returns `ErrDialogScanLimit` (an account with more dialogs than the resolver scans — the channel may sit beyond the cap). Neither latches the throttle. `paginateWarmDialogs` scans BOTH folders every time even if one fails — an archived channel lives only in folder 1, so a main-folder failure or cap must not skip the archive — and remembers the first incompleteness, returning it only after every folder had its chance to seed the cache. Peers cached by pages that did succeed still help: `ResolvePeer` consults the cache before treating a miss as terminal, so a partial warm never loses ground. `warmDialogsCache` returns `(ran, err)` — `ran` is false only when the outer throttle skipped the scan (a singleflight-shared scan still counts as ran) — and `ResolvePeer` acts on both, only for a `PeerChannel` (`channelResolveError`): the terminal `ErrChannelNotCached` (the "read the listing once, then the id works" contract, like stickers / send-as) is drawn ONLY from a warm that RAN this call AND completed. An INCOMPLETE warm (page error or `ErrDialogScanLimit`) surfaces that error, and a warm SKIPPED by the throttle rests on a scan up to `warmDialogsThrottle` old — so a miss there returns the retryable `ErrChannelWarmStale` and invalidates the stamp (`warmedAt = 0`) so an immediate retry re-scans, catching e.g. a channel joined on another session since the last warm. The caller is thus never told a possibly-present channel is absent. Users and basic groups keep the hash-0 fallback regardless of the warm outcome, so the tools-layer `from` / `offsetPeer` guards still label those per parameter.

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

### Resource subscriptions

`tg://chat/{peer}/messages` is subscribable: a new message in a watched chat pushes a `resources/updated` notification for the exact URI the client subscribed with. `SubscriptionBroker` (`internal/telegram/subscriptions.go`) is the app-side registry, modeled on `TranscriptionBroker` — process-global shared state, since the headless daemon serves many sessions from one update goroutine. Bookkeeping mirrors the SDK exactly, because the SDK gates delivery by the LITERAL URI a client subscribed with (NOT by peer) and tracks a SET of sessions per URI. The broker mirrors both: `subscribers[uri]` is the set of subscribing sessions, `owner[uri]` records which peer currently triggers each URI, and `peerURIs[key]` is the peer→URIs reverse index for message lookup. Both keying choices are load-bearing and were reached by fixing the failures of the simpler shapes. **Keying by URI, not peer:** a peer-keyed count breaks because several URI spellings (`@username` vs numeric) resolve to one chat and a URI's `@username` can be reassigned to a different chat; so a message notifies EVERY URI its peer triggers, and `Watch` moves a URI's trigger to the newest peer (carrying the subscriber set across the move). **Keying subscribers by SESSION, not a count:** the SDK calls Subscribe/Unsubscribe handlers BEFORE consulting its own registry, so a duplicate subscribe or an unmatched/cross-session unsubscribe reaches the broker; a plain count would let one client's spurious unsubscribe drop another client's watch. A session set makes `Watch`/`Unwatch` idempotent per session (an unmatched unwatch is a no-op). Session identity is opaque (`any`; the daemon passes `*mcp.ServerSession`) because `ServerSession.ID()` is empty on stdio. `Unwatch` takes `(session, uri)` and resolves no peer — both to skip a round-trip and because a resolution failure must never block the SDK from dropping the subscription. `dispatch` snapshots the URIs + notifier under the lock, then calls `ResourceUpdated` per URI OUTSIDE it (first push error returned after all are attempted). Releasing the lock protects concurrent `Watch`/`Unwatch`, NOT the caller's goroutine: `dispatch` runs on gotd's single update-read loop, and `Server.ResourceUpdated` does a synchronous per-session transport write (10s SDK timeout each), so a slow client would stall updates for every client and the co-registered transcription handler. The notifier must therefore be non-blocking: `resourceUpdater` (`cmd/mcp-tg/subscriptions.go`) hands each push to a goroutine under a detached context (the read-loop ctx is cancelled on Telegram reconnect; MCP delivery is independent). This mirrors `TranscriptionBroker`'s non-blocking-send discipline. The `ResourceUpdater` interface is local, so `internal/telegram` carries no MCP import; `main.go` wires the `resourceUpdater` adapter (via `newResourceUpdater`) after `mcp.NewServer`.

Wiring (`main.go`): `dispatcher.OnNewMessage`/`OnNewChannelMessage` are registered BEFORE `tgClient.Run` (same as `OnTranscribedAudio`); the broker threads through `startServer → startStdio/startHeadless → buildServer → newServerOptions`; `SetNotifier` runs after the server is built. `newServerOptions` installs both a `SubscribeHandler` and `UnsubscribeHandler` (the SDK panics if only one is set, and auto-advertises `Resources.Subscribe` once both exist). Each resolves the URI's peer via `resources.ChatMessagesPeer` (which REQUIRES the `/messages` suffix, so the bare info URI is ignored → `nil`) then `client.ResolvePeer`; a resolution error fails the subscribe.

The peer match comes from `msg.AsNotEmpty().GetPeerID()` — `tg.MessageClass` has no `GetPeerID`; only `NotEmptyMessage` does. A `MessageEmpty` or a message with no `PeerID` yields the ID-0 sentinel and is dropped. That is exactly the self-send echo shape: `updateShortSentMessage` upconverts to an `UpdateNewMessage` with NO `PeerID`, so the account's OWN outgoing message (sent through this server) never fires the notification — a Saved-Messages message from ANOTHER device arrives with `PeerID=PeerUser{selfID}` and does. Live smoke must therefore send from a second device, and the smoke URI uses the numeric self-ID (`resolve.go` has no me/self shorthand).

Known limitation (disconnect drift, cleared only by process restart): the SDK removes a disconnected session from its own subscription registry (`server.go` `disconnect`) but does NOT call `UnsubscribeHandler` (that fires only on the explicit `unsubscribe` RPC). The broker's session set is not pruned, so a dead session's entry lingers. Delivery stays correct — the SDK's registry is authoritative, a dead session receives nothing — but two costs accrue and neither is bounded by the set of distinct chats: the retained `(URI, session)` entries grow with cumulative abnormal-disconnect churn over the daemon's lifetime (each reconnect mints a new `*ServerSession`); and a lingering entry keeps `subscribers[uri]` non-empty, so that URI's `owner`/`peerURIs` are never cleaned and every later message in the chat costs a peer lookup plus one `subscriber_count=0` INFO line. No clean fix exists — `ServerOptions` has no session-closed callback, the headless path never holds the per-session handle, and a prune-on-zero-live-subscribers scheme would race `Watch` (which runs before the SDK registers the subscription). Accept and document; a restart clears it.

### Logging and log level

`resolveLogLevel(os.Args, MCP_LOG_LEVEL)` (`flags.go`) picks the slog level once at startup: `--log-level` flag > `MCP_LOG_LEVEL` env > default `info`; an unknown name is `ErrInvalidLogLevel`, not a silent fallback. `newLogger(level)` builds ONE `*slog.Logger` that is threaded explicitly through `run → startServer → startStdio/startHeadless → buildServer → newServerOptions` (and `waitForTransports`/`runHTTPServer`). It is deliberately NOT a package global. The bug this replaced: `newServerOptions` called `newLogger()` a second time, so the request-logging middleware ignored the level and always logged at info — pinned now by `TestNewServerOptions_UsesThreadedLogger`.

Level routing: FLOOD_WAIT retries → WARN (`flood_wait.go`), context cancellation during a FLOOD_WAIT backoff → WARN, HTTP transport shutdown failures → ERROR (`runHTTPServer`), tool-call and method failures → ERROR (`middleware.NewLogging`). The per-request success summary (`MCP request handled`) stays at INFO as an operational heartbeat; DEBUG is the verbose tier (the gotd zap connection/migration logger, `newGotdLogger`). So a default-`info` daemon already records FLOOD_WAIT and failures for a post-mortem without `--log-level debug`.

`--version` (`versionRequested`/`runVersion` in `flags.go`) prints `mcp-tg <version> (<shortRev>)` to stdout and exits before any server or Telegram work; `logStartupVersion` emits the same build id as one INFO line at startup, so an `info`/`debug` daemon names the running binary in its log (at `warn`/`error` it is filtered like any INFO record — `--version` and `tg_server_version` are the level-independent build check).

### Forum topics

Tools that send messages (`messages_send`, `messages_send_file`, `media_send_album`) accept `topicId` to target a specific forum topic. `messages_list` accepts `topicId` to filter messages by topic (uses `MessagesGetReplies` API instead of `MessagesGetHistory`). Message output includes `topicId` extracted from `MessageReplyHeader.ReplyToTopID`.

### History pagination

`GetHistory`/`GetTopicMessages` share one `pageHistory` loop (`wrapper.go`): it calls a per-page `historyPageFunc` returning a `historyPageData` (converted messages + `RawCount` + `NextOffset` + `Total`) in chunks of at most `maxHistoryPage` (100, Telegram's server page size), advances the cursor by the RAW page's smallest ID (`minRawMessageID` over `rawHistoryMessages` — service messages included), and merges until the limit is reached, the server returns a short RAW page, the cursor is exhausted, `HistoryOpts.MinDate` (a client-side floor — getHistory has no server min_date) is crossed, or `HistoryOpts.SinglePage` limits it to exactly one page. `OffsetDate` anchors the first page only; `total` comes from the first page. FLOOD_WAIT is handled per RPC by the existing gotd middleware, so the loop needs no backoff of its own.

The short-page and empty checks are deliberately on `RawCount`, NOT the visible (post-conversion) message count: `convertMessages` drops `tg.MessageService`, so a full 100-item raw page whose visible entries are all service messages would otherwise read as "history ended" and stop the walk while older normal messages still exist. Advancing the cursor by the raw min ID (not the visible min) likewise keeps it moving past a page that is entirely service messages. Two independent caps keep a single call bounded: `pagedHistoryLimit` hard-caps the effective limit at `maxPagedHistory` (1000), bounding the pre-allocation and the merged result; and `maxHistoryScan` (5000) bounds the RAW messages scanned. The scan cap is what actually bounds the round-trips — because `merged` grows only by VISIBLE messages, a service-message-heavy stretch would let the loop keep fetching full pages without `merged` reaching the limit, so a merged/limit cap alone would NOT bound the RPCs. This mirrors the type-filter path's `maxTypeFilterScan`. `tg_messages_list` rejects a limit above 1000 outright; any other caller that passes a large limit is silently capped (a fixed-window caller like `tg_messages_context` opts out of paging entirely via `SinglePage`).

Because `merged` grows only by VISIBLE messages, the scan cap can stop the walk with the limit unfilled while older history remains — so `hasMore` computed as `len(msgs) >= limit` would falsely read "no more" and strand a caller that pages on it. `GetHistory`/`GetTopicMessages` therefore return a fourth value: `pageHistory`'s `historyMayHaveMore` (last RAW page was full, cursor still live, no floor crossed), which stays true across a scan-cap stop. The non-type `tg_messages_list` path surfaces that bool directly as `HasMore` instead of a len-vs-limit compare (the type-filter path keeps its own `applyScanLimit` hasMore; `messages_context` / resources / prompts ignore the bool). This is why the interface returns `([]Message, int, bool, error)` — the mock keeps its 3-value `getHistoryFn` and rides the bool alongside via a `historyHasMore` field.

`tg_messages_list` caps `limit` at 1000 (`maxMessagesListLimit`, `ErrLimitTooLarge`) — the only tool that raises the >100 auto-pagination — because a bigger limit means more round-trips. The `type` filter path keeps its own outer scan loop; each of its calls bounds `GetHistory` to `telegram.DefaultLimit` (100 VISIBLE messages, not one server page — the inner pager may issue several server RPCs to fill that in a service-heavy chat, bounded by `maxHistoryScan`) so a large limit does not page far beyond one outer step. Do NOT set `SinglePage` on that path to make it literally one server page: it would break the outer loop's `hasMorePage(visible, 100)` signal once service messages thin a page. `tg_messages_context` sets `HistoryOpts.SinglePage`: its window is symmetric by construction (`OffsetID = messageId + radius`, `Limit = 2*radius+1`), and auto-paginating to backfill service-message gaps would extend the window only on the older side and turn a large radius into many round-trips — so it stays one RPC (server-capped to 100), exactly as before auto-pagination. Resource / prompt reads (`Limit` 50 / ≤100) do NOT set `SinglePage`: they want "the last N messages", so filling to N visible past service messages is the intended behavior — normally one RPC, occasionally a second bounded page in a service-heavy chat.

`tg_messages_list`'s `toDate`→`HistoryOpts.OffsetDate` and `fromDate`→`HistoryOpts.MinDate` (reusing `validateDateRange`, so `fromDate > toDate` and negatives are rejected before any RPC). getHistory has no server-side `min_date`, so `fromDate` is a client-side floor — the read still returns at most `limit` messages, so a wide date window needs a matching `limit`. The default limit stays 100. In the `type` path the toDate `offset_date` is threaded onto the first page only while the fromDate floor rides every page.

### Search filters and global pagination

`tg_messages_search` / `tg_messages_search_global` take `filter` — Telegram's server-side `InputMessagesFilter*` kind filter (`internal/telegram/search_filter.go`, a factory map keyed by snake_case names; a map, not a switch, to keep gocyclo flat). This is NOT `tg_messages_list`'s `type`: `type` is a client-side label match over fetched history (`convert.go` labels), `filter` filters on the server; the value sets overlap but don't coincide (`url`, `pinned`, `my_mentions` exist only as filters; `webpage`, `invoice`, `unsupported` only as types). Telegram's `chat_photos` and `phone_calls`/`missed_calls` filters are deliberately NOT offered: they match service messages, which `convertMessages` drops, so every page would come back empty while `total` reports matches — do not re-add them without first surfacing `tg.MessageService` in the conversion. The error text for an unknown filter is built from `telegram.SearchFilters()`, so it cannot drift from the factory map. A search needs at least one of `query`/`filter`/`from` (per-chat) or `query`/`filter` (global); a bare `from` is the official clients' "all messages from this member" search.

Per-chat search scoping to a forum topic uses `messages.search`'s native `top_msg_id` — no extra `GetReplies` RPC like `tg_messages_list`'s `topicId` path needs. The `from` sender filter and the `offsetPeer` cursor resolve through `resolveOptionalPeer` (`tools/helpers.go`), and each call site then rejects a hash-0 resolution with its own parameter-named sentinel (`ErrFromUnresolved`, `ErrOffsetPeerUnresolved`) — `resolveByID` returns hash 0 with a nil error for any numeric ID the client has never seen, and the server error it would cause names neither the parameter nor the fix. The `offsetPeer` guard exempts `PeerChat` (legacy basic groups carry no hash by design and are valid cursor peers); the `from` guard has no exemption, senders are never basic groups.

Global search pagination is a compound cursor (`offsetRate` + `offsetId` + `offsetPeer`); the result returns it ready-made as `nextRate`/`nextOffsetId`/`nextOffsetPeer` (the JSON `messages[].peerId` is a structured object, not the bot-style string `offsetPeer` expects, so callers must not have to convert). When the reply carries no `next_rate`, the wrapper falls back to the last message's date per the documented cursor contract. `SearchGlobal` seeds the peer cache from the reply's `Users[]`/`Chats[]` (same pattern as `tg_chats_get_send_as`) — that is what makes a result's numeric `peerId` usable as `offsetPeer` for chats the account never resolved. Do not remove the seeding: without it, page-2 requests fail on unresolvable numeric peers. The cache is in-memory, so a restart between pages loses it — a cursor peer that resolves without an access hash is rejected with `ErrOffsetPeerUnresolved` ("re-run the first page") instead of being sent on to a server error that names neither the parameter nor the fix.

### Reply metadata

Reading tools (`messages_list`, `messages_context`, `messages_get`, `messages_search`, `messages_search_global`) expose reply-header fields on each `MessageItem`:

- Structured `replyTo` object (`messageId`, `topId`, `quoteText`, `fromPeerId`, plus advisory `fromName`/`fromUsername` for cross-chat replies) when the message replies to another. Omitted otherwise.
- Text `output` emits a `reply to: <parentId>` line (or `reply to: <parentId> in <peer-ref>` for cross-chat replies, followed by a `quote: «...»` line if `quoteText` is present). This applies to `messages_list`, `messages_context`, `messages_get`, and `messages_search`. `messages_search_global` returns only a summary line (`Found N of M message(s)`, M being the server's total) — its `output` does not format individual messages, so callers must read the JSON `replyTo` field for global-search replies. See `docs/messages.md` ("Message Output Format") for the full block layout.

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

### Output format param

The five read tools (`messages_list`, `messages_get`, `messages_context`, `messages_search`, `messages_search_global`) take an optional `format` enum (`full`/`json`/`text`, default `full`) constrained on the wire via `inputSchemaWithEnum` (the `parseMode` precedent) — optional, so it is NOT in the schema `required` set; an omitted value is `full`. `messagesForFormat`/`outputForFormat` (`messages_format.go`) trim the result: `json` clears `Output`, `text` clears `Messages`, `full` keeps both. The two helpers are split (not one two-value function) because gocritic `unnamedResult` and `nonamedreturns` conflict on a multi-value return. Both result fields are now tagged `omitempty`, so a cleared field vanishes from the JSON — this is a deliberate shape change (documented in `docs/messages.md`, "Message Output Format"): a full-mode result with zero messages now also omits `messages`. Handlers apply format LAST, after `resolveReplies`, so the enrichment still runs for `json`/`full`.

### Message entities (formatting on read)

`MessageItem` carries an optional `entities` array with the message's formatting spans as read from MTProto `Message.Entities`. Each entry has `type` (Bot API naming: `bold`, `italic`, `code`, `pre`, `text_url`, `url`, `mention`, `hashtag`, etc.), `offset` and `length` in UTF-16 code units, plus optional `url`/`language`/`userId` for types that carry metadata. Plain messages omit the field entirely.

### Parse mode (formatting on write)

Tools that send or edit text (`messages_send`, `messages_edit`, `messages_send_file`, `media_send_album`) REQUIRE `parseMode` — no default, deliberately: optional formatting params get systematically omitted by LLM callers and markdown then ships as literal asterisks. The choice is enforced at the protocol layer — each tool's input schema carries `enum: [plain, commonmark]` (built by `inputSchemaWithEnum` in `tools/register.go`, which infers the schema exactly as `mcp.AddTool` would and patches one property), validated by the SDK before the handler runs. The schema enum is strict lowercase; `normalizeParseMode`'s case-insensitivity survives only as defense in depth for direct handler calls (tests). `TestParseModeSchema_RequiredEnumOnTheWire` pins required+enum on the wire representation.

- `"plain"` — no formatting. Text (or caption) that looks like markdown is REJECTED with `ErrPlainLooksLikeMarkdown` unless `allowRawMarkdown=true`; the lint (`telegram.LooksLikeMarkdown`, `markdown_lint.go`) conservatively matches fences, inline code, doubled-marker emphasis (`**`/`__`/`~~`/`||` with non-whitespace-flanked content), `[text](url)` links and `<https://...>` autolinks, and deliberately excludes single `*italic*`/`_italic_` and `>` quotes as false-positive-prone. `__init__` is a true positive by design — the parser really would transform it.
- `"commonmark"` — CommonMark subset: `**bold**`, `*italic*`, `` `code` ``, ` ```pre``` `, `~~~pre~~~`, 4-space indented code blocks, `[text](url)`, `<https://autolink>`, `> quote`, `>quote` (no space ok), `~~strike~~`, `__underline__`, `||spoiler||`. Parsed into `tg.MessageEntity` on the server side.
- `"markdown"` — RETIRED alias; rejected with a steering error pointing at `commonmark`. The wrapper's `IsCommonMarkParseMode` still recognises it internally as harmless defense.
- `"html"` / `"markdownv2"` — recognised but not yet implemented; return a clear error (direct-handler path only — over MCP the schema enum rejects them first, like every non-enum value).
- `""` (omitted) — rejected by the schema (`required`), and by `ErrParseModeRequired` on the direct-handler path.

All four results carry `entitiesParsed` — the count of FORMATTING entities in the SERVER echo, serialized even at 0. Auto-detected types (`url`, `mention`, `hashtag`, `cashtag`, `bot_command`, `email`, `phone`, and anything unmapped — `IsFormattingEntity` is an ALLOW-list precisely so a future auto-detected kind cannot silently start counting) are excluded: the server adds them to any message regardless of `parseMode`, and counting them reported `entitiesParsed: 2` for a plain send that merely contained a link and a hashtag — verified live. Only the types a parseMode can produce count (0 after a commonmark send means nothing parsed; the caller can self-correct via `tg_messages_edit`). The echo is the source, not a client-side recount: `messageFromUpdate` converts `updateShortSentMessage.Entities`, and `editedMessageFromUpdate` handles `updateEditMessage`/`updateEditChannelMessage` (before that fix `EditMessage` returned nil — an edit-based self-correction loop would have seen 0 forever). **Identity in an echo envelope comes from `random_id`, not from position.** Every send generates a `random_id`, and the server pairs it with the ID it assigned in `updateMessageID` — that pairing is the ONLY identity signal MTProto gives. An envelope can also bundle other messages entirely: a channel post fans out into the linked discussion group, an album's copies land there too, a topic root's reply counter bumps. Taking the first new message would report a stranger's ID, which the caller would then edit, delete or pin. `ownMessageIDs` filters by our `random_id` set (`messageFromUpdate` for single sends, `messagesFromUpdates` for albums and forwards); when the server sends no `updateMessageID`, the old first-match behavior is the fallback. Every path additionally checks the peer, because message IDs are numbered PER PEER: a channel's counter runs close to its linked discussion group's, so the group's copy of our post can carry the very same ID. Matching on the number alone would read the entity count off a stranger, over-count an album, or (on edit) report the group's copy as the post you edited.

The two extractors are deliberately separate: a send echo may carry an edit update for the topic root (its reply counter bumped), and the send path must never mistake that parent for the message it just sent. `editedMessageFromUpdate` additionally matches on the edited ID rather than taking the first edit update — an envelope can bundle edits of other messages (a channel post's edit propagating into the linked discussion group), and the tool keeps the caller's `messageId` rather than deriving it, since Telegram does not renumber an edited message. Albums report the SUM across echoed messages, since the server's update order is not a contract.

**Verified against a live account (2026-07):** a Saved Messages send — precisely the case `updateShortSentMessage` exists for — answers with the full `*tg.Updates` envelope, in both plain and commonmark mode. The short form never fired, so whether its conditional `entities` flag is set on an unchanged entity set could NOT be observed, and Telegram documents neither. So `shortSentEntities` (`wrapper_helpers.go`) repairs THAT SHAPE ONLY: when the short echo carries no entities, the count falls back to the set the request submitted, which is safe because Telegram rejects malformed entities outright (`ENTITY_BOUND_INVALID` and friends) rather than dropping them silently. The fallback must NOT be widened to the full-updates path: there the server sends back its own message, and a zero means the server really applied nothing — the exact signal `entitiesParsed` exists to carry. Widening it would erase the signal in the name of protecting it.

`messagesFromUpdates` (albums, forwards) shares `extractNewMessage` with the single-message path so scheduled sends (`updateNewScheduledMessage`) are not silently dropped — a scheduled album used to come back with `count: 0`. Of these two callers only albums can be scheduled — `tg_messages_forward` has no `scheduleDate` (`tg_messages_send` and `tg_messages_send_file` do, but they take the single-message path).

On the SINGLE-message paths (`SendMessage`, `EditMessage`, `SendFile`) an echo that cannot be read at all — an unhandled envelope shape, or no update matching the edited ID — is repaired by `echoOrSubmitted` with the submitted entity set. Without it a nil echo reads as `entitiesParsed: 0`, which the documented recipe treats as "the markdown did not parse", and a caller would re-edit correct text forever with nothing erroring. A readable echo always wins, including one that genuinely reports zero.

`SendAlbum` is deliberately NOT repaired: there is no single ID to attach a repaired count to, and the album's anomaly already surfaces as `count: 0` ("Sent album with 0 file(s)"), which is visibly wrong rather than quietly plausible — pinned by `TestMediaSendAlbumHandler_UnreadableEchoReportsNothing`. The single-message repair has its own oddity for symmetry's sake — `entitiesParsed: N` beside `messageId: 0` — which is equally loud: a follow-up edit on id 0 fails outright rather than silently editing the wrong message.

Known CommonMark gaps documented in `docs/messages.md` ("Markdown — Known Limitations"): nested blockquotes (`> > x`), nested emphasis (`**a *b***`), hard line breaks via two trailing spaces or trailing `\`. Each has a commented-out test in `internal/telegram/markdown_audit_test.go`.

### Send-as identity (posting as a channel)

MTProto's `send_as` field appears on exactly five requests, which map onto six tools: `messages.sendMessage`, `messages.sendMedia` (backs both `tg_messages_send_file` and `tg_stickers_send`), `messages.sendMultiMedia`, `messages.forwardMessages`, `messages.createForumTopic`. All six take an optional `sendAs` string.

Wiring: `SendOpts.SendAs *InputPeer` for the three methods that already had an options struct; a trailing `sendAs *InputPeer` argument for `ForwardMessages` / `CreateForumTopic` / `SendSticker`. `applySendAs(sendAs, req.SetSendAs)` (`wrapper_helpers.go`) sets the conditional field for all five request types via the method value — do NOT inline the `if sendAs != nil` check per call site, `dupl` will flag it.

`nil` does NOT mean "post as the account". It leaves the flag clear, and the server then applies the chat's saved default — verified on a live account: with a channel set as the chat default, a send with no `send_as` posts as that channel. This matches the official clients. Do not "fix" it by substituting `inputPeerSelf`: that would make `tg_chats_set_send_as` govern reactions but not messages, which is neither Telegram's model nor a useful one.

`resolveSendAs` (`tools/helpers.go`) resolves the string and rejects a `PeerChannel` whose `AccessHash` is 0. That state is reachable and silent: `resolveByID` returns it with a nil error for any numeric ID the client has never seen, and the resulting server error names neither the parameter nor the fix. `tg_chats_get_send_as` seeds the peer cache from the `Chats`/`Users` of its own reply, which is what makes numeric IDs work afterwards.

`tg_chats_set_send_as` wraps `messages.saveDefaultSendAs`. Omitting `sendAs` there is not a missing argument — it resets the chat to the account (`&tg.InputPeerSelf{}` in the wrapper, since the domain `InputPeer` cannot express self). The default is account-wide server state and the only way to react or vote in a poll as a channel; `messages.sendReaction` and `messages.saveDraft` have no `send_as` field at all. `tg_groups_info` reports the current default from `ChannelFull.DefaultSendAs` at no extra RPC cost.

Both new tools reject non-`PeerChannel` peers before the round trip (`telegram.ErrSendAsUnsupportedPeer`); the server's `CHANNEL_INVALID` explains nothing.

Posting as yourself for a single message, in a chat whose default is a channel, works: pass your own numeric ID as `sendAs`. It resolves with an access hash because the account always has a Saved Messages dialog, so `resolveSendAs` accepts it and `InputPeerUser` is what reaches the wire. Verified end-to-end. `inputPeerSelf` is therefore needed only by `SetDefaultSendAs`, where the identity is absent rather than resolved.

Verified against a live account: a rejected identity comes back as `CHAT_ADMIN_REQUIRED` (channel you don't administrate) or `CHAT_WRITE_FORBIDDEN` (foreign user), NOT `SEND_AS_PEER_INVALID` — that code exists in the schema but the server rarely reaches for it. Both read as a chat-permission problem, so `sendErr` (`tools/errors.go`) names `sendAs` as a suspect whenever one was supplied. Do not "simplify" the six send tools back to plain `telegramErr`.

Also verified: a channel that is the chat default reacts as itself, and `messages.getMessageReactionsList` returns its title in `Chats`, not `Users` — `extractReactionUsers` reads both and carries `ReactionUser.PeerType`.

### Sticker documents (`sticker_cache.go`)

`inputDocument` needs id + access hash + file reference. Only the id is public and stable; the other two arrive with `messages.getStickerSet` and cannot be derived. `GetStickerSet` therefore seeds `StickerCache`, and `SendSticker` looks the document up there, failing with `ErrStickerNotCached` before the RPC when it is absent. Sending a bare id (which is what the code did until this cache existed) answers `MEDIA_EMPTY` on every call. Same shape as the send-as peer cache: read the listing tool once, then the id works.

**`stickerFileId` is a string, and must stay one.** `mcp/tool.go` in go-sdk v1.6.1 unmarshals tool arguments into `map[string]any`, calls `ApplyDefaults`, then re-marshals — so every JSON number passes through `float64`. `internal/json` wraps `segmentio/encoding/json` without `UseNumber`, so there is no escape hatch. A sticker document id needs 63 bits; the mantissa holds 53. Verified end-to-end: `5181593617004757506` reached the wrapper as `5181593617004758000`. Any future tool parameter above 2^53 must be a decimal string for the same reason — this is not paranoia, `stickerFileId` was silently broken by it. Telegram user IDs are well under the limit and are safe as numbers.

### Telegram protocol details

- **RandomID**: All send operations (message, file, album, forward, sticker) generate crypto-random IDs for deduplication
- **FLOOD_WAIT**: `newFloodWaitMiddleware(logger)` auto-retries up to 3 times with the server-specified delay, logging each retry at WARN with `retryAfter` (and a WARN if the context is cancelled mid-backoff). A FLOOD_WAIT that survives all retries reaches the tools layer, where `wrapTelegramError` marks it `ErrFloodWait` with a readable "retry after Ns" (not a raw code, not JSON) — the server stays up, so the caller can back off and retry
- **Peer cache**: Access hashes from username resolution, dialog listing, and `channels.getSendAs` are cached in memory

## Release and distribution

`release.yml` fires on a `v*` tag: it builds and signs the multi-arch container, creates the GitHub release with generated notes, and then a final job runs GoReleaser (`.goreleaser.yaml`) to upload the binary archives (darwin/linux/windows × amd64/arm64) and publish the Homebrew formula to `lexfrei/homebrew-tap`. `release.mode: keep-existing` — the mode governs the release NOTES, not the artifacts, and `append` would stack a second, GoReleaser-generated changelog under the handwritten one on every release.

The two channels must inject the same version strings: the container passes `v{{version}}` and the full SHA (that is what `docker/metadata-action` emits), so GoReleaser's ldflags use `v{{.Version}}` and `{{.FullCommit}}` too. Otherwise one release introduces itself two different ways over the MCP handshake and in Telegram's device list. `TestLdflagsSymbols_ExistInMain` pins the symbol names both files inject against the vars in `main.go` — the linker ignores `-X` for a symbol that does not exist WITHOUT a word, so a rename would leave every release reporting `dev`.

`replace_existing_artifacts: true` is what makes the release job re-runnable. Asset upload is the only non-idempotent step in the publish chain (the release edit and the tap push are both upserts), it answers 422 `already_exists`, and GoReleaser treats that as unrecoverable — so a re-run after a failed tap push, the obvious recovery, would die on the first archive and never reach the tap it was re-run for.

`pr.yml` runs `goreleaser release --snapshot --clean` as a gate (a snapshot implies `--skip=publish`; signing is skipped only for fork PRs, which cannot mint an OIDC token). Without it the release config would first execute on the tag, in the LAST job — after the container is pushed and the release is public — so a broken template would leave a published release with no binaries and no formula. `goreleaser check` cannot be the gate: it exits non-zero on the deliberate `brews` deprecation.

The formula ships an env file at `etc/mcp-tg/mcp-tg.env` and a wrapper in `libexec` that sources it, and the service runs the WRAPPER. This is not decoration: a service manager injects only the variables the unit declares, credentials cannot live in a public formula, and `launchctl setenv` dies with the login session — so a service running the bare binary comes up unconfigured after every reboot and `keep_alive` restarts it forever. The PR gate also runs `ruby -c` over the generated formula: `install`/`service`/`test`/`caveats` are hand-written Ruby pasted into YAML, GoReleaser substitutes them as strings without parsing, so a missing `end` writes a broken formula and still exits 0 — every `brew install` would then fail with a SyntaxError. The formula's `test do` block asserts three things at once (terminates without touching the network, exit code 1, the `TELEGRAM_APP_ID is required` sentence); `TestBrewFormulaContract_NoCredentialsExitsOneWithoutNetwork` pins all three from this repo, because the tap is where a break would surface and nobody reads it.

The tap gets a FORMULA, not a cask, which is the deprecated path in GoReleaser (`goreleaser check` fails on it — the release command only warns). The reason is that both replacements cost something the daemon needs: casks are macOS-only, and they have no `brew services`. A formula gives Linux and `brew services start mcp-tg` (launchd on macOS, systemd on Linux), which is how the shared HTTP daemon is meant to run. Homebrew itself still supports formulae; only GoReleaser's generator is retiring them. Windows gets archives but no tap: Homebrew has no native Windows support at all.

`HOMEBREW_TAP_GITHUB_TOKEN` (a fine-grained PAT scoped to the tap, Contents: read and write) must exist as a repository secret, and `preflight` verifies it can actually PUSH, by probing `info/refs?service=git-receive-pack` — the first exchange of a real `git push`, which mutates nothing. `GET /repos` will NOT do: its `permissions` field reports the authenticated user's ROLE in the repository, not the token's grants, and a fine-grained PAT can read any public repo regardless of its scope — so a read-only token would sail through. A non-empty secret is likewise not the interesting case: a fine-grained PAT expires within a year, and an expired one is still non-empty. A missing secret expands to an empty string, GoReleaser builds a GitHub client with an empty token, and the tap push fails. The snapshot gate cannot catch that — it never publishes — so a `preflight` job checks the secret before ANY job runs: the container push and the public release both happen after it, and a failure there would otherwise land on top of an already-published release.

A pre-release tag (`v1.3.0-rc1`) must not move `latest` anywhere. FIVE places key off the semver hyphen and must agree: `prerelease:` in `create-release`, `prerelease: auto` in `.goreleaser.yaml`, `enable=` on the `type=raw,value=latest` container tag in EACH of the two metadata steps, and `skip_upload: auto` on `brews` — for Homebrew the formula in the tap IS `latest`, so an RC would otherwise land on every `brew upgrade`. `TestPrereleaseGuards_AllFiveSitesAgree` pins all of them: a drift in any one would make an RC the latest image AND the installed formula, with nothing red anywhere.

The release archives are signed too (keyless cosign over the checksums file), because the container is signed and the notes teach `cosign verify` — a binary that hands out a full-access Telegram session should not be the unsigned half of the same release. The signature is a `.bundle`: cosign v3 IGNORES `--output-signature`/`--output-certificate` and still exits 0, so the pre-v3 flags would have shipped an unsigned release whose notes told people to verify files that were never written. The PR gate therefore asserts the bundle EXISTS rather than trusting cosign's exit code.

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
- `github.com/google/jsonschema-go` — the SDK's schema inference, imported directly by `inputSchemaWithEnum` to constrain `parseMode` to an enum
- `github.com/lexfrei/keychain` — cgo-free OS secret store (macOS Keychain / Linux Secret Service / Windows Credential Manager) for the default session backend; pulls `purego` + `godbus/dbus` indirectly
- `golang.org/x/sync` — errgroup for concurrent transports
- `golang.org/x/term` — no-echo TTY password read in `mcp-tg login`

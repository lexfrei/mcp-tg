# mcp-tg

MCP server for Telegram Client API (MTProto). Provides 78 tools, 4 resources, 3 prompts, and argument completions for comprehensive Telegram account management.

Uses [gotd/td](https://github.com/gotd/td) for MTProto protocol — this is a **user account** client, not a bot.

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
| Transports | stdio + HTTP/SSE |
| KeepAlive | 30s ping interval |
| Middleware | Auth guard, request logging, bool coercion |

## Telegram Protocol Features

- **Peer cache** — resolved peers with access hashes are cached in memory, so numeric ID lookups reuse valid hashes instead of failing
- **Invite links** — `t.me/+hash` and `t.me/joinchat/hash` are resolved via `messages.checkChatInvite`
- **FLOOD_WAIT retry** — automatic sleep and retry (up to 3 times) when Telegram rate-limits the client
- **Connection re-init** — when the server forgets a long-lived connection's `initConnection` state and answers `CONNECTION_LAYER_INVALID` / `CONNECTION_NOT_INITED`, the request is retried once wrapped in `initConnection`, recovering the connection in place
- **Auth guard** — tool calls are blocked with a clear error until Telegram authentication completes
- **Pagination** — `offsetDate` for dialog listing, `offsetId` for message search and history; `tg_messages_list` can additionally filter by message `type`

## Tools (78 registered; 67 listed below)

The categorised list below documents 67 of the 78 registered tools — the remaining 11 are wired in `cmd/mcp-tg/main.go` but have not been written up in this file yet. See the source for the full surface area.

### Messages (12)

- `tg_messages_list` — List messages in a chat
- `tg_messages_get` — Get specific messages by ID
- `tg_messages_context` — Get messages around a specific message
- `tg_messages_search` — Search messages in a chat
- `tg_messages_transcribe_audio` — Transcribe a Telegram voice message or video note by message ID
- `tg_messages_send` — Send a text message
- `tg_messages_edit` — Edit an existing message
- `tg_messages_delete` — Delete messages
- `tg_messages_forward` — Forward messages between chats
- `tg_messages_pin` — Pin or unpin a message
- `tg_messages_react` — Add or remove reactions
- `tg_messages_mark_read` — Mark messages as read

### Dialogs (3)

- `tg_dialogs_list` — List all dialogs
- `tg_dialogs_search` — Search dialogs by query
- `tg_dialogs_get_info` — Get chat/channel metadata

### Contacts & Users (6)

- `tg_contacts_get` — Get contact info
- `tg_contacts_search` — Search contacts
- `tg_users_get` — Get user info
- `tg_users_get_photos` — Get user profile photos
- `tg_users_block` — Block or unblock a user
- `tg_users_get_common_chats` — Get chats shared with a user

### Groups (9)

- `tg_groups_list` — List groups
- `tg_groups_info` — Get group info
- `tg_groups_join` — Join a public channel or supergroup
- `tg_groups_leave` — Leave a group or channel
- `tg_groups_rename` — Rename a group
- `tg_groups_members_add` — Add a member
- `tg_groups_members_remove` — Remove a member
- `tg_groups_invite_link_get` — Get invite link
- `tg_groups_invite_link_revoke` — Revoke invite link

### Chat Management (10)

- `tg_chats_create` — Create a new group or channel
- `tg_chats_archive` — Archive or unarchive a chat
- `tg_chats_mute` — Mute or unmute notifications
- `tg_chats_delete` — Delete a channel or supergroup
- `tg_chats_set_photo` — Set chat photo
- `tg_chats_set_description` — Set chat description
- `tg_chats_get_admins` — List administrators (channels/supergroups)
- `tg_chats_set_permissions` — Set default permissions
- `tg_chats_get_send_as` — List the identities this account may post under in a chat
- `tg_chats_set_send_as` — Set the identity this account posts under by default in a chat

### Media & Files (4)

- `tg_messages_send_file` — Send a file with caption
- `tg_media_download` — Download media from a message
- `tg_media_upload` — Upload a file
- `tg_media_send_album` — Send a media album

### Profile (4)

- `tg_profile_get` — Get own profile info
- `tg_profile_set_name` — Update display name
- `tg_profile_set_bio` — Update bio
- `tg_profile_set_photo` — Set profile photo

### Forum Topics (2)

- `tg_topics_list` — List forum topics
- `tg_topics_search` — Search forum topics

### Stickers (3)

- `tg_stickers_search` — Search sticker sets
- `tg_stickers_get_set` — Get a sticker set
- `tg_stickers_send` — Send a sticker

### Drafts (2)

- `tg_drafts_set` — Set a draft message
- `tg_drafts_clear` — Clear a draft

### Folders (4)

- `tg_folders_list` — List chat folders
- `tg_folders_create` — Create a folder
- `tg_folders_edit` — Edit a folder
- `tg_folders_delete` — Delete a folder

### Status (2)

- `tg_typing_send` — Send typing indicator
- `tg_online_status_set` — Set online/offline status

### Server (1)

- `tg_server_version` — Get build metadata (semver tag, git commit SHA, Go runtime version); reachable before authentication completes

## Peer Identifier Format

Every tool that surfaces a peer (sender, forward author, reply target, dialog, group/channel info, contact, user, reaction) uses the **same identifier shape in both text output and JSON**:

- Text form: `Display Name [@username]` / `[user:N]` / `[channel:N]` / `[group:N]` / `[hidden]` / `[unknown:N]`
- JSON form: every peer-bearing entry carries `{id, type, name, username}` where `type` is one of `"user"` / `"channel"` / `"group"` / `"unknown"`

The `"group"` label covers only legacy basic groups (MTProto `PeerChat`). Supergroups and broadcast channels both label as `"channel"` because gotd represents both as `PeerChannel`. A consumer can pivot between text and JSON surfaces by pattern-matching on the same literal `kind:N` form (`group:42` appears identically in `[group:42]` text output and `participants[].type="group"` + `id=42` JSON).

This applies uniformly across:

- `tg_messages_*` — sender, forwarded-from origin, cross-chat reply target, participants
- `tg_dialogs_*` — dialog title + username
- `tg_users_*`, `tg_contacts_*` — user display name + @handle
- `tg_groups_info` / `tg_groups_list` / `tg_groups_members_list` / `tg_chats_admins` — group/channel titles, member display names
- `tg_messages_get_reactions` — reactor display name + @handle
- `tg://chat/{peer}/messages` resource — same multi-line block format as `tg_messages_*`

## Message Output Format

**Breaking changes in this release.** Both text and JSON outputs change shape.

- **Text** — `output` is no longer a one-line-per-message string with a `[ID ↩parent] ts sender: text` shape. Each message is now a multi-line block separated by a `---` line. The `↩<parent>` marker and the `[<media>]` inline prefix are gone — reply targets land on a dedicated `reply to:` line and every message carries a dedicated `type:` line. Dialog rows in `tg_dialogs_*` switch from `[type] Title` to the unified `Title [@username/user:N/...]` identifier shape.
- **JSON** — `MessageItem` gains `type`, `fromType`, `fromUsername`, and `forward`; the legacy `mediaType` field is removed. `DialogItem` gains `username`. `ParticipantItem` gains `type` and `username`. `ReactionUserItem.userName` is removed in favour of separate `name` and `username` fields. `ContactStatusItem` gains the same `name`/`username` slots (currently empty pending a follow-up upstream lookup). `InputPeer.accessHash` becomes `omitempty` — a zero value (the common case for peers from forwarded-message headers or numeric IDs) now disappears from JSON rather than serializing as `"accessHash":0`.

Any consumer that parsed the previous formats must be updated.

Read tools (`tg_messages_list`, `tg_messages_get`, `tg_messages_context`, `tg_messages_search`) return both a JSON `messages` array and a human-readable `output` string. Each message in `output` is a multi-line block; blocks are separated by a literal `---` line so a message body containing its own blank lines stays unambiguous.

```text
[<id>] <ISO-timestamp>
from: Display Name [@username]
forwarded from: Display Name [@username] at <ISO-timestamp>
forwarded from channel: Title [@username] #<post> by "<author>" at <ISO-timestamp>
reply to: <parentId>
reply to: <parentId> in Display Name [@username]
quote: «<quoted fragment>»
type: <message type>
text:
<message body, multi-line preserved>
```

The `type:` line is always emitted. Values are `text`, `photo`, `voice`, `video_note`, `video`, `audio`, `sticker`, `animation`, `document`, `contact`, `location`, `venue`, `poll`, `webpage`, `game`, `invoice`, or `unsupported`. `tg_messages_list` accepts the same values in optional `type` to return only matching messages; it paginates internally until the requested `limit` of matching messages is reached or history ends.

Lines other than `type:` are emitted only when their underlying field is populated. A message body that contains a literal `---` line on its own (rare — typically a Markdown horizontal rule) collides with the block separator, so parse-back from `output` is not strictly round-trip safe. The same caveat applies to **adversarial content**: peer names and usernames are sanitized to stop newline-injection from forging fake `from:` / `reply to:` lines, but the body itself is rendered verbatim. A user can craft a body containing `\n---\n[999] ts\nfrom: Admin\ntext:\nfake` and the rendered output will look like two messages. The JSON `messages` array is the authoritative shape — body verbatim preservation is a deliberate UX choice for code blocks and quoted text, not a security property. Every peer reference — sender, forwarded-from origin, cross-chat reply target — uses the same identifier shape:

- `Display Name [@username]` — public username available; the numeric ID is dropped from the text form for brevity (it's still in the JSON `messages[].fromId`, `forward.from.peer.id`, etc.)
- `Display Name [user:N]` / `[channel:N]` / `[group:N]` — username not exposed, only ID (labels match `participants[].type` and `fromType` exactly)
- `Display Name [hidden]` — name leaked through but the peer ID is privacy-protected; surfaces on `forwarded from:` and `reply to: ... in` lines (typical when the original author enabled forward-privacy). The `from:` sender line is omitted entirely when both name and ID are absent, since the message host already identifies the chat.
- `[user:N]` / `[hidden]` — degenerate forms when display name is also missing
- `[unknown:N]` — defensive fallback when a `PeerType` value is outside the three documented kinds; surfaces only if a future MTProto schema bump introduces a fourth peer kind and the response carries it before this client adds a branch

When `forward.from` is present, `name` and `username` may still be empty if the response's `Users[]`/`Chats[]` arrays did not include the resolved peer — text output then falls back to the bare `[user:N]` / `[channel:N]` form, and JSON exposes `{peer: {type, id}}` without `name`/`username`. Use the peer ID to look the entity up via `tg_users_get` or `tg_dialogs_search` when names are needed.

JSON adds `forward` with structured fields (`from.peer`, `from.name`, `from.username`, `fromName` for privacy-hidden, `date`, `channelPost`, `postAuthor`), `fromUsername` and `fromType` (`"user"` / `"group"` / `"channel"`). The mapping is: `"user"` for regular senders, `"group"` for legacy basic groups only, `"channel"` for both broadcast channels AND supergroups (MTProto represents both as `PeerChannel`). Original authors of forwarded messages are also included in the `participants` array, which carries that `type` string alongside the bare ID so a user and a channel sharing the same numeric ID survive deduplication as distinct entries.

Deep-links to the original message can be constructed from `forward.channelPost` and `forward.from`:

- Public channel: `https://t.me/<username>/<channelPost>`
- Private channel: `https://t.me/c/<from.peer.id>/<channelPost>`

**`accessHash` is omitted from every serialized `InputPeer` shape when zero** — that includes `messages[].peerId`, `messages[].replyTo.fromPeerId`, and `messages[].forward.from.peer`. The omission is deliberate: a zero hash looks like a valid one to MTProto but raises `PEER_ID_INVALID` when passed back. For follow-up tool calls against a peer whose `accessHash` field is missing, resolve it through `@username` (if exposed) or look it up via `tg_dialogs_list` to obtain a usable access hash.

`tg_messages_search_global` is an exception — its `output` is a one-line summary; per-message structure lives only in the JSON `messages` array because results span arbitrary peers. It does NOT return a `participants` field (the per-peer `accessHash` resolution would be unreliable across arbitrary chats). Callers that need to act on a sender or forward-author surfaced by global search must first resolve the peer via `@username` (if present in `messages[].fromUsername`) or `tg_dialogs_list` before passing it back into MTProto — `accessHash` on the embedded `InputPeer` will be omitted whenever it is unknown.

## Markdown — Known Limitations

The CommonMark subset supported via `parseMode: "commonmark"` covers most everyday formatting, but a handful of CommonMark spec features are intentionally not implemented. Each is captured as a commented-out test in `internal/telegram/markdown_audit_test.go` ready to be unblocked when work begins.

- **Nested blockquotes** (`> > x`). The inner `>` is treated as literal content of the outer blockquote rather than producing a nested level. Telegram renders any depth of `>` as a single quote bar visually, so the practical loss is minor.
- **Nested emphasis** (`**bold *italic***`). The inner italic is dropped and its asterisks are kept as literal characters. Implementing the full delimiter-run algorithm from CommonMark §6.4 would be a rewrite of the inline parser.
- **Hard line breaks via two trailing spaces or `\`** (CommonMark §6.7). Telegram has no break entity; a plain `\n` already renders as a line break. Stripping the trailing whitespace would be silent data corruption when the user did not intend a hard break, so the parser passes both forms through unchanged.

## Posting as a channel (`sendAs`)

Telegram lets an account post into a supergroup under the identity of a channel it administrates, rather than under its own name. Six tools take an optional `sendAs` parameter for this: `tg_messages_send`, `tg_messages_send_file`, `tg_media_send_album`, `tg_messages_forward`, `tg_topics_create` and `tg_stickers_send`. Omitting it posts as the account itself.

`tg_chats_get_send_as` lists the identities a chat will accept, together with a `premiumRequired` flag for the ones that need Telegram Premium. It is the authoritative answer — the server decides, not the client — and calling it also caches the access hashes, which is what lets a bare numeric ID be used as `sendAs` for a private channel afterwards. Until then, pass `@username`.

`tg_chats_set_send_as` changes the identity the chat posts under by default, and `tg_groups_info` reports the current one as `defaultSendAs`. Treat the default as account-wide server state: it shows up in every Telegram client, survives restarts, and in a shared daemon another caller sees it too. Prefer the per-call `sendAs` argument wherever it suffices.

What MTProto does not allow, and no amount of client code can add:

- **Reactions and poll votes cannot name an identity per call.** `messages.sendReaction` has no `send_as` field, so those follow the chat default set through `tg_chats_set_send_as`. That is the only lever.
- **Drafts always belong to the account.** `messages.saveDraft` has no `send_as` field, so `tg_drafts_set` is unaffected by any of this.
- **Direct messages and legacy basic groups have no send-as at all.** Both new tools reject those peers before the round trip.
- **Only identities the server offers.** You cannot post as a channel you do not administrate, and `premiumRequired` cannot be bypassed.

## Reactions

`tg_messages_react` adds or removes reactions; `tg_messages_get_reactions` reads who reacted. Both share one encoding for a reaction:

- A standard reaction is the unicode emoji itself (`"👍"`).
- A premium custom-emoji reaction is encoded as `"custom:<document_id>"` (e.g. `"custom:5210952531676504517"`). `tg_messages_get_reactions` emits this exact form, so a reaction read from one message can be sent verbatim to another (read → send round-trip).

`tg_messages_react` parameters:

- `emoji` — a single reaction (standard or `custom:<id>`). Kept for convenience and backward compatibility.
- `emojis` — an array to set several reactions at once. Setting more than one reaction on a message requires Telegram Premium. When both `emoji` and `emojis` are supplied, `emojis` wins.
- `big` — play the large animated reaction.
- `remove` — clear all reactions on the message; `emoji`/`emojis` are ignored.

## Resources

- `tg://dialogs` — List of all dialogs (JSON)
- `tg://profile` — Authenticated user's profile (JSON)
- `tg://chat/{peer}` — Chat/channel metadata (JSON, URI template)
- `tg://chat/{peer}/messages` — Recent messages (text, URI template)

## Prompts

- `reply_to_message` — Fetch context around a message for composing replies
- `summarize_chat` — Fetch recent messages for conversation summarization
- `search_and_reply` — Search messages and prepare reply context

## Peer Resolution

All tools accept `peer` as a string. Supported formats:

- `@username`
- `username` (bare)
- `https://t.me/username`
- `https://t.me/+invite_hash` (invite links, if already joined)
- Numeric ID (bot-API style: positive=user, negative=chat, `-100xxx`=channel)

Peers resolved by username include a valid access hash. Numeric IDs use a cached access hash if available, otherwise AccessHash=0 (some API calls may fail — prefer `@username`).

## Configuration

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

## Authentication

Authentication uses a cascade: environment variable, then MCP elicitation (the client prompts you), then error.

**First run:**

1. Set `TELEGRAM_APP_ID` and `TELEGRAM_APP_HASH` (always required)
2. Optionally set `TELEGRAM_PHONE` — if not set, the server asks via elicitation
3. Telegram sends a code to your device
4. Optionally set `TELEGRAM_AUTH_CODE` — if not set, the server asks via elicitation
5. If 2FA is enabled, optionally set `TELEGRAM_PASSWORD` — or the server asks
6. The session is saved to the OS keychain (or a file with insecure storage — see below)

**Subsequent runs:** the stored session is loaded automatically, no auth needed.

### Session storage

By default the session is stored in the **OS keychain** — macOS Keychain, Linux/\*BSD Secret Service, or Windows Credential Manager — via [github.com/lexfrei/keychain](https://github.com/lexfrei/keychain). No plaintext session file is written. The session is a bearer credential (full account access, already past 2FA), so keeping it out of a loose file protects it from backups, cloud sync, and disk theft. In keychain mode `TELEGRAM_SESSION_FILE` is only the keychain **account key** (its value still distinguishes multiple sessions), not a file that gets created. Because it is the lookup key, `mcp-tg login` and the server must resolve the same value — pin `TELEGRAM_SESSION_FILE` explicitly if their environments might otherwise differ, so both address the same keychain item.

One macOS deployment caveat: a `launchd` **LaunchDaemon** runs in the system security context and reads the *System* keychain, not the *login* keychain that `mcp-tg login` (run as you) writes to — so a system daemon will not find the session no matter how the account key is set. Run the daemon as a **LaunchAgent** (user context, the same login keychain), or use `TELEGRAM_SESSION_INSECURE=true` with a file.

On macOS the item is written through `security(1)` into the stable `apple-tool` access partition, so an unsigned, frequently rebuilt binary — and the headless daemon — keep reading what `mcp-tg login` wrote, with no prompt. Any process of the same user can read it (the same trade `go-keyring` makes): it protects data-at-rest, not against code already running as you.

Where no keychain is reachable — a container, or a headless Linux host with no Secret Service — opt into a plaintext file with `--insecure-storage` (on `mcp-tg login`) or `TELEGRAM_SESSION_INSECURE=true` (for the server/daemon). The two modes must match: an item written to the keychain cannot be read from a file, or vice versa. Without the opt-in, an unreachable keychain fails fast with a clear error instead of silently writing plaintext.

**Upgrading from a file-based session:** earlier versions kept the session in the `~/.mcp-tg/session.json` file. This version reads the keychain by default, so an existing file is no longer picked up — either set `TELEGRAM_SESSION_INSECURE=true` (and pass `--insecure-storage` to `mcp-tg login`) to keep using the file, or run `mcp-tg login` once to move the session into the keychain.

### Logging in

The recommended way to create or refresh the session is the `login` subcommand. It runs an interactive terminal login: the phone, the code, and the 2FA password are read straight from the TTY and never touch MCP, elicitation, a tool call, or any transcript. It writes the session (to the keychain by default) and exits; the server then reuses it.

Native binary:

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
./mcp-tg login
```

Container — note `-it` (an interactive TTY, unlike the `-i` used to run the server). The image defaults to the file backend (a container has no keychain), so the session lands in the mounted volume:

```bash
docker run --rm -it \
  -e TELEGRAM_APP_ID=12345 \
  -e TELEGRAM_APP_HASH=your_app_hash \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest login
```

The login code is delivered by Telegram at runtime, so login is inherently interactive: it needs a real terminal and refuses to run on piped stdin (`docker run` without `-t`). The server's own env → MCP-elicitation cascade still works for a stdio server that prompts through its connected client, but `mcp-tg login` is the only path that works for the headless HTTP daemon.

**Multiple sessions:** by default each Claude Code (or other MCP) client starts its own stdio process — that is how the stdio transport works. This is fine for one or two clients, but every process opens its own MTProto connection on the same auth key and shares the one stored session. Running many at once (5+) wastes connections and risks Telegram flagging the auth key. To share one process across many clients, run the headless HTTP-only daemon described below.

### Shared daemon (HTTP-only)

To serve many MCP clients from a single process and a single Telegram connection, run the server as a headless HTTP-only daemon. Set `MCP_HTTP_ONLY=true` together with `MCP_HTTP_PORT`. In this mode the server skips the stdio transport entirely and listens only on HTTP, multiplexing every connecting client onto the same Telegram session — one MTProto connection and one writer of the stored session regardless of how many clients attach.

Because all clients share that one MTProto connection, they also share its throughput: requests serialize through a single connection, and a FLOOD_WAIT triggered by one client's burst pauses the auto-retry for everyone until the server-specified delay elapses. A good trade for many lightly-used clients, less so for a few high-volume ones.

A headless daemon has no client to prompt through, so it cannot log in itself. Run `mcp-tg login` once, then start the daemon; it reuses the stored session. If the session is missing or revoked, the daemon exits with a message telling you to run `mcp-tg login` — it does not hang or silently fall back to plaintext.

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
export MCP_HTTP_PORT=8787
export MCP_HTTP_ONLY=true
./mcp-tg
```

Point each client at the running daemon over HTTP instead of spawning its own process:

```bash
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

### Recovery — revoked session

Telegram can invalidate a session's auth key server-side at any time — an account security event, or anti-abuse against a user-API session whose exit IP moved (a VPN switch, for example). This is not something the server triggers, and it can happen mid-run on a long-lived daemon. Afterwards every Telegram API call answers `AUTH_KEY_UNREGISTERED` (or a sibling: `SESSION_REVOKED`, `AUTH_KEY_INVALID`, …).

When this happens the server logs one `ERROR` line — `Telegram session revoked — re-login required (run: mcp-tg login)` — instead of a stream of raw 401s, and every subsequent tool call fails fast with an explicit "logged out … run `mcp-tg login`" error rather than an opaque per-tool `AUTH_KEY_UNREGISTERED`. Read-only server-meta tools (`tg_server_version`) still answer, so a client can confirm the daemon itself is alive.

A revoked key can also surface as a fatal connection error rather than a per-call reply — `AUTH_KEY_DUPLICATED` (the same key seen from two places) always does, and the others can when the failure lands during a reconnect. The connection cannot continue, so instead of staying up the daemon exits with the same `run mcp-tg login` guidance; under a supervisor (launchd/systemd) it restarts and exits again until you re-login. Either way the same code list is recognised and the fix is identical.

The daemon cannot re-authenticate itself, and it cannot be fixed from the MCP client — the `/mcp` re-authenticate action does not apply, since this server implements no OAuth. To recover:

1. Stop the daemon.
2. Run `mcp-tg login` in a terminal to refresh the stored session (it prompts for the phone, the code, and the 2FA password if set).
3. Start the daemon again; it reuses the refreshed session.

MTProto connection, migration, and auth-key lifecycle events are logged, so if a revocation recurs the daemon log carries the surrounding context.

## Usage

### With Claude Code (stdio via Docker)

```bash
claude mcp add mcp-tg -- docker run --rm -i \
  -e TELEGRAM_APP_ID \
  -e TELEGRAM_APP_HASH \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

A container has no OS keychain, so the image defaults to the plaintext file backend and reads the session from the mounted volume — the one `mcp-tg login` wrote there (see [Logging in](#logging-in)).

### Direct binary

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
./mcp-tg
```

### Container

```bash
docker run --rm -i \
  -e TELEGRAM_APP_ID=12345 \
  -e TELEGRAM_APP_HASH=your_app_hash \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

## Requirements

- Go 1.26.1+
- Telegram API credentials from [my.telegram.org](https://my.telegram.org)

## Building

```bash
go build ./cmd/mcp-tg
```

```bash
docker build --file Containerfile --tag mcp-tg .
```

## License

BSD 3-Clause License

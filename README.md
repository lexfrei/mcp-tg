# mcp-tg

MCP server for Telegram Client API (MTProto). Provides 58 tools, 4 resources, 3 prompts, and argument completions for comprehensive Telegram account management.

Uses [gotd/td](https://github.com/gotd/td) for MTProto protocol — this is a **user account** client, not a bot.

## MCP Protocol Support

| Feature | Status |
| --- | --- |
| Tools | 58 tools with annotations (read-only / idempotent / write / destructive) |
| Resources | 4 (dialogs, profile, chat info, chat messages) |
| Prompts | 3 (reply, summarize, search and reply) |
| Completions | Peer argument autocompletion from dialogs |
| Elicitation | Auth flow (phone, code, 2FA password) |
| Progress | File uploads, media albums, message search |
| Roots | File path validation for uploads/downloads |
| Transports | stdio + HTTP/SSE |
| KeepAlive | 30s ping interval |
| Middleware | Auth guard, request logging |

## Telegram Protocol Features

- **Peer cache** — resolved peers with access hashes are cached in memory, so numeric ID lookups reuse valid hashes instead of failing
- **Invite links** — `t.me/+hash` and `t.me/joinchat/hash` are resolved via `messages.checkChatInvite`
- **FLOOD_WAIT retry** — automatic sleep and retry (up to 3 times) when Telegram rate-limits the client
- **Auth guard** — tool calls are blocked with a clear error until Telegram authentication completes
- **Pagination** — `offsetDate` for dialog listing, `offsetId` for message search and history

## Tools (59)

### Messages (11)

- `tg_messages_list` — List messages in a chat
- `tg_messages_get` — Get specific messages by ID
- `tg_messages_context` — Get messages around a specific message
- `tg_messages_search` — Search messages in a chat
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

### Chat Management (8)

- `tg_chats_create` — Create a new group or channel
- `tg_chats_archive` — Archive or unarchive a chat
- `tg_chats_mute` — Mute or unmute notifications
- `tg_chats_delete` — Delete a channel or supergroup
- `tg_chats_set_photo` — Set chat photo
- `tg_chats_set_description` — Set chat description
- `tg_chats_get_admins` — List administrators (channels/supergroups)
- `tg_chats_set_permissions` — Set default permissions

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

## Message Output Format

**Breaking change in this release.** The human-readable `output` field returned by read tools is no longer a one-line-per-message string with a `[ID ↩parent] ts sender: text` shape. Each message is now a multi-line block, and blocks are separated by a `---` line. The `↩<parent>` marker and the `[<media>]` inline prefix are gone — reply targets land on a dedicated `reply to:` line and media type on a dedicated `media:` line. Any consumer that parsed the previous format must be updated.

Read tools (`tg_messages_list`, `tg_messages_get`, `tg_messages_context`, `tg_messages_search`) return both a JSON `messages` array and a human-readable `output` string. Each message in `output` is a multi-line block; blocks are separated by a literal `---` line so a message body containing its own blank lines stays unambiguous.

```text
[<id>] <ISO-timestamp>
from: Display Name [@username]
forwarded from: Display Name [@username] at <ISO-timestamp>
forwarded from channel: Title [@username] #<post> by "<author>" at <ISO-timestamp>
reply to: <parentId>
reply to: <parentId> in Display Name [@username]
quote: «<quoted fragment>»
media: <type>
text:
<message body, multi-line preserved>
```

Lines are emitted only when their underlying field is populated. A message body that contains a literal `---` line on its own (rare — typically a Markdown horizontal rule) collides with the block separator, so parse-back from `output` is not strictly round-trip safe; the JSON `messages` array is the authoritative shape. Every peer reference — sender, forwarded-from origin, cross-chat reply target — uses the same identifier shape:

- `Display Name [@username]` — public username available; the numeric ID is dropped from the text form for brevity (it's still in the JSON `messages[].fromId`, `forward.from.peer.id`, etc.)
- `Display Name [user:N]` / `[channel:N]` / `[group:N]` — username not exposed, only ID (labels match `participants[].type` and `fromType` exactly)
- `Display Name [hidden]` — name leaked through but the peer ID is privacy-protected; surfaces on `forwarded from:` and `reply to: ... in` lines (typical when the original author enabled forward-privacy). The `from:` sender line is omitted entirely when both name and ID are absent, since the message host already identifies the chat.
- `[user:N]` / `[hidden]` — degenerate forms when display name is also missing

JSON adds `forward` with structured fields (`from.peer`, `from.name`, `from.username`, `fromName` for privacy-hidden, `date`, `channelPost`, `postAuthor`), `fromUsername` and `fromType` (`"user"` / `"group"` / `"channel"`). The mapping is: `"user"` for regular senders, `"group"` for legacy basic groups only, `"channel"` for both broadcast channels AND supergroups (MTProto represents both as `PeerChannel`). Original authors of forwarded messages are also included in the `participants` array, which carries that `type` string alongside the bare ID so a user and a channel sharing the same numeric ID survive deduplication as distinct entries.

Deep-links to the original message can be constructed from `forward.channelPost` and `forward.from`:

- Public channel: `https://t.me/<username>/<channelPost>`
- Private channel: `https://t.me/c/<from.peer.id>/<channelPost>`

**`accessHash` is omitted from every serialized `InputPeer` shape when zero** — that includes `messages[].peerId`, `messages[].replyTo.fromPeerId`, and `messages[].forward.from.peer`. The omission is deliberate: a zero hash looks like a valid one to MTProto but raises `PEER_ID_INVALID` when passed back. For follow-up tool calls against a peer whose `accessHash` field is missing, resolve it through `@username` (if exposed) or look it up via `tg_dialogs_list` to obtain a usable access hash.

`tg_messages_search_global` is an exception — its `output` is a one-line summary; per-message structure lives only in the JSON `messages` array because results span arbitrary peers. Its `participants` entries (sender peers AND any forward-author peers) carry the bare ID without an `accessHash` even more often than other tools, because the response set is unconstrained by the caller's resolved-peer cache. Resolve such peers via `@username` (if present) or `tg_dialogs_list` before passing them back into MTProto.

## Markdown — Known Limitations

The CommonMark subset supported via `parseMode: "commonmark"` covers most everyday formatting, but a handful of CommonMark spec features are intentionally not implemented. Each is captured as a commented-out test in `internal/telegram/markdown_audit_test.go` ready to be unblocked when work begins.

- **Nested blockquotes** (`> > x`). The inner `>` is treated as literal content of the outer blockquote rather than producing a nested level. Telegram renders any depth of `>` as a single quote bar visually, so the practical loss is minor.
- **Nested emphasis** (`**bold *italic***`). The inner italic is dropped and its asterisks are kept as literal characters. Implementing the full delimiter-run algorithm from CommonMark §6.4 would be a rewrite of the inline parser.
- **Hard line breaks via two trailing spaces or `\`** (CommonMark §6.7). Telegram has no break entity; a plain `\n` already renders as a line break. Stripping the trailing whitespace would be silent data corruption when the user did not intend a hard break, so the parser passes both forms through unchanged.

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
| `TELEGRAM_SESSION_FILE` | Session file path | `~/.mcp-tg/session.json` | No |
| `TELEGRAM_AUTH_CODE` | One-time auth code | — | No (prompted via elicitation) |
| `TELEGRAM_DOWNLOAD_DIR` | Media download directory | `/tmp/mcp-tg/downloads` | No |
| `MCP_HTTP_PORT` | HTTP/SSE transport port | disabled | No |
| `MCP_HTTP_HOST` | HTTP bind address | `127.0.0.1` | No |

## Authentication

Authentication uses a cascade: environment variable, then MCP elicitation (the client prompts you), then error.

**First run:**

1. Set `TELEGRAM_APP_ID` and `TELEGRAM_APP_HASH` (always required)
2. Optionally set `TELEGRAM_PHONE` — if not set, the server asks via elicitation
3. Telegram sends a code to your device
4. Optionally set `TELEGRAM_AUTH_CODE` — if not set, the server asks via elicitation
5. If 2FA is enabled, optionally set `TELEGRAM_PASSWORD` — or the server asks
6. Session is saved to `TELEGRAM_SESSION_FILE`

**Subsequent runs:** session file is loaded automatically, no auth needed.

**Session persistence in containers:** mount a volume for the session file:

```bash
-v ~/.mcp-tg:/home/nobody/.mcp-tg
```

**Multiple sessions:** each Claude Code session starts its own container. This is safe for normal use — Telegram allows multiple MTProto connections with the same auth key. However, avoid running many instances simultaneously (5+), as Telegram may rate-limit or drop connections. Session file writes are rare (only on re-auth or DC migration) so volume sharing is safe in practice.

## Usage

### With Claude Code (stdio via Docker)

```bash
claude mcp add mcp-tg -- docker run --rm -i \
  -e TELEGRAM_APP_ID \
  -e TELEGRAM_APP_HASH \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

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

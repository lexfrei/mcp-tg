# Messages

## Message Output Format

**Breaking changes in v0.13.0.** Both text and JSON outputs changed shape.

- **Text** — `output` is no longer a one-line-per-message string with a `[ID ↩parent] ts sender: text` shape. Each message is now a multi-line block separated by a `---` line. The `↩<parent>` marker and the `[<media>]` inline prefix are gone — reply targets land on a dedicated `reply to:` line and every message carries a dedicated `type:` line. Dialog rows in `tg_dialogs_*` switch from `[type] Title` to the unified `Title [@username/user:N/...]` identifier shape.
- **JSON** — `MessageItem` gains `type`, `fromType`, `fromUsername`, and `forward`; the legacy `mediaType` field is removed. `DialogItem` gains `username`. `ParticipantItem` gains `type` and `username`. `ReactionUserItem.userName` is removed in favour of separate `name` and `username` fields. `ContactStatusItem` gains the same `name`/`username` slots (currently empty pending a follow-up upstream lookup). `InputPeer.accessHash` becomes `omitempty` — a zero value (the common case for peers from forwarded-message headers or numeric IDs) now disappears from JSON rather than serializing as `"accessHash":0`.

Any consumer that parsed the previous formats must be updated.

Read tools (`tg_messages_list`, `tg_messages_get`, `tg_messages_context`, `tg_messages_search`, `tg_messages_search_global`) return both a JSON `messages` array and a human-readable `output` string. Each message in `output` is a multi-line block; blocks are separated by a literal `---` line so a message body containing its own blank lines stays unambiguous.

Those five tools also accept an optional `format` to trim the response: `full` (default) returns both shapes, `json` omits the `output` string, and `text` omits the `messages` array. The omitted field is dropped from the JSON entirely (both keys are now `omitempty`), so a consumer must tolerate either key being absent — `full` does not guarantee presence either, since a zero-result read omits the empty `messages` array as well. `tg_messages_search_global`'s `output` is only a `Found N of M` summary line, so its `text` shape carries just that summary; read the JSON `messages` for the per-message detail.

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

Lines other than `type:` are emitted only when their underlying field is populated. A message body that contains a literal `---` line on its own (rare — typically a Markdown horizontal rule) collides with the block separator, so parse-back from `output` is not strictly round-trip safe. The same caveat applies to **adversarial content**: peer names and usernames are sanitized to stop newline-injection from forging fake `from:` / `reply to:` lines, but the body itself is rendered verbatim. A user can craft a body containing `\n---\n[999] ts\nfrom: Admin\ntext:\nfake` and the rendered output will look like two messages. The JSON `messages` array is the authoritative shape — body verbatim preservation is a deliberate UX choice for code blocks and quoted text, not a security property. Every peer reference — sender, forwarded-from origin, cross-chat reply target — uses the same identifier shape, described in [Peers](peers.md):

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

## Parse Mode

**Breaking change in v1.2.0.** `parseMode` is now REQUIRED on the four text tools, and the `'markdown'` alias is gone. It shipped in a v1 release by deliberate choice: this repository is consumed as an MCP server (container or built binary), nothing imports its packages, and Go's major-version rule would force a `/v2` module path for no benefit to any consumer. The break is loud where it matters — a call without `parseMode` fails schema validation with a message naming the parameter. The enum is strictly lowercase: `'Commonmark'` is rejected by the schema, not silently normalised as it used to be. Migration: a call that omitted `parseMode` (which meant plain text) must now pass `parseMode: "plain"`; a call passing `'markdown'` must pass `'commonmark'`. Both are rejected by schema validation before the request reaches Telegram, so the failure is loud rather than silent. Plain-mode text that looks like markdown is also rejected now — see the lint below.

The four text tools (`tg_messages_send`, `tg_messages_edit`, `tg_messages_send_file`, `tg_media_send_album`) require `parseMode` on every call — `'plain'` or `'commonmark'`, no default. The input schema carries the enum, so a call without a mode (or with the retired `'markdown'` alias) is rejected before it reaches Telegram.

In plain mode, text or captions that look like markdown — code fences, `` `inline code` ``, `**bold**`, `[text](url)`, `<https://autolink>`, `__underline__`, `~~strike~~`, `||spoiler||` — are rejected with "text looks like markdown; pass parseMode='commonmark' to format it, or set allowRawMarkdown=true to send the characters literally". Set `allowRawMarkdown: true` to intentionally send such characters unformatted; it applies to plain mode only and is rejected elsewhere rather than silently ignored. Doubled markers and links only count when they open a word, so ordinary code prose passes untouched — `if (a||b)`, `2**3**2` and `Foo[T](x)` are not markdown. Backticks, fences and `<autolink>` brackets trigger wherever they appear, since they mean nothing else. Single `*italic*`/`_italic_` and `>` quotes never trigger the lint.

`tg_drafts_set` takes no `parseMode` at all: a draft is stored as plain text and its markdown is never parsed. That gap is tracked separately from this contract.

Every result reports `entitiesParsed` — how many FORMATTING entities the sent message carries (bold, code, `[text](url)` links, …), present even when 0. Entities Telegram detects on its own — a bare URL, an `@mention`, a `#hashtag` — are excluded, so they cannot masquerade as parsed markdown in a plain send. The self-correction recipe: if a `commonmark` send returns `entitiesParsed: 0` despite formatting in the text, the markdown did not parse — fix the text and call `tg_messages_edit` with `parseMode: "commonmark"`.

## Markdown — Known Limitations

The CommonMark subset supported via `parseMode: "commonmark"` covers most everyday formatting, but a handful of CommonMark spec features are intentionally not implemented. Each is captured as a commented-out test in `internal/telegram/markdown_audit_test.go` ready to be unblocked when work begins.

- **Nested blockquotes** (`> > x`). The inner `>` is treated as literal content of the outer blockquote rather than producing a nested level. Telegram renders any depth of `>` as a single quote bar visually, so the practical loss is minor.
- **Nested emphasis** (`**bold *italic***`). The inner italic is dropped and its asterisks are kept as literal characters. Implementing the full delimiter-run algorithm from CommonMark §6.4 would be a rewrite of the inline parser.
- **Hard line breaks via two trailing spaces or `\`** (CommonMark §6.7). Telegram has no break entity; a plain `\n` already renders as a line break. Stripping the trailing whitespace would be silent data corruption when the user did not intend a hard break, so the parser passes both forms through unchanged.

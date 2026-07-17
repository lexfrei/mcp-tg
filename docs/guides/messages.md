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

Lines other than `type:` are emitted only when their underlying field is populated. A message body that contains a literal `---` line on its own (rare — typically a Markdown horizontal rule) collides with the block separator, so parse-back from `output` is not strictly round-trip safe. The same caveat applies to **adversarial content**: peer names and usernames are sanitized to stop newline-injection from forging fake `from:` / `reply to:` lines, but the body itself is rendered verbatim. A user can craft a body containing `\n---\n[999] ts\nfrom: Admin\ntext:\nfake` and the rendered output will look like two messages. The JSON `messages` array is the authoritative shape — body verbatim preservation is a deliberate UX choice for code blocks and quoted text, not a security property. Every peer reference — sender, forwarded-from origin, reply target — uses the same identifier shape, described in [Peers](peers.md):

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

## Reply Metadata

A message that replies to another carries a structured `replyTo` object on its `messages[]` entry. The field is omitted entirely when the message is not a reply, so its presence is the reply test — there is no zero-value sentinel to check. All five read tools populate it.

| Field | Meaning |
| --- | --- |
| `messageId` | ID of the parent message this one replies to |
| `topId` | ID of the thread root, for a reply inside a forum topic or a comment thread |
| `quoteText` | The fragment of the parent the sender quoted, when they quoted one |
| `fromPeerId` | The parent's chat, when Telegram supplies one — not always, and not only for cross-chat replies; see below |
| `fromName` / `fromUsername` | Advisory display name and username for `fromPeerId` |

`fromPeerId` is **not** a cross-chat flag, and it is not always present on a reply. Telegram populates it for cross-chat replies, documents it for replies inside a discussion thread the account has not joined (where it carries the discussion group's ID), and also sets it on some in-chat quote-replies, where it points straight back at the chat being read. A reply is cross-chat only when `fromPeerId` differs from that chat in peer type or ID — that comparison is this server's own, and it is what decides whether a parent is reachable, not anything Telegram reports. `fromName` and `fromUsername` are advisory in the same sense as everywhere else on this page: they fill only when the response's own `Users[]`/`Chats[]` arrays carried the peer, and stay empty otherwise.

A message that merely sits under a forum topic's root, without replying to any specific message, carries no `replyTo` at all — its thread is reported through the top-level `topicId` instead. Only a real reply target produces the object.

The text `output` renders the same information as the `reply to:` and `quote:` lines described above, including the `reply to: <parentId> in <peer-ref>` form. That form follows `fromPeerId`, not the cross-chat test, so an in-chat quote-reply can render an `in ...` reference pointing back at the chat you are already reading.

### Resolving Parent Messages

`tg_messages_list`, `tg_messages_get`, `tg_messages_context` and `tg_messages_search` accept an optional `resolveReplies` (default `false`). With it on, every reply whose parent is reachable gains a `replyToMessage` object alongside `replyTo`.

| Field | Meaning |
| --- | --- |
| `fromName` | Parent author's display name |
| `fromUsername` | Parent author's username |
| `text` | Parent body, truncated to 200 runes, with an ellipsis appended when it was cut |

Parents already present in the returned batch are attached with no extra request — the flag costs nothing when every parent is already in hand. Only the parents missing from it are fetched, batched at one request per 200 missing parents, which is the ceiling Telegram documents for a single lookup. A page of replies to recent messages therefore costs one extra round-trip. Going beyond that needs a single call to turn up more than 200 distinct out-of-batch parents, which only `tg_messages_list` (`limit`) can do, its ceiling being 1000. The others cannot get there, though for three different reasons. `tg_messages_context` (`radius`) is bounded by the server page size, returning at most 100 messages however wide a window it is asked for. `tg_messages_get` (`ids`) accepts at most 100 `ids` per call, rejected here before the request goes out. `tg_messages_search` (`limit`) is capped at 100 results by Telegram itself — nothing in this server enforces it and the method page does not document it, but a search with `limit: 300` across 2831 matching messages returned exactly 100.

Cross-chat replies are skipped: the parent lives in another chat, and this call holds no access hash for it. `tg_messages_search_global` does not offer `resolveReplies` at all — its results span arbitrary peers, so no single batched lookup is possible. Its messages still carry structural `replyTo`.

The resolver is best-effort. When the parent fetch fails, or the parent is unreachable, `replyToMessage` is simply absent — the call still succeeds and nothing is surfaced as an error. Treat the field as optional even on a request that asked for it.

**`resolveReplies` enriches the JSON only.** The `output` string is rendered from the batch as originally fetched, so it reads identically whether the flag is on or off: the same `reply to:` line, its `in <peer-ref>` form and any `quote:` line the reply already carried, and never a word of the resolved parent. Callers that need the parent's text must read `messages[].replyToMessage`.

Two consequences follow. Combining `resolveReplies: true` with `format: "text"` does nothing at all: that shape drops the `messages` array the enrichment writes into, so the extra request is skipped rather than wasted — it is not an error, it simply has no effect. And because `tg_messages_search_global`'s `output` is only a `Found N of M` summary line, every reply detail from a global search must be read from the JSON `replyTo`.

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

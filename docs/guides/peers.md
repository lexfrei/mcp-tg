# Peers

## Peer Identifier Format

Every tool that surfaces a peer (sender, forward author, reply target, dialog, group/channel info, contact, user, reaction) uses the **same identifier shape in both text output and JSON**:

- Text form: `Display Name [@username]` / `[user:N]` / `[channel:N]` / `[group:N]` / `[hidden]` / `[unknown:N]`
- JSON form: every peer-bearing entry carries `{id, type, name, username}` where `type` is one of `"user"` / `"channel"` / `"group"` / `"unknown"`

The `"group"` label covers only legacy basic groups (MTProto `PeerChat`). Supergroups and broadcast channels both label as `"channel"` because gotd represents both as `PeerChannel`. A consumer can pivot between text and JSON surfaces by pattern-matching on the same literal `kind:N` form (`group:42` appears identically in `[group:42]` text output and `participants[].type="group"` + `id=42` JSON).

This applies uniformly across:

- `tg_messages_*` — sender, forwarded-from origin, reply target, participants
- `tg_dialogs_*` — dialog title + username
- `tg_users_*`, `tg_contacts_*` — user display name + @handle
- `tg_groups_info` / `tg_groups_list` / `tg_groups_members_list` / `tg_chats_get_admins` — group/channel titles, member display names
- `tg_messages_get_reactions` — reactor display name + @handle
- `tg://chat/{peer}/messages` resource — same multi-line block format as `tg_messages_*`

## Peer Resolution

All tools accept `peer` as a string. Supported formats:

- `@username`
- `username` (bare)
- `https://t.me/username`
- `https://t.me/+invite_hash` (invite links, if already joined)
- Numeric ID (bot-API style: positive=user, negative=chat, `-100xxx`=channel)

Peers resolved by username include a valid access hash. A numeric ID reuses a cached access hash when the peer has been seen before; on a cold miss the client warms its cache from the full dialog list — both the main list and the archive — so a numeric channel ID the account belongs to resolves even when the channel is far down the list or archived. A numeric channel that is still unresolved after that (its access hash was never observed) is rejected with a clear error pointing you to open it once via `tg_dialogs_list`, `tg_dialogs_search`, or `@username`, instead of an opaque `CHANNEL_INVALID`. Prefer `@username` when you have it.

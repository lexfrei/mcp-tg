# Tool Reference

Every tool that takes a `peer` accepts the identifier formats described in [Peers](guides/peers.md). Tools that send or edit text require `parseMode` — see [Messages](guides/messages.md).

## Tools (78)

### Messages (16)

- `tg_messages_list` — List messages in a chat
- `tg_messages_get` — Get specific messages by ID
- `tg_messages_context` — Get messages around a specific message
- `tg_messages_search` — Search messages in a chat (optional topic, sender, kind and date filters)
- `tg_messages_search_global` — Search messages across all chats (optional kind, date and dialog-kind filters, cursor pagination)
- `tg_messages_transcribe_audio` — Transcribe a Telegram voice message or video note by message ID
- `tg_messages_send` — Send a text message
- `tg_messages_edit` — Edit an existing message
- `tg_messages_delete` — Delete messages
- `tg_messages_forward` — Forward messages between chats
- `tg_messages_pin` — Pin or unpin a message
- `tg_messages_react` — Add or remove reactions
- `tg_messages_mark_read` — Mark messages as read
- `tg_messages_get_reactions` — List who reacted to a message
- `tg_messages_get_scheduled` — List messages scheduled for later delivery
- `tg_messages_delete_history` — Delete an entire chat history

### Dialogs (5)

- `tg_dialogs_list` — List all dialogs
- `tg_dialogs_search` — Search dialogs by query
- `tg_dialogs_get_info` — Get chat/channel metadata
- `tg_dialogs_pin` — Pin or unpin a dialog
- `tg_dialogs_mark_unread` — Mark a dialog as unread or read

### Contacts & Users (10)

- `tg_contacts_get` — Get contact info
- `tg_contacts_search` — Search contacts
- `tg_users_get` — Get user info
- `tg_users_get_photos` — Get user profile photos
- `tg_users_block` — Block or unblock a user
- `tg_users_get_common_chats` — Get chats shared with a user
- `tg_contacts_add` — Add a contact
- `tg_contacts_delete` — Delete a contact
- `tg_contacts_get_statuses` — Get online statuses of all contacts
- `tg_contacts_list_blocked` — List blocked users

### Groups (12)

- `tg_groups_list` — List groups
- `tg_groups_info` — Get group info
- `tg_groups_join` — Join a public channel or supergroup
- `tg_groups_leave` — Leave a group or channel
- `tg_groups_rename` — Rename a group
- `tg_groups_members_add` — Add a member
- `tg_groups_members_remove` — Remove a member
- `tg_groups_invite_link_get` — Get invite link
- `tg_groups_invite_link_revoke` — Revoke invite link
- `tg_groups_members_list` — List group members
- `tg_groups_admin_set` — Promote or demote an admin with specific rights
- `tg_groups_slowmode` — Set the slowmode delay

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

### Forum Topics (4)

- `tg_topics_list` — List forum topics
- `tg_topics_search` — Search forum topics
- `tg_topics_create` — Create a forum topic
- `tg_topics_edit` — Rename a forum topic

### Stickers (3)

- `tg_stickers_search` — Search sticker sets
- `tg_stickers_get_set` — Get a sticker set
- `tg_stickers_send` — Send a sticker (read its set first, see below)

A sticker is addressed by three numbers, not one: an id, an access hash and a file reference. Only the id is stable and public; the other two arrive with the sticker set. So `tg_stickers_get_set` must be called before `tg_stickers_send` for that set — it caches what the send needs. Sending an id alone would answer `MEDIA_EMPTY`, a code that names neither the sticker nor the remedy, so an uncached sticker is rejected before the request leaves.

`stickerFileId` is a **decimal string**, not a JSON number. The MCP SDK unmarshals tool arguments into `map[string]any` to apply schema defaults, then re-marshals them, so every JSON number round-trips through `float64`. A sticker document id needs 63 bits and a `float64` mantissa holds 53, which silently corrupts it — `5181593617004757506` arrives as `5181593617004758000`. Quote the id and it survives.

### Drafts (3)

- `tg_drafts_set` — Set a draft message
- `tg_drafts_clear` — Clear a draft
- `tg_messages_clear_all_drafts` — Clear all drafts across every chat

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

## Annotations

Every tool carries MCP annotations so a client can reason about its effects before calling it. The four buckets are mutually exclusive and sum to the tool total:

| Bucket | Count | Meaning |
| --- | --- | --- |
| read-only | 31 | Only reads data |
| idempotent | 28 | Modifies state, safe to retry |
| write | 10 | Creates new entities, not idempotent |
| destructive | 9 | Deletes or removes things |

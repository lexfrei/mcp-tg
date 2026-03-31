# mcp-tg

MCP server for Telegram Client API (MTProto). Provides 58 tools covering messages, dialogs, contacts, groups, channels, media, stickers, folders, and user profile management.

Uses [gotd/td](https://github.com/gotd/td) for MTProto protocol — this is a **user account** client, not a bot.

## Features

### Messages (11 tools)

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

### Dialogs (3 tools)

- `tg_dialogs_list` — List all dialogs
- `tg_dialogs_search` — Search dialogs by query
- `tg_dialogs_get_info` — Get chat/channel metadata

### Contacts & Users (6 tools)

- `tg_contacts_get` — Get contact info
- `tg_contacts_search` — Search contacts
- `tg_users_get` — Get user info
- `tg_users_get_photos` — Get user profile photos
- `tg_users_block` — Block or unblock a user
- `tg_users_get_common_chats` — Get chats shared with a user

### Groups (9 tools)

- `tg_groups_list` — List groups
- `tg_groups_info` — Get group info
- `tg_groups_join` — Join a group or channel
- `tg_groups_leave` — Leave a group or channel
- `tg_groups_rename` — Rename a group
- `tg_groups_members_add` — Add a member
- `tg_groups_members_remove` — Remove a member
- `tg_groups_invite_link_get` — Get invite link
- `tg_groups_invite_link_revoke` — Revoke invite link

### Chat Management (7 tools)

- `tg_chats_create` — Create a new group or channel
- `tg_chats_archive` — Archive or unarchive a chat
- `tg_chats_mute` — Mute or unmute notifications
- `tg_chats_delete` — Delete a chat
- `tg_chats_set_photo` — Set chat photo
- `tg_chats_set_description` — Set chat description
- `tg_chats_get_admins` — List administrators
- `tg_chats_set_permissions` — Set default permissions

### Media & Files (4 tools)

- `tg_messages_send_file` — Send a file with caption
- `tg_media_download` — Download media from a message
- `tg_media_upload` — Upload a file
- `tg_media_send_album` — Send a media album

### Profile (4 tools)

- `tg_profile_get` — Get own profile info
- `tg_profile_set_name` — Update display name
- `tg_profile_set_bio` — Update bio
- `tg_profile_set_photo` — Set profile photo

### Forum Topics (2 tools)

- `tg_topics_list` — List forum topics
- `tg_topics_search` — Search forum topics

### Stickers (3 tools)

- `tg_stickers_search` — Search sticker sets
- `tg_stickers_get_set` — Get a sticker set
- `tg_stickers_send` — Send a sticker

### Drafts (2 tools)

- `tg_drafts_set` — Set a draft message
- `tg_drafts_clear` — Clear a draft

### Folders (4 tools)

- `tg_folders_list` — List chat folders
- `tg_folders_create` — Create a folder
- `tg_folders_edit` — Edit a folder
- `tg_folders_delete` — Delete a folder

### Status (2 tools)

- `tg_typing_send` — Send typing indicator
- `tg_online_status_set` — Set online/offline status

## Configuration

| Variable | Description | Default | Required |
| --- | --- | --- | --- |
| `TELEGRAM_APP_ID` | API app_id from my.telegram.org | — | Yes |
| `TELEGRAM_APP_HASH` | API app_hash from my.telegram.org | — | Yes |
| `TELEGRAM_PHONE` | Phone number (E.164 format) | — | Yes (initial auth) |
| `TELEGRAM_PASSWORD` | 2FA password | — | No |
| `TELEGRAM_SESSION_FILE` | Session file path | `~/.mcp-tg/session.json` | No |
| `TELEGRAM_AUTH_CODE` | One-time auth code (headless) | — | No |
| `TELEGRAM_DOWNLOAD_DIR` | Media download directory | `/tmp/mcp-tg/downloads` | No |
| `MCP_HTTP_PORT` | HTTP/SSE transport port | disabled | No |
| `MCP_HTTP_HOST` | HTTP bind address | `127.0.0.1` | No |

## Authentication

On first run, the server authenticates with Telegram:

1. Set `TELEGRAM_APP_ID`, `TELEGRAM_APP_HASH`, and `TELEGRAM_PHONE`
2. Telegram sends a code to your phone
3. Provide the code via `TELEGRAM_AUTH_CODE` env var (headless) or enter it when prompted on stderr
4. If 2FA is enabled, set `TELEGRAM_PASSWORD`
5. Session is saved to `TELEGRAM_SESSION_FILE` for subsequent runs

## Usage

### With Claude Code (stdio via Docker)

```bash
claude mcp add mcp-tg -- docker run --rm -i \
  -e TELEGRAM_APP_ID \
  -e TELEGRAM_APP_HASH \
  -e TELEGRAM_PHONE \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

### Direct binary

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
export TELEGRAM_PHONE=+1234567890
./mcp-tg
```

### Container

```bash
docker run --rm -i \
  -e TELEGRAM_APP_ID=12345 \
  -e TELEGRAM_APP_HASH=your_app_hash \
  -e TELEGRAM_PHONE=+1234567890 \
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

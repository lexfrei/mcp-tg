# Reactions

`tg_messages_react` adds or removes reactions; `tg_messages_get_reactions` reads who reacted. Both share one encoding for a reaction:

- A standard reaction is the unicode emoji itself (`"👍"`).
- A premium custom-emoji reaction is encoded as `"custom:<document_id>"` (e.g. `"custom:5210952531676504517"`). `tg_messages_get_reactions` emits this exact form, so a reaction read from one message can be sent verbatim to another (read → send round-trip).

A reactor is not always a user. When a chat's default identity is a channel (see [Posting as a channel](send-as.md)), reactions from this account are attributed to that channel. `tg_messages_get_reactions` reports each reactor's kind in `type` (`user` / `channel`), because a channel ID and a user ID with the same number are different peers.

`tg_messages_react` parameters:

- `emoji` — a single reaction (standard or `custom:<id>`). Kept for convenience and backward compatibility.
- `emojis` — an array to set several reactions at once. Setting more than one reaction on a message requires Telegram Premium. When both `emoji` and `emojis` are supplied, `emojis` wins.
- `big` — play the large animated reaction.
- `remove` — clear all reactions on the message; `emoji`/`emojis` are ignored.

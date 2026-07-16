# Posting as a channel (`sendAs`)

Telegram lets an account post into a supergroup under the identity of a channel it administrates, rather than under its own name. Six tools take an optional `sendAs` parameter for this: `tg_messages_send`, `tg_messages_send_file`, `tg_media_send_album`, `tg_messages_forward`, `tg_topics_create` and `tg_stickers_send`.

Omitting `sendAs` does **not** force your own account — it lets the server apply the chat's saved default, exactly as the official clients do. That default is your account until `tg_chats_set_send_as` changes it, after which every send that omits `sendAs` posts as the saved identity. Pass `sendAs` explicitly when the identity matters.

`tg_chats_get_send_as` lists the identities a chat will accept, together with a `premiumRequired` flag for the ones that need Telegram Premium. It is the authoritative answer — the server decides, not the client — and calling it also caches the access hashes, which is what lets a bare numeric ID be used as `sendAs` for a private channel afterwards. Until then, pass `@username`.

`tg_chats_set_send_as` changes the identity the chat posts under by default, and `tg_groups_info` reports the current one as `defaultSendAs`. Treat the default as account-wide server state: it shows up in every Telegram client, survives restarts, silently changes what an omitted `sendAs` means, and in a shared daemon another caller sees it too. Prefer the per-call `sendAs` argument wherever it suffices.

When Telegram refuses an identity it does not say so plainly. Posting as a channel you do not administrate answers `CHAT_ADMIN_REQUIRED`; naming a foreign user answers `CHAT_WRITE_FORBIDDEN`. Both read as "you may not write here" even though the destination was fine, so the send tools add the `sendAs` parameter as a suspect whenever one was given.

## What MTProto does not allow

No amount of client code can add these:

- **Reactions and poll votes cannot name an identity per call.** `messages.sendReaction` has no `send_as` field, so those follow the chat default set through `tg_chats_set_send_as`. That is the only lever.
- **Drafts always belong to the account.** `messages.saveDraft` has no `send_as` field, so `tg_drafts_set` is unaffected by any of this.
- **Direct messages and legacy basic groups have no send-as at all.** Both send-as tools reject those peers before the round trip.
- **Only identities the server offers.** You cannot post as a channel you do not administrate, and `premiumRequired` cannot be bypassed.

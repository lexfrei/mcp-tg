# Resources and Prompts

## Resources

- `tg://dialogs` — List of all dialogs (JSON)
- `tg://profile` — Authenticated user's profile (JSON)
- `tg://chat/{peer}` — Chat/channel metadata (JSON, URI template)
- `tg://chat/{peer}/messages` — Recent messages (text, URI template)

The `{peer}` placeholder accepts the identifier formats described in [Peers](peers.md).

## Subscriptions

`tg://chat/{peer}/messages` is subscribable. After `resources/subscribe`, every new message in that chat pushes a `resources/updated` notification for the exact URI you subscribed with, so a client can re-read the resource instead of polling. Subscribe resolves the peer once; an unresolvable peer fails the subscribe. Only the `/messages` resource is watched — subscribing to the bare `tg://chat/{peer}` info resource is accepted but never pushes.

Two known limitations:

- A client that drops abnormally (a crash, or network loss past the keep-alive) without unsubscribing leaves a stale watch entry that only a daemon restart clears. Delivery stays correct — the SDK's own registry is authoritative, so nothing is sent to the dead session — but the stale entries accumulate with reconnect churn over the daemon's lifetime (not capped by the set of chats), and each orphaned chat then costs a lookup plus a zero-subscriber log line on every later message. The SDK exposes no session-closed hook to prune it.
- The account's own outgoing messages (sent through this server) do not fire the notification: the self-send echo carries no peer. A message arriving from another device does fire it.

### Waiting without a subscription

Not every client acts on `resources/updated` — some never subscribe at all — and an agent usually wants something narrower anyway: send a message, then block until the reply arrives. `tg_messages_wait` does that as an ordinary tool call. It returns as soon as a message newer than `afterMessageId` lands in the chat (by default, newer than the chat's newest message when the call starts), oldest first, or reports `timedOut: true` after `timeoutSeconds` (default 60, at most 300). Pass the returned `lastMessageId` back as `afterMessageId` to keep waiting without missing what arrived in between.

After sending, always pass the sent message's ID as `afterMessageId`. A bot can answer within a few hundred milliseconds — in a live test @BotFather's reply landed before the wait call even started — and the default baseline would then be the reply itself, so the wait would run to its timeout with the answer already in the chat.

It is woken by the same update stream as subscriptions, and re-reads the chat every 15 seconds as well, so a gap in that stream costs a short delay rather than a missed message. Messages the account sent itself — from this server or another device — do not end the wait unless `includeOwn` is set; `from` narrows it further to one sender.

## Prompts

- `reply_to_message` — Fetch context around a message for composing replies
- `summarize_chat` — Fetch recent messages for conversation summarization
- `search_and_reply` — Search messages and prepare reply context

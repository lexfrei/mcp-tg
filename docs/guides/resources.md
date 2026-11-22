# Resources and Prompts

## Resources

- `tg://dialogs` — List of all dialogs (JSON)
- `tg://profile` — Authenticated user's profile (JSON)
- `tg://chat/{peer}` — Chat/channel metadata (JSON, URI template)
- `tg://chat/{peer}/messages` — Recent messages (text, URI template)

The `{peer}` placeholder accepts the identifier formats described in [Peers](peers.md).

## Subscriptions

`tg://chat/{peer}/messages` is subscribable. After `resources/subscribe`, every new message in that chat pushes a `resources/updated` notification for the exact URI you subscribed with, so a client can re-read the resource instead of polling. Subscribe resolves the peer once; an unresolvable peer fails the subscribe. Only the `/messages` resource is watched — subscribing to the bare `tg://chat/{peer}` info resource is accepted but never pushes.

Three known limitations:

- A client that drops abnormally without unsubscribing (a crash, or an HTTP client that leaves without ending its session) leaves a stale watch entry that only a daemon restart clears. Delivery stays correct — the SDK's own registry is authoritative, so nothing is sent to the dead session — but the stale entries accumulate with reconnect churn over the daemon's lifetime (not capped by the set of chats), and each orphaned chat then costs a lookup plus a zero-subscriber log line on every later message. The SDK exposes no session-closed hook to prune it.
- Over HTTP, a session that sends no request for an hour is closed, and its subscriptions go with it. An open notification stream does not count as activity — the SDK pauses the timer only while a request is in flight — so a client that only listens must send something (a `ping` will do) within the hour, or subscribe again after a reconnect. Nothing else closes the session of a client that went away without ending it: the server sends no keepalive pings.
- The account's own outgoing messages (sent through this server) do not fire the notification: the self-send echo carries no peer. A message arriving from another device does fire it.

## Prompts

- `reply_to_message` — Fetch context around a message for composing replies
- `summarize_chat` — Fetch recent messages for conversation summarization
- `search_and_reply` — Search messages and prepare reply context

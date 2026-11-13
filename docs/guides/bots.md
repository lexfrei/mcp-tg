# Bots and Polls

What a bot attaches to a message — a keyboard, a poll — is not part of the message text, so the read tools do not show it. These tools read and act on it.

## Buttons

`tg_messages_get_buttons` lists a message's buttons with a zero-based `row` and `column`, the two numbers `tg_messages_press_button` takes. Each button has a `kind`:

| Kind | Pressing it |
| --- | --- |
| `callback` | Sends the button's data to the bot and returns its answer: `message` (the toast or alert text), `alert` (shown as a modal), and sometimes a `url` |
| `url`, `webview`, `url_auth` | Returns the `url`; nothing is opened |
| `copy` | Returns the `copyText` |
| `reply` | A reply-keyboard button: sends its text as a message to the chat and returns `sentMessageId` |
| `switch_inline`, `game`, `buy`, `user_profile`, `request`, `other` | Refused |

The press reads the message again first, so it uses the keyboard as it is now: a bot that edited its buttons since you listed them gets the current callback data, and a button that moved is reported as missing rather than pressing whatever now sits at that position. A callback button marked `requiresPassword` (bot ownership transfer and the like) is refused, because it would need the account's 2FA password.

A callback press is never resent automatically after a Telegram server error. Nothing in the request lets the bot tell a resend from a second press, so a resend could confirm an order twice. If the press fails with a server error, check whether the bot acted before pressing again.

Most bots answer a press by editing the message or sending a new one: `tg_messages_get` on the same ID shows an edit, `tg_messages_list` shows the new message.

## Polls

`tg_polls_create` sends a poll or a quiz. A quiz has exactly one `correctOption` (zero-based) and cannot be `multipleChoice`. Polls are anonymous unless `publicVoters` is set.

`tg_polls_get` reads the question, each option with its `index`, and the results. Telegram only shows results to an account that voted or once the poll is closed, so before that `voters` stays 0.

`tg_polls_vote` votes by option `index`. Passing several indexes is only accepted by a `multipleChoice` poll; an empty list retracts the vote. To vote as a channel instead of the account, set the chat's default identity first — see [Posting as a channel](send-as.md).

## Bot info

`tg_bots_get_info` returns a bot's description and its command menu (`/command` and what it does). The commands are sent as ordinary messages with `tg_messages_send`.

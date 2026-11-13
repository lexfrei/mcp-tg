# Privacy

`tg_privacy_get` reads one of the account's privacy settings and `tg_privacy_set` changes it. Each setting answers "who may": who sees your last-seen time, who can call you, who can add you to groups.

## Settings

The `key` names the setting:

| Key | Who may |
| --- | --- |
| `last_seen` | See your last-seen time and online status |
| `phone_number` | See your phone number |
| `phone_discovery` | Find you by your phone number (only `everybody` or `contacts`) |
| `profile_photo` | See your profile photos |
| `bio` | See your bio |
| `birthday` | See your birthday |
| `forwards` | Link to your account when forwarding your messages |
| `group_invites` | Add you to groups and channels |
| `calls` | Call you |
| `voice_messages` | Send you voice and video messages (Telegram Premium) |

## Values

The `value` is one of `everybody`, `contacts`, `close_friends` or `nobody`. `allowUsers` and `disallowUsers` list exceptions that hold whatever the value says — for example `nobody` with `allowUsers: ["@mom"]`. Exceptions replace the ones already stored, so pass the full lists you want.

A setting the official apps built from rules this server does not express (only premium users, only members of a chat) reads back as `mixed`. Setting it again replaces those rules with the simple form.

# Agent Setup

Don't want to follow the steps yourself? Tell your agent:

> Read <https://mcp-tg.lexfrei.dev/getting-started/agent-setup/> and set up mcp-tg for me.

Everything below is written for the agent.

## Instructions for the agent

You are setting up **mcp-tg**, an MCP server for the user's Telegram account (MTProto user-account client, not a bot). Follow the steps in order. Two things only the human can do: obtaining the API credentials (step 2) and entering the Telegram login code (step 4) — ask for them, do not try to automate them.

### 1. Install the binary

On macOS or Linux with Homebrew:

```bash
brew install lexfrei/tap/mcp-tg
```

Without Homebrew, download a release archive for the user's OS/arch from the [releases page](https://github.com/lexfrei/mcp-tg/releases), or use the [container image](installation.md#container).

### 2. Get API credentials from the user

`TELEGRAM_APP_ID` and `TELEGRAM_APP_HASH` cannot be obtained programmatically: my.telegram.org logs in with a code sent to the user's Telegram app. Ask the user to:

1. Open <https://my.telegram.org/apps> and log in with their phone number (the login code arrives in their Telegram app, not by SMS).
2. If no application exists yet, create one — any title and short name work, platform *Desktop*, other fields optional.
3. Paste `api_id` and `api_hash` back to you.

These are app credentials, not account access; account access comes from the login in step 4. Still, do not write them anywhere except the destination in the next step.

### 3. Pick a mode

- **stdio** — the client spawns one server process per session. Right default for a single MCP client.
- **shared daemon** — one HTTP daemon serves every client and session on the machine, with a single Telegram connection. Better when the user runs many agent sessions or several MCP clients; a process per session means a separate Telegram connection from each.

If the user's usage is unclear, ask how many MCP clients or parallel sessions they run; when in doubt, stdio is fine and switching later is two commands.

### 4a. stdio setup

```bash
claude mcp add mcp-tg --env TELEGRAM_APP_ID=<api_id> --env TELEGRAM_APP_HASH=<api_hash> -- mcp-tg
```

Done. On the first tool call the server asks for the phone number and login code through the client (MCP elicitation) and stores the session in the OS keychain. Tell the user to expect that prompt; the code arrives in their Telegram app. If the user prefers the 2FA password never to pass through the MCP client, have them run `mcp-tg login` in a terminal first — see [Authentication](authentication.md).

### 4b. Shared daemon setup

Put the credentials into the file the service reads (survives upgrades and reboots):

```bash
$EDITOR "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"
```

The daemon runs headless and cannot prompt, so the session must exist before it starts. `mcp-tg login` needs a real TTY and reads the phone, code and 2FA password interactively — ask the user to run this themselves in a terminal:

```bash
set -a; . "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"; set +a
mcp-tg login
```

Then start the service and register it:

```bash
brew services start mcp-tg
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

Never suggest `sudo brew services start`: a root LaunchDaemon reads the System keychain, while the login above wrote the session to the user's login keychain — the daemon would demand a login that already happened.

### 5. Verify

```bash
claude mcp list
```

`mcp-tg` should report connected. Then call the `tg_server_version` tool — it answers with the build version. On the stdio path the very first tool call is also what triggers the login prompt, so run the verification while the user is present.

If something fails, [Configuration](configuration.md) lists every environment variable and [Authentication](authentication.md) covers session storage and revoked-session recovery.

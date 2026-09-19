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

On Debian or Ubuntu, from the apt repository (`amd64` and `arm64`); this is also what ships the systemd unit step 4b uses:

```bash
curl --fail --silent --show-error --location https://apt.lexfrei.dev/lexfrei.asc \
  | sudo gpg --dearmor --output /usr/share/keyrings/lexfrei.gpg

sudo tee /etc/apt/sources.list.d/lexfrei.sources >/dev/null <<'EOF'
Types: deb
URIs: https://apt.lexfrei.dev
Suites: stable
Components: main
Signed-By: /usr/share/keyrings/lexfrei.gpg
EOF

sudo apt update && sudo apt install mcp-tg
```

Otherwise download a release archive for the user's OS/arch from the [releases page](https://github.com/lexfrei/mcp-tg/releases), or use the [container image](installation.md#container).

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

Done. On the first tool call that needs the account, the server asks for the phone number and login code through the client and stores the session in the OS keychain. Tell the user to expect that prompt; the code arrives in their Telegram app. If the user prefers the 2FA password never to pass through the MCP client, have them run `mcp-tg login` in a terminal first — see [Authentication](authentication.md).

### 4b. Shared daemon setup

The service manager and the config path differ by install method, so start from the one used in step 1.

**Homebrew.** Put the credentials into the file the service reads (it survives upgrades and reboots):

```bash
$EDITOR "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"
```

**apt.** The package ships the unit disabled, and the config file is the user's to create:

```bash
install --directory --mode 700 ~/.mcp-tg
cp /usr/share/doc/mcp-tg/mcp-tg.env.example ~/.mcp-tg/env
chmod 600 ~/.mcp-tg/env
$EDITOR ~/.mcp-tg/env
```

The daemon runs headless and cannot prompt, so the session must exist before it starts. `mcp-tg login` needs a real TTY and reads the phone, code and 2FA password interactively — ask the user to run this themselves in a terminal, with the file from their install method:

```bash
set -a; . "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"; set +a   # Homebrew
set -a; . ~/.mcp-tg/env; set +a                              # apt
mcp-tg login
```

Then start the service and register it:

```bash
brew services start mcp-tg          # Homebrew
systemctl --user enable --now mcp-tg # apt
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

Never suggest `sudo brew services start`: a root LaunchDaemon reads the System keychain, while the login above wrote the session to the user's login keychain — the daemon would demand a login that already happened. The systemd unit is a **user** unit for the same reason, so `sudo systemctl enable` is wrong there too; if the user needs the daemon to survive logout, `loginctl enable-linger $USER` is the answer, not a system service.

On a headless Linux box there is usually no Secret Service to hold the session, so the login needs `mcp-tg login --insecure-storage` and `TELEGRAM_SESSION_INSECURE=true` uncommented in the same file. The backend must match on both sides, or the daemon looks for the session where it was never written.

### 5. Verify

```bash
claude mcp list
```

`mcp-tg` should report connected. Then call the `tg_server_version` tool — it answers with the build version, and it answers whether or not anyone is logged in, which is what makes it a server check rather than an account one. To reach the account, call `tg_profile_get`: on the stdio path a tool that needs the account is what triggers the login prompt, so run that one while the user is present.

If something fails, [Configuration](configuration.md) lists every environment variable and [Authentication](authentication.md) covers session storage and revoked-session recovery.

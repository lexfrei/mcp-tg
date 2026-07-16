# Authentication

Authentication uses a cascade: environment variable, then MCP elicitation (the client prompts you), then error.

**First run:**

1. Set `TELEGRAM_APP_ID` and `TELEGRAM_APP_HASH` (always required)
2. Optionally set `TELEGRAM_PHONE` — if not set, the server asks via elicitation
3. Telegram sends a code to your device
4. Optionally set `TELEGRAM_AUTH_CODE` — if not set, the server asks via elicitation
5. If 2FA is enabled, optionally set `TELEGRAM_PASSWORD` — or the server asks
6. The session is saved to the OS keychain (or a file with insecure storage — see below)

**Subsequent runs:** the stored session is loaded automatically, no auth needed.

## Logging in

The recommended way to create or refresh the session is the `login` subcommand. It runs an interactive terminal login: the phone, the code, and the 2FA password are read straight from the TTY and never touch MCP, elicitation, a tool call, or any transcript. It writes the session (to the keychain by default) and exits; the server then reuses it.

Native binary:

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
./mcp-tg login
```

Container — note `-it` (an interactive TTY, unlike the `-i` used to run the server). The image defaults to the file backend (a container has no keychain), so the session lands in the mounted volume:

```bash
docker run --rm -it \
  -e TELEGRAM_APP_ID=12345 \
  -e TELEGRAM_APP_HASH=your_app_hash \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest login
```

The login code is delivered by Telegram at runtime, so login is inherently interactive: it needs a real terminal and refuses to run on piped stdin (`docker run` without `-t`). The server's own env → MCP-elicitation cascade still works for a stdio server that prompts through its connected client, but `mcp-tg login` is the only path that works for the headless HTTP daemon.

## Session storage

By default the session is stored in the **OS keychain** — macOS Keychain, Linux/\*BSD Secret Service, or Windows Credential Manager — via [github.com/lexfrei/keychain](https://github.com/lexfrei/keychain). No plaintext session file is written. The session is a bearer credential (full account access, already past 2FA), so keeping it out of a loose file protects it from backups, cloud sync, and disk theft. In keychain mode `TELEGRAM_SESSION_FILE` is only the keychain **account key** (its value still distinguishes multiple sessions), not a file that gets created. Because it is the lookup key, `mcp-tg login` and the server must resolve the same value — pin `TELEGRAM_SESSION_FILE` explicitly if their environments might otherwise differ, so both address the same keychain item.

One macOS deployment caveat: a `launchd` **LaunchDaemon** runs in the system security context and reads the *System* keychain, not the *login* keychain that `mcp-tg login` (run as you) writes to — so a system daemon will not find the session no matter how the account key is set. Run the daemon as a **LaunchAgent** (user context, the same login keychain), or use `TELEGRAM_SESSION_INSECURE=true` with a file.

On macOS the item is written through `security(1)` into the stable `apple-tool` access partition, so an unsigned, frequently rebuilt binary — and the headless daemon — keep reading what `mcp-tg login` wrote, with no prompt. Any process of the same user can read it (the same trade `go-keyring` makes): it protects data-at-rest, not against code already running as you.

Where no keychain is reachable — a container, or a headless Linux host with no Secret Service — opt into a plaintext file with `--insecure-storage` (on `mcp-tg login`) or `TELEGRAM_SESSION_INSECURE=true` (for the server/daemon). The two modes must match: an item written to the keychain cannot be read from a file, or vice versa. Without the opt-in, an unreachable keychain fails fast with a clear error instead of silently writing plaintext.

**Upgrading from a file-based session:** earlier versions kept the session in the `~/.mcp-tg/session.json` file. This version reads the keychain by default, so an existing file is no longer picked up — either set `TELEGRAM_SESSION_INSECURE=true` (and pass `--insecure-storage` to `mcp-tg login`) to keep using the file, or run `mcp-tg login` once to move the session into the keychain.

## Recovery — revoked session

Telegram can invalidate a session's auth key server-side at any time — an account security event, or anti-abuse against a user-API session whose exit IP moved (a VPN switch, for example). This is not something the server triggers, and it can happen mid-run on a long-lived daemon. Afterwards every Telegram API call answers `AUTH_KEY_UNREGISTERED` (or a sibling: `SESSION_REVOKED`, `AUTH_KEY_INVALID`, …).

When this happens the server logs one `ERROR` line — `Telegram session revoked — re-login required (run: mcp-tg login)` — instead of a stream of raw 401s, and every subsequent tool call fails fast with an explicit "logged out … run `mcp-tg login`" error rather than an opaque per-tool `AUTH_KEY_UNREGISTERED`. Read-only server-meta tools (`tg_server_version`) still answer, so a client can confirm the daemon itself is alive.

A revoked key can also surface as a fatal connection error rather than a per-call reply — `AUTH_KEY_DUPLICATED` (the same key seen from two places) always does, and the others can when the failure lands during a reconnect. The connection cannot continue, so instead of staying up the daemon exits with the same `run mcp-tg login` guidance; under a supervisor (launchd/systemd) it restarts and exits again until you re-login. Either way the same code list is recognised and the fix is identical.

The daemon cannot re-authenticate itself, and it cannot be fixed from the MCP client — the `/mcp` re-authenticate action does not apply, since this server implements no OAuth. To recover:

1. Stop the daemon.
2. Run `mcp-tg login` in a terminal to refresh the stored session (it prompts for the phone, the code, and the 2FA password if set).
3. Start the daemon again; it reuses the refreshed session.

MTProto connection, migration, and auth-key lifecycle events are logged, so if a revocation recurs the daemon log carries the surrounding context.

# Installation

Every install path needs Telegram API credentials from [my.telegram.org](https://my.telegram.org) — the public builds bake in none. Get the app id and hash first, then pick a path below and log in once (see [Authentication](authentication.md)).

## Homebrew (macOS, Linux)

```bash
brew install lexfrei/tap/mcp-tg

# The public build bakes in no credentials. Put an app id and hash from
# https://my.telegram.org/apps into the config file the service reads:
$EDITOR "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"

# The same file feeds this shell, so login sees the credentials too.
set -a; . "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"; set +a
mcp-tg login                      # interactive, writes the session to the OS keychain

brew services start mcp-tg        # shared HTTP daemon on 127.0.0.1:8787
```

The service runs the headless HTTP mode, so a single daemon serves every MCP client on the machine — point clients at it with `claude mcp add --transport http mcp-tg http://127.0.0.1:8787` (see [Transport modes](../building.md#transport-modes)).

Do not reach for `sudo brew services start`: that installs a LaunchDaemon, which runs as root and reads the **System** keychain, while `mcp-tg login` wrote the session to your **login** keychain. The daemon would insist you log in, which you already did.

A service manager passes only the variables its unit declares, and credentials cannot ship inside a public formula — so the service is a small wrapper that sources `$(brew --prefix)/etc/mcp-tg/mcp-tg.env` on every start. That is the one file to edit, it survives upgrades and reboots, and nothing depends on your login shell. Uncomment `TELEGRAM_SESSION_INSECURE=true` in it if (and only if) you logged in with `--insecure-storage` — the session backend must match on both sides, or the daemon looks for the session where it was never written.

There is no Homebrew build for Windows: Homebrew has no native Windows support, and its casks are macOS-only. Windows users take the binary from the release archives below, or run the container under WSL.

## Container

```bash
docker run --rm -i \
  -e TELEGRAM_APP_ID=12345 \
  -e TELEGRAM_APP_HASH=your_app_hash \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

A container has no OS keychain, so the image defaults to the plaintext file backend and reads the session from the mounted volume — the one `mcp-tg login` wrote there (see [Logging in](authentication.md#logging-in)).

## Direct binary

Release archives carry `darwin`, `linux` and `windows` builds for `amd64` and `arm64`; grab one from the [releases page](https://github.com/lexfrei/mcp-tg/releases). Each release also ships a `checksums.txt` and a `checksums.txt.bundle` — a keyless cosign signature — and the release notes carry the `cosign verify-blob --bundle` and checksum-verification commands. This binary holds a full-access Telegram session; verifying it is worth the two commands.

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
./mcp-tg
```

## Registering with an MCP client

With Claude Code, over stdio via the container:

```bash
claude mcp add mcp-tg -- docker run --rm -i \
  -e TELEGRAM_APP_ID \
  -e TELEGRAM_APP_HASH \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

Or point every client at one already-running HTTP daemon instead of spawning a process each:

```bash
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

The trade-off between the two is described in [Transport modes](../building.md#transport-modes).

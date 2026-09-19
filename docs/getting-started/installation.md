# Installation

Every install path needs Telegram API credentials from [my.telegram.org](https://my.telegram.org) — the public builds bake in none. Get the app id and hash first, then pick a path below and log in once (see [Authentication](authentication.md)).

## Homebrew (macOS, Linux)

```bash
brew install lexfrei/tap/mcp-tg
claude mcp add mcp-tg --env TELEGRAM_APP_ID=12345 --env TELEGRAM_APP_HASH=your_app_hash -- mcp-tg
```

That is the whole setup for a single client: the client starts one server process over stdio, and on the first tool call the server asks for the phone and login code right in the client (see [Authentication](authentication.md)). To keep the 2FA password out of the MCP client, run `mcp-tg login` in a terminal once before the first call — the server then reuses the stored session.

There is no Homebrew build for Windows: Homebrew has no native Windows support, and its casks are macOS-only. Windows users take the binary from the release archives below, or run the container under WSL.

### Shared daemon (many sessions)

Running many agent sessions or several MCP clients, a server process per session adds up — Telegram sees a separate connection from each. The formula ships a `brew services` unit that runs one headless HTTP daemon serving every client on the machine:

```bash
# The service reads its credentials from this file; put the app id and hash there.
$EDITOR "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"

# The same file feeds this shell, so login sees the credentials too.
set -a; . "$(brew --prefix)/etc/mcp-tg/mcp-tg.env"; set +a
mcp-tg login                      # the headless daemon cannot prompt — log in from the terminal

brew services start mcp-tg        # shared HTTP daemon on 127.0.0.1:8787
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

The trade-off between the two modes is described in [Transport modes](../building.md#transport-modes).

Do not reach for `sudo brew services start`: that installs a LaunchDaemon, which runs as root and reads the **System** keychain, while `mcp-tg login` wrote the session to your **login** keychain. The daemon would insist you log in, which you already did.

A service manager passes only the variables its unit declares, and credentials cannot ship inside a public formula — so the service is a small wrapper that sources `$(brew --prefix)/etc/mcp-tg/mcp-tg.env` on every start. That is the one file to edit, it survives upgrades and reboots, and nothing depends on your login shell. Uncomment `TELEGRAM_SESSION_INSECURE=true` in it if (and only if) you logged in with `--insecure-storage` — the session backend must match on both sides, or the daemon looks for the session where it was never written.

## APT (Debian, Ubuntu)

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
claude mcp add mcp-tg --env TELEGRAM_APP_ID=12345 --env TELEGRAM_APP_HASH=your_app_hash -- mcp-tg
```

### Shared daemon with systemd

The package ships a systemd **user** unit at `/usr/lib/systemd/user/mcp-tg.service`, the counterpart of what `brew services` gives on macOS. It is a user unit rather than a system one because `mcp-tg login` writes the session for whoever ran it, and a system service would run as its own user and look for that session where it was never written.

It is installed disabled. Without credentials the server exits on every start, so enabling it before there is a config would just restart it forever.

```bash
install --directory --mode 700 ~/.mcp-tg
cp /usr/share/doc/mcp-tg/mcp-tg.env.example ~/.mcp-tg/env
chmod 600 ~/.mcp-tg/env
$EDITOR ~/.mcp-tg/env          # app id and hash from https://my.telegram.org/apps

set -a; . ~/.mcp-tg/env; set +a
mcp-tg login                   # a headless daemon cannot prompt; log in from the terminal

systemctl --user enable --now mcp-tg
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

The unit reads `~/.mcp-tg/env` on every start, so nothing depends on your login shell. The session lands next to it as `~/.mcp-tg/session.json`, which is the default and needs no variable of its own. On a machine with no Secret Service — a container, or a headless box — log in with `--insecure-storage` and uncomment `TELEGRAM_SESSION_INSECURE` in the same file; the backend must match on both sides.

If the daemon crash-loops, the config is the first suspect: `systemctl --user status mcp-tg` and `journalctl --user --unit mcp-tg`. The unit waits 30 seconds between restarts, so a broken config costs you nothing while you fix it.

For a login session that should keep the daemon alive after you log out, `loginctl enable-linger $USER`.

Packages are built for `amd64` and `arm64`. The repository is [lexfrei/apt](https://github.com/lexfrei/apt) and it serves everything else I publish, so the key and the source file are set up once.

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

The Homebrew paths above already register the server. For the container image, register the same `docker run` command over stdio:

```bash
claude mcp add mcp-tg -- docker run --rm -i \
  -e TELEGRAM_APP_ID \
  -e TELEGRAM_APP_HASH \
  -v ~/.mcp-tg:/home/nobody/.mcp-tg \
  ghcr.io/lexfrei/mcp-tg:latest
```

For a direct binary, `claude mcp add mcp-tg --env TELEGRAM_APP_ID=12345 --env TELEGRAM_APP_HASH=your_app_hash -- /path/to/mcp-tg` works the same way.

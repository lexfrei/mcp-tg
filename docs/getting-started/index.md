# Getting Started

Everything needed to get mcp-tg running against a Telegram account, in the order you need it.

This is a **user account** client over MTProto, not a bot — so it authenticates as you, with your phone number and 2FA, and the stored session is a bearer credential with full account access. That shapes the whole section: install, configure, log in once, and keep the session where it belongs.

## Sections

<div class="grid cards" markdown>

-   :material-download:{ .lg .middle } **Installation**

    ---

    Homebrew, container, or a signed release binary — and how to register the server with an MCP client.

    [:octicons-arrow-right-24: Installation](installation.md)

-   :material-cog:{ .lg .middle } **Configuration**

    ---

    Every environment variable and command-line flag, with its default.

    [:octicons-arrow-right-24: Configuration](configuration.md)

-   :material-key:{ .lg .middle } **Authentication**

    ---

    The `mcp-tg login` flow, where the session is stored, and how to recover a revoked one.

    [:octicons-arrow-right-24: Authentication](authentication.md)

</div>

## The short version

Get an app id and hash from [my.telegram.org](https://my.telegram.org) — the public builds carry no credentials — then:

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash

mcp-tg login                  # interactive, writes the session to the OS keychain
claude mcp add mcp-tg -- mcp-tg
```

Serving several MCP clients at once? Run one shared HTTP daemon instead of a process each — see [Transport modes](../building.md#transport-modes).

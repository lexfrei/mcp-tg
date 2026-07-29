# Getting Started

Everything needed to get mcp-tg running against a Telegram account, in the order you need it.

This is a **user account** client over MTProto, not a bot — so it authenticates as you, with your phone number and 2FA, and the stored session is a bearer credential with full account access. That shapes the whole section: install, configure, log in once, and keep the session where it belongs.

## Sections

<div class="grid cards" markdown>

-   :material-download:{ .lg .middle } **Installation**

    ---

    Homebrew, container, or a signed release binary — and how to register the server with an MCP client.

    [:octicons-arrow-right-24: Installation](installation.md)

-   :material-robot:{ .lg .middle } **Agent Setup**

    ---

    A page written for your AI agent — hand it over and the agent walks you through the whole setup.

    [:octicons-arrow-right-24: Agent Setup](agent-setup.md)

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
brew install lexfrei/tap/mcp-tg
claude mcp add mcp-tg --env TELEGRAM_APP_ID=12345 --env TELEGRAM_APP_HASH=your_app_hash -- mcp-tg
```

On the first tool call the server asks for the phone and login code right in the client, and the session lands in the OS keychain (see [Authentication](authentication.md)).

Running many agent sessions or several MCP clients? Run one shared HTTP daemon instead of a process each — see [Installation](installation.md#shared-daemon-many-sessions).

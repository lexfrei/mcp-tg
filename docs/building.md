# Building and Transports

## Requirements

- Go 1.26.5+
- Telegram API credentials from [my.telegram.org](https://my.telegram.org)

## Building

```bash
go build ./cmd/mcp-tg
```

```bash
docker build --file Containerfile --tag mcp-tg .
```

Tests and linter:

```bash
go test -race ./...
golangci-lint run
```

## Transport modes

The server speaks MCP over stdio, Streamable HTTP, or both. Which one it runs is decided by `MCP_HTTP_PORT` and `MCP_HTTP_ONLY` (see [Configuration](getting-started/configuration.md)).

### stdio (default)

stdio is the primary transport: one server process per MCP client, spawned by the client itself. Auth elicitation runs through that stdio session, so this is the only mode that can prompt an interactive login through the client. Setting `MCP_HTTP_PORT` additionally opens an HTTP transport alongside stdio.

**Multiple sessions:** by default each Claude Code (or other MCP) client starts its own stdio process — that is how the stdio transport works. This is fine for one or two clients, but every process opens its own MTProto connection on the same auth key and shares the one stored session. Running many at once (5+) wastes connections and risks Telegram flagging the auth key. To share one process across many clients, run the headless HTTP-only daemon described below.

### Shared daemon (HTTP-only)

To serve many MCP clients from a single process and a single Telegram connection, run the server as a headless HTTP-only daemon. Set `MCP_HTTP_ONLY=true` together with `MCP_HTTP_PORT`. In this mode the server skips the stdio transport entirely and listens only on HTTP, multiplexing every connecting client onto the same Telegram session — one MTProto connection and one writer of the stored session regardless of how many clients attach.

Because all clients share that one MTProto connection, they also share its throughput: requests serialize through a single connection, and a FLOOD_WAIT triggered by one client's burst pauses the auto-retry for everyone until the server-specified delay elapses. A good trade for many lightly-used clients, less so for a few high-volume ones.

A headless daemon has no client to prompt through, so it cannot log in itself. Run `mcp-tg login` once, then start the daemon; it reuses the stored session. If the session is missing or revoked, the daemon exits with a message telling you to run `mcp-tg login` — it does not hang or silently fall back to plaintext.

```bash
export TELEGRAM_APP_ID=12345
export TELEGRAM_APP_HASH=your_app_hash
export MCP_HTTP_PORT=8787
export MCP_HTTP_ONLY=true
./mcp-tg
```

Point each client at the running daemon over HTTP instead of spawning its own process:

```bash
claude mcp add --transport http mcp-tg http://127.0.0.1:8787 --scope user
```

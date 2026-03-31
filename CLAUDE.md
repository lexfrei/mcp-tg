# CLAUDE.md — Development Guide for mcp-tg

## What is this

MCP server for Telegram Client API (MTProto, not Bot API). Uses gotd/td for protocol, exposes 58 tools + resources + prompts via MCP.

## Build & Test

```bash
go build ./cmd/mcp-tg
go test -race ./...
golangci-lint run
```

## Architecture

```text
cmd/mcp-tg/main.go          Entry point: telegram client → MCP server → transports
internal/config/             Env var loading and validation
internal/telegram/           Telegram abstraction layer
  types.go                   Domain types (Message, User, Dialog, etc.)
  client.go                  Client interface (13 sub-interfaces)
  wrapper.go                 gotd/td implementation of Client
  wrapper_helpers.go         Helper functions for wrapper (extractors, converters)
  convert.go                 tg types → domain types conversion
  auth.go                    Auth flow with MCP elicitation support
  resolve.go                 Peer resolution (@username, numeric ID, t.me/ URLs)
internal/tools/              MCP tool handlers (58 tools)
  annotations.go             Tool annotation helpers (readOnly, idempotent, write, destructive)
  errors.go                  Error sentinels
  helpers.go                 Shared helpers (deref, formatPeer, formatUserName)
  format.go                  Output formatting (timestamps, messages, dialogs)
  roots.go                   File path validation against client roots
  progress.go                Progress notification helper
  mock_test.go               Mock telegram.Client for tests
internal/resources/          MCP resources (4 resources)
internal/prompts/            MCP prompts (3 prompts)
internal/completions/        Argument autocompletion
internal/middleware/         Request logging middleware
internal/testutil/           NoopClient for registration tests
```

## Key Patterns

### Adding a new tool

1. Create `internal/tools/xxx.go` with:
   - `XxxParams` struct (jsonschema tags: `jsonschema:"Description"`)
   - `XxxResult` struct
   - `NewXxxHandler(client telegram.Client) mcp.ToolHandlerFor[XxxParams, XxxResult]`
   - `XxxTool() *mcp.Tool` with appropriate `Annotations`
2. Create `internal/tools/xxx_test.go` (TDD: tests first)
3. Register in `cmd/mcp-tg/main.go` `registerTools()`
4. Add mock method in `internal/tools/mock_test.go`
5. Add noop method in `internal/testutil/noop_client.go`

### Tool annotations

- `readOnlyAnnotations()` — tools that only read data (23 tools)
- `idempotentAnnotations()` — tools that modify state but are safe to retry (22 tools)
- `writeAnnotations()` — tools that create new entities, not idempotent (7 tools)
- `destructiveAnnotations()` — tools that delete/remove things (6 tools)

### Peer resolution

All tools accept `peer` as string. Supported formats:
- `@username`
- `username` (bare)
- `https://t.me/username`
- Numeric ID (bot-API style: positive=user, negative=chat, -100xxx=channel)

WARNING: Numeric IDs have AccessHash=0, which limits some operations. Prefer @username.

### Channels vs Groups

Both are handled transparently. Wrapper checks `peer.Type == PeerChannel` and uses the appropriate API methods (e.g., `ChannelsGetMessages` vs `MessagesGetMessages`). No special flags needed except `isChannel` in `chats_create`.

### Auth flow

Cascade: env var → MCP elicitation → error. No stdin fallback (stdin = MCP protocol).

Session persistence: volume mount `-v ~/.mcp-tg:/home/nobody/.mcp-tg`.

## Linter

Strict config in `.golangci.yml`:
- funlen: 50 lines / 40 statements
- gocyclo/cyclop: 10
- dupl: 100
- lll: 140 characters
- All linters enabled except: depguard, exhaustruct, ireturn

## Dependencies

- `github.com/gotd/td` — Telegram MTProto client
- `github.com/cockroachdb/errors` — Error wrapping
- `github.com/modelcontextprotocol/go-sdk` — MCP protocol SDK
- `golang.org/x/sync` — errgroup for concurrent transports

# gogent

Go port of the Pragma TypeScript CLI. LLM-generic — works with Anthropic, OpenAI, Google, or any provider.

## Status

**Phase 1 complete** — Foundation types, provider interface, event system, tool infrastructure, architecture tests.

## Architecture

```
internal/
  model/        Pure domain types (ContentPart, Message, Conversation, Response, ToolDef)
  provider/     Provider interface — the only translation boundary
  observe/      EventBus backbone — logging, recording, metrics, audit, replay
  permission/   Permission types (Checker interface, Decision, Rule)
  tool/         Tool Descriptor interface, Registry, Orchestrator
  app/          AppState + thread-safe StateStore
  archtest/     Architecture enforcement tests (go/parser scans)
```

Every package follows a strict dependency DAG. Only provider adapters know wire formats. Everything else uses `internal/model/` types exclusively.

## Key Design Decisions

- **Sealed interfaces** (ADR-001): Unexported marker methods create closed type sets
- **RWMutex StateStore** (ADR-002): Single mutation path, value semantics on read
- **json.RawMessage for schemas** (ADR-003): Zero transform between definition and API call
- **Channel-based generators** (ADR-004): `<-chan StreamChunk` replaces AsyncGenerator
- **Constructor-based DI** (ADR-006): No globals, no singletons, everything wired in main
- **Sentinel errors** (ADR-007): `errors.Is` checking, never string matching

## Observability

All observability flows through `internal/observe/EventBus`. No ad-hoc logging. Every boundary crossing emits a typed Event. Events are simultaneously logged, recorded (replay), metriced, and audited. Permission denials are tracked with a `WasExecuted` field to catch enforcement bugs.

## Running Tests

```bash
go test ./internal/...           # all tests
go test ./internal/... -race     # with race detector
go test ./internal/archtest/     # architecture enforcement only
```

## Spec

See [SPEC.md](SPEC.md) for the full entity model, provider interface, processes, observability design, and enforcement rules.

## Libraries

| Library | Purpose |
|---|---|
| `golang.org/x/sync/errgroup` | Concurrent tool execution |
| `github.com/spf13/cobra` | CLI framework (Phase 3+) |
| `github.com/charmbracelet/bubbletea` | TUI framework (Phase 6+) |
| `github.com/anthropics/anthropic-sdk-go` | Claude API adapter (Phase 2+) |
| `github.com/mark3labs/mcp-go` | MCP protocol (Phase 11+) |

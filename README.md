# gogent

Agentic code. LLM-generic — works with Anthropic, OpenAI, Google, or any provider.

## Quick Start

```bash
# Run with a prompt
export ANTHROPIC_API_KEY=your-key
go run ./cmd/gogent -p "What is 2+2?"

# With options
go run ./cmd/gogent -p "Explain Go channels" \
  --model claude-haiku-4-5-20251001 \
  --system-prompt "Be concise" \
  --max-tokens 200 \
  --verbose
```

## Status

**Phase 3 complete** — gogent is runnable. Non-interactive mode works end-to-end: config loading, streaming agentic loop with tool execution, Cobra CLI.

## CLI Flags

| Flag | Description |
|---|---|
| `-p, --prompt` | Prompt text (required for non-interactive) |
| `--model` | Model name (default: claude-sonnet-4-20250514) |
| `--provider` | Provider name (default: anthropic) |
| `--api-key` | API key (or set `ANTHROPIC_API_KEY` env var) |
| `--system-prompt` | System prompt override |
| `--max-tokens` | Max output tokens (default: 16384) |
| `--temperature` | Sampling temperature (0.0–1.0) |
| `--thinking` | Enable extended thinking |
| `--thinking-budget` | Thinking token budget (default: 10000) |
| `--verbose` | Verbose event logging to stderr |
| `--record` | Record events to `gogent-recording.jsonl` |

## Architecture

```
cmd/gogent/           CLI entry point (Cobra root command + wiring)
internal/
  config/             Config loading with 3-scope merge (global → project → CLI)
  query/              Engine + agentic loop (stream → accumulate → tool exec → loop)
  model/              Pure domain types (ContentPart, Message, Conversation, Response, ToolDef)
  provider/           Provider interface + AccumulateStream utility
  provider/anthropic/ Anthropic adapter (translate, stream, retry, cache, normalize)
  observe/            EventBus backbone — logging, recording, metrics, audit, replay
  permission/         Permission types (Checker interface, Decision, Rule)
  tool/               Tool Descriptor interface, Registry, Orchestrator
  app/                AppState + thread-safe StateStore
  archtest/           Architecture enforcement tests (go/parser scans)
```

Every package follows a strict dependency DAG. Only provider adapters know wire formats. Everything else uses `internal/model/` types exclusively.

## Key Design Decisions

- **Sealed interfaces** (ADR-001): Unexported marker methods create closed type sets
- **RWMutex StateStore** (ADR-002): Single mutation path, value semantics on read
- **json.RawMessage for schemas** (ADR-003): Zero transform between definition and API call
- **Channel-based generators** (ADR-004): `<-chan StreamChunk` / `<-chan LoopEvent` replaces AsyncGenerator
- **Constructor-based DI** (ADR-006): No globals, no singletons, everything wired in main
- **Sentinel errors** (ADR-007): `errors.Is` checking, never string matching
- **ThinkingPart.Signature** (ADR-008): Provider attestation survives session persistence
- **StopPauseTurn** (ADR-010): Continuation signal preserved through the agentic loop

## Observability

All observability flows through `internal/observe/EventBus`. No ad-hoc logging. Every boundary crossing emits a typed Event. Events are simultaneously logged, recorded (replay), metriced, and audited. Permission denials are tracked with a `WasExecuted` field to catch enforcement bugs.

## Running Tests

```bash
go test ./...                   # all tests (11 packages)
go test ./internal/... -race    # with race detector
go test ./internal/archtest/    # architecture enforcement only
go test ./internal/query/ -v    # query loop tests (17 test cases)
go test ./internal/config/ -v   # config merge + load tests
```

## Spec

See [SPEC.md](SPEC.md) for the full entity model, provider interface, processes, observability design, and enforcement rules.

## Libraries

| Library | Purpose |
|---|---|
| `golang.org/x/sync/errgroup` | Concurrent tool execution |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/anthropics/anthropic-sdk-go` | Claude API adapter |
| `github.com/charmbracelet/bubbletea` | TUI framework (Phase 6+) |
| `github.com/mark3labs/mcp-go` | MCP protocol (Phase 11+) |

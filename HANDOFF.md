# Phase 3 Handoff: CLI Skeleton + Non-Interactive Mode

## What Was Built

gogent is now runnable. `go run ./cmd/gogent -p "question"` sends a prompt to the Anthropic API, streams the response, and prints it to stdout.

### Files Created (Phase 3)

| File | Lines | Purpose |
|---|---|---|
| `cmd/gogent/main.go` | 195 | Cobra CLI, flag parsing, wiring, non-interactive mode |
| `internal/config/config.go` | 110 | Config struct, 3-scope merge (global → project → CLI flags) |
| `internal/config/config_test.go` | 310 | 12 tests: merge, load, round-trip, permission errors |
| `internal/query/engine.go` | 50 | Engine struct, EngineConfig, NewEngine constructor |
| `internal/query/event.go` | 55 | LoopEvent sealed interface (6 variants) |
| `internal/query/loop.go` | 240 | Agentic loop: stream → accumulate → tool exec → loop |
| `internal/query/loop_test.go` | 310 | 9 core tests with testProvider |
| `internal/query/integration_test.go` | 260 | 2 deep integration tests (response/conversation/cost) |
| `internal/query/manual_test.go` | 360 | 6 edge-case tests (multi-tool, unknown tool, pause, etc.) |

### Files Modified

| File | Change |
|---|---|
| `go.mod` / `go.sum` | Added `github.com/spf13/cobra` |
| `.pragma/AGENT.md` | Updated current state to Phase 3 complete |
| `README.md` | Added Quick Start, CLI flags, updated architecture |

## Test Coverage

| Package | Coverage |
|---|---|
| `internal/config/` | 91.1% |
| `internal/query/` | 97.8% |

## What Works

- **Basic prompt**: `gogent -p "question"` → streams response to stdout
- **Model override**: `--model claude-haiku-4-5-20251001`
- **System prompt**: `--system-prompt "text"` or via `.pragma/settings.json`
- **Max tokens**: `--max-tokens N` — truncation handled cleanly
- **Temperature**: `--temperature 0.0`–`1.0`
- **Extended thinking**: `--thinking --thinking-budget N`
- **Verbose logging**: `--verbose` → API events on stderr with trace/span IDs
- **Event recording**: `--record` → JSONL file with full event payloads
- **Config files**: 3-scope merge (global ~/.pragma/settings.json → project .pragma/settings.json → CLI flags)
- **Prefix caching**: Large system prompts trigger cache_creation/cache_read
- **Cost tracking**: Per-request cost computed from usage + pricing, printed with --verbose
- **Error handling**: Invalid API key, missing key, API errors — all clean messages + exit 1
- **Agentic loop**: StopEndTurn, StopMaxTokens, StopPauseTurn (continuation), StopToolUse (tool execution) — all paths tested

## What Doesn't Exist Yet

- **No tools**: Registry is empty. Tool use path is tested but no real tools (BashTool, FileReadTool, etc.)
- **No TUI**: Interactive mode not implemented. Only `-p` non-interactive mode works
- **No session persistence**: Conversations are not saved to disk
- **No compaction**: Context window management not implemented
- **No MCP**: No MCP server integration

## Bugs Found & Fixed During Verification

1. **Double API events**: Provider AND loop both emitted `APIRequestStarted`/`APIRequestCompleted`. Fixed: loop delegates API events to provider.
2. **Log level too noisy**: Non-verbose mode showed info-level events on stderr. Fixed: non-verbose uses `LevelError`, verbose uses `LevelTrace`.

## Architecture Notes for Next Phase

- The agentic loop (`query/loop.go`) consumes `provider.StreamChunk` inline (not via `provider.AccumulateStream`) because it needs to emit `TextEvent`/`ThinkingEvent` deltas as they arrive.
- `LoopEvent` (query-local) and `observe.Event` (bus events) are separate sealed interfaces serving different audiences.
- `allowAllChecker` in main.go is the non-interactive permission strategy. Interactive mode will need a TUI-based checker.
- The `config.ThinkingConfig` and `provider.ThinkingConfig` are independent types at different layers (DAG rule). main.go field-copies between them.

## Next Phase: Phase 4 — Tool Implementations

Priority tools for Phase 4:
- GOGENT-25: BashTool (8pt)
- GOGENT-27: FileReadTool (3pt)
- GOGENT-29: FileWriteTool (3pt)
- GOGENT-31: FileEditTool (5pt)
- GOGENT-33: GlobTool (3pt)
- GOGENT-36: GrepTool (3pt)

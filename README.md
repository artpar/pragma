# pragma

Agentic code. LLM-generic — works with Anthropic, OpenAI, Google, or any provider.

Harness development: read [agent.md](agent.md) for the required replay-first
verification methodology and [docs/README.md](docs/README.md) for current guidance.

## Quick Start

```bash
# Run with a prompt
export ANTHROPIC_API_KEY=your-key
go run ./cmd/pragma -p "What is 2+2?"

# With options
go run ./cmd/pragma -p "Explain Go channels" \
  --model claude-haiku-4-5-20251001 \
  --max-tokens 200 \
  --verbose
```

## Status

**Pragma loop baseline** — pragma is runnable. The default query loop currently follows the Pragma single bash action loop. The stable provider tool-calling loop is available with `--loop provider-tools`.

## Constraint Decay / SWE Benchmark

Pragma is run against the Constraint Decay SWE benchmark from the sibling checkout, not from this repo directly:

```bash
cd /Users/artpar/workspace/code/constraint-decay

PRAGMA_PATH=/Users/artpar/workspace/code/pragma \
AGENT=pragma_agent \
TASK=node/node-express-openapi-unconstrained.json \
LLM_API_KEY="$MORPH_API_KEY" \
LLM_PROVIDER=morphllm \
LLM_MODEL=morph-glm53-744b \
tools/run_miniswe_with_capture.sh
```

The benchmark adapter is `runtime/agents/pragma_agent.py` in `constraint-decay`. It builds this checkout with `go build -o /tmp/pragma-bin ./cmd/pragma`, then runs Pragma inside the benchmark container against `/repository`.

Default Pragma benchmark settings from the adapter:

| Setting | Value |
|---|---|
| Provider | `morphllm` |
| Model | `$LLM_MODEL`, default `morph-glm53-744b` |
| Permission mode | `bypassPermissions` |
| Context mode | `chat` |
| Allowed tools | `Bash` |
| Temperature | `$PRAGMA_TEMPERATURE`, default `0` |
| Max turns | `$PRAGMA_MAX_TURNS`, default `200` |

Useful overrides:

```bash
PRAGMA_MAX_TURNS=300
PRAGMA_TEMPERATURE=0
PRAGMA_EXTRA_ARGS="--verbose"
```

Outputs are written under `constraint-decay/data/results/<runtime>/pragma_agent/<model>/<task>/<timestamp>/run_0/`. The capture wrapper also prints:

- `result_run_dir`
- `raw_http_dir`
- `raw_http_calls`

For Pragma runs, raw HTTP captures are stored in the run directory under `raw-http-pragma/`, and Pragma logs/session files are under `pragma-home/.pragma/`.

Pragma code is not pushed to `origin` as part of benchmark runs. Keep benchmark validation local unless an explicit push is requested.

## SWE-bench Pro

The official SWE-bench Pro harness is cloned locally at `/Users/artpar/workspace/code/SWE-bench_Pro-os`. Use the repo-local runner for one-instance smoke runs:

```bash
tools/run_swebench_pro_instance.py --prepare-only
```

For a real patch-generation run:

```bash
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-507170da0f7f4da330f6732bffdf11c4df7fc192 \
  --pull-image
```

The runner uses `LLM_API_KEY`, `MORPH_API_KEY`, or the MorphLLM entry in `~/.pragma/credentials.yml`.

By default the runner also prepares and mounts a cached Linux `amd64`
generator toolchain into the benchmark container, including `buf`, `protoc`,
`protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-grpc-gateway`, and
`protoc-gen-openapiv2`. Use
`--no-generator-toolchain` to disable it.

Add `--evaluate` to run the official local-Docker evaluator on the generated patch. See [docs/swe-bench-pro.md](docs/swe-bench-pro.md) for the full workflow, output paths, and scaling notes.

## CLI Flags

| Flag | Description |
|---|---|
| `-p, --prompt` | Prompt text (required for non-interactive) |
| `--model` | Model name (provider default; MorphLLM fallback: `morph-glm53-744b`) |
| `--provider` | Provider name (credential auto-detection; fallback: `morphllm`) |
| `--api-key` | API key (or set the provider-specific environment variable) |
| `--max-tokens` | Max output tokens (default: 16384) |
| `--temperature` | Sampling temperature (0.0–1.0) |
| `--thinking` | Enable extended thinking |
| `--thinking-budget` | Thinking token budget (default: 10000) |
| `--verbose` | Verbose event logging to stderr |
| `--record` | Record events to `pragma-recording.jsonl` |

### Try GLM-5.3 through MorphLLM

```bash
mkdir -p ~/.pragma
touch ~/.pragma/credentials.yml
chmod 600 ~/.pragma/credentials.yml
$EDITOR ~/.pragma/credentials.yml
```

```yaml
providers:
  morphllm:
    api_key: your-morph-key
```

Then run:

```bash
go run ./cmd/pragma --provider morphllm --model morph-glm53-744b \
  --prompt 'Inspect this repository and suggest one high-impact improvement.'
```

Pragma uses Morph's OpenAI-compatible endpoint automatically. Set
`MORPH_API_KEY` or `MORPH_BASE_URL` only when you need a temporary
credential or endpoint override.

## Architecture

```
cmd/pragma/           CLI entry point (Cobra root command + wiring)
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
```

Every package follows a strict dependency DAG. Only provider adapters know wire formats. Everything else uses `internal/model/` types exclusively.

Clean architecture claims must be enforced by source-level ownership, not by
package names or naming intent. See
[docs/clean-architecture-enforcement-doctrine.md](docs/clean-architecture-enforcement-doctrine.md).
For issue-by-issue stabilization work, use
[docs/issues-clean-code-stabilization-goal-prompt.md](docs/issues-clean-code-stabilization-goal-prompt.md).

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

### Self-observation: pragma-watch

`pragma-watch` is a standalone read-only observer for running pragma
sessions. It tails the event logs pragma itself writes
(`~/.pragma/logs/*.jsonl`), discovers live pragma processes via `ps`, and
renders a live dashboard: per-session status (in-flight model call,
executing tools, awaiting user, ended), context fill, turns, tool calls,
retries, failures, and MCP server health. It detects tool loops, retry
storms, context-window pressure (`--ctx <tokens>`), and stalled requests
(`--stall <dur>`).

```bash
make watch                      # build bin/pragma-watch
./bin/pragma-watch               # live dashboard (Ctrl-C to stop)
./bin/pragma-watch --once        # single snapshot
./bin/pragma-watch --json        # NDJSON snapshots for scripted consumers
./bin/pragma-watch --ctx 200000  # enable context % alerts
```

It never touches pragma state — only the logs pragma produces.

### Self-improvement loop: daemon + post-mortems

A dashboard dies with the terminal, so for durable self-observation the
watcher also runs as a detached recording daemon:

```bash
./bin/pragma-watch --daemon       # spawn detached daemon (survives everything)
./bin/pragma-watch --report       # digest of everything recorded so far
```

The daemon polls every second, widens its active window to 1h, and writes
to `~/.pragma/observations/`:

- `alerts.jsonl` — every alert (tool loops, retry storms, context
  pressure, MCP disconnects, stalls) with session, level, and time.
- `postmortems/<session>.postmortem.json` — written the moment a session's
  pragma process disappears: final status, context peak, stop-reason
  histogram, top tools, loop evidence, last error.
- `daemon.log` / `daemon.pid` — daemon activity and lifecycle.

Future sessions consume the record with `--report` (the harness
self-evolution skill points sessions at this). This closes the loop:
sessions die, the daemon survives them and records how they ended, and the
next session reads the record instead of starting blind.

## Running Tests

```bash
go test ./...                   # all tests
go test ./internal/... -race    # with race detector
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
| `github.com/anthropics/anthropic-sdk-go` | Anthropic Messages API adapter |
| `github.com/charmbracelet/bubbletea` | TUI framework (Phase 6+) |
| `github.com/mark3labs/mcp-go` | MCP protocol (Phase 11+) |

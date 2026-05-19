# pragma — AI coding assistant CLI

Agentic coding assistant CLI. Provider-agnostic: works with Anthropic, Google Gemini, OpenAI, Groq, Lilac.

## Key directories

- `internal/model/` — domain types: Message, ContentPart, Conversation, ToolDef, Response
- `internal/provider/` — Provider interface + adapters: anthropic, google, openai, groq, lilac, replay
- `internal/query/` — engine loop, orchestrator, streaming, error classification, compaction
- `internal/tool/` — Tool interface, orchestrator, permission checking
- `internal/tools/` — tool implementations, each in its own package
- `internal/tui/` — bubbletea TUI: input, viewport, dialogs, renderers
- `internal/lifecycle/` — graph executor for structured workflows
- `internal/lifecycle/bridge/` — LLM→graph compilation
- `internal/sysprompt/` — system prompt builder: static blocks + AGENT.md + skills + env
- `internal/config/` — settings, credentials, paths
- `internal/observe/` — EventBus: all observability (logging, replay, metrics, audit)
- `internal/permission/` — rule-based permission checking
- `internal/compact/` — conversation compaction
- `internal/slash/` — slash commands
- `internal/mcp/` — MCP client integration
- `internal/session/` — session persistence
- `SPEC.md` — authoritative specification (entities, processes, provider interface)

## Architecture rules

- Everything imports `internal/model/` — never provider-specific packages
- No circular imports (strict DAG, see AGENT.md for full graph)
- All observability through `internal/observe/EventBus` — no ad-hoc logging
- Sealed interfaces for discriminated unions (unexported marker methods)
- Constructor-based dependency injection — no globals, no init() side effects
- Sentinel errors with `errors.Is` wrapping

## Conventions

- Tools: separate package under `internal/tools/<name>/`, static `inputSchema`
- Context as first parameter to every exported function
- No mocks — tests use real implementations (replay provider for recorded responses)
- No stubs or TODO placeholders — every function does real work or doesn't exist
- No file > 500 lines, no package > 2000 lines (excluding tests)

## Auto-instrumentation (DO NOT manually add or modify)

All `observe.GlobalTrace(...)` and `observe.TraceCtx(...)` calls are **auto-generated** by `cmd/pragma-instrument/`. They are injected by an AST rewriter at every branch point. You must NEVER:
- Add `observe.GlobalTrace` or `observe.TraceCtx` calls manually
- Modify existing trace calls
- Copy the trace call pattern into new or edited code
- Treat these calls as "code style" to follow

If a function you edit already has trace calls, leave them as-is. New code you write should NOT include them — the instrumenter will add them on the next build.

Correct. Revised boundary:

Web UI is **not a mode**. It is the UI used by **interactive Pragma**.

So:

- `pragma` with no one-off command starts interactive Pragma through the web UI.
- `pragma -p ...`, `pragma replay ...`, `pragma audit ...`, `pragma metrics ...`, `pragma lifecycle run ...`, `pragma orchestration run ... --prompt ...`, `pragma completion`, `pragma version`, benchmark harness calls stay CLI paths.
- We should not drag every CLI command into the web UI plan.

**Existing Component Ownership**

`cmd/pragma`
- Owns Cobra command tree only.
- Keeps independent one-off commands independent.
- Root no-arg interactive path enters web UI.
- Should not own runtime state or web behavior.

`internal/cli`
- Owns dependency wiring and launch routing.
- `RunDispatcher` decides: one-off CLI path vs interactive path.
- Interactive path should call the web UI entry instead of Bubbletea TUI.
- Should not become a run manager or UI state container.

`internal/config`
- Owns config loading and merge semantics.
- Web UI can show/edit config through this boundary.
- No UI-specific config interpretation elsewhere.

`internal/app`
- Owns canonical in-memory app/session state.
- Conversation, handoff state, plan/todo state, current model/provider state belong here.
- Web UI reads this through explicit methods/events, not by reconstructing state.

`internal/query`
- Owns the agent loop.
- Streams `query.LoopEvent`.
- Should not know whether the consumer is CLI, TUI, or web.
- No web/session/orchestration policy here.

`internal/observe`
- Owns typed runtime evidence.
- Tool events, API events, permission audit, metrics inputs, session events.
- If web UI needs structured orchestration display, orchestration must emit typed observe/query events, not text that UI parses.

`internal/session`
- Owns durable session JSONL.
- Session listing/resume/prompt history/metadata.
- Should not own raw HTTP replay semantics or UI concepts.

`internal/orchestration`
- Owns FSM semantics.
- Owns state transitions, control states, checklist cursor/status, handoff generation/consumption.
- Must stop relying on global `/tmp/pragma` as hidden shared state if interactive web can run multiple orchestrations.
- Should expose typed progress/state data.

`internal/lifecycle`
- Owns lifecycle graph execution.
- Stays a CLI one-off unless later deliberately made interactive.
- Do not fold into first web UI scope.

`cmd/pragma/replay*.go` plus `internal/observe/replay`, `internal/provider/replay`
- Own replay CLI behavior.
- Do not put “Replay Lab” in core web UI unless we separately decide interactive debugging belongs there.
- For now, replay remains CLI.

`internal/permission`
- Owns permission decisions and rule matching.
- Web UI only renders pending permission requests and sends user decisions.
- It should not decide policy.

`internal/tool` / `internal/tools/*`
- Own tool descriptors and execution.
- Web UI shows tool call/result state from events.
- It must not directly execute tools.

`internal/mcp`
- Owns MCP connection lifecycle and tool adapters.
- Web UI can display status, not manage protocol internals.

`internal/tui`
- Existing Bubbletea presentation layer.
- If web replaces interactive UI, this becomes legacy/fallback UI or can be retired later.
- Do not move runtime responsibilities from TUI into web. Any logic currently trapped in TUI should be pushed to existing runtime/state packages.

**Only Actually New Surface**

A browser presentation package is unavoidable, but it should be thin:

`internal/web` or similar:
- HTTP server.
- Static frontend serving.
- SSE/WebSocket transport.
- Request handlers that call existing component interfaces.
- No agent loop logic.
- No orchestration semantics.
- No session format logic.
- No replay implementation.

So the corrected architecture is:

`cmd/pragma` routes no-arg interactive to web UI
`internal/cli` wires deps
`internal/web` presents interactive UI
`internal/query/app/session/observe/permission/tool/orchestration/config` keep their current domain ownership with stricter interfaces

The web UI plan should be cut down accordingly: interactive run console, live transcript, permissions, tools, session resume, config/model picker, workspace diff/status, orchestration only if launched interactively. Replay/benchmark/audit/metrics/lifecycle command labs should be removed from the core web UI plan unless explicitly requested later.
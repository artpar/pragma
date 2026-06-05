# Component Responsibilities Implementation Change Points

This document prepares implementation of `docs/component-responsisbilites-plan.md`.
It lists exact code change points and the intended change at each point. It is
not a code patch.

## Scope Rule

Web UI is the interactive Pragma UI. It is not a mode and it is not a container
for unrelated one-off CLI commands.

Keep these as independent CLI paths:

- `pragma -p ...`
- `pragma --bg -p ...`
- `pragma replay ...`
- `pragma audit ...`
- `pragma metrics ...`
- `pragma lifecycle run ...`
- `pragma orchestration run ... --prompt ...`
- `pragma version`
- `pragma completion`
- benchmark harness invocations

Only the root interactive path, `pragma` with no one-off command or prompt,
should launch the browser UI.

## Non-Goals For This Change

- Do not add a generic run manager package.
- Do not add replay, benchmark, metrics, audit, or lifecycle labs to the web UI.
- Do not move agent loop logic into the web package.
- Do not make web own permissions, tools, sessions, config, or orchestration
  semantics.
- Do not replace existing package responsibilities with duplicate wrappers.

## Change Points

### 1. `cmd/pragma/main.go`

Responsible component: `cmd/pragma`

Current role:

- Owns Cobra root and subcommand registration.

Change:

- Keep this role unchanged.
- Do not import `internal/web` here.
- Keep `RunE: cli.RunDispatcher`.
- Keep all existing subcommands registered as independent command paths.

Reason:

- `cmd/pragma` should only define command shape. Interactive UI selection belongs
  in `internal/cli`.

### 2. `internal/cli/run.go`: `RunDispatcher`

Responsible component: `internal/cli`

Current location:

- `RunDispatcher` routes `--bg`, `--list-sessions`, `--prompt`, and default
  interactive TUI.

Change:

- Keep existing one-off branches:
  - `--bg` -> `RunBackground`
  - `--list-sessions` -> `RunListSessions`
  - non-empty `--prompt` -> `RunNonInteractive`
- Change only the final interactive branch:
  - no prompt and no one-off flag -> `RunInteractive`
  - `RunInteractive` should launch the web UI instead of Bubbletea.

Do not:

- Route `replay`, `metrics`, `audit`, `lifecycle`, or prompted orchestration
  through web.
- Change Cobra subcommand behavior.

### 3. `internal/cli/run.go`: split current `RunInteractive`

Responsible component: `internal/cli`

Current issue:

- `RunInteractive` does dependency wiring, session start/end hooks, tool
  registration, compaction setup, slash setup, prompt history loading,
  orchestration callback setup, TUI config construction, and Bubbletea launch in
  one function.

Change:

- Keep dependency wiring in `internal/cli`.
- Extract the non-presentation setup into a small `InteractiveRuntime` struct
  and builder function in `internal/cli`.

Suggested shape:

```go
type InteractiveRuntime struct {
    Deps *Deps
    Engine *query.Engine
    SlashCmds *slash.Registry
    SlashDeps slash.Deps
    SessionSave func()
    SessionClose func()
    SessionSwitch func(sessionID string) (func(), func())
    PromptHistorySave func(string)
    PromptHistory []string
    Orchestrate func(context.Context, slash.OrchestrationRequest) <-chan query.LoopEvent
}
```

Implementation change:

- Move the setup currently in `RunInteractive` lines that create:
  - `SetupDeps`
  - session start event
  - session start/end hooks
  - interactive prompter/asker injection point
  - `RegisterTools`
  - tool filters
  - MCP wait
  - compaction deps
  - session save/close
  - slash registry/deps
  - skill registration
  - prompt history
  - orchestration callback
- Keep the type in `internal/cli`, not `internal/web`.

Important interface point:

- The builder must accept `permission.Prompter` and `tool.Asker` supplied by the
  presentation layer. TUI and web can each supply their own implementations.

### 4. `internal/cli/run.go`: new `RunInteractive`

Responsible component: `internal/cli`

Change:

- After extracting the runtime builder, make `RunInteractive`:
  - create web prompter/asker placeholders or a web bridge object,
  - build `InteractiveRuntime`,
  - call `web.Run(cmd.Context(), web.Config{...})`.

Exact responsibility:

- `internal/cli` wires dependencies.
- `internal/web` presents the browser UI and transports events.

Do not:

- Put HTTP server setup into `internal/cli`.
- Put tool execution, orchestration, or session logic into `internal/web`.

### 5. `internal/tui`: preserve as presentation-only fallback

Responsible component: `internal/tui`

Current role:

- Bubbletea presentation layer.
- Contains TUI-specific prompter/asker bridges.
- Contains TUI-specific rendering and local UI state.

Change:

- Do not move TUI rendering code into web.
- Do not let web import TUI types such as `PermRequestMsg`, `AskRequestMsg`,
  toolbar models, input components, or dialogs.
- If fallback TUI is kept, add a clearly named CLI function such as
  `RunTUIInteractive` in `internal/cli` that reuses the same
  `InteractiveRuntime` builder.

Cleanup needed:

- Comments in `internal/tool/asker.go` and `internal/slash/command.go` mention
  TUI as if it is the only interactive presentation. Update wording to
  "interactive UI" when code is touched.

### 6. New thin `internal/web` package

Responsible component: new browser presentation surface only

This is the only new package-level surface expected for the web UI.

Change:

- Add `internal/web`.
- It should contain:
  - HTTP server startup.
  - Static frontend serving.
  - SSE or WebSocket stream endpoint.
  - Request handlers for interactive prompt submission.
  - Request handlers for permission and ask responses.
  - Session list/resume handlers.
  - Model/config display handlers.
  - Workspace status/diff display handlers only if backed by existing safe
    functionality.

Must not contain:

- Agent loop logic.
- Tool execution logic.
- Permission policy logic.
- Session JSONL parsing logic.
- Orchestration transition logic.
- Replay, benchmark, metrics, audit, or lifecycle command implementations.

Primary interface:

- `web.Run(ctx context.Context, cfg web.Config) error`
- `web.Config` should accept already-wired runtime dependencies from
  `internal/cli`, not construct them itself.

### 7. Web prompter implementation

Responsible components:

- Interface owner: `internal/permission`
- Web implementation owner: `internal/web`

Current interface:

- `permission.Prompter` already exists.

Change:

- Implement `permission.Prompter` in `internal/web`.
- It should:
  - create a pending permission request record,
  - publish it to the browser event stream,
  - block until the browser responds or context is canceled,
  - return `permission.Decision` and optional session rule.

Do not change:

- `internal/permission` rule matching semantics.
- `internal/tool.Orchestrator` permission enforcement semantics.

### 8. Web asker implementation

Responsible components:

- Interface owner: `internal/tool`
- Web implementation owner: `internal/web`

Current interface:

- `tool.Asker` already exists.

Change:

- Implement `tool.Asker` in `internal/web`.
- It should:
  - create a pending ask request,
  - publish it to the browser event stream,
  - block until browser answer or context cancellation,
  - return `tool.AskResponse`.

Do not:

- Reuse `internal/tui.InteractiveAsker`; it is Bubbletea-specific.

### 9. `internal/slash/command.go`: presentation-neutral result names

Responsible component: `internal/slash`

Current issue:

- `slash.Result` fields and comments are TUI-specific:
  - `ShowTeamsDialog`
  - `ShowModelDialog`
  - `ShowResumeDialog`
  - comments say "TUI".

Change:

- Keep slash command semantics in `internal/slash`.
- Rename or comment these as presentation-neutral UI intents.

Suggested field naming:

- `ShowTeams` or `OpenTeams`
- `ShowModelPicker` or `OpenModelPicker`
- `ShowResumePicker` or `OpenResumePicker`

Required downstream updates:

- `internal/tui/handlers.go` consumes renamed fields.
- Future `internal/web` consumes the same result intents.

Do not:

- Move slash command execution into web.
- Split web-only slash commands from CLI slash commands in this change.

### 10. `internal/cli/subcommands.go`: keep one-off slash CLI behavior

Responsible component: `internal/cli`

Current role:

- Registers slash-backed CLI subcommands.
- Runs prompt-type slash commands non-interactively.
- Runs local slash commands from CLI.

Change:

- Leave this path independent.
- If extracting shared slash dependency construction from `RunInteractive`, reuse
  it only when it does not alter CLI command behavior.

Do not:

- Route `commit`, `review`, `doctor`, `config`, or `skills` CLI subcommands into
  web.

### 11. `internal/query/event.go`: add orchestration progress events

Responsible component: `internal/query`

Current issue:

- `query.LoopEvent` has typed tool/model/agent/lifecycle events, but
  orchestration progress is currently emitted as `TextEvent` from
  `internal/orchestration`.

Change:

- Add typed `LoopEvent` variants for orchestration progress.

Suggested event types:

```go
type OrchestrationStartedEvent struct { Name string; Initial string }
type OrchestrationStateStartedEvent struct { StateID string; PersonaID string; Control string }
type OrchestrationStateCompletedEvent struct { StateID string; Duration time.Duration }
type OrchestrationControlEvent struct { StateID string; Control string; Event string }
type OrchestrationTransitionEvent struct { From string; Event string; To string }
type OrchestrationHandoffEvent struct { StateID string; Event string; Path string; Direction string }
type OrchestrationCompletedEvent struct { Name string }
```

Use these for UI state. Text may still be emitted for CLI compatibility, but UI
must not parse text for orchestration state.

### 12. `internal/observe/event_catalog.go`: add matching durable orchestration events

Responsible component: `internal/observe`

Current issue:

- `observe` has durable events for query, tools, lifecycle, sessions, MCP, etc.,
  but no typed orchestration events.

Change:

- Add observe events equivalent to the query orchestration progress events.
- Update `observe.UnmarshalEvent` in `internal/observe/event.go` for the new
  event kinds.

Reason:

- Browser live stream can use `query.LoopEvent`.
- Replay/session evidence needs durable `observe.Event` records.

### 13. `internal/orchestration/runner.go`: replace UI-state text with typed events

Responsible component: `internal/orchestration`

Current locations:

- Emits state/control/transition/done messages as `query.TextEvent`.
- Reads and writes handoff paths through helpers hardcoded to `/tmp/pragma`.

Change:

- Emit typed `query.Orchestration*Event` values at:
  - run start,
  - state start,
  - control start/result,
  - state complete,
  - handoff written/read,
  - transition,
  - run complete.
- Also emit matching `observe.Orchestration*` events through the existing engine
  bus if available.

Interface change:

- Add an orchestration run options struct:

```go
type RunOptions struct {
    PersonaDir string
    TaskPrompt string
    ArtifactRoot string
}
```

- Keep compatibility functions if needed:
  - `RunFileEvents(ctx, engine, path, personaDir, taskPrompt)` can call the new
    option-based runner with default artifact root.

Do not:

- Move FSM semantics to web.
- Make web decide transitions.

### 14. `internal/orchestration/runner.go`: remove hidden global handoff root

Responsible component: `internal/orchestration`

Current issue:

- `EnsureRunDirs` removes `/tmp/pragma/handoff-prompts` globally.
- `handoffPromptPath` always writes under `/tmp/pragma/handoff-prompts`.

Change:

- Make handoff/process paths derive from an injected artifact root.
- Replace helpers:
  - `EnsureRunDirs(def)` -> `EnsureRunDirs(def, artifactRoot string)`
  - `handoffPromptPath(stateID, event)` -> method/helper using artifact root
  - `selectedHandoffPrompt(stateID, event)` -> uses artifact root
  - `RenderNextHandoffInstructions(def, state)` -> accepts artifact root
  - `BuildPrompt(...)` -> accepts artifact root or prompt options.

Compatibility:

- Non-web CLI orchestration can keep default root behavior, but it must be
  explicit in one place.
- Interactive web orchestration must pass a per-session or per-run artifact root.

### 15. `cmd/pragma/orchestration.go`: update to new orchestration runner interface

Responsible component: `cmd/pragma` command adapter

Current role:

- Parses `orchestration run`.
- Calls `orchestration.RunEvents`.
- Prints loop events for CLI.

Change:

- Keep command behavior independent from web.
- Update calls to use new option-based orchestration runner.
- For CLI compatibility, use the CLI default artifact root.
- Update `printOrchestrationEvents` to print typed orchestration events in the
  same human-readable form currently produced by text events.

Do not:

- Import or call web.

### 16. `cmd/pragma/orchestration_test.go` and `internal/orchestration/*_test.go`

Responsible components:

- Command adapter tests: `cmd/pragma`
- FSM tests: `internal/orchestration`

Current issue:

- Tests assert hardcoded `/tmp/pragma/handoff-prompts/...` prompt paths.

Change:

- Update tests to pass an artifact root explicitly.
- Assert generated prompt contains the passed root.
- Add coverage for typed event emission if existing tests are already touching
  runner behavior.

Note:

- This is listed as implementation prep. Do not run or add tests until asked.

### 17. `internal/session`

Responsible component: `internal/session`

Current role:

- Owns durable conversation/session JSONL.

Change:

- Keep ownership unchanged.
- Add only minimal fields/entries if web interactive UI needs durable references
  to interactive presentation state.
- Prefer existing session entries and observe logs for evidence.

Do not:

- Store raw HTTP replay payloads in session.
- Store web UI view state in session.
- Parse replay directories.

Likely small changes:

- Ensure prompt history save remains presentation-neutral.
- If artifact root is needed for resume, add a session metadata field or
  separate entry only after confirming no existing metadata field is sufficient.

### 18. `internal/app`

Responsible component: `internal/app`

Current role:

- Owns canonical in-memory app/session state.

Change:

- Keep `StateStore` as canonical live state for interactive UI.
- Add narrow read/update helpers only if web handlers need stable access patterns
  that should not duplicate TUI logic.

Do not:

- Add web-specific fields.
- Add a new run-state package just to mirror `AppState`.

Possible additions:

- Explicit current model/provider update method if model picker becomes shared
  between TUI and web.
- Explicit conversation reset/resume helpers if currently implemented only in
  TUI handlers.

### 19. `internal/cli/run.go`: prompt history ownership cleanup

Responsible component: `internal/cli` for loading, `internal/session` for data

Current issue:

- `promptHistoryFromSessions` uses `tui.ExtractUserPrompts`, creating a CLI to
  TUI dependency for fallback extraction.

Change:

- Move prompt extraction fallback to `internal/session` or `internal/model`.
- `internal/cli` calls the non-presentation helper.
- `internal/tui` may continue to use it through the same helper.

Reason:

- Web should not depend on TUI for prompt history behavior.

### 20. `internal/tui/permission.go`: tool input preview extraction

Responsible component:

- Current owner: `internal/tui`
- Future shared owner if needed: existing `internal/tool` or a small render
  helper under presentation-neutral package.

Current issue:

- `parseToolPreview` is TUI-local but web will need equivalent display summaries.

Change:

- Do not copy this logic into web.
- Either leave web with raw JSON in first implementation or extract a
  presentation-neutral tool summary helper.

Preferred first implementation:

- Web displays tool name, input JSON, and content string from permission prompt.
- Defer shared preview extraction until duplication actually appears.

### 21. `internal/observe/bus.go`

Responsible component: `internal/observe`

Current role:

- EventBus is the runtime event backbone.

Change:

- Add or expose a subscriber suitable for web streaming if current subscribers
  are not enough.
- Preserve existing logging/metrics/audit subscribers.

Needed behavior:

- Web stream should receive ordered events.
- Web stream should not consume events exclusively.
- If reconnect is included in first implementation, keep recent event buffer in
  web presentation layer or add a generic replay-safe subscriber without
  changing event ownership.

### 22. `internal/web` frontend scope

Responsible component: `internal/web`

First UI scope:

- Run console for the active interactive session.
- Live transcript from `query.LoopEvent` and/or `observe.Event`.
- Prompt submit.
- Permission prompt response.
- Ask prompt response.
- Session list/resume.
- Model/config display.
- Tool call/result display.
- Orchestration progress display only when an interactive slash command launches
  orchestration.

Explicitly excluded:

- Replay Lab.
- Benchmark Lab.
- Metrics Lab.
- Audit Lab.
- Lifecycle Lab.
- One-off CLI command execution UI.

### 23. `docs/native-web-ui-plan.md`

Responsible component: documentation

Current issue:

- It still describes web as covering replay, benchmark, lifecycle, metrics, and
  audit surfaces, which conflicts with `docs/component-responsisbilites-plan.md`.

Change:

- Prune to interactive UI scope:
  - active run console,
  - transcript,
  - permissions,
  - ask prompts,
  - sessions/resume,
  - model/config display,
  - workspace status/diff,
  - tool calls/results,
  - interactive orchestration only.
- Remove or mark out of scope:
  - Replay Lab,
  - Benchmark UI,
  - Lifecycle UI,
  - Operations UI for audit/metrics/background command management.

### 24. `docs/component-responsisbilites-plan.md`

Responsible component: documentation

Change:

- Keep as the ownership source.
- Optionally append this implementation change-point document as the execution
  companion.
- Do not broaden the scope back into "web as mode".

## Suggested Implementation Order

1. Documentation alignment:
   - update `docs/native-web-ui-plan.md` to match interactive-only scope.
2. Interface neutralization:
   - clean `slash.Result` naming/comments,
   - move prompt history extraction out of `internal/tui`.
3. Interactive runtime extraction:
   - split `RunInteractive` setup from Bubbletea launch.
4. Web presentation package:
   - add thin `internal/web` with `Run`.
5. Web prompter/asker:
   - implement existing interfaces.
6. Route root interactive path:
   - `RunDispatcher` still calls `RunInteractive`,
   - `RunInteractive` launches web.
7. Orchestration typed events:
   - add query/observe events,
   - update orchestration runner,
   - update CLI printer.
8. Orchestration artifact root:
   - inject artifact root,
   - update prompt path generation,
   - update tests later when requested.

## Boundary Checks Before Coding

- No new package except `internal/web` unless a missing boundary is proven.
- Web imports existing packages, but existing domain packages do not import web.
- TUI and web share interfaces, not message structs or presentation widgets.
- One-off CLI commands continue to work without a browser.
- Orchestration semantics remain in `internal/orchestration`.
- Permission decisions remain in `internal/permission`.
- Tool execution remains in `internal/tool` and `internal/tools/*`.
- Session persistence remains in `internal/session`.

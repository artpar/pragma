# Web UI Backend Gap Report

This report identifies the backend work required to support the web workbench
described in `/Users/artpar/workspace/code/insidious/agent4/pragma-webui-vision.md`.
It is based on the current worktree as of 2026-06-06.

## Requirement Source

The external vision describes Pragma as a browser-based control room for
autonomous software work, not a chat transcript. Backend support must therefore
make these product requirements possible:

- Active run workbench: provider/model, workspace, session ID, run state, recent
  sessions, composer, and typed activity.
- Full-payload data rule: events, messages, sessions, tool calls/results,
  permission requests, ask requests, and orchestration payloads must be visible
  without silently dropping fields.
- Typed event timeline: model, tool, permission, ask, orchestration, retry,
  compaction, MCP, agent, error, session, and file-effect events are distinct
  records.
- Inspector: every selected run, event, session, message, prompt, file, handoff,
  or workflow row must have complete JSON plus related records.
- Session recovery: resume must show what happened, what changed, what remains,
  and the full loaded session payload.
- Permission control: blocking permission prompts must show exact request data,
  policy/rule context, and submitted responses.
- Orchestration view: state/control/transition/handoff rows come from typed
  events, not parsed text.
- Handoff view: orchestration handoff prompt files and session handoff state stay
  distinct.
- Supporting views: MCP/tool health, session library, delegated/background
  agent cards, cost/context state, audit export, and search/filtering over the
  full payloads.

## Current Backend Baseline

The current backend already has the right high-level ownership shape.

- `cmd/pragma/main.go` keeps Cobra wiring and routes execution through
  `cli.RunDispatcher`.
- `internal/cli/run.go` routes the no-prompt interactive path into
  `RunInteractive`, builds an `InteractiveRuntime`, and launches `internal/web`.
- `internal/cli.BuildInteractiveRuntime` owns dependency setup, provider/model
  setup, tool registration, permission checker setup, session writer setup,
  slash registry/deps, compaction, MCP wait, and prompt history.
- `internal/web/web.go` owns HTTP server startup, static UI serving, SSE,
  prompt submission, cancel, permission response, ask response, session list,
  resume, artifact reads, and slash/orchestration completions.
- `internal/query/event.go` defines typed loop events for text, thinking, model
  request/response, tool calls/results, compaction, lifecycle, orchestration,
  agent progress, retry, and errors.
- `internal/observe/event_catalog.go` defines durable evidence events for API,
  tools, permissions, sessions, subagents, MCP, lifecycle, orchestration, hooks,
  and errors.
- `internal/session` stores canonical session JSONL entries and loads a
  `session.Session` with conversation, handoff state, prompt history, file
  state, todos, cost, turn count, token usage, and content replacements.

The main remaining backend work is not another runtime. It is a canonical
source-preserving web data plane around the runtime that already exists.

## Required Backend Changes

### 1. Replace web-specific reduced events with canonical envelopes

Current evidence:

- `internal/web` defines reduced structs such as `webToolCallEvent`,
  `webToolResultEvent`, `webTurnCompleteEvent`, `webCompactionEvent`,
  `webOrchestration*Event`, `webAgentProgressEvent`, and `webErrorEvent`.
- `normalizeLoopEvent` rewrites `query.LoopEvent` values into `web.*` data
  types and drops fields. Examples: `query.ToolCallEvent` becomes ID/name/input
  only; `query.ToolResultEvent` loses the full `model.ToolResultPart` shape;
  `query.TurnCompleteEvent` loses the full `model.Response`.

Required change:

- Introduce a canonical web event envelope that carries:
  - sequence number
  - browser/server received time
  - source stream, for example `loop`, `observe`, `web_request`,
    `web_response`, `runtime_state`
  - event name
  - concrete Go data type
  - run ID
  - session ID when known
  - turn index when known
  - correlation IDs when known: trace ID, span ID, parent span ID, tool call ID,
    state ID, task/agent ID
  - complete source object
  - optional preview metadata outside the source object
- Stop replacing canonical objects with web-specific event structs.
- If the frontend needs a compact row label, generate that as envelope metadata
  or client-side projection, never as the only data payload.

Backend owner:

- `internal/web` can own the transport envelope.
- `internal/query`, `internal/observe`, `internal/session`, `internal/app`,
  `internal/permission`, `internal/tool`, and `internal/orchestration` remain
  the source-object owners.

### 2. Stream `observe.Event` into the web run timeline

Current evidence:

- The web SSE stream is fed by `server.start`, which publishes accepted prompts,
  slash results, and `interactive.LoopEvent` values.
- `observe.EventBus` already carries richer runtime evidence, including full
  API request payloads, API response content, tool execution, permissions,
  session lifecycle, MCP, subagent, lifecycle, orchestration, hooks, and errors.
- Providers already emit full `APIRequestStarted` payloads through
  `provider/shared.RequestStartedEvent`, including messages, system prompt,
  tools, thinking config, temperature, and response schema.

Required change:

- Add a web subscriber to the existing `observe.EventBus`.
- Publish each `observe.Event` into the same web event envelope stream as loop
  events.
- Preserve the original event object. Do not convert it to log text.
- Add correlation so a model request row can link to API events, text/thinking
  deltas, model response, tool calls/results, permission decisions, session save,
  and errors.

Backend owner:

- `internal/web` owns the subscriber and transport.
- `internal/observe` remains the event catalog owner.

### 3. Add a durable run/event store instead of a 200-event live buffer

Current evidence:

- `web.hub` keeps only the last 200 envelopes in memory.
- Session JSONL persists selected session state and messages, not the full live
  web event timeline.
- `query.ShouldPersistSessionEvent` only marks model request/response, tool
  call/result, turn complete, and compaction events as session-persistence
  checkpoints.

Required change:

- Introduce a durable per-run event log, or extend session persistence with a
  distinct source-preserving event-log section. It must persist complete web
  envelopes or canonical source events, not reduced UI rows.
- Store enough metadata to support:
  - reconnect/backfill after browser refresh
  - session resume recovery sheet
  - session library drill-down
  - audit export
  - full-payload search/filtering
  - related-record lookup
- Keep session JSONL and run event history conceptually distinct if needed:
  session JSONL is conversation/state persistence; run event history is runtime
  evidence.

Backend owner:

- Prefer `internal/session` or a new `internal/runhistory`/`internal/eventlog`
  package for persistence.
- `internal/web` should query/stream it, not own file format semantics.

### 4. Expose full session source APIs

Current evidence:

- `session.Store.Load` returns the full `session.Session`.
- `web.handleSessions` currently returns slash resume candidates, not a
  source-complete session library object with raw `SessionSummary` attached.
- `web.resume` publishes `Store.Snapshot().Conversation` as `session_resumed`,
  which loses `session.Session` fields such as handoff state, content
  replacements, prompt history, file state, todos, cost, token usage, summary,
  system override, and git remote.

Required change:

- Add session source endpoints:
  - `GET /api/sessions` returns session rows with the complete
    `session.SessionSummary` object attached.
  - `GET /api/sessions/{id}` returns the full `session.Session`.
  - `GET /api/sessions/{id}/entries` returns exact JSONL entries, preserving
    `kind` and raw `data`.
- Change resume to publish a `session_resumed` envelope whose source object is
  the full loaded `session.Session`, plus the active app-state snapshot after
  resume as a separate state event.
- Keep current workdir validation and provider rebinding in `internal/cli`.

Backend owner:

- `internal/session` owns load/list/entry APIs.
- `internal/web` exposes HTTP handlers over those APIs.

### 5. Define a canonical run object and lifecycle

Current evidence:

- The web server has `running bool` and `cancel context.CancelFunc`.
- `/api/state` returns runtime version/workspace/model/provider/MCP server names
  and `app_state`.
- The external vision treats `run` as the top-level UI object. It also accepts
  the current backend limitation of one active interactive run, but the UI must
  not be shaped as a transcript tab.

Required change:

- Define a canonical run snapshot with:
  - run ID
  - status: idle, accepting_input, running, waiting_permission, waiting_ask,
    cancelling, cancelled, failed, completed
  - provider/model
  - workspace
  - active session ID
  - current prompt/run start time
  - current turn index
  - current orchestration name/state when present
  - latest blocking permission/ask request IDs
  - recent model/tool/error activity IDs
  - current cost/token/metrics snapshot
  - app state source object
- Emit run lifecycle events when status changes.
- Keep single active run enforcement for now, but express it as run-state
  semantics instead of an untyped `running` boolean.

Backend owner:

- Runtime state can live in `internal/app` or a small runtime-owned package.
- `internal/web` can expose snapshots and stream lifecycle events.

### 6. Surface cost, token, context, and metrics state

Current evidence:

- `model.CostTracker`, `observe.Metrics`, and `observe.TokenMonitor` already
  exist.
- TUI receives `TokenMonitor` and displays cost/metrics; web config currently
  receives `CostTracker` and `Metrics` but not `TokenMonitor`.
- `/api/state` does not include cost entries, total cost, metrics snapshot,
  context window budget, token monitor state, or warning state.

Required change:

- Pass `TokenMonitor` into `web.Config`.
- Extend run state with:
  - total cost
  - cost entries
  - token usage
  - turn count
  - API/tool/error counts
  - context window budget
  - current estimated context percent when available
  - latest compaction/retry/context-overflow warning event IDs
- Use existing metrics/cost/token monitors as the source.

Backend owner:

- `internal/cli` wires the existing monitors.
- `internal/web` exposes snapshots.
- `internal/observe` remains metrics/event owner.

### 7. Preserve canonical messages, responses, reasoning, and content parts

Current evidence:

- `model.Message`, `model.Response`, and `model.ContentPart` already have
  discriminator-based JSON for text, image, document, tool call, tool result,
  and thinking parts.
- `query.ThinkingEvent` streams reasoning deltas.
- `query.TurnCompleteEvent` carries a full `model.Response`, but web currently
  reduces it to stop reason only.

Required change:

- Stream full `query.TextEvent`, `query.ThinkingEvent`, and
  `query.TurnCompleteEvent` source objects.
- Make the inspector able to fetch the canonical `model.Message` and
  `model.Response` records by message ID or event ID from the run/session source.
- Preserve thinking signatures/redacted-thinking fields as source data. Any
  collapsed reasoning preview must be a projection with the full object attached.

Backend owner:

- `internal/model` already owns the record shape.
- `internal/web` must stop reducing it.

### 8. Upgrade permission and ask prompt contracts

Current evidence:

- `web.Bridge` implements `permission.Prompter` and `tool.Asker`.
- Permission requests are currently published as a generic map with ID, tool,
  input, content, and reason.
- Permission responses accept only `allow` or `deny` plus `remember`.
- `permission.CheckResult` already carries decision, matched rule, reason, and
  checked content.
- `permission.RuleChecker` supports session rules and persistent local rules.
- `observe` has permission rule/decision events, but those are not the same as a
  complete blocking prompt source object.

Required change:

- Define a canonical `PermissionRequest` source object containing:
  - request ID
  - tool name
  - raw tool input
  - extracted checked content
  - check result: default/matched decision, reason, rule, source
  - available decisions for this prompt
  - whether session rule and persistent local rule creation are supported
  - optional risk/impact fields if the owning tool can provide them
  - created timestamp and run/session/tool call IDs when known
- Define a canonical `PermissionResponse` source object containing:
  - request ID
  - chosen action: allow once, deny once, allow for session, deny for session,
    create local allow rule, create local deny rule
  - created rule object when applicable
  - submitted raw body
  - result/error
- Map response actions to `permission.Decision`, `AddSessionRule`, and
  `AddPersistentRule` in backend code owned by the permission bridge, not in the
  frontend.
- Publish both request and response as full envelopes.
- Keep `internal/permission` as the policy owner; web is transport and
  presentation only.
- Do the same for `tool.AskRequest`/`tool.AskResponse`: request ID, full request,
  full submitted response, run/session correlation, and result/error.

Backend owner:

- `internal/web` can own bridge request/response transport types.
- `internal/permission` and `internal/tool` continue to own semantics.

### 9. Move workflow projection out of the web package or make it explicitly derived

Current evidence:

- `query.Orchestration*Event` types already exist for started, state started,
  control, handoff, transition, state completed, and completed.
- `observe.Orchestration*` events also exist.
- `internal/web` currently reconstructs a `workflowSnapshot` from loop events.
  That snapshot has web-owned state rows, transitions, and handoffs.

Required change:

- The source of truth for the Workflow tab must be the typed orchestration
  events.
- Remove web-owned workflow semantics from `internal/web`, or mark the snapshot
  as an explicitly derived projection that always references the source event
  sequence numbers used to build it.
- Prefer a canonical projector owned by `internal/orchestration` or
  `internal/query` if the frontend needs current-state convenience data.
- Include durations and current state from source events, not inferred from wall
  time in web unless that timestamp is explicitly presentation metadata.

Backend owner:

- `internal/orchestration` owns FSM semantics.
- `internal/query` owns live loop event emission.
- `internal/web` streams and displays source/projected data.

### 10. Support handoff file and session handoff source access

Current evidence:

- `query.OrchestrationHandoffEvent` carries state ID, event, path, and
  direction.
- `web.handleArtifact` only allows reading paths remembered in the current live
  server process.
- Session handoff state is persisted in `session.Session.HandoffState` and
  `app.AppState.HandoffState`.

Required change:

- Add durable handoff index records keyed by run/session/orchestration event.
- Add source endpoints for:
  - exact orchestration handoff prompt file content by event/path
  - exact `query.OrchestrationHandoffEvent`/`observe.OrchestrationHandoff`
  - current and resumed session handoff state JSON
- Rehydrate known handoff artifact paths on resume from session/run history, not
  only from the live in-memory `artifacts` map.
- Keep file reads constrained to paths emitted by canonical handoff events.

Backend owner:

- `internal/orchestration`/`internal/query` emit events.
- Run/session event history stores the index.
- `internal/web` exposes safe read endpoints.

### 11. Add canonical file-effect and diff records

Current evidence:

- `query.ToolResultEvent` carries `[]tool.FileEffect`.
- `webToolResultEvent` reduces file effects to path and operation.
- The vision calls for file effects grouped by session, turn, and originating
  tool call, with drill-down to file/hunk and freshness labels.

Required change:

- Extend file-effect records or add a canonical file-change record containing:
  - session ID
  - run ID
  - turn index
  - tool call ID
  - tool name
  - path
  - operation
  - before/after metadata when available
  - diff/hunk data when available
  - created time
  - freshness/worktree drift state when inspected later
- Add backend endpoints for current git status/diff scoped to the active
  workspace, plus mapping from diff hunks back to originating file-effect events
  where known.
- Do not make web infer file changes by parsing display text.

Backend owner:

- `internal/tool`/individual tools should emit file-effect truth.
- A workspace/diff helper can live outside `internal/web`.
- `internal/web` exposes the records.

### 12. Expose tool registry, toolset, and MCP status as canonical state

Current evidence:

- `internal/cli` registers tools and applies tool filters/toolsets.
- `internal/mcp` owns MCP manager state, and slash deps already expose MCP
  status for `/mcp`.
- Web config currently gets only MCP server names and slash completions.
- The vision expects MCP/tool registry views with connection health, auth state,
  exposed tools/resources, latency/last-seen, and loaded scope.

Required change:

- Add backend endpoints for:
  - active tool registry descriptors and schemas
  - active tool exposure policy: allowed/disallowed/toolset-derived
  - MCP server status with source/config path if available
  - MCP auth/OAuth status events
  - MCP tools/resources visible to the agent
  - health/last-seen/latency where available
- Keep MCP protocol internals in `internal/mcp`; web only exposes summaries and
  source objects.

Backend owner:

- `internal/cli` wires registry/MCP manager into `web.Config`.
- `internal/mcp` owns MCP source data.
- `internal/tool` owns tool descriptors.

### 13. Add delegated-agent/task board APIs

Current evidence:

- `internal/task.Registry` can create/list/cancel tasks, list teammates, deliver
  messages, heartbeat tasks, and reap dead agents.
- Tools exist for `TaskCreate`, `SendMessage`, and `TaskStop`.
- `query.AgentProgressEvent` and observe subagent events exist.
- Web currently has no task/agent list, cancel, message, or agent-card endpoint.

Required change:

- Add task/agent endpoints:
  - list all tasks
  - list running/all teammates
  - get task by ID
  - cancel task
  - send message to task/agent
  - fetch task event history/progress
- Stream task status changes and `query.AgentProgressEvent`/observe subagent
  events into the run timeline.
- Correlate each agent card with current task, branch/worktree when available,
  status, phase/progress, last activity, last tool, token/cost estimate, errors,
  and controls.
- Decide whether user-driven cancel/message should call registry methods
  directly or go through the existing tools. The backend should own that policy;
  the frontend must not fake tool calls.

Backend owner:

- `internal/task` owns task state and controls.
- `internal/web` exposes HTTP/SSE transport.

### 14. Add run configuration and provider/model APIs

Current evidence:

- CLI flags cover model, provider, max tokens, temperature, thinking,
  thinking-budget, context mode, handoff schema, toolset, allowed/disallowed
  tools, permission mode, output schema, prompt, resume, and continue.
- `RunInteractive` builds one runtime with a fixed effective config at process
  startup.
- Slash `/model` can switch model through `slash.Deps.ModelSwitcher`, but web has
  no canonical config endpoint or mutation policy.

Required change:

- Add `GET /api/config/effective` for the effective runtime config, including
  source/default information where available.
- Add provider/model list endpoints sourced from provider model listers.
- Define which fields are mutable before a run starts, between turns, or never
  mutable without restarting:
  - likely mutable between turns: model when provider-compatible, max tokens,
    temperature, thinking settings if engine/provider can accept updated config,
    permission mode if checker supports it
  - likely restart-required: provider if credentials/base URL differ, context
    mode, handoff schema, toolset/tool filters, system prompt
- Add mutation endpoints only where the owning runtime can update the canonical
  state safely. Otherwise expose read-only state and explicit restart guidance.
- If structured-output schema becomes interactive, move it into
  `query.EngineConfig.ResponseSchema` before provider requests, not a web-only
  field.

Backend owner:

- `internal/config`, `internal/provider`, and `internal/cli` own config/provider
  resolution.
- `internal/query.Engine` owns per-request config use.
- `internal/web` exposes controls.

### 15. Add full-payload search, filtering, and related-record lookup

Current evidence:

- The current web stream can filter client-side only over recent in-memory
  events.
- There is no backend event index keyed by session, event type, tool call ID,
  trace/span ID, state ID, file path, or task/agent ID.

Required change:

- Add query endpoints over durable event/session history:
  - filter by item type
  - full-payload text search
  - event by ID/sequence
  - related records by tool call ID, trace ID, span ID, session ID, state ID,
    file path, task/agent ID
  - pagination/range backfill
- The filter result may be reduced as an index row, but opening a row must return
  the full source envelope.

Backend owner:

- Event-log/session-store package owns indexing.
- `internal/web` owns HTTP handlers.

### 16. Add audit export based on the same source-preserving records

Current evidence:

- `observe.Recorder` can write event JSONL when `--record` is enabled.
- `SetupDeps` always writes per-execution JSON logs.
- The web backend does not expose structured audit export for the active run or
  session.

Required change:

- Add export endpoints for:
  - active run event log
  - session event history
  - permission decisions
  - file effects
  - cost/token summary
  - errors/retries/compactions
- Export from durable canonical records, not rendered UI state.

Backend owner:

- Event-log/session-store package provides data.
- `internal/web` exposes export.

### 17. Tighten security boundaries for browser endpoints

Current evidence:

- Server binds to `127.0.0.1:0`, which is good for local-only default.
- Artifact reads are constrained to paths emitted by the current run, but only in
  memory.
- Completion endpoints read the filesystem under the workspace for orchestration
  path suggestions.

Required change:

- Keep local-only bind by default.
- Add CSRF/session token protection for mutating endpoints if the browser UI is
  intended to be left open.
- Apply method checks consistently to all endpoints, including session listing if
  expanded.
- For file/artifact endpoints, constrain reads to canonical event-emitted paths
  or active workspace paths according to endpoint purpose.
- Never expose API keys or credential contents through config/state endpoints.
- Redact or explicitly gate sensitive fields only where the owning source layer
  defines redaction semantics. Do not silently omit unknown fields.

Backend owner:

- `internal/web` owns HTTP safety.
- `internal/config` owns credential loading/redaction rules.

### 18. Add plan-review-run backend support

Current evidence:

- The vision calls for explicit Plan and Run launch actions, with the plan shown
  as an artifact that can be edited, approved, or rejected before tool execution.
- Current interactive input goes through `InteractiveRuntime.RunInput`, which
  routes either a slash command or a normal prompt directly into the engine.
- There is task/todo support and prompt guidance about planning, but no
  canonical run mode that produces a plan artifact and gates tool execution on
  user approval.

Required change:

- Add a plan-first run contract:
  - launch mode: `plan` or `run`
  - plan artifact ID
  - proposed plan source object
  - approval/rejection/edit response object
  - transition event from planning to execution
  - clear rule for whether model/tool calls are allowed before approval
- Store plan artifacts in the run/session event history.
- If the plan is edited, preserve both original and edited source objects.
- Execution after approval should call the existing `query.Engine`; do not add a
  parallel web-only agent loop.
- Decide whether plan generation is implemented by a dedicated prompt, a slash
  command, or a query engine option. The owner must be runtime/query/slash, not
  frontend parsing.

Backend owner:

- `internal/query` or `internal/slash` should own plan-generation semantics.
- `internal/web` owns Plan/Run transport and approval response submission.
- Run/event history owns durable plan artifacts.

### 19. Add recovery and workspace drift APIs

Current evidence:

- Resume currently validates workdir and reloads canonical session state.
- Session state already contains conversation, handoff state, todos, file state,
  cost, tokens, prompt history, and content replacements.
- Worktree tools can detect worktree changes, and git status/diff information is
  available through tools/slash commands, but the web backend has no canonical
  recovery summary or workspace drift endpoint.

Required change:

- Add a resume recovery snapshot with:
  - full loaded session payload
  - last user goal/prompt
  - last run status and terminal/error event
  - files changed during the session
  - current git status/diff summary
  - pending todos and handoff state
  - unresolved blockers/errors
  - current branch/worktree
  - permission mode and active session rules
  - drift since last session save, including changed/deleted files when
    detectable
- The summary can be a derived projection, but each field must link to source
  records or command output records.
- Do not have web reconstruct recovery state from message text.

Backend owner:

- `internal/session` supplies loaded session source.
- A workspace status helper outside `internal/web` should own git status/diff
  reads.
- Run/event history supplies last run/error/file-effect evidence.

### 20. Add background process session visibility if full vision includes away-mode work

Current evidence:

- `pragma --bg -p ...` uses `internal/background.Registry` to write PID files
  under `~/.pragma/active-sessions`.
- `background.ProcessInfo` includes PID, PGID, session ID, CWD, status, log
  path, model, provider, prompt, and timestamps.
- `background.StatusSubscriber` updates process status from runtime events.
- The current web plan explicitly keeps background process management out of the
  first interactive UI scope, but the external vision emphasizes visible and
  interruptible background work.

Required change:

- If the full workbench should include background process runs, add backend
  endpoints for:
  - list active background processes
  - get process info by PID/session ID
  - stream or fetch safe log tails
  - cancel process group
  - attach session history when a background process reports a session ID
  - remove stale entries
- Keep this separate from delegated in-process task/agent cards. Background
  process sessions are OS processes; `internal/task` entries are in-process
  agent/task records.
- If background process management stays out of first release, record that as an
  explicit product-scope decision rather than leaving an accidental gap.

Backend owner:

- `internal/background` owns PID registry semantics.
- `internal/web` can expose local-only management endpoints if included.

## Change Priority

### Required before the first acceptable version

1. Canonical web event envelopes with full source objects.
2. Remove or bypass web-specific reduced event structs for source payloads.
3. Stream loop events without field loss.
4. Full permission/ask request and response envelopes.
5. Full session list/detail/resume source payloads.
6. Durable run/event history or enough session-backed history to survive refresh
   and resume.
7. Canonical run snapshot with session/model/workspace/status/blocking prompt.
8. Handoff source access for emitted files plus session handoff state.
9. `/api/state` expanded with cost, metrics, token/context, and app state.
10. Security hardening for mutating browser endpoints.
11. Plan/Run launch contract if plan-first is part of the first release.

### Required for seamless implementation of the full vision

1. Observe-event bridge into web.
2. Related-record index and full-payload search.
3. Workflow projection moved out of web or marked as derived from event IDs.
4. File-effect/diff records with origin mapping.
5. Tool registry/toolset/MCP source endpoints.
6. Delegated-agent/task board APIs and controls.
7. Run configuration/provider/model APIs with mutation policy.
8. Audit export from canonical records.
9. Resume recovery/workspace drift projections linked to source evidence.
10. Background process session APIs if away-mode `--bg` work is in scope.

## Explicit Non-Gaps

These do not require a backend rewrite:

- The agent loop should remain in `internal/query`.
- Tool execution should remain in `internal/tool` and `internal/tools/*`.
- Permission policy should remain in `internal/permission`.
- Session JSONL ownership should remain in `internal/session`.
- Orchestration state-transition semantics should remain in
  `internal/orchestration` and its query/observe events.
- The root interactive path already launches the web UI through `internal/cli`;
  the required work is data-plane quality, not routing.

## Completion Criteria For Backend Readiness

Backend support is ready for the web UI when the following can be proven from
runtime behavior and source payload inspection:

- Starting `pragma` opens a browser UI backed by a canonical run object.
- `/api/state` exposes full runtime/app state plus cost/metrics/context data
  without dropping source fields.
- Every loop event is streamed as a complete source object with concrete type
  metadata.
- Every observe event relevant to the run is streamed or persisted as a complete
  source object.
- Refreshing the browser can backfill event history from durable storage.
- Opening any event returns the full source envelope.
- Session list rows carry complete `SessionSummary` data.
- Resume publishes the full loaded `session.Session`, not just the conversation.
- Permission and ask prompts show full request objects and publish full response
  objects.
- Orchestration workflow rows are built from typed events, with source event IDs
  available in the inspector.
- Handoff file reads are backed by emitted handoff events and resume-safe
  history.
- File effects can be grouped by session, turn, and tool call without parsing
  display text.
- Plan-first runs produce durable plan artifacts and approval/rejection/edit
  records before execution.
- Resume recovery can explain last state, changed files, blockers, todos,
  branch/worktree, and drift from source records.
- Task/agent cards can be backed by `internal/task` state and agent progress
  events.
- Background process runs are either explicitly out of scope or backed by
  `internal/background` process/session APIs.
- Search/filter/related lookups operate over stored full payloads and return
  complete source objects on drill-down.

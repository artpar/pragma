# Architecture Boundary Audit

Current-state audit of responsibility boundaries, duplicated control flow, lifecycle mistakes, persistence boundary errors, ownership leaks, and state-machine/control-flow smells in this checkout.

This document intentionally excludes scratch findings that no longer hold in the current worktree. Examples verified as fixed or narrowed during this pass: prompt-submit hooks now run in `InteractiveRuntime.RunInput`; TUI model/resume picker selections route back through slash/runtime commands; in-process resume rehydrates provider, cost, metrics, permissions, content replacement, and file-state tracking; web cancellation has a real `/api/cancel` route; provider accounting is wrapped at the provider boundary; REPL nested execution goes through the tool orchestrator; forked skills delegate to the Agent runner; scoped registries preserve schemas; and active orchestration entrypoints pass run-scoped artifact roots.

## Deduplication Review

These findings are intentionally grouped by boundary theme, but each retained item must have its own concrete owner split, source surface, observable failure, and fix boundary. Related findings should not be merged merely because they share a larger architectural root. For example, session persistence has several independent failures: event-consumer save triggers, lossy manual rewrite, void save callbacks, stale writer capture, UI close ordering, sidecar artifact lifecycle, `/clear` cache carryover, stale resume timing, and role-derived metadata are different bugs with different fixing points.

The same distinction applies to tool and subagent artifacts. "Tool result blobs live outside the session store lifecycle" is the general session-store sidecar lifecycle bug; "subagent tool result artifacts are written under unsaved forked session IDs" is the child-run ownership bug for the same artifact type. They are related evidence for a weak artifact boundary, but they fail in different code paths and require different minimal fixes.

## 1. Session Persistence Is Triggered By UI/Event Consumers Instead Of Conversation Mutation

Severity: high

Concrete files/functions involved:

- `internal/query/loop.go`: `Engine.Run`, `Engine.runLoop`
- `internal/query/persistence.go`: `ShouldPersistSessionEvent`
- `internal/cli/run.go`: `InteractiveRuntime.runEngine`, `runNonInteractive`, `persistSessionAfterLoopEvent`, `makeSessionSaveClose`

What responsibility is split or misplaced:

The query engine owns conversation mutation through `appendConversationMessage`, but CLI/interactive callers decide when to persist by filtering emitted `LoopEvent` values through `ShouldPersistSessionEvent`. That persistence policy lives outside the mutation owner.

Why this is wrong in ownership/lifecycle terms:

A conversation append is the domain event that justifies persistence. Instead, persistence depends on whether later runtime progress emits one of a hand-maintained list of events, plus an extra `ErrorEvent` check in `persistSessionAfterLoopEvent`. The mutation owner still does not declare the persistence transaction; event consumers infer it from presentation/runtime progress events.

Observable bug or likely failure mode:

New event types or slash/runtime paths that mutate conversation, handoff, todo, or file-state data will be lost unless every caller updates its event-consumer filter or adds a separate save call. Persistence behavior is therefore controlled by streaming consumption paths instead of the code that actually changed the durable session state.

Minimal direction for fixing the boundary:

Move save/checkpoint ownership to the runtime/session boundary that applies conversation mutations. Treat accepted messages, internal hook messages, handoff/todo/file-state mutations, and terminal errors as explicit persistence transactions from the owner of the mutation.

What not to do:

Do not just add more cases to `ShouldPersistSessionEvent` or more terminal-event checks in each UI. That keeps persistence coupled to event consumption instead of mutation ownership.

Status:

Resolved in current worktree. The invariant owner is now the query/runtime mutation boundary plus the session save transaction bound into it by interactive and non-interactive runtime setup. UI and CLI event consumers no longer infer persistence from `LoopEvent` types.

Source evidence:

- `internal/query/engine.go`: `EngineConfig.SessionCheckpoint`, `SetSessionCheckpoint`, and `checkpointSession` bind an error-returning session checkpoint into the engine. `appendConversationMessage` returns the checkpoint error after applying the conversation mutation.
- `internal/query/loop.go`: the legacy query loop handles checkpoint errors from every `appendConversationMessage` call; auto-compaction checkpoints immediately after `compact.ApplyResult`; `executeHandoffPatch` and `executeCertifyFact` checkpoint after engine-owned handoff mutations and return errors to the loop.
- `internal/query/miniswe_loop.go`: the current Pragma loop handles checkpoint errors from user, assistant, and shell-observation message appends.
- `internal/cli/run.go`: `BuildInteractiveRuntime`, in-process resume, clear-session reset, and non-interactive setup call `engine.SetSessionCheckpoint(sessionSaveFn)` after the active session save transaction exists.
- Source deletion: `internal/query/persistence.go` was removed, so `query.ShouldPersistSessionEvent` no longer exists as a reusable event-consumer persistence policy. `rg -n "ShouldPersistSessionEvent|persistSessionAfterLoopEvent" internal cmd -g'*.go'` returns no matches.

Verification evidence:

- Non-test verification: `gofmt -w internal/query/engine.go internal/query/loop.go internal/query/miniswe_loop.go internal/query/loop_test.go internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/session ./internal/slash ./internal/web ./internal/tui ./internal/query`; `git diff --check`.

## 2. Manual `/compact` Rewrites Only Part Of The Durable Session State

Severity: high

Concrete files/functions involved:

- `internal/slash/commands.go`: `handleCompact`
- `internal/query/loop.go`: `Engine.autoCompactBeforeRequest`
- `internal/cli/run.go`: `InteractiveRuntime.runSlash`, `rewriteCurrentSession`, `makeSessionSaveClose`
- `internal/session/writer.go`: `Writer.Rewrite`
- `internal/session/store.go`: `Store.Load`

What responsibility is split or misplaced:

Auto-compaction and manual `/compact` both call `compact.ApplyResult`, but manual `/compact` returns `RewriteSession`, causing `InteractiveRuntime.runSlash` to call `rewriteCurrentSession`. That rewrite truncates the session file and writes only header, messages, and metadata, while normal session saves also persist handoff state, file-state records, todos, and content-replacement records.

Why this is wrong in ownership/lifecycle terms:

Conversation replacement is a domain mutation, but it is not the only durable session state. A rewrite that claims to replace the full session log must be owned by the session persistence layer and preserve every durable entry type that `Store.Load` understands.

Observable bug or likely failure mode:

After `/compact`, the JSONL file can lose persisted handoff state, todos, file-state freshness records, prompt history, and content-replacement records even though those were still valid session state. A later resume can load the compacted messages but miss file freshness information or replacement mappings needed to keep old tool outputs out of provider requests.

Minimal direction for fixing the boundary:

Make session rewrite accept and commit a complete session snapshot, including all durable entry categories, or route compaction through the same session persistence transaction that owns those categories.

What not to do:

Do not repair this by re-saving only file state after rewrite or by adding a second append after truncation. The rewrite operation itself must be complete if it owns truncation.

Status:

Resolved in current worktree. The invariant owner is now `session.Writer.Rewrite`, which accepts a `session.RewriteData` snapshot instead of a header/messages/metadata tuple. The rewrite API now owns every durable entry category that `session.Store.Load` can reconstruct: header, messages, handoff state, content replacements, prompt history, file-state records, todos, and metadata.

Source evidence:

- `internal/session/writer.go`: `RewriteData` declares the complete rewrite snapshot, and `Writer.Rewrite` writes the non-conversation session entry categories before metadata after truncating the file.
- `internal/cli/run.go`: `rewriteCurrentSession` loads the existing session before truncation to preserve `ContentReplacements` and `PromptHistory`, uses current runtime `HandoffState`, `Todos`, and engine-owned `FileStateRecords`, then calls `d.SessionWriter.Rewrite(session.RewriteData{...})`.
- `rg -n "\\.Rewrite\\(|func \\(w \\*Writer\\) Rewrite|RewriteData|loadCurrentSessionForRewrite" internal -g'*.go'` shows the production caller uses `session.RewriteData`; there is no remaining production call to a partial rewrite API.

Verification evidence:

- Non-test verification: `gofmt` has been run for `internal/session/writer.go`, `internal/session/writer_test.go`, `internal/cli/run.go`, and `internal/cli/deps.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/session`; `git diff --check`.

## 3. Worktree Tools Change Runtime CWD But Session Persistence Still Stores The Original Header WorkDir

Severity: high

Concrete files/functions involved:

- `internal/tools/worktree/enter.go`: `EnterTool.Invoke`
- `internal/tools/worktree/exit.go`: `ExitTool.Invoke`, `restoreSessionCWD`
- `internal/app/state.go`: `AppState.Worktree`, `AppState.CWD`
- `internal/cli/run.go`: `sessionMetadataForSnapshot`, `validateResumeWorkDir`
- `internal/session/store.go`: `Store.Load`, `Store.List`

What responsibility is split or misplaced:

The worktree tools claim to switch the session working directory and they mutate `AppState.CWD` and `AppState.Worktree`. Session persistence, however, stores `WorkDir` only in the immutable session header; metadata does not include current CWD or worktree state, and load/list return `header.WorkDir`.

Why this is wrong in ownership/lifecycle terms:

A "session working directory" transition belongs to the session/runtime owner. Here it is a tool-local state mutation with no durable session transition. Resume validation compares the active runtime CWD against the old header workdir, not the worktree CWD the session may have entered.

Observable bug or likely failure mode:

After `EnterWorktree`, tools can execute in the worktree for the current process, but a saved/resumed session still identifies its workdir as the original repo. A crash or restart loses `AppState.Worktree`, and `ExitWorktree` reports no active worktree session even though the filesystem worktree remains.

Minimal direction for fixing the boundary:

Make CWD/worktree transitions session events owned by the runtime/session layer, and persist enough state to resume or reject them deliberately.

What not to do:

Do not repair this with prompt text telling the model to remember the worktree path, or by scanning `.pragma/worktrees` on resume. The persisted session state is the owner.

Status:

Resolved in current worktree. The invariant owner is the mutable session metadata snapshot written by the session save transaction. `HeaderData.WorkDir` remains the session creation workdir, while `MetadataData.WorkDir` and `MetadataData.Worktree` now carry the current runtime workdir and typed worktree session state.

Source evidence:

- `internal/session/entry.go`: `MetadataData` now includes `WorkDir` and `Worktree *app.WorktreeSession`.
- `internal/cli/run.go`: `sessionMetadataForSnapshot` writes `snap.CWD` and a copy of `snap.Worktree` on every session checkpoint.
- `internal/session/store.go`: `Store.Load` sets `Conversation.WorkDir` from `meta.WorkDir` before falling back to the header, and returns a typed `Session.Worktree`. `Store.List` also reports `meta.WorkDir` before falling back to the header.
- `internal/cli/deps.go`: startup resume restores `AppState.CWD` from the loaded session workdir and restores the typed worktree state.
- `internal/cli/run.go`: interactive resume restores `AppState.CWD` and `AppState.Worktree`, and `validateResumeWorkDir` allows resuming a worktree session from the original repo only when the persisted worktree directory still exists; otherwise it rejects deliberately.

Verification evidence:

- Non-test verification: `gofmt -w internal/session/entry.go internal/session/session.go internal/session/store.go internal/cli/run.go internal/cli/deps.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/session ./internal/slash ./internal/web ./internal/tui ./internal/query ./internal/tools/worktree`; `git diff --check`.

## 4. Query Loop Events Carry Presentation-Only Display Data

Severity: medium

Concrete files/functions involved:

- `internal/tool/tool.go`: `InvokeResult.Display`
- `internal/tool/orchestrator.go`: `ExecuteResult.Displays`, `executeSingle`
- `internal/query/event.go`: `ToolResultEvent.Display`
- `internal/query/loop.go`: `ToolResultEvent{Display: ...}`
- `internal/tui/render/toolrender.go`
- `internal/web/web.go`: `normalizeLoopEvent`

What responsibility is split or misplaced:

Tool invocation returns domain/model-visible content plus an optional presentation display payload. The query runtime then exposes that presentation payload as part of `ToolResultEvent`, and both TUI and web render from it.

Why this is wrong in ownership/lifecycle terms:

The query event surface is a runtime contract. Carrying "TUI-only" or renderer-shaped content through it makes presentation details part of the engine output stream. Runtime consumers have to know whether `Content` or `Display` is authoritative for their use case.

Observable bug or likely failure mode:

A future non-UI consumer can accidentally persist, replay, or model-feed display-only diffs/exit markers. A tool display format change for TUI rendering can also become a web/event API change because web normalizes `ToolResultEvent.Display`.

Minimal direction for fixing the boundary:

Keep query events domain-first: tool result content, file effects, and stable metadata. Move renderer-specific display material into UI adapters or a separate presentation projection created after runtime events.

What not to do:

Do not add more renderer conditionals to `ToolResultEvent` or teach every consumer how to interpret display strings.

Status:

Resolved in current worktree. The query/runtime contract no longer has a display side channel. Tool invocation returns model-visible content plus supplements only, the orchestrator result API carries result parts, supplements, and file-effect receipts, and `ToolResultEvent` exposes only stable runtime data. TUI and web render from tool name, input, result content, and file effects after the query event boundary.

Source evidence:

- `internal/tool/tool.go`: `InvokeResult` no longer has a presentation `Display` field.
- `internal/tool/orchestrator.go`: `ExecuteResult` no longer carries per-result `Displays`, and `singleResult` no longer stores display payloads.
- `internal/query/event.go` and `internal/query/loop.go`: `ToolResultEvent` no longer includes or emits display data.
- `internal/web/web.go`: normalized web `tool_result` events no longer include `display`.
- `internal/tui/model.go`, `internal/tui/handlers.go`, and `internal/tui/render/*`: TUI segment/render data no longer accepts display payloads; renderers project from stable tool input/content.
- `internal/query/handoff_failures.go`: handoff failure classification derives shell status from model-visible result content instead of display metadata.

Verification evidence:

- Non-test verification: `gofmt -w internal/tool/tool.go internal/tool/orchestrator.go internal/query/event.go internal/query/loop.go internal/query/handoff_failures.go internal/query/loop_test.go internal/web/web.go internal/tui/model.go internal/tui/handlers.go internal/tui/render/grouprender.go internal/tui/render/toolrender.go internal/tui/render/lifecycle.go internal/tui/render/render_test.go internal/tui/e2e_test.go internal/tools/bash/bash.go internal/tools/filewrite/filewrite.go internal/tools/fileedit/fileedit.go internal/tools/applypatch/applypatch.go internal/tools/fileread/fileread.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/tool ./internal/tools/bash ./internal/tools/filewrite ./internal/tools/fileedit ./internal/tools/applypatch ./internal/tools/fileread ./internal/query ./internal/web ./internal/tui ./internal/tui/render ./internal/orchestration ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "InvokeResult\\{[^\\n}]*Display|ExecuteResult\\{[^\\n}]*Displays|\\.Displays|ToolResultEvent\\{[^\\n}]*Display|\\.Display\\b|Display\\s+string|Display:" internal/tool internal/tools internal/query internal/web internal/tui/render internal/tui/model.go internal/tui/handlers.go -g'*.go'` returned no matches.

## 5. Web Reconstructs Orchestration Workflow State From Event Fragments

Severity: medium

Concrete files/functions involved:

- `internal/web/web.go`: `server.start`, `server.updateWorkflow`, `workflowSnapshot`
- `internal/query/event.go`: orchestration `LoopEvent` types
- `internal/orchestration/runner.go`: `runEvents`

What responsibility is split or misplaced:

The web server owns a workflow snapshot and updates it by interpreting orchestration event fragments. Orchestration owns the FSM state, transitions, handoffs, completion, and run identity, but it does not emit a single typed projection that UI can render.

Why this is wrong in ownership/lifecycle terms:

The UI layer is rebuilding domain state from a lossy event stream. It has to infer current state, status, durations, handoffs, and completion from event order. That is orchestration runtime ownership leaking into presentation code.

Observable bug or likely failure mode:

If an event is missed, renamed, reordered, or extended, the web workflow view can show a stale current state or incomplete handoff list while the runtime proceeds correctly. Other interfaces will need to duplicate their own reconstruction if they want the same workflow view.

Minimal direction for fixing the boundary:

Have orchestration emit a stable run snapshot/projection event from the orchestration runtime, and let web render that projection.

What not to do:

Do not add more field aliases or event-order heuristics to `server.updateWorkflow`.

Status:

Resolved in current worktree. The orchestration runtime now owns the workflow projection. It emits a typed `query.OrchestrationSnapshotEvent` after orchestration state changes, and web only normalizes that event into the existing `workflow_snapshot` frontend contract.

Source evidence:

- `internal/query/event.go`: adds `OrchestrationSnapshotEvent` and typed snapshot records as sealed loop-event data.
- `internal/orchestration/projection.go`: owns snapshot mutation from orchestration events and returns cloned projections.
- `internal/orchestration/runner.go`: creates one projection per orchestration run and routes orchestration events through `emitOrchestration`, which emits both the raw event and the snapshot event.
- `internal/web/web.go`: no longer stores or reconstructs workflow state with `updateWorkflow`; it normalizes `OrchestrationSnapshotEvent` to `workflow_snapshot`.

Verification evidence:

- Non-test verification: `gofmt -w internal/query/event.go internal/orchestration/projection.go internal/orchestration/runner.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/query ./internal/orchestration ./internal/web ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "updateWorkflow|func \\(s \\*server\\).*Workflow|type workflow|workflowOn|firstNonEmpty|newWorkflowSnapshot|cloneWorkflowSnapshot|ensureWorkflow\\(" internal/web/web.go` returns no backend reconstruction matches.

## 6. Slash Command Completion Logic Is Duplicated Across Web And TUI

Severity: medium

Concrete files/functions involved:

- `internal/slash/orchestrate.go`: `parseOrchestrateArgs`, `ParseOrchestrateCompletionState`
- `internal/tui/input.go`: `slashCompletionItems`, `orchestrateCompletionItems`, `pathCompletionItems`
- `internal/web/web.go`: `completionItems`, `orchestrateCompletionItems`, `pathCompletionItems`

What responsibility is split or misplaced:

Slash command execution grammar lives in `internal/slash`, but web and TUI each implement command/path completion state machines and filesystem role handling. Some shared completion state exists in `slash.ParseOrchestrateCompletionState`, but each interface still owns the path completion rules, replacement construction, limits, and filtering behavior.

Why this is wrong in ownership/lifecycle terms:

Command grammar and argument role semantics belong with the command layer. UI should render completion candidates, not encode what `--persona-dir`, `--prompt`, and the orchestration YAML position mean.

Observable bug or likely failure mode:

A new `/orchestrate` flag or changed positional rule can execute correctly while one interface suggests invalid completions or stops suggesting required paths. Web and TUI can drift on replacement text and candidate filtering even while calling the same command handler.

Minimal direction for fixing the boundary:

Move slash completion candidate generation to the slash command layer, returning presentation-neutral candidate records. Keep only rendering, keyboard navigation, and visual limits in web/TUI.

What not to do:

Do not patch the two completion copies in parallel whenever a slash command changes.

Status:

Resolved in current worktree. Slash completion candidate generation is now owned by `internal/slash`. Web and TUI keep only presentation-local adaptation and menu limits.

Source evidence:

- `internal/slash/completion.go`: owns command, alias, orchestration flag, path role, path filtering, fuzzy scoring, replacement construction, and sorting for slash completions.
- `internal/tui/input.go`: `slashCompletionItems` now adapts `slash.CompletionItems` into the TUI-local presentation type.
- `internal/web/web.go`: `completionItems` now adapts `slash.CompletionItems` into the web JSON presentation type.

Verification evidence:

- Non-test verification: `gofmt -w internal/slash/completion.go internal/tui/input.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/slash ./internal/tui ./internal/web ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "orchestrateCompletionItems|pathCompletionItems|flagCompletionItems|fuzzyCompletionScore|isCompletionBoundary|replaceCurrentToken|firstSlashToken|pathCompletionKind|scoredCompletionItem|sortedCompletionItems|endsWithSpace" internal/tui/input.go internal/web/web.go` returned no matches.

## 7. Cron Job Creation And Cron Execution Are Separate Scheduler Instances With No Live Synchronization

Severity: high

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps` creates `CronSched` for normal CLI/web/TUI runtime
- `internal/tools/cron/create.go`: `CreateTool.Invoke`
- `internal/cron/scheduler.go`: `NewScheduler`, `Create`, `Start`, `tick`
- `internal/cli/cron.go`: `RunCronDaemon`

What responsibility is split or misplaced:

The model-facing cron tools mutate a scheduler instance attached to the current runtime. The daemon command creates a separate scheduler instance, loads durable jobs only at construction, and then ticks its in-memory job map. There is no owner that synchronizes live tool-created jobs into an already-running daemon.

Why this is wrong in ownership/lifecycle terms:

Scheduling is a runtime/daemon responsibility. Persisting a job is not the same as registering it with the executor that will fire it. The tool currently creates durable state, but the running scheduler may never observe that state.

Observable bug or likely failure mode:

If `pragma cron run` is already running and an interactive session calls `CronCreate`, the daemon's scheduler does not reload the store, so the new durable job will not fire until the daemon restarts. Non-durable jobs created in an ordinary interactive runtime are stored only in a scheduler that is never started.

Minimal direction for fixing the boundary:

Make one cron runtime owner handle creation, persistence, and live registration. The tool should call that owner or clearly be a durable-store editor for a daemon that watches/reloads changes.

What not to do:

Do not start a cron ticker inside every `SetupDeps` caller or inside `CronCreate`. That would make arbitrary CLI/web/TUI invocations become schedulers.

Status:

Resolved in current worktree. The invariant owner is the explicit cron daemon scheduler. Model-facing cron creation is now a durable daemon-store edit, and the daemon reloads durable jobs from that store before ticking instead of relying only on construction-time state.

Source evidence:

- `internal/cron/scheduler.go`: `Scheduler.ReloadDurable` reloads durable jobs from the store, merges them into the running scheduler, removes durable jobs deleted from the store, preserves non-durable in-process jobs, and advances the scheduler sequence from loaded IDs. `Scheduler.Start` calls `ReloadDurable` on each tick before firing due jobs and emits a cron warning event on reload failure.
- `internal/tools/cron/create.go`: `CronCreate` now defaults `durable` to true and rejects `durable=false` with an explicit error because non-durable jobs require a live in-process scheduler and are not supported by the daemon-backed tool contract.
- `internal/cli/cron.go`: `RunCronDaemon` remains the only path that calls `scheduler.Start`; no ticker was added to `SetupDeps` or `CronCreate`.

Verification evidence:

- Non-test verification: `gofmt -w internal/cron/scheduler.go internal/tools/cron/create.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cron ./internal/tools/cron ./internal/cli ./internal/session ./internal/slash ./internal/web ./internal/tui ./internal/query ./internal/tools/worktree`; `git diff --check`.

## 8. "Remember For Session" Permission UI Persists Local Permission Rules

Severity: medium

Concrete files/functions involved:

- `internal/tui/permission.go`: option label `Yes, for this session`
- `internal/web/web.go`: permission checkbox text `remember for session`
- `internal/permission/prompter.go`: `Prompter.Prompt`
- `internal/tool/orchestrator.go`: remember branch calls `AddPersistentRule`
- `internal/permission/rulechecker.go`: `AddPersistentRule`, `AddSessionRule`
- `internal/permission/persist.go`: `PersistRule`

What responsibility is split or misplaced:

The UI collects a "for this session" remember intent, but the orchestrator turns that boolean into a persistent local settings rule. The permission layer has both session rules and persisted rules, but the UI wording and runtime action disagree.

Why this is wrong in ownership/lifecycle terms:

Permission scope is a security boundary. A UI label that says session-scoped must map to a session-scoped enforcement action, or the permission owner must expose a distinct durable option.

Observable bug or likely failure mode:

A user who selects "Yes, for this session" in TUI, or checks "remember for session" in web, can create a rule in local settings that applies to future sessions.

Minimal direction for fixing the boundary:

Represent permission scope explicitly: allow once, allow for session, and allow persistently if desired. Have the permission owner map each scope to `AddSessionRule` or `AddPersistentRule`.

What not to do:

Do not fix this by changing only the UI text to "remember". The runtime still lacks an explicit scope contract.

Status:

Resolved in current worktree. The invariant owner is now the permission prompt/checker contract: UI adapters return an explicit `permission.RememberScope`, and the orchestrator maps scope to the correct permission rule owner. Session scope calls `AddSessionRule`; only persistent scope calls `AddPersistentRule`.

Source evidence:

- `internal/permission/permission.go`: declares `RememberScope` values for none, session, and persistent remember decisions.
- `internal/permission/prompter.go`: `Prompter.Prompt` returns `(Decision, RememberScope)`, so remember intent is no longer a boolean with implicit persistence semantics.
- `internal/tool/orchestrator.go`: maps `RememberSession` to `Checker.AddSessionRule` and reserves `Checker.AddPersistentRule` for `RememberPersistent`.
- `internal/tui/permission.go`: the TUI option labeled "Yes, for this session" returns `RememberSession`.
- `internal/web/web.go`: the browser permission response sends and parses explicit `scope` values; the legacy `remember` boolean is only a compatibility alias for `RememberSession`.

Verification evidence:

- Non-test verification: `gofmt -w internal/permission/permission.go internal/permission/prompter.go internal/tool/orchestrator.go internal/tui/messages.go internal/tui/permission.go internal/tui/prompter.go internal/tui/permission_test.go internal/tui/prompter_test.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/permission ./internal/tool ./internal/tui ./internal/web ./internal/cli`; `git diff --check`.
- Contract scan: `rg -n "Remember bool|resp\\.Remember|Prompt\\(ctx context.Context.*bool|\\(permission\\.Decision, bool\\)|remember for session.*AddPersistentRule|Remember:" internal -g'*.go'` returned no matches.

## 9. TUI Teams Dialog Directly Controls Task Lifecycle

Severity: medium

Concrete files/functions involved:

- `internal/tui/teams.go`: `teamsDialog.Show`, `shutdownSelected`, `killSelected`
- `internal/task/registry.go`: `RequestShutdown`, `Cancel`
- `internal/tools/taskstop/taskstop.go`: `TaskStop.Invoke`
- `internal/tools/teamdelete/teamdelete.go`: `activeTeammateNames`

What responsibility is split or misplaced:

The TUI dialog stores a `*task.Registry`, lists teammate tasks directly, and invokes shutdown/cancel mutations from key handlers. Tool-based task cancellation goes through `TaskStop`; team cleanup reads from the same registry through a separate tool.

Why this is wrong in ownership/lifecycle terms:

Task lifecycle transitions are runtime/domain commands. The TUI should ask for a shutdown/kill command and render the result, not directly mutate registry state from presentation code.

Observable bug or likely failure mode:

TUI-only teammate controls can diverge from tool/web behavior on authorization, events, messages, and future lifecycle states. Shutdown semantics become encoded in key handling instead of one task/team owner.

Minimal direction for fixing the boundary:

Expose one runtime/task command for teammate shutdown and cancellation, and have TUI, tools, and any web surface call it.

What not to do:

Do not copy `RequestShutdown`/`Cancel` calls into more UI surfaces.

Status:

Resolved in current worktree. The invariant owner is now the task registry lifecycle command boundary. TUI and tools request typed lifecycle commands and render the returned result; they no longer choose shutdown/kill implementation details directly.

Source evidence:

- `internal/task/registry.go`: adds `LifecycleCommand`, `LifecycleCommandResult`, and `Registry.ApplyLifecycleCommand`, which owns shutdown and kill transitions. Legacy `Cancel` and `RequestShutdown` delegate to this command boundary.
- `internal/tui/teams.go`: the teams dialog uses `ApplyLifecycleCommand` for shutdown and kill actions, and `ListTeammates` for teammate visibility.
- `internal/tui/model.go`: teammate toolbar refresh uses `ListTeammates`.
- `internal/tools/taskstop/taskstop.go`: `TaskStop` uses `ApplyLifecycleCommand(..., LifecycleCommandKill)` instead of directly cancelling the registry.
- `internal/tools/teamdelete/teamdelete.go`: active teammate lookup uses `ListTeammates`.

Verification evidence:

- Non-test verification: `gofmt -w internal/task/registry.go internal/tui/teams.go internal/tui/model.go internal/tools/taskstop/taskstop.go internal/tools/teamdelete/teamdelete.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/task ./internal/tui ./internal/tools/taskstop ./internal/tools/teamdelete ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "d\\.taskReg\\.(RequestShutdown|Cancel|ListAllTeammates|ListRunningTeammates)|t\\.Tasks\\.Cancel\\(|t\\.Tasks\\.ListRunningTeammates\\(|m\\.taskReg\\.ListRunningTeammates\\(" internal/tui internal/tools -g'*.go'` returned no matches.

## 10. Lifecycle Graph Entrypoints Each Build Runner Context And Project Results

Severity: medium

Concrete files/functions involved:

- `cmd/pragma/lifecycle.go`: `lifecycleRunCmd`
- `internal/tools/lifecycle/lifecycle.go`: `Tool.Invoke`
- `internal/query/engine.go`: `Engine.RunGraph`, `runGraph`
- `internal/lifecycle/bridge/runner.go`: `Runner`, `InitialState`, `ProjectResult`

What responsibility is split or misplaced:

There is a shared `bridge.Runner`, but CLI lifecycle, lifecycle tool execution, and `Engine.RunGraph` each build runner inputs, consume progress, and project user-visible results in their own entrypoint-specific way.

Why this is wrong in ownership/lifecycle terms:

Lifecycle graph execution is runtime behavior. Entry points should choose transport and permissions, not re-own graph initial state, progress event projection, final assistant text handling, and error behavior.

Observable bug or likely failure mode:

The same graph can be run via CLI, as a tool, or through `Engine.RunGraph` with different permission defaults, output handling, progress events, and final message/session integration. A bug fix in final result projection or progress emission can land in one path and miss the others.

Minimal direction for fixing the boundary:

Keep one lifecycle run adapter that builds runner config, streams progress, projects final result, and exposes entrypoint-neutral events/results. CLI/tool/query surfaces should adapt that result to their transport only.

What not to do:

Do not copy missing fields or extra print branches between the three entrypoints.

Status:

Resolved in current worktree for the duplicated runner context and progress/result projection boundary. The lifecycle bridge runner now owns runner config construction, initial state setup, progress projection, and final result projection. CLI, query, and the LifecycleRun tool supply their runtime dependencies and adapt the typed bridge projection to their transport surfaces instead of each rebuilding executor context and progress records independently.

Source evidence:

- `internal/lifecycle/bridge/runner.go`: `NewRunnerConfig` is the single constructor for lifecycle runner context, `RunnerConfig` fields are private to the bridge package, `Runner.InitialState` owns state construction, `RunEvent` includes `Progress bridge.ProgressEvent`, and `ProjectProgress` is the single projection from `lifecycle.ExecutionEvent` to transport-neutral lifecycle progress.
- `internal/query/engine.go`: `runGraph` converts `runEv.Progress` to `query.LifecycleProgressEvent` through a small query adapter instead of reading executor event fields.
- `internal/tools/lifecycle/lifecycle.go`: `LifecycleRun` converts `runEv.Progress` to `tool.ProgressEvent` through a tool adapter instead of rebuilding progress from executor events.
- `cmd/pragma/lifecycle.go`: the standalone CLI prints from `runEv.Progress` and uses `runEv.Result` for completion result/error handling.

Verification evidence:

- Non-test verification: `gofmt -w internal/lifecycle/bridge/runner.go internal/query/engine.go internal/tools/lifecycle/lifecycle.go cmd/pragma/lifecycle.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/lifecycle/bridge ./internal/query ./internal/tools/lifecycle ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "runEv\\.Event|ev := runEv\\.Event|ev\\.Err|ev\\.Step|ev\\.Node|ev\\.Nodes|ev\\.Type|ev\\.Duration|ev\\.FromNode|ev\\.ToNode|ev\\.RouteKey" cmd/pragma/lifecycle.go internal/query/engine.go internal/tools/lifecycle/lifecycle.go` returned no matches.
- Contract scan: `rg -n "bridge\\.RunnerConfig\\{|RunnerConfig\\{" cmd internal -g'*.go'` returned no external runner config literals.

## 11. Generated Lifecycle Graphs Get Runtime Reducer Defaults That YAML Graphs Do Not

Severity: medium

Concrete files/functions involved:

- `internal/lifecycle/bridge/resolve.go`: `GenerateAndResolveGraph`, `EnsureGeneratedReducers`, `ResolveGraph`
- `internal/lifecycle/bridge/llm_node.go`: LLM node updates `turn_count` and `total_usage`
- `internal/lifecycle/state.go`: default overwrite reducer
- `cmd/pragma/lifecycle.go`: `loadYAMLGraph`, `generateGraph`

What responsibility is split or misplaced:

Bridge LLM nodes emit `turn_count` and `total_usage`, and the bridge defines reducers that know how to sum them. Generated graphs receive default reducers for those keys through `EnsureGeneratedReducers`; YAML-loaded graphs go through `ParseFile` and `ResolveGraph` without that bridge defaulting step.

Why this is wrong in ownership/lifecycle terms:

Reducer semantics for bridge-owned state keys should belong to the bridge/runtime definition, not to the graph source. Whether a graph was generated from a prompt or loaded from YAML should not change how `total_usage` and `turn_count` accumulate.

Observable bug or likely failure mode:

A YAML graph with multiple LLM nodes can overwrite `total_usage` and `turn_count` with the last update unless the YAML author manually declares bridge-specific reducers. Generated graphs accumulate those values by default.

Minimal direction for fixing the boundary:

Apply bridge-owned reducer defaults inside the common bridge resolve path, while still allowing explicit YAML reducers to override them.

What not to do:

Do not add more prompt text telling generated graphs to include reducers while leaving YAML graphs responsible for bridge internals.

Status:

Resolved in current worktree. Bridge-owned reducer defaults are now applied in the common bridge graph resolution path, so generated and YAML-loaded bridge graphs get the same missing defaults for `total_usage` and `turn_count`. Explicit reducer declarations still win because defaults are only filled when the key is absent, and the generic definition resolver now resolves by reducer name instead of treating a matching state key in `CustomReducers` as an override.

Source evidence:

- `internal/lifecycle/bridge/resolve.go`: `ResolveGraph` calls `ApplyBridgeReducerDefaults` before `definition.Resolve`; `GenerateAndResolveGraph` no longer has generated-only reducer mutation.
- `internal/lifecycle/bridge/resolve.go`: `ApplyBridgeReducerDefaults` fills missing `KeyTotalUsage: total_usage` and `KeyTurnCount: sum` reducers without replacing explicit graph reducers.
- `internal/lifecycle/definition/resolve.go`: reducer resolution now uses the declared reducer name, allowing explicit YAML reducers such as `overwrite` to override bridge defaults.

Verification evidence:

- Non-test verification: `gofmt -w internal/lifecycle/bridge/resolve.go internal/lifecycle/definition/resolve.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/lifecycle/bridge ./internal/lifecycle/definition ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "EnsureGeneratedReducers|ApplyBridgeReducerDefaults|CustomReducers\\[key\\]" internal/lifecycle -g'*.go'` showed only the common bridge default application and no state-key custom reducer override.

## 12. Background Session Registry Stores Process Startup Records Before Session Start

Severity: medium

Concrete files/functions involved:

- `internal/cli/run.go`: `RunBackground`, `runNonInteractive`
- `internal/background/registry.go`: `NewRegistry`, `Register`, `UpdateSessionID`, `List`
- `internal/background/subscriber.go`: `StatusSubscriber.HandleEvent`
- `internal/background/info.go`: `ProcessInfo`
- `cmd/pragma/sessions.go`: `sessionsCmd`, `sessionsListRun`, `sessionsKillCommand`, `sessionsLogsCommand`

What responsibility is split or misplaced:

The background parent creates the log file, starts the child process, and immediately writes a record under `~/.pragma/active-sessions/{pid}.json` with `SessionID: ""`, `Status: starting`, prompt text, model, provider, PID, and log path. The child process later subscribes a `StatusSubscriber` inside `runNonInteractive`; only after `beginSessionLifecycle` emits `SessionStarted` does the subscriber call `Registry.UpdateSessionID`. The CLI command that manages this directory is `pragma sessions`, with subcommands described as listing, killing, and logging "background sessions", even though the first durable record is only a process-startup record.

Why this is wrong in ownership/lifecycle terms:

Session identity belongs to the child runtime once a domain session has actually started. The parent can know the child PID and log file, but it does not own the session lifecycle. Writing that process record into an `active-sessions` registry and surfacing it through session-management commands makes process startup state masquerade as session state until the child catches up.

Observable bug or likely failure mode:

Immediately after `pragma --bg --prompt ...`, `pragma sessions` can list a row for a "background session" that has no session ID because the parent wrote `SessionID: ""`. During slow setup, MCP startup, provider initialization, or any failure before `SessionStarted`, the user can kill or tail logs for an entry that is not yet a session. If the child exits before registering its own cleanup defer, the stale process record is cleaned only when a later `Registry.List` notices the PID is dead.

Minimal direction for fixing the boundary:

Split process-run tracking from session tracking, or make the registry explicitly process-owned until `SessionStarted` attaches a session ID. The parent can print the PID/log path as a launched process; `pragma sessions` should either hide records without a session ID or label them as startup processes, not active sessions.

What not to do:

Do not fake a session ID from the prompt, PID, or log file, and do not just add a "starting" label while still treating the record as a session. The ownership boundary is whether the record represents a process or a domain session.

Status:

Resolved in current worktree. The parent still records the launched child as a process-owned PID record, but the background registry now exposes domain-session listing separately from process listing. `pragma sessions list` uses only records that have attached a real runtime `SessionID`, while PID-scoped operations such as logs and kill remain process operations.

Source evidence:

- `internal/background/info.go`: `ProcessInfo` is documented as background process metadata, with `SessionID` attached only after the child runtime starts a domain session; `HasSession` makes that distinction explicit.
- `internal/background/registry.go`: `ListProcesses` owns PID/process records and stale-process cleanup; `ListSessions` filters to process records with a real session ID.
- `cmd/pragma/sessions.go`: `sessionsListRun` calls `ListSessions`, so startup-only records are not presented as active background sessions; `kill` and `logs` are labeled as PID process operations.
- `internal/cli/run.go`: background launch output now reports a background process and says `pragma sessions` is useful after the runtime session starts.

Verification evidence:

- Non-test verification: `gofmt -w internal/background/info.go internal/background/registry.go internal/background/kill_unix.go internal/background/kill_windows.go cmd/pragma/sessions.go internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/background ./internal/cli ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "reg\\.List\\(|List\\(" cmd/pragma/sessions.go internal/background -g'*.go'` returned no stale background registry `List` callers.
- Contract scan: `rg -n "background session started|Killed background session|Show logs for a background session|Status represents.*session|running background session" internal/background cmd/pragma/sessions.go internal/cli/run.go -g'*.go'` returned no stale process-as-session wording.

## 13. Session Save And Close Swallow Persistence Errors Behind Void Callbacks

Severity: high

Concrete files/functions involved:

- `internal/session/writer.go`: `Writer.writeEntry`, `WriteMessage`, `WriteHandoffState`, `WriteFileState`, `WriteTodos`, `WriteMetadata`, `Close`
- `internal/cli/run.go`: `makeSessionSaveClose`, `persistSessionAfterLoopEvent`, `InteractiveRuntime.RunInput`, `runNonInteractive`

What responsibility is split or misplaced:

`session.Writer` correctly returns errors from encode, sync, append, rewrite, and close operations. `makeSessionSaveClose` converts that error-returning API into `saveFn func()` and `closeFn func()`, then silently returns on the first write or close error.

Why this is wrong in ownership/lifecycle terms:

Session persistence is a durability boundary. The runtime that owns the session lifecycle must know whether a checkpoint was committed, failed, or only partially written. A void callback makes persistence best-effort side work and prevents the caller from reporting, retrying, aborting, or marking the session dirty.

Observable bug or likely failure mode:

Disk full, permission changes, closed writers, JSON encode failures, or sync failures can leave `Conversation.Messages`, handoff state, todos, or file-state records advanced in memory while the session file remains stale or partially updated. The UI can continue as if the turn completed normally, `SessionSaved` may simply not appear, and resume later loads an older state with no direct error shown to the user.

Minimal direction for fixing the boundary:

Keep the writer errors in the runtime contract: make session save/close return errors, propagate them through the interactive/non-interactive event path, and decide at the session owner whether to retry, show a fatal persistence error, or mark the session as unsaved.

What not to do:

Do not solve this with stderr logging inside `makeSessionSaveClose` while keeping `func()` callbacks. Logs are not a durability contract and cannot stop callers from presenting stale state as saved.

Status:

Resolved in current worktree. The invariant owner is the session runtime boundary around `session.Writer`: save and close callbacks now return `error`, and callers that present runtime success must either propagate or surface persistence failure.

Source evidence:

- `internal/cli/run.go`: `makeSessionSaveClose` now returns `(func() error, func() error)` and returns errors from `WriteMessage`, `WriteHandoffState`, `WriteFileState`, `WriteTodos`, `WriteMetadata`, and `Close` instead of silently returning.
- `internal/cli/run.go`: `persistSessionAfterLoopEvent` now returns `error`; `runNonInteractive`, `InteractiveRuntime.runEngine`, and `InteractiveRuntime.runOrchestration` stop and report a `query.ErrorEvent` or return the error when persistence fails.
- `internal/slash/command.go` and `internal/slash/commands.go`: slash `SessionSave` is now `func() error`, and `/model` plus `/exit` return persistence errors instead of reporting successful state changes after a failed save.
- `internal/web/web.go` and `internal/tui/model.go`: `CloseSession` is now `func() error`; web returns close errors during shutdown and TUI quit renders the error instead of silently exiting.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go internal/slash/command.go internal/slash/commands.go internal/slash/commands_test.go internal/web/web.go internal/tui/model.go internal/tui/handlers.go internal/session/writer.go internal/session/writer_test.go internal/cli/deps.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/session ./internal/slash ./internal/web ./internal/tui`; `git diff --check`.

## 14. `--record` Creates And Truncates A Fixed Replay File During Dependency Setup

Severity: medium

Concrete files/functions involved:

- `internal/cli/flags.go`: `RegisterFlags`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/observe/recorder.go`: `NewRecorder`
- `cmd/pragma/replay.go`: `replayRun`, `replayLiveFromCheckpoint`
- `internal/observe/replay.go`: `LoadReplay`

What responsibility is split or misplaced:

The global `--record` flag is handled in `SetupDeps`, which immediately calls `observe.NewRecorder("pragma-recording.jsonl")`. `NewRecorder` uses `os.Create`, so the cwd-relative recording is created or truncated before a prompt, session, replay run ID, or first recordable domain event exists.

Why this is wrong in ownership/lifecycle terms:

Replay recordings are execution artifacts. Their path and lifecycle should be owned by the run/session recording boundary, not by generic dependency construction. Dependency setup is shared by interactive runs, non-interactive runs, and local slash subcommands; it is too early and too broad to own a durable replay artifact.

Observable bug or likely failure mode:

Starting any command path that reaches `SetupDeps` with recording enabled can overwrite the previous `pragma-recording.jsonl` before the new run has proven it will execute. Because the path is fixed and cwd-relative, two attempts in the same workspace collide, and failed startup can leave an empty or partial recording in place of the useful previous one.

Minimal direction for fixing the boundary:

Move recording creation to a run/session-owned recorder factory with an explicit output path or unique run directory. Create the file when the run starts producing recordable events, and surface the path as part of the run artifact metadata.

What not to do:

Do not add cleanup that deletes empty `pragma-recording.jsonl` after failure. The core bug is that setup owns and truncates a run artifact at a fixed path.

Status:

Resolved in current worktree. `SetupDeps` no longer creates or truncates a fixed recording file. Recording starts from the session lifecycle boundary, immediately before `SessionStarted`, and writes to a unique per-session recording artifact under Pragma home.

Source evidence:

- `internal/cli/deps.go`: removed `observe.NewRecorder("pragma-recording.jsonl")` from dependency setup; `Deps` now tracks `RecordingPath` and the active recorder for cleanup.
- `internal/cli/run.go`: `beginSessionLifecycle` starts recording through `startSessionRecording` before emitting `SessionStarted`, and now returns recorder creation errors to callers.
- `internal/cli/run.go`: `sessionRecordingPath` creates `~/.pragma/recordings/<session-id>/<utc-run-timestamp>.jsonl`, avoiding cwd-relative fixed-path truncation and collisions.
- `internal/cli/run.go`: recording path is printed to stderr when recording starts, so the run-owned artifact location is visible to the caller.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/deps.go internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/observe ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "NewRecorder\\(|pragma-recording\\.jsonl" internal/cli internal/observe cmd -g'*.go'` shows runtime recorder creation only in `startSessionRecording`; fixed filename references remain only in examples/tests.
- Contract scan: `rg -n "beginSessionLifecycle\\(" internal/cli -g'*.go'` confirms all lifecycle call sites handle the new error-returning contract.

## 15. Local Slash Subcommands Start Full Runtime Dependencies Before Running Local Logic

Severity: medium

Concrete files/functions involved:

- `internal/slash/command.go`: `CommandType`, `TypeLocal`
- `internal/slash/commands.go`: local CLI commands `cost`, `model`, `advisor`, `doctor`, `config`, `skills`, `resume`, `mcp`
- `internal/cli/subcommands.go`: `RunLocalCommand`
- `internal/cli/deps.go`: `SetupDeps`

What responsibility is split or misplaced:

`TypeLocal` commands are documented as commands that need no engine, but CLI execution first calls full `SetupDeps`. Only if full setup fails does it fall back to lightweight config-only dependencies. Full setup builds providers, token/cost monitors, task registries, permission policy, hooks, log files, cron scheduler state, MCP manager state, MCP connection goroutines, and the MCP watchdog.

Why this is wrong in ownership/lifecycle terms:

Local subcommands should own only local command dependencies. Provider, tool, MCP, and runtime session infrastructure belongs to prompt execution. `RunLocalCommand` currently makes runtime startup the default path for commands whose type says they are local.

Observable bug or likely failure mode:

Running `pragma doctor`, `pragma config`, `pragma skills`, or `pragma resume` can create per-execution logs, start MCP connection work, start the watchdog goroutine, and create or truncate the fixed recording file when `record` is enabled, even though the command only needs to inspect config or session metadata. The fallback protects only provider setup failure; it does not prevent side effects when full setup succeeds.

Minimal direction for fixing the boundary:

Build local slash dependencies from a local-command dependency constructor first. Add optional capability hooks for commands that genuinely need runtime status, such as MCP status, without forcing all local commands through full prompt-runtime setup.

What not to do:

Do not patch individual commands like `doctor` or `config` to undo setup side effects. The wrong owner is `RunLocalCommand`, not the command handlers.

Status:

Resolved in current worktree. Local slash subcommands now use a local-command dependency constructor directly; they no longer attempt full prompt-runtime dependency setup before running local logic.

Source evidence:

- `internal/cli/subcommands.go`: `RunLocalCommand` calls `BuildLocalSlashDeps` and no longer calls `SetupDeps`, defers runtime cleanup, subscribes runtime loggers, or reaches provider/MCP runtime construction.
- `internal/cli/subcommands.go`: `BuildLocalSlashDeps` builds only local command dependencies: cwd/config flag state, a lightweight state store, zero cost tracker, session store, skill loader, and static MCP status capability.
- `internal/cli/subcommands.go`: `localMcpStatuses` reads merged MCP config and reports configured servers without creating an MCP manager, connecting clients, or starting the watchdog.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/subcommands.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/slash ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "RunLocalCommand|BuildLocalSlashDeps|SetupDeps|Cleanup|McpManager|NewRecorder|NewMCPWatchdog|ConnectAllAndRegister|task\\.NewRegistry|CreateProvider" internal/cli/subcommands.go` showed only the local command entrypoint and local dependency constructor, with no runtime setup calls.

## 16. Background And Teammate Agents Are Cancelled By Task Records, Not By Runtime Lifecycle

Severity: medium

Concrete files/functions involved:

- `internal/tools/agent/agent.go`: `runBackground`, `runTeammate`
- `internal/task/registry.go`: `Registry.Create`, `Cancel`, `RequestShutdown`, `Shutdown`, `List`
- `internal/cli/deps.go`: `SetupDeps`, `compositeCleanup`
- `internal/cli/run.go`: `InteractiveRuntime.Cleanup`, `runNonInteractive`

What responsibility is split or misplaced:

Background and teammate agents create child contexts with `context.WithCancel(context.Background())` and store the cancel function on the task record. The task registry supports per-task cancel and teammate shutdown, but `Deps.Cleanup` cancels MCP and drains the event bus without cancelling or shutting down running tasks.

Why this is wrong in ownership/lifecycle terms:

Agent goroutines execute runtime work: they stream provider calls, run tools, mutate task state, and emit events. Their lifetime should be rooted in the runtime/session lifecycle with task-level cancellation as a control operation inside that lifecycle. Here the task record becomes the only owner of cancellation, while session/runtime cleanup has no visible task-drain boundary.

Observable bug or likely failure mode:

Closing an interactive runtime or ending a non-interactive command can disconnect MCP and drain the event bus while background or teammate goroutines still hold contexts derived from `context.Background()`. Those goroutines can continue until their own task cancel path fires, and late events may be emitted after the normal runtime cleanup path has already drained subscribers.

Minimal direction for fixing the boundary:

Give the runtime a root task context or task supervisor. Derive background and teammate agent contexts from it, and have `Deps.Cleanup` request graceful task shutdown and wait or force-cancel before disconnecting shared infrastructure and draining the bus.

What not to do:

Do not make every background agent use the foreground request context directly; background work can outlive one tool call. The missing owner is the runtime/session supervisor, not the individual request context.

Status:

Resolved in current worktree. Runtime dependencies now own a root task context and cleanup drains active tasks before shared MCP/event-bus infrastructure is torn down. Background and teammate agents still outlive a single foreground tool call, but their child contexts are rooted in the runtime lifecycle instead of `context.Background()`.

Source evidence:

- `internal/cli/deps.go`: `SetupDeps` creates `TaskContext` with a runtime-owned cancel function and `Deps.Cleanup` calls `TaskReg.ShutdownActive(500 * time.Millisecond)` before MCP disconnect and event-bus drain.
- `internal/cli/tools.go`: the Agent tool receives `d.TaskContext` when registered.
- `internal/tools/agent/agent.go`: `runBackground` and `runTeammate` derive child contexts from `t.runtimeTaskContext()` instead of `context.Background()`.
- `internal/task/registry.go`: `ShutdownActive` requests graceful shutdown for all pending/running tasks, waits up to the cleanup deadline, then force-cancels remaining active tasks through the existing registry lifecycle command boundary.

Verification evidence:

- Non-test verification: `gofmt -w internal/task/registry.go internal/cli/deps.go internal/cli/tools.go internal/tools/agent/agent.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/task ./internal/cli ./internal/tools/agent ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "context\\.WithCancel\\(context\\.Background\\(\\)\\)|TaskContext|ShutdownActive\\(" internal/tools/agent internal/task internal/cli -g'*.go'` showed production agent child contexts derive from `TaskContext`; the remaining direct background contexts are task unit-test setup only.

## 17. Request-Time Tool Result Replacement Captures A Stale Session Writer

Severity: high

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`, `engineCfg.RecordContentReplacements`
- `internal/cli/run.go`: `startSessionForCurrentConversation`, `makeSessionSaveClose`
- `internal/query/loop.go`: `Engine.applyToolResultBudget`, request preparation in `Engine.runLoop`
- `internal/toolresult/storage.go`: `ApplyToolResultBudget`, `ReconstructContentReplacementState`
- `internal/session/writer.go`: `WriteContentReplacement`

What responsibility is split or misplaced:

`SetupDeps` wires `EngineConfig.RecordContentReplacements` as a closure over the local `sessionWriter` variable. Fresh sessions start with that local set to nil, and `startSessionForCurrentConversation` later assigns the real writer only to `d.SessionWriter`. The engine therefore keeps a request-time persistence callback that is disconnected from the active session writer.

Why this is wrong in ownership/lifecycle terms:

Tool-result budgeting is a persistence transaction: it creates replacement records that explain how old tool-result content was elided for future provider requests. The callback plumbing makes that transaction depend on setup-time local state instead of the session writer owner that is created when the session actually starts.

Observable bug or likely failure mode:

In a fresh session, `ApplyToolResultBudget` can persist full output files and return content-replacement records, but the callback sees nil and writes no JSONL `content_replacement` entries. Resume then reconstructs replacement state from messages without those records, so old large tool results can be sent to the provider again and persisted-output files can become unreferenced artifacts.

Minimal direction for fixing the boundary:

Make content-replacement recording owned by the live session persistence boundary, for example by routing the engine callback through `Deps.SessionWriter` or a session writer service that updates when the session starts and resumes.

What not to do:

Do not add a nil guard or retry inside `ApplyToolResultBudget`. The bad split is that the engine owns a stale persistence callback instead of the current session persistence owner.

Status:

Resolved in current worktree. The invariant owner is the active session persistence boundary, represented by `Deps.SessionWriter`. `SetupDeps` now wires `EngineConfig.RecordContentReplacements` through `contentReplacementRecorder`, which resolves the current `*Deps` and dereferences `d.SessionWriter` at record time. Fresh session creation in `startSessionForCurrentConversation` and interactive resume in `InteractiveRuntime.Resume` both update `d.SessionWriter`, so request-time replacement records now follow the live session writer instead of the setup-time local `sessionWriter`.

Verification evidence:

- Source: `internal/cli/deps.go` no longer captures the setup-time `sessionWriter` local in `engineCfg.RecordContentReplacements`; the callback is installed as `contentReplacementRecorder(func() *Deps { return deps })`.
- Source: `contentReplacementRecorder` writes through `d.SessionWriter.WriteContentReplacement(records)`, so the same persistence owner used by `startSessionForCurrentConversation`, `makeSessionSaveClose`, and `InteractiveRuntime.Resume` owns the content-replacement record write.
- Non-test verification: `gofmt -w internal/cli/deps.go`; `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/session`; `git diff --check`.

## 18. MCP Watchdog Lifetime Is Not Owned By Dependency Cleanup

Severity: medium

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`, `compositeCleanup`
- `internal/observe/watchdog.go`: `MCPWatchdog.Start`, `MCPWatchdog.check`
- `internal/observe/bus.go`: `EventBus.Emit`, `EventBus.Drain`
- `internal/mcp/manager.go`: `Manager.ServerStatus`, `DisconnectAll`

What responsibility is split or misplaced:

`SetupDeps` starts the MCP watchdog with `go watchdog.Start(cmd.Context())`, while `compositeCleanup` cancels only the MCP connection context, disconnects servers, drains the event bus, and closes cleanup files. The watchdog lifetime is rooted in the command context instead of the dependency lifecycle that created its bus and MCP manager.

Why this is wrong in ownership/lifecycle terms:

The watchdog polls MCP state and emits health events into the same bus that `Deps.Cleanup` drains. A goroutine that observes and reports dependency state must stop before that dependency graph is torn down. Cleanup currently tears down the observed state and event bus without owning the watchdog's cancellation.

Observable bug or likely failure mode:

After cleanup drains the event bus, the watchdog can continue ticking until `cmd.Context()` is cancelled. Later `MCPHealthCheck` or `MCPServerDisconnected` events are silently dropped because `EventBus.Emit` returns when the bus is closed. In long-lived command contexts, this also leaves a goroutine polling a manager after `DisconnectAll`.

Minimal direction for fixing the boundary:

Create a dependency-lifecycle context for the watchdog, cancel it in `compositeCleanup`, and wait for it alongside MCP connection goroutines before `DisconnectAll` and `bus.Drain`.

What not to do:

Do not add an `EventBus` closed check inside the watchdog as the fix. That hides the symptom while leaving the dependency cleanup path without ownership of a goroutine it started.

Status:

Resolved in current worktree. MCP connection work and the MCP watchdog now share a dependency-lifecycle context and wait group owned by `SetupDeps` cleanup. Cleanup cancels and waits for those goroutines before disconnecting MCP servers and draining the event bus.

Source evidence:

- `internal/cli/deps.go`: `SetupDeps` creates `depsCtx`/`depsCancel` and `depsWG` for dependency-owned goroutines.
- `internal/cli/deps.go`: MCP `ConnectAllAndRegister` and `watchdog.Start` both run under `depsCtx` and register with `depsWG`.
- `internal/cli/deps.go`: `compositeCleanup` cancels `depsCtx`, waits for `depsWG` with a bounded timeout, then calls `DisconnectAll` and `bus.Drain`.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/deps.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/observe ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "watchdog\\.Start\\(cmd\\.Context\\(\\)|mcpCancel|mcpWG|depsCtx|depsCancel|depsWG|watchdog\\.Start" internal/cli/deps.go` showed no command-context watchdog startup and no old MCP-only cleanup context.

## 19. Config Tool Maintains A Separate Settings Contract From Runtime Config

Severity: medium

Concrete files/functions involved:

- `internal/tools/config/settings.go`: `SupportedSettings`, `SettingDef`
- `internal/tools/config/config.go`: `Tool.handleSet`, `updateSettingsFile`, `applyLiveValue`
- `internal/config/config.go`: `Config`, `Load`, `merge`
- `internal/cli/tools.go`: `validateLiveConfigValue`, `applyLiveConfigValue`
- `internal/cli/run.go`: `BuildCompactionDeps`

What responsibility is split or misplaced:

The model-facing `Config` tool owns its own settings table and writes dotted keys directly to JSON settings files. That table includes settings such as `theme` and `autoCompactEnabled` that are not fields on `internal/config.Config`; live runtime application only handles `model`, and auto-compaction is actually controlled by the `DISABLE_AUTO_COMPACT` environment variable in `BuildCompactionDeps`.

Why this is wrong in ownership/lifecycle terms:

The runtime config package is the owner of supported settings and their merge/apply semantics. A tool can request a config change, but it should not define a parallel config schema or write keys that the runtime never reads. Otherwise the model-facing tool reports successful configuration changes that do not change runtime behavior.

Observable bug or likely failure mode:

The model can call `Config` to set `autoCompactEnabled` or `theme` and receive "Set ..." success, leaving keys in settings files that `config.Load` ignores. The user sees a durable config change, but future sessions do not apply it; for auto-compaction, the only checked switch remains the environment variable.

Minimal direction for fixing the boundary:

Make the tool derive writable settings from the runtime config contract or route writes through config-owned setters that know whether a key is persisted, live-applied, ignored, or unsupported.

What not to do:

Do not add more tool-local entries to `SupportedSettings` or special-case ignored keys in the UI. The tool-local schema is the boundary leak.

Status:

Resolved in current worktree. Supported writable settings are now defined by the runtime config package, and the model-facing Config tool reads that config-owned catalog instead of maintaining a separate schema with ignored keys.

Source evidence:

- `internal/config/settings.go`: added the config-owned `SettingDef`, `SupportedSettings`, `FindSetting`, and `SettingNames` catalog for real `internal/config.Config` fields.
- `internal/tools/config/config.go`: `Tool.Invoke`, description generation, get/set handling, and live sync use `goconfig` setting definitions directly.
- `internal/tools/config/settings.go`: the tool package now only re-exports the config-owned setting catalog for package compatibility; it no longer defines tool-local settings.
- `internal/tools/config/config.go`: removed user-facing examples and input descriptions for unsupported `theme`; `autoCompactEnabled` and `theme` are absent from the production supported settings catalog.

Verification evidence:

- Non-test verification: `gofmt -w internal/config/settings.go internal/tools/config/config.go internal/tools/config/settings.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/config ./internal/tools/config ./internal/cli ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "autoCompactEnabled|theme|var SupportedSettings|goconfig\\.SupportedSettings|syncToAppState|FindSetting\\(|SettingNames\\(" internal/config internal/tools/config internal/cli -g'*.go' -g'!*_test.go'` showed no production references to unsupported `autoCompactEnabled` or `theme`, and showed the settings catalog owned by `internal/config`.

## 20. Teammate Messages Are Drained By Both Agent Loop And Query Engine

Severity: medium

Concrete files/functions involved:

- `internal/tools/sendmsg/sendmsg.go`: `Tool.Invoke`
- `internal/task/registry.go`: `DeliverMessage`, `DrainPendingMessages`
- `internal/tools/agent/agent.go`: `Tool.Invoke`, `runTeammate`
- `internal/query/engine.go`: `SetTaskRegistry`, `SetTaskID`
- `internal/query/loop.go`: `Engine.runLoop`

What responsibility is split or misplaced:

`SendMessage` delivers messages into `Task.PendingMessages`. Teammate agents then have two consumers for that queue: `runTeammate` drains pending messages after a notify and passes the joined text as a new `engine.Run` prompt, while `Engine.runLoop` also drains pending messages whenever `TaskID` is set and injects them into the conversation as "Messages from teammates".

Why this is wrong in ownership/lifecycle terms:

Message delivery to a persistent teammate is a task/agent lifecycle operation. The queue should have one owner that decides when a pending message becomes a user-visible turn. Splitting the drain across the outer Agent loop and the inner query engine makes turn construction depend on timing between notify delivery and engine startup.

Observable bug or likely failure mode:

Messages that exist before the notify is handled are consumed by `runTeammate` and become the direct prompt. Messages that arrive after that drain but before or during `Engine.runLoop` can be consumed by the engine and inserted under a different prefix in the same run. That creates inconsistent teammate conversation shape and makes progress/idle state reflect only the outer drain path.

Minimal direction for fixing the boundary:

Choose one queue consumer. Either let the teammate loop own message batching and pass an explicit prompt to the engine, or let the engine own task-message injection and have the teammate loop only wake it. Keep heartbeat separate from message-drain ownership if needed.

What not to do:

Do not add more timing guards around `DrainPendingMessages` or rely on the notify channel being coalesced. The problem is two layers consuming the same task queue.

Status:

Resolved in current worktree. The teammate agent loop is now the single production consumer of `Task.PendingMessages`; the query engine keeps task heartbeat/reaping responsibilities but no longer drains teammate message queues or injects its own teammate-message prompt.

Source evidence:

- `internal/tools/agent/agent.go`: `runTeammate` remains the only production call site for `DrainPendingMessages`, batches pending messages after notify, and passes the joined text as the explicit next engine prompt.
- `internal/query/loop.go`: removed the engine-side `DrainPendingMessages` block and `"Messages from teammates:"` injection from `Engine.runLoop`.
- `internal/query/engine.go`: `TaskID`/`SetTaskRegistry` comments now describe heartbeat/reaping ownership instead of pending-message drain ownership.

Verification evidence:

- Non-test verification: `gofmt -w internal/query/loop.go internal/query/engine.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/query ./internal/tools/agent ./internal/task ./internal/tools/sendmsg ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "DrainPendingMessages|Messages from teammates|PendingMessages drain|task heartbeat|SetTaskRegistry|SetTaskID" internal/tools/agent internal/query internal/task internal/tools/sendmsg -g'*.go' -g'!*_test.go'` showed the only production queue drain is in `internal/tools/agent/agent.go`.

## 21. Bash Command Lifecycle Is Duplicated Between Bash Tool And Pragma Loop

Severity: medium

Concrete files/functions involved:

- `internal/query/loop.go`: `Engine.runLoop`
- `internal/query/miniswe_loop.go`: `Engine.runPragmaLoop`, `Engine.RunPragmaLoopWithSystem`, `runPragmaLoopBash`, `newPragmaLoopCommandFiles`, `writePragmaLoopCommandStatus`, `appendPragmaLoopRunningProcess`, `formatPragmaLoopObservation`
- `internal/orchestration/runner.go`: `RunStateEvents`
- `internal/tools/bash/bash.go`: `Tool.Invoke`, `invokeBackground`, `newCommandFiles`, `writeCommandStatus`, `runningCommandContent`, `commandForShellRun`

What responsibility is split or misplaced:

There are two independent owners for shell command execution and command artifacts. The Bash tool owns process creation, timeout handling, background/running behavior, status files, console logs, and process summaries for tool calls. The pragma-loop query path owns another shell runner with its own timeout constants, temp directory, status writer, running-process message, pipefail wrapper, and result formatter. This second path is on the default engine path because `Engine.runLoop` enters `runPragmaLoop` unless `PRAGMA_LEGACY_TOOL_LOOP=1`, and orchestration states call `RunPragmaLoopWithSystem` directly.

Why this is wrong in ownership/lifecycle terms:

Shell process lifecycle is a tool/runtime boundary. The query loop should decide what the model asked to do and advance the conversation, but it should not also implement process-group ownership, log/status artifact creation, timeout semantics, and poll instructions. Those concerns belong to a single command execution owner so process cleanup, background behavior, and user-visible command artifacts have one contract.

Observable bug or likely failure mode:

A fix to process-group cleanup, timeout semantics, status-file format, console-log paths, running-command instructions, or background-command handling can land in `internal/tools/bash/bash.go` and not in `internal/query/miniswe_loop.go`, or vice versa. Users and models then get different PID/status/console formats and different timeout behavior depending on whether the command was invoked through the Bash tool or parsed from pragma-loop text.

Minimal direction for fixing the boundary:

Route pragma-loop bash actions through the same command execution component used by the Bash tool, returning a structured command result that the query loop can wrap in pragma-loop observation XML. Keep pragma-loop parsing, response validation, and conversation advancement in `internal/query`, but move process lifecycle and command artifacts behind the shared command runner.

What not to do:

Do not copy the newest timeout/status/process-summary changes between the two files or normalize the prompt strings around the duplicated runners. That preserves two process lifecycle owners and guarantees future divergence.

Status:

Resolved in current worktree. Shell command process lifecycle and command artifacts are now owned by `internal/shellrun`. The Bash tool and pragma-loop query path both route shell execution through that shared runner; query keeps pragma-loop parsing and XML observation formatting, while the Bash tool keeps tool-facing input validation and output wording.

Source evidence:

- `internal/shellrun/run.go`, `internal/shellrun/proc_unix.go`, and `internal/shellrun/proc_windows.go`: `shellrun.Execute` owns bash process creation, process-group cancellation, timeout handling, foreground-running handoff, background execution, command/status/console artifact files, output reads, status writes, and process summaries.
- `internal/tools/bash/bash.go`: `Tool.Invoke` calls `shellrun.Execute` for foreground and background commands and no longer owns command files, status writes, process creation, or process-group setup.
- `internal/query/miniswe_loop.go`: `runPragmaLoopBash` calls `shellrun.Execute` with pragma-loop timeout, foreground-wait, pipefail, and artifact namespace options, then projects the structured result into pragma-loop observations.
- `internal/tools/bash/proc_unix.go` and `internal/tools/bash/proc_windows.go`: removed the Bash-local process lifecycle helpers.

Verification evidence:

- Non-test verification: `gofmt -w internal/shellrun/run.go internal/shellrun/proc_unix.go internal/shellrun/proc_windows.go internal/tools/bash/bash.go internal/query/miniswe_loop.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/shellrun ./internal/tools/bash ./internal/query ./cmd/pragma`; `git diff --check`.
- Contract scan: `rg -n "exec\\.CommandContext|SysProcAttr|Kill\\(-|newPragmaLoopCommandFiles|writePragmaLoopCommandStatus|appendPragmaLoopRunningProcess|commandForPragmaLoopShellRun|newCommandFiles|writeCommandStatus|runningCommandContent|commandForShellRun|processGroupSummary" internal/query/miniswe_loop.go internal/tools/bash internal/shellrun -g'*.go'` showed shell process lifecycle and artifact helpers only in `internal/shellrun`.

## 22. Deterministic Replay Replays Provider Responses But Still Runs Live Tools

Severity: high

Concrete files/functions involved:

- `cmd/pragma/replay.go`: `replayDeterministic`, `extractFirstUserPrompt`
- `internal/observe/replay.go`: `LoadReplay`, `ToolOutput`, `APIResponse`, `APIRequest`, `indexAPIResponses`
- `internal/observe/recorder.go`: `RecordedToolOutput`, `Recorder.HandleEvent`
- `internal/provider/replay/provider.go`: `Provider.Complete`, `Provider.Stream`, `nextResponse`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/query/miniswe_loop.go`: `Engine.runPragmaLoop`, `runPragmaLoopBash`

What responsibility is split or misplaced:

Replay has two incompatible owners. `observe.LoadReplay` loads recorded tool outputs and provider request/response artifacts, but deterministic replay only swaps the provider for `internal/provider/replay.Provider`. The query engine, tool orchestrator, and pragma-loop shell runner are still the normal live runtime. `extractFirstUserPrompt` also cannot reconstruct the original prompt from events and substitutes `"Replay: continue from recorded session"`.

Why this is wrong in ownership/lifecycle terms:

Deterministic replay is a runtime execution mode, not just a provider adapter. If replay owns recorded provider responses, it must also own whether side-effecting tool calls are replayed from artifacts or executed live. Loading recorded tool outputs into `ReplayEngine` while no execution path consults `ToolOutput` leaves the replay persistence boundary unused.

Observable bug or likely failure mode:

A replayed response containing a Bash action on the default pragma-loop path will be executed again by `runPragmaLoopBash`. On the legacy tool-loop path, recorded tool-call responses can drive the normal orchestrator to invoke real tools instead of returning the recorded output. A "deterministic" replay can therefore mutate the current workspace, hit the network, or produce different tool results even though recorded tool-output artifacts were loaded.

Minimal direction for fixing the boundary:

Make replay a first-class execution mode for the runtime/tool boundary. Either inject a replaying tool orchestrator/command runner that returns recorded outputs by tool call ID, or make deterministic replay read-only and only render the recorded event stream without running the engine. Store enough prompt/request data to restart from the original prompt when execution replay is truly needed.

What not to do:

Do not add another provider-side fallback or keep relying on the generic prompt from `extractFirstUserPrompt`. Provider replay alone cannot make tool execution deterministic.

Status:

Resolved in current worktree for pure deterministic replay. Deterministic replay is now read-only: it renders recorded API responses from `observe.ReplayEngine` instead of constructing the live runtime, registering tools, running the query engine, or replaying from a synthetic prompt. The explicit `--then-live` handoff remains the only path that creates runtime dependencies and live provider calls.

Source evidence:

- `cmd/pragma/replay.go`: `replayDeterministic` returns `replayRecordedResponses` immediately when `--then-live` is not set, so pure deterministic replay never calls `cli.SetupDeps`, `RegisterTools`, or `Engine.Run`.
- `cmd/pragma/replay.go`: removed replay-provider construction, non-interactive prompter/asker setup, live tool registration, and `queryEngine.Run` from deterministic replay.
- `cmd/pragma/replay.go`: removed `extractFirstUserPrompt` and the `"Replay: continue from recorded session"` fallback prompt.
- `cmd/pragma/replay.go`: deterministic replay help text now says it renders recorded API responses read-only and executes no tools.

Verification evidence:

- Non-test verification: `gofmt -w cmd/pragma/replay.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./cmd/pragma ./internal/observe ./internal/provider/replay`; `git diff --check`.
- Contract scan: `rg -n "extractFirstUserPrompt|Replay: continue|replayprov|NonInteractivePrompter|NonInteractiveAsker|RegisterTools|queryEngine\\.Run|runPragmaLoopBash|shellrun\\.Execute" cmd/pragma/replay.go internal/query/miniswe_loop.go` showed no deterministic replay runtime/tool-registration path; `runPragmaLoopBash` remains only in the normal query path.

## 23. Lifecycle Conditional Routing Treats Unknown Route Keys As END

Severity: high

Concrete files/functions involved:

- `internal/lifecycle/graph.go`: `RouterFunc`, `ConditionalEdge`, `Builder.AddConditionalEdges`, `Builder.Build`
- `internal/lifecycle/executor.go`: `Executor.resolveNextNodes`
- `internal/lifecycle/definition/routers.go`: `fieldRouter`, `stopReasonRouter`, `passFailRouter`
- `internal/lifecycle/bridge/routers.go`: `FieldRouter`, `StopReasonRouter`, `PassFailRouter`
- `internal/lifecycle/definition/resolve.go`: conditional-edge resolution into `AddConditionalEdges`

What responsibility is split or misplaced:

The graph definition owns valid route keys through `ConditionalEdge.PathMap`, while routers compute dynamic route keys at runtime. `Executor.resolveNextNodes` performs `target := ce.PathMap[key]` without checking whether `key` exists, then treats `target == ""` as an END transition. That makes an unmapped route key indistinguishable from an explicit path mapping to END.

Why this is wrong in ownership/lifecycle terms:

Routing is the control-plane contract for the lifecycle state machine. A state machine executor should not silently convert an unhandled state into successful termination. If the graph has no path for a router output, either the graph definition is incomplete or the router produced invalid state; that is a control-flow error owned by the lifecycle executor/graph boundary.

Observable bug or likely failure mode:

A `field:<key>` router returns an arbitrary string value or `""` when the field is missing. If the YAML or generated graph does not include that key in `paths`, the executor emits a transition to `END` and completes the graph instead of surfacing a missing route. A typo in a generated path key or unexpected LLM/evaluator state can therefore look like a clean lifecycle completion.

Minimal direction for fixing the boundary:

Distinguish explicit END from missing routes in `resolveNextNodes`, for example by checking `target, ok := ce.PathMap[key]`. Treat `!ok` as a lifecycle error with the node and route key, while preserving `target == ""` only for explicit END mappings.

What not to do:

Do not add more default `""` paths to generated graphs or make routers coerce unknown values to `"end"`. That hides invalid control state instead of making the graph contract explicit.

Status:

Resolved in current worktree. Conditional route resolution now checks the `PathMap` membership bit before interpreting the target. Missing route keys return a lifecycle error from `Executor.resolveNextNodes`, while explicit `target == ""` mappings still emit an END transition. `RouterFunc` documentation now states that routers return route keys and `ConditionalEdge.PathMap` owns the route-key-to-node/END mapping.

Source evidence:

- `internal/lifecycle/executor.go`: `Executor.Stream` now handles `resolveNextNodes` errors by emitting/completing with that error instead of falling through to normal graph completion.
- `internal/lifecycle/executor.go`: `Executor.resolveNextNodes` now uses `target, ok := ce.PathMap[key]`; `!ok` returns `lifecycle: node %q router returned unmapped route key %q`.
- `internal/lifecycle/executor.go`: explicit END behavior remains tied to `target == ""` after a successful `PathMap` lookup.
- `internal/lifecycle/graph.go`: `RouterFunc` contract documentation now names route keys and keeps node/END mapping ownership with `ConditionalEdge.PathMap`.

Verification evidence:

- Non-test verification: `gofmt -w internal/lifecycle/executor.go internal/lifecycle/graph.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/lifecycle ./internal/lifecycle/definition ./internal/lifecycle/bridge ./cmd/pragma`.
- Contract scan: `rg -n "resolveNextNodes\\(|PathMap\\[key\\]|unmapped route key|RouterFunc" internal/lifecycle -g'*.go'` showed the executor uses `target, ok := ce.PathMap[key]`, the missing-key error is present, and no unchecked `PathMap[key]` lookup remains in lifecycle routing.

## 24. UI Close Paths Close The Session Writer Before Session Lifecycle Ends

Severity: medium

Concrete files/functions involved:

- `internal/cli/run.go`: `InteractiveRuntime.CloseSession`, `InteractiveRuntime.Cleanup`, `endSessionLifecycle`, `closeCurrentSessionAfterClear`, `InteractiveRuntime.Resume`
- `internal/web/web.go`: `Run`
- `internal/tui/handlers.go`: `Model.quit`
- `internal/cli/run.go`: `makeSessionSaveClose`

What responsibility is split or misplaced:

The session lifecycle has two owners depending on how the session ends. Runtime-owned transitions such as `/clear` and resume call `endSessionLifecycle` before `sessionClose`. UI shutdown paths call the exported `CloseSession` callback, which only closes the session writer; `SessionEnd` hooks are deferred until `InteractiveRuntime.Cleanup` runs after the web server or TUI exits.

Why this is wrong in ownership/lifecycle terms:

Closing durable session persistence and ending the domain session are one lifecycle transition. Presentation code should not be able to close the writer while leaving `Deps.SessionStarted` true and the hook lifecycle unfinished. The runtime/session owner should define one close sequence for all callers.

Observable bug or likely failure mode:

On web shutdown, `web.Run` calls `cfg.CloseSession()` when the parent context is done, then `RunInteractive`'s deferred cleanup calls `endSessionLifecycle`. On TUI quit, `Model.quit` calls `m.closeSession()`, then the deferred runtime cleanup ends the lifecycle. In both paths `SessionEnded` can be emitted and the session file closed before `SessionEnd` hooks run, while `/clear` and resume run the opposite order. Hooks and observability therefore see different lifecycle ordering for the same logical "session is ending" transition.

Minimal direction for fixing the boundary:

Make the UI callback call a single runtime method that performs the complete close transition: save if needed, run `SessionEnd`, close the writer, emit `SessionEnded`, and mark the session not started. Reuse that same method for `/clear`, resume, TUI quit, and web shutdown.

What not to do:

Do not add another `endSessionLifecycle` call in web or TUI. That would preserve presentation ownership of session lifecycle ordering.

Status:

Resolved in current worktree. Interactive session shutdown now goes through a single runtime-owned close transition. The exported UI callback `InteractiveRuntime.CloseSession` delegates to `closeCurrentSession`, and the same method is reused by `/clear`, resume replacement, and deferred runtime cleanup. Web and TUI still call only the runtime callback; they do not own `SessionEnd` hook ordering.

Source evidence:

- `internal/cli/run.go`: `InteractiveRuntime.closeCurrentSession` saves pending session state, runs `endSessionLifecycle`, then closes the session writer through `sessionClose`, preserving `SessionEnd` before `SessionEnded`.
- `internal/cli/run.go`: `InteractiveRuntime.CloseSession` now calls `closeCurrentSession(context.Background())` instead of directly calling `sessionClose`.
- `internal/cli/run.go`: `closeCurrentSessionAfterClear`, `InteractiveRuntime.Resume`, and `InteractiveRuntime.Cleanup` now reuse `closeCurrentSession`, so clear, resume, UI shutdown, and deferred cleanup share the same close ordering.
- `internal/web/web.go` and `internal/tui/handlers.go`: presentation code continues to call only the runtime-provided close callback; no new presentation-owned `endSessionLifecycle` call was added.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/web ./internal/tui ./cmd/pragma`.
- Ownership scan: `rg -n "CloseSession\\(|closeCurrentSession\\(|closeCurrentSessionAfterClear|endSessionLifecycle\\(|sessionClose\\(\\)|sessionSave\\(\\)" internal/cli/run.go internal/web/web.go internal/tui/handlers.go -g'*.go'` showed web/TUI still call only the callback, while interactive runtime paths route through `closeCurrentSession`; the remaining direct `endSessionLifecycle` hit is the separate non-interactive command runner.

## 25. Model And Provider Switches Leave Compaction Runtime Bound To Old State

Severity: high

Concrete files/functions involved:

- `internal/cli/run.go`: `BuildInteractiveRuntime`, `BuildCompactionDeps`, `InteractiveRuntime.applyResumeProvider`
- `internal/cli/tools.go`: `switchActiveModel`, `applyLiveConfigValue`
- `internal/slash/commands.go`: `handleModel`, `handleCompact`
- `internal/slash/command.go`: `Deps.Compactor`, `ContextWindowFunc`, `ModelSwitcher`, `OnModelChanged`
- `internal/query/engine.go`: `Engine.SetCompaction`, `Engine.RebindProvider`
- `internal/query/loop.go`: `Engine.autoCompactBeforeRequest`, `Engine.isAtBlockingLimit`
- `internal/compact/compact.go`: `Service`, `NewService`, `Compact`

What responsibility is split or misplaced:

The active provider/model can be changed by `/model`, the `Config` tool's live `model` setting, and interactive resume. Those paths update visible state, provider bindings, and the token monitor, but compaction dependencies are built once in `BuildInteractiveRuntime` and installed with `Engine.SetCompaction`. The engine keeps the old `WindowConfig`, and `slash.Deps.Compactor` plus `Engine.compactor` keep a `compact.Service` that captured the provider and secondary model at construction.

Why this is wrong in ownership/lifecycle terms:

Provider/model switching is a runtime configuration transition. Every runtime component that derives provider, model, context window, or secondary-model behavior must be rebound by the same owner. Updating the main provider and token monitor while leaving compaction with old thresholds and an old provider snapshot splits one logical model transition across unrelated callbacks.

Observable bug or likely failure mode:

After switching from a small-context model to a large-context model, `Engine.autoCompactBeforeRequest` still calls `ShouldAutoCompact` with the old `windowConfig`, so it can compact too early or block at the wrong limit. After resuming a session with a different provider, `applyResumeProvider` rebinds `rt.Engine` and provider-backed tools, but existing manual `/compact` and auto-compaction services can still summarize through the provider captured before resume.

Minimal direction for fixing the boundary:

Make model/provider rebind rebuild and reinstall compaction dependencies through the same runtime owner that rebinds the provider. Update `Engine.SetCompaction`, `SlashDeps.Compactor`, context-window callbacks, and secondary-model selection together whenever the active provider/model changes or a session is resumed.

What not to do:

Do not patch only the toolbar/token monitor or recompute thresholds inside `handleModel`. The stale state is in the runtime compaction owner, not the presentation text around model switching.

Status:

Resolved in current worktree. Interactive model changes and resume provider changes now rebind compaction through `InteractiveRuntime`, the same owner that rebinds the active provider/model runtime. `/model` and the Config tool both route live interactive model switches through the runtime callback, which updates the active model state and reinstalls compaction dependencies into both the query engine and slash command deps. Resume provider changes rebuild compaction after provider/model rebinding as well.

Source evidence:

- `internal/cli/run.go`: `InteractiveRuntime.switchActiveModel` wraps `switchActiveModel`, updates slash model state, and calls `rebindCompaction`.
- `internal/cli/run.go`: `InteractiveRuntime.rebindCompaction` rebuilds `BuildCompactionDeps`, calls `Engine.SetCompaction`, updates `SlashDeps.Compactor`, and refreshes `SlashDeps.ContextWindowFunc`.
- `internal/cli/run.go`: `InteractiveRuntime.applyResumeProvider` now calls `rebindCompaction` after provider/model rebind and provider-backed tool rebind.
- `internal/cli/run.go`: `BuildInteractiveRuntime` installs `rt.switchActiveModel` as both `SlashDeps.ModelSwitcher` and `Deps.ModelSwitcher`, so slash `/model` and runtime-owned tool callbacks share the same rebind path.
- `internal/cli/tools.go`: the Config tool live `model` setting delegates to `Deps.ModelSwitcher` when interactive runtime installs one; the fallback helper still supports non-interactive setup.
- `internal/cli/tools.go`: `switchActiveModel` updates `Deps.Cfg.Model`, so rebuilt compaction uses the current model when deriving the context window.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/deps.go internal/cli/tools.go internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/query ./internal/slash ./cmd/pragma`.
- Ownership scan: `rg -n "ModelSwitcher|switchActiveModel|rebindCompaction|BuildCompactionDeps|SetCompaction|SlashDeps\\.Compactor|ApplyLiveValue|applyLiveConfigValue|applyResumeProvider" internal/cli internal/slash internal/query -g'*.go'` showed interactive `/model`, Config live model changes, and resume provider rebind now converge on `InteractiveRuntime.rebindCompaction`; initial setup and non-interactive command setup remain separate setup-time paths.

## 26. Subagent And Graph Execution Use Setup-Time Model State After `/model`

Severity: high

Concrete files/functions involved:

- `internal/cli/tools.go`: `switchActiveModel`, `RegisterTools` `engineFactory`
- `internal/slash/commands.go`: `handleModel`
- `internal/tools/agent/agent.go`: `Tool.invoke`, `Tool.RunForked`, `Tool.runGraphSync`, `Tool.compileStructure`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/query/miniswe_loop.go`: `Engine.runPragmaLoopWithInitialPrompt`
- `internal/query/engine.go`: `Engine.runGraph`
- `internal/model/conversation.go`: `Conversation.Fork`

What responsibility is split or misplaced:

The active model is stored in more than one place with different readers. `/model` calls `switchActiveModel`, which updates only `AppState.Model` and the token monitor. The main query loops resolve the model from `store.Snapshot().Model`, but `RegisterTools` builds subagent stores from setup-time `d.Cfg.Model`, and subagent configs inherit setup-time `d.EngineCfg.Model` unless the Agent input explicitly provides a model override. `Engine.runGraph` also passes `e.config.Model` into lifecycle runner config instead of using the current store model.

Why this is wrong in ownership/lifecycle terms:

Model selection is a runtime state transition. All execution surfaces that issue model calls should consult the same active model owner or be rebound by the transition. Instead, foreground chat, subagent chat, forked skills through Agent, and graph-backed agent execution each read a different copy of model state.

Observable bug or likely failure mode:

After a user switches models with `/model`, the next foreground turn can use the new model because `Engine.runLoop` and `runPragmaLoopWithInitialPrompt` prefer `snap.Model`. A subsequent `Agent` call without an explicit `model` can still create a subengine seeded from the old `d.Cfg.Model` and old `d.EngineCfg.Model`. Structured Agent runs that enter `Engine.runGraph` can likewise run LLM nodes with the old `EngineConfig.Model`.

Minimal direction for fixing the boundary:

Make the runtime model owner explicit and have subagent factories and graph execution resolve from it at run time. At minimum, update provider/model rebind to keep `Deps.Cfg`, `Deps.EngineCfg`, root store state, child engine factory defaults, and graph runner config consistent.

What not to do:

Do not require every Agent call to pass a `model` argument or patch the Agent prompt text to mention the active model. The execution boundary should inherit the runtime's active model by default.

Status:

Resolved in current worktree. Model switching now keeps the runtime model copies used by execution in sync, and subagent/graph execution resolve the active model at run time. Agent child engines inherit the active model unless the Agent input explicitly supplies a model override; lifecycle graph execution now follows the same `AppState.Model` over engine-config fallback rule used by normal foreground turns.

Source evidence:

- `internal/cli/tools.go`: `switchActiveModel` now updates `Deps.Cfg.Model`, `Deps.EngineCfg.Model`, and the root `query.Engine` fallback model through `Engine.SetModel`.
- `internal/cli/tools.go`: the Agent engine factory now derives `subModel` with `activeModelForDeps`, applies explicit Agent model overrides only after that, and writes the resolved model into the forked conversation, child `AppState.Model`, and child `EngineConfig.Model`.
- `internal/cli/tools.go`: `activeModelForDeps` resolves the current store model first, then engine config, then config defaults.
- `internal/query/engine.go`: `Engine.SetModel` owns fallback model rebinding for execution paths without an app-state override.
- `internal/query/engine.go`: `Engine.runGraph` now resolves `snap.Model` before falling back to `e.config.Model`, matching foreground query and pragma-loop execution behavior.
- `internal/tools/agent/agent.go`: `SubAgentSpawned` now reports the actual resolved child model from the child store instead of the explicit input field, which is empty for inherited-model runs.

Verification evidence:

- Non-test verification: `gofmt -w internal/query/engine.go internal/cli/tools.go internal/tools/agent/agent.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/query ./internal/tools/agent ./cmd/pragma`.
- Ownership scan: `rg -n "d\\.Cfg\\.Model|d\\.EngineCfg\\.Model|e\\.config\\.Model|SetModel|activeModelForDeps|subCfg\\.Model|forkedConv\\.Model|Model:\\s+in\\.Model|actualModel|runGraph" internal/cli/tools.go internal/query/engine.go internal/tools/agent/agent.go -g'*.go'` showed subagent defaults now use `activeModelForDeps`, graph execution resolves `snap.Model`, and no subagent spawn event reports `in.Model` directly.

## 27. Orchestration FSM Transitions Depend On Prompt-Directed Files And Parsed Text

Severity: high

Concrete files/functions involved:

- `internal/orchestration/runner.go`: `runEvents`, `RunNodeEvents`, `RunStateEvents`, `BuildPromptWithArtifactRoot`, `RenderNextHandoffInstructionsWithArtifactRoot`, `selectedHandoffPrompt`
- `internal/orchestration/runner.go`: `SelectStateEvent`, `selectDecisionEvent`, `lastDecisionValue`
- `internal/orchestration/orchestration.go`: `State.Event`, `FileEventRule`, `Transition`, `Runtime.FSM`
- `internal/query/miniswe_loop.go`: `Engine.RunPragmaLoopWithSystem`

What responsibility is split or misplaced:

The orchestration runtime owns the FSM, but persona states decide the next event through prompt-shaped side effects. `RunStateEvents` runs a pragma-loop model turn, `RenderNextHandoffInstructionsWithArtifactRoot` tells the model to write handoff files before final completion, `selectedHandoffPrompt` reads the handoff file for the selected event, and `SelectStateEvent` can choose the event by reading a configured file and parsing `Decision:` text or substring rules.

Why this is wrong in ownership/lifecycle terms:

FSM transition selection and handoff production are runtime control-plane data. They should be produced as structured state by the orchestration runner or a controlled node result, not inferred from model prose and files the model was asked to create through shell commands. The current split makes the state machine depend on prompt compliance and filesystem side effects outside the runtime's transition transaction.

Observable bug or likely failure mode:

A persona can finish without writing the requested handoff file; `selectedHandoffPrompt` treats missing files as empty handoff and the FSM still transitions. A stale or malformed file can control `SelectStateEvent`, and `lastDecisionValue` uses the last `Decision:` line rather than a typed event result. The orchestration can therefore advance with missing handoff context or route from incidental text that matched a rule.

Minimal direction for fixing the boundary:

Make persona state execution return a structured result containing the event and handoff payload, and have the runtime persist any handoff artifact as part of applying that result. Keep file-based imports as explicit inputs if needed, but do not make model-authored files and parsed prose the normal transition boundary.

What not to do:

Do not add stricter prompt wording, more substring rules, or a web-side warning for missing handoff files. Those preserve prompt/filesystem side effects as the FSM control interface.

Status:

Resolved in current worktree. Orchestration FSM event selection no longer reads model-authored files or parses `Decision:` prose. Persona states now select events only through explicit `event.default` or runtime control states; `from_file` selection fails fast with an ownership-boundary error. Handoff propagation no longer depends on prompt-directed handoff files: the runtime captures the completed persona output from `RunStateEvents` and passes that text as the next phase handoff payload.

Source evidence:

- `internal/orchestration/runner.go`: `runEvents` now receives `(event, nextHandoff)` from `RunNodeEvents` and emits a runtime handoff event without reading a handoff file path.
- `internal/orchestration/runner.go`: `RunNodeEvents` now returns the selected event and runtime-captured handoff text; control states return only their control event.
- `internal/orchestration/runner.go`: `SelectStateEvent` now rejects `state.Event.FromFile` with `file-based orchestration event selection is unsupported...` instead of reading files and parsing text.
- `internal/orchestration/runner.go`: the `Decision:` parser helpers, `selectedHandoffPrompt`, `handoffPromptPath`, and prompt-rendered handoff-file instructions were removed.
- `internal/orchestration/runner.go`: `EnsureRunDirs` no longer resets or creates `handoff-prompts`; it only prepares runtime artifact/process directories.

Verification evidence:

- Non-test verification: `gofmt -w internal/orchestration/runner.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/orchestration ./cmd/pragma`.
- Ownership scan: `rg -n "selectedHandoffPrompt|handoffPromptPath|RenderNextHandoff|handoff-prompts|selectDecisionEvent|lastDecisionValue|Decision:|from_file|RunNodeEvents\\(" internal/orchestration cmd/pragma/orchestration.go -g'*.go'` showed no handoff-file reader, no prompt-rendered handoff target, and no `Decision:` parser remains; only the schema `from_file` field remains, with runtime rejection.

## 28. MCP Resource Reads Persist Binary Blobs In Workspace Cache Outside Session Artifacts

Severity: medium

Concrete files/functions involved:

- `internal/tools/mcp/read.go`: `ReadTool.Invoke`, `ReadTool.persistBinary`, `readInput`, `contentEntry`
- `internal/mcp/client.go`: `Client.ReadResource`
- `internal/toolresult/storage.go`: `persistToolResult`, `PersistedOutputPath`
- `internal/session/writer.go`: `WriteMessage`, `WriteMetadata`, `WriteFileState`

What responsibility is split or misplaced:

`ReadMcpResourceTool` is flagged read-only but writes binary MCP resource blobs to `snap.WorkDir()/.pragma/cache` with random filenames and returns the path in the tool result. This artifact is not written through the session writer, tool-result persistence, or a runtime artifact registry. Text resources remain in the tool result; binary resources become workspace files owned by the tool implementation.

Why this is wrong in ownership/lifecycle terms:

Persisting external resource content is a session/runtime artifact decision, not a read-tool implementation detail. The runtime already has session-local tool-result persistence for oversized outputs; MCP binary blobs bypass that boundary and create durable workspace state from a read-only tool call.

Observable bug or likely failure mode:

A read-only MCP resource call can leave files under the project `.pragma/cache` directory, outside the session artifact tree and without metadata that ties the file to a session, tool call, or cleanup policy. Resume, replay, and session export can preserve the JSON path in conversation history while the actual blob is missing, stale, or from another checkout.

Minimal direction for fixing the boundary:

Route binary MCP resource persistence through the same session artifact/tool-result owner used for other large tool outputs, or return a structured in-memory/tool-result reference that the session writer commits. If workspace materialization is needed, make it an explicit write-capable operation.

What not to do:

Do not rename `.pragma/cache` or add periodic cleanup inside `ReadTool`. The boundary leak is that a read-only tool owns durable artifact creation.

Status:

Resolved in current worktree. Binary MCP resource content is no longer written by the read-only MCP tool into the workspace `.pragma/cache`. The MCP read tool now asks the session tool-result artifact owner to persist decoded binary bytes under the session `tool-results` tree and returns a structured `blob_artifact_path`. If no session ID is available, the tool returns a structured inline base64 payload instead of creating workspace files.

Source evidence:

- `internal/tools/mcp/read.go`: `ReadTool` no longer has `CacheDir` and no longer imports or calls `os.WriteFile`, `os.MkdirAll`, random filename generation, or workspace `.pragma/cache` paths.
- `internal/tools/mcp/read.go`: binary entries now report `blob_artifact_path`, `blob_bytes`, and optional `blob_base64` fallback instead of `blob_saved_to`.
- `internal/tools/mcp/read.go`: `persistBinaryArtifact` gets the session ID from `tool.SessionIDFrom` and delegates persistence to `toolresult.PersistBinaryOutput`.
- `internal/toolresult/storage.go`: `PersistBinaryOutput` and the shared byte persistence helper write binary artifacts under the existing session `tool-results` directory.

Verification evidence:

- Non-test verification: `gofmt -w internal/tools/mcp/read.go internal/toolresult/storage.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/tools/mcp ./internal/toolresult ./cmd/pragma`.
- Ownership scan: `rg -n "persistBinary|CacheDir|\\.pragma.*cache|MkdirAll|WriteFile|PersistBinaryOutput|blob_saved_to|blob_artifact_path|BlobBase64|SessionIDFrom" internal/tools/mcp/read.go internal/toolresult/storage.go -g'*.go'` showed MCP no longer owns workspace cache writes; the remaining write directory creation is in `internal/toolresult/storage.go`.

## 29. CLI Tool Filters Do Not Apply To Subagent Registries

Severity: high

Concrete files/functions involved:

- `internal/cli/run.go`: `applyToolFilters`
- `internal/cli/tools.go`: `RegisterTools`, `engineFactory`, `shouldRegisterBuiltinTool`, `baseTools`
- `internal/tools/agent/agent.go`: `Tool.invoke`, `RunForked`, `excludeTool`
- `internal/tool/registry.go`: `Registry.Unregister`, `Registry.Scoped`
- `internal/slash/skills_cmd.go`: skill execution through `Agent.RunForked`

What responsibility is split or misplaced:

Tool exposure policy is applied in two different places. Toolsets are checked during registration through `shouldRegisterBuiltinTool`, but CLI `--allowed-tools` and `--disallowed-tools` are applied later by physically unregistering tools from the root registry. The Agent tool's `engineFactory` does not clone the filtered root registry; it builds a fresh subagent registry from `baseTools(d, subStore)` and only applies optional Agent/skill scoped tool names.

Why this is wrong in ownership/lifecycle terms:

Tool availability is a runtime execution policy. Root turns, subagent turns, and skill-forked turns should all inherit the same effective policy unless a narrower child scope is explicitly selected. Applying CLI filters as root-registry mutation leaves child engine creation outside the policy owner.

Observable bug or likely failure mode:

Running with `--allowed-tools Agent` can leave only Agent visible to the root model after `applyToolFilters`, but an Agent call with no explicit scoped tool list creates a subengine with the full built-in registry permitted by the toolset. A user who disallowed `Bash`, `Write`, or editing tools at the CLI boundary can still get those tools executed indirectly by a subagent.

Minimal direction for fixing the boundary:

Represent the effective tool policy in `Deps` or a registry factory and apply it whenever any engine registry is built. Child registries should start from the same filtered tool set as the parent and then apply any additional Agent/skill scope.

What not to do:

Do not special-case Agent prompts or add deny checks only inside the Agent tool. The tool-exposure policy must live at the registry construction boundary shared by all engines.

Status:

Resolved in current worktree. CLI `--allowed-tools` and `--disallowed-tools` filtering is now enforced through `Deps.ToolPolicy` at shared registry construction instead of by mutating only the root registry after construction. Root registry construction and Agent child registry construction both call `shouldRegisterBuiltinTool`, so subagents inherit the same effective CLI exposure policy before any narrower Agent/skill scope is applied.

Source evidence:

- `internal/cli/tools.go`: root and child registries both register descriptors through `shouldRegisterBuiltinTool`, which checks `Deps.ToolPolicy.Allows`.
- `internal/cli/tools.go`: child registries still apply `Registry.Scoped(scopedToolNames)` after shared policy filtering, so explicit Agent/skill scopes can only narrow the already-filtered set.
- `internal/cli/run.go`: `BuildInteractiveRuntime` and non-interactive setup no longer call `applyToolFilters` after `RegisterTools`; the active runtime path uses registration-time policy instead of root-only registry mutation.
- `internal/cli/run.go`: late synthetic `StructuredOutput` registration is now gated by `d.ToolPolicy.Allows` so it cannot bypass the shared exposure policy.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/tools/agent ./internal/slash ./cmd/pragma`.
- Ownership scan: `rg -n "applyToolFilters\\(|shouldRegisterBuiltinTool\\(|ToolPolicy\\.Allows|ModelSwitcher|RegisterTools|baseTools\\(|Scoped\\(" internal/cli internal/tools/agent internal/slash -g'*.go'` showed no active runtime calls to `applyToolFilters`; root and child registry construction share `shouldRegisterBuiltinTool`, with child scoping applied afterward.

## 30. Subagent File Mutations Are Tracked In Child Engine Caches Only

Severity: high

Concrete files/functions involved:

- `internal/cli/tools.go`: `RegisterTools`, `engineFactory`
- `internal/query/engine.go`: `NewEngine`, `ResetFileState`, `FileStateRecords`
- `internal/query/loop.go`: `Engine.executeToolBatch`, `progressSnapshot.ReadFileState`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/tool/filestate.go`: `FileStateCache`, `RecordFileWriteState`, `RecordFileDelete`, `EffectsSince`
- `internal/tools/agent/agent.go`: `Tool.invoke`, `runSync`, `drainAgentRunEvents`
- `internal/cli/run.go`: `makeSessionSaveClose`

What responsibility is split or misplaced:

File freshness and mutation state is owned by each `query.Engine` instance. The root engine creates and restores its own `FileStateCache`, tool execution records writes through the cache exposed by that engine's `progressSnapshot`, and session save writes only `d.Engine.FileStateRecords()`. Subagents are built by `engineFactory` with a new state store and a new engine, so their file-state cache is separate; the Agent drain path reduces child execution to result text, usage, tool counts, and task state.

Why this is wrong in ownership/lifecycle terms:

File freshness is workspace/session state when root and child engines can mutate the same checkout. Making it private to an engine instance means the component that persists the session and enforces later freshness checks does not own all mutations that happened during the session.

Observable bug or likely failure mode:

A synchronous Agent running in the shared worktree can edit or delete files through its child engine. The child tool execution records those effects in the child cache, but the parent session writer persists only the root engine cache. Later root `Edit` or `Write` calls can make decisions from stale file state, and resuming the session loses the file-state knowledge produced by subagent tools even though the parent conversation contains the Agent result.

Minimal direction for fixing the boundary:

Move file-state tracking to a shared workspace/session runtime owner, or explicitly merge child engine file-state effects into the parent/session owner when a subagent completes. The session writer should persist the authoritative shared cache, not whichever cache belongs to the root query engine.

What not to do:

Do not add a note to the Agent result asking the parent model to re-read files, and do not add one-off parent cache invalidation after Agent text is returned. That leaves freshness as a prompt convention instead of a runtime invariant.

Status:

Resolved in current worktree. Subagent engines now share the parent query engine's file-state cache instead of creating an isolated authoritative cache for shared-workspace execution. The session writer continues to persist `d.Engine.FileStateRecords()`, and child tool file effects now land in that same cache because the Agent engine factory injects the parent cache into child engines at creation time.

Source evidence:

- `internal/query/engine.go`: `Engine.FileStateCache` and `Engine.SetFileStateCache` expose a controlled cache handoff for runtime-owned sharing.
- `internal/cli/tools.go`: the Agent `engineFactory` creates the child engine, then calls `subEngine.SetFileStateCache(d.Engine.FileStateCache())` when the parent engine exists.
- `internal/cli/run.go`: session save still writes `d.Engine.FileStateRecords()`, which is now the shared cache used by parent and child engines.
- `internal/query/loop.go`: tool execution still obtains file-state tracking from the engine-owned cache through `progressSnapshot.ReadFileState`, so no prompt-level or Agent-result convention was added.

Verification evidence:

- Non-test verification: `gofmt -w internal/query/engine.go internal/cli/tools.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/query ./internal/cli ./internal/tools/agent ./cmd/pragma`.
- Ownership scan: `rg -n "FileStateCache\\(|SetFileStateCache|FileStateRecords|NewFileStateCache|ReadFileState|engineFactory|d\\.Engine" internal/query internal/cli/tools.go internal/cli/run.go internal/tools/agent -g'*.go'` showed child engine creation now shares `d.Engine.FileStateCache()`, while session persistence remains rooted at `d.Engine.FileStateRecords()`.

## 31. Subagent Tool Result Artifacts Are Written Under Unsaved Forked Session IDs

Severity: high

Concrete files/functions involved:

- `internal/tools/agent/agent.go`: `Tool.invoke`, `runSync`, `drainAgentRunEvents`
- `internal/cli/tools.go`: `RegisterTools`, `engineFactory`
- `internal/model/conversation.go`: `Conversation.Fork`
- `internal/app/state.go`: `AppState.SessionID`
- `internal/query/loop.go`: `progressSnapshot.SessionID`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/tool/tool.go`: `SessionIDFrom`
- `internal/toolresult/storage.go`: `ProcessToolResult`, `persistToolResult`
- `internal/tools/toolresultread/toolresultread.go`: `Tool.Invoke`

What responsibility is split or misplaced:

Oversized tool-result persistence is keyed by whatever `SessionIDFrom(state)` returns at tool execution time. For subagents, `Tool.invoke` forks the conversation with a new ID and `engineFactory` runs the child engine with that forked conversation. `AppState.SessionID` therefore returns the child conversation ID, and `persistToolResult` writes to `~/.pragma/sessions/<forked-id>/tool-results/...` even though the durable session writer and metadata belong to the parent conversation.

Why this is wrong in ownership/lifecycle terms:

Tool-result artifacts are durable session artifacts, not private child-conversation files. A forked conversation used for model context is not the same thing as a resumable session with a writer, header, metadata, close lifecycle, and export policy.

Observable bug or likely failure mode:

If a subagent tool output exceeds the max result size, the child run can persist the full output under a session directory named for the forked conversation ID. The parent session records only the Agent result text, not a child session header or artifact index. During the child run, `tool_result.read` can resolve the artifact through the child state, but after the Agent returns, parent resume/export has no durable mapping from the parent session to that orphaned tool-result file.

Minimal direction for fixing the boundary:

Pass an explicit artifact owner/run owner into tool-result processing, and make subagent tool-result blobs attach to the real parent session with child conversation or tool-call namespacing as metadata. If child sessions become first-class, create them through the session lifecycle, not implicitly through tool-result storage.

What not to do:

Do not create empty session headers for every forked conversation just to make the files look valid. That would bless a model-context fork as a session lifecycle owner instead of fixing artifact ownership.

Status:

Resolved in current worktree. Subagent stores now distinguish model-context conversation IDs from durable artifact session ownership. Forked child conversations keep their fork IDs, but `AppState.SessionID()` can return an explicit `ArtifactSessionID`, and the Agent engine factory sets child `ArtifactSessionID` to the parent session ID. Tool-result processing continues to use `tool.SessionIDFrom(state)`, so oversized child tool outputs and `tool_result.read` now attach to the real parent session artifact tree instead of an unsaved forked-session directory.

Source evidence:

- `internal/app/state.go`: `AppState` now has `ArtifactSessionID`; `SessionID()` returns that owner when present and falls back to `Conversation.ID` for normal root state.
- `internal/cli/tools.go`: the Agent `engineFactory` derives `parentSessionID` from the parent store and writes it into each child store's `ArtifactSessionID`.
- `internal/query/loop.go`: `progressSnapshot.SessionID` still delegates to `tool.SessionIDFrom`, so the artifact owner is supplied by state rather than hard-coded in tool-result processing.
- `internal/tool/orchestrator.go` and `internal/toolresult/storage.go`: oversized tool-result persistence still flows through `ProcessToolResult` with the session ID from state, now resolving child runs to the parent artifact owner.

Verification evidence:

- Non-test verification: `gofmt -w internal/app/state.go internal/cli/tools.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/app ./internal/cli ./internal/query ./internal/tool ./internal/toolresult ./internal/tools/agent ./internal/tools/toolresultread ./cmd/pragma`.
- Ownership scan: `rg -n "ArtifactSessionID|SessionID\\(|SessionIDFrom|ProcessToolResult|persistToolResult|tool_result.read|forkedConv|parentSessionID|Conversation\\.Fork" internal/app internal/cli/tools.go internal/query internal/tool internal/toolresult internal/tools/agent internal/tools/toolresultread -g'*.go'` showed child stores set parent artifact ownership while forked conversations remain the model-context identity.

## 32. Background Agent Completion Lives Only In The Volatile Task Registry

Severity: high

Concrete files/functions involved:

- `internal/tools/agent/agent.go`: `Tool.invoke`, `runBackground`, `drainAgentRunEvents`, `completeAgentTask`, `failAgentTask`
- `internal/task/registry.go`: `Registry.Create`, `Registry.Update`, `Registry.Get`, `Registry.List`
- `internal/task/task.go`: `Task.Result`, `Task.TokensUsed`, `Task.Cancel`, `Task.PendingMessages`
- `internal/tools/taskoutput/taskoutput.go`: `Tool.Invoke`
- `internal/tools/taskget/taskget.go`: `Tool.Invoke`
- `internal/tools/tasklist/tasklist.go`: `Tool.Invoke`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/session/writer.go`: session entry writers

What responsibility is split or misplaced:

The parent conversation records only the immediate Agent tool result with `Status: "async_launched"` and a task ID. The actual background child result is written later by `completeAgentTask` into `task.Registry`, and `TaskOutput`, `TaskGet`, and `TaskList` read that in-memory registry. `SetupDeps` creates a fresh `task.NewRegistry(bus)` for the runtime; the session writer has no task-result entry and no path that persists completed background task results.

Why this is wrong in ownership/lifecycle terms:

An async Agent result is part of the user-visible session outcome. Storing the final result only in a process-local task registry makes the task registry both a live coordination object and the only source of durable-looking result state, while the real session lifecycle records only the launch.

Observable bug or likely failure mode:

A background Agent can complete after the parent turn has persisted the launch response. If the process exits, the web/TUI runtime is restarted, or the user resumes the session later, the task ID remains in the parent conversation but `TaskOutput` returns `not_ready` or `TaskGet` reports the task missing because the registry was recreated. The final child result, token count, and error state are lost unless the user asked for them before the process died.

Minimal direction for fixing the boundary:

Make background task completion commit a session-owned result record or append a structured background-result event through the same persistence owner that records tool results. Keep the task registry as live coordination state, but do not make it the durable source of completed async work.

What not to do:

Do not serialize the entire in-memory task registry as a shutdown cleanup step. That would preserve the wrong owner and still miss crashes, resumed sessions, and child artifact/session relationships.

Status:

Resolved in current worktree. Background Agent terminal outcomes now append a session-owned `task_result` entry when the background goroutine completes or fails. The task registry remains the live coordination owner for running tasks, but completed async results, errors, token counts, run counters, and timestamps are committed through the session writer instead of living only in memory.

Source evidence:

- `internal/session/entry.go`: added `EntryTaskResult` and `TaskResultData` for durable background task outcomes.
- `internal/session/writer.go`: added `WriteTaskResult`; session rewrite preserves existing `TaskResults`.
- `internal/session/store.go` and `internal/session/session.go`: session load now reads `task_result` entries into `Session.TaskResults`.
- `internal/cli/run.go`: session rewrite carries `existing.TaskResults` forward.
- `internal/cli/tools.go`: the Agent tool receives a narrow `TaskResultWriter` callback that appends to the current session writer when available.
- `internal/tools/agent/agent.go`: `runBackground` calls `writeBackgroundTaskResult` after terminal registry update; foreground Agent and graph runs do not serialize the registry.

Verification evidence:

- Non-test verification: `gofmt -w internal/session/entry.go internal/session/session.go internal/session/writer.go internal/session/store.go internal/cli/run.go internal/cli/tools.go internal/tools/agent/agent.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/session ./internal/task ./internal/tools/agent ./internal/tools/taskoutput ./internal/tools/taskget ./internal/tools/tasklist ./internal/cli ./cmd/pragma`.
- Ownership scan: `rg -n "EntryTaskResult|TaskResultData|WriteTaskResult|TaskResults|TaskResultWriter|writeBackgroundTaskResult|completeAgentTask|failAgentTask|runBackground|task.NewRegistry|Registry\\.List|Registry\\.Get" internal/session internal/cli internal/tools/agent internal/task -g'*.go'` showed durable writes flow through `WriteTaskResult` from the background Agent completion path, not from registry shutdown serialization.

## 33. TaskUpdate Bypasses The Task State Machine And Accepts Invalid Statuses

Severity: medium

Concrete files/functions involved:

- `internal/tools/taskupdate/taskupdate.go`: `TaskUpdateInput`, `inputSchema`, `taskUpdateDescription`, `Tool.Invoke`
- `internal/task/registry.go`: `Registry.Update`, `Cancel`, `RequestShutdown`, `DeliverMessage`, `Heartbeat`, `ReapDead`
- `internal/task/task.go`: `TaskStatus`
- `internal/tools/taskoutput/taskoutput.go`: `Tool.Invoke`
- `internal/tools/tasklist/tasklist.go`: `Tool.Invoke`
- `internal/tools/agent/agent.go`: Agent-owned task lifecycle updates

What responsibility is split or misplaced:

Task lifecycle transitions are spread between the Agent runner, registry methods like `Cancel` and `RequestShutdown`, and a generic `TaskUpdate` tool that mutates `Task.Status` directly. `TaskUpdate` does not validate the status against `TaskStatus`; it casts arbitrary input with `task.TaskStatus(in.Status)`. Its schema allows `pending`, `running`, `completed`, and `failed`, while its description instructs the model to use `in_progress` and `deleted`, which are not runtime states.

Why this is wrong in ownership/lifecycle terms:

The task registry is the state-machine owner for cancellation, shutdown, heartbeat, delivery, and dead-agent detection. A tool that writes raw status strings bypasses those transition rules and lets prompt guidance define lifecycle states the runtime does not handle.

Observable bug or likely failure mode:

If the model follows the `TaskUpdate` description and sets a task to `in_progress`, the registry stores an unknown status. `DeliverMessage`, `Cancel`, and `RequestShutdown` reject it because they only treat `running` and `pending` as active; `Heartbeat` and `ReapDead` ignore it because they only recognize `running`; and `TaskOutput` never returns the terminal result because it only treats `completed`, `failed`, and `cancelled` as success states. The task can become neither active nor terminal.

Minimal direction for fixing the boundary:

Move status transitions behind registry-owned methods that validate allowed states and apply side effects consistently. Align the tool schema and description with the actual `TaskStatus` constants, or split planner task updates from agent lifecycle updates if they are different domains.

What not to do:

Do not add display-side aliases for `in_progress` or silently map unknown strings in `TaskOutput`. That hides the bad state after it has already escaped the lifecycle owner.

Status:

Resolved in current worktree. `TaskUpdate` no longer casts arbitrary status strings into `task.TaskStatus` or documents non-runtime statuses. Status validation now lives with the task domain constants, and `TaskUpdate` routes updates through a registry-owned field update method that validates status before mutating task state.

Source evidence:

- `internal/task/task.go`: added `ParseStatus`, accepting only `pending`, `running`, `completed`, `failed`, and `cancelled`.
- `internal/task/registry.go`: added `UpdateFields`, a registry-owned update boundary that validates any supplied status before applying task field changes.
- `internal/tools/taskupdate/taskupdate.go`: removed `task.TaskStatus(in.Status)` raw cast; the tool now calls `task.ParseStatus` and `Registry.UpdateFields`.
- `internal/tools/taskupdate/taskupdate.go`: schema enum and description now match actual runtime task statuses and no longer mention `in_progress` or `deleted`.

Verification evidence:

- Non-test verification: `gofmt -w internal/task/task.go internal/task/registry.go internal/tools/taskupdate/taskupdate.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/task ./internal/tools/taskupdate ./internal/tools/taskoutput ./internal/tools/tasklist ./internal/tools/agent ./cmd/pragma`.
- Ownership scan: `rg -n "TaskUpdate|UpdateFields|ParseStatus|TaskStatus\\(in\\.Status\\)|in_progress|deleted|enum|TaskPending|TaskRunning|TaskCompleted|TaskFailed|TaskCancelled" internal/task internal/tools/taskupdate internal/tools/taskoutput internal/tools/tasklist -g'*.go'` showed no raw `TaskUpdate` status cast and no non-runtime status guidance remains in the tool.

## 34. Background Permission-Waiting Status Is Driven By A Post-Decision Event

Severity: medium

Concrete files/functions involved:

- `internal/background/subscriber.go`: `StatusSubscriber.HandleEvent`, `updateStatusLocked`
- `internal/background/registry.go`: `Registry.UpdateStatus`
- `internal/background/info.go`: `StatusWaiting`, `StatusBusy`, `StatusIdle`
- `internal/tool/orchestrator.go`: `Orchestrator.Execute`, `Orchestrator.executeSingle`, `emitPermissionDecision`
- `internal/observe/event_catalog.go`: `ToolPermissionPrompted`, `PermissionDecisionFinal`, `ToolBatchStarted`, `ToolBatchCompleted`

What responsibility is split or misplaced:

The background status subscriber treats `ToolPermissionPrompted` as the start of a waiting state by setting `waiting = true`. The orchestrator emits that event only after `prompter.Prompt(...)` has already returned a user decision. The actual final permission outcome is emitted separately as `PermissionDecisionFinal`, but the status subscriber does not consume it.

Why this is wrong in ownership/lifecycle terms:

Runtime status is derived from observe events whose semantics are not owned by the status layer. A post-decision audit event is being interpreted as a live lifecycle state transition. The component tracking background process state should not have to infer "waiting" from an event that already contains the completed decision.

Observable bug or likely failure mode:

For an asked permission that the user denies, `ToolPermissionPrompted` sets `waiting = true`; the denial path returns without emitting `ToolExecutionStarted`, and `ToolBatchCompleted` decrements `activeToolBatch` without clearing `waiting`. The active-sessions PID file can remain in `waiting` even though the prompt is over and the tool batch has completed, until a later API/tool event happens to clear the flag.

Minimal direction for fixing the boundary:

Emit a real "permission prompt started" event before blocking on the prompter, and have a corresponding decision/completion event clear waiting state. Alternatively, make the status subscriber derive waiting from an explicit permission lifecycle pair rather than from audit-style decision records.

What not to do:

Do not clear `waiting` only in `ToolBatchCompleted` as a one-off repair. That would still leave the status layer guessing permission lifecycle from unrelated batch events and would be wrong for concurrent prompts or future permission flows.

Status:

Resolved in current worktree. Permission waiting is now driven by an explicit pre-prompt lifecycle event, while the existing post-prompt decision record remains audit/duration data. The background status subscriber no longer treats a completed prompt record as the beginning of the waiting state.

Source evidence:

- `internal/observe/event_catalog.go`: added `ToolPermissionPromptStarted` as a first-class observe event with the tool call ID and tool name.
- `internal/observe/event.go` and `internal/observe/logger.go`: registered `ToolPermissionPromptStarted` for event decoding, logging, and tool-topic classification.
- `internal/tool/orchestrator.go`: emits `ToolPermissionPromptStarted` immediately before `prompter.Prompt(...)`; `ToolPermissionPrompted` remains after prompt completion with user decision and prompt duration.
- `internal/background/subscriber.go`: sets `waiting = true` only on `ToolPermissionPromptStarted`; clears it on `ToolPermissionPrompted` and `PermissionDecisionFinal`.

Verification evidence:

- Non-test verification: `gofmt -w internal/observe/event_catalog.go internal/observe/event.go internal/observe/logger.go internal/tool/orchestrator.go internal/background/subscriber.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/background ./internal/tool ./internal/observe ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "ToolPermissionPromptStarted|ToolPermissionPrompted|PermissionDecisionFinal|waiting|Prompt\\(|emitPermissionDecision" internal/background/subscriber.go internal/tool/orchestrator.go internal/observe/event_catalog.go internal/observe/event.go internal/observe/logger.go -g'*.go'` confirmed prompt-start, post-prompt, and final-decision events are handled at their owning boundaries.

## 35. Subagent ToolSearch Reads The Root Registry Instead Of The Child Registry

Severity: medium

Concrete files/functions involved:

- `internal/cli/tools.go`: `RegisterTools`, `engineFactory`, `baseTools`
- `internal/tools/toolsearch/toolsearch.go`: `Tool.Registry`, `Tool.Invoke`, `formatResult`
- `internal/tool/registry.go`: `Registry.List`, `Registry.Get`, `Registry.ToolDefs`, `Registry.Scoped`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/tools/agent/agent.go`: `Tool.invoke`, `RunForked`

What responsibility is split or misplaced:

Subagent execution uses a child registry built inside `engineFactory`; the query loop exposes only that child registry through `ToolDefs`, and the orchestrator resolves calls against it. But `baseTools` constructs `ToolSearch` with `Registry: d.Registry`, the root registry. When those tool descriptors are registered into a child registry and then optionally narrowed with `Registry.Scoped`, the `ToolSearch` descriptor still searches the root registry rather than the effective child registry.

Why this is wrong in ownership/lifecycle terms:

Tool discovery is part of the same execution contract as tool invocation. A child engine's visible and callable tools should come from one effective registry. Splitting prompt-time discovery from execution-time lookup makes ToolSearch describe a different runtime than the one that will execute the model's tool calls.

Observable bug or likely failure mode:

A subagent or skill-forked run that has a scoped tool set can call `ToolSearch` and receive schema definitions for root tools that are not present in the child registry. The root registry can also include tools such as `Agent` that child registries do not register through `baseTools`. The model then has a schema that says the tool is callable, but the child orchestrator cannot resolve the call, producing unknown-tool failures or misleading tool-selection behavior.

Minimal direction for fixing the boundary:

Construct ToolSearch against the registry that will actually be used by the engine, or bind it after child registry construction so root and child engines each search their own effective registry. If a broader marketplace search is needed, return availability metadata that distinguishes discoverable from callable tools.

What not to do:

Do not add an "unknown tool" hint telling the model to try another search. The search result itself must be scoped to the execution registry, not patched after an invalid call.

Status:

Resolved in current worktree. ToolSearch is now bound to the same effective registry that supplies `ToolDefs` and backs orchestrator lookup for that engine. Root engines search the root registry, and subagent engines search the child registry after optional tool scoping has been applied.

Source evidence:

- `internal/cli/tools.go`: `baseTools` now receives an explicit `searchRegistry` instead of closing ToolSearch over `d.Registry`.
- `internal/cli/tools.go`: `engineFactory` passes the newly created child registry into `baseTools`, then calls `bindToolSearchRegistry` after `Registry.Scoped(scopedToolNames)` so ToolSearch points at the final effective child registry.
- `internal/cli/tools.go`: root registration calls `bindToolSearchRegistry(d.Registry)` after root-only tools are registered, keeping root ToolSearch aligned with the root execution registry.
- `internal/query/loop.go`: unchanged runtime contract remains that model-visible tools come from `e.registry.ToolDefs()`, now matching ToolSearch discovery for child engines.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/tools.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/tools/toolsearch ./internal/tool ./internal/query ./internal/tools/agent ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "baseTools\\(|BaseTools\\(|bindToolSearchRegistry|ToolSearch|Registry: d\\.Registry|Registry: searchRegistry|Scoped\\(scopedToolNames\\)|ToolDefs\\(" internal/cli/tools.go internal/tools/toolsearch/toolsearch.go internal/tool/registry.go internal/query/loop.go internal/tools/agent/agent.go -g'*.go'` confirmed ToolSearch is bound through `searchRegistry` and rebound after scoping.

## 36. MCP Startup Goroutines Can Mutate The Registry After Runtime Cleanup Continues

Severity: medium

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`, `compositeCleanup`
- `internal/mcp/manager.go`: `ConnectAllAndRegister`, `connectAll`, `registerClientTools`, `DisconnectAll`
- `internal/mcp/client.go`: `Client.Connect`, `Client.Disconnect`
- `internal/tool/registry.go`: `Registry.Register`, `Registry.Unregister`

What responsibility is split or misplaced:

MCP connection startup is launched from `SetupDeps` in a goroutine. Cleanup cancels `mcpCtx`, waits for the startup wait group for only 500ms, then calls `mcpManager.DisconnectAll()` and proceeds to drain the bus and run cleanup functions. The manager has no closed/stopped state; `connectAll` goroutines that finish after that point can still assign `m.clients[server]`, set status to connected, and call `registerClientTools`, which mutates the shared tool registry.

Why this is wrong in ownership/lifecycle terms:

Runtime cleanup must be the owner of MCP connection lifetime. A cleanup path that merely asks startup work to stop, then proceeds while that startup work can still publish connected clients and register tools, leaves the lifecycle split between the CLI cleanup function and manager goroutines.

Observable bug or likely failure mode:

During shutdown or early startup cancellation, a slow MCP `Connect` or `ListTools` call can outlive the 500ms cleanup wait. `DisconnectAll` may run while no client is registered yet, then the late goroutine can store a newly connected client and register MCP tools after cleanup has already drained the bus. A subsequent runtime in the same process can see stale MCP tools or statuses, and a process exit can leave transports closing outside the intended cleanup sequence.

Minimal direction for fixing the boundary:

Make `Manager` own a stopped state or lifecycle generation and have connection/register paths check it before publishing clients or tools. Cleanup should wait for manager-owned startup work to finish, or the manager should synchronously cancel and join its own workers before unregistering and disconnecting.

What not to do:

Do not increase the 500ms timeout or add another delayed `DisconnectAll`. That preserves a race between startup and cleanup instead of making MCP lifecycle transitions atomic inside the manager.

Status:

Resolved in current worktree. MCP connection and tool-registration workers now publish clients and mutate the tool registry only while their manager lifecycle generation is still active. Cleanup cancels runtime work and moves the manager into its stopped/disconnected generation before waiting on background goroutines, so late startup work cannot register stale MCP tools after cleanup has continued.

Source evidence:

- `internal/mcp/manager.go`: added manager-owned `generation` and `stopped` lifecycle state, with `activeGenerationLocked`, `publishConnectedClient`, `recordConnectFailure`, and `markDisconnectedIfActive` as the only publish/failure helpers used by async connect workers.
- `internal/mcp/manager.go`: `connectAll` captures one generation for each startup run; successful connections publish through `publishConnectedClient`, and ready-tool registration passes the same generation into `registerClientTools`.
- `internal/mcp/manager.go`: `registerClientTools`, `RegisterTools`, and `ReconnectServer` check the active generation before mutating `registeredTools` or the shared tool registry.
- `internal/mcp/manager.go`: `DisconnectAll` marks the manager stopped and advances the generation before unregistering tools and disconnecting clients.
- `internal/cli/deps.go`: cleanup now calls `mcpManager.DisconnectAll()` immediately after cancellation and before waiting on dependency goroutines, establishing the manager-owned shutdown boundary early.

Verification evidence:

- Non-test verification: `gofmt -w internal/mcp/manager.go internal/cli/deps.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/mcp ./internal/cli ./internal/tool ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "generation|stopped|activeGenerationLocked|publishConnectedClient|recordConnectFailure|markDisconnectedIfActive|registerClientTools\\(|DisconnectAll\\(|ConnectAllAndRegister|depsCancel\\(|depsWG\\.Wait|registry\\.(Register|Unregister)\\(" internal/mcp/manager.go internal/cli/deps.go -g'*.go'` confirmed async publish/register paths are generation-gated and cleanup stops the manager before waiting.

## 37. `/exit` Has UI-Specific Quit Semantics Instead Of A Runtime Session Command

Severity: medium

Concrete files/functions involved:

- `internal/slash/commands.go`: `handleExit`
- `internal/slash/command.go`: `Result.Quit`
- `internal/cli/run.go`: `InteractiveRuntime.runSlash`
- `internal/tui/handlers.go`: `handleRuntimeSlashResult`, `quit`
- `internal/web/web.go`: request run loop publishing `slash_result`, browser event rendering

What responsibility is split or misplaced:

The slash command handler for `/exit` only calls `deps.SessionSave()` and returns `Result{Quit: true}`. The TUI interprets that presentation flag by calling `m.quit()`, which cancels and closes the session. The web server receives the same `SlashResultEvent` but only publishes it as a `slash_result` envelope; the browser code renders it as "Command result" and has no shutdown or close-session behavior.

Why this is wrong in ownership/lifecycle terms:

Exiting an interactive runtime is a session/runtime lifecycle command, not a UI display hint. The command's effect depends on which presentation surface consumes the result. TUI owns the real close path; web merely displays the command result.

Observable bug or likely failure mode:

Typing `/exit` in TUI exits and closes the active session. Typing `/exit` in web saves the session but leaves the web runtime and session writer open until the HTTP server shuts down or another close path runs. Hooks, session lifecycle events, and writer close behavior therefore differ for the same slash command.

Minimal direction for fixing the boundary:

Handle quit as a runtime-level command in `InteractiveRuntime.runSlash`: after saving, close the active session and signal an explicit runtime-terminated event that UIs can render or act on. Presentation layers should not decide whether `/exit` actually exits the runtime.

What not to do:

Do not add a JavaScript branch that calls `window.close()` or hides the prompt when it sees `Quit`. That would make web imitate TUI while leaving slash command lifecycle ownership in presentation code.

Status:

Resolved in current worktree. `/exit` is now a runtime lifecycle command: the slash handler only returns quit intent, `InteractiveRuntime.runSlash` closes the active session, and presentation surfaces receive an explicit runtime termination event instead of deciding whether the session should close.

Source evidence:

- `internal/slash/commands.go`: `handleExit` no longer calls `SessionSave`; it returns `Result{Quit: true}` as command intent only.
- `internal/slash/command.go`: `Result.Quit` is documented as runtime termination intent rather than a presentation exit instruction.
- `internal/cli/run.go`: `InteractiveRuntime.runSlash` handles `result.Quit` by calling `rt.CloseSession()` before emitting `interactive.RuntimeTerminatedEvent`.
- `internal/interactive/event.go`: added `RuntimeTerminatedEvent` as the presentation-facing signal that runtime shutdown has completed.
- `internal/tui/handlers.go`: TUI no longer closes the session from `SlashResultEvent`; it exits UI on `RuntimeTerminatedEvent` and skips a second session close.
- `internal/web/web.go`: web publishes `runtime_terminated` as an explicit runtime lifecycle event instead of treating `/exit` as only a command-result display.

Verification evidence:

- Non-test verification: `gofmt -w internal/interactive/event.go internal/slash/command.go internal/slash/commands.go internal/cli/run.go internal/tui/handlers.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/interactive ./internal/slash ./internal/cli ./internal/tui ./internal/web ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "RuntimeTerminatedEvent|result\\.Quit|handleExit|CloseSession\\(\\)|finishRuntimeTermination|quitWithSessionClose|runtime_terminated|slash_result|SessionSave|Quit" internal/interactive/event.go internal/slash/command.go internal/slash/commands.go internal/cli/run.go internal/tui/handlers.go internal/web/web.go -g'*.go'` confirmed `result.Quit` is consumed by runtime and presentations react to `RuntimeTerminatedEvent`.

## 38. SessionStart Hook Blocks Are Converted Into Model Context Instead Of Stopping Session Start

Severity: high

Concrete files/functions involved:

- `internal/hook/hook.go`: `OutcomeBlock`, `AggregatedResult`
- `internal/hook/manager.go`: `Manager.Execute`
- `internal/cli/run.go`: `BuildInteractiveRuntime`, `InteractiveRuntime.RunInput`, `InteractiveRuntime.runEngine`, `InteractiveRuntime.runOrchestration`, `hookContextStrings`, `startSessionForCurrentConversation`, `beginSessionLifecycle`, `runNonInteractive`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`

What responsibility is split or misplaced:

The hook manager defines a generic blocking outcome: exit code 2 sets `AggregatedResult.Blocked`, fills `BlockMsg`, emits `HookBlocked`, and returns immediately. Prompt submission and tool execution treat that result as enforcement. `UserPromptSubmit` blocks the input before slash or engine execution, and `PreToolUse` returns an error tool result before invoking the tool. But session lifecycle treats `SessionStart` differently: `beginSessionLifecycle` emits `SessionStarted`, executes the hook, sets `d.SessionStarted = true`, and returns the result without checking `Blocked`. The interactive and non-interactive runners then feed only the hook feedback/stdout into engine context through `hookContextStrings`.

Why this is wrong in ownership/lifecycle terms:

`SessionStart` is a lifecycle gate, not model advice. The lifecycle owner is responsible for deciding whether the session is allowed to start before committing started state, emitting start events, creating durable session state, or running the engine. Instead, the block decision is dropped at the lifecycle boundary while later engine code treats the hook result as context material.

Observable bug or likely failure mode:

A configured `SessionStart` hook that exits 2 to block an unsafe workspace, missing secret, policy violation, or disallowed session launch still allows Pragma to create or reuse the session writer, emit `SessionStarted`, mark the runtime as started, append any allowed hook feedback/stdout as model context, and run the prompt in both interactive and non-interactive paths. The hook's `BlockMsg` is ignored entirely, so the user sees no enforced lifecycle stop even though the same block mechanism works for prompts and tools.

Minimal direction for fixing the boundary:

Make `SessionStart` hook execution part of the session lifecycle gate. Either execute it before committing session-start side effects, or make `beginSessionLifecycle` return an error/block result that all callers must honor before running the engine or orchestration. If the session writer must exist for hook input, then a blocked start should close or mark the writer as failed and emit a distinct failed/blocked lifecycle event instead of `SessionStarted`.

What not to do:

Do not solve this by adding a warning to the model context, web UI, or TUI output. A blocked `SessionStart` must prevent runtime execution at the lifecycle boundary, and the fix must cover interactive engine runs, orchestration runs, and non-interactive prompts together.

Status:

Resolved in current worktree. `SessionStart` is now enforced inside the session lifecycle boundary before recording, `SessionStarted`, or `SessionStarted=true` are committed. A blocked start returns an error from `beginSessionLifecycle`, so interactive engine runs, orchestration runs, resume startup, and non-interactive prompts stop before engine execution or hook context injection.

Source evidence:

- `internal/cli/run.go`: `beginSessionLifecycle` now executes `hook.SessionStart` before `startSessionRecording`, `observe.SessionStarted`, and `d.SessionStarted = true`.
- `internal/cli/run.go`: blocked `SessionStart` results call `closeUnstartedSessionWriter` and return `blockedSessionStartError`, preventing callers from appending hook output as model context.
- `internal/cli/run.go`: `runEngine`, `runOrchestration`, `runNonInteractive`, `BuildInteractiveRuntime`, and `Resume` already stop on errors from `startSessionForCurrentConversation` or `beginSessionLifecycle`, so the enforcement point covers all listed runtime paths.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/hook ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "beginSessionLifecycle|startSessionForCurrentConversation|blockedSessionStartError|closeUnstartedSessionWriter|SessionStart|SessionStarted|appendHookContext\\(hook\\.SessionStart|AppendHookContext\\(string\\(hook\\.SessionStart\\)|hookResult\\.Blocked|SessionWriter = nil" internal/cli/run.go -g'*.go'` confirmed the hook block is enforced before committed session start and all callers stop before context injection.

## 39. Hook JSON Decisions Are Parsed But Enforcement Is Owned By Exit Codes

Severity: high

Concrete files/functions involved:

- `internal/hook/hook.go`: `JSONOutput`, `AggregatedResult`, `OutcomeBlock`
- `internal/hook/executor.go`: `ExecCommand`
- `internal/hook/manager.go`: `Manager.Execute`
- `internal/hook/executor_test.go`: `TestExecCommandJSONOutput`
- `internal/hook/manager_test.go`: `TestManagerPreToolUseBlock`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/cli/run.go`: `InteractiveRuntime.RunInput`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/query/miniswe_loop.go`: `Engine.runPragmaLoopWithInitialPrompt`

What responsibility is split or misplaced:

The hook executor parses structured JSON output with semantic control fields: `decision`, `reason`, `continue`, `stopReason`, and `additionalContext`. The hook manager is the only component that aggregates hook results into the `AggregatedResult` contract that runtime callers consume. But `Manager.Execute` only turns process exit code 2 into `Blocked`; for JSON output it preserves only `additionalContext`. The actual enforcement callers, such as `Orchestrator.executeSingle` for `PreToolUse` and `InteractiveRuntime.RunInput` for `UserPromptSubmit`, can only see `AggregatedResult.Blocked`, so they enforce exit-code blocks but cannot enforce parsed JSON decisions. Stop hooks are also executed from query-loop defers while their JSON `continue`/`stopReason` fields are never surfaced into a runtime control result.

Why this is wrong in ownership/lifecycle terms:

Hook output interpretation belongs at the hook boundary, not in process-exit convention scattered through callers. The code has two hook contracts: a typed JSON schema in `hook.JSONOutput`, and an enforcement contract in `AggregatedResult`. Because the manager never translates the former into the latter, semantic policy decisions are parsed as data and then discarded before reaching the runtime/control layer.

Observable bug or likely failure mode:

A `PreToolUse` hook can emit `{"decision":"block","reason":"dangerous"}` on stdout and exit 0. `ExecCommand` parses the JSON, as covered by `TestExecCommandJSONOutput`, but `Manager.Execute` treats the outcome as `OutcomeOK`, does not set `Blocked`, and does not copy `reason` into `BlockMsg`. The orchestrator then executes the tool. Similarly, a hook that emits `{"continue":false,"stopReason":"policy stop"}` has no path to stop the query loop, because stop-control fields are absent from `AggregatedResult` and the Stop hook result is ignored by the query-loop defers.

Minimal direction for fixing the boundary:

Make `Manager.Execute` the single owner of hook decision semantics. Translate event-appropriate JSON decisions into a typed aggregate result before returning to callers: `decision:"block"` should produce the same enforcement result as an exit-code block for events that support blocking, and stop/continue fields should either be represented in the aggregate result and consumed by the engine lifecycle or removed from the supported schema. The tool, prompt, and loop callers should consume one hook-result contract instead of knowing which low-level hook convention produced it.

What not to do:

Do not add a second JSON parse in `Orchestrator.executeSingle`, `InteractiveRuntime.RunInput`, or the query loop. That would duplicate hook semantics across runtime paths and leave the manager as a passive transport for decisions it is supposed to own.

Status:

Resolved in current worktree. Hook JSON control fields are now translated once in `Manager.Execute` into the aggregate result that runtime callers already consume. Tool, prompt, and session callers still enforce `AggregatedResult.Blocked`; they do not parse hook JSON themselves. Stop-hook aggregate blocks are also consumed by the query loop through a shared stop-hook helper.

Source evidence:

- `internal/hook/manager.go`: added `applyJSONControl`, `eventSupportsDecisionBlock`, and `hookBlockMessage`; `decision:"block"` for blocking-capable events and `continue:false` now set `AggregatedResult.Blocked` and `BlockMsg`.
- `internal/hook/manager.go`: JSON-driven blocks emit `HookBlocked` through the same observe path as exit-code blocks.
- `internal/tool/orchestrator.go`: unchanged caller continues to block PreToolUse through `hookResult.Blocked`.
- `internal/cli/run.go`: unchanged prompt and session lifecycle callers continue to block UserPromptSubmit and SessionStart through `hookResult.Blocked`.
- `internal/query/loop.go` and `internal/query/miniswe_loop.go`: Stop hooks now route through `Engine.runStopHook`, which emits an `ErrorEvent` when the manager returns a blocked aggregate.

Verification evidence:

- Non-test verification: `gofmt -w internal/hook/manager.go internal/query/loop.go internal/query/miniswe_loop.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/hook ./internal/query ./internal/cli ./internal/tool ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "applyJSONControl|eventSupportsDecisionBlock|hookBlockMessage|emitHookBlocked|JSON\\.Decision|JSON\\.Continue|JSON\\.StopReason|result\\.Blocked|runStopHook|hook\\.Stop|HookMgr.Execute|json\\.Unmarshal" internal/hook internal/query internal/cli/run.go internal/tool/orchestrator.go -g'*.go'` confirmed JSON parsing remains in hook/executor and JSON control interpretation remains in hook/manager.

## 40. Interactive UIs Render Prompt Submission Before Runtime Acceptance

Severity: medium

Concrete files/functions involved:

- `internal/interactive/event.go`: `AcceptedPromptEvent`
- `internal/cli/run.go`: `InteractiveRuntime.RunInput`
- `internal/tui/handlers.go`: `Model.submitPrompt`, `Model.handleRuntimeEvent`
- `internal/web/web.go`: `server.handlePrompt`, `server.start`, `normalizeWebEvent`, browser `promptForm.onsubmit`, browser `appendEnvelope`

What responsibility is split or misplaced:

`InteractiveRuntime.RunInput` owns the runtime acceptance point for an interactive prompt: it runs `UserPromptSubmit` hooks, returns immediately on a blocked hook, and only emits `AcceptedPromptEvent` after the prompt is allowed to continue into slash handling or engine execution. The presentation surfaces do not wait for that event before showing the prompt as user-visible conversation state. TUI renders a `model.RoleUser` message in `Model.submitPrompt` before calling `m.runInput`. Web appends a local `prompt_submitted` envelope in the browser before posting to `/api/prompt`, and the server publishes `user_prompt` before reading the runtime event stream. Both surfaces also handle the later `prompt_accepted` event, but by then they have already shown the prompt.

Why this is wrong in ownership/lifecycle terms:

Prompt acceptance is a runtime decision because hooks, slash parsing, session state, and engine execution all depend on it. UI code should render the runtime's accepted-prompt event, not invent an accepted user turn ahead of the runtime. Otherwise presentation owns a domain transition that the runtime can still reject.

Observable bug or likely failure mode:

When a `UserPromptSubmit` hook blocks a prompt, `InteractiveRuntime.RunInput` emits a `query.ErrorEvent` and does not emit `AcceptedPromptEvent`. In TUI, the prompt has already been rendered as a user message and streaming state has already started; the blocked-hook error appears after a user turn that never actually entered the runtime. In web, the browser has already appended `prompt_submitted`, and the server has broadcast `user_prompt`, so the event stream shows the blocked prompt as if it were submitted even though the runtime rejected it. This also risks duplicate user entries when clients render both the optimistic/local prompt and the later `prompt_accepted`.

Minimal direction for fixing the boundary:

Make `AcceptedPromptEvent` the single source of truth for accepted user prompts in interactive surfaces. TUI should defer rendering and history updates until it receives that event. Web should remove the pre-runtime `user_prompt` broadcast and either treat `prompt_submitted` as transient local UI state outside the event log or replace it when `prompt_accepted` arrives. Rejected prompts should render as rejected/blocked attempts, not accepted conversation turns.

What not to do:

Do not add a UI-side filter that hides the later `prompt_accepted` event or suppresses blocked-hook errors. That preserves the incorrect ownership by making the presentation layer guess which prompts count instead of using the runtime acceptance event.

Status:

Resolved in current worktree. `AcceptedPromptEvent` is now the single source of visible accepted prompt state for interactive surfaces. TUI still starts the runtime immediately, but it renders and remembers the user prompt only after runtime acceptance. Web no longer appends or broadcasts pre-runtime prompt events into the event log; it renders user turns from `prompt_accepted`.

Source evidence:

- `internal/tui/handlers.go`: `submitPrompt` no longer creates and renders a `model.RoleUser` message before calling `runInput`.
- `internal/tui/handlers.go`: `AcceptedPromptEvent` now calls `renderAcceptedPrompt`, which renders the user message and records input history after runtime acceptance.
- `internal/web/web.go`: removed the server-side `user_prompt` broadcast before reading the runtime event stream.
- `internal/web/web.go`: removed the browser-side local `prompt_submitted` event-log append before `/api/prompt` returns.
- `internal/web/web.go`: active prompt titles and compact text now treat only `prompt_accepted` as the user prompt event.

Verification evidence:

- Non-test verification: `gofmt -w internal/tui/handlers.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/tui ./internal/web ./internal/interactive ./internal/cli ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "AcceptedPromptEvent|renderAcceptedPrompt|prompt_accepted|prompt_submitted|user_prompt|RoleUser|runInput\\(|appendEnvelope|hub\\.publish\\(\\\"user_prompt\\\"|input\\.remember|submitPrompt|streaming = true" internal/tui/handlers.go internal/web/web.go internal/cli/run.go internal/interactive/event.go -g'*.go'` confirmed prompt rendering/history now flows from `AcceptedPromptEvent`.

## 41. UserPromptSubmit Hooks Only Gate Interactive Runtime Prompts

Severity: high

Concrete files/functions involved:

- `internal/hook/hook.go`: `HookInput.PromptText`, `UserPromptSubmit`
- `internal/cli/run.go`: `RunDispatcher`, `RunBackground`, `InteractiveRuntime.RunInput`, `RunNonInteractive`, `runNonInteractive`
- `internal/cli/subcommands.go`: `RunPromptCommand`
- `internal/tui/handlers.go`: `Model.submitPrompt`

What responsibility is split or misplaced:

Prompt-submission policy is implemented inside `InteractiveRuntime.RunInput`: it executes `UserPromptSubmit`, blocks on `AggregatedResult.Blocked`, and only then emits `AcceptedPromptEvent` or enters slash/engine execution. Non-interactive prompt execution does not use that runtime entrypoint. `RunDispatcher` sends `--prompt` directly to `RunNonInteractive`, background mode spawns a child that runs the same non-interactive path, and prompt-type slash subcommands call `runNonInteractive` through `RunPromptCommand`. In that path, `runNonInteractive` starts the session and calls `engine.Run(ctx, prompt)` without executing `UserPromptSubmit` at all. The TUI has a fallback hook check only when `m.runInput == nil`, which is not the normal runtime path.

Why this is wrong in ownership/lifecycle terms:

Submitting a prompt is one domain event regardless of whether it came from web, TUI, `--prompt`, background mode, or a prompt-type slash subcommand. The hook contract even carries `PromptText` for `UserPromptSubmit`, but the enforcement point lives in the interactive adapter instead of the shared prompt-execution boundary. That splits prompt policy by entrypoint instead of by the runtime event being controlled.

Observable bug or likely failure mode:

A `UserPromptSubmit` hook that blocks or augments dangerous prompts works in web/TUI interactive runs but is skipped by `pragma --prompt "..."`, `pragma --bg --prompt "..."`, and CLI slash subcommands whose handlers return `InjectPrompt`. Those prompts still start sessions, execute the model, and can run tools. This contradicts `RunPromptCommand`'s comment that hooks fire in all modes, and it produces a policy gap exactly in automation/background paths where prompt gating is most likely to matter.

Minimal direction for fixing the boundary:

Move prompt submission handling to a shared runtime function used by both interactive and non-interactive entrypoints before session start or engine execution. That function should execute `UserPromptSubmit`, return a blocked/error result consistently, and expose accepted prompt context to both `InteractiveRuntime.runEngine` and `runNonInteractive`. Prompt-type slash subcommands should pass through the same gate for the actual injected prompt they ask the engine to run.

What not to do:

Do not copy the interactive hook block into `RunNonInteractive`, `RunBackground`, and `RunPromptCommand` separately. That would preserve entrypoint-specific prompt gates and keep future prompt lifecycle changes duplicated across modes.

Status:

Resolved in current worktree. Prompt submission policy is now owned by a shared CLI runtime gate instead of the interactive adapter or TUI presentation. Interactive, slash-injected, orchestration, non-interactive, background child, and prompt-type slash subcommand prompts all pass through `acceptPromptSubmission` before session start or engine/orchestration execution.

Source evidence:

- `internal/cli/run.go`: added `acceptPromptSubmission`, the single runtime owner for `hook.UserPromptSubmit`; it executes the hook with `HookInput.PromptText`, returns a blocked error on `AggregatedResult.Blocked`, and returns hook context for accepted prompts.
- `internal/cli/run.go`: `InteractiveRuntime.RunInput` now calls `acceptPromptSubmission` instead of owning `UserPromptSubmit` directly.
- `internal/cli/run.go`: slash-injected prompts and `/orchestrate` task prompts are gated with `acceptPromptSubmission` before `runEngine` or `runOrchestration`.
- `internal/cli/run.go`: `runNonInteractive` gates the final resolved prompt with `acceptPromptSubmission` before `startSessionForCurrentConversation` and appends accepted `UserPromptSubmit` context before `engine.Run`.
- `internal/cli/subcommands.go`: prompt-type slash subcommands still feed their final `InjectPrompt` through `runNonInteractive`, so they inherit the shared prompt gate.
- `internal/tui/handlers.go` and `internal/tui/model.go`: removed the TUI-local fallback hook check and hook-manager field, leaving presentation dependent on the runtime `RunInput` boundary.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go internal/tui/handlers.go internal/tui/model.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/tui ./internal/web ./internal/interactive ./internal/hook ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "acceptPromptSubmission|promptBlockedError|UserPromptSubmit|HookMgr|hookMgr|AcceptedPromptEvent|AppendHookContext\\(string\\(hook\\.UserPromptSubmit\\)|appendHookContext\\(hook\\.UserPromptSubmit|RunPromptCommand|runNonInteractive|RunBackground|RunNonInteractive|HookInput\\{[^}]*PromptText|HookMgr.Execute\\(.*UserPromptSubmit" internal/cli internal/tui internal/web internal/interactive -g'*.go'` confirmed `UserPromptSubmit` execution is centralized in `acceptPromptSubmission`, with TUI/web only consuming runtime events.

## 42. Resumed Interactive Sessions Fire SessionStart Before Any Engine Turn Can Consume Hook Context

Severity: medium

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`
- `internal/cli/run.go`: `BuildInteractiveRuntime`, `InteractiveRuntime.Resume`, `InteractiveRuntime.runEngine`, `startSessionForCurrentConversation`, `beginSessionLifecycle`, `runNonInteractive`
- `internal/query/engine.go`: `Engine.AppendHookContext`
- `internal/session/store.go`: `Store.Open`

What responsibility is split or misplaced:

`SessionStart` hook execution returns context that prompt execution is expected to append into the engine through `Engine.AppendHookContext`. For new interactive sessions, `InteractiveRuntime.runEngine` calls `startSessionForCurrentConversation`, receives the `SessionStart` hook result, and appends it before running the engine. Non-interactive resumed sessions do the same in `runNonInteractive`. But interactive resumed sessions start lifecycle earlier. `SetupDeps` opens a session writer when `--resume` is set; then `BuildInteractiveRuntime` sees `d.SessionWriter != nil` and calls `beginSessionLifecycle` during runtime construction, ignoring the returned hook result. Runtime `/resume` does the same in `InteractiveRuntime.Resume`: it opens the writer, resets state, calls `beginSessionLifecycle`, and ignores the result. When the next prompt later reaches `runEngine`, `startSessionForCurrentConversation` sees `d.SessionStarted == true`, returns an empty hook result, and no `SessionStart` context is appended.

Why this is wrong in ownership/lifecycle terms:

Session-start hook output is part of the next engine-turn context contract, but resumed interactive lifecycle starts are owned by setup/resume mechanics before there is an engine turn to receive that context. The hook execution point and the hook consumption point are split across different lifecycle phases, so resume mode changes hook semantics.

Observable bug or likely failure mode:

A `SessionStart` hook that emits `additionalContext` or stdout will affect a fresh interactive prompt and a non-interactive `--resume --prompt` run, but the same hook output is lost for interactive `--resume` startup and for `/resume` followed by a user prompt. Any resume-specific policy note, environment diagnostic, or context injection from the hook silently disappears in the interactive path.

Minimal direction for fixing the boundary:

Make the interactive resume path either defer `SessionStart` until the first accepted post-resume prompt, or store the returned hook result on the runtime/session state and append it exactly once when the next engine or orchestration run begins. The same session-start result handling should be used for fresh, resumed, interactive, and non-interactive sessions.

What not to do:

Do not rerun `SessionStart` on the next prompt just to recover the missing context. That would duplicate lifecycle hooks and could repeat external side effects. The single lifecycle event needs a single owner and a durable handoff to the engine context consumer.

Status:

Resolved in current worktree. Interactive resumed sessions now preserve the single `SessionStart` hook result until the next engine or orchestration run appends it to model context. The hook is not rerun; the runtime stores the returned result from the lifecycle start and consumes it exactly once after successful context append.

Source evidence:

- `internal/cli/run.go`: `InteractiveRuntime` now has `pendingSessionStartHook` and `hasPendingSessionStartHook` runtime state for a lifecycle-start result that occurs before a turn can consume it.
- `internal/cli/run.go`: `BuildInteractiveRuntime` captures the `beginSessionLifecycle` result when startup opened a resumed `SessionWriter` and stores it on the runtime instead of discarding it.
- `internal/cli/run.go`: `InteractiveRuntime.Resume` captures the `beginSessionLifecycle` result for `/resume` and stores it on the runtime instead of discarding it.
- `internal/cli/run.go`: `runEngine` and `runOrchestration` use `sessionStartHookForNextRun`, append `hook.SessionStart` context through the existing engine context owner, then clear pending context exactly once after a successful append.
- `internal/cli/run.go`: `closeCurrentSession` clears pending `SessionStart` context so an abandoned resumed session cannot leak hook output into a later session.
- `internal/cli/run.go`: non-interactive runs still use `startSessionForCurrentConversation` and append the returned `SessionStart` context directly, preserving the existing non-interactive path.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/cli ./internal/query ./internal/session ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "pendingSessionStartHook|hasPendingSessionStartHook|sessionStartHookForNextRun|clearPendingSessionStartHook|beginSessionLifecycle|startSessionForCurrentConversation|appendHookContext\\(hook\\.SessionStart|AppendHookContext\\(string\\(hook\\.SessionStart\\)|SessionWriter != nil|Resume\\(" internal/cli/run.go internal/cli/deps.go internal/query/engine.go -g'*.go'` confirmed early interactive lifecycle starts store one hook result and turn execution consumes it through the same engine context append path.

## 43. Web Resume Can Mutate Runtime Session State While A Turn Is Running

Severity: high

Concrete files/functions involved:

- `internal/web/web.go`: `server.start`, `server.handlePrompt`, `server.handleResume`, `server.resume`, browser `openSession`
- `internal/cli/run.go`: `InteractiveRuntime.runEngine`, `InteractiveRuntime.Resume`, `makeSessionSaveClose`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/app/store.go`: `StateStore`

What responsibility is split or misplaced:

The web server serializes prompt execution through `server.start`: `/api/prompt` refuses new prompts while `s.running` is true, and the active run owns `ctx`, `cancel`, event streaming, and session persistence. `/api/resume` bypasses that runtime gate. `handleResume` publishes `resume_requested` and calls `s.resume(sessionID)` without checking `s.running`; the browser `openSession` also posts to `/api/resume` without checking `state.running`. The resume callback then calls `InteractiveRuntime.Resume`, which closes the current session writer, opens another writer, resets `SessionLastIdx`, swaps provider/model state, replaces the shared conversation in `StateStore`, resets engine file/content-replacement state, rebuilds `rt.sessionSave`, and fires `SessionStart`.

Why this is wrong in ownership/lifecycle terms:

Resuming a session is a runtime lifecycle transition. It cannot safely be owned by a web route that runs independently of the active turn lifecycle. The active engine loop and session persistence path assume the current store, engine state, and session writer remain the same for the duration of the run. Web resume mutates those shared runtime objects from the side while the run goroutine still owns them.

Observable bug or likely failure mode:

While a web prompt is streaming, a user can open a saved session or call `/api/resume` directly. The active `runEngine` loop continues ranging over `rt.Engine.Run(ctx, input)` and calls `persistSessionAfterLoopEvent(ev, rt.sessionSave)` for each event. But `Resume` may already have replaced `rt.sessionSave`, `d.SessionWriter`, `d.SessionLastIdx`, and the conversation store. The old turn can then append assistant/tool events into the resumed conversation, persist data to the wrong session file, or compute request snapshots from a conversation that no longer matches the prompt that started the run.

Minimal direction for fixing the boundary:

Route resume through the same interactive runtime lifecycle gate as prompt execution. Either reject resume while a turn is running, or cancel and fully drain/close the active run before swapping session state. The guard must live at the runtime/web boundary that owns `s.running` and `rt.Resume`, not only in the browser.

What not to do:

Do not just disable the session buttons in JavaScript. Direct `/api/resume` calls and stale browser state would still mutate the runtime while a turn is active. The server/runtime transition must enforce the lifecycle rule.

Status:

Resolved in current worktree. Web resume now uses the same server-owned runtime transition gate as prompt execution. The server marks the runtime busy before calling `InteractiveRuntime.Resume`, rejects direct `/api/resume` calls with HTTP 409 while a prompt/resume transition is active, and releases the gate with the same `run_idle` event used by prompt runs.

Source evidence:

- `internal/web/web.go`: added `beginRuntimeTransition`, the single server-side gate for web runtime transitions that mutate shared runtime/session state.
- `internal/web/web.go`: `server.start` now uses `beginRuntimeTransition(cancel)` instead of owning its own `s.running`/`s.cancel` mutation path.
- `internal/web/web.go`: `server.resume` now uses `beginRuntimeTransition(nil)` before publishing `resume_requested` or calling `s.cfg.Resume(sessionID)`, so resume cannot overlap an active prompt run.
- `internal/web/web.go`: `handleResume` maps `errRuntimeBusy` to HTTP 409, enforcing the rule for direct API callers.
- `internal/web/web.go`: browser `openSession` has a convenience `state.running` guard, but server-side `beginRuntimeTransition` is the authoritative enforcement.

Verification evidence:

- Non-test verification: `gofmt -w internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/web ./internal/cli ./internal/query ./internal/app ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "errRuntimeBusy|beginRuntimeTransition|handleResume|resume\\(|start\\(|handlePrompt|handleCancel|cancelRun|s\\.running|s\\.cancel|run_idle|resume_requested|session_resumed|openSession|state\\.running|Resume\\(" internal/web/web.go internal/cli/run.go -g'*.go'` confirmed prompt and resume share the server runtime gate, with resume rejected before `InteractiveRuntime.Resume` while busy.

## 44. AskUserQuestion Waiting State Lives Only In UI Bridges

Severity: medium

Concrete files/functions involved:

- `internal/tools/ask/ask.go`: `Tool.Invoke`, `Tool.CheckPerm`, `Tool.Flags`, `Tool.Name`
- `internal/tool/asker.go`: `Asker`, `AskRequest`, `AskResponse`
- `internal/web/web.go`: `Bridge.Ask`, `server.handleAsk`
- `internal/tui/asker.go`: `InteractiveAsker.Ask`, `NonInteractiveAsker.Ask`
- `internal/tui/handlers.go`: `handleAskRequest`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/observe/event_catalog.go`: `ToolPermissionPrompted`, `ToolExecutionStarted`, `ToolExecutionCompleted`
- `internal/background/subscriber.go`: `StatusSubscriber.HandleEvent`

What responsibility is split or misplaced:

`AskUserQuestion` is a runtime tool pause that requires user input before the engine can continue. The tool's `Invoke` method parses the model request and blocks on `t.Asker.Ask(ctx, req)`. In web mode, that wait is represented by `Bridge.Ask` publishing an `ask_request` hub envelope and waiting on a local channel until `/api/ask/{id}` resolves it. In TUI mode, `InteractiveAsker.Ask` sends an `AskRequestMsg` directly to Bubble Tea and waits on a local response channel while `handleAskRequest` opens the dialog and sets the toolbar to "waiting for answer...". The shared orchestration and observer layer only sees a tool execution start before `desc.Invoke` and a completion after the answer returns. There is no observe event equivalent to `ToolPermissionPrompted` for "the runtime is waiting on a user question."

Why this is wrong in ownership/lifecycle terms:

Waiting for a user answer is domain/runtime state, not just a UI transport detail. The engine is suspended until an external user decision arrives, and that suspension affects session lifecycle, status, tracing, cancellation, and recovery. By making the wait visible only to web hub envelopes or TUI messages, the runtime boundary cannot distinguish "the tool is actively executing" from "the run is blocked on a user answer." Permission prompts at least have an observer-level event, even though issue 34 covers that event's incorrect timing; ask prompts have no shared event at all.

Observable bug or likely failure mode:

During an interactive web or TUI run, `AskUserQuestion` can block indefinitely while background/session observers and self-trace consumers see only a running tool. `StatusSubscriber` updates status from tool batches and tool execution events, and it has a permission-specific waiting path, but there is no ask-specific waiting path to consume. A browser can show an ask modal from the web hub and the TUI can show an ask dialog from `AskRequestMsg`, while active-session status remains busy instead of waiting for user input. If the web page reloads or the TUI state loses the pending dialog, the runtime still has a goroutine waiting on a local channel with no observer-level prompt record to recover, display, or cancel consistently.

Minimal direction for fixing the boundary:

Emit shared runtime observe events for ask prompt requested, resolved, and cancelled at the asker/orchestrator boundary. Those events should include the session, trace/tool call identity, question id, options, and enough prompt metadata for background status, selftrace, and UIs to render the same waiting state. Web and TUI adapters can still provide the response transport, but they should not be the only owners of the fact that the runtime is waiting on the user.

What not to do:

Do not teach `StatusSubscriber` to inspect web hub envelopes, TUI `AskRequestMsg` values, or the string name `AskUserQuestion`. Do not infer ask waiting from long tool duration. That would keep the runtime state hidden behind presentation-specific transports and make cancellation/recovery behavior depend on UI implementation details.

Status:

Resolved in current worktree. `AskUserQuestion` now publishes ask prompt requested, resolved, and cancelled events from the runtime tool invocation path before and after the blocking `Asker.Ask` call. Background status consumes those shared observe events for waiting state, while web hub `ask_request` envelopes and TUI `AskRequestMsg` remain response/display transports.

Source evidence:

- `internal/tool/tool.go`: added `InvocationContext` helpers so the orchestrator can pass trace, span, parent span, tool call ID, and tool name through the shared tool invocation contract.
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle` wraps `desc.Invoke` with `WithInvocationContext`, making ask prompt events carry the same runtime tool identity as `ToolExecutionStarted`.
- `internal/tools/ask/ask.go`: `Tool.Invoke` now emits `AskPromptRequested` before blocking on `t.Asker.Ask`, emits `AskPromptResolved` after an answer, and emits `AskPromptCancelled` on ask errors or cancellation.
- `internal/observe/event_catalog.go` and `internal/observe/event.go`: added serializable observe events for `AskPromptRequested`, `AskPromptResolved`, and `AskPromptCancelled`, including session, tool call, prompt ID, question, option, answer count, duration, and cancellation error metadata.
- `internal/background/subscriber.go`: `StatusSubscriber.HandleEvent` now sets status waiting on `AskPromptRequested` and clears waiting on `AskPromptResolved` or `AskPromptCancelled`, without inspecting web envelopes, TUI messages, tool names, or durations.
- `internal/cli/tools.go`: the registered ask tool receives the shared event bus, so prompt lifecycle emission is tied to the runtime tool registration rather than UI bridge code.

Verification evidence:

- Non-test verification: `gofmt -w internal/tool/tool.go internal/tool/orchestrator.go internal/tools/ask/ask.go internal/cli/tools.go internal/observe/event_catalog.go internal/observe/event.go internal/observe/logger.go internal/background/subscriber.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/tool ./internal/tools/ask ./internal/cli ./internal/observe ./internal/background ./internal/web ./internal/tui ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "AskPromptRequested|AskPromptResolved|AskPromptCancelled|WithInvocationContext|InvocationContextFrom|AskUserQuestion|waiting|Asker\\.Ask|ToolExecutionStarted|Bridge\\.Ask|AskRequestMsg" internal/tool internal/tools/ask internal/cli/tools.go internal/observe internal/background/subscriber.go internal/web/web.go internal/tui -g'*.go'` confirmed shared ask wait ownership is in observe/background, while web/TUI references remain prompt transport and display paths.

## 45. Orchestration Handoff Artifact Access Is Owned By A Volatile Web Allowlist

Severity: medium

Concrete files/functions involved:

- `internal/orchestration/runner.go`: `RunFileEventsWithOptions`, `RunNodeEvents`, `selectedHandoffPrompt`, `handoffPromptPath`, `emitQueryObserve`
- `internal/cli/run.go`: `InteractiveRuntime.runOrchestration`, `interactiveOrchestrationArtifactRoot`, `makeSessionSaveClose`
- `internal/web/web.go`: `server.start`, `server.rememberArtifact`, `server.handleArtifact`, browser `handoffRow`, browser `loadArtifact`
- `internal/query/event.go`: `OrchestrationHandoffEvent`
- `internal/session/writer.go`: `Writer.WriteHandoffState`

What responsibility is split or misplaced:

Orchestration handoff files are runtime artifacts. The orchestration runner emits `OrchestrationHandoffEvent` values with concrete handoff file paths, and interactive orchestration places those files under a per-session/per-run artifact root. But the only access index for the web UI lives in `server.artifacts`: `server.start` watches loop events, calls `rememberArtifact` for each `OrchestrationHandoffEvent`, and `handleArtifact` will read a file only if that exact path was observed by this web server process. Session persistence writes conversation messages, handoff state, file state, todos, and metadata, but it does not persist an orchestration artifact manifest or the handoff event list. The browser then renders handoff rows from the web-maintained workflow snapshot and fetches `/api/artifact?path=...` through that volatile allowlist.

Why this is wrong in ownership/lifecycle terms:

The runtime creates a user-visible artifact path and later uses the same path as part of orchestration control flow, but durability and access are delegated to presentation state. A web route should be able to render or serve artifacts from session/runtime metadata; it should not be the only component that remembers which runtime artifacts exist. The artifact's lifecycle belongs with the orchestration run/session that created it, not with the current browser server's in-memory event history.

Observable bug or likely failure mode:

After a web server restart, browser reload, or session resume, handoff files can still exist on disk under the orchestration artifact root, but `server.artifacts` is empty and `handleArtifact` rejects the path with "artifact path was not emitted by this run." The saved session can restore `HandoffState`, messages, todos, and file state, but the orchestration handoff file list and access rights are gone because they were never persisted as session artifact metadata. The UI therefore loses access to user-visible handoff artifacts even though the runtime produced them and the files may still be present.

Minimal direction for fixing the boundary:

Record orchestration artifact metadata at the runtime/session boundary when `OrchestrationHandoffEvent` is emitted or when the orchestration runner creates the run artifact root. Persist the artifact root and handoff manifest with the session, and have web/TUI render and serve artifacts from that session-owned metadata. The web server can still enforce that requested files belong to the session artifact manifest, but it should not be the source of truth for whether an artifact exists.

What not to do:

Do not make `handleArtifact` fall back to arbitrary path reads when the allowlist misses. Do not repopulate `server.artifacts` by scanning `~/.pragma/orchestrations` from the web route. Both approaches keep artifact ownership in presentation code and either weaken the file boundary or rebuild runtime state from filesystem guesses.

Status:

Resolved in current worktree. Orchestration handoff artifacts are now recorded in shared app/session state when the runtime receives an `OrchestrationHandoffEvent`, persisted as session JSONL metadata, restored on resume/startup resume, and served by web only when the requested path appears in that session-owned manifest. The web server no longer owns artifact existence through a volatile `server.artifacts` allowlist.

Source evidence:

- `internal/app/state.go` and `internal/app/store.go`: added `OrchestrationArtifact` and `AppState.OrchestrationArtifacts`, with snapshot copying so the runtime store owns the artifact manifest.
- `internal/cli/run.go`: `InteractiveRuntime.runOrchestration` computes the run artifact root once, records each non-empty handoff path through `recordOrchestrationArtifact`, and writes the updated manifest through the session writer when the event is emitted.
- `internal/session/entry.go`, `internal/session/writer.go`, `internal/session/store.go`, and `internal/session/session.go`: added `orchestration_artifacts` session entries, load/rewrite support, and `Session.OrchestrationArtifacts` so artifact metadata survives close, compaction rewrite, startup resume, and explicit resume.
- `internal/cli/deps.go` and `internal/cli/run.go`: startup `--resume` and `InteractiveRuntime.Resume` hydrate `AppState.OrchestrationArtifacts` from the loaded session.
- `internal/web/web.go`: removed `server.artifacts` and `rememberArtifact`; `handleArtifact` now calls `sessionOwnsArtifact`, which validates the cleaned path against `cfg.Store.Snapshot().OrchestrationArtifacts`.
- `internal/web/web.go`: browser `workflowView` merges persisted `app_state.orchestration_artifacts` with live workflow snapshot handoffs, so resumed sessions can render and open durable handoff artifact rows.

Verification evidence:

- Non-test verification: `gofmt -w internal/app/state.go internal/app/store.go internal/session/entry.go internal/session/session.go internal/session/store.go internal/session/writer.go internal/cli/run.go internal/cli/deps.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/app ./internal/session ./internal/cli ./internal/web ./internal/orchestration ./internal/query ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "OrchestrationArtifacts|orchestration_artifacts|OrchestrationHandoffEvent|WriteOrchestrationArtifacts|EntryOrchestrationArtifacts|sessionOwnsArtifact|rememberArtifact|artifacts map|handleArtifact|workflowView|handoffs|server\\.artifacts" internal/app internal/session internal/cli/run.go internal/cli/deps.go internal/web/web.go internal/orchestration internal/query -g'*.go'` confirmed the durable manifest lives in app/session/runtime state, with no web-owned artifact allowlist remaining.

## 46. Standalone Orchestration Run Bypasses Prompt And Session Lifecycle

Severity: high

Concrete files/functions involved:

- `cmd/pragma/orchestration.go`: `runOrchestration`, `printOrchestrationEvents`
- `internal/cli/run.go`: `InteractiveRuntime.runOrchestration`, `runNonInteractive`, `startSessionForCurrentConversation`, `persistSessionAfterLoopEvent`, `makeSessionSaveClose`
- `internal/orchestration/runner.go`: `RunEventsWithOptions`, `RunStateEvents`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/cli/flags.go`: `RegisterFlags`

What responsibility is split or misplaced:

`pragma orchestration run` is a prompt-driven runtime execution path, but it is implemented as a standalone Cobra command that builds dependencies, registers tools, creates an engine, runs `orchestration.RunEventsWithOptions`, and prints loop events directly. It does not go through `InteractiveRuntime.runOrchestration`, `runNonInteractive`, or any shared prompt execution boundary. The interactive orchestration path starts the session, appends `SessionStart` and `UserPromptSubmit` hook context, writes prompt history, streams loop events, and persists after each relevant event. The root non-interactive prompt path also starts session lifecycle and persists loop progress through `makeSessionSaveClose`. The standalone orchestration subcommand does none of that.

Why this is wrong in ownership/lifecycle terms:

Executing a task prompt through the model and tools is runtime behavior, not command-printing behavior. The command layer should select an orchestration definition and output transport; it should not own engine setup, permission defaults, event consumption, prompt/session lifecycle, and final result handling. This creates a third orchestration execution path with different lifecycle semantics from both interactive `/orchestrate` and root `--prompt` execution.

Observable bug or likely failure mode:

`pragma orchestration run workflow.yaml --prompt "..."` can run model turns and tools without a `SessionStart` lifecycle, without `UserPromptSubmit` gating, without prompt history, without session save/resume metadata, and without `SessionEnd` lifecycle. The run may mutate the workspace and emit orchestration/tool events, but there is no saved session record comparable to an interactive `/orchestrate` run. A hook, persistence, or observability fix made in `InteractiveRuntime.runOrchestration` or `runNonInteractive` can therefore miss the standalone command, even though the command accepts the same kind of user task prompt and drives the same orchestration runner.

Minimal direction for fixing the boundary:

Route standalone orchestration execution through a shared runtime orchestration entrypoint that owns prompt acceptance, session lifecycle, hook handling, event streaming, and persistence. The Cobra command should load/validate the orchestration definition and adapt the shared event/result stream to stdout/stderr. If a truly sessionless orchestration mode is required, make that an explicit runtime mode with documented lifecycle events, not an accidental consequence of the subcommand owning the loop.

What not to do:

Do not copy `startSessionForCurrentConversation`, hook handling, prompt-history writes, and `persistSessionAfterLoopEvent` into `cmd/pragma/orchestration.go`. That would preserve the duplicated execution path and make future lifecycle fixes depend on keeping another command-local loop in sync.

Status:

Resolved in current worktree. `pragma orchestration run` now delegates to a CLI/runtime orchestration entrypoint that builds the shared interactive runtime, accepts the prompt through the normal hook boundary, runs `InteractiveRuntime.runOrchestration`, and only adapts the resulting event stream to stdout/stderr. The Cobra command no longer owns dependency setup, tool registration, compaction, engine construction, or direct orchestration runner execution.

Source evidence:

- `internal/cli/run.go`: added `BuildInteractiveRuntimeWithOptions`, allowing command-specific dependency configuration before tool/orchestrator registration while preserving the existing `BuildInteractiveRuntime` call contract.
- `internal/cli/run.go`: added `RunStandaloneOrchestration`, which configures non-interactive prompt/ask transports, applies the standalone command's default bypass permission policy before registration, calls `acceptPromptSubmission`, and routes execution through `InteractiveRuntime.runOrchestration`.
- `internal/cli/run.go`: standalone orchestration output is handled by `consumeStandaloneOrchestrationEvents` and `printStandaloneOrchestrationLoopEvent`, which adapt shared runtime events to stdout/stderr without owning lifecycle or orchestration control flow.
- `cmd/pragma/orchestration.go`: `runOrchestration` now reads only `--prompt`, `--persona-dir`, and the definition path, then calls `cli.RunStandaloneOrchestration`; direct calls to `cli.SetupDeps`, `cli.RegisterTools`, `cli.BuildCompactionDeps`, and `orchestration.RunEventsWithOptions` were removed.

Verification evidence:

- Non-test verification: `gofmt -w internal/cli/run.go cmd/pragma/orchestration.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./cmd/pragma ./internal/cli ./internal/orchestration ./internal/query ./internal/session ./internal/web`; `git diff --check`.
- Ownership scan: `rg -n "RunStandaloneOrchestration|StandaloneOrchestrationOptions|BuildInteractiveRuntimeWithOptions|runOrchestration\\(|RunEventsWithOptions|RunFileEventsWithOptions|SetupDeps\\(|RegisterTools\\(|BuildCompactionDeps\\(|startSessionForCurrentConversation|acceptPromptSubmission|sessionSave|printStandaloneOrchestrationLoopEvent|printOrchestrationEvents" cmd/pragma/orchestration.go internal/cli/run.go internal/orchestration -g'*.go'` confirmed the command delegates to `internal/cli`, with orchestration runner and lifecycle ownership centralized under the shared runtime path.

## 47. Web Session List Uses Slash Resume Candidates As Its API Contract

Severity: medium

Concrete files/functions involved:

- `internal/web/web.go`: `server.handleSessions`, browser `renderSessions`, `sessionTitle`, `sessionMeta`, `openSession`
- `internal/slash/resume_cmd.go`: `ResumeCandidate`, `ResumeCandidatesResult`, `BrowseResumeCandidates`, `handleResume`
- `internal/session/store.go`: `Store.List`, `readJSONLSummary`
- `internal/session/session.go`: `SessionSummary`
- `internal/cli/run.go`: `InteractiveRuntime.Resume`

What responsibility is split or misplaced:

Saved-session browsing is a runtime/session-store concern, but the web API delegates it to the slash command layer. `server.handleSessions` calls `slash.BrowseResumeCandidates(s.cfg.SlashDeps)` and returns `ResumeCandidate` values directly as JSON. That type is shaped for `/resume`: it filters to current-directory sessions when any exist, falls back to all sessions otherwise, carries `InCurrentWorkDir`, and omits fields present in `session.SessionSummary` such as `Model`, `Provider`, and `CostUSD`. The browser session sidebar then treats this slash-command result as its API contract, while `sessionMeta` tries to render `session.model` even though `ResumeCandidate` never supplies it.

Why this is wrong in ownership/lifecycle terms:

The slash command should implement `/resume` command behavior: argument matching, ambiguity messages, picker intent, and display text. It should not define the web API shape for saved sessions. Session summaries already belong to `session.Store.List`; web should adapt that session-owned data to HTTP. By using slash resume candidates as a shared backend, command-specific filtering and field selection become hidden policy for the browser.

Observable bug or likely failure mode:

The web sidebar cannot show the model even though session summaries include it, because `/api/sessions` returns `ResumeCandidate` without a model field and the browser's `sessionMeta` reads `session.model`. The API also silently drops all other workdir sessions whenever any current-workdir sessions exist, because that is the slash picker policy. A future change to `/resume` matching or picker fields can unintentionally change the web session list, and a web metadata fix has to edit slash-command data types rather than the session API.

Minimal direction for fixing the boundary:

Have the web route read `session.Store.List` or a runtime/session-owned browse function that returns a web/API session summary with the required metadata and explicit scope/filter parameters. Keep `BrowseResumeCandidates` as slash-command behavior, or make it consume the shared session summary instead of being the shared contract itself. Resume execution should still flow through `InteractiveRuntime.Resume`; only the browse/list contract needs to move out of slash command ownership.

What not to do:

Do not add `Model` and `Provider` fields to `slash.ResumeCandidate` just to satisfy the web sidebar. That keeps the command-layer picker type as the HTTP API and invites the next web/session metadata requirement to grow the slash command contract again.

Status:

Resolved in current worktree. The web session list API now reads session summaries from the session store and returns a web-owned summary DTO with model, provider, cost, timestamps, turn count, workdir, and an explicit current-workdir marker. Slash `ResumeCandidate` remains the `/resume` command picker contract and is no longer the HTTP response type for `/api/sessions`.

Source evidence:

- `internal/web/web.go`: `handleSessions` now handles `GET /api/sessions` through `session.Store.List` via `web.Config.SessionStore`, falling back to `session.NewStore` only when no store was supplied.
- `internal/web/web.go`: added `webSessionSummary` and `webSessionSummaries`, preserving all session-owned metadata needed by the browser and marking `InCurrentWorkDir` without filtering away other sessions.
- `internal/cli/run.go`: `RunInteractive` passes the runtime session store into `web.Config`, so web does not reach through slash deps for browsing.
- `internal/web/web.go`: browser `sessionMeta` now renders `model`, `provider`, `cost_usd`, `in_current_work_dir`, and `updated_at` from the web session summary contract.
- `internal/slash/resume_cmd.go`: `BrowseResumeCandidates` remains unchanged and slash-owned; web no longer calls it from `handleSessions`.

Verification evidence:

- Non-test verification: `gofmt -w internal/web/web.go internal/cli/run.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/web ./internal/cli ./internal/session ./internal/slash ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "handleSessions|webSessionSummary|webSessionSummaries|SessionStore|BrowseResumeCandidates|ResumeCandidate|sessionTitle|sessionMeta|openSession|in_current_work_dir|cost_usd|Store\\.List\\(" internal/web/web.go internal/cli/run.go internal/session internal/slash -g'*.go'` confirmed web browsing is session-store backed and slash resume candidates remain only in slash command code.

## 48. Web Runtime State Freezes Setup-Time Model And Provider

Severity: medium

Concrete files/functions involved:

- `internal/web/web.go`: `Config.ModelName`, `Config.Provider`, `server.handleState`, browser `renderState`, browser active-run summary rendering
- `internal/cli/run.go`: `RunWeb`, `BuildInteractiveRuntime`, `InteractiveRuntime.applyResumeProvider`
- `internal/slash/commands.go`: `handleModel`
- `internal/slash/command.go`: `Deps.ModelName`, `Deps.Provider`, `ModelSwitcher`, `OnModelChanged`
- `internal/app/store.go`: `StateStore.Snapshot`

What responsibility is split or misplaced:

The active model/provider are mutable runtime state, but the web state endpoint reports them from the immutable `web.Config` captured when the server starts. `RunWeb` fills `web.Config.ModelName` and `Provider` from `rt.Deps.Cfg` once. Later, resume can call `InteractiveRuntime.applyResumeProvider`, rebinding `rt.Deps.Cfg`, provider, engine, tools, and slash deps, and `/model` can switch the active model through `slash.Deps.ModelSwitcher` and update the app store. `server.handleState` still returns `"runtime": {"model": s.cfg.ModelName, "provider": s.cfg.Provider}`. The browser then prioritizes `runtime.model` and `runtime.provider` over `app_state.model`, so stale setup-time values override the live mutable state.

Why this is wrong in ownership/lifecycle terms:

Runtime state projection should be owned by the live runtime/store, not by the web server's startup configuration. Configuration is an input to runtime construction; it is not the authoritative state after model switches, provider rebinding, or session resume. By mixing setup config and live app state in the same `/api/state` payload, the web presentation layer has to guess which source is current and currently guesses wrong.

Observable bug or likely failure mode:

After resuming a session with a different model/provider, or after `/model <id>` succeeds in the web UI, the engine can use the new binding while `/api/state` still reports the original model/provider in `runtime`. The browser's header and active-run summary render `runtime.model || appState.model`, so the stale startup model hides the updated app-state model. A user can therefore see one model in the web state panel while subsequent model request events are sent to another.

Minimal direction for fixing the boundary:

Expose the live model/provider through a runtime-owned state snapshot or derive the web runtime block from `StateStore.Snapshot` plus the current provider binding after resume/model switch. Treat `web.Config` fields as initial labels only, or remove them from the state projection once the runtime has a mutable source of truth. The browser should render one authoritative current model/provider field instead of merging startup config with app state.

What not to do:

Do not patch the browser to prefer `appState.model` over `runtime.model` while leaving `/api/state.runtime` stale. That would hide one symptom in one view and leave other web consumers, event summaries, and future UI code reading the wrong runtime contract.

Status:

Resolved in current worktree. `/api/state.runtime` now projects model, provider, and workspace from the live `StateStore.Snapshot`, using startup config only as a fallback when the store has no value. The browser renders the runtime projection directly, so resume and `/model` updates flow through one authoritative state contract.

Source evidence:

- `internal/web/web.go`: `handleState` now calls `runtimeState(snap)` instead of filling runtime model/provider/workspace from immutable `web.Config`.
- `internal/web/web.go`: `runtimeState` derives `model` from `AppState.Model` or conversation model, `provider` from `AppState.Provider` or conversation provider, and `workspace` from `AppState.CWD` or conversation workdir, with config fields only as fallback labels.
- `internal/web/web.go`: browser `renderState` and `renderOverview` now render `runtime.model` directly rather than masking stale runtime fields with app-state fallbacks.
- Existing runtime owners remain unchanged: `InteractiveRuntime.Resume` updates `AppState.Model` and `AppState.Provider`, and `switchActiveModel` updates `AppState.Model`.

Verification evidence:

- Non-test verification: `gofmt -w internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/web ./internal/cli ./internal/slash ./internal/app ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "handleState|runtimeState|firstNonEmptyString|Config\\.ModelName|Config\\.Provider|runtime\\.model|runtime\\.provider|appState\\.model|app\\.model|applyResumeProvider|switchActiveModel|st\\.Model|st\\.Provider|s\\.cfg\\.ModelName|s\\.cfg\\.Provider" internal/web/web.go internal/cli/run.go internal/cli/tools.go internal/slash/commands.go -g'*.go'` confirmed web runtime projection uses the live store and config appears only as fallback.

## 49. Prompt History Has Separate Session, Runtime, And UI Owners

Severity: medium

Concrete files/functions involved:

- `internal/cli/run.go`: `InteractiveRuntime.runEngine`, `InteractiveRuntime.runOrchestration`, `writePromptHistory`, `promptHistoryFromSessions`, `sessionPromptHistory`
- `internal/session/writer.go`: `Writer.WritePromptHistory`
- `internal/session/store.go`: `Store.Load`
- `internal/web/web.go`: `Config.PromptHistory`, `server.handleState`, browser `appendEnvelope`, `rememberPrompt`, history navigation
- `internal/tui/handlers.go`: `handleLoopEvent`, `reloadConversationFromStore`
- `internal/tui/model.go`: `New`, resume rendering/history seeding

What responsibility is split or misplaced:

Accepted prompt history is persisted by the runtime session writer, seeded at interactive startup by scanning saved sessions, and then mutated independently by each UI. `InteractiveRuntime.runEngine` and `runOrchestration` call `writePromptHistory`, which appends `prompt_history` JSONL entries through `Writer.WritePromptHistory`. Separately, `BuildInteractiveRuntime` builds `rt.PromptHistory` once from `promptHistoryFromSessions`. Web exposes that frozen slice from `server.handleState` as `prompt_history`, while the browser mutates its own `state.promptHistory` when it receives `prompt_accepted`. TUI also keeps its own input ring, seeding it at startup and then calling `m.input.remember` on `AcceptedPromptEvent`; on resume it only replaces history from conversation messages when the input history is empty.

Why this is wrong in ownership/lifecycle terms:

Prompt history is accepted-prompt runtime state. The runtime already has the acceptance point and the session writer, but the current design leaves the active history split between durable JSONL entries, a setup-time runtime slice, browser-local state, and TUI-local state. The UI should navigate a runtime-owned accepted-prompt history projection; it should not be responsible for keeping its own shadow history consistent with persistence and resume.

Observable bug or likely failure mode:

In web mode, `/api/state` returns the startup `s.cfg.PromptHistory` forever. A prompt accepted after the web server starts is written to the session file and added to the current browser's local `state.promptHistory`, but a refreshed browser or a second browser connection receives the stale startup history until the whole process restarts and rescans sessions. In TUI mode, resuming a session after the input ring already has global startup history does not replace it with the resumed conversation's prompts because `reloadConversationFromStore` and the resume render path only call `SetHistory` when `len(m.input.history) == 0`. The user can therefore navigate prompt history from a previous session while viewing a resumed one, even though the session file has the resumed prompt history.

Minimal direction for fixing the boundary:

Make accepted prompt history part of the interactive runtime/session state. Update that runtime history at the same point that emits `AcceptedPromptEvent` and writes `prompt_history`, and expose it through a live state projection or event. Web and TUI should render and navigate that projection, and resume should replace or explicitly scope the active history from the resumed session/runtime state instead of relying on UI-local rings.

What not to do:

Do not patch only the browser to push more strings into `state.promptHistory`, and do not make TUI overwrite history on every render. Those are presentation-side repairs that keep the durable history, runtime state, and UI navigation state as independent owners.

Status:

Resolved in current worktree. Accepted prompt history now has a live runtime projection in `AppState.PromptHistory`. The runtime updates it when a prompt is accepted, session persistence still writes durable `prompt_history` entries, web state exposes the live projection, and TUI resume/reload replaces input history from the runtime snapshot instead of preserving stale UI-local history.

Source evidence:

- `internal/app/state.go` and `internal/app/store.go`: added `AppState.PromptHistory` with snapshot copying, making prompt history part of shared runtime state.
- `internal/cli/deps.go`: startup resume hydrates `AppState.PromptHistory` from the loaded session's persisted prompt history.
- `internal/cli/run.go`: `RunInput` calls `rememberAcceptedPrompt` before emitting `AcceptedPromptEvent`; standalone orchestration does the same before running the shared orchestration path.
- `internal/cli/run.go`: `rememberAcceptedPrompt` updates `AppState.PromptHistory` through the runtime store and keeps `InteractiveRuntime.PromptHistory` in sync for UI initial seeding.
- `internal/cli/run.go`: `InteractiveRuntime.Resume` replaces prompt history from the resumed session, while fresh interactive startup seeds the runtime store from `promptHistoryFromSessions` only when the active session has no prompt history.
- `internal/web/web.go`: `/api/state` now returns `snap.PromptHistory`; the stale `web.Config.PromptHistory` owner was removed.
- `internal/tui/handlers.go` and `internal/tui/model.go`: resume/reload paths call `promptHistoryFromSnapshot` and replace the input ring from runtime state, falling back to extracted conversation prompts only when the runtime projection is absent.

Verification evidence:

- Non-test verification: `gofmt -w internal/app/state.go internal/app/store.go internal/cli/deps.go internal/cli/run.go internal/tui/handlers.go internal/tui/model.go internal/web/web.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/app ./internal/cli ./internal/web ./internal/tui ./internal/session ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "PromptHistory|prompt_history|writePromptHistory|rememberAcceptedPrompt|appendPromptHistory|promptHistoryFromSnapshot|promptHistoryFromSessions|sessionPromptHistory|rememberPrompt|AcceptedPromptEvent|SetHistory|extractUserPrompts|s\\.cfg\\.PromptHistory|Config\\.PromptHistory" internal/app internal/cli/run.go internal/cli/deps.go internal/web/web.go internal/tui internal/session -g'*.go'` confirmed app/session runtime state is the canonical projection, web no longer has a config history owner, and TUI resume consumes the runtime snapshot.

## 50. Tool Result Blobs Live Outside The Session Store Lifecycle

Severity: medium

Concrete files/functions involved:

- `internal/toolresult/storage.go`: `ProcessToolResult`, `persistToolResult`, `PersistedOutputPath`, `ApplyToolResultBudget`
- `internal/tools/toolresultread/toolresultread.go`: `Tool.Invoke`
- `internal/query/loop.go`: `Engine.applyToolResultBudget`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/session/store.go`: `Store.Load`, `Store.List`, `Store.Delete`
- `cmd/pragma/replay_export.go`: `loadExportConversation`, `loadExportConversationFile`
- `internal/session/writer.go`: `WriteContentReplacement`

What responsibility is split or misplaced:

Large tool-result persistence writes session-owned blobs under `~/.pragma/sessions/<session-id>/tool-results/<tool-call-id>.txt`, but the session store lifecycle only owns the `<session-id>.jsonl` file. `ProcessToolResult` and `ApplyToolResultBudget` can create blob files and return `<persisted-output>` markers or `content_replacement` records. `tool_result.read` later reconstructs the file path from the active session id and tool call id. Meanwhile `Store.Load` reads JSONL entries, `Store.List` lists only `.jsonl` files, and `Store.Delete` removes only `<session-id>.jsonl`. Replay export similarly reconstructs conversations from JSONL messages and ignores the sidecar `tool-results` directory.

Why this is wrong in ownership/lifecycle terms:

Persisted tool-result blobs are durable session artifacts. Their lifecycle cannot be owned by helper functions in `toolresult` while the session store owns only the JSONL file. The marker in the conversation and the blob on disk form one persistence transaction, but creation, load, delete, and export are split across unrelated code paths with no session-owned manifest or cleanup policy.

Observable bug or likely failure mode:

Deleting a session through `Store.Delete` removes the JSONL file and leaves `~/.pragma/sessions/<session-id>/tool-results` behind. Exporting or reconstructing a session from JSONL preserves `<persisted-output>` markers in messages but does not include the referenced blob files, so `tool_result.read` cannot recover the full output in the exported environment. Conversely, orphaned blob directories can accumulate after deleted or compacted sessions because the session store does not know they belong to the session it is managing.

Minimal direction for fixing the boundary:

Make sidecar tool-result blobs first-class session artifacts. The session store should know the artifact root for a session and own delete/export behavior for both JSONL and sidecar blobs. Content-replacement records or an artifact manifest should be loaded with the session and used by `tool_result.read`, rather than reconstructing paths from convention alone.

What not to do:

Do not add ad hoc cleanup to `tool_result.read`, replay export, or the delete caller. That would keep sidecar artifact ownership scattered. The session store should own the lifecycle of all durable files that make up a session.

Status:

Resolved in current worktree. Session sidecar artifact paths now have a shared session path helper, `session.Store` exposes the session artifact root and tool-result directory, `Store.Delete` removes the session artifact tree along with the JSONL file, tool-result persistence uses the shared session path convention, and replay export includes tool-result sidecar files as portable base64 checkpoint artifacts.

Source evidence:

- `internal/sessionpath/path.go`: added the shared session artifact path convention for `<session-id>/tool-results`, without importing session or tool packages.
- `internal/session/store.go`: added `ArtifactDir` and `ToolResultsDir`, making the session store expose the artifact root for a session.
- `internal/session/store.go`: `Store.Delete` now removes the session artifact directory with `os.RemoveAll` after removing `<session-id>.jsonl`, so session deletion owns sidecar cleanup.
- `internal/toolresult/storage.go`: `persistToolResultBytes` and `PersistedOutputPath` now use `sessionpath.ToolResultsDir` instead of open-coded `"tool-results"` path construction.
- `cmd/pragma/replay_export.go`: checkpoint exports now include `tool_result_artifacts` entries loaded from the session tool-result directory, with relative path, byte count, and base64 content.
- `cmd/pragma/replay_export.go`: loading by session ID uses `Store.ToolResultsDir`; loading by explicit JSONL path uses the same `sessionpath` convention beside that JSONL file.

Verification evidence:

- Non-test verification: `gofmt -w cmd/pragma/replay_export.go internal/session/store.go internal/toolresult/storage.go internal/sessionpath/path.go`.
- Non-test verification: `go build ./cmd/pragma`; `go vet ./internal/session ./internal/sessionpath ./internal/toolresult ./internal/tools/toolresultread ./internal/query ./internal/tool ./cmd/pragma`; `git diff --check`.
- Ownership scan: `rg -n "ToolResultsDir|ToolResultsDirName|ArtifactDir|Store\\) Delete|RemoveAll|tool-results|PersistedOutputPath|persistToolResultBytes|SessionsDir\\(\\)|sessionpath|tool_result_artifacts|loadExportToolResultArtifacts|ContentBase64" internal/session internal/sessionpath internal/toolresult internal/tools/toolresultread internal/query internal/tool cmd/pragma/replay_export.go -g'*.go'` confirmed session store owns delete/export boundaries and tool-result helpers share the session path convention.

## 51. Anthropic Provider Repairs Tool-Result Pairing Instead Of Runtime Owning The Invariant

Severity: medium

Concrete files/functions involved:

- `internal/provider/anthropic/translate_out.go`: `buildWireParams`, `messagesToWire`, `contentPartToWireUser`
- `internal/provider/anthropic/normalize.go`: `normalizeMessages`, `ensureToolResultPairing`, `mergeConsecutiveSameRole`
- `internal/query/loop.go`: `Engine.executeToolBatch`, `messageHasToolCall`, `messageHasToolResult`
- `internal/model/message.go`: message role contract for tool results
- `internal/model/content.go`: `ToolCallPart`, `ToolResultPart`

What responsibility is split or misplaced:

Tool-call and tool-result pairing is a conversation/runtime invariant. `Engine.executeToolBatch` creates a `ToolResultPart` for each model-requested tool call and the query loop is the component that appends those results back into conversation state. The Anthropic provider nevertheless runs `normalizeMessages` on every request and `ensureToolResultPairing` mutates the request copy: it injects synthetic user messages with `"Tool execution was interrupted"` for assistant tool calls that have no following result, appends missing synthetic results into the next user message, and drops orphaned `ToolResultPart` values with no matching call. This repair happens only while translating to Anthropic wire params.

Why this is wrong in ownership/lifecycle terms:

A provider adapter should translate a valid runtime conversation into provider wire format. It should not decide semantic recovery for incomplete tool execution or silently remove persisted tool results. If the runtime has an interrupted tool call, orphaned result, or invalid message sequence, that state needs to be represented by the runtime/session layer with observable events and durable conversation updates. Provider-local repair hides the broken invariant from persistence, replay, other providers, and diagnostics.

Observable bug or likely failure mode:

The same stored conversation can be sent differently depending on provider. Anthropic requests may continue because the adapter invents interruption results or drops orphaned results; Google, OpenAI, replay, export, and local inspectors see the original conversation. Because the synthetic repair is not written back to the session, every Anthropic request can repeatedly repair the same bad history without any session event explaining that a tool call was interrupted. Dropping orphaned results in the adapter can also remove evidence from the model context without removing it from session history, so the user and provider see different task histories.

Minimal direction for fixing the boundary:

Move tool-result pairing validation and interruption repair to the query/session runtime boundary. The runtime should either persist an explicit interrupted-tool result when a tool batch is cancelled, or surface a session consistency error before provider translation. Provider adapters can enforce provider-specific role alternation, but they should fail or consume a runtime-owned normalized conversation rather than inventing or deleting tool-result semantics.

What not to do:

Do not copy `ensureToolResultPairing` into other providers to make behavior consistent. That would spread semantic repair into every adapter. Also do not just log when Anthropic injects a synthetic result; the invariant still belongs to the runtime conversation owner.

## 52. Lifecycle Tool Nodes Track File State In A Private Cache Outside Session Persistence

Severity: medium

Concrete files/functions involved:

- `internal/lifecycle/bridge/tool_node.go`: `ToolNode`, `simpleSnapshot`, `executeAllowedToolCalls`
- `internal/tools/lifecycle/lifecycle.go`: `Tool.Invoke`
- `internal/query/engine.go`: `Engine.RunGraph`, `runGraph`, `appendLifecycleMessages`, `FileStateRecords`
- `internal/query/loop.go`: `progressSnapshot.ReadFileState`, `Engine.executeToolBatch`
- `internal/cli/run.go`: `makeSessionSaveClose`
- `internal/tools/filewrite/filewrite.go`, `internal/tools/fileedit/fileedit.go`, `internal/tools/applypatch/applypatch.go`: file-state recording through `tool.RecordFileWriteState`

What responsibility is split or misplaced:

File freshness state belongs to the session/runtime engine, because it is used to know which file reads are still valid after writes and is persisted through `makeSessionSaveClose`. The normal query tool path passes the root engine's `FileStateCache` through `progressSnapshot.ReadFileState`, and session save writes `d.Engine.FileStateRecords()`. Lifecycle bridge tool nodes create a separate `tool.NewFileStateCache()` inside `ToolNode`, wrap it in `simpleSnapshot`, and pass that private cache to the same tool orchestrator. Tools that write files update the private lifecycle cache, not the root engine cache that the session writer persists.

Why this is wrong in ownership/lifecycle terms:

Lifecycle execution is still runtime tool execution against the user's workspace. Its file mutations should update the same file-state owner as ordinary tool calls, or be explicitly committed back to that owner. Keeping a hidden cache inside the lifecycle node makes the lifecycle runner the owner of file freshness during the graph, while the session/runtime owner remains unaware after the graph finishes.

Observable bug or likely failure mode:

A `LifecycleRun` tool or `Engine.RunGraph` can execute `FileWrite`, `FileEdit`, `apply_patch`, or notebook edits inside a lifecycle tool node. Those writes can be real workspace mutations, and the lifecycle result messages are appended to the main conversation, but `makeSessionSaveClose` persists only `d.Engine.FileStateRecords()`. After resume, the session can contain lifecycle messages that mention file writes while the file freshness records from those writes are missing. Later read-cache checks can treat stale reads as valid or lose evidence that a lifecycle tool modified a file.

Minimal direction for fixing the boundary:

Give lifecycle tool nodes the runtime engine's file-state owner, or have the lifecycle runner return file-state effects that the root engine/session persistence commits before the turn completes. The bridge can still isolate graph state, but file mutation receipts need one session-owned cache and one persistence path.

What not to do:

Do not make `ToolNode` write its private cache directly to session files. That would create a second file-state persistence writer. The fix is to route lifecycle tool effects through the existing runtime/session file-state owner.

## 53. `/clear` Starts A New Conversation Without Resetting Engine-Owned Session State

Severity: medium

Concrete files/functions involved:

- `internal/slash/commands.go`: `handleClear`
- `internal/cli/run.go`: `InteractiveRuntime.runSlash`, `closeCurrentSessionAfterClear`, `startSessionForCurrentConversation`, `makeSessionSaveClose`
- `internal/query/engine.go`: `ResetContentReplacementState`, `ResetFileState`, `FileStateRecords`
- `internal/tool/filestate.go`: `FileStateCache`, `RecordFileWriteState`, `RecordFileDelete`
- `internal/toolresult/storage.go`: `ContentReplacementState`, `ReconstructContentReplacementState`, `ApplyToolResultBudget`

What responsibility is split or misplaced:

`/clear` is treated as a new conversation/session boundary in the store and session writer, but not in the engine-owned runtime state. `handleClear` removes conversation messages and assigns a new conversation ID. `runSlash` then calls `closeCurrentSessionAfterClear`, which ends and closes the current session, clears `SessionWriter`, `SessionHeader`, `SessionLastIdx`, and `SessionStarted`, and rebuilds save callbacks. It does not reset `Engine` content-replacement state or file-state cache. Those caches are reset on resume through `ResetContentReplacementState` and `ResetFileState`, but not when `/clear` creates a fresh conversation.

Why this is wrong in ownership/lifecycle terms:

Starting a new conversation is a runtime lifecycle transition, not only a store mutation and writer swap. Engine-owned session state such as content-replacement tracking and file freshness records belongs to the active conversation/session. If `/clear` creates a new session identity, the same owner must also reset or explicitly carry over engine state. Leaving caches behind makes the engine span two logical sessions.

Observable bug or likely failure mode:

After `/clear`, the next prompt starts a new session file from the new conversation ID. `makeSessionSaveClose` will later persist `d.Engine.FileStateRecords()` into that new session, even if those records came from file reads or writes in the cleared conversation. The new session can therefore resume with stale file freshness state that predates its own messages. Content-replacement state can also retain seen tool-call IDs and replacements from the cleared conversation, so request-time budgeting decisions are made with hidden state that is no longer represented by the conversation history.

Minimal direction for fixing the boundary:

Make the clear transition go through a runtime method that resets all conversation-scoped engine state together: session lifecycle, writer/header, content-replacement tracking, file-state records, and any other per-session caches. If some file-state information is intentionally workspace-scoped, move it to a separately named workspace owner and keep it out of session persistence for the new conversation.

What not to do:

Do not patch `makeSessionSaveClose` to skip file-state writes only after `/clear`. That would hide one persistence symptom while leaving the engine using stale session-scoped state during the new conversation.

## 54. Runtime Resume Leaves SessionStart Bound To The Previous Session

Severity: medium

Concrete files/functions involved:

- `internal/cli/run.go`: `InteractiveRuntime.Resume`, `beginSessionLifecycle`, `endSessionLifecycle`, `makeSessionSaveClose`
- `internal/cli/deps.go`: `SetupDeps`, `Deps.SessionStart`
- `internal/tui/model.go`: `New`
- `internal/tui/toolbar.go`: `newToolbar`, `toolbar.View`
- `internal/web/web.go`: `Config.SessionStart`

What responsibility is split or misplaced:

Session timing is lifecycle metadata, but interactive runtime resume updates only some session-owned fields. `SetupDeps` sets `Deps.SessionStart` from the resumed session's `Conversation.CreatedAt` when the process starts with `--resume`. Runtime `/resume` and web resume go through `InteractiveRuntime.Resume`, which swaps the session writer, metrics, cost tracker, provider binding, store conversation, content replacements, file state, save callbacks, and lifecycle start event. It does not update `rt.Deps.SessionStart`. The stale field is later used by `makeSessionSaveClose` to compute `SessionEnded.DurationMs`, and TUI initializes its toolbar elapsed clock from the setup-time `SessionStart`.

Why this is wrong in ownership/lifecycle terms:

Resuming another session is a session lifecycle transition. The runtime owner cannot update writer, conversation, metrics, and hooks while leaving timing metadata owned by the previous session. Duration and elapsed time are not presentation-only fields; they are emitted in session lifecycle events and displayed by clients as part of the active session state.

Observable bug or likely failure mode:

Start an interactive session, then resume a saved session from within the same process. When the resumed session is later closed, `makeSessionSaveClose` computes `DurationMs` from the original process/session `Deps.SessionStart`, not from the resumed session's creation/start time. In TUI, the toolbar elapsed display also continues from the initial session because `newToolbar` received the old `SessionStart` and `InteractiveRuntime.Resume` has no path to update it. Lifecycle telemetry and UI timing can therefore report a duration for the wrong session.

Minimal direction for fixing the boundary:

Make resume update session lifecycle metadata in the same runtime transition that swaps the writer and conversation. `Deps.SessionStart` should be set from the resumed session or from the actual resume-start instant according to one documented session-duration policy, and UIs should receive the updated value through a runtime event or state projection.

What not to do:

Do not patch only the TUI toolbar elapsed clock. The stale value also feeds session close metadata, so the fix belongs in the runtime resume lifecycle, not in a presentation timer.

## 55. Team Leadership State Is Stored In AppState But Has No Session Persistence Boundary

Severity: medium

Concrete files/functions involved:

- `internal/app/state.go`: `AppState.TeamContext`, `TeamContext`
- `internal/tools/teamcreate/teamcreate.go`: `Tool.Invoke`
- `internal/tools/teamdelete/teamdelete.go`: `Tool.Invoke`
- `internal/session/entry.go`: `EntryKind`, session entry data types
- `internal/session/writer.go`: `Writer.WriteHandoffState`, `WriteFileState`, `WriteTodos`
- `internal/session/store.go`: `Store.Load`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/cli/run.go`: `InteractiveRuntime.Resume`, `makeSessionSaveClose`

What responsibility is split or misplaced:

Team leadership state is runtime session state. `TeamCreate` writes a durable team file and tasks directory, then stores the active team identity in `AppState.TeamContext`. `TeamDelete` relies on `snap.TeamContext` to know which team should be cleaned up and to reject deletion while teammates are active. Session persistence, however, has entries for messages, metadata, handoff state, content replacements, prompt history, file state, and todos, but no entry for `TeamContext`. `makeSessionSaveClose` never writes it, `Store.Load` never reads it, `SetupDeps` never hydrates it from a resumed session, and `InteractiveRuntime.Resume` only restores conversation, model/provider, handoff state, and todos.

Why this is wrong in ownership/lifecycle terms:

Creating or deleting a team is not just a tool-local filesystem side effect. It changes the active runtime's coordination state and future tool behavior. If `AppState.TeamContext` is the owner of "this session is leading team X", then the session persistence boundary must save and restore it with the rest of session-scoped mutable state. Otherwise the team tools and the session store disagree about whether the session owns an active team.

Observable bug or likely failure mode:

A user can create a team, continue working, and save the session. After process restart or `/resume`, the team file and tasks directory can still exist, but `AppState.TeamContext` is nil because it was never persisted. `TeamDelete` then returns "No active team to clean up" instead of cleaning up the team that the resumed conversation created, and `TeamCreate` can allow another team because the in-memory guard sees no active team. The resumed session history can show team creation while the runtime no longer knows it is leading that team.

Minimal direction for fixing the boundary:

Make team context a first-class persisted session state entry, or move active team ownership to a separate runtime/team store that is explicitly keyed by session ID and hydrated during setup and resume. The same owner should handle create/delete transitions, active teammate checks, session save, and resume restoration.

What not to do:

Do not make `TeamDelete` scan `~/.pragma/teams` or infer the active team from recent tool-result text. That would recreate session state from side effects. The active team identity needs one durable owner and one resume path.

## 56. State-Handoff Tool Side Effects Are Reordered Ahead Of Real Tool Execution

Severity: medium

Concrete files/functions involved:

- `internal/query/loop.go`: `Engine.executeToolBatch`, `executeHandoffPatch`, `executeCertifyFact`, `certifyToolResultContains`
- `internal/query/loop.go`: `systemWithHandoffState`, `handoffPatchToolDef`, `certifyFactToolDef`
- `internal/tool/orchestrator.go`: `Orchestrator.Execute`
- `internal/model/handoff.go`: `HandoffState`, `CertifiedFact`

What responsibility is split or misplaced:

Tool-call ordering is part of the runtime execution contract. In state-handoff mode, the engine treats `PatchHandoffState` and `CertifyFact` as special pseudo-tools inside `executeToolBatch`. It scans the whole model tool-call batch, executes those handoff mutations immediately, collects all other tool calls into `realCalls`, and only then runs the real calls through `Orchestrator.Execute`. The returned `ToolResultPart` values are placed back into the original indexes, so the transcript looks ordered, but the side effects are not executed in transcript order.

Why this is wrong in ownership/lifecycle terms:

The runtime is the owner of tool execution order and state mutation order. If handoff state is a durable interpretation of tool evidence, its mutations need to be sequenced against the actual tool effects they describe. A query loop should not make special state tools bypass the normal execution scheduler while making the conversation output appear as if ordinary order was respected.

Observable bug or likely failure mode:

If the model emits `Write` followed by `PatchHandoffState`, the patch is applied before the write runs even though the resulting conversation records the write result before the patch result. If the model emits a real tool and a same-batch `CertifyFact` for that tool's result, `executeCertifyFact` runs first and `certifyToolResultContains` searches only `snap.Conversation.Messages`, so it cannot see the result that will be produced later in the same batch. The runtime can therefore reject or mis-sequence a certification that the transcript ordering suggests should have evidence.

Minimal direction for fixing the boundary:

Execute handoff pseudo-tools through the same ordered tool execution pipeline as other calls, or explicitly reject invalid state-handoff ordering before side effects occur. If `PatchHandoffState` must be first, enforce that as a runtime validation error instead of relying on prompt text while silently reordering. If same-batch certification is supported, the verifier needs access to already-executed results in the current batch according to declared order.

What not to do:

Do not add more prompt wording that tells the model to call `PatchHandoffState` first. The runtime already depends on ordering for correctness, so the order must be enforced or executed by the runtime owner, not repaired by instructions.

## 57. Tool Supplements Lose Their Tool-Call Association At The Batch Boundary

Severity: medium

Concrete files/functions involved:

- `internal/tool/tool.go`: `InvokeResult.Supplements`
- `internal/tool/orchestrator.go`: `ExecuteResult`, `singleResult`, `Orchestrator.Execute`, `executeSingle`
- `internal/query/loop.go`: `Engine.runLoop`, `Engine.executeToolBatch`
- `internal/tools/fileread/fileread.go`: `readImage`
- `internal/tools/fileread/pdf.go`: `readPDF`, `readPDFPages`
- `internal/tools/repl/repl.go`: `Tool.Invoke`

What responsibility is split or misplaced:

Document and image supplements are tool-call-specific model context. `FileRead` returns `ImagePart` or `DocumentPart` supplements alongside the textual result for the same call, and hooks can return supplemental context for a specific `PreToolUse` or `PostToolUse` execution. `singleResult` keeps those supplements with one tool invocation, but `ExecuteResult` flattens all supplements into one batch-level `[]ContentPart`. `Engine.runLoop` then builds the user result message by appending all `ToolResultPart` values first and all supplements afterward.

Why this is wrong in ownership/lifecycle terms:

The orchestrator preserves per-call ownership for `Results`, `Displays`, and `FileEffects`, but drops it for supplements even though supplements are part of the model-visible evidence produced by a particular tool call. The query loop then has to guess a message layout from a batch-level bag of content. That makes tool evidence association an accidental ordering convention between the tool, orchestrator, and query loop instead of an explicit runtime contract.

Observable bug or likely failure mode:

If the model requests two tools in one batch, such as `FileRead` on an image and `Bash`, the conversation message contains both tool results first, followed by the image supplement. The image is no longer adjacent to or structurally tied to the `FileRead` result that describes it. Nested `REPL` compounds this by collecting supplements from its internal orchestrator execution and returning them as supplements for the outer `REPL` result, erasing the original primitive call association. Provider translators and later model turns can see a document/image block as generic user content rather than evidence attached to a specific tool result.

Minimal direction for fixing the boundary:

Make supplemental model-visible content part of the per-tool result contract. Either attach supplements to the corresponding `ToolResultPart` envelope when constructing the result message, or change `ExecuteResult` to carry per-index supplements just like displays and file effects so the query loop can preserve association and ordering explicitly.

What not to do:

Do not rely on appending explanatory text to the tool result or sorting supplements after the batch. That keeps the association implicit and still forces the query/provider layer to infer which tool produced which non-text evidence.

## 58. Fallback Context-Window Decisions Count A Different Request Than Provider Token Counters

Severity: medium

Concrete files/functions involved:

- `internal/query/loop.go`: `Engine.autoCompactBeforeRequest`, `isAtBlockingLimit`, `requestTokenCount`
- `internal/compact/tokens.go`: `EstimateConversationTokens`, `EstimateSystemPromptTokens`
- `internal/compact/window.go`: `WindowConfig`, `EffectiveWindow`, `AutoCompactThreshold`
- `internal/provider/provider.go`: `TokenCounter`, `RequestParams`
- `internal/provider/google/provider.go`: `Provider.CountTokens`
- `internal/cli/run.go`: `BuildCompactionDeps`

What responsibility is split or misplaced:

Context-window admission is a full provider-request decision. `requestTokenCount` uses provider `TokenCounter` when available and passes `RequestParams` containing model, messages, system prompt, and tools. If the provider does not implement `TokenCounter`, the same function falls back to `compact.EstimateConversationTokens(messages)`, which counts only conversation messages. `BuildCompactionDeps` separately subtracts an estimated system prompt size from `WindowConfig`, but tool schemas and other request-level content are not part of the fallback count.

Why this is wrong in ownership/lifecycle terms:

The runtime should have one definition of "how large is the next request" for compaction and blocking decisions. Instead, request sizing is split by provider capability: Google-style precise counting measures the actual request envelope, while fallback providers measure a conversation-only approximation and rely on window math elsewhere to partially compensate for system prompt size. Whether tools and system content affect admission should not depend on which provider adapter happens to expose a token-count endpoint.

Observable bug or likely failure mode:

For Anthropic, OpenAI/Groq through AnyLLM, replay, or any provider without `TokenCounter`, a turn with large tool schemas or large request-level non-message content can stay below `AutoCompactThreshold` because fallback counting ignores `tools` entirely. The same conversation on Google can compact or block earlier because `CountTokens` sees the full request. This can produce provider-specific context overflows or late compaction even though `autoCompactBeforeRequest` and `isAtBlockingLimit` are supposed to guard the runtime before sending the request.

Minimal direction for fixing the boundary:

Make fallback sizing estimate the same logical request as the precise path: messages, system prompt, tools, and provider-visible request options that affect context. Keep provider `TokenCounter` as a precision override, not a different admission contract. The compact/window package can own a request-level estimator that `requestTokenCount`, warnings, and compaction all use consistently.

What not to do:

Do not patch individual providers by adding arbitrary buffers, and do not only lower the auto-compaction threshold. That hides the undercount for some models while preserving two different definitions of request size.

## 59. Session Metadata Treats Internal Context And Tool Results As User Turns

Severity: medium

Concrete files/functions involved:

- `internal/cli/run.go`: `sessionMetadataForSnapshot`, `countUserTurns`, `extractSummary`, `makeSessionSaveClose`
- `internal/observe/metrics.go`: `Metrics.HandleEvent`, `MetricsSnapshot.TurnCount`
- `internal/query/engine.go`: `appendConversationMessage`, `AppendHookContext`, `emitMessageAppended`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/model/message.go`: `RoleUser`, `MessageFlags`, `ToolResultPart`
- `internal/session/session.go`: `Session.TurnCount`, `SessionSummary.TurnCount`
- `internal/session/store.go`: `Store.Load`, `readJSONLSummary`

What responsibility is split or misplaced:

Session metadata is supposed to summarize user-visible session progress, but it is computed from raw conversation storage shape. `sessionMetadataForSnapshot` ignores `MetricsSnapshot.TurnCount` and calls `countUserTurns`, which counts every `RoleUser` message. In this model, tool results are stored as `ToolResultPart` values inside `RoleUser` messages, hook context is appended as an internal `RoleUser` message by `AppendHookContext`, teammate-delivered messages are appended as `RoleUser`, and pause-turn continuation prompts are also appended as `RoleUser`. `extractSummary` similarly returns the first non-empty text from any `RoleUser` message without checking `MessageFlags`.

Why this is wrong in ownership/lifecycle terms:

The runtime owns accepted user prompts and completed turns; the session writer should persist metadata from that runtime event contract, not infer it from the low-level serialization trick that stores tool results in user-role messages. Role is a provider conversation primitive, not a session-progress counter. By deriving durable metadata from roles, session listing, resume seeding, metrics, and actual prompt lifecycle each get a different definition of a "turn."

Observable bug or likely failure mode:

A single user prompt that triggers three tool-use rounds can produce one real user prompt plus three `RoleUser` tool-result messages, so `SessionSummary.TurnCount` can show four turns. If `SessionStart` or `UserPromptSubmit` hooks add context before the engine runs, `extractSummary` can choose `"Hook context from SessionStart:..."` as the saved session summary instead of the user's prompt, because the internal hook message is a non-empty `RoleUser` text message. On resume, `SetupDeps` seeds `Metrics` from `sess.TurnCount`, so the inflated persisted count becomes the runtime counter baseline.

Minimal direction for fixing the boundary:

Make the prompt/turn lifecycle owner emit or store an explicit accepted-prompt/turn counter and summary source. Session metadata should count accepted user prompts or completed engine turns according to one documented policy, and summary extraction should ignore internal/meta/tool-result-only messages. If `Metrics` remains the owner, persist its turn count instead of recomputing from storage roles.

What not to do:

Do not add a special case that subtracts tool-result messages from `countUserTurns` while leaving summaries and metrics on separate definitions. The fix needs one session-progress owner, not another heuristic over serialized message roles.

## 60. Fallback Structured Output Is Selected By CLI Event Scraping

Severity: medium

Concrete files/functions involved:

- `internal/cli/run.go`: `runNonInteractive`, `loadOutputSchema`
- `internal/tools/synthetic/synthetic.go`: `Tool`, `New`, `InputSchema`, `Invoke`
- `internal/provider/provider.go`: `FeatureStructuredOutput`, `RequestParams.ResponseSchema`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/query/miniswe_loop.go`: `Engine.runPragmaLoopWithInitialPrompt`
- `internal/provider/google/provider.go`: `Provider.SupportsFeature`, `buildRequest`
- `internal/provider/openai/provider.go`, `internal/provider/anthropic/provider.go`, `internal/provider/groq/provider.go`, `internal/provider/lilac/provider.go`: `SupportsFeature`

What responsibility is split or misplaced:

Structured output is a model response contract. The provider request type already has `ResponseSchema`, and `runNonInteractive` uses it when the selected provider advertises `FeatureStructuredOutput`. For providers that do not advertise the feature, the same CLI path registers a synthetic `StructuredOutput` tool, suppresses normal text output, watches `query.ToolCallEvent` values, captures `e.Call.Input` when the tool name is `StructuredOutput`, and prints that captured input as the command's final stdout after the engine finishes.

Why this is wrong in ownership/lifecycle terms:

The non-interactive CLI is deciding which runtime event is the "real answer" by inspecting tool-call events. That makes fallback structured output a presentation/command-loop behavior instead of a runtime response contract. The synthetic tool's `Invoke` validates the input and returns only `"Structured output provided successfully"`, but the actual JSON is not returned as a typed final result by the runtime; it is recovered from the tool call input by the CLI event consumer. Interactive, web, replay, and any non-CLI runtime consumer do not share that final-output contract.

Observable bug or likely failure mode:

For OpenAI, Anthropic, Groq, Lilac, replay, or any provider whose `SupportsFeature` omits `FeatureStructuredOutput`, `pragma --output-schema --prompt ...` can print JSON from a `ToolCallEvent` even though the engine conversation records a tool call plus a success tool result, not a structured assistant response. If the model emits valid schema-shaped JSON as text, the CLI suppresses it and warns that the model did not call `StructuredOutput`. If a later SDK or web caller wants the same fallback behavior, it must duplicate the CLI's event-scraping rule or it will see a normal tool result instead of the structured final answer.

Minimal direction for fixing the boundary:

Keep native `ResponseSchema` at the provider request boundary, and move fallback structured output into a runtime-owned response mode. The runtime should expose a typed final structured-output result after the synthetic tool validates successfully, or fail the turn with a structured-output error if no valid output is produced. The CLI should print that runtime result rather than choosing it from raw tool-call events.

What not to do:

Do not patch this by adding more `ToolCallEvent` scraping to web, SDK, replay, or prompt subcommands. Also do not make the synthetic tool return the JSON as ordinary text while leaving the CLI to decide final output. The fallback response contract needs one runtime owner.

## 61. CLI Tool Filters Are A One-Time Registry Sweep While MCP Registers Tools Later

Severity: high

Concrete files/functions involved:

- `internal/cli/run.go`: `BuildInteractiveRuntime`, `runNonInteractive`, `applyToolFilters`, `waitForToolsetMCP`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/cli/tools.go`: `ToolExposurePolicy`, `toolExposurePolicyFromFlags`, `shouldRegisterBuiltinTool`
- `internal/mcp/manager.go`: `SetToolFilter`, `ConnectAllAndRegister`, `registerClientTools`, `ReconnectServer`
- `internal/tool/registry.go`: `Registry.Register`, `Registry.Unregister`, `ToolDefs`

What responsibility is split or misplaced:

CLI `--allowed-tools` and `--disallowed-tools` are represented as `ToolExposurePolicy` during built-in registration, but they are also applied later by `applyToolFilters`, which physically unregisters whatever tools are visible in the root registry at that moment. MCP tools are registered asynchronously after `SetupDeps` starts `ConnectAllAndRegister`; interactive and non-interactive setup both call `applyToolFilters(cmd, d.Registry)` before `waitForToolsetMCP`. The MCP manager only receives `SetToolFilter(activeToolset.AllowMCPTool)` when a compiled toolset is active; it does not receive the CLI allow/disallow policy. Its initial `registerClientTools` path applies that manager filter, while `ReconnectServer` registers listed tools directly and does not apply the manager filter either.

Why this is wrong in ownership/lifecycle terms:

Tool exposure is a runtime execution policy, not a cleanup pass over a mutable registry snapshot. A tool source that registers after the sweep can bypass the same policy that governed initial built-in tools. The registry, MCP manager, and CLI flag handling therefore each own part of the effective tool surface, and timing decides which owner wins.

Observable bug or likely failure mode:

Run with `--allowed-tools Bash` or `--disallowed-tools mcp__server__dangerous_tool` while an MCP server is still connecting. `applyToolFilters` can remove non-allowed built-ins from the registry, then `waitForToolsetMCP` allows the asynchronous MCP manager to finish registering remote tools. Because `registerClientTools` applies only the toolset filter, those MCP tools can appear in `Registry.ToolDefs()` for the very first model request despite the CLI allow/disallow flags. OAuth `ReconnectServer` is worse: after authentication it unregisters old tools and registers newly listed MCP tools without either the CLI exposure policy or the manager toolset filter.

Minimal direction for fixing the boundary:

Make the effective tool exposure policy a registration-time predicate shared by all tool sources. Built-in registration, synthetic/fallback tools, MCP initial registration, MCP reconnect/OAuth registration, and child registry factories should all consult the same policy object. If `applyToolFilters` remains, it should only reconcile legacy state after the authoritative registration policy is installed, not be the primary enforcement point.

What not to do:

Do not call `applyToolFilters` again after `waitForToolsetMCP` or after OAuth reconnect. That patches one timing window while preserving a policy split for future late registrars. Also do not add MCP-name-specific deny checks in `registerClientTools`; the existing CLI tool exposure policy should be the owner.

## 62. SendUserMessage Claims User Delivery Through An Observe-Only Side Channel

Severity: medium

Concrete files/functions involved:

- `internal/tools/brief/brief.go`: `Tool.Invoke`
- `internal/cli/tools.go`: `baseTools` registration of `toolbrief.Tool`
- `internal/tool/orchestrator.go`: `executeSingle`
- `internal/observe/event_catalog.go`: `BriefMessageSent`
- `internal/observe/logger.go`: `categoryForKind`
- `internal/interactive/event.go`: `Event`, `LoopEvent`
- `internal/web/web.go`: `normalizeWebEvent`, `normalizeLoopEvent`
- `internal/tui/handlers.go`: `handleLoopEvent`

What responsibility is split or misplaced:

`SendUserMessage` is described as a tool for communicating directly with the user. Its implementation resolves attachments and emits `observe.BriefMessageSent` with the real message, attachments, and status. It then returns a model-visible tool result saying `"Message delivered to user."` The normal interactive presentation stream, however, carries `interactive.AcceptedPromptEvent`, `interactive.LoopEvent`, and slash results. Web and TUI render query loop events such as text, tool call, and tool result events; they do not subscribe to or render `BriefMessageSent`. That observe event is categorized for logs and replay parsing, not for direct user delivery.

Why this is wrong in ownership/lifecycle terms:

User-facing communication is presentation/runtime output, not an observability side effect of a tool invocation. The tool layer can request that a user message be delivered, but the runtime or interactive stream must own whether it becomes visible to the user, persisted as user-visible output, and replayed/resumed consistently. Here the model receives a success result even though the only structured copy of the actual message went to the observe bus.

Observable bug or likely failure mode:

In web or TUI, a model can call `SendUserMessage` with a detailed message and attachments. The UI receives normal `ToolCallEvent` and `ToolResultEvent` values, so it can show the tool invocation and the generic `"Message delivered to user."` result, but there is no presentation event that renders the message as a first-class assistant/user-facing message. In a normal saved session, the message exists only inside the tool-call input and the generic tool result; the `BriefMessageSent` event is not a session entry. A replay/export path that reconstructs from session JSONL can therefore miss the user-facing brief even though the model was told it was delivered.

Minimal direction for fixing the boundary:

Move the brief delivery contract to the runtime/interactive event layer. `SendUserMessage` can remain a model-callable request, but successful invocation should produce a typed query or interactive event that web, TUI, CLI, session persistence, and replay can consume as user-visible output. Attachments should either be session-owned artifacts or explicit references in that event.

What not to do:

Do not patch this by making web or TUI subscribe directly to the observe bus. Observability events are not the user-output contract. Also do not rely on rendering the raw tool-call input as the delivered message; that keeps the presentation layer reverse-engineering domain output from a tool invocation payload.

## 63. `/advisor` Writes Runtime-Looking State That No Runtime Path Owns

Severity: low

Concrete files/functions involved:

- `internal/slash/commands.go`: `registerBuiltins` entry for `advisor`
- `internal/slash/advisor.go`: `handleAdvisor`
- `internal/app/state.go`: `AppState.AdvisorModel`
- `internal/cli/run.go`: `BuildInteractiveRuntime`, `makeSessionSaveClose`
- `internal/cli/subcommands.go`: `RunLocalCommand`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/query/miniswe_loop.go`: `Engine.runPragmaLoopWithInitialPrompt`
- `internal/session/writer.go`: session entry writers
- `internal/session/store.go`: `Store.Load`

What responsibility is split or misplaced:

The `advisor` slash command presents itself as a runtime model mode: "Show or set the advisor model." In interactive mode, `handleAdvisor` writes `AppState.AdvisorModel` directly through `deps.Store.Update`. No query engine path, provider request builder, tool, session writer, or resume loader reads `AdvisorModel`. `makeSessionSaveClose` persists messages, handoff state, file state, todos, and metadata, but not advisor state.

Why this is wrong in ownership/lifecycle terms:

An advisor model setting is runtime execution policy if it exists. It should have an owner that decides how advisor calls are made, which provider/model they use, when they run, how their output affects the turn, and whether the setting is session-scoped or config-scoped. Instead, the slash layer owns a field in shared app state that looks like runtime state but has no execution or persistence contract.

Observable bug or likely failure mode:

In an interactive session, `/advisor opus` returns "Advisor set to opus." A later model turn still follows the normal `Engine.runLoop` or pragma-loop request path; those paths resolve only the active model from store/config and never consult `AdvisorModel`. Resuming the session also drops the setting because `Store.Load` and `SetupDeps` do not hydrate it.

Minimal direction for fixing the boundary:

Either delete the stateful `/advisor` command until an actual runtime advisor path exists, or move advisor configuration behind the runtime component that performs advisor work. That owner should define whether advisor model selection is a session setting, config setting, or per-command option, and the slash command should call that owner rather than writing an inert app-state field.

What not to do:

Do not fix this by merely adding `AdvisorModel` to session metadata or `/api/state`. Persisting and displaying dead state would make the claim more durable without creating the missing execution owner. Also do not make the engine check the field opportunistically without defining the advisor turn semantics.

## 64. Replay Export Has A Separate JSONL Loader That Ignores Session Metadata

Severity: medium

Concrete files/functions involved:

- `cmd/pragma/replay_export.go`: `loadExportConversation`, `loadExportConversationFile`, `buildBugHuntCheckpoints`
- `internal/session/store.go`: `Store.Load`, `loadJSONL`
- `internal/session/store.go`: `readJSONLSummary`
- `internal/session/entry.go`: `EntryMetadata`, `MetadataData`
- `internal/cli/run.go`: `sessionMetadataForSnapshot`

What responsibility is split or misplaced:

Replay export accepts either a session ID or a JSONL path. For a session ID, `loadExportConversation` calls `session.Store.Load`, which applies the session store's JSONL semantics: messages come from `message` entries and mutable model/provider values come from the last `metadata` entry when present. For a JSONL path, `loadExportConversationFile` reimplements parsing locally and reads only `header` and `message` entries. It ignores `metadata`, prompt history, content replacements, file state, todos, and every other store-owned entry type. `buildBugHuntCheckpoints` then emits checkpoint `model` and `provider` fields from that locally parsed `exportConversation`.

Why this is wrong in ownership/lifecycle terms:

Session JSONL parsing belongs to the session store. Export is a consumer of reconstructed session state, not a second owner of the session format. By implementing its own partial loader, replay export creates a second persistence contract where mutable metadata entries are invisible if the user passes a file path instead of a session ID.

Observable bug or likely failure mode:

A session can start with one model in the header and later switch models, causing `sessionMetadataForSnapshot` to append updated `MetadataData.Model` and `Provider`. Running replay export by session ID uses `Store.Load` and exports checkpoints with the updated model/provider. Running the same export against the `.jsonl` file path uses only the immutable header values, so the checkpoint can target the wrong model/provider even though the session file contains the correct latest metadata. The same saved session therefore exports differently depending only on how it was addressed.

Minimal direction for fixing the boundary:

Make replay export use the session store's loader for JSONL files too, or move path-based loading into `session.Store` as a shared "load from path" operation with the same semantics as `Load`. If export intentionally wants a reduced snapshot, it should start from the canonical loaded `Session` and project fields from it, rather than reparsing the persistence format.

What not to do:

Do not patch `loadExportConversationFile` by copying just `EntryMetadata` handling into it. That would still leave export with its own drifting JSONL parser and would repeat the same mistake for the next session entry type.

## 65. Local Slash Command Fallback Does Not Satisfy TypeLocal Handler Dependencies

Severity: medium

Concrete files/functions involved:

- `internal/cli/subcommands.go`: `RunLocalCommand`
- `internal/slash/command.go`: `Deps`, `CommandType`, `TypeLocal`
- `internal/slash/commands.go`: `registerBuiltins`, `handleCost`, `handleModel`
- `internal/slash/advisor.go`: `handleAdvisor`

What responsibility is split or misplaced:

`TypeLocal` marks commands that run without an engine, and `RunLocalCommand` has an explicit fallback path for cases where full runtime setup fails. That fallback constructs slash deps with only model/provider strings, cwd, session store, and skill loader. The handlers still receive the same `slash.Deps` type used by full interactive execution, and some TypeLocal handlers unconditionally require fields that the fallback does not populate: `handleCost` calls `deps.CostTracker.Snapshot()` and `deps.CostTracker.TotalUSD()`, `handleModel` calls `deps.Store.Snapshot()` for `/model` and `deps.Store.Update(...)` for `/model <name>`, while `handleAdvisor` calls `deps.Store.Snapshot()` and `deps.Store.Update(...)` for every path.

Why this is wrong in ownership/lifecycle terms:

The local-command runner owns the dependency contract for local commands. Instead, command handlers are written against an implicit full-runtime dependency bag while the runner sometimes sends a partial lightweight bag. Whether a local command is safe therefore depends on which branch of setup happened before the handler was called, not on the command's declared type or an explicit dependency requirement.

Observable bug or likely failure mode:

If `SetupDeps` fails because provider/API/MCP/runtime setup is unavailable, `RunLocalCommand` deliberately falls back so local commands like `doctor` can still run. In that fallback branch, invoking `pragma cost` can nil-pointer on `deps.CostTracker`, and invoking `pragma model`, `pragma model <name>`, `pragma advisor`, or `pragma advisor <model>` can nil-pointer on `deps.Store` instead of returning local command output or a controlled error. The same slash command therefore has different safety properties depending on unrelated runtime setup success.

Minimal direction for fixing the boundary:

Define a real local slash dependency contract. Either build the minimal `StateStore` and other required local services in the fallback before dispatch, or have each command declare required capabilities so the runner can reject unsupported local commands with a clear error. `TypeLocal` should mean the runner can satisfy the handler without engine/provider startup, not "maybe full deps, maybe partial deps."

What not to do:

Do not add nil guards inside `handleCost`, `handleModel`, and `handleAdvisor` that silently fall back to stale config strings or no-op updates. That hides the fact that the local runner is violating the handler contract. Also do not remove the fallback just to avoid the crash; that would preserve the earlier lifecycle bug where local commands require full runtime setup.

## 66. `/commit` Runs Shell Commands In Slash Prompt Construction Instead Of The Tool Runtime

Severity: medium

Concrete files/functions involved:

- `internal/slash/commit.go`: `commitPromptTemplate`, `handleCommit`
- `internal/slash/shellexec.go`: `ExecShellInPrompt`, `execShellCommand`
- `internal/cli/subcommands.go`: `RunPromptCommand`
- `internal/cli/run.go`: `InteractiveRuntime.runSlash`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/tools/bash/bash.go`: `Tool.CheckPerm`, `Tool.Invoke`

What responsibility is split or misplaced:

Shell execution is owned by the tool runtime: the Bash tool parses command input, checks permissions, emits permission and execution events through the orchestrator, runs in the runtime workdir, and returns a tool result. The `/commit` slash command has a second shell execution path in prompt construction. `commitPromptTemplate` embeds `!` command substitutions for `git status`, `git diff HEAD`, `git branch --show-current`, and `git log --oneline -10`; `handleCommit` calls `ExecShellInPrompt`; and `ExecShellInPrompt` runs each command with `exec.CommandContext(ctx, "sh", "-c", command)` before returning `Result{InjectPrompt: prompt}`.

Why this is wrong in ownership/lifecycle terms:

Slash handlers should translate slash intent into a runtime action, not perform side-effect-capable process execution before the runtime accepts the prompt. The command runs before `InteractiveRuntime.runSlash` calls `runEngine`, and before the CLI prompt path hands the injected prompt to `runNonInteractive`. That bypasses the same permission owner, tool event stream, tool-result persistence, timeout policy, workdir abstraction, and hook/tool supplement boundary used for normal Bash execution.

Observable bug or likely failure mode:

Running `/commit` or `pragma commit` executes four shell commands even if Bash is disallowed, disabled by tool filters, or would normally ask for permission. Those commands do not produce `ToolPermissionChecked`, `ToolExecutionStarted`, or Bash tool result entries, so replay/audit/self-trace views can show the later model turn but not the shell reads that supplied the prompt context. Failures are also converted into inline text by `ExecShellInPrompt`, so the engine may receive an error string as if it were ordinary repository context rather than a runtime/tool failure.

Minimal direction for fixing the boundary:

Move commit-context collection behind the existing runtime/tool boundary. The slash command should either inject a prompt instructing the model to call Bash for the needed git reads, or call an existing runtime-owned context collector that records the same permission, events, workdir, timeout, and persistence semantics as tool execution. If precomputed commit context is required, make it a typed runtime operation with observed results, not an ad hoc shell preprocessor in slash.

What not to do:

Do not patch this by hardcoding a safer allowlist inside `ExecShellInPrompt` or by suppressing specific commands. That keeps a parallel shell execution mechanism alive. Also do not add these commands to `/commit` `AllowedTools`; those rules affect later model tool calls, not the pre-execution that already happened in the slash handler.

## 67. Cron Scheduler Records Fires Without Knowing Whether Prompt Launch Succeeded

Severity: high

Concrete files/functions involved:

- `internal/cron/scheduler.go`: `Scheduler.Start`, `Scheduler.tick`
- `internal/cli/cron.go`: `RunCronDaemon`, `launchCronPrompt`
- `internal/cron/store.go`: `Store.Save`

What responsibility is split or misplaced:

The scheduler owns durable cron job state: `LastFired`, `NextFire`, recurring reschedule, one-shot deletion, and persistence. The actual scheduled work is launched by the CLI handler passed to `Scheduler.Start`. That handler has type `func(job *Job)`, so it cannot report whether launching the background prompt succeeded. `RunCronDaemon` logs `launchCronPrompt` errors to stderr inside the handler, but `Scheduler.tick` continues as if the job fired successfully.

Why this is wrong in ownership/lifecycle terms:

A cron fire is a domain transition only if the scheduler can account for the attempted work outcome. Here the component that persists job progress cannot observe success or failure, while the component that knows failure has no way to stop or annotate the state transition. The lifecycle boundary is split between "try to launch" in CLI code and "mark fired/reschedule/delete" in scheduler code, with no result contract between them.

Observable bug or likely failure mode:

If `launchCronPrompt` fails because the executable cannot be resolved, the background child cannot start, or `pragma --prompt ... --bg` exits with an error, `RunCronDaemon` prints `cron job <id> failed to launch`. `Scheduler.tick` still sets `live.LastFired = now`; recurring jobs advance `NextFire`, and non-recurring jobs are deleted. `saveDurable` then persists that state, so a one-shot durable job can disappear even though no prompt session was launched.

Minimal direction for fixing the boundary:

Make scheduled work execution return a result to the scheduler. `Start`/`tick` should accept a handler that reports success or failure, and the scheduler should define the retry, failure metadata, and one-shot deletion policy from that result. If the CLI remains the launcher, it should return launch success to cron instead of only printing stderr.

What not to do:

Do not patch `RunCronDaemon` to recreate the job after a launch error. That would make the CLI repair scheduler-owned state from the outside. Also do not just log failures in the store; the scheduler still needs to avoid treating failed launches as successful fires unless that policy is explicit.

## 68. Replay `--then-live` Streams Providers Outside The Query Runtime

Severity: high

Concrete files/functions involved:

- `cmd/pragma/replay.go`: `replayDeterministic`, `replayLiveFromCheckpoint`, `requestParamsFromEvent`, `printReplayResponse`
- `internal/query/loop.go`: `Engine.runLoop`, `Engine.consumeStream`
- `internal/provider/accumulate.go`: `AccumulateStream`

What responsibility is split or misplaced:

The normal runtime owns provider streaming through `Engine.runLoop`: it builds request params from current state, emits model request/response events, applies retry policy, consumes stream chunks while emitting text/thinking events, appends assistant messages, handles malformed/content-filtered/pause/tool-use stop reasons, executes tools, and persists through the normal event path. The replay `--then-live` path bypasses all of that. `replayDeterministic` calls `replayLiveFromCheckpoint`; that function reconstructs `provider.RequestParams` from a recorded `APIRequestStarted` event, calls `d.Prov.Stream` directly from the Cobra command, accumulates chunks with `provider.AccumulateStream`, and prints the response with `printReplayResponse`.

Why this is wrong in ownership/lifecycle terms:

Live model continuation is runtime execution, not a replay command formatting operation. The CLI replay command is reconstructing enough runtime state to call a provider, but not enough to own the lifecycle consequences of a live model response. It uses a different stream accumulator than the engine and has no ownership of retries, conversation mutation, tool execution, session persistence, or runtime events.

Observable bug or likely failure mode:

`pragma replay <dir> --until-turn=N --then-live` can receive a live response with `StopToolUse`; instead of routing through `Engine.runLoop` and `executeToolBatch`, it prints `[tool: ...]` to stderr and exits. A live `StopPauseTurn` does not append the continuation prompt. A live `StopMalformedToolCall` or content-filtered response does not follow the engine's correction/drop behavior. Stream errors do not use the engine retry policy, and replay/self-trace output will not contain the normal `ModelRequestEvent`, text/thinking deltas, tool-call/result events, or session persistence triggered by the query loop.

Minimal direction for fixing the boundary:

Route live checkpoint continuation through a runtime-owned execution path. Either hydrate enough session/runtime state to run the query engine from the checkpoint, or expose a single runtime operation for "execute this recorded provider request live" that shares the engine's stream accumulator, retry policy, stop-reason handling, event emission, and tool execution decisions. The replay command should request that operation and render its events, not call the provider directly.

What not to do:

Do not copy retry loops, malformed-tool handling, or tool execution into `cmd/pragma/replay.go`. That would create a second query runtime. Also do not patch `printReplayResponse` to execute printed tool calls; tool execution belongs to the runtime/tool boundary, not a replay formatter.

## 69. Non-Interactive Mode Replaces Resolved Permission Policy With Bypass By Flag Presence

Severity: high

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`, permission loading in `config.LoadPermissions`, `permission.NewRuleChecker`
- `internal/cli/run.go`: `runNonInteractive`
- `internal/permission/load.go`: `RulesFromConfigEntries`
- `internal/permission/rulechecker.go`: `NewRuleChecker`

What responsibility is split or misplaced:

Permission policy is resolved in `SetupDeps`: it loads permission entries and mode from config scopes, applies `cfg.PermissionMode`, validates the mode, converts entries into rules, and builds `d.Checker`. Non-interactive execution then applies a second policy decision in `runNonInteractive`: if the `permission-mode` flag was not explicitly changed on the Cobra command, it replaces `d.Checker` with `permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)`.

Why this is wrong in ownership/lifecycle terms:

Permission policy resolution should have one owner and one effective result. `SetupDeps` already resolved config, credentials, CLI overrides, rules, and mode into a checker. The non-interactive runner then ignores that resolved policy based only on whether one CLI flag was present, not on whether the user configured permissions in files or whether loaded rules exist. Runtime execution mode is deciding security policy after the policy owner has done its work.

Observable bug or likely failure mode:

A user can configure `permission_mode: dontAsk` or deny rules in project/user settings, then run `pragma --prompt ...` without passing `--permission-mode`. `SetupDeps` builds a checker from those settings, but `runNonInteractive` replaces it with bypass mode and no rules. The non-interactive model can then run tools that the resolved config policy would have denied or prompted for. Passing the same mode explicitly as `--permission-mode=dontAsk` has different behavior from relying on the same value loaded from config.

Minimal direction for fixing the boundary:

Make permission resolution return both the effective checker and the source of the effective mode. If non-interactive mode intentionally defaults to bypass, that default should be applied inside permission/config resolution only when no config/flag policy exists. Otherwise `runNonInteractive` should use the resolved checker from `SetupDeps`.

What not to do:

Do not patch this by adding more ad hoc checks in `runNonInteractive`, such as looking for one settings file or one rule list. That keeps security policy split between execution mode and config resolution. Also do not silently preserve bypass while documenting it; the bug is that configured policy and flag-provided policy with the same value are not equivalent.

## 70. TUI Owns A Single-Slot Prompt Queue Outside Runtime Turn Admission

Severity: medium

Concrete files/functions involved:

- `internal/tui/input.go`: `inputComponent.Update`, `inputComponent.SetStreaming`, `inputComponent.SetQueued`
- `internal/tui/handlers.go`: `Model.handleInputSubmitted`, `Model.submitPrompt`, `Model.finishTurn`
- `internal/tui/e2e_test.go`: `TestInputQueuingDuringStreaming`, `TestInputQueuingTruncatesLongLabel`
- `internal/cli/run.go`: `InteractiveRuntime.RunInput`

What responsibility is split or misplaced:

Runtime prompt admission is owned by `InteractiveRuntime.RunInput`: it handles slash commands, session start, prompt history persistence, hook execution, query engine execution, and session save events. The TUI adds a separate prompt queue in `Model.pendingInput`. `inputComponent` is explicitly always active during streaming, and `handleInputSubmitted` stores submitted text in `pendingInput` when `m.streaming` is true. `finishTurn` later turns that UI field back into an `InputSubmittedMsg`, which then calls `submitPrompt` and eventually `RunInput`.

Why this is wrong in ownership/lifecycle terms:

Whether a prompt is accepted, queued, rejected, persisted, or ordered behind the current turn is runtime control flow. The TUI is making that sequencing decision before the runtime sees the prompt. It also owns queue capacity and replacement semantics, while the runtime and session store only learn about the prompt after the previous turn completes. This is distinct from rendering a prompt before runtime acceptance: here the UI is storing future work and deciding which future user turn exists at all.

Observable bug or likely failure mode:

While a response is streaming, the input remains active and Enter emits `InputSubmittedMsg`. The first submitted follow-up sets `m.pendingInput`. A second submitted follow-up before the current turn completes overwrites the same string field. When `finishTurn` runs, only the last queued prompt is auto-submitted; earlier queued prompts are silently lost and never pass through hooks, prompt history persistence, slash handling, or session messages. Other interfaces do not share this behavior because the queue lives only in the TUI model.

Minimal direction for fixing the boundary:

Move prompt admission and queueing into the interactive runtime, or make the runtime explicitly reject concurrent prompt submission and have the UI render that runtime decision. If queued prompts are a supported product behavior, the runtime should own the queue as ordered domain state and emit accepted/queued/rejected events that every interface can render.

What not to do:

Do not patch this by changing `pendingInput string` to `[]string` inside the TUI. That would keep turn ordering and queue policy in the presentation layer. Also do not only disable the textarea while streaming; that hides this TUI path but leaves prompt concurrency policy implicit instead of owned by runtime admission.

## 71. Global Trace State Outlives The Runtime Dependencies That Own Its Bus

Severity: medium

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`, `compositeCleanup`
- `internal/observe/trace.go`: `globalBus`, `SetGlobalBus`, `GlobalTrace`, `activeFilter`, `SetTraceFilter`
- `internal/observe/bus.go`: `EventBus.Emit`, `EventBus.Drain`
- `internal/observe/event_catalog.go`: `EventBus.Trace`

What responsibility is split or misplaced:

`SetupDeps` creates a per-execution `EventBus`, stores it on `Deps`, and also installs it into package-global observe state with `observe.SetGlobalBus(bus)`. If `PRAGMA_TRACE_FILTER` is set, it also stores a package-global trace filter with `observe.SetTraceFilter(...)`. Cleanup is owned by the returned deps object, but `compositeCleanup` only disconnects MCP, drains the per-run bus, and closes cleanup files. It never clears or restores the process-global bus or trace filter.

Why this is wrong in ownership/lifecycle terms:

The bus is a runtime dependency with a clear lifetime, but `GlobalTrace` is process-global state. Runtime setup mutates global observe state, while runtime cleanup only tears down the instance field. After cleanup, global tracing still points at a bus that has been drained and closed. The trace filter has an even longer accidental lifetime: if a later runtime setup does not set `PRAGMA_TRACE_FILTER`, the old filter remains active because there is no cleanup or default clear.

Observable bug or likely failure mode:

After a command calls `d.Cleanup()`, later package-level `observe.GlobalTrace(...)` calls load the old bus and call `bus.Trace`; `EventBus.Emit` drops the event because `closed` is true. In a long-lived process, test process, embedded command runner, or command path that invokes multiple runtime setups, a trace filter from the first setup can also suppress events for later runs that did not request that filter. Observability then depends on prior command lifetime rather than the current runtime configuration.

Minimal direction for fixing the boundary:

Make global trace installation return an owned restore function, or move global tracing behind the runtime-owned bus/context instead of mutable package state. `SetupDeps` should either always install and later restore the previous bus/filter, or cleanup should clear globals only if they still point at the values this deps instance installed.

What not to do:

Do not patch `GlobalTrace` to silently ignore closed buses and call that sufficient; it already effectively drops closed-bus events. The boundary bug is that a per-run dependency is installed globally without an ownership token or cleanup contract. Also do not only clear the filter at startup; that still leaves previous runs able to affect code before the next setup.

## 72. Standalone Lifecycle Run Replaces The Resolved Permission Checker

Severity: high

Concrete files/functions involved:

- `cmd/pragma/lifecycle.go`: `runLifecycle`, `generateGraph`, `loadYAMLGraph`
- `internal/cli/deps.go`: `SetupDeps`, permission loading and `d.Checker` construction
- `internal/cli/tools.go`: `RegisterTools`
- `internal/permission/rulechecker.go`: `NewRuleChecker`
- `internal/lifecycle/bridge/runner.go`: `Runner.Stream`
- `internal/tool/orchestrator.go`: `NewOrchestrator`

What responsibility is split or misplaced:

`pragma lifecycle run` calls `cli.SetupDeps`, which resolves configured permissions and stores the effective checker on `d.Checker`. The command then creates a different checker locally with `permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)`, registers tools, and builds a standalone orchestrator with that bypass checker. The lifecycle runner then executes graph tool nodes through the command-local orchestrator, not through the resolved permission checker owned by dependency setup.

Why this is wrong in ownership/lifecycle terms:

Permission policy has already been resolved by the runtime dependency setup layer before the standalone lifecycle command starts executing tools. The command layer should choose the lifecycle graph source and render progress; it should not replace security policy with a new bypass checker. This is a different failure from the root non-interactive prompt path's flag-presence bypass: here the lifecycle subcommand unconditionally creates its own bypass checker after setup, so fixes to the normal prompt runner do not change lifecycle command behavior.

Observable bug or likely failure mode:

A project can configure deny rules or a non-bypass permission mode, and `SetupDeps` will load those rules into `d.Checker`. Running `pragma lifecycle run workflow.yaml --prompt "..."` then discards that checker for lifecycle tool execution and uses bypass mode with no rules. The same tool calls that a normal prompt or `LifecycleRun` tool invocation would gate through the resolved checker can run unchecked in the standalone lifecycle command.

Minimal direction for fixing the boundary:

Use the resolved permission checker from `SetupDeps` when constructing the lifecycle command orchestrator. If standalone lifecycle intentionally needs a non-interactive default, apply that default in the shared permission-resolution layer only when no config or explicit CLI policy exists. Keep the effective permission decision in one owner and pass the resulting checker into lifecycle execution.

What not to do:

Do not keep a second `permission.NewRuleChecker` in `cmd/pragma/lifecycle.go` and add ad hoc exceptions for one settings file or one flag. That preserves the split policy owner. Also do not rely on lifecycle graph tooling to self-police dangerous tools; permission enforcement belongs at the orchestrator boundary.

## 73. Session Summary Listing Uses A Smaller Partial JSONL Reader Than Session Load

Severity: medium

Concrete files/functions involved:

- `internal/session/store.go`: `Store.loadJSONL`, `Store.List`, `Store.readJSONLSummary`
- `internal/session/writer.go`: `Writer.WriteFileState`, `Writer.WriteMetadata`
- `internal/cli/run.go`: `makeSessionSaveClose`, `RunListSessions`, `promptHistoryFromSessions`
- `internal/web/web.go`: session list/resume surfaces that depend on session summaries

What responsibility is split or misplaced:

Full session load and session summary listing both parse the same JSONL persistence format, but they use different readers with different correctness limits. `loadJSONL` sets the scanner buffer to 64MB with a comment that `file_state` entries can include cached file contents. `readJSONLSummary` uses only a 1MB scanner buffer while scanning the rest of the file for the last metadata entry. Normal saves write messages, handoff state, file state, todos, and then metadata, so metadata can appear after a large file-state line.

Why this is wrong in ownership/lifecycle terms:

The session store owns the JSONL format and should provide one consistent parsing contract for session metadata. Listing should be a store-owned projection of the same durable format, not a second partial parser with different line-size behavior. Otherwise the persistence format has two effective schemas: one for resume/load and another for list/search/resume-picker summaries.

Observable bug or likely failure mode:

After a run writes a large `file_state` entry, `Store.loadJSONL` can still load the session because it allows large lines. `Store.readJSONLSummary` can stop scanning at that same line before it reaches the latest metadata because its scanner limit is 1MB and it does not check `scanner.Err()` after the metadata scan. `pragma --list-sessions`, prompt-history seeding from session summaries, and web/session picker surfaces can then show stale or empty summary, model/provider, turn count, cost, or updated time even though the full session resumes correctly.

Minimal direction for fixing the boundary:

Make session summary reading use the same JSONL reader limits and error handling as full session load, or centralize entry streaming in the store so both load and summary projections consume one parser. If summaries must stay fast, write a summary/index entry before large side-state entries or keep summary metadata in a store-owned index with explicit update semantics.

What not to do:

Do not patch individual UI/session-list callers to fall back to loading full sessions when a field is blank. That spreads recovery outside the store and leaves the duplicated parser broken. Also do not just increase the summary scanner limit without checking scan errors; the summary projection should fail or degrade explicitly when it cannot parse the persisted format.

## 74. StateStore Snapshots Leak The Mutable TeamContext Pointer

Severity: medium

Concrete files/functions involved:

- `internal/app/store.go`: `StateStore.Snapshot`, `StateStore.Update`
- `internal/app/state.go`: `AppState.TeamContext`, `TeamContext`
- `internal/tools/teamcreate/teamcreate.go`: `Tool.Invoke`
- `internal/tools/teamdelete/teamdelete.go`: `Tool.Invoke`

What responsibility is split or misplaced:

`StateStore` claims to provide thread-safe state access through `Snapshot()` for reads and `Update()` for writes. `Snapshot` deep-copies `Conversation`, `HandoffState`, `Temperature`, `Thinking`, `Todos`, and `Worktree`, but it returns `AppState.TeamContext` as the same pointer stored inside the live state. Any caller with a snapshot can mutate `snap.TeamContext.TeamName`, `TeamFilePath`, or `LeadAgentID` without going through `StateStore.Update`.

Why this is wrong in ownership/lifecycle terms:

The state store is the owner of mutable runtime state. A snapshot must be an immutable copy from the caller's perspective; otherwise read paths can become hidden write paths outside the store lock. This is a different boundary error from team context not being persisted: even before persistence, the in-process state owner leaks one of its pointer fields and lets consumers bypass the update contract.

Observable bug or likely failure mode:

A tool, UI renderer, web handler, or future diagnostic path can call `Store.Snapshot()`, inspect `snap.TeamContext`, and accidentally mutate the pointed-to struct. The live store changes immediately without `Update`, without lock protection, without any event, and without any persistence trigger. Concurrent `TeamCreate` or `TeamDelete` calls can then see a partially mutated active team identity, or a rendered/debug path can corrupt the active team name used for cleanup.

Minimal direction for fixing the boundary:

Make `StateStore.Snapshot` deep-copy `TeamContext` the same way it already copies `Worktree`, pointer scalars, and slices. Longer term, keep pointer fields in `AppState` covered by a single copy policy or helper so adding a new pointer field cannot silently escape the store boundary.

What not to do:

Do not rely on callers to treat snapshots as read-only by convention. The store API exists to enforce that ownership boundary. Also do not fix only team tools; the leak is in the generic snapshot projection used by all surfaces.

## 75. Worktree Session Teardown Is Promised By The Tool But Not Owned By Runtime Shutdown

Severity: medium

Concrete files/functions involved:

- `internal/tools/worktree/enter.go`: `EnterTool.Invoke`, `enterDescription`
- `internal/tools/worktree/exit.go`: `ExitTool.Invoke`, `restoreSessionCWD`
- `internal/app/state.go`: `AppState.Worktree`, `WorktreeSession`
- `internal/cli/run.go`: `InteractiveRuntime.CloseSession`, `InteractiveRuntime.Cleanup`, `makeSessionSaveClose`, `endSessionLifecycle`
- `internal/tui/handlers.go`: `Model.quit`
- `internal/web/web.go`: shutdown path through `CloseSession`

What responsibility is split or misplaced:

`EnterWorktree` creates a git worktree, changes `AppState.CWD`, and records an active `AppState.Worktree`. Its description promises that "On session exit, if still in the worktree, the user will be prompted to keep or remove it." The only code that resolves that lifecycle is `ExitWorktree`: it checks the active worktree, removes or keeps it based on changes, restores `AppState.CWD`, and clears `AppState.Worktree`. Normal runtime/session close paths never inspect `AppState.Worktree` and never run an owned worktree-exit transition.

Why this is wrong in ownership/lifecycle terms:

Entering a worktree creates a session-scoped cleanup obligation. That obligation cannot be owned solely by a later model/tool call, because the user can end the session without the model invoking `ExitWorktree`. Session shutdown is the runtime lifecycle owner; if active worktree state is part of session state, runtime close must either resolve it, persist it, or explicitly mark it abandoned. Instead, the tool owns entry and optional manual exit, while runtime shutdown only closes the session writer and emits session lifecycle events.

Observable bug or likely failure mode:

A session can call `EnterWorktree`, then the user can type `/exit`, press Ctrl+C twice in TUI, stop the web server, or hit any path that calls `CloseSession`/cleanup. The session closes without prompting to keep/remove the worktree, without restoring `AppState.CWD`, and without clearing or persisting the active worktree state. The filesystem worktree remains under `.pragma/worktrees`, and a later resume has no active `Worktree` state because issue 3 covers the missing persistence boundary.

Minimal direction for fixing the boundary:

Make worktree enter/exit a runtime-owned session sub-lifecycle. Session close should ask that owner for the active worktree policy and either block for an explicit user decision, persist the active worktree for resume, or mark it abandoned with a durable artifact. `ExitWorktree` can remain a command/tool for manual exit, but it should call the same runtime worktree transition used by shutdown.

What not to do:

Do not solve this by adding more text to the worktree tool prompt or by making the TUI print a warning on quit. That leaves cleanup policy in presentation or model behavior. Also do not have cleanup blindly remove `.pragma/worktrees`; the runtime must preserve worktrees with changes and must only act on the active session-owned worktree.

## 76. Subagent Engines Use Setup-Time CWD Instead Of The Active Session WorkDir

Severity: high

Concrete files/functions involved:

- `internal/cli/tools.go`: `RegisterTools`, `engineFactory`, `baseTools`
- `internal/tools/agent/agent.go`: `Tool.invoke`, `RunForked`
- `internal/tools/worktree/enter.go`: `EnterTool.Invoke`
- `internal/app/state.go`: `AppState.CWD`, `WorktreeSession`

What responsibility is split or misplaced:

The active session working directory lives in `AppState.CWD`; `EnterWorktree` updates that field when it switches the session into a worktree. Subagent engine creation is owned elsewhere. `RegisterTools` closes over setup-time `d.Cwd` in `engineFactory`, and every forked subagent store is initialized with `CWD: d.Cwd`. `Agent.invoke` snapshots the parent store and forks the conversation, but for normal non-isolated Agent calls it never passes the active `snapshot.CWD` or `state.WorkDir()` into the child store. Only the special `isolation:"worktree"` path updates the child store to a newly-created worktree path.

Why this is wrong in ownership/lifecycle terms:

The active workdir is runtime session state, not dependency setup state. A child engine that executes tools on behalf of the active session should inherit the same working directory owner as the parent unless it explicitly creates its own isolation boundary. Here foreground tools read `AppState.CWD`, while subagent tools default to `Deps.Cwd`; session workdir state is split between the live store and a setup-time closure.

Observable bug or likely failure mode:

After `EnterWorktree`, the parent session has `AppState.CWD` set to `.pragma/worktrees/<slug>`. A subsequent `Agent` call without `isolation:"worktree"` creates a subengine whose store still has `CWD: d.Cwd`, the original checkout. The subagent's `Read`, `Grep`, `Edit`, `Bash`, and other path-sensitive tools can inspect or mutate the original repository while the parent session appears to be working inside the worktree. The parent conversation can then summarize subagent work as if it happened in the active worktree even though the child executed elsewhere.

Minimal direction for fixing the boundary:

Make child engine creation receive the parent runtime snapshot or explicit execution context, and initialize child `AppState.CWD` from the current active session CWD by default. Keep `isolation:"worktree"` as an explicit child-owned override that creates a new worktree, but do not let the default child path fall back to dependency setup CWD.

What not to do:

Do not patch this by telling the model to pass `isolation:"worktree"` whenever the parent is already in a worktree. The runtime knows the active session workdir and should propagate it. Also do not update only the Agent prompt text; every forked skill using `RunForked` shares the same `engineFactory` default.

## 77. Config Tool Writes Project Settings Through Setup-Time WorkDir

Severity: medium

Concrete files/functions involved:

- `internal/tools/config/config.go`: `Tool.Invoke`, `Tool.handleGet`, `Tool.handleSet`, `readSettingFromFile`, `updateSettingsFile`
- `internal/tools/config/settings.go`: project-scoped settings such as `model` and `permission_mode`
- `internal/cli/tools.go`: `baseTools`, `toolconfig.Tool{WorkDir: d.Cwd}`
- `internal/tool/orchestrator.go`: `Orchestrator.Execute`
- `internal/app/state.go`: `AppState.WorkDir`
- `internal/tools/worktree/enter.go`: `EnterTool.Invoke`

What responsibility is split or misplaced:

The orchestrator passes every tool invocation a `tool.StateSnapshot`, and `AppState.WorkDir()` exposes the current session working directory. Most path-sensitive tools use `state.WorkDir()` at invocation time. The `Config` tool accepts the same snapshot in `Invoke`, but ignores it. Its read/write helpers use `t.WorkDir`, which `baseTools` initialized from setup-time `d.Cwd`. Project-scoped settings are therefore read from and written to `d.Cwd/.pragma/settings.json` even when the active session CWD has changed.

Why this is wrong in ownership/lifecycle terms:

Project-scoped configuration belongs to the active project/workdir boundary. After a runtime transition such as `EnterWorktree`, the active session workdir lives in `AppState.CWD`, not in dependency setup. A tool invocation should either operate against the current tool state or explicitly declare that it mutates the original checkout's config. Here the config persistence boundary is split between setup-time deps and runtime state.

Observable bug or likely failure mode:

After `EnterWorktree` switches `AppState.CWD` into `.pragma/worktrees/<slug>`, `Read`, `Grep`, `Bash`, and edit tools operate in the worktree because they use `state.WorkDir()`. A model call to `Config` for `model` or `permission_mode` still reads or writes the original repository's `.pragma/settings.json`. The model receives a successful "Set ..." result while the active worktree/project config is unchanged, and future sessions in the original checkout can inherit a setting that was made while the user believed the session was scoped to the worktree.

Minimal direction for fixing the boundary:

Use the invocation state as the source of truth for project-scoped config paths. `Config.Invoke` should pass `state.WorkDir()` into get/set helpers, or the config layer should receive an explicit runtime config scope from the orchestrator. Keep global settings global, but make project settings follow the current runtime workdir owner.

What not to do:

Do not patch this by special-casing worktree paths inside the Config tool or by adding prompt guidance telling the model where settings are written. The same bug exists for any future runtime CWD transition. Also do not make every caller update `Tool.WorkDir`; the orchestrator already supplies the correct per-invocation state.

## 78. Project Skills Are Advertised And Loaded From A Frozen Startup WorkDir

Severity: medium

Concrete files/functions involved:

- `internal/sysprompt/builder.go`: `Builder.Build`, `buildSkillBlock`
- `internal/skill/loader.go`: `NewLoader`, `LoadAll`, `Load`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/cli/run.go`: `BuildInteractiveRuntime`
- `internal/cli/tools.go`: `RegisterTools`
- `internal/tools/skill/skill.go`: `Tool.Invoke`, `availableSkillNames`
- `internal/slash/skills_cmd.go`: `handleSkills`

What responsibility is split or misplaced:

Project skills are defined as `<workDir>/.pragma/skills` resources. `SetupDeps` builds the system prompt once with `sysprompt.New(cwd, ...)`, which advertises the skills found under setup-time `cwd`. `BuildInteractiveRuntime` separately builds a slash registry and `/skills` dependency from `skill.NewLoader(d.Cwd)`, and `RegisterTools` gives the model-facing `Skill` tool another loader from the same setup-time `d.Cwd`. None of these paths consult the active `AppState.CWD` after runtime workdir transitions.

Why this is wrong in ownership/lifecycle terms:

The active project's callable skill set is runtime context, not a process-start constant. The system prompt, slash command surface, `/skills` listing, and `Skill` tool execution should all be projections of one current project-skill owner. Instead, skill discovery is split across system prompt construction, interactive slash registration, slash deps, and tool construction, all pinned to dependency setup.

Observable bug or likely failure mode:

If a session enters a worktree or any future project-scoped runtime CWD, the model still sees the skills advertised from the original checkout's `.pragma/skills`. `/skills` lists the original loader's project skills, and `Skill.Invoke` executes from that same original loader. A skill added, removed, or changed in the active worktree branch is invisible, while a stale original-checkout skill can still be advertised and executed even though the current tools operate in a different workdir.

Minimal direction for fixing the boundary:

Make project-skill discovery runtime-scoped. The runtime should own a current skill catalog derived from the active workdir and expose it to the system prompt/reminder surface, slash registry, `/skills`, and `Skill` tool. When the active workdir changes, that catalog should be refreshed or invalidated through the same runtime transition.

What not to do:

Do not patch this by rebuilding only the `/skills` output or only the `Skill` tool loader. That would keep the advertised model context and executable skill set out of sync. Also do not add worktree-specific fallback lookup; the boundary is that multiple consumers each create their own frozen loader instead of sharing runtime-owned project skill state.

## 79. Permission Checks Resolve Paths Against Setup CWD While Tools Invoke In Runtime CWD

Severity: high

Concrete files/functions involved:

- `internal/tool/tool.go`: `Descriptor.CheckPerm`, `Descriptor.Invoke`, `StateSnapshot`
- `internal/tool/orchestrator.go`: `executeSingle`
- `internal/permission/rulechecker.go`: `NewRuleChecker`, `Check`, `acceptEditsDecision`, `AddPersistentRule`
- `internal/permission/match.go`: `MatchContent`, `MatchPathContent`
- `internal/permission/dangerous.go`: `resolvePathsForCheck`, `IsDangerousPath`
- `internal/cli/deps.go`: `SetupDeps`
- file/path tools such as `internal/tools/fileedit/fileedit.go`, `internal/tools/filewrite/filewrite.go`, `internal/tools/bash/bash.go`

What responsibility is split or misplaced:

Tool invocation state includes `StateSnapshot.WorkDir()`, and path-sensitive tools execute using that runtime workdir. Permission checking is a separate interface: `CheckPerm(ctx, input, checker)` receives no state snapshot, and the production checker was constructed in `SetupDeps` with setup-time `cwd`. `Orchestrator.executeSingle` calls `desc.CheckPerm(...)` first, then later calls `desc.Invoke(..., state)`, so policy resolution and tool execution can use different workdir owners.

Why this is wrong in ownership/lifecycle terms:

Permission policy is part of the runtime execution boundary. It must be evaluated against the same execution context that the tool will use. A checker whose path matching, dangerous-path detection, `acceptEdits` scope, and persistent-rule path are all bound to dependency setup cannot correctly authorize a tool invocation whose workdir is owned by runtime state.

Observable bug or likely failure mode:

After a runtime CWD transition, `Edit` or `Write` can execute inside the active worktree because the tool uses `state.WorkDir()`, but the permission checker still matches rules and resolves path safety against the original checkout. `acceptEdits` can allow a file path because it is inside the original setup cwd even though the invoked tool writes the same relative path under a different active cwd. "Remember" persistence also writes the allowed rule to `settings.local.json` under the setup cwd through `AddPersistentRule`, not the active runtime project scope.

Minimal direction for fixing the boundary:

Make permission checks receive the same runtime execution context as invocation. The orchestrator should pass the current state or a narrower execution context to permission evaluation, and the checker should resolve path rules, dangerous paths, accept-edits scope, and persisted-rule target from that context. Keep policy rules loaded from config, but do not freeze the execution cwd inside the checker.

What not to do:

Do not patch individual file tools to send absolute paths from `CheckPerm`; that still leaves Bash/apply-patch/worktree and persistent-rule scope on a different owner. Also do not rebuild the checker on every workdir change as a hidden side effect unless the permission layer has an explicit runtime context contract; the bug is that authorization and invocation currently have different inputs.

## 80. File Permission Policy Infers Path Semantics From Raw String Syntax

Severity: high

Concrete files/functions involved:

- `internal/tools/fileread/fileread.go`: `Tool.CheckPerm`
- `internal/tools/fileedit/fileedit.go`: `Tool.CheckPerm`
- `internal/tools/filewrite/filewrite.go`: `Tool.CheckPerm`
- `internal/tools/notebookedit/notebookedit.go`: `Tool.CheckPerm`
- `internal/tools/glob/glob.go`: `Tool.CheckPerm`
- `internal/tools/grep/grep.go`: `Tool.CheckPerm`
- `internal/permission/rulechecker.go`: `Check`, `acceptEditsDecision`
- `internal/permission/match.go`: `MatchContent`
- `internal/permission/dangerous.go`: `isFilePath`, `resolvePathsForCheck`, `IsDangerousPath`

What responsibility is split or misplaced:

File tools know that their extracted content is a file path, but they pass it to the generic permission checker as an untyped string. The checker then decides whether to use path matching and dangerous-path logic by inspecting the string: `isFilePath` returns true only for strings starting with `/` or `~`, and `MatchContent` uses path matching only for actual content with those prefixes. Relative paths from file tools are treated like shell/content strings even though invocation resolves them as paths.

Why this is wrong in ownership/lifecycle terms:

Path authorization is semantic policy. It should be driven by the tool's declared input meaning and execution context, not by whether the model happened to include an absolute path. The file tool layer extracts typed path intent, but the permission layer discards that type and reconstructs intent from a string prefix. That puts a security decision in a mechanical parser instead of the tool/runtime contract.

Observable bug or likely failure mode:

In `acceptEdits` mode, a call such as `Write` with `file_path: ".env"` reaches `RuleChecker.Check` with content `.env`. The dangerous-path branch is skipped because `.env` does not start with `/` or `~`. `acceptEditsDecision` then resolves the same relative content under `rc.workDir`, sees it is inside the project, and returns allow. The tool later writes the dangerous file. Path-specific allow/deny rules also behave differently for `foo.txt` versus `/abs/project/foo.txt` even though both can target the same file.

Minimal direction for fixing the boundary:

Make permission content typed enough for the checker to know when it is authorizing a path. File/path tools should produce a permission request with semantic kind `path` and the raw path, and the checker should resolve dangerous paths and path rules for that kind regardless of whether the input is relative, absolute, or home-relative.

What not to do:

Do not patch this by adding `.` to `isFilePath` or by making every file tool stringify absolute paths before checking. That preserves syntax-based policy and will keep producing edge cases. Also do not add per-tool dangerous-file checks; the permission owner should enforce path safety consistently for all path-bearing tools.

## 81. Hook Discovery And Execution Stay Bound To Startup WorkDir

Severity: high

Concrete files/functions involved:

- `internal/hook/manager.go`: `NewManager`, `Reload`, `Execute`
- `internal/hook/loader.go`: `LoadHooks`
- `internal/hook/executor.go`: `ExecCommand`
- `internal/hook/hook.go`: `HookInput`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/tool/orchestrator.go`: `executeSingle`
- `internal/cli/run.go`: `InteractiveRuntime.RunInput`, `beginSessionLifecycle`, `endSessionLifecycle`
- `internal/tools/worktree/enter.go`: `EnterTool.Invoke`

What responsibility is split or misplaced:

Hooks are project-scoped runtime policy, but `SetupDeps` creates one `hook.Manager` with setup-time `cwd`. The manager loads project and local hooks through `LoadHooks(workDir)`, stores that `workDir`, reloads only from that same path, fills `HookInput.CWD` from it, sets `PRAGMA_CWD` from it, and runs every hook command with `ExecCommand(..., m.workDir, ...)`. Tool execution and prompt runtime can later operate under `AppState.CWD`, but hook selection and hook process execution remain pinned to the startup checkout.

Why this is wrong in ownership/lifecycle terms:

Hooks are control policy around the same prompt/tool/session events that the runtime is executing. They must observe and run in the same project scope as the event they are gating. Freezing hook config and hook cwd in dependency setup splits runtime execution from runtime policy: the tool owner changes workdir, but the hook owner keeps enforcing and executing in the original project.

Observable bug or likely failure mode:

After `EnterWorktree`, `Bash`, `Edit`, `Write`, and other tools execute in the worktree, but `PreToolUse` and `PostToolUse` hooks are still discovered from the original `.pragma/settings.json` and `.pragma/settings.local.json`. Hook scripts receive `cwd` and `PRAGMA_CWD` for the original checkout and run with `shellCmd.Dir` set to that checkout. A worktree-specific hook that should block a write never runs, while an original-checkout hook can inspect or mutate the wrong tree while approving or denying an active-worktree tool event.

Minimal direction for fixing the boundary:

Make hook execution receive the current runtime execution context. The runtime should own the active hook policy for the active workdir, or the manager should resolve hooks and command cwd from the event context it is gating. Workdir transitions should refresh or replace the project/local hook scope through an explicit runtime transition.

What not to do:

Do not patch individual hook inputs to print the active cwd while still loading and running hooks from `m.workDir`. That would make hook payloads lie about where the hook process runs. Also do not special-case worktrees inside `Reload`; any runtime project-scope transition needs the same hook policy ownership.

## 82. Conversation System Prompt Freezes Project Instructions And Environment Before Runtime WorkDir Changes

Severity: high

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`
- `internal/sysprompt/builder.go`: `Builder.Build`, `buildSkillBlock`
- `internal/sysprompt/agentmd.go`: `LoadAgentMD`, `agentMDBlock`
- `internal/sysprompt/env.go`: `DetectEnv`, `envBlock`
- `internal/model/conversation.go`: `NewConversation`, `Conversation.System`, `Conversation.WorkDir`
- `internal/query/loop.go`: `Engine.runLoop`
- `internal/tools/worktree/enter.go`: `EnterTool.Invoke`

What responsibility is split or misplaced:

`SetupDeps` builds the system prompt once from setup-time `cwd`. The builder loads project and local `AGENT.md`, detects environment metadata such as working directory and git branch, and embeds that into `Conversation.System` when `model.NewConversation` is created. Later query requests use `snap.Conversation.System` directly. Runtime workdir transitions update `AppState.CWD`, but they do not rebuild or replace the conversation system prompt or conversation `WorkDir`.

Why this is wrong in ownership/lifecycle terms:

The model's project instructions and environment context are runtime project context, not immutable startup metadata. If the runtime changes the active project/workdir, the model-facing system context must either change with it or the transition must be represented as a new execution scope. Here filesystem tools, permission context, skill catalog, hooks, and model prompt context can all point at different project scopes.

Observable bug or likely failure mode:

After entering a worktree on another branch, tools operate under `AppState.CWD`, but the model request still includes the original checkout's `AGENT.md` contents and an environment block whose working directory and git branch came from setup. If the worktree branch changes local instructions, safety rules, build commands, or project layout guidance, the model continues following stale instructions while editing the active worktree. The request can also tell the model it is in one directory while tools execute in another.

Minimal direction for fixing the boundary:

Make active project context a runtime-owned projection used to build model requests. Either rebuild the project instruction/environment blocks when the active workdir changes, or make workdir transitions create a clearly separate conversation/session scope whose system prompt and metadata match the new project. The query loop should consume the current runtime project context, not a stale startup prompt.

What not to do:

Do not patch this by appending a one-line "current cwd changed" user message. That leaves stale higher-priority system instructions and environment text in place. Also do not only update `Conversation.WorkDir`; the model-visible `Conversation.System` is the part that continues to steer behavior.

## 83. Project Toolset And MCP Capability Surface Is Frozen At Dependency Setup

Severity: high

Concrete files/functions involved:

- `internal/cli/deps.go`: `SetupDeps`
- `internal/toolset/toolset.go`: `Resolve`, `Load`, `FilterMCPServers`, `configToolsetPaths`
- `internal/mcp/config.go`: `LoadConfig`
- `internal/mcp/manager.go`: `ConfigureServers`, `SetToolFilter`, `ServerStatuses`, `PendingServerNames`
- `internal/cli/tools.go`: `RegisterTools`, `mcpStatusesForQuery`, `baseTools`
- `internal/query/loop.go`: `systemWithMCPStatus`
- `internal/tools/mcp/list.go`: `ListMcpResourcesTool`
- `internal/tools/toolsearch/toolsearch.go`: `Tool.Invoke`

What responsibility is split or misplaced:

Project toolset definitions and MCP server configs are loaded in `SetupDeps` from setup-time `cwd`. The compiled toolset decides which built-ins are registered and filters MCP servers/tools. The MCP manager is then configured and connected from the setup-time MCP config, and the query engine captures a status callback to that manager. Runtime workdir changes update `AppState.CWD`, but they do not reload project/local toolsets, MCP configs, MCP connections, MCP tools, or the MCP status surface.

Why this is wrong in ownership/lifecycle terms:

The active tool and MCP capability surface is runtime execution policy. If workdir/project scope is mutable, then the project-scoped capability config must be owned by the same runtime transition that changes project scope. Here built-in tool exposure, remote MCP servers, registered remote tools, `ToolSearch` pending names, `/mcp` status, and the model's authoritative MCP system block all remain attached to the startup project while local tools execute elsewhere.

Observable bug or likely failure mode:

After entering a worktree with different `.pragma/toolsets.*`, `.mcp.json`, `.pragma/mcp.json`, or `.pragma/mcp.local.json`, the model request still receives MCP statuses from the original manager through `systemWithMCPStatus`. `ListMcpResourcesTool` and MCP tools continue targeting servers configured for the original checkout, and `ToolSearch` reports pending server names from that original manager. A server or tool allowed only by the original project can stay available in the active worktree, while worktree-specific MCP servers and tool restrictions are invisible.

Minimal direction for fixing the boundary:

Make the effective toolset and MCP manager part of runtime project context. A workdir transition should explicitly decide whether to keep the old capability scope, rebuild it for the new active workdir, or start a new session scope. Built-in registration, MCP connection lifecycle, MCP status injection, `/mcp`, and `ToolSearch` should all read from that one current capability owner.

What not to do:

Do not patch this by only reloading MCP config after `EnterWorktree`. That would leave built-in toolset filtering, tool search, and system prompt MCP status split from the active capability owner. Also do not keep both old and new MCP managers live without an explicit session boundary; remote tools and resources need one authoritative active project scope.

## 84. Resume Rebuilds The System Prompt Instead Of Loading The Persisted Session Prompt

Severity: high

Concrete files/functions involved:

- `internal/session/entry.go`: `HeaderData.System`
- `internal/session/store.go`: `Store.loadJSONL`
- `internal/cli/deps.go`: `SetupDeps`
- `internal/sysprompt/builder.go`: `Builder.Build`
- `internal/model/conversation.go`: `Conversation.System`
- `internal/query/loop.go`: `Engine.runLoop`

What responsibility is split or misplaced:

The session store persists the conversation's system prompt in the JSONL header and `Store.loadJSONL` reconstructs `Conversation.System` from `header.System`. `SetupDeps` then resumes the conversation with `conv = sess.Conversation`, but immediately overwrites `conv.System` with a freshly built startup prompt when `sess.SystemOverride` is empty. That rebuild can read current `AGENT.md`, current environment metadata, current tool filters, and current config instead of the prompt that was persisted with the conversation.

Why this is wrong in ownership/lifecycle terms:

A resumed conversation's model context is durable session state. Resume should restore the stored conversation boundary before new user-visible activity occurs. Rebuilding the system prompt in startup setup makes the persistence layer load the correct state and then lets the CLI dependency builder silently mutate it based on current filesystem/config state. That turns resume into a partial migration every time the process starts, without a domain event, user action, or persisted record explaining the changed model instructions.

Observable bug or likely failure mode:

Start a session, change `AGENT.md`, project instructions, tool filters, or environment-detected context, then run `pragma --resume <id>` or `pragma --continue`. The old messages are sent with a new system prompt even though the session file still contains the original `header.System`. The model can answer and edit according to instructions that did not exist when the prior conversation happened, while session export/replay still claims the original header prompt was the durable conversation context. Cached/system-block assumptions can also change across resume without any recorded session transition.

Minimal direction for fixing the boundary:

On resume, keep `sess.Conversation.System` as loaded from the session header. Only replace it through an explicit user-controlled operation that records the system-prompt change as session state, or through a clearly defined migration path that rewrites the durable header and metadata consistently. New sessions should build current project context; resumed sessions should restore their persisted context.

What not to do:

Do not patch this by appending the current `AGENT.md` or environment as another user/system-looking message after resume. That leaves two competing sources of system truth. Also do not silently rebuild only when the prompt text differs; an unrecorded automatic refresh is the boundary violation.

## 85. MCP OAuth Authentication Bypasses The Tool Permission Boundary

Severity: high

Concrete files/functions involved:

- `internal/tools/mcpauth/mcpauth.go`: `Tool.Flags`, `Tool.CheckPerm`, `Tool.Invoke`
- `internal/mcp/oauth_flow.go`: `Manager.StartOAuthFlow`, `handleOAuthCallback`
- `internal/mcp/oauth.go`: `SaveToken`
- `internal/mcp/manager.go`: `ReconnectServer`, `registerClientTools`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/permission/rulechecker.go`: `RuleChecker.Check`

What responsibility is split or misplaced:

The MCP authentication pseudo-tool marks itself non-read-only with `Tool.Flags` returning `ReadOnly: false`, but its `CheckPerm` ignores the runtime checker and always returns `permission.DecisionAllow`. Invoking it starts a local OAuth callback server through `Manager.StartOAuthFlow`. The background callback exchanges the code, writes a token under `~/.pragma/mcp-tokens/<server>.json` with `SaveToken`, reconnects the server, unregisters the auth pseudo-tool, and registers the real MCP tools.

Why this is wrong in ownership/lifecycle terms:

Permission checking is the runtime boundary for user-approved side effects. Starting an OAuth flow, storing credentials, and changing the active tool registry are not ordinary read-only discovery operations. The MCP auth tool places those side effects behind a tool-local unconditional allow, so the runtime permission policy, remembered decisions, non-interactive mode, and configured tool rules do not own whether the credential/capability transition is allowed.

Observable bug or likely failure mode:

If the model calls `mcp__<server>__authenticate`, the runtime starts the auth flow without consulting the user's permission mode or project rules for that tool. In interactive mode the user may only see the returned URL after the callback server is already listening. In non-interactive or permissive automated runs, the model can initiate a credential-writing flow and, after browser completion, the manager mutates the registry by swapping in newly authenticated MCP tools. That new capability surface was authorized by the auth tool itself rather than by the runtime permission owner.

Minimal direction for fixing the boundary:

Route MCP authentication through the normal permission checker with a permission subject that names the server and the credential persistence/capability change. The manager should perform token save and reconnect only after the runtime has approved that transition. The same effective tool exposure policy should then govern the post-auth registered tools.

What not to do:

Do not patch this by only changing the tool description to tell the model to ask the user first. The model is not the permission boundary. Also do not hard-code an allowlist inside `mcpauth`; permission and capability policy should remain in the runtime checker/registration policy, not in a pseudo-tool shortcut.

## 86. MCP Tool Adapters Reconnect Clients Outside The Manager-Owned Capability State

Severity: medium

Concrete files/functions involved:

- `internal/mcp/adapter.go`: `MCPToolAdapter.Invoke`
- `internal/mcp/client.go`: `Client.CallTool`, `Client.Reconnect`, `Client.Disconnect`, `Client.Connect`
- `internal/mcp/manager.go`: `ReconnectServer`, `ServerStatus`, `ServerStatuses`, `registeredTools`
- `internal/tool/registry.go`: `Registry.Register`, `Registry.Unregister`

What responsibility is split or misplaced:

The MCP manager owns server status, configured clients, registered tool names, and reconnect logic through `ReconnectServer`. But each registered `MCPToolAdapter` also owns a self-healing path: if `Client.CallTool` returns `ErrServerNotConnected`, the adapter calls `a.client.Reconnect(ctx)` directly and retries the tool call. `Client.Reconnect` only disconnects and reconnects the transport. It does not update manager `statuses`, `lastErrors`, `registeredTools`, registry entries, or the listed tool schema cache exposed through the manager.

Why this is wrong in ownership/lifecycle terms:

MCP connection recovery is a capability lifecycle transition, not a per-tool transport detail. The owner that decides a server is connected, which tools are registered, and how many tools are exposed must also own reconnect. Letting adapters reconnect their embedded client pointer bypasses the manager's registry/status transaction and leaves the runtime with two sources of truth: the client transport state and the manager's capability state.

Observable bug or likely failure mode:

If a server disconnects and a tool call triggers adapter-level reconnect, the call can succeed through the reconnected client while `Manager.ServerStatuses` still reports the previous status and tool count from `registeredTools`. If reconnect fails, `Client.Disconnect` clears the client transport and tool cache, but `ServerStatuses` can still show the old manager status and old registered tool count because `MCPToolAdapter.Invoke` never records the failure through `Manager.ReconnectServer`. If the remote server's tool list changed across reconnect, the registry can keep stale adapter entries while the manager never unregisters removed tools or applies current registration filters.

Minimal direction for fixing the boundary:

Route tool-call recovery through the manager or give adapters a manager-owned reconnect callback that performs one authoritative transition: disconnect old client, reconnect, refresh tool list, apply registration policy, update statuses/errors, and then retry if the requested tool is still registered. The adapter should not mutate transport state in isolation from the registry owner.

What not to do:

Do not patch `ServerStatuses` to infer more from `client.Connected()` while leaving adapters to reconnect independently. That would only hide stale status text and would not refresh registered tools, filters, or tool schemas. Also do not make each adapter unregister itself; registry membership belongs to the MCP manager.

## 87. Background Session Control Trusts Stale PID Files As Process Authority

Severity: high

Concrete files/functions involved:

- `internal/background/registry.go`: `Registry.Register`, `List`, `Get`, `UpdateStatus`
- `internal/background/info.go`: `ProcessInfo`
- `internal/background/kill_unix.go`: `Registry.Kill`, `isProcessAlive`
- `internal/background/kill_windows.go`: `Registry.Kill`, `isProcessAlive`
- `internal/cli/run.go`: `RunBackground`
- `internal/background/subscriber.go`: `StatusSubscriber.HandleEvent`

What responsibility is split or misplaced:

The background registry persists control records keyed only by PID under `~/.pragma/active-sessions/{pid}.json`. `Registry.List` treats `syscall.Kill(pid, 0)` on Unix as proof that the record is still active, and removes records as a side effect of listing when that check fails. `Registry.Kill` reads the persisted PID/PGID and sends signals to that process group. On Windows, `isProcessAlive` uses `os.FindProcess`, which does not prove the process is still running, so stale records can be treated as live indefinitely.

Why this is wrong in ownership/lifecycle terms:

Process control belongs to a runtime/process supervisor that can prove the process it is managing is the child session it launched. A durable PID file is only a hint. Here listing, status display, and kill commands all treat the PID file as authoritative domain state, while cleanup/reaping is hidden inside the read path. There is no ownership token, process start-time check, session handshake, or child-owned heartbeat that validates the record before control actions use it.

Observable bug or likely failure mode:

If a background child exits before unregistering and the OS later reuses the PID, `Registry.List` can keep showing the stale record because a process with that PID exists. `Registry.Kill` can then send `SIGTERM`/`SIGKILL` to the persisted process group ID, which may no longer belong to the original Pragma background session. On Windows, stale records are even more likely to remain because `os.FindProcess` can succeed for a non-running process handle, so `pragma sessions` can show dead sessions as active and `Kill` can report success while only deleting the stale file.

Minimal direction for fixing the boundary:

Make background process records carry verifiable process identity and lifecycle state, such as a child-owned session handshake, heartbeat timestamp, and OS-specific process start identity where available. Listing should be a read projection or call an explicit reaper that validates identity before deleting records. Kill should refuse or degrade to stale-record cleanup unless the supervisor can prove the PID/PGID still belongs to the recorded Pragma child.

What not to do:

Do not patch this by adding more PID existence checks or by deleting files more aggressively in `List`. PID existence is the wrong authority. Also do not silently kill by PID when process-group kill fails; that widens the blast radius of a stale control record.

## 88. Google Context Cache Lifetime Is Owned By Provider Requests Instead Of Session Cleanup

Severity: medium

Concrete files/functions involved:

- `internal/provider/google/provider.go`: `Provider`, `New`, `Complete`, `Stream`, `applyCache`
- `internal/provider/google/cache.go`: `cacheManager`, `getOrCreateCache`
- `internal/provider/provider.go`: `Provider` interface
- `internal/query/engine.go`: `NewEngine`, `RebindProvider`
- `internal/cli/run.go`: `InteractiveRuntime.applyResumeProvider`, `CloseSession`
- `internal/cli/deps.go`: `SetupDeps`, `compositeCleanup`

What responsibility is split or misplaced:

The Google provider owns a `cacheManager` that creates remote Gemini cached-content resources in `getOrCreateCache`. That manager deletes the previous cache only when a later request produces a different cache hash. The `Provider` interface has no close/cleanup method, `Engine.RebindProvider` simply swaps providers, and session/runtime cleanup does not notify providers. The active remote cache therefore has no owner at session end, runtime cleanup, or provider rebind.

Why this is wrong in ownership/lifecycle terms:

Remote cached content is a runtime/session resource derived from the current request's system prompt, stable message prefix, and tools. Its lifetime should be attached to the session or provider binding that created it. Leaving cleanup to the next cache miss makes normal session close, `/model` or resume provider rebinding, command cancellation, and process cleanup invisible to the owner of the remote resource.

Observable bug or likely failure mode:

A Google-backed session with a large stable prefix can create a remote cache named by the Gemini API with display name `pragma-session-cache`. If the user exits, resumes into another provider, switches providers, or the command finishes without another Google request with a different prefix, the cache is not deleted by Pragma; it survives until provider-side TTL expiry. Because Google bills cached content storage separately from request tokens, this can leave avoidable remote resources and cost after the session that created them has ended. A provider rebind also loses the pointer to `cacheManager.current`, so the old cache can no longer be deleted by a later request.

Minimal direction for fixing the boundary:

Add an explicit provider/runtime cleanup boundary for providers that own remote resources. Session close and provider rebind should call that cleanup before dropping the provider binding. The Google cache manager should expose a close/delete-current operation with the same context/lifecycle ownership as the session or engine, while still keeping per-request cache hit/miss logic inside the provider.

What not to do:

Do not patch this by shortening the TTL or relying on provider-side expiry. That leaves resource lifetime outside the runtime. Also do not delete the cache after every request; that would defeat the cache feature while still avoiding the real boundary, which is cleanup when the session/provider binding ends.

## 89. Team Creation Commits Durable Files Before The Runtime Team Transition Succeeds

Severity: medium

Concrete files/functions involved:

- `internal/tools/teamcreate/teamcreate.go`: `Tool.Invoke`
- `internal/team/team.go`: `WriteTeamFile`, `TeamExists`, `TeamFilePath`, `TasksDir`
- `internal/app/state.go`: `AppState.TeamContext`
- `internal/tools/teamdelete/teamdelete.go`: `Tool.Invoke`

What responsibility is split or misplaced:

`TeamCreate.Invoke` performs a multi-step team creation transaction itself. It checks `AppState.TeamContext`, chooses a team slug, writes `~/.pragma/teams/<team>/config.json` with `team.WriteTeamFile`, creates `~/.pragma/tasks/<team>`, then updates `AppState.TeamContext` and emits `TeamCreated`. The durable team file is committed before the task directory exists and before the runtime state says this session is leading that team.

Why this is wrong in ownership/lifecycle terms:

Creating a team is one domain transition: durable team config, task workspace, active session ownership, and event emission need to become true together or fail together. Here a tool method owns the transaction by sequencing independent filesystem and runtime-state writes without rollback or a team-level commit boundary. The `team` package owns file helpers, the app store owns active state, and the tool glues them together after partial durability is already visible.

Observable bug or likely failure mode:

If `WriteTeamFile` succeeds but `os.MkdirAll(team.TasksDir(finalName))` fails, `TeamCreate` returns an error with a durable team config still on disk. `AppState.TeamContext` remains nil and no `TeamCreated` event is emitted. A later `TeamCreate` with the same requested name sees `TeamExists(finalName)` and silently generates a different slug, even though the previous team was never active in the session. `TeamDelete` also cannot clean that orphan through normal active-team flow because it relies on `snap.TeamContext` and will return "No active team to clean up."

Minimal direction for fixing the boundary:

Move team creation into a single team/runtime operation that stages all durable files first, commits active `TeamContext` only after the durable workspace is complete, and rolls back staged files on failure. The operation should emit `TeamCreated` only after the transaction is complete, and it should expose orphan recovery through the same team owner rather than through ad hoc file checks.

What not to do:

Do not patch only by deleting `TeamFilePath(finalName)` after `MkdirAll` fails inside the tool. That still leaves transaction ownership in the model-facing tool and will miss future durable team steps. Also do not make `TeamDelete` scan all team directories to guess orphans; active team ownership and durable team creation need one commit boundary.

## 90. Team Deletion Deletes Its Cleanup Ledger After Ignoring Worktree Failures

Severity: medium

Concrete files/functions involved:

- `internal/tools/teamdelete/teamdelete.go`: `Tool.Invoke`, `activeTeammateNames`
- `internal/team/team.go`: `CleanupTeamDirectories`, `ReadTeamFile`, `TeamDir`, `TasksDir`
- `internal/app/state.go`: `AppState.TeamContext`
- `internal/task/registry.go`: `Registry.ListRunningTeammates`

What responsibility is split or misplaced:

`TeamDelete.Invoke` decides that a team can be removed by checking the volatile task registry for currently running teammates. It then delegates filesystem cleanup to `team.CleanupTeamDirectories`. That helper reads the durable team config to discover member worktrees, but it ignores any `ReadTeamFile` error. For every member worktree it runs `git worktree remove --force <path>` and ignores the command result. It then removes the team directory and tasks directory, returns nil, and `TeamDelete.Invoke` clears `AppState.TeamContext`, emits `TeamDeleted`, and reports success.

Why this is wrong in ownership/lifecycle terms:

The team config is the durable cleanup ledger for member worktrees. Deletion is the one transition that must either consume that ledger authoritatively or preserve enough information to retry or recover. Instead, cleanup is treated as a best-effort side effect hidden in a filesystem helper. The runtime-facing tool then mutates domain state to "no active team" after the ledger may have been unreadable or after worktree removal may have failed. Cleanup policy, external git state, durable metadata deletion, and active runtime state are split across layers with no commit boundary.

Observable bug or likely failure mode:

If `config.json` is corrupt, temporarily unreadable, or has a parse error, `CleanupTeamDirectories` silently skips member worktree cleanup and still removes `~/.pragma/teams/<team>`. If `git worktree remove --force` fails because the recorded `CWD` is wrong, the path is locked, the worktree has already been pruned differently, or git returns an error, the helper still deletes the team config and tasks directory. `TeamDelete` then clears `TeamContext` and emits a successful `TeamDeleted` event. The remaining worktrees are no longer associated with an active team and the durable record that named them has been deleted.

Minimal direction for fixing the boundary:

Make team deletion an authoritative team cleanup operation. It should fail or enter an explicit recoverable state when the team config cannot be read, propagate or record worktree removal failures, and only delete the team config and clear `TeamContext` after external worktree cleanup has succeeded or been deliberately marked abandoned. The same owner should coordinate the task-registry quiescence check, durable ledger consumption, and final state/event commit.

What not to do:

Do not patch this by logging ignored `git worktree remove` errors while still deleting the config. Logging loses the only retry authority. Also do not skip broken worktree paths and clear `TeamContext` anyway; that turns a cleanup failure into an orphaned filesystem state that no normal team command can recover.

## 91. RemoteTrigger Schema Forbids The Body Its Runtime Requires

Severity: medium

Concrete files/functions involved:

- `internal/tools/remote/remote.go`: `inputSchema`, `remoteInput`, `Tool.InputSchema`, `Tool.Invoke`
- `internal/remote/remote.go`: `Request`, `Service.Execute`, `Validate`
- `internal/provider/anthropic/remotetrigger.go`: `RemoteTriggerClient.Execute`, `requestParts`
- `internal/tool/orchestrator.go`: `Orchestrator.executeSingle`
- `internal/cli/tools.go`: `baseTools` RemoteTrigger registration

What responsibility is split or misplaced:

The provider-neutral remote trigger service accepts `remote.Request.Body` as `json.RawMessage`, validates that `create` and `update` include a body, and the Anthropic adapter sends that body directly as the HTTP request payload. The model-facing `RemoteTrigger` tool, however, advertises `body` as a JSON object with `additionalProperties: false` and no declared properties. Normal tool execution validates model tool input against this schema in `Orchestrator.executeSingle` before `Tool.Invoke` reaches the remote service.

Why this is wrong in ownership/lifecycle terms:

The tool schema is the runtime contract between the model and the domain operation. It should describe the same operation that the remote service can execute. Here the executable domain boundary and the advertised/validated model boundary are split: the service requires arbitrary provider-defined trigger JSON, while the tool descriptor forbids any useful trigger fields. The command registration simply wires the tool when `PRAGMA_FEATURE_REMOTE_TRIGGERS=1`, so there is no later owner that reconciles the mismatch.

Observable bug or likely failure mode:

A model call such as `{"action":"create","body":{"name":"nightly","prompt":"run checks"}}` is rejected by schema validation before permission checks or `remote.Service.Validate` run, because `body` contains properties that the schema disallows. Omitting `body` gets past the schema shape but then `remote.Validate` rejects `create` or `update` as missing a required body. The direct unit tests exercise `Tool.Invoke` with arbitrary bodies, but the normal orchestrated tool path cannot successfully express the same operation.

Minimal direction for fixing the boundary:

Make the RemoteTrigger descriptor derive or declare the same body contract that the remote service accepts. If trigger bodies are provider-specific, the schema should allow an object payload with provider-defined properties, or the remote service should expose typed trigger fields that the tool schema and provider adapter both use. Keep validation in one remote-trigger contract so the model-facing schema, service validation, permission subject, and provider request body cannot diverge.

What not to do:

Do not patch this by bypassing schema validation for RemoteTrigger or by telling the model to pass an escaped JSON string in another field. That hides a broken tool contract. Also do not only relax the schema to `additionalProperties: true` while leaving all semantic validation in the Anthropic adapter; the remote service should still own the provider-neutral validation boundary.

## 92. Auto-Compaction Summarizes The Provider-Request Projection And Commits It As Conversation Truth

Severity: high

Concrete files/functions involved:

- `internal/query/loop.go`: `Engine.runLoop`, `applyToolResultBudget`, `autoCompactBeforeRequest`
- `internal/toolresult/storage.go`: `ApplyToolResultBudget`, `replaceToolResultContents`, `persistAndBuildReplacement`
- `internal/compact/compact.go`: `Service.Compact`, `ApplyResult`
- `internal/query/persistence.go`: `ShouldPersistSessionEvent`
- `internal/session/writer.go`: session save path reached after `CompactionEvent`

What responsibility is split or misplaced:

`Engine.runLoop` first builds `messagesForQuery` from the durable conversation, then calls `applyToolResultBudget`. That budgeting step can persist large tool results to sidecar files and replace their message content with `<persisted-output>` previews for provider requests. The same budgeted `messagesForQuery` slice is then passed into `autoCompactBeforeRequest`. If compaction triggers, `compact.Service.Compact` summarizes those already-replaced messages, and `autoCompactBeforeRequest` calls `compact.ApplyResult(e.store, compResult)`, replacing the live conversation with the summary of the provider-request projection.

Why this is wrong in ownership/lifecycle terms:

Tool-result budgeting is a provider request projection: it decides what the next model call can afford to see. Conversation compaction is a durable conversation rewrite: it decides what history remains as session truth. Those two boundaries should not share the same mutated message slice as source of truth. The compaction owner is currently downstream of the request-budgeting owner, so a lossy transport projection can become a committed conversation summary.

Observable bug or likely failure mode:

A conversation with a large Bash, grep, or other tool result can have the full output persisted to `~/.pragma/sessions/<session>/tool-results/...` while `messagesForQuery` contains only a `<persisted-output>` tag and preview. If the same turn crosses the auto-compaction threshold, the compaction prompt sees the tag and preview, not the original tool result. `ApplyResult` then replaces the session conversation with a compact summary based on that elided view, and `CompactionEvent` causes normal session persistence to save the rewritten conversation. The durable summary can say only that a large output existed and include the small preview, while losing facts that were present in the actual tool result sidecar.

Minimal direction for fixing the boundary:

Run durable compaction from a conversation-owned source, not from the provider-request projection. Either compact before applying request-only tool-result budgeting, or make compaction explicitly resolve persisted tool results from the session artifact owner when it needs to summarize them. The commit step should record which source was summarized and only replace conversation state after the session owner has a complete compaction transaction.

What not to do:

Do not patch this by increasing `MaxToolResultsPerMessageChars` or by adding a warning to the compaction prompt about `<persisted-output>` tags. That leaves durable session rewriting dependent on a lossy provider projection. Also do not make `compact.Service` ad hoc-read sidecar paths from the tag string; artifact resolution belongs to the session/tool-result owner, not to the summarizer prompt helper.

## 93. SelfTrace Uses A Smaller Ad Hoc Log Reader Than The Observe Log Format Requires

Severity: medium

Concrete files/functions involved:

- `internal/tools/selftrace/selftrace.go`: `Tool.Invoke`, `invokeQuery`, `invokeSummary`
- `internal/cli/deps.go`: `SetupDeps` per-execution JSONL logger setup
- `internal/observe/logger.go`: `Logger.writeJSON`
- `internal/observe/event_catalog.go`: `APIRequestStarted`, `ToolCallReceived`, `ToolExecutionStarted`, `ToolExecutionCompleted`, `APIRequestCompleted`
- `internal/observe/replay.go`: `LoadReplay`

What responsibility is split or misplaced:

`SetupDeps` installs a per-execution JSONL logger at `LevelTrace`, and the observe event schema includes full request and tool payload fields: `APIRequestStarted.Messages`, `SystemPrompt`, `Tools`, `ResponseSchema`, tool call inputs, tool execution inputs and outputs, and API response content. `SelfTrace` then implements its own log reader by opening `t.LogFilePath`, scanning with a 1 MB maximum token size, unmarshalling each line, and ignoring both malformed lines and `scanner.Err()`. The replay loader for the same observe event format uses a 64 MB scanner buffer and returns scanner errors.

Why this is wrong in ownership/lifecycle terms:

The observe JSONL format is an observability artifact with payload sizes defined by runtime events, not by the SelfTrace tool. A model-facing inspector should consume the same log parsing contract as replay/debug tooling, or delegate to an observe-owned reader. Instead, SelfTrace has a separate partial parser with a smaller size limit and no error reporting, so the log format has different effective limits depending on which consumer reads it.

Observable bug or likely failure mode:

A large model request, large tool input, large tool output, or structured response can produce a single JSONL event larger than 1 MB because `Logger.writeJSON` writes the event payload as one line. When SelfTrace reaches that line, `bufio.Scanner` returns `ErrTooLong`; `invokeQuery` and `invokeSummary` do not check `scanner.Err()`, so they return a partial page, a partial summary, or "No events match the query" with no indication that the log scan stopped early. Replay over the same log can still succeed because `observe.LoadReplay` uses a much larger buffer and reports scanner errors.

Minimal direction for fixing the boundary:

Move observe-log reading behind one observe-owned JSONL reader used by SelfTrace, replay, and any future inspectors. That reader should define the supported maximum line size, surface parse and scanner errors, and decide whether to skip individual malformed events or fail the query. SelfTrace should format/filter events after receiving a trustworthy stream, not own the low-level log format.

What not to do:

Do not patch this only by changing SelfTrace's buffer from 1 MB to 64 MB. That keeps a second log parser that can drift again. Also do not continue silently ignoring scanner errors; an observability tool that cannot read its own source must report that the trace is incomplete.

## 94. Raw HTTP Capture Has No Completion Contract For Replay Evidence

### Source

- `README.md`: benchmark output documents `raw-http-pragma/` as the raw HTTP capture directory for Pragma runs
- `docs/prompt-control-ab-tests.md`: raw HTTP replay guidance uses `pragma replay raw-http audit --require-responses` to ensure usable `response.raw` evidence
- `internal/provider/openai/provider.go`: `New`, `Complete`, `Stream`
- `internal/provider/lilac/provider.go`: `New`, `Complete`, `Stream`
- `internal/provider/rawcapture/rawcapture.go`: `HTTPClientFromEnv`, `Transport.RoundTrip`, `recordingBody.Read`, `recordingBody.Close`, `writeJSON`, `writeFile`
- `cmd/pragma/replay_raw_http.go`: `auditRawHTTPReplayEvidence`, `classifyRawHTTPResponseEvidence`, `dumpRawHTTPCaptures`
- `cmd/pragma/inspect.go`: `loadInspectRawHTTPTurn`

### What The Code Does

The benchmark-facing docs describe `raw-http-pragma/` as a run artifact, and the raw HTTP replay tooling consumes `request.json` and `response.raw` as replay/debug evidence. OpenAI and Lilac providers enable capture by swapping in `rawcapture.HTTPClientFromEnv`, then attach trace IDs before provider requests. `rawcapture.Transport.RoundTrip` writes a numbered directory with request payload, request metadata, response headers, `response.raw`, and `response.meta.json`.

The capture writer is best-effort. If the capture directory cannot be created, the transport silently performs the live request without capture. Request and metadata writes go through helpers that discard `os.WriteFile` errors. If opening `response.raw` fails, the response is returned without capture. While the response body is read, each chunk write ignores file write errors; `Close` ignores `response.raw` close errors and writes completed metadata anyway. If the body is not closed, the final `completed_at`, byte count, and hash are never written.

The replay/audit side does not enforce a completion contract. `auditRawHTTPReplayEvidence` treats a case as usable when `response.raw` exists and `classifyRawHTTPResponseEvidence` returns `response_ok`; it does not require `response.meta.json.completed_at`, compare `response_bytes`, or validate the stored hash. `dumpRawHTTPCaptures` reads request/response metadata opportunistically and ignores metadata read failures for the exported index. `inspect raw-http` also treats metadata as optional and falls back to file size when response metadata is absent.

### Why It Is A Boundary Problem

Raw HTTP capture is written by a provider HTTP transport, but consumed later as a run-level replay artifact. Those are different reliability contracts. A transport-level debug hook can be best-effort, but a replay/audit evidence store needs an explicit completed/incomplete state and integrity signal. Right now the writer can silently produce missing, partial, or metadata-inconsistent captures, and the reader can still report those captures as usable evidence because existence and parseability of `response.raw` are the only real gate.

This is independent from the session JSONL replay/export issues. The artifact format here is not the observe replay log and not the session store; it is a separate provider-transport capture format that benchmark docs and raw-http commands already treat as authoritative enough for replay experiments.

### Failure Mode

During a long streaming response, cancellation, disk-full, permission loss, or a missed `Body.Close` can leave `response.raw` truncated or leave `response.meta.json` without final completion data. The audit command can still pass if the truncated bytes are non-empty SSE or parse as JSON with choices. A benchmark or prompt-control report can then document a replay batch as having usable responses even though the capture writer never committed that response as complete.

### Correct Fix Shape

Make raw HTTP capture an explicit artifact writer with a completion manifest or atomic finalization step. The writer should record incomplete captures when directory creation, request writes, raw response writes, close, or metadata finalization fails. Replay, audit, dump, and inspect should use the same capture reader and require completed metadata plus byte/hash agreement when `--require-responses` asks for usable evidence. Provider transport code should stream bytes into that writer, not decide artifact validity itself.

### What Not To Do

Do not only make `writeFile` return errors while leaving replay/audit to classify raw bytes directly. That improves one write path but still leaves no shared definition of a complete capture. Also do not hide incomplete captures by deleting their directories on failure; for debugging, incomplete artifacts are useful if they are explicitly marked incomplete.

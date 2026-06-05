
Findings

1. Interactive startup fires session lifecycle before any user action
   Severity: high
   Files/functions: internal/cli/deps.go:149, internal/cli/deps.go:339, internal/cli/run.go:351, internal/cli/run.go:274, internal/cli/run.go:750
   Misplaced responsibility: BuildInteractiveRuntime emits SessionStarted and runs SessionStart hooks while merely launching the web UI. The actual session file is only created later by
   startSessionForCurrentConversation on prompt/orchestration.
   Why wrong: UI startup is not a domain session. Hooks and observe events now mean “browser opened”, while persistence means “first real run happened”.
   Failure mode: hooks can mutate/check the workspace before the user does anything; logs/replays can contain phantom session starts; a hook receiving the session id may refer to a session file that
   does not exist yet.
   Fix direction: move SessionStarted/SessionStart to the first domain action that starts an engine/orchestration or resumes an existing session. Use a separate UI/server-start event if needed.
   Do not: add empty-session cleanup or filter phantom sessions in listing/replay.

2. Session persistence policy is split across CLI loops instead of owned by runtime/session
   Severity: high
   Files/functions: internal/cli/run.go:282, internal/cli/run.go:627, internal/cli/subcommands.go:142, internal/cli/run.go:886
   Misplaced responsibility: interactive runtime, non-interactive mode, and slash subcommands each decide when to call sessionSaveFn.
   Why wrong: persistence is derived from presentation/subcommand event loops rather than from domain mutations or one canonical runtime boundary.
   Failure mode: new LoopEvent types will silently not persist unless every consumer updates its save switch; prompt history is interactive-only; metadata can be written as a generic safety net on
   request/response/tool events.

3. Prompt slash subcommands duplicate non-interactive execution
   Severity: high
   Files/functions: internal/cli/subcommands.go:55, internal/cli/subcommands.go:62, internal/cli/subcommands.go:145, internal/cli/run.go:556
   Misplaced responsibility: RunPromptCommand rebuilds deps, hooks, tools, compaction, session start/save, and event consumption instead of delegating to the standard non-interactive executor with an
   injected prompt.
   Why wrong: command translation and prompt execution are separate responsibilities. The subcommand layer now owns part of engine lifecycle.
   Failure mode: permission defaults differ: RunNonInteractive bypasses permissions when --permission-mode is not set, while RunPromptCommand does not and only adds per-command allowed-tool rules.
   Structured-output/background behavior can also drift.
   Fix direction: have slash subcommands produce a prompt/result, then call one shared non-interactive run path.
   Do not: patch parity by copying more branches from RunNonInteractive into RunPromptCommand.

4. Runtime event package carries UI-specific slash and display concepts
   Severity: medium
   Files/functions: internal/query/event.go:7, internal/query/event.go:24, internal/query/event.go:62
   Misplaced responsibility: query imports slash so SlashResultEvent can carry OpenModelPicker, Quit, OpenResumePicker, etc. ToolResultEvent.Display is documented as TUI-only.
   Why wrong: query is the engine/runtime event surface; it should not expose presentation commands or TUI rendering payloads.
   Failure mode: every query consumer inherits UI semantics; web and TUI must branch around UI-only event contents; future runtime consumers can accidentally depend on display fields instead of domain
   state.
   Fix direction: keep engine events domain-only. Put interactive UI command results in an interactive-runtime/web/TUI adapter stream outside query, or expose a presentation-neutral command result
   type from the interactive layer.
   Do not: make the engine understand slash commands to “centralize” this.

5. Model switching is controlled both by slash runtime and TUI UI callbacks
   Severity: medium
   Files/functions: internal/slash/commands.go:300, internal/slash/commands.go:350, internal/tui/handlers.go:576
   Misplaced responsibility: /model <name> updates runtime state in slash handling, while the TUI model picker directly mutates Store and invokes OnModelChanged.
   Why wrong: model selection is a runtime state transition, not a dialog callback side effect.
   Failure mode: picker behavior and slash behavior can diverge on validation, token budget updates, confirmation text, or future provider changes.
   Fix direction: route picker selection through the same runtime model-change path used by /model.
   Do not: add toolbar sync or extra UI-side validation as a repair layer.

6. Orchestration uses global mutable artifacts and the web UI reconstructs workflow state
   Severity: medium
   Files/functions: internal/orchestration/runner.go:17, internal/orchestration/runner.go:77, internal/orchestration/runner.go:121, cmd/pragma/orchestration.go:72, internal/web/web.go:1457
   Misplaced responsibility: orchestration coordination artifacts default to global /tmp/pragma, and EnsureRunDirs removes the shared handoff directory. Separately, web derives current workflow state
   by parsing event type strings and field aliases.
   Why wrong: orchestration owns run identity, state, transitions, and artifacts. UI should render a typed projection, not rebuild FSM state from a lossy event log.
   Failure mode: concurrent or sequential orchestration runs can stomp handoff files; UI workflow display can become wrong if event naming/casing changes or an event is missed.
   Fix direction: scope artifact root by session/run id and have orchestration emit a typed run snapshot/projection.
   Do not: hide this with web-side artifact allowlists or JavaScript field-name fallbacks.


      Additional source-backed findings from the continued pass:

7. User prompt hooks are owned by TUI, so web bypasses them
   Misplaced responsibility: UserPromptSubmit hook execution lives inside TUI submitPrompt, not the interactive runtime. Web submits directly to RunInput.
   Why wrong: prompt submission policy is runtime/lifecycle behavior, not presentation behavior.
   Failure mode: a hook that blocks or rewrites user prompt submission works in TUI but is bypassed in web interactive mode.
   Fix direction: run prompt-submit hooks inside the shared interactive runtime before runEngine or slash execution.
   Do not: add another hook call in internal/web; that preserves the split.

8. Subagent task lifecycle is split across Agent tool, task registry, and query engine
   Severity: high
   Files/functions: internal/tools/agent/agent.go:213, internal/tools/agent/agent.go:315, internal/tools/agent/agent.go:428, internal/tools/agent/agent.go:524, internal/task/registry.go:135, internal/
   Fix direction: one agent runner or registry-owned transition path should consume engine events and apply task state transitions consistently.
   Do not: patch individual missing TaskFailed updates in each loop.

9. Lifecycle execution has three divergent runners
   Severity: high
   Files/functions: cmd/pragma/lifecycle.go:99, cmd/pragma/lifecycle.go:142, internal/tools/lifecycle/lifecycle.go:109, internal/query/engine.go:183, internal/tools/agent/agent.go:742
   Misplaced responsibility: CLI lifecycle, LifecycleRun tool, Engine.RunGraph, and Agent structured runs each build initial lifecycle state and consume lifecycle events.
   Why wrong: graph execution is domain/runtime behavior; CLI/tool/subagent entrypoints should not each encode system prompt, tools, reducers, and final result extraction.
   Failure mode: CLI lifecycle uses a hard-coded system prompt; LifecycleRun uses active conversation system; Engine.RunGraph uses the engine store. Same graph/prompt can run with different system/
   tool state.
   Fix direction: move graph initial-state construction and result projection behind one lifecycle runner path.
   Do not: copy missing system-prompt fields between the three runners.

10. Permission “remember” rule scope is decided by UI implementations
    Severity: medium
    Files/functions: internal/permission/prompter.go:10, internal/tui/permission.go:100, internal/web/web.go:232, internal/tool/orchestrator.go:359
    Misplaced responsibility: Prompter returns a full permission.Rule, so TUI and web construct policy. TUI remembers a tool-wide allow rule; web remembers toolName + content.
    Why wrong: UI should collect user intent, not decide permission rule semantics.
    Failure mode: “Yes, for this session” has broader scope in TUI than web for the same tool prompt.
    Fix direction: have UI return decision plus remember intent; permission/orchestrator constructs the session rule from the original CheckResult.
    Do not: make web mimic TUI or TUI mimic web in presentation code.

11. Cleanup removes agent worktrees as a hidden completion side effect
    Severity: medium
    Files/functions: internal/tools/agent/agent.go:382, internal/tools/agent/agent.go:408, internal/tools/agent/agent.go:723
    Misplaced responsibility: task completion/error cleanup directly removes worktrees if they appear unchanged.
    Why wrong: cleanup is mutating external/domain artifacts after execution, not just releasing runtime resources.
    Failure mode: an isolated agent run can erase its inspectable workspace on success or failure when no diff remains; removal errors are ignored, so task state can claim completion while cleanup
    silently failed.
    Fix direction: make artifact retention/removal an explicit worktree/task policy owned outside the result-draining loops.
    Do not: add more “empty” checks or ignored cleanup calls in more exit branches.



12. Background registry writes process metadata before a real session exists
    Severity: medium
    Files/functions: internal/cli/run.go:194, internal/background/info.go:17, internal/background/subscriber.go:24
    The parent process creates ~/.pragma/active-sessions/{pid}.json with SessionID: "" immediately after os.StartProcess. The child later emits SessionStarted, but the background subscriber only updates
    status, not the session identity. This splits process-launch state and domain session state.
    Failure mode: pragma sessions shows a live “session” that cannot be mapped back to the real persisted conversation; a child that fails before session creation still leaves valid-looking process
    metadata until it exits.
    Minimal fix: either make the registry explicitly process-only, or have the child register/update the background record after the session is created. Do not parse logs or copy the prompt into a fake
    session identity.

13. Cron jobs are persisted, but no runtime owner starts the scheduler
    Severity: high
    Files/functions: internal/cron/scheduler.go:199, internal/cli/deps.go:370, internal/tools/cron/create.go:137
    CronCreate persists jobs through Scheduler.Create, and NewScheduler loads durable jobs, but code search only found the Scheduler.Start definition, not a caller. The tool advertises scheduled
    Minimal fix: put cron execution under one real runtime/daemon owner that starts Scheduler.Start and routes fired prompts through the canonical prompt execution path. Do not start a ticker inside
    CronCreate or unconditionally in SetupDeps; that would make arbitrary CLI commands become schedulers.

14. Tool-result persistence is outside the session writer transaction
    Severity: medium
    Files/functions: internal/tool/orchestrator.go:461, internal/query/loop.go:416, internal/toolresult/storage.go:401, internal/cli/deps.go:390
    Large tool outputs are written directly under sessions/{sessionID}/tool-results, while the JSONL session writer separately records replacement metadata. The callback in SetupDeps ignores
    WriteContentReplacement errors.
    Failure mode: a persisted output file can exist without a matching session JSONL replacement record, or a replacement record can fail silently after the model-visible content was changed. Resume then
    depends on partial side effects rather than a single session persistence boundary.
    Minimal fix: make session persistence own the artifact-plus-replacement write as one operation, or return a durable artifact record that the writer commits. Do not repair this by scanning tool-results
    directories on resume.

15. MCP OAuth tool mutates MCP lifecycle after the tool call has returned
    Severity: high
    Files/functions: internal/tools/mcpauth/mcpauth.go:70, internal/tools/mcpauth/mcpauth.go:123, internal/tools/mcpauth/mcpauth.go:137, internal/mcp/manager.go:541
    The pseudo-tool starts a callback server on context.Background(), then later saves tokens, reconnects the MCP manager, unregisters the auth tool, and registers real tools. That is runtime lifecycle
    work hidden behind a tool invocation.
    Failure mode: after the user cancels or the session cleanup calls DisconnectAll, the OAuth goroutine can still complete and mutate token storage/tool registry.
    Minimal fix: MCP manager should own OAuth flow lifetime under the same root/session context used for MCP connections. The tool should request/start auth through the manager and return the URL. Do not
    add another “is session active?” guard inside the callback.

16. Forked skills implement another sub-agent event loop
    Severity: medium
    Files/functions: internal/tools/skill/skill.go:156, internal/tools/skill/skill.go:173, internal/tools/agent/agent.go:428
    Forked skills create a forked conversation, call engine.Run, consume query events themselves, emit SubAgentSpawned, and synthesize a final tool result. That duplicates the sub-agent execution pattern
    already in the Agent tool, but without task registry ownership, cancellation controls, or the same event handling.
    Failure mode: forked skills can lose non-text events and have different cancellation/progress/accounting behavior from other sub-agent runs.
    Minimal fix: route forked skill execution through the existing sub-agent/task execution owner, with skill content as the prompt/config. Do not patch more event cases into skill.invokeForked.

17. Team delete checks stale config state while live teammate state lives in task registry
    Severity: high
    Files/functions: internal/tools/teamdelete/teamdelete.go:89, internal/tools/teamdelete/teamdelete.go:106, internal/tui/teams.go:74, internal/team/team.go:139
    TeamDelete decides whether teammates are active from config.json member IsActive, but the TUI reads live teammates from task.Registry. Search shows production WriteTeamFile only in team creation, so
    runtime teammate activity is not maintained in the team file.
    Failure mode: cleanup can be blocked forever by stale nil/active config members, or allow cleanup while real task-registry teammates still exist if the config drifts.
    Minimal fix: make task registry, or a team runtime owner backed by it, the authority for active teammate lifecycle. The team file should be durable metadata, not the active-state gate. Do not “fix”
    this by sprinkling more WriteTeamFile calls from agent goroutines.

18. TUI directly controls task lifecycle
    Severity: medium
    Files/functions: internal/tui/teams.go:45, internal/tui/teams.go:135, internal/tui/teams.go:152, internal/tools/taskstop/taskstop.go:67
    The teams dialog stores a *task.Registry, mutates ShutdownRequested, calls NotifyTask, and directly cancels tasks. Separately, TaskStop also cancels tasks as a tool. Presentation is now a lifecycle
    controller.
    Failure mode: TUI-only controls can diverge from tool/web behavior, and shutdown semantics are encoded in key handlers instead of the task/team lifecycle layer.
    Minimal fix: expose one runtime command/service for teammate shutdown/kill and have TUI, web, and tools call it. Do not duplicate the same registry mutations in each interface.


19. Resume swaps conversation state without changing the runtime provider
    Severity: high
    Files/functions: internal/cli/run.go:303, internal/cli/run.go:322, internal/query/loop.go:127, internal/cli/tools.go:181
    InteractiveRuntime.Resume loads a saved conversation and sets AppState.Model from sess.Conversation.Model, but it never updates AppState.Provider, Deps.Cfg.Provider, or the already-constructed
    provider instance in query.Engine. The next request uses snap.Model as the model ID, but still sends it through the old provider object.
    Failure mode: resuming a Google session while running an Anthropic provider can display/use a Gemini model ID against the Anthropic client.
    Minimal fix: resume must be owned by the same runtime boundary that owns provider/model selection, rebuilding or switching the provider consistently. Do not patch the toolbar or stored conversation
    only.
    Failure mode: hooks after resume receive the abandoned session ID while persistence writes to the resumed session file; observability has no lifecycle event explaining the switch.
    Minimal fix: make resume a session lifecycle transition in the runtime/session owner. Do not let each UI repair its display after a silent writer swap.

21. Slash commands mutate conversation state outside the session persistence trigger
    calls command by command.

22. Recording and deterministic replay use incompatible persistence contracts
    Severity: high
    Files/functions: internal/cli/deps.go:177, internal/observe/recorder.go:27, internal/observe/replay.go:21, internal/provider/replay/provider.go:130, cmd/pragma/replay.go:151
    --record subscribes a recorder that writes one pragma-recording.jsonl. LoadReplay expects a directory containing events.jsonl plus optional tool-outputs/ and api-responses/. The replay provider pulls
    deterministic responses only from api-responses/turn-N.json, even though recorded APIRequestCompleted events can contain response content.
23. Manual and auto compaction duplicate state replacement policy
    Severity: medium
    Files/functions: internal/query/loop.go:437, internal/query/loop.go:489, internal/slash/commands.go:177, internal/slash/commands.go:184
    Auto-compaction and /compact both call Compactor.Compact and both directly replace Conversation.Messages. The service explicitly says callers apply state, so the actual mutation policy is split
    between query loop and slash command handling.


Severity: high
Files/functions: internal/tools/config/settings.go:42, internal/tools/config/config.go:147, internal/config/config.go:36, internal/cli/deps.go:220
Responsibility split: the model-facing Config tool owns its own setting schema instead of using the runtime config contract. It exposes permissions.defaultMode, writes dotted paths into JSON, and
accepts plan; the runtime expects permission_mode plus permissions as an array, and accepts bypassPermissions instead of plan.
Observable failure: asking the tool to set permissions.defaultMode can write a permissions object into .pragma/settings.json, after which config.Load tries to unmarshal that object into
[]RawPermission and startup/config load can fail.
Minimal fix: make config setting names and enums come from the runtime config package, or explicitly map the tool setting to permission_mode.
What not to do: do not add another compatibility branch inside the CLI parser while leaving the tool’s independent schema in place.
Files/functions: internal/permission/persist.go:17, internal/observe/event_catalog.go:451, internal/tool/orchestrator.go:365, internal/permission/rulechecker.go:120
Responsibility split: persistent permission rules and PermissionPersisted are defined, but the runtime prompt path only calls AddSessionRule. PersistRule has tests and storage code, but no production
caller.
Observable failure: “remember” / “always allow” decisions are session-only even though there is a local settings persistence API and event type suggesting durable behavior.
Minimal fix: the permission runtime should own session-vs-local persistence and emit one persistence event when it writes a local rule.
What not to do: do not make TUI/web directly call PersistRule; that would deepen the UI/runtime split.

Severity: medium
Files/functions: internal/tool/orchestrator.go:332, internal/tool/orchestrator.go:370, internal/tool/orchestrator.go:380, internal/observe/auditor.go:37
Responsibility split: permission outcome is emitted as separate “checked”, “prompted”, and “denial enforced” events, and the auditor tries to rebuild a final decision. On denial it appends a second
audit entry instead of updating the checked entry. Hook blocks emit denial without a preceding permission check.
Observable failure: the audit trail can contain two entries for the same tool call, with denial entries losing rule/source/user-decision context. Violations() only sees synthetic deny entries, not a
single authoritative permission decision.
Minimal fix: emit one final permission decision event from the orchestrator after hooks, rule checks, prompts, and enforcement are resolved.
What not to do: do not add more auditor heuristics keyed by call ID; the audit layer should not infer domain state.

27. SendMessage Owns Dead-Agent Reaping
    Observable failure: attempting to message a stale agent mutates lifecycle state as a side effect of a communication tool, producing a different failure path than normal reaping.
    Minimal fix: move deliverability into the registry/task service. SendMessage should ask for a delivery result, not directly transition task status.
    What not to do: do not duplicate the same timeout check in more tools that touch tasks.

28. Remote Trigger Tool Owns Provider-Specific Agent Lifecycle
    events.
    Minimal fix: keep the HTTP client behind a provider adapter, but route lifecycle-visible operations through the existing runtime owner for tasks/scheduled work.
    What not to do: do not patch this by teaching the UI to parse RemoteTrigger HTTP output into task state.


29. Early Error Paths Leave Session State Half-Persisted
    Severity: high
    Files/functions: internal/cli/run.go:274, internal/cli/run.go:782, internal/query/loop.go:81, internal/query/loop.go:161
    Responsibility split: the CLI decides when to save from emitted loop events, while the engine mutates conversation state before emitting some terminating errors. runEngine creates the session and
    writes prompt history before Engine.Run; the engine appends the user message, but context-overflow and cancellation can emit ErrorEvent before any save-triggering event.
    Observable failure: a failed turn can leave a session file with header/prompt history but no corresponding user message or metadata.
    Minimal fix: make the runtime/session boundary persist accepted conversation mutations transactionally, including terminal errors, instead of relying on UI/CLI event filters.
    What not to do: do not just add ErrorEvent to shouldSaveOnEvent; that still leaves persistence coupled to presentation event handling.

30. Conversation Events Are Defined But Not Emitted By The Conversation Owner
    Severity: medium
    Files/functions: internal/model/conversation.go:38, internal/observe/event_catalog.go:22, internal/observe/metrics.go:95, cmd/pragma/replay.go:257
    Responsibility split: Conversation.Append owns message mutation, but it emits no MessageAppended event. Meanwhile metrics, replay, selftrace, and logger code define and consume MessageAppended as if
    it were authoritative. Current production search only finds test construction of these events.
    Observable failure: event-derived turn counts and replay prompt extraction do not reflect real conversation appends. Selftrace’s session topic advertises message events that normal runs do not
    produce.
    Minimal fix: move message append event emission to the runtime path that owns conversation mutation, or delete the event consumers and use the session/conversation store as the source of truth.
    What not to do: do not sprinkle MessageAppended emits at individual UI or command call sites.

31. Hook Feedback Contract Is Collected And Then Dropped
    Severity: medium
    Files/functions: internal/hook/hook.go:54, internal/hook/manager.go:135, internal/tool/orchestrator.go:294, internal/tui/handlers.go:766, internal/cli/run.go:357
    Responsibility split: hook JSON declares additionalContext as “shown to model”, and Manager.Execute aggregates Feedback and Stdout. But production callers either only check Blocked or discard the
    result entirely. PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, and SessionEnd do not route feedback into the runtime conversation.
    Observable failure: hooks can appear to succeed and return structured context, but the model never sees it. SessionStart stdout is likewise dropped despite the AggregatedResult contract saying it is
    shown to the model.
    Minimal fix: one runtime owner should define which hook outputs are semantic inputs and inject them into the conversation/request path consistently.
    What not to do: do not have each UI decide how to splice hook feedback into prompts.



1. Lifecycle executor has two owners for the same state machine.
   Run and Stream each implement their own execution loop, update application, checkpointing, completion handling, and transition resolution: internal/lifecycle/executor.go:60, internal/lifecycle/
   executor.go:120. Transition logic is also duplicated between resolveNextNodes and resolveNextNodesWithEvents: internal/lifecycle/executor.go:249, internal/lifecycle/executor.go:298. Production
   paths use Stream, while many lifecycle tests exercise Run, so semantic fixes can land in one path and miss the other. The fix should be one execution core, with Run draining it and Stream exposing
   it.

2. Lifecycle execution owns a parallel conversation blackboard, then projects only final output back to query events.
3. Graph semantics are split between prompt guidance, generated-only repair, and structural validation.
   The generation prompt says tool-using graphs must route LLM nodes through tools via stop_reason: internal/lifecycle/bridge/generate.go:66. GenerateGraph then applies FixLLMToolRouting: internal/
   lifecycle/bridge/generate.go:253. That repair itself documents that missing routing silently discards tool calls: internal/lifecycle/definition/fixup.go:8. But YAML-loaded graphs go straight
   through parse/resolve without the fixup: cmd/pragma/lifecycle.go:238, and parser validation is only structural node/edge checking: internal/lifecycle/definition/parse.go:44. The canonical graph
   definition layer should own this validation or normalization for all graph sources.
   ApplyPatch, legacy apply patch, Bash, and NotebookEdit: internal/cli/tools.go:257. The execution side already has FileStateCache flowing through lifecycle tool execution: internal/lifecycle/bridge/
   tool_node.go:42, and tools like apply-patch/notebook edit record state there: internal/tools/applypatch/applypatch.go:614, internal/tools/notebookedit/notebookedit.go:407. File-effect receipts
   should come from the tool execution boundary, not another tool-name parser.


1. Slash command grammar is duplicated in both UI completers
   Severity: medium
   Files/functions: internal/slash/orchestrate.go:30, internal/web/web.go:531, internal/tui/input.go:448
   The real /orchestrate parser owns --persona-dir, --prompt, trailing prompt handling, and validation. Web and TUI each reimplement the argument state machine and filesystem path role logic for
   completions. This is wrong because command grammar now has three owners. Likely failure: a new flag or changed positional rule works at execution time but one or both interactive UIs suggest
   invalid completions. Minimal fix: move command completion metadata/parsing help to the slash command layer and have UIs render it. Do not patch this by updating the two UI copies in parallel.

2. Session discovery is UI-owned while resume mutation is runtime-owned
   Severity: medium
   Files/functions: internal/slash/resume_cmd.go:25, internal/tui/handlers.go:128, internal/web/web.go:876, internal/cli/run.go:303
   /resume with no args returns OpenResumePicker, then TUI and web directly list sessions from SessionStore; only the final selected ID goes through InteractiveRuntime.Resume. Session discovery/
   filtering/picker data is split across slash, TUI, web, and runtime. Likely failure: TUI filters to cwd, web lists all sessions, slash prefix matching follows a third path. Minimal fix: make session
   browsing/resume selection a runtime/session command result with a single candidate contract. Do not add more UI-specific session filters.

   WritePromptHistory because it uses runOrchestration, not runEngine. Likely failure: rejected web prompts appear in history, orchestration prompts do not persist as prompt history, and reload/resume
   shows a different history from the in-memory UI. Minimal fix: treat accepted user submissions as a runtime event and derive all UI history from that. Do not hide this with client-side dedupe or
   another fallback extraction pass.

4. Web event API has no owned wire schema
   type, changing a field, or adding JSON tags silently breaks the web UI. Minimal fix: define explicit web/event DTOs at the runtime/event boundary and make the browser consume stable event kinds and
   fields. Do not add more fieldAny aliases or type-name substring checks.


      internal/tool/orchestrator.go:294, internal/hook/manager.go:121, internal/observe/auditor.go:61

PreToolUse hooks run before desc.CheckPerm. If a hook blocks, the hook manager emits HookExecuted/HookBlocked, but the orchestrator also emits PermissionDenialEnforced and returns a tool error before
the permission checker ever runs.

Observable failure mode: permission audit/selftrace shows a permission denial for something that was actually a hook block. Auditors cannot distinguish configured permission policy from arbitrary hook
control flow.

Minimal direction: let hook blocking remain a hook/tool-execution outcome, or add a distinct tool-blocked event if needed. Reserve PermissionDenialEnforced for denial by the permission checker or user
permission prompt.

Files/functions:
internal/observe/event_catalog.go:293, internal/cli/run.go:881, internal/tools/selftrace/selftrace.go:299, internal/observe/metrics.go:99

SessionSaved and SessionEnded are first-class events. Logger, replay formatting, selftrace session topics, and metrics all know about them. But the actual save/close path in makeSessionSaveClose

Observable failure mode: selftrace --topic session can show SessionStarted without save/end boundaries, and metrics only close session duration if a SessionEnded event arrives, which this path never
sends.

Minimal direction: have the session save/close owner emit SessionSaved and SessionEnded after successful writes/closes with the real counters it already computes, or remove the dead event surface and



High: active model changes are not persisted into session identity

Files/functions:
/model updates AppState.Model, and the engine uses that live field for the next request. But session identity uses immutable header data: load/list derive Conversation.Model and SessionSummary.Model
from the original header. makeSessionSaveClose writes metadata, not model changes.

Responsibility is split between runtime request state and session persistence. The session store claims a model for the session that may no longer be the model actually used.

Observable failure mode: a session can run later turns on model B while pragma sessions, resume metadata, and loaded Conversation.Model still say model A.

Minimal direction: persist model/provider changes as explicit session events or mutable metadata owned by the session writer, then load/list from the latest value.

Files/functions:
internal/tools/config/settings.go:14, internal/tools/config/config.go:147, internal/tools/config/config.go:171, internal/cli/run.go:405

/model validates through ContextWindowFunc and updates TokenMonitor through OnModelChanged. The Config tool also changes the live model by syncing model to AppState.Model, but it only writes the JSON
settings file and mutates the store. It has no provider/model lister, no context-window validation, and no token budget update.

Minimal direction: route all active model changes through one runtime-owned model switch path, and let Config either persist default config only or call that same path when changing the live session.

What not to do: duplicate ContextWindowFunc and token monitor callbacks inside the Config tool.

Medium: scoped sub-agent registries drop compiled tool schemas

engineFactory registers base tools, then applies subRegistry.Scoped(scopedToolNames) for sub-agents and forked skills. Registry.Scoped copies tools and hidden flags, but not schemas. The orchestrator
validates input only when GetSchema returns a schema.

The registry owns tool descriptors and compiled schemas, but scoping only preserves half of that contract.


51. Worktree tools promise session CWD transitions but only mutate git state

EnterWorktree tells the model it “switches the current session” and “switches the session’s working directory” in internal/tools/worktree/enter.go:55, but Invoke only runs git worktree add and returns
JSON with the path in internal/tools/worktree/enter.go:107. The tool is registered bare in internal/cli/tools.go:285, with no store/session dependency.
internal/tools/fileedit/fileedit.go:226 and internal/tools/filewrite/filewrite.go:138.

But the cache is created fresh per query.Engine in internal/query/engine.go:95, then passed to tools through progressSnapshot in internal/query/loop.go:883. Session load restores messages/model/
workdir, not file freshness state, in internal/session/store.go:134. Runtime resume swaps in the loaded conversation in internal/cli/run.go:303, but it does not reconstruct or persist the file-state
cache.


53. TUI bypasses the shared interactive runtime for slash execution

Severity: high

Files/functions:
internal/cli/run.go:228, internal/web/web.go:412, internal/tui/handlers.go:60, internal/tui/handlers.go:186

InteractiveRuntime.RunInput owns prompt-vs-slash dispatch for web. Web posts input into RunInput, so slash side effects go through rt.runSlash: clear conversation, resume, orchestration, injected
prompt execution, and save behavior. TUI receives the same RunInput, but still parses and executes slash commands itself through slashCmds.Execute, then separately handles query.SlashResultEvent if it
came from runtime.

That means the slash state machine is split between runtime and TUI. Observable failure: TUI slash behavior can drift from web slash behavior. /resume, /reset, orchestration, and injected prompt
commands depend on whether the input path used TUI-local slash execution or runtime slash execution.

Minimal direction: TUI should submit slash input through RunInput and render SlashResultEvent; keep picker/modals as presentation responses only. Do not fix this by copying more rt.runSlash branches
into TUI.

Severity: high

Files/functions:
internal/session/store.go:134, internal/cli/deps.go:314, internal/cli/deps.go:356, internal/cli/run.go:322, internal/tui/resumedlg.go:76, internal/web/web.go:881

Session load preserves Conversation.WorkDir, but initial --resume seeds AppState.CWD from the current process cwd, not sess.Conversation.WorkDir. In-process resume replaces Conversation and
HandoffState, but never updates AppState.CWD. The TUI picker can toggle to all sessions, and web lists all sessions with no directory filter, so cross-directory resume is possible.

The runtime therefore displays/resumes one session while tools execute in another directory. This is a persistence boundary bug: the persisted session owns the working directory, but execution uses
ambient process state.

Minimal direction: make resume either reject sessions whose WorkDir differs from the active runtime, or perform a real runtime CWD transition that also updates permission/workdir-dependent services.
Do not hide this with UI-only filtering; web and explicit --resume still bypass that.

55. Interactive resume does not rehydrate cost/token trackers before saving metadata

Severity: medium

Files/functions:
internal/cli/deps.go:324, internal/cli/deps.go:353, internal/cli/deps.go:379, internal/cli/run.go:303, internal/cli/run.go:897

Initial startup resume correctly seeds Metrics and CostTracker from the loaded session. In-process InteractiveRuntime.Resume only swaps the conversation, handoff state, writer, and content replacement
records. The existing CostTracker and Metrics remain attached to the previous interactive runtime state, but makeSessionSaveClose later writes metadata from those live trackers into the resumed
session file.

Observable failure: after /resume, saved cost_usd and token usage can be from the previous session or process lifetime, not the resumed session plus new work.

Minimal direction: put “load session into runtime” behind one owner that rehydrates all runtime state used for future persistence. Do not patch WriteMetadata to preserve old fields; the live state
feeding metadata is wrong.

56. Session-scoped permission rules survive in-process resume



Severity: high

Files/functions:
internal/permission/rulechecker.go:117, internal/tool/orchestrator.go:359, internal/cli/deps.go:228, internal/cli/run.go:303, internal/web/web.go:247, internal/tui/permission.go:110

RuleChecker.AddSessionRule appends rules marked SourceSession. Those rules are created by permission prompts when the user chooses a remembered/session allow. InteractiveRuntime.Resume does not clear
or recreate the checker, so a rule granted in one session remains active after resuming a different session.

This is an ownership leak: permission scope says “current session,” but the checker lifecycle is the interactive process lifecycle.

Observable failure: allow Bash or a file/tool permission in session A, /resume session B, and the grant can apply in B without a prompt.

Minimal direction: session resume must reset session-scoped permissions or bind them to a session ID inside the permission owner. Do not add UI warnings or prompt text; the enforcement layer must own
the scope.


• I found a better candidate that is not the same as the prior scoped-schema issue: CLI tool filters may apply only to the root registry, while subagent engines rebuild their own registry from base
tools. I’m tracing whether --allowed-tools/--disallowed-tools actually constrain subagents or only the parent agent.

• This one looks distinct and real: the advertised “physically removed, can’t be bypassed” guarantee is implemented as a root registry mutation, but the Agent tool later recreates a fresh registry from
BaseTools. I’m checking the Agent schema path so the failure mode is precise rather than theoretical.

• The subagent filter bypass is solid, so I have one new finding. I’m doing one more pass through lifecycle bridge execution because it is a separate runtime path, but I’ll only report it if it exposes
a different state/persistence owner problem from the already reported lifecycle runners.



57. Tool exposure policy is only applied to the root visible registry, not to nested execution registries

Severity: high

Files/functions:
internal/cli/run.go:1023, internal/cli/tools.go:63, internal/tools/agent/agent.go:202, internal/tool/registry.go:92, internal/cli/tools.go:148, internal/tools/repl/repl.go:174

What is split/misplaced:
--allowed-tools / --disallowed-tools is described as physically removing tools from the registry in internal/cli/run.go:1023, but that policy is only applied to the root registry after tool

REPL has the same boundary leak through hidden tools: hidden tools are excluded from List() and ToolDefs() but remain available through Get() in internal/tool/registry.go:92. applyToolFilters iterates
registry.List(), so hidden REPL primitives are not removed, while REPL later calls them via Registry.Get.

Why wrong:
Tool exposure policy is being implemented as a presentation/root-registry mutation, but nested execution surfaces create or retain their own callable tool universe. The execution boundary is not
authoritative.

Observable failure:
pragma --allowed-tools Agent can let the model call Agent, and the subagent gets all built-in tools except Agent. With PRAGMA_REPL=1 --allowed-tools REPL, the root model sees only REPL, but REPL can
still invoke hidden primitives such as Bash, Read, Write, and Edit.

Minimal direction:
Represent tool exposure policy once and apply it before any executable registry is used, including subagent factories and REPL primitive lookup.

What not to do:
Do not add prompt text telling agents not to use tools. Do not rely on hiding tools from ToolDefs(); hidden-but-callable is still callable.

58. REPL executes nested tools outside the orchestrator boundary

Severity: high

Files/functions:
internal/tools/repl/repl.go:98, internal/tools/repl/repl.go:160, internal/tool/orchestrator.go:251, internal/tool/orchestrator.go:294, internal/tool/orchestrator.go:359, internal/tool/
orchestrator.go:451

What is split/misplaced:
The orchestrator owns schema validation, pre-tool hooks, permission prompting, execution events, and post-tool hooks. REPL bypasses that by taking nested operations and directly calling
desc.Invoke(...) in internal/tools/repl/repl.go:184.

Its permission check only delegates to the inner tool when there is exactly one operation. For multi-operation batches, it returns a generic DecisionAsk for REPL in internal/tools/repl/repl.go:131,
then invokes every primitive directly.

Why wrong:
A composite tool is making itself a second orchestrator without implementing the orchestrator’s contract. The result is not just duplicated control flow; it skips the owner of tool safety and
observability.


Observable failure:
A REPL batch can execute Bash and Write after one generic REPL approval, without each inner operation getting its own schema validation, hook checks, permission audit, or ToolExecutionCompleted event
as that tool.

Minimal direction:
REPL should submit nested operations back through the real tool.Orchestrator, or be removed as an execution wrapper if the same batching can be modeled at the provider/runtime layer.

What not to do:
Do not add more checks inside REPL piecemeal. That just recreates the orchestrator badly and will keep drifting.


59. update_plan stores “session” checklist state in memory, but the session store cannot persist it
    That means a model can call update_plan, see the checklist during the current process, and then lose it after save/resume/restart. There is also a separate durable handoff-state path that can contain
    todos, so the codebase has two planning-state owners with different persistence semantics.

Fix direction: make update_plan update the durable handoff/session owner, or add an explicit persisted session entry for AppState.Todos. Don’t reconstruct it from tool-result text or UI state.

paths for lifecycle ownership mistakes.




60. Web active-turn cancellation is stored but unreachable

Files/functions:
internal/web/web.go:392-423, internal/web/web.go:66-72, internal/web/web.go:2475-2513, internal/tui/handlers.go:615-621, internal/tui/handlers.go:832-843

The web server creates a per-run context.WithCancel and stores s.cancel, but no route, key handler, or UI action ever calls it. The only prompt endpoint starts work, rejects concurrent work with 409,
and waits for RunInput to finish. TUI has an explicit interrupt path: Ctrl+C during streaming calls interruptTurn, cancels the active context, and returns to input.

Responsibility split:
Active-turn lifecycle is owned by individual UIs instead of the shared interactive runtime. TUI can cancel a turn; web can only mark itself running/idle and has a dead cancel field.

Why wrong:
Cancellation is a runtime control transition, not presentation state. The web UI receives a running state it cannot resolve.

Observable failure:
A long web run cannot be interrupted from the browser. Submitting another prompt gets interactive run already in progress; the practical escape hatch is process shutdown, which closes the session/
server instead of cancelling the turn.

Fix direction:
Move active-turn cancellation into the shared interactive runtime, or add a web cancel endpoint that calls the runtime-owned active-turn cancel and emits the same runtime event shape TUI gets.

What not to do:
Do not just re-enable the send button, drop SSE events, or add a browser-only “cancelled” label. That hides the stuck runtime work.

61. SessionEnd hooks run under the already-cancelled command context

Severity: medium

Files/functions:
cmd/pragma/main.go:43-46, internal/cli/run.go:430-437, internal/cli/run.go:490, internal/cli/run.go:455, internal/cli/subcommands.go:84-89, internal/cli/run.go:567-571, internal/hook/executor.go:17-31

The root command context is cancelled by SIGINT/SIGTERM. Cleanup then passes that same context into HookMgr.Execute(..., hook.SessionEnd, ...). ExecCommand derives its timeout context from the
supplied context, so if shutdown was caused by the signal, the hook process is born under an already-cancelled parent.

Responsibility split:
The signal context is being used both for cancelling active work and for running shutdown lifecycle hooks. Those are different lifecycle phases.

Why wrong:
SessionEnd is cleanup/finalization work. It needs a bounded shutdown context, not the cancelled operation context that triggered shutdown.

Observable failure:
On Ctrl+C/SIGTERM paths, SessionEnd hooks can fail immediately with context cancellation instead of running. Normal quit may work, signal quit may not, so hook behavior depends on exit path.

Fix direction:
Run SessionEnd hooks with a fresh bounded shutdown context, for example context.WithTimeout(context.Background(), ...), and pass cancellation reason separately if needed.

What not to do:
Do not special-case context.Canceled as hook success or silently skip SessionEnd hooks on signal. That preserves the lifecycle split.



• 60. Web active-turn cancellation is stored but unreachable

Files/functions:
internal/web/web.go:392-423, internal/web/web.go:66-72, internal/web/web.go:2475-2513, internal/tui/handlers.go:615-621, internal/tui/handlers.go:832-843

The web server creates a per-run context.WithCancel and stores s.cancel, but no route, key handler, or UI action ever calls it. The only prompt endpoint starts work, rejects concurrent work with 409,
and waits for RunInput to finish. TUI has an explicit interrupt path: Ctrl+C during streaming calls interruptTurn, cancels the active context, and returns to input.

Responsibility split:
Active-turn lifecycle is owned by individual UIs instead of the shared interactive runtime. TUI can cancel a turn; web can only mark itself running/idle and has a dead cancel field.
Observable failure:
A long web run cannot be interrupted from the browser. Submitting another prompt gets interactive run already in progress; the practical escape hatch is process shutdown, which closes the session/
server instead of cancelling the turn.

Fix direction:
61. SessionEnd hooks run under the already-cancelled command context

Severity: medium

Files/functions:
Responsibility split:
The signal context is being used both for cancelling active work and for running shutdown lifecycle hooks. Those are different lifecycle phases.

Why wrong:
SessionEnd is cleanup/finalization work. It needs a bounded shutdown context, not the cancelled operation context that triggered shutdown.
Run SessionEnd hooks with a fresh bounded shutdown context, for example context.WithTimeout(context.Background(), ...), and pass cancellation reason separately if needed.

What not to do:
Do not special-case context.Canceled as hook success or silently skip SessionEnd hooks on signal. That preserves the lifecycle split.



What is split:
The query engine records provider usage into CostTracker after a model response. The compactor repeats that accounting manually. But other direct provider.Complete callers do not use the same
accounting path: WebFetch.summarize calls the secondary model and returns text only; lifecycle LLM nodes put usage into graph state as total_usage, but the CLI completion path only prints the final
assistant text.

Why wrong:
Provider usage/cost is a runtime accounting responsibility. It should not depend on which component happened to call provider.Complete. Right now each caller has to remember to account for cost, and
some do not.

Observable failure:
A session that uses WebFetch summarization can spend secondary-model tokens without increasing CostTracker, so the TUI/CLI total cost and saved session metadata underreport cost. Lifecycle graph runs
can accumulate total_usage internally while never reconciling it into the normal cost/session accounting path.

Minimal fix direction:
Centralize model-call accounting at the provider invocation boundary or provide a shared runtime helper that every non-query model call must use. Then delete the compactor’s manual duplicate
accounting.

What not to do:
Do not patch only WebFetch by injecting a CostTracker field and copying the same Pricing/Record block again. That preserves the accounting responsibility leak and guarantees the next direct model
caller will miss it too.






What is split:
The query engine records provider usage into CostTracker after a model response. The compactor repeats that accounting manually. But other direct provider.Complete callers do not use the same
accounting path: WebFetch.summarize calls the secondary model and returns text only; lifecycle LLM nodes put usage into graph state as total_usage, but the CLI completion path only prints the final
assistant text.
some do not.

Observable failure:
A session that uses WebFetch summarization can spend secondary-model tokens without increasing CostTracker, so the TUI/CLI total cost and saved session metadata underreport cost. Lifecycle graph runs
can accumulate total_usage internally while never reconciling it into the normal cost/session accounting path.

What not to do:
Do not patch only WebFetch by injecting a CostTracker field and copying the same Pricing/Record block again. That preserves the accounting responsibility leak and guarantees the next direct model
caller will miss it too.


63. Structured output is implemented as a CLI-scraped synthetic tool instead of the provider response contract

Files/functions:
internal/cli/run.go:588-612, internal/cli/run.go:637-681, internal/cli/run.go:729-734, internal/tools/synthetic/synthetic.go:16-18, internal/provider/provider.go:48-58, internal/provider/google/
provider.go:441-448

What is split:
The provider boundary already has RequestParams.ResponseSchema, and Google maps it into native JSON response schema. But --output-schema does not flow into that provider contract. CLI mode registers a
fake StructuredOutput tool, hides normal text output, then scrapes ToolCallEvent.Call.Input as the final JSON.

Why wrong:
Structured output is a model response contract, not a tool execution side channel and not CLI presentation state. The CLI is deciding what the “real” model answer is by watching tool-call events.

Observable failure:
Providers with native structured output support never receive the schema through ResponseSchema. If the model emits valid JSON as text or the provider could enforce schema natively, CLI still warns
model did not call StructuredOutput tool. Conversely, the CLI can print the input of a tool call as final output even though that is not a provider response.

Minimal fix direction:
Route --output-schema into the runtime/provider request path as ResponseSchema when the provider supports it. Keep the synthetic tool only as an explicit fallback for providers without native
structured output, owned by the runtime, not by CLI event scraping.

What not to do:
Do not add more CLI parsing of text/tool events to guess JSON. Do not make every UI learn about StructuredOutput.

64. Background session status treats provider idle as runtime idle while tools are still running

Severity: medium

Files/functions:
internal/background/subscriber.go:24-40, internal/tool/orchestrator.go:409-416, internal/tool/orchestrator.go:441-445, internal/query/miniswe_loop.go:145-180

What is split:
Background status is derived from observe events, but it only maps APIRequestStarted to busy and APIRequestCompleted/APIRequestFailed to idle. Tool execution events exist and are emitted by the
orchestrator, but the background status owner ignores them.

Why wrong:
A background session’s state should represent the runtime turn, not just the provider request phase. The model response can complete with a tool call, after which the agent is still actively doing
work.

Observable failure:
A background run that receives a tool call can flip to idle immediately after the model response, then spend seconds or minutes in Bash, Sleep, or another tool. pragma sessions can show the session as
idle while it is mutating files or running commands.

Minimal fix direction:
Drive background status from turn/tool lifecycle: mark busy for tool batch/execution start and idle only when the turn completes or fails. If using event counters, track active model/tool work rather
than overwriting status on APIRequestCompleted.

What not to do:
Do not rename idle to something vague or add polling against process liveness. The process being alive is not the same as runtime activity.

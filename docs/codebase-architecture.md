# Pragma Codebase Architecture

Last source pass: 2026-06-11.

This document describes the current checkout by source files, Go types, Kotlin
classes, and the methods that own the major runtime flows. It treats the source
tree as authoritative.

## System Shape

Pragma is primarily a Go CLI agent runtime with:

- A Cobra command surface in `cmd/pragma/`.
- Runtime construction, dependency injection, and presentation routing in `internal/cli/`.
- A normalized model layer in `internal/model/`.
- A query engine in `internal/query/`.
- Provider adapters in `internal/provider/*`.
- Tool descriptors and execution machinery in `internal/tool/` and `internal/tools/*`.
- Session/event persistence under `internal/session/` and observability under `internal/observe/`.
- Terminal UI in `internal/tui/`.
- FSM/orchestration runtimes in `internal/orchestration/` and `internal/lifecycle/`.
- A JetBrains IDE MCP plugin in `plugins/jetbrains-reflective-mcp/`.

High-level runtime flow:

```text
cmd/pragma/main.go
  -> internal/cli.RunDispatcher
     -> SetupDepsWithOptions
        -> config, credentials, provider, EventBus, permission checker,
           StateStore, MCP manager, task registry, session wiring
     -> RegisterTools
        -> tool.Registry, tool.Orchestrator, query.Engine
     -> interactive TUI, non-interactive stdout, background process,
        orchestration, lifecycle, replay, inspect, metrics, audit commands
```

The active default conversation loop is the Pragma shell-action loop:

```text
InteractiveRuntime.RunInput or RunNonInteractive
  -> query.Engine.Run
  -> Engine.runLoop
  -> Engine.runPragmaLoop
  -> provider.Provider.Complete
  -> parse exactly one fenced bash block
  -> runPragmaLoopBash
  -> append command observation as a user message
  -> repeat until final answer or completion sentinel
```

The registry/orchestrator tool system still exists and is used by several
runtime surfaces, but `Engine.Run` currently routes to `runPragmaLoop` rather
than the older native provider tool-call loop.

## Entry Points

| File | Type or function | Responsibility |
| --- | --- | --- |
| `cmd/pragma/main.go` | `main()` | Builds the Cobra root command, registers command groups, installs signal-aware context, and runs Cobra. |
| `cmd/pragma/main.go` | `versionCmd()`, `completionCmd()` | Built-in version and shell completion commands. |
| `cmd/pragma/sessions.go` | `sessionsCmd()` | Session listing and related session command surface. |
| `cmd/pragma/replay.go`, `cmd/pragma/replay_raw_http.go`, `cmd/pragma/replay_export.go` | `replayCmd()`, `replayRawHTTPCmd()`, `replayExportCmd()` | Replay and raw HTTP inspection/export command surfaces. |
| `cmd/pragma/inspect.go` | `inspectCmd()`, `inspectRawHTTPCmd()`, `inspectPhasesCmd()` | Offline inspection commands. |
| `cmd/pragma/audit.go`, `cmd/pragma/metrics.go` | `auditCmd()`, `metricsCmd()` | Audit and metrics command surfaces. |
| `cmd/pragma/lifecycle.go` | `lifecycleCmd()`, `lifecycleRunCmd()`, `lifecycleListCmd()` | Standalone lifecycle graph command surface. |
| `cmd/pragma/orchestration.go` | `orchestrationCmd()`, `orchestrationRunCmd()` | Standalone orchestration FSM command surface. |
| `cmd/pragma/cron.go` | `cronCmd()` | Scheduled task daemon command surface. |

The command layer should stay thin. Most runtime construction belongs to
`internal/cli`, not `cmd/pragma`.

## CLI Runtime

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/cli/flags.go` | `RegisterFlags(*cobra.Command)` | Defines persistent runtime flags and root-only prompt/session flags. |
| `internal/cli/run.go` | `RunDispatcher` | Selects background, session list, non-interactive prompt, or interactive TUI. |
| `internal/cli/run.go` | `RunBackground` | Starts a detached child process, sets `PRAGMA_BG_SESSION*`, writes log path, and registers process metadata. |
| `internal/cli/run.go` | `RunInteractive`, `RunTUIInteractive` | Builds the interactive runtime and launches the Bubble Tea TUI. |
| `internal/cli/run.go` | `RunNonInteractive` | Builds dependencies, starts a session, runs the engine, and streams output to stdout/stderr. |
| `internal/cli/run.go` | `InteractiveRuntime` | Presentation-neutral interactive runtime with the active `Deps`, `query.Engine`, slash registry, session hooks, and cleanup function. |
| `internal/cli/run.go` | `InteractiveRuntime.RunInput` | Admission-controls a user turn, handles slash commands, or forwards prompts to the engine. |
| `internal/cli/run.go` | `InteractiveRuntime.runSlash` | Executes slash commands and interprets `slash.Result` effects such as clear, resume, quit, injected prompts, or orchestration. |
| `internal/cli/run.go` | `InteractiveRuntime.runEngine` | Appends hook context, records the accepted turn/history, then forwards `query.LoopEvent`s from `Engine.Run`. |
| `internal/cli/run.go` | `InteractiveRuntime.runOrchestration` | Runs a YAML orchestration definition through `internal/orchestration` and records handoff artifacts. |
| `internal/cli/run.go` | `BuildInteractiveRuntimeWithOptions` | Calls `SetupDepsWithOptions`, `RegisterTools`, starts session lifecycle hooks, builds compaction, slash deps, and runtime cleanup. |
| `internal/cli/deps.go` | `Deps` | Shared runtime object graph: config, credentials, provider, bus, checker, state store, registry, engine, task registry, MCP manager, cron scheduler, hook manager, metrics, session writer, and cleanup. |
| `internal/cli/deps.go` | `SetupDepsWithOptions` | Main dependency injection root. Loads config/credentials, resolves provider, initializes observe subscribers, permissions, hooks, sessions/resume state, state store, tasks, cron, MCP manager, and cleanup. |
| `internal/cli/deps.go` | `CreateProvider` | Factory for `anthropic`, `openai`, `google`, `google-vertex`, `groq`, and `lilac` providers. |
| `internal/cli/deps.go` | `buildRuntimeSystemPrompt` | Builds the runtime system prompt from override text or `internal/sysprompt.Builder`. |
| `internal/cli/tools.go` | `RegisterTools` | Registers built-in tools, Agent, AskUserQuestion, Skill, REPL, LifecycleRun, binds ToolSearch, creates `tool.Orchestrator`, then creates `query.Engine`. |
| `internal/cli/tools.go` | `BaseTools`, `baseTools` | Defines the built-in descriptor set: search/read/write/edit/bash/apply_patch, task tools, worktree, cron, web, config, selftrace, MCP, and optional feature tools. |
| `internal/cli/subcommands.go` | `RegisterSubcommands`, `RunPromptCommand`, `RunLocalCommand` | Exposes slash commands as regular Cobra subcommands where applicable. |

## Query Engine

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/query/engine.go` | `EngineConfig` | Runtime execution configuration: model, tokens, turns, structured output, compaction/session hooks, MCP status callbacks, and restored state records. |
| `internal/query/engine.go` | `Engine` | Owns provider, tool registry/orchestrator, app state, cost tracker, event bus, compaction, hooks, task registry, file state, and content replacement state. |
| `internal/query/engine.go` | `NewEngine` | Constructs an engine with injected dependencies and restored file/content replacement state. |
| `internal/query/engine.go` | `Engine.ForkFreshConversation` | Creates a scoped engine with a separate conversation store for orchestration/persona states. |
| `internal/query/engine.go` | `Engine.RebindProvider`, `Engine.SetModel` | Switches provider/model at runtime. |
| `internal/query/engine.go` | `Engine.SetCompaction`, `Engine.SetHookManager`, `Engine.SetTaskRegistry`, `Engine.SetSessionCheckpoint` | Attaches optional runtime services after construction. |
| `internal/query/engine.go` | `Engine.appendConversationMessage` | Single append path for conversation state; emits `observe.MessageAppended` and checkpoints the session. |
| `internal/query/engine.go` | `Engine.AppendHookContext` | Adds hook-produced context as an internal user message. |
| `internal/query/loop.go` | `Engine.Run` | Public async entrypoint; returns a channel of sealed `LoopEvent` values. |
| `internal/query/loop.go` | `Engine.runLoop` | Current default route; calls `runPragmaLoop`. |
| `internal/query/loop.go` | `Engine.runStopHook` | Runs Stop hooks at the end of a loop. |
| `internal/query/loop.go` | `messagesForRequestChecked` | Selects API-visible messages and validates tool-result pairing. |
| `internal/query/miniswe_loop.go` | `PragmaLoopSystemPrompt` | System contract for the shell-action transport. |
| `internal/query/miniswe_loop.go` | `Engine.runPragmaLoop` | Wraps the user prompt with current working directory/system metadata and enters the shell-action loop. |
| `internal/query/miniswe_loop.go` | `Engine.RunPragmaLoopWithSystemCompletionCheck` | Orchestration-facing variant with explicit system prompt and optional completion artifact check. |
| `internal/query/miniswe_loop.go` | `Engine.runPragmaLoopWithInitialPrompt` | Core loop: request model completion, extract one bash action, execute it, append observation, and terminate on final answer/completion. |
| `internal/query/miniswe_loop.go` | `Engine.completePragmaLoopResponse` | Calls `Provider.Complete` with retry handling/classified errors. |
| `internal/query/miniswe_loop.go` | `runPragmaLoopBash` | Executes shell actions via `internal/shellrun` and routes shell-embedded `apply_patch` through the applypatch tool implementation. |
| `internal/query/miniswe_loop.go` | `pragmaLoopSourceMutationReason` | Blocks direct repository source mutation through shell redirection, `sed -i`, `tee`, or write-intent scripts. |
| `internal/query/event.go` | `LoopEvent` and concrete event types | Query-to-presentation event surface: text, thinking, model request/response, tool call/result, structured output, user message, turn complete, compaction, lifecycle, orchestration, agent progress, retry, and error events. |
| `internal/query/engine.go` | `Engine.RunGraph` | Executes a lifecycle graph through the lifecycle bridge and emits lifecycle progress events. |

## Model And App State

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/model/conversation.go` | `Conversation` | Ordered message history plus system prompt, model/provider, workdir, parent, and timestamps. |
| `internal/model/conversation.go` | `NewConversation`, `Append`, `Fork`, `DeepCopy`, `APIMessages` | Construct, mutate, copy, fork, and filter messages sent to providers. |
| `internal/model/message.go` | `Message`, `Role`, `MessageFlags`, `SystemPrompt`, `SystemBlock` | Provider-neutral message and system prompt representation. |
| `internal/model/content.go` | `ContentPart` and variants | Sealed content hierarchy: text, image, document, tool call, tool result, thinking. |
| `internal/model/response.go` | `Response` | Provider-normalized model response with content, stop reason, usage, and model ID. |
| `internal/model/tool.go` | `ToolDef` | Provider-neutral tool definition shape. |
| `internal/model/usage.go` | `CostTracker`, `TokenUsage`, `Pricing` | Token and cost accounting. |
| `internal/app/state.go` | `AppState` | Mutable runtime state snapshot: conversation, cwd, model/provider, todos, team, prompt history, orchestration artifacts, worktree state. |
| `internal/app/state.go` | `AppState.WorkDir`, `AppState.SessionID` | Adapters for tool snapshots and artifact/session ownership. |
| `internal/app/store.go` | `StateStore` | Thread-safe application state store. |
| `internal/app/store.go` | `StateStore.Snapshot`, `StateStore.Update` | Deep-copy reads and single locked mutation path. |

The architectural rule is that provider adapters translate wire data into
`internal/model` types before the query/runtime layers see it.

## Providers

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/provider/provider.go` | `Provider` | Boundary interface for LLM adapters: `Name`, `Stream`, `Complete`, `SupportsFeature`, `Pricing`, and `ContextWindow`. |
| `internal/provider/provider.go` | `RequestParams` | Provider input in internal types: model, tokens, messages, system prompt, tools, temperature, thinking, response schema. |
| `internal/provider/provider.go` | `StreamChunk` | Provider-neutral streaming chunk representation. |
| `internal/provider/accumulate.go` | `AccumulateStream` | Converts `StreamChunk`s into a complete `model.Response`. |
| `internal/provider/anthropic/provider.go` | `anthropic.Provider` | Anthropic Messages API adapter; builds wire params, emits request events, retries, and converts wire responses back to `model.Response`. |
| `internal/provider/openai/provider.go` | `openai.Provider` | OpenAI adapter. |
| `internal/provider/google/provider.go` | `google.Provider` | Google Gemini adapter. |
| `internal/provider/googlevertex/provider.go` | `googlevertex.Provider` | Vertex AI adapter. |
| `internal/provider/groq/provider.go` | `groq.Provider` | Groq adapter. |
| `internal/provider/lilac/provider.go` | `lilac.Provider` | Lilac/OpenAI-compatible adapter backed by any-llm-go plus raw capture support. |
| `internal/provider/replay/provider.go` | `replay.Provider` | Replays captured API responses with optional fallback provider. |
| `internal/provider/rawcapture/rawcapture.go` | raw capture helpers | HTTP capture plumbing for provider request/response inspection. |
| `internal/provider/shared/retry.go` | `WithRetry` | Shared retry policy and API retry event emission. |

## Tools And Permissions

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/tool/tool.go` | `Descriptor` | Contract every tool implements: `Name`, `Description`, `InputSchema`, `Invoke`, `CheckPerm`, and `Flags`. |
| `internal/tool/tool.go` | `InvokeResult`, `ToolFlags`, `StateSnapshot` | Tool result payloads, execution traits, and read-only state interface. |
| `internal/tool/registry.go` | `Registry` | Descriptor lookup, JSON Schema compilation, hidden tools, exposure filtering, scoped registries, and provider-facing `ToolDefs`. |
| `internal/tool/registry.go` | `Register`, `Unregister`, `Get`, `ToolDefs`, `Scoped` | Registry mutation and lookup methods. |
| `internal/tool/orchestrator.go` | `Orchestrator` | Permission checking, hook execution, serial/concurrent batching, panic recovery, and tool observability. |
| `internal/tool/orchestrator.go` | `Orchestrator.Execute` | Executes a batch of tool calls and preserves result order. |
| `internal/tool/orchestrator.go` | `Orchestrator.executeSingle` | Validates JSON input, checks permission, invokes hooks, calls one descriptor, and emits events. |
| `internal/permission/permission.go` | `Checker`, `Rule`, `CheckResult`, `PermissionMode` | Permission boundary abstractions and rule data. |
| `internal/permission/rulechecker.go` | `RuleChecker` | Production permission checker with scoped workdir support, session/persistent rules, mode defaults, and dangerous path checks. |
| `internal/permission/persist.go` | `PersistRule`, `LoadPersistedRules` | Local permission rule persistence. |

Built-in tools are registered from `internal/cli/tools.go` and implemented under
`internal/tools/*`. The common class shape is a `Tool` type implementing
`tool.Descriptor` methods. Notable tool groups:

| Package | Main type(s) | Purpose |
| --- | --- | --- |
| `internal/tools/glob`, `grep`, `fileread` | `Tool` | Read/search workspace state. |
| `internal/tools/filewrite`, `fileedit`, `notebookedit`, `applypatch` | `Tool`, `LegacyTool` | Source and notebook mutation; patch mode blocks shell mutation in the Pragma loop. |
| `internal/tools/bash`, `powershell`, `sleep` | `Tool` | Shell/process execution primitives. |
| `internal/tools/agent` | `Tool` | Sub-agent execution through an engine factory, task registry, and shared provider/cost state. |
| `internal/tools/ask` | `Tool` | User-question tool backed by an `Asker`. |
| `internal/tools/task*`, `sendmsg` | `Tool` | Task creation, state updates, lifecycle messages, output, stopping, and inter-agent messages. |
| `internal/tools/todo` | `Tool` | Mutates session todos in `app.StateStore`. |
| `internal/tools/worktree` | `EnterTool`, `ExitTool` | Enters/exits temporary worktrees and refreshes runtime system prompt/capabilities. |
| `internal/tools/webfetch`, `websearch` | `Tool` | Web fetch/search helpers. |
| `internal/tools/mcp`, `mcpauth` | `ListTool`, `ReadTool`, `Tool` | MCP resource and OAuth support. |
| `internal/tools/cron` | `CreateTool`, `DeleteTool`, `ListTool` | Scheduled task tool surface. |
| `internal/tools/config` | `Tool` | Runtime config inspection and selected live mutation. |
| `internal/tools/lifecycle` | `Tool` | Runs lifecycle graphs as a tool. |
| `internal/tools/toolsearch`, `toolresultread`, `selftrace`, `brief`, `skill`, `synthetic`, `repl`, `remote` | `Tool` variants | Discovery, large-result readback, trace inspection, brief events, skill invocation, structured-output fallback, REPL, and optional remote trigger support. |

## Sessions And Persistence

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/session/session.go` | `Session`, `SessionSummary` | Durable session projection and list view. |
| `internal/session/entry.go` | `Entry`, `HeaderData`, `MetadataData`, entry data structs | JSONL record schema. |
| `internal/session/store.go` | `Store` | Owns `~/.pragma/sessions`, session loading/listing/deletion, artifact/tool-result directory paths. |
| `internal/session/store.go` | `Store.Create`, `Open`, `Load`, `List`, `Delete` | Session file lifecycle. |
| `internal/session/writer.go` | `Writer` | Thread-safe append/rewrite writer for session JSONL files. |
| `internal/session/writer.go` | `WriteHeader`, `WriteMessage`, `WriteMetadata`, `WriteContentReplacement`, `WritePromptHistory`, `WriteFileState`, `WriteTodos`, `WriteTeamContext`, `WriteOrchestrationArtifacts`, `WriteTaskResult`, `Rewrite`, `Close` | Durable entry write surface. |

The session log is append-oriented JSONL. `Store.Load` reconstructs the latest
conversation plus metadata, replacements, prompt history, file state, todos,
team context, orchestration artifacts, task results, and worktree state.

## Observability

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/observe/event.go` | `Event`, `EventHeader`, concrete event structs | Sealed event catalog and JSON event unmarshalling. |
| `internal/observe/bus.go` | `EventBus` | Lock-free multi-producer/single-consumer ring buffer and subscriber dispatch. |
| `internal/observe/bus.go` | `Emit`, `Subscribe`, `Drain` | Event publishing, subscription, and shutdown delivery. |
| `internal/observe/logger.go` | `Logger` | Text/JSON/compact log subscriber with level/topic filtering. |
| `internal/observe/recorder.go` | `Recorder` | Event recording. |
| `internal/observe/replay.go` | `ReplayEngine` | Loads and indexes event logs for replay/inspection. |
| `internal/observe/metrics.go` | `Metrics` | Aggregates runtime metrics such as token/turn counts. |
| `internal/observe/auditor.go` | `Auditor` | Tracks audit entries and violations. |
| `internal/observe/token_monitor.go` | `TokenMonitor` | Watches token budget and emits warnings. |
| `internal/observe/watchdog.go` | MCP watchdog | Watches MCP server health. |

The event bus is the runtime backbone for logs, metrics, auditing, session
status subscribers, MCP status, and trace/replay tooling.

## Interactive UI

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/interactive/event.go` | interactive event wrappers | Presentation-facing events around query loop events, accepted/rejected prompts, slash results, and runtime termination. |
| `internal/tui/model.go` | `Config` | Dependency bundle passed from `RunTUIInteractive`. |
| `internal/tui/model.go` | `Model` | Bubble Tea state machine for viewport, input, permission dialog, ask dialog, teams, model picker, resume picker, toolbar, streaming segments, tasks, and layout. |
| `internal/tui/model.go` | `New` | Constructs the TUI model and initial context/components. |
| `internal/tui/handlers.go` | update handlers | Bubble Tea update/event handling. |
| `internal/tui/render/*` | render helpers | Markdown, tool results, lifecycle progress, agent progress, errors, welcome screen, and grouped output rendering. |
| `internal/tui/permission.go`, `prompter.go` | interactive permission prompter | Bridges permission prompts into the TUI. |
| `internal/tui/ask.go`, `asker.go` | interactive ask surface | Bridges tool-driven user questions into the TUI. |

The TUI does not build providers or tools directly. It receives `RunInput`,
`Resume`, `CloseSession`, state, metrics, and slash dependencies from
`internal/cli`.

## Slash Commands

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/slash/command.go` | `Registry`, `Command`, `Result`, `Deps` | Slash command catalog, handler contract, runtime dependencies, and command side-effect result. |
| `internal/slash/command.go` | `NewRegistry`, `Register`, `Execute`, `CommandsWithDeps` | Build and execute command registry, including skill-backed prompt commands. |
| `internal/slash/commands.go` | `registerBuiltins` | Registers built-ins: compact, clear, copy, help, exit, insights, cost, model, doctor, review, security-review, commit, init, teams, config, skills, resume, mcp, orchestrate. |
| `internal/slash/*_cmd.go`, `review.go`, `commit.go`, `doctor.go`, `orchestrate.go` | command handlers | Command-specific local behavior or injected prompt construction. |

Slash commands return declarative `Result` values. `InteractiveRuntime.runSlash`
owns applying those results to the active runtime.

## Orchestration FSM

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/orchestration/orchestration.go` | `Definition`, `State`, `Transition`, `Runtime`, `Control`, `Artifact` | YAML FSM schema and runtime data structures. |
| `internal/orchestration/orchestration.go` | `LoadDefinitionFile`, `NewRuntime`, `TransitionFor` | Load/validate definitions and build looplab FSM runtime. |
| `internal/orchestration/runner.go` | `RunFileEventsWithOptions`, `RunEventsWithOptions` | Starts an orchestration run and streams `query.LoopEvent`s. |
| `internal/orchestration/runner.go` | `runEvents` | Executes the FSM: creates run dirs, forks per-state engines, runs persona/control nodes, transitions, emits snapshots. |
| `internal/orchestration/runner.go` | `RunNodeEvents`, `runNodeEvents` | Executes one persona or control state. |
| `internal/orchestration/runner.go` | `EnsureRunDirs` | Creates artifact roots and directories referenced by states/transitions. |
| `internal/orchestration/projection.go` | `Projection` | Projects orchestration events into `query.OrchestrationSnapshot`. |
| `internal/persona/persona.go` | persona types/loaders | YAML persona definitions used by orchestration states. |

Orchestration reuses the query engine but forks conversations for persona states
so each state keeps scoped history.

## Lifecycle Graph Runtime

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/lifecycle/graph.go` | `Graph`, `Builder`, `NodeFunc`, `RouterFunc`, `ConditionalEdge` | Immutable graph definition and builder. |
| `internal/lifecycle/graph.go` | `Builder.AddNode`, `AddEdge`, `AddConditionalEdges`, `SetReducer`, `SetInitialNode`, `Build` | Graph construction and validation. |
| `internal/lifecycle/state.go` | `State`, `StateUpdate` | Graph state map and typed key helpers. |
| `internal/lifecycle/executor.go` | `Executor` | Superstep graph executor. |
| `internal/lifecycle/executor.go` | `Executor.Stream`, `Executor.Run`, `executeSuperstep`, `resolveNextNodes` | Parallel node execution, reducer application, checkpointing, and transition resolution. |
| `internal/lifecycle/definition/*` | definition parser/resolver | YAML/definition parsing, schema validation, fixups, routing resolution. |
| `internal/lifecycle/bridge/*` | bridge nodes and runner | Bridges lifecycle graph state to provider calls, tool calls, LLM nodes, eval nodes, reducers, and progress projection. |
| `internal/lifecycle/bridge/runner.go` | `Runner`, `RunnerConfig`, `RunEvent`, `RunResult` | Runs lifecycle graphs with model/system/tools context and projects final assistant response. |
| `internal/tools/lifecycle/lifecycle.go` | `lifecycletool.Tool` | Exposes lifecycle graph execution as a tool. |
| `cmd/pragma/lifecycle.go` | lifecycle CLI | Standalone lifecycle execution/listing. |

Lifecycle is a separate graph execution model from orchestration. Orchestration
is YAML FSM/persona oriented; lifecycle is graph/superstep oriented.

## MCP Runtime

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/mcp/config.go` | `ServerConfig` and config loaders | MCP server configuration loading. |
| `internal/mcp/client.go` | `Client` | Connects to one MCP server, lists tools/resources, calls tools, disconnects/reconnects. |
| `internal/mcp/manager.go` | `Manager` | Owns all MCP clients, statuses, generation, configured servers, tool registration, reconnection, and registered tool names. |
| `internal/mcp/manager.go` | `ConfigureServers`, `ReplaceServers`, `ConnectAllAndRegister`, `RegisterTools`, `DisconnectAll`, `ServerStatuses`, `WaitForRegisteredTools` | MCP lifecycle and registry integration. |
| `internal/mcp/adapter.go` | `MCPToolAdapter` | Wraps remote MCP tools as `tool.Descriptor`s. |
| `internal/mcp/adapter.go` | `Name`, `Description`, `InputSchema`, `Invoke`, `CheckPerm`, `Flags` | Descriptor implementation for MCP tools. |
| `internal/tools/mcp/list.go`, `read.go` | `ListTool`, `ReadTool` | Built-in MCP resource tools. |
| `internal/tools/mcpauth/mcpauth.go` | `Tool` | MCP OAuth helper. |

MCP tools become normal registry descriptors after adaptation. Registry-facing
names are normally `mcp__<server>__<tool>` unless the server is configured to
preserve tool names.

## Config, System Prompt, Hooks, Tasks

| Package | Key types/methods | Responsibility |
| --- | --- | --- |
| `internal/config` | `Config`, `Load`, `LoadPermissions`, `Credentials`, `PragmaHome`, `SessionsDir`, settings path helpers | Reads global/project/local settings, credentials, permissions, toolsets, and path conventions under `~/.pragma` and project `.pragma`. |
| `internal/sysprompt` | `Builder`, `Builder.Build`, `LoadAgentMD`, `DetectEnv`, git helpers | Builds system prompt blocks from static prompt, environment, git state, AGENT.md files, and skills. |
| `internal/hook` | `Manager`, `Execute`, `ExecuteInWorkDir`, `LoadHooks`, `ExecCommand` | Loads and executes hooks for events such as session start, user prompt submit, pre/post tool use, and stop. |
| `internal/task` | `Registry`, `Task`, `Create`, `Update`, `DeliverMessage`, `RequestShutdown`, `Heartbeat`, `ReapDead` | In-memory task/teammate registry and lifecycle messaging. |
| `internal/cron` | `Scheduler`, `Job`, `Create`, `Delete`, `List`, `Start`, `Store` | Scheduled prompt/job management. |
| `internal/compact` | `Service`, `Compact`, `AutoTracker`, token/window helpers | Manual and automatic conversation compaction. |
| `internal/team` | `TeamFile`, `TeamMember`, workspace/team file helpers | Optional agent-team workspace metadata. |
| `internal/skill` | `Loader`, `RuntimeCatalog`, `Skill`, `SubstituteArgs` | Skill discovery/loading and command/tool integration. |

## Background Process Support

| File | Type or method | Responsibility |
| --- | --- | --- |
| `internal/background/registry.go` | `Registry`, `ProcessInfo` | Tracks background sessions/processes. |
| `internal/background/heartbeat.go` | heartbeat helpers | Keeps background process status fresh. |
| `internal/background/subscriber.go` | `StatusSubscriber` | Subscribes to observe events and updates background status/session ID/tool counts. |
| `internal/background/kill_unix.go`, `kill_windows.go` | `Registry.Kill` | Platform-specific process termination. |

`RunBackground` starts a child process. The child still runs the normal
non-interactive runtime; background support adds process registry, log path, and
heartbeat/status updates around it.

## Replay, Inspect, And Artifacts

| Area | Files | Responsibility |
| --- | --- | --- |
| Replay CLI | `cmd/pragma/replay*.go` | Replay and raw HTTP dump/audit/export commands. |
| Inspect CLI | `cmd/pragma/inspect.go` | Conversation/raw HTTP/phases inspection command surface. |
| Observed replay | `internal/observe/replay.go` | Loads event logs and indexes requests/responses/tool output. |
| Session artifacts | `internal/sessionpath/path.go`, `internal/toolresult/storage.go` | Artifact and large tool-result storage paths. |
| SWE runners | `tools/run_swebench_pro_instance.py`, `tools/run_swebench_pro_codex_instance.py` | Local SWE-bench Pro harness integration. |

## JetBrains Reflective MCP Plugin

The plugin under `plugins/jetbrains-reflective-mcp/` is a separate Kotlin/Gradle
runtime loaded inside JetBrains IDEs. It exposes a local HTTP MCP server backed
by IntelliJ Platform APIs and writes discovery metadata for Pragma under
`~/.pragma/jetbrains-mcp/latest.json`.

| File | Class or function | Responsibility |
| --- | --- | --- |
| `plugins/jetbrains-reflective-mcp/src/main/resources/META-INF/plugin.xml` | plugin registration | Registers the IntelliJ plugin and startup activity. |
| `McpStartupActivity.kt` | `McpStartupActivity` | Starts the project service on project startup. |
| `ReflectiveMcpProjectService.kt` | `ReflectiveMcpProjectService.start`, `dispose` | Starts loopback `HttpServer`, registers `McpHttpHandler`, writes status/discovery/port files, and stops server on disposal. |
| `McpHttpHandler.kt` | `McpHttpHandler.handle` | HTTP adapter for GET server info, POST JSON-RPC/MCP messages, and DELETE acknowledgement. |
| `McpProtocol.kt` | `McpProtocol`, `ReflectiveToolRegistryView` | JSON-RPC/MCP protocol handling for initialize, ping, tools/list, and tools/call. |
| `ToolDescriptor.kt`, `ToolDescriptors.kt` | `ToolDescriptor`, `ReflectiveToolCatalog`, `defaultToolDescriptors` | Fixed and reflective tool catalog, schema metadata, and execution wrappers. |
| `Ports.kt` | `IdePorts`, `AgentIdePort`, `ReflectionPort`, input data classes | Stable port interfaces and tool argument shapes. |
| `IntelliJIdePorts.kt` | `IntelliJIdePorts` | Concrete IntelliJ Platform implementation of the stable port interfaces. |
| `AgentIdeRuntime.kt` | `AgentIdeRuntime` | Stable agent object interface for files, editors, search, plugins, breakpoints, and run configurations. |
| `ReflectiveRuntime.kt` | `ReflectiveRuntime` | Reflection-backed object store, roots, class/method/field/constructor access, read actions, write commands, and handle release. |
| `Domain.kt`, `ValueMaps.kt` | data classes and `toBoundaryValue` | Serializable boundary values returned through MCP. |
| `Schema.kt`, `JsonArgs.kt` | schema and JSON argument helpers | JSON Schema generation and request argument decoding. |

The plugin boundary is intentionally MCP/HTTP. The Go runtime should consume it
through `internal/mcp.Manager` as an MCP server, not by importing plugin code.

## Source Ownership Rules

- `cmd/pragma` owns command declarations only; durable runtime construction
  belongs in `internal/cli`.
- `internal/cli` is the composition root. It may wire many packages together.
- `internal/model` owns provider-neutral data. Provider adapters should not leak
  wire types out of `internal/provider/*`.
- `internal/query` owns loop control, conversation appends, and query-local
  event emission.
- `internal/tool` owns the generic descriptor/orchestrator contract.
- `internal/tools/*` own individual tool behavior.
- `internal/observe` owns cross-cutting runtime events; avoid ad-hoc side
  channels when an event is the correct integration point.
- `internal/session` owns durable session JSONL schema and reconstruction.
- `internal/tui` owns presentation, not provider/tool construction.
- `internal/orchestration` and `internal/lifecycle` are distinct runtimes and
  should not be conflated.
- `plugins/jetbrains-reflective-mcp` is a separate Kotlin MCP server boundary.

## Common Extension Points

- Add a CLI command: create `cmd/pragma/<name>.go`, register it in
  `cmd/pragma/main.go`, and put runtime work in `internal/cli` or the owning
  internal package.
- Add a slash command: register a `slash.Command` in
  `internal/slash/commands.go` or a focused `*_cmd.go` file; return a
  declarative `slash.Result`.
- Add a provider: implement `provider.Provider` under `internal/provider/<name>`
  and add selection to `internal/cli/deps.go:CreateProvider`.
- Add a tool: implement `tool.Descriptor` under `internal/tools/<name>` and add
  registration in `internal/cli/tools.go:baseTools` or `RegisterTools`.
- Add a persistent session field: add an entry/data type in `internal/session`,
  write it from the owning runtime, and reconstruct it in `Store.Load`.
- Add an event: define the observe event type/catalog entry in `internal/observe`
  and emit it from the owning runtime boundary.
- Add an orchestration state/control: extend `internal/orchestration` schema,
  validation, execution, projection if needed, and YAML definitions.
- Add a lifecycle node: define or generate it in `internal/lifecycle/bridge` or
  the definition resolver, then surface progress through lifecycle events.
- Add a JetBrains MCP tool: add/update a Kotlin `ToolDescriptor`, port/runtime
  implementation, and target API coverage tests in the plugin.

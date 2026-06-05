# Native Web UI Plan

This plan is governed by `docs/pragma-web-ui-vision.md`. The vision document is
the source of truth for product direction, raw payload display rules, and the
minimum acceptable user experience.

## Scope

The web UI is Pragma's interactive UI. It replaces the default terminal
interactive surface for `pragma` with no one-off command or `--prompt`.

It is not a mode and it is not a browser container for every CLI command.

Out of scope for this UI:

- replay labs
- benchmark setup or benchmark result dashboards
- metrics labs
- audit labs
- lifecycle graph execution UI
- background process management UI
- one-off CLI command execution surfaces

These remain independent CLI paths.

## Responsible Surfaces

`internal/cli` owns runtime wiring:

- config loading
- provider/model setup
- tool registration
- permission checker setup
- session writer setup
- slash dependency setup
- orchestration callback setup

`internal/web` owns browser presentation:

- HTTP server startup
- static frontend serving
- event streaming
- prompt submission
- permission responses
- ask responses
- session list/resume actions

`internal/web` must not own:

- agent loop logic
- tool execution
- permission policy semantics
- session JSONL parsing
- orchestration transitions
- replay, audit, metrics, lifecycle, or benchmark command behavior

## First UI

The first screen is the active interactive run workbench:

- current workspace, provider, model, and session ID
- run overview
- workflow state/progress view when an orchestration is active
- orchestration handoff file list and exact file inspector
- compact activity timeline from `query.LoopEvent`
- prompt composer
- slash command submission through the existing slash registry
- tool calls and tool results
- permission prompt panel
- ask prompt panel
- session list and resume
- interactive orchestration progress when launched by slash command

The activity timeline is not the whole UI. It is one workbench tab beside
Workflow, Handoffs, and State.

## Event Model

The web UI consumes structured events from existing runtime interfaces:

- `query.LoopEvent` for live interactive progress
- `observe.Event` for durable runtime evidence when exposed by existing
  subscribers

Web must not parse bracketed transcript text for control state. Orchestration
progress uses typed `query.Orchestration*Event` values and matching durable
`observe.Orchestration*` events.

## Interaction Boundaries

Prompt submission calls the existing `query.Engine`.

Slash command submission calls the existing `slash.Registry` with existing
`slash.Deps`.

Interactive orchestration calls the existing orchestration runner through the
callback wired by `internal/cli`.

Permission prompts implement `permission.Prompter`.

Ask prompts implement `tool.Asker`.

Session resume uses `internal/session.Store`, updates the existing
`app.StateStore`, resets the engine content replacement state, and switches the
session writer through the callback supplied by `internal/cli`.

## Research-Informed UI Principles

Existing workflow and agent UIs converge on these concepts:

- Runs/executions are the primary navigation object.
- Workflow structure and run history are separate from logs.
- A graph or state list answers "where are we?"
- A timeline or event list answers "what happened?"
- Details panes preserve exact event/input/output/source payloads.
- Saved filters or views help users revisit the same operational slice.
- Failed/pending/blocking work needs a first-class surface.

Pragma should map those concepts to its own runtime:

- run: current interactive execution plus resumable session history
- workflow: typed `query.Orchestration*Event` state and transition events
- handoff: emitted handoff prompt files plus session handoff state JSON
- activity: model/tool/text/error/permission/ask events
- source: complete event envelopes, session payloads, app state, and file
  contents shown without field loss

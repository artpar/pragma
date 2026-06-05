# Pragma Web UI Vision

## Product Stance

Pragma is an interactive agent workbench for people who need to understand and
control long-running coding runs. The web UI is not a marketing page, not a
dashboard lab, and not a prettier terminal dump.

The best UI for Pragma is an end-user run workbench. The primary object is a
run or workflow execution, not a chat transcript. Chat-like text, tool activity,
model requests, state handoffs, orchestration handoff files, and raw events are
all inspectable surfaces under the active run.

The workbench should borrow the durable workflow UI pattern used by workflow
and agent systems:

- a run list/history for navigation
- a live workflow/state view for current progress
- a compact event timeline for evidence
- tabs for activity, graph/state, handoffs, and raw source data
- drill-down details for the selected run, state, event, tool call, or file
- full source payloads available without silent reducers

- minimal chrome
- fixed, ergonomic work areas
- clear activity, decision, and session surfaces
- complete source data always available
- no hidden reducers
- no silent summaries
- no transformed state standing in for source payloads

The UI should help users run Pragma day to day. Complete records are available
for trust, review, and escalation, but the primary surface should read like a
usable workbench, not a debug console.

## Hard Data Rule

The UI must never silently drop or transform fields from events, messages,
sessions, tool calls, tool results, permission requests, ask requests, or
orchestration payloads.

Allowed presentation operations:

- JSON indentation
- syntax highlighting
- wrapping
- folding and expanding, with all fields still present
- filtering by item type while preserving the complete item when opened
- labels that point to raw fields
- search and highlight
- stable ordering
- timestamps and data type metadata added outside the payload

Not allowed:

- replacing payloads with simplified structs
- whitelisting selected fields as the only visible content
- hiding unknown fields
- converting events into prose-only transcript entries
- rebuilding session state from partial message text
- showing a summary without the full raw object attached
- parsing bracketed text to infer control state

Every rendered item has two layers:

1. A compact, user-facing header and preview.
2. The complete source object, formatted but unmodified.

The header is a working view, not a substitute for the source object.

## Layout

The app uses a fixed-height shell. The browser page itself must not scroll.
Each region scrolls independently.

Desktop:

- left rail: runs, active run identity, recent sessions
- center: run workbench with tabs for Overview, Workflow, Handoffs, Activity,
  and State
- right details pane: selected item, complete source object, related records,
  or selected file content
- bottom composer: always visible

Mobile:

- top rail: active run identity and horizontally scrollable session/run list
- center: selected workbench tab
- bottom composer: always visible
- details pane opens as a full-height drawer

The input box must never be pushed below the viewport by session list content or
event stream content.

## Visual Direction

The UI should be quiet, dense, and utilitarian.

Use:

- near-white tinted surfaces
- one restrained blue action color
- thin borders
- compact typography
- generous line-height inside JSON details
- fixed-width font only for source objects and command-like text

Avoid:

- decorative gradients
- colorful status confetti
- card-heavy SaaS layout
- giant hero empty states
- marketing copy
- rounded pill clutter
- icon noise

This should feel like a quiet professional tool for steering agent work, not a
debugger, packet inspector, or startup dashboard.

## Primary Objects

### Session

A session row is not a prompt snippet pretending to be a title.

Display:

- session ID
- model
- workdir
- turn count
- cost
- updated time
- complete `SessionSummary` JSON in an expandable details block

Clicking a session resumes it and appends a `session_resumed` event containing
the full loaded session payload. The UI must not replace this with `{id}` or
try to reconstruct a conversation from text-only messages.

### Run

A run is the top-level UI object.

Display:

- current status
- provider and model
- workspace
- active session ID
- current orchestration, when present
- latest blocking permission or ask prompt
- recent tool/model/error activity
- complete runtime and app state JSON in the State view

The current backend supports one active interactive run at a time. The UI should
be shaped as a run workbench so multiple active runs can be added later without
turning the interface back into a scrollback transcript.

### Event

The activity timeline is one workbench tab, not the whole product.

Each event row shows:

- sequence number
- readable activity name
- event envelope type, such as `loop_event`, `permission_request`, `ask_request`
- concrete data type, such as `query.ToolCallEvent`, available in details
- timestamp received by browser
- complete JSON source object

Rows can be folded, but folding must not remove data. The default can be
compact for navigation, but opening the row must reveal the complete envelope.

### Message

Messages are records, not just chat bubbles.

When shown, a message displays:

- role
- flags
- all content parts
- raw message JSON

A transcript-like view can exist later, but only as a secondary projection
beside the raw record, never as the source view.

### Tool Call And Result

Tool call and result rows must show the complete source object.

Useful visual affordances:

- tool name in the row header
- call ID
- error marker when present
- duration if present
- input JSON
- result JSON
- display text as an additional field, not a replacement for raw result content

### Permission And Ask

Permission and ask prompts are blocking interaction surfaces for end users.

The prompt panel shows:

- clear decision or answer controls first
- exact request object
- available decision controls
- session-rule option when supported
- submitted response object after answer

No permission request should be reduced to only a command preview.

### Orchestration

Interactive orchestration is a first-class workflow view built from typed
events, not parsed text.

Display:

- `query.OrchestrationStartedEvent`
- `query.OrchestrationStateStartedEvent`
- `query.OrchestrationControlEvent`
- `query.OrchestrationHandoffEvent`
- `query.OrchestrationTransitionEvent`
- `query.OrchestrationStateCompletedEvent`
- `query.OrchestrationCompletedEvent`

The Workflow tab displays current state, state history, transitions, control
events, and state durations. Selecting any workflow row opens the exact source
event in the inspector.

The UI may draw a graph later, but the typed events remain the complete source
record.

### Handoffs

Pragma has two handoff surfaces and the UI must keep them distinct:

- orchestration handoff prompt files emitted by
  `query.OrchestrationHandoffEvent.Path`
- session handoff state stored in the current app/session state

The Handoffs tab lists handoff files by state, event, path, and direction. The
file inspector reads the exact emitted file path when available and shows the
content beside the source event. It must not invent a transcript summary of the
handoff.

Session handoff state is shown from the source JSON already carried in app or
session state.

## First Screen

The first screen should immediately show the active run workbench.

Required visible elements:

- Pragma wordmark, small and plain
- provider/model
- workspace path
- session ID
- recent session/run list
- run overview tab
- workflow tab, even when empty
- handoff tab, even when empty
- activity tab
- fixed composer

Empty state text should be short:

> Start a message or run a slash command.

No feature explanations. No onboarding hero.

## Composer

The composer is always visible.

Behavior:

- Enter sends
- Shift+Enter inserts newline
- disabled while a run is actively accepting no new input
- visible error if submit fails
- no page scroll needed to reach it

Slash commands are typed in the same composer. A fuzzy command menu can be
added, but command execution still goes through `slash.Registry`.

## Inspector

Selecting any run, workflow row, handoff, event, session, permission prompt, ask
prompt, or file opens the details pane.

Details tabs:

- Full JSON: full envelope JSON
- Related: links to same tool call ID, session ID, trace ID, or state ID
- File/Text: optional exact file content or readable text fields extracted from
  the same raw object

The Full JSON tab is complete and always available.

## Search And Filtering

Search is full-payload search.

Filters are visual filters only:

- all
- model
- text
- tool
- permission
- ask
- orchestration
- error

Filtering hides rows from the current view, but it does not alter stored event
objects or session payloads.

## Implementation Boundary

`internal/web` owns presentation and transport only.

It can:

- serve HTML/CSS/JS
- stream raw event envelopes
- submit prompts
- send permission answers
- send ask answers
- request session resume
- render complete source objects

It must not:

- define simplified event structs
- define simplified message structs
- parse session JSONL into UI-specific models
- execute tools
- decide permission policy
- decide orchestration transitions
- own replay, metrics, audit, lifecycle, benchmark, or one-off CLI behavior

If the UI needs a field, the owning component should expose the actual canonical
record. The web layer should not reconstruct it.

## Minimum Acceptable Version

A good first version is:

- fixed shell with independent scroll regions
- clean session rail
- activity stream with foldable complete JSON envelopes
- always-visible composer
- permission and ask panels that show full raw requests
- session resume event with full session payload
- no silent reducers
- no raw payload field loss

This version may be visually simple. It is not acceptable if it is unclear,
scroll-broken, or silently hides data.

## Completion Bar

The web UI is good enough only when a user can:

- start `pragma`
- see the active workspace and model immediately
- use the composer without scrolling
- submit `/help`, `/doctor`, `/model`, and `/orchestrate`
- see every emitted event as a complete envelope
- inspect session summaries without losing fields
- resume a session and see the full loaded session payload
- answer permission and ask prompts while seeing complete request objects
- use desktop and narrow viewports without losing the composer

Screenshots must prove:

- desktop first screen
- desktop after slash command output
- desktop expanded event/session JSON
- narrow viewport with composer visible

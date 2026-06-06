# Web UI Implementation Goals

These goals translate `docs/pragma-web-ui-vision.md` into implementation
targets for a new React/Vite/Vitest frontend subproject that is statically
built and embedded into the Pragma Go binary for native serving.

The vision document remains the product source of truth. This document defines
the implementation bar.

`docs/clean-architecture-enforcement-doctrine.md` is a governing implementation
rule for this work. Any web UI implementation change that affects runtime
state, durable records, lifecycle transitions, protocol contracts, permissions,
sessions, orchestration, external side effects, or user-visible workflow must
identify the single source-enforced owner before code is edited.

## Goal 0: Enforce Ownership Before Building UI Paths

Implement the web UI by routing through existing owners or by creating missing
owners when no enforced path exists. Do not create browser, transport, or
frontend-specific copies of runtime behavior.

Before each implementation slice, answer the clean architecture ownership test:

- What invariant changes?
- Which component owns that invariant?
- Which method or transaction applies the mutation?
- Which consumers may observe it?
- Which consumers are forbidden from reconstructing, duplicating, or inferring
  it?
- What happens on failure, cancellation, resume, replay, crash, retry, and
  shutdown?
- What test or architecture check prevents a future bypass?

Acceptance:

- Work notes or PR text name the owner for each changed invariant.
- UI code receives typed records, source objects, or explicit projections from
  owners instead of reconstructing domain state.
- Duplicate web/TUI/CLI/runtime ownership paths are deleted or routed through a
  single typed owner where possible.
- Every implementation slice adds a focused test, type restriction, or
  architecture check, or explicitly states the missing enforcement mechanism.

## Goal 1: Ship A Native Run Workbench

Implement the web UI as Pragma's native interactive surface for no-prompt
interactive runs.

The first screen shows an active run workbench, not a landing page, marketing
page, transcript clone, or debug console. The user must immediately see the
workspace, provider, model, session identity, recent sessions, workbench tabs,
activity, and composer.

Acceptance:

- `pragma` starts the native web server for interactive use.
- The browser receives the compiled React app from the Go binary.
- The fixed shell renders without page-level scrolling.
- The composer is visible on desktop and narrow viewports.

## Goal 2: Preserve Source Payloads End To End

Render every event, session, message, tool call, tool result, permission
request, ask request, orchestration event, and state object with its complete
source payload available.

The frontend may create headers, labels, filters, folded rows, search matches,
and previews, but those are projections. They must never replace the source
object.

Acceptance:

- Every rendered record has a compact working row and a complete JSON view.
- Unknown fields remain visible in the full JSON view.
- Filtering hides rows visually without mutating or reducing stored records.
- No frontend code rebuilds control state from transcript text.

## Goal 3: Keep Runtime Ownership In Go

Use the existing interactive runtime and web transport ownership. The React app
is a presentation client over canonical HTTP/SSE contracts. It does not execute
tools, decide permission policy, parse session JSONL into substitute models, or
own orchestration semantics.

Acceptance:

- Runtime setup continues through `internal/cli.BuildInteractiveRuntime`.
- `internal/web` serves static assets and exposes transport endpoints.
- Tool execution, permission decisions, slash commands, session loading, and
  orchestration remain owned by their existing Go packages.
- Frontend types model API contracts and raw payload containers, not alternate
  domain ownership.
- Any new backend convenience endpoint names its source owner and exposes that
  owner's record or projection instead of a web-owned substitute.

## Goal 4: Build A Real React/Vite/Vitest Subproject

Create a dedicated frontend subproject with normal modern tooling and clear
integration points into the Go build.

Target shape:

- `webui/package.json`
- `webui/vite.config.ts`
- `webui/vitest.config.ts` or a shared Vite test config
- `webui/tsconfig*.json`
- `webui/src/`
- `webui/src/api/`
- `webui/src/components/`
- `webui/src/features/`
- `webui/src/styles/`
- `webui/src/test/`

Acceptance:

- `npm install` or the chosen package-manager install works from `webui/`.
- `npm run dev` runs against a local Pragma web API.
- `npm run build` emits static assets into a deterministic dist directory.
- `npm test` runs Vitest unit tests.
- The Go build can embed the production output without requiring a Node runtime
  at execution time.

## Goal 5: Embed Static Assets Into The Go Binary

Compile the React app to static files and serve them from `internal/web` using
Go embedding.

The native binary should be self-contained after build. Running the binary must
not depend on the frontend source directory or a development server.

Acceptance:

- A Go `embed.FS` owns the built `webui/dist` assets or copied equivalent.
- `internal/web` serves `index.html`, JS, CSS, and static assets from the embed.
- Unknown non-API paths fall back to `index.html` for client-side routing if
  routing is introduced.
- API routes keep their existing `/api/...` namespace and are not shadowed by
  static serving.

## Goal 6: Implement The Minimum Workbench Surface First

Prioritize the minimum acceptable version from the vision before adding richer
views.

Required first-version surfaces:

- fixed shell with independent scroll regions
- left or top session rail depending on viewport
- Overview, Workflow, Handoffs, Activity, and State tabs
- right details pane on desktop
- full-height details drawer on narrow viewports
- foldable complete JSON records
- always-visible composer
- permission and ask blocking panels

Acceptance:

- Empty Workflow and Handoffs tabs still exist.
- The Activity tab is one tab, not the entire app.
- The State tab shows complete runtime/app state JSON.
- Selecting any supported item opens Full JSON in the inspector.

## Goal 7: Make Blocking Decisions Explicit

Permission and ask prompts must be end-user decision surfaces, not generic
event rows or command previews.

Acceptance:

- The active prompt panel shows controls before raw details.
- The exact request object is visible before submission.
- Submitted responses are emitted back into the activity stream.
- Session-rule controls appear only when backend support exists.
- Denied, expired, answered, and unavailable prompts have visible states.

## Goal 8: Support Session Navigation And Resume Without Field Loss

The session rail and session inspector must expose real session records rather
than prompt snippets or frontend summaries.

Acceptance:

- Session rows show ID, model, workdir, turn count, cost, and updated time when
  available.
- Each session row exposes the complete `SessionSummary` JSON.
- Resume calls the backend resume endpoint.
- Resume emits a `session_resumed` record with the full loaded session payload.

## Goal 9: Treat Orchestration And Handoffs As Typed Records

Workflow and handoff views are built from typed events and source state, not
from parsed transcript text.

Acceptance:

- Workflow rows represent `query.Orchestration*Event` records.
- Selecting a workflow row opens the exact source event.
- Handoff files and session handoff state are displayed as distinct surfaces.
- File content is shown only when fetched through a backend-approved artifact
  or file endpoint.

## Goal 10: Verify With Unit, Contract, And Browser Evidence

Use Vitest for frontend behavior and focused Go tests for embed/server behavior.
Use browser screenshots for the user-visible completion bar.

Acceptance:

- Vitest covers event ingestion, filtering, selection, JSON rendering, composer
  behavior, and prompt panel state.
- Go tests cover static asset serving, API/static route separation, and fallback
  behavior.
- Screenshots prove desktop first screen, desktop slash command output,
  expanded event/session JSON, and narrow viewport with composer visible.

## Non-Goals For The First Implementation

- No full code editor.
- No marketing or onboarding hero.
- No benchmark, replay, metrics-lab, audit-lab, or lifecycle-lab UI.
- No browser-side tool execution.
- No frontend-owned session parser.
- No web-owned lifecycle state machine that duplicates runtime ownership.
- No permission, ask, slash, session, or orchestration semantics copied into
  React for convenience.
- No workflow graph unless the typed list view is already complete.
- No hidden reducers introduced for convenience.

## Architecture Review Gate

The implementation is not acceptable merely because the UI renders or tests
pass once. Reject the change if any user-visible workflow depends on display
text parsing, event-order inference, duplicate permission/session/orchestration
logic, web-owned persistence, or lifecycle cleanup outside the runtime owner.

The acceptable completion claim is: this invariant is owned here, every caller
uses that owner, and this check fails if a future caller bypasses it.

## Completion Statement

The implementation is complete when a user can start `pragma`, use the embedded
web UI without a development server, submit slash commands and prompts, inspect
complete source payloads for emitted records, answer blocking prompts with full
request visibility, resume sessions without field loss, and use the interface
on desktop and narrow viewports without losing the composer.

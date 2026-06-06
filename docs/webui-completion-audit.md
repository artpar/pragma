# Web UI Completion Audit

This audit checks the current native web UI against `docs/webui-implementation-goals.md`,
`docs/clean-architecture-enforcement-doctrine.md`, and `pragma-webui-vision.md`.

## Goal 0: Ownership Before UI Paths

Status: achieved for the implemented first-version surface.

Evidence:

- Owner notes are recorded in `docs/webui-ownership-notes.md`.
- Runtime mutations route through `internal/cli.BuildInteractiveRuntime`,
  `web.Config.RunInput`, `web.Config.Resume`, `internal/web.Bridge`,
  `session.Store`, `session.Writer`, `app.StateStore`, and `internal/slash`.
- Durable web event history is owned by `session.Writer` and loaded by
  `session.Store`; `internal/web.hub` owns only transport sequencing and
  in-process recent events.
- Enforcement exists in `internal/web`, `internal/session`, `internal/cli`, and
  `webui` tests.

Residual risk:

- This is an implementation-slice audit, not a claim that every future richer
  workbench surface has an owner yet.

## Goal 1: Native Run Workbench

Status: achieved.

Evidence:

- `RunInteractive` launches `web.Run` for no-prompt interactive use.
- `internal/web/static_assets.go` serves the embedded React app.
- Embedded browser evidence:
  `.tmp-web-native-live-desktop-embedded-4817.png` and
  `.tmp-web-native-live-narrow-embedded-4817.png`.
- Live geometry checks showed no body scroll and visible composer on desktop and
  narrow viewports.

## Goal 2: Source Payload Preservation

Status: achieved for events, messages, sessions, prompts, state, artifacts, and
orchestration records in the first-version workbench.

Evidence:

- `webui/src/components/Inspector.tsx` and `JsonView.tsx` expose complete source
  JSON.
- `webui/src/features/workbench.ts` stores source objects in
  `SelectableRecord.source`.
- Vitest covers unknown field preservation, filtering without mutation, full
  event JSON, message source records, loaded full-session JSON, and typed
  orchestration JSON.
- Session web events are persisted as `session.WebEventData` raw payloads.

## Goal 3: Runtime Ownership In Go

Status: achieved.

Evidence:

- Runtime setup remains in `internal/cli.BuildInteractiveRuntime`.
- `internal/web` exposes transport endpoints and static serving only.
- Slash completions delegate to `internal/slash.CompletionItems`.
- Session list/load/resume delegate to `session.Store` and `web.Config.Resume`.
- Permissions and asks resolve through `internal/web.Bridge`, not React policy.
- Artifacts are fetched only after backend ownership checks.

## Goal 4: React/Vite/Vitest Subproject

Status: achieved.

Evidence:

- `webui/package.json`, `vite.config.ts`, `vitest.config.ts`,
  `tsconfig*.json`, `src/api`, `src/components`, `src/features`,
  `src/styles`, and `src/test` exist.
- `npm test` runs Vitest.
- `npm run build` emits production assets.
- Go embeds copied production assets under `internal/web/static`.

## Goal 5: Embedded Static Assets

Status: achieved.

Evidence:

- `internal/web/static_assets.go` owns embedded static serving.
- `internal/web/static` contains `index.html` and built JS/CSS assets.
- `internal/web/web_static_test.go` covers index serving, client route fallback,
  API/static separation, and explicit bind address behavior.
- `go build ./cmd/pragma` succeeds without a Node runtime.

## Goal 6: Minimum Workbench Surface

Status: achieved.

Evidence:

- `webui/src/App.tsx` renders session rail, top strip, Overview, Workflow,
  Handoffs, Activity, State, inspector, prompt slot, and composer.
- `webui/src/styles/*.css` implement fixed shell, independent scroll regions,
  desktop inspector, and narrow drawer layout.
- Vitest covers first-screen workbench controls.
- Browser geometry checks verified desktop and narrow composer visibility.

## Goal 7: Explicit Blocking Decisions

Status: achieved for the supported permission and ask prompt surfaces.

Evidence:

- `webui/src/components/PromptPanel.tsx` renders decision controls before raw
  request JSON.
- `internal/web/bridge.go` emits permission and ask request/expired events.
- `internal/web/prompt_responses.go` publishes response events.
- Vitest covers pending, denied, expired, answered, and unavailable states.
- Backend tests cover expired prompts and submitted response events.

## Goal 8: Session Navigation And Resume

Status: achieved.

Evidence:

- Session rail renders ID, model, workdir, turn count, cost, and updated time.
- Session row click loads `/api/sessions/{id}` without mutating resume state.
- Resume is an explicit inspector action calling `/api/resume`.
- `internal/web/sessions.go` emits `session_resumed` with the full loaded
  `session.Session`.
- Vitest and Go tests cover these paths.

## Goal 9: Typed Orchestration And Handoffs

Status: achieved for the first-version typed list view.

Evidence:

- `internal/web/events.go` maps `query.Orchestration*Event` values to typed web
  event names.
- `webui/src/features/workbench.ts` builds workflow and handoff records from
  typed events and `app_state` surfaces.
- `internal/web/artifacts.go` only reads recorded orchestration artifact paths.
- Vitest covers typed workflow event inspection, handoff event inspection,
  handoff state, and backend-mediated artifact fetches.

Residual risk:

- The non-goal workflow graph is not implemented, by design.

## Goal 10: Unit, Contract, And Browser Evidence

Status: achieved for the current implementation.

Evidence:

- Vitest covers event ingestion, filtering, selection, JSON rendering, composer
  behavior, session behavior, prompt panel state, orchestration, and artifacts.
- Go tests cover JSON:API contracts, pagination, static serving, route
  separation, fallback behavior, bind address selection, durable web events,
  prompt response events, and session resume payloads.
- Browser evidence exists for desktop first screen, desktop slash command
  output, expanded event JSON, expanded session JSON, and narrow composer.

## Completion Statement Check

Current evidence supports the first implementation completion statement:

- A user can start `pragma` and use the embedded web UI without a development
  server.
- The user can submit slash commands and prompts through JSON:API.
- Emitted records expose complete source payloads in the inspector.
- Blocking prompts expose full request visibility and explicit controls.
- Session navigation and resume preserve full session payloads.
- Desktop and narrow viewports preserve the composer.

The remaining work is future-scope enrichment from the vision document:
diff review, validation dock, MCP registry, policy admin, agent boards, audit
exports, and orchestration graph views.

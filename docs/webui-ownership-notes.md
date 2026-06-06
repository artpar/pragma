# Web UI Ownership Notes

These notes record source-enforced owners for the current native web UI slices.
They support `docs/webui-implementation-goals.md` Goal 0 and should be updated
when a web UI change touches runtime state, protocol contracts, durable records,
permissions, sessions, orchestration, or user-visible workflow.

## Runtime And Run State

- Invariant: a prompt starts at most one active interactive run, and cancellation
  stops that run through its runtime context.
- Owner: `internal/cli.BuildInteractiveRuntime` wires the runtime dependencies;
  `internal/web.server.beginRuntimeTransition` guards the web transport's active
  run slot.
- Mutation path: `/api/prompt` calls `Config.RunInput`; `/api/cancel` calls the
  stored cancel function.
- Allowed observers: React reads `/api/state` and `/api/events`; it does not
  execute tools or mutate runtime state directly.
- Enforcement: focused web tests cover JSON:API prompt/cancel transport and
  frontend composer submission.

## Web Transport Boundary

- Invariant: the native web surface is an HTTP/SSE transport over existing
  runtime owners, not a second runtime, session, permission, slash, or
  orchestration implementation.
- Owner: `internal/web.server` owns route registration and transport lifecycle;
  responsibility-specific files expose runtime, event, session, prompt,
  artifact, completion, static, and JSON:API handlers without changing domain
  owners.
- Mutation path: mutating routes delegate to existing owners through
  `Config.RunInput`, `Config.Resume`, `Bridge`, `session.Store`,
  `app.StateStore`, and `internal/slash`.
- Allowed observers: React receives typed API records, event envelopes, source
  JSON, and owner-produced projections.
- Forbidden consumers: `internal/web` must not grow alternate domain state
  machines or parse display text to reconstruct runtime state.
- Enforcement: production web files remain below the project file-size limit,
  and `go test ./internal/web` exercises the split transport contracts.

## Native Web Dev Binding

- Invariant: Vite development uses the same Go-owned web API transport as the
  embedded binary, with a deterministic bind address when requested.
- Owner: `internal/web.Run` owns the HTTP listener address; `internal/cli`
  owns interactive runtime wiring and passes `--web-addr` into `web.Config`.
- Mutation path: `pragma --web-addr 127.0.0.1:4817` starts the native web API
  on the address that `webui/vite.config.ts` proxies to by default.
- Allowed observers: Vite may proxy `/api` to the configured local Pragma API,
  or to `PRAGMA_WEB_API` when the developer chooses a different address.
- Forbidden consumers: React must not start a second runtime or guess a session
  owner to satisfy development mode.
- Enforcement: `internal/web` tests cover bind-address selection; `internal/cli`
  tests cover flag registration.

## JSON:API Protocol Contract

- Invariant: every non-static HTTP API response is a JSON:API document, and
  every HTTP collection uses `page[number]`/`page[size]` pagination with
  top-level pagination `links` and `meta.page`.
- Owner: `internal/web/jsonapi.go` owns media-type validation, document
  envelopes, request resource decoding, error documents, and pagination
  contracts.
- Mutation path: mutating routes require `application/vnd.api+json` request
  resources; collection routes call `pageRequestFromQuery` and reject invalid
  pagination as JSON:API errors with `source.parameter`.
- Allowed observers: React and external clients consume resource documents,
  collection documents, error documents, links, and page metadata instead of
  ad hoc JSON shapes.
- Streaming exception: `/api/events` is an EventSource stream for browser
  compatibility, but each `data:` payload is a JSON:API event resource document.
  `/api/events/recent` exposes the same event resources as a paginated
  JSON:API HTTP collection.
- Enforcement: focused web tests cover resource, collection, error, SSE event
  resource, invalid pagination, and paginated recent-event contracts.

## Event Backfill And Streaming

- Invariant: browser refresh and live streaming show the same web event
  envelopes without dropping source payloads or duplicating replayed events.
- Owner: `internal/web.hub` owns in-process web event sequencing, recent event
  retention, SSE subscribers, and `/api/events/recent` collection data;
  `internal/session.Writer` owns durable `web_event` entries for active
  sessions and `internal/session.Store` owns loading them back.
- Mutation path: runtime, bridge, session, prompt, permission, ask, slash, and
  orchestration handlers publish normalized envelopes through `hub.publish`;
  the web transport records the emitted envelope through the configured
  session event recorder, which buffers the first bounded event window until
  the runtime-owned session writer exists.
- Allowed observers: React may request `/api/events/recent` for JSON:API
  backfill and subscribe to `/api/events` for live updates. Server startup and
  resume may seed recent events from the session-owned durable event entries.
- Forbidden consumers: React must not reconstruct event history from transcript
  text, session summaries, rendered rows, or EventSource ordering alone.
- Enforcement: `internal/session` tests cover durable event round-trip and
  rewrite preservation; `internal/web` tests cover session-seeded recent-event
  backfill and event-recorder publication; Vitest covers JSON:API recent-event
  backfill plus sequence-based dedupe of replayed SSE events.

## Workbench Layout

- Invariant: the native workbench shell has no page-level scroll, the tab panel
  owns the flexible vertical space, and the composer remains a compact visible
  row on desktop and narrow viewports.
- Owner: React owns the rendered workbench slots in `webui/src/App.tsx`; CSS
  owns the fixed shell and responsive geometry in `webui/src/styles/*.css`.
- Mutation path: user-visible layout changes go through the React slot
  structure and stylesheet ownership slices, not through runtime state or
  backend transport handlers.
- Allowed observers: browser tests, screenshots, and DOM geometry checks may
  verify placement and overflow.
- Forbidden consumers: runtime, session, permission, slash, and orchestration
  owners must not encode presentation layout assumptions.
- Enforcement: Vitest covers the expected shell controls, and live browser
  geometry checks verify no body scroll plus visible compact composer rows on
  desktop and narrow viewports.

## Sessions And Resume

- Invariant: session summaries and full resumed sessions preserve their source
  payloads without frontend-owned parsing, and inspecting a saved session does
  not mutate the active runtime.
- Owner: `internal/session.Store` owns listing and loading session records,
  including durable web event entries; `Config.Resume` owns runtime resume
  mutation.
- Mutation path: `/api/sessions` lists `SessionSummary` records;
  `/api/sessions/{id}` loads the full `session.Session` through
  `session.Store.Load`; `/api/resume` calls `Config.Resume`, then loads the
  full `session.Session` from `session.Store` and emits `session_resumed`.
- Allowed observers: React renders `SessionSummary` records and
  `session.Session` payloads as source JSON.
- Forbidden consumers: React must not parse session JSONL or reconstruct session
  state from transcript text.
- Enforcement: `internal/web` tests cover JSON:API session collections,
  read-only full-session resources, and resume events; Vitest covers session row
  JSON, explicit resume, and loaded full-session inspection.

## Slash Completions

- Invariant: slash command and path completion semantics are owned by the slash
  subsystem, not the browser.
- Owner: `internal/slash.CompletionItems` owns completion grammar and matching.
- Mutation path: completions do not mutate runtime state; `/api/completions`
  exposes a paginated JSON:API collection of slash-owned completion records.
- Allowed observers: React asks the endpoint for records and applies the
  selected `replacement` string.
- Forbidden consumers: React must not duplicate slash command parsing,
  orchestrate flag rules, or filesystem completion rules.
- Enforcement: Go tests cover the JSON:API collection contract; Vitest covers
  composer fetch and selection behavior.

## Permissions And Ask Prompts

- Invariant: permission and ask decisions resolve exactly one pending request
  created by the runtime bridge.
- Owner: `internal/web.Bridge` implements `permission.Prompter` and
  `tool.Asker`; existing permission and tool packages own policy and execution.
- Mutation path: `/api/permission/{id}` and `/api/ask/{id}` resolve bridge
  channels and publish response events.
- Allowed observers: React renders explicit controls and raw request JSON.
- Forbidden consumers: React must not decide permission policy, infer request
  state from display text, or persist permission rules without backend support.
- Enforcement: Vitest covers pending, denied, expired, answered, and unavailable
  prompt states; web tests cover expiration and submitted response events.

## Orchestration Artifacts

- Invariant: orchestration and handoff views render typed owner-produced records;
  file content is only shown when it belongs to a recorded session artifact.
- Owner: `app.StateStore` owns current app state; orchestration runtime owns
  artifact records in that state; `internal/query` owns
  `query.Orchestration*Event` event payloads.
- Mutation path: `/api/artifact` checks `app_state.orchestration_artifacts`
  before reading file content; workflow and handoff tabs observe normalized web
  event envelopes produced from query orchestration events.
- Allowed observers: React displays typed workflow events, handoff events,
  handoff state, artifact records, and fetched artifact content.
- Forbidden consumers: React must not read arbitrary filesystem paths or infer
  artifact ownership from transcript text.
- Enforcement: `internal/web` tests cover artifact access; Vitest covers typed
  workflow event inspection, handoff event inspection, and backend-mediated
  artifact fetches.

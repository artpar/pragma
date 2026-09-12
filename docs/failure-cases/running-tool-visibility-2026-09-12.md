# TUI-003 — A running tool call shows no elapsed time, so the operator
# cannot tell a long call from a hung one

Opened 2026-09-12 from the operator's live report. Methodology: `agent.md`.

## Source events (authentic, recorded)

1. **Operator live report, 2026-09-12 19:02:27 (verbatim):** *"the ui for
   human is quite bad and doesnt tell the human at all whats *really*
   going on."* Delivered at the exact moment a long tool call returned.
2. **The anchor event — a 361-second silent tool gap in this very
   session.** Log `~/.pragma/logs/2026-09-12T18-53-02.jsonl`: 42
   `APIRequestCompleted` events; the only inter-request gap over 60s is
   **361.2s, 18:57:27 → 19:03:28** — the window in which the session's
   `go test ./internal/tui` + `go build ./...` tool call ran (aborted at
   19:02:27 after ~5 minutes, then re-run and verified). The operator
   watched the TUI through that entire window and, by their report, could
   not tell what was really going on.

Source revision at case opening: `be42052` (working tree, clean).

## Production path and the earliest wrong transition

What the default view renders between `ToolCallEvent` and
`ToolResultEvent` (verified in the production path,
`internal/tui/handlers.go:357-380`): the tool-call line
`render.RenderToolCall` (tool name plus the primary param truncated to
width — for Bash the `command` string, `internal/tui/render/content.go:203`,
`primaryParams["Bash"]="command"`), a spinner line at the bottom of the
viewport (`internal/tui/model.go:471-473`: `spin.View() + " " +
m.spinnerTool + "..."`), and the toolbar status `"executing: " +
e.Call.Name` (`handlers.go:377`). The spinner ticks re-render the
viewport each tick (`model.go:317-326`), but nothing anywhere records or
renders *when the call started* or *how long it has been running*.
`ToolResultEvent` (`handlers.go:382+`) then renders the full output only
at completion.

The earliest wrong transition is the running-state render decision: the
spinner line is the dedicated "a tool is running" surface, refreshed
~10×/s, yet it carries no elapsed time — for six minutes the operator saw
the same animating `⣾ Bash...` they would see for six seconds, with no
factual basis to distinguish a long build from a hung call (and no live
output until completion, which magnified it).

## Observed vs expected

Observed (this session, the 361s gap, production render path): a truncated
command line, the toolbar's `executing: Bash`, and a bare animating
spinner `⣾ Bash...` — no duration information of any kind. The operator
could not tell how long the call had been running or whether it was
progressing; their report is the failing observation.

Expected: while a tool call is running, the default view shows its
elapsed time, continuously — 10s must look different from 6m, and an
advancing elapsed with an animating spinner states the call is live.
Source: the operator-communication contract in `agent.md` (the harness
must keep the operator informed; the operator's own words are the
requirement: *"whats really going on"*) and the TUI delivery class
established by TUI-001/TUI-002 (operator-facing information is visible by
default). Not claimed: streaming live tool output (a separate, larger
executor-level mechanism — open only if evidence warrants); interruption
UX; multiplexing several parallel calls onto the single spinner line.

## Proposed mechanism (one)

A `spinnerToolStartedAt` timestamp captured when the spinner activates
for a tool call — set when the spinner was inactive, and reset when the
named tool changes (sibling same-name parallel calls keep the first
stamp) — rendered as a human-readable suffix on the existing spinner
line, refreshed by the existing per-tick viewport re-render:
`⣾ Bash... 45s` / `⣾ Bash... 5m11s`. One surface (the viewport spinner
line), one piece of state; no executor, loop, or toolbar changes. The
spinner line already disappears when the spinner deactivates
(`model.go:471` renders it only `if m.spinnerActive`), so the elapsed
clears with it.

## Executable assertion (gate)

`go test ./internal/tui -run TestRunningToolShowsElapsedTime -count=1`
must fail on `be42052` (baseline RED: the running-tool view carries no
elapsed information) and pass on the candidate: drive the production path
with this session's real Bash call shape, stamp a fixed past start time,
and assert the default `View()` renders a human-readable elapsed
(311s → `5m11s`). Adjacent assertions:
`TestSpinnerElapsedClearsAfterResult` (after `ToolResultEvent` neither
spinner line nor elapsed remains), `TestNoSpinnerWhenIdle`, and the
TUI-001/TUI-002 families unchanged. Package suite
`go test ./internal/tui -count=1` green.

## Evidence that would refute or reshape

 Elapsed time already visible somewhere in the running state (would
  refute the gap — verified absent at case opening).
- Operators reporting the elapsed suffix as noise (would reshape
  formatting/placement, not the visibility principle).
- The spinner line proving unreachable in the live route's running state
  (would refute the chosen surface — it is the code path that renders
  during `spinnerActive`, ticked continuously).
- A cheaper existing surface (toolbar already re-renders per Update)
  serving better (reshape: move the suffix to the status line).

## Claim boundary

Mechanism claim gated locally through the production TUI event path
(`LoopEventMsg` → `Model.Update` → real `View()` with the spinner
active): a running tool call's elapsed time renders by default. Not
claimed: that elapsed time alone satisfies "whats really going on" (live
output and richer running state are separate future cases); any live-route
rendering beyond this session's observed window; changed loop or
executor behavior. Recorded adjacent watch item discovered while opening
this case: `ToolResultEvent` clears `spinnerActive` unconditionally, so
with parallel sibling calls the spinner disappears while a sibling still
runs — pre-existing, unfixed here, noted for its own case if it bites.

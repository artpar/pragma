# TUI-005 — The running-tool spinner dies on the first sibling result,
# hiding still-running parallel calls and their elapsed time

Opened 2026-09-12 from the TUI-003/TUI-004 watch item (adjacent
discovery recorded in the TUI-003 case). Methodology: `agent.md`.

## Source evidence (recorded)

1. **Deterministic code-level gap, reachable since PAR-001.** The
   provider-tools loop dispatches sibling tool calls in parallel
   (`internal/query/provider_tools_loop.go:145-150`, PAR-001, gated by
   `TestProviderToolsLoopDispatchesSiblingCallsConcurrently`), emitting
   all `ToolCallEvent`s up-front and each `ToolResultEvent` as it
   completes. The TUI's `ToolResultEvent` handler sets
   `m.spinnerActive = false` unconditionally
   (`internal/tui/handlers.go:388`) — the first sibling result kills the
   spinner line (and, since TUI-003, its elapsed time) while the other
   siblings are still executing.
2. **No operator-observed occurrence yet.** This session's own parallel
   five-`add_task` batch (18:55:47, log `2026-09-12T18-53-02.jsonl`,
   five `MCPToolCallCompleted` within one second) closed the window in
   milliseconds — not observable. Same honest labeling as TUI-002: a
   deterministic reachable-shape gap in the TUI-003/004 visibility
   family, reproduced through the production event path; no live
   occurrence claimed.

Source revision at case opening: `ceaeba3` (working tree, clean).

## Production path and the earliest wrong transition

`ToolCallEvent` (`internal/tui/handlers.go:357-380`) registers the call
in `m.activeToolCalls` and activates the spinner; `ToolResultEvent`
(`handlers.go:385+`) sets `m.spinnerActive = false` before consulting
`activeToolCalls` — even though the map still holds the running
siblings. The spinner line (`model.go`, `viewportContent`,
`if m.spinnerActive`) and its elapsed suffix vanish; the toolbar flips
to "streaming...". The earliest wrong transition is the spinner
deactivation decision: it keys on "a result arrived" instead of "no
active calls remain".

## Observed vs expected

Observed (deterministic, production event path): with two parallel Bash
calls, the first result removes the spinner line and its elapsed time
while the second call still runs; the toolbar reads "streaming...".

Expected: the running-tool indicator survives while any sibling call
is still active — the spinner keeps animating, shows the remaining
work (the shared tool name, or a count when names differ), and the
elapsed stays anchored to the oldest still-running call; only the last
result clears it. Source: the same operator-communication contract
behind TUI-003/004 (a running tool must not look finished). Not
claimed: per-call spinner multiplexing (the spinner names one surface);
any loop change; parallel dispatch behavior (PAR-001, unchanged).

## Proposed mechanism (one)

Key the spinner lifecycle to `activeToolCalls`: each `ToolCallEvent`
records the call's start time in the registry (value type gains
`StartedAt`); `ToolResultEvent` deletes the completed call and then
recomputes the spinner — still active while any call remains, naming
the surviving calls (one shared name shows the name; mixed names show
`N tools`), elapsed re-anchored to the oldest still-running call;
cleared when the registry empties. The toolbar keeps
"executing: …" while calls remain. Single-call behavior is unchanged.

## Executable assertion (gate)

`go test ./internal/tui -run TestSiblingSpinnerSurvivesPartialResults -count=1`
must fail on `ceaeba3` (baseline RED: after the first of two sibling
results the spinner line and its elapsed are gone while the second
call still runs) and pass on the candidate (spinner + elapsed persist
between the results, clear after the last).
`TestSiblingSpinnerShowsRemainingName` (distinct names: the survivor's
name replaces the completed one) and the existing TUI-003/004 gates
(single-call behavior unchanged) adjacent. Package suite green.

## Evidence that would refute or reshape

- The loop emitting results strictly after all calls complete (would
  make the gap unreachable — the parallel-dispatch gates prove
  otherwise).
- Operators reporting the surviving spinner as wrong ("both calls
  finished") — would reshape the naming, not the survival principle.
- Elapsed re-anchoring showing a misleading (decreasing or reset) clock
  — anchored to the oldest remaining call, it is monotonic within a
  batch.

## Claim boundary

Mechanism claim gated locally through the production TUI event path:
the running-tool indicator and its elapsed time persist across partial
sibling results and clear after the last. Not claimed: any live-route
rendering; per-call timing precision for same-name siblings (the
elapsed anchors the batch's oldest call); any change to dispatch,
execution, or result pairing.

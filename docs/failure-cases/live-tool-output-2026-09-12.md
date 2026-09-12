# TUI-004 — A running foreground tool call emits no output until it
# completes, so the operator cannot see what the command is doing

Opened 2026-09-12 from the operator's live report and the TUI-003
residue. Methodology: `agent.md`.

## Source events (authentic, recorded)

1. **The 361-second silent tool gap (this session, measured).** Log
   `~/.pragma/logs/2026-09-12T18-53-02.jsonl`: the only inter-request gap
   over 60s across 42 requests is **361.2s, 18:57:27 → 19:03:28** — the
   window in which `go test ./internal/tui` + `go build ./...` ran
   (aborted by the outer harness at 19:02:27, ~4m56s in, just short of
   the 300s `pragmaLoopForegroundWait`). The command's stdout — test
   results, build progress — was invisible the entire window; only the
   final result arrived at completion.
2. **Operator live report, 19:02:27 (verbatim):** *"the ui for human is
   quite bad and doesnt tell the human at all whats *really* going on."*
   TUI-003 (elapsed time) answered *how long*; this case answers *what
   is coming out*.

Source revision at case opening: `7f4a558` (working tree, clean).

## Production path and the earliest wrong transition

Bash execution: `internal/query/provider_tools_loop.go:599`
(`executeProviderBashTool`) → `internal/shellrun/run.go:46`
(`Execute`) — the command runs with stdout/stderr redirected to
`Files.Console`, and the foreground wait is a `select` over
{process done, `ForegroundWait` expiry, context deadline} with **no
output read until completion** (the `ForegroundWait` branch's mid-run
`TailLines(ReadOutput(files), …)` proves mid-run reads are safe and
already established). The executor returns one complete
`ToolResultPart`; the parallel dispatch goroutine
(`provider_tools_loop.go:145-150`, `ch` in closure scope) emits exactly
two events per call: `ToolCallEvent`, then `ToolResultEvent`. The TUI
renders the result only on `ToolResultEvent`
(`internal/tui/handlers.go:382+`).

The earliest wrong transition is the executor's wait: the output file
grows continuously while the process runs, and the executor knows it,
but nothing reads or emits it during the wait — the event vocabulary
(`internal/query/event.go`) has no intermediate tool-output event at
all.

## Observed vs expected

Observed (this session, real processes): a 6-minute foreground command
produced zero operator-visible output until it completed/aborted — the
operator watched a spinner (now with elapsed, post-TUI-003) and an empty
result area.

Expected: while a foreground tool call runs, its output-so-far renders
inline in the default view (bounded tail), updating as it grows; the
final result replaces it in place. Source: the operator-communication
contract in `agent.md` and the operator's verbatim requirement (*"whats
really going on"*). Not claimed: live output beyond the foreground-wait
window (commands that exceed `pragmaLoopForegroundWait` follow the
existing still-running/background flow unchanged); streaming for
non-Bash tools (MCP, websearch, sub-agents — separate cases on
evidence); any change to what the model receives (the live events are
operator-visibility only; the conversation records the final result as
today).

## Proposed mechanism (one)

Incremental live output for foreground Bash: `shellrun.Options` gains
`OnLiveOutput func(soFar string)` (+ interval, default 250ms); the
foreground wait `select` becomes a loop with a ticker case that reads
the console file and invokes the callback only when the tail changed
(tail bounded by the existing `RunningOutputLines`, 100). The loop's
dispatch goroutine passes a closure emitting a new sealed
`ToolOutputEvent{ToolCallID, Output, Running}` to the existing event
channel (the same channel `ToolResultEvent` already uses from the same
goroutine). The TUI handles it: a `segLive` segment renders the tail
inline (dim bracket box); on `ToolResultEvent` the live segment is
filled with the final content in place (kind flips to `segTool`), so
the completed rendering is byte-identical to today's and nothing
duplicates. No provider, storage, permission, or conversation changes.

## Executable assertion (gate)

**Real local integration (RED on `7f4a558`, compiles on baseline):**
`go test ./internal/query -run TestBashToolEmitsLiveOutputWhileRunning -count=1`
— a real command (`echo TUI004_STEP_ONE; sleep 1.5; echo TUI004_STEP_TWO`)
through the real loop dispatch and real executor; the gate asserts an
event strictly between the `ToolCallEvent` and its `ToolResultEvent`
carries the first step's output. Baseline: zero between-events → fails
for exactly the stated reason (no live output exists).

**TUI guards (candidate):**
`go test ./internal/tui -run TestLiveToolOutputRendersInline -count=1` —
the live tail renders inline while running, and the final result
replaces it exactly once (the step text appears exactly once in the
completed view). Adjacent: the full `internal/query` suite (parallel
dispatch, retry, turn-budget, apply-patch pairing),
`internal/tui` suite, `go build ./...`.

## Evidence that would refute or reshape

- Mid-run console reads proving unsafe (would refute the mechanism —
  the `ForegroundWait` branch already does this in production).
- Event-channel backpressure freezing tool completion (would reshape:
  drop-on-full or larger buffer; the send has the same blocking
  semantics as the existing parallel `ToolResultEvent` sends).
- Operators reporting the live tail as noise for short commands (would
  reshape: first-emit delay or a minimum-duration threshold).
- Live tails corrupting the final result rendering (would refute the
  in-place fill; the count-once guard exists for that).

## Claim boundary

Real local integration claim: a foreground Bash call's output-so-far is
emitted as bounded live events through the production dispatch channel
and rendered inline in the production TUI path, replaced in place by
the final result. Not claimed: live output past the foreground-wait
window; live output for non-Bash tools; any effect on model behavior or
conversation contents; any live-route rendering beyond this session's
observed window (the next real long call after a rebuild is the
live-effect check).

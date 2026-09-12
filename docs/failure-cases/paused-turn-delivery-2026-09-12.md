# TUI-002 — A paused stop reason (pause_turn) has no operator-facing TUI
# notice, and a thinking-only paused response renders no readable output

Opened 2026-09-12 from ORCH-002 finding 2 (adversarial dogfood review,
designated the first TUI-001-family follow-up candidate). Methodology:
`agent.md`.

## Source evidence (recorded)

1. **ORCH-002 live-route adversarial review (2026-09-12)** — run label
   `e2e/orchestration/results/orch-dogfood-2026-09-12/` (implementer report
   check 4, residual edge; prosecutor verdict concurs), session log
   `~/.pragma/logs/2026-09-12T17-59-46.jsonl`. The implementer verified by
   write/read-site analysis: `finalResponseHadText` and the promotion gate
   are read only at `internal/tui/handlers.go:441` and gated on
   `StopEndTurn`, the stop-notice family at `handlers.go:431-437` covers
   only max_tokens, content-filter, and error, and
   **pause_turn is in the loop's reachable terminal-stop set**
   (`internal/query/provider_tools_loop.go:116-123`: end_turn,
   content_filtered, pause_turn, error). Quote: *"StopPauseTurn has no
   notice branch in the TUI; a thinking-only pause_turn response would
   render only the collapsed hint. Anthropic maps pause_turn
   (translate_in.go:119), so the shape is representable even if rare."*
2. **No production occurrence yet.** Zero `stop_reason:"pause_turn"`
   events across all `~/.pragma/logs/*.jsonl` (checked 2026-09-12, every
   session log on disk). The defect is therefore **not an observed
   production failure** — it is a deterministic harness gap in the
   operator-delivery contract, reachable through the production event
   path and reproduced by the gate below. The claim boundary reflects this.

Source revision at case opening: `afac61d` (working tree, clean).

## Production path and the earliest wrong transition

`internal/provider/anthropic/translate_in.go:118-120` maps the SDK's
`StopReasonPauseTurn` to `model.StopPauseTurn` (defined
`internal/model/stop.go:11`); the morph route passes the response's stop
reason through (`internal/provider/morphllm/complete.go:137`).
`internal/query/provider_tools_loop.go:121-123` emits
`TurnCompleteEvent{StopReason: StopPauseTurn}` for any non-tool stop with
no tool calls — pause_turn is in the loop's reachable set.
`internal/tui/handlers.go` (`case query.TurnCompleteEvent`, ~:429-446)
has notice branches for `StopMaxTokens` (:431), `StopContentFiltered`
 (:434), `StopError` (:437) and the TUI-001 textless promotion for
`StopEndTurn` (:441); `StopPauseTurn` matches nothing. `finishTurn`
(:799-809) then sets the toolbar to "ready", indistinguishable from a
normally completed turn.

The earliest wrong transition is the TUI's stop-shape handling: a stop
reason meaning "the provider paused the turn mid-flight" renders no notice,
and for a textless paused response the TUI-001 delivery guarantee (the
final response's only content is visible by default) does not apply.

## Observed vs expected

Observed (deterministic, production event path, reproduced by the gate
below): `ThinkingEvent → TurnCompleteEvent{StopPauseTurn}` (textless) →
default view shows only `∴ Thinking (ctrl+o to expand)` and nothing else;
`TextEvent → TurnCompleteEvent{StopPauseTurn}` (text-bearing) → text
renders but nothing indicates the turn was paused rather than completed;
in both cases the toolbar reads "ready".

Expected: (a) every reachable terminal stop shape that carries
operator-actionable meaning gets a notice in the existing family —
max_tokens, content-filter, and error already do; pause_turn must,
stating the turn is paused and how to continue (sending a message is how
the harness resumes a paused turn); (b) the TUI-001 delivery class extends
to the pause stop: a textless paused response's thinking is its only
content and renders in the default view. Source: the operator-communication
contract in `agent.md` (end-of-turn output addressed to the operator must
be visible by default) and the existing stop-notice family at
`handlers.go:431-437`. Not claimed: that any provider route has emitted
pause_turn (none observed); special resume semantics for paused turns
(a new user message starts a new request; the notice says so).

## Proposed mechanism (one)

Handle the pause_turn stop shape at `TurnCompleteEvent` in the TUI: add a
notice branch in the existing family — `[turn paused by the provider —
send a message to continue]` — and extend the textless-promotion gate from
`StopEndTurn` to `StopEndTurn || StopPauseTurn`. The promotion must run
**before** the notice is appended: the notice is a non-whitespace text
segment, and `promoteTrailingThinking`'s backward scan halts at any
non-whitespace segText (the ORCH-002 implementer report documented this
scan property at check 3) — an appended-notice-first ordering silently
blocks the promotion it accompanies; the first candidate attempt failed
exactly there and was caught by the gate. The notice renders for every
pause_turn regardless of text; the promotion applies only when the final
response produced no visible text (`finalResponseHadText` logic unchanged:
reset at turn start and on every `ToolResultEvent`, set only by
`TextEvent`). end_turn, max_tokens, content-filter, and error behavior is
unchanged, as is collapsed thinking for text-bearing responses.

## Executable assertion (gate)

`go test ./internal/tui -run TestThinkingOnlyPauseTurnShowsInstruction -count=1`
must fail on `afac61d` (baseline RED: default `View()` shows only the
collapsed hint; the paused response's thinking content and any pause
notice are absent) and pass on the candidate (content visible plus the
pause notice). Adjacent assertions:
`TestPausedTextBearingTurnShowsNotice` (text-bearing pause shows the
notice, thinking stays collapsed), and the TUI-001 family —
`TestThinkingOnlyEndTurnShowsInstruction`,
`TestThinkingThenTextStaysCollapsed`,
`TestTextlessFinalResponseAfterToolWorkIsPromoted`,
`TestReloadShowsTrailingThinkingOnlyMessage` — must stay green (end_turn
behavior unchanged). Package suite `go test ./internal/tui -count=1` green.

## Evidence that would refute or reshape

- A provider contract or loop change that makes pause_turn unreachable at
  `TurnCompleteEvent` (would remove the reachable shape; today the loop
  explicitly passes it through).
- The anthropic SDK dropping `StopReasonPauseTurn` (pause would then map
  to the default `StopError`, which already has a notice).
- Operators reporting the notice or promoted thinking as noise (would
  reshape wording or the promotion, not the delivery-class principle).
- The promotion firing on paused responses that carried text (would
  refute the flag mechanism; the adjacent gate exists for that).
- A first live pause_turn occurrence that renders differently than the
  gate asserts (would reopen with production evidence).

## Claim boundary

Mechanism claim gated locally through the production TUI event path
(streaming loop events through `Model.Update`, asserting the real
`View()`). No production pause_turn occurrence is claimed or required for
the fix's validity; whether any active route ever pauses a turn, and any
live behavioral effect, is unclaimed. The fix does not change loop
semantics: a paused turn still ends as a completed turn at the harness
level (`finishTurn`); the notice states the operator action, it does not
reopen the turn.

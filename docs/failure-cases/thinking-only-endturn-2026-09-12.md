# TUI-001 — A final response that is thinking-only renders no operator-readable
# output in the default view

Opened 2026-09-12 after a live operator report at session start. Methodology:
`agent.md`.

## Source events (authentic, recorded)

1. **2026-09-12, midday dogfood session** —
   `~/.pragma/logs/2026-09-12T12-27-56.jsonl`, final API request
   (2026-09-12T15:37:39.392+05:30, the session's 50th completed request):
   `stop_reason: "end_turn"`, content = a single thinking block
   (~809 output tokens), **empty text body**. The thinking block carried the
   session's entire closing instruction to the operator — including
   *"If you want a status check, open a FRESH session and say 'continue'"* —
   and the message appended to the conversation had
   `content_types: ["thinking"]` only.
2. **Operator report** (the failing observation, 2026-09-12 16:29, fresh
   session `~/.pragma/logs/2026-09-12T16-23-21.jsonl`): *"you told me to
   restart the session, but didnt give me the follow up prompt."* The operator
   acted on the restart instruction but never received the follow-up prompt,
   because it never rendered in their default view.

Source revision at observation: `eb0e264` (working tree, clean).

## Production path and the earliest wrong transition

`internal/query/provider_tools_loop.go:442` emits `query.ThinkingEvent` for
the response's thinking part; `:123` then emits
`query.TurnCompleteEvent{StopReason: end_turn}`.
`internal/tui/handlers.go:263` appends the thinking as a raw `segThinking`
segment; `:417` (TurnComplete, end_turn) appends only `"\n"` — end_turn has
no notice path (the notices cover max_tokens, content-filter, and error
stops only). `internal/tui/model.go:423` renders `segThinking` through
`render.RenderThinking(part, m.verbose)`, and
`internal/tui/render/content.go:184` renders the non-verbose case as
`∴ Thinking (ctrl+o to expand)` — the content itself is hidden.

The earliest wrong transition is the render decision: a final response that
produced no text renders as a single collapsed hint, so the turn's entire
operator-facing output sits behind a mode toggle the operator was never
prompted to use. The model emitting the instruction as thinking-only is the
triggering condition; the harness defect is that a textless end_turn
guarantees invisible output.

## Observed vs expected

Observed: thinking-only end_turn → default view shows
`∴ Thinking (ctrl+o to expand)` and nothing else; the operator misses the
turn's instruction (reported live).

Expected: when a turn's final response emitted no text, the remaining
content (the thinking block) renders in the default view — the operator
receives the turn's only output without pressing Ctrl+O. Source:
the operator-communication contract in `agent.md` ("request it explicitly
with exact instructions instead of waiting for spontaneous action") —
end-of-turn output addressed to the operator must be visible by default.
Not claimed: any contract that models must emit text; thinking stays
collapsed for responses that do carry text.

## Proposed mechanism (one)

Promote the trailing thinking run when the final response produced no
visible text. `segment.forceShow` makes a `segThinking` render expanded even
non-verbose; `promoteTrailingThinking` marks the trailing consecutive
thinking segments (skipping whitespace-only text). It is applied at
`TurnCompleteEvent` with `StopEndTurn` when a `finalResponseHadText` flag is
false — the flag resets at turn start and on every `ToolResultEvent` (each
new request within the turn), and is set only by `TextEvent`, so the
check tracks the final response, not the whole turn. The same promotion runs
in `reloadConversationFromStore` when the conversation's last message is an
assistant message with thinking content and no text parts, so a resumed
session shows the missed instruction too. Collapsed-thinking behavior for
responses that contain text is unchanged, as are all stop-reason notices.

## Executable assertion (gate)

`go test ./internal/tui -run TestThinkingOnlyEndTurnShowsInstruction -count=1`
must fail on `eb0e264` (baseline RED: default `View()` shows only the
collapsed hint, the instruction text is absent) and pass on the candidate
(instruction visible in default mode). Adjacent assertions:
`TestThinkingThenTextStaysCollapsed` (text-bearing turns keep thinking
collapsed by default), `TestTextlessFinalResponseAfterToolWorkIsPromoted`
(multi-request turn: earlier text + tool work does not block promotion of a
thinking-only final response), and
`TestReloadShowsTrailingThinkingOnlyMessage` (resume path).

## Evidence that would refute or reshape

- The route never emitting thinking-only end_turn responses again (the
  promotion becomes dead code; the collapsed default still holds for
  text-bearing responses).
- A cheaper existing mechanism covering textless end_turn (e.g., an existing
  notice path) — none: `TurnCompleteEvent` notices cover only max_tokens,
  content filter, and error.
- Operators reporting the auto-expanded thinking as noise (would reshape:
  promote to a visible bracketed pointer instead of full expansion).
- The promotion firing on turns that did render text (would refute the
  flag/scan mechanism; the adjacent gates exist to catch exactly that).

## Claim boundary

Default-mode visibility of a textless final response's thinking content is a
mechanism claim gated locally through the production TUI event path
(streaming loop events through `Model.Update`, asserting the real `View()`).
Whether an operator reads or acts on the promoted content is live effect,
not claimed. The model's choice to place instructions in thinking is route
behavior; this case guarantees the delivery path, not the emission.

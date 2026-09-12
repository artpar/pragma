# INT-001 — Mid-turn operator input is rejected-and-dropped; the only path to
# deliver it is an interrupt that forfeits the in-flight request's input spend

Opened 2026-09-12 by the session that continued the self-evolution roadmap on
the operator's explicit optional call ("Do something about interrupt-forfeit
spend (~350K tokens lost tonight from messages submitted mid-request) — or
wait"). Methodology: `agent.md`. Roadmap observation being upgraded:
`docs/self-evolution-roadmap.md` (2026-09-11/12 night session notes).

## Source events (authentic, recorded)

Session log `~/.pragma/logs/2026-09-11T22-55-12.jsonl` (the night session
that committed TURN-002, CLK-001, PAR-001), four non-retryable request
failures, each `error_type=request_failed`,
`error_message="[openai-compatible] provider_error: context canceled"`,
`retryable=false`, `attempt=1`:

| # | APIRequestFailed time | trace_id | next UserTurnAccepted | input spend forfeited (next completed request's input_tokens as the conversation-size proxy) |
|---|---|---|---|---|
| 1 | 23:05:33.999+05:30 | 412ec5130de3cb5bb3f00b0b5c0835be | 23:05:45.097 | ≈47,011 (23:09:02.223 completion) |
| 2 | 23:20:46.393+05:30 | e3d987744991d1aafaf40074de018ca4 | 23:20:47.738 | ≈68,233 (23:24:21.648) |
| 3 | 23:22:10.339+05:30 | d03de7aa4d4ddf80a8ea5bdeb407b7b0 | 23:22:21.009 | same request as #2's successor |
| 4 | 23:29:36.473+05:30 | 691b24e3d439731d88da693d8bc76246 | 23:30:13.488 | ≈79,921 (23:34:27.869) |

#4 is the PAR-001 evidence batch (operator interrupt during tool execution,
deliberate). #1–#3 are the message-motivated forfeits: the operator wanted to
inject input while a request was in flight. Total forfeited input ≈ 3
requests × 40–120K input tokens ≈ 350K (operator tally; consistent with the
conversation sizes above). All four continue cleanly after (requests resume
from the preserved conversation), so the spend — not the conversation — is
the loss.

## Corrected mechanism (this case amends the roadmap note)

The roadmap recorded "submitting a message while a model request is in flight
cancels it." Code trace at source revision `36ac918` shows submission itself
does not cancel anything — the actual production path is two steps:

1. `internal/tui/input.go` Enter → `InputSubmittedMsg`; while streaming,
   `internal/tui/handlers.go handleInputSubmitted` calls
   `m.runInput(parentCtx, text)` → `internal/cli/run.go RunInput` →
   `beginInputTurn()` sees `turnActive` → emits
   `RejectedPromptEvent{Reason:"busy"}`, which the TUI renders as
   "Prompt rejected: another turn is still running". **The submitted text is
   dropped**: the input component cleared on Enter
   (`c.textarea.Reset()` before submission), the rejected prompt is not
   added to input history (history remembers only on
   `AcceptedPromptEvent`), and nothing stores the text.
2. The only way to deliver input mid-turn is `Esc`/`Ctrl+C` →
   `interruptTurn()` → `m.cancel()` → the in-flight request dies with
   `context canceled` (**full input spend forfeited**), then a fresh prompt
   is accepted as a new turn.

So the night-session events are: reject-and-drop (step 1) followed by the
operator's interrupt (step 2) to get their message through. The forfeit is
the cost of the only delivery path.

## Expected behavior and its source

Source: explicit operator request in the 2026-09-12T00:46:37+05:30 prompt
(quantified ~350K loss), consistent with the harness's own message-fold
mechanism: the provider-tools loop re-snapshots the store before every
request (`snap := engine.store.Snapshot()` →
`messagesForRequestChecked(snap.Conversation)`,
`internal/query/provider_tools_loop.go`), so any user-role message appended
between requests is delivered to the next request — proven live by the
CLK-001 companion messages (two consecutive user messages on the wire, 13+
requests, zero failures, `~/.pragma/logs/2026-09-12T00-46-33.jsonl`).

Expected: submitting plain-text input while a turn is running must deliver
the message into the running conversation at the next request boundary —
without dropping the text and without canceling the in-flight request. Slash
commands (runtime side effects) stay rejected while a turn runs. Explicit
interrupt (`Esc`/`Ctrl+C`) keeps today's semantics: cancel in-flight, forfeit
that request's spend — the operator's deliberate choice.

## Observed (defect), precisely

Through the production path, with a turn active: `RunInput` returns
`RejectedPromptEvent` and the submitted text is absent from the conversation
store; the operator must retype it after an interrupt. The in-flight request
is not canceled by the submission itself.

## Proposed mechanism (one)

Queue on the busy path: when `beginInputTurn()` fails and the input is not a
slash command, append it to the conversation as a regular stamped user
message (the CLK-001 append shape) and emit a new `QueuedPromptEvent` for
rendering. The loop's existing snapshot-per-request fold delivers it. One
hazard must be handled inside the same mechanism: appending between an
assistant `tool_use` message and its tool-results message would interleave a
user message inside the tool-result pair (unproven wire order;
`validateToolResultPairing` guards it, and most openai-compatible routes
require results directly after the call). Therefore the append is atomic with
a dangling-tail check: if the conversation tail is an assistant message with
unanswered tool calls, the message is parked and drained at the loop's next
safe point (after the companion append); otherwise it appends immediately.

Unchanged: slash-command rejection while busy, explicit interrupt semantics,
turn admission (one accepted turn at a time — a queued message is not a turn:
no `UserTurnAccepted`, no new engine run), turn cap, error paths, pragma loop
mode, assistant message handling, hook execution (queued prompts do not run
`UserPromptSubmit` hooks; they are conversation content, not turn starts).

## Executable assertion (gate)

`go test ./internal/cli -run TestRunInputQueuesPlainTextWhileTurnActive -count=1`
must fail on `36ac918` (baseline RED: `RejectedPromptEvent` received, text
absent from the conversation) and pass on the candidate (queued event, text
appended with the wall-clock stamp). Engine-side hazard gates:
`go test ./internal/query -run TestAppendUserInputParksBehindDanglingToolUse -count=1`
and `... -run TestProviderToolsLoopDeliversQueuedInputAtNextRequest -count=1`
(real loop, real Bash tool, queued message mid-tool-execution, request wire
asserted, `-race`). TUI render gate:
`go test ./internal/tui -run TestQueuedPromptEventRenders -count=1`.

## Evidence that would refute or reshape

- The route rejecting consecutive user messages or a third consecutive user
  message (would reshape: queue would need merging or a different delivery
  point — the live wire gate must watch this).
- The fold breaking request pairing or loop termination invariants (park/drain
  gates exist for exactly this; a failure there refutes the mechanism).
- The operator finding queued delivery worse than reject-drop (e.g. messages
  silently sitting past a turn's death — mitigated by persistence + rendering,
  but a live complaint would reopen).
- A cheaper existing mechanism discovered (e.g. an admission path that
  already queues) — none exists at `36ac918`: the busy path is a hard reject.

## Claim boundary

Deliverability and no-forfeit are mechanism claims gated locally. Whether
the model *acts* on a mid-turn queued message is a live-effect observation,
not claimed. Live TUI confirmation (actual Enter-during-streaming on a real
route) is the operator dogfood step, exactly like TURN-001's live-effect
boundary; the headless gates drive the same production functions.

Live confirmation 2026-09-12 13:35 (midday dogfood session, log
`~/.pragma/logs/2026-09-12T12-27-56.jsonl`): with a turn active and a
`sleep 45` tool call executing, the operator submitted plain text
mid-execution — no rejection, no interrupt, no canceled request; the
message parked behind the dangling tool_use, drained after the companion
append (results, companion, queued message appended as three
microsecond-aligned `MessageAppended` events at 13:35:49.256), and was
delivered on the next request wire with its 13:35:05 submission-time
stamp, received by the model mid-turn. The TUI `QueuedPromptEvent` is
not persisted to the session log (render-only); the log-side proof is
the append trio plus delivery on the wire. Whether the model acts on
queued input remains the open live-effect boundary.

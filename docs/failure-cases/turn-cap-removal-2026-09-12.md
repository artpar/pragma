# TURN-003 — Default 100-turn cap removed from the provider-tools loop

Opened 2026-09-12 on the operator's explicit directive ("can you remove
your limit of 100 turns as well please"), the standing authority this
change rests on. Methodology: `agent.md`. Lineage: TURN-001 (warn-at-N),
TURN-002 (window widened to maxTurns/5) — both accepted the cap itself;
the operator now removes it by default.

## Evidence trail (all recorded)

- `DefaultMaxTurns = 100` killed the 2026-09-11 18:50 operator session
  twice mid-task with no advance signal (TURN-001's opening evidence).
- The same cap killed three Terminal-Bench 2.1 tasks the same way (TB-2.1
  audit, TURN-001 record).
- TURN-001's first live run: the notice worked, but 10 remaining turns
  were not enough for a realistic wrap-up; the loop died at the cap with
  the commit undone (TURN-002's opening evidence).
- TURN-002's widened 20-turn window fired live and the wrap-up completed
  (2026-09-12T00-46-33 session, turn 80 of 100) — but the cap's death
  behavior remains a proven task-killer, and the 2026-09-12 night session
  itself was cut by it mid-work (the continuation resumed the next
  morning). The operator's directive closes the sequence: no default cap.

## Expected behavior and its source

Source: explicit operator directive 2026-09-12T11:44:53+05:30, consistent
with the accumulated evidence above. Expected: an interactive
provider-tools run with no explicit `--max-turns` does not terminate by
turn count; it ends on the model's end-turn, an error, or the operator's
interrupt. `--max-turns N` (and a config-file value) still bounds the loop
exactly as before, including the TURN-001/002 warning window and the
honest cap-termination error.

## Mechanism (one)

`MaxTurns <= 0` no longer substitutes `DefaultMaxTurns = 100`; it means no
turn cap (negative values clamp to none). The loop condition admits any
iteration count; the cap error stays reachable only for bounded runs.
`turnBudgetWarnTurn(0)` already returns -1 (no warning without a cap —
the notice is defined by distance to a cap). Sub-agents keep a runaway
guard: `ForkFreshConversation` pins the sub's `MaxTurns` to the former
default (100, now `DefaultSubAgentMaxTurns`) when the parent runs
uncapped — a drifting sub otherwise blocks the parent's synchronous fork
indefinitely with no one watching, and the parent's only recourse is the
forfeit-interrupt INT-001 just removed a cause of. Explicit parent bounds
still inherit to subs unchanged. Pragma loop mode (separate budget
mechanism) and the miniswe loop are untouched.

## Gates

Baseline RED → candidate GREEN:
`go test ./internal/query -run TestProviderToolsLoopNoDefaultTurnCap -count=1`
— a real loop with `MaxTurns` unset and 105 scripted tool-call turns plus
a final end-turn response: baseline dies at turn 100 with "exceeded
maximum of 100 turns"; candidate completes all 106 requests and emits
TurnCompleteEvent. Candidate invariants:
`... -run TestForkPinsSubAgentTurnCapWhenParentUncapped` (uncapped parent
→ sub capped at `DefaultSubAgentMaxTurns`; bounded parent → sub inherits)
and the existing suite — every cap/window test uses explicit MaxTurns
values, so bounded behavior (warn window at maxTurns/5, cap error text,
`--max-turns` flag path) is unchanged and re-verified by the full suite.

## Hazards, recorded

- Unbounded spend on unattended runs (background, cron, benchmark runs
  that don't pass an explicit cap) — accepted by the directive for the
  interactive case; unattended paths keep no pragma-side turn guard now.
  Reopen condition: an observed unattended runaway burn.
- A model that never ends its turn loops indefinitely (spend guard is the
  operator's interrupt; no cost ceiling exists in the loop today).
- Sub-agent cap (100) firing on a legitimate long bounded task — reopen
  with a real sub-task that needed more; until then it is a dormant guard.

## Claim boundary

Termination-semantics change, gated by recorded regression and the full
bounded-path suite. No task-success claim (a session that would have died
at 100 now doing better is a live-effect observation for later, not
claimed here). No wire change: request shape is untouched; the existing
wire gates (e2e/mcp-injection profiles use explicit budgets) are
unaffected by construction.

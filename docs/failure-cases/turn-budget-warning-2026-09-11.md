# TURN-001 — Turn-budget exhaustion kills the provider-tools loop mid-task with no advance signal

- Case ID: `TURN-001`
- Source revision: `e915c39` (2026-09-11, post-SUB-001)
- Source observation (authentic, recorded): the operator session running
  M4 (`morph-glm53-744b`, provider-tools) was terminated twice by
  `DefaultMaxTurns` mid-task with no warning. Log
  `~/.pragma/logs/2026-09-11T18-50-33.jsonl`:
  - User turn accepted `18:50:37` produced **exactly 100**
    `APIRequestCompleted` events; the 100th (`19:25:30`, input 102,763 tok)
    ended `stop_reason=tool_use` — the model was mid-work, still issuing
    tool calls, when the loop terminated. Content kinds of the last five
    completions are all `tool_call`/`thinking+tool_call`; the model never
    began wrapping up. The operator re-prompted at `19:27:05` (164 chars)
    to continue.
  - The continuation again consumed **exactly 100** completions; the 100th
    (`19:55:52`, input 164,377 tok, `stop_reason=tool_use`) killed it a
    second time; the operator continued again at `19:58:16`.
  - The model's own recorded thinking at `19:27:59` (trace
    `604f1cea2582f18c6518d13be4642255`) quotes the termination error
    ("provider tools loop exceeded maximum of 100 turns") as news relayed
    by the operator after the fact — direct evidence the model had **no
    prior awareness of the budget**.
  - Independent corroboration: Terminal-Bench 2.1 job
    `pragma-lilac-glm52-final-89` (2026-08-30, `--loop provider-tools
    --max-turns 100`) lost three clean-failure tasks to the same cap
    mid-task — `break-filter-js-from-html`, `fix-code-vulnerability`,
    `largest-eigenval` (100 model responses each, required artifacts absent
    or incomplete; see `docs/terminal-bench-2.1-final-run-audit-2026-09-11.md`).
- Responsible path: `internal/query/provider_tools_loop.go` — the loop
  spends the `MaxTurns` budget silently. No turn-budget information exists
  anywhere the model can see: `systemWithMCPStatus` adds MCP status,
  `systemWithPatchGuidance` adds patch guidance, and no conversation message
  carries budget state. The budget's only appearance is the terminal
  `ErrorEvent` `"provider tools loop exceeded maximum of %d turns"` after
  turn 100 — after which no wrap-up, handoff, or prioritization is possible.
- Expected behavior (warn-at-N, chosen of the three options
  warn-at-N / continuation / override): when the budget approaches
  exhaustion, the model is told in-conversation, once, with a user-role
  turn-budget notice naming used/remaining turns and instructing it to
  complete the step and either finish or write a handoff. The cap itself is
  unchanged (cost control); continuation already exists — the conversation
  persists across the loop's death (observed input tokens 164,377 →
  203,896 across kills, operator re-prompt or `--resume` resumes it) — and
  an explicit override already exists (`--max-turns`). Sources: operator
  directive 2026-09-11; the harness's own orchestration runner already
  declares runtime budgets to the model ("This state has a runtime budget
  of %d shell actions. Plan command batches so you either complete the
  state or write the required route artifacts before the budget is
  exhausted." — `internal/orchestration/runner.go`); Claude Code's agent
  loop surfaces `error_max_turns` as a result subtype and its fleet budget
  caps warn toward the cap before rejecting.
- Warning window: `maxTurns/10` turns remaining, floored at 5, and at least
  half the budget for budgets below 10 (so a warning always precedes the
  cap with enough room to act). For `DefaultMaxTurns=100`: notice at the
  91st request, 10 turns remaining.
- Reproduction (deterministic, hermetic, no spend): in-package scripted
  provider returning `stop_reason=tool_use` every turn with a small
  `MaxTurns` (20), run through `runProviderToolsLoop` — the same mechanism
  that killed the recorded sessions, compressed.
- Executable assertion (baseline RED): across all requests the scripted
  provider sees, **no** message contains turn-budget information, and the
  loop terminates with `provider tools loop exceeded maximum of 20 turns`
  while the model's last response was `tool_use`.
- Executable assertion (candidate GREEN): the request for the 11th turn
  (index 10; 20−10) carries a single user-role notice with "10 of 20 ...
  10 remain"; the notice appears exactly once; later requests retain it in
  history; the loop **still** terminates with `exceeded maximum of 20
  turns` (cap enforcement unchanged) if the model keeps tool-calling.
- Adjacent checks: pragma loop mode (`runPragmaLoop`) emits no such notice
  and is untouched (the recorded kills are all provider-tools; pragma mode
  has its own completion-check budget and its own follow-up case if ever
  evidenced); sub-agent forks share the loop and hence the mechanism
  consistently (their own budget, their own notice); `DefaultMaxTurns`,
  `--max-turns`, `--resume`, and the terminal error text are unchanged;
  full suite clean.
- Proposed mechanism (one): `turnBudgetWarnTurn(maxTurns)` helper plus a
  single `appendConversationMessage` user-role notice injected at the top
  of the warn iteration in `runProviderToolsLoop`. No new config, no CLI
  changes, no new event kinds (the appended message already emits
  `MessageAppended` for operator/log visibility).
- Evidence that would refute the case: turn-budget information already
  reaching the model through some other path (none found — system prompt
  blocks carry none, no message contains it); or the recorded kills being
  misread (they are not: both segments are exactly 100 completions ending
  `stop_reason=tool_use`, followed by operator re-prompts, and the model's
  recorded thinking shows surprise at the termination).

## Verification record

- Baseline (revision `e915c39`, unchanged harness, 2026-09-11):
  deterministic reproduction through the production loop
  (`runProviderToolsLoop` driven by the in-package scripted provider, 20
  tool-use responses, `MaxTurns: 20`):
  - `TestProviderToolsLoopTurnBudgetNoticeBeforeCap`: FAIL at the notice
    assertion — `warn-iteration request carries 0 notices, want 1` — while
    every assertion before it passed (20 provider requests, termination
    error `provider tools loop exceeded maximum of 20 turns`). The failure
    is exactly the missing mechanism, not setup.
  - `TestPragmaLoopNoTurnBudgetNotice` (adjacent): PASS on baseline — pragma
    mode carried no notice before the change either.
- Candidate (`turnBudgetWarnTurn` + single user-role
  `turnBudgetNotice` appended at the warn iteration; cap, error text,
  `--max-turns`, `--resume` untouched):
  - `TestProviderToolsLoopTurnBudgetNoticeBeforeCap`: PASS — no notice on
    requests 0–9, exactly one on request 10 (naming `10 of 20`, `10
    remain`), retained exactly once through the final request, cap error
    unchanged, `messagesForRequestChecked` pairing validation intact on
    every request (a pairing break would have surfaced as an ErrorEvent
    instead of the cap error).
  - `TestTurnBudgetWarnTurnWindow`: PASS — windows at 100→90, 50→45,
    20→10, 12→6, 8→4, 6→3, 2→1, 1/0→none.
  - `TestPragmaLoopNoTurnBudgetNotice`: PASS — pragma mode gains no notice.
  - `go test ./internal/query/ -count=1` ok; `go test ./... -count=1`
    exit 0 (29 packages ok); `make instrument` idempotent (6 branch points
    added on first run, 0 on rerun); `make smoke` OK.
- Wire gate (real local integration, added same day after the engine gates):
  a real session boot against the scripted provider with `--max-turns 8`
  (`e2e/mcp-injection`: new `turnbudget` profile + `tbgreen` assertion,
  rerunnable via `run_probe.sh <label> turnbudget && assert_boot.py
  results/<label> tbgreen`). Preserved evidence:
  `e2e/mcp-injection/results/candidate-tbgreen/` — the notice appears on
  the captured wire at request 5 ("4 of 8 ... 4 remain") as a proper user
  message through the real OpenAI-compatible serialization, is absent from
  requests 1–4 and retained exactly once through request 8, and the loop
  still terminates with `provider tools loop exceeded maximum of 8 turns`
  — the same error text and boot-level behavior as the recorded kills,
  now preceded by a warning.
- Live-effect boundary: this gate proves the notice is constructed and
  injected in-conversation through the production path. Whether a specific
  model *acts* on the notice (wraps up, writes a handoff) is a
  model-dependent behavioral claim that would need a bounded live
  continuation; none is claimed here. The next long operator session on the
  live route is the natural observation point.

# TURN-002 — The turn-budget warning window is too small for a real wrap-up

- Case ID: `TURN-002`
- Source revision: `e9180a1` (2026-09-11, post-TOK-001; TURN-001's window
  unchanged since `e05726f`)
- Source observation (authentic, recorded, first live run of TURN-001):
  operator session `~/.pragma/logs/2026-09-11T21-21-59.jsonl`
  (`morph-glm53-744b`, provider-tools, default 100-turn budget):
  - The notice fired exactly as designed at turn 90 (~21:53,
    "90 of 100 … 10 remain") — injection and visibility proven live.
  - The model acknowledged it, stopped exploration, and switched to
    wrap-up (finishing the in-flight TOK-001 docs + commit).
  - The 10 remaining turns were insufficient: the wrap-up spanned a
    multi-file documentation sequence with one retry cycle (two failed
    `apply_patch` attempts), and the loop terminated at turn 100
    (~22:06, `provider tools loop exceeded maximum of 100 turns`) with
    the commit undone. The operator re-prompted (22:06:29) and the
    preserved conversation continued; recovery cost ~6 turns plus the
    operator's attention.
  - Operator directive (2026-09-11, the re-prompt itself and the follow-up
    delegation): "why haven't you fixed this limit yet? you killed
    yourself again" … "no it's all on you" — the window policy is handed
    to the harness to fix.
- Responsible path: `internal/query/provider_tools_loop.go`
  `turnBudgetWarnTurn` — the window is `maxTurns/10` (10 turns for the
  default 100). TURN-001's case anticipated exactly this evidence class:
  "If a session still dies mid-task despite the notice, that is new
  evidence about notice efficacy — record it; do not silently strengthen
  the mechanism without a case." This is that case.
- Earliest wrong transition: the window size — 10% of budget assumed a
  wrap-up fits in 10 turns; the observed wrap-up (finish in-flight step
  with one retry cycle, commit, handoff) needed more than 10.
- Expected behavior: for large budgets the notice fires with enough
  remaining turns to complete a realistic wrap-up: `maxTurns/5` — 20
  remaining at the default 100 (notice at turn 80). The small-budget
  fallback (`window < 5 → maxTurns/2`) is unchanged, so TURN-001's pinned
  small-budget behavior (20→10, 12→6, 8→4, 6→3, 2→1) is byte-identical.
  Notice text, single-injection semantics, cap, error text, `--max-turns`,
  `--resume`, and pragma loop mode are all unchanged. A premature-wrap-up
  side effect (model abandoning useful work at 80% of budget that it
  would have continued under the 10% window) is the refutation risk —
  watch for it in live runs; if observed, the remedy is an escalation
  design, not a silent re-narrowing.
- Explicitly out of scope: notice escalation (a second, stronger notice
  near the cap) — a separate mechanism with no recorded need yet (no
  session has died *with 20 turns of warning*); and any cap change (the
  cap is the cost control; continuation and override already exist).
- Executable assertion (baseline RED): `turnBudgetWarnTurn(100) = 90` and
  `turnBudgetWarnTurn(50) = 45` on current code; the case requires 80 and
  40 respectively, with 20/12/8/6/2/1/0 values unchanged.
- Executable assertion (candidate GREEN): the pinned window table passes
  (100→80, 50→40, 20→10, 12→6, 8→4, 6→3, 2→1, 1/0→none); a 100-turn
  scripted loop through `runProviderToolsLoop` carries no notice before
  request 81 (index 80), exactly one on request 81 naming "80 of 100 …
  20 remain", retained once through request 100, and still terminates
  with `exceeded maximum of 100 turns` (cap enforcement unchanged); the
  existing 20-turn loop test still expects its notice at index 10 (small
  budgets unchanged); pragma mode still emits no notice; the real-boot
  tbgreen wire gate (`--max-turns 8`, warn at 4) still passes on the
  rebuilt binary; full suite clean.
- Proposed mechanism (one): `window := maxTurns / 5` in
  `turnBudgetWarnTurn` (the `window < 5 → maxTurns/2` fallback stays).

## Verification record

- Baseline (revision `e9180a1` unchanged, 2026-09-11): engine gates via
  `go test ./internal/query/ -count=1`:
  - `TestTurnBudgetWarnTurnWindow` FAIL: `turnBudgetWarnTurn(100) = 90,
    want 80` — the old window.
  - `TestProviderToolsLoopTurnBudgetNoticeAtDefaultBudget` FAIL: the
    warn-iteration request (index 80 of the 100-turn scripted loop) carries
    0 notices — the notice only existed at index 90 under the old window.
    All assertions before it passed (100 provider requests, exact
    `exceeded maximum of 100 turns` cap error), so the failure is the
    missing window, not setup.
  - Adjacent tests passed on baseline: the 20-turn loop gate (small
    budgets) and `TestPragmaLoopNoTurnBudgetNotice`.
- Candidate (`window := maxTurns / 5` in `turnBudgetWarnTurn`; the
  `window < 5 → maxTurns/2` fallback, notice text, single injection, cap,
  error text, flags, pragma mode all untouched):
  - `TestTurnBudgetWarnTurnWindow` PASS — 100→80, 50→40, and every
    TURN-001 pinned small-budget value byte-identical (20→10, 12→6, 8→4,
    6→3, 2→1, 1/0→none).
  - `TestProviderToolsLoopTurnBudgetNoticeAtDefaultBudget` PASS — no
    notice on requests 1–80, exactly one on request 81 ("80 of 100 …
    20 remain"), retained exactly once through request 100, loop still
    ends with `exceeded maximum of 100 turns` after exactly 100 requests.
  - Adjacent: `TestProviderToolsLoopTurnBudgetNoticeBeforeCap` PASS (the
    20-turn loop's notice still at index 10 — small budgets unmoved);
    `TestPragmaLoopNoTurnBudgetNotice` PASS;
    `TestProviderToolsLoopErrorsOnToolFreeMaxTokensResponse` PASS
    (TOK-001 unaffected).
  - Real-boot wire gate on the rebuilt binary (rerunnable):
    `results/candidate-tbgreen-turn002/` — `turnbudget` profile with
    `--max-turns 8`: notice still at request 5 ("4 of 8 … 4 remain"),
    retained once, cap error exact. Small-budget wire behavior unchanged.
  - `make build` instruments 0/7719 branch points (idempotent); full
    suite exit 0, 29 packages ok, 3 consecutive parallel runs clean.
- Live-effect boundary: this proves the wider window is constructed and
  injected through the production path (deterministically at the operator
  default budget and on the real boot wire at a small budget). It does
  not claim the model now completes wrap-ups — the next live kill (if
  any) with 20 turns of warning becomes the next evidence step (either
  notice-efficacy limits, arguing for escalation, or a wrap-up-side
  problem outside the harness).

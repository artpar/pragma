# PAR-001 — Sibling tool calls in one assistant turn execute serially

- Case ID: `PAR-001`
- Source revision: `7353f07` (dispatch loop shape predates the current
  milestone work; observed live at `7353f07`, still present at `a021806`)
- Source observation (authentic, recorded; live operator session
  `~/.pragma/logs/2026-09-11T22-55-12.jsonl`, `morph-glm53-744b`,
  provider-tools loop):
  1. A four-call batch dispatched at 23:03:20.605 (`Agent` fork first,
     then `WebSearch`, two MCP calls) sat ~2m13s behind the first call.
     The operator interrupted at 23:05:33; the fork's in-flight request
     died (`APIRequestFailed … provider_error: context canceled`) and the
     three queued sibling calls failed instantly with "context canceled"
     **without ever executing** — the wall-clock story is in
     `docs/failure-cases/wall-clock-stamps-2026-09-11.md` (CLK-001),
     which recorded the same event.
  2. Operator directive: "why not make the sync thing async ? and then
     divide and conquer" — independent calls must not be coupled in
     latency, and independent investigations must be able to run
     concurrently.
  3. The harness's own model-facing contract promises it: the restored
     `Agent` tool description says "Use it to parallelize evidence
     gathering" — the engine serializes every batch.
- Responsible path: `internal/query/provider_tools_loop.go`
  `runProviderToolsLoop` — `for _, call := range toolCalls {
  executeProviderToolCall(ctx, call) }`: call N+1 cannot start until call
  N returns.
- Earliest wrong transition: the dispatch itself — sibling calls are
  independent by pairing contract (results are matched by
  `tool_call_id`, not execution order), yet execution is strictly
  sequential.
- Expected behavior: independent sibling calls in one assistant turn
  execute concurrently:
  - `ToolCallEvent`s are emitted in call order at dispatch;
    `ToolResultEvent`s are emitted as each call completes.
  - Results are collected by call index and appended as one user message
    in call order (wire shape unchanged — results were never
    execution-ordered on the wire).
  - Interrupt semantics unchanged: a canceled context kills in-flight
    calls; the difference is that already-completed results are kept and
    quick calls are no longer lost behind a slow first call.
  - `apply_patch` calls within one batch stay sequential in call order:
    two same-batch patches can target the same file, and concurrent
    execution is a lost-update hazard. This serialization is part of the
    dispatch mechanism's safety contract, not a second feature.
  - Turn counting, turn-budget notice, cap, error text, flags,
    sub-agent envelope, pragma loop mode: all unchanged.
- Concurrency audit (read-only, 2026-09-11): `model.CostTracker`
  (mutex), `app.StateStore` (RWMutex), the MCP client and manager
  (mutexes), and `observe.EventBus` (lock-free atomic producers) are safe
  under concurrent `executeProviderToolCall`; provider clients are
  per-call HTTP/SDK usage; `Bash` runs independent processes against a
  read-only cwd snapshot; `Agent` forks get separate engines and stores
  (shared provider/bus/costs are the mutexed/lock-free ones above). The
  only write-race hazard among the built-ins is `apply_patch` on shared
  files — handled by the sequential lane.
- Proposed mechanism: partition the batch — non-`apply_patch` calls run
  in goroutines (WaitGroup), `apply_patch` calls run sequentially in call
  order, both writing `results[i]`; the rest of the loop (results
  append, CLK-001 companion, notice, cap) is untouched.
- Evidence that would refute it: any data race or suite failure; any
  pairing or order break; any prior wire gate (MCPINJ `green`, CLK
  `clkgreen`, TURN/TOK) regressing on the candidate boot.
- Gates:
  - Baseline RED (unchanged `a021806`):
    `go test ./internal/query -run TestProviderToolsLoopDispatchesSiblingCallsConcurrently -count=1`
    fails at the concurrency assertion — the quick call's result is not
    observed before the 1s blocker's result because dispatch is
    sequential. The message-shape sanity assertions (loop completes,
    results order, single results message, companion stamp) pass first,
    so the failure is about concurrency, not setup. Observed (2026-09-12,
    `a021806`): `quick result (event 5) not observed before blocker
    result (event 3): sibling calls executed serially`.
  - Candidate GREEN: quick-before-blocker event order; results in call
    order in one message; pairing intact; CLK-001 companion present.
    Observed: both tests pass
    (`TestProviderToolsLoopDispatchesSiblingCallsConcurrently`,
    `TestProviderToolsLoopSerializesSameBatchApplyPatch`), and the same
    set plus the CLK/sub-agent tests pass under `-race`.
  - Hazard invariant: two same-batch `apply_patch` calls on one file both
    succeed in call order (passes baseline and candidate; guards the
    sequential lane).
  - Adjacent: the whole `internal/query` gate set (TURN, TOK, SUB, CLK,
    MCP, WEB) and the full suite ×3 parallel runs (exit 0, 29 packages
    ok each). Real-boot adjacent: the MCPINJ-001 `green` and CLK-001
    `clkgreen` assertion sets pass on a fresh candidate boot
    (`results/candidate-par-adjacent-2026-09-12/`) — a Bash + real MCP
    call executed concurrently in one turn (129 `mcp__` defs from 4
    servers, 59,768-byte tool result, pairing intact, stamps intact).
  - Real-boot wire gate: a new `parallel` probe profile returns one turn
    with two `sleep 3` `Bash` calls, then final text. The wire evidence is
    the request-2-minus-request-1 gap measured from raw capture
    `started_at` timestamps (delay 2s + tool phase): baseline ≥ 6.0s
    (serialized 3+3, measured 8.14s,
    `results/baseline-par-2026-09-12/`), candidate < 6.0s (concurrent
    max(3,3), measured 5.25s, `results/candidate-par-2026-09-12/`). Both
    runs completed the profile (exit 0, `PROBE_COMPLETE`, both results
    paired on request 2) — the difference was timing, not behavior.
- Claim boundary: concurrent execution of independent sibling calls and
  preserved result semantics are proven. Background (async) sub-agents
  and worktree isolation for writers remain separate follow-up
  mechanisms, each with their own case; this change does not detach
  forks from the turn context.

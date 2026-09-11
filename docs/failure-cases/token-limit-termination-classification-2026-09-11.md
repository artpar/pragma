# TOK-001 — Tool-free token-limit truncation is classified as successful session completion

- Case ID: `TOK-001`
- Source revision: `048cf8a` (2026-09-11, post-TURN-001; the defect predates
  it — same branch shape as the pre-`da6c7b1` tree)
- Source observations (authentic, recorded):
  - Terminal-Bench 2.1 job `pragma-lilac-glm52-final-89` (2026-08-30,
    provider-tools loop): the agent sessions for `circuit-fibsqrt__GazjRwB`
    and `dna-assembly__MRNvBTx` each end with their **last** recorded model
    response at `stop=max_tokens`
    (`.../circuit-fibsqrt__GazjRwB/agent/pragma-output.log` line 11,
    `.../dna-assembly__MRNvBTx/agent/pragma-output.log` line 79) followed by
    the cost line and nothing else — no error, exit 0. The harness
    verification record (2026-09-05) confirmed these events as the defect:
    "Token truncation is reported as task completion … Defect confirmed."
  - GLM-5.3 D01-v4 `regex-log` (glm53 ledger, 2026-08-30): the first model
    response ended at the 2,048-token `max_tokens` boundary without a tool
    call; Pragma "emitted successful turn completion and exited 0" and never
    created `/app/regex.txt`; the clean official verifier then failed the
    task — a harness classification failure recorded as a task failure.
  - Direct API probes (2026-09-06, `.pragma/verification/20260906-truncation/`):
    the truncation shape (`finish_reason length`, `content null`, no tool
    calls, reasoning ending mid-thought) reproduces at the provider boundary
    on two providers — it is not route-specific. The active route maps it
    too: morphllm converts `finish_reason=length` through
    `anyllm.StopReasonFromAnyLLM` → `model.StopMaxTokens`.
- Responsible production path: `internal/query/provider_tools_loop.go` — the
  tool-free terminal branch classifies **any** `StopReason != StopToolUse`
  response without tool calls as `TurnCompleteEvent` ("signals the agentic
  loop has finished", `internal/query/event.go`), including `StopMaxTokens`,
  where the model did not choose to stop — the output was cut by the limit.
  Downstream, `runNonInteractive` (`internal/cli/run.go`) returns nil →
  **exit 0**, indistinguishable from a deliberate `end_turn`; background
  sessions inherit the same classification. Only the TUI re-derives honesty
  from the stop reason (its `[response truncated — hit max_tokens limit]`
  notice, `internal/tui/handlers.go`).
- Earliest wrong transition: the loop's terminal classification. The turn
  budget's precedent classifies resource-exhaustion terminations as errors
  (`provider tools loop exceeded maximum of %d turns` → `ErrorEvent` →
  exit != 0); the token limit — the same class of resource exhaustion — is
  classified as success.
- Expected behavior: a tool-free `StopMaxTokens` termination is an **error
  termination** with a distinct, greppable message naming the truncation,
  the partial content still appended (conversation preserved — re-prompt or
  `--resume` continues it, exactly like the turn cap), and no
  auto-continuation. Sources: the turn-cap precedent in the same loop;
  `event.go`'s own contract (`ErrorEvent` = "an error that terminated the
  loop"); the recorded benchmark consumers (the Harbor adapter recognizes
  the turn-cap error text as non-fatal so the verifier still runs — the
  truncation error must get the same treatment, or truncated trials would
  turn from verifier-evaluated into censored `RuntimeError` trials).
- Explicitly out of scope (recorded boundary): the continuation mechanism of
  E003 (`da6c7b1`) stays reverted. It was removed at `6f3271c` after a frozen
  15-task comparison tied the starting product 8/15 to 8/15, and the TB-2.1
  audit restates that restoring it is not shown to improve benchmarks. This
  case claims classification honesty only — not task-success benefit.
- Executable assertion (baseline RED): a scripted provider returning a
  tool-free `StopMaxTokens` response through `runProviderToolsLoop` —
  current code: `TurnCompleteEvent`, no error, loop exits as success. Want:
  an `ErrorEvent` naming the truncation, and the partial assistant content
  still in the conversation.
- Executable assertion (candidate GREEN): the same script yields exactly the
  truncation `ErrorEvent`; `StopEndTurn` tool-free still yields
  `TurnCompleteEvent` (normal completion unchanged); a `StopMaxTokens`
  response **with** tool calls still executes its tools and continues
  (truncation-with-tool-call is the model still working, not a terminal);
  pragma loop mode emits no such error (its own completion semantics,
  untouched); a sub-agent whose final response is truncated pairs
  `Agent failed: …truncated…` as an error result instead of a
  `{"status":"completed",…}` envelope carrying partial text.
- Proposed mechanism (one): in `runProviderToolsLoop`, classify tool-free
  `StopMaxTokens` as an `ErrorEvent` (terse text, stable prefix
  `final response truncated by max_tokens output limit`) placed before the
  generic tool-free terminal branch; no loop continuation, no cap changes,
  no new event kinds. Plus the recorded consumer's compatibility exception:
  `tools/harbor_pragma_agent.py` recognizes the truncation error text as
  non-fatal (same shape as the turn-cap exception) so verifiers still run on
  truncated trials. The TUI truncation notice stays as-is (defensive for
  any other emitter; interactive truncations now surface via the error
  line, which preserves the session the same way).
- Evidence that would refute the case: a recorded model behavior where
  `stop_reason=max_tokens` is a deliberate completion signal (the protocol
  reserves it for limit cuts — no such event exists in the records); or a
  consumer that requires exit 0 on truncation to function (none found —
  the adapter's fatal-exit path exists to catch crashes, and the turn-cap
  exception already establishes the pattern for budget-exhaustion exits).

## Verification record

- Baseline (revision `048cf8a` unchanged, 2026-09-11):
  - Engine gates (`go test ./internal/query/ -count=1`, scripted provider,
    no spend): `TestProviderToolsLoopErrorsOnToolFreeMaxTokensResponse` and
    `TestProviderToolsLoopErrorsOnReasoningOnlyMaxTokensResponse` FAIL at
    the first assertion ("must terminate with an ErrorEvent") — the loop
    emitted `TurnCompleteEvent` instead;
    `TestProviderToolsLoopAgentSubTruncationPairsError` FAIL ("truncated
    sub-agent result must be an error result") — the sub was reported as a
    completed envelope carrying partial text. Both failures are the
    missing classification, not setup (the loops ran to their terminal
    branch in each case). The adjacent tests passed on baseline:
    truncation-with-tool-call continues; pragma mode completes a no-action
    truncated final text as before.
  - Wire gate (real binary, real session boot, scripted local provider —
    `e2e/mcp-injection`: new `toklimit` profile, rerunnable via
    `run_probe.sh <label> toklimit && assert_boot.py results/<label> tokred`):
    preserved in `e2e/mcp-injection/results/baseline-tokred/` — the
    captured wire shows the tool-free `finish_reason=length` response; the
    process exited **0**; stderr ends `[model response: gpt-4o
    stop=max_tokens]` + cost line with no truncation signal — byte-shape
    identical to the recorded circuit-fibsqrt/dna-assembly log endings.
- Candidate (the loop branch + the Harbor adapter exception; no other
  production code touched):
  - Engine gates: all three RED tests PASS. Truncation (text and
    reasoning-only shapes) → `ErrorEvent` naming
    `final response truncated by max_tokens output limit …; the conversation
    is preserved — continue with a new prompt or --resume`; the partial
    assistant message is preserved in the conversation (both TextPart and
    ThinkingPart) for re-prompt/`--resume` continuation; the sub-agent
    truncation pairs `Agent failed: …truncated…` with `IsError` and intact
    pairing on the parent's next request.
  - Adjacent: `StopEndTurn` tool-free still `TurnCompleteEvent`
    (`TestPragmaLoopTextOnlyResponseCompletesWithoutRetry`,
    `TestProviderToolsLoopExecutesBashToolCall` final leg);
    `StopMaxTokens` **with** tool calls still executes the tools and
    continues (`TestProviderToolsLoopMaxTokensWithToolCallsContinues`);
    pragma loop mode unchanged
    (`TestPragmaLoopMaxTokensTextStillCompletes`,
    `TestPragmaLoopNoTurnBudgetNotice`); TURN-001's budget notice and cap
    unchanged (`TestProviderToolsLoopTurnBudgetNoticeBeforeCap`,
    `TestTurnBudgetWarnTurnWindow`); turn-cap error text unchanged.
  - Wire gate: preserved in `e2e/mcp-injection/results/candidate-tokgreen/`
    — exit **1**; stderr's last line is
    `error: final response truncated by max_tokens output limit before completion; the conversation is preserved — continue with a new prompt or --resume`
    (same `error:` prefix shape as the turn-cap error the adapter already
    recognizes); partial text still delivered on stdout; exactly **1**
    model request — no auto-continuation (E003 stays reverted).
  - Adapter: `python3 -m py_compile` OK; classifier exercised against the
    **real captured stderr** files: truncated trial non-fatal (verifier
    still runs), turn-cap trial still non-fatal, ordinary failures still
    fatal, exit 0 always non-fatal.
  - `make build` instruments 0/7719 branch points on consecutive runs
    (the pre-written trace line matches the instrumenter's output;
    idempotent); `make smoke` OK; `go test ./... -count=1` exit 0 across
    5 consecutive parallel runs (29 packages ok each) — the house standard.
- Claim boundary: the false-success classification is repaired for
  tool-free token-limit truncations in the provider-tools loop and its
  consumers (batch exit code, background sessions, sub-agent pairing,
  Harbor adapter). No claim is made that classifying truncations honestly
  improves task success; auto-continuation remains unported pending real
  affected-run evidence (the `6f3271c` boundary).

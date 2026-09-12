# Pragma harness development methodology

Current development policy, established 2026-09-06. Read this before planning,
changing, or evaluating the harness. Historical goal prompts and experiment
plans in docs/ are evidence, not standing instructions or authorization to run.
MorphLLM with `morph-glm53-744b` is the active benchmark route; Lilac was removed. Do not revive or
migrate historical Lilac configurations without an explicit current request.

## Operator role

The operator initiates work and grants authority ("continue", "you decide",
"proceed" are standing delegations within this methodology); they do not
author this methodology, the roadmap, or technical plans, and they do not
improvise input mid-session. Amendment 2026-09-12 ("not my call, you are
the master, its your call always"): technical and sequencing decisions are
the agent's call by default — bounded self-improvement work proceeds on
the agent's own judgment without asking; the operator initiates work and
handles human-only actions. Announce, rather than request, anything
unusually costly or irreversible before doing it. Treat every operator
message as one of: task
initiation, delegated authority, or the answer to a question the session
asked — a prompt the operator relays may itself have been written by a
prior session, so verify against the committed records rather than the
sender. When a step needs a human (interactive input during a running
turn, publishing to a remote, spending credits, credential or physical
actions), request it explicitly with exact instructions instead of waiting
for spontaneous action; otherwise proceed autonomously. Operator reports
of observed behavior are evidence; operator technical claims are verified
like any other claim.

## Objective and scope

Improve the harness through demonstrated defects and repeatable checks. Do not
equate more code, passing synthetic tests, more model calls, or one task win with
progress. Separate harness correctness, generated-solution correctness,
evaluation infrastructure health, and model-dependent task success.

## Required workflow

1. Inspect existing records before generating new ones. Locate a real failing
   event, the responsible production path, and the earliest wrong transition.
   State the expected behavior and its source (task requirement, protocol
   contract, or explicit harness policy). A preferred model answer is not a
   deterministic harness contract. If evidence is missing, collect only the
   missing observation; label the issue unconfirmed until reproduced.
2. Write a failure case before fixing code. Record the case ID, source run and
   event IDs, source revision, exact observed/expected behavior, executable
   assertion, proposed mechanism, and evidence that would refute it.
3. Reproduce through the actual production code with authentic recorded input.
   Restore the minimum relevant conversation/filesystem state and execute to
   the decision under test. The unchanged harness must fail the assertion for
   the stated reason, not because a dependency is absent or setup is broken.
4. Make one mechanism-level change. Run the identical case against the changed
   harness; require the assertion to pass. Check adjacent successful cases and
   affected integration paths. Preserve the failing baseline and result logs.
5. Advance only to the cheapest additional test needed for the remaining claim.
   Do not rerun an entire solver task to verify serialization or classification.
6. Report what is proven, what is not, evidence locations, commands, durations,
   costs, and the next unresolved decision. Keep failed experiments in the record.

## Authentic replay and its boundary

Use captured provider responses, tool results, sessions, and filesystem snapshots,
not fabricated successful model outputs. Exercise real parsers, state machines,
serialization, storage, and tool execution where those are the subject of the test.
Synthetic unit tests may supplement coverage; they cannot establish that an
observed production defect existed or that its production path was repaired.

Replay is a deterministic code regression test, not a fresh model trial. Assert
semantic fields and ordering precisely; normalize only documented volatile data
such as request IDs or timestamps. Never normalize away the disputed behavior.

Stop counterfactual replay when the changed harness would send a different model
request or execute a different external action. Assert that boundary's output.
Do not consume recorded downstream responses as if the changed interaction had
actually produced them. A live continuation is needed to observe that reaction.

Reexecute real local commands in isolated restored environments when testing
execution behavior. Never replay writes against production services. Preserve
input checksums, capture provenance, runtime/dependency/image versions, and
sanitization notes. Exclude credentials and prevent hidden verifier/oracle data
from entering solver context. Keep fixtures durable, not solely under /tmp.

## Verification gates

| Gate | Evidence required | Claim allowed |
| --- | --- | --- |
| Recorded regression | Real failing input; old code fails and changed production code passes exact assertion | This harness defect is repaired for the case |
| Real local integration | Actual processes/files/tools and restored state; observable completion assertions | This execution behavior works in this environment |
| Live provider contract | Small real request through changed adapter; inspect outgoing wire and returned result | Provider accepts the changed interaction |
| Bounded live continuation | Same pre-intervention state, declared control, matched budgets and functioning verifier | Observed behavioral effect on the diagnostic cases |
| Held-out evaluation | Frozen candidate, declared task set/repetitions/metrics and healthy evaluator | Evidence for task-success generalization, with uncertainty |

Use applicable gates, not every gate mechanically. A local classification fix
need not spend inference credits. A replay passing cannot establish score uplift.
Do not claim deterministic live inference because temperature is zero.

## Live experiments are exceptions to the replay loop

Before spending inference, write the question replay cannot answer, treatment,
control, exact checkpoint, model/provider and effective settings, call/token/time/
cost ceilings, repetitions, success/rejection criteria, and stopping conditions.
Verify outgoing settings rather than trusting CLI flags or provider metadata.
Keep actual call counts and costs: equal turn limits can hide retries or extra calls.

Change one mechanism per causal comparison. Restore identical pre-intervention
state where possible; record provider routing and other unmatched conditions.
The intervention must occur before the decision it allegedly changes. Do not
attribute a first-response improvement to a later reasoning-replay change.
An equal-budget ordinary continuation is required when claiming a special review
instruction helps beyond simply allowing more work. Cases selected after seeing
their failures are diagnostic, not held-out evidence.

Do not launch a broad benchmark to discover whether a code change works. Require
local mechanism evidence first and a bounded behavioral signal for score claims.
No repeated runs until a favorable sample appears, moving acceptance criteria,
unreported losses, or bundled persona/runtime changes presented as an ablation.

## Evaluator and feedback-loop discipline

Preflight the actual verifier with a known reference in the target environment.
Check task/test hashes, architecture, dependencies, resources, disk, and that
tests actually execute. Classify infrastructure failure, solver failure, timeout,
operator cancellation, and missing result separately; retain original rewards.
Stop a batch on infrastructure failure instead of accumulating misleading zeros.

Cache pinned verifier dependencies and rerun unchanged real tests against existing
submissions in fresh isolated containers. Label this a cached diagnostic, not a
fresh official full-script run. Measure setup, inference, tools, and verification
separately. Assertions must observe completed required work, not entry markers,
successful imports, command submission, or reassuring final text.

## Review, delegation, and completion

When subagents are requested, give each a bounded independent evidence question
and concrete deliverable. Track running/completed/shelved tasks explicitly. The
main agent must read findings, reconcile contradictions, and incorporate or reject
them with reasons. A subagent's confidence is not verification.

A fix is complete only with a reproducible command, authentic case provenance,
baseline failure, candidate pass, adjacent regression checks, and an explicit
claim boundary. A task-score claim additionally needs model-dependent evidence.
If a hypothesis fails, record rejection and stop pursuing it without new evidence.

## Documentation maintenance

This file is the methodology source of truth. docs/README.md identifies current
entry points. Keep dated observations as historical evidence; mark superseded
plans and old goal prompts inactive so future agents do not restart them.
Preserve unique traces, results, checksums, and negative findings. Remove only
verified redundant/disposable material within the user's scope; prefer recoverable
archiving and record what moved or was removed. Age alone does not imply irrelevance.

First implemented regression: OpenRouter nonstreaming reasoning-field loss,
`go test ./internal/provider/openrouter -run TestRecordedReasoningSurvivesSessionReplay -count=1`.
Provenance and claim limits are in internal/provider/openrouter/testdata/README.md.
Examined 2026-09-11: verifier-startup failures misclassified as solver
failures has no pragma-owned production path. The TB-2.1 audit attributes
the 24 false-looking zeros to Harbor's own aggregate; this repo's only
trial-outcome decision point is the adapter's fatal-exit gate
(`tools/harbor_pragma_agent.py`), which is deliberate and gated (turn-cap
and truncation exits are non-fatal so verifiers run); no reward classifier
exists under `evaluation/` or `tools/`, and Harbor's source is neither
vendored nor installed locally. The target is closed as external unless a
pragma-owned benchmark runner ever appears. Token-limit termination
classification is implemented (2026-09-11, TOK-001:
docs/failure-cases/token-limit-termination-classification-2026-09-11.md).
Second implemented regression: mid-turn operator input is queued into
the conversation instead of rejected-and-dropped (2026-09-12, INT-001),
`go test ./internal/cli -run TestRunInputQueuesPlainTextWhileTurnActive -count=1`.
Third: Bash tool input accepts the observed `command` key alias and
names the received keys on empty-input errors (2026-09-12, INT-002),
`go test ./internal/query -run TestProviderBashToolAcceptsCommandAlias -count=1`.
The provider-tools loop has no default turn cap since 2026-09-12
(TURN-003; explicit --max-turns bounds unchanged),
`go test ./internal/query -run TestProviderToolsLoopNoDefaultTurnCap -count=1`.
Fourth: a thinking-only final response rendered no operator-readable output
in the default view; its trailing thinking is now promoted at
end_turn-without-text and on resume (2026-09-12, TUI-001),
`go test ./internal/tui -run TestThinkingOnlyEndTurnShowsInstruction -count=1`.
Fifth: a paused stop reason (pause_turn) rendered nothing in the TUI — no
notice for any paused turn, and a thinking-only paused response stayed a
lone collapsed hint; pause_turn now carries a pause notice and the
TUI-001 textless promotion (2026-09-12, TUI-002),
`go test ./internal/tui -run TestThinkingOnlyPauseTurnShowsInstruction -count=1`.
Sixth: a running tool call rendered no elapsed time — the animating
spinner line carried no duration information, so a 361-second call was
indistinguishable from a 6-second one in the default view
(operator-observed, 2026-09-12, TUI-003); the spinner line now renders the
running call's elapsed time (`⣾ Bash... 5m11s`), stamped at the tool
call and refreshed each spinner tick,
`go test ./internal/tui -run TestRunningToolShowsElapsedTime -count=1`.
Seventh: a running foreground tool call emitted no output until it
completed — a 6-minute command was invisible the whole time
(operator-observed, 2026-09-12, TUI-004); the executor now streams the
bounded output-so-far to the event channel while the command runs, and
the TUI renders it inline, replaced in place by the final result,
`go test ./internal/query -run TestBashToolEmitsLiveOutputWhileRunning -count=1`
and `go test ./internal/tui -run TestLiveToolOutputRendersInline -count=1`.
Eighth: the running-tool spinner (and, with TUI-003, its elapsed time)
 died on the first of several parallel sibling results while the others
still ran (2026-09-12, TUI-005); the spinner now survives partial
sibling results, names the remaining work, and anchors its elapsed clock
to the oldest still-running call,
`go test ./internal/tui -run TestSiblingSpinnerSurvivesPartialResults -count=1`.

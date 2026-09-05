# Pragma benchmark investigation — 2026-09-05

Historical read-only audit. Its scope statements describe this initial audit,
not the later live experiments in [the verification record](harness-verification-plan-2026-09-05.md).
Current development policy is [agent.md](../agent.md).

Scope: read-only investigation of product commit `1315159`, historical ledgers,
evaluation configurations, saved Harbor trial results, verifier output, agent
output, and current provider/query/shell implementation. No new paid inference,
benchmark executions, product changes, or environment changes were performed.
This report is the only new file. Historical SWE findings below are attributed
to the saved reports; not every original SWE run directory remains available.
The evidence does not establish which individual edits were authored by Luna
versus Sol, so conclusions concern the recorded engineering process.

## Finding

Pragma does solve benchmark tasks. What has not been demonstrated is a reliable
improvement over its starting implementation, or an advantage over a competing
harness under matched conditions. Three problems are being conflated:
unusable evaluation infrastructure, genuine product defects, and insufficient
task-solving/verification behavior. The experiments repeatedly improve a local
mechanism without establishing that it changes the dominant source of task
failure.

## 1. The latest full evaluation is badly contaminated and unreported

The committed GLM-5.2 ledger ends with a precommitment to an 89-task run. The
saved run actually finished on August 30, after about 6h28m:

`../.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/result.json`

I aggregated all 89 trial `result.json` files and inspected the endings of all
37 saved verifier `test-stdout.txt` files:

| Observation | Count |
| --- | ---: |
| Official reward 1 | 4 |
| Official reward 0 | 33 |
| No reward | 52 |
| Exceptions, including one timeout that also received zero | 53 |
| Runtime exceptions explicitly containing disk exhaustion | 49 |
| Runtime exception due to requesting more than the VM's two CPUs | 1 |
| Other exceptions | 3: test upload, missing reward, agent timeout |
| Reward-bearing trials whose verifier failed before running its tests | 24 |
| Verifier logs with an actual pytest results summary | 13 |

The last two rows partition the 37 reward-bearing trials: 23 logs report
`uvx: command not found`; another (`bn-fit-modify`) times out downloading
Python. The 13 pytest summaries contain four passes and nine failures. That
subset is not a representative benchmark score, and successful verifier
startup alone does not prove the agent's preceding environment was healthy.

The aggregate 4/89 (4.49%) must remain the recorded outcome, but cannot be
interpreted as a clean model/harness capability estimate. For example,
`fix-git__Pgo2UGe/verifier/test-stdout.txt` explicitly says there is insufficient
space in `/var/cache/apt/archives/`, then fails to find curl and uvx. Its zero
does not measure whether the Git task was solved.

This repeats the failure already documented in the B02 correction: disk and
verifier bootstrap failures can be serialized as reward zero, with no Harbor
exception. The final job kept advancing through many disk failures, and no
completed outcome/classification was appended to the ledger. The evaluation
workflow lacks an effective infrastructure-health gate and completion audit.

## 2. The clean comparison is a tie, not an ablation of each fix

The cleaned B02 result is 8/15 START versus 8/15 cumulative candidate: three
gains, three losses, nine ties. I cross-checked the task rewards in the saved
infrastructure and residual reruns against the ledger. The three gains are
constraints-scheduling, financial-document-processor, and
feal-linear-cryptanalysis; the losses are crack-7z-hash, db-wal-recovery, and
extract-elf.

See `terminal-bench-glm52-ledger.md`, lines 563–739, and the jobs named
`pragma-lilac-glm52-b02-{start,champion}-infra-rerun-v3`,
`...-residual-infra-rerun-v4`, and the clean count-dataset-tokens discordant
reruns.

Withholding an improvement claim is correct. But fifteen paired tasks with
multiple simultaneous product differences do not identify each mechanism's
effect, nor establish equivalence. The work reverted reasoning retention,
model metadata, and truncation continuation together. That implements the
precommitted rollback policy, but the policy conflates protocol correctness
with demonstrated aggregate uplift. The evidence supports neither promoting
each fix as a score improvement nor declaring each fix ineffective.

Other limitations: tasks were a lexical block; START always ran first; most
clean task pairs have one sample per arm; repeated runs already exhibit
different pass/fail outcomes. Temperature zero did not make these observed
runs repeatable. These comparisons also compare Pragma against Pragma. They
do not isolate Pragma overhead against another harness using the same model,
provider, tasks, tools, and resource budget.

## 3. Known product defects are present again

Current source confirms:

- `internal/query/provider_tools_loop.go:86`: any tool-free response whose stop
  reason is not tool-use emits `TurnCompleteEvent` and returns. This includes
  `StopMaxTokens`. The final run's `circuit-fibsqrt` and `dna-assembly` logs both
  terminate at `stop=max_tokens`. Their verifiers also failed to bootstrap, so
  these traces prove premature termination, not a recoverable benchmark pass.
- `internal/provider/lilac/provider.go:575`: `toAnyLLM` copies role, content,
  and tool calls but omits the already-decoded reasoning. That is observable
  response data loss. Whether resending reasoning improves this exact model
  remains an empirical question separate from preserving its response.
- `internal/provider/lilac/models.go`: GLM-5.2 is absent after rollback. Final
  logs warn that the model is unknown and that costs will be zero. Consequently
  their `$0.000000` totals do not measure real inference cost.

The benchmark path is also much thinner than the repository's architecture
suggests. The Harbor adapter invokes `--loop provider-tools`. That loop exposes
only Bash and apply_patch and returns on ordinary tool-free completion. It
does not run the SWE persona/FSM machinery. Its request preparation forwards
the conversation and checks tool pairing; the loop increments a compaction
tracker but never invokes the compactor.

Its Bash schema exposes only `cmd`; execution uses a fixed 300-second timeout
and foreground wait, and returns completed command output without a size cap.
There is no native stdin/PTY or model-controlled timeout in this path. These
are concrete affordance limitations and plausible long-task bottlenecks, not
established explanations for the aggregate B02 tie. They need branch-specific
trajectory evidence before being chosen as the next optimization.

## 4. Clean failures show weak requirement coverage and premature confidence

The final run contains real task failures as well as infrastructure failures:

- `cancel-async-tasks`: five tests pass, but queued-task SIGINT cleanup fails.
  Agent output says “Everything works” and claims cleanup was verified. The
  same behavioral gap was recorded in the initial diagnostic panel. The local
  check did not establish the full cancellation contract.
- `build-cython-ext`: ten verifier tests pass; the Cython complexity function
  fails on a remaining NumPy `int` alias. The agent reports all 18 repository
  tests pass and asserts the compiled extensions work. Those checks missed
  a required runtime path.
- `build-pov-ray`: rendering and version checks pass, but source provenance
  fails because expected authentic source files are absent.

These are failures to establish the requested result. More provider retries,
accurate model metadata, or additional generic review prose do not directly
repair them. A useful next hypothesis must change how the agent derives and
tests the task contract, then demonstrate improved completion on unseen tasks.
Benchmark hidden tests may inform offline diagnosis, but must not be supplied
to the agent during evaluation.

## 5. The older SWE work overdeveloped the workflow before proving it

The June 24 goal summary explicitly reports no proven evaluated success.
It nevertheless estimates overall completion at 60–70%, supported largely by
runtime hardening and local Go tests. That percentage has little predictive
value for a goal whose essential outcome has not occurred.

The saved trajectory comparison describes a no-change repair loop when an
earlier item had already added the required enum. The orchestration AB report
describes 160 turns culminating in approval without validating the failing
configuration-loading behavior. Those reports distinguish model-produced
approval from official benchmark success, but engineering effort continued
to accumulate around local handoffs, evidence gates, and repair policies.

Current artifacts show the resulting complexity: 1,031 lines in the SWE
orchestration and 527 in the engineering-worker persona. The worker is forbidden
from running build/test commands and must route necessary validation through
other states. Such separation introduces extra handoffs into the edit/test
feedback loop. The historical failures support concern about that overhead;
they do not prove that every decomposition is harmful or that removing it will
win a benchmark.

Sources: `swe-bench-pro-vibethink-minimax-goal-summary-2026-06-24.md`,
`codex-vs-pragma-flipt-kubernetes-trajectories-20260603.md`, and
`swe-bench-pro-orchestration-ab-ledger.md`.

## What the next campaign needs

1. A healthy, reproducible evaluator first: compatible compute, sufficient
   Docker storage/CPU, tested dependency bootstrap, and automatic suspension
   on infrastructure collapse. Preserve official rewards while separately
   auditing whether tests actually ran. Finish classifying the existing run.
2. One explicit target and product path. For harness advantage, use a matched
   same-model reference harness; use a stronger model separately to diagnose
   capability limits. Cross-model comparisons cannot isolate harness quality.
3. A correctness baseline independent of score experiments: preserve response
   data, represent truncation honestly, and report model/cost metadata accurately.
   Treat continuation strategy and reasoning replay as separately measurable
   behavior choices. Do not claim these guarantee higher scores.
4. Rank recurring clean failures by causal mechanism and frequency. Test one
   intervention at a time, including its actual branch activation, with repeated
   paired tasks and balanced arm order. Keep untouched evaluation tasks.
5. For SWE orchestration, compare a simple continuous engineer loop against the
   full FSM under matched budgets before adding more persona policies. For
   Terminal-Bench, focus first on observed requirement/verification gaps rather
   than assuming unused repository subsystems will improve the active loop.
6. Define the measurable stopping criterion: paired task success, uncertainty,
   cost, and elapsed time. A full-suite result without a comparator measures
   absolute performance, not improvement.

No evidence here guarantees that the fixed GLM model can exceed a particular
external leaderboard. The strongest conclusion is that the present process
cannot reliably distinguish model limits from harness limits and repeatedly
spends effort before that distinction is established.

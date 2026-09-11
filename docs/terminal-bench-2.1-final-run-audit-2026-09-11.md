# Terminal-Bench 2.1 final-run audit, 2026-09-11

## Executive finding

The 2026-08-30 `pragma-lilac-glm52-final-89` job is not a valid full-set
Terminal-Bench 2.1 score. Harbor moved all 89 tasks to a terminal state, but
only 13 tasks have clean verifier evidence: 4 passes and 9 failures. The other
76 tasks are censored by evaluation failures or have no trustworthy verifier
outcome.

The aggregate mean `0.0449438202247191` is exactly 4/89, but it must not be
reported as a 4.49% Pragma score. It treats infrastructure-contaminated and
unexecuted trials as benchmark failures.

This run did not use personas, an orchestration FSM, subagents, or a reviewer.
It used Pragma's ordinary single-agent `provider-tools` loop. It also used
GLM-5.2 through the historical Lilac provider, not GLM-5.3.

## Provenance

| Field | Recorded value |
|---|---|
| Benchmark | Terminal-Bench 2.1, 89 tasks |
| Job | `pragma-lilac-glm52-final-89` |
| Job ID | `35597e47-3684-478d-af5a-dcb79305b44c` |
| Start | `2026-08-30T11:16:16.891546` |
| Finish | `2026-08-30T17:44:43.563273` |
| Product revision | `6f3271c` (`GLM52_FINAL_CANDIDATE`) |
| Model | `zai-org/glm-5.2` |
| Provider | Lilac, now removed from the active product |
| Agent path | `tools.harbor_pragma_agent:PragmaAgent` |
| Loop | `provider-tools` |
| Sampling | temperature 0; maximum output 32,768 |
| Limits | 100 model turns; 1,800-second agent timeout; serial concurrency 1 |
| Permissions | `bypassPermissions` inside the task container |
| Raw aggregate | 89 terminal states; 53 error records; 37 reward records |

Primary evidence:

- `.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/result.json`
- `.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/job.log`
- `.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/*/result.json`
- `.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/*/agent/pragma-output.log`
- `.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/*/verifier/test-stdout.txt`
- `evaluation/terminal-bench/final-lilac-glm52-89.json`
- `tools/harbor_pragma_agent.py`

## Was the persona system running?

No. The evaluation configuration names `PragmaAgent`, and the adapter command
is the normal direct CLI path:

```text
/opt/pragma/pragma --prompt <instruction>
  --loop provider-tools
  --provider lilac
  --model zai-org/glm-5.2
  --temperature 0
  --max-tokens 32768
  --max-turns 100
  --permission-mode bypassPermissions
  --record
```

There is no `orchestration run`, persona directory, orchestration YAML, team,
or delegation argument. This was the simple LLM/tool loop, not the earlier SWE
persona experiment.

## Corrected outcome accounting

The aggregate has 37 reward records: 4 rewards of 1.0 and 33 rewards of 0.0.
That split is misleading because 24 of the 33 zeros never ran a trustworthy
task verifier.

| Outcome class | Tasks | Interpretation |
|---|---:|---|
| Clean verified pass | 4 | Genuine task successes |
| Clean verified failure | 9 | Genuine model-plus-harness failures |
| Zero emitted after verifier bootstrap failure | 24 | Censored; not task failures |
| Error without a usable reward | 52 | Censored; task was not validly evaluated |
| Total | 89 | Only 13 have clean task outcomes |

The 53 error records consist of the 52 error-only trials plus
`extract-moves-from-video`, which has both a zero reward and an
`AgentTimeoutError` record.

The clean subset is 4/13, or 30.8%, but it is not a benchmark estimate. The
subset was determined by which environments and verifiers happened to survive
the host failures, so it is neither frozen nor representative.

## Four clean passes

| Task | Verifier result |
|---|---|
| `adaptive-rejection-sampler` | 9/9 tests passed |
| `build-pmars` | 4/4 tests passed |
| `financial-document-processor` | 7/7 tests passed |
| `kv-store-grpc` | 7/7 tests passed |

These passes establish that the uploaded binary, provider route, normal tool
loop, workspace mutation, and official verifier path could work in at least
some trials. They do not rescue the full-run score.

## Nine clean failures

| Task | Agent termination | Verifier evidence | Earliest supported failure classification |
|---|---|---|---|
| `break-filter-js-from-html` | 100-turn cap after 100 model responses | 0/1; `/app/out.html` absent | The loop exhausted its budget without producing the required artifact. |
| `build-cython-ext` | Normal `end_turn` after 58 responses | 10/11; runtime uses removed `numpy.int` | The delivered extension was mostly correct but incompatible with the verifier's NumPy runtime. The agent declared completion without catching it. |
| `build-pov-ray` | Normal `end_turn` after 64 responses | 2/3; authentic POV-Ray 2.2 files such as `file_id.diz` absent | The binary and render worked, but the source tree was incomplete. The final answer explicitly claimed the source was complete. |
| `cancel-async-tasks` | Normal `end_turn` after 8 responses | 5/6; two running tasks printed no `Cleaned up.` on cancellation | The implementation mishandled the above-limit cancellation edge case. The agent's final answer said cancellation cleanup had been verified. |
| `chess-best-move` | Normal `end_turn` after 46 responses | 0/1; output contained only `g2g4`, not both `g2g4` and `e2e4` | The produced answer was incomplete or wrong. |
| `configure-git-webserver` | Normal `end_turn` after 69 responses | 0/1; HTTP 200 returned an empty body instead of `hello world` | Service availability was mistaken for functional correctness. The verifier also lacked `git`, but its decisive observation was the wrong served content. |
| `fix-code-vulnerability` | 100-turn cap after 100 responses | 2/6; `/app/report.jsonl` absent and `_hkey` accepted a newline-containing key | The loop exhausted its budget with both missing deliverables and an incomplete security fix. |
| `headless-terminal` | Normal `end_turn` after 8 responses | 6/7; interactive Vim command did not create `/app/vim.txt` | Non-interactive and persistence behavior worked, but interactive-terminal handling did not. |
| `largest-eigenval` | 100-turn cap after 100 responses | 22/27; five matrix-size performance checks missed the reference | Numerical correctness largely worked, but the required speedup did not; the loop spent its full budget without reaching the performance target. |

This clean sample exposes two genuine failure modes:

1. **Unproductive long trajectories:** three tasks reached the 100-turn ceiling.
2. **False completion confidence:** six tasks ended normally despite a wrong or
   incomplete deliverable. Several final messages asserted that everything had
   been verified even though the official verifier later found a direct
   counterexample.

The evidence does not isolate these as Pragma-code defects. They are observed
failures of the combined GLM-5.2 + prompt + provider serialization + tool loop
+ budget system. A causal harness claim would require authentic replay to the
earliest wrong decision and a matched live continuation.

## Twenty-four false-looking zeros

These trials have reward 0.0 in the aggregate, but the verifier did not reach a
trustworthy task assertion.

### Verifier Python download timeout: 1

- `bn-fit-modify`: the agent created the requested artifacts and ended
  normally, but the verifier failed downloading the standalone Python runtime
  from GitHub after three retries.

### Explicit verifier disk exhaustion: 11

- `circuit-fibsqrt`
- `cobol-modernization`
- `compile-compcert`
- `crack-7z-hash`
- `distribution-search`
- `dna-insert`
- `extract-elf`
- `extract-moves-from-video`
- `feal-linear-cryptanalysis`
- `fix-git`
- `git-leak-recovery`

Their verifier logs contain direct evidence such as `No space left on device`,
APT reporting insufficient free space, or package downloads failing during the
verifier bootstrap. `extract-moves-from-video` also exhausted the 1,800-second
agent budget, but its task result remains unknown because the verifier itself
did not run successfully.

### APT signature/install failure before verifier execution: 12

- `constraints-scheduling`
- `count-dataset-tokens`
- `custom-memory-heap-crash`
- `db-wal-recovery`
- `dna-assembly`
- `feal-differential-cryptanalysis`
- `filter-js-from-html`
- `gcode-to-text`
- `large-scale-text-editing`
- `llm-inference-batching-scheduler`
- `log-summary-date-ranges`
- `merge-diff-arc-agi-task`

These logs show invalid package signatures or failed package installation,
followed by missing `curl`, missing the installed Python environment, and
missing `uvx`. They do not contain completed task-test output. Disk exhaustion
was already repeatedly present in the same job, so it is a strong common-cause
hypothesis, but these twelve records do not individually prove that disk was
the only cause.

## Fifty-two error-only trials

| Error class | Count | Tasks |
|---|---:|---|
| Docker/host disk exhaustion | 49 | `fix-ocaml-gc`, `git-multibranch`, `gpt2-codegolf`, `hf-model-inference`, `install-windows-3.11`, `mailman`, `make-mips-interpreter`, `mcmc-sampling-stan`, `model-extraction-relu-logits`, `modernize-scientific-stack`, `mteb-leaderboard`, `mteb-retrieve`, `multi-source-data-merger`, `nginx-request-logging`, `openssl-selfsigned-cert`, `overfull-hbox`, `password-recovery`, `path-tracing-reverse`, `path-tracing`, `polyglot-c-py`, `polyglot-rust-c`, `portfolio-optimization`, `protein-assembly`, `prove-plus-comm`, `pypi-server`, `pytorch-model-cli`, `pytorch-model-recovery`, `qemu-alpine-ssh`, `qemu-startup`, `query-optimize`, `raman-fitting`, `regex-chess`, `regex-log`, `reshard-c4-data`, `rstan-to-pystan`, `sam-cell-seg`, `sanitize-git-repo`, `schemelike-metacircular-eval`, `sparql-university`, `sqlite-db-truncate`, `sqlite-with-gcov`, `torch-pipeline-parallelism`, `torch-tensor-parallelism`, `train-fasttext`, `tune-mjcf`, `video-processing`, `vulnerable-secret`, `winning-avg-corewars`, `write-compressor` |
| Docker CPU allocation exceeds host capacity | 1 | `caffe-cifar-10` requested more than the two CPUs available to Docker |
| Harbor tests-directory setup failure | 1 | `code-from-image` |
| Verifier reward artifact missing | 1 | `make-doom-for-mips` |

The job log's first explicit `no space left on device` occurs near the start of
the run and the condition recurs throughout image pulls, layer registration,
container startup, package installation, and upload of the Pragma binary. The
job nevertheless continued. Under the repository's current methodology, the
batch should have stopped on the first infrastructure failure.

## Why the run looked catastrophically bad

### Evaluation operation was the dominant failure

- The host was not preflighted against task CPU requirements.
- Docker storage was not provisioned or monitored for a full 89-image run.
- Verifiers depended on live package and Python downloads, adding network and
  disk failure modes after agent execution.
- The batch did not stop after the first systemic disk failure.
- Harbor's aggregate allowed verifier-bootstrap zeros and infrastructure
  exceptions to collapse into an apparent 4/89 model score.
- The run record was never reconciled after completion; the ledger ends with
  the precommitted run plan.

These are benchmark-operation defects. They explain most of the apparent
failure and prevent a full-set conclusion about Pragma.

### The simple loop also showed real weaknesses

On the nine clean failures, Pragma had no independent completion gate beyond
the model's `end_turn` or the hard 100-turn ceiling. It did not invoke personas
or a review pass. This left two obvious patterns uncorrected: repeated tool use
until budget exhaustion, and confident completion after partial self-testing.

The final bundle also intentionally omitted previously tested Lilac changes
for reasoning retention, exact-model registry metadata, and continuation after
reasoning-only truncation. Those mechanisms were reverted because a frozen
15-task comparison tied the starting product 8/15 to 8/15. Therefore it is
accurate to say the historical run used a minimal loop with known protocol
limitations; it is not accurate to claim that simply restoring those changes
would improve the benchmark.

### This was not the model now under discussion

The run used GLM-5.2. It provides no direct estimate for GLM-5.3, whose
Terminal-Bench 4.0 result prompted this audit. The active repository has since
removed Lilac, standardized on OpenRouter, and repaired an independently
recorded OpenRouter reasoning-field loss. Those changes make the current
product materially different from the historical binary, but no broad
GLM-5.3 score uplift has been demonstrated.

## Defensible conclusion

The evidence does **not** show that Pragma scored 4.49% on Terminal-Bench 2.1,
nor that Pragma is generally useless. It shows:

- the full evaluation was operationally invalid;
- Pragma + GLM-5.2 solved four of the thirteen tasks with clean verifiers;
- on nine clean failures, the simple loop either exhausted its turn budget or
  accepted incomplete work as done; and
- no valid full-set run was completed for GLM-5.2 or GLM-5.3.

## Minimum credible next evaluation

Before any TB4 score attempt:

1. Pin the TB4 dataset and verifier revision.
2. Preflight the oracle in the target environment, including every declared CPU,
   memory, disk, architecture, and dependency requirement.
3. Cache task images and verifier dependencies and record available Docker disk.
4. Run a small heterogeneous smoke panel and require clean verifier completion.
5. Stop the batch immediately on a repeated infrastructure failure.
6. Run one frozen 66-task GLM-5.3 pass with unchanged settings and classify
   passes, verified failures, timeouts, provider failures, and infrastructure
   failures separately.
7. Only fund repeated trials after that single pass is healthy and competitive.

No score projection is justified from the 2026-08-30 run.

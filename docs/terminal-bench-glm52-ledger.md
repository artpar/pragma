# GLM-5.2 Terminal-Bench 2.1 experiment ledger

This ledger starts a new, user-authorized model regime. GLM-5.3 measurements in
`terminal-bench-glm53-ledger.md` are historical and are not comparable evidence.
Official Terminal-Bench verifier rewards remain the primary outcome.

## Fixed evaluation configuration

- Provider: Lilac Chat Completions (`https://api.getlilac.com/v1`)
- Model identifier: `zai-org/glm-5.2`
- Reasoning mode: no explicit Pragma thinking/reasoning parameter
- Temperature: `0`
- Other sampling parameters: provider defaults
- Advertised context limit: 524,288 tokens
- Maximum output: 16,384 tokens
- Tool protocol: provider-native tool calls; Pragma `Bash` and `apply_patch`
- Pragma product path: ordinary non-interactive CLI with `--loop provider-tools`
- Turn limit: 100 model turns
- Permission mode: `bypassPermissions` inside the isolated task container
- Development task timeout: 1,800 seconds active agent execution
- Benchmark: unchanged local `terminal-bench/terminal-bench-2-1` manifest,
  89 tasks, with official per-task verifiers
- Harbor: 0.22.0; Docker environment; verifier timeouts unchanged
- Concurrency: one trial at a time until provider behavior is established

## Starting point

- `GLM52_START_BASELINE`: `3804be0581de202606f60d729aacc65d572559cd`;
  product-equivalent to `START_BASELINE` and tagged before GLM-5.2
  evaluator/configuration changes.
- Evaluation bundle: existing product-equivalent `START_BASELINE` bundle;
  Linux amd64 Pragma SHA-256
  `a67186ddb18989c4685f04e4cac64ac2c10fb253aed6a47777d198dca0ecbb6c`.
- Prior GLM-5.3 candidates E001 and E002 are provisional and absent from this
  product baseline.
- All GLM-5.2 baseline and candidate comparisons must use this fixed regime.

## Baseline diagnostic panel D01 (precommitted)

The existing diverse six-task rotation is retained without looking at GLM-5.2
results:

1. `fix-git`
2. `regex-log`
3. `cancel-async-tasks`
4. `sqlite-db-truncate`
5. `build-cython-ext`
6. `path-tracing`

The first execution is `regex-log`, selected before any GLM-5.2 trajectory was
observed. Classify its earliest failure layer before proposing a product change.
Results on this diagnostic panel establish baseline behavior but do not alone
promote a candidate designed from the same tasks.

Pre-result configuration calibration:

- The first `regex-log` launch used the inherited 4,096-token OpenRouter limit.
  Lilac returned one response with exactly 4,096 completion tokens,
  `finish_reason: length`, and no content or tool calls; the official verifier
  failed because `/app/regex.txt` did not exist.
- A direct protocol probe confirmed that GLM-5.2 emits its internal reasoning in
  Lilac's `message.reasoning` field and can exhaust a small completion budget
  before producing content or an action. The API advertises up to 524,288
  completion tokens for this model. The 4,096 limit was a constraint inherited
  from the depleted OpenRouter account, not a deliberate GLM-5.2 inference
  setting.
- Before any candidate exists, the fixed GLM-5.2 maximum output is therefore
  calibrated once to 16,384. The 4,096 run is retained as configuration evidence
  but is not comparable baseline task evidence. All subsequent baseline and
  candidate runs use 16,384.

Baseline results:

- `regex-log`: reward 1.0, no exception, official verifier, 7m19s wall clock.
  The agent made 14 model requests (13 tool-use, one end-turn), created the
  deliverable, identified and repaired an IPv4-pattern error, and used 39,019
  output tokens. This is the first comparable GLM-5.2 baseline result.
- `fix-git`: reward 1.0, no exception, official verifier, 55s wall clock.
- `cancel-async-tasks`: reward 0.0, no exception, official verifier, 1m28s
  wall clock. Five of six verifier tests passed. In the queued-task SIGINT
  case, two jobs started but neither printed its cleanup marker before process
  exit. Pragma delivered the task, ten valid model responses, tool results, and
  enough time. The agent wrote a weaker ad-hoc SIGINT check that slept after
  catching cancellation, accepted that result, and ended normally. Earliest
  useful cause: **C, likely model capability failure**; no observed harness
  affordance was missing.
- `sqlite-db-truncate`: reward 1.0, no exception, official verifier, 1m51s wall
  clock.
- `build-cython-ext`: reward 1.0, no exception, official verifier, 6m51s wall
  clock.
- `path-tracing`: reward 0.0, no exception, official verifier, 27m32s wall
  clock. The agent used all 100 configured model turns, completed `image.c`,
  compiled and ran it, and measured 0.9985 cosine similarity locally. The
  official verifier passed file existence, compilation, and no-dependency
  checks, then its chrooted static x86-64 executable failed before user code ran
  with Rosetta's `Unable to open /proc/self/exe: 2`; consequently no verifier
  image existed. Record the official zero unchanged. Earliest useful cause:
  **D, benchmark/environment incompatibility** on the ARM Mac's emulated
  linux/amd64 Docker runtime, not a Pragma or model failure.

D01 aggregate: four official passes and two official zeros (4/6). One zero is
an ordinary task failure (`cancel-async-tasks`); one is separately classified as
an environment/verifier execution failure (`path-tracing`). All six trials
reached the official verifier and none had a Harbor or provider exception.

## E001 — preserve Lilac reasoning across tool turns (precommitted)

- Parent product champion: `GLM52_START_BASELINE`
  (`3804be0581de202606f60d729aacc65d572559cd`); evaluation-only commits through
  `8d0d13b` do not change the bundled product.
- Observed failure: a live GLM-5.2 response contained a non-empty string in
  Lilac's `message.reasoning`. `completionMessage` decoded it successfully, but
  `completionResponse.toAnyLLM` copied only role, content, and tool calls. The
  shared converter therefore could not emit a `model.ThinkingPart`, and later
  provider-tools requests omitted that portion of the assistant turn. This is
  deterministic category **A** response/context loss.
- Hypothesis: copying the already-decoded reasoning into the any-llm message
  will preserve it as a thinking part and in subsequent provider turns, without
  changing ordinary text or tool-call parsing.
- Smallest mechanism: assign only the decoded reasoning field during Lilac
  completion conversion and add a focused conversion test.
- Diagnostic: deterministic unit test plus an exact-model normal-path smoke
  whose recording must contain a thinking part.
- Frozen controls: `cancel-async-tasks` (known model-error diagnostic),
  `fix-git` and `regex-log` (previously solved unrelated controls), and
  `adaptive-rejection-sampler` (the lexicographically first previously unseen
  task, selected before its result). Obtain a comparable START_BASELINE result
  for the new task before using its candidate result.
- Reject if the reasoning still disappears, deterministic tests fail, or the
  candidate causes a repeatable verified regression on a previously solved
  control. A changed-looking trajectory alone is not promotion evidence.
- Candidate commit: `90d90c5` (`Preserve Lilac reasoning across tool turns`).
  The product change is one field assignment; its focused existing provider
  integration test now asserts both the thinking and text parts. `go test
  ./...` passes. Linux amd64 candidate bundle SHA-256:
  `0db4ddc1d1e5366a0030a24100e7c62f147756099850b33a787b7b5c01e5928c`.
- Mechanical diagnostic: exact-model normal CLI smoke completed
  `E001_REASONING_OK`. The first recorded response contained an 83-character
  thinking part plus a tool call; the next recorded request contained that
  thinking part and tool call in its assistant message. This directly confirms
  the predicted response and continuation-context change.
- Frozen-control results at the candidate:
  - `cancel-async-tasks`: reward 1.0, no exception, official verifier, 1m19s
    wall clock (baseline 0.0). Eight model responses, 2,807 output tokens.
  - `fix-git`: reward 1.0, no exception, official verifier, 1m09s wall clock
    (baseline 1.0). Sixteen model responses, 2,002 output tokens.
  - `regex-log`: reward 1.0, no exception, official verifier, 4m58s wall clock
    (baseline 1.0). Twelve model responses, 21,857 output tokens; its first
    response included 12,655 tokens and the preserved thinking part.
  - `adaptive-rejection-sampler`: candidate reward 0.0, no exception, official
    verifier, 10m59s. Two identical 150-second provider timeouts were retried;
    the third response used all 16,384 output tokens as reasoning and ended at
    `max_tokens` without creating `ars.R`. START_BASELINE was stopped without a
    verifier result after three identical timeouts on the same fourth request,
    so this task is not a comparable E001 outcome and provides no promotion
    credit or regression evidence. It does expose a separate completion-budget
    limitation for later investigation.
- Decision: **ACCEPTED** as the new product champion on mechanically strong
  harness evidence, with supportive but not independently conclusive benchmark
  evidence. E001 deterministically stops Lilac response/context loss, preserves
  both previously solved controls, and changed the diagnostic failure to an
  official pass. The single-run diagnostic gain is not by itself a claim that
  Pragma solves materially more tasks; later unseen and broad checkpoints must
  confirm cumulative task-success value.

## Development regime v2 — completion-budget recalibration

`adaptive-rejection-sampler` demonstrated that the 16,384-token setting can be
fully consumed by GLM-5.2 reasoning before any content or action. Beginning with
E002, the fixed maximum output is 32,768 tokens. This is a transparent inference
configuration recalibration, not a Pragma improvement. Results from v1 remain
valid historical/mechanical evidence but are not score-comparable with v2.
Provider, model, temperature, tool protocol, turn limit, task timeout, and all
other fixed settings remain unchanged.

## E002 — allow slow Lilac reasoning responses to finish (precommitted)

- Parent champion: E001 product commit `90d90c5`.
- Observed failure: after R installation on `adaptive-rejection-sampler`, the
  same fourth model request hit Lilac's hard-coded 150-second client deadline
  repeatedly. At the candidate-independent v1 run, two attempts timed out and a
  third response arrived just under the deadline after roughly 148 seconds.
  Rejected attempts discard all in-flight model generation and restart the
  identical request. This is category **A** premature response termination.
- Hypothesis: raising the bounded Lilac request deadline to 360 seconds will let
  legitimately slow GLM-5.2 reasoning responses complete once rather than be
  repeatedly discarded, while the 30-minute task timeout still bounds total
  execution.
- Smallest mechanism: change only Lilac's per-request timeout constant. Do not
  alter retry count, prompts, tools, or agent policy.
- Diagnostic: `adaptive-rejection-sampler` under v2. The expected observable
  change is a response completing after 150 seconds without an intervening
  timeout/retry and producing a tool action or final content.
- Frozen controls: `regex-log` and `cancel-async-tasks` from prior panels, plus
  `bn-fit-modify`, the next lexicographically selected previously unseen task.
  Obtain parent-champion v2 results for the diagnostic and new task before using
  their candidate results.
- Reject if slow responses still fail to complete, the response completes but
  again contains no action at 32,768 tokens, or a previously solved control has
  a repeatable verified regression. Latency alone is not promotion evidence.
- Candidate commit: `5f469c1` (`Allow slow Lilac reasoning responses`). The
  product change raised only Lilac's request timeout from 150 to 360 seconds and
  added a focused timeout test. `go test ./...` passed. Linux amd64 candidate
  bundle SHA-256:
  `2026d77e56f81f17973b12c91a96b0466d84da9d9837303edc62622cab544520`.
- Parent-champion v2 calibration:
  - `bn-fit-modify`: reward 1.0, no exception, official verifier, 3m56s wall
    clock; all 9 verifier tests passed.
  - `adaptive-rejection-sampler`: reward 0.0, `AgentTimeoutError`, official
    verifier, 30m41s wall clock. After the setup tool calls, the identical
    fourth request hit the 150-second deadline ten times and never admitted a
    response before Harbor's 1,800-second agent timeout.
- Candidate v2 results:
  - `adaptive-rejection-sampler`: reward 0.0, no exception, official verifier,
    7m56s wall clock. The decisive fourth request crossed the old 150-second
    boundary and completed once after 4m57s, mechanically confirming the
    timeout change. It then used all 32,768 output tokens, stopped at
    `max_tokens`, emitted no action, and did not create `ars.R`.
  - `bn-fit-modify`: reward 1.0, no exception, official verifier, 2m51s wall
    clock.
  - `regex-log`: reward 1.0, no exception, official verifier, 4m40s wall clock.
  - `cancel-async-tasks`: reward 0.0, no exception, official verifier, 1m49s
    wall clock. Three of six verifier tests passed; the three cancellation cases
    printed only one of two required cleanup markers. Its prior pass is in the
    non-comparable v1 inference regime, so this is not called a repeatable v2
    regression.
- Decision: **REJECTED**. The candidate fixed the narrow transport symptom but
  hit the precommitted rejection condition: the newly admitted diagnostic
  response exhausted the doubled completion budget without content or a tool
  action, so it delivered no task-success value. Commit `921adc1` reverts only
  the E002 product change; E001 remains the accepted champion.

## E003 — ask Lilac to preserve GLM 5.2 thinking (precommitted)

- Parent champion: E001 product commit `90d90c5`, with rejected E002 reverted by
  `921adc1`.
- Observed failure: E001 now retains GLM 5.2 reasoning in Pragma's assistant
  messages, but Lilac's current official model documentation says GLM 5.2 clears
  previous assistant thinking blocks by default. The documented opt-in for
  preserved thinking is `chat_template_kwargs.clear_thinking: false`; Pragma's
  Lilac request type cannot currently send that field. This is category **A**
  continuation-context loss at the provider request boundary.
- Hypothesis: sending only the documented `clear_thinking: false` chat-template
  option for exact `zai-org/glm-5.2` requests will allow the provider to use the
  thinking E001 already preserves, improving multi-turn tool trajectories
  without changing prompts, tools, reasoning effort, or sampling.
- Smallest mechanism: add the typed optional chat-template request field and set
  `clear_thinking` to false only for exact GLM 5.2 requests.
- Mechanical diagnostic: a direct-request test must capture the JSON body and
  prove the exact model sends `{"chat_template_kwargs":{"clear_thinking":false}}`
  while another Lilac model does not.
- Frozen benchmark controls under v2: `break-filter-js-from-html`, the next
  lexicographically selected previously unseen task; `cancel-async-tasks`, the
  unstable cancellation diagnostic; and solved `regex-log`. Obtain parent
  results for all three before using candidate results.
- Reject if the request field is absent or leaks to another model, tests fail,
  or a previously solved control has a repeatable verified regression. Require
  official verifier outcomes; token or trajectory differences alone do not
  establish task-success value.
- Candidate commit: `ed63e44` (`Preserve GLM-5.2 thinking in Lilac requests`).
  The exact model gets only
  `chat_template_kwargs: {"clear_thinking": false}`; the focused captured-body
  tests prove the field and value for GLM 5.2 and its absence for MiniMax M2.7.
  `go test ./...` passes. Linux amd64 candidate binary SHA-256:
  `2483b6a7f33a3c8282f63369d113f3b1abd3eedb0ee5e538200af5b5c9e45c21`;
  the bundle reuses the pinned CA file with SHA-256
  `bc363a289a53946a9e18092dc1f2f8cfabdc9d293f49bb1fd10f4c8d55f1c214`.
- Invalid infrastructure attempt: job
  `pragma-lilac-glm52-e003-controls-v2` omitted the required CA file and all
  three trials stopped in setup with `FileNotFoundError`. It made no model
  request and is excluded from every comparison. The repaired, distinctly
  named job is `pragma-lilac-glm52-e003-controls-rerun-v2`.
- Parent-champion v2 results:
  - `break-filter-js-from-html`: no reward and no verifier result, 5m54s wall
    clock. Pragma used all 100 turns and exited nonzero; the Harbor adapter
    raised `RuntimeError`, so Harbor skipped the verifier. Preserve this as an
    exception, not a zero score.
  - `cancel-async-tasks`: reward 1.0, no exception, official verifier, 2m38s
    wall clock and 8 model responses.
  - `regex-log`: reward 1.0, no exception, official verifier, 4m44s wall clock
    and 7 model responses.
- Candidate v2 results:
  - `break-filter-js-from-html`: the same no-reward `RuntimeError` after all 100
    turns, 9m29s wall clock. It again had no verifier result, so slower turn
    churn is not promotion evidence.
  - `cancel-async-tasks`: reward 0.0, no exception, official verifier, 2m16s
    wall clock and 22 model responses. Three of six tests passed; each
    cancellation case emitted only one of two cleanup markers. This is a paired
    v2 loss from the parent's official pass.
  - `regex-log`: reward 1.0, no exception, official verifier, 6m50s wall clock
    and 10 model responses. Its first request crossed the unchanged 150-second
    deadline and was discarded once before a retry completed just under that
    boundary.
- Decision: **REJECTED**. The request field is documented and mechanically
  correct, but it did not improve the unseen diagnostic, lost the paired
  cancellation control, and introduced a timeout/retry on the other solved
  control. Commit `568b607` reverts only E003's product change; E001 remains the
  accepted champion. The separate 100-turn adapter/verifier gap is retained for
  a future isolated experiment.

## E004 — verify filesystem work after the provider-tools turn cap (precommitted)

- Parent product champion: E001 product commit `90d90c5`; this experiment
  changes only the Terminal-Bench adapter, not the shipped Pragma binary.
- Observed failure: both E003 arms of `break-filter-js-from-html` executed 100
  valid model/tool turns and left their filesystem work in the task container.
  Pragma then exited 1 with the exact terminal message `provider tools loop
  exceeded maximum of 100 turns`. The adapter raised `RuntimeError`, causing
  Harbor to skip the unchanged official verifier. No score exists for either
  trial even though Terminal-Bench tasks are judged from filesystem state.
- Hypothesis: treating only this explicit exhausted-turn terminal as a completed
  agent phase will let Harbor run the official verifier and produce the task's
  legitimate score. Preserve return code 1 in trial metadata; all other nonzero
  exits must still raise.
- Smallest mechanism: add a strict terminal-message predicate in
  `tools/harbor_pragma_agent.py` and suppress its exception only for that case.
- Mechanical diagnostic: focused adapter tests must prove the exact turn-limit
  output is allowed and arbitrary nonzero failures remain fatal.
- Benchmark diagnostic: rerun parent-champion `break-filter-js-from-html` under
  v2 and require an official verifier result. Frozen control: solved
  `fix-git`, which must still run and verify normally.
- Reject if the turn-limit trial still lacks an official verifier, another
  nonzero exit is swallowed, or the normal control does not reach its official
  verifier. This experiment may improve measurement coverage but receives task
  success credit only from the unchanged official verifier.
- Candidate commits: `d446828` (`Run verifiers after provider-tools turn
  exhaustion`) and test-strengthening commit `504d54d` (`Test adapter exit
  classification`). The anchored predicate accepts only the exact terminal
  provider-tools turn-limit message. Four focused tests prove that exit 1 with
  that message is nonfatal, arbitrary exit 1 remains fatal, and exit 0 remains
  nonfatal. The adapter preserves Pragma's actual return code in Harbor
  metadata.
- Candidate job `pragma-lilac-glm52-e004-turn-cap-v2`:
  - `break-filter-js-from-html`: reward 1.0, no exception, official verifier,
    4m38s wall clock and 49 model responses. This stochastic rerun ended
    normally with Pragma return code 0, so it confirms official task success and
    broadens champion evidence but does not exercise or earn causal credit for
    the turn-cap exception branch.
  - `fix-git`: reward 1.0, no exception, official verifier, 43s wall clock.
- Decision: **ACCEPTED as evaluation infrastructure**, not as a product-score
  improvement. The deterministic classification closes the observed path that
  skipped verifiers after preserved filesystem work, while keeping unrelated
  failures fatal. Because the benchmark diagnostic happened to finish before
  the cap, no task-success delta is attributed to E004. E001 remains the product
  champion; E004 is the active Terminal-Bench adapter.

## E005 — register exact Lilac GLM 5.2 metadata (precommitted)

- Parent product champion: E001 product commit `90d90c5`; accepted E004 changes
  only evaluation infrastructure.
- Observed failure: every exact-model run warns that `zai-org/glm-5.2` is
  unknown, reports total cost as zero, and falls back to the generic 200K
  context assumption. Lilac's official model catalog publishes the exact ID,
  524,288-token context, text-only input, tool use, reasoning, and prices of
  $0.90/M input, $0.17/M cache read, and $3.00/M output. The provider model API
  advertises the same 524,288 maximum completion limit.
- Hypothesis: adding only exact registry metadata will eliminate warnings,
  provide accurate context/cost accounting, and avoid premature generic-context
  compaction without changing the fixed benchmark request parameters.
- Smallest mechanism: one `ModelInfo` entry plus exact registry/pricing tests.
  Do not change aliases, defaults, prompts, tools, or inference controls.
- Mechanical diagnostic: model lookup/listing, capabilities, context/output
  limits, and all three price fields must match the published values. A normal
  exact-model run must no longer print unknown-model or missing-pricing warnings
  and must report nonzero cost.
- Frozen benchmark controls under v2: solved `fix-git` plus
  `build-pmars`, the next lexicographically selected previously unseen task.
  These guard integration and broaden evidence; because explicit max output is
  fixed at 32,768 and task prompts are far below the new context boundary, no
  score delta is attributed to metadata alone.
- Reject if metadata is wrong, warnings/cost remain broken, tests fail, or a
  solved control has a verified regression.
- Candidate commit: `7a4a82a` (`Register Lilac GLM-5.2 metadata`). The change is
  one exact registry entry plus focused assertions covering lookup, listing,
  limits, pricing, and capabilities. `go test ./...` passes. Linux amd64 binary
  SHA-256:
  `90188f2cc29dc84df89a7e4057890653c0853a0f12dcdb60860cf31a7792d9e4`;
  pinned CA SHA-256:
  `bc363a289a53946a9e18092dc1f2f8cfabdc9d293f49bb1fd10f4c8d55f1c214`.
- Live mechanical diagnostic: the exact-model `fix-git` run printed neither the
  unknown-model warning nor the missing-pricing warning and reported a nonzero
  total cost of `$0.014065` instead of `$0.000000`.
- Candidate v2 controls:
  - `fix-git`: reward 1.0, no exception, official verifier, 48s wall clock.
  - `build-pmars`: reward 1.0, no exception, official verifier, 2m14s wall
    clock; all 4 verifier tests passed. This is a previously unseen task and
    broadens champion evidence, but no causal score gain is attributed to
    metadata whose inference request remains fixed.
- Decision: **ACCEPTED** as the new product champion. The deterministic registry
  defects are closed, normal-path warnings and cost accounting are corrected,
  and both integration controls pass. Product champion is `7a4a82a`; accepted
  E004 remains the active evaluation adapter.

## B01 — six-task unseen broad checkpoint under v2 (precommitted)

- Objective: compare the current champion against `GLM52_START_BASELINE` on a
  broader, untouched panel under the same GLM 5.2 v2 inference regime. This is
  the first aggregate product comparison after the fixed-model switch.
- Selection: the next six lexicographically ordered tasks not previously used in
  GLM 5.2 development: `build-pov-ray`, `caffe-cifar-10`, `chess-best-move`,
  `circuit-fibsqrt`, `cobol-modernization`, and `code-from-image`. Selection was
  frozen before any result.
- Arms: START binary SHA-256
  `a67186ddb18989c4685f04e4cac64ac2c10fb253aed6a47777d198dca0ecbb6c`
  versus champion E005 binary SHA-256
  `90188f2cc29dc84df89a7e4057890653c0853a0f12dcdb60860cf31a7792d9e4`.
  Both use the accepted E004 adapter, exact `zai-org/glm-5.2`, max output
  32,768, temperature 0, provider tools, 100 turns, 1,800-second task timeout,
  serial execution, and unchanged official verifiers.
- Run START_BASELINE first. Preserve official rewards and exceptions unchanged;
  classify environment/verifier incompatibilities separately. Do not use
  trajectory aesthetics or partial local tests as score substitutions.
- Interpret aggregate paired official outcomes, task-level flips, exceptions,
  and verifier compatibility. A single stochastic flip is not a broad success
  claim; materially better reliability requires a directional panel result with
  no systematic new failure mode.

### B01 observed outcome (censored by user-directed timeout change)

- START completed all six entries in 1h05m21s. Five tasks were scoreable and
  all received official reward 0.0. `caffe-cifar-10` failed environment setup
  before agent execution because its Docker CPU request exceeded the two-CPU
  runtime; exclude it from both arms. `circuit-fibsqrt` and `code-from-image`
  each reached exactly 100 turns with return code 1, and the accepted E004
  adapter correctly allowed both official verifiers to run.
- Champion completed `build-pov-ray` at reward 0.0 after an 1,800-second
  `AgentTimeoutError`; its verifier ran but passed zero of three assertions,
  compared with two of three for START. The registry fix remained mechanically
  active: no unknown-model or missing-pricing warnings appeared.
- Champion reproduced the pre-agent `caffe-cifar-10` CPU incompatibility.
  `chess-best-move` recovered after three consecutive 150-second provider
  request timeouts, reached exactly 100 turns, and received official reward 0.0
  with no adapter exception, providing another live E004 validation.
- On `circuit-fibsqrt`, one request produced six consecutive 150-second provider
  timeouts after only two completed responses. At the user's explicit request
  to increase the timeout, B01 was stopped cleanly after 1h14m; circuit was
  recorded canceled and the final two champion trials were not started.
- Decision: **CENSORED / INCONCLUSIVE**, not an accepted benchmark comparison.
  The completed paired official rewards tie, while champion subtest and timeout
  evidence is worse. Do not impute outcomes for the unrun tasks or use B01 as
  evidence that E005 improves task-solving quality. Retain its live registry and
  adapter validation plus the sustained provider-timeout diagnostic.

## E006 — extend Lilac reasoning-request timeout (precommitted)

- Parent product champion: E005 product commit `7a4a82a`; accepted E004 remains
  the evaluation adapter.
- User direction: increase the timeout. Observed trigger: during the B01
  champion window, `chess-best-move` suffered three consecutive 150-second
  request timeouts before recovering, and `circuit-fibsqrt` suffered six
  consecutive timeouts on one request without recovery before B01 was stopped.
- Hypothesis: increasing only Lilac's per-request deadline from 150 to 360
  seconds will allow slow GLM 5.2 reasoning responses to complete and reduce
  retry storms, while retaining the existing retry policy and all frozen model,
  prompt, tool, token, turn, task-timeout, and verifier settings.
- Smallest mechanism: change `lilacRequestTimeout` to 360 seconds and add a
  focused source-level regression test for that configured constant. This
  intentionally revisits rejected E002 because the user explicitly requested
  it after new sustained-timeout evidence; prior E002 evidence remains valid
  and is not overwritten.
- Mechanical gate: focused Lilac tests and `go test ./...` must pass; the rebuilt
  exact-model binary must retain E005 registry behavior.
- Benchmark diagnostic: first run an isolated task from the censored failure
  path under the new binary and require at least one response that exceeds 150
  seconds without a client timeout, or completion without the prior retry
  storm. Official verifier reward remains the only task-success measure.
- Reject as a performance improvement if slow requests merely consume more of
  the fixed 1,800-second task budget, exhaust 32,768 output tokens without an
  action, or introduce a verified regression. Provider-reliability evidence and
  task-solving evidence must be reported separately.

### E006 observed outcome

- Product commit `6424eb4` changes only `lilacRequestTimeout` from 150 to 360
  seconds and adds the focused constant regression test. `go test ./...`
  passes. Linux amd64 binary SHA-256:
  `d57beb3c445044515a80a3e5c1c77ff23ccddee5edc4d07abd75367b88563691`;
  pinned CA SHA-256 remains
  `bc363a289a53946a9e18092dc1f2f8cfabdc9d293f49bb1fd10f4c8d55f1c214`.
- Isolated job `pragma-lilac-glm52-e006-timeout-v2` reran the censored
  `circuit-fibsqrt` path. After two quick tool-use responses, the third request
  remained in flight beyond the old 150-second boundary and returned before
  360 seconds with zero timeout/retry events. Agent execution lasted 309.5
  seconds, returned code 0, retained exact-model pricing with total cost
  `$0.105950`, and reached the unchanged official verifier.
- The third response stopped at the fixed 32,768-token maximum. Official reward
  remained 0.0: file existence and size passed, while functional correctness
  failed. Thus the larger timeout converts this observed retry storm into one
  completed response, but that response does not improve task success and
  illustrates the previously observed cost/latency tradeoff.
- Decision: **RETAINED by explicit user direction as a provider-reliability
  policy, not accepted as a benchmark hill-climb gain**. E005 remains the last
  task-performance champion by evidence; the current product additionally
  carries E006's 360-second timeout. Do not claim reward improvement from E006.

## E007 — continue reasoning-only max-token responses (precommitted)

- Parent product state: commit `fc34636`, comprising E005 plus the explicitly
  retained E006 timeout; accepted E004 remains the evaluation adapter.
- Observed failure: E002 `adaptive-rejection-sampler` and E006
  `circuit-fibsqrt` each returned `stop=max_tokens` after consuming the fixed
  32,768-token output budget without a tool call or final answer. The provider
  tools loop appended the partial assistant reasoning, emitted
  `TurnCompleteEvent`, and Pragma exited 0 despite unfinished task work.
- Hypothesis: when and only when a response stops at `max_tokens` with no tool
  call, preserving that assistant content and appending a neutral user
  continuation marker will let the same GLM 5.2 session resume active work
  instead of falsely terminating. The existing 100-turn bound prevents an
  infinite continuation loop.
- Smallest mechanism: special-case `StopMaxTokens` in the provider-tools loop;
  append `Continue from the truncated response.` as a user message and advance
  to the next turn. Do not change prompts, tools, model settings, output budget,
  ordinary end-turn handling, complete tool-call handling, or evaluator logic.
- Deterministic gate: a scripted provider must prove that a reasoning-only
  max-token response is preserved into the next request, the neutral marker is
  appended, subsequent tool use executes, and a normal final response still
  terminates. Existing provider-tools tests and `go test ./...` must pass.
- Diagnostics: rerun `circuit-fibsqrt` and
  `adaptive-rejection-sampler`, the two observed max-token failures. Require at
  minimum a subsequent model request after the max-token response; official
  verifier reward is the only task-success signal.
- Frozen promotion controls: solved `fix-git` plus `compile-compcert`, the next
  lexicographically selected previously unseen task. Obtain a current-parent
  result for the new control before candidate evaluation and a START result when
  practical. Reject if the predicted continuation does not occur, ordinary
  final completion regresses, the candidate loses a verified parent control,
  or continued requests merely add cost/latency without credible completion
  evidence. A diagnostic flip alone remains insufficient broad evidence.

### E007 observed outcome

- Candidate commit `d52a741` special-cases only reasoning/text responses with
  `StopMaxTokens` and no tool call. It preserves the assistant content, appends
  the neutral continuation marker, and advances within the existing turn cap.
  A focused scripted-provider test proves preservation, request sequencing,
  subsequent Bash execution, and ordinary final completion. `go test ./...`
  passes. Linux amd64 binary SHA-256:
  `69f06d040d466242ba6b1bed709f45cd8d43f05df4c1ea0abf6bdb4c59b7fd0d`.
- Parent controls under the 360-second regime: `fix-git` reward 1.0;
  `compile-compcert` reward 0.0 after `AgentTimeoutError`, with the official
  verifier unable to find `/tmp/CompCert/ccomp`.
- Candidate diagnostics:
  - `circuit-fibsqrt`: five `max_tokens` responses and four tool-use responses.
    Each truncation caused another model request, proving the live product path
    no longer exits falsely after the first truncation. The task exhausted its
    1,800-second budget and remained reward 0.0 (file/size checks passed,
    functional correctness failed).
  - `adaptive-rejection-sampler`: reward 1.0 with all nine verifier tests
    passing. This trajectory never returned `max_tokens`, so the pass broadens
    candidate evidence but receives no causal E007 credit.
- Candidate controls: `fix-git` retained reward 1.0. The first
  `compile-compcert` attempt was excluded after a host/network outage produced
  repeated DNS failures and no verifier over a 23-hour suspended interval. A
  clean frozen-settings rerun had zero API failures, never activated E007, and
  tied the parent at reward 0.0 after `AgentTimeoutError`.
- Decision: **ACCEPTED as a deterministic harness fix, not as demonstrated
  task-success improvement**. E007 removes a repeated false-success terminal
  and preserves active model work; frozen controls show no verified regression.
  Repeated 32,768-token continuations can consume the task window, and the one
  branch-exercising diagnostic did not improve reward. Require a broad
  checkpoint before treating the cumulative product as materially stronger.

## B02 — fifteen-task unseen broad checkpoint (precommitted)

- Trigger: four retained product mechanisms (E001, E005, user-directed E006,
  and E007) plus a censored six-task B01. Per the broad-checkpoint cadence,
  local mechanical evidence is no longer sufficient to support the cumulative
  hill.
- Selection: the next fifteen lexicographically ordered Terminal-Bench 2.1
  tasks never used in GLM 5.2 development: `configure-git-webserver`,
  `constraints-scheduling`, `count-dataset-tokens`, `crack-7z-hash`,
  `custom-memory-heap-crash`, `db-wal-recovery`, `distribution-search`,
  `dna-assembly`, `dna-insert`, `extract-elf`, `extract-moves-from-video`,
  `feal-differential-cryptanalysis`, `feal-linear-cryptanalysis`,
  `filter-js-from-html`, and `financial-document-processor`. Selection is
  frozen before any arm result.
- Arms: START binary SHA-256
  `a67186ddb18989c4685f04e4cac64ac2c10fb253aed6a47777d198dca0ecbb6c`
  versus cumulative E007 binary SHA-256
  `69f06d040d466242ba6b1bed709f45cd8d43f05df4c1ea0abf6bdb4c59b7fd0d`.
  Run START first and candidate second.
- Fixed evaluator regime: exact Lilac `zai-org/glm-5.2`, temperature 0,
  provider-tools loop, max output 32,768, max turns 100, bypass permissions,
  1,800-second task timeout, serial concurrency 1, accepted E004 adapter, and
  unchanged official tasks/verifiers. Product-internal timeout/continuation
  behavior is intentionally part of the compared product state.
- Record every task as official pass, official fail, task timeout,
  provider/infrastructure failure, or benchmark/environment failure. Do not
  silently drop incompatible or inconvenient tasks; exclude only from paired
  score inference with explicit classification.
- Decision gate: require a credible directional improvement in paired verified
  task completion without a systematic new failure mode. Rerun important
  discordant cases when stochastic noise could explain a small delta. If broad
  performance is flat or worse, stop trusting local gains and return to the
  strongest empirically supported product commit rather than rationalizing the
  discrepancy.

### B02 discordant-pair rerun (precommitted)

- Initial outcome: START recorded official passes on
  `configure-git-webserver` and `count-dataset-tokens`; cumulative E007 recorded
  official failures on both. No task flipped from a scoreable START failure to
  a champion pass. Several later trials were separately censored after Docker
  storage exhaustion and do not affect selection of these two reruns.
- Rerun exactly the two pass-to-fail discordances, START first and cumulative
  E007 second, preserving the B02 model, inference, timeout, concurrency, task,
  and verifier settings. Docker capacity cleanup is limited to stopped Harbor
  containers and recreatable Terminal-Bench image cache; unrelated containers
  and volumes remain untouched.
- Decision rule: if the cumulative arm does not recover both START passes, the
  broad checkpoint remains flat or worse and the cumulative product hill is
  rejected. Restore the strongest broadly supported product state while
  retaining evaluation records and adapter fixes.

### B02 observed outcome and rollback

> **Correction after verifier-log audit:** the outcome below was recorded
> before every verifier bootstrap log was inspected. Docker VM storage
> exhaustion caused package-install, image-extraction, `curl`, and `uvx`
> failures that Harbor frequently serialized as reward 0.0 rather than as an
> exception. Consequently the stated 10-task scoreable comparison and its
> rollback decision are withdrawn as a provisional conclusion, not erased from
> the experiment history. The configure/count rerun also had a network timeout
> in both configure trials; only count produced a clean paired verifier result
> (0.0 versus 0.0). After targeted Docker cleanup, rerun both arms on the union
> of contaminated tasks: all B02 tasks except `count-dataset-tokens`, preserving
> arm order and all frozen settings. Treat any renewed bootstrap failure as
> censored and base the final broad decision only on clean paired official
> verifier outcomes.

- Initial START arm: 15 trials in 2h18m. Official rewards were 2 passes
  (`configure-git-webserver`, `count-dataset-tokens`) and 8 failures, including
  an `AgentTimeoutError` on `crack-7z-hash`. Five additional tasks were censored
  for benchmark/environment failures: `extract-elf`,
  `extract-moves-from-video`, `feal-linear-cryptanalysis`,
  `filter-js-from-html`, and `financial-document-processor`.
- Initial cumulative-E007 arm: 15 trials in 2h33m. Thirteen tasks produced an
  official reward and all were 0.0; `crack-7z-hash` and
  `extract-moves-from-video` hit `AgentTimeoutError`. The remaining two tasks,
  `feal-linear-cryptanalysis` and `filter-js-from-html`, were censored for
  benchmark/environment failures. On the ten initially scoreable paired tasks,
  START was 2/10 and cumulative E007 was 0/10: no gains and two losses.
- Docker storage exhaustion affected several late environment starts. Cleanup
  removed only two stopped Harbor containers and recreatable Terminal-Bench
  image cache; unrelated running containers and volumes were preserved.
- Frozen discordant reruns: START scored 0/2 and cumulative E007 scored 0/2 on
  `configure-git-webserver` and `count-dataset-tokens`. The original START
  passes therefore did not reproduce, but the cumulative arm also failed the
  precommitted requirement to recover both. The broad result is flat or worse,
  with no verified cumulative gain.
- Decision: **REJECT the cumulative product hill**. Revert E001 reasoning
  retention, E005 model-registry metadata, and E007 max-token continuation.
  Retain E006's 360-second request timeout solely because the user explicitly
  requested the increased timeout, not because B02 demonstrated task-success
  value. Retain the accepted E004 evaluation adapter and all experiment records.
  Revert commits: `e031ef7`, `121986d`, and `d345998`. After rollback, the only
  product diff from `GLM52_START_BASELINE` is the bounded timeout change and its
  focused test. `go test ./...` passes.
- Conclusion: optimization was performed, but a meaningful task-success
  improvement was not demonstrated.

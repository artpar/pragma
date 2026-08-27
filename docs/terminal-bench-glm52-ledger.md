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

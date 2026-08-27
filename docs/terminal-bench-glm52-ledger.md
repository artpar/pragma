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

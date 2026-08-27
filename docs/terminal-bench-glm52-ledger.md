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
- Maximum output: 4,096 tokens
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

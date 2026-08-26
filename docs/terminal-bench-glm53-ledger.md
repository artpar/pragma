# GLM-5.3 Terminal-Bench 2.1 experiment ledger

This is the compact append-only ledger for the GLM-5.3 Pragma hill climb.
Official Terminal-Bench verifier rewards are the primary outcome.

## Fixed evaluation configuration

- Provider: OpenRouter Chat Completions (`https://openrouter.ai/api/v1`)
- Model identifier: `z-ai/glm-5.3` (not the Flash variant)
- Reasoning mode: no explicit Pragma thinking/reasoning parameter
- Temperature: `0`
- Other sampling parameters: provider defaults (Pragma sends none)
- Advertised context limit: 1,048,576 tokens
- Maximum output: 8,192 tokens
- Tool protocol: provider-native tool calls; Pragma `Bash` and `apply_patch`
- Pragma product path: ordinary non-interactive CLI with `--loop provider-tools`
- Turn limit: 100 model turns
- Permission mode: `bypassPermissions` inside the isolated task container
- Development task timeout: 1,800 seconds active agent execution
- Benchmark: the unchanged local `terminal-bench/terminal-bench-2-1` manifest,
  89 tasks, with official per-task verifiers
- Harbor: 0.22.0; Docker environment; verifier timeouts unchanged
- Infrastructure retry: no whole-task retries by default; transient model-request
  retries are Pragma's bounded same-endpoint retry behavior

## Starting point

- `START_BASELINE`: `d0423f86d800d23d2241ca403c5b5f794285f34e`
- Baseline deterministic checks: `go test ./...` passed.
- Provider smoke: exact output `GLM53_PRAGMA_OK` was returned by
  `z-ai/glm-5.3` through the provider-tools loop.
- Two earlier `adaptive-rejection-sampler` attempts are excluded from
  comparable evidence: one used `z-ai/glm-5.3-flash` and failed TLS setup; the
  other used the Flash model plus an SWE-specific orchestration and hit Harbor's
  900-second timeout without producing `/app/ars.R` (official reward 0).

## Baseline diagnostic panel D01 (precommitted)

The first panel contains six tasks across repository recovery, data processing,
async coding, binary/database recovery, native extension build repair, and C
implementation. It was fixed before inspecting any result:

1. `fix-git`
2. `regex-log`
3. `cancel-async-tasks`
4. `sqlite-db-truncate`
5. `build-cython-ext`
6. `path-tracing`

Purpose: establish task-completion evidence and expose the earliest observable
harness defects in the normal product path. This panel is diagnostic baseline
evidence, not promotion evidence for a candidate that has not yet been defined.

Pre-result infrastructure notes:

- The first Harbor launch did not add the repository to `PYTHONPATH`; it failed
  while importing the custom adapter, before task setup or any model call.
- The next launch requested 65,536 maximum output tokens. OpenRouter rejected
  all four model requests with HTTP 402 because the credential's authorization
  allowed at most 8,703. The other two queued trials were cancelled. These six
  outcomes are excluded as infrastructure/configuration failures. The fixed
  maximum output was reduced to 8,192 before any valid baseline task result.
- With 8,192 output tokens, two concurrent trials were accepted, but the
  zero-credit account's in-flight authorization only sustained one request
  budget. `fix-git` made seven useful model turns, then received transient HTTP
  402 `in_flight_budget_exhausted` while `regex-log` held the other request.
  Pragma terminated instead of honoring the returned 120-second Retry-After.
  The job was cancelled and excluded as an infrastructure/harness interaction.
  All comparable development evaluations are serial from D01-v3 onward.

## Experiments

### E001 — exact OpenRouter model identity and context (precommitted)

- Parent champion: `d0423f86d800d23d2241ca403c5b5f794285f34e`
- Observed failure: normal runs of `z-ai/glm-5.3` warn that the model is
  unknown, report no authoritative context window, fall back to a 200K
  compaction budget, and select `z-ai/glm-5.3-flash` as OpenRouter's secondary
  compaction model.
- Hypothesis: registering the exact model and using the active OpenRouter model
  for compaction will remove the false warning, budget the advertised 1,048,576
  context correctly, and prevent a hidden model switch without changing tool or
  task behavior.
- Harness mechanism: OpenRouter model metadata and compaction-model selection.
- Diagnostic: focused provider/CLI tests plus an exact-model smoke invocation.
- Frozen controls: all six D01 tasks, unchanged.
- Expected observable change: exact model is the OpenRouter default/listed
  model, `ContextWindow("z-ai/glm-5.3")` returns `(1048576, true)`, and OpenRouter
  compaction resolves to the active model rather than Flash.
- Reject if any deterministic assertion fails or later frozen controls show a
  meaningful regression.
- Promotion status before implementation: PROVISIONAL; mechanical evidence may
  retain it temporarily, but benchmark controls require provider capacity.

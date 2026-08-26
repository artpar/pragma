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
- Maximum output: 65,536 tokens
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

## Experiments

No product candidate has yet been precommitted.

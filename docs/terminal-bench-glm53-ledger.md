# GLM-5.3 Terminal-Bench 2.1 experiment ledger

This is the compact append-only ledger for the GLM-5.3 Pragma hill climb.
Official Terminal-Bench verifier rewards are the primary outcome.

## Fixed evaluation configuration

- Provider: OpenRouter Chat Completions (`https://openrouter.ai/api/v1`)
- Model identifier: `z-ai/glm-5.3` (not the Flash variant)
- Reasoning mode: model-native reasoning always on at OpenRouter's default
  `max` effort; Pragma sends no explicit reasoning parameter
- Temperature: `0`
- Other sampling parameters: provider defaults (Pragma sends none)
- Advertised context limit: 1,048,576 tokens
- Maximum output: 2,048 tokens (supersedes the capacity-blocked 4,096-token
  development regime described below)
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

- `START_BASELINE`: `d0423f8f1a388422ad73c5006760cd04753a1f28`
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
- The first serial launch still requested 8,192 output tokens after the
  promotional allowance had fallen to 7,859; three requests were rejected
  before model execution and the job was cancelled. The fixed per-turn maximum
  was reduced to 4,096. D01 is thereafter launched one task at a time so an
  infrastructure rejection cannot cascade through the panel.

## Experiments

### E001 — exact OpenRouter model identity and context (precommitted)

- Parent champion: `d0423f8f1a388422ad73c5006760cd04753a1f28`
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
- Candidate commit: `caf03c5f7591c214595dcd48d9fda5e91b13d115`.
- Deterministic result: focused OpenRouter/CLI/compaction tests and `go test
  ./...` passed; exact-model smoke returned `E001_OK` without the false unknown
  model warning.
- D01 baseline result available so far: `fix-git` reward 1.0, no exception,
  START_BASELINE, 109 seconds wall clock.
- D01 candidate result: no verifier result. E001 correctly completed and checked
  the merge in the workspace, then an additional model request received
  transient 402 `in_flight_budget_exhausted`; Pragma terminated. This is an
  infrastructure/harness failure, not a verified task fail or pass.
- Decision: PROVISIONAL. The mechanical defect is removed, but the frozen
  control did not produce a comparable verifier outcome. Do not stack E001 into
  the next isolated candidate.

### E002 — retry transient OpenRouter in-flight budget responses (precommitted)

- Parent champion product: `d0423f8f1a388422ad73c5006760cd04753a1f28`
  (`START_BASELINE`; evaluation-only commits remain in history).
- Observed failure: three GLM-5.3 task executions preserved workspace work but
  terminated when OpenRouter returned HTTP 402 with reason
  `in_flight_budget_exhausted` and `Retry-After: 120`. Pragma classified the
  response as permanent `request_failed`. Ordinary permanent 402 responses
  instead reported `limit_source: openrouter_credits` and fewer affordable
  tokens.
- Hypothesis: an OpenRouter-specific classifier that retries only the explicit
  transient in-flight reason and honors its Retry-After value will allow active
  work to continue without retrying permanent insufficient-credit errors.
- Harness mechanism: provider error classification and existing bounded retry.
- Diagnostic: focused classification tests for transient and permanent 402s.
- Frozen control: `fix-git` first; remaining D01 tasks as capacity permits.
- Expected observable change: transient 402 is retryable with a 120-second
  delay; permanent credit 402 remains non-retryable; the task resumes in the
  same workspace and reaches the official verifier.
- Reject if classification conflates the two 402 forms, ignores Retry-After,
  destroys workspace state, or fails the frozen task without a distinct cause.
- Candidate commit: `97c196340a0ba4cbf785909d97ac701fee2eb4d4`.
- Deterministic result: transient 402 classifies as retryable rate-limit with a
  120-second delay; permanent credit 402 remains non-retryable; standard 429
  behavior is preserved; `go test ./...` passed.
- Frozen-control execution: no verifier result. On `fix-git`, the candidate
  preserved the recovered commit on a branch, then received the exact transient
  402. The recording contains `APIRetryScheduled` with `delay_ms: 120000` and a
  retryable `APIRequestFailed` at attempt 1, proving the changed branch executed
  in the ordinary CLI path. Attempt 2 ran after 120 seconds in the same Harbor
  trial and workspace, but OpenRouter then returned a distinct permanent 402:
  `limit_source: openrouter_credits`, with only 3,160 of the requested 4,096
  output tokens affordable. Pragma correctly classified that response as
  non-retryable. Harbor reported a runtime exception and did not invoke the
  verifier; this is excluded from task reward evidence.
- Decision: PROVISIONAL. The live provider trace validates both halves of the
  classifier and the Retry-After behavior, but depleted provider authorization
  prevents the precommitted frozen control from reaching an official verifier.
  Do not claim a benchmark improvement from E002 without renewed exact-model
  capacity and comparable verifier outcomes.
- Champion disposition: the candidate was removed from the checked-out product
  by `4317af7` because a provisional change must not become the foundation for
  further experiments. The complete candidate remains recoverable at
  `97c1963` for reevaluation when exact-model capacity is restored. The current
  product behavior therefore matches `START_BASELINE`; evaluation scaffolding
  and the append-only experiment history remain committed separately.

## D01-v4 capacity-compatible baseline rerun (precommitted)

- Current OpenRouter metadata reconfirms exact model `z-ai/glm-5.3`, native
  always-on reasoning with default `max` effort, a 1,048,576-token context
  window, and provider-native tool calling. Sampling and tool settings remain
  unchanged.
- Capacity probe on 2026-08-30: the exact endpoint accepted 64 and 2,048
  maximum-output-token requests, but rejected 4,096 before inference with
  permanent HTTP 402 `openrouter_credits`, reporting only 3,160 tokens
  affordable. This is evaluation infrastructure, not a task outcome.
- Supersede the unusable 4,096-token development regime with a fixed 2,048-token
  maximum output for both START and candidates. Earlier 4,096-token task
  outcomes are retained as history but are not treated as comparable promotion
  evidence. Context, temperature 0, provider-tools protocol, 100 turns, serial
  execution, and the 1,800-second active-task limit remain fixed.
- Rerun all six frozen D01 tasks on `START_BASELINE` before using the new regime
  for candidate promotion. Record every official verifier outcome and classify
  provider or benchmark failures separately. Run serially; do not interpret a
  capacity rejection as a task failure.

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

### D01-v4 observed outcome

- `regex-log` was the only task to reach an official verifier: reward 0.0. Its
  first model response ended at the 2,048-token `max_tokens` boundary without
  a tool call; START treated that truncation as successful completion and never
  created `/app/regex.txt`. This is a harness failure, not evidence that the
  model completed the task incorrectly.
- `fix-git` made nine useful provider/tool turns in the same workspace, then
  terminated on transient HTTP 402 `in_flight_budget_exhausted` with
  `Retry-After: 120`. `cancel-async-tasks`, `sqlite-db-truncate`, and
  `build-cython-ext` were rejected by permanent `openrouter_credits` responses
  after their prompts reduced affordable maximum output to 1,727–1,900 tokens.
  These four trials are infrastructure/harness exceptions without verifier
  scores. `path-tracing` was operator-cancelled during environment startup once
  the repeated capacity blocker was established.
- Conclusion: 2,048 is accepted for a tiny smoke but is not a stable funded
  evaluation regime for task prompts. Do not lower the cap repeatedly and call
  the resulting runs comparable. Broad promotion remains capacity-blocked.

### E001 reconsideration — exact model preservation (precommitted)

- New controlling evidence: the goal explicitly prohibits hidden alternative
  models for context compression, while START deterministically registers only
  `z-ai/glm-5.3-flash`, warns that exact `z-ai/glm-5.3` is unknown, budgets it
  at the generic 200K fallback, and selects Flash for compaction. OpenRouter's
  current official metadata confirms exact `z-ai/glm-5.3` has a 1,048,576-token
  context window and provider-native tool support.
- Reopen E001 despite its earlier provisional disposition. Implement only the
  deterministic identity/context/compaction fix, updated against current model
  metadata. Reject if exact GLM-5.3 is not the OpenRouter default/listed model,
  its context is not 1,048,576, compaction does not preserve the active model,
  focused tests fail, or the exact-model smoke changes protocol behavior.
- Promotion basis: mechanically strong correction of a hard fixed-model
  invariant. Provider-credit failures cannot establish benchmark improvement,
  but they also must not force normal Pragma to violate the declared model.

### E001 reconsideration outcome

- Candidate commit: `9590458`. Exact `z-ai/glm-5.3` is now OpenRouter's
  default and a listed model with a 1,048,576-token context; Flash remains
  separately listed. OpenRouter compaction receives the active configured model
  instead of substituting the provider default.
- Focused OpenRouter, CLI, and compaction tests passed, followed by `go test
  ./...`. A normal provider-tools smoke with explicit exact model returned
  `E001_REOPENED_OK` and emitted no unknown-model warning. The first smoke that
  intentionally omitted `--model` resolved the user's persisted `gemma-4`
  override and was rejected by OpenRouter; it did not exercise provider-default
  selection and is not candidate evidence.
- Decision: **ACCEPTED on mechanically strong harness evidence**. The candidate
  corrects deterministic model identity, context budgeting, and hidden model
  substitution, directly enforcing the fixed-model invariant. No claim of
  increased verified task completion is made; funded broad controls remain
  required for that stronger claim.

### E003 — continue truncated provider-tools work (precommitted)

- Parent champion: `9590458` (E001 exact-model preservation; ledger-only commit
  `2b479ce` records its acceptance).
- Observed failure: on D01-v4 `regex-log`, exact GLM-5.3's first response ended
  with `stop=max_tokens` at the fixed 2,048-token boundary, contained no tool
  call, and produced no deliverable. Pragma emitted successful turn completion
  and exited 0; the clean official verifier failed because `/app/regex.txt`
  did not exist.
- History check: the same mechanism was E007 in the GLM-5.2 campaign. It was
  mechanically accepted with non-regressing controls, then removed when a
  cumulative broad checkpoint tied START. Reopening is justified by new exact
  GLM-5.3 branch-exercising evidence and a materially smaller output cap; the
  earlier absence of broad task-success improvement remains controlling
  caution.
- Hypothesis: preserving a tool-free max-token assistant response and requesting
  a neutral continuation in the same bounded provider-tools loop will prevent
  false successful termination and give GLM-5.3 its missing opportunity to act.
- Smallest mechanism: special-case only `StopMaxTokens` with no tool calls;
  append `Continue from the truncated response.` and continue within the
  existing 100-turn bound. Do not change prompts, tools, ordinary end-turn/tool
  handling, model settings, or evaluator behavior.
- Diagnostic: `regex-log`. Frozen controls: `fix-git` and
  `cancel-async-tasks`, under the same eventual funded regime. Deterministic
  gate must prove partial content preservation, a subsequent provider request,
  subsequent tool execution, normal final termination, and bounded turn-cap
  failure. Reject if the branch does not activate, drops state, changes normal
  completion, or only adds repeated truncations without credible progress.
- Current-capacity diagnostic amendment: run `regex-log` once at 1,536 maximum
  output tokens because the unfunded authorization now rejects task-scoped
  2,048-token requests. This run may prove live branch activation but is not
  comparable promotion evidence. Frozen controls remain deferred to one stable
  funded regime shared by champion and candidate.

### E003 observed outcome

- Candidate commit: `da6c7b1`; Linux amd64 binary SHA-256:
  `453671399b5319fc1d3abde3e519dc176de4602122ca467d328c64462d6cac43`.
  The implementation changes only provider-tools handling of tool-free
  `StopMaxTokens` responses and retains the existing turn cap.
- Deterministic tests prove partial assistant-content preservation, neutral
  continuation ordering, subsequent Bash execution, exactly one ordinary final
  completion, and an error after repeated truncations exhaust the turn cap.
  `go test ./...` passes.
- Live exact-model diagnostic: `regex-log` returned two consecutive
  `stop=max_tokens` responses. Candidate Pragma issued a subsequent model
  request after each, proving the ordinary product branch no longer falsely
  terminates. The third request hit transient 402
  `in_flight_budget_exhausted`; no verifier ran and no task-success credit is
  assigned.
- Decision: **ACCEPTED on mechanically strong harness evidence**, with the
  GLM-5.2 broad-tie caution still active. The change removes a deterministic
  false-success terminal and preserves active model work; it has not yet shown
  increased GLM-5.3 task completion and must survive funded frozen controls and
  a later broad checkpoint.

### E004 — retry transient OpenRouter in-flight authorization (precommitted)

- Parent champion: `da6c7b1` (E001 + accepted E003).
- Observed failure: START `fix-git` and E003 `regex-log` both made useful model
  progress in an intact workspace, then Pragma terminated immediately on HTTP
  402 with explicit reason `in_flight_budget_exhausted` and
  `Retry-After: 120`. Permanent 402 responses instead identify
  `limit_source: openrouter_credits` and must not be retried.
- History check: GLM-5.3 E002 previously proved both classifier branches live
  but was reverted while provider capacity prevented a verifier. The new E003
  diagnostic independently reproduces the transient failure after two
  truncation continuations, materially strengthening the same hypothesis.
- Hypothesis: route only the explicit transient in-flight reason through the
  existing bounded retry mechanism and honor its provider delay, preserving
  conversation and workspace state; leave permanent credit errors terminal.
- Smallest mechanism: allow the OpenAI-compatible adapter to receive a
  provider-specific classifier, and add an OpenRouter classifier for the exact
  transient reason. No whole-task restart, prompt change, model change, or
  evaluator recovery.
- Diagnostic: rerun E003 `regex-log`. Frozen controls remain `fix-git` and
  `cancel-async-tasks` under one funded regime. Deterministic rejection gates:
  transient and permanent 402s are conflated, `Retry-After` is ignored, normal
  429 behavior regresses, workspace/conversation state restarts, or retry is
  unbounded.
- Candidate commit: `60bc3eb`; Linux amd64 binary SHA-256:
  `d3f299521d114c3acaa1a3eb4d883b8e8e793db2b1172c9a7c65f4391d482656`.
  Use the same diagnostic-only 1,536-token `regex-log` setup as E003 to test
  live retry scheduling and same-session continuation; it remains ineligible
  as comparable promotion evidence.

### E004 observed outcome

- Focused tests confirm the explicit `in_flight_budget_exhausted` payload is
  retryable as `rate_limit` with its 120-second provider delay, permanent
  `openrouter_credits` 402 is non-retryable, and ordinary 429 classification is
  unchanged. The existing shared retry loop bounds attempts and respects
  context cancellation. `go test ./...` passes.
- The new live diagnostic was rejected before inference by a permanent credit
  response: with the task prompt, OpenRouter reported only 1,048–1,153 output
  tokens affordable versus the requested 1,536. Candidate Pragma correctly
  emitted `retryable=false` and did not wait or retry. This is useful negative
  branch evidence but provides no verifier outcome.
- The earlier E002 live recording remains direct evidence for the identical
  positive branch: `APIRetryScheduled delay_ms: 120000`, followed by attempt 2
  in the same Harbor trial and workspace. That attempt then received a distinct
  permanent credit error and terminated correctly.
- Decision: **ACCEPTED on mechanically strong provider-recovery evidence**.
  This enforces the goal's explicit requirement to recover transient provider
  failures without restarting task state while refusing permanent failures. It
  does not demonstrate increased task completion.

## Broad-gate hold after three retained mechanisms

- Retained GLM-5.3 mechanisms are now E001 exact-model/context preservation,
  E003 max-token continuation, and E004 transient in-flight retry. Each removes
  a directly observed deterministic harness defect; none has broad task-success
  evidence.
- Per the broad-checkpoint cadence, do not begin another product experiment or
  claim a task-success champion until exact-model capacity supports one stable
  output regime for paired controls and a 15–30 task checkpoint. Current
  unfunded OpenRouter authorization rejects task-scoped requests even at 1,536
  output tokens and is therefore insufficient.

### E001 live-metadata correction (precommitted)

- OpenRouter's live `/api/v1/models` response on 2026-08-30 reports
  `context_length: 1310720` for both `z-ai/glm-5.3` and
  `z-ai/glm-5.3-flash`. This contradicts the 1,048,576 value in page/FAQ text
  used by the original E001 test. The live API is the runtime-facing authority
  for Pragma's compaction budget.
- Correct only the two registered GLM-5.3 context values and focused assertions
  to 1,310,720. Preserve active-model compaction, provider default, tools,
  prompts, and all other behavior. Reject if focused or full tests fail.

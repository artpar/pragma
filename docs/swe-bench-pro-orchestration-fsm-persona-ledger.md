# SWE-bench Pro Orchestration FSM Persona Implementation Ledger

## 1. Objective and Current Status

Objective: implement and iterate the SWE-bench Pro persona-based FSM until it
preserves evaluator-facing task acceptance across long runs. The Flipt
Kubernetes authentication failure is the first calibration baseline, not the
technology, stack, or task template for the orchestration.

Current status: first production FSM/persona implementation pass complete, with
production-shaped prompt replay passing for the retained checkpoints. Fresh
baseline runs exposed follow-up prompt and payload risks: task-specific ID
drift, weak structural validation being counted as behavior coverage,
coordination-artifact reads, scope drift, and missing original task text in the
mapper payload. The personas and orchestration payloads have since been
generalized so acceptance IDs come from the current task and validation gates
are behavior-surface based, not Go/Flipt/Kubernetes specific. The current
prompt/payload AB chain reaches mapper, theory, planner, worker, targeted
validator, and reviewer for the first producer-discovery baseline without
starting a new full benchmark run. A later prompt-level loopback exposed a
second-order drift source: the theory keeper could preserve producer evidence
but corrupt acceptance-map validation requirements, and the planner could pick
the right slice while overclaiming coverage. The FSM now separates those
responsibilities with an acceptance auditor and a slice-plan auditor, and the
runtime records handoff bytes and SHA-256 hashes.

## 2. Baseline Evidence Imported From Prior AB Ledger

Source evidence from `docs/swe-bench-pro-orchestration-ab-ledger.md`:

- Failed run:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Failed instance:
  `flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Failed evaluator package: `go.flipt.io/flipt/internal/config`
- Failed evaluator subtests:
  - `TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)`
  - `TestLoad/authentication_kubernetes_defaults_when_enabled_(ENV)`
  - `TestLoad/advanced_(YAML)`
  - `TestLoad/advanced_(ENV)`
- Failed-run diagnosis: config YAML, ENV, defaults, and custom binding
  acceptance were visible early but weakened into prose; the loop then
  prioritized runtime token validation and self-authored runtime tests.
- Surviving AB mechanisms:
  - add `swe_acceptance_mapper` before theory keeping
  - persist `/tmp/pragma/swe/acceptance-map.json`
  - require stable acceptance IDs
  - add acceptance coverage fields to `slice-plan.json`
  - make worker reports preserve acceptance IDs, validation commands, approved
    paths, forbidden scope, and completion-claim status
  - make targeted validation mark missing acceptance coverage as
    `insufficient`
  - make reviewer/final validator/final reviewer block unvalidated blocking
    acceptance items

## 3. Current FSM/Persona Architecture Before Edits

Baseline current FSM: `orchestrations/swe-bench-pro-engineering-loop.yaml`.

Current states:

1. `swe_repo_survey`
2. `swe_theory_keeper`
3. `swe_slice_planner`
4. `swe_engineering_worker`
5. `swe_targeted_validator`
6. `swe_engineering_reviewer`
7. `route_review_decision`
8. `swe_scope_expander`
9. `swe_final_validator`
10. `swe_final_reviewer`
11. `route_final_verdict`
12. `swe_diagnosis_router`
13. `route_diagnosis_decision`
14. terminal `done` / `unresolved`

Current durable artifact problem: `engineering-context.md` is the only central
task memory. There is no first-class task-derived acceptance map handed through
the loop, and no per-slice acceptance coverage schema.

## 4. Proposed FSM/Persona Changes

Implement the smallest coherent production change set proven by AB evidence:

1. Add `swe_acceptance_mapper` between repo survey and theory keeping.
2. Add `/tmp/pragma/swe/acceptance-map.json` as a required artifact after the
   mapper and hand it to all later persona states.
3. Have `swe_theory_keeper` refresh `engineering-context.md` only.
4. Have `swe_acceptance_auditor` own acceptance-map updates and write an exact
   handoff audit before planning.
5. Extend `slice-plan.json` with `acceptance_ids`,
   `validation_covers_acceptance_ids`, and `missing_acceptance_ids`.
6. Have `swe_slice_plan_auditor` own pre-worker coverage-claim correction.
7. Extend worker, targeted validator, reviewer, final validator, final
   reviewer, scope expander, and diagnosis router prompts so acceptance coverage
   is an explicit gate rather than an implied checklist.

## 5. File-by-File Implementation Log

Production edits made:

- `orchestrations/swe-bench-pro-engineering-loop.yaml`
  - Added `swe_acceptance_mapper` after `swe_repo_survey`.
  - Added `swe_acceptance_auditor` between theory keeping and planning.
  - Added `swe_slice_plan_auditor` between planning and worker execution.
  - Added `acceptance_map` as a required output of the mapper and acceptance
    auditor.
  - Added `handoff_audit` and `plan_audit` artifacts so handoff integrity and
    coverage corrections are explicit before work begins.
  - Added `/tmp/pragma/swe/acceptance-map.json` to all downstream handoffs
    that feed planning, work, validation, review, final validation, final
    review, diagnosis, scope expansion, and loop repair.
- `personas-research-v2/swe_acceptance_auditor.yaml`
  - New persona. Owns acceptance-map mutation after initial mapping.
  - Preserves task-derived IDs and non-status fields, especially
    `required_validation`, and records exact input artifact bytes/SHA values in
    `/tmp/pragma/swe/handoff-audit.md`.
- `personas-research-v2/swe_slice_plan_auditor.yaml`
  - New persona. Audits planner coverage claims before worker execution.
  - Removes IDs from `validation_covers_acceptance_ids` when a slice only
    implements prerequisites or covers a subset of required surfaces/commands.
- `personas-research-v2/swe_acceptance_mapper.yaml`
  - New persona. Converts the original task prompt plus repo survey into a
    stable task-derived acceptance map.
  - Requires separate blocking items for independently failing surfaces.
  - Uses task-derived IDs from the requested feature or failure and explicitly
    avoids hard-coding IDs from examples, prior benchmark cases, repository
    names, languages, frameworks, or technologies.
- `personas-research-v2/swe_theory_keeper.yaml`
  - Now writes only `engineering-context.md`; acceptance-map mutation moved to
    `swe_acceptance_auditor`.
  - Treats the audited acceptance map as authoritative over lossy prose
    summaries without rewriting it.
- `personas-research-v2/swe_slice_planner.yaml`
  - Requires `acceptance-map.json` as input.
  - Requires `handoff-audit.md` as input.
  - Adds `acceptance_ids`, `validation_covers_acceptance_ids`, and
    `missing_acceptance_ids` to the required JSON schema.
  - Adds `worker_track` so no-edit discovery can route to a separate discovery
    worker while edit/refactor/producer slices stay on the engineering worker.
  - Prioritizes pending blocking acceptance surfaces before lower-priority
    runtime work.
- `personas-research-v2/swe_slice_plan_auditor.yaml`
  - Adds `worker_track` when correcting slice plans and records the chosen track
    in `plan-audit.md`.
  - Routes no-edit producer/context discovery to `worker_track: "discovery"` and
    edit/refactor/producer/validation-preparation work to
    `worker_track: "implementation"`.
- `personas-research-v2/swe_discovery_worker.yaml`
  - New no-edit discovery persona for read-only discovery reports.
  - Limits discovery to one inspection turn before reporting and forbids
    producer/build/lint/test/preflight/dry-run execution in this persona.
- `personas-research-v2/swe_engineering_worker.yaml`
  - Requires acceptance-map input.
  - Requires handoff-audit and plan-audit inputs.
  - Adds acceptance coverage, validation-required, task-completion-claim, and
    scope-adherence sections to the report.
- `personas-research-v2/swe_targeted_validator.yaml`
  - Validates acceptance IDs instead of worker confidence.
  - Marks missing ID coverage as `Result: insufficient`.
- `personas-research-v2/swe_engineering_reviewer.yaml`
  - Blocks final-validation routing while blocking acceptance items are
    pending, insufficient, missing, weakened, or unsupported by direct evidence.
  - Adds structured `acceptance_status` to `review-decision.json`.
- `personas-research-v2/swe_final_validator.yaml`
  - Computes broad validation from the full acceptance map and final diff.
  - Chooses broad validation from acceptance-map behavior surfaces and blocks
    compile-only/package-only evidence when the acceptance item requires a
    fixture load, API call, UI workflow, migration check, producer
    regeneration, schema validation, or runtime behavior check.
- `personas-research-v2/swe_final_reviewer.yaml`
  - Approves only when every blocking original acceptance item is validated or
    explicitly not applicable.
  - Blocks when final validation omits acceptance coverage.
- `personas-research-v2/swe_scope_expander.yaml`
  - Ties scope decisions back to blocking acceptance IDs.
- `personas-research-v2/swe_diagnosis_router.yaml`
  - Adds `blocking_acceptance_ids` so final blocks route by missing task
    acceptance surface.
- `internal/orchestration/runner.go`, `internal/query/event.go`,
  `internal/orchestration/projection.go`, `internal/observe/event_catalog.go`,
  `internal/app/state.go`, `internal/cli/run.go`
  - Handoff reads and normal state output writes now carry byte counts and
    SHA-256 hashes through orchestration events, snapshots, and persisted
    artifact records.
- `internal/query/miniswe_loop.go`
  - Added runtime-authored shell command evidence recording for states that
    opt in with an artifact whose `runtime_capture.type` is
    `command_evidence`. The evidence is JSONL with command/output hashes,
    return code, timeout, completion-sentinel, and declared-report-write
    markers; it avoids storing full command output.
  - Added generic declared shell-policy enforcement for states that opt in with
    regex deny patterns. The policy strips heredoc bodies before matching so a
    report can quote a forbidden command without executing it.
  - Sends shell-action loop turns with reasoning disabled by default so
    reasoning-only model responses cannot consume the full output budget before
    producing the required bash action.
- `internal/provider/anyllm/translate.go`
  - Maps an explicit disabled thinking config to `reasoning_effort: none` for
    any-LLM/OpenAI-compatible providers.
- `internal/orchestration/runner.go`
  - States declaring `runtime_capture.type: command_evidence` receive a fresh
    evidence file path in the shell loop context.
  - Required artifacts may now declare generic integrity checks in
    orchestration YAML. The SWE loop uses these checks for source-quoted JSON
    acceptance maps, behavior-validation command quality, forbidden report
    text, and command-evidence-backed report claims.
  - Behavior-validation command checks now reject placeholders such as
    `unknown`, `none`, `todo`, and `tbd`; acceptance maps must name concrete
    validation commands before the loop can proceed.
- `orchestrations/swe-bench-pro-engineering-loop.yaml`
  - Added `/tmp/pragma/swe/worker-command-evidence.jsonl` as a required
    runtime-authored worker output and handed it to targeted validation,
    engineering review, scope/final routes, and theory/acceptance loopbacks.
  - Declares the acceptance-map and worker-report integrity checks in YAML
    instead of relying on generic runtime branches for those artifact IDs.
  - Adds `route_worker_track`, routes no-edit discovery to
    `swe_discovery_worker`, and keeps implementation work on
    `swe_engineering_worker`.
  - Declares a generic shell policy on `swe_discovery_worker` that rejects
    generator/generated-output, build, test, lint, preflight, and dry-run command
    lines before shell execution.
- `personas-research-v2/swe_engineering_worker.yaml`
  - Clarifies that `worker-command-evidence.jsonl` is runtime-authored and must
    not be written, edited, truncated, or fabricated by the worker.
  - Requires completion reports to be backed by prior state-local runtime
    evidence instead of handoff facts alone.
- `personas-research-v2/swe_targeted_validator.yaml`,
  `personas-research-v2/swe_engineering_reviewer.yaml`,
  `personas-research-v2/swe_theory_keeper.yaml`,
  `personas-research-v2/swe_acceptance_auditor.yaml`
  - Treat runtime worker evidence as distinct from worker report prose and use
    it to detect unsupported action, validation, or discovery claims.
- `internal/orchestration/orchestration.go`
  - Added schema support and load-time validation for declarative artifact
    checks and runtime capture configuration.
- `personas-research-v2/swe_targeted_validator.yaml`
  - Removed stack-specific producer/build examples from the production prompt
    and replaced them with generic producer plus compile/targeted-validation
    evidence wording.

## 6. AB Replay Cases and Verdicts for Changed Prompts

Prior AB cases imported as design evidence:

- `A2-acceptance-mapper-stable-ids`: pass
- `B-theory-keeper-acceptance-map-handoff`: limited pass; proved missing map
  gate
- `C1b-slice-planner-strict-schema`: pass with caveat
- `C2c-slice-planner-acceptance-map-status`: pass
- `W-worker-report-acceptance-handoff`: pass
- `D3-targeted-validator-artifact-gate`: pass
- `E-reviewer-final-validation-gate`: pass
- `F-final-validator-coverage-gate`: pass
- `G-final-reviewer-acceptance-block`: pass

Production prompt replay directory:

- `.pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts`

Method:

- Reused prior AB handoff/user payloads from
  `.pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep`.
- Replaced each case system prompt with the current production persona YAML
  prompt.
- Replayed with `go run ./cmd/pragma replay raw-http <case-dir> --provider
  lilac`.

Current production replay matrix:

| Case | Status | Evidence |
| --- | --- | --- |
| `A2-acceptance-mapper-stable-ids` | Pass | Wrote `acceptance-map.json` with stable `ACCEPT-K8S-*` IDs. |
| `B2-theory-keeper-production-handoff` | Pass | Wrote `engineering-context.md` and preserved pending acceptance-map IDs. |
| `C1b-slice-planner-strict-schema` | Rejected | Malformed/old fixture now writes the schema but still uses IDs absent from the acceptance map and assumes the producer command; superseded by C10. |
| `C2c-slice-planner-acceptance-map-status` | Pass | Planned `k8s-config-validation`, selected `go test ./internal/config/...`, and deferred runtime token validation. |
| `D3-targeted-validator-artifact-gate` | Rejected | Old malformed fixture lacks usable acceptance-map/slice IDs and overclaims pass from build/vet evidence; superseded by D3b and D4. |
| `D3b-targeted-validator-production-handoff` | Pass | Wrote `targeted-validation.md`, set `Result: insufficient`, and named missing config-load IDs and `go test ./internal/config/...`. |
| `E-reviewer-final-validation-gate` | Pass | Wrote `review-decision.json`, set `decision: continue_implementation`, `failure_type: insufficient_validation`, and blocked final readiness. |
| `F-final-validator-coverage-gate` | Pass | Chose `go test ./internal/config/...` before auth runtime package tests. |
| `G-final-reviewer-acceptance-block` | Pass | Wrote `Decision: BLOCK` for missing config-loading validation. |
| `W2-worker-report-production-handoff` | Pass | Wrote worker report with acceptance handoff fields and `task_completion_claim_allowed: false`. |

Prompt iterations during replay:

- Added handoff `Content:` rules to planner and all downstream personas.
- Strengthened planner to require fenced bash first, acceptance IDs for
  discovery/validation slices, and acceptance-map rebuild behavior when the map
  is missing.
- Strengthened targeted validator against `/tmp` reads, ad hoc grep/source
  checks, malformed handoffs, and rerunning worker evidence. The original
  malformed D3 remained rejected; production-shaped D3b passed.
- Strengthened final validator with invalid `cat /tmp` examples and Flipt
  config-validation priority; replay passed after this patch.

Generalized prompt replay directory:

- `.pragma/prompt-ab/20260612T000002Z-generalized-fsm-prompts`

Method:

- Copied the first production replay matrix and refreshed each case's embedded
  system prompt from the current persona YAML after removing task/stack-specific
  design rules.
- Kept the Flipt/Kubernetes user fixtures as calibration inputs; those strings
  are expected in user/context content and are not persona design rules.

Generalized replay results:

| Case | Status | Evidence |
| --- | --- | --- |
| `A2-acceptance-mapper-stable-ids` | Pass | Wrote a task-derived acceptance map from the calibration prompt without relying on the prior hard-coded ID list. |
| `A3-acceptance-mapper-live-survey` | Pass | Replayed the stopped fresh run's live mapper request after the input-matrix patch; emitted separate `CONFIG-INPUT-FILE`, `CONFIG-INPUT-ENV`, and `CONFIG-DEFAULTS` acceptance items from repo survey evidence. |
| `C2c-slice-planner-acceptance-map-status` | Pass | Planned config-loading validation before runtime token work and listed only directly covered IDs in `validation_covers_acceptance_ids`. |
| `D3b-targeted-validator-production-handoff` | Pass | Wrote `targeted-validation.md`, kept build/vet as insufficient for config-load IDs, and suggested `go test ./internal/config/...`. |
| `W2-worker-report-production-handoff` | Pass after patch | Response now starts with fenced `bash`, preserves missing config-load IDs, and sets `task_completion_claim_allowed: false`. |
| `E-reviewer-final-validation-gate` | Pass | Wrote `review-decision.json`, kept `decision: continue_implementation`, `failure_type: insufficient_validation`, and blocked final readiness. |
| `F-final-validator-coverage-gate` | Pass after patch | Response now writes `final-validation-status.md` in the same bash block and includes config-package validation. |
| `G-final-reviewer-acceptance-block` | Pass | Wrote `Decision: BLOCK` for missing `ACCEPT-K8S-YAML`, `ACCEPT-K8S-ENV`, `ACCEPT-K8S-DEFAULTS`, and `ACCEPT-K8S-CUSTOM-BINDINGS` evidence. |

Replay-driven prompt fixes after generalization:

- Removed hard-coded Flipt/Kubernetes acceptance ID guidance from
  `swe_acceptance_mapper`.
- Replaced Go-specific build/vet language with generic compile-only/lint/source
  inspection evidence classes.
- Strengthened `swe_engineering_worker` to forbid `<think>` or bare shell
  preambles and to show the required fenced completion shape.
- Strengthened `swe_final_validator` to write
  `/tmp/pragma/swe/final-validation-status.md` in the same bash block as the
  validation command.

## 7. Local Validation Commands and Results

Commands run:

```bash
go run ./cmd/pragma orchestration visualize --help
go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact
go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details full
go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details full >/tmp/pragma-swe-loop-visualize.txt
rg -n "swe_acceptance_mapper|acceptance_map|swe_final_validator|swe_final_reviewer|swe_diagnosis_router" /tmp/pragma-swe-loop-visualize.txt
go test ./internal/orchestration -run '^TestNewRuntimeUsesLooplabFSM$'
go test ./internal/query -run 'TestExtractPragmaLoopCommand'
go test ./internal/orchestration
go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep --require-responses
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/A2-acceptance-mapper-stable-ids --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/A2-acceptance-mapper-stable-ids/response.raw --timeout 120s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/C1b-slice-planner-strict-schema --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/C1b-slice-planner-strict-schema/response.raw --timeout 120s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/C2c-slice-planner-acceptance-map-status --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/C2c-slice-planner-acceptance-map-status/response.raw --timeout 120s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/D3-targeted-validator-artifact-gate --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/D3-targeted-validator-artifact-gate/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/E-reviewer-final-validation-gate --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/E-reviewer-final-validation-gate/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/F-final-validator-coverage-gate --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/F-final-validator-coverage-gate/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/G-final-reviewer-acceptance-block --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/G-final-reviewer-acceptance-block/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/B2-theory-keeper-production-handoff --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/B2-theory-keeper-production-handoff/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/D3b-targeted-validator-production-handoff --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/D3b-targeted-validator-production-handoff/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/W2-worker-report-production-handoff --provider lilac --format raw --out .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts/W2-worker-report-production-handoff/response.raw --timeout 180s
go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260612T000001Z-flipt-k8s-production-fsm-prompts --require-responses
```

Results:

- `orchestration visualize --help`: passed and confirmed current command
  shape.
- Compact visualization: passed and showed `swe_repo_survey ->
  swe_acceptance_mapper -> swe_theory_keeper`.
- Full visualization: passed and showed `acceptance_map` in all intended
  downstream handoffs, including planner, worker, targeted validator, reviewer,
  final validator, final reviewer, diagnosis router, scope-expansion, and loop
  repair transitions.
- Focused compile/runtime test:
  `go test ./internal/orchestration -run '^TestNewRuntimeUsesLooplabFSM$'`
  passed.
- Focused shell-action parser test:
  `go test ./internal/query -run 'TestExtractPragmaLoopCommand'` passed,
  including rejection for prose before or after the single fenced bash action.
- Generalized replay audit:
  `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260612T000002Z-generalized-fsm-prompts --require-responses`
  passed after replaying retained old fixtures and writing response metadata for
  raw replay outputs.
- Package-wide `go test ./internal/orchestration` failed on an unrelated
  existing fixture:
  `playful-checklist-loop.yaml` state `parade_next` has `foreach_next
  handoff_path` without a persona. This failure is outside the edited
  SWE-bench Pro orchestration.
- Production replay audit for the new directory passes with response evidence
  for all retained cases. Rejected malformed/old cases remain in the directory
  with responses and verdicts in `cases.json`.
- Declarative runtime-check validation after removing magic artifact-ID
  branches:
  - `go test ./internal/orchestration -run 'TestRequiredOutputCompletionCheck|TestNewRuntimeRejectsInvalidArtifacts'`:
    passed.
  - `go test ./internal/query -run 'TestExtractPragmaLoopCommand|TestPragmaLoopInstancePromptIncludesTaskAndWorkdir|TestRunPragmaLoopBash'`:
    passed.
  - `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`:
    passed.
  - `go test ./internal/query ./internal/orchestration -run '^$'`:
    passed.
  - `go test ./internal/cli ./internal/observe ./internal/app -run '^$'`:
    passed.
  - `git diff --check`: passed.
  - Production hardcoding scan over SWE orchestration/personas and shared
    runtime for `Flipt`, `Kubernetes`, `ACCEPT-K8S`,
    `ACCEPT-KUBERNETES`, `auth.proto`, `METHOD_KUBERNETES`, `mage Proto`,
    `go build ./...`, and `go test ./internal/config`: no matches outside
    tests.
  - Generic runtime special-case scan for old artifact-ID/path branches:
    no matches outside tests.
  - Focused replay of the 20260613 `swe_slice_plan_auditor` failure:
    `.pragma/prompt-ab/20260613T000000Z-slice-plan-auditor-failure/P5-slice-plan-auditor-20260613-no-deliberation`
    failed with `finish_reason="length"`, no content, and repeated reasoning.
  - Same focused replay with `reasoning_effort: none`:
    `.pragma/prompt-ab/20260613T000000Z-slice-plan-auditor-failure/P6-slice-plan-auditor-reasoning-none`
    passed with `finish_reason="stop"`, a bash block writing both required
    artifacts, no reasoning field, and empty `validation_covers_acceptance_ids`.
  - After wiring explicit disabled thinking into the Pragma loop:
    `go test ./internal/query -run 'TestExtractPragmaLoopCommand|TestPragmaLoopInstancePromptIncludesTaskAndWorkdir|TestRunPragmaLoopBash'`
    passed.
  - Provider compile check after `reasoning_effort: none` mapping:
    `go test ./internal/provider/anyllm ./internal/provider/lilac -run '^$'`
    passed.
  - Placeholder validation-command rejection:
    `go test ./internal/orchestration -run 'TestRequiredOutputCompletionCheck|TestNewRuntimeRejectsInvalidArtifacts'`
    passed.

## 8. Fresh SWE-bench Pro Baseline Runs and Evaluator Results

Run in progress:

- Run directory:
  `.pragma/swe-bench-pro/20260612T121012Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Instance:
  `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --max-turns 350 --orchestration /pragma/orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir /pragma/personas-research-v2`
- Status: stopped manually after 94 raw HTTP turns because the run started
  before the generalized prompt patch and was no longer valid as current
  validation evidence.

Observed prompt/FSM issues from this run:

- The first worker read `/tmp/pragma/swe/*` coordination artifacts despite the
  handoff content rule.
- The planner/validator/reviewer counted grep/struct/build evidence as
  validation for input-loading/default/custom-binding acceptance.
- The second planner used an ID absent from the acceptance map
  (`ACCEPT-K8S-SCHEMA`) while the mapper had emitted
  `ACCEPT-K8S-JSON-SCHEMA`.
- The run began before the generalization patch, so it is evidence for
  iteration, not validation of the current local prompts. The obsolete runner
  and orphaned Docker process were terminated after capturing turn-94 evidence.
- By turn 94 the old prompt set had moved into broad runtime auth server
  implementation while config-loading acceptance was still not proven, further
  confirming that the first calibration run should not be treated as a passing
  validation of the generalized FSM/persona set.

Fresh generalized run:

- Run directory:
  `.pragma/swe-bench-pro/20260612T124504Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Instance:
  `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --max-turns 350 --orchestration /pragma/orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir /pragma/personas-research-v2`
- Initial status: running; at first artifact inspection it was still in
  `swe_repo_survey` at 20 raw HTTP turns.
- Stopped after 28 raw HTTP turns because the generalized acceptance mapper
  still collapsed repo-supported config input channels/default/custom binding
  behavior into broader structural/config items. This was a prompt/FSM issue,
  not evaluator evidence.
- Follow-up fix: `swe_acceptance_mapper` now has a generic input-matrix rule
  requiring separate acceptance items for repo-supported input channels,
  defaults, implicit values, auto-detection, fallbacks, custom overrides, and
  structural/schema existence when those can fail independently.

Second fresh generalized run:

- Run directory:
  `.pragma/swe-bench-pro/20260612T125425Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped after 26 raw HTTP turns because the mapper improved to
  include file-input/default acceptance but the preceding survey still did not
  surface environment/custom override channels strongly enough.
- Follow-up fix: `swe_repo_survey` now has an `Input surface matrix` section
  and must inspect loader/testdata/default/custom/compatibility channels for
  input/config-like tasks before handing off to the acceptance mapper.

Third fresh generalized run:

- Run directory:
  `.pragma/swe-bench-pro/20260612T125807Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped after 30 raw HTTP turns. The survey produced an input matrix,
  and the mapper preserved file-input/default acceptance, but ENV/custom
  channels were still omitted because the survey did not explicitly search and
  report those channels.
- Follow-up fix: `swe_repo_survey` now requires explicit environment-binding
  and custom/advanced/default/compatibility discovery for config-like tasks,
  and each channel must be listed as `present`, `absent`, or `unknown`.

Fourth fresh generalized run:

- Run directory:
  `.pragma/swe-bench-pro/20260612T130300Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped after 54 raw HTTP turns. The survey and acceptance mapper
  now preserved the baseline task's independently failing input/config
  surfaces, including file/config structure, defaults, environment binding,
  validation, and fixtures.
- Failure classified: runtime transport and slice-scope enforcement gap. The
  worker emitted prose before a bash block on turn 44 and edited
  `rpc/flipt/auth/auth.proto` even though the active slice only approved
  `internal/config/authentication.go`.
- Follow-up fixes:
  - `swe_engineering_worker` now states that every intermediate and completion
    response must be exactly one fenced bash block, and that necessary writes
    outside `approved_edit_paths` must become a scope request instead of an
    edit.
  - `internal/query/miniswe_loop.go` now rejects shell-action responses that
    contain prose before or after the single bash fence. A response with no bash
    fence is still classified as a final/prose response as before.
  - `internal/query/miniswe_loop_test.go` now covers prose-before-bash and
    prose-after-bash rejection.
- Replay probe:
  `.pragma/prompt-ab/20260612T000002Z-generalized-fsm-prompts/W3-worker-live-approved-path-scope-breach`
  uses the live turn-44 payload with the refreshed worker prompt. The provider
  still emitted prose before the fence, but the content changed from an
  out-of-scope proto edit to a scope-request report. The runtime parser fix is
  therefore required to enforce the transport contract mechanically.

Fifth fresh generalized run:

- Run directory:
  `.pragma/swe-bench-pro/20260612T131620Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped after acceptance-mapper evidence. Do not restart full
  baseline runs for every prompt iteration; use prompt/payload replay first to
  classify direction and only rerun the full task after replay validates the
  changed prompt/payload contract.
- Failure classified: payload-level source gap. The mapper prompt said to use
  the original task prompt, but the mapper state only received
  `repo_survey`; the original task was consumed by the first survey state and
  cleared before the mapper.
- Follow-up fix:
  - `internal/orchestration/runner.go` now preserves the original task prompt
    and passes it to persona states that explicitly declare
    `task_prompt: full`.
  - `orchestrations/swe-bench-pro-engineering-loop.yaml` now sets
    `task_prompt: full` on `swe_acceptance_mapper` only.
  - `swe_acceptance_mapper` now requires `source_quote` for every acceptance
    item and uses stronger shell-envelope instructions.

Acceptance-mapper AB replay sequence:

| Case | Payload | Verdict |
| --- | --- | --- |
| `A4-acceptance-mapper-hypothesis-boundary` | Live mapper payload without original task, refreshed prompt | Partial improvement: removed session-compatible and cleanup truth-value guesses, but still created build/test/path-style acceptance items. |
| `A5-acceptance-mapper-behavior-vs-surface` | Same live payload, stricter behavior-vs-surface prompt | Rejected direction: still created generated/proto/middleware/path items as blocking acceptance. |
| `A6-acceptance-mapper-source-traceability` | Same live payload, added `source_quote` rule | Rejected direction: model fabricated source quotes from survey because the original task was absent from the payload. |
| `A7-acceptance-mapper-source-quote` | Same live payload, structural `source_quote` | Rejected direction: proved prompt-only quote rules are insufficient without original task text in the payload. |
| `A8-acceptance-mapper-with-original-task` | Corrected payload with original task plus survey handoff | Directionally good map: preserved the original requirement bullets, but emitted `<think>` before the bash fence. |
| `A9-acceptance-mapper-task-prompt-shell-clean` | Corrected payload plus shell-hardened mapper prompt | Pass for direction: clean fenced shell action, source-quoted task requirements, repo mechanisms limited to verification fields/notes. |

Downstream source-quoted-map replay sequence:

| Case | Payload | Verdict |
| --- | --- | --- |
| `B3-theory-keeper-source-quoted-map` | A9 acceptance map plus live repo survey | Partial: preserved all source-quoted acceptance IDs, but set next objective to combined proto/config implementation while producer command was still unknown. |
| `B4-theory-keeper-producer-unknown` | Same payload, added generated-producer unknown rule | Partial: recorded producer evidence as unknown but still chose runtime auth-method discovery first. |
| `B5-theory-keeper-producer-priority` | Same payload, added producer-priority rule | Partial: producer evidence remained unknown, but next objective still preferred runtime auth-method discovery. |
| `B6-theory-keeper-planning-constraint` | Same payload, added planning constraint section | Pass for direction: preserved source-quoted map, recorded producer unknown, and set next objective to exact proto regeneration command discovery. |
| `C3-slice-planner-source-quoted-map` | B3 engineering context and acceptance map | Rejected: claimed `go build ./...` covered acceptance IDs while deferring required proto generation. |
| `C4-slice-planner-deferred-producer-coverage` | Same planner payload after deferred-producer coverage rule | Partial: no longer overclaimed validation coverage, but still planned an edit with deferred unknown producer. |
| `C5-slice-planner-generated-producer-discovery` | Same planner payload after generated-producer discovery rule | Rejected: still planned edit and hallucinated a producer command. |
| `C6-slice-planner-overrides-runtime-next-objective` | B5 context with producer unknown but runtime next objective | Rejected: planned runtime discovery with empty acceptance IDs. |
| `C7-slice-planner-producer-discovery-required` | Same payload with invalid empty-acceptance discovery rule | Rejected: named acceptance IDs but still chose runtime auth-pattern discovery before producer discovery. |
| `C8-slice-planner-concrete-producer-trigger` | Same payload with concrete producer trigger | Rejected: buried producer command discovery inside broad auth-pattern discovery. |
| `C9-slice-planner-prescribed-producer-shape` | Same payload with prescribed producer-discovery shape | Rejected: still followed runtime-focused context. |
| `C10-slice-planner-b6-producer-context` | B6 producer-focused context and source-quoted map | Pass for direction: planned proto-regeneration discovery, named `ACCEPT-K8S-AUTH-METHOD` and `ACCEPT-K8S-INTROSPECTION`, left validation coverage empty, and required exact producer command or blocker. |
| `W4-worker-producer-discovery-slice` | B6 context plus C10 slice plan | Rejected: fabricated discovery findings and completed without running repository inspection. |
| `W5-worker-discovery-requires-command` | Same worker payload after discovery-command rule | Pass for direction: first action is read-only producer discovery over build files, scripts, proto source, and generated output headers; no `/tmp/pragma/swe/*` reads and no edits. |
| `W6-worker-producer-discovery-report` | Real W5 command output showing `magefile.go` `Proto()` wraps `buf generate` and auth enum values | Partial: worker requested one more read-only inspection of Buf configuration instead of fabricating completion. |
| `W7-worker-producer-discovery-buf-report` | Added real Buf configuration output: root `buf.gen.yaml` plugins and output path `rpc/flipt` | Partial: worker requested one more read-only inspection of workspace/public Buf config and dry-run feasibility. |
| `W8-worker-producer-discovery-final-report` | Added real workspace/public Buf output | Partial: worker requested final confirmation of generated header and mage function. |
| `W9-worker-producer-discovery-complete` | Added final real producer-discovery output | Pass for direction: worker wrote a no-edit discovery report, preserved acceptance IDs, kept validation coverage empty, set `task_completion_claim_allowed: false`, and recorded producer coupling. |
| `D4-targeted-validator-producer-discovery-report` | W9 worker report plus C10 plan | Pass for direction: validator wrote `Result: insufficient`, kept validated IDs empty, and required auth.proto edit plus producer run before validation can pass. |
| `E2-reviewer-producer-discovery-route` | D4 targeted validation plus W9 worker report | Pass for direction: reviewer wrote `decision: continue_implementation`, listed all blocking IDs as unvalidated, set `next_owner: swe_theory_keeper`, and kept `final_validation_ready: false`. |
| `B7-theory-keeper-after-producer-review` | E2 loopback with theory keeper still owning acceptance-map writes | Partial: preserved producer evidence but rewrote `required_validation`, proving theory should not own acceptance-map mutation. |
| `B8-theory-keeper-preserve-required-validation` | Same payload with prompt-only guard against narrowing validation | Rejected: still narrowed `required_validation`, confirming need for a dedicated acceptance auditor. |
| `B9-theory-keeper-context-only-after-producer-review` | E2 loopback with theory keeper stripped of acceptance-map write responsibility | Pass: wrote only `engineering-context.md`, preserved producer evidence, and did not rewrite `/tmp/pragma/swe/acceptance-map.json`. |
| `H1-acceptance-auditor-after-producer-review` | B9 context plus original acceptance map, W9/D4/E2 evidence, and explicit input hashes | Pass: preserved all IDs and `required_validation` fields, wrote exact input byte/SHA ledger, and kept discovery-only items pending. |
| `C11-slice-planner-after-producer-known` | B7 map/context with producer known | Rejected: chose narrow proto edit but overclaimed both proto-related acceptance IDs as validated. |
| `C12-slice-planner-prereq-coverage` | Same payload after prerequisite-coverage prompt rule | Partial: removed the coverage overclaim but omitted the known producer command from `targeted_validation`. |
| `C13-slice-planner-producer-validation-command` | Same payload after producer-command prompt rule | Rejected: restored producer command but again overclaimed whole acceptance IDs. |
| `C14-slice-planner-after-acceptance-audit` | H1 audited map and B9 context | Rejected: planned the right proto edit and producer command but still overclaimed `ACCEPT-K8S-AUTH-METHOD` validation coverage. |
| `C15-slice-planner-all-surface-coverage` | Same payload with stricter planner coverage rules | Rejected: still overclaimed partial coverage and emitted `<think>` before the shell action. |
| `P1-slice-plan-auditor-removes-overclaim` | Overclaiming C14 plan plus H1 audit/map | Pass: rewrote `validation_covers_acceptance_ids` to empty while preserving the narrow proto edit and `mage Proto && go build ./...` command. |
| `H2-acceptance-auditor-environment-specific-tools` | Acceptance auditor prompt with environment-specific tool warning | Partial: preserved the map, but did not explicitly add the current-runner tool warning needed by downstream planning/work. |
| `P2-slice-plan-auditor-environment-specific-tools` | Slice-plan auditor prompt with stale-tool warning but no current `environment_context` artifact | Partial: removed validation coverage overclaim and kept `mage Proto && go build ./...`, but still lacked a current-runner tool source and removed `ACCEPT-K8S-INTROSPECTION` from `acceptance_ids`. |
| `ENV1-environment-survey-preflight` | New `swe_environment_survey` persona with repo survey and acceptance map | Pass: wrote a clean bash preflight that records current runner, PATH, tool availability, repo tooling signals, and producer preflight into `environment-context.md`. |
| `P3-slice-plan-auditor-with-current-env` | P2 plus synthetic current-runner `environment_context` from real toolchain preflight | Rejected: environment context was visible, but the response leaked `<think>` and reintroduced `ACCEPT-K8S-AUTH-METHOD` validation overclaim. |
| `P4-slice-plan-auditor-current-env-output-gate` | P3 plus final payload output gate and concrete surface/validation coverage rule | Pass: started with bash, recorded the `environment_context` hash, preserved `mage Proto && go build ./...`, and removed all validation coverage overclaim. |
| `W10-worker-audited-proto-edit-first-action` | B9/H1/P1 audited proto-edit handoff | Rejected: completed a worker report from handoff facts instead of editing or running a real command. |
| `W11-worker-edit-slice-execution-required` | Same worker payload with edit-slice execution rule | Rejected: claimed a tool check was attempted without actually running it in the worker phase. |
| `W12-worker-edit-first-response-command` | Same worker payload with first-response command rule | Rejected: converted the edit slice into a discovery report, fabricated phase-local inspections, and did not edit. |
| `W13-worker-edit-with-current-env` | W12 plus synthetic current-runner `environment_context` | Partial: chose the right first repository action against `auth.proto`, but leaked `<think>` before the bash block. |
| `W14-worker-edit-current-env-output-gate` | W13 plus final payload output gate | Pass: started with bash and first edit-slice action was a real target read of `auth.proto`, not a completed report. |
| `W15-worker-edit-after-auth-proto-read` | W14 conversation plus real enum output from `auth.proto` | Pass: edited only `auth.proto` to add `METHOD_KUBERNETES = 3` and verified the enum block. |
| `W16-worker-run-producer-after-proto-edit` | W15 conversation plus edited enum output | Pass: ran `mage Proto` in `/app` instead of completing from edit evidence alone. |
| `W17-worker-build-after-producer-success` | W16 conversation plus producer success output | Partial: inserted an extra generated-symbol check before build; acceptable intermediate, but not sufficient validation. |
| `W18-worker-build-after-generated-symbol` | W17 conversation plus generated enum-symbol output | Pass: ran `go build ./...` with status capture into `worker-validation.log`. |
| `W19-worker-report-after-build-success` | W18 conversation plus successful build output | Partial: wrote a good no-completion-claim report, but omitted generated `auth.pb.go` from `Changed files`. |
| `W19b-worker-diff-before-report` | W19 payload with refreshed worker prompt requiring diff authority | Pass: ran `git diff --name-only` before reporting changed files. |
| `W20-worker-report-with-generated-diff` | W19b conversation plus diff output showing source and generated file | Pass: report carried both `rpc/flipt/auth/auth.proto` and `rpc/flipt/auth/auth.pb.go`, left validation coverage empty, and kept `task_completion_claim_allowed: false`. |
| `D5-targeted-validator-after-proto-edit-worker-report` | W20 worker report plus P4 audited slice plan | Partial: reported `Result: insufficient` with no validated IDs, but reran producer/build and hardcoded success status. |
| `D6-targeted-validator-uses-worker-command-evidence` | Same payload after worker-command evidence rule | Partial: still reran producer/build despite worker evidence and wrote static success text. |
| `D7-targeted-validator-evidence-first-gate` | Same payload after evidence-first gate and status-template fix | Pass: wrote targeted validation from worker evidence without rerunning, carried both changed files, validated no IDs, and returned `Result: insufficient`. |
| `E3-reviewer-after-proto-edit-insufficient` | D7 targeted validation plus W20 worker report | Pass: routed `continue_implementation`, kept `final_validation_ready: false`, and listed proto-related IDs as blocking/unvalidated. |
| `W22-worker-command-evidence-contract` | Current runtime/prompt contract derived from the 20260612T154716Z worker edit turn | Pass: refreshed worker payload returned one fenced bash block with a real repository verification command and did not write a report-only completion from handoff facts. |

Replay-driven fixes from this sequence:

- `internal/orchestration/runner.go` preserves the original task prompt for
  states that explicitly request `task_prompt: full`.
- `swe_acceptance_mapper` now emits source-quoted, task-derived acceptance
  items and has a strict shell envelope.
- `swe_theory_keeper` preserves extra acceptance-map fields such as
  `source_quote`, records `producer unknown`, and emits a planning constraint
  when generated artifacts need an unproven producer command.
- `swe_slice_planner` treats unknown generated producers as a discovery
  prerequisite, avoids validation overclaiming when producer work is deferred,
  and rejects empty-acceptance discovery while blocking items are pending.
- `swe_engineering_worker` must run actual read-only repository inspection for
  discovery slices before claiming findings, and it must preserve
  `slice_plan.acceptance_ids` in the worker report.
- The worker, targeted validator, and reviewer now pass the producer-discovery
  baseline at the prompt/payload level: discovery evidence does not become
  acceptance validation, and the review route returns to theory/planning for
  the first edit slice.
- Persona responsibility is now split where replay showed drift:
  `swe_theory_keeper` owns prose context only, `swe_acceptance_auditor` owns
  acceptance-map preservation/mutation and handoff hashes, `swe_slice_planner`
  proposes the next slice, and `swe_slice_plan_auditor` owns coverage-claim
  correction before worker execution.
- A current-runner `swe_environment_survey` state now follows acceptance
  mapping. Its `environment_context` artifact is handed through planning,
  work, validation, review, scope, final-validation, and diagnosis loopbacks so
  stale tool facts from another shell/image/run cannot silently steer the next
  persona.
- The orchestration prompt builder now appends a final response output gate to
  persona-state user payloads. P4/W14 showed this payload-level gate fixes
  `<think>` leakage without changing persona responsibilities.
- `swe_slice_plan_auditor` now has a concrete surface/validation gate: if an
  acceptance item names a required repo surface outside the approved edit or
  generated-output envelope, or the targeted command omits a required
  validation command, the auditor must remove that ID from
  `validation_covers_acceptance_ids`.
- `swe_engineering_worker` now treats `git diff --name-only` as the authority
  before completing edit/refactor/producer reports. This specifically protects
  generated output paths from being dropped between worker and validator
  handoffs.
- `swe_targeted_validator` now has an evidence-first gate: when the worker
  report already contains command/status and changed-file evidence, the
  validator writes from that evidence instead of rerunning producer/build
  commands. Its command template now requires captured shell variables in
  reports when reruns are actually necessary.
- The worker evidence contract is now machine-assisted rather than prompt-only:
  the runtime writes `worker-command-evidence.jsonl` for worker shell turns, and
  worker report completion is rejected when report claims are not backed by
  prior non-completion worker command evidence.

Fresh runtime iteration after W20/D7/E3:

- Run directory:
  `.pragma/swe-bench-pro/20260612T154716Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped before evaluator completion.
- Good signal: the mapper, environment survey, theory keeper, acceptance
  auditor, planner, plan auditor, first worker, targeted validator, and
  reviewer advanced through the first discovery cycle. The worker discovery
  report was handed forward and the reviewer routed back to theory/planning.
- Failure classified: later theory/context text corrupted `Flipt` to `Flixt`,
  and the acceptance auditor copied that corruption into an acceptance map
  source quote. Follow-up runtime fix threads the original task prompt into
  acceptance-map checks for every persona-backed state, so source-quote
  corruption is rejected regardless of which persona writes the map.
- Secondary risk classified: worker reports still depended on self-authored
  prose to tell later personas what happened. The latest change adds a
  runtime-authored `worker-command-evidence.jsonl` handoff so report claims can
  be checked against actual worker shell turns.

Interrupted validation run:

- Run directory:
  `.pragma/swe-bench-pro/20260612T160356Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped on user request before useful result collection.
- Next action before another full run: focused compile/visualize validation and
  prompt/payload AB for the worker evidence contract.

Fresh run after declarative artifact checks:

- Run directory:
  `.pragma/swe-bench-pro/20260613T051112Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --output-dir .pragma/swe-bench-pro/20260613T051112Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --provider lilac --model minimaxai/minimax-m2.7 --orchestration /pragma/orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir /pragma/personas-research-v2 --generator-toolchain --evaluate`
- Agent status: failed before final answer with
  `state "swe_slice_plan_auditor" failed: model returned no bash action after 3 retries`.
- Evaluator status: ran despite agent status 1; `eval/eval_results.json`
  reports the instance as `false`, and the wrapper printed overall accuracy
  `0.0`.
- Good signal: the mapper produced source-quoted acceptance, the environment
  survey and handoff audit flowed through the loop, declarative acceptance-map
  and worker-report checks did not require artifact-ID runtime branches, and
  runtime-authored worker command evidence was captured and used by worker
  report completion.
- Failure classified: `swe_slice_plan_auditor` correctly identified that the
  current slice's `targeted_validation` did not match the acceptance map's
  required commands, but it repeated that reasoning until the response hit the
  model length limit instead of writing the audited `slice-plan.json` and
  `plan-audit.md` artifacts.
- Evaluator failure surface: selected evaluator tests ran and `TestLoad`
  failed for Kubernetes config loading/defaults. The log showed the patch's
  expected advanced YAML/ENV config included Kubernetes enabled, issuer, CA
  path, token path, and cleanup, while the actual loaded config kept
  Kubernetes disabled with empty fields. This confirms the run had not solved
  config input binding/default behavior.
- Follow-up fixes applied:
  - `swe_slice_plan_auditor` now has a bounded three-step decision procedure
    and explicitly treats "explaining the correction without writing
    `/tmp/pragma/swe/slice-plan.json` and `/tmp/pragma/swe/plan-audit.md`" as
    invalid output.
  - Pragma shell-action loop turns now explicitly disable reasoning by default,
    which maps to `reasoning_effort: none` for Lilac/OpenAI-compatible
    requests.
- Validation after follow-up fix:
  `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed.
- Focused replay after prompt-only fix still failed:
  `.pragma/prompt-ab/20260613T000000Z-slice-plan-auditor-failure/P5-slice-plan-auditor-20260613-no-deliberation`
  returned HTTP 200 but `finish_reason="length"`, no content, and repeated
  hidden reasoning.
- Focused replay after transport-level reasoning suppression passed:
  `.pragma/prompt-ab/20260613T000000Z-slice-plan-auditor-failure/P6-slice-plan-auditor-reasoning-none`
  returned HTTP 200, `finish_reason="stop"`, no reasoning field, a single bash
  block writing `slice-plan.json` and `plan-audit.md`, and
  `validation_covers_acceptance_ids: []`.

Fresh run after reasoning suppression:

- Run directory:
  `.pragma/swe-bench-pro/20260613T060458Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped before agent status because it was invalid as current
  validation evidence.
- Good signal: early states no longer produced reasoning-only/length failures;
  raw turns around the repo survey, mapper, environment survey, and acceptance
  auditor returned normal content after a transient provider 502 sequence.
- Failure classified: the acceptance mapper wrote
  `required_validation: ["unknown"]` for all ten acceptance items, and the
  acceptance auditor preserved those placeholders as `required_validation_preserved: yes`.
  That weakened the acceptance map enough that the remaining run could not
  prove evaluator-facing behavior even if it continued.
- Follow-up fix applied: generic `json_each_behavior_validation_commands`
  integrity checks now reject placeholder validation commands including
  `unknown`, `none`, `todo`, and `tbd`.

### 2026-06-13T061430Z Fresh Run, Stopped as Invalid Evidence

- Run directory:
  `.pragma/swe-bench-pro/20260613T061430Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped before agent status because the acceptance map again emitted
  `required_validation: ["unknown"]` for all ten acceptance items.
- Important distinction from the previous stopped run: the runtime integrity
  check likely rejected the submitted artifact and asked for repair, but the
  acceptance mapper prompt still explicitly allowed `unknown` and its required
  JSON template showed `required_validation: ["<command or unknown>"]`. That
  contradiction let the model repeat the invalid artifact instead of repairing
  it.
- External noise: the provider then entered repeated retryable 502 responses,
  so continuing the run would not produce useful evaluator evidence.
- Follow-up fix applied: `swe_acceptance_mapper` no longer permits placeholder
  validation in `required_validation`, and `swe_acceptance_auditor` treats
  placeholder `required_validation` entries as corruption to replace with
  concrete behavior-proof descriptions or commands.
- Verification after fix:
  `go test ./internal/orchestration -run 'TestRequiredOutputCompletionCheck|TestNewRuntimeRejectsInvalidArtifacts' -count=1`
  passed, and `go run ./cmd/pragma orchestration visualize
  orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir
  personas-research-v2 --details compact` passed.

### 2026-06-13T061938Z Fresh Run, Stopped on Provider Instability

- Run directory:
  `.pragma/swe-bench-pro/20260613T061938Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Status: stopped manually with no `agent-status.txt`, prediction patch, or
  evaluator output. The run was inside `swe_targeted_validator` when stopped.
- Provider blocker evidence: `pragma.stderr.log` records repeated Lilac
  failures during the run, including 502 "All upstream targets failed", 500
  `EngineCore` errors, and a client timeout. The final validator call reached
  attempt 10 after a sequence of retryable 502/500/timeout errors, so the run
  could not produce trustworthy fresh evaluator evidence.
- Positive runtime evidence before the provider blocker:
  - The acceptance mapper no longer emitted placeholder
    `required_validation: ["unknown"]`; it emitted concrete behavior-proof
    descriptions and passed the declared integrity checks.
  - The prior `swe_slice_plan_auditor` reasoning-only failure did not recur;
    the auditor emitted normal bash artifacts.
  - The discovery slice correctly identified `cd /app && buf generate` as the
    proto producer and kept all acceptance IDs unvalidated.
  - The next implementation slice exposed a real generated-code/tooling
    problem: `buf generate` rewrote proto outputs with Go 1.20+ constructs
    incompatible with the repo's `go 1.18`, so targeted validation remained
    insufficient.
- New product/run risk found by the live run: the worker accepted manual
  `auth.pb.go` patching as a workaround after `buf generate` broke
  compatibility, then subsequent validation became blocked by the generated
  file state. A future run should either prevent manual generated-file
  workarounds unless explicitly approved by scope, or route this as a
  producer/toolchain scope decision before allowing more feature work.
- Follow-up prompt replay from the exact worker failure turn:
  - P1 added worker-only generated-output wording; failed because the model
    still chose manual generated-file patching.
  - P2/P3 added prior-producer-failure and no-alternate-producer wording; the
    model stopped manual patching but tried alternate producer commands, which
    is still outside the audited slice contract.
  - P4 added the same prohibition to the handoff Worker Contract as the slice
    auditor would emit in a fresh run; after provider retries, the best
    response still continued repository/toolchain inspection instead of writing
    the blocker report.
  - P5 made the post-producer-failure next action explicit; failed because the
    model still ran another producer/debug command.
  - P6 moved the intervention earlier to the edit-mode worker turn immediately
    after the approved source edit was confirmed; failed because the worker
    still ran the producer command.
  - P7 added a handoff Worker Contract saying targeted validation owns
    producer/build execution; failed because other slice-plan fields still
    encoded producer/build as the worker stop condition.
  - P8 changed the audited slice stop condition to the worker-phase stop and
    left producer/build in `targeted_validation`; after retry, this passed for
    the worker turn. The worker wrote `worker-report.md`, did not run
    producer/build, did not touch generated output, preserved empty validation
    coverage, and marked targeted validation as still required.
  - P9 tested whether the slice-plan auditor would emit that boundary from the
    captured auditor turn; failed because it still preserved producer/build as
    worker work.
  - P10 tested exact Worker Contract labels and stop-condition rewrite rules;
    blocked by repeated provider 502 responses.
  - P11 used a shorter version of P10; failed because it emitted the exact
    labels but still said the worker must run producer/build.
  - P12 added a first-class `worker_stop_condition` field to the slice-plan
    contract; partial because it emitted the field but still mentioned build
    success in the worker stop condition.
  - P13 tightened `worker_stop_condition` so edit/refactor worker stops cannot
    require producer/build/lint/test success when those commands remain in
    `targeted_validation`; passed. The auditor emitted a clean
    `worker_stop_condition` that stops after confirming the approved edit and
    leaves producer/build pending for targeted validation.
  - P14 tested whether the worker consumes a P13-style
    `worker_stop_condition`; blocked by repeated provider 502 responses.
  - P15 tested whether targeted validation rejects the original manual
    generated-output patching workaround instead of suggesting it as a future
    path; blocked by repeated provider 502 responses.
  - Additional retry after the P13 pass still returned provider 502 for both
    P14 and P15. Replay metadata now records these as
    `response_transport_error` cases, while P8 and P13 remain `response_ok`.
  - P16 reduced P14 handoff bulk while preserving the prior edit-confirmation
    history; still blocked by provider 502.
  - P17 reduced the worker replay further to a minimal no-history payload; it
    returned 200 but chose a source-read confirmation before reporting, so it
    was partial and not promoted.
  - P18 added the resulting source-confirmation output to the minimal worker
    payload; passed. The worker wrote `worker-report.md`, did not run
    producer/build, did not edit generated output, and left validation pending
    for targeted validation.
  - P19 reduced the targeted-validator replay to the manual-generated-output
    failure surface; passed. The validator wrote `Result: insufficient`,
    marked scope as `needs_expansion`, identified the manual generated-output
    patch as violating `generated_policy`, and told the next actor to fix and
    rerun the producer rather than manually patch generated output.
  - Replay metadata and `source.txt` hypothesis notes were added under
    `.pragma/prompt-ab/20260613T000000Z-worker-generated-output-policy/`.
  - Current status: the auditor boundary (P13), worker boundary after source
    confirmation (P18), and targeted-validator rejection of manual generated
    output (P19) have passing focused evidence. Exact full-context P14/P15
    remain provider-blocked, but smaller successor payloads now cover the same
    contract surfaces. A fresh full run can be considered only after a static
    audit confirms these contracts are represented in production prompts
    without baseline leakage.

Fresh full-run regression and AB follow-up:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T072406Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  was stopped after the generated-output policy failed through a different
  route: targeted validation suggested generated-file manual repair, reviewer
  requested generated-file scope, scope expander approved it, theory keeper
  preserved it as durable context, and the slice auditor converted it into a
  worker contract.
- Regression class: generated-output workaround laundering through
  validator/reviewer/scope/theory/auditor handoffs. This was not a generic
  runtime issue; it belonged in the persona/FSM contract layer.
- New replay directory:
  `.pragma/prompt-ab/20260613T000001Z-generated-policy-scope-laundering`.
- P20 replayed the captured targeted-validator turn after prompt tightening;
  passed. The validator now writes the deterministic scope signal:
  source-of-truth or producer/toolchain repair required; generated output hand
  edits are not a valid validation recommendation.
- P21 replayed the captured reviewer turn; passed after tightening. It now
  chooses `revise_theory`, leaves `scope_request.paths` empty, and rejects the
  generated-output hand-edit request as a producer/toolchain issue.
- P22 replayed the captured scope-expander turn; passed. It rejects generated
  output edit scope and routes to source-of-truth/toolchain/producer repair.
- P23 replayed the captured theory-keeper turn; passed. It no longer preserves
  prior generated-file scope approval as durable policy and records the manual
  generated-output workaround as rejected.
- P24 replayed the captured slice-plan-auditor turn; passed behaviorally. It
  rewrites the generated-file patch plan into a no-edit discovery/blocker slice
  with `approved_edit_paths: []`, `targeted_validation: "none"`, and
  `Worker may edit: none`. The response mentions the rejected prior policy only
  as a rejected input, not as an approved repair.

Second fresh-run regression and AB follow-up:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T075933Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  was stopped after targeted validation laundered a failed build into partial
  acceptance. The worker obeyed the boundary and did not run producer/build or
  touch generated files, but targeted validation then ran `buf generate &&
  go build ./...`, saw build failure, ran `git stash`, claimed the failure was
  pre-existing/toolchain-only without baseline evidence, and marked config/proto
  acceptance IDs validated. Reviewer and theory keeper propagated that claim.
- Regression class: failed-validation laundering through validator,
  reviewer, and theory keeper. Required generic rule: failed or insufficient
  targeted validation cannot validate planned acceptance IDs without direct
  passing evidence for each ID, and "pre-existing/toolchain issue" requires
  baseline command evidence from before worker edits.
- New replay directory:
  `.pragma/prompt-ab/20260613T000002Z-failed-validation-laundering`.
- P25 replayed the captured targeted-validator failure turn after prompt
  tightening; passed. The validator writes `validated_acceptance_ids: []`,
  keeps all planned IDs insufficient, does not run `git stash`, and treats the
  failed build as a validation blocker requiring producer/toolchain diagnosis.
- P26 full reviewer replay remained provider-blocked by repeated 502s, so P28
  used a reduced payload preserving the same contradictory targeted-validation
  surface. P28 passed: reviewer chooses `revise_theory`, clears `validated`,
  and puts all planned IDs in `blocking_unvalidated`.
- P27 full theory replay hit provider length/transport failure, so P29 used a
  reduced payload preserving the bad reviewer claim and failed validation.
  P29 passed: theory keeper keeps IDs insufficient, rejects the unsupported
  pre-existing/toolchain claim, and makes the next objective build-failure
  diagnosis rather than promoting validation.

Third fresh-run regression and AB follow-up:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T082028Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  was stopped after the worker breached the audited Worker Contract. The slice
  auditor approved a consumer-file-only edit while the expected observable still
  required cross-file method/registration participation. The worker then ran a
  validation-owned build command, saw a missing source-of-truth symbol, edited an
  out-of-scope source-of-truth file, ran the producer, and continued with build
  commands.
- Regression class: worker-contract/source-of-truth boundary breach. Required
  generic rule: a missing source-of-truth declaration outside
  `approved_edit_paths` is a scope signal, not permission to widen edits or keep
  probing. The auditor must reject or narrow consumer-only slices whose
  objective, expected observable, or stop condition depends on unapproved
  cross-file contracts.
- New replay directory:
  `.pragma/prompt-ab/20260613T000003Z-worker-contract-source-of-truth-boundary`.
- P30/P32 exact worker replays failed: the model still edited the out-of-scope
  source-of-truth path after the missing-symbol output. P36 reduced worker
  replay improved to read-only probing but still violated the required stop
  point. P38 reduced worker replay passed for the corrected rule: it wrote
  `worker-report.md`, preserved the scope blocker, did not run another command,
  and did not edit the out-of-scope path.
- P31/P35 exact/reduced auditor replays failed or partially failed: they
  preserved a consumer-only plan while retaining method/registration observable
  text and loose stop conditions. P37 narrowed the observable and recorded the
  scope correction. P39 passed after the stop-condition rule: it removed
  cross-file method/registration work, added the source-of-truth/generated paths
  as suspected coupling, and changed the stop condition to worker-phase stop
  language with targeted validation pending.
- Provider/transport notes: P33 hit a 500, P34 hit a 502, and one P35 exact
  replay was cancelled after hanging. The useful passing evidence for the final
  prompt text is P38/P39.

Fourth fresh-run regression and AB follow-up:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T084204Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  was stopped in `swe_environment_survey`. The environment survey put package
  test/build probes inside shell command substitutions while writing
  `environment-context.md`; the first package test was still compiling after
  nearly two minutes and blocked the artifact write.
- Regression class: unbounded environment preflight. Required generic rule:
  environment survey is a read-only current-runner fact capture, not acceptance
  validation. Build, test, lint, producer, and dependency-resolution probes must
  be skipped or bounded with timeout/status/log sampling, and must not be placed
  inside command substitutions that can block artifact creation.
- New replay directory:
  `.pragma/prompt-ab/20260613T000004Z-environment-preflight-bounds`.
- P40 replayed the captured environment-survey turn after prompt tightening.
  The first attempt hit provider 500; retry passed. The response writes a
  compact environment artifact from tool/version and manifest checks, records
  producer-tool availability and constraints, and does not run unbounded build or
  test probes.

Post-P40 full-run attempt:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T084741Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  did not reach the first persona response. `pragma.stderr.log` records seven
  consecutive Lilac 502 responses before any `swe_repo_survey` output; the run
  was manually stopped during retry/backoff. No evaluator output or useful
  orchestration regression was produced.

Fifth fresh-run regression and AB follow-up:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T085521Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  reached the first worker slice and was stopped after the worker executed
  producer-discovery commands despite an audited Worker Contract that assigned
  producer/build/lint/test work away from the worker.
- Regression class: producer-discovery execution breach. The worker correctly
  needed to discover the proto producer command and output paths, but after the
  command/config/output paths were known it still ran `buf generate --dry-run`
  and actual `buf generate` variants. A no-producer discovery slice must stop at
  report-writing once the command and outputs are identified.
- The suspected acceptance-ID drift was not the actual failure in this run. The
  live acceptance map used `ACCEPT-K8S-AUTH-METHOD-RECOGNIZED`; the failure was
  command ownership and stop-condition compliance.
- New replay directory:
  `.pragma/prompt-ab/20260613T000006Z-producer-discovery-no-execution`.
- P41 exact worker replay failed: the worker still pursued producer/help/config
  command variants instead of writing the report.
- P42 exact slice-auditor replay passed for the needed contract shape: the
  Worker Contract says the worker may edit no files, must run no producer,
  build, lint, or test commands, must not run `buf generate` or proto
  regeneration, and must report a blocker if the producer command/config cannot
  be identified.
- P43 exact worker replay failed after the worker prompt was tightened: despite
  the command being known, the response tried to "test actual buf generate
  command" and ran the producer.
- P44 reduced worker replay passed: with a generic known producer command and a
  no-producer Worker Contract, the worker wrote `worker-report.md`, preserved
  acceptance IDs, claimed no validation coverage, and did not run another
  command.
- P45 exact worker replay failed after moving the no-producer hard stop to the
  top of the worker prompt: the worker still ran another read/help/config probe
  instead of writing the report.
- P46 exact slice-auditor replay passed after adding the route contract: the
  corrected `slice-plan.json` includes `worker_track: "discovery"` and
  `plan-audit.md` records `worker_track: discovery` while preserving the
  no-producer Worker Contract.
- P47 exact worker replay with a dedicated discovery-worker prompt failed: it
  still tried to test the discovered producer command with explicit config.
- P48 exact discovery-worker replay failed after the identification-vs-proof
  rule: it inspected generated output and then ran a producer command.
- P49 exact discovery-worker replay after the no-output-polishing rule was
  provider-blocked twice by Lilac 502 and produced no usable model content.
- P50 exact discovery-worker replay after the one-inspection rule still failed:
  it inspected generated output and ran producer commands. This proves
  prompt-only worker separation is insufficient against the sticky history.
- Promoted fix after P50: add a generic declarative shell policy to the
  Pragma shell loop and enable it only on the discovery-worker state. The
  runtime policy is task-agnostic and configured by YAML deny regexes; it does
  not mention the baseline task, repo, state artifacts, or persona-specific
  report semantics.
- Current conclusion: the auditor/FSM route now separates no-edit discovery
  from implementation, and generic declared shell policy is required to enforce
  forbidden command families when the model ignores the discovery stop.

Post-policy fresh-run attempt:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T092557Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  was manually stopped after `swe_engineering_reviewer` routed back to
  `swe_theory_keeper`. The wrapper exited 137 because the container was stopped
  from the host.
- Initial suspicion was acceptance-ID drift because this run used
  `ACCEPT-K8S-AUTH-RECOGNIZED` instead of the earlier
  `ACCEPT-K8S-AUTH-METHOD-RECOGNIZED`. Inspection showed that was not an
  intra-run drift: the live acceptance mapper emitted
  `ACCEPT-K8S-AUTH-RECOGNIZED`, and downstream personas preserved that ID.
- Useful evidence from the stopped run:
  - `route_worker_track` emitted `discovery`.
  - `swe_discovery_worker` ran read-only discovery and did not execute the
    producer command.
  - The worker report identified the producer command as `buf generate` and
    named the generated outputs while leaving validation coverage empty.
  - `swe_targeted_validator` treated the discovery slice as not applicable for
    validation, and `swe_engineering_reviewer` routed to continue implementation.
- Remaining issue from this attempt: the run was stopped before the next
  implementation slice, so evaluator outcome remains unproven. The discovery
  worker also ran multiple read-only inspection commands despite the
  one-inspection prompt rule; this did not violate the producer-execution
  boundary, but the next full run should be inspected for repeated read-only
  churn if it delays progress.

Follow-up fresh-run attempt:

- A fresh run at
  `.pragma/swe-bench-pro/20260613T093115Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
  was manually stopped while the first implementation worker was active. The
  wrapper exited 137 because the container was stopped from the host.
- Useful evidence from the run:
  - The planner chose an initial auth-framework discovery slice because prior
    context still contained an `internal/authn/` path discrepancy.
  - The discovery worker completed without edits and without producer
    execution, resolving the auth framework path to `internal/server/auth/`.
  - The acceptance auditor updated repo surfaces from `internal/authn/` to
    `internal/server/auth/` based on the discovery report.
- Regression found before the stop: the slice-plan auditor emitted an
  internally contradictory implementation handoff for
  `k8s-auth-config-proto`. Its Worker Contract said targeted validation owns
  `buf generate` and `go build` after the worker report, but
  `worker_stop_condition` still required proto regeneration completion and
  `stop_condition` still required `buf generate`/`go build` success.
- Exact evidence is in
  `.pragma/prompt-ab/20260613T093115Z-live-worker-multifence-format/dump`,
  especially `turn-000056/response_content.md`.

Focused AB for the worker-stop ownership regression:

- New replay directory:
  `.pragma/prompt-ab/20260613T000007Z-worker-stop-targeted-validation-ownership`.
- P51 replayed the exact `turn-000056` auditor request with only the refreshed
  system prompt after adding a buried final consistency rule. It failed: the
  response still preserved `worker_stop_condition` requiring proto regeneration
  completion and `stop_condition` requiring `buf generate`/`go build` success.
- P52 replayed the same exact request after moving the generic ownership
  invariant to the top of the auditor prompt and making it override planner
  stop-condition preservation. It passed: both stop fields were rewritten to
  worker-phase completion with targeted validation pending, while the Worker
  Contract kept `buf generate` and `go build` owned by targeted validation.
- The production fix is generic: it does not mention Flipt, Kubernetes,
  baseline files, acceptance IDs, or any benchmark-specific command. It states
  the ownership rule for producer/build/lint/test/generate/compile/validation
  command families whenever targeted validation owns them after the worker
  report.

Fresh full run after P52:

- Run directory:
  `.pragma/swe-bench-pro/20260613T095107Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Agent status: `1`. Terminal error:
  `state "swe_slice_plan_auditor" failed: model returned no bash action after 3 retries`.
- Evaluator status: ran after the agent failure and reported
  `{"instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446": false}`
  / overall accuracy `0.0`.
- Useful positive evidence:
  - Discovery route still separated no-edit producer discovery from
    implementation and did not execute the producer in the discovery worker.
  - The P52 ownership invariant held on later edit slices: producer/build
    commands stayed in targeted validation, and workers reported after
    source-edit phases.
  - The loop made forward progress through proto enum addition, config struct
    addition, proto service scope expansion, generated binding validation, and
    a module-version repair before the final blocker.
- Current blocker exposed by this run:
  - The slice-plan auditor repeatedly hit provider `finish_reason=length` on
    the `k8s-auth-server-implementation` slice after context growth. Turns
    143, 144, 145, and 146 in the dumped raw HTTP evidence have empty extracted
    `response_content.md`; the wrapper then failed the state after retries.
  - Raw dump:
    `.pragma/prompt-ab/20260613T095107Z-live-slice-auditor-length-blocker/dump`.
  - This is not an evaluator-facing pass or task blocker: the agent failed
    before final validation or final answer.
- Patch surface at failure was large and incomplete: `go.mod`, `go.sum`,
  `internal/config/authentication.go`, `rpc/flipt/auth/auth.proto`, and many
  generated protobuf files under `rpc/flipt/auth/`, `rpc/flipt/`, and
  `rpc/flipt/meta/`; no `internal/server/auth/method/kubernetes/` server
  implementation was produced.
- Next correction should target auditor output budget and context handling for
  late-run large handoffs. The production fix should remain generic and should
  not encode the Kubernetes/auth/proto slice.

Focused AB for the late-run auditor length blocker:

- New replay directory:
  `.pragma/prompt-ab/20260613T000008Z-slice-auditor-length-fastpath`.
- P53 replayed the exact late-run `turn-000143` slice-plan-auditor request
  with only the refreshed system prompt. The failed live request had
  `max_tokens=16384`, `reasoning_effort=none`, and still produced
  `finish_reason="length"` with null content.
- Production change: add a generic fast path for slices where
  `validation_covers_acceptance_ids` is already empty, plus a small
  `plan-audit.md` output budget. The auditor must write one coverage bullet,
  apply scope/worker-track/stop-ownership checks, and avoid per-acceptance
  deliberation.
- P53 passed: the same request returned a normal shell action with
  `finish_reason="stop"`, kept `validation_covers_acceptance_ids: []`,
  normalized both stop fields to worker-phase completion, and kept `go build`
  owned by targeted validation.

Fresh full run after the first fast path:

- Run directory:
  `.pragma/swe-bench-pro/20260613T102944Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Agent status: `1`.
- Evaluator result: `false`, overall accuracy `0.0`.
- Agent failure:
  `state "swe_slice_plan_auditor" failed: model returned no bash action after 3 retries`.
- Failure point: late `swe_slice_plan_auditor` on the
  `k8s-auth-config-struct` slice after the run had already handled producer
  discovery, proto enum generation, Go version update, and grpc dependency
  update.
- Raw failure shape:
  turns `000083`, `000084`, `000085`, and `000087` all returned HTTP 200 with
  `finish_reason="length"`, `content:null`, `completion_tokens=16384`,
  `prompt_tokens=10289`; turn `000086` was a retryable Lilac 500.
- Request size at failure:
  `system_chars=21374`, `user_chars=24197`, `reasoning_effort="none"`.
- Patch surface at evaluator time was incomplete: `go.mod`, `go.sum`, and
  proto/generated auth files had changed, but the config slice never ran.
- Eval failure evidence:
  `internal/config` failed to build because evaluator tests referenced missing
  `AuthenticationMethods.Kubernetes` and
  `AuthenticationMethodKubernetesConfig`.

Focused AB for the second late-run auditor length blocker:

- New replay directory:
  `.pragma/prompt-ab/20260613T000009Z-slice-auditor-compact-fastpath`.
- P54 replayed live turn `000083` with a compact auditor prompt. It fixed the
  length collapse (`finish_reason="stop"`) but wrongly narrowed same-file
  scope by treating `AuthenticationMethods` inside the already approved file as
  out of scope.
- P55 added the generic file-level scope rule. It preserved same-file
  `AuthenticationMethods` work but did not normalize both stop fields to the
  exact worker-phase sentence.
- P56 added fast-path stop normalization. It passed: same live payload returned
  `finish_reason="stop"`, preserved same-file scope, kept
  `validation_covers_acceptance_ids: []`, and normalized both stop fields while
  keeping `go build ./internal/config/...` owned by targeted validation.
- Production change: replace the oversized slice-plan auditor prompt with a
  compact generic contract, explicitly state that approval is file-level, and
  force exact stop-field normalization on the empty-coverage fast path.
- Additional generic prompt hygiene: the targeted validator command template now
  forbids markdown backticks and shell metacharacters inside unquoted heredocs,
  and prefers single-quoted heredocs or `printf` for runtime values.

Fresh full run after compact auditor:

- Run directory:
  `.pragma/swe-bench-pro/20260613T110405Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Status: manually stopped before evaluation after the first worker slice
  exposed a bad audited contract.
- Failure class: the compact auditor preserved empty validation coverage but
  allowed `targeted_validation: "none"` while `generated_policy` said
  producer-owned `auth.pb.go` must be regenerated from the source edit. The
  audit text said targeted validation owned `buf generate`/`go build`, but the
  JSON contract that downstream states consume still said `none`.
- Secondary observation: the worker then proceeded under the contradictory
  contract and began source edits before a producer/build validation owner was
  encoded in `slice-plan.json`.

Focused AB for generated-policy targeted validation:

- New replay directory:
  `.pragma/prompt-ab/20260613T000010Z-slice-auditor-producer-targeted-validation`.
- P57 replayed stopped-run turn `000033` after adding a producer-targeted
  validation rule to the compact prompt. It was not accepted: two upstream 502s
  were followed by a `finish_reason="length"` null-content response.
- P58 replaced the auditor prompt with a minimal 9-step algorithm. It fixed the
  length collapse and set non-`none` `targeted_validation`, but incorrectly put
  the producer command in `Worker must run before reporting`.
- P59 added the exact Worker Contract ownership line. It passed: the same
  request returned `finish_reason="stop"`, set non-`none`
  `targeted_validation`, kept generated output out of `approved_edit_paths`,
  normalized stop fields, and assigned producer/build commands to targeted
  validation after the worker report.
- P60 added a generic exact-producer-command rule to prevent invented flags or
  working directories. Both replay attempts hit upstream 502, so this small
  tightening is provider-blocked for live AB evidence.
- Production change: `swe_slice_plan_auditor` is now a minimal generic
  algorithm instead of a long rule ledger, with explicit gates for
  generated-output producer validation, file-level scope approval, exact stop
  normalization, and targeted-validation command ownership.

## 9. Regressions, Rejected Variants, and Why

Imported rejected variants:

- Prose-only acceptance reminders: rejected because planner/validator variants
  ignored appended hints or still passed insufficient evidence.
- Final-reviewer-only gate: rejected because context loss can occur before
  final validation.
- Broad tests without acceptance IDs: rejected because failed run already chose
  build/runtime tests while missing evaluator-facing config tests.
- Automatic "write more tests" slices: rejected because self-authored runtime
  tests hid missing config loader behavior.
- Generic runtime `git_diff_scope` check: rejected and reverted because it
  moved a SWE-bench persona/workflow scope policy into Pragma generic runtime.
  Scope discipline for this goal must remain in the persona/FSM/declarative
  contract layer unless the runtime support is truly task-agnostic and not
  tailored to this benchmark flow.

## 10. Generalization Notes for Non-Flipt Tasks

The acceptance map must be task-derived and stable. The first task in a run is
a baseline calibration case; it must not cause the personas to hard-code a
language, framework, repository, generated-artifact toolchain, or validation
command family.

Current generalized rules:

- Acceptance IDs come from the requested feature or failure and must be
  preserved exactly after they are emitted.
- Later personas may validate only IDs present in `acceptance-map.json`; absent
  IDs are context drift and require remapping or theory repair.
- Validation is classified by behavior surface. Grep, source inspection,
  compile-only checks, lint, generated-symbol presence, and self-authored tests
  are supporting evidence only unless they directly exercise the acceptance
  surface.
- Input-loading, evaluator fixture, default, custom binding, schema, generated
  producer, migration, API, UI, runtime, and compatibility requirements remain
  separate when they can fail independently.
- `behavior_surface: "generated"` is reserved for generated output
  freshness/compatibility requirements. A named introduced source function,
  method, route, command, migration, schema field, or API is classified by its
  user-facing behavior instead.

Non-Flipt generalization check:

- New replay directory:
  `.pragma/prompt-ab/20260613T000005Z-nonflipt-generalization-check`.
- NF1 used a synthetic Django permission-cache task with an introduced source
  function. It had no Flipt/Kubernetes/Go/proto leakage and produced task-derived
  acceptance IDs, but it mislabeled the introduced source function as
  `behavior_surface: "generated"`.
- NF2 replayed the same non-Flipt payload after tightening the mapper prompt.
  It passed: the acceptance map uses `ACCEPT-TEAM-*` / `ACCEPT-CACHE-*`
  task-derived IDs, exact source quotes from the Django/cache task, runtime
  behavior surfaces, and no baseline-task strings.

## 11. Remaining Risk and Next Iteration Plan

Immediate next actions:

1. Start another fresh full baseline with the split discovery/implementation
   worker route, generic declared shell policy, corrected auditor ownership
   invariant, and empty-validation fast path.
3. If the fresh run fails, classify the failure from exact raw HTTP/log
   artifacts and continue prompt/payload AB from that state instead of
   restarting from scratch.

Known risk: P38/P39 and P44 are reduced replays after exact worker/auditor turns
exposed provider or prompt-stickiness failures, P40 is a single
environment-survey turn replay after a stopped run, and P47/P48/P50 prove that
prompt-only discovery separation is insufficient. The next provider-healthy full
run still needs to prove that the live loop plans a coherent
source-of-truth/consumer sequence, keeps environment survey bounded, routes
no-edit discovery through the discovery worker, rejects forbidden discovery
commands through the declared shell policy, and that targeted validator and
reviewer use the evidence correctly.

## 12. Completion Audit Against Goal Prompt

| Requirement | Status | Evidence |
| --- | --- | --- |
| Production orchestration/persona files updated | Complete for first implementation pass | Files listed in section 5 |
| Implementation ledger explains every production edit | In progress | Section 5 plus fresh-run observations in section 8 |
| Generic runtime contains no SWE/persona artifact special cases | Complete for current pass | Declarative checks and shell policies are configured in `orchestrations/swe-bench-pro-engineering-loop.yaml`; runtime special-case scan for old artifact names/paths has no matches outside tests |
| Production prompts contain no baseline-task or stack-specific leakage | Complete for current pass | Production scan for Flipt/Kubernetes/ACCEPT-K8S/auth.proto/METHOD_KUBERNETES/mage/go-build/go-test-config strings has no matches outside tests |
| AB replay evidence passes for changed states | Partial for current prompt/runtime pass | Production replay has A2, B2, C2c, D3b, E, F, G, W2 passing; generalized replay has A2, C2c, D3b, W2, E, F, G passing after worker/final-validator shell-contract patches; the corrected producer-discovery and first edit-slice chain now passes A9, B6, C10, W9, D4, E2, B9, H1, P1, ENV1, P4, W14, W15, W16, W18, W19b, W20, D7, E3, P6, P25, P28, P29, P38, P39, P40, P42, P44, P46, P52, and P53; P47/P48/P50 exact discovery-worker replays fail and document why generic declared shell-policy enforcement was required; P51 failed and documents why the ownership invariant had to be moved to the top of the auditor prompt |
| Fresh baseline run passes evaluator or external blocker proven | Pending after latest regression | `.pragma/swe-bench-pro/20260613T085521Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446` exposed producer-discovery command execution after a no-producer Worker Contract; `.pragma/swe-bench-pro/20260613T093115Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446` exposed the worker-stop/targeted-validation ownership contradiction; `.pragma/swe-bench-pro/20260613T095107Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446` failed in `swe_slice_plan_auditor` after repeated length completions and evaluator reported false |
| Generalization check beyond original failed replay | Complete for mapper-level non-Flipt evidence | `.pragma/prompt-ab/20260613T000005Z-nonflipt-generalization-check`: NF1 rejected the generated-surface misclassification; NF2 passed on a Django permission-cache task with task-derived IDs, runtime behavior surfaces, and no Flipt/Kubernetes/Go/proto leakage |
| Ledger completion audit fully marked complete | Pending | This table |

## 13. Latest Live-Run Regression and Fix

Run:
`.pragma/swe-bench-pro/20260613T111621Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: manually stopped after the loop pursued a project dependency/toolchain
upgrade as the repair for current-runner producer output. The run had already
proved useful generic failures:

- `swe_slice_plan_auditor` recovered from an initial length completion and
  eventually preserved worker/targeted-validation ownership.
- `swe_engineering_worker` used destructive git commands inside the benchmark
  container; this is now blocked by the `swe_engineering_worker` shell policy.
- `swe_targeted_validator` initially wrote static success text before command
  status existed, then corrected itself after failure; the prompt now requires
  status-derived report writing after command completion.
- Reviewer, scope expansion, theory keeping, planning, plan auditing, and
  targeted validation now reject dependency, lockfile, language-version, or
  toolchain-manifest expansion based solely on current-runner producer output.
  They require original task text, repository-owned configuration, or
  clean-baseline producer evidence before approving a project upgrade.

Verification after the fix:

- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- `go test ./internal/orchestration ./internal/query ./internal/cli ./internal/observe ./internal/app -run '^$'`
- Production leak scan for `git_diff_scope`, Flipt/Kubernetes/ACCEPT-K8S,
  `auth.proto`, `METHOD_KUBERNETES`, `auth.pb.go`, and calibration build/test
  strings only finds existing unit-test fixture strings in
  `internal/orchestration/orchestration_test.go`.

Follow-up run:
`.pragma/swe-bench-pro/20260613T113818Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: manually stopped after `swe_slice_plan_auditor` removed
`acceptance_ids` and `missing_acceptance_ids` from a no-edit discovery slice.
That was a handoff-loss bug: discovery and prerequisite slices should keep the
acceptance linkage while leaving `validation_covers_acceptance_ids` empty. The
auditor prompt now explicitly preserves `acceptance_ids` and
`missing_acceptance_ids` unless IDs are absent from `acceptance-map.json` or
the corrected slice directly covers them.

Verification after the follow-up fix:

- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- `go test ./internal/orchestration ./internal/query ./internal/cli ./internal/observe ./internal/app -run '^$'`
- Production leak scan still only finds the existing unit-test fixture strings
  in `internal/orchestration/orchestration_test.go`.

Second follow-up run:
`.pragma/swe-bench-pro/20260613T114256Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: manually stopped after the loop reached the producer/toolchain
compatibility failure again and `swe_theory_keeper` promoted a Go-version
upgrade into durable context and `Next Objective` without clean-baseline
producer evidence. This violated the intended generic guardrail: current-runner
producer output is not proof that the benchmark project should upgrade
dependencies or language version.

Fix:

- `swe_engineering_reviewer` now must not put dependency/language-version
  upgrade options in `required_context_update` unless original task text,
  repository-owned configuration, or clean-baseline producer evidence proves the
  project upgrade is intended.
- `swe_theory_keeper` now treats "determine whether to upgrade" wording as
  insufficient and explicitly forbids durable `go.mod requires upgrade`,
  `go_version must be upgraded`, `upgrade go.mod`, and equivalent next-objective
  language without the same evidence. The required durable outcome is an
  unresolved runner/producer contract blocker plus a clean-baseline evidence
  requirement.

Verification after the second follow-up fix:

- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- `go test ./internal/orchestration ./internal/query ./internal/cli ./internal/observe ./internal/app -run '^$'`
- Production leak scan still only finds the existing unit-test fixture strings
  in `internal/orchestration/orchestration_test.go`.

Third follow-up run:
`.pragma/swe-bench-pro/20260613T115302Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: manually stopped as provider-blocked. The run recovered from an early
Lilac 502 burst, completed repo survey, acceptance mapping, environment survey,
theory keeping, acceptance audit, an initial producer-discovery slice, targeted
validation, review, theory update, and acceptance audit. The loop correctly
classified the first producer-discovery slice as incomplete instead of moving
to implementation: the worker only inspected the first part of `magefile.go`,
targeted validation marked the discovery insufficient, and reviewer routed back
to theory/planning. The next planner turn then hit repeated Lilac 502s and made
no useful new response, so no prompt/runtime patch was made from that run.

Focused replay follow-up:
`.pragma/prompt-ab/20260613T120551Z-provider-blocked-planner-replay`.

- P61 replayed the exact missing-response planner payload from
  `raw-http-pragma/000065-e9775e14f9e9-17025f7f47de` unchanged. Provider
  returned HTTP 200, proving the full-run stop was provider instability rather
  than an unreplayable payload. The response still had a planner contract
  failure: a discovery slice with `acceptance_ids: []` while
  `missing_acceptance_ids` listed blocking acceptance IDs.
- P62 changed only the planner system prompt by moving acceptance-linkage into
  an early hard JSON validity gate. Replay returned HTTP 200 and produced a
  discovery slice with non-empty `acceptance_ids` and empty
  `validation_covers_acceptance_ids`.
- Promoted P62 into `personas-research-v2/swe_slice_planner.yaml`.
- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T120551Z-provider-blocked-planner-replay --require-responses`
  passes for P61 and P62.
- Post-promotion checks passed: `git diff --check`, orchestration visualize,
  compile-only package sweep, and production leak scan with only existing
  unit-test fixture hits.

Fourth follow-up run:
`.pragma/swe-bench-pro/20260613T120846Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Status: run reached repeated schema-producer discovery. Earlier planner and
auditor fixes held: discovery slices preserved acceptance linkage, validation
did not claim acceptance coverage from source reads, and reviewer kept routing
unresolved producer evidence back through theory/scope instead of moving to
implementation. The run also exposed a generic discovery shell-policy
overmatch: the deny pattern rejected read-only grep commands because their
search terms or paths contained `generate` or `build`, even though those
commands were inspecting producer definitions rather than executing producers.

Fix:

- Narrowed the discovery-worker `shell_policy.deny_patterns` in
  `orchestrations/swe-bench-pro-engineering-loop.yaml` to command-level
  producer/build/test/lint invocations such as `go generate`, `buf generate`,
  `go build`, `go test`, and package-manager or task-runner
  generate/build/test/lint targets. Read-only `grep`/`ls`/source inspection
  commands that mention those words as search terms or paths are no longer
  denied by the shell policy.

Verification after the shell-policy fix:

- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- `go test ./internal/orchestration -run '^$'`

Fifth follow-up run:
`.pragma/swe-bench-pro/20260613T122432Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: manually stopped as invalid after `swe_theory_keeper` executed
repository and producer commands directly while trying to collect clean-baseline
producer evidence. The earlier fixes held long enough to expose useful
behavior: discovery no longer rejected read-only `generate` searches, the first
implementation slices added the proto enum and config/method-package stubs, and
validation/review correctly kept failed builds from becoming acceptance claims.
The known producer/toolchain blocker was also handled better by
`swe_engineering_reviewer`: it routed to `revise_theory` and explicitly
required clean-baseline evidence before any dependency or Go-version upgrade
plan. The new failure was command ownership: theory keeping is a context-update
state and must not run `git stash`, `buf generate`, `go build`, `git restore`,
or `git checkout`.

Fix:

- Added a `shell_policy` to `swe_theory_keeper` in
  `orchestrations/swe-bench-pro-engineering-loop.yaml` denying destructive git
  mutations and command-level producer/build/test/lint invocations.
- Tightened `personas-research-v2/swe_theory_keeper.yaml` to explicitly forbid
  repository commands, producers, generated-artifact refreshes, builds, tests,
  dry-runs, preflights, and clean-baseline command collection. If that evidence
  is needed, theory keeper must record it as a next objective or
  scope/validation requirement instead.

Verification after the theory-keeper command-ownership fix:

- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- `go test ./internal/orchestration -run '^$'`

Focused replay follow-up:
`.pragma/prompt-ab/20260613T124439Z-theory-keeper-command-policy`.

Current replay ledger:

| Case | Source request | Persona changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| TK1 exact failing payload | `.pragma/swe-bench-pro/20260613T122432Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000119-470ab1b760d0-e18497f73f52` | system prompt only: current `swe_theory_keeper` | Write `/tmp/pragma/swe/engineering-context.md`; preserve pending acceptance IDs; record clean-baseline producer evidence as next-owner work; do not run git, producer, build, lint, test, preflight, or dry-run commands | Provider returned HTTP 502 `All upstream targets failed`; source response was `cd /app && git stash && buf generate 2>&1 \| head -50` | Provider-blocked; not behavior evidence | Do not retry blindly; require reduced equivalent before any full run |
| TK2 reduced command-ownership payload | `.pragma/prompt-ab/20260613T124439Z-theory-keeper-command-policy/TK2-reduced-command-ownership` | none from TK1; current `swe_theory_keeper` retained | Same as TK1, with reduced clean-baseline pressure | HTTP 200, `finish_reason="stop"`; response writes only `engineering-context.md`, preserves `ACCEPT-K8S-AUTH-RECOGNIZED-METHOD` and `ACCEPT-K8S-AUTH-CONFIG-PARAMS`, and records clean-baseline producer evidence as producer/build/test owner work | Pass for reduced equivalent | Keep the theory_keeper command-ownership fix; do not start a full run until the slice-plan-auditor bottleneck has current replay evidence and hard preflight passes |

Replay audit for the directory currently reports TK1 as `response_error` and
TK2 as `response_ok`; that is expected because provider-blocked TK1 is not a
usable response, while TK2 is usable reduced evidence.

Current `swe_slice_plan_auditor` replay ladder:

| Case | Source request | Persona changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| SP1 exact length-collapse payload | `.pragma/swe-bench-pro/20260613T095107Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000143-6c61b6f2c7c9-bda4f01c23b5` | system prompt only: current `swe_slice_plan_auditor` | Produce one bash block; preserve acceptance IDs; keep `validation_covers_acceptance_ids` empty for partial/prerequisite work; route build/test commands to targeted validation | Provider returned HTTP 502 `All upstream targets failed` | Provider-blocked; not behavior evidence | Do not retry blindly; reduced equivalent required |
| SP2 reduced length-collapse equivalent | `.pragma/prompt-ab/20260613T095107Z-live-slice-auditor-length-blocker/SP2-reduced-current-prompt` | none from SP1; current `swe_slice_plan_auditor` retained | Same as SP1 on a reduced implementation slice with `go build` in `targeted_validation` | HTTP 200, `finish_reason="stop"`; response preserves all five acceptance IDs, keeps coverage empty, sets `worker_track: implementation`, and assigns `go build ./internal/server/auth/method/kubernetes/... && go build ./...` to targeted validation after worker report | Pass for reduced equivalent | Keep the current compact auditor prompt; no new auditor prose patch from this case |
| SP3 exact acceptance-ID preservation payload | `.pragma/swe-bench-pro/20260613T113818Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000275-d06cd9ca1d0b-61061bed1c13` | system prompt only: current `swe_slice_plan_auditor` | Preserve discovery-slice `acceptance_ids` and `missing_acceptance_ids`; keep `validation_covers_acceptance_ids` empty; `worker_track` remains `discovery`; worker owns no producer/build/test commands | HTTP 200, `finish_reason="stop"`; preserved `ACCEPT-K8S-AUTH-FRAMEWORK-INTEGRATION`, `ACCEPT-K8S-AUTH-RECOGNIZED-METHOD`, all missing IDs, empty coverage, and discovery worker ownership | Pass | Satisfies current acceptance-ID preservation replay gate before any future full run |

Current hard preflight before any next full run:

| Gate | Command or evidence | Result |
| --- | --- | --- |
| Last exact failing turn replay | SP1 exact current-prompt replay from `20260613T095107Z` auditor length-collapse turn | Provider-blocked with HTTP 502; not behavior evidence |
| Reduced equivalent replay | SP2 reduced current-prompt auditor replay | Pass; preserved IDs, empty coverage, targeted-validation ownership |
| Acceptance-ID preservation replay | SP3 exact current-prompt discovery-slice replay from `20260613T113818Z` | Pass; preserved `acceptance_ids` and `missing_acceptance_ids` |
| FSM/static graph | `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact` | Pass |
| Focused compile-only | `go test ./internal/orchestration -run '^$'` | Pass |
| Diff hygiene | `git diff --check` | Pass |
| Forbidden production string scan | `rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\\.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config|git_diff_scope" personas-research-v2 orchestrations internal` | Only existing `internal/orchestration/orchestration_test.go` fixture hits |

Promotion run:
`.pragma/swe-bench-pro/20260613T131109Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Stopped under the explicit stop-run criteria. The first auditor turn preserved
IDs and worker track, but left a no-edit discovery slice requiring producer
success confirmation: objective/observable said to verify `buf generate` works
while `targeted_validation` was `none` and the Worker Contract forbade
producer/build/test commands. This is a `swe_slice_plan_auditor`
validation-routing failure; no evaluator result was produced.

Current `swe_slice_plan_auditor` validation-routing replay ladder:

| Case | Source request | Persona changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| SP4 exact current-run payload | `.pragma/swe-bench-pro/20260613T131109Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000030-47edd43be485-71d3c067fe39` | none; exact captured payload | Discovery with `targeted_validation: none` must not require producer/build/test execution, success confirmation, verification, exit status, or "works" | Provider returned HTTP 502 `All upstream targets failed` | Provider-blocked; not behavior evidence | Do not retry blindly; reduced equivalent required |
| SP5 reduced validation-routing payload | `.pragma/prompt-ab/20260613T131109Z-slice-auditor-validation-routing/SP5-reduced-validation-routing` | none from SP4 | Same invariant on a reduced producer-discovery slice | HTTP 200; removed producer success confirmation from worker-owned fields, preserved IDs, kept coverage empty | Pass for reduced equivalent | Patch still required because live exact output failed |
| SP6 exact patched prompt | `.pragma/swe-bench-pro/20260613T131109Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000030-47edd43be485-71d3c067fe39` | first patched auditor prompt | Same invariant | HTTP 200, but still left producer success confirmation in `objective`/`expected_observable` with `targeted_validation: none` | Rejected | Prompt rule was too low-priority |
| SP7 exact patched gate | same source request as SP6 | auditor prompt with critical invalid-output gate | Same invariant | HTTP 200; removed producer success/works/verification requirement from discovery fields, preserved IDs, kept coverage empty, and kept producer/build/test execution out of worker ownership | Pass | Current auditor patch promoted; no full run until hard preflight is rerun |
| SP8 exact acceptance-ID preservation after gate | `.pragma/swe-bench-pro/20260613T113818Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000275-d06cd9ca1d0b-61061bed1c13` | latest auditor prompt | Preserve discovery-slice IDs after validation-routing gate | HTTP 200; preserved IDs and ownership, but mislabeled `internal/authn/kubernetes.go` as generated/producer-owned in audit text | Rejected | Added generated-label guard |
| SP9 exact generated-label guard | same source request as SP8 | latest auditor prompt with generated-label guard | Preserve IDs and do not label ordinary source paths as generated output | HTTP 200; preserved IDs, kept coverage empty, preserved discovery ownership, and identified `internal/authn/kubernetes.go` as source, not generated output | Pass | Current acceptance-ID preservation gate passes |
| SP10 exact validation-routing after generated-label guard | `.pragma/swe-bench-pro/20260613T131109Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000030-47edd43be485-71d3c067fe39` | latest auditor prompt | Preserve SP7 validation-routing behavior after generated-label guard | Provider returned HTTP 502 `All upstream targets failed` | Provider-blocked; not behavior evidence | Reduced equivalent required |
| SP11 reduced validation-routing after generated-label guard | `.pragma/prompt-ab/20260613T131109Z-slice-auditor-validation-routing/SP11-reduced-latest-gate` | latest auditor prompt | Same as SP10 on reduced payload | Provider returned HTTP 502 `All upstream targets failed` | Provider-blocked; not behavior evidence | Full run may only proceed as promotion gate because exact replay is provider-blocked and reduced equivalent was attempted, not because behavior passed |

Post-SP7 checks:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T131109Z-slice-auditor-validation-routing/SP7-exact-patched-gate --require-responses`: pass.
- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T113818Z-slice-auditor-acceptance-id-preservation/SP9-exact-generated-label-guard --require-responses`: pass.
- `git diff --check`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go test ./internal/orchestration -run '^$'`: pass.
- Production leak scan still only finds existing fixture strings in
  `internal/orchestration/orchestration_test.go`.

Next promotion run:
`.pragma/swe-bench-pro/20260613T132710Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Stopped under the explicit stop-run criteria. The first
`swe_slice_plan_auditor` turn passed the discovery/producer-routing fix. The
second auditor turn then cleared `acceptance_ids` on a prerequisite
implementation slice. The planner proposed `ACCEPT-K8S-AUTH-RECOGNIZED` and
`ACCEPT-K8S-AUTH-INTROSPECTION`; the auditor wrote `acceptance_ids: []`, moved
those IDs to `missing_acceptance_ids`, and still called the slice a
prerequisite. That was handoff loss by the current bottleneck persona.

Current prerequisite-ID replay ladder:

| Case | Source request | Persona changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| SP12 exact current-run payload | `.pragma/swe-bench-pro/20260613T132710Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000040-e75b7aa59e3c-f9d63139777e` | none; exact captured payload | Preserve valid nonempty prerequisite `acceptance_ids`; keep coverage empty; do not move linked IDs solely into missing IDs | Provider returned HTTP 502 `All upstream targets failed` | Provider-blocked; not behavior evidence | Reduced equivalent required |
| SP13 reduced prerequisite-ID payload | `.pragma/prompt-ab/20260613T132710Z-slice-auditor-prereq-id-preservation/SP13-reduced-prereq-id-preservation` | none from SP12 | Same invariant on reduced prerequisite implementation slice | HTTP 200; preserved linked IDs, kept coverage empty, and assigned `buf generate && go build ./...` to targeted validation | Pass for reduced equivalent | Patch still required because live exact output failed |
| SP14 exact patched ID gate | same source request as SP12 | auditor prompt with critical ID-preservation gate | Same invariant | HTTP 200; preserved IDs, but assigned `buf generate` to worker pre-report contract | Rejected | Added ownership gate |
| SP15 exact patched ID+ownership gate | same source request as SP12 | auditor prompt with ID and ownership gates | Same invariant plus worker owns no producer/build commands | Provider returned HTTP 500 | Provider-blocked; not behavior evidence | Reduced equivalent required |
| SP16 reduced patched ID+ownership gate | `.pragma/prompt-ab/20260613T132710Z-slice-auditor-prereq-id-preservation/SP16-reduced-patched-id-ownership-gate` | latest auditor prompt | Preserve linked IDs, keep coverage empty, and keep producer/build commands in targeted validation | HTTP 200; preserved IDs, kept coverage empty, and Worker Contract says no producer/build/lint/test before report while targeted validation owns `buf generate && go build ./...` | Pass for reduced equivalent | Current auditor patch promoted; exact payload is provider-blocked after reduced equivalent attempted |

Post-SP16 checks:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T132710Z-slice-auditor-prereq-id-preservation/SP16-reduced-patched-id-ownership-gate --require-responses`: pass.
- `git diff --check`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go test ./internal/orchestration -run '^$'`: pass.
- Production leak scan still only finds existing fixture strings in
  `internal/orchestration/orchestration_test.go`.

Post-replay-block checks:

- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- Production leak scan only finds existing unit-test fixture strings in
  `internal/orchestration/orchestration_test.go`.

Next promotion run:
`.pragma/swe-bench-pro/20260613T133803Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: stopped during `swe_targeted_validator` after the run reached a new
producer/toolchain validation blocker. The current `swe_slice_plan_auditor`
fixes held: the implementation slice preserved
`ACCEPT-K8S-AUTH-RECOGNIZED`, `ACCEPT-K8S-AUTH-CONFIG-PARAMS`,
`ACCEPT-K8S-AUTH-DEFAULTS`, and `ACCEPT-K8S-AUTH-DEPLOYMENT-MODES`; it kept
`validation_covers_acceptance_ids` empty; and it moved `mage proto && go test
./internal/config/...` into targeted validation after the worker report. The
worker did not run producer/build/test commands before reporting.

The new failure class is `swe_targeted_validator` handling of command-output
continuations and generated-code/toolchain blockers:

- `000071` first tried `mage proto`; the command failed with `mage: command not
  found`.
- `000072` switched to `buf generate && go test ./internal/config/...`; `buf
  generate` succeeded, then `go test` failed because generated protobuf/gRPC
  output referenced newer APIs/language features (`grpc.SupportPackageIsVersion9`,
  `grpc.StaticMethod`, `grpc.NewClient`, `unsafe.StringData`) while the module
  declares Go 1.18.
- `000073` was the missing validator response after that failed command output.
  The run was stopped rather than waiting on a full-run continuation.

Targeted-validator replay ladder:

| Case | Source request | Persona changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| P17 exact validator go-test failure | `.pragma/swe-bench-pro/20260613T133803Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000073-b9345723b10d-23d7f567a1b8` | none; exact captured request | Write `targeted-validation.md` from the existing command output; do not rerun producer/build/test; mark planned IDs insufficient; do not recommend dependency/language/toolchain upgrades without clean-baseline evidence | HTTP 200, but reran `buf generate && go test`, called the mismatch a validation blocker, and suggested upgrading Go or changing generation compatibility | Rejected | Patch required |
| P18 exact patched command-output blocker | same source request as P17 | validator prompt with command-output continuation rule and stronger no-upgrade text | Same invariant | HTTP 200, but still reran `buf generate`, called the issue pre-existing from weak environment context, and suggested upgrading `go.mod` or regenerating for Go 1.18 | Rejected | Soft rule was too low priority |
| P19 exact critical output gate | same source request as P17 | validator prompt with critical invalid-output gate first | Same invariant | Provider returned HTTP 500 | Provider-blocked; not behavior evidence | Reduced equivalent required |
| P20 reduced toolchain-output gate | `.pragma/prompt-ab/20260613T133803Z-targeted-validator-toolchain-blocker/P20-reduced-toolchain-output-gate` | latest validator prompt | Same invariant on reduced command-output payload | HTTP 200; wrote `targeted-validation.md` directly, did not rerun commands, put all planned IDs in `insufficient_acceptance_ids`, avoided upgrade recommendations, avoided pre-existing claims without clean-baseline evidence, and used the required producer/toolchain scope signal | Pass for reduced equivalent | Keep validator patch; do not promote again until hard preflight remains green |

Post-P20 checks:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T133803Z-targeted-validator-toolchain-blocker/P20-reduced-toolchain-output-gate --require-responses`: pass.
- `git diff --check`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go test ./internal/orchestration -run '^$'`: pass.
- Production leak scan still only finds existing fixture strings in
  `internal/orchestration/orchestration_test.go`.

Next promotion run:
`.pragma/swe-bench-pro/20260613T140331Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: stopped during `swe_engineering_worker`. The earlier fixes held through
the initial discovery cycle and the next slice-plan audit:

- `swe_theory_keeper` only wrote context and did not run repository commands.
- The first discovery slice preserved acceptance IDs, kept coverage empty, and
did not run producer/build/test commands.
- `swe_targeted_validator` did not validate acceptance IDs from discovery.
- `swe_slice_plan_auditor` corrected the second planner's worker contract by
  moving `buf generate && go build ./internal/config/... && go test
  ./internal/config/... -run Authentication` to targeted validation and
  keeping the worker contract source-edit-only.

The new failure was worker repair behavior after a malformed approved-path
edit. The worker inserted `AuthenticationMethodKubernetesConfig` into the body
of `AuthenticationCleanupSchedule`, then attempted repair with destructive git
rollback and broad line-deletion commands:

- `000056`: proposed a fragile multiline `sed -i '305i...'` insertion.
- `000058`/`000059`: proposed `git checkout internal/config/authentication.go`.
- `000060` onward: continued broad `sed -i '308,323d'` repair attempts.

This violates the worker boundary: after an approved edit leaves a source file
malformed, the worker must report a blocker/scope issue, not restore git state,
delete broad line ranges, or continue repair loops.

Worker destructive-repair replay ladder:

| Case | Source request | Persona changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| P21 exact worker git-checkout gate | `.pragma/swe-bench-pro/20260613T140331Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000058-0a531a249bd0-2af754564ac6` | worker prompt with destructive-git and corrupted-edit gate | Start with bash fence; write `worker-report.md` blocker; do not run `git checkout`, rollback, repair, producer, build, or test commands | HTTP 200 but proposed prose plus `cd /app && git checkout internal/config/authentication.go` | Rejected | Gate was too low-salience |
| P22 exact top git-checkout gate | same source request as P21 | worker prompt with compact hard gate moved to the top | Same invariant | HTTP 200 but still emitted prose and continued repair via follow-up source inspection instead of writing a report | Rejected | Needed malformed-insertion-specific stop |
| P23 exact malformed-insertion gate | same source request as P21 | worker prompt with explicit declaration-inside-declaration malformed-output rule | Same invariant | Provider returned HTTP 500 | Provider-blocked; not behavior evidence | Reduced equivalent required |
| P24 reduced malformed-insertion gate | `.pragma/prompt-ab/20260613T140331Z-worker-destructive-repair/P24-reduced-malformed-insertion-gate` | latest worker prompt | Same invariant on reduced malformed-insertion payload | HTTP 200; wrote `worker-report.md` directly, did not run git rollback or repair commands, preserved pending ID as insufficient, and named the malformed approved-path edit as a blocker | Pass for reduced equivalent | Keep worker patch; no new full run until hard preflight remains green |

Post-P24 checks:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T140331Z-worker-destructive-repair/P24-reduced-malformed-insertion-gate --require-responses`: pass.
- `git diff --check`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go test ./internal/orchestration -run '^$'`: pass.
- Production leak scan still only finds existing fixture strings in
  `internal/orchestration/orchestration_test.go`.

Next promotion run:
`.pragma/swe-bench-pro/20260613T142229Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: stopped during `swe_engineering_worker`. The auditor bottleneck held in
this run:

- `swe_theory_keeper` only wrote context and did not run repository commands.
- The first `swe_slice_plan_auditor` preserved all 10 acceptance IDs, kept
  discovery coverage empty, and routed to `swe_discovery_worker`.
- The second `swe_slice_plan_auditor` preserved the three implementation-slice
  IDs, kept `validation_covers_acceptance_ids` empty, and assigned
  `buf generate && go build ./internal/config/...` to targeted validation after
  the worker report.

The new failure was the same worker repair class, but at an earlier post-diff
decision point. After the worker made approved-path edits and inspected
`git diff`, the correct next action was the worker report. Instead, `000064`
tried to fix a duplicate comment with in-place `sed` deletion, then `000066`
reported that `internal/config/authentication.go` had become malformed.

Patch:

- Added a priority-zero post-diff terminal gate to
  `personas-research-v2/swe_engineering_worker.yaml`: after a successful
  approved-path `git diff`, the worker must write `worker-report.md` and must
  not continue with comment cleanup, placement cleanup, repository reads,
  producer/build/test commands, or further mutation.
- Added a generic declared FSM shell policy for `swe_engineering_worker` in
  `orchestrations/swe-bench-pro-engineering-loop.yaml` to reject in-place
  `sed` deletion repair commands. This is declarative state policy, not generic
  runtime knowledge of this task.
- Added a generic completion-shell guard so report-writing responses do not
  leave a stray heredoc delimiter after the completion echo.

Worker post-diff repair replay ladder:

| Case | Source request | Persona/policy changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| P25 exact post-diff stop gate | `.pragma/swe-bench-pro/20260613T142229Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000064-1add28b0e6ef-f9528f60dea4` | worker prompt with post-diff report rule | After successful approved-path `git diff`, write `worker-report.md`; do not run more `sed` repair or cleanup | HTTP 200 but still attempted in-place `sed` comment repair | Rejected | Prompt rule too low-priority |
| P26 exact terminal gate | same source request as P25 | strengthened worker prompt | Same invariant | HTTP 200 but still attempted in-place `sed` comment repair | Rejected | Gate still too low-priority |
| P27 exact priority-zero gate | same source request as P25 | priority-zero worker prompt gate | Same invariant | HTTP 200 but still attempted in-place `sed` comment repair | Rejected | Exact prompt-only replay remains sticky to prior shell-edit pattern |
| P28 exact policy-denial continuation | exact P27 request plus rejected assistant command and declared shell-policy denial message | worker prompt plus declared shell-policy recovery path | After policy rejects in-place `sed` deletion, write `worker-report.md`; do not attempt another repair | Provider returned HTTP 502 | Provider-blocked; not behavior evidence | Reduced equivalent required |
| P29 reduced policy-denial continuation | `.pragma/prompt-ab/20260613T142229Z-worker-post-diff-stop/P29-reduced-policy-denial-continuation` | worker prompt plus declared shell-policy recovery path | Same invariant on reduced payload | HTTP 200; wrote `worker-report.md`, did not run another mutation command, preserved IDs with validation pending; shell block had an extra trailing `EOF` after completion echo | Partially passed; completion-shell guard required | Patch completion-shell shape before promotion |
| P30 reduced clean-completion continuation | `.pragma/prompt-ab/20260613T142229Z-worker-post-diff-stop/P30-reduced-policy-denial-clean-completion` | worker prompt with completion-shell guard | Same invariant, no stray heredoc after echo | Provider returned HTTP 502 | Provider-blocked; not behavior evidence | Smaller reduced equivalent required |
| P31 minimal policy-denial clean-completion | `.pragma/prompt-ab/20260613T142229Z-worker-post-diff-stop/P31-minimal-policy-denial-clean-completion` | latest worker prompt plus declared shell-policy recovery path | After policy rejects in-place `sed` deletion, write only `worker-report.md` and completion echo; no mutation command and no stray heredoc | HTTP 200; wrote a clean `worker-report.md` block, no `sed`/repair command, validation stayed pending, and completion shell ended immediately after the echo | Pass for reduced equivalent | Current worker prompt plus declarative shell policy can be promoted only after hard preflight remains green |

Post-P31 checks:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T142229Z-worker-post-diff-stop/P31-minimal-policy-denial-clean-completion --require-responses`: pass.
- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T142229Z-worker-post-diff-stop/P29-reduced-policy-denial-continuation --require-responses`: pass.
- `git diff --check`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2`: pass.
- `go test ./internal/orchestration -run '^$'`: pass.
- `go test ./internal/query -run '^$'`: pass.
- Production leak scan over `internal` and `cmd` found no Flipt/Kubernetes,
  `git_diff_scope`, worker, or auditor task strings.
- Forbidden-command scan only found intentional worker-prompt policy text.

Next promotion run:
`.pragma/swe-bench-pro/20260613T144437Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Outcome: stopped during `swe_engineering_worker`; full run was not continued to
targeted validation as an exploratory debugger. The upstream auditor and
ownership boundaries held long enough to expose a worker-specific failure:

- `swe_slice_plan_auditor` preserved the implementation slice IDs and assigned
  producer/build commands to targeted validation.
- `swe_engineering_worker` violated the response gate at `000061` by emitting
  prose plus a bash block.
- After the runtime formatting retry, `000062` repeated the same append-only
  source mutation using `cat >> internal/config/authentication.go`, creating
  duplicate declaration risk.
- After `git diff`, `000067` wrote a report that claimed behavior it also said
  was not implemented: defaults and file accessibility validation remained
  missing, but related acceptance IDs were treated as addressed.

Patch:

- Added a retry and policy-denial priority gate to
  `personas-research-v2/swe_engineering_worker.yaml`: after a formatting retry
  or declared shell-policy rejection following a repository mutation, the
  worker must write `worker-report.md` and must not repeat or replace the
  mutation.
- Added exact-string formatting retry triggers for runtime retry messages such
  as `Please always provide EXACTLY ONE bash action`, `Found 2 actions`, and
  `format your response exactly`.
- Added an opaque acceptance-ID rule: worker reports must copy IDs exactly and
  must not create, rename, pluralize, normalize, or infer IDs.
- Added a claim rule: `acceptance_ids_addressed` is only for behavior actually
  implemented by the worker in approved paths; defaults, validation, schema,
  generated, API, runtime, and integration behavior remain missing or
  insufficient when not implemented.
- Extended the declarative `swe_engineering_worker.shell_policy` to reject
  append-only source/config writes and in-place `sed -i` source/config edits.
  This is state-declared generic worker policy, not generic runtime knowledge
  of any task, stack, artifact, or baseline.

Worker retry/append/overclaim replay ladder:

| Case | Source request | Persona/policy changed | Expected invariant | Actual result | Verdict | Promotion decision |
| --- | --- | --- | --- | --- | --- | --- |
| P32 exact format-retry current worker | `.pragma/swe-bench-pro/20260613T144437Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000062-0b3e258f3642-b65d7e368e72` | worker prompt with retry/append gates, then exact-string retry trigger | After formatting retry for prose plus append mutation, write `worker-report.md`; do not repeat `cat >>` or any source mutation | Earlier replay repeated `cat >>`; after exact-string trigger, provider returned HTTP 502 | Provider-blocked after latest patch; earlier behavior rejected | Reduced equivalent required |
| P33 exact append-policy denial | P32 source plus simulated declared shell-policy denial | latest worker prompt and declared shell policy denial | After append-only source mutation is rejected, write `worker-report.md`; no alternate edit command | Provider returned HTTP 502 | Provider-blocked; not behavior evidence | Reduced equivalent required |
| P34 exact post-diff overclaim current worker | `.pragma/swe-bench-pro/20260613T144437Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma/000067-69198529fccd-8091eca93cc9` | worker prompt with claim gate | After approved-path `git diff`, report only; do not overclaim IDs whose required behavior remains unimplemented | Provider returned HTTP 502 after latest patch | Provider-blocked; not behavior evidence | Reduced equivalent required |
| P35 reduced format-retry after mutation | `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P35-reduced-format-retry-after-mutation` | latest worker prompt | Same invariant as P32 on reduced payload | Provider returned HTTP 500 | Provider-blocked; not behavior evidence | Smaller reduced equivalent required |
| P36 reduced append-policy denial | `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P36-reduced-append-policy-denial` | latest worker prompt and declared shell policy denial | Report only, no mutation, preserve IDs exactly | HTTP 200; reported only and did not mutate, but invented one acceptance ID | Rejected | Added opaque-ID rule and reran smaller reduced equivalent |
| P37 reduced post-diff no-overclaim | `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P37-reduced-post-diff-no-overclaim` | latest worker prompt | Report only after approved-path diff; only implemented struct/enum IDs addressed; defaults and file validation missing | Earlier replay passed; after latest prompt refresh provider returned HTTP 500 | Provider-blocked after latest patch | Smaller reduced equivalent required |
| P38 minimal format-retry after mutation | `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P38-minimal-format-retry-after-mutation` | latest worker prompt | Formatting retry after prior prose plus append mutation writes report only; no mutation; exact IDs preserved | HTTP 200; wrote `worker-report.md`, no mutation, IDs `ID-A` and `ID-B` preserved as missing | Pass for reduced equivalent | Keep retry gate plus declared shell policy |
| P39 minimal policy-denial ID preservation | `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P39-minimal-policy-denial-id-preservation` | latest worker prompt and declared shell policy denial | Policy denial writes report only; no alternate mutation; exact IDs preserved | HTTP 200; wrote `worker-report.md`, no mutation, IDs `ID-A` and `ID-B` preserved as missing | Pass for reduced equivalent | Keep policy-denial and opaque-ID gates |
| P40 minimal post-diff no-overclaim | `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P40-minimal-post-diff-no-overclaim` | latest worker prompt | Approved-path diff for struct/enum only writes report; defaults and file-validation IDs remain missing | HTTP 200; wrote `worker-report.md`, no edits, addressed only `ID-STRUCT`, and left `ID-DEFAULT` and `ID-VALIDATE` missing | Pass for reduced equivalent | Keep claim gate |

Post-P40 checks:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P38-minimal-format-retry-after-mutation --require-responses`: pass.
- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P39-minimal-policy-denial-id-preservation --require-responses`: pass.
- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P40-minimal-post-diff-no-overclaim --require-responses`: pass.
- `git diff --check`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go test ./internal/orchestration -run '^$'`: pass.
- `go test ./internal/query -run '^$'`: pass.
- Production leak scan over `personas-research-v2`, `orchestrations`, and
  `internal` for Flipt/Kubernetes/task strings and `git_diff_scope`: no
  matches.

Promotion status: do not start a new full run until the next hard preflight
also passes. The current failure class has passing reduced replay evidence for
the live declared-policy path, the formatting-retry path, and the post-diff
overclaim path. Exact full-payload replays for the latest prompts are currently
provider-blocked, and the earlier exact format-retry behavior was rejected, so
the promotion gate remains closed until the next hard preflight and replay
decision explicitly justify promotion.

## 2026-06-14 Acceptance Auditor Handoff Provenance Gate

Source run:
`.pragma/swe-bench-pro/20260614T000003Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Failure source:

- Exact dumped turn:
  `.pragma/prompt-ab/20260614T000004Z-acceptance-handoff-provenance/source-dump/turn-000061`.
- The acceptance auditor entered a retry loop after the runtime rejected
  corrupted `source_quote` values. It then read coordination artifacts directly
  from shell actions instead of using the rendered handoff blocks.
- The rendered `acceptance_map` handoff still contained correct task source
  quotes, while the surrounding engineering context and later rewritten
  acceptance map contained corrupted project naming.
- Turn `000061` response attempted another direct read of the rendered
  acceptance artifact path instead of producing corrected outputs from the
  handoff content.

Invariant:

- Declared task-derived acceptance fields must be preserved from the rendered
  handoff artifact unless a declared contract allows mutation.
- A state that receives rendered handoff inputs must not reread those declared
  handoff artifacts before writing its required outputs.
- Runtime checks must derive protected paths and preserved fields from
  orchestration declarations, not from task, stack, repository, persona, state,
  command, or baseline strings.

Patch:

- Added `shell_policy.handoff_inputs: rendered` as a generic state policy. The
  runtime derives protected input paths from the transition handoff artifacts
  and declared writable paths from the state's output artifacts.
- Added `json_each_fields_equal_handoff_artifact` as a generic artifact
  integrity check. It compares declared fields by a declared key field against
  the rendered handoff snapshot captured before the state runs.
- Applied that check to the acceptance auditor's acceptance-map artifact for
  existing task-derived fields: `task_text`, `source_quote`,
  `behavior_surface`, `repo_surfaces_to_verify`, and `blocking_if_missing`.
- Removed the prompt-level command-name warning and shell example from
  `swe_acceptance_auditor.yaml`; runtime policy now owns the enforceable
  boundary.
- Kept the fix generic: no observed command names, task strings, repository
  names, provider/model names, or concrete failure paths were added to generic
  runtime code.

Evidence:

- `go test ./internal/orchestration ./internal/query -run '^$'`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `git diff --check`: pass.
- Production baseline leak scan:
  `rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\\.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config|Flixt" personas-research-v2 orchestrations internal --glob '!**/*_test.go'`: no matches.

Replay decision:

- The exact captured model turn is retained as failure evidence, but this
  particular fix is an enforceable shell-loop/runtime contract that runs after
  model output. A single raw HTTP replay cannot prove the rejection hook because
  raw replay stops at model text and does not execute Pragma's command-policy
  layer.
- The dumped payload still matters: it proves the abstract contract failure and
  the concrete model response that the new state policy would reject before
  execution.

Promotion status:

- Static and schema checks are green.
- Start the next full run only after this entry is reviewed against any
  remaining production hardcoding scan findings. The prior transport timeout
  patch remains in place and captured retryable provider failures instead of
  silent hangs.

## 2026-06-14 Reviewer Validation Dominance Gate

Source run:
`.pragma/swe-bench-pro/20260614T085207Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Failure source:

- Raw turn:
  `raw-http-pragma/000110-ff5a66957cd1-7e9e720b27b2/response.raw`.
- Dumped payload:
  `.pragma/prompt-ab/20260614T000005Z-reviewer-validation-overclaim/source-dump/turn-000110`.
- The targeted validator reported no validated acceptance IDs for the current
  slice and marked multiple acceptance IDs insufficient.
- The engineering reviewer then promoted one acceptance ID into
  `acceptance_status.validated` and classified the slice as complete, even
  though the validator artifact did not support that promotion.

Invariant:

- Reviewer completion claims must be dominated by the validator artifact passed
  in the rendered handoff.
- A reviewer may explain why a failure is environmental or unrelated, but it
  must not mark an acceptance ID validated unless that ID appears in the
  validator's declared validated-ID list.
- Runtime checks must derive the allowed values from artifact handoff content
  and declared schema fields, not from task, stack, repository, persona, state,
  command, or baseline strings.

Patch:

- Added `json_array_subset_of_handoff_text_list` as a generic artifact
  integrity check. It reads a JSON string array by a dotted field path and
  requires every value to be present in a named text-list field from a rendered
  handoff artifact snapshot.
- Applied that check to `review_decision`: values in
  `acceptance_status.validated` must be a subset of the handoff
  `targeted_validation` artifact's `validated_acceptance_ids` field.
- Added schema validation for the check's required fields:
  `artifact_id`, `field`, and `text_field`.
- Kept the fix generic: no observed task strings, acceptance IDs, repository
  names, provider/model names, file paths from the benchmark repo, or concrete
  validation commands were added to runtime code or orchestration policy.

Evidence:

- `go test ./internal/orchestration ./internal/query -run '^$'`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `git diff --check`: pass.
- Production baseline leak scan:
  `rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\\.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config|Flixt" personas-research-v2 orchestrations internal --glob '!**/*_test.go'`: no matches.

Replay decision:

- The exact raw payload remains the failure witness. Raw HTTP replay cannot
  prove this gate because the enforcement happens after model output, when the
  shell-loop completion check reads written artifacts and rendered handoff
  snapshots.
- The next promotion step is a fresh run that reaches reviewer completion and
  demonstrates either a blocked retry for unsupported validated IDs or a
  reviewer decision whose validated IDs are a subset of the validator artifact.

## 2026-06-14 Slice Plan ID Preservation Gate

Source run:
`.pragma/swe-bench-pro/20260614T091841Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.

Failure source:

- `pragma.stdout.log` lines 949-953: the planner wrote three
  `acceptance_ids` into `slice-plan.json`.
- `pragma.stdout.log` lines 1010-1014: the slice-plan auditor rewrote one ID
  while rewriting the audited `slice-plan.json`.
- `pragma.stdout.log` line 1065: the auditor's prose still referenced the
  original ID, proving the corruption was an output artifact mutation rather
  than a deliberate routing correction.

Invariant:

- A plan auditor may correct coverage, scope, route, and worker-contract fields,
  but task-identity ID lists inherited from the rendered handoff must remain
  exact unless a declared contract explicitly allows mutation.
- Runtime checks must preserve declared JSON fields by artifact contract, not by
  task strings, ID prefixes, repository names, paths, commands, or regexes over
  observed bad output.

Patch:

- Added `json_fields_equal_handoff_artifact` as a generic artifact integrity
  check. It compares declared dotted JSON fields in the current output artifact
  against the rendered handoff snapshot for a declared artifact.
- Applied it to the slice-plan auditor's `slice_plan` output for the existing
  task-identity fields `acceptance_ids` and `missing_acceptance_ids`.
- Kept the fix generic: no observed acceptance ID values, task strings,
  repository names, provider/model names, exact file paths from the benchmark
  repo, commands, or observed corrupted strings were added to production code or
  orchestration policy.

Evidence:

- `go test ./internal/orchestration ./internal/query -run '^$'`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `git diff --check`: pass.
- Production baseline leak scan:
  `rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\\.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config|ACCEPT-K8-AUTH|Flixt" personas-research-v2 orchestrations internal --glob '!**/*_test.go'`: no matches.

Replay decision:

- The exact raw payload remains useful failure evidence, but the promoted fix is
  a runtime artifact-completion gate. Raw HTTP replay can show the model's bad
  output, but cannot prove the shell-loop rejection because that happens after
  the response writes artifacts and submits completion.
- The next promotion step is a fresh run that reaches the slice-plan auditor and
  demonstrates either a rejected mutated ID list or a preserved audited
  `slice_plan` ID list.

## 2026-06-14 Runtime Orchestration Hardcoding Cleanup

Failure source:

- Static production-code scan found generic runtime branches on concrete
  orchestration/state/persona/artifact names.
- Removed branches included a definition-name invariant for one orchestration,
  a persistent-conversation check for one control state name, and prompt
  attachment injection for one artifact/persona pair.
- Full `go test ./internal/orchestration` then exposed an active YAML/load
  mismatch: one `foreach_next` state declared a handoff path without a persona.

Invariant:

- Generic orchestration runtime must not branch on concrete orchestration,
  state, persona, artifact, task, repository, command, provider, or benchmark
  strings.
- Handoff ownership must be declared in orchestration data, not inferred from
  specific state names or persona IDs.

Patch:

- Added declarative `conversation: persistent` on states and changed runtime
  persistent-conversation selection to read that field.
- Added declarative artifact `prompt_attachments` and moved source-edit
  transport injection from an artifact/persona name branch to a declared
  attachment.
- Removed the definition-specific invariant function; required transition
  handoff artifacts are now represented only by transition `handoff` entries.
- Added `foreach_next.handoff_mode` with generic `persona` and `control`
  ownership modes. Persona mode preserves the existing persona-authored
  selected-item handoff. Control mode writes a minimal selected-item handoff
  directly after cursor selection.
- Updated existing YAML to declare those modes and attachments explicitly.

Evidence:

- `go test ./internal/orchestration`: pass.
- `go test ./internal/query -run '^$'`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/playful-checklist-loop.yaml --persona-dir personas-research-v2 --details compact`: pass with expected missing-persona-file warnings for that fixture.
- `go run ./cmd/pragma orchestration visualize orchestrations/prompt-control-v2-benchmark.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- Production runtime hardcoded-branch scan:
  `rg -n "prompt-control-v2-benchmark|state\\.ID == \\\"next_item\\\"|artifact\\.ID == \\\"current_item\\\"|state\\.Persona == \\\"item_worker\\\"|validateDefinitionSpecificInvariants|requireTransitionHandoff|parade_next|float_artist" internal/orchestration internal/query --glob '*.go' --glob '!**/*_test.go'`: no matches.
- Production baseline leak scan:
  `rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\\.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config|ACCEPT-K8-AUTH|Flixt" personas-research-v2 orchestrations internal --glob '!**/*_test.go'`: no matches.
- `git diff --check`: pass.

Provider status:

- Live replay of the first captured benchmark turn remains provider-blocked:
  `.pragma/prompt-ab/20260614T093231Z-provider-recovery-check/first-turn-replay/response-latest.raw`
  returned HTTP 500 with `InternalServerError`.
- Do not start another full benchmark until this first-turn replay returns a
  usable model response.

## 2026-06-14 Validation Command Hardcoding Cleanup

Failure source:

- Static production-code scan found runtime classification of validation command
  intent using a fixed command-name list.
- The list treated selected command names as discovery-only validation and
  rejected them from acceptance-map `required_validation`.

Invariant:

- Generic runtime must not infer task, evidence, or validation intent from
  concrete command names.
- Runtime checks may enforce generic structure they can prove, such as
  non-placeholder validation requirements and path portability, but semantic
  command intent must come from declared orchestration contracts or persona
  responsibility.

Patch:

- Removed the command-name classifier from
  `json_each_behavior_validation_commands`.
- Kept generic validation checks for placeholder validation entries and absolute
  repository paths.
- Updated the focused test fixture to assert the remaining generic behavior
  instead of a concrete command-name rejection.

Evidence:

- `go test ./internal/orchestration -run 'TestRequiredOutputCompletionCheckRejectsWeakAcceptanceMap|TestRequiredOutputCompletionCheckAcceptsBehaviorAcceptanceMap'`: pass.
- `go test ./internal/orchestration ./internal/query -run '^$'`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- Production runtime hardcoded-branch scan:
  `rg -n "prompt-control-v2-benchmark|state\\.ID == \\\"next_item\\\"|artifact\\.ID == \\\"current_item\\\"|state\\.Persona == \\\"item_worker\\\"|validateDefinitionSpecificInvariants|requireTransitionHandoff|parade_next|float_artist|discoveryOnlyValidation|case \\\"grep\\\"|\\\"rg\\\", \\\"ls\\\"" internal/orchestration internal/query --glob '*.go' --glob '!**/*_test.go'`: no matches.
- Production baseline leak scan:
  `rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\\.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config|ACCEPT-K8-AUTH|Flixt" personas-research-v2 orchestrations internal --glob '!**/*_test.go'`: no matches.
- `git diff --check`: pass.

Provider status:

- The first-turn replay recovered:
  `.pragma/prompt-ab/20260614T093231Z-provider-recovery-check/first-turn-replay/response-current.raw`
  returned HTTP 200 with a valid bash response.
- A fresh full benchmark can be started from the current worktree.

## 2026-06-14 Approved Hardcoding Removal Slice

Scope correction:

- The benchmark harness is out of scope for this goal. The cleanup in this
  slice was limited to Pragma runtime, orchestration YAML, production persona
  prompts, and production-looking legacy orchestration/persona assets.
- The user explicitly approved the revised in-scope abstraction boundaries
  before implementation.

Approved abstraction boundaries:

1. Generic `foreach_next` runtime must not own a checklist-worker item schema.
   Item schema, cursor artifact metadata, and dependency semantics must be
   declared by orchestration YAML.
2. Legacy checklist-loop assets must not live in production orchestration and
   persona directories unless they are still the production flow. They are now
   isolated as examples.
3. Production prompts must not require a specific shell transport or a specific
   version-control output marker when the real invariant is artifact completion
   or changed-file evidence.
4. Production orchestration names must describe reusable flow behavior, not
   benchmark calibration status.

Patch:

- `internal/orchestration/orchestration.go`
  - Reduced `ChecklistItem` to generic runtime fields `id`, `status`, and
    extra JSON preservation.
  - Added `foreach_next` declarations for cursor artifact ID/description,
    item contract text, and dependency behavior.
  - Added neutral dependency config keys:
    `dependency_ids_field`, `dependency_reason_field`,
    `deferred_dependency_field`, `blocked_status`, `unblocked_status`,
    `auto_block_deferred`, and `auto_unblock_dependents`.
  - Removed runtime ownership of concrete item fields such as editable-file,
    validation, deferred-validation, dependency-ID, and dependency-reason
    semantics. Those fields now exist only as values in orchestration/persona
    contracts.
- `internal/orchestration/runner.go`
  - `controlPersonaHandoffArtifacts` now emits a cursor handoff only when the
    control declares the cursor artifact ID.
  - `RenderNextForEachContract` now renders the declared item contract from
    YAML, falling back only to the minimal generic list shape with `id` and
    `status`.
  - Dependency rule text now references declared dependency field names instead
    of runtime-owned checklist field names.
- `orchestrations/task-evidence-item-loop.yaml`
  - Renamed from `orchestrations/prompt-control-v2-benchmark.yaml`.
  - Changed orchestration name from `prompt-control-v2-benchmark` to
    `task-evidence-item-loop`.
  - Declares the checklist item contract and dependency field mapping that this
    flow expects.
- `orchestrations/playful-checklist-loop.yaml`
  - Declares cursor artifact metadata and a minimal list item contract for the
    non-Flipt/generalization fixture.
- `examples/orchestrations/architect-checklist-item-loop-final.yaml`
  - Moved from production `orchestrations/`.
- `examples/personas/checklist-loop/*.yaml`
  - Moved checklist-loop-only personas out of production `personas/`.
- `personas-research-v2/checklist_writer.yaml`
  - Removed the requirement to use a heredoc shell script. The prompt now
    requires creating the declared output artifact inside the fenced bash block.
- `personas-research-v2/swe_targeted_validator.yaml`
  - Removed quoted-heredoc/static-success wording. The prompt now requires the
    report to be derived from captured command status after the command result
    is known.
- `personas-research-v2/swe_engineering_worker.yaml`
  - Removed `diff --git` as the terminal gate marker. The prompt now keys on a
    successful command result plus changed files inside approved edit paths.
- `internal/orchestration/orchestration_test.go`
  - Updated test fixtures for the now-generic item model and renamed/moved
    orchestration paths. No new test coverage was added.

Evidence:

- Compile-only checks:
  - `go test ./internal/orchestration ./internal/query -run '^$'`: pass.
- Active orchestration visualization:
  - `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
  - `go run ./cmd/pragma orchestration visualize orchestrations/task-evidence-item-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- Example/non-Flipt checks:
  - `go run ./cmd/pragma orchestration visualize examples/orchestrations/architect-checklist-item-loop-final.yaml --persona-dir examples/personas/checklist-loop --details compact`: pass.
  - `go run ./cmd/pragma orchestration visualize orchestrations/playful-checklist-loop.yaml --persona-dir personas-research-v2 --details compact`: graph rendered; warnings were limited to the expected absent playful persona files in that fixture.
- Formatting and whitespace:
  - `gofmt -w internal/orchestration/orchestration.go internal/orchestration/runner.go internal/orchestration/orchestration_test.go`: applied.
  - `git diff --check`: pass.
- Production baseline/string leakage scan:
  - `rg -n 'Flipt|Kubernetes|kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\.proto|METHOD_KUBERNETES|instance_flipt|go\.flipt|mage Proto|go build ./\.\.\.|go test ./internal/config|SWE-bench Pro|SWE-bench|benchmark|baseline calibration|first task|prior benchmark|prompt-control-v2-benchmark|minimaxai/minimax-m2\.7|lilac' internal/orchestration internal/query cmd/pragma/orchestration.go orchestrations personas personas-research-v2 --glob '!**/*_test.go' --glob '!testdata/**' --glob '!**/*.md'`: no matches.
- Prompt transport and VCS-marker scan:
  - `rg -n 'diff --git|heredoc|blocked_by_field|block_reason_field|deferred_until_field|missing_block_reason|BlockedBy|BlockReason' internal/orchestration cmd/pragma/orchestration.go orchestrations personas personas-research-v2 --glob '!**/*_test.go' --glob '!testdata/**' --glob '!**/*.md'`: no matches.
- Runtime-owned item schema scan:
  - `rg -n 'current_item|allowed_files|acceptance_check|validation_command|validation_deferred_until|blocked_by|block_reason|report_changed_files|producer_command|generated_files|current-item\.json|checklist\.json' internal/orchestration cmd/pragma/orchestration.go --glob '!**/*_test.go' --glob '!testdata/**'`: no production runtime-owned item schema hits beyond generic control/check type names.

Remaining completion requirements:

- This slice did not run a fresh full benchmark.
- The full goal remains incomplete until replay evidence, full-run/generalization
  evidence, and a final completion audit prove every requirement in
  `pragma-goal.md`.

## 2026-06-14 Prompt Replay Evidence For Hardcoding Cleanup

Replay bundle:

- `.pragma/prompt-ab/20260614T114813Z-hardcoding-cleanup-current-prompts/`

Cases:

- `HW1-worker-post-diff-generic-terminal-gate`
  - Source capture:
    `.pragma/prompt-ab/20260613T144437Z-worker-retry-append-overclaim/P40-minimal-post-diff-no-overclaim/request.json`
  - Mutation: replaced only `messages[0].content` with the current prompt from
    `personas-research-v2/swe_engineering_worker.yaml`.
  - Result: HTTP 200, `response_ok`.
  - Behavioral read: produced `/tmp/pragma/swe/worker-report.md` and
    `COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`; no further edit command was
    emitted after the successful changed-file output.
- `TV1-targeted-validator-status-derived-report`
  - Source capture:
    `.pragma/prompt-ab/20260613T133803Z-targeted-validator-toolchain-blocker/P20-reduced-toolchain-output-gate/request.json`
  - Mutation: replaced only `messages[0].content` with the current prompt from
    `personas-research-v2/swe_targeted_validator.yaml`.
  - Result: first attempt returned provider HTTP 500
    `InternalServerError`; retry returned HTTP 200, `response_ok`.
  - Behavioral read: produced `/tmp/pragma/swe/targeted-validation.md` with
    `EXIT_STATUS: 1` and failing compiler evidence from the captured command
    output. It did not issue a new validation command and did not emit a static
    success report.
- `CW1-checklist-writer-artifact-transport`
  - Source capture:
    `.pragma/prompt-ab/persona-v2-artifact-turn-20260602T162715Z/checklist_writer_after_verdict_and_plan/request.json`
  - Mutation: replaced `messages[0].content` with the current prompt from
    `personas-research-v2/checklist_writer.yaml`.
  - Scope note: reduced replay, because the source checklist capture used the
    generic tool system prompt rather than a persona-specific system prompt.
  - Result: HTTP 200, `response_ok`.
  - Behavioral read: produced `/tmp/pragma/checklist.json`; the embedded JSON
    parsed successfully and contained three checklist items.

Audit evidence:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260614T114813Z-hardcoding-cleanup-current-prompts --require-responses`: pass, all three cases `response_ok`.
- `jq -r '.choices[0].message.content' .../CW1-checklist-writer-artifact-transport/response.raw | sed -n '/^{/,/^EOF/p' | sed '$d' | jq -e '.items | length'`: returned `3`.

Remaining completion requirements:

- This slice still did not run a fresh full benchmark.
- The full goal remains incomplete until full-run/generalization evidence and a
  final completion audit prove every requirement in `pragma-goal.md`.

Post-replay verification:

- `go test ./internal/orchestration ./internal/query -run '^$'`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/task-evidence-item-loop.yaml --persona-dir personas-research-v2 --details compact`: pass.
- `go run ./cmd/pragma orchestration visualize examples/orchestrations/architect-checklist-item-loop-final.yaml --persona-dir examples/personas/checklist-loop --details compact`: pass.
- `go run ./cmd/pragma orchestration visualize orchestrations/playful-checklist-loop.yaml --persona-dir personas-research-v2 --details compact`: graph rendered; warnings were limited to expected missing playful persona files in that fixture.
- `git diff --check`: pass.
- Production baseline/string leakage scan:
  `rg -n 'Flipt|Kubernetes|kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth\.proto|METHOD_KUBERNETES|instance_flipt|go\.flipt|mage Proto|go build ./\.\.\.|go test ./internal/config|SWE-bench Pro|SWE-bench|benchmark|baseline calibration|first task|prior benchmark|prompt-control-v2-benchmark|minimaxai/minimax-m2\.7|lilac' internal/orchestration internal/query cmd/pragma/orchestration.go orchestrations personas personas-research-v2 --glob '!**/*_test.go' --glob '!testdata/**' --glob '!**/*.md'`: no matches.
- Prompt transport and VCS-marker scan:
  `rg -n 'diff --git|heredoc|blocked_by_field|block_reason_field|deferred_until_field|missing_block_reason|BlockedBy|BlockReason' internal/orchestration cmd/pragma/orchestration.go orchestrations personas personas-research-v2 --glob '!**/*_test.go' --glob '!testdata/**' --glob '!**/*.md'`: no matches.
- Runtime-owned item schema scan:
  `rg -n 'current_item|allowed_files|acceptance_check|validation_command|validation_deferred_until|blocked_by|block_reason|report_changed_files|producer_command|generated_files|current-item\.json|checklist\.json' internal/orchestration cmd/pragma/orchestration.go --glob '!**/*_test.go' --glob '!testdata/**'`: only generic control/check type names remained.
- Harness scope check:
  `git status --short -- tools && git diff --name-only -- tools`: no output.

## 2026-06-14 Fresh Full Run Provider Blocker

Run:

- `.pragma/swe-bench-pro/20260614T115443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

Command:

- `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --output-dir .pragma/swe-bench-pro/20260614T115443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --provider lilac --model minimaxai/minimax-m2.7 --orchestration /pragma/orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir /pragma/personas-research-v2 --generator-toolchain --evaluate`

Observed run state:

- The container launched and the generator toolchain preflight was healthy.
- `agent-status.txt` was not written because the container was stopped after a
  repeated provider 502 loop.
- Harness process exit was `137`, caused by stopping the stuck container.
- No evaluator result was produced.

Inspection artifacts:

- Raw HTTP dump:
  `.pragma/analysis/20260614T115443Z-flipt-full-run-provider-blocker/`
- Last successful model turn before the blocker:
  `turn-000060`
- Blocked request family:
  `turn-000061` through `turn-000070`

Failure classification:

- Source: provider behavior.
- The blocked request was a targeted-validator follow-up after the immediate
  chat history contained the validation command result. The request was
  well-formed and included the command output needed for the validator to write
  the validation artifact.
- Ten consecutive captured responses for the same request returned
  `HTTP 502 Bad Gateway` with body `All upstream targets failed`.
- The ten 502 response bodies had identical SHA256:
  `25a0ca34cd9a5848654b00b163641588335a29a200380d03b1cbef667ad42dad`.
- No production fix was identified. Adding behavior keyed to the provider,
  task, command output, paths, state, or artifact names would be hardcoding.

Exact replay recheck:

- Replay bundle:
  `.pragma/prompt-ab/20260614T121000Z-full-run-provider-blocker-recheck/`
- Case:
  `TV2-exact-blocked-targeted-validator-turn-061`
- Mutation:
  none; the replay copied the exact `turn-000061` request payload.
- Command:
  `go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260614T121000Z-full-run-provider-blocker-recheck/TV2-exact-blocked-targeted-validator-turn-061 --provider lilac --format raw --out .pragma/prompt-ab/20260614T121000Z-full-run-provider-blocker-recheck/TV2-exact-blocked-targeted-validator-turn-061/response.raw --timeout 180s`
- Result:
  `HTTP 502 Bad Gateway` with body `All upstream targets failed`.
- Audit:
  `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260614T121000Z-full-run-provider-blocker-recheck`: completed and reported `response_error`.
  `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260614T121000Z-full-run-provider-blocker-recheck --require-responses`: failed as expected because the only replay response is an error response, not usable model output.

Minimal next action:

- Retry the exact blocked replay or rerun the full benchmark only after the
  provider returns usable model output for this request class.

Completion audit status:

- Production orchestration/persona files express the current FSM and handoff
  contracts declaratively: evidence currently passes visualization and static
  scans.
- Generic runtime contains no known special cases for this SWE-bench Pro
  persona set: current scans show only generic control/check names.
- Production prompts contain no known baseline-task or stack-specific leakage:
  current scans are clean.
- AB replay evidence passes for changed persona/FSM contracts from the approved
  hardcoding cleanup slice.
- Fresh Flipt full-run evidence exists, but evaluator pass is blocked by a
  provider-side 502 loop, not by a completed evaluator result.
- Non-Flipt/generalization check exists via the playful checklist-loop
  visualization and the moved legacy example visualization.
- The goal remains active because the full-run requirement is satisfied only by
  the external-blocker branch, and that branch should be rechecked when provider
  health returns.

## 2026-06-14 Second Fresh Full Run Provider Blocker

Run:

- `.pragma/swe-bench-pro/20260614T121148Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

Reason for run:

- The exact replay of the previous full-run blocker recovered with usable model
  output in
  `.pragma/prompt-ab/20260614T122000Z-full-run-provider-blocker-third-check/TV3-exact-blocked-targeted-validator-turn-061`.
- A fresh full benchmark was therefore started from the same cleaned
  orchestration/persona system.

Observed run state:

- The container launched and the generator toolchain preflight was healthy.
- The run progressed through:
  - `swe_repo_survey`
  - `swe_acceptance_mapper`
  - `swe_environment_survey`
- The run blocked during `swe_theory_keeper`.
- `agent-status.txt` was not written because the container was stopped after
  repeated provider 502 responses.
- Harness process exit was `137`, caused by stopping the stuck container.
- No evaluator result was produced.

Inspection artifacts:

- Raw HTTP dump:
  `.pragma/analysis/20260614T121148Z-flipt-full-run-provider-blocker/`
- First blocked request:
  `turn-000038`
- Blocked request family:
  `turn-000038` through `turn-000055`

Failure classification:

- Source: provider behavior.
- The blocked request was a `swe_theory_keeper` payload after the environment
  survey completed. The request was well-formed and included rendered handoff
  content.
- The provider returned repeated `HTTP 502 Bad Gateway` responses with body
  `All upstream targets failed`.
- The response bodies in `turn-000038` through `turn-000055` had identical
  SHA256:
  `25a0ca34cd9a5848654b00b163641588335a29a200380d03b1cbef667ad42dad`.
- No production fix was identified. Adding behavior keyed to the provider,
  task, command output, paths, state, or artifact names would be hardcoding.

Exact replay recheck:

- Replay bundle:
  `.pragma/prompt-ab/20260614T123000Z-full-run-theory-keeper-provider-recheck/`
- Case:
  `TK1-exact-blocked-theory-keeper-turn-038`
- Mutation:
  none; the replay copied the exact `turn-000038` request payload.
- Command:
  `go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260614T123000Z-full-run-theory-keeper-provider-recheck/TK1-exact-blocked-theory-keeper-turn-038 --provider lilac --format raw --out .pragma/prompt-ab/20260614T123000Z-full-run-theory-keeper-provider-recheck/TK1-exact-blocked-theory-keeper-turn-038/response.raw --timeout 180s`
- Result:
  `HTTP 502 Bad Gateway` with body `All upstream targets failed`.
- Audit:
  `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260614T123000Z-full-run-theory-keeper-provider-recheck`: completed and reported `response_error`.

Minimal next action:

- Retry the exact blocked replay before another full benchmark. Rerun the full
  benchmark only after this request class returns usable model output.

Second resumed provider recheck:

- Replay bundle:
  `.pragma/prompt-ab/20260614T124000Z-full-run-theory-keeper-provider-second-resume-check/`
- Case:
  `TK2-exact-blocked-theory-keeper-turn-038`
- Mutation:
  none; the replay copied the exact `turn-000038` request payload.
- Result:
  `HTTP 500 Internal Server Error` with provider body
  `EngineCore encountered an issue. See stack trace (above) for the root cause.`
- Audit:
  `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260614T124000Z-full-run-theory-keeper-provider-second-resume-check`: completed and reported `response_error`.
- Interpretation:
  the exact same full-run request still does not return usable model output.
  The error class changed from provider 502 to provider 500, but the blocker is
  still external provider transport failure, not a production Pragma behavior
  signal.

Current completion audit status:

- The full benchmark requirement remains externally blocked by provider
  transport failures.
- This is the first resumed goal turn after the prior blocked status with the
  same external provider blocker; do not mark blocked again until the same
  blocker repeats for three consecutive resumed goal turns.

## 2026-06-14 Runner Capability Context Fix

Source run:

- `.pragma/swe-bench-pro/20260614T123209Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

Observed terminal result:

- Agent phase completed with `agent_status=0`.
- Evaluator ran and reported `Overall accuracy: 0.0`.
- Evaluator stderr contained:
  `internal/config/authentication.go:334:27: undefined: auth.Method_METHOD_KUBERNETES`.

Failure identification:

- Invariant failed: current runner/tool capability facts must remain
  authoritative and contradiction-aware across persona handoffs.
- Generic abstraction missing: runner capability evidence and capability
  contradiction handling in the environment context.
- Concrete failure strings in the evidence included the target task's generated
  symbol and generated-source surfaces. These were not promoted into production
  prompts.
- Fixing by naming the observed command, generated file, task, or repository
  would be hardcoding. The promoted contract is capability-based and generic.

Contradictory evidence:

- `toolchain-preflight.log` in the run output recorded generator capabilities as
  available.
- The environment survey artifact recorded local missing-tool facts and later
  personas preserved those as blockers.
- The harness/tooling files were not edited.

Approved abstraction boundary:

- Add generic runner capability context to `environment_context`.
- Preserve runner capability evidence separately from local probes.
- Record disagreements as capability contradictions / environment contract
  drift instead of converting them into durable missing-tool blockers.
- Add bounded executable-root discovery when repository evidence says a
  producer capability is relevant but PATH probing misses it.

Focused replay evidence:

- Dump:
  `.pragma/prompt-ab/20260614T130000Z-runner-capability-context/dump`
- Source turn:
  `turn-000025`, the first environment-survey request.
- Audit cases:
  `.pragma/prompt-ab/20260614T130000Z-runner-capability-context/audit-cases`

Replay variants:

- `RC1`: added runner capability evidence and a generic preservation rule.
  Result: response preserved a runner capability section, but wrote static local
  availability conclusions. Rejected as too weak.
- `RC2`: required runner/local separation and a contradiction section.
  Result: response wrote the required sections, but still relied on static
  availability prose. Rejected as partial.
- `RC3`: required variable-rendered capability sections and computed
  contradictions. Result: first replay hit provider 500; retry returned 200 but
  still wrote static conclusions in a quoted artifact. Rejected as proof that
  prompt-only wording is not sufficient.
- `RC4-production-prompt`: replaced the source turn's system prompt with the
  edited production `swe_environment_survey` prompt and supplied generic runner
  capability evidence. Result: response returned 200, wrote `Runner Capability
  Evidence`, wrote `Capability Contradictions`, and no longer made local missing
  tool facts the durable blocker. Promoted as the production-prompt replay
  evidence for this contract change, with full-run integration still required.

Promoted production changes:

- `personas-research-v2/swe_environment_survey.yaml`
  - Adds runner capability evidence and capability contradiction sections.
  - Requires bounded executable-root discovery for repository-derived producer
    candidates when PATH probes miss relevant capabilities.
  - States that local probe misses do not downgrade runner-provided available
    status.
- `personas-research-v2/swe_theory_keeper.yaml`
  - Preserves runner capability evidence and contradiction records.
  - Prevents stale missing-tool notes from becoming durable blockers when
    current runner evidence says a capability is available.
- `personas-research-v2/swe_targeted_validator.yaml`
  - Classifies runner/local capability disagreement as environment contract
    drift unless the validator's own command directly exercised the capability
    and failed.
  - Rejects generated-output hand edits as the next validation suggestion when
    the needed producer capability was not actually exercised.
- `orchestrations/swe-bench-pro-engineering-loop.yaml`
  - Updates environment-context handoff descriptions to include runner
    capability evidence and capability contradictions.

Verification:

- `go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260614T130000Z-runner-capability-context/audit-cases --require-responses`
  passed with `response_ok` for `RC1`, `RC2`, `RC3`, and
  `RC4-production-prompt`.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed.
- `git diff --check -- personas-research-v2/swe_environment_survey.yaml personas-research-v2/swe_theory_keeper.yaml personas-research-v2/swe_targeted_validator.yaml orchestrations/swe-bench-pro-engineering-loop.yaml`
  passed.
- Production leakage scan over the edited files for the prior task, stack,
  provider, model, and observed tool strings returned no matches.
- `go test ./internal/orchestration ./internal/query -run '^$'` passed.

Next action:

- Run a fresh full benchmark because the last full run reached evaluator and
  failed on an environment/capability-contract issue that is now addressed at
  the production contract level.

## 2026-06-15 Seeded Run Artifacts

Fresh-run failure prompting this change:

- Run:
  `.pragma/swe-bench-pro/20260614T131049Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- The benchmark harness preflight showed the runner process had generator
  toolchain capability, but the environment-survey artifact reported no runner
  capability evidence and converted a local probe miss into durable missing-tool
  constraints.
- The harness/tooling files remained out of scope and were not edited.

Approved abstraction boundary:

- Add a generic artifact seed declaration under orchestration artifacts.
- Materialize declared seeded artifacts before the first orchestration state.
- Support caller-provided seed content through a generic CLI option.
- Support a generic process-environment seed source so runtime evidence can be
  handed to personas without benchmark-harness changes.
- Emit normal artifact write metadata for seeded artifacts.

Production changes:

- `internal/orchestration/orchestration.go`
  - Adds `artifact.seed.source` to the artifact domain model.
- `internal/orchestration/runner.go`
  - Adds `RunOptions.SeedArtifacts`.
  - Materializes declared seeded artifacts after run directories are created
    and before persona execution.
  - Adds the built-in `process_environment` seed source, recording cwd, PATH,
    OS/architecture, path entries, and environment variable names without
    probing concrete tools.
- `cmd/pragma/orchestration.go`, `internal/cli/run.go`,
  `internal/slash/command.go`
  - Adds generic seed-content plumbing and the standalone
    `--seed-artifact source=path` option.
- `orchestrations/swe-bench-pro-engineering-loop.yaml`
  - Declares `runner_capability_evidence` as a seeded handoff into
    `swe_environment_survey`.
- `personas-research-v2/swe_environment_survey.yaml`
  - Treats `/tmp/pragma/swe/runner-capability-evidence.md` as a declared input
    artifact and copies it before local probe conclusions.

Verification:

- `go test ./internal/orchestration ./internal/query ./internal/cli ./internal/slash ./cmd/pragma -run '^$'`
  passed.
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed.
- `git diff --check -- internal/orchestration/orchestration.go internal/orchestration/runner.go internal/cli/run.go internal/slash/command.go cmd/pragma/orchestration.go orchestrations/swe-bench-pro-engineering-loop.yaml personas-research-v2/swe_environment_survey.yaml`
  passed.
- Production leakage scan over the edited runtime, CLI, orchestration, and
  persona files for the observed task, harness path, and concrete generator
  strings returned no matches.

Next action:

- Run a fresh full benchmark without editing the benchmark harness.

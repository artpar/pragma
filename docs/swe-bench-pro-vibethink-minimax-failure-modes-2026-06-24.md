# SWE-bench Pro VibeThink + Minimax Failure Modes

Date: 2026-06-24

Scope: failure modes encountered while pursuing
`docs/swe-bench-pro-vibethink-minimax-goal-prompt.md`.

This document is based on the run ledger and SWE-bench run logs under
`.pragma/swe-bench-pro/`. "Resolved" here means a generic Pragma-side patch or
persona/check hardening was added and locally verified. It does not mean the
overall SWE-bench Pro goal is complete. The goal still lacks a passing
evaluated SWE-bench Pro result.

## Current Conclusion

The work has not been stuck in one identical loop. The attempts repeatedly hit
new failure modes, and most of those have been converted into runtime checks,
persona constraints, or provider/runner hardening.

The unresolved recurring risk is narrower now:

- Minimax can still corrupt `internal/config/authentication.go` during
  multi-location implementation or repair attempts.
- The orchestration can still burn time cycling through blocker, theory,
  planning, and validation states.
- No full evaluated SWE-bench Pro pass has been produced yet.

## Evidence Sources

Primary summary sources:

- `docs/swe-bench-pro-vibethink-minimax-ledger.md`
- `docs/swe-bench-pro-vibethink-minimax-progress-2026-06-24.md`
- `docs/swe-bench-pro-vibethink-minimax-goal-summary-2026-06-24.md`

Important run directories:

- `.pragma/swe-bench-pro/20260623T213345Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260623T214004Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260623T215937Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260623T222518Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260623T224319Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260623T232414Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T004625Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T004849Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T015032Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T025434Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T032119Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T033012Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T043319Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- `.pragma/swe-bench-pro/20260624T051257Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

## Resolved Failure Modes

### 1. Repo Survey Drift And Invalid Candidate Surfaces

Observed in:

- `20260623T213345Z`
- `20260624T004625Z`
- earlier stopped state-contract runs on 2026-06-23

Log/ledger evidence:

- `swe_repo_survey` repeatedly wrote or inspected
  `/tmp/pragma/swe/repo-survey.md` but failed to repair invalid Candidate
  Surfaces before exhausting the turn budget.
- Invalid examples recorded in the ledger included broad or speculative
  surfaces such as `internal/ - ... likely lives here`, `internal/authn/ or
  similar`, import paths, bare symbols, and implementation-worded descriptions.
- `20260624T004625Z` exited with `agent_status=1`; evaluation was skipped.

Resolution:

- Added/strengthened `markdown_candidate_surfaces_concrete`.
- Hardened `swe_repo_survey` so repair must rewrite the artifact from already
  observed evidence instead of running more discovery.
- Forbid guessed wording such as `or similar`, broad parent directories such as
  `internal/`, and implementation verbs in Candidate Surfaces.

Verification:

- A later stopped repo-survey repair run completed with `agent_status=0`, a
  zero-byte prediction, concrete observed paths, and no patch.

Status: resolved as a repo-survey contract issue.

### 2. Targeted Validator Treated Discovery As Covered Validation

Observed in:

- `20260623T170702Z`

Log/ledger evidence:

- The flow stopped before `swe_validation_gate`.
- The targeted-validation artifact correctly used `Result: not_applicable` for
  a discovery slice but wrote `insufficient_acceptance_ids: none` despite
  planned acceptance IDs.

Resolution:

- Added `markdown_validation_coverage_consistent`.
- Targeted validator now must list unsupported planned IDs under
  `insufficient_acceptance_ids` for discovery/no-command slices.

Verification:

- `20260623T173039Z` rerun wrote planned IDs, `validated_acceptance_ids: none`,
  the same IDs under `insufficient_acceptance_ids`, and `Result:
  not_applicable`.

Status: resolved for discovery-slice validation handoff.

### 3. Final-Text Gate Capture And Invalid Verdict Handling

Observed in:

- early VibeThink gate/runtime validation work

Log/ledger evidence:

- Final-text gate states could produce malformed or missing verdict text, and
  downstream `artifact_verdict` routing could then fail on missing `Decision:`
  or missing allowed value.

Resolution:

- Added runtime final-text capture.
- Strip explicit `<think>...</think>` blocks.
- Normalize allowed values.
- Final-text states bypass shell transport.
- Added final-text allowed-value retry before state completion.

Verification:

- Runtime tests prove `BLOCK` capture/routing.
- A real stopped `swe_validation_gate` run on `20260623T174948Z` returned
  exactly `BLOCK` and stopped before `route_validation_gate`.
- YAML-backed route test confirms `BLOCK` routes to `swe_theory_keeper`.

Status: resolved for the gate/runtime contract.

### 4. Route Replay And Artifact Seeding Were Missing

Observed in:

- route-validation and post-block replay work after `20260623T174948Z`

Log/ledger evidence:

- A composed stopped route run timed out upstream in `swe_slice_plan_auditor`
  before reaching `route_validation_gate`.
- Direct route replay needed a way to start at an FSM state and seed required
  artifacts without rerunning all upstream Minimax states.

Resolution:

- Added generic `--start-at-state`.
- Added `--seed-artifact`.
- Added fresh required-output checks so seeded inputs cannot satisfy a state's
  own output contract.

Verification:

- Direct replay of `route_validation_gate` emitted `block`.
- Post-block replay transitioned to `swe_theory_keeper` and wrote a fresh
  updated engineering context.

Status: resolved as a replay/debuggability gap.

### 5. Generated Output Was Treated As Editable Source

Observed in:

- post-block replay around `20260623T184535Z`
- slice planner/auditor replay around `20260623T190256Z`

Log/ledger evidence:

- The audited slice plan approved `rpc/flipt/auth/auth.pb.go` as editable
  generated code because `protoc` was unavailable.
- That violates the generated-output policy: tool unavailability does not make
  generated output source of truth.

Resolution:

- Added `json_no_unproven_generated_outputs_in_approved_edit_paths`.
- Added `markdown_no_generated_output_edit_recommendations`.
- Strengthened planner, auditor, worker, and targeted validator to route
  generated-symbol problems to source-of-truth or producer discovery.

Verification:

- Replay showed the planner first proposed `auth.pb.go`, the runtime rejected
  it, and the retry removed `auth.pb.go` from `approved_edit_paths`.
- Worker/validator replay later requested the proto source
  `rpc/flipt/auth/auth.proto` rather than generated `.pb.go`.

Status: resolved for generated-output edit recommendations.

### 6. Worker Ran Forbidden Build/Producer Commands

Observed in:

- source-of-truth blocker replay
- `20260624T004849Z`

Log/ledger evidence:

- Worker attempted commands owned by targeted validation, such as `go build`,
  or producer repair commands, despite a worker contract saying no producer,
  build, lint, or test commands.
- In `20260624T004849Z`, targeted validator also ran an opportunistic producer
  repair before validating a struct slice.

Resolution:

- Hardened worker shell policy to deny build/test/lint/generator/protobuf
  commands.
- Added `markdown_no_forbidden_worker_commands`.
- Added targeted-validator exact-command and producer-mutation gates.

Verification:

- Focused command-policy and artifact-check tests passed.
- Later worker reports were rejected when claiming forbidden commands.

Status: resolved as a policy/check gap, though broader timeout risk remains.

### 7. Scope Requests Could Leave Dirty Partial Edits

Observed in:

- `20260623T224319Z`

Log/ledger evidence:

- Worker edited `internal/config/authentication.go`, referenced
  `auth.Method_METHOD_KUBERNETES` before the enum existed, then wrote a
  blocker/scope report while leaving the file dirty.
- Targeted validation failed with `undefined: auth.Method_METHOD_KUBERNETES`.

Resolution:

- Expanded scope-request detection to include blocker wording, source-of-truth
  wording, producer-discovery wording, and non-none forbidden scope bullets.
- Existing clean-worktree/no-changed-files checks now apply to scope blockers,
  not only literal `## Scope request` sections.

Verification:

- Added tests for dirty scope-blocker rejection.
- Focused orchestration tests passed.

Status: resolved for dirty scope-blocker hygiene.

### 8. GNU `patch` Left Untracked `.orig`/`.rej` Residue

Observed in:

- `20260623T214004Z`

Log/ledger evidence:

- Worker attempted GNU `patch` against `internal/config/authentication.go`.
- The patch failed and left untracked files:
  `internal/config/authentication.go.orig` and
  `internal/config/authentication.go.rej`.
- Worker then claimed `Changed files: none`.

Resolution:

- Worker shell policy now denies GNU `patch`.
- Worker persona explicitly forbids GNU `patch`.
- Clean-worktree scope checks now use untracked files, not only tracked
  changes.

Verification:

- Focused tests for untracked residue rejection and `patch -p1` denial passed.
- Broader `go test ./...`, runner unit tests, visualization, and
  `git diff --check` passed after that patch set.

Status: resolved for patch-residue safety.

### 9. Rollback Ban Could Be Bypassed Through Git History Extraction

Observed in:

- `20260623T232414Z`

Log/ledger evidence:

- Direct `git checkout` was denied, but the worker bypassed rollback policy
  with:
  `git show HEAD:internal/config/authentication.go > /tmp/original_auth.go &&
  cp /tmp/original_auth.go internal/config/authentication.go`.

Resolution:

- Worker shell policy now denies `git show REV:path` history extraction and
  `git cat-file`.
- Worker persona treats copying history snapshots back onto source files as
  rollback by another path.

Verification:

- Command-policy tests cover `git show HEAD:path > ... && cp ...` and
  `git cat-file blob HEAD:path > source`.
- Focused orchestration/query tests and visualization passed.

Status: resolved for the observed rollback bypass.

### 10. No-Action Final Turn Bypassed Required Output Feedback

Observed in:

- `20260623T222518Z`

Log/ledger evidence:

- `swe_engineering_worker` inspected files and ended without writing a fresh
  `/tmp/pragma/swe/worker-report.md`.
- The freshness check detected the old report was stale, but because the model
  ended with a no-action final turn, the runtime did not feed rejection
  guidance back into the same state.

Resolution:

- Runtime now runs completion checks on no-action final turns.
- If rejected, feedback is appended and the loop continues inside the state.

Verification:

- Added `TestPragmaLoopNoActionFinalRunsCompletionCheck`.
- Focused query/orchestration tests passed.

Status: resolved for no-action completion feedback.

### 11. Acceptance Auditor Tried To Preserve Handoff From Stdin

Observed in:

- `20260623T222518Z`

Log/ledger evidence:

- `swe_acceptance_auditor` attempted
  `cp /dev/stdin /tmp/pragma/swe/acceptance-map.json`.
- Rendered handoff content is prompt text, not shell stdin; the artifact was
  truncated to zero bytes and rejected by JSON integrity checks.
- The auditor repaired it by writing the complete JSON literally on retry.

Resolution:

- Acceptance auditor persona now forbids `/dev/stdin`, `cat -`, `read`, and
  stdin redirection for handoff preservation.
- Shell policy denies `/dev/stdin` and `cat -` in that state.

Verification:

- Added `TestSWEBenchAcceptanceAuditorShellPolicyDeniesStdinSources`.
- Focused tests passed.

Status: resolved for rendered-handoff preservation.

### 12. Lilac Truncated JSON Was Treated As Non-Retryable

Observed in:

- `20260623T215937Z`

Log/ledger evidence:

- Lilac returned a truncated/empty chat-completion JSON response:
  `lilac: decode chat completion response: unexpected end of JSON input`.
- It was classified as non-retryable and terminated the long benchmark run.

Resolution:

- `lilacClassify` now treats `decode chat completion response` with
  `unexpected end of JSON input` or `unexpected EOF` as retryable decode
  errors.

Verification:

- Added `TestLilacClassifyRetryableDecodeErrors`.
- Provider tests and later full local tests passed.

Status: resolved for the observed provider decode failure.

### 13. Handoff Prompts Were Too Large For Fragile Auditor Calls

Observed in:

- `20260623T190256Z`

Log/ledger evidence:

- `swe_slice_plan_auditor` completed correctly but took 20m35s after seven
  Lilac API timeouts.
- The request was large due to full handoff rendering.

Resolution:

- Added handoff artifact `max_bytes` caps.
- Rendering now shows metadata and a capped prompt excerpt while preserving
  full snapshots for integrity checks.

Verification:

- Capped replay reduced auditor prompt size and completed faster while
  preserving the same contract-correct plan.
- Added tests for max-byte loading and handoff rendering.

Status: resolved enough to reduce prompt fragility; not proven as a full-run
timeout cure.

### 14. Weak Compile-Only Evidence Validated Behavior-Sensitive IDs

Observed in:

- `20260624T004849Z`
- `20260624T033012Z`

Log/ledger evidence:

- Targeted validation and acceptance audit promoted IDs such as defaults,
  framework integration, and config behavior based on `go build` or static
  compile evidence.
- In `20260624T004849Z`, the ledger records `go build ./internal/config/...`
  being used too broadly for behavior-sensitive acceptance.

Resolution:

- `markdown_validation_coverage_consistent` now rejects pass reports where
  only compile/static evidence validates behavior-sensitive IDs.
- Acceptance auditor now refuses to promote behavior-sensitive items from weak
  compile/static evidence.

Verification:

- Focused validation-coverage tests passed.
- Later run `20260624T043319Z` correctly rejected treating the proto prerequisite
  as behavior validation.

Status: resolved for the observed weak-coverage promotion.

### 15. Worker Source Mutation Spiral Had No Runtime Budget

Observed in:

- `20260624T025434Z`

Log/ledger evidence:

- Worker reached a structural implementation slice and repeatedly mutated
  `rpc/flipt/auth/auth.proto` and `internal/config/authentication.go`.
- Source inspection showed `authentication.go` was malformed, but the worker
  continued repair attempts instead of stopping.

Resolution:

- Runtime command evidence now records `repo_mutation`.
- Added `command_evidence_repo_mutation_limit`.
- SWE engineering worker limit set to two repository mutation commands.

Verification:

- Tests cover mutation classification and limit rejection.
- Focused checks, visualization, `git diff --check`, and broader Go tests
  passed after the patch.

Status: resolved for detecting and stopping mutation spirals, but the
underlying source-corruption tendency remains an open risk.

### 16. Mutation Classifier Produced False Positives

Observed in:

- `20260624T032119Z`

Log/ledger evidence:

- Newly added `repo_mutation` classifier flagged read-only discovery commands
  that mentioned `/app/internal/...` and redirected stderr with `2>/dev/null`.

Resolution:

- Classifier no longer treats arbitrary `>` redirection as mutation just
  because a repository path is present.
- It still detects explicit writes to `/app/...`, temp-file moves/copies into
  repo paths, in-place stream edits, patch commands, and programmatic writes.

Verification:

- Regression test covers read-only source commands with stderr redirection.
- Focused mutation/policy tests, `go test ./...`, visualization, and
  `git diff --check` passed.

Status: resolved for the observed false positive.

### 17. Worker Claimed Edits Without Runtime Mutation Evidence

Observed in:

- `20260624T033012Z`

Log/ledger evidence:

- Agent exited with `agent_status=124`; evaluation was skipped.
- After `go build ./...` failed, a repair worker claimed it removed imports,
  added gRPC embedding, and fixed OIDC API usage.
- Targeted validation showed the same compile errors remained.
- Runtime command evidence for that worker contained file reads and report
  writes, but no non-report repository mutation.

Resolution:

- Added `command_evidence_claimed_changes`.
- Implementation worker reports listing changed files now require at least one
  non-report repo mutation in runtime command evidence.

Verification:

- Tests cover false-edit reports and legitimate one-mutation reports.
- Focused orchestration tests, visualization, `go test
  ./internal/orchestration ./internal/query -count=1`, `go test ./...`, and
  `git diff --check` passed.

Status: resolved for false edit claims.

### 18. Mutation-Limit Rejection Created A Report-Rewrite Loop

Observed in:

- `20260624T043319Z`

Log/ledger evidence:

- Run was manually interrupted after mutation-limit rejection.
- The claimed-change guard worked for the proto enum slice.
- The next config worker exceeded the two-mutation budget while repeatedly
  repairing `authentication.go`.
- The mutation-limit check rejected the over-budget report, but the worker kept
  rewriting/resubmitting reports because there was no valid accepted blocker
  form.

Resolution:

- `command_evidence_repo_mutation_limit` now permits an explicit blocker report
  when evidence is over budget.
- Success-like reports are still rejected.

Verification:

- Regression tests cover rejection, allowed two-mutation edits, and explicit
  blocker escape hatch.
- Focused tests, broader local checks, visualization, and `git diff --check`
  passed.

Status: resolved for the report-rewrite loop.

### 19. Targeted Validator Ran Build After Explicit Worker Blocker

Observed in:

- `20260624T051257Z`

Log/ledger evidence:

- Run was manually interrupted after targeted validation ran a build despite an
  explicit worker blocker.
- Worker blocker had already shown mutation budget was exceeded.
- Targeted validator then ran:
  `go build ./internal/config/... ./rpc/flipt/auth/...`
- The build predictably failed against known-corrupted source and added no new
  evidence.

Resolution:

- Added `markdown_worker_blocker_validation_no_commands`.
- When a handoff worker report is an explicit blocker with no acceptance
  coverage claims, targeted validation must run no build/test/producer commands
  and record the blocker as insufficient evidence.

Verification:

- Tests cover rejecting build commands after a worker blocker and allowing a
  no-command insufficient report.
- Focused mutation/claimed-change/blocker-validation tests, `go test
  ./internal/orchestration ./internal/query -count=1`, visualization,
  `git diff --check`, and `go test ./...` passed.

Status: resolved for the observed targeted-validation-after-blocker behavior.

### 20. Validation Slice Was Rewritten Back To Discovery

Observed in:

- `20260624T053546Z`

Log/ledger evidence:

- After a correct no-command discovery result, VibeThink returned `BLOCK` and
  routed back to theory.
- The planner then proposed `worker_track: "implementation"`, `mode:
  "validation"`, and `targeted_validation: "go build ./internal/config/..."`.
- The slice-plan auditor rewrote that into `worker_track: "discovery"`,
  `mode: "discovery"`, and `targeted_validation: "none"`.
- The run repeated discovery and again produced insufficient validation.

Resolution:

- Added `json_worker_track_targeted_validation_consistent`.
- Wired it into both slice planner and slice-plan auditor outputs.
- Tightened the auditor persona so validation-only no-edit slices with concrete
  targeted validation keep `worker_track: "implementation"`.

Verification:

- Regression tests cover rejecting discovery plus `go build`, accepting
  implementation validation slices, and rejecting validation mode with no
  command.
- Focused orchestration tests, visualization, `go test
  ./internal/orchestration ./internal/query -count=1`, `git diff --check`, and
  `go test ./...` passed.

Status: resolved locally; needs a fresh evaluated rerun to prove the loop is
gone in practice.

### 21. Discovery Worker Over-Read Before Reporting

Observed in:

- `20260624T054853Z`

Log/ledger evidence:

- The discovery worker was supposed to run at most one bounded inspection
  command and then write `/tmp/pragma/swe/worker-report.md`.
- It instead ran `cat internal/config/authentication.go`, multiple `sed -n`
  chunks, enum greps, and an `auth.pb.go` inspection before reporting.
- The commands were read-only and useful, but they violated the state contract
  and contributed to another discovery/theory/audit cycle before timeout.

Resolution:

- Added `command_evidence_non_report_limit`.
- Wired it to `swe_discovery_worker` with limit `1`.
- Discovery persona now states runtime evidence rejects reports after more than
  one non-report command.

Verification:

- Regression tests cover rejecting discovery overread and allowing one
  inspection plus report write.
- Focused command-evidence tests, orchestration visualization, `go test
  ./internal/orchestration ./internal/query -count=1`, `git diff --check`, and
  `go test ./...` passed.

Status: resolved locally; needs a fresh evaluated rerun to prove discovery
states now report or block after one inspection.

## Partially Resolved Or Still Open

### A. `internal/config/authentication.go` Source Corruption

Observed in:

- `20260623T203344Z`
- `20260624T025434Z`
- `20260624T043319Z`
- `20260624T051257Z`

Evidence:

- Workers repeatedly malformed `internal/config/authentication.go` while trying
  to insert Kubernetes method fields, config structs, and registration logic.
- Later repair slices tried to recover from corrupted state and consumed time.

What is resolved:

- Unsafe commands are denied.
- Mutation spirals are detected.
- Over-budget mutation can route to blocker instead of report loop.
- Targeted validator should no longer run build after explicit blocker.

What is not resolved:

- The worker still needs a stronger behavior pattern to avoid corrupting the Go
  source in the first place.
- If source is already corrupted, repair from weak evidence should probably
  stop earlier instead of trying reconstruction.

Status: open reliability risk.

### B. Full Evaluated SWE-bench Pro Pass

Observed in:

- all full evaluated attempts so far

Evidence:

- `20260624T033012Z` ended with `agent_status=124`.
- `20260624T043319Z` and `20260624T051257Z` were manually interrupted before
  evaluation.
- No run has produced a passing SWE-bench Pro evaluation result.

Status: open; this is the actual goal-completion blocker.

### C. Timeout And Planning/Audit Overhead

Observed in:

- `20260624T004849Z`
- `20260624T015032Z`
- other long composed runs with Minimax/Lilac retries

Evidence:

- One-hour timeout fired during planning or worker implementation states before
  an evaluable patch was produced.
- Large handoffs and repeated theory/audit cycles consume substantial runtime.

What is resolved:

- Handoff caps reduce prompt size.
- Replay/start-state support reduces debugging cost.
- Some unnecessary loops are now blocked by artifact checks.

What is not resolved:

- End-to-end evaluated runtime is still too fragile.
- The loop can still spend too much time before reaching a final patch.

Status: open performance/reliability risk.

### D. Current `20260624T053546Z` Run

Observed in:

- `.pragma/swe-bench-pro/20260624T053546Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

Evidence before this document:

- The run reached `swe_acceptance_auditor`, then planned a read-only discovery
  slice.
- `swe_discovery_worker` documented auth config patterns.
- `swe_targeted_validator` correctly marked the discovery-only slice
  insufficient because no validation command was assigned.
- No evaluation result had been observed before goal execution was paused at
  user request.

Status: not classified as resolved or failed in this document. It was not
resumed for this report.

## Local Verification Summary

The resolved items above are backed by local checks recorded in the ledger. The
most recent broad verification set included:

- `go test ./internal/orchestration ./internal/query -count=1`
- `go test ./...`
- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`

Important caveat: these verify Pragma-side behavior and regressions. They do
not prove SWE-bench Pro success.

## Review Takeaway

The repeated attempts have generated real hardening, not just repeated
retries. The strongest evidence is that later failures moved forward from
earlier ones:

- Candidate-surface drift became an artifact check.
- Discovery validation ambiguity became validation coverage enforcement.
- Generated-output confusion became source-of-truth checks.
- Dirty blockers became clean-worktree checks.
- Rollback bypasses became command-policy denials.
- Mutation spirals became command-evidence limits.
- False edit claims became mutation-evidence requirements.
- Mutation-limit report loops became explicit blocker handling.
- Blocker-followed-by-build became no-command targeted validation.
- Validation-slice-to-discovery rewrites became worker-track consistency
  checks.
- Discovery overreads became non-report command evidence limits.

The system is still not done. The remaining question is whether the next
hardening step can prevent or short-circuit the `authentication.go` corruption
path quickly enough to complete an evaluated run.

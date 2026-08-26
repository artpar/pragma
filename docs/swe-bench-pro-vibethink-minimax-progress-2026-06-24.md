# SWE-bench Pro VibeThink + Minimax Progress Report

Date: 2026-06-24

## Goal

Make Pragma reliably complete SWE-bench Pro tasks using Minimax execution with VibeThink narrow gates, while preserving the general long-task worker. The current target artifact remains `docs/swe-bench-pro-vibethink-minimax-goal-prompt.md`.

This goal is not complete. No evaluated SWE-bench Pro pass has been proven yet.

## Current Live Run

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Session: `33625`
- Output directory:
  `.pragma/swe-bench-pro/20260624T051257Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Container: `45e6c26c087e`
- Status at latest sample: still running, elapsed about 18 minutes.

Latest live state:

- The run reached implementation and corrupted `internal/config/authentication.go` during a multi-location edit.
- It then planned repair slice `k8s-auth-repair-v1`.
- The worker submitted an explicit blocker-style report after exceeding mutation budget.
- The targeted validator has started `go build ./internal/config/... ./rpc/flipt/auth/...`.
- Current container git status shows:
  - `M internal/config/authentication.go`
  - `M rpc/flipt/auth/auth.proto`

This is useful signal: the mutation-budget escape hatch is now being exercised instead of trapping the worker in pure report rewrites.

## Hardening Completed

Implemented runtime and orchestration improvements:

- State/persona `llm` overrides.
- Final-text runtime capture for VibeThink gates.
- `<think>...</think>` stripping and allowed-value normalization for final-text verdicts.
- Final-text states bypass shell response gates.
- CLI/runtime/SWE runner support for `--start-at-state`, `--stop-after-state`, and seed artifacts.
- Required output freshness checks.
- Shell `require_patterns`.
- Runtime command evidence artifact.
- OpenAI-compatible env propagation into Docker, with localhost rewritten to `host.docker.internal`.
- Handoff artifact `max_bytes` caps while preserving full source snapshots.
- `gitChangedPaths()` includes untracked files.
- No-action final turns run completion checks and feed rejection back.

Implemented SWE-specific quality gates:

- VibeThink validation gate persona: `personas-research-v2/swe_validation_gate.yaml`.
- Acceptance validator rejects weak compile/static evidence for behavior-sensitive acceptance IDs.
- Targeted validator exact-command gate.
- Targeted validator producer-mutation gate.
- Targeted validator worker-no-edit and blocker gates.
- Artifact checks for:
  - concrete candidate surfaces,
  - JSON equality/subset preservation,
  - markdown validation coverage consistency,
  - no generated-output edit recommendations,
  - no forbidden worker commands,
  - scope request clean-worktree requirements,
  - source-of-truth/generated output discipline.

Implemented worker hardening:

- Worker shell policy denies rollback/history commands:
  `git checkout`, `git restore`, `git reset`, `git clean`, `git show REV:path`, `git cat-file`.
- Worker shell policy denies producer/build/lint/test commands inside worker states.
- Worker shell policy denies brittle edit commands:
  `sed -i`, `perl -pi`, GNU `patch`.
- Worker persona now forbids rollback/history restore, generated/source-of-truth confusion, and brittle in-place stream edits.
- Planner/auditor now require coherent vertical slices instead of file-narrow slices when observed source-of-truth/registration/config paths are already known.

Implemented runtime evidence hardening:

- `repo_mutation` is recorded in worker command evidence.
- `command_evidence_repo_mutation_limit` rejects mutation spirals.
- `command_evidence_claimed_changes` rejects reports that list changed files without any non-report repo mutation evidence.
- Mutation classifier false positive on read-only commands with `2>/dev/null` was fixed.
- Mutation-limit check now accepts an explicit blocker report with `Changed files: none`, allowing the run to leave a bad worker state instead of looping on report rewrites.
- `json_worker_track_targeted_validation_consistent` rejects validation slices that are rewritten to discovery/no-command routes.
- `command_evidence_non_report_limit` rejects discovery worker overreads; discovery is limited to one non-report command before reporting.

Provider hardening:

- Lilac JSON decode errors for unexpected EOF / unexpected end JSON are retryable.

## Verification Completed

Recent local verification passed:

- `go test ./internal/orchestration -run 'TestValidateCommandEvidenceClaimedChanges|TestValidateCommandEvidenceRepoMutationLimit|TestValidateMarkdownNoForbiddenWorkerCommandsRejectsRollbackFromWorker|TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands' -count=1`
- `go test ./internal/orchestration ./internal/query -count=1`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
- `git diff --check`
- `go test ./...`

## Recent Evaluation Attempts

### `20260624T033012Z`

Result: `agent_status=124`, evaluation skipped.

Progress:

- Added Kubernetes auth proto/config pieces.
- Discovered and executed `buf generate`.
- Recovered from stale generated protobuf output.
- Implemented Kubernetes default config values.
- Began framework/server implementation.

Failure:

- Worker later claimed it had fixed compile errors, but runtime command evidence showed no actual repo mutation for those claimed changed files.

Patch added:

- `command_evidence_claimed_changes`.

### `20260624T043319Z`

Result: manually interrupted, evaluation skipped.

Progress:

- Claimed-change guard worked on proto enum slice.
- Added `METHOD_KUBERNETES = 3`.
- `buf generate && go build ./internal/config/...` passed.
- Validation gate correctly refused to count proto prerequisite as behavior validation.

Failure:

- Config-struct worker exceeded mutation budget while repeatedly repairing `authentication.go`.
- The mutation-limit check rejected the report, but the worker got stuck rewriting/resubmitting reports.

Patch added:

- Mutation-limit blocker escape hatch: over-budget mutation reports can pass only when they explicitly report a blocker and `Changed files: none`.

### `20260624T051257Z`

Result: manually interrupted, evaluation skipped.

Progress:

- The run reproduced the config edit corruption path.
- It reached repair slice `k8s-auth-repair-v1`.
- Worker produced a blocker report after mutation budget trouble, proving the blocker escape hatch avoided the previous report-rewrite loop.

Failure:

- Targeted validation still ran `go build ./internal/config/... ./rpc/flipt/auth/...` after the explicit blocker report.
- The build predictably failed against known-corrupted source and consumed time without adding useful evidence.

Patch added:

- `markdown_worker_blocker_validation_no_commands` now rejects targeted-validation reports that run commands after an explicit worker blocker.
- Mutation-limit blocker guidance now tells workers to list changed files truthfully and avoid acceptance claims, instead of forcing `Changed files: none`.

### `20260624T053546Z`

Result: manually interrupted after capturing a validation-route loop, evaluation skipped.

Progress:

- The run avoided the earlier config-source corruption path initially.
- Discovery-only validation was correctly marked insufficient.
- VibeThink returned `BLOCK` and routed back to theory.

Failure:

- The planner proposed a validation slice with `targeted_validation: "go build ./internal/config/..."`, but the slice-plan auditor rewrote it into a discovery slice with `targeted_validation: "none"`.
- This caused another no-command discovery/validation cycle instead of running the intended targeted validation.

Patch added:

- `json_worker_track_targeted_validation_consistent` now rejects discovery routes with concrete targeted-validation commands and rejects validation mode without a command.
- Slice-plan auditor instructions now preserve `worker_track: "implementation"` for validation-only no-edit slices that need concrete targeted validation.

### `20260624T054853Z`

Result: `agent_status=124`, evaluation skipped.

Progress:

- The run got through discovery, targeted validation, VibeThink `BLOCK`, and post-block theory update.
- The post-block theory update preserved the source-of-truth scope request for `rpc/flipt/auth/auth.proto` and server-pattern discovery.

Failure:

- The discovery worker violated the one-inspection contract by running several read-only commands before reporting.
- The useful but overlong discovery pass contributed to another theory/audit cycle and eventual timeout.

Patch added:

- `command_evidence_non_report_limit` is now wired to discovery worker reports with limit `1`.
- Discovery persona now states the runtime will reject reports after more than one non-report command.

## Current Main Risks

- Timeout remains the biggest risk. The loop still spends several minutes in planning/auditing/theory cycles after each blocked slice.
- Multi-location edits in `internal/config/authentication.go` still tend to corrupt source when the worker uses programmatic ad hoc rewrites.
- The worker may need stronger instruction to use one coherent AST/block replacement for related Go declarations instead of repeated patch scripts.
- The current task itself may require broader runtime implementation than config/proto changes, including token validation, error handling, introspection, and deployment behavior.

## Next Actions

- Rerun with the blocker-validation command rejection active.
- Rerun with validation-track consistency checks active.
- Rerun with discovery non-report command limits active.
- If it fails on the same repair path, harden the planner/worker to avoid repair slices that reconstruct source from corrupted state and instead emit a blocker sooner.
- If it gets past config/proto, watch runtime auth implementation for guessed APIs and add source/API discovery gates before edit slices that call external libraries.
- Keep the goal active until an evaluated SWE-bench Pro success is proven.

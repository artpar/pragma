# SWE-bench Pro VibeThink + Minimax Goal Summary

Date: 2026-06-24

## Goal Being Pursued

Make Pragma reliably complete SWE-bench Pro tasks using Minimax execution plus
VibeThink-style narrow gates, while preserving Pragma's general long-task
worker behavior.

The concrete target document for the goal remains:

- `docs/swe-bench-pro-vibethink-minimax-goal-prompt.md`

The goal is not complete. No evaluated SWE-bench Pro success has been proven
yet.

## Current Status

The work is in the hardening and evaluation-loop phase.

What is done:

- The orchestration can run the SWE-bench Pro task end to end inside Docker.
- The runner now supports evaluated attempts, state start/stop controls, seed
  artifacts, provider environment propagation, and captured runtime artifacts.
- The VibeThink narrow-gate states exist and are wired into the SWE-bench Pro
  engineering loop.
- Multiple runtime and artifact-integrity checks now catch failures that were
  previously silent or late.
- Local Pragma test verification passes after the current hardening patches.

What is not done:

- There is still no passing SWE-bench Pro evaluation result.
- The current Flipt Kubernetes-authentication instance still exposes worker
  behavior that can corrupt `internal/config/authentication.go`.
- The orchestration can still spend too much time cycling through repair,
  blocker, validation, and planning states.
- Reliability has not yet been demonstrated across even one full evaluated
  instance, let alone multiple instances.

## Progress Estimate

This is the clearest split:

- Runtime/orchestration infrastructure: about 75-85% complete for this target.
- Guardrails against known Minimax failure modes: about 65-75% complete.
- SWE-bench Pro evaluated proof: 0% complete, because no eval pass exists yet.
- Overall goal: about 60-70% complete, depending on whether the next hardening
  patch gets the run past the current config-edit failure path.

The reason the overall number is not higher is that this goal is proof-driven.
Many pieces are implemented and tested locally, but the success criterion is an
evaluated SWE-bench Pro pass.

## Why It Looks Like A Loop

It is partly a loop, but it has not been the exact same loop each time. Each
full attempt has exposed a different failure mode:

1. The worker claimed it had changed files when runtime evidence showed no real
   repo mutation.
2. After that was blocked, the worker hit a mutation-limit failure and got
   stuck rewriting reports.
3. After that was fixed, targeted validation still ran build commands after an
   explicit worker blocker.
4. The latest fresh run is checking whether the no-command-after-blocker patch
   breaks that cycle.

So the visible pattern is repeated SWE-bench attempts, but the underlying
failures have been moving. The concern is valid: if the next run again returns
to source corruption and repair cycling, the next patch should stop trying to
repair corrupted source from weak evidence and force a clean blocker earlier.

## Completed Hardening

Runtime and orchestration:

- Added state/persona `llm` overrides.
- Added final-text runtime capture for narrow-gate states.
- Added `<think>...</think>` stripping and final-value normalization.
- Allowed final-text states to bypass shell-response gates.
- Added `--start-at-state` and `--stop-after-state` support.
- Added seed-artifact support.
- Added required-output freshness checks.
- Added shell `require_patterns`.
- Preserved full handoff snapshots while allowing `max_bytes` caps.
- Included untracked files in changed-path detection.
- Made no-action final turns run completion checks and feed rejections back.
- Propagated OpenAI-compatible environment variables into Docker.
- Rewrote localhost OpenAI base URLs to `host.docker.internal` for containers.

SWE-bench Pro orchestration and personas:

- Added `personas-research-v2/swe_validation_gate.yaml`.
- Hardened repo survey, acceptance mapping, planner, theory, worker, auditor,
  reviewer, and targeted validator contracts.
- Required concrete observed paths instead of guessed candidate surfaces.
- Required coherent vertical-slice plans when observed source-of-truth paths are
  already known.
- Tightened generated/source-of-truth discipline.

Worker command policy:

- Denied rollback/history restore commands such as `git checkout`,
  `git restore`, `git reset`, `git clean`, `git show REV:path`, and
  `git cat-file`.
- Denied build/test/lint/generator commands from worker states.
- Denied brittle edit commands such as `sed -i`, `perl -pi`, and GNU `patch`.
- Added guidance against ad hoc in-place stream edits and generated-output
  confusion.

Evidence and validation gates:

- Recorded `repo_mutation` in command evidence.
- Added `command_evidence_claimed_changes`.
- Added `command_evidence_repo_mutation_limit`.
- Fixed mutation-classifier false positives for read-only commands that mention
  repo paths.
- Added a blocker escape hatch for mutation-limit failures.
- Added targeted-validator exact-command and producer-mutation checks.
- Added targeted-validator worker-no-edit and worker-blocker checks.
- Added `markdown_worker_blocker_validation_no_commands`.
- Made behavior-sensitive acceptance items reject weak compile/static evidence.

Provider and runner:

- Made Lilac unexpected-EOF JSON decode failures retryable.
- Added and verified the SWE-bench Pro runner path with evaluated attempts.

## Verification Completed

Recent local verification passed:

- `go test ./internal/orchestration ./internal/query -count=1`
- `go test ./...`
- `git diff --check`
- `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`

These tests prove the Pragma-side hardening builds and passes local tests. They
do not prove the SWE-bench Pro goal is complete.

## Evaluated Attempts So Far

### `20260624T033012Z`

Result: timed out, eval skipped.

Useful progress:

- Added Kubernetes auth proto/config/default pieces.
- Ran generation.
- Began server/framework implementation.

Failure exposed:

- Worker claimed fixes without actual repo mutation evidence.

Patch added:

- `command_evidence_claimed_changes`.

### `20260624T043319Z`

Result: manually interrupted, eval skipped.

Useful progress:

- Claimed-change guard worked.
- Proto enum slice passed generation/config build.
- Validation gate correctly rejected behavior overclaim.

Failure exposed:

- Mutation-limit rejection trapped the worker in report rewrites.

Patch added:

- Mutation-limit explicit blocker escape hatch.

### `20260624T051257Z`

Result: manually interrupted, eval skipped.

Useful progress:

- Blocker escape hatch let the run leave the worker/report loop.

Failure exposed:

- Targeted validator still ran build commands after an explicit worker blocker.

Patch added:

- `markdown_worker_blocker_validation_no_commands`.

### `20260624T053546Z`

Result: currently running at the time of this report.

Current state:

- Session: `2580`
- Output directory:
  `.pragma/swe-bench-pro/20260624T053546Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Latest observed orchestration state: `swe_acceptance_auditor`
- No evaluation result yet.

## Main Remaining Work

1. Let the current evaluated attempt reach a terminal result or fail with a new
   concrete defect.
2. If it repeats the config-source corruption path, harden planner/worker rules
   so they do not attempt broad repair from corrupted state without a stable
   source-of-truth block.
3. Require explicit blocker behavior earlier when source is corrupted and the
   worker lacks enough observed evidence to perform one coherent replacement.
4. Continue evaluated attempts until at least one SWE-bench Pro instance passes.
5. After one pass, run additional instances or at minimum one rerun to confirm
   the fix is not overfit to this single Flipt task.

## Bottom Line

The Pragma-side machinery is substantially built, and many previously invisible
failure modes are now caught by runtime checks. The goal is still not achieved
because the system has not produced a passing SWE-bench Pro evaluation.

The remaining problem is not basic wiring. It is reliability under Minimax:
getting the worker to make bounded, evidence-grounded edits without corrupting
source or burning the run in repair loops.

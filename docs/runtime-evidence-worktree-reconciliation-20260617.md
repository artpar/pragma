# Runtime Evidence Worktree Reconciliation, 2026-06-17

## Purpose

This note reconciles the current dirty worktree around the generic
runtime-evidence ownership slice. It does not claim the full SWE-bench trajectory
is solved. It names which dirty files belong to the generic runtime-evidence
baseline, which files belong to higher-level orchestration/persona experiments,
and what proof exists today.

## Current Worktree Status

The worktree is intentionally not treated as clean. It contains several
independent change groups:

- generic runtime-evidence/runtime plumbing;
- SWE-bench orchestration and persona experiments;
- moved legacy checklist-loop examples;
- prompt-control and retrospective docs;
- provider/raw-capture robustness changes.

No broad revert was performed because many dirty files predate this
reconciliation pass and may be user-owned or part of an interrupted experiment.

## Generic Runtime-Evidence Slice

These files are the core runtime-evidence ownership baseline:

- `internal/orchestration/orchestration.go`
  - Adds schema support and validation for `runtime_capture: command_evidence`.
- `internal/orchestration/runner.go`
  - Separates model-authored outputs from runtime-authored artifacts in the
    model-visible artifact contract.
  - Separates model-authored required outputs from runtime-authored required
    artifacts in the completion contract.
  - Splits completion rejection guidance into model-authored and
    runtime-authored failures.
  - Converts runtime-authored output artifact paths into protected write paths
    for the shell loop.
- `internal/query/miniswe_loop.go`
  - Records runtime-authored command evidence as JSONL with command and output
    hashes, return code, timeout status, completion sentinel, and report-write
    markers.
  - Rejects shell commands that attempt to modify protected runtime-authored
    artifact paths before execution.
- `internal/query/event.go`, `internal/app/state.go`,
  `internal/orchestration/projection.go`, `internal/observe/event_catalog.go`,
  `internal/cli/run.go`
  - Carry artifact byte counts and SHA-256 hashes through orchestration events,
    projections, and app state.
- `cmd/pragma/orchestration.go`, `internal/slash/command.go`
  - Plumb seeded artifacts through standalone or slash-driven orchestration
    requests.

## Runtime-Evidence Declarations

The current orchestration declarations using the generic runtime-capture
contract are:

- `orchestrations/swe-single-owner-engineering-loop.yaml`
  - `engineer_command_evidence` is `runtime_capture: command_evidence`.
- `orchestrations/swe-bench-pro-engineering-loop.yaml`
  - `worker_command_evidence` is `runtime_capture: command_evidence` for both
    implementation and discovery worker states.

These YAML declarations are not the ownership mechanism by themselves. They
only declare the runtime-authored artifact. The runtime code above is the
enforcement mechanism.

## Not The Generic Runtime-Evidence Slice

These dirty areas should be reviewed or committed separately:

- `personas-research-v2/*.yaml`
  - Many prompt changes are SWE/persona experiment work. Some are generic
    wording cleanup, but many are higher-level orchestration policy and should
    not be bundled as proof of the runtime-evidence boundary.
- `orchestrations/task-evidence-item-loop.yaml`
  - New experimental orchestration surface.
- `examples/` plus deletions under `personas/` and
  `orchestrations/architect-checklist-item-loop-final.yaml`
  - Legacy checklist-loop assets appear to have moved out of production paths
    into examples.
- `docs/swe-bench-pro-*.md`, `pragma-goal.md`,
  `docs/pragma-codex-session-retrospective-20260617.md`
  - Investigation, goal, and retrospective artifacts.
- `internal/provider/lilac/provider.go`,
  `internal/provider/rawcapture/rawcapture.go`
  - Provider timeout and capture-error metadata robustness, adjacent to
    replay/debugging but not part of the runtime-authored artifact boundary.

## Verification Performed

Persistent verification:

```bash
go test ./internal/query ./internal/orchestration
go test ./cmd/pragma
go build ./cmd/pragma
git diff --check
```

All commands passed.

Temporary behavioral verification was also performed with package-local tests
that were removed after running. They proved:

- a declared runtime-authored artifact becomes a protected write path;
- the generated artifact and completion contracts separate model-authored and
  runtime-authored artifacts;
- shell commands using `>`, `>>`, `rm`, or `sed -i` against the protected
  runtime-authored evidence path are rejected before execution;
- a read-only `cat` of the same runtime-authored evidence path is not rejected.

No temporary test files remain in the worktree.

## Completion Boundary

For the generic runtime-evidence slice, the runtime now has three layers:

1. model-visible contract separation;
2. owner-aware completion rejection;
3. source-enforced protected write paths for runtime-authored artifacts.

This satisfies the generic ownership boundary exposed by the single-owner
evidence loop. It does not prove a full SWE-bench task will pass. The next
benchmark-level proof is to replay or rerun from a named baseline and preserve
the raw payloads, phase map, and evaluator result.

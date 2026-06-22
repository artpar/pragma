You are working in `/Users/artpar/workspace/code/pragma` with one objective:

Build, test, and iterate the SWE-bench Pro orchestration FSM and persona prompt
set until Pragma can reliably complete SWE-bench Pro tasks, starting with the
Flipt Kubernetes authentication failure as the baseline instance.

This is an implementation goal, not another design-only investigation.

The previous investigation produced:

- Goal prompt:
  `docs/swe-bench-pro-orchestration-ab-goal-prompt.md`
- Evidence ledger:
  `docs/swe-bench-pro-orchestration-ab-ledger.md`
- Prompt AB replay cases:
  `.pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep`

Use that evidence as the starting point. Do not redo the RCA from scratch unless
new evidence contradicts it. The key conclusion is that prompt-only reminders
failed; the next design must use stronger persona/FSM structure, handoff
discipline, and validation/review behavior. The exact mechanism is open.

## Baseline target

Primary baseline failed run:

- Run dir:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Failed instance:
  `flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Known failed evaluator package:
  `go.flipt.io/flipt/internal/config`
- Known failed evaluator subtests:
  - `TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)`
  - `TestLoad/authentication_kubernetes_defaults_when_enabled_(ENV)`
  - `TestLoad/advanced_(YAML)`
  - `TestLoad/advanced_(ENV)`

This instance is the first acceptance gate for the new FSM/persona set. The
goal is not complete until a fresh SWE-bench Pro run using the new orchestration
and persona set passes the evaluator for this instance, or until a real external
blocker is proven with logs and a precise next action.

## Meaning of "perfect" for this goal

"Perfect" here means operationally disciplined and benchmark-effective, not
philosophically flawless.

The FSM/persona set must:

1. Solve long, complex SWE-bench Pro tasks through a persona-based FSM.
2. Preserve the real task objective across long multi-step execution.
3. Keep implementation, validation, review, and final approval aligned with the
   user's task and evaluator-facing behavior.
4. Prevent drift into attractive but unvalidated side paths.
5. Prevent self-authored tests or local confidence from replacing real task
   acceptance.
6. Recover cleanly when scope, generated files, producer commands, validation
   evidence, or assumptions expose a gap.
7. Complete the Flipt Kubernetes baseline instance under the real runner.
8. Generalize to other long complex SWE-bench Pro tasks, not overfit only to
   Flipt.

## Mandatory living document

Maintain exactly one implementation ledger for this goal:

- `docs/swe-bench-pro-orchestration-fsm-persona-ledger.md`

Create it before editing production orchestration/persona files. Keep it
current as work progresses.

The ledger must contain:

1. Objective and current status
2. Baseline evidence imported from the prior AB ledger
3. Current FSM/persona architecture before edits
4. Proposed FSM/persona changes
5. File-by-file implementation log
6. AB replay cases and verdicts for changed prompts
7. Local validation commands and results
8. Fresh SWE-bench Pro baseline runs and evaluator results
9. Regressions, rejected variants, and why
10. Generalization notes for non-Flipt tasks
11. Remaining risk and next iteration plan
12. Completion audit against this goal prompt

## Implementation scope

The implementation may patch the existing SWE-bench Pro FSM/personas, or create
a new orchestration and a new corresponding persona set if the current files are
not the right foundation.

Do not treat the existing files as the required destination. Treat them as
examples, references, and possible reusable parts. If a clean new FSM/persona
set is the clearer path to the objective, create it.

Current reference files include:

- `orchestrations/swe-bench-pro-engineering-loop.yaml`
- `personas-research-v2/swe_repo_survey.yaml`
- `personas-research-v2/swe_theory_keeper.yaml`
- `personas-research-v2/swe_slice_planner.yaml`
- `personas-research-v2/swe_engineering_worker.yaml`
- `personas-research-v2/swe_targeted_validator.yaml`
- `personas-research-v2/swe_engineering_reviewer.yaml`
- `personas-research-v2/swe_final_validator.yaml`
- `personas-research-v2/swe_final_reviewer.yaml`
- `personas-research-v2/swe_scope_expander.yaml`
- `personas-research-v2/swe_diagnosis_router.yaml`

Acceptable implementation outcomes include:

- revise the existing `swe-bench-pro-engineering-loop.yaml` and existing
  `swe_*` personas if that is sufficient
- add a new orchestration file, for example a next-generation SWE-bench Pro FSM
- add a new persona set under `personas-research-v2/` or another appropriate
  repo-local persona directory
- retire or bypass existing SWE personas from the new FSM when evidence shows
  they are the wrong abstraction
- add runtime/handoff code only if current orchestration support cannot pass the
  required artifacts between states

Out of scope unless the evidence proves it is necessary:

- broad CLI rewrites
- provider rewrites
- unrelated personas
- benchmark harness changes unrelated to orchestration/persona execution
- pushing to origin

## Hard constraints

- Do not stop at a proposed design. Implement, replay-test, run locally, and
  iterate.
- Do not rely on prompt vibes. Every production prompt/FSM change must be tied
  to failed-run evidence or AB replay evidence.
- Do not make a final approval path depend only on model self-confidence.
- Do not mark the goal complete from a passing prompt replay alone. The Flipt
  baseline needs a fresh runner/evaluator result.
- Do not add Pragma tests unless the user explicitly asks for tests. Prefer
  focused CLI validation, raw HTTP replay, YAML/schema inspection, compile
  checks, and real benchmark runs.
- Do not push to origin.
- Do not leak API keys, provider credentials, raw request headers, or secrets
  into docs or prompt-ab fixtures.
- Use the current checkout and current CLI help as authoritative.
- Preserve unrelated worktree changes.

## Candidate mechanisms from prior evidence

The previous AB work suggests several useful mechanisms. They are not mandatory
requirements. Use them, change them, or replace them if a better persona-based
FSM design emerges.

The only real direction is: Pragma must become a strong persona-based FSM for
long complex tasks.

Candidate mechanism: preserve task acceptance explicitly before solution theory
hardens. One possible artifact shape was:

```json
{
  "acceptance_items": [
    {
      "id": "ACCEPT-...",
      "task_text": "...",
      "behavior_surface": "...",
      "repo_surfaces_to_verify": ["..."],
      "required_validation": ["..."],
      "status": "pending|addressed|validated|insufficient|not_applicable",
      "validation_evidence": [],
      "blocking_if_missing": true,
      "notes": "..."
    }
  ]
}
```

Candidate rules, if this mechanism is used:

- Stable task-derived IDs were useful in the Flipt AB baseline.
- The mapper must not invent alternate prefixes when stable IDs are already in
  use for a known task family.
- For Flipt Kubernetes auth, YAML, ENV, defaults, and custom bindings must be
  separate blocking items.
- Runtime-token validation must not subsume config-loading acceptance.

Candidate mechanism: make planning name what behavior it is addressing and what
validation would prove it. One possible extension to a slice plan was:

```json
{
  "acceptance_ids": ["ACCEPT-..."],
  "validation_covers_acceptance_ids": ["ACCEPT-..."],
  "missing_acceptance_ids": ["ACCEPT-..."]
}
```

Candidate rules, if this mechanism is used:

- In the Flipt AB baseline, schema-level structure worked better than prose
  guidance.
- Planner must read acceptance-map status as authoritative over lossy
  engineering context.
- If blocking acceptance items are pending, the next slice must address or
  validate them before lower-priority implementation work.
- If a slice touches config/schema/proto/generated/runtime surfaces, the plan
  must name the relevant acceptance IDs and the validation that can prove them.

Candidate mechanism: make worker reports preserve enough evidence for later
personas to review the work without trusting a completion claim. Possible
fields:

- `acceptance_ids_addressed`
- `validation_commands_run`
- `validation_required_before_claiming_complete`
- `approved_paths_touched`
- `forbidden_scope_touched`
- `task_completion_claim_allowed`

Candidate rules, if this mechanism is used:

- Worker may report edits and evidence, but cannot claim task completion.
- Build/vet/grep may be supporting evidence, but cannot validate behavior that
  requires config loading, runtime execution, generated producer checks, or
  evaluator-facing fixtures.
- If validation has not proved an acceptance ID, the worker must name the
  required validation and set completion claim to false.

Candidate mechanism: targeted validation should validate behavior and task
coverage, not merely repeat worker claims. Possible behavior:

- Validate acceptance IDs, not worker claims.
- Use exact commands from slice plan when present.
- If worker evidence already exists, write the validation artifact first rather
  than starting unrelated inspection.
- Missing acceptance coverage is `Result: insufficient`, not a passing result
  with a next suggestion.

Candidate mechanism: reviewer and final gates should be independent enough to
catch drift.

Reviewer:

- Must reject final validation readiness if any blocking acceptance item is
  pending, missing, weakened, or unsupported by validation evidence.
- Must route to theory/planning/scope expansion based on the missing acceptance
  surface.

Final validator:

- Must compute validation from the full final diff and acceptance map, not only
  the latest slice.
- Must run commands covering incomplete/high-risk acceptance IDs.
- Must write missing validation explicitly when coverage is incomplete.

Final reviewer:

- Must approve only if every blocking original acceptance item is present and
  validated or explicitly proven not applicable.
- Must block if final validation omits acceptance coverage even when the
  commands that did run passed.

## Implementation workflow

1. Read the prior ledger sections 8-13 and import the survivor/rejected design
   into the new implementation ledger.
2. Inspect current orchestration/persona files and identify the minimum set of
   production edits needed to express the new FSM and artifact contracts.
3. Implement the smallest coherent FSM/persona change set.
4. Run local static validation appropriate to YAML/persona files.
5. Rebuild or replay prompt AB cases for each changed state:
   - acceptance mapper
   - theory keeper handoff
   - slice planner initial config slice
   - slice planner runtime-ordering case
   - worker report handoff
   - targeted validator coverage
   - engineering reviewer final gate
   - final validator coverage gate
   - final reviewer block
6. If a replay fails, revise the production prompt/FSM and rerun the failed
   replay. Record both the failed and surviving variants.
7. Run a fresh SWE-bench Pro baseline for the Flipt Kubernetes instance using
   the edited orchestration/persona set.
8. Inspect raw HTTP and evaluator output from the fresh run.
9. If the fresh run fails, treat it as the next iteration:
   - identify whether the failure is orchestration, persona, runtime/handoff,
     model, provider, or task implementation drift
   - patch the FSM/persona set or runtime handoff if needed
   - replay targeted payloads before running the full benchmark again
10. Once Flipt passes, run at least one additional SWE-bench Pro task or a
    documented smoke subset if available, to check for overfitting.

## Baseline command discovery

Before running the benchmark, verify current command shapes with `--help`.
Record the exact commands in the ledger.

Likely command families:

```bash
go run ./cmd/pragma inspect raw-http --help
go run ./cmd/pragma replay raw-http --help
go run ./cmd/pragma replay raw-http audit --help
go run ./cmd/pragma replay raw-http dump --help
go run ./cmd/pragma run --help
```

Use the current CLI output over this prompt if anything differs.

## Baseline pass criteria

The Flipt baseline is passing only when all are true:

- Fresh run uses the edited SWE-bench Pro orchestration/persona set.
- Fresh run reaches final approval through the new persona-based FSM.
- Fresh evaluator result for
  `flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446` is true.
- Raw HTTP inspection shows the FSM preserved the task objective across long
  execution and did not drift away from evaluator-facing behavior.
- Planning, work, validation, review, and final approval remain aligned with
  the task's real behavioral requirements.
- Config-loading behavior is validated before runtime-token work is treated as
  sufficient for the Flipt task.
- Final validation includes `go test ./internal/config/...` or a stronger
  evaluator-facing config validation for this task.

## Completion criteria

Do not mark this goal complete until:

1. Production orchestration/persona files have been updated.
2. The implementation ledger explains every production edit.
3. AB replay evidence passes for the changed states.
4. Fresh Flipt SWE-bench Pro run passes evaluator, or a real external blocker
   is proven with exact logs and a minimal next action.
5. At least one generalization check beyond the original failed turn replay has
   been performed or explicitly blocked.
6. The ledger's completion audit is fully marked complete.

If you cannot reach completion in one continuous session, leave the ledger in a
state that the next agent can resume without guessing:

- current run directories
- exact commands used
- last passing and failing AB cases
- production files changed
- fresh benchmark result
- next concrete action

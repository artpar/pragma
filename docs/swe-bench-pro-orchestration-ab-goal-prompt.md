> Inactive historical task specification. Do not execute as a standing goal.
> Follow [agent.md](../agent.md); this prompt does not authorize new experiments.

You are working in `/Users/artpar/workspace/code/pragma` with one objective:

Design and prove a SWE-bench Pro orchestration/persona flow that prevents the
Flipt Kubernetes authentication task from drifting away from evaluator-facing
acceptance requirements, using turn-by-turn raw HTTP replay and direct API AB
testing from the original task prompt through final approval.

This is not an implementation rush. Your job is to investigate, AB test,
document, and only then propose precise flow/persona/context changes.
Do not edit orchestration, persona, or runtime files unless the user explicitly
asks for implementation after this investigation.

Primary failed run:

- Run dir:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Raw HTTP dir:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma`
- Evaluation result:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/eval_results.json`
- Evaluator stdout copies:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma_stdout.log`
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/workspace/stdout.log`
- Runner command:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/runner-command.txt`
- Run metadata:
  `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/metadata.json`

Known failed outcome:

- `eval_results.json` says the instance is false.
- Overall accuracy was 0.0.
- The evaluator failed in `go.flipt.io/flipt/internal/config`.
- Failed tests included:
  - `TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)`
  - `TestLoad/authentication_kubernetes_defaults_when_enabled_(ENV)`
  - `TestLoad/advanced_(YAML)`
  - `TestLoad/advanced_(ENV)`
- The patch missed config YAML/env/default loading for Kubernetes auth.
- The orchestration drifted into runtime token validation and self-authored
  runtime tests instead of closing config acceptance requirements.
- The failed run used provider `lilac`, model `minimaxai/minimax-m2.7`,
  run mode `orchestration`, orchestration
  `/pragma/orchestrations/swe-bench-pro-engineering-loop.yaml`, persona dir
  `/pragma/personas-research-v2`, and max turns `350`.
- `pragma inspect raw-http` currently reports 160 captured LLM turns and 3
  errors for this run. Captured turn numbering starts at `000001`; treat
  "turn 0" as the original task prompt and run setup, not a raw HTTP directory
  that must exist.

Mandatory single living document:

Maintain exactly one comprehensive markdown ledger and update it incrementally
as the goal progresses:

- `docs/swe-bench-pro-orchestration-ab-ledger.md`

This ledger is the durable working memory for the goal. Do not scatter findings
across ad hoc docs. Every material insight, payload observation, AB-test setup,
AB-test result, prompt change candidate, rejected hypothesis, and final
recommendation must be recorded there before moving on.
If the ledger already exists, continue it in place instead of replacing useful
evidence. If it does not exist, create it before doing analysis beyond basic
artifact verification.

Ledger sections to maintain:

1. Objective and current hypothesis
2. Source artifacts and run directories
3. Baseline failed trajectory, turn by turn
4. Acceptance requirements inferred from the original task prompt
5. Where each acceptance requirement first appeared, disappeared, or was
   rewritten
6. Persona/state-by-state RCA
7. Context/handoff payload RCA
8. AB-test matrix
9. AB-test transcripts and verdicts
10. Flow/state/persona changes that survived testing
11. Rejected fixes and why
12. Proposed final orchestration/persona design
13. Confidence assessment and remaining risk
14. Exact commands run
15. Completion audit against this prompt

Governing local docs and files:

- Read `docs/personas-from-prompt-control-research.md` for the established
  persona prompt style.
- Read `docs/prompt-control-ab-tests.md` for prior AB-test methodology and
  failure patterns.
- Read `docs/swe-bench-pro-pragma-runbook.md` for raw HTTP and SWE-bench Pro
  run commands.
- Read `docs/swe-bench-pro.md` for runner behavior.
- Read current orchestration/persona files:
  - `orchestrations/swe-bench-pro-engineering-loop.yaml`
  - `orchestrations/prompt-control-v2-benchmark.yaml`
  - `personas-research-v2/swe_repo_survey.yaml`
  - `personas-research-v2/swe_theory_keeper.yaml`
  - `personas-research-v2/swe_slice_planner.yaml`
  - `personas-research-v2/swe_engineering_worker.yaml`
  - `personas-research-v2/swe_targeted_validator.yaml`
  - `personas-research-v2/swe_engineering_reviewer.yaml`
  - `personas-research-v2/swe_final_validator.yaml`
  - `personas-research-v2/swe_final_reviewer.yaml`
  - relevant older personas under `personas-research-v2/` and `personas/`

Hard constraints:

- Do not treat this as prompt vibes or hit-and-try.
- Do not assume the fix is "add acceptance_map" until AB tests prove the exact
  artifact shape, placement, and handoff content.
- Do not rerun the full SWE-bench task repeatedly as the main experiment.
  First use raw payload replay and direct API AB tests to isolate the prompt,
  state, and context failures.
- Do not add tests to the Pragma repo unless explicitly asked by the user.
- Do not push to origin.
- Do not leak API keys or repeat secrets from process output.
- Keep edits scoped. If implementation is later requested, edit only the
  orchestration/persona/runtime files needed by the proven design.
- Treat raw HTTP request headers, provider credentials, environment variables,
  and process output as sensitive until inspected. Redact secrets before saving
  any copied payloads or summaries under `.pragma/prompt-ab/` or the ledger.
- Use the current checkout and current CLI help as authoritative. If this
  prompt's command examples drift from the CLI, record the drift in the ledger
  and use the current CLI.

Required investigation workflow:

0. Verify the source facts.
   - Confirm the run directory, raw HTTP capture directory, `metadata.json`,
     `runner-command.txt`, `agent-status.txt`, evaluator result, and evaluator
     stdout files exist.
   - Record the provider, model, orchestration, persona dir, max-turn setting,
     raw turn count, raw HTTP error count, and evaluator failures in the
     ledger before drawing conclusions.
   - Read the original task prompt from `prompt.txt` and/or `metadata.json`.
   - Verify the current command shapes with `--help` before using replay,
     dump, inspect, or audit commands.

1. Reconstruct the baseline trajectory.
   - Export or inspect all raw HTTP payloads from the failed run.
   - Build a turn-by-turn table from the original task prompt through final
     approval. Use captured raw HTTP turn IDs for LLM turns and a separate
     "task prompt" row for pre-capture context.
   - For each LLM turn, record:
     - state/persona
     - request context artifacts included
     - user/tool results included
     - assistant action
     - artifact written
     - acceptance requirements preserved, lost, or rewritten
     - whether the turn moved toward evaluator success or away from it

2. Build the original acceptance map manually from the task prompt.
   It must include, at minimum:
   - `ACCEPT-K8S-CONFIG-STRUCT`: Kubernetes config struct exists with
     `IssuerURL`, `CAPath`, `ServiceAccountTokenPath`
   - `ACCEPT-K8S-YAML`: Kubernetes method can be enabled through YAML config
   - `ACCEPT-K8S-ENV`: Kubernetes method can be enabled through ENV config
   - `ACCEPT-K8S-DEFAULTS`: Kubernetes enabled without explicit config uses
     standard in-cluster defaults
   - `ACCEPT-K8S-CUSTOM-BINDINGS`: custom issuer URL, CA path, and service
     account token path bind correctly
   - `ACCEPT-K8S-CLEANUP-SESSION`: cleanup/session policy integration is either
     implemented or explicitly proven not applicable
   - `ACCEPT-K8S-SCHEMA`: schema accepts the Kubernetes config block
   - `ACCEPT-K8S-INTROSPECTION`: `AllMethods()` and introspection expose the
     method
   - `ACCEPT-K8S-PROTO`: proto enum/generated code remain consistent
   - `ACCEPT-K8S-RUNTIME-TOKEN`: runtime token validation is implemented only
     after config acceptance is not missing
   Use stable acceptance IDs like these throughout the ledger, AB payloads,
   state artifacts, verdicts, and final plan. Rename or split an ID only if the
   task prompt evidence proves a better boundary, and record the reason.

3. Trace acceptance loss.
   For every acceptance item, identify:
   - first turn where it was visible
   - first turn where it was omitted from an artifact that should preserve it
   - first turn where it was rewritten into a weaker or different objective
   - which state/persona caused the loss
   - which downstream state should have caught it but did not

4. Compare old and new orchestration mechanics.
   Determine exactly which mechanisms from `prompt-control-v2-benchmark` and
   older personas should be ported, such as:
   - acceptance quality rule
   - input matrix preservation rule
   - contract-edge preservation rule
   - generated producer/source-of-truth rules
   - repair authority discipline
   - completed/preserved artifact discipline
   Determine exactly which old mechanisms should not be ported, such as:
   - rigid static checklist as the main SWE-bench Pro driver
   - file-local checklist decomposition that hides behavior
   - administrative repair taxonomy

5. Design AB-test candidates.
   Test candidates at the payload level before editing production prompts.
   Include at least:
   - New `acceptance_mapper` state before theory keeper
   - `acceptance_map` as persistent handoff into theory, planner, worker,
     targeted validator, reviewer, final validator, final reviewer, and any
     diagnosis router
   - `slice_plan.acceptance_ids`
   - worker handoff fields that bind approved paths, acceptance IDs, forbidden
     scope, and validation commands
   - targeted validator required to validate `acceptance_ids`, not just worker
     report claims
   - reviewer required to reject unvalidated acceptance IDs and scope laundering
   - final validator required to run commands covering all incomplete or
     high-risk acceptance IDs
   - final reviewer required to block if any acceptance item is unvalidated,
     weakened, or missing from final validation
   - constraints preventing automatic "write more tests" slices unless an
     acceptance item or failing evidence requires tests

6. AB test from the original task prompt through final approval in slices.
   You do not need to execute a whole live task for every candidate. Use the
   existing raw payloads and replay tooling to simulate the exact state inputs.
   For each tested flow:
   - Start at the original task prompt.
   - Test the first state response.
   - Feed the resulting artifact into the next state payload.
   - Continue through planner, worker handoff, validator, reviewer, final
     validation, and final review using controlled artifact snapshots.
   - Where a prior bad artifact exists, test whether the revised downstream
     state blocks it.
   - Record exact request variants and response summaries in the ledger.

7. Required AB-test checkpoints for this failed task:
   - The initial acceptance mapper must produce config YAML/ENV/defaults as
     separate visible acceptance items.
   - Theory keeper must not collapse config acceptance into broad runtime token
     validation.
   - Slice planner must prioritize config-loading/default acceptance before
     runtime token validation if those items are unvalidated.
   - Worker handoff for config slice must point to config loader/testdata/schema
     surfaces, not auth server implementation.
   - Targeted validator must select `go test ./internal/config/...` when
     validating config acceptance.
   - Reviewer must reject a worker report that adds `authenticator.go` outside
     approved paths without scope expansion.
   - Reviewer must reject "tests pass" if the tests are self-authored around a
     drifted implementation and do not cover acceptance IDs.
   - Final validator must include evaluator-relevant config tests before final
     approval.
   - Final reviewer must block if YAML/ENV/defaults acceptance is unvalidated.

8. Use replay/export commands from the repo.
   Useful commands include:
   - `go run ./cmd/pragma inspect raw-http "$RUN_DIR"`
   - `go run ./cmd/pragma inspect raw-http "$RUN_DIR" --errors`
   - `go run ./cmd/pragma inspect raw-http "$RUN_DIR" --turn 000090`
   - `go run ./cmd/pragma inspect raw-http "$RUN_DIR" --format markdown`
   - `go run ./cmd/pragma replay raw-http dump "$RUN_DIR/raw-http-pragma" --out <out-dir> --overwrite`
   - direct raw file inspection under
     `$RUN_DIR/raw-http-pragma/<turn-id>/request.json`,
     `response.raw`, `request.meta.json`, and `response.meta.json`
   Verify exact command shapes against the current CLI before relying on them.

9. Direct API AB testing.
   Use the same provider/model setup as the failed run unless a deliberate
   comparison is being made:
   - provider: `lilac`
   - model: `minimaxai/minimax-m2.7`
   - temperature: `0`
   Keep every AB-test payload and response under `.pragma/prompt-ab/` with a
   timestamped directory. Redact secrets. Record the directory path and verdict
   in the ledger.
   If the required API key is not available, do not fake direct API evidence.
   Use offline payload inspection and local replay preparation for progress,
   record the missing credential as a blocker for the direct API portion, and
   keep the goal incomplete.

10. Decision standard.
    A candidate flow/persona change is not accepted merely because it sounds
    better. It must pass the Flipt Kubernetes checkpoint tests above using
    payload-level AB evidence. Prefer deterministic artifact structure over
    phrasing-only fixes.

Expected final deliverables:

- `docs/swe-bench-pro-orchestration-ab-ledger.md`, fully updated with the
  investigation, AB tests, conclusions, and proposed final design.
- A concise proposed implementation plan inside the ledger naming:
  - new states, if any
  - existing states to modify
  - exact artifacts and fields to add
  - exact handoff paths to update
  - exact persona rules to port from the old flow
  - exact persona rules to delete or avoid
  - validation commands for the next full SWE-bench run
- If implementation is explicitly requested after this goal, apply only the
  proven changes.

Completion criteria:

- The ledger reconstructs the failed run from the original task prompt to final
  approval with enough detail to show where acceptance drift began and why it
  was not caught.
  The reconstruction must account for all captured LLM turns, either with
  individual rows or with justified ranges plus detailed rows at every state
  transition, acceptance loss, validation claim, review verdict, and final
  approval.
- The ledger contains AB evidence for the proposed flow/persona changes across
  the full state chain, not just isolated prompt snippets.
- The proposed design would have blocked this failed run before final approval
  for the concrete reason that config YAML/ENV/defaults acceptance was
  unvalidated.
- The proposed design would have forced evaluator-relevant config validation
  before final approval.
- Every proposed change maps to at least one failed-run observation and one
  passing AB-test verdict. Any untested idea is listed as future work, not part
  of the proposed final design.
- The ledger's completion-audit section checks every requirement in this prompt
  and marks it complete with evidence, incomplete, or blocked.
- The final answer states whether confidence is high, medium, or low and why.

Failure condition:

If the work ends with "add acceptance_map" as an untested idea, or with a
phrasing-only prompt patch that was not AB tested through the state chain, the
goal is not complete.

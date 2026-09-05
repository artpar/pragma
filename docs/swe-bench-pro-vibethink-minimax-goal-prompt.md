> Inactive historical task specification. Model defaults and experiments below
> are not current instructions. Follow [agent.md](../agent.md).

Goal: Make Pragma reliably complete SWE-bench Pro tasks using a Minimax + VibeThink multi-LLM orchestration, while preserving Pragma as a general-purpose long-task worker.

Context:
- Repo: /Users/artpar/workspace/code/pragma
- SWE-bench Pro harness: /Users/artpar/workspace/code/SWE-bench_Pro-os
- Main runner: tools/run_swebench_pro_instance.py
- Current orchestration: orchestrations/swe-bench-pro-engineering-loop.yaml
- Current persona dir: personas-research-v2
- Default execution LLM should be Lilac/Minimax:
  provider=lilac
  model=minimaxai/minimax-m2.7
- VibeThink is available through an OpenAI-compatible local server:
  provider=openai
  model=mlx-community/VibeThinker-3B-4bit
  OPENAI_BASE_URL=http://127.0.0.1:8080/v1
  OPENAI_API_KEY=dummy
- VibeThink cannot be trusted for tool use or artifact authoring. Treat it as a reasoning-only model that receives bounded handoff context and returns small final outputs such as PASS/BLOCK, route labels, critiques, or missing-evidence findings.
- Minimax should perform normal agent/tool work, artifact writing, code edits, command execution, validation, and final patch production.

Primary Objective:
Find and implement a maintainable persona/orchestration combination that improves SWE-bench Pro task completion using Minimax for execution and VibeThink for narrow reasoning gates.

The work must be manual-first. Before any autonomous full SWE-bench Pro run, manually walk one target instance through the intended persona/FSM turns and write the expected state-by-state trace. The trace is the design baseline. The autonomous orchestration is then judged against that baseline.

Do not start by launching the full orchestration and observing what happens. Do not let the model discover the process by trial-and-error. First define, manually and explicitly, what a competent human team would do at each turn for the chosen task, what each state should receive, what it should produce, and what evidence would justify routing forward or blocking.

Only after the manual trace is complete, reviewed, and encoded into personas/orchestration/contracts should real SWE-bench Pro runs be used as validation.

Manual-First Sequence:
1. Select exactly one fixed SWE-bench Pro target instance for the baseline trace.
2. Read the problem statement and inspect the repository enough to understand the task yourself.
3. Manually write the intended turn-by-turn FSM trace for the target instance.
4. For each planned state, write the expected input artifacts, role responsibility, allowed actions, required output, evidence standard, pass/block condition, and next route.
5. Include the first implementation slice in the manual trace: expected files, expected minimal diff shape, expected command evidence, and expected validation interpretation.
6. Use that trace to update personas, orchestration, and runtime contracts.
7. Validate state contracts on fixed inputs or single-state/stop-after-state runs.
8. Only after the manual trace and state-level evidence are acceptable, run the autonomous full task.

Hard Gate:
- Do not run `tools/run_swebench_pro_instance.py --evaluate` for a full autonomous attempt until the manual turn-by-turn trace exists and has been used to update the FSM/persona contracts.
- Do not treat an autonomous run as exploratory replacement for the manual trace.
- If a full run is started before the trace exists, stop it and return to the trace phase.

Incremental State-Contract Requirement:
Do not optimize this by running the full orchestration for many turns first and then doing log archaeology. Work state-by-state. For each state under development, decide what must happen on that state before running it, then make that state reliably satisfy its contract on fixed inputs before composing it into longer flows.

The state contracts must be derived from the manual baseline trace, not from post-hoc interpretation of an autonomous run.

For every state being designed or changed, first decide what should actually
have happened at that stage for the task to be solved deterministically and
reliably, the way a competent team of humans would divide a large engineering
challenge. The state contract is not just output formatting. It is the
responsibility boundary for one role in that team.

For each state, write down:
- human-team role: what kind of teammate this state represents, such as surveyor, requirements analyst, environment investigator, slice planner, implementer, validator, reviewer, or release judge
- stage objective: what progress this teammate must create for the whole task
- task-progress invariant: what must be true after this state completes so the next teammate can act reliably without rediscovering or guessing
- inputs: exact artifacts/context this teammate receives and whether the original task prompt is included
- allowed actions: whether commands/tools are allowed, whether edits are allowed, and what kinds of commands are in scope
- forbidden actions: what this teammate must not attempt because it belongs to another role
- required output artifact path
- exact output schema or allowed values
- evidence requirement: what parts of the output must be grounded in command output, file contents, task text, or previous artifacts
- pass/fail checks: how to decide whether the state fulfilled its role
- failure classifications: what kind of miss happened if it fails
- model/provider assignment: which model should run it and why
- handoff quality: what the next state should be able to do using only this output plus its declared inputs

Start with the first productive states, not the full benchmark. First write their expected manual trace entries, then test or encode them:
1. repo survey
2. acceptance mapper
3. environment survey
4. slice planner
5. first implementation worker

Use VibeThink aggressively where it fits: bounded reasoning, critique, contradiction detection, route selection, acceptance coverage checks, and other judgment work where the handoff contains all needed context and the output is small. Do not force it into roles it cannot perform reliably, such as tool use, patch writing, command execution, or large artifact authorship. For each VibeThink state, define the bounded question it answers and the tiny output contract it must satisfy.

State 1 Expected Contract: repo survey.
- Input: original SWE-bench Pro problem statement and repository mounted in /app.
- Model: Minimax.
- Allowed actions: inspect repository and run non-mutating discovery commands.
- Not allowed: code edits, solving the task, broad implementation planning.
- Output: /tmp/pragma/swe/repo-survey.md.
- Required content: repo summary, stack, likely relevant paths, likely test commands, generator/build tools, uncertainties.
- Pass condition: grounded in actual command output, names concrete files/directories, does not invent task-specific implementation details, and does not propose code changes.

State 2 Expected Contract: acceptance mapper.
- Input: original task prompt and repo-survey.md.
- Model: Minimax first; VibeThink may audit only after the artifact exists.
- Allowed actions: map task requirements into acceptance IDs.
- Output: /tmp/pragma/swe/acceptance-map.json.
- Required content: blocking acceptance items with id, requirement, evidence_needed, likely_files, and validation_surface.
- Pass condition: every blocking requirement from the task appears as an acceptance item; no item depends on prior benchmark knowledge; no item names a file unless grounded in the task or repo survey.

State 3 Expected Contract: environment survey.
- Input: repo-survey.md and acceptance-map.json.
- Model: Minimax.
- Allowed actions: run cheap non-mutating commands to establish tool/build/test availability.
- Output: /tmp/pragma/swe/environment-context.md.
- Pass condition: proves which commands exist and distinguishes command availability/build success from acceptance validation.

State 4 Expected Contract: slice planner.
- Input: repo-survey.md, acceptance-map.json, and environment-context.md.
- Model: Minimax first; VibeThink may gate the resulting plan with PASS/BLOCK.
- Output: /tmp/pragma/swe/slice-plan.json.
- Pass condition: one thin slice, named acceptance IDs, likely files to inspect/edit, and a validation command tied to those IDs. It must not be a broad "fix everything" plan.

State 5 Expected Contract: implementation worker.
- Input: slice-plan.json plus repo, acceptance, and environment context.
- Model: Minimax.
- Allowed actions: inspect exact files, make the smallest code change for the slice, run planned validation or produce a concrete blocked reason.
- Output: code diff, /tmp/pragma/swe/worker-report.md, and command evidence.
- Pass condition: real diff or concrete blocked reason, commands recorded, and work maps back to acceptance IDs.

General-Purpose Constraint:
Do not add benchmark-specific guardrails, hidden constraints, postfixing, special-case routing, hardcoded artifact fixes, model-output patchups, task-name checks, repo-name checks, instance-ID checks, or clever runtime hacks to make one SWE-bench Pro task pass.

Pragma is a general-purpose long-task worker. SWE-bench Pro is only the evaluation surface. Any change must improve the generic orchestration/persona/runtime framework, not encode benchmark answers or benchmark-specific behavior.

Allowed:
- Declarative persona/orchestration configuration.
- Generic runtime capabilities such as persona/state LLM overrides, final-text artifact capture, provider env propagation, artifact routing, and schema validation.
- Generic prompt improvements that would also make sense outside SWE-bench Pro.
- Generic evidence/validation mechanisms that operate on declared artifacts, declared allowed values, declared handoffs, or declared state transitions.

Not Allowed:
- Hardcoding SWE-bench Pro instance IDs, repos, task names, file paths, acceptance IDs, evaluator behavior, or known fixes.
- Runtime branches that know about `swe-bench`, `flipt`, `kubernetes`, `prompt-control`, particular persona names, or particular artifact names.
- Post-processing model output into the desired answer except through generic declared mechanisms, such as trimming whitespace, stripping explicit `<think>` blocks, validating allowed values, or routing on declared artifacts.
- Adding guardrails that silently force success, skip states, override model decisions, invent missing evidence, or convert malformed artifacts into valid ones.
- Prompt instructions that leak benchmark-specific prior failures as rules unless they are reframed as general engineering principles.
- Any one-off fix whose justification is "this helps the current benchmark task" rather than "this is a reusable long-task orchestration capability."

Working Hypothesis:
The reliable direction is not "VibeThink plans the whole task." The reliable direction is:
- Minimax drives repository inspection, edits, tests, and artifact production.
- VibeThink should be used heavily where all required reasoning context can fit in the handoff and the expected output is tiny and machine-routable.
- Good VibeThink states are validators, blockers, route selectors, contradiction detectors, acceptance-coverage gates, and maybe "is this plan underspecified?" checks.
- Bad VibeThink states are tool states, broad planners, patch writers, or long JSON artifact authors.

Initial Investigation:
1. Audit the current SWE-bench Pro orchestration and personas.
   - Inspect:
     - orchestrations/swe-bench-pro-engineering-loop.yaml
     - personas-research-v2/*.yaml
     - internal/orchestration/runner.go
     - internal/orchestration/orchestration.go
     - tools/run_swebench_pro_instance.py
     - docs/swe-bench-pro*.md
   - Identify where failures historically happen: acceptance mapping, slice planning, insufficient validation, reviewer false-pass, scope expansion loops, final validation.

   Before changing the full FSM, determine whether the current first five states already have explicit contracts matching the state-contract requirement. If they do not, update the personas/orchestration/tests so those contracts are visible and testable.

2. Build the manual baseline trace before autonomous execution.
   - Pick one fixed target instance. Use the Flipt Kubernetes-auth task only as the selected manual baseline target, not as a hardcoded benchmark special case.
   - Manually read the problem statement.
   - Manually inspect the relevant repository surfaces.
   - Write a trace document that answers, state by state:
     - what should this state know at entry?
     - what exact question is this state responsible for answering?
     - what should this state not do?
     - what artifact should it produce?
     - what would count as grounded evidence?
     - what would count as insufficient evidence?
     - what route should follow for PASS/BLOCK/continue cases?
   - Include at least:
     - repo survey
     - acceptance mapper
     - environment survey
     - theory keeper
     - acceptance auditor
     - slice planner
     - slice plan auditor/gate
     - first worker
     - targeted validator
     - VibeThink validation gate
     - reviewer
   - The trace may include concrete files and commands discovered from the selected instance, but the resulting persona/runtime changes must be phrased as reusable state responsibilities rather than task-specific instructions.
   - Store the trace in a doc or run ledger before running the autonomous benchmark.

3. Treat model assignment as part of orchestration design.
   - Default all normal states to the configured default LLM, expected to be Minimax.
   - Use persona/state-level `llm` only for VibeThink states.
   - If an `llm` block is absent, preserve current behavior: use the default configured LLM.
   - Keep persona LLM overrides local to orchestration/persona config. Do not hardcode model choices into generic runtime.

4. Add or refine VibeThink-only reasoning states where they have bounded context and tiny outputs.
   Candidate states:
   - acceptance_map_auditor: checks whether acceptance IDs are concrete, task-grounded, and testable.
   - slice_plan_gate: PASS/BLOCK whether the slice is thin, unambiguous, and tied to blocking acceptance IDs.
   - validation_gate: PASS/BLOCK whether targeted validation actually proves the current acceptance IDs.
   - review_gate: PASS/BLOCK whether reviewer evidence supports moving to final validation.
   - final_patch_gate: PASS/BLOCK whether the patch plus validation evidence plausibly satisfies the task.
   - contradiction_gate: route label when handoff artifacts contradict each other.

5. Use VibeThink aggressively, but do not misuse it.
   - Each VibeThink state must have:
     - no command/tool expectation
     - `task_prompt: none` unless full task is absolutely required
     - explicit handoff artifacts
     - `runtime_capture: final_text` or another tiny structured output
     - `allowed_values`
     - deterministic routing via `artifact_verdict` or similar control state
   - Prefer adding more VibeThink judgment states over making one broad VibeThink state do too much.
   - Good repeated uses include checking whether requirements are complete, whether a plan is underspecified, whether validation proves the intended behavior, whether artifacts contradict each other, whether a reviewer is over-approving, and whether a handoff gives the next worker enough context.
   - If output requires large JSON, markdown reports, code patches, command evidence, or repository mutation, use Minimax unless direct evidence later proves VibeThink can perform that specific role reliably.

6. Make runtime support robust for reasoning-only models only through generic mechanisms.
   - Ensure final-text capture strips explicit `<think>...</think>` blocks if that behavior is generic and covered by tests.
   - Ensure bare outputs like `PASS`, `BLOCK`, or route labels can be routed through declared artifact values.
   - Ensure max_tokens for VibeThink states is high enough to finish reasoning, but outputs are normalized to tiny artifacts.
   - Ensure orchestration does not require bash blocks for states declared as final-text artifact states.
   - Ensure Docker runner passes both Lilac and OpenAI-compatible env vars into the benchmark container.
   - Ensure local host OpenAI base URLs are reachable from Docker, e.g. rewrite `127.0.0.1` / `localhost` to `host.docker.internal`.

7. Validate incrementally before real full runs.
   First static checks:
   - go test ./...
   - python3 -m py_compile tools/run_swebench_pro_instance.py
   - git diff --check
   - ./bin/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact
   - tools/run_swebench_pro_instance.py --prepare-only --no-generator-toolchain

   Then validate state contracts on fixed inputs from the manual baseline trace. Prefer a small local/replay harness that can run one state or stop the FSM after a named state. If that harness does not exist, implement a generic one rather than using ad hoc manual edits.

   Required evidence before a full run:
   - manual turn-by-turn trace exists for the chosen target instance.
   - each early state contract is explicitly linked back to that trace.
   - the trace identifies what the first implementation slice should be and what validation would prove it.
   - the trace identifies where VibeThink gates should PASS, BLOCK, or request rework.
   - repo survey artifact satisfies the State 1 contract on at least one fixed SWE-bench Pro task prompt.
   - acceptance map artifact satisfies the State 2 contract on that same fixture.
   - environment survey artifact satisfies the State 3 contract.
   - slice plan artifact satisfies the State 4 contract.
   - first worker pass satisfies the State 5 contract or produces a valid blocked reason.

   Only after the manual trace exists and the early state contracts are reliable, run one real known Flipt task:
   - tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate

   Capture:
   - agent-status.txt
   - .pred size and content
   - pragma.stdout.log
   - pragma.stderr.log
   - raw HTTP captures
   - evaluator result
   - orchestration artifacts under /tmp/pragma/swe if captured in logs/output

   If the run fails, classify failure into one of:
   - model/provider/env failure
   - orchestration dead-end
   - invalid artifact format
   - missing handoff context
   - Minimax tool/edit failure
   - VibeThink false PASS
   - VibeThink false BLOCK
   - insufficient validation
   - final patch semantically wrong
   - benchmark/evaluator infra issue

8. Iterate empirically.
   - Do not make broad speculative changes.
   - Do not begin with "let it run for N turns and see what happens." Start from the manual baseline trace and the earliest state whose contract is not reliably met.
   - For each failed run, identify the smallest orchestration/persona/runtime change that would have prevented that failure.
   - Prefer one new gate or one prompt correction at a time.
   - Keep a short ledger entry per run with:
     - command
     - output dir
     - models used by state
     - state where failure started
     - evaluator result
     - root cause
     - next change
   - Compare against direct Minimax-only runs when useful to prove VibeThink is helping rather than adding friction.

Review Requirement:
Before finalizing changes, scan production code and active orchestration/persona files for accidental benchmark coupling. If any specific benchmark/repo/task/model workaround is present, remove it or justify why it is a generic capability. The desired outcome is a better general-purpose long-task worker that happens to be evaluated on SWE-bench Pro, not a SWE-bench Pro bot.

Success Criteria:
- A manual turn-by-turn trace exists for one fixed SWE-bench Pro target instance and was used to design/update the FSM/persona contracts.
- The first autonomous run is launched only after the trace and early state contract evidence exist.
- The runner can execute a full SWE-bench Pro task using Minimax default plus VibeThink persona/state overrides without provider/env/runtime failure.
- At least one real SWE-bench Pro instance reaches evaluator with a non-empty patch.
- The orchestration artifacts show VibeThink states are receiving bounded handoff and producing routeable tiny outputs.
- There is evidence that at least one VibeThink gate improved trajectory quality, such as blocking insufficient validation, catching a contradiction, or preventing premature final review.
- The implementation remains maintainable:
  - no hardcoded benchmark/model logic in generic runtime
  - model selection stays declarative in persona/state `llm`
  - runner changes are limited to provider environment support and benchmark execution
  - tests cover new runtime behavior

Important Constraints:
- Do not claim SWE-bench Pro success from visualization, prepare-only, or unit tests.
- Do not judge success by model confidence. Use the official evaluator where possible.
- Do not let VibeThink write patches, commands, or large artifacts unless evidence proves it can do so reliably.
- Do not regress normal single-provider Pragma runs.
- Do not remove existing user changes.
- Do not use destructive git commands.
- Keep changes small, tested, and tied to observed failures.

Recommended First Real Run Setup:
Do not run this until the manual baseline trace and early state contract checks are complete.

export LILAC_API_KEY=...
export OPENAI_API_KEY=dummy
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1

tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
  --pull-image \
  --evaluate

Deliverables:
1. Manual turn-by-turn baseline trace for one fixed SWE-bench Pro target instance.
2. Code/config changes needed to run Minimax + VibeThink orchestration end to end.
3. Updated personas/orchestration for the chosen model split, derived from the manual trace.
4. Tests for runtime behavior touched by the orchestration.
5. State-level validation evidence before full autonomous execution.
6. One or more real SWE-bench Pro run reports with output paths and evaluator results, produced only after the manual-first gate is satisfied.
7. A clear recommendation: keep, adjust, or remove each VibeThink state based on evidence.

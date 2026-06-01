# Agent Prompt Behavior Learnings

This document records observed LLM/agent behavior from Pragma benchmark
experiments. It is about prompt effects, not orchestration architecture.

## Evidence Snapshot

Successful reference run:

- Task: `node/node-express-openapi-unconstrained.json`
- Model: `minimaxai/minimax-m2.7`
- Orchestration shape: architect, implementer, prosecutor
- Run ID: `orchestration-boundary-express-unconstrained-20260601-142353`
- Raw LLM calls: 30
- Official evaluator: pass, 32 requests, 290 assertions, 0 failures

The important behavioral result was not just that the task passed. The
architect stopped after writing its brief, the implementer did the patch, and
the prosecutor reviewed without modifying the repository.

## Durable Artifact Bias

The model appears to treat conversational responses as volatile and files as
durable artifacts. When asked to produce a plan "in the response", it still
often tried to write a file. This was visible during prompt mutation tests where
the architect wrote files such as `/repository/IMPLEMENTATION_BRIEF.md` even
though the intent was planning, not implementation.

Useful prompt pattern:

- Give each persona an exact output artifact path.
- Make the artifact the persona's deliverable.
- Put handoff artifacts outside `/repository` unless that persona is supposed
  to implement the patch.
- Tell the persona what to do immediately after the artifact exists.

Bad pattern:

- Ask for an abstract plan without naming a concrete artifact.
- Assume the model will understand that its chat message is the durable output.

## Boundary Needs Concrete Completion Mechanics

"Do not implement" by itself was not strong enough. The model can obey for one
turn, then continue into implementation on the next turn because the larger
task prompt still says to solve the task.

The prompt became much more reliable when the boundary had all of these:

- A named deliverable file.
- A local definition of completion.
- A required sentinel command after completion.
- An explicit instruction not to reread, polish, validate, or continue after
  the deliverable exists.

Example boundary shape:

- Once `/tmp/pragma/architect-brief.md` exists, this state is complete.
- Run only `echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`.
- Do not run validation.
- Do not continue into implementation.

## Positive And Negative Examples Matter

The strongest prompt mutation was not another abstract rule. It was a local
`Good`, `Bad`, and `Boundary` section for the current persona.

Observed result from live replay testing: adding concrete good/bad examples made
the architect reliably stop instead of writing implementation files.

Good examples should say what the persona does in operational terms:

- Inspect just enough of `/repository` to understand whether it is empty or
  existing.
- Extract required files, ports, endpoints, validation commands, compatibility
  rules, and evaluator behavior.
- Write ordered guidance for another persona.
- Stop after the artifact exists.

Bad examples should name the failure modes we actually saw:

- Create `package.json`, source files, tests, migrations, generated files, or
  other implementation artifacts from an architect prompt.
- Treat an empty repository as permission to scaffold.
- Continue because the next steps are obvious.
- Reread or polish the handoff artifact after it already exists.

## Keep Persona Prompts Local

Do not tell every persona about every other persona. Cross-persona prompt tables
create maintenance overhead and make the prompt harder to reason about.

Each persona prompt should describe only:

- Its own deliverable.
- Its own allowed work.
- Its own disallowed work.
- Its own completion condition.
- Its own report or artifact format.

The orchestrator owns transitions. The persona does not need to know the full
state machine.

## Do Not Expose Internal State Unless Needed

Passing internal orchestration state to the model is unnecessary unless the
state itself is part of the task. The model should be told its current role in
plain task terms, not implementation internals.

Better:

- "You are the architect for this task."
- "Your deliverable is `/tmp/pragma/architect-brief.md`."

Worse:

- "Current FSM state: architect."
- "Orchestration State: architect."

The first form gives useful behavioral direction. The second form leaks runtime
machinery without improving task execution.

## Prompt Conflicts Must Be Resolved In The Active Role Prompt

The benchmark loop includes generic solver instructions: inspect, edit, test,
submit. A persona prompt that merely adds "you are an architect" can conflict
with the default solver loop. The active role prompt must explicitly override
the parts of the generic loop that do not apply to that role.

For a planning persona, the prompt must say:

- Do not edit `/repository`.
- Do not install dependencies.
- Do not start servers.
- Do not run validation.
- Stop after writing the brief.

For an implementation persona, those same restrictions would be wrong. The
right boundary depends on the persona.

## Empty Repository Is A Trigger

When the repository is empty, the model strongly tends to scaffold immediately.
That is correct for an implementer, but wrong for an architect or reviewer.

Planning prompts must explicitly say that an empty repository is not permission
to scaffold. Review prompts must explicitly say that missing or weak
implementation should be reported, not repaired.

## Exact Artifact Paths Beat Vague Output Names

Specific paths worked better than vague names.

Good:

- `/tmp/pragma/architect-brief.md`
- `/tmp/pragma/implementer-report.md`
- `/tmp/pragma/prosecutor-verdict.md`

Risky:

- "write a plan"
- "create a brief"
- "document your findings"

Exact paths also make it easy for later personas and the harness to inspect the
handoff without parsing arbitrary chat text.

## Test Prompt Changes Against Real Payloads

Do not evaluate prompt changes only by reading them. Replay captured request
payloads against the real API when possible.

This caught important behavior:

- "Brief in response only" still caused file-writing behavior.
- Artifact-file prompting worked for one turn but initially let the architect
  continue into implementation later.
- Positive/negative examples fixed the observed continuation failure.

The useful unit of prompt testing is the actual captured payload at the failure
turn, not a simplified invented prompt.

## Self-Reported Validation Is Weak Evidence

The agent's own report is useful, but not authoritative. In the successful
Express OpenAPI run, the prosecutor performed only shallow validation: file
inspection and health-check. The official Newman evaluation later proved the
patch passed.

Rule:

- Treat official evaluator pass/fail as the result.
- Treat local validation and persona reports as diagnostic evidence.
- Do not call a run solved because the model says it is complete.

## Hard Subproblems Can Replace The Checklist

Observed in SWE-bench Pro task
`instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`:
the architect correctly identified config fixture and test updates, including
`internal/config/testdata/authentication/kubernetes.yml`, but the implementer
lost that checklist after protobuf generation became difficult.

The exact trigger was toolchain friction. The implementer tried
`which buf || go install github.com/bufbuild/buf/cmd/buf@latest`; installation
failed because `buf@v1.70.0` required Go `>= 1.25.10` while the benchmark image
had Go `1.24.3` with `GOTOOLCHAIN=local`. The model then hand-edited generated
protobuf files for many turns. That generated-code repair became the center of
gravity, and the model never returned to the architect's full requirement list.

Failure pattern:

- The model starts from a good handoff.
- A technically noisy subproblem appears.
- The model spends many turns making that subproblem compile.
- Once local compile or visible tests look good, the model reports completion.
- Earlier checklist items that were not part of the noisy subproblem are skipped.

Prompt implication: implementer and repair personas need an explicit final
reconciliation step before writing their report. The report should map each
task or handoff requirement to concrete evidence, not only summarize files
touched and commands run. Visible local tests and grep summaries are not enough
when the handoff named specific fixtures, configs, or call sites.

## Long-Running Commands Need Prompt Awareness

The command environment now soft-waits and returns active process/log
information instead of blocking indefinitely. The prompt must tell the model
this behavior so it knows how to continue after a server or watcher keeps
running.

Useful instruction:

- Commands wait briefly for immediate output.
- If still running, the command is not killed.
- The model receives the PID and log paths and can inspect them later.

Without this, the model may assume a long-running command failed, hung, or
needs to be killed immediately.

## Current Prompt Rules Of Thumb

- Prefer concrete artifacts over abstract instructions.
- Put handoff files outside `/repository`.
- Make the persona boundary local and explicit.
- Use persona-local `Good`, `Bad`, and `Boundary` examples.
- Do not describe the whole orchestration to every persona.
- Do not leak internal state-machine labels unless they are useful to the task.
- Do not rely on "do not implement" alone.
- Test prompt mutations against captured real API payloads.
- Judge success with the benchmark evaluator, not model confidence.
- Require final checklist-to-evidence reconciliation before implementation or
  repair reports are written.

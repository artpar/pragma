# Transition-Scoped Handoff Runtime Implementation Goal Prompt

> Inactive historical task specification. Do not execute as a standing goal.
> Follow [agent.md](../agent.md); verify current code before assuming the bug remains.

Use this prompt to drive an implementation agent that must convert Pragma
orchestration prompt handoff from destination-state inputs to transition-scoped
handoff artifacts.

```text
You are working in this repository with one objective:

Implement transition-scoped orchestration handoff so stale or wrong handoff
artifacts cannot reach multi-entry states such as `item_worker`, `patch_planner`,
and `checklist_writer`.

Mandatory governing documents and source files:

- Read `docs/clean-architecture-enforcement-doctrine.md` first and treat it as
  an execution rule.
- Read `docs/transition-scoped-handoff-runtime-plan.md` as the feature plan.
- Inspect the current orchestration runtime before editing:
  - `internal/orchestration/orchestration.go`
  - `internal/orchestration/runner.go`
  - `internal/orchestration/projection.go`
  - `internal/query/event.go`
  - `internal/app/state.go`
  - `internal/session/writer.go`
  - `internal/session/store.go`
  - `internal/cli/run.go`
  - `cmd/pragma/orchestration.go`
  - `orchestrations/prompt-control-v2-benchmark.yaml`
  - every other YAML file under `orchestrations/`

Do not summarize the plan and stop. Implement the runtime, YAML migration, and
proofs.

Core rule:

The transition that was actually taken is the only owner of prompt handoff for
the destination state. Destination states may keep output artifact contracts, but
they must not guess predecessor context by rendering static `artifacts.inputs`.

Strict operating constraints:

- Work from the actual current source and call graph.
- Keep the implementation inside the orchestration runtime boundary. Do not
  repair this with persona prompt prose, UI-side parsing, replay-only behavior,
  downstream artifact cleanup, or generic "read all possible artifacts" logic.
- Do not preserve a compatibility fallback that silently renders
  destination-state input artifacts as prompt handoff.
- Do not change the state-owned output artifact contract except where required
  to keep it distinct from handoff input rendering.
- Do not make control states promptable. Control states emit events and may write
  artifacts; persona states receive prompts.
- Do not infer transition identity from printed text, event order in logs, model
  memory, filenames, or destination state alone. Resolve it from the FSM source
  state and emitted event.
- Do not leave existing orchestration YAML files in a half-migrated state.

Required implementation workflow:

1. Extend `orchestration.Transition` with `Handoff []Artifact
   yaml:"handoff,omitempty"`.
2. Add validation for transition handoff artifacts using the same artifact rules
   as state artifacts: id required, path required, non-empty allowed values.
3. Add validation that every `(from_state, event)` pair resolves to exactly one
   transition. Reject duplicate transition keys during definition validation.
4. Add a runtime transition lookup keyed by the current source state and emitted
   event. Use it before or while applying the FSM event, and carry the exact
   transition's handoff artifacts into the next loop iteration.
5. Change prompt construction so `## Handoff From Previous Phase` renders only
   the handoff artifacts carried from the transition that led to the current
   persona state.
6. Stop using `state.Artifacts.Inputs` as the prompt handoff source. If the type
   remains for compatibility with old definitions, it must no longer be the
   runtime source for prompt handoff after migration.
7. Preserve current task-prompt semantics precisely: the run prompt is supplied
   to the first non-control persona state that is allowed to receive it, then
   cleared. `task_prompt: none` must still suppress task prompt rendering.
8. Preserve control-state behavior:
   - `foreach_next` still writes `cursor_path`.
   - `foreach_next.handoff_path` still writes the selected-item handoff file.
   - `mark_current_item`, `artifact_verdict`, and `artifact_decision` still only
     emit their configured event after reading or writing their control artifacts.
   - Transition handoff rendering happens for the next persona state after the
     control state emits its event.
9. Preserve required/optional handoff semantics:
   - Missing required transition handoff artifacts fail before calling the model.
   - Missing optional transition handoff artifacts are omitted.
   - Non-missing read errors must not be hidden as successful handoff omission.
10. Preserve state-owned output artifact contracts and completion checks:
    required output paths must still be enforced before persona state completion.
11. Update run-directory preparation so directories required by transition
    handoff artifacts are created just like state artifact and control paths.
12. Update orchestration events/projection/session artifact recording only as
    needed to make handoff reads and writes auditable. If adding handoff read
    events, include enough identity to prove source state, event, destination
    state, artifact id, path, and direction.
13. Migrate `orchestrations/prompt-control-v2-benchmark.yaml` so prompt handoff
    lives on transitions, not destination-state inputs.
14. Inspect and either migrate or intentionally retire any other orchestration
    YAML that depends on destination-state `artifacts.inputs`, including
    `orchestrations/playful-checklist-loop.yaml`.
15. Keep persona YAML changes minimal. Only update persona text if current text
    directly contradicts the new runtime-owned handoff contract.

Required prompt behavior:

- Initial task work receives the task prompt according to current runtime
  semantics.
- Later persona prompts include session context, the transition-scoped
  `## Handoff From Previous Phase` when the taken transition has readable
  handoff artifacts, and the state-owned output artifact contract.
- `next_item --item_available--> item_worker` must render `current_item` and
  `current_item_handoff`, and must not render router-only artifacts.
- `route_item_block_classification --redo_item_worker--> item_worker` must render
  the retry/block handoff artifacts declared on that transition.
- `mark_item_completed --complete--> checklist_writer` must not render stale
  `item_block_classification`.
- `route_item_block_classification --repair_checklist_scope--> checklist_writer`
  must render the classification and checklist repair context declared on that
  transition.

Required tests and proofs:

- Add focused unit tests for:
  - transition handoff unmarshalling and validation,
  - duplicate `(from,event)` transition rejection,
  - exact transition lookup from source state and emitted event,
  - required transition handoff failure before model call,
  - optional transition handoff omission when absent,
  - no destination-state input handoff rendering after migration,
  - state-owned output artifact completion checks still enforcing required
    outputs.
- Add prompt construction tests for the branch cases listed above.
- Add YAML load tests proving every active orchestration definition is valid
  after migration.
- Add or update an integration-style proof that runs enough of the orchestration
  loop with controlled artifact files to show first-attempt `item_worker`,
  retry `item_worker`, normal `checklist_writer`, and repair
  `checklist_writer` receive different, correct handoff prompts.

Completion criteria:

- No active orchestration runtime path renders prompt handoff from destination
  `state.Artifacts.Inputs`.
- Every emitted non-terminal event maps to exactly one transition for the source
  state.
- The taken transition's handoff artifacts are the only prompt handoff source
  after the task prompt phase.
- Required transition handoff artifacts fail before model invocation when
  missing.
- Optional transition handoff artifacts never cause stale branch artifacts to be
  rendered.
- Output artifact contracts and required-output completion checks still work.
- All active orchestration YAML files load under the migrated schema.
- The final answer lists changed runtime files, migrated YAML files, tests added,
  and exact verification commands run.

Failure condition:

If the implementation relies on destination state alone, old `artifacts.inputs`
rendering, generic artifact scanning, persona prompt instructions, replay-only
filtering, UI/session reconstruction, or "the model should know which branch it
came from," the implementation is not acceptable. Move ownership back to the
taken transition before continuing.
```

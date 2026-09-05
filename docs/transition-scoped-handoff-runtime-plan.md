# Transition-Scoped Handoff Runtime

> Historical design plan, not an active implementation queue. Statements about
> the current bug below refer to the plan's original context. Revalidate against
> current source and follow [agent.md](../agent.md) before making changes.

## Summary

Fix the handoff architecture by moving prompt handoff ownership from destination-state `artifacts.inputs` to the actual transition that led to the destination.

Current bug:

- A destination state statically declares inputs.
- Runtime renders those inputs regardless of which predecessor routed there.
- Multi-entry states like `item_worker`, `patch_planner`, and `checklist_writer` can receive stale or wrong artifacts.

Target behavior:

- Each transition declares the handoff artifacts it sends to the next state.
- Runtime renders only the handoff for the transition that was actually taken.
- Destination states no longer guess their predecessor context.

## Key Changes

- Extend `Transition` with edge-scoped handoff:

  ```go
  type Transition struct {
      Event   string
      From    []string
      To      string
      Handoff []Artifact `yaml:"handoff,omitempty"`
  }
  ```

- Add a transition lookup helper keyed by `(from_state, event)`:
  - Validate there is exactly one transition for each `(from, event)` pair.
  - After a state emits an event, find the exact transition before or while applying the FSM event.
  - Carry that transition's `handoff` into the next loop iteration.

- Replace destination-input prompt rendering:
  - Stop using `state.Artifacts.Inputs` as the source for `## Handoff From Previous Phase`.
  - Render only the current transition's `handoff` artifacts.
  - Keep `state.Artifacts.Outputs` unchanged for runtime output contracts and completion checks.

- Preserve the special `foreach_next` behavior as a normal transition handoff:
  - `next_item` still writes `current_item` and `current_item_handoff`.
  - The `next_item --item_available--> item_worker` transition declares those artifacts as its handoff.
  - The `route_item_block_classification --redo_item_worker--> item_worker` transition declares the retry/block artifacts instead.

## YAML Migration

Update `orchestrations/prompt-control-v2-benchmark.yaml` so handoffs live on transitions:

- Linear planning chain:
  - `surface_mapper -> evidence_collector`: handoff `surface_map`.
  - `evidence_collector -> behavior_evidence_mapper`: handoff `surface_map`, `repo_evidence`.
  - `behavior_evidence_mapper -> patch_planner`: handoff `surface_map`, `evidence_map`.
  - `patch_planner -> checklist_writer`: handoff `patch_plan` plus current active repair context only when relevant.

- Item loop:
  - `next_item --item_available--> item_worker`: handoff `current_item`, `current_item_handoff`.
  - `item_worker -> item_reviewer`: handoff `current_item`, `implementer_report`.
  - `item_reviewer --item_block--> item_block_router`: handoff `current_item`, `implementer_report`, `item_verdict`, and repair context needed for routing.
  - `route_item_block_classification --redo_item_worker--> item_worker`: handoff `current_item`, `current_item_handoff`, `implementer_report`, `item_verdict`, `item_block_classification`.
  - `route_item_block_classification --repair_checklist_scope--> checklist_writer`: handoff `checklist`, `patch_plan`, `current_item`, `item_verdict`, `item_block_classification`.
  - `mark_item_completed -> checklist_writer`: handoff `checklist`, `patch_plan`, and no stale item-block classification.

- Remove prompt-relevant `artifacts.inputs` from states after migration. States keep only `artifacts.outputs`.

## Runtime Behavior Details

- Initial state still receives the full task prompt.
- Every later persona state receives:
  - session context,
  - transition-scoped `## Handoff From Previous Phase`,
  - its output artifact contract.
- If a transition declares a required handoff artifact and the file is missing, fail that state before calling the model.
- Optional handoff artifacts are rendered only when listed on the taken transition and present on disk.
- Stale artifacts from older branches are not rendered unless the current transition explicitly names them.

## Verification Scenarios

- Existing focused check:
  - `go test ./internal/orchestration`

- Manual payload proof:
  - Build/replay `next_item --item_available--> item_worker`; confirm prompt includes `current_item` and `current_item_handoff`, not router artifacts.
  - Build/replay `route_item_block_classification --redo_item_worker--> item_worker`; confirm prompt includes `item_block_classification` and previous report/verdict.
  - Build/replay `mark_item_completed -> checklist_writer`; confirm prompt does not include stale `item_block_classification`.
  - Build/replay `repair_checklist_scope -> checklist_writer`; confirm prompt includes the classification and checklist.
  - Run the benchmark task again and confirm `redo_item_worker` does not receive the same prompt as first-attempt `item_worker`.

## Assumptions

- No compatibility fallback to destination-state input rendering.
- No generic "read every possible artifact" behavior.
- Transition handoff is the only prompt handoff source after the initial task prompt.
- Output artifact contracts remain state-owned.

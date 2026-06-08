# Orchestration Experiments

Pragma's current baseline is the single-agent Pragma loop: one model, one bash action per turn, no explicit delegation. That baseline is strong enough on SWE-bench, while SWE-bench Pro is the current pressure test.

The next experimental phase is not "add more tools, structure, and gates" as standalone improvements. The variable under test is delegation: how planning, implementation, verification, critique, and authority are distributed across agents or roles.

## Experiment Principles

- Treat orchestration pattern as the independent variable.
- Keep the model, temperature, benchmark harness, max turns, and evaluator fixed unless the experiment explicitly changes them.
- Prefer patterns that attack observed SWE-bench Pro failures: broad unfocused exploration, partial API-compatible fixes, false confidence from weak tests, syntax/compile-invalid patches, and context/cost exhaustion.
- Measure against the current Pragma loop, not against an imagined ideal.
- A run is successful only if the official benchmark evaluator passes. Patch generation, local targeted tests, or agent confidence are secondary signals.

## Orchestration Scenarios

Only keep scenarios that change delegation mechanics: who investigates, who edits, who verifies, who can veto, or when a prior result is handed to another agent. Prompt tone, generic checklists, and "be careful before submit" are not separate orchestration scenarios.

| Scenario | Delegation Pattern | Authority Model | What It Tests | Main Risk |
|---|---|---|---|---|
| Baseline single solver | One agent investigates, edits, tests, submits | Solver has full authority | Control group | Anchoring, context bloat, weak self-verification |
| Planner then executor | Planner investigates and writes a concrete plan; executor edits only from the plan | Executor may reject impossible steps but cannot re-plan freely | Whether separating diagnosis from editing reduces thrash | Planner misses implementation detail |
| Architect / implementer / prosecutor | Architect proposes repair theory; implementer patches; prosecutor tries to break it, including API contract and compile evidence | Prosecutor can veto submission | Catches hidden contract/API mismatches before final patch | Extra cost and latency |
| Competing planners | Multiple planners independently produce diagnoses; one executor implements selected plan | Judge chooses the plan before edits | Whether independent hypotheses beat early anchoring | Judge may select persuasive but wrong plan |
| Competing solvers with late merge | Multiple solvers produce candidate patches or patch plans | Judge selects one patch or synthesizes minimal merge | Diversity at full execution depth | Expensive; merge can introduce inconsistency |

## Fast Test Strategy

Use a staged ladder. Phase one runs only the current hard task:

```text
instance_flipt-io__flipt-05d7234fa582df632f70a7cd10194d61bd7043b9
```

Do not broaden the task set until a delegation pattern shows signal on this instance.

1. **Replay and transcript grading**
   - Use existing failed Pragma trajectories to simulate where a delegate would have intervened.
   - Score whether the proposed role would have caught the known failure before submission.
   - Fastest target: the Flipt ETag failure, where the missing `NewFile` API contract is known.

2. **Prompt-only harness variants**
   - Encode one scenario at a time inside the current Pragma loop prompt without adding runtime machinery.
   - Example variants: "planner then executor in one transcript", "architect / implementer / prosecutor in one transcript".
   - Run only on the current Flipt instance.

3. **Offline judge on generated artifacts**
   - For each run, collect `prompt.txt`, `pragma.stderr.log`, raw HTTP count, `.pred`, eval result, patch size, and turn count.
   - Add lightweight post-run classification: compile error, wrong solution, timeout, no patch, oversized patch, artifact leakage, evaluator pass.
   - This gives signal without needing every orchestration variant to be fully automated.

4. **Minimal runtime delegation**
   - Implement only the smallest orchestration primitive needed for the best prompt-only scenario.
   - First candidates should be low surface area: planner/executor or prosecutor-after-patch.
   - Avoid rebuilding the whole tool loop unless the experiment requires actual parallel tool use.

5. **Promotion to broader slices**
   - Only after a scenario improves the current Flipt task outcome or produces a clearly better failure mode, promote it to a small fixed slice from `docs/swe-bench-pro-task-results.md`.
   - The first promotion slice should include regression tasks and harder tasks, but it is not part of phase one.

## First Experiments

Start with scenarios that directly target the latest observed failure and require the least harness change.

| Order | Scenario | Why First | Fast Acceptance Signal |
|---:|---|---|---|
| 1 | Architect / implementer / prosecutor | Cleanly separates solution creation from adversarial verification | Prosecutor blocks compile-invalid or API-incomplete patches |
| 2 | Planner then executor | Simple to simulate in prompt and later implement | Fewer exploratory turns and smaller patch than baseline |
| 3 | Competing planners | Diversity without parallel patch merges | Selected plan identifies hidden acceptance contract more often |

## Measurement Template

Record each run in a small table or JSONL with:

- `instance_id`
- orchestration scenario and prompt/runtime variant
- model/provider/temperature/max turns
- official eval result
- failure category if failed
- raw model request count
- visible turn count
- wall time
- cost
- patch size
- changed file count
- whether patch compiles in evaluator
- notes on which delegate caught or missed the decisive issue

The key comparison is not just pass/fail. A failed experiment is still useful if it moves the failure from "unfocused exploration" to "small wrong patch", or from "compile error" to "hidden behavioral miss". That tells us which delegation boundary improved and which one remains weak.

## Implementation Leverage Points

The current repo already has pieces that can support these experiments:

- The Pragma loop is the current benchmark path and can host prompt-only role simulations quickly.
- The Agent tool, task tools, worktree tools, MCP adapters, and skills are retained and can be reused as implementation mechanisms once a scenario needs real delegation.
- The SWE-bench Pro runner already captures prompts, stderr, raw HTTP, predictions, and evaluator output, which is enough for early experiment scoring.

Do not start by exposing every old tool to the benchmark model. Start with the cheapest orchestration change that can falsify a delegation hypothesis.

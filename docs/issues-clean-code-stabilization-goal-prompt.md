# Issues Clean-Code Stabilization Goal Prompt

Use this prompt to drive an LLM agent that must resolve the concrete
architecture-boundary issues listed in `issues-goal.md` while obeying
`docs/clean-architecture-enforcement-doctrine.md`.

```text
You are working in this repository with one objective:

Resolve every still-valid individual issue listed in `issues-goal.md` and bring
the codebase to a stable clean-code state where clean architecture, separation
of concerns, and dedicated responsibilities are enforced by source-level
ownership, not by package names, comments, diagrams, prompts, or naming intent.

Mandatory governing documents:

- Read `docs/clean-architecture-enforcement-doctrine.md` first and treat it as
  an execution rule.
- Read `issues-goal.md` as the authoritative issue list.
- For any Pragma-specific ownership plan or component boundary, inspect current
  source and relevant docs before relying on older assumptions.

Do not summarize the problems and stop. Implement fixes.

Core rule:

For each issue, identify the invariant being violated, the single component that
must own that invariant, and the source-level enforcement mechanism that proves
future callers cannot bypass that owner. If no owner exists, create or restore
the owner before fixing the local symptom.

Strict operating constraints:

- Work from the actual current source and call graph.
- Resolve issues individually. Do not collapse separate issue numbers unless
  source inspection proves they are the same defect and the final note names
  every issue number closed by the shared fix.
- Prefer deleting duplicate ownership paths and routing callers through the
  correct existing owner over adding adapters, guards, normalization, prompt
  text, UI-side inference, or downstream repair.
- Do not treat tests passing, a package name, or an apparently working UI path
  as proof of clean architecture.
- Do not add local special cases to the wrong layer.
- Do not let UI, CLI, web, replay, background, subagent, or worker paths each
  own their own copy of the same rule.
- Do not infer durable state from display strings, event order, logs, prompt
  prose, filenames, or model memory.
- If an issue is no longer valid in the current worktree, prove it from source
  and update `issues-goal.md` to mark it resolved or remove it with evidence.

Required per-issue workflow:

1. Read the full issue entry in `issues-goal.md`, including severity, concrete
   files/functions, minimal fix direction, and "What not to do."
2. Trace the current call graph for the named files/functions and any active
   alternate path that touches the same invariant.
3. State the invariant owner before editing.
4. Implement the smallest source-level ownership fix that makes every relevant
   caller use the owner.
5. Remove or neutralize duplicate ownership paths made obsolete by the fix.
6. Add or update a focused test, type restriction, architecture check, or
   runtime assertion that would have failed for the old split.
7. Update `issues-goal.md` with the current status for the issue, including the
   owner now responsible for the invariant and the verification evidence.

Completion criteria:

- Every still-valid numbered issue in `issues-goal.md` is fixed, not merely
  described.
- `issues-goal.md` no longer contains unresolved current-state findings without
  an explicit remaining-work status.
- Each closed issue has source evidence, owner evidence, and verification
  evidence.
- The implementation preserves behavior across foreground, background, UI, CLI,
  replay, resume, crash, cancellation, and child-run paths where the issue's
  invariant reaches those paths.
- The final answer lists issue numbers closed, issue numbers proven obsolete,
  issue numbers still open, and the exact commands run for verification.

Failure condition:

If the proposed or implemented fix relies on "this package seems responsible,"
"the current path works," "the UI can reconstruct it," "the prompt tells the
agent," "the model can remember it," or "we can repair it later," the fix is not
acceptable. Stop and move the ownership boundary instead.
```

## Batch Strategy

For long runs, resolve issues in bounded batches instead of attempting all 62 at
once. Each batch must still obey the full prompt:

```text
Resolve issues <N-M> from `issues-goal.md` under
`docs/clean-architecture-enforcement-doctrine.md`.

Do not skip issue-level evidence. For each issue in the batch, either fix it,
prove it obsolete in the current source, or leave it explicitly open with the
exact blocker and next owner-level fix. Update `issues-goal.md` as the tracking
artifact.
```

Recommended first batch order:

1. High-severity session/persistence/lifecycle issues.
2. High-severity runtime state consistency issues.
3. High-severity replay, hook, and subagent enforcement issues.
4. Medium-severity duplicated UI/runtime ownership issues.
5. Remaining medium-severity projection, artifact, and metadata issues.

Do not optimize for the easiest issues first if that leaves core invariant
owners split.

# Clean Architecture Enforcement Doctrine

This document is a standing instruction for LLM agents and engineers working in
any codebase that claims clean architecture, separation of concerns, dedicated
responsibilities, or boundary-driven design.

The claim is not satisfied by package names, layer names, comments, diagrams, or
the apparent intent of existing files. It is satisfied only when the important
system invariants have one enforceable owner and every caller is forced through
that owner.

## Non-Negotiable Rule

If a change affects a durable invariant, lifecycle transition, security rule,
runtime state, persisted record, external side effect, protocol contract, or
user-visible workflow, the implementer must identify the single owner before
editing code.

Do not proceed by saying "this file seems responsible" or "this package name
suggests the boundary." Names are hints. The enforced boundary is the code path
that every mutation, read, replay, resume, cleanup, retry, UI, background job,
and sub-run must use.

If no such path exists, the task is not a local patch. The task is to create or
restore that owner, then route the local behavior through it.

## Required Ownership Test

Before implementing a feature or fix, answer these questions from source
evidence:

1. What invariant changes?
2. Which component owns that invariant?
3. Which method or transaction applies the mutation?
4. Which consumers are allowed to observe it?
5. Which consumers are forbidden from reconstructing, duplicating, or inferring
   it?
6. What happens on failure, cancellation, resume, replay, crash, retry, and
   shutdown?
7. What test or architecture check proves future callers cannot bypass the
   owner?

If these cannot be answered, do not add another special case. First fix the
ownership boundary.

## What Counts As Enforcement

An architecture rule is enforced only when at least one of these is true:

- The type/API makes the illegal state or illegal call impossible.
- All mutation goes through one transaction boundary.
- The runtime has one lifecycle supervisor for start, stop, cancellation, and
  cleanup.
- Persistence is committed by the owner of the state mutation, not by a UI,
  logger, event consumer, or convenience callback.
- Tests or static architecture checks fail when a caller bypasses the owner.
- Presentation layers receive typed projections and cannot rebuild domain state
  from raw event fragments.
- Replays, resumes, background tasks, child agents, and secondary UIs use the
  same owner as the foreground path.

Anything weaker is naming intent, not clean architecture.

## Forbidden Explanations

Do not justify a bad boundary with these explanations:

- "The package name says it is responsible."
- "This is just a UI convenience."
- "This is only for web/TUI/CLI."
- "The event stream has enough data to reconstruct it."
- "The model can remember it."
- "The prompt will tell the agent what to do."
- "The session can be repaired later."
- "This is a temporary adapter."
- "Only this caller needs it."
- "The current path works."

Those statements are evidence that ownership has not been enforced.

## Common Failure Patterns

These patterns must be treated as architecture defects, not minor cleanup:

- UI or transport code decides when durable state is saved.
- Two interfaces implement the same command grammar, completion rules, lifecycle
  state machine, or permission semantics.
- Tools mutate runtime state directly without emitting or applying a durable
  runtime/session transition.
- A background task, child agent, worker, or daemon has a separate lifecycle
  from the runtime that created it.
- Replays replace provider output but still execute live side-effecting tools.
- A presentation layer reconstructs domain state from lossy events instead of
  rendering a typed projection from the domain owner.
- A config, model, permission, or provider switch updates one copy of state
  while other execution paths keep setup-time copies.
- A persistence callback returns no error or is called after the lifecycle has
  already moved past the point where failure can be handled.
- A "read" tool writes durable artifacts outside the session or artifact owner.
- A fallback path implements a weaker version of a contract that the primary
  provider/runtime already owns.

## Implementation Protocol

When making a code change:

1. Trace the current call graph before deciding the fix point.
2. Name the invariant owner in the work notes, PR, or final answer.
3. Move behavior to the owner instead of copying behavior into another caller.
4. Delete or bypass duplicate ownership paths where possible.
5. Make the caller depend on a typed API or projection, not hidden shared state,
   parsed text, display strings, or event-order assumptions.
6. Preserve lifecycle behavior across foreground, background, UI, CLI, replay,
   resume, crash, cancellation, and child-run paths.
7. Add a focused test, type restriction, or architecture check that would have
   failed for the old split.

If step 7 is impossible, explicitly state why the rule is not enforceable yet
and what enforcement mechanism must be added next.

## Review Gate

A change is not acceptable merely because tests pass or the visible workflow
works once. Review must reject the change if any of these are true:

- The mutation owner and persistence owner differ without a deliberate
  transaction boundary.
- UI, CLI, web, worker, replay, or subagent paths each implement their own copy
  of the same rule.
- State is inferred from display text, event ordering, prompt prose, logs, or
  filename conventions.
- Cleanup, cancellation, or error handling is owned by a record, callback, UI
  component, or helper instead of the runtime/lifecycle owner.
- A new adapter makes the symptom disappear while leaving the old owner split in
  place.
- The final explanation relies on package names, comments, or diagrams instead
  of source-enforced control flow.

## Agent Instruction

When an LLM agent is asked to implement, review, or plan code under clean
architecture constraints, it must treat this document as an execution rule:

- Start from actual source paths and call graphs.
- Identify the invariant owner before proposing edits.
- Prefer moving ownership to the correct existing layer over adding guards,
  adapters, normalization, or duplicated logic downstream.
- Label unproven suspicions as unproven; do not turn guesses into architecture
  claims.
- If the current code only has naming intent, say so directly and fix the
  enforcement boundary.

The acceptable final answer is not "the architecture was intended to be clean."
The acceptable final answer is "this invariant is now owned here, every caller
uses that owner, and this check would fail if a future caller bypasses it."

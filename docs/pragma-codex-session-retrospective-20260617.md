# Pragma Codex Session Retrospective, 2026-06-17

## Scope

This report investigates the Codex work done on Pragma using the local Codex
session logs under `~/.codex/sessions`, then cross-checks those findings against
the durable repo artifacts created during the work.

The goal is not to declare Pragma solved. The goal is to record what has been
tried, what the evidence says, what kept failing, and what should be done next.

## Evidence corpus

Primary source:

- Exact Codex sessions where `session_meta.payload.cwd` equals
  `/Users/artpar/workspace/code/pragma`.
- Count: 54 exact-cwd sessions.
- Historical work sessions before this report continuation: 53.
- Date range: 2026-05-08 through 2026-06-17.
- Parsed totals across the 54 exact-cwd sessions: 3,270 `turn_context` records,
  3,509 non-environment user messages, and 45,742 function/tool-call records.
- Extraction method: JSONL session parsing by `session_meta`, `turn_context`,
  user/assistant message items, tool calls, and touched-path hints from tool
  arguments. Environment-context and goal-context messages were ignored for
  prompt summaries. Goal continuation wrappers were parsed for their
  `<objective>` text when the visible user message was only the wrapper.

Adjacent context:

- The primary corpus is exact-cwd only. A broader scan found two adjacent
  benchmark-context sessions outside this checkout that are relevant background
  but not counted as Pragma project sessions:
  `/Users/artpar/workspace/code/constraint-decay` session
  `019e6278-c073-7d11-ae73-c67229625671` and
  `/Users/artpar/workspace/code/swe-pragma` session
  `019e62b5-a68a-7491-a64f-63372f860e76`, both on 2026-05-26.
  They explain Mini-SWE harness parity and repo-selection context. They are not
  part of the primary "what we tried in this checkout" count.

Repo artifacts cross-checked:

- `docs/prompt-control-ab-tests.md`
- `docs/personas-from-prompt-control-research.md`
- `docs/codex-vs-pragma-flipt-kubernetes-trajectories-20260603.md`
- `docs/codex-flipt-kubernetes-run-20260603-report.md`
- `docs/swe-bench-pro-flipt-kubernetes-incident.md`
- `docs/swe-bench-pro-flipt-kubernetes-run-20260603T064623Z-rca.md`
- `docs/swe-bench-pro-orchestration-ab-goal-prompt.md`
- `docs/swe-bench-pro-orchestration-ab-ledger.md`
- `docs/swe-bench-pro-orchestration-fsm-persona-goal-prompt.md`
- `docs/swe-bench-pro-orchestration-fsm-persona-ledger.md`
- `docs/transition-scoped-handoff-runtime-plan.md`
- `docs/transition-scoped-handoff-runtime-implementation-goal-prompt.md`
- `.pragma/exports/20260616T173446Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446-single-owner-evidence-adjudication-lilac-minimax-turn-by-turn/turn-by-turn-investigation.md`
- `.pragma/exports/20260616T173446Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446-single-owner-evidence-adjudication-lilac-minimax-turn-by-turn/phases.md`
- `.pragma/prompt-ab/20260617T000000Z-runtime-evidence-ownership-ab/report.md`
- `.pragma/prompt-ab/20260617T000000Z-runtime-evidence-ownership-ab/results.md`

Current worktree state was also checked. At the time this report was written,
there were no staged changes. The tracked diff contained broad ongoing changes
across runtime, CLI, orchestration, query loop, persona YAML, deleted legacy
personas, and deleted legacy orchestration YAML. There were also untracked SWE
orchestration docs, examples, a new `task-evidence-item-loop.yaml`, several new
SWE personas, and `pragma-goal.md`. This report treats that worktree as
unfinished current state, not as a completed verified baseline.

## Executive diagnosis

The Pragma work has not been one linear attempt. It went through several
different hypotheses:

1. Fix basic runtime/tool-loop defects.
2. Build and compare Mini-SWE/SWE-bench style harnesses.
3. Use persona/FSM decomposition to preserve task truth over long trajectories.
4. Use raw HTTP replay and direct AB testing to find prompt controls that alter
   turn behavior.
5. Move handoff ownership from destination states to transition edges.
6. Remove task-specific hardcoding from runtime, YAML, and persona prompts.
7. Collapse or redesign ownership topology when too many personas failed to
   maintain truth.
8. Separate runtime-owned evidence from model-authored deliverables when the
   model was being asked to "fix" evidence that only runtime can truthfully
   produce.

The main learning is narrower than "personas are bad" or "runtime needs
hardcoded guardrails." The repeated failures point to contract ownership
problems:

- A model can follow a persona prompt and still corrupt the system if the latest
  runtime message presents a runtime-owned artifact as a writable model output.
- A reviewer persona can only validate truth if the orchestration gives it the
  right state and evidence. If the FSM loops in the worker forever, reviewer
  quality does not matter.
- Prompt-only fixes work for some local behaviors, but many controls only hold
  when the relevant decision packet is in the latest model-visible context.
- Full-trajectory reliability cannot be claimed from one-turn AB evidence. AB
  evidence can justify the next implementation slice, not the end state.

The strongest current evidence supports one immediate direction:

Generic artifact ownership semantics should be completed and verified. Model
authored outputs and runtime-authored evidence must be declared, rendered, and
rejected differently. This is not a benchmark-specific command probe or
task-specific guardrail. It is a generic ownership boundary.

That direction is supported only for the observed evidence-file loop. It does
not yet prove that the full SWE-bench Pro task will finish or pass.

## Timeline of attempts

### 2026-05-08 to 2026-05-11: runtime loop and tool reliability

Early exact-cwd sessions were about basic Pragma behavior:

- File-edit tool crashes while using Pragma on Pragma.
- Context-limit failures.
- Models saying they would act but not executing tools.
- Handoff presentation experiments.

The touched areas included `internal/tool`, file read/edit tools,
`internal/query/loop.go`, provider/debug code, and early handoff models.

What this established:

- Pragma needed better runtime observability and control of model/tool loops
  before benchmark work could be meaningful.
- Model behavior defects were not only prompt defects; some were runtime-loop
  presentation and completion-contract defects.

### 2026-05-17 to 2026-05-20: toolsets, MCP, TUI, and project hygiene

The next cluster explored:

- codebase size and project state;
- JetBrains/MCP tool integration;
- cleaning Claude-specific naming from Pragma;
- toolset selection such as an "idea" toolset;
- slash command and TUI behavior.

This phase touched `internal/mcp`, `internal/cli/tools.go`, TUI input/model
code, replay commands, token accounting, and project agent docs.

What worked:

- The codebase gained better understanding of tool boundaries and local
  interactive surfaces.

What did not become central:

- Toolset/MCP improvements did not address the later SWE-bench failure mode.
  The benchmark failures were mostly contract/state/evidence failures, not a
  lack of external tools.

### 2026-05-28 to 2026-05-31: Mini-SWE, SWE-bench, raw capture, and prompt-control base

The large May 28 and May 31 sessions were the first major benchmark push.
Session `019e6c91-2028-75a1-b881-4b50daac47a4` had 407 turns; session
`019e7d67-52a0-7d32-b009-c793e98fd2ae` had 743 turns.

This work produced or used:

- Mini-SWE/SWE-bench style harness behavior.
- Raw request/response capture.
- Replay commands.
- Early prompt-control AB artifacts.
- `docs/prompt-control-ab-tests.md`.
- `docs/personas-from-prompt-control-research.md`.

The prompt-control evidence base eventually reached 162 experiments. The
completion audit says the research objective was satisfied for artifact
handling, validation, scope control, implementation discipline, review gates,
command shape, stop conditions, repair behavior, and uncertainty handling.

Important boundary:

- That evidence base explicitly says it is not a full benchmark-performance
  claim.
- It also records that long noisy histories require short phases or latest
  message control footers; static persona headers are not enough.

### 2026-06-03: Flipt Kubernetes task analysis and Codex comparison

The June 3 analysis centered on the SWE-bench Pro Flipt Kubernetes auth task.
The repo contains several reports comparing Codex, old Pragma, and new v2
Pragma trajectories.

The Codex baseline failed because:

- it implemented Kubernetes config defaults unconditionally;
- it changed expected config tests without updating fixtures;
- it could not run the relevant Go tests and missed `TestLoad` failures;
- it manually patched generated code.

Old Pragma was better at narrowing scope through a contract boundary, but it
also failed:

- one representative run narrowed too aggressively and still missed evaluator
  defaulting behavior;
- a stricter run overreacted to generated-file correctness, got pulled into
  generator/toolchain churn, and still failed config expectations.

New v2 improved:

- surface/evidence/patch/checklist artifacts;
- artifact precedence over original task prose;
- validation status authority;
- generated producer evidence checks.

New v2 also introduced a new failure:

- too much local item authority;
- reviewers did not consistently merge already-satisfied future items;
- checklist repair loops turned into repeated concrete changed-file evidence
  demands.

The key finding here is that trajectory changed because of prompt and FSM
structure, but passing the evaluator still required better acceptance retention
and validation coverage.

### 2026-06-05 to 2026-06-07: architecture cleanup and web UI detour

Several sessions addressed broad codebase maintainability:

- repeated or duplicate issue reports;
- stale tests;
- codebase architecture docs;
- web UI spec/planning;
- JSON:API API shape;
- deletion of web UI HTML/CSS/JS while preserving APIs.

This phase produced docs such as `docs/codebase-architecture.md`,
`docs/clean-architecture-enforcement-doctrine.md`, and web UI planning docs.

Important learning:

- The user repeatedly pushed against adding tests when not asked.
- Future Pragma work should keep verification proportional and avoid test churn
  unless explicitly needed.
- The web UI work was mostly orthogonal to the SWE-bench orchestration goal and
  should not be mixed into the benchmark reliability narrative.

### 2026-06-08: normal chat loop, session inspection, and slash command behavior

This cluster investigated:

- normal chat getting stuck after a trivial prompt;
- turn-by-turn investigation of Pragma sessions;
- `/copy` clipboard behavior.

Touched areas included `internal/query/loop.go`, `internal/query/miniswe_loop.go`,
session storage, provider capture, CLI, TUI, and slash commands.

This established that Pragma needed better introspection into its own turns and
runtime events. Later `inspect`, raw timeline, and phase export tooling became
important because of this need.

### 2026-06-08 to 2026-06-10: transition-scoped handoff runtime

The transition-scoped handoff plan moved prompt handoff ownership from
destination-state `artifacts.inputs` to the actual transition edge that led to
the destination.

The plan's core rule:

- the transition that was actually taken is the only owner of prompt handoff for
  the destination state;
- state outputs remain state-owned output contracts;
- missing required transition handoff artifacts fail before model invocation.

This was a strong abstraction. It addressed stale/wrong branch artifacts by
making ownership declarative in the FSM edge, not inferred from state names.

Remaining limitation:

- This solved one handoff ownership class. It did not solve acceptance truth,
  runtime evidence ownership, or full-trajectory quality by itself.

### 2026-06-11 to 2026-06-12: SWE-bench Pro FSM/persona goal prompts and ledgers

The next goal was to build a stronger persona/FSM for the Flipt task.

Important artifacts:

- `docs/swe-bench-pro-orchestration-ab-goal-prompt.md`
- `docs/swe-bench-pro-orchestration-ab-ledger.md`
- `docs/swe-bench-pro-orchestration-fsm-persona-goal-prompt.md`
- `docs/swe-bench-pro-orchestration-fsm-persona-ledger.md`

The goal prompt explicitly required:

- raw payload replay and direct API AB tests;
- no implementation rush;
- no edits before evidence unless approved;
- acceptance preservation across mapper, theory keeper, planner, worker,
  validator, reviewer, and final reviewer;
- payload-level proof before promoting prompt/FSM changes.

Promising mechanism:

- make acceptance IDs explicit and persistent;
- require validators and reviewers to validate acceptance IDs, not worker
  claims;
- reject final approval if blocking acceptance remains unvalidated.

User pushback changed the boundary:

- a frozen `acceptance-map.json` for one task is not acceptable as a generic
  solution;
- task-specific YAML, persona wording, command names, repo paths, and benchmark
  fixtures count as hardcoding if they leak into production behavior.

### 2026-06-13 to 2026-06-14: hardcoding audit and cleanup

The hardcoding cleanup sessions were driven by a clear rule: no hardcoding in
code, YAML, or prompts that makes Pragma task/persona/orchestration-specific.

The ledger records several cleaned classes:

- concrete item-field ownership removed from runtime;
- checklist-loop-only personas moved to examples;
- task-specific strings and benchmark-stack references scanned out of
  production runtime/orchestration/persona files;
- legacy production-looking orchestration/persona assets retired or moved.

Evidence recorded in the ledger included visualization passes and scans for
Flipt/Kubernetes/task/model/provider strings. The scans did not find the prior
task leakage in the checked production surfaces.

Important correction:

- Hardcoding cannot be moved from Go into YAML or persona prompts. That is the
  same failure in a different layer.
- The acceptable direction is declaring generic contracts and ownership
  semantics, then proving them with raw replay and full-run evidence.

### 2026-06-14: provider blockers, runner capability context, and seeded artifacts

Fresh full runs after cleanup hit several issues:

- provider 502 loops on well-formed targeted-validator requests;
- a later evaluator failure where generated symbols were missing despite
  preflight capability evidence;
- persona/environment artifacts converted local PATH probe misses into durable
  missing-tool blockers.

The accepted abstraction was not "detect this command" or "hardcode this tool."
It was:

- preserve runner capability evidence separately from local probes;
- record runner/local disagreements as capability contradictions;
- seed generic runner/process environment evidence into declared artifacts;
- avoid editing the benchmark harness.

This was implemented generically through seeded artifacts and environment
context, according to the ledger.

Remaining limitation:

- These changes needed fresh full-run integration. They were supported by
  focused replay and compile/visualization checks, but not by a final passing
  SWE-bench trajectory.

### 2026-06-14 to 2026-06-17: single-owner evidence adjudication and runtime evidence ownership

The most recent major run was:

`20260616T173446Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446-single-owner-evidence-adjudication-lilac-minimax`

Exported investigation:

- captured turns: 201;
- completion rejection turns: 93;
- the active state stayed `swe_single_engineer` after the early run and never
  reached evidence adjudicator or reviewer;
- the engineer wrote `/tmp/pragma/single-owner/engineer-command-evidence.jsonl`
  manually;
- runtime integrity validation expected runtime-captured fields such as command
  preview, hashes, return code, timeout, submitted flag, sentinel, and report
  status;
- the runtime kept rejecting completion because the runtime-owned evidence was
  invalid or incomplete.

RCA:

- The immediate issue was an artifact ownership conflict.
- The model was presented with required output artifacts in a way that made a
  runtime-owned evidence file look like something it should write or fix.
- The rejection message reinforced the same conflict by telling the model to fix
  an output artifact instead of separating runtime evidence from model outputs.

AB test result:

- current/control failed;
- persona prompt-only ownership warning failed;
- completion-contract-only ownership split failed;
- owner-aware rejection only fixed the rejection checkpoint but did not cover
  the pre-loop checkpoint;
- full model-visible ownership contract passed at both tested checkpoints.

What this proves:

- For the observed evidence-file loop, full model-visible ownership semantics
  plus owner-aware rejection changes the model's next action.

What this does not prove:

- It does not prove the full SWE-bench task will complete.
- It does not solve the secondary task-quality problem where the engineer was
  willing to submit while runtime validation/error handling was still pending or
  framed as follow-up work.

## Session corpus

This table is intentionally compact. It records the exact session ids, dates,
turn counts, and dominant work. Full raw logs remain under `~/.codex/sessions`.

| Date | Turns | Session id | Dominant work |
| --- | ---: | --- | --- |
| 2026-05-08 | 10 | `019e06ac-c79d-7723-9a4d-178f08ca7f07` | File-edit/tool crash investigation while Pragma edited Pragma. |
| 2026-05-09 | 18 | `019e0b2a-a492-7540-855f-63fe16dfbcfe` | Context-limit failures and query loop behavior. |
| 2026-05-09 | 7 | `019e0d15-06f4-71f2-9602-03799624d7f3` | Model says it will act but stalls without tools; provider/query debugging. |
| 2026-05-11 | 77 | `019e14f7-409a-7940-94ad-f1809407a6f3` | Conversation presentation and handoff experiment. |
| 2026-05-17 | 72 | `019e342b-1ba7-73c0-a15a-3e7524191ec7` | Codebase survey, MCP/JetBrains/tooling work. |
| 2026-05-19 | 7 | `019e3e91-bdfd-7dd0-babd-c9fc16d11db9` | Remove Claude-specific project references. |
| 2026-05-19 | 10 | `019e3ecb-8196-7080-88bc-e41fb4d71b14` | File edit and handoff failure work. |
| 2026-05-19 | 8 | `019e3f12-7237-7953-b693-7f7663aa4525` | Last-agent-message hallucination and replay/token checks. |
| 2026-05-19 | 87 | `019e3f58-bd81-7eb3-9451-9511b1625a72` | Toolsets, MCP manager, README and runtime integration. |
| 2026-05-19 | 1 | `019e40e2-7b9a-7e31-afac-d4ae2d3e272d` | Project state question. |
| 2026-05-20 | 46 | `019e4375-460a-7d32-9d16-6010b0769da5` | Goal mode and idea-toolset editing experiment. |
| 2026-05-28 | 407 | `019e6c91-2028-75a1-b881-4b50daac47a4` | First major SWE-bench/Mini-SWE benchmark investigation. |
| 2026-05-31 | 743 | `019e7d67-52a0-7d32-b009-c793e98fd2ae` | Major prompt-control, raw replay, benchmark, persona research session. |
| 2026-06-03 | 279 | `019e8c64-b786-7671-ac57-c6b4e13ce77d` | Flipt Kubernetes SWE-bench Pro RCA and architecture audit. |
| 2026-06-04 | 10 | `019e9321-6b8a-7c70-98bd-cb152407c2d7` | Raw HTTP replay failure and inspect/readability improvements. |
| 2026-06-05 | 28 | `019e9867-834f-7142-873c-6bbf7bd7dc9c` | Repeated architecture issue quality and deduplication concern. |
| 2026-06-05 | 10 | `019e9896-7f9f-7a90-89eb-57c78eb4a396` | Remove stale tests; no-test preference reinforced. |
| 2026-06-06 | 55 | `019e9adf-6952-7011-9f8f-e702b6135d3d` | Source-backed architecture cleanup and issue stabilization. |
| 2026-06-06 | 2 | `019e9b02-c3e1-7db2-ac0a-ff3d6290847f` | Technical project description for UI/UX team. |
| 2026-06-06 | 2 | `019e9b18-29a3-76e0-a5e2-052c6c3ea46c` | Commit of web/backend gap artifact. |
| 2026-06-06 | 4 | `019e9b2e-9318-7740-929e-4a9d80a43837` | Clean-code doctrine and maintainability goal prompt. |
| 2026-06-06 | 22 | `019e9b68-84f9-7a22-8182-d7cb37c9b945` | Issue-order confusion and continued cleanup. |
| 2026-06-06 | 1 | `019e9c8a-e395-7730-ae9a-200913b35d2c` | Browser/UI inspection work. |
| 2026-06-06 | 5 | `019e9c97-9976-7901-a5c8-d0d61bd0030d` | Delete Pragma web UI static assets while preserving APIs. |
| 2026-06-06 | 4 | `019e9c9b-c1c0-7061-9782-d63f95955666` | New web UI subproject planning. |
| 2026-06-06 | 35 | `019e9ca1-de9c-7340-ab15-b9380bf3a3ae` | JSON:API shape and web API/UI integration. |
| 2026-06-07 | 1 | `019ea02d-9b57-7783-b25b-b37eafdd430c` | UX goal/spec review with screenshot. |
| 2026-06-07 | 14 | `019ea095-f57a-7512-9555-ec40afb4be12` | Web UI changes constrained by clean-code doctrine. |
| 2026-06-07 | 9 | `019ea0b7-05c6-7ba3-b8c3-0efcf1bc69bf` | Delete tests after explicit no-test correction. |
| 2026-06-07 | 1 | `019ea0d3-dc35-7552-abf4-573bb1c8676f` | Web UI goal context. |
| 2026-06-07 | 2 | `019ea0d9-d768-74f3-a850-a49d47175641` | Persona/state-machine management spec. |
| 2026-06-07 | 2 | `019ea0e5-5290-70a2-9a22-c9e84a043e57` | Web UI implementation bridge/spec completeness. |
| 2026-06-07 | 16 | `019ea109-f443-7c62-b5ef-a5b2dae7f2f6` | Full browser/API surface deletion and verification. |
| 2026-06-08 | 27 | `019ea533-73ac-7f03-a9fe-2949d64aa6b2` | Normal chat loop getting stuck; bash-block behavior RCA. |
| 2026-06-08 | 6 | `019ea586-9c5f-76a0-966b-4c5331081a66` | Turn-by-turn investigation of last Pragma conversation. |
| 2026-06-08 | 7 | `019ea597-7c9e-7293-a01c-c53ef333fd74` | `/copy` clipboard command behavior. |
| 2026-06-08 | 653 | `019ea5af-5eb5-76b2-8d11-a1d709593c33` | Transition-scoped handoff runtime plan and implementation direction. |
| 2026-06-10 | 2 | `019eb1a3-4328-7b72-95dd-a115a678d4e7` | Transition-scoped handoff implementation goal prompt. |
| 2026-06-10 | 48 | `019eb1b5-dc62-7f01-99f4-ca4b84f4c266` | Commit/run from scratch; inspect phases and benchmark run investigation. |
| 2026-06-11 | 10 | `019eb4f7-4567-70e2-9139-81be87afde77` | Codebase architecture documentation. |
| 2026-06-11 | 24 | `019eb507-aa6c-7782-ae83-3a51b766d46b` | Query loop entry point cleanup. |
| 2026-06-11 | 5 | `019eb554-d0e9-7810-9f0a-8a9fa073018a` | `internal/sysprompt` liveness investigation. |
| 2026-06-11 | 7 | `019eb563-de97-7241-aca7-0ca106c0760f` | System completion loop API differences. |
| 2026-06-11 | 8 | `019eb585-c73a-7540-bc02-c72c4546d235` | `miniswe_loop.go` value-boundary restructure planning. |
| 2026-06-11 | 5 | `019eb594-3409-7f52-a557-eb270e57320a` | Duplicate message-construction methods. |
| 2026-06-11 | 190 | `019eb59d-e7b7-74f2-be9f-c6190c697e33` | SWE-bench Pro AB goal prompt, raw replay, orchestration/persona ledger. |
| 2026-06-11 | 3 | `019eb5f8-6de9-70c0-839e-4d2df96793a8` | CLI visualization for orchestration FSM/persona. |
| 2026-06-12 | 1 | `019ebb67-5d18-7a03-bdfa-07f732c03814` | SWE-bench orchestration AB goal prompt artifact. |
| 2026-06-12 | 8 | `019ebb6b-0a22-7ae1-b95a-f6621d64315d` | Summarize and refine benchmark orchestration goal. |
| 2026-06-12 | 25 | `019ebba3-eecd-7223-a106-0068652be3c8` | Goal statement, exact AB methodology, command blocks, ledgers. |
| 2026-06-13 | 43 | `019ebf5e-20e4-7153-82f6-b1a9451e68ed` | Hardcoding audit across runtime, YAML, prompts, and production surfaces. |
| 2026-06-13 | 4 | `019ec106-909f-7411-a1da-dddcf167ef66` | Check previous Codex goal logs and reliability of that goal loop. |
| 2026-06-14 | 198 | `019ec50b-b5f9-73b1-9fd8-31d80648cd7c` | Single-owner evidence-adjudication run, turn-by-turn RCA, ownership AB tests. |
| 2026-06-17 | 1 | `019ed48a-de69-7ea3-af91-8a152f17746f` | Continuation of this retrospective/report goal; updated the inventory and report against the full exact-cwd corpus. |

## What worked

Raw HTTP capture and replay worked. It made prompt behavior inspectable and
allowed exact failed turns to be replayed with controlled variants.

The prompt-control AB ledger worked for local behavior. It avoided pure prompt
vibes by retaining only variants with replay evidence and explicitly marking
static YAML failures as orchestration-only.

Transition-scoped handoff was a good abstraction. It moved handoff ownership to
the FSM edge that actually fired and stopped treating destination states as the
source of every input.

Visualization and inspect tooling worked. The later questions about phases,
turns, and "what happened" could be answered from exports instead of guessing.

The hardcoding audit moved the system in the right direction. It removed or
moved production-looking task-specific surfaces and reinforced the rule that
hardcoding in YAML or persona prompts is still hardcoding.

The runtime evidence ownership AB test produced actionable evidence. It proved
that a full model-visible ownership contract changes the model behavior at the
observed pre-loop and rejection checkpoints.

## What failed or kept recurring

Persona headers were overtrusted. Many failures happened after the persona
prompt already contained a rule. Later model-visible runtime text, handoff
artifacts, or rejection messages could still steer the model into the wrong
action.

The FSM often failed to get the right persona to the right point. The latest
single-owner run never reached the adjudicator/reviewer. That is not a reviewer
prompt problem; it is a state-transition/completion-contract problem.

"Maintain truth" was treated too abstractly. Roles like theory keeper or
reviewer do not preserve truth by existing. They need specific state,
artifacts, authority, and rejection semantics that force the next transition.

Hardcoding was repeatedly tempting. Proposed fixes drifted toward command
lists, path names, specific benchmark facts, provider names, or state-specific
patches. The user correctly rejected those as the same failure class.

One-turn AB evidence was repeatedly overinterpreted. A direct replay can prove
that a prompt/runtime change alters the next turn. It cannot prove full
trajectory completion.

Acceptance retention remained unsolved. The Flipt task exposed that a system
can do many reasonable things and still miss the evaluator's hidden acceptance
shape, especially around config defaults and fixtures.

Runtime evidence and model-authored outputs were mixed. This produced the
latest evidence-file loop, where the model fabricated or edited a file whose
truth can only come from executed commands captured by runtime.

The current worktree is not a clean verified baseline. It includes broad
uncommitted changes across multiple subsystems. Any future run must first make
the baseline explicit.

## Things tried again and again

This is the important anti-forgetting section. The problem was not that nothing
worked. The problem was that several partially useful moves were repeated after
their limits were already visible.

| Repeated move | Where it showed up | What it bought us | Why repeating it became wasteful |
| --- | --- | --- | --- |
| Add another persona rule or warning | prompt-control experiments, SWE persona revisions, runtime evidence ownership AB | Helped identify which words could steer a single checkpoint when placed in the right packet. | Static persona text repeatedly lost to later runtime messages, handoff artifacts, rejection text, and accumulated history. The latest evidence file AB showed persona-only ownership warnings still failed. |
| Add another reviewer/adjudicator role | v2 trajectory, SWE-bench Pro FSM/persona work, single-owner evidence adjudication | Made the desired responsibility split clearer: mapper, planner, validator, reviewer, final reviewer, adjudicator. | Roles do nothing if the FSM never reaches them or if their input artifacts already lost the truth. The latest run stayed in `swe_single_engineer` for 199 of 201 captured turns. |
| Convert missing truth into a new artifact | prompt-control ledgers, acceptance map, handoff audit, plan audit, worker reports, command evidence | This was useful when the artifact had a real owner and a validation contract. Acceptance IDs and handoff hashes were real progress. | It became harmful when runtime-owned evidence was presented as a model deliverable. The model then fabricated `/tmp/pragma/single-owner/engineer-command-evidence.jsonl` instead of running commands. |
| Run another full benchmark trajectory before a focused AB checkpoint changed | Flipt Kubernetes runs and the later single-owner run | Produced the raw captures that exposed real failure classes. | Once a failed turn was known, repeating full runs without first changing that turn behavior just burned provider/model time and created more logs to re-debug. |
| Treat "more tests" as validation progress | older cleanup/stabilization work and the failed Flipt trajectory | Focused checks were useful when tied to exact acceptance. | Self-authored tests and package-local checks displaced evaluator-facing config-load acceptance. The failed run passed runtime auth tests while evaluator config tests still failed. |
| Use broad architecture audits to find concrete bugs | June 5-6 architecture and issue-ledger work | Produced useful doctrine and found real ownership/lifecycle problems. | Re-running broad audits produced duplicate or stale findings unless each issue was traced through current call sites and closed with source-backed evidence. |
| Move hardcoding to a different layer | Go runtime fixes, YAML shell policies, persona prompt denylists, command/path examples | Some targeted constraints blocked a symptom in one captured case. | The user correctly treated Go/YAML/persona denylists as the same hardcoding failure. A fix that only works for Flipt, Kubernetes, a command name, a repo path, or a persona name is not the Pragma abstraction. |
| Pretty-print or inspect the logs after the fact | raw HTTP replay, `inspect raw-http`, `inspect phases`, turn-by-turn exports | This was extremely useful infrastructure and should stay. | It does not itself improve agent behavior. After the RCA is known, the next step must change the model-visible contract and replay the exact failed turn. |
| Rebuild or delete web UI surfaces while benchmark reliability was still unresolved | June 6-7 web UI planning, implementation, deletion, bridge docs | Clarified product/API boundaries and produced useful historical docs. | It was mostly orthogonal to SWE-bench reliability. Mixing UI work into the benchmark narrative made the overall effort feel larger without moving the hard task closer to passing. |

The most expensive repeated mistake was treating symptoms as if they were root
causes: "the model searched too broadly", "the model forgot acceptance", "the
model wrote fake evidence", "the reviewer approved too early". Each symptom
tempted a new slogan, guardrail, or persona. The useful pattern only emerged
when the exact failed turn payload was inspected and the model-visible contract
was changed at the decision point.

## Useful work versus wasteful work

Useful work:

- Raw capture, raw replay, replay audit, turn dump, `inspect raw-http`, and
  `inspect phases`. These made failures reproducible instead of anecdotal.
- Prompt-control AB experiments, especially the negative results. They proved
  that small late reminders and negative-only warnings are weak.
- Acceptance-map and per-acceptance validation design. This addressed the real
  Flipt failure class: evaluator-facing acceptance decayed into prose and then
  disappeared from validation.
- Transition-scoped handoff. This was a clean generic ownership move from
  destination-state inference to actual transition-edge ownership.
- Handoff byte/hash recording. This made artifact handoff integrity observable.
- Hardcoding audits. These prevented one-off Flipt/Kubernetes fixes from being
  mistaken for Pragma runtime design.
- Runtime evidence ownership AB. This is the latest actionable evidence: full
  visible ownership semantics changed behavior at both the pre-loop and
  rejection checkpoints.

Wasteful work:

- Treating persona count as progress. More roles did not help when state
  transition contracts and artifact ownership were wrong.
- Treating static prompt text as durable control. The prompt-control ledger
  repeatedly showed that placement and latest context matter more than a slogan
  in a persona header.
- Re-running broad trajectories from ambiguous or dirty baselines. The result
  becomes hard to attribute, so the next RCA starts by rediscovering which
  baseline was used.
- Using command names, file paths, state names, provider names, benchmark facts,
  or regex denylists as "fixes". That moved hardcoding around instead of
  deleting it.
- Letting self-authored tests replace evaluator acceptance. Passing tests only
  mattered when they covered the original acceptance surface.
- Re-investigating known loops from scratch. The recurring loop was not lack of
  information; it was failure to carry forward the already-proven boundary:
  inspect exact payload, identify contract contradiction, AB the generic fix,
  then run the full trajectory.

## Current state

The most recent hard evidence is:

- the single-owner full run did not reach reviewer/adjudicator;
- it looped in `swe_single_engineer`;
- completion was rejected 93 times;
- the immediate blocker was runtime evidence ownership;
- a focused AB test supports full visible ownership semantics and owner-aware
  rejection for that blocker;
- the same AB report explicitly says it does not prove full task completion.

The current repo state is not clean. Tracked changes include:

- `cmd/pragma/orchestration.go`
- `internal/app/state.go`
- `internal/cli/run.go`
- `internal/observe/event_catalog.go`
- `internal/orchestration/*`
- `internal/provider/lilac/provider.go`
- `internal/provider/rawcapture/rawcapture.go`
- `internal/query/*`
- `internal/slash/command.go`
- multiple orchestration YAML files
- many `personas-research-v2/*.yaml` files
- deletions under legacy `personas/`

Untracked files include:

- SWE-bench orchestration goal/ledger docs
- `examples/`
- `orchestrations/task-evidence-item-loop.yaml`
- new SWE persona YAML files
- `pragma-goal.md`

This means the next step should not be another speculative run from an
ambiguous tree. First decide which parts of the current dirty worktree are the
intended baseline.

## What should be next

### 1. Freeze and reconcile the worktree

Before more benchmark runs:

- inspect the current diff as a baseline candidate;
- separate completed generic changes from interrupted experimental changes;
- either commit the intended baseline or explicitly revert/stash abandoned
  slices with user approval;
- do not run another "proof" trajectory from an unlabelled mixed tree.

This is not bureaucracy. Without a named baseline, every run result becomes
ambiguous.

### 2. Complete the generic runtime evidence ownership slice

The next implementation slice should be exactly the abstraction supported by
the AB evidence:

- artifact declarations distinguish model-authored outputs from
  runtime-authored evidence;
- model-visible artifact contract rendering lists model-authored deliverables
  separately from runtime-owned required evidence;
- completion checks may still require runtime-owned evidence, but the model is
  not told to write it;
- rejection text is owner-aware;
- runtime evidence invalidity tells the model to run real commands or revise
  model-authored status/report claims, not to fabricate the runtime evidence
  file.

Forbidden for this slice:

- no task names;
- no repo path names from the Flipt task;
- no command availability tables;
- no command allow/deny lists;
- no state-name special cases unless they correspond to declared generic
  ownership data;
- no benchmark harness edits to make Pragma pass.

### 3. Re-run the exact AB checkpoints, then one adjacent turn

Minimum proof after implementation:

- replay checkpoint `000078`;
- replay checkpoint `000079`;
- replay at least one follow-up turn after a corrected rejection path to ensure
  the model does not route into another artificial evidence edit;
- audit with `pragma replay raw-http audit --require-responses`;
- preserve payloads and results under `.pragma/prompt-ab/`.

This still proves only the blocker slice.

### 4. Run the full single-owner trajectory only after the AB slice passes

After the exact failed-turn behavior changes, run the full SWE-bench Pro task
again with the same provider/model/orchestration/persona baseline.

Required evidence:

- raw HTTP capture preserved;
- phase export preserved;
- turn-by-turn investigation generated if the run fails or loops;
- evaluator result preserved if reached;
- no claim of "usable" unless the run either passes the evaluator or fails on a
  clearly external/provider condition proven by exact replay.

### 5. Treat task-quality completion as the next separate RCA if it appears

The exported investigation already notes a secondary issue: before the evidence
loop, the engineer was willing to submit while runtime validation/error handling
remained pending or follow-up.

Do not hide this behind the evidence ownership fix. If the next full run reaches
reviewer/adjudicator and still approves incomplete work, that is a separate
acceptance-quality failure.

Likely abstraction boundary for that future slice:

- reviewers/adjudicators compare model-authored completion reports against the
  original task acceptance and current diff/validation evidence;
- missing acceptance coverage blocks transition;
- the fix must stay generic and artifact-contract based, not Flipt-specific.

### 6. Keep benchmark harness out of scope unless evidence says otherwise

The user already clarified that the benchmark harness is not in scope for this
goal. Use it only to run and evaluate. If artifacts are missing from run dirs,
fix generic Pragma artifact preservation, not benchmark-task behavior.

### 7. Stop treating guardrails as RCA

The recurring process error in these sessions was jumping from a symptom to a
guardrail:

- max self-loop counts;
- command/path restrictions;
- state-specific patches;
- persona prompt slogans;
- provider/task-specific probes.

Those are not root-cause fixes unless the evidence proves the abstraction. The
acceptable workflow is:

1. export or locate the exact turn payloads;
2. identify the model-visible contradiction or missing contract;
3. state the abstraction boundary;
4. AB test the boundary against the failed turn and an adjacent turn;
5. implement only the generic boundary;
6. run the full trajectory after local turn behavior changes.

### 8. Keep a small "already tried" ledger before new experiments

Before the next Pragma benchmark or orchestration experiment, update a tiny
ledger entry with:

- the exact failed run or turn payload being addressed;
- the hypothesis;
- the generic abstraction boundary;
- the old attempts that are explicitly not being repeated;
- the exact AB checkpoint that must change before a full run is allowed.

This should live next to the active experiment artifact, not only in chat. The
minimum entries already known are:

- do not repeat persona-only warnings for runtime-owned evidence;
- do not repeat command/path/provider/state-name denylists as fixes;
- do not repeat full benchmark runs from an unlabeled dirty worktree;
- do not repeat self-authored test additions as proof of evaluator acceptance;
- do not repeat "add a reviewer" unless the FSM has already proven it reaches
  that reviewer with the right artifacts.

## Bottom line

Pragma is not yet proven usable for complex SWE-bench Pro tasks. The work so far
has produced valuable infrastructure and evidence:

- raw capture/replay;
- prompt-control AB methodology;
- transition-scoped handoff;
- orchestration visualization and phase export;
- hardcoding cleanup discipline;
- runtime evidence ownership AB proof for the latest loop.

But the system has not yet produced a clean passing full trajectory for the
target hard task. The correct next move is not another persona slogan, not a
hardcoded runtime guardrail, and not another benchmark run from an ambiguous
worktree. The correct next move is to reconcile the worktree, finish the generic
runtime evidence ownership slice supported by the latest AB tests, replay the
failed checkpoints, and only then run the full trajectory.

## Evidence paths to keep close

- `docs/prompt-control-ab-tests.md`
- `docs/personas-from-prompt-control-research.md`
- `docs/codex-vs-pragma-flipt-kubernetes-trajectories-20260603.md`
- `docs/swe-bench-pro-orchestration-fsm-persona-ledger.md`
- `.pragma/prompt-ab/20260617T000000Z-runtime-evidence-ownership-ab/report.md`
- `.pragma/prompt-ab/20260617T000000Z-runtime-evidence-ownership-ab/results.md`
- `.pragma/exports/20260616T173446Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446-single-owner-evidence-adjudication-lilac-minimax-turn-by-turn/turn-by-turn-investigation.md`
- `.pragma/exports/20260616T173446Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446-single-owner-evidence-adjudication-lilac-minimax-turn-by-turn/phases.md`

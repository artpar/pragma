# Prompt Control A/B Tests

This document is the ongoing log for phrase-level prompt experiments. The goal
is to learn which prompt phrases, positions, ordering, and context shapes
reliably produce specific agent behavior before designing or rewriting personas.

## Objective

Find prompt controls that survive increasing context pressure:

1. Minimal synthetic prompt.
2. Synthetic prompt plus realistic benchmark task prose.
3. Real captured request payload from the failed Flipt Kubernetes run.
4. Later real failure turns after history and artifacts dominate behavior.

Do not treat a phrase as useful because it sounds good. Treat it as useful only
when replayed against the real model and the resulting action changes in the
intended direction.

## Current Target Behaviors

- First inspection should use the task-named source surface before broad repo
  search.
- Config/defaulting/fixture tasks should inspect config loader, tests, testdata,
  schema, and defaults before runtime auth services, protobuf services,
  middleware, generated files, or dependencies.
- A final prosecutor must not approve when visible validation output contains
  failures.
- A repair checklist must not reopen broad product scope from original task
  prose after a final block.
- Repeated failed symbol searches must stop instead of producing no-op grep
  loops.

## Evidence Sources

- Failed run directory:
  `.pragma/swe-bench-pro/20260602T145119Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Raw request captures:
  `raw-http-pragma/`
- First-principles batch:
  `.pragma/prompt-ab/first-principles-20260602T160000Z`
- Build-up batch:
  `.pragma/prompt-ab/build-up-20260602T160056Z`

The model tested so far is `minimaxai/minimax-m2.7` through Lilac, temperature
0, OpenAI-compatible chat completions.

## Replay Harness Notes

2026-06-03:

- Initial retake attempts for validation-runner indexed failure-package
  extraction and item-worker validation-only wrapper shape were inconclusive
  because Lilac chat completion POSTs did not return headers. Later bounded
  replays succeeded and are documented in Experiments 148 and 149.
- `pragma replay raw-http` now has a `--timeout` duration flag for fail-fast
  replay probes. The default remains 10 minutes, matching the previous
  hard-coded timeout.
- `pragma replay raw-http audit <dir> --require-responses` can be used before
  documenting a replay batch to ensure every request case has usable
  `response.raw` evidence.

## Experiment 1: Minimal First-Action Control

Question: can a prompt reliably control the first command in a tiny context?

Expected first command:

```bash
sed -n '1,220p' internal/config/authentication.go
```

Results:

| Case | Prompt Shape | Observed First Action | Result |
| --- | --- | --- | --- |
| `p00_open` | Open task, no steering | Broad `find` plus `ls` plus fallback `cat` | Failed |
| `p01_exact_top` | Exact command contract before task | Exact `sed` command | Passed |
| `p02_exact_late` | Exact command contract after task | Exact `sed` command | Passed |
| `p03_ordered_allowlist` | Ordered list, first applicable command | Exact `sed` command | Passed |
| `p04_negative_only` | Only says what not to inspect | Broad `find` | Failed |
| `p05_positive_negative` | Good/bad examples, no exact command | Direct config `cat` plus fallback `find` | Partial |
| `p06_role_before_task` | Role plus exact first command | Exact `sed` command | Passed |
| `p07_conflict_exact_top` | Exact command plus conflicting generic workflow | Exact `sed` command | Passed |

Learning:

- Negative constraints alone are weak.
- Good/bad examples improve direction but do not prevent fallback/broad search.
- Exact first-action contracts are strong in small context.
- Ordered allowlists are strong in small context.
- Exact first-action contracts survive generic workflow conflict in small
  context.

## Experiment 2: Add Realistic Task Prose

Question: do the same controls survive the full Flipt task prose?

Results:

| Case | Context | Prompt Shape | Observed First Action | Result |
| --- | --- | --- | --- | --- |
| `m10_full_task_open` | Synthetic role plus full task text | No steering | Broad authentication grep | Failed |
| `m11_full_task_exact` | Synthetic role plus full task text | Exact first-action contract | Exact `sed` command | Passed |
| `m12_full_task_ordered` | Synthetic role plus full task text | Ordered allowlist | Exact `sed` command | Passed |
| `m13_full_task_direct_nofallback` | Synthetic role plus full task text | Direct-read, no fallback, no broad search | Direct `/app/internal/config/authentication.go` `cat` | Partial |

Learning:

- Full task prose alone pushes the model toward broad authentication search.
- Exact first-action contracts still survive full task prose.
- Ordered allowlists still survive full task prose.
- A behavioral phrase like "direct read, no fallback" changes behavior toward a
  direct config read, but not to the exact command. This is useful for steering
  category, not exact action.

## Experiment 3: Real Captured Turn-1 Payload

Question: do controls survive the actual contract analyst payload that failed?

Results:

| Case | Mutation | Observed First Action | Result |
| --- | --- | --- | --- |
| Baseline captured turn 1 | None | Broad Go-file grep over authentication/config | Failed |
| `r20_real_exact_top` | Exact first-action contract prepended | Exact `sed` command | Passed |
| `r21_real_ordered_top` | Ordered allowlist prepended | Chose third allowlist entry, `find internal/config` | Partial/Failed |
| `r22_real_replace_work_method` | Replaced numbered work method with direct first step | Direct `/app/internal/config/authentication.go` `sed` | Passed |

Learning:

- In a real persona payload, ordered allowlists are less stable than exact
  first-action contracts.
- Replacing the numbered work method is stronger than adding another rule,
  because the model follows the local procedural list.
- Prepended exact commands can override the old prompt, but this is too brittle
  for real personas unless the command is generated from the task/context.
- The phrase category that worked was not "be careful about config"; it was
  "first action must be a direct read of this source path" placed as either an
  exact first-action contract or inside the numbered work method.

## Experiment 3.5: Source Path Section Instead Of Exact Command

Question: can the prompt control first inspection without hardcoding the exact
shell command?

Results:

| Case | Context | Prompt Shape | Observed First Action | Result |
| --- | --- | --- | --- | --- |
| `q00_open` | Tiny prompt | `Task-named source path` section only | `cat internal/config/authentication.go` | Passed |
| `q01_direct_named_top` | Tiny prompt | Source path plus direct-read rule | `cat internal/config/authentication.go` | Passed |
| `q02_numbered_step` | Tiny prompt | Source path consumed by numbered step 1 | `cat internal/config/authentication.go` | Passed |
| `q03_examples` | Tiny prompt | Source path plus good/bad examples | `cat internal/config/authentication.go` | Passed |
| `q04_schema_output` | Tiny prompt | Explicit `TASK_NAMED_SOURCE_PATH` variable | `cat internal/config/authentication.go` | Passed |
| `q05_only_allowed_verbs` | Tiny prompt | Allowed verbs and allowed argument | `cat internal/config/authentication.go` | Passed |
| `q06_no_compound` | Tiny prompt | Single simple command, no compound/fallback | `cat internal/config/authentication.go` | Passed |
| `q07_late_direct` | Tiny prompt | Direct-read rule after task | `cat internal/config/authentication.go` | Passed |
| `q08_after_format` | Tiny prompt | Output-format constraints name path | `cat internal/config/authentication.go` | Passed |

Learning:

- The phrase `Task-named source path:` is a strong control primitive.
- Once the path is normalized into its own small section, even an open prompt
  chooses direct file reading.
- This is better than hardcoding an exact shell command because a persona can
  extract task variables and then consume them.

## Experiment 3.6: Source Path Section With Full And Real Context

Question: does `Task-named source path` survive full task prose and the real
captured persona payload?

Results:

| Case | Context | Prompt Shape | Observed First Action | Result |
| --- | --- | --- | --- | --- |
| `s10_full_open_with_source_section` | Full task prose | Source path section only | `cat /app/internal/config/authentication.go` | Passed |
| `s11_full_source_section_rule` | Full task prose | Source path plus direct-read rule | `cat /app/internal/config/authentication.go` | Passed |
| `s12_full_source_section_forbidden` | Full task prose | Source path plus forbidden search/runtime terms | `cat /app/internal/config/authentication.go` | Passed |
| `s20_real_source_section_top` | Real captured turn 1 | Source path section prepended | `cat /app/internal/config/authentication.go` | Passed |
| `s21_real_source_section_before_work` | Real captured turn 1 | Source path section before work method | `cat /app/internal/config/authentication.go` | Passed |
| `s22_real_work_step_source_path` | Real captured turn 1 | Numbered work method extracts source path | `cat /app/internal/config/authentication.go` | Passed |

Learning:

- `Task-named source path:` is currently more stable than ordered allowlists in
  real persona context.
- Position was not sensitive in this batch: top, before work method, and inside
  the work method all worked.
- The likely useful persona pattern is:
  1. Extract normalized task variables into named sections.
  2. Make numbered work steps consume those variables.
  3. Avoid starting from broad verbs like "find relevant files."

## Experiment 4: Failed Real-Payload Mutations

Before resetting to first principles, a broad mutation batch was run directly
against real failure turns. It mostly failed to steer behavior:

| Target | Mutation Type | Result |
| --- | --- | --- |
| Contract turn 1 | "config first" rule at top | Still broad search |
| Contract turn 1 | negative "do not inspect runtime until config" | Still broad file discovery |
| Contract turn 1 | same rule before task text | Still broad search |
| Checklist repair turn 254 | "read verdict first" | Still read contract scope first |
| Checklist repair turn 254 | "do not reopen original task" | Still read contract scope first |
| Final prosecutor turn 706 | "diff and validation first" | Still read patch plan first |
| Final prosecutor turn 751 | "if FAIL in history, block" | Still emitted completion sentinel |
| Repeated grep turn 302 | "loop breaker" | Still searched Kubernetes proto/generated symbols |
| Repeated grep turn 302 | "three strikes" | Still searched Kubernetes proto/generated symbols |

Learning:

- Small phrase additions do not beat an existing persona's numbered work method
  or accumulated local history.
- Once the model is deep in a trajectory, broad admonitions are nearly useless.
- Controls must be introduced at the point where behavior is selected:
  first-action contract, numbered work method, or artifact format.
- Later-turn repair requires context shaping, not just an extra sentence.

## Experiment 5: Final Prosecutor Blocks On Visible Failure

Question: in a tiny final-prosecutor prompt, does visible validation output with
`--- FAIL` make the model write a blocking verdict?

Visible failure snippet included:

```text
--- FAIL: TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)
--- FAIL: TestLoad/advanced_(YAML)
FAIL
FAIL go.flipt.io/flipt/internal/config 0.188s
```

Results:

| Case | Prompt Shape | Observed Action | Result |
| --- | --- | --- | --- |
| `f00_base` | Basic prosecutor prompt plus visible failure | Wrote `Decision: BLOCK` | Passed semantically |
| `f01_visible_fail_is_block` | Automatic block rule | Wrote detailed block verdict and sentinel | Passed semantically |
| `f02_step_order` | Numbered failure scan first | Wrote block verdict | Passed semantically |
| `f03_output_template_forced` | Forced block output shape | Wrote block verdict and sentinel | Passed semantically |
| `f04_boundary_exception` | Completion forbidden while failures visible | Wrote block verdict | Passed semantically |
| `f05_bad_approve` | Good/bad examples | Wrote block verdict and sentinel | Passed semantically |
| `f06_failure_summary_var` | Extracted failed-test variables | Wrote detailed block verdict and sentinel | Passed semantically |

Learning:

- If failure text is visible in the immediate prompt, blocking is easy.
- The previous real `turn-751` mutation failed because the prompt did not
  actually contain evaluator `FAIL` output. A phrase cannot make the model react
  to evidence that is not in context.
- Many semantically correct outputs also emitted the completion sentinel in the
  same shell script. That is acceptable only if the verdict file is already
  written with an event-readable marker.

## Experiment 5.1: Exact Verdict Marker Shape

Question: can the prompt force the two-line marker consumed by the FSM?

Required marker:

```text
Decision:
BLOCK
```

Results:

| Case | Prompt Shape | Observed Marker | Result |
| --- | --- | --- | --- |
| `v00_format_example` | "Marker must be exactly two lines" | Heredoc with two-line marker | Passed |
| `v01_literal_required` | Must contain exact substring, no one-line marker | `echo -e "Decision:\nBLOCK"` | Passed in file, not visible as literal response |
| `v02_shell_template` | Literal heredoc template | Heredoc with two-line marker | Passed |
| `v03_bad_good_marker` | Good/bad marker examples | Two-line marker but wrong response format/prose | Failed format |
| `v04_event_contract` | Abstract FSM event contract | Broad `find`/`ls` search | Failed |

Learning:

- Literal heredoc templates are the strongest way to control event-readable
  artifact format.
- Good/bad marker examples can produce the right marker while violating the
  required bash-only response format.
- Abstract runtime terms like "FSM event contract" are actively bad. They do
  not give the model an operational next action.
- For event-driven personas, the prompt should include the exact file template,
  not only a prose description of the event rule.

## Experiment 6: Repeated Search Loop Breaker

Question: can the model stop repeating a failed symbol search when recent
command history is explicit?

Synthetic recent history included three failed commands:

```bash
grep -n "service AuthenticationMethodKubernetes" /app/rpc/flipt/auth/auth.proto
```

Results:

| Case | Prompt Shape | Observed Action | Result |
| --- | --- | --- | --- |
| `l00_open` | Recent failures only | Searched broader `Kubernetes` in same proto | Partial |
| `l01_no_repeat` | Loop-breaker sentence | Listed directory and grepped `kubernetes` | Failed/partial |
| `l02_history_table` | Repeated failed search table | Listed directory and read proto head | Partial |
| `l03_allowed_list` | Ordered next actions | Read `auth.proto` via `sed` | Partial |
| `l04_exact_blocker` | Must write blocker report | Wrote blocker-like report to stdout, not artifact | Partial |
| `l05_bad_good` | Good/bad examples | Broad generator/config search | Partial |
| `l06_state_variable` | Explicit search-state variables | Read directory/proto with fallback | Partial |

Learning:

- Explicit repeated-command history usually prevents the exact same grep from
  repeating.
- It does not prevent adjacent searches such as grepping broader `Kubernetes`,
  listing the directory, or inspecting generated/generator surfaces.
- "Do not repeat" is not enough when the intended behavior is "stop and report
  blocker." The prompt must name a required artifact and a state transition.

## Experiment 6.1: Repeated Search To Blocker Artifact

Question: after repeated failed searches, what phrase makes the model write the
blocker artifact instead of continuing investigation?

Results:

| Case | Prompt Shape | Observed Action | Result |
| --- | --- | --- | --- |
| `b00_exact_template` | Exact blocker template | Ignored; searched all proto files | Failed |
| `b01_allowed_only_template` | Allowed command only, forbidden inspection | Ignored; grepped service names | Failed |
| `b02_state_to_artifact` | Search-state variables plus `required_artifact` | Wrote `/tmp/pragma/implementer-report.md` with Blocker | Passed |
| `b03_boundary` | Boundary says report exists after file created | Wrote report artifact, but weak blocker semantics | Partial |

Learning:

- A literal template alone is not enough if the model still thinks more
  investigation is productive.
- Allowed/forbidden command lists are weak for stopping a stuck investigation.
- The useful phrase shape is a state variable plus a decision rule:
  `repeated_no_new_information = true`, `repeat_count = 3`,
  `required_artifact = /tmp/pragma/implementer-report.md`, and
  `when repeated_no_new_information is true, write required_artifact with a
  Blocker section. Do not inspect more files.`
- Boundary language helps produce an artifact but may not preserve strong
  blocker semantics unless the artifact format is also explicit.

## Experiment 7: Checklist Repair Scope

Question: when a final verdict contains both concrete failing config tests and
tempting broad runtime prose, does the checklist planner reopen product scope?

Synthetic prompt contained:

- concrete failures for `authentication_kubernetes_defaults_when_enabled`
  YAML/ENV,
- concrete failures for `advanced` YAML/ENV,
- missing `./testdata/authentication/kubernetes.yml`,
- wrong advanced Kubernetes fields and cleanup,
- tempting broad text about runtime token validation and auth method server
  behavior.

Results:

| Case | Prompt Shape | Scope Result | Artifact Result |
| --- | --- | --- | --- |
| `k00_base` | Basic checklist prompt with bounded patch-plan summary | Narrow config/testdata/defaulting items | Failed: returned JSON in chat |
| `k01_concrete_only` | Concrete failed-validation lines only | Narrow config items | Failed: returned JSON in chat |
| `k02_blocker_variables` | Extracted failed tests, missing surfaces, forbidden scope | Narrow config items | Failed: returned JSON in chat |
| `k03_allowed_forbidden` | Allowed/forbidden topics | Narrow config items | Failed: returned JSON in chat |
| `k04_template_narrow` | Fixed three-item checklist template | Narrowest config items | Failed: returned JSON in chat |

Learning:

- In small context, a bounded patch-plan summary plus concrete failure text was
  enough to prevent broad runtime checklist items.
- Extracted `forbidden_scope` variables also worked, but were not necessary in
  this small prompt.
- The failure was artifact discipline: without explicit shell/file-output
  discipline, the model returned JSON directly instead of writing
  `/tmp/pragma/checklist.json`.
- Scope control and artifact control are separate behaviors. A persona needs
  both:
  - normalized repair inputs to control scope,
  - explicit shell/file deliverable instructions to control output mechanics.

## Experiment 8: Real Payload Control Replay

Question: do the strongest controls survive the real captured failure payloads?

Targets:

- turn 254: checklist planner starts repair pass by reading contract scope
  first.
- turn 260: checklist planner writes broad runtime/proto/server items.
- turn 302: item implementer repeats Kubernetes proto service searches.
- turn 706: final prosecutor starts by reading handoff artifacts.
- turn 751: final prosecutor emits completion despite later evaluator failure.

Results when controls were prepended to the phase-opening user message:

| Case | Target | Control | Observed Action | Result |
| --- | --- | --- | --- | --- |
| `c254_verdict_path_top` | turn 254 | Required input path = final verdict | Read final verdict first | Passed |
| `c254_workmethod_verdict_first` | turn 254 | Replaced work method | Listed `/tmp/pragma` | Partial |
| `c260_repair_inputs_top` | turn 260 | Repair inputs from verdict | Inspected runtime auth method dir | Failed |
| `c260_no_original_reopen_top` | turn 260 | Do not reopen original task | Inspected runtime auth method dir | Failed |
| `i302_state_required_artifact` | turn 302 | Search-state plus required artifact | Repeated service grep | Failed |
| `i302_no_repeat_top` | turn 302 | No-repeat rule | Switched to adjacent interface grep | Partial |
| `f706_evidence_paths_top` | turn 706 | Diff/validation first | Read patch plan first | Failed |
| `f706_workmethod_diff_first` | turn 706 | Replaced work method | Read patch plan first | Failed |
| `f751_visible_fail_status` | turn 751 | Injected failure status | Completion sentinel only | Failed |
| `f751_heredoc_block_template` | turn 751 | Exact block template | Completion sentinel only | Failed |

Message-structure finding:

- turn 254 and 706 had only 2 messages, so phase-opening controls were still
  close to the decision point.
- turn 260 had 14 messages.
- turn 302 had 80 messages.
- turn 751 had 92 messages.

Learning:

- Controls prepended to the phase-opening message degrade badly once the phase
  accumulates local history.
- One-time persona headers are not enough for long phases.
- If a phase runs many turns, control text must either be refreshed near the
  latest user message or the phase history must be summarized/cut before the
  next decision.

## Experiment 8.1: Latest-Message Control Replay

Question: do the same controls work if attached to the latest user message
instead of the stale phase-opening message?

Results:

| Case | Target | Position | Observed Action | Result |
| --- | --- | --- | --- | --- |
| `i302_latest_state_footer` | turn 302 | latest user footer | Wrote blocker report artifact | Passed |
| `i302_latest_state_prefix` | turn 302 | latest user prefix | Wrote blocker report artifact | Passed |
| `f751_latest_fail_footer` | turn 751 | latest user footer | Ran focused tests instead of block verdict | Partial/failed |
| `f751_latest_fail_prefix` | turn 751 | latest user prefix | Ran broad test failure search | Partial/failed |
| `c260_latest_scope_footer` | turn 260 | latest user footer | Inspected runtime auth method dir | Failed |
| `c260_latest_scope_prefix` | turn 260 | latest user prefix | Inspected runtime auth method dir | Failed |

Learning:

- Latest-message controls can break repeated no-op search loops.
- Latest-message controls do not automatically undo a semantically wrong plan
  already established by earlier artifacts.
- For final prosecutor completion, injected failure status made the model seek
  validation but did not force a blocking verdict. The model preferred to verify
  rather than directly trust the injected status.

## Experiment 8.2: Exact Control Hierarchy

Question: can exact next-action contracts beat deep accumulated history?

Results:

| Case | Target | Position | Observed Action | Result |
| --- | --- | --- | --- | --- |
| `i302_latest_absolute_blocker` | turn 302 | latest user | Wrote blocker artifact | Passed |
| `i302_system_absolute_blocker` | turn 302 | system prepend | Repeated service grep | Failed |
| `c260_latest_absolute_checklist` | turn 260 | latest user | Wrote narrow checklist artifact | Passed |
| `c260_system_absolute_checklist` | turn 260 | system prepend | Wrote broad checklist preserving old scope | Failed |
| `f751_latest_absolute_block` | turn 751 | latest user | Wrote verdict artifact but kept APPROVE content | Failed |
| `f751_system_absolute_block` | turn 751 | system prepend | Completion sentinel only | Failed |

Learning:

- Latest user message position is stronger than prepending to system in these
  captured chat payloads. This is likely because the model is continuing an
  established transcript and weighs the newest user/tool result heavily.
- Exact latest-message contracts can control mechanical outputs for checklist
  and item-blocker cases.
- They still failed to reverse final prosecutor's already-written approval
  trajectory. Once the phase has written a verdict and the persona boundary says
  "after verdict exists, finish", correction text must arrive before the verdict
  is written or the orchestrator must reset the phase.
- System-level abstract or exact controls are not reliable if they conflict
  with long phase history and existing artifacts.

Design implication:

- New personas must not rely on a one-time prompt header for long-running
  behavior.
- Each model turn needs a short, current control footer derived from phase state:
  active artifact, stopping condition, known failure/stuck state, and forbidden
  next-action class.
- Alternatively, each phase must run in short subepisodes with explicit
  handoff artifacts and no long local transcript.
- Final prosecutor must be split so that validation/failure parsing happens
  before verdict writing. After a verdict exists, prompt text cannot reliably
  reverse it.

## Experiment 9: Persona V2 Smoke

Question: does the first ground-up persona prompt preserve the strongest
source-path control against the real Flipt task text?

Persona tested:

- `personas-research-v2/surface_mapper.yaml`

Context:

- actual Flipt Kubernetes task text from captured turn 1.

Result:

| Case | Observed First Action | Result |
| --- | --- | --- |
| `surface_mapper_flipt` | `cat /app/internal/config/authentication.go` | Passed |

Learning:

- The new surface mapper prompt preserved the `Task-named source path` behavior
  against the real task text.
- This only validates the first-action gate. It does not prove the full persona
  set or orchestration.

## Experiment 10: Persona V2 Second-Turn Controls

Question: after the first required read, do the ground-up personas write the
right artifact instead of drifting back into inspection, broad repair, or prose?

Payload batch:

- `.pragma/prompt-ab/persona-v2-second-turn-20260602T162615Z`
- `.pragma/prompt-ab/persona-v2-artifact-turn-20260602T162715Z`
- `.pragma/prompt-ab/persona-v2-after-mechanics-fix-20260602T162829Z`

Results:

| Case | Expected Behavior | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_writer` first action after prompt-format fix | fenced bash reads verdict or patch plan | read verdict/patch-plan with fenced bash | Passed |
| `item_reviewer_visible_fail` | write item BLOCK from visible `--- FAIL` | wrote `/tmp/pragma/item-verdict.md` with `Decision:\nBLOCK` | Passed |
| `final_reviewer_failed_status` | write final BLOCK from failed validation status | wrote `/tmp/pragma/final-prosecutor-verdict.md` with `Decision:\nBLOCK` | Passed |
| `item_worker_stuck_state` | write blocker artifact, no more inspection | wrote `/tmp/pragma/implementer-report.md` with blocker | Passed |
| `checklist_writer_repair_scope` | write narrow config/testdata checklist | first read `/tmp/pragma/patch-plan.md` as its work method requires | Not final turn |
| `validation_runner_extract_failures` | write validation status from failure text | first read patch plan/checklist as its work method requires | Not final turn |
| `checklist_writer_after_verdict_and_plan` | write narrow checklist artifact | wrote narrow config/testdata/defaulting checklist; no runtime/proto items | Passed semantically |
| `validation_runner_after_inputs_and_failures` | write failure variables | wrote `/tmp/pragma/validation-status.md` with failed tests/packages | Passed |

Mechanical defects found:

- Checklist prompt said `acceptance` should be an array, but the first artifact
  replay produced string values.
- Some artifact-writing personas treated the completion sentinel as optional.
- Item worker put an extra `Boundary:` section inside
  `/tmp/pragma/implementer-report.md`.

Prompt fixes applied and replayed:

- Checklist now states that `acceptance`, `allowed_files`, and
  `forbidden_files` are JSON arrays, not strings.
- Every artifact-writing persona now says the final line of the same shell
  script must be `echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`.
- Item worker now says not to add extra sections to
  `/tmp/pragma/implementer-report.md`.

Post-fix results:

| Case | Observed Behavior | Result |
| --- | --- | --- |
| `checklist_artifact_schema` | wrote JSON with array fields and sentinel; scope stayed config/testdata/defaulting | Passed |
| `final_reviewer_sentinel` | wrote BLOCK verdict and sentinel | Passed |
| `item_worker_no_extra_section` | wrote only the requested report sections and sentinel | Passed |

Learning:

- The persona prompts should describe both semantic behavior and artifact
  mechanics. Scope control can pass while file schema still fails.
- Exact "final line of the same shell script" wording is stronger than
  "after writing it, run only" for sentinel emission.
- For multi-step personas, a replay at only the first post-tool turn can look
  like failure when the persona is correctly consuming its next required input.
  Test the artifact-writing turn separately.

## Experiment 11: No Task-Named Source Path Fallback

Question: what prompt controls first inspection when the task names no source
path?

Synthetic task:

```text
Add support for the configuration key
`authentication.kubernetes.cleanup.grace_period` so it can be loaded from YAML
and environment variables and has the documented default when Kubernetes
authentication is enabled. The task text names no source file path.
```

Payload batches:

- `.pragma/prompt-ab/no-named-path-fallback-20260602T163052Z`
- `.pragma/prompt-ab/no-named-path-fallback-tight-20260602T163137Z`
- `.pragma/prompt-ab/no-named-path-fallback-longest-literal-20260602T163214Z`
- `.pragma/prompt-ab/surface-mapper-v2-branches-20260602T163314Z`

Results:

| Case | Prompt Shape | Observed First Action | Result |
| --- | --- | --- | --- |
| `n00_current_surface_mapper` | current v2 prompt, no path named | read `/tmp/pragma/surface-map.md` output artifact | Failed |
| `n01_literal_anchor_fallback_top` | abstract literal-anchor fallback | used chained `rg -l` fallbacks and odd type flags | Partial |
| `n02_numbered_literal_anchor_step` | numbered fallback step | constrained search, but shortened anchor and piped to `head` | Partial |
| `n03_extract_literals_section` | explicit task literal anchors | searched task literals, but with broad `.` and exclusions | Partial |
| `n04_output_not_input_rg_shape` | output-not-input plus command shape | avoided artifact read, but shortened anchor and added `head` | Partial |
| `n05_artifact_literals_then_rg` | explicit literal anchors plus exact search shape | exact constrained `rg -n` command | Passed |
| `n06_negative_artifact_only` | only says output is not input | still read output artifact | Failed |
| `n07_longest_backticked_literal` | longest literal plus command shape | exact search but added completion sentinel after read | Partial |
| `n08_required_first_command_field` | internal `Required first command` field | exact constrained `rg -n`, no sentinel | Passed |
| `surface-mapper-v2/named_path` | patched v2, path named | `cat /app/internal/config/authentication.go` | Passed |
| `surface-mapper-v2/no_named_path` | patched v2, no path named | constrained exact literal-anchor `rg -n` | Passed |

Learning:

- Without a no-path fallback, the surface mapper read its own output artifact.
- "Output artifact is not input" is necessary but not sufficient.
- Abstract "use literal names" is too weak: the model shortened literals,
  added `head`, or built fallback chains.
- The reusable phrase that worked was:

```text
Before responding, derive this field internally:
Required first command: rg -n "<longest backticked or dotted literal from task,
unchanged>" . --glob "internal/config/**" --glob "config/**" --glob
"testdata/**" --glob "**/*test*" --glob "**/*schema*"

Your response must be exactly the Required first command in one fenced bash
block. Do not add `head`, `tail`, fallback commands, or a second command.
```

- Boundary/sentinel text can leak into read-only turns unless it explicitly says
  not to echo the sentinel after read-only inspection.

## Experiment 12: Artifact Following And Review Discipline

Question: do downstream personas consume handoff artifacts instead of reopening
broad scope, and do reviewers reject weak success evidence?

Payload batches:

- `.pragma/prompt-ab/artifact-following-review-discipline-20260602T163505Z`
- `.pragma/prompt-ab/artifact-review-fix-replay-20260602T163611Z`
- `.pragma/prompt-ab/patch-planner-schema-fix-20260602T163651Z`
- `.pragma/prompt-ab/item-reviewer-no-validation-command-20260602T163754Z`
- `.pragma/prompt-ab/item-reviewer-no-validation-command-fix-20260602T163826Z`

Results:

| Case | Expected Behavior | Observed Behavior | Result |
| --- | --- | --- | --- |
| `evidence_mapper_after_surface_map` | inspect only paths listed in surface map | read source, config test, and listed testdata paths | Passed |
| `patch_planner_after_evidence_map` | write patch plan from artifacts | reopened repository source files | Failed |
| `patch_planner_after_evidence_map_fixed` | write patch plan from artifacts | wrote `/tmp/pragma/patch-plan.md`, but added extra `Boundary:` section | Partial |
| `patch_planner_schema_fix` | write only requested patch-plan sections | wrote patch plan with no extra section and sentinel | Passed |
| `item_reviewer_grep_only_claim` | do not approve grep-only claim | ran focused validation instead of approving | Passed semantically |
| `final_reviewer_skipped_validation` | block when validation was not run | approved because failure variables were `none` | Failed |
| `final_reviewer_skipped_validation_fixed` | block when validation was not run | wrote BLOCK verdict requiring validation | Passed |
| `item_reviewer_no_validation_command` | block when validation is missing and no exact command is available | ran another grep over repository files | Failed |
| `item_reviewer_no_validation_command_fixed` | block when validation is missing and no exact command is available | wrote item BLOCK verdict | Passed |

Prompt fixes applied:

- Patch planner now has an `After-input rule`: after surface/evidence maps are
  read, write `/tmp/pragma/patch-plan.md` from those artifacts; do not reopen
  repository source files. Missing proof goes under `Unresolved until evidence`.
- Patch planner now says not to add extra sections to the patch-plan artifact.
- Final reviewer now has a `Validation-not-run rule`: `not run`, `skipped`,
  `not executed`, `none`, or no command with exit status 0 means incomplete
  validation and automatic BLOCK.
- Item reviewer now has a `Weak-evidence rule`: grep/search-only evidence or
  "Validation: Not run" cannot be approved; run the exact focused validation if
  available, otherwise block.
- Item reviewer now has an after-input review rule: if validation is missing
  and no exact focused command is available in the item/report, write BLOCK and
  do not inspect repository files to compensate.
- Validation runner now records unavailable/skipped validation in
  `unexpected_errors` instead of writing `none`.

Learning:

- Handoff artifacts need an explicit after-input rule. "Read required inputs
  first" does not imply "write the artifact next"; the model may reopen source
  files unless told not to.
- Failure variables alone are not enough for final review. A status with all
  `none` values but `Commands run: not run` must be treated as failure.
- For item review, a good response to weak evidence can be either BLOCK or a
  focused validation command. The critical control is "do not approve."
- If no exact validation command is available, the reviewer needs a stronger
  after-input rule. Without it, the model tries more grep/search to compensate.

## Experiment 13: Long-Context Positional Durability

Question: do v2 persona controls survive irrelevant local history, or do they
need latest-message refresh?

Payload batch:

- `.pragma/prompt-ab/long-context-position-20260602T164031Z`

Synthetic pressure:

- Surface mapper received 12 prior assistant/user pairs repeatedly searching
  runtime/proto/server surfaces before the no-path first inspection decision.
- Item reviewer received 12 prior assistant/user pairs repeatedly finding grep
  occurrences but no validation result before the review decision.

Results:

| Case | Control Position | Observed Behavior | Result |
| --- | --- | --- | --- |
| `surface_long_no_latest_control` | v2 prompt only at phase opening | searched shortened `kubernetes` in config and piped to `head` | Failed |
| `surface_long_latest_control` | exact next-action control in latest user message | exact constrained `rg -n "authentication.kubernetes.cleanup.grace_period"` | Passed |
| `item_reviewer_long_no_latest_control` | v2 prompt only at phase opening | grepped docs again instead of blocking | Failed |
| `item_reviewer_long_latest_control` | state variables and decision rule in latest user message | wrote item BLOCK verdict | Passed |

Learning:

- This confirms the earlier real-payload finding for the new v2 prompts:
  one-time phase-opening controls do not survive long local history.
- Latest-message controls do survive the same noisy history when they include
  explicit state variables, required artifact, and exact forbidden next-action
  class.
- The orchestration design cannot rely on static persona prompts for phases
  that run many turns. Either keep phases short or append a compact control
  footer to the latest user/tool-result message on every turn.

## Experiment 14: Validation Runner Status Preservation

Question: what prompt shape makes the validation runner save full output while
preserving the validation command status?

Payload batches:

- `.pragma/prompt-ab/validation-runner-discipline-20260602T164111Z`
- `.pragma/prompt-ab/validation-status-wrapper-mutations-20260602T164139Z`
- `.pragma/prompt-ab/validation-runner-wrapper-fix-20260602T164230Z`
- `.pragma/prompt-ab/validation-runner-status-write-fix-20260602T164252Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| current validation runner | existing discipline prose | used `tee` and status capture, but shape was not guaranteed | Partial |
| `v01_pipefail_rule` | loose pipefail rule | wrote status artifact in same turn and used `head` | Failed |
| `v02_required_wrapper` | exact required validation shell shape | exact `set -o pipefail`, `tee`, `${PIPESTATUS[0]}`, `exit "$status"` | Passed |
| `v03_bad_good` | good/bad examples | captured status but added extraction/status artifact and `head` | Partial/failed |
| patched validation runner wrapper | exact shell shape in persona | exact status-preserving wrapper only | Passed |
| patched validation status write | visible failure output plus `EXIT_STATUS: 1` | wrote validation-status with failed tests/package/log path | Passed |

Winning phrase:

```text
Required validation command shell shape:
set -o pipefail
log=/tmp/pragma/validation.log
<validation command> 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
echo "EXIT_STATUS: $status"
exit "$status"

Use this shape when running validation after reading the inputs. Do not use
`$?` after `tee` as the validation command status. Do not add `head`, `tail`,
fallback commands, or status-artifact writing to the validation-running
command.
```

Status-writing control:

```text
Write /tmp/pragma/validation-status.md only after validation output and
EXIT_STATUS are visible in the conversation. Extract failed tests/packages from
the visible output and log path.
```

Learning:

- Loose "preserve status" prose is not enough. The model may write a plausible
  script that hides status behind `tee`, extracts only with `head`, or writes
  the status artifact before the tool result is visible.
- Exact shell shape is strong. Good/bad examples were weaker because the model
  copied the good status primitive but still added extra behavior.

## Experiment 15: Reviewer Approval Discipline

Question: do reviewers approve when validation evidence is actually clean?

Payload batch:

- `.pragma/prompt-ab/reviewer-approval-discipline-20260602T164434Z`

Results:

| Case | Expected Behavior | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_clean_validation` | write item APPROVE verdict | wrote `/tmp/pragma/item-verdict.md` with `Decision:\nAPPROVE` and sentinel | Passed |
| `final_reviewer_clean_validation` | write final APPROVE verdict | wrote `/tmp/pragma/final-prosecutor-verdict.md` with `Decision:\nAPPROVE` and sentinel | Passed |

Learning:

- The same exact verdict template supports both BLOCK and APPROVE when evidence
  is explicit.
- Final reviewer approved from clean validation status without inspecting diff
  evidence. That may be acceptable if validation-status is the final contract,
  but if final review must include diff evidence, that needs a separate control
  and replay test.

## Experiment 16: Cross-Task No-Path Fallback And Early Handoff

Question: does the no-path fallback generalize beyond config-shaped tasks, and
can early personas hand off through artifacts rather than shared chat history?

Payload batches:

- `.pragma/prompt-ab/cross-task-no-path-20260602T164640Z`
- `.pragma/prompt-ab/surface-generic-fallback-fix-20260602T164739Z`
- `.pragma/prompt-ab/artifact-only-chain-early-20260602T164900Z`
- `.pragma/prompt-ab/artifact-only-chain-early-fix-20260602T165005Z`
- `.pragma/prompt-ab/surface-write-after-candidate-read-20260602T165144Z`
- `.pragma/prompt-ab/surface-write-after-source-test-read-20260602T165206Z`
- `.pragma/prompt-ab/surface-artifact-schema-fix-20260602T165241Z`

Cross-task results:

| Case | Expected Behavior | Observed Behavior | Result |
| --- | --- | --- | --- |
| `x00_current_surface_mapper` | exact `--max-retries` literal search | searched `background` under config/testdata/schema globs | Failed |
| `x01_generic_literal_repo_owned` | generic repo-owned literal search | kept `--max-retries` but old config globs still won | Failed |
| `x02_required_field_before_persona` | generic repo-owned literal search | kept `--max-retries` but old config globs still won | Failed |
| `x03_exact_task_literal_section` | exact generic literal search | exact generic `rg -n --glob ... -- "--max-retries" .` | Passed |
| `surface-generic/config_no_path` | exact generic config-key search | exact generic literal search | Passed |
| `surface-generic/cli_no_path` | exact generic flag search | exact generic literal search | Passed |
| `surface-generic/named_path` | direct named-path read | `cat /app/cmd/pragma/jobs.go` | Passed |

Early handoff results:

| Case | Expected Behavior | Observed Behavior | Result |
| --- | --- | --- | --- |
| `surface_write_after_literal_search` | write surface map after literal search | read candidate source path | Not enough context |
| `surface_write_after_candidate_read` | write surface map after source read | read adjacent test path | Not enough context |
| `surface_write_after_source_test_read` | write surface map | wrote surface map, but misclassified candidate as task-named and added extra section | Partial |
| `surface_artifact_schema_fix` | write clean surface map | wrote `Task-named source path: none`, separate candidate paths, no extra section | Passed |
| `evidence_write_after_surface_and_reads` | write evidence map from fresh phase artifacts | initially chased explicit unknown `internal/jobs/runner.go` | Failed |
| `evidence_write_after_surface_and_reads` after prompt fix | write evidence map from listed surfaces | wrote evidence map and put runner unknown under `Not evidenced` | Passed |

Prompt fixes applied:

- Surface mapper no-path fallback is now generic, not config-only:

```text
Required first command: rg -n --glob "!vendor/**" --glob "!node_modules/**"
--glob "!dist/**" --glob "!build/**" --glob "!**/*.pb.go" --
"<longest backticked or dotted literal from task, unchanged>" .
```

- Surface mapper artifact now separates:
  - `Task-named source path`
  - `Candidate source paths`
- Surface mapper no longer reclassifies `rg` results as task-named paths.
- Surface mapper stable handoff is: exact literal search -> bounded primary
  source/test inspection -> surface-map artifact.
- Evidence mapper now consumes both task-named and candidate source paths, and
  writes `Not evidenced` instead of chasing explicit unknowns.

Learning:

- The earlier config/testdata/schema globs were benchmark-shaped and failed on
  a CLI flag task.
- A generic literal search must put `--glob` options before `--` so flag-like
  literals such as `--max-retries` are treated as patterns, not options.
- Static prompt text saying "do not read candidate paths" did not hold; the
  stable behavior was bounded candidate source/test inspection followed by an
  artifact.

## Experiment 17: Final Diff Evidence Gate

Question: can final reviewer approval require diff evidence after clean
validation?

Payload batches:

- `.pragma/prompt-ab/final-reviewer-diff-control-20260602T165325Z`
- `.pragma/prompt-ab/final-reviewer-diff-fix-20260602T165414Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `d00_current` | current prompt | approved from validation status alone | Failed |
| `d01_no_approve_until_diff` | no approval before diff command | ran `git diff --stat && git diff -- .` | Passed |
| `d02_required_diff_command` | exact required diff command | ran `git diff --stat && git diff -- .` | Passed |
| `d03_state_variables` | state variables and required action | ran `git diff --stat && git diff -- .` | Passed |
| `clean_no_diff` after prompt fix | clean validation, no diff visible | ran diff command | Passed |
| `clean_with_diff` after prompt fix | clean validation plus diff visible | wrote APPROVE verdict with diff summary | Passed |

Winning phrase:

```text
Clean-validation diff rule:
If validation status is clean, do not approve yet. First run exactly:
git diff --stat && git diff -- .
Only after diff output is visible may you write an APPROVE verdict. Do not
approve from validation status alone.
```

Learning:

- "Inspect final diff evidence" was too weak.
- Exact command plus "do not approve from validation status alone" worked.
- This makes final approval a two-evidence decision: clean validation status and
  visible diff evidence.

## Experiment 18: Artifact-Only Full Chain And Non-Config Reviewer

Question: can the v2 persona packet operate through artifacts across the full
chain on a non-config CLI task shape?

Payload batches:

- `.pragma/prompt-ab/artifact-only-chain-full-20260602T165842Z`
- `.pragma/prompt-ab/artifact-chain-item-fixes-20260602T170007Z`
- `.pragma/prompt-ab/item-reviewer-command-format-fix-20260602T170050Z`
- `.pragma/prompt-ab/item-reviewer-after-own-validation-20260602T170119Z`

Results:

| Case | Expected Behavior | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_planner_cli_from_artifacts` | write patch plan from surface/evidence artifacts | wrote patch plan and kept runner internals unresolved | Passed |
| `checklist_writer_cli_from_patch` | write checklist from patch plan | wrote checklist JSON with arrays and sentinel | Passed |
| `item_worker_cli_after_item` | inspect repo-relative allowed file | initially read `/tmp/pragma/cmd/pragma/jobs.go` | Failed |
| `item_worker_path_rule` | inspect repo-relative allowed file | read `cmd/pragma/jobs.go` | Passed |
| `item_reviewer_cli_good` | approve visible PASS in implementer report | initially reran validation | Failed |
| `item_reviewer_visible_pass` | approve visible PASS in implementer report | wrote item APPROVE verdict | Passed |
| `item_reviewer_cli_weak` | do not approve grep-only evidence | ran validation, but emitted prose and prepended `cd /tmp/pragma` | Partial |
| `item_reviewer_command_format_fix` | exact validation command, no prose | emitted only `go test ./cmd/pragma/... -run ...` | Passed |
| `item_reviewer_after_validation_pass` | write APPROVE after reviewer-run validation passes | wrote item APPROVE verdict | Passed |
| `item_reviewer_after_validation_fail` | write BLOCK after reviewer-run validation fails | wrote item BLOCK verdict | Passed |
| `validation_runner_cli_after_inputs` | run exact status-preserving wrapper | used `set -o pipefail`, `tee`, `${PIPESTATUS[0]}` | Passed |
| `final_reviewer_cli_clean_no_diff` | run diff before approval | ran `git diff --stat && git diff -- .` | Passed |
| `final_reviewer_cli_clean_with_diff` | approve after clean validation and diff | wrote final APPROVE verdict | Passed |

Prompt fixes applied:

- Item worker now has a path rule distinguishing `/tmp/pragma/*` coordination
  artifacts from repository paths in `allowed_files` and `forbidden_files`.
- Item reviewer now has the same path/command rule for repository paths and
  validation commands.
- Item reviewer now has a visible-pass rule: if implementer report already
  contains exact focused validation command, PASS, and `EXIT_STATUS: 0`, write
  the verdict artifact immediately instead of rerunning validation.
- Item reviewer now explicitly requires exactly one fenced bash block and no
  prose outside it.

Learning:

- Path vocabulary must distinguish artifact paths from repository paths. Without
  that, the model rewrites repo-relative paths under `/tmp/pragma`.
- "Validation was run and passed" must be an explicit visible-pass rule.
  Otherwise the reviewer may rerun validation even when the implementer report
  has enough evidence.
- If the reviewer must run validation, it needs both command-preservation and
  response-format controls.

## Experiment 19: Artifact Precedence, Missing Fields, Malformed Inputs

Question: which prompt variants control conflict resolution and invalid handoff
artifacts?

Payload batches:

- `.pragma/prompt-ab/artifact-precedence-malformed-20260602T172814Z`
- `.pragma/prompt-ab/artifact-precedence-malformed-fix-20260602T17M34Z`
- `.pragma/prompt-ab/checklist-missing-hard-blocker-20260602T173030Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_conflict_current` | current patch planner | kept `cmd/pragma/jobs.go` in route and put `internal/jobs/runner.go` unresolved | Passed |
| `patch_conflict_precedence` | explicit evidence-map precedence | same behavior, cleaner route and unresolved text | Passed |
| `patch_conflict_block` | conflict means narrower unresolved | same behavior, explicit unresolved proof | Passed |
| `checklist_missing_current` | current checklist writer | broadened into repo-search and implementation tasks | Failed |
| `checklist_missing_block` | missing required field rule | blocker checklist items, but several evidence-gathering tasks | Partial |
| `checklist_missing_after_patch` | persona patched with missing-field rule | blocker items, still broad "inspect/search" approach language | Partial |
| `checklist_missing_hard_blocker` | hard blocker item wording | single blocker item, `allowed_files: []`, `forbidden_files: ["**/*"]` | Passed |
| `item_worker_malformed_current` | current item worker | reread malformed input artifact | Failed |
| `item_worker_malformed_block` | malformed artifact rule | wrote blocker report | Passed |
| `item_worker_malformed_after_patch` | persona patched with malformed rule | wrote blocker report | Passed |

Accepted phrases:

```text
Artifact precedence rule:
Use this precedence order: evidence map > surface map > original task prose. If
surface map and evidence map conflict, evidence map wins. Surfaces listed under
`Not evidenced` must not become patch route items; put them under `Unresolved
until evidence`.
```

```text
Missing required field rule:
If the patch plan lacks a concrete source path, consumer/defaulting path,
validation command, or allowed files for a behavior, do not infer or broaden.
Write one blocker checklist item for the missing proof. Its description must
name every missing field and proof required. Its approach must say "Do not
inspect repository files; return a blocker until the missing proof is provided."
Its allowed_files must be an empty array and forbidden_files must be ["**/*"].
```

```text
Malformed artifact rule:
If the required input artifact is invalid JSON or cannot be parsed, write the
required output artifact as a blocker report. Do not inspect repository files.
Do not repair the JSON.
```

Learning:

- Patch planner already handled conflicts reasonably because the evidence map
  had `Not evidenced`; explicit precedence made the behavior more auditable.
- Checklist writer needed hard wording. "Missing proof" alone still let the
  model create repository-search tasks.
- Malformed input control needs both blocker output and "do not inspect/do not
  repair" language.

## Experiment 20: Partial Validation And Generated File Policy

Question: does the final reviewer block partial validation, and what prompt
keeps item worker from touching generated files?

Payload batches:

- `.pragma/prompt-ab/partial-validation-generated-policy-20260602T173110Z`
- `.pragma/prompt-ab/generated-file-policy-fix-20260602T173200Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_partial_current` | current final reviewer | blocked because one command was `not run` | Passed |
| `final_reviewer_partial_rule` | explicit partial validation rule | blocked and named missing command | Passed |
| `item_worker_generated_current` | current item worker | tried to read `/tmp/pragma/rpc/service.pb.go` | Failed |
| `item_worker_generated_policy` | generated file policy injection | blocked, but used malformed `cat /tmp/... <<EOF` shell | Partial |
| `generated-file-policy-fix` | persona patched with policy plus blocker shell shape | wrote valid blocker report | Passed |

Accepted phrases:

```text
Partial validation rule:
All required validation commands must have exit status 0. If any required
command is missing, skipped, not run, unavailable, or lacks exit status 0, write
BLOCK. Passing one command does not satisfy another command.
```

```text
Generated file policy:
If the current item targets generated or derived files and no successful
producer command is visible in the current item, write
/tmp/pragma/implementer-report.md with a Blocker section. Do not inspect, edit,
or hand-edit generated files. Do not search for producers in this state.
```

Learning:

- Final reviewer already blocked partial validation through the not-run rule,
  but explicit partial-validation wording makes the approval gate clearer.
- "Do not hand-edit generated files" was too weak. The model still inspected a
  generated file and rewrote the repo path under `/tmp/pragma`.
- Generated-file blocking also needs an exact report-writing shell shape; an
  injected policy alone produced malformed shell.

## Experiment 21: Failed Producer Command Stop

Question: after a generator/producer command for generated files fails, does the
item worker write a blocker report or fall back into tool discovery, alternate
producer search, source inspection, or hand-editing generated output?

Payload batch:

- `.pragma/prompt-ab/item-worker-failed-producer-20260602T190641Z`

Fixture:

- current item targeted generated protobuf/gateway files,
- current item named `producer_command: buf generate`,
- the attempted producer command returned `bash: buf: command not found` and
  `EXIT_STATUS: 127`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_failed_producer` | current item worker generated-file policy | searched for `buf`, used `find`, listed `/tmp/pragma`, and piped `cat Makefile` to `head` | Failed |
| `item_worker_failed_producer_rule` | failed producer command rule | wrote blocker report immediately with no files changed and sentinel | Passed |
| `item_worker_failed_producer_terminal` | failed producer rule plus terminal-decision sentence | wrote equivalent blocker report | Passed/redundant |

Accepted phrase:

```text
Failed producer command rule:
If a producer/generator command for generated or derived files has visible
nonzero status, command-not-found output, unavailable tool output, timeout, or
killed status, write /tmp/pragma/implementer-report.md with a Blocker section
now. Do not inspect source files, inspect generated files, edit files, search
for alternate producers, or hand-edit generated output after producer failure.
```

Learning:

- The prior generated-file policy blocked when no producer was visible, but it
  did not stop the agent after a producer command visibly failed.
- After failed code generation, the model's natural fallback was environment
  discovery and `head`, which recreates the benchmark failure mode without
  needing a task-specific prompt.
- The useful control is a visible command-status stop rule tied to the derived
  artifact class.
- An extra "terminal" sentence was redundant once the failed-producer rule
  named the exact report artifact and forbidden fallback actions.

## Experiment 22: Patch Minimality And Uncertainty Handling

Question: which prompt variants constrain implementation edits and keep
uncertain surfaces out of patch scope?

Payload batch:

- `.pragma/prompt-ab/patch-minimality-uncertainty-20260602T173459Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_minimal_current_after_source` | current item worker | one-line `sed` changing only `flags.Int("max-retries", 0, ...)` to `3` | Passed |
| `item_worker_minimal_rule_after_source` | explicit patch minimality rule | one-line `sed`, shorter match, same allowed file | Passed |
| `item_worker_diff_budget_after_source` | explicit diff budget | one-line `sed`, same allowed file | Passed |
| `patch_planner_uncertainty_current` | current patch planner | evidenced CLI route; runner internals under unresolved | Passed |
| `patch_planner_uncertainty_rule` | explicit uncertainty rule | evidenced CLI route; runner internals under unresolved with clearer proof requirement | Passed |

Accepted phrases:

```text
Patch minimality rule:
Make the smallest edit that satisfies the current item acceptance. Change only
files listed in allowed_files. Do not edit tests, runner internals, comments,
formatting, nearby TODOs, or future checklist items unless the current item
explicitly requires them.
```

```text
Diff budget rule:
Prefer one semantic change in one allowed file. Any edit outside allowed_files
is a blocker. Do not run broad formatters or broad tests before the item patch.
```

```text
Uncertainty rule:
If evidence says a surface is unknown, not evidenced, maybe required, or proof
required, do not promote it into patch route or implementation scope. Put it
under `Unresolved until evidence` with exact proof required. Speculation is
output, not work.
```

Learning:

- The existing item worker already chose a minimal edit in this simple case.
  The new phrases are retained as guardrails because they directly state the
  intended edit budget and forbidden drift classes.
- The existing patch planner already handled the uncertainty case because
  `Not evidenced` was present. The uncertainty phrase improved the proof
  language and makes the behavior portable to less-structured artifacts.

## Experiment 23: Repair Authority And Verdict Template Strictness

Question: during repair, does checklist generation obey the concrete final
verdict instead of reopening broad product scope, and which artifact template
shape reliably produces an event-readable verdict file?

Payload batch:

- `.pragma/prompt-ab/repair-scope-template-strictness-20260602T180658Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `repair_scope_current` | current checklist writer | wrote one narrow `cmd/pragma/jobs.go` item; mentioned forbidden broad work only as things not to do | Passed |
| `repair_scope_authority` | final verdict as repair authority | wrote a cleaner one-file default-value item with no broad product-scope leakage | Passed |
| `repair_scope_whitelist` | explicit allowed/forbidden repair surfaces | wrote a narrow item, but invented glob-like forbidden paths beyond the evidence | Partial |
| `template_schema_only` | "write file with Findings/Required repair/Decision" | ignored artifact writing and inspected `/tmp` | Failed |
| `template_exact_heredoc` | exact heredoc, exact file path, two-line `Decision:\nBLOCK`, sentinel | wrote the verdict artifact correctly | Passed |
| `template_good_bad` | good/bad examples without exact full shell template | wrote `verdict.txt`, omitted required path and sentinel | Failed |
| `template_json` | JSON-ish schema | wrote the required file, but omitted the event-readable two-line marker | Failed |
| `item_reviewer_visible_failure_after_inputs` | actual item reviewer persona after adding exact shell shapes | wrote `/tmp/pragma/item-verdict.md` with visible failure findings, `Decision:\nBLOCK`, and sentinel | Passed |

Accepted phrases:

```text
Repair authority rule:
During repair, /tmp/pragma/final-prosecutor-verdict.md is the authority.
The only implementation checklist items allowed are concrete failed tests,
missing files or surfaces, wrong fields/contracts/surfaces, and Required repair
entries from the verdict. The patch plan may provide source paths, allowed
files, and validation commands for those same surfaces only. Original task
prose and tempting product prose are not inputs.
```

```text
Required BLOCK shell shape:
cat > /tmp/pragma/item-verdict.md <<'EOF'
Findings:
<concrete findings>

Required repair:
<repair>

Decision:
BLOCK
EOF
```

Learning:

- The existing bounded repair controls were enough for the compact synthetic
  task, but the repair-authority wording removed residual product-scope
  references from the generated item.
- Explicit whitelists can overfit the prompt and encourage invented forbidden
  patterns. The safer general control is authority plus concrete blocker
  surfaces, not a large allow/deny list.
- Artifact templates need the full shell shape, exact output path, parser
  marker, and sentinel. Schema descriptions and good/bad prose did not reliably
  create the right artifact.

## Experiment 24: Transcript Suppression And Verdict Size

Question: what prompt controls prevent reviewer verdicts from copying noisy
tool transcripts or repeated failed commands into artifacts, while preserving
event-readable verdict markers?

Payload batch:

- `.pragma/prompt-ab/transcript-suppression-cardinality-20260602T181132Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_repeated_log` | current final reviewer with repeated `sed` transcript in validation status | did not copy `sed`; wrote 574-byte BLOCK verdict with marker and sentinel | Passed |
| `final_reviewer_non_reproduction` | transcript non-reproduction only | shorter 397-byte BLOCK verdict; no copied transcript | Passed |
| `final_reviewer_cardinality` | section cardinality only | shorter 342-byte BLOCK verdict; no duplicate headings | Passed |
| `final_reviewer_both` | transcript compression plus section cardinality | shortest 331-byte BLOCK verdict; no copied transcript, one Findings section, marker and sentinel intact | Passed |
| `item_reviewer_current_repeated_report` | current item reviewer with repeated failed `sed` attempts in implementer report | did not copy `sed`, but wrote a larger verdict and repeated the failed attempts as a separate finding | Partial |
| `item_reviewer_suppression_cardinality` | transcript compression plus loose section cardinality | shorter verdict and no copied `sed`, but still wrote four Findings bullets despite a three-bullet limit | Partial |
| `item_reviewer_strict_size_rule` | strict bullet-count wording | kept three Findings bullets and no copied `sed`, but inferred a concrete indentation repair from failed commands | Failed |
| `item_reviewer_failed_command_not_design_evidence` | transcript compression, failed-command evidence rule, strict verdict size | kept three Findings bullets, did not copy `sed`, did not infer indentation mechanics, wrote BLOCK marker and sentinel | Passed |

Accepted phrases:

```text
Transcript compression rule:
Never copy prior tool output, shell scripts, diffs, stack traces, repeated log
lines, or transcript text into the verdict. Summarize repeated evidence instead
of reproducing it.
```

```text
Failed-command evidence rule:
Failed or repeated commands prove only what failed, not how to repair it. Do
not infer implementation mechanics from failed sed commands, failed patches,
stack traces, or repeated attempts. Required repair must name the failing
surface and evidence needed, unless a successful source inspection proves a
concrete fix.
```

```text
Strict verdict size rule:
The Findings section must contain no more than three lines beginning with
`- `. If there are more than three facts, merge related facts into the same
bullet. The Required repair section must be one sentence. The Decision section
must be exactly two lines: `Decision:` then `BLOCK` or `APPROVE`.
```

Learning:

- Exact verdict templates already made the final reviewer resistant to copying
  noisy repeated command transcripts. Compression and size rules still reduced
  output length without breaking the event marker.
- Loose "at most three bullets" wording was too weak in item review. Counting
  the concrete line prefix, `lines beginning with "- "`, was obeyed.
- Compression alone can make the model summarize a failed command as if it were
  repair evidence. Reviewer prompts need to distinguish failure evidence from
  successful source evidence before stating Required repair.

## Experiment 25: Item Scope Authority

Question: when a checklist item's prose asks for one path but `allowed_files`
and `forbidden_files` say another, does the item worker treat file constraints
as authority or follow the prose?

Payload batch:

- `.pragma/prompt-ab/item-scope-authority-20260602T181630Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_scope_conflict` | current item worker; title/description/approach require forbidden runtime auth files | wrote `/tmp/pragma/implementer-report.md` blocker; no file inspection; no edits | Passed |
| `item_worker_scope_authority` | explicit allowed/forbidden authority rule | wrote blocker naming the forbidden file and allowed-file mismatch; no inspection; marker and sentinel intact | Passed |
| `item_worker_scope_conflict_decision` | authority rule plus explicit pre-inspection path comparison | also wrote blocker; more verbose but not meaningfully stronger | Passed/redundant |

Accepted phrase:

```text
Scope authority rule:
allowed_files and forbidden_files are hard constraints. They have higher
priority than the item title, description, approach, and acceptance text. If the
required edit path is outside allowed_files or inside forbidden_files, write
/tmp/pragma/implementer-report.md with a Blocker section. Do not inspect
repository files to resolve that conflict.
```

Learning:

- The existing item worker already respected `forbidden_files` in this compact
  conflict payload because patch minimality and diff budget rules were already
  present.
- The explicit authority phrase is retained because it names precedence among
  conflicting fields. This is cleaner than adding another mechanical
  path-comparison checklist to the persona prompt.
- The stronger decision variant was redundant and increased prompt bulk without
  changing behavior.

## Experiment 26: Unavailable Validation Tool Handling

Question: after the exact required validation command has already failed because
the tool is unavailable, does the validation runner write status or try to
install/substitute/rerun?

Payload batch:

- `.pragma/prompt-ab/unavailable-validation-tool-20260602T181928Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_buf_missing` | current validation runner after `buf generate` returned `EXIT_STATUS: 127` and `buf: command not found` | wrote `/tmp/pragma/validation-status.md`, recorded `unexpected_errors`, no substitution/install/rerun | Passed |
| `validation_runner_unavailable_status` | explicit unavailable-tool status rule | same behavior; recorded unavailable tool and exit status | Passed/redundant |
| `validation_runner_no_substitution` | unavailable-tool rule plus command-authority/no-substitution rule | same behavior; no behavior improvement | Passed/redundant |

Accepted existing control:

```text
Not-run rule:
If a required validation command is not run, skipped, unavailable, or cannot
be executed, record that fact in unexpected_errors. Do not write "none" for
unexpected_errors in that case.
```

Learning:

- The current validation runner already treats `command not found` plus
  `EXIT_STATUS: 127` as validation status, not as a reason to install tools or
  substitute another command.
- Explicit no-substitution wording was not retained in the persona because it
  did not change behavior in this replay; the existing not-run rule plus
  status-writing rule was enough.
- This evidence supports letting the final reviewer block on
  `unexpected_errors` instead of asking validation runner to repair the tool
  environment.

## Experiment 27: Final Diff Hygiene

Question: after clean validation and visible diff evidence, does the final
reviewer reject unrelated or out-of-scope diff hunks instead of approving just
because tests passed?

Payload batch:

- `.pragma/prompt-ab/final-diff-hygiene-20260602T182150Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_out_of_scope_diff` | current final reviewer; clean validation for config/testdata surfaces, visible diff also adds auth-handler debug bypass | wrote `Decision:\nAPPROVE` despite unrelated auth-handler change | Failed |
| `final_reviewer_diff_hygiene` | diff hygiene rule | wrote BLOCK and named unexplained handler change | Passed |
| `final_reviewer_diff_scope_decision` | diff hygiene plus compare changed files to validated surfaces | wrote BLOCK and explicitly named `internal/server/auth/handler.go` as out-of-scope | Passed |

Accepted phrase:

```text
Diff hygiene rule:
Clean validation is not enough to approve if visible diff evidence contains
unrelated, out-of-scope, forbidden, generated, test-only, broad formatting, or
unexplained changes. Before APPROVE, compare changed files and hunks in
`git diff --stat` and `git diff -- .` with the validated surfaces named in
validation-status. APPROVE only when every changed file and hunk is directly
tied to those surfaces. Otherwise write BLOCK with the out-of-scope file path.
```

Learning:

- The prior final diff gate only forced the model to inspect diff evidence
  before approval; it did not teach the model what makes a diff approvable.
- Clean validation can actively mask out-of-scope or unsafe changes unless the
  reviewer has a diff-hygiene rule.
- The useful authority is `validated_surfaces` from validation-status plus the
  visible changed files/hunks, not the fact that tests passed.

## Experiment 28: Contradictory Validation Status

Question: if validation-status summary fields say `none` but `Commands run`
contains a nonzero exit status, does the final reviewer block from command
status or continue inspecting?

Payload batch:

- `.pragma/prompt-ab/contradictory-validation-status-20260602T182410Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_status_contradiction` | current final reviewer; `Commands run` has `EXIT_STATUS: 1` while all failure fields say `none` | noticed discrepancy but wrote prose and ran `cat /tmp/pragma/validation.log` instead of verdict artifact | Failed |
| `final_reviewer_command_status_authority` | concise command-status authority rule | wrote BLOCK verdict artifact immediately; no diff or log inspection | Passed |
| `final_reviewer_conservative_contradiction` | command-status authority plus conservative contradiction rule | wrote BLOCK artifact, but added prose outside the shell block | Partial/rejected |

Accepted phrase:

```text
Command-status authority rule:
The Commands run section is authoritative for whether validation passed. If any
command has nonzero status, EXIT_STATUS other than 0, failed, timeout, killed,
or unknown status, write BLOCK even if failed_tests, failed_packages,
unexpected_errors, and missing_files_or_surfaces say "none". Do not run diff or
inspect logs when command status contradicts summary fields.
```

Learning:

- The existing final reviewer could identify inconsistent validation status,
  but without an exact authority rule it tried to gather more evidence and
  broke the artifact-writing boundary.
- A conservative "if fields conflict, choose safer interpretation" rule was
  semantically correct but too verbose; in replay it leaked prose outside the
  shell block.
- Command status should be treated as the approval gate authority. Summary
  fields can add detail, but they cannot override a nonzero command status.

## Experiment 29: Final Reviewer Response Shape

Question: after the command-status authority fix, does the final reviewer still
need an additional persona-local "exactly one fenced bash block" response-shape
rule to prevent prose leakage?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-response-shape-20260602T182637Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_shape_baseline` | current final reviewer with command-status authority | wrote only one fenced bash block, BLOCK verdict artifact, and sentinel | Passed |
| `final_reviewer_verbose_contradiction` | added conservative contradiction wording that previously leaked prose before command-status authority existed | still wrote only one fenced bash block | Passed |
| `final_reviewer_shell_only_verbose` | added explicit response-shape rule plus verbose contradiction wording | wrote only one fenced bash block; no improvement over current | Passed/redundant |

Rejected phrase:

````text
Response shape rule:
Your response must be exactly one fenced ```bash code block and nothing else.
Do not write explanatory prose before or after the code block. If you need to
explain a finding, put it inside the verdict artifact written by the shell
script.
````

Learning:

- The prior prose leak in Experiment 28 was primarily caused by missing
  command-status authority, not by the absence of a local response-shape rule.
- Once the final reviewer had a direct BLOCK condition, it obeyed the existing
  global/system shell-only response shape in this replay.
- Do not add redundant response-shape wording to the final reviewer from this
  evidence. Stronger decision authority was the causal fix.

## Experiment 30: Item Reviewer Contradictory Report

Question: if an implementer report claims PASS/success but also contains visible
failure output and `EXIT_STATUS: 1`, does item reviewer trust the failure status
or the success prose?

Payload batch:

- `.pragma/prompt-ab/item-review-contradictory-report-20260602T182903Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_contradictory_report` | current item reviewer; implementer report says PASS while showing `--- FAIL` and `EXIT_STATUS: 1` | wrote BLOCK verdict artifact, named the contradiction, no validation rerun, no prose leakage | Passed |
| `item_reviewer_command_status_authority` | added item command-status authority rule | wrote BLOCK artifact, but longer; no behavior improvement | Passed/redundant |
| `item_reviewer_conservative_contradiction` | added command-status authority plus conservative contradiction rule | wrote BLOCK artifact, but longer; no behavior improvement | Passed/redundant |

Accepted existing control:

```text
Visible validation failure rule:
Any visible line containing FAIL, --- FAIL, unexpected error, panic, or
nonzero validation status is an automatic BLOCK for the current item.
```

Learning:

- The current item reviewer already treats visible failure output and nonzero
  status as authoritative over success prose.
- Unlike final review, item review did not need an additional command-status
  authority phrase in this replay. The existing visible-failure rule is more
  compact and sufficient for this contradiction shape.
- Do not copy the final-review command-status rule into item reviewer unless a
  future replay shows a real failure. It increases prompt bulk without changing
  behavior here.

## Experiment 31: Repair Checklist Completed-Item Preservation

Question: during repair checklist generation, does checklist writer preserve
completed prior items and add only the new pending repair item?

Payload batch:

- `.pragma/prompt-ab/checklist-repair-preservation-20260602T183135Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_repair_preservation` | current checklist writer with prior checklist, final verdict, and patch plan visible | preserved two completed prior items, added one pending `cleanup.grace_period` repair item, wrote JSON artifact and sentinel | Passed |
| `checklist_completed_preservation` | explicit completed-item preservation rule | preserved completed items and added one pending repair, but gave the pending item `allowed_files: [fixture]` with `forbidden_files: ["**/*"]` | Partial/rejected |
| `checklist_repair_delta_preservation` | preservation plus repair-delta rule | same preservation behavior, same conflicting `forbidden_files: ["**/*"]` on pending item | Partial/rejected |
| `checklist_preservation_file_scope_consistency` | preservation plus file-scope consistency rule | still produced `forbidden_files: ["**/*"]` on the pending item despite the consistency rule | Failed/rejected |

Accepted existing control:

```text
`status` is `pending` unless the item was already completed by a prior
checklist and is being preserved during a repair pass
```

Learning:

- The current checklist writer already preserved completed prior items in this
  compact repair payload.
- Stronger completed-item preservation wording was semantically attractive but
  made the new pending item worse by producing an impossible scope contract:
  one allowed file plus `forbidden_files: ["**/*"]`.
- A generic "must not forbid allowed files" consistency rule was not strong
  enough to fix that behavior. Do not retain it from this evidence.
- Checklist prompt mutations must be judged by the next persona contract, not
  only by checklist JSON validity. A syntactically valid checklist can still
  block item worker if its file-scope fields conflict.

## Experiment 32: Checklist Pending Item File Scope

Question: what wording makes checklist writer produce pending items whose
`allowed_files` and `forbidden_files` form a usable item-worker contract?

Payload batch:

- `.pragma/prompt-ab/checklist-file-scope-rules-20260602T183507Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_file_scope_current` | current checklist writer on the Experiment 31 repair payload | preserved completed items, but pending item had `allowed_files: [fixture]` and `forbidden_files: ["**/*"]` | Failed |
| `checklist_forbidden_from_unsafe_shortcut` | pending item file-scope rule deriving forbidden scope from unsafe shortcuts | pending item used `allowed_files: [fixture]` and `forbidden_files: ["internal/server/auth/**"]` | Passed |
| `checklist_no_forbid_allowed_rule` | forbid `**/*` when `allowed_files` is non-empty; use empty array for abstract forbidden scope | avoided conflict but lost the explicit runtime-auth unsafe surface | Partial/rejected |
| `checklist_file_scope_example_rule` | unsafe-shortcut rule plus good/bad example | passed with `forbidden_files: ["internal/server/auth/**"]`, but example was not needed | Passed/redundant |

Accepted phrase:

```text
Pending item file-scope rule:
For a pending implementation item, allowed_files is the list of repository
files the item may edit. forbidden_files is only for concrete unsafe shortcuts
or broad surfaces from the patch plan/verdict. If allowed_files is non-empty,
do not put "**/*" in forbidden_files. For runtime-auth unsafe shortcuts, use
a concrete forbidden pattern such as "internal/server/auth/**".
```

Learning:

- Experiment 31's abstract consistency rule failed because it told the model
  what not to do but not how to construct a replacement.
- Deriving `forbidden_files` from the concrete unsafe shortcut preserved the
  useful guardrail without blocking the allowed file.
- The no-forbid-allowed rule was safe but weaker: it produced an empty
  `forbidden_files` array and lost the concrete runtime-auth guardrail.
- Good/bad examples were redundant once the construction rule was explicit.

## Experiment 33: Validation Runner Multiple Commands

Question: when patch plan/checklist names multiple required validation commands,
does validation runner run all of them and preserve a failing status from any
command?

Payload batch:

- `.pragma/prompt-ab/validation-runner-multiple-commands-20260602T183820Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_multi_first_action` | current validation runner after reading patch plan/checklist with two commands | ran both commands, but final `exit "$status2"` only preserved the second command's status | Failed |
| `validation_runner_all_commands_rule` | all-commands rule with per-command status requirement | ran both commands and exited nonzero if either failed; verbose but correct | Passed |
| `validation_runner_multi_shell_shape` | exact multi-command shell shape | compact loop, per-command logs/statuses, `overall=1` if any command fails | Passed |

Accepted phrase:

```text
Multiple validation commands rule:
If patch plan/checklist names more than one required validation command, run
every required command in the same validation-running shell script. Preserve and
echo each command's exit status separately as `EXIT_STATUS[1]: <status>`,
`EXIT_STATUS[2]: <status>`, and so on. The script's final exit status must be
nonzero if any required command fails. Do not stop after the first passing
command.
```

```text
Required multi-command shell shape:
set -o pipefail
overall=0
i=0
for cmd in '<command 1>' '<command 2>'; do
  i=$((i+1))
  log="/tmp/pragma/validation-$i.log"
  echo "COMMAND[$i]: $cmd"
  bash -lc "$cmd" 2>&1 | tee "$log"
  status=${PIPESTATUS[0]}
  echo "EXIT_STATUS[$i]: $status"
  if [ "$status" -ne 0 ]; then overall=1; fi
done
exit "$overall"
```

Learning:

- The current runner could run multiple commands but still hide a failure in an
  earlier command by exiting with only the last command's status.
- "Run every required command" is not sufficient by itself; the prompt must
  specify per-command statuses and combined failure status.
- The compact loop shape is safer than ad hoc two-command scripts because it
  scales with command count and keeps the "any failure fails validation" rule
  local to the shell script.

## Experiment 34: Multi-Command Validation Status Extraction

Question: after indexed multi-command validation output is visible, does
validation runner write status for every command and extract failures from the
nonzero command?

Payload batch:

- `.pragma/prompt-ab/validation-status-multi-command-20260602T184111Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_status_current_multi_output` | current validation runner after `COMMAND[1]`/`EXIT_STATUS[1]` and `COMMAND[2]`/`EXIT_STATUS[2]` output | wrote both commands and statuses and `failed_tests: TestRetryHelp`, but left `failed_packages: none` despite `FAIL ./cmd/pragma` | Partial |
| `validation_status_indexed_rule` | indexed status-writing rule | wrote both commands and statuses, extracted `failed_tests: TestRetryHelp` and `failed_packages: ./cmd/pragma`, but dropped bracketed status labels | Passed |
| `validation_status_indexed_template` | indexed rule plus multi-command status template | wrote `COMMAND[1]`/`EXIT_STATUS[1]`, `COMMAND[2]`/`EXIT_STATUS[2]`, `failed_tests: TestRetryHelp`, `failed_packages: ./cmd/pragma`, and both log paths | Passed |

Accepted phrase:

```text
Indexed status-writing rule:
When validation output contains `COMMAND[n]: ...` and `EXIT_STATUS[n]: ...`,
write every indexed command and status under Commands run. If any indexed
status is nonzero, record the failing command's tests/packages in failed_tests
or failed_packages. Do not collapse multiple indexed statuses into one status.
```

```text
Multi-command validation-status template:
Commands run:
- COMMAND[1]: <exact command>; EXIT_STATUS[1]: <status>
- COMMAND[2]: <exact command>; EXIT_STATUS[2]: <status>

failed_tests:
- <tests from any nonzero command, or none>

failed_packages:
- <packages from any nonzero command, or none>

unexpected_errors:
- <unexpected errors or none>

missing_files_or_surfaces:
- <missing surface or none>

Validation logs:
- /tmp/pragma/validation-1.log
- /tmp/pragma/validation-2.log
```

Learning:

- The current status writer could preserve indexed command statuses, but it
  missed the failed package field for the nonzero command.
- The indexed rule improved failure extraction; the explicit template preserved
  the indexed `COMMAND[n]`/`EXIT_STATUS[n]` shape as well.
- Multi-command validation needs two prompt controls: one for executing all
  commands with combined failure status, and another for writing status from
  indexed output.

## Experiment 35: Item Worker Failed Validation Report

Question: after item worker has attempted a patch and focused validation output
is visible with `EXIT_STATUS: 1`, what prompt shape makes it write
`/tmp/pragma/implementer-report.md` instead of continuing investigation?

Payload batch:

- `.pragma/prompt-ab/item-worker-failed-validation-report-20260602T184500Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_failed_validation_report` | current item worker after visible failed validation | inspected `cmd/pragma/jobs_test.go` instead of writing report | Failed |
| `item_worker_failed_validation_rule` | persona-level failed-validation report rule | inspected `cmd/pragma/jobs_test.go` | Failed |
| `item_worker_failed_validation_template` | persona-level failed-validation rule plus exact report shell shape | grepped source instead of writing report | Failed |
| `item_worker_validation_state_stop` | persona-level validation state variables and stop decision | inspected test file | Failed |
| `item_worker_validation_state_template_stop` | persona-level state variables plus exact report shell shape | grepped source | Failed |
| `item_worker_latest_validation_state` | latest-message state variables and required artifact | inspected source | Failed |
| `item_worker_latest_exact_report` | latest-message exact next-action contract with full report shell script | wrote implementer report with BLOCKED evidence, failed command/status, blocker, and sentinel | Passed |

Accepted control:

```text
Your next response must be exactly this shell script in one fenced bash block
and no other command:
cat > /tmp/pragma/implementer-report.md <<'EOF'
Current item:
<id and title>

Changed:
<files changed>

Acceptance evidence:
BLOCKED - <focused validation command> failed

Validation:
<focused validation command> failed with EXIT_STATUS: <status>

Remaining risk:
validation failure remains

Blocker:
<failing test and first failure signal>
EOF
echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT
```

Learning:

- Persona-level failed-validation stop rules were not enough for item worker.
  The model treated visible test failure as a reason to inspect tests/source and
  keep repairing.
- Latest-message state variables alone were also too weak in this trajectory.
- The successful control was a latest-message exact next-action contract with
  the complete report-writing shell script.
- This should not be added as a static item-worker persona rule from this
  evidence. It is an orchestration/footer control for the point where the
  runtime decides the item-worker phase must stop after failed validation.

## Experiment 36: Item Worker Passed Validation Report

Question: after item worker has attempted a patch and focused validation output
is visible with `PASS` and `EXIT_STATUS: 0`, what prompt shape makes it write a
complete `/tmp/pragma/implementer-report.md`?

Payload batch:

- `.pragma/prompt-ab/item-worker-passed-validation-report-20260602T184940Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_passed_validation_report` | current item worker after visible passing validation | wrote report and sentinel, but omitted `EXIT_STATUS: 0` and the required `Blocker: none` section | Partial |
| `item_worker_passed_validation_rule` | success report rule only | included status and blocker, but added an unnecessary `grep` before report | Partial |
| `item_worker_passed_validation_template` | success report rule plus exact shell shape | wrote complete report with PASS evidence, `EXIT_STATUS: 0`, `Remaining risk: none`, `Blocker: none`, no extra command | Passed |
| `item_worker_latest_exact_success_report` | latest-message exact report action | wrote complete report | Passed |
| `item_worker_passed_validation_template_only` | exact shell shape without success report rule | wrote complete report but added an unnecessary `grep` before report | Partial/rejected |

Accepted phrase:

```text
Passed validation report rule:
If focused validation output is visible and contains the exact validation
command, PASS or ok output, and EXIT_STATUS: 0, write
/tmp/pragma/implementer-report.md now. Acceptance evidence must name the passed
validation command. Validation must include EXIT_STATUS: 0. Remaining risk must
be none. Blocker must be none. Do not inspect more files or continue editing
after focused validation passed.
```

```text
Passed validation report shell shape:
cat > /tmp/pragma/implementer-report.md <<'EOF'
Current item:
<id and title>

Changed:
<files changed>

Acceptance evidence:
PASS - <acceptance passed by focused validation>

Validation:
<exact validation command> passed with EXIT_STATUS: 0

Remaining risk:
none

Blocker:
none
EOF
```

Learning:

- Current item worker already stopped after passing validation, but produced an
  incomplete report that could weaken item review.
- The success report rule alone made the report complete but allowed an extra
  inspection command.
- The shell shape alone made the report complete but also allowed an extra
  inspection command.
- The combination of semantic success rule plus exact report shell shape was
  the clean static persona fix. Unlike failed-validation stop, this does not
  need to be only a latest-message orchestration footer in this replay.

## Experiment 37: Item Reviewer Go `ok` Output Approval

Question: if an implementer report contains the exact focused validation command,
Go-style `ok <package>` output, and `EXIT_STATUS: 0`, but no literal `PASS`
line in the validation section, does item reviewer approve without rerunning?

Payload batch:

- `.pragma/prompt-ab/item-review-ok-output-pass-20260602T185322Z`

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_ok_output` | current item reviewer; implementer report has `ok ./cmd/pragma` and `EXIT_STATUS: 0` | wrote APPROVE verdict, no validation rerun | Passed |
| `item_reviewer_pass_or_ok_rule` | visible-pass rule changed from PASS-only to PASS-or-ok | also approved; more verbose findings | Passed/redundant |
| `item_reviewer_pass_or_ok_with_go_rule` | PASS-or-ok plus explicit Go ok-output rule | approved with compact ok/status findings | Passed/redundant |

Accepted existing behavior:

```text
Visible pass rule:
If the implementer report already contains the exact focused validation command,
PASS, and EXIT_STATUS: 0, write the verdict artifact now. Do not rerun the
validation before writing APPROVE.
```

Learning:

- Despite the PASS-specific wording, current item reviewer generalized to
  Go-style `ok <package>` output plus `EXIT_STATUS: 0`.
- No persona change is retained from this replay. The explicit ok-output
  variants did not improve behavior enough to justify more prompt text.
- Keep watching this if other languages/tools are introduced; this replay only
  proves the Go `ok` shape.

## Experiment 38: Item Reviewer Forbidden Changed File

Question: if an implementer report contains exact focused validation success but
also lists a forbidden file under `Changed`, does item reviewer block the item?

Payload batches:

- `.pragma/prompt-ab/item-review-forbidden-changed-file-20260602T185821Z`
- `.pragma/prompt-ab/item-review-scope-before-pass-20260602T185937Z`

Fixture:

- current item allowed `cmd/pragma/jobs.go`,
- current item forbade `internal/jobs/runner.go` and `internal/jobs/**`,
- implementer report listed both files under `Changed`,
- implementer report also claimed `PASS` and `EXIT_STATUS: 0` for the exact
  focused validation command.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_forbidden_changed_file` | current item reviewer | approved based only on focused validation success | Failed |
| `item_reviewer_changed_file_scope_rule` | additive changed-file scope rule before weak-evidence rule | still approved based only on focused validation success | Failed |
| `item_reviewer_scope_rule_with_block_shape` | additive scope rule plus BLOCK shape | still approved based only on focused validation success | Failed |
| `item_reviewer_scope_authority_rule` | short additive authority rule | still approved based only on focused validation success | Failed |
| `item_reviewer_conditional_visible_pass` | rewrote visible-pass rule so scope is checked before validation success | blocked `internal/jobs/runner.go` as forbidden | Passed |
| `item_reviewer_scope_gate_order` | added gate order plus conditional visible-pass rule | blocked forbidden file | Passed |
| `item_reviewer_scope_gate_order_and_shape` | gate order, conditional pass, and exact scope BLOCK shape | blocked forbidden file | Passed/redundant |

Accepted phrase:

```text
Visible pass rule:
Before applying validation success, check the implementer report Changed
section against current item allowed_files and forbidden_files. Validation
success can approve only in-scope changes. If any changed file is outside
allowed_files or matches forbidden_files, write BLOCK even when the exact
focused validation command, PASS, and EXIT_STATUS: 0 are visible. If file scope
passes and the implementer report already contains the exact focused validation
command, PASS, and EXIT_STATUS: 0, write the verdict artifact now. Do not rerun
the validation before writing APPROVE.
```

Learning:

- A separate additive scope rule did not beat the existing unconditional
  validation-success approval rule.
- The reliable control was to make the approval rule itself conditional on
  scope passing.
- Longer gate-order and BLOCK-shape variants were redundant once the visible
  pass rule carried the scope precondition.
- Prompt ordering alone is weaker than rewriting the rule that creates the bad
  attractor.

## Experiment 39: Final Reviewer Test-Only Diff

Question: if validation is clean but the visible diff changes only a test file
while validation-status names a production source surface, does final reviewer
block the proof-only patch?

Payload batch:

- `.pragma/prompt-ab/final-review-test-only-diff-20260602T190229Z`

Fixture:

- validation status had `EXIT_STATUS: 0`,
- `validated_surfaces` named `cmd/pragma/jobs.go`,
- failure variables were all `none`,
- visible `git diff` changed only `cmd/pragma/jobs_test.go`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_test_only_diff` | current final reviewer with diff hygiene rule naming test-only changes | approved because the test file directly tested the validated source surface | Failed |
| `final_reviewer_test_only_rule` | direct test-only diff rule, but also named fixtures too broadly | blocked the test-only patch | Passed/risky |
| `final_reviewer_approval_precondition` | approval requires non-test source/config/fixture change tied to validation-status | blocked the test-only patch | Passed |
| `final_reviewer_proof_only_rule` | proof-only rule scoped to tests/snapshots/validation expectations when production/config/source surfaces are unchanged | blocked the test-only patch | Passed/accepted |

Accepted phrase:

```text
Proof-only diff rule:
If validation-status names required production/config/source surfaces and the
visible diff changes only tests, snapshots, or validation expectations while
those required surfaces are unchanged, write BLOCK. A patch that only changes
the proof cannot satisfy the task.
```

Learning:

- Listing `test-only` inside a broad diff-hygiene rule was not enough. The
  reviewer rationalized the test change as directly tied to the validated source
  surface and approved.
- The reliable framing is causal: changing only the proof does not satisfy a
  task whose required source/config surface is unchanged.
- Avoid phrasing that treats fixtures as always proof-only; fixture/testdata
  files can be valid task surfaces when validation-status names them.
- Approval preconditions can work, but the narrower proof-only rule preserves
  legitimate fixture/config tasks better.

## Experiment 40: Generated File Producer Command Available

Question: if the current item targets generated/derived files and names an
explicit `producer_command`, does item worker run that producer before source
inspection, generated-file inspection, hand editing, or blocker reporting?

Payload batch:

- `.pragma/prompt-ab/item-worker-producer-command-available-20260602T191018Z`

Fixture:

- current item targeted generated protobuf/gateway files,
- current item named `producer_command: buf generate`,
- no producer output was visible yet,
- expected first action was the named producer command with status capture.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_producer_available` | current generated-file policy | wrote prose and checked `which buf && buf --version` | Failed |
| `item_worker_producer_available_policy` | simple run-producer policy | wrote prose, prepended `cd /tmp/pragma`, omitted status capture | Failed |
| `item_worker_exact_producer_next_action` | exact producer status-capture rule | ran producer with status capture but prepended `cd /tmp/pragma` | Partial/rejected |
| `item_worker_exact_producer_good_bad` | exact rule plus hardcoded `buf generate` good/bad example | ran exact command with status capture | Passed/rejected as task-specific |
| `item_worker_exact_copy_no_cd` | copy `producer_command` exactly, no `cd /tmp/pragma`, status capture | ran exact command with status capture | Passed/accepted |
| `item_worker_placeholder_good_bad_no_cd` | accepted rule plus generic placeholder good/bad examples | ran exact command with status capture | Passed/redundant |

Accepted phrase:

```text
Generated file policy:
If the current item targets generated or derived files and includes
producer_command, do not hand-edit generated files. If the current item also
names a non-generated source file that must change, inspect and patch that
source file before running producer_command. Run producer_command only after the
source change is visible in this phase. If no source change is required or the
source change is already visible, copy producer_command exactly from the current
item and run it from the current working directory with:
<producer_command>; status=$?; echo EXIT_STATUS: $status; exit $status
Do not prepend `cd /tmp/pragma`, do not check whether the command exists, do
not inspect generated files, do not search for producers, and do not write a
blocker report before the named producer_command has been attempted.

Generated file no-producer policy:
If the current item targets generated or derived files and neither
producer_command nor successful producer command is visible, write
/tmp/pragma/implementer-report.md with a Blocker section. Do not search for
producers in this state.
```

Learning:

- "No successful producer command is visible" was too ambiguous: it did not
  distinguish a named-but-untried producer from an absent producer.
- A simple "run the producer" rule was not enough; the model still inserted
  prose, changed directories to `/tmp/pragma`, and omitted status capture.
- The reliable control copies the `producer_command` value exactly, forbids
  `cd /tmp/pragma`, and gives the status-capture shell shape.
- Hardcoded good/bad examples can work but should be rejected when a generic
  copy-exact rule passes.

## Experiment 41: Successful Producer Command To Validation

Question: after a generated/derived-file producer command succeeds, does item
worker run focused validation with status capture instead of inspecting
generated files, hand-editing, or writing the report prematurely?

Payload batch:

- `.pragma/prompt-ab/item-worker-producer-success-validation-20260602T191420Z`

Fixture:

- current item targeted generated protobuf/gateway files,
- current item named `producer_command: buf generate`,
- `buf generate` output was visible with `EXIT_STATUS: 0`,
- current item named `validation_command: go test ./internal/server/auth/...`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_producer_success` | current item worker after successful producer output | ran `go test ./internal/server/auth/...` with status capture | Passed |
| `item_worker_successful_producer_validation_rule` | added semantic successful-producer validation rule | same validation command with status capture | Passed/redundant |
| `item_worker_successful_producer_validation_exact` | added exact validation shell shape | same validation command with status capture | Passed/redundant |
| `item_worker_successful_producer_validation_good_bad` | exact shape plus good/bad examples | same validation command with status capture | Passed/redundant |

Accepted existing behavior:

```text
Work method:
4. Run focused validation only for this item when runnable.
```

Learning:

- Once the producer command has visibly succeeded, the existing work method is
  enough in this compact replay to move to focused validation.
- No additional successful-producer rule is retained. Extra wording would
  duplicate behavior already produced by the current prompt.
- The generated-file controls now form a three-state contract: named producer
  runs first, failed producer blocks, successful producer proceeds to focused
  validation.

## Experiment 42: Evidence Mapper Generated Unknowns

Question: after source/defaulting and fixture/test evidence are already visible,
does evidence mapper write `/tmp/pragma/evidence-map.md` and keep tempting
runtime/proto/generated surfaces under `Not evidenced`, or does it continue
reading/searching?

Payload batch:

- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-20260602T191720Z`

Fixture:

- `/tmp/pragma/surface-map.md` named config/test/fixture surfaces,
- the next tool output already showed config structs, config test, and fixture
  content,
- explicit unknowns included `internal/server/auth/method/kubernetes/server.go`
  and generated `rpc/auth.pb.go`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `evidence_mapper_current_generated_unknowns` | current evidence mapper | reread `testdata/authentication/kubernetes.yml` | Failed |
| `evidence_mapper_explicit_unknowns_rule` | explicit unknowns are not adjacent surfaces | reread fixture and grepped config with `head` | Failed |
| `evidence_mapper_completion_unknowns_rule` | unknowns rule plus semantic completion rule | reread fixture | Failed |
| `evidence_mapper_completion_gate` | work-method completion gate | reread fixture and grepped fixture | Failed |
| `evidence_mapper_completion_gate_shape` | completion gate plus exact evidence-map shell shape | grepped source and used `head`; did not write artifact | Failed |
| `evidence_mapper_state_completion_shape` | completion state variables, required artifact, exact shell shape | wrote evidence map and put runtime/proto/generated paths under `Not evidenced` | Passed |

Accepted phrase:

```text
Evidence completion state:
source/defaulting evidence = <visible or missing>
fixture/test evidence = <visible or missing>
focused validation evidence = <visible or missing>
explicit_unknowns = must_not_inspect
required_artifact = /tmp/pragma/evidence-map.md

Completion decision rule:
When source/defaulting evidence, fixture/test evidence, and focused validation
evidence are visible, write required_artifact now. Do not inspect more files.
Put explicit unknowns and generated/runtime/proto surfaces under
`Not evidenced`.
```

Learning:

- The older evidence-mapper unknown rule was not enough for this replay. Current
  prompt avoided generated/runtime chasing but still repeated source/fixture
  reads.
- A semantic completion rule and even an exact artifact shell shape were still
  too weak without explicit state variables.
- The reliable pattern matches earlier loop-stop controls: name the evidence
  state, set `explicit_unknowns = must_not_inspect`, set `required_artifact`,
  then define the completion decision.
- The useful failure mode here is not only broad generated/runtime search; it
  is repeated local reads that delay artifact handoff after enough evidence is
  already visible.

## Experiment 43: Final Reviewer Validated Generated Diff

Question: does final reviewer over-block generated-file diffs when generated
files are explicitly named as validated surfaces and successful producer
evidence is visible?

Payload batch:

- `.pragma/prompt-ab/final-review-generated-validated-diff-20260602T192132Z`

Fixture:

- validation status included `Producer evidence: buf generate; EXIT_STATUS: 0`,
- focused validation had `EXIT_STATUS: 0`,
- `validated_surfaces` named `rpc/auth.proto`, `rpc/auth.pb.go`, and
  `rpc/auth.pb.gw.go`,
- visible diff changed only those proto/generated surfaces.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_generated_validated_diff` | current final reviewer with diff hygiene and proof-only rules | approved; treated generated files as tied to validated surfaces and producer evidence | Passed |
| `final_reviewer_generated_validated_rule` | explicit generated diff allowance rule | approved with more detailed generated-surface findings | Passed/redundant |
| `final_reviewer_generated_approval_precondition` | explicit generated approval precondition | approved with more detailed producer/validation findings | Passed/redundant |

Accepted existing behavior:

```text
Diff hygiene rule:
... compare changed files and hunks in `git diff --stat` and `git diff -- .`
with the validated surfaces named in validation-status. APPROVE only when every
changed file and hunk is directly tied to those surfaces.
```

Learning:

- The existing final-review diff hygiene rule is not a blanket generated-file
  ban in this compact replay.
- When generated files are explicitly named in `validated_surfaces` and producer
  evidence is clean, current final reviewer approves without extra prompt text.
- Do not add generated-file allowance text unless a future replay shows
  overblocking; the current rule already balances unsafe generated diffs against
  validated producer-backed generated diffs.

## Experiment 44: Checklist Generated Producer Handoff

Question: when a patch plan contains generated/derived files plus a producer
command, does checklist writer preserve `producer_command`, `generated_files`,
and `validation_command` as machine-readable item fields for item worker?

Payload batch:

- `.pragma/prompt-ab/checklist-generated-producer-handoff-20260602T192405Z`

Fixture:

- patch plan had one patch-route bullet with `source path: rpc/auth.proto`,
  generated files `rpc/auth.pb.go`, `rpc/auth.pb.gw.go`,
  `producer command: buf generate`, and validation
  `go test ./internal/server/auth/...`,
- patch plan also had one unresolved runtime-auth behavior requiring proof.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_generated_producer` | current checklist writer after patch-plan read but no verdict state | checked `/tmp/pragma/final-prosecutor-verdict.md` again | Failed/not artifact turn |
| `checklist_producer_shape_fields` | schema allows producer fields but no visible no-verdict state | checked final verdict path again | Failed/not artifact turn |
| `checklist_producer_handoff_rule` | producer handoff rule plus schema fields | preserved producer fields, but split source edit, generated output, and validation into separate checklist items | Partial |
| `checklist_current_generated_producer_after_no_verdict` | current prompt with `NO_VERDICT_FILE` and patch plan visible | wrote checklist but hid producer command in prose and split validation into a separate item | Failed |
| `checklist_producer_handoff_after_no_verdict` | producer handoff rule plus schema fields | preserved `producer_command`, `generated_files`, and `validation_command`; still created a separate blocker item for unresolved runtime behavior | Passed |
| `checklist_producer_single_item_after_no_verdict` | handoff rule plus patch-route preservation and no-verdict rule | preserved source/generated/producer/validation on one item | Passed |
| `checklist_producer_single_item_no_noverdict_rule` | handoff rule plus patch-route preservation, no no-verdict rule | preserved source/generated/producer/validation on one item | Passed/accepted |

Accepted phrases:

```text
Generated producer handoff rule:
If a patch-route item includes generated/derived files and a producer command,
the checklist item must include:
- `producer_command`: exact producer command from the patch plan
- `generated_files`: exact generated/derived files from the patch plan
- `validation_command`: exact focused validation command from the patch plan
Do not hide producer commands inside description, approach, or acceptance.

Patch-route item preservation rule:
Preserve one patch-route bullet as one checklist item unless the patch plan
explicitly splits it. Do not split producer, generated files, and validation
into separate checklist items when they belong to the same patch-route bullet.
Put source path, generated files, producer command, and validation command on
that same item.
```

Learning:

- Current checklist writer could mention producer commands in prose but lost the
  machine-readable fields item worker needs.
- Merely allowing producer fields in the schema was not enough in the first
  replay because the model rechecked final-verdict existence before writing.
- Producer handoff fixed field preservation, but without patch-route
  preservation the model split one route into source, generated, and validation
  items.
- The retained control is the combination of producer-field handoff plus
  patch-route item preservation. The visible no-verdict rule was unnecessary
  once the artifact-turn payload already included `NO_VERDICT_FILE`.

## Experiment 45: Patch Planner Generated Producer Handoff

Question: when evidence map names generated/derived files and a producer command,
does patch planner carry them into one patch-route item for checklist writer?

Payload batch:

- `.pragma/prompt-ab/patch-planner-generated-producer-handoff-20260602T192939Z`

Fixture:

- evidence map named `rpc/auth.proto` as source,
- evidence map named generated files `rpc/auth.pb.go`,
  `rpc/auth.pb.gw.go`,
- evidence map named producer command `buf generate`,
- validation was `go test ./internal/server/auth/...`,
- runtime auth server behavior was `Not evidenced`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_planner_current_generated_producer` | current patch planner | lost producer command and split proto, pb.go, and gateway output into separate routes | Failed |
| `patch_planner_generated_format` | format includes `producer command` and `generated files` | preserved fields but added a second config route not needed by the producer route | Partial |
| `patch_planner_generated_route_rule` | generated producer route rule plus format fields | wrote one route with source, generated files, producer command, validation, and unresolved runtime behavior | Passed |

Accepted phrase:

```text
Generated producer route rule:
If evidence-map names generated/derived paths and a producer command for an
evidenced source path, keep them on the same patch-route item. The patch route
must include `producer command:` and `generated files:` exactly from evidence.
Do not hide producer information in prose, and do not split generation or
validation into separate patch-route items.
```

Learning:

- Current patch planner could route generated work semantically, but it lost the
  producer command and split generated outputs into separate items.
- Adding fields to the format preserved metadata but did not prevent route
  proliferation.
- The reliable control pairs schema fields with a route-preservation rule tied
  to generated/derived paths plus producer command.
- This closes the upstream side of the generated-file handoff: evidence map can
  name producer/generated surfaces, patch planner preserves them, checklist
  writer copies them, and item worker executes them.

## Experiment 46: Item Worker Source Before Producer

Question: when a single checklist item contains a non-generated source file,
generated files, and `producer_command`, does item worker inspect/patch the
source file before running the producer?

Payload batch:

- `.pragma/prompt-ab/item-worker-source-before-producer-20260602T193248Z`

Fixture:

- current item named `source_files: ["rpc/auth.proto"]`,
- `allowed_files` included `rpc/auth.proto`, `rpc/auth.pb.go`, and
  `rpc/auth.pb.gw.go`,
- current item named `producer_command: buf generate`,
- current item named generated files and validation command.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_source_before_producer` | current generated-file policy from Experiment 40 | semantically inspected `rpc/auth.proto` first, but leaked prose outside the bash block and contradicted the policy text | Partial |
| `item_worker_source_before_producer_policy` | source-before-producer policy | responded with exactly `cat rpc/auth.proto` in one fenced bash block | Passed |
| `item_worker_ordered_source_to_producer` | explicit numbered source-to-producer order | inspected source first but leaked prose | Partial/rejected |

Accepted phrase:

```text
Generated file policy:
If the current item targets generated or derived files and includes
producer_command, do not hand-edit generated files. If the current item also
names a non-generated source file that must change, inspect and patch that
source file before running producer_command. Run producer_command only after the
source change is visible in this phase. If no source change is required or the
source change is already visible, copy producer_command exactly from the current
item and run it from the current working directory with:
<producer_command>; status=$?; echo EXIT_STATUS: $status; exit $status
Do not prepend `cd /tmp/pragma`, do not check whether the command exists, do
not inspect generated files, do not search for producers, and do not write a
blocker report before the named producer_command has been attempted.
```

Learning:

- Experiment 40's producer-first wording was too broad for combined
  source-plus-generated checklist items.
- The model's underlying behavior wanted to inspect the source first, but the
  prompt text contradicted that and caused response-shape leakage in replay.
- The reliable wording preserves the no-hand-edit generated-file rule while
  making producer execution conditional on source-change state.
- A numbered ordering rule was semantically correct but leaked prose; the
  compact policy rewrite was cleaner.

## Experiment 47: Item Worker Source Change Exactness

Question: after the source file is visible, if the item requires a source edit
but does not name exact source symbols, field names, field types, field numbers,
or source lines, does item worker block instead of searching or inventing
fields?

Payload batch:

- `.pragma/prompt-ab/item-worker-source-change-exactness-20260602T193828Z`

Fixture:

- current item said to add generic "cleanup fields" to `rpc/auth.proto`,
- current item did not name exact proto field names, types, or numbers,
- `rpc/auth.proto` was visible and lacked those fields,
- expected safe behavior was an implementer blocker report with no source edit,
  no generated-file inspection, and no producer command.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_missing_source_exactness` | current source-before-producer prompt | inspected generated file with `head` | Failed |
| `item_worker_source_exactness_rule` | semantic "do not invent source fields" rule | wrote a correct blocker, but leaked prose outside the bash block | Partial/rejected |
| `item_worker_source_exactness_shape` | semantic rule plus blocker shell shape | wrote a correct blocker, but leaked prose outside the bash block | Partial/rejected |
| `item_worker_source_exactness_state_shape` | explicit state variables, required artifact, blocker shell shape | wrote blocker report with no prose, no search, no producer, no invented fields | Passed |
| `item_worker_source_exactness_conditional_shape` | conditional exact-response rule without state variables | inspected generated file | Failed |

Accepted control shape for latest-message/state footer use:

```text
Current source-edit state:
source_file_visible = true
exact_source_change_visible = false
producer_command_attempted = false
required_artifact = /tmp/pragma/implementer-report.md

Source exactness decision rule:
When source_file_visible is true and exact_source_change_visible is false,
write required_artifact with a Blocker section now. Do not run grep, find,
source search, producer_command, validation, or a source patch.
```

Learning:

- Current source-before-producer control prevents premature producer execution
  but does not prevent broad/generated inspection when the source edit is
  underspecified.
- A static "do not invent fields" rule can produce correct semantics, but it
  leaked prose and is therefore not reliable enough to retain as a persona rule
  by itself.
- A conditional exact-response rule without explicit state variables failed and
  inspected generated output.
- The reliable control is a latest-message/state footer naming
  `exact_source_change_visible = false`, `required_artifact`, and forbidden next
  actions. This matches earlier findings that uncertain stop conditions need
  explicit state, not broad cautionary prose.

## Experiment 48: Item Worker Exact Source Edit Present

Question: when the item supplies exact source changes and the source file is
already visible, does item worker patch only the named non-generated source file
before running producer or validation commands?

Payload batch:

- `.pragma/prompt-ab/item-worker-exact-source-edit-20260602T194214Z`

Fixture:

- current item targeted `rpc/auth.proto` plus generated outputs,
- `exact_source_changes` named two precise proto fields,
- `allowed_files` included `rpc/auth.proto`, `rpc/auth.pb.go`, and
  `rpc/auth.pb.gw.go`,
- `producer_command` was `buf generate`,
- source file content was already visible and lacked the fields,
- expected next action was one fenced bash block patching only `rpc/auth.proto`,
  with no producer command, validation, generated-file inspection, or report.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_exact_source_edit` | current source-before-producer prompt | patched only `rpc/auth.proto` with the two exact fields | Passed |
| `item_worker_exact_source_changes_rule` | added static exact-source-changes rule | same correct `rpc/auth.proto` patch | Passed |
| `item_worker_exact_source_changes_state` | static rule plus explicit source-edit state footer | same correct `rpc/auth.proto` patch | Passed |

Learning:

- The current item worker prompt already handles the positive counterpart of
  Experiment 47: exact source changes present plus source visible leads to a
  bounded source patch before producer execution.
- Additional static exact-source wording did not improve the output for this
  fixture, so no persona change was retained.
- The split is now evidence-backed: exact source-change details present can be
  handled by the static item-worker policy, while exact details absent still
  needs latest-message state control to stop search/invention.

## Experiment 49: Item Worker Source Preservation

Question: when exact source changes are present and the visible source file has
unrelated comments, options, fields, and messages, does item worker preserve
that unrelated context while making a bounded source patch?

Payload batch:

- `.pragma/prompt-ab/item-worker-preserve-context-20260603T000000Z`

Fixture:

- current item was the same generated-output route as Experiment 48,
- `exact_source_changes` named two precise proto fields,
- visible `rpc/auth.proto` included `go_package`, comments, an existing
  `provider` field, and an unrelated `ExistingConfig` message,
- expected next action was a patch to `rpc/auth.proto` only, preserving all
  unrelated visible content, with no producer command, generated-file
  inspection, validation, or report.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_preserve_context` | current source-before-producer prompt | used `sed -i` to append fields, preserving unrelated context, but chained `&& cat rpc/auth.proto` after the patch | Partial |
| `item_worker_preserve_context_rule` | static source preservation rule before patch minimality | emitted one bounded `sed -i` patch inside `message AuthConfig`; no post-patch read, producer, validation, generated-file inspection, or report | Passed |
| `item_worker_preserve_context_state` | latest-message preservation state footer | preserved all visible content but rewrote the entire file from memory with `cat > rpc/auth.proto` | Partial/rejected |

Accepted item-worker persona rule:

```text
Source preservation rule:
When editing a visible source file, preserve every unrelated visible line and
declaration exactly. Patch only the smallest source region required for the
current item. Do not rewrite the file from memory, remove comments, remove
options/imports/packages, remove unrelated declarations, or normalize formatting
unless the current item explicitly requires it.
```

Learning:

- The base prompt had the right semantic target but still added a post-patch
  read. The retained static rule narrowed the next action to the edit itself.
- A latest-message footer can over-constrain semantics while still permitting a
  full-file rewrite. For preservation, the static rule closer to patch
  minimality was stronger than state-footer wording.
- Preservation wording should be about unrelated visible lines/declarations and
  smallest source region, not about a task-specific file type or generator.

## Experiment 50: Item Worker Post-Source-Patch Producer

Question: after a generated-file item's required non-generated source patch is
visibly present, does item worker run the named producer command exactly with
status propagation before re-reading files, inspecting generated output,
validating, or writing a report?

Payload batch:

- `.pragma/prompt-ab/item-worker-post-source-patch-producer-20260603T000000Z`

Fixture:

- current item was the generated-output route from Experiments 48 and 49,
- source file had been inspected,
- a source patch command had visibly added the two exact proto fields and shown
  the patched `rpc/auth.proto`,
- `producer_command` was `buf generate`,
- expected next action was exactly:

```bash
buf generate; status=$?; echo EXIT_STATUS: $status; exit $status
```

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_post_source_patch` | current item-worker prompt after Experiment 49 | ran `buf generate` and echoed status, but omitted `exit $status` | Partial |
| `item_worker_post_source_patch_rule` | static post-source-patch producer rule | ran exact producer wrapper with `exit $status` | Passed |
| `item_worker_post_source_patch_state` | latest-message producer handoff state footer | ran exact producer wrapper with `exit $status` | Passed/redundant |

Accepted item-worker persona rule:

```text
Post-source-patch producer rule:
If current item includes producer_command and the required non-generated source
change is visible in this phase, run producer_command exactly next using the
required status wrapper. Do not re-read source files, inspect generated files,
run validation, or write the implementer report before producer_command has
been attempted.
```

Learning:

- The base prompt had the correct state transition but not the exact command
  shape; it dropped `exit $status`, which would hide producer failure from the
  shell status.
- Repeating the producer transition near the source-patch state fixed command
  shape better than relying on the earlier generated-file policy alone.
- Latest-message state also passed, but the static rule was sufficient and
  cheaper to maintain for this recurring state transition.

## Experiment 51: Item Worker Failed Validation Retest

Question: after the current item-worker prompt improvements, can a stronger
static failed-validation rule make item worker write
`/tmp/pragma/implementer-report.md` after visible focused validation failure, or
does this still require a latest-message exact report script?

Payload batch:

- `.pragma/prompt-ab/item-worker-failed-validation-report-current-20260603T000000Z`

Fixture:

- reused the known Experiment 35 trajectory,
- current item allowed only `cmd/pragma/jobs.go`,
- item worker had already patched `cmd/pragma/jobs.go`,
- focused validation output was visible:
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault`, `--- FAIL`, and
  `EXIT_STATUS: 1`,
- expected terminal behavior was to write
  `/tmp/pragma/implementer-report.md` with BLOCKED evidence and sentinel.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_failed_validation_retest` | current item-worker prompt after Experiments 49 and 50 | read `cmd/pragma/jobs_test.go` | Failed |
| `item_worker_failed_validation_terminal_rule` | static terminal focused-validation failure rule | read `cmd/pragma/jobs_test.go` | Failed |
| `item_worker_failed_validation_terminal_shape` | static terminal rule plus exact failed-report shell shape | read `cmd/pragma/jobs_test.go` | Failed |
| `item_worker_failed_validation_latest_exact` | latest-message exact report-writing shell script | wrote implementer report with BLOCKED evidence and sentinel | Passed |

Learning:

- Current item-worker improvements did not change the failed-validation
  trajectory: visible failure still attracts test/source inspection.
- A stronger static rule placed near the passed-validation rule still failed,
  even with an exact shell shape available in the persona prompt.
- Failed focused-validation stop remains a latest-message exact-next-action
  control. Do not add static failed-validation stop wording to item-worker YAML
  from this evidence.

## Experiment 52: Item Reviewer Contradictory Validation

Question: when the implementer report claims PASS and `EXIT_STATUS: 0` but the
same visible validation text contains `--- FAIL`, does item reviewer block from
the raw failure signal instead of approving from summary fields?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-contradictory-validation-20260603T000000Z`

Fixture:

- current item required
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
- implementer report changed only allowed file `cmd/pragma/jobs.go`,
- report claimed:
  `PASS - go test ./cmd/pragma/... -run TestMaxRetriesDefault passed`,
- report validation text also contained
  `--- FAIL: TestMaxRetriesDefault`, `expected default 3, got 0`, and
  `EXIT_STATUS: 0`,
- expected behavior was `Decision: BLOCK` from the raw failure line.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_contradictory_validation` | current item reviewer prompt | wrote BLOCK verdict naming visible `--- FAIL` despite PASS/EXIT_STATUS:0 claims | Passed |
| `item_reviewer_contradiction_priority_rule` | explicit contradiction-priority rule | wrote BLOCK verdict naming contradictory evidence | Passed/redundant |
| `item_reviewer_contradiction_template_rule` | contradiction-priority plus exact decision guidance | wrote BLOCK verdict naming contradiction | Passed/redundant |

Learning:

- Current item reviewer already gives raw visible failure lines priority over
  contradictory PASS summaries.
- Additional contradiction-priority wording did not improve the behavior and is
  not retained.
- The existing `Visible validation failure rule` is sufficient for item-review
  contradictory validation in this compact replay.

## Experiment 53: Item Reviewer Ambiguous Changed Scope

Question: when validation is green but the implementer report `Changed` section
is vague instead of listing concrete repository paths, does item reviewer block
because file scope cannot be audited against `allowed_files` and
`forbidden_files`?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-ambiguous-changed-scope-20260603T000000Z`

Fixture:

- current item allowed only `cmd/pragma/jobs.go`,
- current item forbade `cmd/pragma/jobs_test.go` and
  `internal/jobs/runner.go`,
- implementer report `Changed` section said:
  `Updated the retry default implementation and related local files.`,
- validation text showed `ok` and `EXIT_STATUS: 0`,
- expected behavior was `Decision: BLOCK` because the changed files were not
  auditable.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_ambiguous_changed` | current item reviewer prompt | returned empty content with `finish_reason: length` | Failed |
| `item_reviewer_changed_auditability_rule` | static changed-section auditability rule | wrote BLOCK verdict naming vague "related local files" and validation/scope conflict | Passed |
| `item_reviewer_changed_auditability_template` | auditability rule plus exact decision guidance | wrote BLOCK verdict; similar to static rule | Passed/redundant |

Accepted item-reviewer persona rule:

```text
Changed-section auditability rule:
Validation success can approve only when the implementer report Changed section
names concrete repository file paths. If Changed is missing, vague, prose-only,
says related/local files, or cannot be compared exactly to allowed_files and
forbidden_files, write Decision: BLOCK even when validation passed. Do not
inspect git diff to compensate for an unauditable implementer report.
```

Learning:

- The visible-pass scope rule needs an auditability precondition, not just a
  check for explicitly out-of-scope listed files.
- The base prompt did not merely approve incorrectly; it got stuck until output
  length on this ambiguity. The explicit auditability rule made the decision
  small and finite.
- Adding a second exact-decision rule was unnecessary once the auditability rule
  named the missing proof and the forbidden fallback.

## Experiment 54: Item Reviewer No Files Changed Pass

Question: when an implementation item explicitly asks for a repository edit but
the implementer report says `Changed: No files changed`, does item reviewer
write a BLOCK verdict even if focused validation is green?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-no-files-changed-pass-20260603T000000Z`

Fixture:

- current item asked to change `cmd/pragma/jobs.go`,
- `allowed_files` contained only `cmd/pragma/jobs.go`,
- implementer report said `Changed: No files changed`,
- report also claimed focused validation passed with `EXIT_STATUS: 0`,
- expected behavior was `Decision: BLOCK` because the report provides no
  changed-file proof for the requested implementation.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_no_files_changed_pass` | current reviewer after Experiment 53 | identified the issue, but wrote prose outside the bash block and tried to inspect `cmd/pragma/jobs.go` | Failed |
| `item_reviewer_no_change_implementation_rule` | static no-change implementation report rule | wrote BLOCK verdict naming requested edit and missing changed-file proof | Passed |
| `item_reviewer_no_change_implementation_template` | no-change rule plus exact decision guidance | wrote similar BLOCK verdict | Passed/redundant |

Accepted item-reviewer persona rule:

```text
No-change implementation report rule:
If the current item asks to edit, change, add, remove, update, or fix repository
code/config and the implementer report says Changed: No files changed,
validation success cannot approve the item. Write Decision: BLOCK because the
report contains no changed-file proof for the requested implementation. Do not
inspect git diff to compensate for a no-change implementer report.
```

Learning:

- The changed-section auditability rule did not cover the special string
  `No files changed`; current prompt recognized the inconsistency but violated
  response shape and attempted repository inspection.
- No-change implementation reports need a separate static rule because
  `No files changed` is syntactically clear but semantically incompatible with
  an implementation item.
- The exact-decision template was unnecessary once the static rule named the
  item/action condition and forbade git-diff compensation.

## Experiment 55: Final Reviewer Missing Validated Surfaces

Question: after clean validation and visible diff evidence, does final reviewer
block when `validation-status` does not name concrete `validated_surfaces` for
the changed production/config/source files?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-missing-validated-surfaces-20260603T000000Z`

Fixture:

- validation status had one passing command:
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault EXIT_STATUS: 0`,
- all failure fields were `none`,
- `validated_surfaces` was `none`,
- visible diff changed `cmd/pragma/jobs.go`,
- expected behavior was `Decision: BLOCK` because the diff could not be tied to
  a named validated surface.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_missing_validated_surfaces` | current final reviewer prompt | approved the `cmd/pragma/jobs.go` change from test command plus diff relevance despite `validated_surfaces: none` | Failed |
| `final_reviewer_validated_surface_requirement` | static validated-surface requirement rule | wrote BLOCK verdict naming missing `validated_surfaces` proof for `cmd/pragma/jobs.go` | Passed |
| `final_reviewer_validated_surface_template` | requirement rule plus exact decision guidance | wrote similar BLOCK verdict | Passed/redundant |

Accepted final-reviewer persona rule:

```text
Validated-surface requirement rule:
After clean validation and visible diff evidence, APPROVE only if
validation-status names concrete validated_surfaces that include every changed
production/config/source file. If validated_surfaces is missing, empty, none,
vague, or does not name a changed file, write BLOCK even when validation passed
and the diff looks relevant. Do not infer validated surfaces from test command
names or diff paths.
```

Learning:

- Diff relevance and a passing test command are not enough for final approval;
  the final reviewer needs explicit validated-surface proof.
- The current prompt inferred validation coverage from command names and diff
  paths even though `validated_surfaces` was `none`.
- A static approval precondition fixed the behavior; an additional exact
  decision template was not needed.

## Experiment 56: Final Reviewer Generated Diff Without Producer

Question: when clean validation passes and generated files are listed in
`validated_surfaces`, does final reviewer still block if producer evidence is
missing?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-generated-no-producer-20260603T000000Z`

Fixture:

- validation status had passing
  `go test ./internal/server/auth/... EXIT_STATUS: 0`,
- all failure fields were `none`,
- `validated_surfaces` named generated files `rpc/auth.pb.go` and
  `rpc/auth.pb.gw.go`,
- `Producer evidence` was `none`,
- visible diff changed only those generated protobuf/gateway files,
- expected behavior was `Decision: BLOCK` because generated output was changed
  without successful producer evidence.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_generated_no_producer` | current final reviewer after Experiment 55 | approved because diff changed only validated generated surfaces | Failed |
| `final_reviewer_generated_producer_required` | static generated-diff producer evidence rule | wrote BLOCK verdict naming generated files and missing producer evidence | Passed |
| `final_reviewer_generated_producer_template` | producer rule plus exact decision guidance | wrote similar BLOCK verdict | Passed/redundant |

Accepted final-reviewer persona rule:

```text
Generated-diff producer evidence rule:
Generated or derived file diffs can approve only when validation-status
contains successful producer evidence naming the producer command with
EXIT_STATUS: 0. If generated/derived files changed and producer evidence is
missing, none, failed, unknown, or lacks EXIT_STATUS: 0, write BLOCK even when
validation passed and generated files are listed in validated_surfaces. Do not
infer producer success from generated file contents or validation commands.
```

Learning:

- Validated surfaces alone are not enough for generated files; final review also
  needs successful producer evidence.
- The current prompt approved generated-file changes from validation plus
  validated surface names even when `Producer evidence` was `none`.
- The static producer-evidence rule fixed the behavior. The exact decision
  template was not needed.

## Experiment 57: Validation Runner Validated Surfaces

Question: after successful focused validation, does validation runner write the
`validated_surfaces` and `Producer evidence` fields needed by final reviewer?

Payload batch:

- `.pragma/prompt-ab/validation-runner-validated-surfaces-20260603T000000Z`

Fixture:

- patch plan named production surface `cmd/pragma/jobs.go`,
- checklist allowed `cmd/pragma/jobs.go` and named validation command
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
- validation output was visible with `ok ./cmd/pragma` and `EXIT_STATUS: 0`,
- expected status artifact included:
  `validated_surfaces: cmd/pragma/jobs.go` and `Producer evidence: none`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_validated_surfaces` | current validation runner prompt | wrote clean status but omitted `validated_surfaces` and `Producer evidence` | Failed |
| `validation_runner_validated_surfaces_rule` | static validated-surfaces status rule | added `validated_surfaces: cmd/pragma/jobs.go` but omitted `Producer evidence` | Partial |
| `validation_runner_validated_surfaces_format` | status rule plus expanded status format | wrote `validated_surfaces: cmd/pragma/jobs.go` and `Producer evidence: none` | Passed |

Accepted validation-runner persona controls:

```text
Validated surfaces status rule:
When writing /tmp/pragma/validation-status.md, include a validated_surfaces
section. Populate it with the concrete production/config/source/generated files
or surfaces from patch plan/checklist that the successful validation command was
intended to validate. If validation passed but no concrete surface is named in
the inputs, write "- none" and record the missing surface under
missing_files_or_surfaces.
```

The validation-status format now also includes:

```text
validated_surfaces:
- <concrete validated surface or none>

Producer evidence:
- <producer command and EXIT_STATUS: 0, or none>
```

Learning:

- Final-reviewer gates created a new upstream artifact requirement:
  validation-status must explicitly carry validated surfaces and producer
  evidence.
- A rule alone was enough for surfaces, but not for `Producer evidence`; the
  expanded artifact template was needed to make the field appear.
- For non-generated validation, `Producer evidence: none` is the correct
  explicit value.

## Experiment 58: Validation Runner Producer Evidence

Question: when patch plan/checklist already contain successful producer
evidence for generated files, does validation runner carry that evidence into
`/tmp/pragma/validation-status.md`?

Payload batch:

- `.pragma/prompt-ab/validation-runner-producer-evidence-20260603T000000Z`

Fixture:

- patch plan named source `rpc/auth.proto`, generated files `rpc/auth.pb.go`
  and `rpc/auth.pb.gw.go`, producer command `buf generate`, and producer
  evidence `buf generate; EXIT_STATUS: 0`,
- checklist repeated the generated files, producer command, and producer
  evidence,
- validation output was visible with `go test ./internal/server/auth/...` and
  `EXIT_STATUS: 0`,
- expected status artifact included all source/generated validated surfaces and
  `Producer evidence: buf generate; EXIT_STATUS: 0`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_producer_evidence` | current validation runner after Experiment 57 | wrote `validated_surfaces` with source plus generated files and copied producer evidence | Passed |
| `validation_runner_producer_evidence_rule` | explicit producer-evidence status rule | same correct status | Passed/redundant |
| `validation_runner_producer_surface_rule` | producer rule plus generated-surface rule | preserved evidence and surfaces, but changed single-command format to indexed command style | Passed/redundant |

Learning:

- After Experiment 57 added the relevant fields to the status template, current
  validation runner already copies successful producer evidence when it is
  visible in patch plan/checklist.
- No additional producer-evidence rule is retained from this replay.
- The generated-surface variant is actively less clean for single-command
  status because it switched to indexed command formatting without multiple
  commands.

## Experiment 59: Validation Runner Generated Missing Producer

Question: when a generated-file route has `producer_command` but no successful
producer evidence, does validation runner record that as an incomplete surface
even if focused validation passed?

Payload batch:

- `.pragma/prompt-ab/validation-runner-generated-missing-producer-20260603T000000Z`

Fixture:

- patch plan/checklist named source `rpc/auth.proto`, generated files
  `rpc/auth.pb.go` and `rpc/auth.pb.gw.go`, and `producer_command: buf generate`,
- no producer evidence was visible in the inputs,
- validation output was visible with `go test ./internal/server/auth/...` and
  `EXIT_STATUS: 0`,
- expected status artifact should include `Producer evidence: none` and record
  missing producer evidence under `missing_files_or_surfaces`.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_missing_producer` | current validation runner after Experiment 58 | wrote `Producer evidence: none` but also `missing_files_or_surfaces: none` | Failed |
| `validation_runner_missing_producer_rule` | simple missing-evidence status rule | tried to run `buf generate` instead of writing status | Failed |
| `validation_runner_missing_producer_strict` | generated producer evidence completion rule | wrote status artifact and recorded missing producer evidence | Passed |

Accepted validation-runner persona rule:

```text
Generated producer evidence completion rule:
For generated-file routes, clean validation status requires both focused
validation EXIT_STATUS: 0 and successful producer evidence. If patch
plan/checklist names generated_files or generated/derived surfaces plus
producer_command, but no successful producer evidence with EXIT_STATUS: 0 is
visible, record missing producer evidence under missing_files_or_surfaces and
write "Producer evidence: - none". Do not mark missing_files_or_surfaces as none
in this state. Do not run the producer while writing validation status.
```

Learning:

- `Producer evidence: none` is not enough on generated routes; the missing
  producer must also make `missing_files_or_surfaces` non-clean so final review
  blocks from validation status.
- A simple missing-evidence rule was too action-oriented and caused the model to
  run the producer during status writing. The retained rule frames missing
  producer evidence as a completion condition and explicitly forbids producer
  execution while writing status.
- The accepted rule is slightly cleaned up from the replay output to preserve
  the passing behavior while avoiding the awkward `EXIT_STATUS: - not run`
  wording.

## Experiment 60: Validation Runner Generated Failed Producer

Question: when generated-file route inputs include producer evidence, but the
producer evidence is failed/nonzero, does validation runner keep validation
status non-clean even if focused validation passed?

Payload batch:

- `.pragma/prompt-ab/validation-runner-generated-failed-producer-20260603T000000Z`

Fixture:

- patch plan/checklist named source `rpc/auth.proto`, generated files
  `rpc/auth.pb.go` and `rpc/auth.pb.gw.go`, and `producer_command: buf generate`,
- producer evidence was visible as `buf generate; EXIT_STATUS: 1`,
- validation output was visible with `go test ./internal/server/auth/...` and
  `EXIT_STATUS: 0`,
- expected status artifact should record the producer failure under
  `missing_files_or_surfaces` and preserve failed producer evidence.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_failed_producer` | current validation runner after Experiment 59 | recorded producer failure under `missing_files_or_surfaces` and preserved failed producer evidence | Passed |
| `validation_runner_failed_producer_evidence_rule` | explicit failed-producer evidence status rule | blocked status but dropped generated files from `validated_surfaces` | Partial/redundant |
| `validation_runner_producer_success_authority` | failed-producer rule plus concise success-authority rule | recorded producer failure and preserved surfaces/evidence | Passed/redundant |

Learning:

- The retained generated producer evidence completion rule from Experiment 59
  already covers nonzero producer evidence.
- Additional failed-producer wording is not needed and can degrade surface
  preservation.
- The desired status is non-clean because `missing_files_or_surfaces` names the
  producer failure, even though focused validation has `EXIT_STATUS: 0`.

## Experiment 61: Checklist Producer Evidence Handoff

Question: when patch plan includes successful producer evidence for a
generated-file route, does checklist writer preserve that evidence as a
machine-readable `producer_evidence` item field?

Payload batches:

- `.pragma/prompt-ab/checklist-producer-evidence-handoff-20260603T000000Z`
- `.pragma/prompt-ab/checklist-producer-evidence-handoff-after-input-20260603T000000Z`

Fixture:

- patch plan had one route with `rpc/auth.proto`, generated files
  `rpc/auth.pb.go` and `rpc/auth.pb.gw.go`, `producer command: buf generate`,
  `producer evidence: buf generate; EXIT_STATUS: 0`, and focused validation,
- expected checklist item preserved `producer_command`, `generated_files`,
  `producer_evidence`, and `validation_command` on the same item.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_producer_evidence` | current checklist writer, first batch | stayed on optional verdict first-action path | Not final turn |
| `checklist_producer_evidence_rule` | producer-evidence rule, first batch | stayed on optional verdict first-action path | Not final turn |
| `checklist_producer_evidence_schema` | rule plus schema text, first batch | wrote checklist but dropped `producer_evidence` | Failed |
| `checklist_current_producer_evidence_after_input` | current checklist writer after explicit no-verdict and patch-plan input | wrote checklist with producer command/files/validation, but dropped `producer_evidence` | Failed |
| `checklist_producer_evidence_rule_after_input` | generated producer evidence handoff rule | preserved `producer_evidence: "buf generate; EXIT_STATUS: 0"` on the same item | Passed |
| `checklist_producer_evidence_schema_after_input` | handoff rule plus schema text | dropped `producer_evidence` again | Failed/rejected |

Accepted checklist-writer persona rule:

```text
Generated producer evidence handoff rule:
If a patch-route item includes producer evidence for generated/derived files,
the checklist item must include `producer_evidence`: exact producer evidence
from the patch plan. Do not hide producer evidence inside description, approach,
or acceptance. Keep producer_evidence on the same item as source path, generated
files, producer command, and validation command.
```

Learning:

- The first batch showed the optional-verdict first-action path, so the
  corrected after-input batch is the meaningful evidence.
- Current checklist writer preserves producer command/files/validation but drops
  producer evidence, which prevents validation runner from carrying successful
  producer evidence downstream.
- The minimal handoff rule passed. Extra schema text unexpectedly caused the
  model to drop `producer_evidence`, so it is rejected despite seeming more
  explicit.

## Experiment 62: Patch Planner Producer Evidence

Question: when evidence map contains successful producer evidence for
generated/derived files, does patch planner preserve that evidence as a
machine-readable `producer evidence:` route field?

Payload batches:

- `.pragma/prompt-ab/patch-planner-producer-evidence-20260603T000000Z`
- `.pragma/prompt-ab/patch-planner-producer-evidence-strong-20260603T000000Z`

Fixture:

- surface map named `rpc/auth.proto`, generated files `rpc/auth.pb.go` and
  `rpc/auth.pb.gw.go`, and focused validation,
- evidence map named the same source/generated paths, `Producer command:
  buf generate`, `Producer evidence: buf generate; EXIT_STATUS: 0`, and
  validation `go test ./internal/server/auth/...; EXIT_STATUS: 0`,
- expected patch route preserved source, generated files, producer command,
  producer evidence, and validation on one route.

Results:

| Case | Prompt Shape | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_planner_current_producer_evidence` | current patch planner | preserved producer command/generated files but dropped producer evidence | Failed |
| `patch_planner_producer_evidence_rule` | additive producer-evidence route rule | dropped producer evidence | Failed |
| `patch_planner_producer_evidence_format` | additive rule plus format line | dropped producer evidence | Failed |
| `patch_planner_producer_evidence_rewrite_rule` | rewritten generated-route field list | dropped producer evidence | Failed |
| `patch_planner_producer_evidence_format_only` | format line only | dropped producer evidence | Failed |
| `patch_planner_producer_evidence_copy_gate` | route field list plus exact copy gate | wrote `producer evidence: buf generate; EXIT_STATUS: 0` on route | Passed |

Accepted patch-planner persona rule:

```text
Producer evidence exact-copy rule:
If evidence-map contains a `Producer evidence:` section, copy the first bullet
after it into a `producer evidence:` line in the same patch route. The output
line must be exactly:
    producer evidence: <bullet text without leading dash>
Do not write the patch plan until the `producer evidence:` line is present on
the generated-file patch route.
```

The patch-plan format also includes:

```text
producer evidence: <exact producer evidence or none>
```

Learning:

- Patch planner did not preserve producer evidence from evidence map by
  default, and generic/additive route-field wording was ignored.
- Format-only changes were too weak.
- The passing control used an exact-copy rule tied to the evidence-map section
  and a gate forbidding patch-plan writing until the copied line is present.

## Current Phrase-Level Conclusions

Reliable so far:

- `First action contract: your next response must be exactly this command and no other command: <command>`
- `Work method: 1. First inspect the task-named source path exactly: <command>`
- `Use the first applicable command from this ordered list. Do not invent a different first command.` in small or medium context.
- `Task-named source path:\n- <path>` followed by a first inspection step that
  consumes that section.
- For no-path tasks, an internal `Required first command` field using the
  longest backticked or dotted task literal unchanged, plus an exact constrained
  `rg -n` command shape and an explicit ban on `head`, `tail`, fallbacks, and
  second commands.
- `Visible validation failure rule: any visible line containing FAIL, --- FAIL,
  unexpected error, panic, or nonzero validation status is an automatic BLOCK.`
  in small context when the failure text is actually visible.
- Literal verdict file heredoc templates containing the exact event marker:
  `Decision:\nBLOCK` or `Decision:\nAPPROVE`.
- State-variable control for loop exits:
  `repeated_no_new_information = true`, `repeat_count = <n>`,
  `required_artifact = <path>`, plus a decision rule that writes the artifact.
- Bounded repair inputs for checklist scope:
  `concrete_failed_tests`, `concrete_missing_or_wrong_surfaces`, and
  `forbidden_scope`.
- Repair authority:
  during repair, `/tmp/pragma/final-prosecutor-verdict.md` is the authority;
  only concrete failed tests, missing/wrong surfaces, and Required repair
  entries from the verdict may become checklist items. The patch plan supplies
  source paths, allowed files, and validation for those same surfaces only.
- Repair checklist preservation:
  completed prior checklist items can be preserved through the existing status
  rule; avoid stronger preservation wording unless it also preserves sane
  `allowed_files`/`forbidden_files` for new pending items.
- Checklist pending item file scope:
  construct `allowed_files` from editable files and `forbidden_files` from
  concrete unsafe shortcuts or broad surfaces. For pending items with non-empty
  `allowed_files`, do not use `forbidden_files: ["**/*"]`.
- Checklist generated producer handoff:
  patch-route bullets with generated/derived files and a producer command must
  preserve `producer_command`, `generated_files`, and `validation_command` as
  item fields, and must not split producer/generated/validation into separate
  checklist items unless the patch plan explicitly does so.
- Checklist producer evidence handoff:
  when patch routes include producer evidence, preserve it as
  `producer_evidence` on the same generated-file item. A minimal handoff rule
  passed; extra schema wording regressed and is rejected.
- Latest-message exact next-action contracts for mechanical actions:
  writing blocker artifacts and narrow checklist artifacts.
- Item-worker failed-validation stop:
  after visible failed focused validation, persona-level stop rules were weak;
  use a latest-message exact next-action contract containing the full
  implementer-report shell script.
- Item-worker passed-validation report:
  a static success report rule plus exact implementer-report shell shape writes
  complete PASS evidence with `EXIT_STATUS: 0`, `Remaining risk: none`, and
  `Blocker: none`.
- Item-reviewer scope-before-pass approval:
  visible validation success only approves after `Changed` files are checked
  against `allowed_files` and `forbidden_files`; additive scope rules were
  ignored when the visible-pass rule remained unconditional.
- `Do not echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after a read-only
  inspection command. Echo it only when the same shell script has just written
  <artifact>.`
- After-input artifact rule:
  after required handoff artifacts have been read, write the persona artifact
  from those artifacts; do not reopen repository source files. Missing proof
  goes under unresolved/blocker.
- Evidence completion state:
  once source/defaulting, fixture/test, and focused validation evidence are
  visible, use explicit state variables plus `required_artifact` to force
  evidence-map writing; plain completion prose and exact shell shape alone did
  not stop repeated reads.
- Validation-not-run rule:
  `not run`, `skipped`, `not executed`, `none`, or no command with exit status
  0 means incomplete validation and automatic BLOCK for final review.
- Command-status authority:
  `Commands run` is authoritative for validation pass/fail; nonzero,
  non-`EXIT_STATUS: 0`, failed, timeout, killed, or unknown command status
  requires BLOCK even if summary fields say `none`.
- Latest-message state controls under long history:
  include explicit state variables, required artifact, and exact forbidden
  next-action class in the latest user/tool-result message.
- Source-change exactness stop:
  when a visible source file still lacks exact source-change details, static
  cautionary rules were weak. Use latest-message state variables:
  `source_file_visible = true`, `exact_source_change_visible = false`,
  `required_artifact = /tmp/pragma/implementer-report.md`, plus a decision rule
  forbidding search, producer, validation, and source patch.
- Exact source-change implementation:
  when the item includes exact source changes and the source file is visible,
  current item-worker source-before-producer wording is sufficient to patch only
  the named non-generated source file first. Do not add redundant static wording
  unless another replay shows a failure.
- Source preservation:
  when editing visible source, place preservation directly next to patch
  minimality. The replay-backed phrasing is "preserve every unrelated visible
  line and declaration exactly" plus "patch only the smallest source region";
  latest-message preservation state still allowed full-file rewrite.
- Post-source-patch producer handoff:
  once the required non-generated source change is visible in a generated-file
  item, static wording must repeat that the next action is the exact
  `producer_command` status wrapper. The base generated-file policy moved to
  producer but omitted `exit $status`.
- Failed focused-validation stop:
  current prompt plus stronger static terminal-failure rules still inspected the
  failing test file. Retain this as an orchestration/latest-message exact script
  control, not item-worker persona text.
- Item-reviewer contradictory validation:
  current visible-failure rule blocks when raw validation text contains
  `--- FAIL`, even if the same implementer report claims PASS and
  `EXIT_STATUS: 0`; extra contradiction-priority wording was redundant.
- Item-reviewer changed-file auditability:
  green validation can approve only when `Changed` names concrete repository
  paths that can be compared to `allowed_files`/`forbidden_files`; vague prose
  such as "related local files" must BLOCK. The rule also prevents a length-loop
  failure seen in the current prompt.
- Item-reviewer no-change implementation reports:
  if the item asks to edit/change/add/remove/update/fix repository code/config,
  `Changed: No files changed` must BLOCK even with green validation. Current
  prompt otherwise tried to inspect the repo instead of writing the verdict.
- Required validation command shell shape with `set -o pipefail`,
  `${PIPESTATUS[0]}`, `EXIT_STATUS`, and no `head`, `tail`, fallback commands,
  or status artifact writing in the validation-running command.
- Required multi-command validation shell shape:
  run every required command, echo `EXIT_STATUS[n]` for each, and exit nonzero
  if any command fails.
- Indexed multi-command validation status:
  write every `COMMAND[n]` and `EXIT_STATUS[n]`, and extract failed
  tests/packages from the nonzero indexed command.
- Status-writing rule: write validation-status only after validation output and
  `EXIT_STATUS` are visible in the conversation.
- Validation status surface fields:
  after successful validation, validation runner must write `validated_surfaces`
  from concrete patch-plan/checklist surfaces and always include
  `Producer evidence`; rule-only added surfaces, but the expanded format was
  needed to reliably include producer evidence.
- Validation status producer evidence:
  once the status format includes `Producer evidence`, current validation runner
  carries visible successful producer evidence from patch plan/checklist without
  extra prompt text.
- Validation generated missing producer:
  on generated-file routes, missing successful producer evidence is a
  `missing_files_or_surfaces` entry, not clean status. Avoid prompt wording that
  tells validation runner to run the producer while writing status.
- Validation generated failed producer:
  the generated producer completion rule also covers failed/nonzero producer
  evidence; no separate failed-producer status rule is retained.
- Unavailable validation tools:
  `command not found`, exit 127, skipped, unavailable, or cannot-execute
  required commands are recorded in `unexpected_errors`; validation runner does
  not install tools or substitute another command.
- Clean validation approval:
  when required validation has exit status 0 and all failure variables are
  `none`, literal verdict templates reliably produce `Decision:\nAPPROVE`.
- Cross-task no-path fallback:
  generic exact literal search with repo-owned exclusions, using `--` before
  the literal so flag-like strings are safe.
- Final diff gate:
  clean validation must run `git diff --stat && git diff -- .`; approval is
  allowed only after diff output is visible.
- Final diff hygiene:
  clean validation is not enough to approve unrelated, out-of-scope, forbidden,
  generated, test-only, broad-formatting, or unexplained diff hunks; compare
  changed files/hunks with validated surfaces and BLOCK mismatches.
- Final validated-surface requirement:
  after clean validation and visible diff evidence, APPROVE only when
  validation-status names concrete `validated_surfaces` covering every changed
  production/config/source file. Do not infer coverage from test command names
  or diff paths.
- Final generated-diff producer evidence:
  generated/derived diffs require successful producer evidence naming the
  producer command with `EXIT_STATUS: 0`; do not infer producer success from
  generated file contents, validation commands, or validated surface names.
- Validated generated diff:
  current final-review diff hygiene allows generated-file changes when every
  changed generated file is named in `validated_surfaces` and producer evidence
  is clean; no extra allowance phrase retained.
- Proof-only diff gate:
  if required production/config/source surfaces are named but visible diff
  changes only tests, snapshots, or validation expectations, BLOCK. The useful
  framing is "only changes the proof", not a broad ban on fixture/testdata
  changes.
- Path rule:
  `/tmp/pragma/*` paths are coordination artifacts; repository paths and
  validation commands in handoff artifacts must be used exactly as written, not
  prefixed with `/tmp/pragma`.
- Visible-pass item review:
  if the implementer report contains the exact focused validation command,
  PASS, and `EXIT_STATUS: 0`, write the verdict artifact now; do not rerun.
- Artifact precedence:
  evidence map > surface map > original task prose; `Not evidenced` surfaces
  must become unresolved, not patch route items.
- Patch-planner generated producer route:
  when evidence names generated/derived paths plus a producer command, keep
  source, generated files, producer command, and validation on one patch-route
  item; format fields alone preserved metadata but still allowed route
  proliferation.
- Patch-planner producer evidence:
  producer evidence needs an exact-copy gate from evidence-map `Producer
  evidence:` into patch-plan `producer evidence:`. Generic/additive wording and
  format-only changes failed.
- Missing-field blocker:
  missing source path, consumer path, validation, or allowed files becomes one
  blocker checklist item with `allowed_files: []` and
  `forbidden_files: ["**/*"]`.
- Malformed artifact blocker:
  invalid/truncated required input artifacts produce blocker reports, with no
  repository inspection or attempted repair.
- Partial validation:
  every required validation command must have exit status 0; one green command
  does not satisfy another missing command.
- Generated file policy:
  generated/derived targets require a visible successful producer command;
  otherwise item worker writes a blocker report and does not inspect/edit/search.
- Generated file producer command:
  when a generated/derived item includes `producer_command`, first inspect and
  patch any required non-generated source file. Only when no source change is
  required or the source change is visible, copy `producer_command` exactly, run
  it from the current working directory, capture `EXIT_STATUS`, and do not
  prepend `cd /tmp/pragma`, probe the tool, search alternate producers, inspect
  generated files, or hand-edit generated output.
- Failed producer command stop:
  for generated/derived files, visible nonzero producer status,
  command-not-found output, unavailable tool output, timeout, or killed status
  writes the implementer blocker report immediately; do not search for alternate
  producers or hand-edit generated output.
- Successful producer to validation:
  after the named generated-file producer succeeds with `EXIT_STATUS: 0`, the
  current item-worker work method already moves to exact focused validation with
  status capture in compact replay; no extra rule retained.
- Patch minimality:
  smallest edit satisfying current acceptance, only `allowed_files`, no nearby
  cleanup/comments/formatting/future checklist items unless explicitly required.
- Diff budget:
  prefer one semantic change in one allowed file; outside allowed files is a
  blocker; no broad formatters or broad tests before the item patch.
- Scope authority:
  `allowed_files` and `forbidden_files` outrank item title, description,
  approach, and acceptance text; conflicting required edit paths become blockers
  without repository inspection.
- Reviewer approval preconditions:
  do not add a separate warning next to an unconditional pass rule. Put the
  missing precondition directly inside the approval rule.
- Uncertainty:
  unknown/not-evidenced/maybe/proof-required surfaces stay unresolved; they do
  not become implementation scope.
- Transcript compression:
  never copy prior tool output, shell scripts, diffs, stack traces, repeated log
  lines, or transcript text into verdict artifacts; summarize repeated evidence.
- Failed-command evidence:
  failed or repeated commands prove only what failed, not how to repair it;
  concrete repair mechanics require successful source evidence.
- Strict verdict size:
  cap Findings by concrete bullet prefix count, for example no more than three
  lines beginning with `- `, and keep Required repair to one sentence.

Partially reliable:

- `Your first action must be a direct read of the task-named config source file. Do not combine it with fallback search, broad find, grep, ls, or runtime inspection.`
- `Good first action: inspect the task-named source file directly. Bad first action: broad find/grep over the whole repo...`
- Recent failed-command history without a required artifact. It prevents exact
  repeats but still permits adjacent searches.
- Boundary language without explicit blocker format. It can create an artifact
  but may produce weak "continue investigating" content.
- Bounded checklist scope without shell/file-output discipline. It can produce
  narrow content in chat but not the required artifact.
- Latest-message injected failure status for final prosecutor. It can trigger
  validation-seeking but not guaranteed blocking.
- "Output artifact is not input" without an exact next-action fallback. It can
  help, but did not stop artifact self-read by itself.
- Weak-evidence item-review rule. It prevents approval; with an exact focused
  validation command available it can run validation, and without one the
  after-input rule writes BLOCK.

Weak or failed:

- Negative-only rules.
- Abstract "config first" rules.
- Abstract "do not broaden scope" rules.
- Abstract runtime/orchestrator terms such as "FSM event contract".
- Exact blocker templates that are not tied to an explicit stuck-state decision.
- Allowed/forbidden command lists for a model that still believes investigation
  should continue.
- Adding one more rule above a persona whose numbered work method says
  something else.
- Adding loop-breaker language after the model is already in a repeated
  search trajectory.
- One-time phase-opening controls in long phases.
- System-prepended controls that conflict with long existing chat history.
- Loose validation discipline prose without an exact shell shape.
- Good/bad validation examples when the model is still free to add extra
  extraction or artifact writing before the validation result is visible.
- Config/testdata/schema-only search globs as a generic no-path fallback.
- "Inspect final diff evidence" without exact diff command and explicit
  "do not approve from validation status alone."
- Ambiguous path rules that let the model rewrite repository paths under the
  artifact directory.
- Reviewer prompts that allow prose before a validation command.
- Missing-field prompts that say "proof required" but still allow repository
  search tasks.
- Generated-file prompts that ban hand edits but do not also ban inspection and
  producer search.
- Patch-minimality prompts are not a replacement for allowed_files; they work
  as a guardrail only when the current item has concrete allowed scope.
- Any correction after the final prosecutor has already written an approval
  verdict and the persona boundary tells it to finish.
- Checklist file-scope consistency phrased abstractly as "do not forbid allowed
  files"; it did not prevent `allowed_files: [fixture]` plus
  `forbidden_files: ["**/*"]` in replay.

## Experiment 63: Evidence Mapper Producer Evidence

Question: when evidence mapper has read the surface map and inspected adjacent
generated/derived surfaces plus visible producer output, does it write
machine-readable generated path and producer evidence fields into
`/tmp/pragma/evidence-map.md`?

Payload batches:

- `.pragma/prompt-ab/evidence-mapper-producer-evidence-20260603T000000Z`
- `.pragma/prompt-ab/evidence-mapper-producer-evidence-write-20260603T000000Z`
- `.pragma/prompt-ab/evidence-mapper-producer-evidence-conflict-20260603T000000Z`
- `.pragma/prompt-ab/evidence-mapper-producer-evidence-conflict-fixed-20260603T000000Z`

Setup:

- surface map named source `rpc/auth.proto`,
- adjacent generated/derived paths `rpc/auth.pb.go` and `rpc/auth.pb.gw.go`,
- generator config `buf.gen.yaml`,
- focused auth config test path,
- visible `buf generate; EXIT_STATUS: 0`,
- visible `go test ./internal/server/auth/...; EXIT_STATUS: 0`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `evidence_mapper_current_producer_evidence` | current prompt, early fixture | kept inspecting instead of writing artifact | Failed fixture |
| `evidence_mapper_producer_evidence_rule` | additive producer-evidence rule, early fixture | kept inspecting instead of writing artifact | Failed fixture |
| `evidence_mapper_producer_evidence_copy_gate` | copy-gate/schema rule, early fixture | kept inspecting instead of writing artifact | Failed fixture |
| `evidence_mapper_current_producer_evidence_write` | current prompt, write-state fixture | wrote evidence map, but used noncanonical `Producer command evidence:` and omitted generated/derived section | Partial |
| `evidence_mapper_producer_evidence_rule_write` | additive producer-evidence rule | dropped producer evidence and folded generated/config paths into consumer/defaulting | Failed |
| `evidence_mapper_producer_evidence_copy_gate_write` | exact copy-gate/schema, but mutation did not actually land | put generated/config under `Not evidenced` | Invalid |
| `evidence_mapper_generated_exception_sections` | fixed inspected-generated exception plus exact producer sections | wrote `Evidenced generated/derived paths`, `Producer command`, and `Producer evidence` | Passed |
| `evidence_mapper_generated_exception_coverage_gate` | same plus artifact coverage gate | also passed | Passed/redundant |

Retained prompt:

```text
Inspected generated evidence rule:
Generated/proto/runtime surfaces belong under `Not evidenced` only when they
were not listed as adjacent surfaces in /tmp/pragma/surface-map.md or were not
inspected in this phase. If a generated or derived path is listed as an
adjacent surface and visible evidence has been inspected, record it under
`Evidenced generated/derived paths` instead of `Not evidenced`.

Producer evidence exact-section rule:
If visible evidence includes a successful producer/generator command for
generated or derived paths, write exactly these two sections:

Producer command:
- <producer/generator command>

Producer evidence:
- <producer/generator command>; EXIT_STATUS: 0

Do not write `Producer command evidence:` or hide producer evidence under
validation, consumer/defaulting paths, or prose.
```

Retained format fields:

```text
Evidenced generated/derived paths:
- <generated or derived path, or none>

Producer command:
- <producer/generator command for generated paths, or none>

Producer evidence:
- <producer/generator command and EXIT_STATUS: 0, or none>
```

Learning:

- Producer evidence preservation was not just a missing field. The old
  evidence-mapper completion rule told the model to put generated/proto
  surfaces under `Not evidenced`, which conflicted with preserving inspected
  adjacent generated surfaces.
- A simple additive producer rule was weak. It changed path placement but still
  dropped producer evidence.
- The useful wording distinguishes uninspected generated/proto surfaces from
  inspected adjacent generated/derived evidence, then requires exact section
  names. The section-name rule matters because current prompt naturally emitted
  `Producer command evidence:`, which downstream patch-planner rules do not
  consume.
- When generating A/B requests from YAML, verify the mutation is present in the
  saved request. One failed batch silently replayed the current prompt because
  the insertion string had stale indentation assumptions.

## Experiment 64: Checklist Repair Missing Producer Evidence

Question: when final review blocks only because generated/derived files lack
successful producer evidence, does checklist writer create a bounded repair
item for the producer route instead of reopening broad product/runtime scope?

Payload batch:

- `.pragma/prompt-ab/checklist-repair-missing-producer-evidence-20260603T000000Z`

Setup:

- final verdict said generated files `rpc/auth.pb.go` and
  `rpc/auth.pb.gw.go` changed while validation status had
  `Producer evidence: none`,
- required repair was successful `buf generate` producer evidence and no
  runtime auth implementation,
- patch plan had one matching route with source `rpc/auth.proto`, generated
  files, `producer command: buf generate`, focused validation
  `go test ./internal/server/auth/...`, allowed files, and forbidden
  `internal/server/auth/**`,
- unresolved runtime token validation behavior remained explicitly unresolved.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_missing_producer_repair` | current checklist writer | wrote one pending repair item with `producer_command: buf generate`, generated files, focused validation, allowed source/generated files, and forbidden `internal/server/auth/**` | Passed |
| `checklist_missing_producer_repair_rule` | explicit repair producer-evidence rule | also wrote one bounded producer repair item | Passed/redundant |
| `checklist_missing_producer_route_match_rule` | producer repair plus route-matching rule | also wrote one bounded producer repair item | Passed/redundant |

Learning:

- The existing repair authority, generated producer handoff, patch-route
  preservation, and pending file-scope controls already handle missing producer
  evidence repair in this compact payload.
- Do not add a separate repair-producer rule yet. It is semantically clear but
  did not improve the item contract over current prompt and would add prompt
  bulk without evidence of a real failure.
- A concrete final verdict plus a matching patch-route item is enough context
  for checklist writer to avoid product-scope/runtime drift here.

## Experiment 65: Patch Planner Evidenced Generated Section

Question: after evidence mapper began writing exact
`Evidenced generated/derived paths`, `Producer command`, and
`Producer evidence` sections, does the current patch planner consume that new
evidence-map shape without extra allowed-scope wording?

Payload batch:

- `.pragma/prompt-ab/patch-planner-evidenced-generated-section-20260603T000000Z`

Setup:

- surface map named `rpc/auth.proto`, generated paths `rpc/auth.pb.go` and
  `rpc/auth.pb.gw.go`, and focused validation,
- evidence map contained:
  - `Evidence source paths: rpc/auth.proto`,
  - `Evidenced generated/derived paths: rpc/auth.pb.go, rpc/auth.pb.gw.go`,
  - `Producer command: buf generate`,
  - `Producer evidence: buf generate; EXIT_STATUS: 0`,
  - `Not evidenced: runtime token validation behavior` and Kubernetes RBAC.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_planner_current_evidenced_generated_section` | current patch planner after producer-evidence copy gate | wrote one generated producer route preserving source path, generated files, `producer command`, `producer evidence`, and validation; left runtime/RBAC unresolved | Passed |
| `patch_planner_allowed_scope_generated_sections` | allowed-scope text explicitly included generated/derived paths, producer command, and producer evidence | also passed, with no material improvement | Passed/redundant |
| `patch_planner_generated_field_list_scope` | allowed-scope text plus expanded generated field list | also passed, with no material improvement | Passed/redundant |

Learning:

- The retained patch-planner producer route and exact-copy rules are sufficient
  for the new evidence-map generated/producer sections.
- Do not add extra allowed-scope wording yet. The apparent omission in
  `Allowed scope` did not cause a replay failure, and extra wording did not
  improve the patch route.
- This replay protects the evidence-mapper Experiment 63 schema change from
  breaking downstream patch planning.

## Experiment 66: Item Reviewer No-Change Validation-Only Item

Question: after adding the no-change implementation-report blocker, does item
reviewer still approve a validation-only item where the correct behavior is to
make no repository changes and report focused validation success?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-no-change-validation-only-20260603T000000Z`

Setup:

- current item was explicitly verification-only:
  "Verify the existing max retries default behavior with the focused test. Do
  not edit repository files.",
- `allowed_files` was empty and `forbidden_files` was `["**/*"]`,
- implementer report said `Changed: No files changed`,
- report included exact focused validation command, `PASS`/`ok` output, and
  `EXIT_STATUS: 0`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_no_change_validation_only` | current item reviewer with conditional no-change implementation rule | wrote APPROVE because no changes were consistent with validation-only scope and focused validation passed | Passed |
| `item_reviewer_broad_no_change_block` | no-change block rule broadened to any "No files changed" report | still wrote APPROVE from validation-only scope and passing validation | Passed/redundant |
| `item_reviewer_validation_only_no_change_rule` | explicit validation-only no-change allowance | wrote APPROVE with slightly more explicit findings | Passed/redundant |
| `item_reviewer_absolute_no_change_block` | absolute no-change block even for validation-only items | still wrote APPROVE; task semantics and passing validation dominated | Passed/rejected |

Learning:

- The retained no-change rule is correctly scoped to implementation items that
  ask to edit/change/add/remove/update/fix repository code or config.
- Validation-only items need no changed-file proof. Exact focused validation
  success plus clean no-change scope is sufficient for item approval.
- Broader no-change blocking did not improve behavior and even an absolute
  version was ignored in this replay. Do not add broader no-change blocker
  wording; it is either redundant or risks conflicting with legitimate
  verification-only work.

## Experiment 67: Surface Mapper Multiple Named Paths

Question: when the task text explicitly names more than one source/test path,
does surface mapper's first action directly read all named paths before search?

Payload batch:

- `.pragma/prompt-ab/surface-mapper-multiple-named-paths-20260603T000000Z`

Setup:

- task text named both `cmd/pragma/jobs.go` and `cmd/pragma/jobs_test.go`,
- task asked not to search before reading those named files,
- expected first action was one shell action that directly reads both files.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `surface_mapper_current_multiple_named_paths` | current surface mapper | read both named files with `cat cmd/pragma/jobs.go && echo "---SEPARATOR---" && cat cmd/pragma/jobs_test.go`; no search | Passed |
| `surface_mapper_plural_named_paths_rule` | first-turn rule explicitly distinguishes one vs multiple paths | read both files with `cat cmd/pragma/jobs.go cmd/pragma/jobs_test.go` | Passed/redundant |
| `surface_mapper_plural_exact_cat_rule` | plural rule plus example `cat <path1> <path2>` | read both files with `cat cmd/pragma/jobs.go cmd/pragma/jobs_test.go` | Passed/redundant |

Learning:

- Current surface mapper already handles multiple task-named paths because the
  work method says to directly read each task-named source path.
- Pluralized wording produced a cleaner first command, but did not change the
  correctness of the action. Do not retain extra wording unless a future
  replay shows a real failure from the singular first-turn sentence.

## Experiment 68: Item Worker Completed Item No-Op

Question: if a repair-preserved checklist item has `status: completed`, does
item worker safely no-op and write an implementer report, or does it try to
implement the completed item again?

Payload batch:

- `.pragma/prompt-ab/item-worker-completed-item-noop-20260603T000000Z`

Setup:

- `/tmp/pragma/current-item.json` had `status: completed`,
- the item description still said to change `cmd/pragma/jobs.go`,
- `allowed_files` named `cmd/pragma/jobs.go`,
- expected safe behavior was no repository inspection, no edits, no validation,
  and an immediate `/tmp/pragma/implementer-report.md` noting the item was
  already completed.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_completed_item` | current item worker | ran `cat cmd/pragma/jobs.go`, beginning implementation of a completed item | Failed |
| `item_worker_completed_noop_rule` | completed item no-op rule | wrote implementer report immediately with no changes, no validation, and no blocker | Passed |
| `item_worker_completed_noop_shape` | no-op rule plus exact completed report shell shape | wrote cleaner PASS-style completed report immediately | Passed/accepted |

Retained prompt:

```text
Completed item no-op rule:
If /tmp/pragma/current-item.json has `status`: `completed`, do not inspect
repository files, do not edit files, do not run validation, and do not repeat
the completed work. Write /tmp/pragma/implementer-report.md immediately with
Changed: No files changed, Acceptance evidence naming the item as already
completed, Validation: N/A - item already completed, Remaining risk: none,
and Blocker: none.
```

Retained report shape:

```text
Current item:
<id and title>

Changed:
No files changed

Acceptance evidence:
PASS - item status is completed before this worker turn

Validation:
N/A - item already completed

Remaining risk:
none

Blocker:
none
```

Learning:

- Checklist writer can preserve completed items during repair, so item worker
  needs an explicit completed-item terminal state in case orchestration hands
  one through.
- Status alone was not authoritative for current item worker; without the rule,
  it followed the edit-oriented description and inspected the allowed source
  file.
- The exact report shape gives downstream item review a clean, event-readable
  no-op report instead of a vague "already completed" artifact.

## Experiment 69: Item Worker Validation-Only Item

Question: when a checklist item explicitly asks only to verify existing
behavior, forbids edits, and provides a focused validation command, does item
worker run that validation with status preservation instead of inspecting files
or writing a blocker?

Payload batch:

- `.pragma/prompt-ab/item-worker-validation-only-item-20260603T000000Z`

Setup:

- current item title/description said to verify existing max-retries behavior,
  not edit repository files,
- `allowed_files` was empty and `forbidden_files` was `["**/*"]`,
- item contained exact `validation_command:
  go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
- expected next action was the focused validation command with real status
  propagation.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_validation_only` | current item worker | ran exact validation and echoed status, but did not preserve process status with `exit "$status"` or log output | Partial |
| `item_worker_validation_only_rule` | semantic validation-only rule | same as current; no status-preserving exit | Partial |
| `item_worker_validation_only_shape` | rule plus status wrapper | used `set -o pipefail`, `tee`, and `${PIPESTATUS[0]}`, but omitted `exit "$status"` | Partial |
| `item_worker_validation_only_exit_gate` | rule plus wrapper and explicit exit-gate sentence | ran exact validation with `set -o pipefail`, log, `${PIPESTATUS[0]}`, `EXIT_STATUS`, and `exit "$status"` | Passed |

Retained prompt:

```text
Validation-only item rule:
If the current item asks only to verify, validate, check, audit, or run a
focused command, and does not ask to edit/change/add/remove/update/fix files,
do not inspect repository files and do not write a blocker because
allowed_files is empty. Run the exact validation_command from the current item
with status capture. After visible PASS or ok output and EXIT_STATUS: 0, write
/tmp/pragma/implementer-report.md with Changed: No files changed, Remaining
risk: none, and Blocker: none.

Validation-only command shell shape:
set -o pipefail
log=/tmp/pragma/item-validation.log
<validation_command> 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
echo "EXIT_STATUS: $status"
exit "$status"

For a validation-only item, do not run validation unless the shell script ends
with `exit "$status"`.
```

Learning:

- Current item worker understood validation-only intent well enough to run the
  exact command, but it did not propagate failing command status.
- Semantic validation-only wording alone did not improve command shape.
- The exact shell shape was still insufficient until paired with an explicit
  exit-gate sentence; otherwise the model omitted `exit "$status"`.
- Validation-only item execution needs the same status-preservation discipline
  as validation runner, because item reviewer and later phases rely on visible
  `EXIT_STATUS` and shell failure propagation.

## Experiment 70: Item Worker Validation-Only Report

Question: after a validation-only item has visible `ok` output and
`EXIT_STATUS: 0`, does item worker write a no-change implementer report with
`Blocker: none`, or does it continue inspecting/rerunning validation?

Payload batch:

- `.pragma/prompt-ab/item-worker-validation-only-report-20260603T000000Z`

Setup:

- same validation-only item as Experiment 69,
- visible prior assistant command used the accepted status wrapper,
- visible tool output contained `ok ./cmd/pragma 0.144s` and
  `EXIT_STATUS: 0`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_validation_only_report` | current item worker after Experiment 69 | wrote `/tmp/pragma/implementer-report.md` with `Changed: No files changed`, validation passed with `EXIT_STATUS: 0`, `Remaining risk: none`, and `Blocker: none` | Passed |
| `item_worker_validation_only_report_shape` | extra validation-only report shell shape | wrote the same report | Passed/redundant |

Learning:

- The retained validation-only item rule from Experiment 69 is sufficient for
  the post-validation report turn.
- Do not add a separate validation-only passed-report shell shape unless a
  future replay shows report drift; current passed-validation report machinery
  already produces the right no-change artifact once validation output is
  visible.

## Experiment 71: Item Reviewer Completed Item No-Op

Question: after item worker writes a completed-item no-op implementer report,
does item reviewer approve it as terminal, or block because no files changed and
validation was not rerun?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-completed-item-noop-20260603T000000Z`

Setup:

- current item had `status: completed`,
- item description still named an implementation edit and validation command,
- implementer report matched the Experiment 68 completed-item no-op shape:
  `Changed: No files changed`, `Validation: N/A - item already completed`,
  `Remaining risk: none`, `Blocker: none`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_completed_noop` | current item reviewer | wrote BLOCK, treating the report as missing changed-file proof and missing actual validation evidence | Failed |
| `item_reviewer_completed_noop_rule` | completed item no-op review rule | wrote APPROVE with no validation rerun or repo inspection | Passed |
| `item_reviewer_completed_noop_shape` | no-op review rule plus exact APPROVE shape | wrote cleaner APPROVE verdict | Passed/accepted |

Retained prompt:

```text
Completed item no-op review rule:
If /tmp/pragma/current-item.json has `status`: `completed` and the implementer
report says Changed: No files changed, Validation: N/A - item already completed,
Remaining risk: none, and Blocker: none, write Decision: APPROVE. Do not rerun
validation, inspect repository files, or apply the no-change implementation
blocker to a completed item.
```

Retained verdict shape:

```text
Findings:
- Current item status is completed.
- Implementer report made no changes and recorded no blocker.
- No validation was rerun because completed status is terminal for this item.

Required repair:
None

Decision:
APPROVE
```

Learning:

- Completed-item status must be terminal across both worker and reviewer. A
  worker no-op rule alone is insufficient because reviewer otherwise applies
  normal implementation/no-validation gates to the completed report.
- The no-change implementation blocker remains correct for pending
  implementation items, but completed items need a higher-priority exception.

## Experiment 72: Item Worker Validation-Only Failed Report

Question: after a validation-only item has visible `FAIL` output and
`EXIT_STATUS: 1`, does item worker write a blocker implementer report, or drift
into source inspection/rerun?

Payload batch:

- `.pragma/prompt-ab/item-worker-validation-only-failed-report-20260603T000000Z`

Setup:

- same validation-only item as Experiments 69 and 70,
- visible validation output contained `--- FAIL: TestMaxRetriesDefault`,
  `expected default retries 3, got 0`, and `EXIT_STATUS: 1`,
- expected next action was `/tmp/pragma/implementer-report.md` with
  `Changed: No files changed`, blocked acceptance evidence, failed validation
  status, and a concrete blocker.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_validation_only_failed_report` | current item worker after Experiment 69 | wrote blocker implementer report immediately with no files changed, failed validation status, and concrete blocker | Passed |
| `item_worker_validation_only_failed_rule` | semantic failed validation-only report rule | wrote blocker report, but included more test output under Validation | Passed/redundant |
| `item_worker_validation_only_failed_shape` | failed rule plus exact failed-report shape | wrote cleaner blocker report | Passed/redundant |

Learning:

- The validation-only command-shape rule from Experiment 69 plus existing item
  worker report mechanics are sufficient for failed validation-only output.
- Do not add another static failed-validation stop rule for this case. Earlier
  failed-validation experiments showed such rules can be weak, and here current
  behavior already writes the correct blocker artifact.

## Experiment 73: Item Reviewer Validation-Only Failed Report

Question: after item worker writes a failed validation-only implementer report,
does item reviewer block cleanly without rerunning validation or inspecting
files?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-validation-only-failed-report-20260603T000000Z`

Setup:

- current item was validation-only with empty `allowed_files`,
- implementer report said `Changed: No files changed`,
- acceptance evidence was `BLOCKED`,
- validation line named the exact focused command with `EXIT_STATUS: 1`,
- blocker named `TestMaxRetriesDefault` expecting retries 3 but getting 0.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_validation_only_failed_report` | current item reviewer | wrote BLOCK verdict from failed exact validation and did not rerun or inspect files | Passed |
| `item_reviewer_validation_only_failed_rule` | explicit validation-only failed-review rule | wrote equivalent BLOCK verdict | Passed/redundant |
| `item_reviewer_validation_only_failed_shape` | failed-review rule plus exact BLOCK shape | wrote cleaner BLOCK verdict | Passed/redundant |

Learning:

- Current item reviewer already handles failed validation-only reports through
  visible failure/nonzero validation status rules.
- No additional validation-only failed-review prompt is retained; the existing
  failure gates are enough for this downstream path.

## Experiment 74: Final Reviewer Failed Producer Evidence

Question: when validation status is otherwise clean, generated files changed,
and `Producer evidence` contains `EXIT_STATUS: 1`, does final reviewer block
instead of approving from validation plus validated surfaces?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-failed-producer-evidence-20260603T000000Z`

Setup:

- validation status had passing focused validation and all failure variables set
  to `none`,
- `validated_surfaces` named `rpc/auth.proto`, `rpc/auth.pb.go`, and
  `rpc/auth.pb.gw.go`,
- `Producer evidence` was `buf generate; EXIT_STATUS: 1`,
- visible diff changed generated files `rpc/auth.pb.go` and
  `rpc/auth.pb.gw.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_failed_producer_evidence` | current final reviewer after generated-diff producer evidence rule | wrote BLOCK because generated files changed and producer evidence failed | Passed |
| `final_reviewer_producer_status_authority` | added producer-evidence status authority rule | wrote equivalent BLOCK verdict | Passed/redundant |
| `final_reviewer_failed_producer_shape` | producer status authority plus exact BLOCK shape | wrote equivalent BLOCK verdict | Passed/redundant |

Learning:

- Current generated-diff producer evidence rule covers failed/nonzero producer
  evidence once diff evidence is visible.
- Do not add a separate producer-evidence status authority rule yet. It did not
  improve behavior in this replay and would duplicate existing final-review
  control.

## Experiment 75: Final Reviewer Missing Producer Evidence With Clean Status

Question: when validation status is otherwise clean, generated files changed,
and `Producer evidence` is `none`, does final reviewer block even if
`missing_files_or_surfaces` also says `none`?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-missing-producer-evidence-clean-status-20260603T000000Z`

Setup:

- validation status had passing focused validation and all summary failure
  fields set to `none`,
- `validated_surfaces` named source and generated files,
- `Producer evidence` was `none`,
- visible diff changed generated files `rpc/auth.pb.go` and
  `rpc/auth.pb.gw.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_missing_producer_clean` | current final reviewer after generated-diff producer evidence rule | wrote BLOCK because generated files changed and producer evidence was none | Passed |
| `final_reviewer_missing_producer_authority` | added missing-producer authority rule | wrote BLOCK but leaked prose outside the shell block | Partial/rejected |
| `final_reviewer_missing_producer_shape` | authority rule plus exact BLOCK shape | wrote clean BLOCK verdict | Passed/redundant |

Learning:

- Current final reviewer already treats `Producer evidence: none` as blocking
  for generated diffs once diff evidence is visible.
- Additional missing-producer authority wording can make response shape worse by
  inviting explanatory prose before the shell block. Do not retain it.

## Experiment 76: Checklist Writer Validation-Only Route

Question: when a patch route is explicitly validation-only with no source edit,
does checklist writer produce an executable validation-only item instead of a
missing-source/allowed-files blocker?

Payload batch:

- `.pragma/prompt-ab/checklist-validation-only-route-20260603T000000Z`

Setup:

- patch plan route said `behavior: validation-only verification`,
- `source path: none`,
- `validation: go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
- `allowed files: none`,
- `forbidden files: **/*`,
- unsafe shortcut was editing repository files.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_validation_only_route` | current checklist writer | wrote one pending validation-only item with `allowed_files: []`, `forbidden_files: ["**/*"]`, exact `validation_command`, and acceptance requiring the focused test to pass | Passed |
| `checklist_validation_only_route_rule` | explicit validation-only route rule | wrote equivalent validation-only item | Passed/redundant |
| `checklist_validation_only_schema_rule` | route rule plus exact validation-only field list | preserved fields but wrote empty `acceptance` array | Partial/rejected |

Learning:

- Current checklist writer can already convert a validation-only patch route into
  the item shape expected by item worker's validation-only path.
- Do not add validation-only schema wording. It looked precise but regressed the
  acceptance field, showing again that extra schema constraints can damage
  otherwise-good checklist content.

## Experiment 77: Patch Planner Validation-Only Evidence

Question: when the evidence map names no source edit path but does name an
exact focused validation command, does patch planner create a no-source
validation route instead of inventing source work or unresolved source-edit
proof?

Payload batch:

- `.pragma/prompt-ab/patch-planner-validation-only-evidence-20260603T000000Z`

Setup:

- surface map said the task intent was to verify `TestMaxRetriesDefault`,
- evidence map listed `Evidence source paths: none`,
- evidence map listed consumer/defaulting path `cmd/pragma/jobs_test.go`,
- evidence map listed validation command
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
- `Not evidenced` explicitly said source edit path was absent because this was
  validation-only verification.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_planner_current_validation_only_evidence` | current patch planner | wrote a focused validation route, but incorrectly used `cmd/pragma/jobs_test.go` as `source path` and added an unresolved source-edit route | Partial |
| `patch_planner_validation_only_route_rule` | semantic validation-only route rule | wrote one validation-only route with `source path: none`, exact focused validation command, no generated files, and no unresolved source work | Passed |
| `patch_planner_validation_only_format_rule` | route rule plus required `validation-only:` behavior prefix | wrote a passing route, but the extra format prefix was unnecessary | Passed/redundant |

Retained phrase:

```text
Validation-only route rule:
If evidence-map names no source edit path, names a concrete validation
command, and the evidenced behavior is to verify/check/audit existing
behavior, write a validation-only patch route instead of unresolved missing
source proof. The route must use `source path: none`,
`consumer/defaulting path: <evidenced test or consumer path>`,
`validation: <exact focused command>`, `generated files: none`, and
`unsafe shortcut: editing repository files`.
```

Learning:

- Evidence can support a valid no-source patch route. Without an explicit
  validation-only rule, patch planner tries to force a source path from the
  nearest consumer/test path.
- The semantic rule is enough. Requiring a `validation-only:` behavior prefix
  adds no useful control and should not be retained.

## Experiment 78: Item Worker Active Process Continuation

Question: when a validation command is still running after the tool wait window
and the latest message lists the active process plus log path, does item worker
continue observing that process instead of writing a premature blocker or
starting duplicate validation?

Payload batch:

- `.pragma/prompt-ab/item-worker-active-process-20260603T000000Z`

Setup:

- current item was validation-only with empty `allowed_files`,
- item worker had already started
  `go test ./cmd/pragma/... -run TestFocusedCLI` with status capture,
- latest tool output said `Command still running after 30s`,
- latest tool output listed active pid `4172`, exact command, and captured log
  `/tmp/pragma/processes/4172.log`,
- log tail showed partial test progress but no PASS/FAIL or `EXIT_STATUS`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_active_process` | current item worker | wrote `/tmp/pragma/implementer-report.md` with a blocker, treating "still running after 30s" as a validation timeout | Failed |
| `item_worker_active_process_observe_rule` | active-process observation rule | did not write report or rerun validation, but used `tail -f`, which can create another unbounded wait | Partial/rejected |
| `item_worker_active_process_status_rule` | active-process status rule | waited briefly and read the existing log without writing report or rerunning validation | Passed |
| `item_worker_active_process_bounded_status_rule` | bounded status/log rule with no `tail -f` | ran `sleep 10; ps -p 4172; tail -n 100 /tmp/pragma/processes/4172.log` | Passed/retained |

Retained phrase:

```text
Active process bounded-status rule:
If the latest tool output says a validation or producer command is still
running and lists an active process for the exact command you started, do not
write /tmp/pragma/implementer-report.md, do not start a second copy of the
command, and do not judge success or failure yet. The next action must be one
bounded status check of that process and its captured log, for example
`sleep 10; ps -p <pid>; tail -n 100 <log>`. Do not use `tail -f`.
```

Learning:

- "Still running after 30s" is a process-state signal, not a validation failure.
  Without a specific prompt rule, item worker converts it into a false timeout
  blocker.
- Telling the model to "observe" the process is too weak; it chose `tail -f`,
  which can wedge the next command. The retained control must say bounded status
  check and explicitly disallow `tail -f`.

## Experiment 79: Validation Runner Active Process Continuation

Question: when validation runner has already started a required validation
command and the latest tool output says it is still running with an active
process and log path, does it continue with a bounded process/log check instead
of writing validation status, starting duplicate validation, or idling?

Payload batch:

- `.pragma/prompt-ab/validation-runner-active-process-20260603T000000Z`

Setup:

- patch plan/checklist named required validation command
  `go test ./cmd/pragma/... -run TestFocusedCLI`,
- validation runner had already run the exact status-preserving wrapper,
- latest tool output said `Command still running after 30s`,
- latest tool output listed active pid `9021`, exact command, and captured log
  `/tmp/pragma/processes/9021.log`,
- log tail showed partial test progress but no PASS/FAIL or `EXIT_STATUS`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_active_process` | current validation runner | emitted `sleep 60`; it did not write a false status, but it also did not check the active process or log | Partial |
| `validation_runner_active_process_status_rule` | loose active-process status rule | emitted `sleep 10 && cat /tmp/pragma/processes/9021.log`; no status artifact or duplicate command | Passed |
| `validation_runner_active_process_bounded_rule` | bounded status/log rule with no `tail -f` | emitted `sleep 10; ps -p 9021; tail -n 100 /tmp/pragma/processes/9021.log` | Passed/retained |

Retained phrase:

```text
Active process bounded-status rule:
If the latest tool output says a required validation command is still running
and lists an active process for the exact command you started, do not write
/tmp/pragma/validation-status.md yet, do not start a second copy of the
command, and do not judge success or failure. The next action must be one
bounded status check of that process and its captured log, for example
`sleep 10; ps -p <pid>; tail -n 100 <log>`. Do not use `tail -f`.
```

Learning:

- Current validation runner avoided the worst failure, but `sleep 60` is still
  poor command shape because it does not gather new evidence and can make a
  phase feel stuck.
- The same bounded process/log check control that worked for item worker also
  works for validation runner when scoped to required validation commands and
  `/tmp/pragma/validation-status.md`.

## Experiment 80: Checklist Writer Missing Handoff Artifacts

Question: when checklist writer cannot read the handoff artifacts it needs, does
it write a blocker checklist item instead of reopening the repository, reopening
task prose, or emitting an empty checklist?

Payload batches:

- `.pragma/prompt-ab/checklist-missing-input-artifact-20260603T000000Z`
- `.pragma/prompt-ab/checklist-missing-input-artifact-both-20260603T000000Z`

Setup:

- first batch: `/tmp/pragma/patch-plan.md` had just returned `No such file or
  directory`,
- second batch: both `/tmp/pragma/patch-plan.md` and
  `/tmp/pragma/final-prosecutor-verdict.md` had returned `No such file or
  directory`,
- expected unrecoverable behavior after both checks was one blocked checklist
  item with empty `allowed_files`, `forbidden_files: ["**/*"]`, and no
  repository inspection.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_missing_patch_plan` | current checklist writer after only patch plan missing | checked `/tmp/pragma/final-prosecutor-verdict.md` | Passed as intermediate |
| `checklist_missing_input_artifact_rule` | broad missing-artifact rule after only patch plan missing | also checked final verdict | Passed as intermediate |
| `checklist_missing_input_schema_rule` | explicit missing-patch-plan template after only patch plan missing | wrote a blocker item immediately, but `acceptance` was empty and optional fields were `null`; also skipped the possible repair verdict check | Partial/rejected |
| `checklist_current_both_inputs_missing` | current checklist writer after both artifacts missing | wrote `/tmp/pragma/checklist.json` with `"items": []` | Failed |
| `checklist_missing_input_artifact_rule_both` | semantic missing-input artifact rule after both artifacts missing | wrote one blocked item naming missing artifacts, with acceptance, empty allowed files, forbidden `**/*`, and no repo inspection | Passed/retained |
| `checklist_missing_input_artifact_template` | exact blocker-item template after both artifacts missing | also wrote a valid blocked item | Passed/redundant |

Retained phrase:

```text
Missing input artifact rule:
If required handoff artifacts have been checked and neither
/tmp/pragma/patch-plan.md nor /tmp/pragma/final-prosecutor-verdict.md is
readable, do not inspect repository files, do not reopen original task prose,
and do not infer checklist items. Write /tmp/pragma/checklist.json with one
blocker item. The item must name the missing artifacts, set acceptance to
["required handoff artifact is provided"], allowed_files to [],
forbidden_files to ["**/*"], status to "blocked", and approach to "Do not
inspect repository files; return a blocker until the missing input artifact
is provided."
```

Learning:

- A missing patch plan alone is not necessarily the terminal state for checklist
  writer, because repair mode may still have a final-verdict authority artifact.
- Once both handoff artifacts are visibly missing, an empty checklist is a false
  success. Missing-artifact handling should produce one blocked item, not an
  empty `items` array.
- The exact template is unnecessary here; the semantic rule produced the right
  artifact and avoids overfitting to one artifact name.

## Experiment 81: Item Worker Missing Current Item Artifact

Question: after the required first read shows
`/tmp/pragma/current-item.json` is missing, does item worker write a blocker
implementer report instead of inspecting repository files, inventing an item, or
running validation?

Payload batch:

- `.pragma/prompt-ab/item-worker-missing-current-item-20260603T000000Z`

Setup:

- item worker first action was `cat /tmp/pragma/current-item.json`,
- tool output returned `No such file or directory`,
- expected behavior was `/tmp/pragma/implementer-report.md` with no changed
  files, blocked acceptance evidence, validation `N/A`, concrete blocker, and
  completion sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_missing_current_item` | current item worker | wrote a blocker implementer report, no repository inspection, no validation, sentinel present | Passed |
| `item_worker_missing_input_artifact_rule` | explicit missing-current-item rule | wrote equivalent blocker report | Passed/redundant |
| `item_worker_missing_input_shell_shape` | exact missing-current-item shell shape | wrote equivalent blocker report, but changed heredoc quoting style | Passed/redundant |

Learning:

- The existing malformed-artifact/blocker-report machinery already covers a
  missing required current item, even though the prompt says invalid/truncated
  or cannot be parsed rather than explicitly "missing".
- Do not add a separate missing-current-item rule yet. It increases prompt size
  without improving behavior in this replay.

## Experiment 82: Item Reviewer Missing Implementer Report

Question: after the required input read shows the current item is present but
`/tmp/pragma/implementer-report.md` is missing, does item reviewer write a
BLOCK verdict instead of inspecting the repository, running validation, or
approving from the item alone?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-missing-implementer-report-20260603T000000Z`

Setup:

- `/tmp/pragma/current-item.json` was readable and described an implementation
  item for `cmd/pragma/jobs.go`,
- `/tmp/pragma/implementer-report.md` returned `No such file or directory`,
- expected behavior was `/tmp/pragma/item-verdict.md` with concrete Findings,
  one-sentence Required repair, `Decision:\nBLOCK`, and sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_missing_report` | current item reviewer | wrote BLOCK verdict naming missing implementer report and missing Changed/validation/blocker evidence; no repo inspection or validation rerun | Passed |
| `item_reviewer_missing_input_artifact_rule` | generic missing-input rule | wrote shorter BLOCK verdict naming missing report | Passed/redundant |
| `item_reviewer_missing_report_specific_rule` | specific missing-report rule | wrote BLOCK verdict requiring item worker rerun with changed files, acceptance evidence, validation, risk, and blocker status | Passed/redundant |

Learning:

- Current item reviewer already treats a missing implementer report as missing
  review evidence and blocks cleanly.
- Do not retain a separate missing-report rule yet. It does not change the
  decision or action shape in this replay.

## Experiment 83: Final Reviewer Missing Validation Status

Question: after the required first read shows
`/tmp/pragma/validation-status.md` is missing, does final reviewer write a BLOCK
verdict instead of inspecting diff, running validation, or approving without the
validation contract?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-missing-validation-status-20260603T000000Z`

Setup:

- final reviewer first action was `cat /tmp/pragma/validation-status.md`,
- tool output returned `No such file or directory`,
- expected behavior was `/tmp/pragma/final-prosecutor-verdict.md` with
  `Decision:\nBLOCK`, one-sentence Required repair, and sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_missing_validation_status` | current final reviewer | wrote BLOCK verdict naming missing `/tmp/pragma/validation-status.md`; no diff, validation, or repo inspection | Passed |
| `final_reviewer_missing_input_artifact_rule` | generic missing-input rule | wrote equivalent BLOCK verdict, but Findings lost bullet formatting | Passed/redundant |
| `final_reviewer_missing_status_specific_rule` | specific missing-status rule | wrote equivalent BLOCK verdict with more detailed required repair | Passed/redundant |

Learning:

- Current final reviewer already treats missing validation status as incomplete
  validation and blocks directly.
- Do not add missing-status wording yet. The generic rule slightly worsened
  verdict formatting and neither mutation changed the action.

## Experiment 84: Surface Mapper After Direct Read And Task/Source Conflict

Question: after a task-named source file has already been directly read, does
surface mapper write `/tmp/pragma/surface-map.md` from the visible source output
instead of continuing search, and can it avoid treating task prose values as
source evidence when source shows a different value?

Payload batches:

- `.pragma/prompt-ab/surface-mapper-task-source-conflict-20260603T000000Z`
- `.pragma/prompt-ab/surface-mapper-after-direct-read-conflict-20260603T000000Z`

Setup:

- task named `cmd/pragma/jobs.go`,
- task prose said the CLI default for `--max-retries` should be `5`,
- visible direct source read showed `const defaultMaxRetries = 3` and
  `cmd.Flags().Int("max-retries", defaultMaxRetries, ...)`,
- expected behavior was to write the surface map from the visible source read,
  not run another search.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `surface_mapper_current_task_source_conflict` | current surface mapper | ran another `rg` over `addJobFlags|max-retries|defaultMaxRetries` instead of writing surface map | Failed |
| `surface_mapper_conflict_unknown_rule` | task/source conflict rule only | still ran another `rg` | Failed |
| `surface_mapper_conflict_no_promotion_rule` | no-promotion-from-task-prose rule only | still ran another `rg` | Failed |
| `surface_mapper_after_read_rule` | after-direct-source-read rule only | still ran another `rg` | Failed |
| `surface_mapper_after_read_conflict_rule` | after-direct-source-read rule plus task/source conflict rule | wrote `/tmp/pragma/surface-map.md` from visible `cmd/pragma/jobs.go`, with candidate paths `none`, source value `3`, task target `5`, and unknown adjacent tests/references | Passed/retained |
| `surface_mapper_after_read_shell_shape` | combined rule plus exact shell shape | wrote surface map, but set `Explicit unknowns: none`, losing adjacent proof uncertainty | Partial/rejected |

Retained phrase:

```text
After direct source read with task/source conflict rule:
If the latest visible tool output is the content of a task-named source path,
write /tmp/pragma/surface-map.md from that output now. Do not run rg, find,
ls, grep, validation, or another source inspection before writing the
artifact. Candidate paths may be none when the task-named source path was
already read. If task prose names a literal, field, key, expected value, or
behavior, but the direct inspection shows a different literal/value or does
not prove the requested behavior, do not treat the task prose as evidence.
Record the task prose under Named literals/fields/keys/tests/fixtures, record
what the inspected source actually showed under First inspections, and put
unresolved mismatches or adjacent proof needs under Explicit unknowns.
```

Learning:

- The surface mapper needed a state-transition control, not just conflict
  wording. Conflict/uncertainty rules alone did not stop another `rg`.
- The after-direct-read rule alone also failed, likely because the existing work
  method still invited adjacent-surface discovery. Pairing it with explicit
  task/source evidence discipline made the artifact write happen.
- Exact shell-shape wording can overconstrain uncertainty; in this replay it
  wrote the artifact but dropped useful unknowns.

## Experiment 85: Evidence Mapper Conflict Surface Completion

Question: after surface mapper writes a conflict-shaped surface map and evidence
mapper has visible inspections for the listed source/test surfaces, does
evidence mapper write `/tmp/pragma/evidence-map.md` instead of continuing broad
search or promoting task prose values into evidence?

Payload batch:

- `.pragma/prompt-ab/evidence-mapper-conflict-surface-completion-20260603T000000Z`

Setup:

- surface map named `cmd/pragma/jobs.go` as task-named source path,
- surface map recorded task prose target value `5` and inspected source value
  `defaultMaxRetries = 3`,
- adjacent surface was `cmd/pragma/jobs_test.go`,
- visible source/test output showed the default constant and
  `TestMaxRetriesDefault`,
- explicit unknown was whether other code references `defaultMaxRetries`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `evidence_mapper_current_conflict_surface_completion` | current evidence mapper | wrote evidence map with source path, consumer/defaulting test path, focused validation command, generated/producer `none`, and explicit unknowns under `Not evidenced` | Passed |
| `evidence_mapper_after_listed_inspections_rule` | explicit after-listed-inspections write rule | wrote equivalent evidence map | Passed/redundant |
| `evidence_mapper_conflict_preservation_rule` | explicit surface-conflict preservation rule | wrote equivalent evidence map | Passed/redundant |

Learning:

- The retained surface-mapper conflict output composes with current evidence
  mapper. Once source/defaulting/test evidence is visible, the existing
  completion decision rule writes the evidence artifact.
- Do not add extra evidence-mapper conflict wording yet. Current evidence
  mapper already treats unresolved references as `Not evidenced` and does not
  promote task prose values into implementation proof in this replay.

## Experiment 86: Patch Planner Conflict Evidence Route

Question: after evidence mapper produces a source/test evidence map from a
task/source value conflict, does patch planner write the bounded source patch
route while keeping unrelated `Not evidenced` references unresolved?

Payload batch:

- `.pragma/prompt-ab/patch-planner-conflict-evidence-route-20260603T000000Z`

Setup:

- surface map said task target value was `5` while inspected source value was
  `defaultMaxRetries = 3`,
- evidence map listed source path `cmd/pragma/jobs.go`,
- evidence map listed consumer/defaulting path `cmd/pragma/jobs_test.go`,
- evidence map listed focused validation command
  `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- `Not evidenced` listed unknown references to `defaultMaxRetries` and broad
  runtime/proto/generated surfaces.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `patch_planner_current_conflict_evidence_route` | current patch planner | wrote source route changing `defaultMaxRetries` from 3 to 5 and left other references unresolved | Passed |
| `patch_planner_not_evidenced_nonblocking_rule` | explicit rule that unresolved references should not block an evidenced route | wrote equivalent source route | Passed/redundant |
| `patch_planner_conflict_source_route_rule` | explicit conflict-source route rule | wrote equivalent source route, but changed unsafe shortcut to `editing repository files`, which is not useful | Passed/redundant |

Learning:

- Current patch planner already treats concrete source/test/validation evidence
  as sufficient for a bounded patch route, while keeping unrelated
  `Not evidenced` references under unresolved proof.
- Do not add downstream conflict-route wording yet. The existing evidence
  precedence and uncertainty rules cover this replay, and one mutation degraded
  the unsafe-shortcut field.

## Experiment 87: Checklist Writer Conflict Patch Route

Question: when checklist writer receives a source-owned patch route with a
consumer/defaulting test path and unrelated unresolved proof, does it create one
source-edit item instead of splitting consumer/test edits or turning unresolved
proof into implementation scope?

Payload batches:

- `.pragma/prompt-ab/checklist-conflict-patch-route-20260603T000000Z`
- `.pragma/prompt-ab/checklist-conflict-patch-route-strict-20260603T000000Z`

Setup:

- patch route behavior was `Change defaultMaxRetries constant from 3 to 5`,
- `source path: cmd/pragma/jobs.go`,
- `consumer/defaulting path: cmd/pragma/jobs_test.go`,
- `fixture/schema tier: cmd/pragma/jobs_test.go`,
- `validation: go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- `Unresolved until evidence` only mentioned whether other code references
  `defaultMaxRetries`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_conflict_patch_route` | current checklist writer | wrote three items: source edit, test edit, and blocked unresolved-proof item; validation stayed only in acceptance | Failed |
| `checklist_exact_source_change_handoff_rule` | exact source-change field only | preserved `exact_source_changes`, removed test-edit item, but still wrote blocked unresolved-proof item and missed `validation_command` | Partial |
| `checklist_unresolved_not_pending_rule` | unresolved-proof separation only | abandoned artifact writing and checked final verdict | Failed |
| `checklist_route_role_rule` | route role rule only | removed test-edit item but still wrote blocked unresolved-proof item | Partial |
| `checklist_validation_field_handoff_rule` | validation field handoff only | copied `validation_command`, but allowed source and test edits and wrote blocked unresolved-proof item | Partial |
| `checklist_route_role_unresolved_rule` | route role plus unresolved separation | wrote one pending source item with `allowed_files: ["cmd/pragma/jobs.go"]` and exact `validation_command`; no test-edit item and no unresolved-proof item | Passed/retained |
| `checklist_full_handoff_rule` | route role, unresolved separation, exact source changes, validation handoff | wrote one source item with validation command, but lost `exact_source_changes` and used numeric id | Passed/rejected |

Retained phrase:

```text
Patch-route role and unresolved separation rule:
For each patch-route bullet, create one pending item for the `source path` only.
`consumer/defaulting path` and `fixture/schema tier` are evidence and validation
context, not editable files, unless the route behavior explicitly says to edit
them. Entries under `Unresolved until evidence` are not checklist items during
initial implementation; do not create pending or blocked items for them unless a
final verdict explicitly requests that repair.
```

Learning:

- Checklist writer was over-expanding evidence roles into work roles. A
  consumer/defaulting test path is not an editable file just because it appears
  in a patch route.
- Unresolved proof is not a checklist item during initial implementation. It
  remains proof debt unless a final verdict turns it into a concrete repair.
- Exact-source-change handoff is still not stable in this shape. One mutation
  preserved it but failed unresolved separation; the combined mutation fixed the
  item shape but dropped `exact_source_changes`. Do not retain that field rule
  from this replay.

## Experiment 88: Item Worker Conflict Source Item

Question: after checklist writer creates one source-owned item without an
`exact_source_changes` field, can item worker use the item title/description and
visible allowed source file to make the bounded source edit?

Payload batch:

- `.pragma/prompt-ab/item-worker-conflict-source-item-20260603T000000Z`

Setup:

- current item allowed only `cmd/pragma/jobs.go`,
- title/description/approach all said to change `defaultMaxRetries` from `3`
  to `5`,
- validation command was `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- visible source contained `const defaultMaxRetries = 3`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_conflict_source_item` | current item worker | emitted one edit command changing `const defaultMaxRetries = 3` to `const defaultMaxRetries = 5` in the allowed file | Passed |
| `item_worker_title_approach_source_edit_rule` | explicit source-edit-from-item-text rule | emitted the same edit command | Passed/redundant |
| `item_worker_source_edit_then_validation_rule` | explicit source-edit-then-validation rule | emitted the same edit command | Passed/redundant |

Learning:

- For a simple visible value edit, item worker does not require an
  `exact_source_changes` field when title/description/approach and the visible
  allowed source file are aligned.
- Do not add source-edit-from-title wording yet. Current item-worker source
  minimality and allowed-file controls are enough in this replay.

## Experiment 89: Item Worker Post Source Edit Validation

Question: after item worker has successfully edited an allowed source file, does
it run the focused item validation command with the same status-preserving
wrapper required for validation-only items?

Payload batch:

- `.pragma/prompt-ab/item-worker-post-source-edit-validation-20260603T000000Z`

Setup:

- current item allowed only `cmd/pragma/jobs.go`,
- source file had been visibly read,
- item worker had just run a successful `sed` edit changing
  `defaultMaxRetries = 3` to `defaultMaxRetries = 5`,
- current item contained `validation_command:
  go test -run TestMaxRetriesDefault ./cmd/pragma/`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_post_source_edit_validation` | current item worker | ran focused validation, but only as `go test ... 2>&1; echo "EXIT_STATUS: $?"`; no log, no `set -o pipefail`, no final `exit "$status"` | Partial |
| `item_worker_post_source_edit_validation_rule` | semantic post-source-edit validation rule | same weak command shape as current | Partial |
| `item_worker_post_source_edit_wrapper_rule` | exact post-source-edit validation shell rule | ran exact validation command with `set -o pipefail`, log capture, `${PIPESTATUS[0]}`, `EXIT_STATUS`, and `exit "$status"` | Passed/retained |

Retained phrase:

```text
Post-source-edit validation shell rule:
After a successful edit to an allowed source file, the next action must run
validation_command with this exact status-preserving shape:
set -o pipefail
log=/tmp/pragma/item-validation.log
<validation_command> 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
echo "EXIT_STATUS: $status"
exit "$status"
Do not inspect more files before this validation.
```

Learning:

- Current item worker knows to validate after a successful source edit, but the
  command shape is too weak for downstream evidence because it does not preserve
  the process exit as the shell exit.
- Semantic "run validation next" wording does not fix command shape. The
  retained control needs the exact shell wrapper, mirroring validation-only item
  behavior.

## Experiment 90: Item Worker Post Source Edit Report

Question: after an allowed source edit and focused validation output with
`EXIT_STATUS: 0`, does item worker write the implementer report with the changed
file and clean blocker state?

Payload batch:

- `.pragma/prompt-ab/item-worker-post-source-edit-report-20260603T000000Z`

Setup:

- current item allowed only `cmd/pragma/jobs.go`,
- item worker had already edited `defaultMaxRetries` from `3` to `5`,
- exact validation command ran with the retained status-preserving wrapper,
- visible output included `ok ./cmd/pragma 0.42s` and `EXIT_STATUS: 0`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_post_source_edit_report` | current item worker after Experiment 89 | wrote `/tmp/pragma/implementer-report.md` with `Changed: cmd/pragma/jobs.go`, passed validation, `Remaining risk: none`, and `Blocker: none` | Passed |
| `item_worker_changed_file_report_rule` | explicit changed-file report rule | wrote equivalent report with slightly more output detail in acceptance evidence | Passed/redundant |
| `item_worker_changed_file_report_shape` | exact changed-file report shell shape | wrote equivalent report | Passed/redundant |

Learning:

- Once focused validation output has the exact command, ok/PASS output, and
  `EXIT_STATUS: 0`, the existing passed-validation report rule handles changed
  source files correctly.
- No changed-file report rule is retained. The missing piece was the validation
  wrapper before this state, not report generation after the state is clean.

## Experiment 91: Item Reviewer Source Edit Approval

Question: when item reviewer sees a clean implementer report for one allowed
source edit, does it approve without rerunning validation or inspecting the
repository?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-source-edit-approve-20260603T000000Z`

Setup:

- current item allowed only `cmd/pragma/jobs.go`,
- implementer report `Changed` named only `cmd/pragma/jobs.go`,
- acceptance evidence said PASS,
- validation named the exact focused command
  `go test -run TestMaxRetriesDefault ./cmd/pragma/` and `EXIT_STATUS: 0`,
- remaining risk and blocker were both `none`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_source_edit_approve` | current item reviewer | wrote APPROVE verdict with changed file in allowed scope and exact validation success; no rerun or repo inspection | Passed |
| `item_reviewer_source_edit_visible_pass_rule` | explicit source-edit visible-pass rule | wrote APPROVE, but used four Findings bullets, exceeding the strict-size rule | Partial/rejected |
| `item_reviewer_source_edit_approve_shape` | exact source-edit approve shell shape | wrote equivalent APPROVE verdict | Passed/redundant |

Learning:

- Current item reviewer already handles clean source-edit implementer reports
  through the visible-pass and changed-section auditability rules.
- Do not add source-edit approval wording. It did not improve the decision, and
  the semantic rule regressed verdict compactness.

## Experiment 92: Validation Runner Source Edit Chain Command

Question: after the source-edit item is approved and patch plan/checklist name
one required focused validation command, does validation runner run the exact
command with status preservation instead of writing status too early or using
acceptance prose?

Payload batch:

- `.pragma/prompt-ab/validation-runner-source-edit-chain-20260603T000000Z`

Setup:

- patch plan route validation was
  `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- checklist item had the same exact `validation_command`,
- checklist item status was `completed`,
- expected action was one focused validation run with status preservation and no
  `/tmp/pragma/validation-status.md` write in the same turn.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_source_edit_chain` | current validation runner | ran exact command with `set -o pipefail`, log capture, `${PIPESTATUS[0]}`, `COMMAND[1]`, `EXIT_STATUS[1]`, and shell exit status | Passed |
| `validation_runner_single_command_wrapper_restated` | explicit single-command wrapper rule | ran equivalent non-indexed wrapper | Passed/redundant |
| `validation_runner_no_acceptance_text_rule` | explicit command-source rule | ran equivalent indexed wrapper | Passed/redundant |

Learning:

- Current validation runner already preserves status for the source-edit chain.
  Indexed command/status output for a single command is acceptable and compatible
  with its indexed status-writing rule.
- Do not add single-command restatement or command-source wording yet; neither
  changed the meaningful behavior in this replay.

## Experiment 93: Validation Runner Source Edit Status

Question: after indexed focused validation output shows `EXIT_STATUS[1]: 0`,
does validation runner write a clean validation-status artifact with concrete
validated surfaces and without turning unrelated unresolved proof into a
failure?

Payload batch:

- `.pragma/prompt-ab/validation-runner-source-edit-status-20260603T000000Z`

Setup:

- patch plan and checklist named required command
  `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- visible validation output contained `COMMAND[1]`, `ok ./cmd/pragma`, and
  `EXIT_STATUS[1]: 0`,
- patch plan also had unrelated `Unresolved until evidence` about other
  references to `defaultMaxRetries`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_source_edit_status` | current validation runner | wrote clean status with command/status, failures none, `missing_files_or_surfaces: none`, validated surfaces `cmd/pragma/jobs.go` and `cmd/pragma/jobs_test.go`, producer evidence none | Passed |
| `validation_runner_source_edit_status_surfaces_rule` | explicit source-edit surfaces rule | wrote validated surfaces, but incorrectly copied unresolved proof into `missing_files_or_surfaces` | Failed/rejected |
| `validation_runner_clean_source_status_shape` | exact clean source status shape | wrote equivalent clean status | Passed/redundant |

Learning:

- Current validation runner already writes the right clean status for the
  source-edit chain.
- Do not add wording that asks it to reason about unresolved proof while writing
  clean validation status. That caused unrelated `Unresolved until evidence`
  entries to become false missing surfaces.

## Experiment 94: Final Reviewer Source Edit Clean Diff

Question: after clean validation status and visible source-only diff, does final
reviewer approve without requiring producer evidence, rerunning validation, or
blocking on earlier unresolved proof that is not in validation status?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-source-edit-clean-diff-20260603T000000Z`

Setup:

- validation status had `EXIT_STATUS[1]: 0`, all failure fields `none`,
  `missing_files_or_surfaces: none`, validated surfaces
  `cmd/pragma/jobs.go` and `cmd/pragma/jobs_test.go`, and producer evidence
  `none`,
- visible diff changed only `cmd/pragma/jobs.go`,
- diff changed `const defaultMaxRetries = 3` to `const defaultMaxRetries = 5`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_source_edit_clean_diff` | current final reviewer | wrote APPROVE verdict; recognized single validated production source diff and clean validation | Passed |
| `final_reviewer_source_edit_approve_rule` | explicit source-edit clean approval rule | wrote equivalent APPROVE verdict | Passed/redundant |
| `final_reviewer_ignore_unseen_unresolved_rule` | explicit final-review validation-status authority rule | wrote equivalent APPROVE verdict | Passed/redundant |

Learning:

- Current final reviewer already approves clean source-only diffs when validation
  status names the changed source file and no failure/missing fields are present.
- Producer evidence is not required for non-generated source-only diffs.
- Earlier unresolved proof does not affect final review unless it appears in
  validation status or visible diff evidence; no extra authority rule is
  retained.

## Experiment 95: Checklist Repair Out-of-Scope Diff

Question: during repair checklist generation, when the final reviewer blocks
because the visible diff contains one out-of-scope file, does checklist writer
preserve completed prior items and add only one bounded repair item for the
out-of-scope file?

Payload batch:

- `.pragma/prompt-ab/checklist-repair-out-of-scope-diff-20260603T000000Z`

Setup:

- final verdict said validation passed for `cmd/pragma/jobs.go`, but visible
  diff also changed `internal/server/auth/debug.go`,
- verdict named `internal/server/auth/debug.go` as out-of-scope because it was
  not validated, not allowed, and not required by the patch route,
- Required repair said to remove or revert only
  `internal/server/auth/debug.go` and keep the completed
  `cmd/pragma/jobs.go` item unchanged,
- patch plan allowed only `cmd/pragma/jobs.go` and listed
  `internal/server/auth/**` as forbidden product scope,
- prior checklist contained one completed `cmd/pragma/jobs.go` item.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_out_of_scope_repair` | current checklist writer | preserved the completed `cmd/pragma/jobs.go` item and added one pending repair item with `allowed_files: ["internal/server/auth/debug.go"]`, empty `forbidden_files`, and no broad task reopening | Passed |
| `checklist_out_of_scope_repair_rule` | explicit out-of-scope diff repair rule | produced equivalent bounded repair item and preserved the completed source item | Passed/redundant |
| `checklist_out_of_scope_repair_preserve_rule` | out-of-scope rule plus completed-item preservation rule | preserved the completed source item and added the bounded repair item, but put `cmd/pragma/jobs.go` in the pending repair item's `forbidden_files` even though that file was the preserved completed item | Partial/rejected |

Learning:

- The existing repair authority and completed-item preservation behavior is
  enough for compact out-of-scope-diff repair payloads.
- Do not add a dedicated out-of-scope repair rule yet. It did not improve the
  current behavior.
- Do not add an extra completed-item preservation rule in this form. It can
  create misleading per-item `forbidden_files` entries for already-completed
  items.

## Experiment 96: Item Worker Out-of-Scope Repair First Action

Question: after checklist writer creates a pending repair item for one
out-of-scope changed file, does item worker inspect only the exact allowed-file
diff before editing, instead of broadening into the surrounding subsystem or
reverting from memory?

Payload batch:

- `.pragma/prompt-ab/item-worker-out-of-scope-repair-first-action-20260603T000000Z`

Setup:

- current item was the pending repair item from Experiment 95,
- title/description/approach all said to remove or revert only
  `internal/server/auth/debug.go`,
- `allowed_files` contained only `internal/server/auth/debug.go`,
- `forbidden_files` was empty,
- validation command was
  `git diff --exit-code -- internal/server/auth/debug.go`,
- no file content or diff was visible yet.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_oos_repair_first_action` | current item worker | ran exactly `git diff -- internal/server/auth/debug.go` | Passed |
| `item_worker_oos_repair_diff_rule` | explicit out-of-scope repair diff-inspection rule | ran the same exact bounded diff command | Passed/redundant |

Learning:

- Current item worker already treats an out-of-scope repair item as an
  allowed-file-local diff problem on the first implementation turn.
- Do not retain a dedicated out-of-scope repair inspection rule yet; the
  existing path, scope authority, scope control, and work-method rules were
  sufficient in this compact payload.

## Experiment 97: Item Worker Out-of-Scope Repair Visible Diff

Question: after item worker sees the exact allowed-file diff for a newly-added
out-of-scope file, does it remove only that file instead of broadening into the
surrounding subsystem or touching the completed item?

Payload batch:

- `.pragma/prompt-ab/item-worker-out-of-scope-repair-visible-diff-20260603T000000Z`

Setup:

- current item was the pending repair item from Experiments 95 and 96,
- `allowed_files` contained only `internal/server/auth/debug.go`,
- visible diff showed `internal/server/auth/debug.go` as a newly-added file,
- prior `cmd/pragma/jobs.go` item was completed and explicitly not to be
  touched.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_oos_repair_visible_diff` | current item worker | ran exactly `rm internal/server/auth/debug.go` | Passed |
| `item_worker_oos_visible_diff_repair_rule` | explicit visible-diff repair rule, including same-script validation wording | ran exactly `rm internal/server/auth/debug.go`; did not include validation in the same script | Passed for bounded edit; validation wording redundant/ineffective |

Learning:

- Current item worker already makes the bounded repair edit once the exact
  out-of-scope allowed-file diff is visible.
- The extra visible-diff repair rule is not retained. It did not improve the
  edit behavior, and its same-script validation clause did not change the
  model's turn boundary.
- For this loop shape, validation is tested as the next turn after visible edit
  success, not assumed to happen in the same shell script.

## Experiment 98: Item Worker Out-of-Scope Repair Post-Remove Validation

Question: after the bounded out-of-scope file removal visibly succeeds, does
item worker run the exact validation command with the status-preserving wrapper?

Payload batch:

- `.pragma/prompt-ab/item-worker-out-of-scope-repair-post-rm-validation-20260603T000000Z`

Setup:

- same repair item as Experiments 95-97,
- visible prior command was `rm internal/server/auth/debug.go`,
- validation command was
  `git diff --exit-code -- internal/server/auth/debug.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_oos_repair_post_rm_validation` | current item worker | ran exact validation command using `set -o pipefail`, `tee`, `${PIPESTATUS[0]}`, `EXIT_STATUS`, and `exit "$status"` | Passed |
| `item_worker_oos_repair_post_rm_validation_rule` | explicit out-of-scope post-edit validation rule | generated the same wrapper | Passed/redundant |

Learning:

- The existing post-source-edit validation shell rule generalizes to bounded
  out-of-scope repair removals.
- No repair-specific post-edit validation rule is retained.

## Experiment 99: Item Worker Out-of-Scope Repair Report

Question: after the repair validation command visibly exits 0, does item worker
write a clean implementer report and stop?

Payload batch:

- `.pragma/prompt-ab/item-worker-out-of-scope-repair-report-20260603T000000Z`

Setup:

- same repair item as Experiments 95-98,
- visible validation output included
  `git diff --exit-code -- internal/server/auth/debug.go` and
  `EXIT_STATUS: 0`,
- prior completed item remained out of scope for this worker turn.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_worker_current_oos_repair_report` | current item worker | wrote `/tmp/pragma/implementer-report.md` with changed file `internal/server/auth/debug.go - removed out-of-scope file`, PASS evidence, exact validation command, `Remaining risk: none`, and `Blocker: none` | Passed |
| `item_worker_oos_repair_report_rule` | explicit out-of-scope repair report rule | wrote the same report | Passed/redundant |

Learning:

- Current passed-validation report machinery already handles out-of-scope
  repair items after a clean exact-file diff validation.
- Do not retain a repair-specific report rule.

## Experiment 100: Item Reviewer Out-of-Scope Repair Approval

Question: after item worker reports a clean out-of-scope repair, does item
reviewer approve without rerunning validation or reopening surrounding auth
scope?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-out-of-scope-repair-approve-20260603T000000Z`

Setup:

- current item allowed only `internal/server/auth/debug.go`,
- implementer report changed
  `internal/server/auth/debug.go - removed out-of-scope file`,
- acceptance evidence and validation both named
  `git diff --exit-code -- internal/server/auth/debug.go` with
  `EXIT_STATUS: 0`,
- remaining risk and blocker were both `none`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `item_reviewer_current_oos_repair_approve` | current item reviewer | wrote APPROVE with three findings: repair file changed, exact validation passed, changed file in allowed scope | Passed |
| `item_reviewer_remove_revert_approve_rule` | explicit remove/revert approval rule | wrote APPROVE, but used four Findings bullets and violated the strict verdict size rule | Partial/rejected |

Learning:

- Current item reviewer already approves clean remove/revert repair reports
  through the generic visible-pass and changed-section auditability rules.
- Do not retain a remove/revert-specific approval rule in this form; it
  regressed compactness.

## Experiment 101: Validation Runner Out-of-Scope Repair Chain

Question: after both original implementation and out-of-scope repair checklist
items are completed, does validation runner execute both required validation
commands with indexed statuses?

Payload batch:

- `.pragma/prompt-ab/validation-runner-out-of-scope-repair-chain-20260603T000000Z`

Setup:

- patch plan required
  `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- checklist had a completed original item with the same command,
- checklist also had a completed repair item with
  `git diff --exit-code -- internal/server/auth/debug.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_oos_repair_chain` | current validation runner | ran both commands in the required indexed multi-command wrapper and exited nonzero if any command failed | Passed |
| `validation_runner_repair_item_command_rule` | explicit repair-item validation command rule | generated the same wrapper | Passed/redundant |

Learning:

- Current validation runner already treats completed repair item validation
  commands as required validation commands when they are present in checklist.
- No repair-command execution rule is retained.

## Experiment 102: Validation Runner Out-of-Scope Repair Status

Question: after both indexed validation commands exit 0, does validation runner
write status with both command statuses, no missing surfaces, and validated
surfaces covering the repair diff command path?

Payload batch:

- `.pragma/prompt-ab/validation-runner-out-of-scope-repair-status-20260603T000000Z`
- `.pragma/prompt-ab/validation-runner-repair-surface-mapping-20260603T000000Z`
- `.pragma/prompt-ab/validation-runner-git-diff-surface-mechanical-20260603T000000Z`
- `.pragma/prompt-ab/validation-runner-git-diff-surface-position-20260603T000000Z`
- `.pragma/prompt-ab/validation-runner-git-diff-surface-retained-yaml-20260603T000000Z`

Setup:

- visible validation output included:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `EXIT_STATUS[1]: 0`,
  - `COMMAND[2]: git diff --exit-code -- internal/server/auth/debug.go`,
  - `EXIT_STATUS[2]: 0`,
- checklist item for command 2 allowed only
  `internal/server/auth/debug.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `validation_runner_current_oos_repair_status` | current validation runner | wrote both command statuses and no missing surfaces, but omitted `internal/server/auth/debug.go` from `validated_surfaces` | Failed |
| `validation_runner_repair_status_surface_rule` | repair validation status surface rule near existing validated-surfaces rule | still omitted `internal/server/auth/debug.go`; added only `cmd/pragma/jobs_test.go` | Failed |
| `validation_runner_command_to_checklist_surface_rule` | exact-match command-to-checklist wording | omitted repair path | Failed |
| `validation_runner_surface_extraction_order_rule` | ordered extraction from patch plan and checklist | omitted repair path | Failed |
| `validation_runner_repair_diff_surface_rule` | explicit repair `git diff --exit-code -- X` surface wording | omitted repair path | Failed |
| `validation_runner_git_diff_surface_rule` | mechanical git-diff path rule near indexed status rule | omitted repair path | Failed |
| `validation_runner_surface_command_parser_rule` | command parser wording near indexed status rule | omitted repair path | Failed |
| `validation_runner_invalid_omission_guard` | omission-invalid guard near indexed status rule | omitted repair path | Failed |
| `validation_runner_git_diff_footer_in_persona` | same mechanical rule appended as persona footer | included `internal/server/auth/debug.go` in `validated_surfaces` | Passed |
| `validation_runner_git_diff_latest_user_footer` | same mechanical rule as latest user footer | included `internal/server/auth/debug.go` in `validated_surfaces` | Passed |
| `validation_runner_git_diff_system_footer` | system-message footer | still omitted repair path | Failed |
| `validation_runner_retained_footer_current` | updated `personas-research-v2/validation_runner.yaml` with retained footer | included `internal/server/auth/debug.go` in `validated_surfaces` | Passed |

Retained control:

```text
Last-mile status rule:
Before writing /tmp/pragma/validation-status.md, inspect the visible
Commands run lines. For every successful command matching
`git diff --exit-code -- <path>`, copy `<path>` verbatim into
validated_surfaces. Do this even if the command produced no output. Do not
omit repair diff paths from validated_surfaces.
```

Learning:

- This was a position-sensitive prompt control. Similar semantic and mechanical
  rules failed when inserted earlier in the persona prompt, including near the
  indexed status-writing rule.
- Persona-footer and latest-user-footer placement both worked; system-footer
  placement did not.
- The retained form is the persona-footer wording because it can live in
  `personas-research-v2/validation_runner.yaml` without requiring a
  task-specific latest-message injection.

## Experiment 103: Final Reviewer Out-of-Scope Repair Clean Diff

Question: after validation status includes both the original focused validation
and the successful out-of-scope repair diff validation, does final reviewer
approve when the visible final diff contains only the intended source change?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-out-of-scope-repair-clean-20260603T000000Z`

Setup:

- validation status contained:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/;
    EXIT_STATUS[1]: 0`,
  - `COMMAND[2]: git diff --exit-code -- internal/server/auth/debug.go;
    EXIT_STATUS[2]: 0`,
  - all failure/missing fields `none`,
  - `validated_surfaces` with `cmd/pragma/jobs.go`,
    `cmd/pragma/jobs_test.go`, and `internal/server/auth/debug.go`,
- visible diff contained only `cmd/pragma/jobs.go`, changing
  `defaultMaxRetries` from 3 to 5,
- the repaired auth file had no remaining diff.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_oos_repair_clean` | current final reviewer | wrote APPROVE, summarizing both passing commands and the single remaining `cmd/pragma/jobs.go` diff | Passed |
| `final_reviewer_repair_surface_no_diff_rule` | explicit rule that repaired validation surfaces need not appear in final diff | wrote equivalent APPROVE | Passed/redundant |

Learning:

- Current final reviewer already handles the clean end state after
  out-of-scope repair: every visible changed file must be covered by
  `validated_surfaces`, but successful repair validation surfaces do not need
  to remain in the final diff.
- Do not retain a final-review repair-surface no-diff rule yet; it did not
  improve behavior in this replay.

## Experiment 104: Final Reviewer Dirty Repair Diff Block

Question: if validation status contains a successful
`git diff --exit-code -- <path>` repair command but the visible final diff still
contains `<path>`, can final reviewer write a BLOCK verdict instead of approving
or getting stuck in reasoning?

Payload batches:

- `.pragma/prompt-ab/final-reviewer-out-of-scope-repair-still-dirty-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-diff-controls-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-diff-precise-controls-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-diff-codeonly-controls-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-diff-minimal-fix-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-output-first-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-latest-control-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-exact-latest-as-persona-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-generic-footer-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-retained-generic-footer-verify-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-dirty-repair-contradiction-footer-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-retained-contradiction-shortcut-verify-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-decision-precedence-20260603T000000Z`

Setup:

- validation status contained:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/;
    EXIT_STATUS[1]: 0`,
  - `COMMAND[2]: git diff --exit-code -- internal/server/auth/debug.go;
    EXIT_STATUS[2]: 0`,
  - all failure/missing fields `none`,
  - `validated_surfaces` with `cmd/pragma/jobs.go`,
    `cmd/pragma/jobs_test.go`, and `internal/server/auth/debug.go`,
- visible final diff still contained both:
  - intended `cmd/pragma/jobs.go` default value change,
  - out-of-scope new file `internal/server/auth/debug.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_oos_repair_still_dirty` | current final reviewer | entered repeated hidden reasoning and finished with `finish_reason: length`, no verdict content | Failed |
| `final_reviewer_repair_diff_command_authority_rule` | semantic no-diff repair command authority rule before diff hygiene | also repeated reasoning to `finish_reason: length` | Failed |
| `final_reviewer_dirty_repair_exact_footer` | footer with exact BLOCK shape | ran `git status` instead of writing verdict | Failed |
| `final_reviewer_dirty_repair_footer_rule` | footer semantic rule | repeated reasoning to length | Failed |
| `final_reviewer_dirty_repair_immediate_rule` | immediate rule before diff hygiene | wrote a code-only BLOCK artifact, but included a bad producer-evidence finding for a non-generated file | Partial/rejected |
| `final_reviewer_dirty_repair_latest_user_footer` | latest user footer from first batch | wrote code-only BLOCK, but Required repair said “or clarify its validated surface status” | Partial/rejected |
| `final_reviewer_dirty_repair_system_rule` | system-message rule | repeated reasoning to length | Failed |
| `final_reviewer_dirty_repair_precise_before_work` | precise rule before work method | wrote BLOCK but emitted prose before the bash block | Failed |
| `final_reviewer_dirty_repair_precise_immediate` | precise rule before diff hygiene | wrote BLOCK but emitted prose before the bash block | Failed |
| `final_reviewer_dirty_repair_precise_with_template` | precise rule plus template | repeated reasoning to length | Failed |
| `final_reviewer_dirty_repair_codeonly_exactish` | exact-ish code-only template | repeated reasoning to length | Failed |
| `final_reviewer_dirty_repair_codeonly_shortcut` | code-only shortcut | wrote BLOCK but emitted prose before the bash block | Failed |
| `final_reviewer_dirty_repair_codeonly_shorter` | shorter code-only shortcut | wrote BLOCK but emitted prose before the bash block | Failed |
| `final_reviewer_dirty_repair_minimal_no_producer_rule` | minimal no-producer addition | wrote BLOCK but emitted prose before the bash block | Failed |
| `final_reviewer_dirty_repair_original_authority_rule` | original authority rule retest | repeated reasoning to length | Failed |
| `final_reviewer_output_first_dirty_footer` | top output-first rule plus footer dirty rule | repeated reasoning to length | Failed |
| `final_reviewer_output_first_dirty_rule` | top output-first rule plus inserted dirty rule | wrote BLOCK but emitted prose before the bash block | Failed |
| `final_reviewer_output_first_only_dirty_case` | output-first rule without dirty rule | repeated reasoning to length | Failed |
| `final_reviewer_latest_exact_dirty_block` | exact latest-message instruction | repeated reasoning to length | Failed |
| `final_reviewer_latest_precise_dirty_block` | precise latest-message decision rule | wrote code-only BLOCK with correct no-diff repair reason and exact repair command | Passed/latest-message only |
| `final_reviewer_exact_latest_persona_footer` | exact latest-message wording moved into persona footer with hardcoded path | wrote code-only BLOCK with correct repair command | Passed but task-specific/rejected |
| `final_reviewer_exact_latest_persona_top` | exact latest-message wording near top of persona | repeated reasoning to length | Failed |
| `final_reviewer_decision_shortcut_before_work` | generic decision shortcut before work method | wrote BLOCK but emitted prose before bash block | Failed |
| `final_reviewer_decision_shortcut_after_diff_hygiene` | generic decision shortcut after diff hygiene | wrote BLOCK but emitted prose before bash block | Failed |
| `final_reviewer_generic_footer_contract` | generic no-diff repair contract footer | repeated reasoning to length | Failed |
| `final_reviewer_generic_footer_parse_x` | generic parse `git diff --exit-code -- X` footer | wrote code-only BLOCK with correct path and repair command when task prose also said the file should have been repaired | Partial |
| `final_reviewer_generic_footer_substitute_path` | generic substitute-path footer | repeated reasoning to length | Failed |
| `final_reviewer_retained_footer_clean_repair` | attempted retained generic parse footer on clean diff | wrote APPROVE | Passed clean |
| `final_reviewer_retained_footer_dirty_repair` | attempted retained generic parse footer on dirty diff without task-prose hint | repeated reasoning to length | Failed/rejected |
| `final_reviewer_no_diff_contradiction_before_work` | no-diff contradiction rule before work method | repeated reasoning to length | Failed |
| `final_reviewer_no_diff_contradiction_footer` | longer no-diff contradiction footer | repeated reasoning to length | Failed |
| `final_reviewer_no_diff_contradiction_short_footer` | short no-diff contradiction footer | wrote code-only BLOCK with correct contradiction and repair command | Passed dirty only |
| `final_reviewer_no_diff_contradiction_work_step` | no-diff contradiction as numbered work step | repeated reasoning to length | Failed |
| `final_reviewer_conditional_nodiff_clean` | attempted retained conditional no-diff rule on clean diff | wrote APPROVE | Passed clean |
| `final_reviewer_conditional_nodiff_dirty` | attempted retained conditional no-diff rule on dirty diff | repeated reasoning to length | Failed/rejected |
| `final_reviewer_dirty_variables_footer` | derive `dirty_no_diff_paths` variable from command/diff intersection | repeated reasoning to length | Failed |
| `final_reviewer_first_match_before_work` | first-match condition list before work method | wrote code-only BLOCK, but hallucinated producer-evidence requirement for non-generated file and ignored the no-diff contradiction | Failed/rejected |
| `final_reviewer_first_match_footer` | first-match condition list as footer | incorrectly APPROVED because changed files were in `validated_surfaces` | Failed |
| `final_reviewer_precedence_table_after_diff` | ordered decision table before validated-surface rule | repeated reasoning to length | Failed |
| `final_reviewer_precedence_table_before_work` | ordered decision table before work method | wrote BLOCK but emitted prose before the bash block and hallucinated producer-evidence requirement | Failed |

Passing latest-message control:

```text
Latest decision rule: successful `git diff --exit-code -- internal/server/auth/debug.go` means internal/server/auth/debug.go must have no final diff. Because visible final diff still contains internal/server/auth/debug.go, write BLOCK now. Required repair: Remove the remaining final diff for internal/server/auth/debug.go and rerun `git diff --exit-code -- internal/server/auth/debug.go`. Do not mention producer evidence. The assistant message must start with ```bash.
```

Learning:

- This is a hard position-sensitive failure. The final reviewer can understand
  the block condition, but persona-level rules often trigger hidden reasoning
  loops or visible prose before the required bash block.
- Top-level output-first wording did not fix the prose-before-code regression.
- Exact BLOCK templates increased looping risk.
- The exact latest-message wording worked as a persona footer only when it
  hardcoded the concrete path, which is not retainable as a persona prompt.
- The best generic persona footer either required task-prose hinting or passed
  only the dirty case and regressed/failed the clean+dirty verification pair.
- Ordered decision tables and first-match framing did not solve the issue. They
  either looped, emitted prose before the bash block, hallucinated producer
  evidence, or even approved because the dirty file was listed in
  `validated_surfaces`.
- The only clean, generic-enough replay remains a precise latest-message
  control footer. Do not retain a `final_reviewer.yaml` change yet; the
  persona-level variants are not reliable enough.
- The durable next research path is a general latest-message decision footer
  mechanism for final reviewer high-risk contradiction cases, not another
  broad semantic paragraph inside the persona body.

## Experiment 105: Final Reviewer Generic Latest Footer Transfer

Question: can the dirty repaired-path BLOCK control be made transferable as a
generic latest-message footer, preserving clean approval and blocking dirty
paths across different concrete files?

Payload batches:

- `.pragma/prompt-ab/final-reviewer-generic-latest-footer-transfer-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-variable-latest-footer-transfer-20260603T000000Z`

Setup:

- clean auth case:
  - validation status had successful
    `git diff --exit-code -- internal/server/auth/debug.go`,
  - final diff changed only `cmd/pragma/jobs.go`,
- dirty auth case:
  - validation status had successful
    `git diff --exit-code -- internal/server/auth/debug.go`,
  - final diff still contained `internal/server/auth/debug.go`,
- dirty cache case:
  - validation status had successful
    `git diff --exit-code -- internal/cache/debug_trace.go`,
  - final diff still contained `internal/cache/debug_trace.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_clean_auth_current` | current final reviewer | wrote APPROVE | Passed |
| `final_reviewer_clean_auth_generic_latest_footer` | fully generic latest footer | wrote APPROVE | Passed |
| `final_reviewer_dirty_auth_current` | current final reviewer | repeated reasoning to length | Failed |
| `final_reviewer_dirty_auth_generic_latest_footer` | fully generic latest footer | repeated reasoning to length | Failed |
| `final_reviewer_dirty_cache_current` | current final reviewer | repeated reasoning to length | Failed |
| `final_reviewer_dirty_cache_generic_latest_footer` | fully generic latest footer | repeated reasoning to length | Failed |
| `final_reviewer_clean_auth_variable_latest_footer` | latest footer with extracted `No-diff validation path` variable | wrote APPROVE | Passed |
| `final_reviewer_dirty_auth_variable_latest_footer` | latest footer with extracted auth path variable | repeated reasoning to length | Failed |
| `final_reviewer_dirty_cache_variable_latest_footer` | latest footer with extracted cache path variable | wrote BLOCK, but hallucinated producer-evidence requirement for a non-generated file | Failed |

Learning:

- A fully generic latest-message footer is not enough. It preserves clean
  approval but does not reliably stop dirty repaired-path reasoning loops.
- Filling the extracted path as a variable is still not enough. It preserved
  clean approval, but failed one dirty path by looping and another by blocking
  for the wrong producer-evidence reason.
- The earlier one-off latest-message success appears to depend on very precise
  wording plus concrete path binding. A future mechanism should test a stricter
  generated footer that writes the exact BLOCK artifact body from extracted
  variables, rather than asking the final reviewer to derive the verdict.

## Experiment 106: Final Reviewer Exact Generated Verdict Footer

Question: can a generated footer that contains the exact verdict shell block
avoid dirty repaired-path reasoning loops, and does placement as a separate
final user message matter?

Payload batches:

- `.pragma/prompt-ab/final-reviewer-exact-generated-footer-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-output-only-generated-footer-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-isolated-exact-final-user-20260603T000000Z`

Setup:

- dirty auth and dirty cache cases both had:
  - clean validation status,
  - successful `git diff --exit-code -- <path>` with `EXIT_STATUS: 0`,
  - final diff still containing `<path>`,
- clean auth case had:
  - successful `git diff --exit-code -- internal/server/auth/debug.go`,
  - final diff changing only `cmd/pragma/jobs.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_clean_auth_no_footer` | clean control with no exact footer | wrote APPROVE | Passed |
| `final_reviewer_dirty_auth_exact_footer` | exact shell script appended to original task message | wrote code-only BLOCK with correct auth path and repair command | Passed |
| `final_reviewer_dirty_cache_exact_footer` | exact shell script appended to original task message | repeated reasoning to length | Failed |
| `final_reviewer_dirty_auth_output_only_footer` | latest footer included already fenced bash block, appended to original task message | repeated reasoning to length | Failed |
| `final_reviewer_dirty_cache_output_only_footer` | already-fenced footer appended to original task message | incorrectly APPROVED because changed files were in `validated_surfaces` | Failed |
| `final_reviewer_dirty_auth_isolated_exact` | separate final user message containing only exact verdict instruction after diff | wrote code-only BLOCK with correct auth path and repair command | Passed |
| `final_reviewer_dirty_cache_isolated_exact` | separate final user message containing only exact verdict instruction after diff | wrote code-only BLOCK with correct cache path and repair command | Passed |

Learning:

- Exact generated verdict content is not sufficient when appended to the
  original final-review task message. It can still be overridden by the
  existing persona/diff reasoning context.
- A separate final user message containing only the exact verdict instruction
  transferred across both dirty paths in this replay.
- This suggests the reliable mechanism is a context-boundary/placement change:
  generate an isolated latest decision message after extracting contradiction
  variables, rather than appending semantic or exact footer text to the
  original persona task.
- No `final_reviewer.yaml` change is retained. This is orchestration evidence
  for a future final-review latest-decision-message mechanism.

## Experiment 107: Final Reviewer Isolated Decision Template Minimality

Question: for the isolated latest decision message that worked in Experiment
106, which parts of the message are necessary for reliable artifact writing?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-isolated-decision-template-minimal-20260603T000000Z`

Setup:

- same dirty cache repaired-path contradiction as Experiment 106:
  - `git diff --exit-code -- internal/cache/debug_trace.go` had
    `EXIT_STATUS: 0`,
  - final diff still contained `internal/cache/debug_trace.go`,
- each variant used a separate final user message after visible diff evidence.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_isolated_full_leadin` | “The final verdict is already determined. Respond with exactly this fenced bash block…” plus full shell block | wrote exact BLOCK artifact | Passed |
| `final_reviewer_isolated_short_leadin` | “Respond exactly with this fenced bash block…” plus full shell block | wrote exact BLOCK artifact | Passed |
| `final_reviewer_isolated_artifact_only` | artifact body plus instruction to write the file and echo sentinel | constructed correct shell wrapper and wrote BLOCK | Passed |
| `final_reviewer_isolated_block_only` | bare fenced shell block as the final user message | repeated reasoning to length | Failed |
| `final_reviewer_isolated_decision_only_then_block` | one-line decision summary followed by fenced shell block | tried to `cat /tmp/pragma/final-prosecutor-verdict.md` instead of writing it | Failed |

Learning:

- The reliable isolated latest decision message needs an explicit imperative to
  write the artifact. A bare fenced block can be treated as context to analyze,
  not an instruction to execute.
- The shell wrapper does not have to be prewritten if the final user message
  provides exact artifact content plus “write this file and echo sentinel”;
  the model constructed the correct wrapper in this replay.
- Adding a decision summary before the block is harmful; it made the model
  assume the artifact already existed and ask to read it.
- The most compact currently reliable template is:

````text
Respond exactly with this fenced bash block and nothing else:
```bash
cat > /tmp/pragma/final-prosecutor-verdict.md <<'EOF'
...
EOF
echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT
```
````

## Experiment 108: Final Reviewer Generated Diff Producer Block

Question: does the isolated latest decision-message pattern also help final
reviewer with a generated-file diff missing producer evidence, and is it needed
when the base persona already has an explicit producer-evidence rule?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-isolated-generated-producer-block-20260603T000000Z`

Setup:

- validation status was clean and included `rpc/auth.pb.go` in
  `validated_surfaces`,
- `Producer evidence` was `none`,
- visible diff changed generated file `rpc/auth.pb.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_generated_missing_producer` | current final reviewer | wrote BLOCK for changed generated file with no producer evidence | Passed |
| `final_reviewer_isolated_generated_artifact_only` | isolated artifact body plus write/echo instruction | wrote exact BLOCK artifact | Passed |
| `final_reviewer_isolated_generated_block_only` | isolated bare fenced shell block | wrote exact BLOCK artifact | Passed |
| `final_reviewer_isolated_generated_short_leadin` | isolated “Respond exactly…” plus shell block | wrote exact BLOCK artifact | Passed |

Learning:

- Missing producer evidence for generated/derived diffs is already handled by
  current final reviewer. The isolated latest decision message is not needed
  for this ordinary rule-match case.
- Bare fenced shell block worked here, unlike the dirty repaired-path
  contradiction case. The failure mode is not “bare block always fails”; it is
  tied to contradiction cases where existing persona rules pull the model back
  into analysis.
- This strengthens the boundary: isolated decision messages are most useful for
  high-conflict cases where validation-status and diff evidence appear to point
  in opposite directions, not for straightforward final-review rule matches.

## Experiment 109: Final Reviewer Command-Status Contradiction

Question: if validation summary fields incorrectly say `none` but
`Commands run` has a nonzero exit status and visible failure output, does final
reviewer need an isolated decision message?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-command-status-contradiction-20260603T000000Z`

Setup:

- validation status contained:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `EXIT_STATUS[1]: 1`,
  - visible `--- FAIL: TestMaxRetriesDefault`,
- summary fields incorrectly said:
  - `failed_tests: none`,
  - `failed_packages: none`,
  - `unexpected_errors: none`,
  - `missing_files_or_surfaces: none`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `final_reviewer_current_command_status_contradiction` | current final reviewer | wrote BLOCK from nonzero command status and visible test failure | Passed |
| `final_reviewer_isolated_command_status_artifact_only` | isolated artifact body plus write/echo instruction | wrote BLOCK artifact | Passed |
| `final_reviewer_isolated_command_status_short_leadin` | isolated “Respond exactly…” plus shell block | wrote BLOCK artifact | Passed |
| `final_reviewer_isolated_command_status_block_only` | isolated bare fenced shell block | tried to `cat /tmp/pragma/final-prosecutor-verdict.md` instead of writing it | Failed |

Learning:

- Current final reviewer already handles command-status contradictions through
  the command-status authority rule.
- Isolated decision messages are not needed for this class.
- The isolated template boundary repeats: artifact/write instruction and
  “Respond exactly…” work, while bare block-only can be misread as an existing
  artifact to inspect.

## Experiment 110: Checklist Writer No-Diff Repair Reopen

Question: after final reviewer blocks with a no-diff contradiction for a repair
item that the prior checklist marked `completed`, does checklist writer reopen
that concrete repair as pending instead of preserving stale completion?

Payload batch:

- `.pragma/prompt-ab/checklist-repair-no-diff-contradiction-20260603T000000Z`

Setup:

- final verdict said
  `git diff --exit-code -- internal/server/auth/debug.go` had
  `EXIT_STATUS: 0`, but the final diff still contained
  `internal/server/auth/debug.go`,
- required repair was to remove the remaining final diff for
  `internal/server/auth/debug.go` and rerun the same no-diff command,
- patch plan still contained the original completed source route for
  `cmd/pragma/jobs.go`,
- prior checklist had:
  - completed original item for `cmd/pragma/jobs.go`,
  - completed repair item for `internal/server/auth/debug.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `checklist_current_no_diff_repair` | current checklist writer | preserved completed `cmd/pragma/jobs.go` item and rewrote the `internal/server/auth/debug.go` repair item as `pending` with exact no-diff validation command | Passed |
| `checklist_repair_completion_override_rule` | explicit rule that final verdict overrides prior completed status | equivalent bounded pending repair item | Passed/redundant |
| `checklist_no_diff_repair_rule` | explicit no-diff contradiction repair rule | equivalent bounded pending repair item | Passed/redundant |
| `checklist_combined_no_diff_repair_rules` | both explicit rules | equivalent bounded pending repair item | Passed/redundant |

Learning:

- Current checklist writer already treats final verdict repair authority as
  stronger than stale prior completion for this no-diff contradiction shape.
- Extra completion-override and no-diff repair wording is not retained because
  it did not change behavior.
- The downstream checklist step is not the weak link for the no-diff
  contradiction chain. The hard part remains producing the final reviewer BLOCK
  reliably in high-conflict dirty repaired-path cases.

## Experiment 111: Final Reviewer Isolated Variable Decision

Question: can the successful isolated final-review decision message be made
more reusable than a fully prewritten verdict while still avoiding dirty
repaired-path reasoning loops?

Payload batches:

- `.pragma/prompt-ab/final-reviewer-isolated-variable-decision-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-isolated-variable-decision-strict-20260603T000000Z`

Setup:

- reused the dirty auth and dirty cache repaired-path contradiction payloads
  from Experiment 106,
- both cases had a successful
  `git diff --exit-code -- <path>` command in validation status,
- visible final diff still contained the same `<path>`,
- only the isolated final user decision message was changed.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| dirty auth | condition-only extracted state | repeated reasoning to `finish_reason: length`, no content | Failed |
| dirty cache | condition-only extracted state | wrote BLOCK, but drifted to producer/generated-file reasoning instead of no-diff contradiction | Failed |
| dirty auth | variable shell-template instruction | wrote BLOCK for no-diff contradiction, but repair sentence said "rerun the validation command" instead of copying command verbatim | Partial |
| dirty cache | variable shell-template instruction | same partial behavior as auth | Partial |
| dirty auth | YAML decision packet | wrote BLOCK, but invented a validation-tool repair for untracked-file detection | Failed |
| dirty cache | YAML decision packet | wrote BLOCK, but used vague contradiction wording and added generated-file framing | Partial/failed |
| dirty auth | slot template with verbatim variables | wrote code-only BLOCK with correct path and exact validation command | Passed |
| dirty cache | slot template with verbatim variables | wrote code-only BLOCK with correct path and exact validation command | Passed |
| dirty auth | exact field lines plus shell-wrapper instruction | wrote code-only BLOCK with correct path and exact validation command | Passed |
| dirty cache | exact field lines plus shell-wrapper instruction | wrote code-only BLOCK with correct path and exact validation command | Passed |

Accepted latest-message primitive:

```text
The final verdict is already determined. Write it from these exact fields.

output_file = /tmp/pragma/final-prosecutor-verdict.md
findings_line_1 = `<validation_command>` had EXIT_STATUS: 0, but the final diff still contains <path>.
findings_line_2 = This is a no-diff repair contradiction for <path>.
required_repair = Remove the remaining final diff for <path> and rerun `<validation_command>`.
decision = BLOCK
event_marker = COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT

Response constraints:
- one fenced bash block only
- write output_file with exactly the two findings lines, required_repair, and decision above
- echo event_marker after writing the file
- do not add or replace any reason
```

Learning:

- Reusable does not mean semantic. The passing control still makes the final
  decision concrete by giving exact output fields and asking only for the shell
  wrapper.
- Condition-only rules are too weak in this high-conflict context. They can
  loop or select a different blocking rationale.
- YAML-like decision packets are also too weak here; the model may reinterpret
  the packet as evidence to analyze and invent repairs outside the intended
  no-diff contradiction.
- The strongest transferable template so far is exact field lines plus a
  wrapper instruction. It is less brittle than a fully prewritten bash block
  because the orchestrator can fill fields, but it still avoids asking final
  reviewer to reason under conflicting validation/diff evidence.
- No `final_reviewer.yaml` change is retained. This is a latest-message
  orchestration primitive for dirty repaired-path contradictions.

## Experiment 112: Item Worker Failed Validation Field Lines

Question: can the failed-validation stop control for item worker be made less
brittle than a fully prewritten report shell script, and does the final-reviewer
field-lines primitive transfer to implementer reports?

Payload batches:

- `.pragma/prompt-ab/item-worker-failed-validation-field-lines-20260603T000000Z`
- `.pragma/prompt-ab/item-worker-failed-validation-field-lines-transfer-20260603T000000Z`

Setup:

- reused the known Experiment 51 item-worker trajectory where static
  failed-validation rules failed,
- case 1:
  - current item: `item-005 Set max retries default`,
  - changed file: `cmd/pragma/jobs.go`,
  - failed command: `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
  - failure signal: `TestMaxRetriesDefault expected default 3, got 0`,
- case 2:
  - current item: `item-ttl-default Set cache TTL default`,
  - changed file: `internal/cache/options.go`,
  - failed command: `go test ./internal/cache -run TestDefaultTTL`,
  - failure signal: `TestDefaultTTL expected default TTL 60, got 0`,
- only the latest user message after visible failed validation was changed.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| max retries | full prewritten report script | wrote blocker implementer report and sentinel | Passed |
| cache TTL | full prewritten report script | wrote blocker implementer report and sentinel | Passed |
| max retries | exact field lines plus shell-wrapper instruction | wrote blocker implementer report with exact command, changed file, failure signal, and sentinel | Passed |
| cache TTL | exact field lines plus shell-wrapper instruction | wrote blocker implementer report with exact command, changed file, failure signal, and sentinel | Passed |
| max retries | YAML report packet | wrote blocker implementer report and sentinel | Passed |
| cache TTL | YAML report packet | wrote blocker implementer report and sentinel | Passed |
| max retries | slot template with variables | inspected `cmd/pragma/jobs_test.go` | Failed |
| cache TTL | slot template with variables | inspected `internal/cache/options.go` | Failed |
| max retries | condition-only failed-validation state | grepped `cmd/pragma/jobs.go` | Failed |
| cache TTL | condition-only failed-validation state | read `internal/cache/options.go` | Failed |

Accepted latest-message primitive:

```text
The implementer report is already determined. Write it from these exact fields.

output_file = /tmp/pragma/implementer-report.md
current_item = <id and title>
changed = <changed file or "No files changed">
acceptance_evidence = BLOCKED - <validation_command> failed
validation = <validation_command> failed with EXIT_STATUS: <status>
remaining_risk = validation failure remains
blocker = <first concrete failure signal>
event_marker = COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT

Response constraints:
- one fenced bash block only
- write output_file with exactly the fields above in /tmp/pragma/implementer-report.md format
- echo event_marker after writing the file
- do not inspect files, rerun validation, or add/replace any reason
```

Learning:

- The final-reviewer field-lines primitive transfers to item-worker failed
  validation reports. It avoids a fully prewritten bash script while still
  preventing the model from continuing repair.
- YAML report packets passed for item-worker in both tested cases. This differs
  from final reviewer, where YAML packets drifted in high-conflict
  validation/diff contradictions. YAML appears acceptable for lower-conflict
  report materialization, but field lines are the safer cross-persona primitive.
- Condition-only state remains too weak: the model treats visible failure as a
  reason to inspect source/tests.
- Slot templates with placeholders are also weak here. Despite explicit
  "copy verbatim" language, the model chose more investigation rather than
  materializing the report.
- No `item_worker.yaml` change is retained. This is an orchestration
  latest-message control for the point where the runtime decides to stop the
  item-worker phase after failed focused validation.

## Experiment 113: Item Worker Repeated Search Field Lines

Question: does current item-worker stuck-state control reliably stop repeated
searches across item shapes, and do field-lines/YAML materialization packets
transfer to repeated-search blockers?

Payload batch:

- `.pragma/prompt-ab/item-worker-repeated-search-field-lines-20260603T000000Z`

Setup:

- case 1 reused the generated/proto blocker shape:
  - current item: `item-002 Generate Kubernetes auth protobuf bindings`,
  - allowed file: `rpc/flipt/auth/auth.proto`,
  - forbidden generated files: `rpc/flipt/auth/*.pb.go`,
  - search state: `repeated_no_new_information = true`, `repeat_count = 3`,
  - repeated commands found no producer command and no service symbol,
- case 2 used a config-symbol transfer shape:
  - current item: `item-auth-config-symbol Wire Kubernetes auth config symbol`,
  - allowed file: `internal/config/authentication.go`,
  - forbidden runtime auth scope: `internal/server/auth/**`,
  - search state: `repeated_no_new_information = true`, `repeat_count = 4`,
  - repeated commands found no `KubernetesTokenValidator` source symbol,
- variants compared current state-only behavior, full prewritten script,
  exact field lines, YAML packet, and condition-only latest state.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| proto | current state-only | wrote blocker implementer report and sentinel | Passed |
| config | current state-only | read `/tmp/pragma/current-item.json` and `internal/config/authentication.go` instead of writing report | Failed |
| proto | full prewritten report script | wrote exact blocker implementer report and sentinel | Passed |
| config | full prewritten report script | wrote exact blocker implementer report and sentinel | Passed |
| proto | exact field lines plus shell-wrapper instruction | wrote exact blocker implementer report and sentinel | Passed |
| config | exact field lines plus shell-wrapper instruction | wrote exact blocker implementer report and sentinel | Passed |
| proto | YAML report packet | wrote exact blocker implementer report and sentinel | Passed |
| config | YAML report packet | wrote exact blocker implementer report and sentinel | Passed |
| proto | condition-only latest state | wrote blocker report and sentinel, but with less exact generated/protobuf rationale | Partial |
| config | condition-only latest state | wrote blocker report and sentinel, but changed risk/validation wording | Partial |

Learning:

- Current item-worker stuck-state wording is not fully transferable. It handled
  the generated/proto shape, likely because generated-file policy also points to
  a blocker, but it failed the config-symbol shape by inspecting the allowed
  file again.
- Field-lines materialization passed both repeated-search blocker shapes and
  preserved exact report fields.
- YAML report packets also passed both repeated-search shapes, matching
  Experiment 112's item-worker behavior. This remains lower-risk for
  item-worker report materialization than for final-reviewer high-conflict
  validation/diff contradictions.
- Condition-only latest state can stop search in this repeated-search setting,
  but it allows the model to rewrite report rationale and validation/risk
  wording. Treat it as weaker than field lines or YAML packet.
- Do not claim current static state variables alone are generally enough for
  repeated-search stop. For robust orchestration stop points, use a latest
  materialization packet, preferably exact field lines.
- No `item_worker.yaml` change is retained from this experiment.

## Experiment 114: Item Reviewer Approval Field Lines

Question: for item-reviewer approval states where extra semantic rules can
regress strict verdict size, do latest-message materialization packets preserve
compact three-finding APPROVE verdicts?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-approval-field-lines-20260603T000000Z`

Setup:

- source-edit approval case:
  - current item allowed only `cmd/pragma/jobs.go`,
  - implementer report `Changed` named only `cmd/pragma/jobs.go`,
  - validation named `go test -run TestMaxRetriesDefault ./cmd/pragma/` with
    `EXIT_STATUS: 0`,
  - current reviewer already passed this case, while an added semantic
    source-edit approval rule previously wrote four Findings bullets,
- out-of-scope repair approval case:
  - current item allowed only `internal/server/auth/debug.go`,
  - implementer report changed
    `internal/server/auth/debug.go - removed out-of-scope file`,
  - validation named `git diff --exit-code -- internal/server/auth/debug.go`
    with `EXIT_STATUS: 0`,
  - current reviewer already passed this case, while an added remove/revert
    approval rule previously wrote four Findings bullets.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| source edit | field lines plus shell-wrapper instruction | wrote APPROVE with exact three findings and sentinel | Passed |
| repair | field lines plus shell-wrapper instruction | wrote APPROVE with exact three findings and sentinel | Passed |
| source edit | YAML verdict packet | wrote APPROVE with exact three findings and sentinel | Passed |
| repair | YAML verdict packet | wrote APPROVE with exact three findings and sentinel | Passed |
| source edit | slot template with variables | wrote APPROVE with exact three findings and sentinel | Passed |
| repair | slot template with variables | wrote APPROVE with exact three findings and sentinel | Passed |
| source edit | condition-only approval state | wrote compact APPROVE, but rewrote findings more generically | Passed/weaker |
| repair | condition-only approval state | wrote compact APPROVE, but rewrote findings more generically | Passed/weaker |

Learning:

- Item-reviewer approval materialization is lower-conflict than item-worker
  stop states and final-reviewer dirty repaired-path contradictions. In this
  setting, field lines, YAML packets, slot templates, and even condition-only
  state all produced compact APPROVE verdicts.
- Field lines and YAML packets preserve exact intended findings best.
  Condition-only state is acceptable for the decision/shape but lets the model
  rewrite evidence phrasing.
- Slot templates are not universally weak. They failed item-worker stop states
  where the model still wanted to investigate, but passed here because the
  underlying approval decision was already aligned with the current prompt.
- Do not add source-edit or remove/revert-specific approval prose to
  `item_reviewer.yaml`; prior replay showed those semantic additions can
  regress strict verdict size. Generic current rules plus optional latest
  materialization are better.
- No `item_reviewer.yaml` change is retained.

## Experiment 115: Item Worker Validation Command Packets

Question: can latest-message command packets produce the exact
status-preserving validation shell shape, or does command construction still
require a fully pasted wrapper?

Payload batch:

- `.pragma/prompt-ab/item-worker-validation-command-packets-20260603T000000Z`

Setup:

- validation-only case:
  - current item asked only to run
    `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
  - `allowed_files` was empty and `forbidden_files` was `["**/*"]`,
  - expected script needed `set -o pipefail`, log capture through `tee`,
    `${PIPESTATUS[0]}`, `EXIT_STATUS`, and final `exit "$status"`,
- post-source-edit case:
  - current item allowed only `cmd/pragma/jobs.go`,
  - item worker had just changed `defaultMaxRetries = 3` to
    `defaultMaxRetries = 5`,
  - validation command was `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - expected script needed the same status-preserving wrapper.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| validation-only | current baseline | ran exact command with `$?`, no `tee`, no `${PIPESTATUS[0]}`, no final `exit` | Failed/known |
| post-source-edit | current baseline | ran exact command with `$?`, no `tee`, no `${PIPESTATUS[0]}`, no final `exit` | Failed/known |
| validation-only | full exact wrapper | emitted full status-preserving wrapper | Passed |
| post-source-edit | full exact wrapper | emitted full status-preserving wrapper | Passed |
| validation-only | field-lines command packet | emitted status-preserving one-line script with `tee`, `${PIPESTATUS[0]}`, `EXIT_STATUS`, and final `exit`, but used log path directly instead of the `log` variable | Passed/shape drift |
| post-source-edit | field-lines command packet | emitted full status-preserving wrapper | Passed |
| validation-only | YAML command packet | emitted full status-preserving wrapper | Passed |
| post-source-edit | YAML command packet | emitted full status-preserving wrapper | Passed |
| validation-only | condition-only command state | used `$?`, no `tee`, no `${PIPESTATUS[0]}`, and no quoted final `exit "$status"` | Failed |
| post-source-edit | condition-only command state | emitted full status-preserving wrapper | Passed |

Learning:

- Exact shell wrapper remains the strongest command-shape control.
- YAML command packets transferred cleanly across both item-worker validation
  states. For command construction, YAML fields were less interpretive than in
  final-reviewer high-conflict decision cases.
- Field-lines command packets also transferred semantically, but one response
  compressed the script and skipped the named `log` variable. This is acceptable
  for status preservation but weaker if byte-level script shape matters.
- Condition-only command state is unreliable: it passed after source edit but
  failed validation-only by falling back to `$?` and omitting pipe/log status
  preservation.
- For command-shape forcing, prefer a full exact wrapper when exact script
  layout matters, and a YAML command packet when orchestration wants a
  generated/latest-message representation. Do not rely on condition-only state.
- No `item_worker.yaml` change is retained because the existing retained static
  wrapper rules already encode the passing shape.

## Experiment 116: Patch Planner Surface-Only Source Uncertainty

Question: when surface map names a plausible source file but evidence map says
the source edit path was not evidenced, does patch planner keep the source
change unresolved, or does it create a route from surface-map/task intent?

Payload batches:

- `.pragma/prompt-ab/patch-planner-surface-only-source-uncertainty-20260603T000000Z`
- `.pragma/prompt-ab/patch-planner-source-change-unresolved-route-20260603T000000Z`

Setup:

- surface map said the task intent was:
  `Change the CLI default for --max-retries from 3 to 5`,
- surface map named `cmd/pragma/jobs.go`,
- evidence map said:
  - `Source-of-truth files: none`,
  - `Evidence source paths: none`,
  - `cmd/pragma/jobs.go` was named by surface map but no source content or
    exact current value was inspected,
  - consumer path was `cmd/pragma/jobs_test.go`,
  - validation command was `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `Not evidenced` contained source edit path `cmd/pragma/jobs.go` with proof
    required.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| first batch | current patch planner | avoided source route, but created a validation-only route for validating current default before source edit; source change stayed unresolved | Failed/partial |
| first batch | source requires evidence rule | still created validation-only/pre-validation route; source change stayed unresolved | Failed/partial |
| first batch | validation-only guard rule | created a `source path: none` route whose behavior was the source change itself | Failed |
| first batch | source-evidence plus validation guard | still created validation-only/pre-validation route; source change stayed unresolved | Failed/partial |
| second batch | source-change unresolved rule | still created validation-only/pre-validation route | Failed |
| second batch | source-change unresolved rule plus empty-route shape | still created validation-only/pre-validation route | Failed |
| second batch | latest exact field plan | wrote `Patch route:\n- none` and put source change under `Unresolved until evidence` | Passed/latest-message only |

Accepted latest-message primitive:

```text
The patch plan is already determined for this evidence state. Write it from
these exact fields; do not derive another route.

output_file = /tmp/pragma/patch-plan.md
goal = <goal>
patch_route = - none
unresolved_behavior = <source-change behavior>
unresolved_proof_required = <exact source proof required>
event_marker = COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT

Response constraints:
- one fenced bash block only
- write output_file with Goal, Patch route, and Unresolved until evidence sections
- Patch route must contain exactly `- none`
- do not add a validation-only route
- do not add a source path: none route for the change behavior
- echo event_marker after writing the file
```

Learning:

- Current patch planner has a source-change uncertainty gap. It does not
  promote the surface-map source path directly, which is good, but it still
  creates a validation-only/pre-validation route when the actual requested
  source change lacks source evidence.
- Static semantic guards were too weak. Even an explicit "do not create any
  patch-route item" rule plus an empty-route shape did not stop route creation.
- The validation-only route rule has a strong pull whenever a consumer path and
  validation command are evidenced. For source-change tasks with missing source
  evidence, orchestration needs a latest exact field plan or a stronger
  generated state boundary.
- Do not retain a static `patch_planner.yaml` change from these failed static
  variants. Mark this as an unresolved persona-body gap requiring latest-message
  materialization for now.

## Experiment 117: Item Worker Source Preservation Transfer

Question: does the retained source-preservation rule transfer beyond the
original proto fixture, and can prompt variants prevent full-file rewrites and
invented adjacent comments during bounded source edits?

Payload batches:

- `.pragma/prompt-ab/item-worker-source-preservation-transfer-20260603T000000Z`
- `.pragma/prompt-ab/item-worker-source-preservation-struct-transfer-20260603T000000Z`
- `.pragma/prompt-ab/item-worker-source-preservation-exact-insertion-20260603T000000Z`

Setup:

- simple replacement case:
  - visible Go source file had imports, comments, unrelated constants,
    `RetryPolicy`, `NewRetryPolicy`, and helper function,
  - exact source change was to replace
    `const DefaultRetryCount = 3` with `const DefaultRetryCount = 5`,
  - expected behavior was one minimal edit, no full-file rewrite, no post-edit
    read, no validation, and no report,
- struct insertion case:
  - visible Go source file had comments and unrelated fields around
    `type RetryPolicy struct`,
  - exact source change was to add `Backoff time.Duration` after
    `Delay time.Duration`,
  - expected behavior was one inserted field line only, preserving unrelated
    context and not inventing comments.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| simple replacement | current prompt with retained source-preservation rule | one-line `sed` replacing only the constant | Passed |
| simple replacement | prompt without source-preservation rule | same one-line `sed` | Passed |
| simple replacement | latest state footer | same one-line `sed` | Passed |
| simple replacement | latest exact diff packet | same one-line `sed` | Passed |
| simple replacement | latest full-file temptation | same one-line `sed` | Passed |
| struct insertion | current prompt with retained source-preservation rule | minimal `sed`, but inserted an invented Backoff comment plus field | Partial |
| struct insertion | prompt without source-preservation rule | same minimal edit with invented comment | Partial |
| struct insertion | latest state footer | same minimal edit with invented comment | Partial |
| struct insertion | latest insert packet | same minimal edit with invented comment | Partial |
| struct insertion | latest full-file temptation | same minimal edit with invented comment | Partial |
| exact insertion | static exact-insertion rule | still inserted an invented Backoff comment plus field | Failed |
| exact insertion | latest exact insertion packet with one listed line | inserted only `Backoff time.Duration` | Passed |
| exact insertion | negative-only latest message forbidding comments/extra lines | inserted only `Backoff time.Duration` | Passed |
| exact insertion | exact command field | emitted exact ruby one-line edit command | Passed |

Accepted latest-message primitive:

```text
The source edit is already determined. Apply only this exact insertion.

file = <path>
anchor_line = <exact existing line>
insert_after_anchor_exact_lines:
- <exact inserted line>
number_of_inserted_lines = <n>

Response constraints:
- one fenced bash block only
- edit only file
- insert exactly the listed line(s) immediately after anchor_line
- do not insert comments
- do not insert blank lines
- do not rewrite the whole file
- do not read the file after editing
- do not run validation or write /tmp/pragma/implementer-report.md
```

Learning:

- The retained source-preservation rule transfers for preventing full-file
  rewrites and post-edit reads in these Go fixtures.
- Simple line replacements are easy; even prompts without the preservation rule
  produced the same minimal `sed`.
- Struct insertions expose a different implementation-discipline failure:
  the model preserves context but invents a nearby explanatory comment not
  requested by `exact_source_changes`.
- Static "add exactly one line" wording did not prevent the invented comment.
  The reliable controls were latest-message exact insertion packet,
  negative-only latest comment ban, or exact command field.
- Do not add the failed static exact-insertion rule to `item_worker.yaml`.
  Treat exact insertion without invented adjacent context as a latest-message
  materialization/control-packet need for now.

## Experiment 118: Final Reviewer Decision-Card Transfer

Question: for dirty repaired-path final review, can a latest-message decision
card be less prescribed than exact `findings_line_*` fields and still avoid
reasoning loops, producer/generated drift, and softened repair wording?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-decision-card-transfer-20260603T000000Z`

Setup:

- reused the dirty auth and dirty cache repaired-path contradiction payloads
  from Experiment 111,
- each case had clean validation status plus a successful
  `git diff --exit-code -- <path>` command,
- visible final diff still contained the same repaired path,
- only the final latest user message was changed.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| dirty auth | decision card with fact fields | wrote BLOCK and exact command/path, but changed findings to generic "command/status contradiction" and did not preserve the no-diff contradiction phrase | Partial |
| dirty cache | decision card with fact fields | same shape drift; also wrote "repaired_path" and "no_diff_command" labels into the repair sentence | Partial |
| dirty auth | decision card without negative examples | repeated reasoning to `finish_reason: length`, no verdict | Failed |
| dirty cache | decision card without negative examples | repeated reasoning to `finish_reason: length`, no verdict | Failed |
| dirty auth | minimal variables with good/bad examples | repeated reasoning to `finish_reason: length`; drifted into producer/generated reasoning | Failed |
| dirty cache | minimal variables with good/bad examples | repeated reasoning to `finish_reason: length`; drifted into producer/generated reasoning | Failed |
| dirty auth | placeholder verdict pattern with variables | wrote code-only BLOCK with exact no-diff contradiction wording, exact path, exact command, exact repair, and sentinel | Passed |
| dirty cache | placeholder verdict pattern with variables | wrote code-only BLOCK with exact no-diff contradiction wording, exact path, exact command, exact repair, and sentinel | Passed |

Accepted latest-message primitive:

```text
The final verdict is determined by this no-diff repair contradiction.

repaired_path = <path>
no_diff_command = <validation command>

Write the verdict by filling this pattern:
Findings:
- `<no_diff_command>` had EXIT_STATUS: 0, but the final diff still contains <repaired_path>.
- This is a no-diff repair contradiction for <repaired_path>.

Required repair:
Remove the remaining final diff for <repaired_path> and rerun `<no_diff_command>`.

Decision:
BLOCK

Response constraints:
- one fenced bash block only
- write /tmp/pragma/final-prosecutor-verdict.md
- replace placeholders with the variables above
- echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after writing the file
```

Learning:

- The high-conflict dirty repaired-path state still cannot be handled by a
  semantic decision card. When the model is asked to decide from fields, it may
  loop or reinterpret the reason.
- Exact field lines are not the only passing form. A placeholder verdict
  pattern with two variables transfers across both dirty paths and is less
  brittle than precomputing every output line.
- The passing pattern is still materialization, not reasoning. It gives the
  verdict skeleton and asks the model to substitute variables; it does not ask
  the final reviewer to choose a rationale.
- Good/bad examples are actively harmful here when used as a loose state card:
  they did not stop the high-conflict reasoning loop and reopened
  producer/generated drift.
- No `final_reviewer.yaml` change is retained. This remains a latest-message
  orchestration primitive for an already-detected no-diff repair
  contradiction.

## Experiment 119: Patch Planner Unresolved-Source Materialization

Question: when a requested source change has no evidenced source path/content,
can the successful exact patch-plan field packet be relaxed into reusable
materialization patterns without reopening the validation-only route failure?

Payload batch:

- `.pragma/prompt-ab/patch-planner-unresolved-source-materialization-20260603T000000Z`

Setup:

- reused the unresolved source-change payload from Experiment 116,
- surface map named `cmd/pragma/jobs.go`, but evidence map had no evidenced
  source path/content,
- consumer/defaulting path and validation command were present,
- the desired patch plan was:
  - `Patch route: - none`,
  - the source change under `Unresolved until evidence`,
  - no validation-only, pre-validation, or `source path: none` route.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| placeholder section pattern | wrote code-only patch plan with `Patch route: - none`, exact unresolved behavior/proof, no invented route, and sentinel | Passed |
| empty route template only | wrote the same correct route-none plan and unresolved proof | Passed |
| minimal state with good/bad examples | kept `Patch route: - none`, but rewrote the goal/proof instead of preserving the exact unresolved proof text | Partial |
| route decision card | preserved unresolved proof, but wrote `Patch route:\nnone` instead of the required `- none` list shape | Partial |

Accepted latest-message primitive:

```text
The patch plan is determined by unresolved source evidence.

goal = <goal>
unresolved_behavior = <source-change behavior>
unresolved_proof_required = <exact source proof required>

Write /tmp/pragma/patch-plan.md by filling this pattern:
Goal:
<goal>

Patch route:
- none

Unresolved until evidence:
- behavior: <unresolved_behavior>
  proof required: <unresolved_proof_required>

Response constraints:
- one fenced bash block only
- replace placeholders with the variables above
- do not add any other Patch route entry
- do not add validation-only or pre-validation routes
- echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after writing the file
```

Learning:

- Patch planner follows a concrete section template reliably in this
  unresolved-source state. It does not need every output line precomputed, but
  it does need the `Patch route: - none` shape and unresolved section skeleton.
- Route decision cards are too loose for downstream parsability; the model
  converted `patch_route: none` into a scalar-looking `Patch route:\nnone`.
- Good/bad examples preserve the high-level decision but allow evidence text
  rewriting. That is weaker than materialization when the proof string is a
  downstream contract.
- No `patch_planner.yaml` change is retained. Static planner text already
  failed in Experiment 116; the retained control is latest-message plan
  materialization for an already-classified unresolved-source state.

## Experiment 120: Item Worker Repeated-Search Materialization

Question: for item-worker repeated no-new-information stop states, can the
successful exact field/YAML blocker report packets be relaxed into reusable
section templates or decision cards while preserving exact handoff fields?

Payload batch:

- `.pragma/prompt-ab/item-worker-repeated-search-materialization-20260603T000000Z`

Setup:

- reused the two repeated-search cases from Experiment 113:
  - proto/generated producer search with no producer command or service symbol,
  - config-symbol search with no `KubernetesTokenValidator` source symbol,
- only the latest user message was changed,
- pass required:
  - write `/tmp/pragma/implementer-report.md`,
  - preserve `Current item`, `Changed`, `Acceptance evidence`, `Validation`,
    `Remaining risk`, and `Blocker` fields exactly,
  - no file inspection, repeated search, validation, or editing,
  - completion sentinel after writing the file.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| config | placeholder report pattern | wrote exact report fields and sentinel; no extra action | Passed |
| proto | placeholder report pattern | wrote exact report fields and sentinel; no extra action | Passed |
| config | report decision card | wrote exact report fields and sentinel; no extra action | Passed |
| proto | report decision card | wrote exact report fields and sentinel; no extra action | Passed |
| config | section template with no variables | wrote exact report fields and sentinel; no extra action | Passed |
| proto | section template with no variables | wrote exact report fields and sentinel; no extra action | Passed |
| config | minimal state with good/bad examples | stopped and wrote a report, but rewrote acceptance, validation, and remaining-risk fields | Partial/rejected |
| proto | minimal state with good/bad examples | stopped and wrote a report, but rewrote acceptance, validation, and remaining-risk fields | Partial/rejected |

Accepted latest-message primitives:

```text
The implementer report is determined by repeated no-new-information searches.

current_item = <item id/title>
changed = No files changed
acceptance_evidence = <exact blocker acceptance line>
validation = <exact validation line>
remaining_risk = <exact remaining risk line>
blocker = <exact blocker line>

Write /tmp/pragma/implementer-report.md by filling this pattern:
Current item:
<current_item>

Changed:
<changed>

Acceptance evidence:
<acceptance_evidence>

Validation:
<validation>

Remaining risk:
<remaining_risk>

Blocker:
<blocker>

Response constraints:
- one fenced bash block only
- replace placeholders with the variables above
- do not inspect files, rerun searches, run validation, or add another reason
- echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after writing the file
```

```text
Use this report decision card to write /tmp/pragma/implementer-report.md.

report_decision:
  status: blocked
  reason_type: repeated no-new-information searches
  current_item: <item id/title>
  changed: No files changed
  acceptance_evidence: <exact blocker acceptance line>
  validation: <exact validation line>
  remaining_risk: <exact remaining risk line>
  blocker: <exact blocker line>
  forbidden_next_actions:
    - inspect files
    - rerun searches
    - run validation
    - edit files

Write the implementer report now and echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT.
Use one fenced bash block only.
```

Learning:

- Item-worker report materialization is more tolerant than final-review dirty
  contradiction materialization. YAML packets, exact fields, placeholder
  section patterns, full section templates, and decision cards all work when
  the stop state is already supplied.
- Good/bad examples alone are not enough for exact handoff fields. They make
  the model stop, but it rewrites validation and risk into generic language.
- The core unresolved problem remains stop-state classification. Current
  item-worker static prompt can still inspect again in the config-symbol shape.
  Once orchestration supplies the stop state, several materialization formats
  are reliable.
- No `item_worker.yaml` change is retained from this experiment.

## Experiment 121: Item Worker Exact-Insertion Materialization

Question: for exact source insertions where the model previously invented an
adjacent comment, which latest-message forms prevent extra lines without
requiring a fully prewritten shell command?

Payload batch:

- `.pragma/prompt-ab/item-worker-exact-insertion-materialization-20260603T000000Z`

Setup:

- reused the `RetryPolicy` exact insertion payload from Experiment 117,
- visible source had comments around `Attempts`, `Delay`, and `Name`,
- required edit was one inserted line after `Delay time.Duration`:
  `Backoff time.Duration`,
- pass required:
  - edit only `internal/config/retry.go`,
  - insert `Backoff time.Duration`,
  - do not add a comment or blank line,
  - do not rewrite the whole file,
  - do not read, validate, or report after editing.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| insertion variables without negative constraints | inserted `Backoff time.Duration`, but also invented `// Backoff is the backoff duration between retries.` | Failed |
| source edit decision card | inserted only `Backoff time.Duration` after the exact anchor | Passed |
| unified hunk pattern | inserted only `Backoff time.Duration` after the anchor | Passed |
| minimal good/bad examples | inserted only `Backoff time.Duration` after the anchor | Passed |

Accepted latest-message primitives:

```text
Use this source edit decision card for the next action.

source_edit:
  file: <path>
  edit_type: insert_after_anchor
  anchor_line: "<exact existing line>"
  inserted_lines:
    - "<exact inserted line>"
  exact_inserted_line_count: <n>
  forbidden_extra_lines:
    - comments
    - blank lines
    - rewritten file content
  forbidden_next_actions:
    - read file after edit
    - run validation
    - write implementer report

Apply the source edit now. Use one fenced bash block only.
```

```text
Apply exactly this source hunk and nothing else.

file = <path>
hunk:
  <context line>
+ <inserted line>
  <context line>

Response constraints:
- one fenced bash block only
- edit only file
- add only the line marked with +, without the + marker
- do not add comments or blank lines
- do not rewrite the whole file
- do not read, validate, or report after editing
```

Learning:

- Plain insertion variables are not enough. Without an explicit negative
  constraint, the model added the same kind of invented nearby comment that
  caused the original failure.
- Source edit decision cards and unified hunk patterns are reliable for this
  local implementation-discipline case. They preserve enough edit structure
  without requiring a fully prewritten command.
- Minimal good/bad examples can work when the bad behavior is concrete and
  local (`adds a comment, blank line, rewrites, reads, validates, or reports`).
  This differs from report materialization, where good/bad examples still
  allowed field rewriting.
- No `item_worker.yaml` change is retained from this experiment. The current
  static prompt can still invent adjacent comments; the reliable controls are
  latest-message edit packets for already-determined exact insertions.

## Experiment 122: Validation Runner Active Process Exact Match

Question: when a validation-related process is visible, does validation runner
distinguish the exact required command it just started from a similar broader
process?

Payload batch:

- `.pragma/prompt-ab/validation-runner-active-process-exact-match-20260603T000000Z`

Setup:

- exact-running case:
  - required command:
    `go test ./cmd/pragma/... -run TestFocusedCLI`,
  - latest output says that exact command is still running,
  - active process pid/log are `9021` and `/tmp/pragma/processes/9021.log`.
- mismatched-running case:
  - same required command,
  - latest output says the required focused command has not been started,
  - visible active process is only broader
    `go test ./cmd/pragma/...` with pid/log `7710` and
    `/tmp/pragma/processes/7710.log`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `exact_running_current` | current validation runner | ran `sleep 10; ps -p 9021; tail -n 100 /tmp/pragma/processes/9021.log` | Passed |
| `mismatched_running_current` | current validation runner | status-checked pid/log `7710` for the broader process instead of running the required focused command | Failed |
| `exact_running_loose_no_exact_match` | active-process rule without exact-command/started-by-agent constraints | status-checked pid/log `9021` | Passed |
| `mismatched_running_loose_no_exact_match` | loose active-process rule | status-checked pid/log `7710` for the broader process | Failed |
| `exact_running_decision_card` | latest active-process decision card with `command_match: exact` | ran the exact bounded status check for pid/log `9021` | Passed |
| `mismatched_running_decision_card` | latest active-process decision card with `command_match: mismatch` | ran the required focused validation command with status preservation | Passed |
| `exact_running_action_template` | latest action template with exact comparison variables | ran the exact bounded status check for pid/log `9021` | Passed |
| `mismatched_running_action_template` | latest action template with mismatch comparison variables | ran the required focused validation command with indexed status preservation | Passed |
| `exact_running_exact_match_gate_after_active_rule` | static exact-match gate placed immediately after active-process rule | status-checked pid/log `9021` | Passed |
| `mismatched_running_exact_match_gate_after_active_rule` | same static gate immediately after active-process rule | still status-checked pid/log `7710` for the broader process | Failed |
| `exact_running_exact_match_gate_footer` | same static exact-match gate as persona footer | status-checked pid/log `9021` | Passed |
| `mismatched_running_exact_match_gate_footer` | same static exact-match gate as persona footer | ran the required focused validation command with status preservation | Passed |
| `exact_running_retained_current` | checked-in `personas-research-v2/validation_runner.yaml` after retaining the footer | status-checked pid/log `9021` | Passed |
| `mismatched_running_retained_current` | checked-in `personas-research-v2/validation_runner.yaml` after retaining the footer | ran the required focused validation command with indexed status preservation | Passed |

Retained control:

```text
Active process exact-match gate:
The active-process rule applies only when the latest tool output is the direct
result of the validation command you just started and the active process
command text exactly matches the required validation command from patch
plan/checklist. If the active process command differs from the required
command, or the latest output says the required command has not been started,
ignore that process and run the required command with the validation wrapper.
Similar or broader commands are not substitutes for the required command.
```

Learning:

- Current active-process handling was too broad. It passed the exact-running
  case but treated a broader unrelated `go test` process as sufficient for the
  focused validation command.
- Loose active-process wording is misleading because it strengthens the wrong
  behavior: any validation-looking process can suppress the required command.
- Latest-message decision cards and action templates both reliably transfer the
  match/mismatch decision when the orchestrator has already computed it.
- The static phrase is position-sensitive. Placed next to the active-process
  rule it failed the mismatch case; appended as a persona footer it passed both
  exact and mismatch cases. Retain only the footer placement.

## Experiment 123: Patch Planner Source Change vs Validation-Only

Question: can patch planner distinguish a requested source/default change with
missing source evidence from a true validation-only verify/check task, without
regressing either route class?

Payload batch:

- `.pragma/prompt-ab/patch-planner-source-change-vs-validation-only-20260603T000000Z`

Setup:

- source-change missing-source case:
  - task intent: change `--max-retries` default from 3 to 5,
  - surface map named `cmd/pragma/jobs.go`,
  - evidence map had `Evidence source paths: none`,
  - evidence map had consumer path `cmd/pragma/jobs_test.go` and validation
    `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `Not evidenced` listed source edit path `cmd/pragma/jobs.go` with proof
    required:
    `inspect cmd/pragma/jobs.go and show the exact defaultMaxRetries source line before implementation`.
- validation-only verify case:
  - task intent: verify max retries default remains covered,
  - no source edit path,
  - consumer path `cmd/pragma/jobs_test.go`,
  - validation `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
  - `Not evidenced` explicitly said the missing source edit path was because
    this was validation-only verification.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `source_change_missing_source_current_retained` | current retained patch planner before this experiment | created a validation/pre-validation route from `cmd/pragma/jobs_test.go` and left the source change unresolved | Failed/partial |
| `validation_only_verify_current_retained` | current retained patch planner before this experiment | wrote a validation-only route with `source path: none`, exact validation, and no unresolved work | Passed |
| `source_change_missing_source_source_change_gate_after_validation_rule` | source-change gate inserted after validation-only rule | still created a validation/pre-validation route | Failed |
| `validation_only_verify_source_change_gate_after_validation_rule` | same inserted gate | kept the validation-only route | Passed |
| `source_change_missing_source_source_change_gate_footer` | source-change gate as persona footer | wrote `Patch route: - none` and the source-change proof under unresolved | Passed |
| `validation_only_verify_source_change_gate_footer` | same footer gate | kept the validation-only route | Passed |
| `source_change_missing_source_source_change_shape_footer` | stronger route-shape footer | wrote `Patch route: - none` and unresolved proof | Passed/redundant |
| `validation_only_verify_source_change_shape_footer` | stronger route-shape footer | kept the validation-only route | Passed |
| `source_change_missing_source_source_change_exact_copy_gate_footer` | footer gate plus verbatim proof-copy constraint | wrote `Patch route: - none` and copied proof-required text verbatim | Passed |
| `validation_only_verify_source_change_exact_copy_gate_footer` | exact-copy footer gate | kept the validation-only route | Passed |
| `source_change_missing_source_source_change_exact_shape_footer` | route-shape footer plus verbatim proof-copy constraint | wrote `Patch route: - none` and copied proof-required text verbatim | Passed/redundant |
| `validation_only_verify_source_change_exact_shape_footer` | exact route-shape footer | kept the validation-only route | Passed |
| `source_change_missing_source_retained_current` | checked-in `patch_planner.yaml` after retaining exact-copy footer | wrote `Patch route: - none` and copied proof-required text verbatim | Passed |
| `validation_only_verify_retained_current` | checked-in `patch_planner.yaml` after retaining exact-copy footer | kept the validation-only route with `source path: none` and exact validation command | Passed |

Retained control:

```text
Source-change unresolved gate:
The validation-only route rule applies only when the evidenced behavior is to
verify, check, or audit existing behavior. If the task intent or evidence
behavior is to change, modify, update, or set a source/default value, and the
evidence map says the source edit path or source/defaulting evidence is
missing/not evidenced, do not create a validation-only or pre-validation route
from a consumer/test path. Write `Patch route:` with exactly `- none`, and put
the requested source change under `Unresolved until evidence`. For the
`proof required:` line, copy the exact proof-required text from the evidence
map after `proof required:`; do not paraphrase it and do not add words such as
current value if they are not present in that evidence line.
```

Learning:

- The validation-only route rule needs an explicit behavior gate. Without it,
  patch planner correctly handles verify/check tasks but over-applies
  validation-only routing to source-change tasks with missing source evidence.
- Placement matters. The same semantic gate failed when placed immediately
  after the validation-only rule, but passed as a persona footer.
- A route-shape footer also passed but is more prescriptive than needed. The
  retained gate is the smallest passing static phrase after adding exact proof
  copying.
- Verbatim proof copying matters. A looser retained-current check fixed the
  route shape but rewrote the proof-required text, so the final retained phrase
  explicitly bans paraphrasing.

## Experiment 124: Item Worker Active Process Exact Match

Question: when item worker sees an active validation process, does it distinguish
the exact current-item command it started from a similar broader process?

Payload batch:

- `.pragma/prompt-ab/item-worker-active-process-exact-match-20260603T000000Z`

Setup:

- current item was validation-only with `validation_command`:
  `go test ./cmd/pragma/... -run TestFocusedCLI`,
- exact-running case:
  - item worker had just started the exact validation command,
  - latest output said it was still running,
  - active pid/log were `4172` and `/tmp/pragma/processes/4172.log`,
- mismatched-running case:
  - latest output said the required focused command had not been started,
  - visible active process was broader `go test ./cmd/pragma/...`,
  - active pid/log were `7331` and `/tmp/pragma/processes/7331.log`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| `exact_running_current` | current item worker | ran a bounded status/log check on pid/log `4172` | Passed |
| `mismatched_running_current` | current item worker | status-checked pid/log `7331` for the broader process | Failed |
| `exact_running_exact_match_gate_after_active_rule` | generic exact-match gate placed after active-process rule | status-checked pid/log `4172` | Passed |
| `mismatched_running_exact_match_gate_after_active_rule` | same generic gate after active-process rule | still status-checked pid/log `7331` | Failed |
| `exact_running_exact_match_gate_footer` | same generic exact-match gate as footer | status-checked pid/log `4172` | Passed |
| `mismatched_running_exact_match_gate_footer` | same generic exact-match gate as footer | still status-checked pid/log `7331` | Failed |
| `exact_running_latest_decision_card` | latest active-process decision card with `command_match: exact` | ran exact bounded status check | Passed |
| `mismatched_running_latest_decision_card` | latest active-process decision card with `command_match: mismatch` | ran the required focused validation command with status preservation | Passed |
| `exact_running_last_mile_decision_table_footer` | generic last-mile comparison table as footer | ran exact bounded status check | Passed |
| `mismatched_running_last_mile_decision_table_footer` | generic last-mile comparison table as footer | ran the required focused validation command with status preservation | Passed |
| `exact_running_mismatch_override_footer` | task-specific mismatch override | ran a bounded status check | Passed/task-specific |
| `mismatched_running_mismatch_override_footer` | task-specific mismatch override | ran the required focused validation command | Passed/task-specific |
| `exact_running_combined_table_and_override_footer` | generic table plus task-specific override | ran exact bounded status check | Passed/redundant |
| `mismatched_running_combined_table_and_override_footer` | generic table plus task-specific override | ran required focused validation command | Passed/redundant |
| `exact_running_retained_current` | checked-in `item_worker.yaml` after retaining the strict table | ran `sleep 10; ps -p 4172; tail -n 100 /tmp/pragma/processes/4172.log` | Passed |
| `mismatched_running_retained_current` | checked-in `item_worker.yaml` after retaining the strict table | ran the required focused validation command with status preservation and no prose | Passed |

Retained control:

```text
Last-mile active process decision table:
Before waiting on an active process, compare the active process command text to
the current item's exact validation_command or producer_command.
- If latest output is from the command you just started and active process
  command equals the current-item command exactly, run one bounded status
  check: `sleep 10; ps -p <pid>; tail -n 100 <log>`.
- If active process command is different, broader, missing flags, or latest
  output says the required current-item command has not been started, do not
  wait on that process and do not read its log. Run the current item's exact
  validation_command or producer_command with the required status wrapper.
Similar commands are mismatches. Do not explain the comparison in prose. The
assistant response for either branch must start with ```bash and contain only
one fenced bash block.
```

Learning:

- Current item worker had the same broad active-process suppression failure as
  validation runner, but the validation-runner style exact-match footer did not
  transfer. It still waited on the broader process.
- Latest-message decision cards transfer the already-computed match/mismatch
  decision cleanly, but a static persona needs an explicit last-mile comparison
  table rather than a semantic exact-match paragraph.
- The first checked-in table chose the right mismatch action but leaked prose
  before the bash block. The retained version therefore includes branch-local
  response-shape control.
- Task-specific mismatch overrides pass but are not retainable; the generic
  table is the retained primitive.

## Experiment 125: Item Worker Repeated-Search Static Footer

Question: can a static persona footer make repeated no-new-information states
write blocker reports across item shapes, without requiring latest-message exact
field materialization?

Payload batch:

- `.pragma/prompt-ab/item-worker-repeated-search-static-footer-20260603T000000Z`

Setup:

- reused the two repeated-search cases from Experiments 113 and 120:
  - config-symbol case:
    `item-auth-config-symbol Wire Kubernetes auth config symbol`,
    `repeat_count = 4`, repeated searches for `KubernetesTokenValidator` and
    adjacent config names found no new information,
  - proto/generated case:
    `item-002 Generate Kubernetes auth protobuf bindings`,
    `repeat_count = 3`, repeated searches found no producer command and no
    service symbol.
- pass required:
  - no further file inspection, search, edit, or validation,
  - write `/tmp/pragma/implementer-report.md`,
  - include concrete current item, no changed files, repeated-search blocker,
    blocked validation, concrete remaining risk, and sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| config | current retained item worker | read `/tmp/pragma/current-item.json` and `internal/config/authentication.go` instead of writing report | Failed |
| proto | current retained item worker | wrote blocker report and sentinel | Passed |
| config | last-mile repeated-search report shape footer | wrote blocker report, no inspection, stable blocked validation/risk, concrete `KubernetesTokenValidator` blocker | Passed |
| proto | last-mile repeated-search report shape footer | wrote blocker report, no inspection, stable blocked validation/risk, but missed the service-symbol detail | Partial |
| config | repeated-search state-copy footer | wrote blocker report and sentinel, copied concrete search phrase, but omitted repeat count in blocker | Partial |
| proto | repeated-search state-copy footer | wrote blocker report and sentinel, copied `no producer command and no service symbol`, but omitted repeat count in blocker | Partial |
| config | repeated-search precedence table footer | wrote blocker report and sentinel, but used `Validation: N/A - blocked before implementation` and `Remaining risk: none` | Partial |
| proto | repeated-search precedence table footer | wrote blocker report and sentinel, but used weaker validation/risk and dropped service-symbol detail | Partial |
| config | combined shape plus precedence footer | wrote blocker report and sentinel, no inspection, stable validation/risk, repeat count, and concrete missing symbol | Passed |
| proto | combined shape plus precedence footer | wrote blocker report and sentinel, no inspection, stable validation/risk, repeat count, and concrete missing producer/service evidence | Passed |
| config | checked-in `item_worker.yaml` after retaining combined footer | wrote blocker report and sentinel with repeat count and `KubernetesTokenValidator`/config evidence | Passed |
| proto | checked-in `item_worker.yaml` after retaining combined footer | wrote blocker report and sentinel with repeat count and missing producer/service evidence | Passed |

Retained controls:

```text
Last-mile repeated-search report shape:
If the latest user message contains `Search state:` with
`repeated_no_new_information = true`, stop implementation immediately. Do not
inspect files, rerun searches, run validation, edit files, or follow the normal
work method. Write /tmp/pragma/implementer-report.md now with exactly these
sections: Current item, Changed, Acceptance evidence, Validation, Remaining
risk, Blocker. Use `Changed: No files changed`. Use `Validation: N/A - blocked
after repeated no-new-information searches`. Use `Remaining risk: exact
implementation evidence remains unknown`. The Blocker must mention the repeat
count and the missing symbol/producer/service named in the latest user
message. The response must start with ```bash and contain only one fenced bash
block ending with echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT.

Repeated-search precedence table:
Before applying the normal work method, check visible Search state.
- If repeated_no_new_information is true, write /tmp/pragma/implementer-report.md
  immediately and stop.
- This condition outranks allowed_files, source-inspection, validation, and
  patching steps.
- Use the visible current item id/title, repeat_count, and Recent repeated
  commands sentence as the report evidence.
- Do not read /tmp/pragma/current-item.json again and do not read any
  repository path.
The assistant response must be one fenced bash block only.
```

Learning:

- The original static search-state variables were not enough. They passed the
  generated/proto case but failed the config-symbol transfer by reopening the
  allowed source file.
- A report shape alone fixed the config case but could drop one concrete detail
  in the proto case. A precedence table alone stopped inspection but weakened
  validation/risk fields.
- The passing static control is the combination: shape fixes report fields,
  precedence makes repeated-search outrank the normal work method.
- Latest-message exact field lines/YAML/section templates remain stronger when
  exact report wording is required. The retained static footer is for robust
  stop-and-report behavior from visible search state, not field-perfect
  materialization.

## Experiment 126: Item Worker Exact-Insertion Static Footer

Question: can a static item-worker prompt prevent invented adjacent comments
for exact one-line insertions, while preserving ordinary minimal value
replacement behavior?

Payload batch:

- `.pragma/prompt-ab/item-worker-exact-insertion-static-footer-20260603T000000Z`

Setup:

- exact-insertion case reused the `RetryPolicy` source from Experiments 117 and
  121:
  - current item said to add exactly one line:
    `Backoff time.Duration`,
  - insertion anchor was visible:
    `Delay time.Duration`,
  - pass required a single edit command that inserts only
    `Backoff time.Duration`, with no comment, blank line, validation, report, or
    second command.
- simple value-edit control reused the `defaultMaxRetries` item:
  - visible source had `const defaultMaxRetries = 3`,
  - pass required the normal minimal replacement to `5`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| exact insertion | current retained item worker | inserted an invented Backoff comment plus `Backoff time.Duration` | Failed |
| exact insertion | exact-insertion gate footer | still inserted an invented Backoff comment plus the field | Failed |
| exact insertion | source edit decision table footer | still inserted an invented Backoff comment plus the field | Failed |
| exact insertion | source edit decision table plus exact command-shape hint | inserted only `Backoff time.Duration` | Passed |
| simple value edit | current retained item worker | changed only `const defaultMaxRetries = 3` to `5` | Passed |
| simple value edit | exact-insertion gate footer | changed only `const defaultMaxRetries = 3` to `5` | Passed |
| simple value edit | source edit decision table footer | changed only `const defaultMaxRetries = 3` to `5` | Passed |
| simple value edit | source edit decision table plus exact command-shape hint | changed only `const defaultMaxRetries = 3` to `5` | Passed |
| exact insertion | checked-in `item_worker.yaml` after retaining table plus command-shape hint | inserted only `Backoff time.Duration` | Passed |
| simple value edit | checked-in `item_worker.yaml` after retaining table plus command-shape hint | changed only `const defaultMaxRetries = 3` to `5` | Passed |

Retained controls:

```text
Last-mile source edit decision table:
Before editing a visible allowed source file, classify the requested edit.
- Exact insertion: if the current item says `add exactly one line: <line>` or
  `insert exactly one line: <line>` after an anchor, insert only `<line>` after
  the visible anchor. Comments and blank lines are forbidden even if they seem
  helpful. Do not validate or report in the same response.
- Value replacement: if the current item asks to change one visible value from
  `<old>` to `<new>`, replace only that value in the allowed file. Do not add
  fields, comments, formatting, validation, or reports in the same response.
The assistant response must start with ```bash and contain one edit command
only.

Exact insertion command shape:
For exact one-line insertion after a visible anchor, prefer an insertion
command whose inserted payload is exactly the requested source line. The edit
command must not contain `//`, `/*`, blank inserted lines, or any inserted text
other than the requested line.
```

Learning:

- A semantic exact-insertion gate is not enough. The model still added a helpful
  comment next to the requested field.
- A source-edit decision table is also not enough by itself. It correctly
  classifies the edit but still allows the generated edit command to include a
  comment.
- The necessary control is command-shape level: the inserted payload must be
  exactly the requested line, and the edit command must not contain comment
  tokens or blank inserted lines.
- The retained static footer did not regress the simpler value-replacement
  edit, so this can move from latest-message-only materialization into
  `item_worker.yaml`.

## Experiment 127: Final Reviewer No-Diff Static Artifact Branches

Question: can the successful dirty repaired-path placeholder primitive transfer
from an isolated latest user message into static `final_reviewer.yaml` footer
text if the footer is written as an artifact-first branch?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-nodiff-static-artifact-20260603T000000Z`

Setup:

- reused the six-message final reviewer dirty auth, dirty cache, and clean auth
  control transcripts from Experiment 105,
- replaced only the persona prompt message, preserving the original validation
  read and diff-observation turns,
- dirty cases had a successful `git diff --exit-code -- <path>` command with
  `EXIT_STATUS: 0` and visible final diff still containing that same path,
- clean auth had the same successful repair no-diff command but final diff only
  contained `cmd/pragma/jobs.go`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean auth | code-only no-diff branch footer | wrote APPROVE verdict with sentinel | Passed clean |
| clean auth | literal-match branch footer | wrote APPROVE verdict with sentinel | Passed clean |
| clean auth | placeholder-pattern static footer | wrote APPROVE verdict with sentinel | Passed clean |
| dirty auth | code-only no-diff branch footer | repeated hidden reasoning to `finish_reason: length`; no verdict content | Failed |
| dirty auth | literal-match branch footer | repeated hidden reasoning to `finish_reason: length`; no verdict content | Failed |
| dirty auth | placeholder-pattern static footer | repeated hidden reasoning to `finish_reason: length`; no verdict content | Failed |
| dirty cache | code-only no-diff branch footer | wrote BLOCK with no-diff contradiction, exact path/command, and sentinel | Passed dirty only |
| dirty cache | literal-match branch footer | repeated hidden reasoning to `finish_reason: length`; no verdict content | Failed |
| dirty cache | placeholder-pattern static footer | repeated hidden reasoning to `finish_reason: length`; no verdict content | Failed |

Learning:

- Static footer transfer still is not reliable. All variants preserved clean
  approval, but only one dirty-cache case blocked; every dirty-auth case
  looped to `finish_reason: length`.
- The passing latest-message placeholder pattern does not become reliable just
  because the same shape is appended to the persona prompt. The isolated final
  user message remains the control boundary.
- Code-only/artifact-first wording can help one concrete dirty path, but it
  does not generalize across repaired paths. Do not retain a
  `final_reviewer.yaml` change from this batch.
- The durable orchestration contract is unchanged: detect the no-diff repair
  contradiction outside the final reviewer persona and send a latest-message
  materialization packet with `repaired_path`, `no_diff_command`, and the
  placeholder verdict pattern.

## Experiment 128: Final Reviewer Materialization Packet Position

Question: for dirty repaired-path final review, does the successful
placeholder materialization packet need to be an isolated final user message,
or is it reliable when co-located with the latest diff-output message?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-materialization-position-20260603T000000Z`

Setup:

- reused the dirty auth and dirty cache repaired-path contradiction transcripts,
- both cases had a successful `git diff --exit-code -- <path>` command with
  `EXIT_STATUS: 0` and visible final diff still containing that same path,
- compared three positions for the same placeholder verdict packet:
  - separate final user message after diff output,
  - appended to the existing diff-output user message,
  - prepended to the existing diff-output user message.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| dirty auth | separate final user packet | wrote BLOCK with exact no-diff contradiction, exact path/command, one fenced bash block, and sentinel | Passed |
| dirty auth | packet appended to diff output | wrote the same exact BLOCK artifact | Passed |
| dirty auth | packet prepended to diff output | wrote the same exact BLOCK artifact | Passed |
| dirty cache | separate final user packet | wrote BLOCK with exact no-diff contradiction, exact path/command, one fenced bash block, and sentinel | Passed |
| dirty cache | packet appended to diff output | wrote the same exact BLOCK artifact | Passed |
| dirty cache | packet prepended to diff output | wrote the same exact BLOCK artifact | Passed |

Learning:

- Message separation is not the necessary property. The necessary property
  tested here is latest-turn materialization: the placeholder verdict packet is
  present in the final visible user/tool-output turn, after the final reviewer
  has already seen validation status and diff evidence.
- The packet is robust to being prepended or appended around raw diff output in
  that latest turn. This gives orchestration two viable implementations:
  inject a final user message, or attach the packet to the diff-observation
  message when the no-diff contradiction is detected.
- This does not make the packet retainable as persona body text. Experiment 127
  showed the same placeholder pattern fails when moved into static
  `final_reviewer.yaml` footer text.

## Experiment 129: Final Reviewer Noisy History Materialization

Question: if a no-diff contradiction materialization packet is introduced after
validation and diff evidence, does it have to remain the literal latest turn, or
can it survive short aligned reminder turns?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-materialization-noisy-history-20260603T000000Z`

Setup:

- reused the dirty auth and dirty cache repaired-path contradiction transcripts,
- both cases had visible validation status plus final diff evidence,
- added two synthetic assistant/user reminder turns after diff evidence:
  - one note that validation status and final diff are visible,
  - one reminder to compare the repaired-path no-diff command with final diff,
- compared:
  - materialization packet after the reminder noise,
  - materialization packet before the reminder noise,
  - no materialization packet, only reminder noise.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| dirty auth | no packet after reminder noise | repeated reasoning to `finish_reason: length`; no verdict content | Failed |
| dirty auth | packet buried before reminder noise | wrote exact BLOCK no-diff contradiction artifact with sentinel | Passed |
| dirty auth | packet latest after reminder noise | wrote exact BLOCK no-diff contradiction artifact with sentinel | Passed |
| dirty cache | no packet after reminder noise | incorrectly wrote APPROVE because both changed files were in `validated_surfaces` | Failed |
| dirty cache | packet buried before reminder noise | wrote exact BLOCK no-diff contradiction artifact with sentinel | Passed |
| dirty cache | packet latest after reminder noise | wrote exact BLOCK no-diff contradiction artifact with sentinel | Passed |

Learning:

- Latest-turn placement is useful but not strictly necessary in short aligned
  noisy history. Once the placeholder verdict packet is introduced after diff
  evidence, it survives two later non-conflicting reminder turns across both
  dirty paths.
- Reminder-only wording is not enough. Without the materialization packet, the
  final reviewer either loops on dirty auth or approves dirty cache because the
  repaired path appears in `validated_surfaces`.
- The durable boundary is now narrower and more accurate: inject a
  conversation-level materialization packet after validation and final diff
  evidence are visible. It may be the latest turn or may precede short aligned
  reminders, but it is not replaceable by persona-body text or semantic
  reminder prose.

## Experiment 130: Item Worker Failed-Validation Packet Position

Question: for item-worker failed focused-validation stop states, does the
field-lines implementer-report packet need to stay latest, or can it be buried
before later reminder turns as in the final-reviewer no-diff case?

Payload batch:

- `.pragma/prompt-ab/item-worker-failed-validation-materialization-position-20260603T000000Z`

Setup:

- reused the two failed-validation report materialization cases from
  Experiment 112:
  - max retries: `go test ./cmd/pragma/... -run TestMaxRetriesDefault` failed
    with `TestMaxRetriesDefault expected default 3, got 0`,
  - cache TTL: `go test ./internal/cache -run TestDefaultTTL` failed with
    `TestDefaultTTL expected default TTL 60, got 0`,
- compared five positions:
  - field-lines packet appended to the failed validation output,
  - field-lines packet as a separate final user message,
  - field-lines packet before two aligned reminder turns,
  - field-lines packet after two aligned reminder turns,
  - no packet, only the reminder turns.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| cache TTL | no packet after reminder noise | wrote a blocker report, but rewrote item title, remaining risk, and blocker into weaker test-inspection language | Failed |
| cache TTL | packet appended to failure output | wrote exact blocker implementer report with sentinel | Passed |
| cache TTL | packet buried before reminder noise | inspected `internal/cache/options.go` instead of writing report | Failed |
| cache TTL | packet latest after reminder noise | wrote exact blocker implementer report with sentinel | Passed |
| cache TTL | packet as separate final user | wrote exact blocker implementer report with sentinel | Passed |
| max retries | no packet after reminder noise | inspected `cmd/pragma/jobs.go` with grep instead of writing report | Failed |
| max retries | packet appended to failure output | wrote exact blocker implementer report with sentinel | Passed |
| max retries | packet buried before reminder noise | wrote exact blocker implementer report with sentinel | Passed |
| max retries | packet latest after reminder noise | wrote exact blocker implementer report with sentinel | Passed |
| max retries | packet as separate final user | wrote exact blocker implementer report with sentinel | Passed |

Learning:

- Item-worker failed-validation stop is more placement-sensitive than
  final-reviewer no-diff materialization. The field-lines packet transferred
  when appended to failed validation output, sent as a separate final message,
  or sent after reminder noise, but a buried packet failed the cache TTL
  transfer by returning to source inspection.
- Reminder-only wording is weak. One no-packet case inspected source, while
  the other wrote a report but rewrote the exact risk/blocker fields into
  weaker test-inspection language.
- For this stop condition, orchestration should make the failed-validation
  field-lines packet the latest action instruction or attach it directly to the
  failed validation output. Do not rely on a packet buried earlier in the
  conversation.
- No `item_worker.yaml` change is retained; static failed-validation stop
  wording already failed in earlier replay.

## Experiment 131: Item Worker Repeated-Search Packet Position

Question: for item-worker repeated no-new-information stop states, does the
placeholder report packet need to stay latest, or can it be buried before later
aligned reminder turns?

Payload batch:

- `.pragma/prompt-ab/item-worker-repeated-search-materialization-position-20260603T000000Z`

Setup:

- reused the two repeated-search stop cases from Experiments 113 and 120:
  - config: no `KubernetesTokenValidator` source symbol after 4 repeated
    searches,
  - proto: no protobuf producer command or service symbol after 3 repeated
    searches,
- started from the state-only current-item output,
- compared five positions:
  - placeholder report packet appended to the current-item/search-state output,
  - placeholder report packet as a separate final user message,
  - placeholder report packet before two aligned reminder turns,
  - placeholder report packet after two aligned reminder turns,
  - no packet, only the reminder turns.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| config | no packet after reminder noise | wrote a report, but rewrote item title, acceptance, validation, remaining risk, and blocker fields | Failed |
| config | packet appended to state output | wrote exact blocker report with sentinel | Passed |
| config | packet buried before reminder noise | wrote exact blocker report with sentinel | Passed |
| config | packet latest after reminder noise | wrote exact blocker report with sentinel | Passed |
| config | packet as separate final user | wrote exact blocker report with sentinel | Passed |
| proto | no packet after reminder noise | wrote a report, but rewrote item title, acceptance, validation, remaining risk, and blocker fields | Failed |
| proto | packet appended to state output | wrote exact blocker report with sentinel | Passed |
| proto | packet buried before reminder noise | wrote exact blocker report with sentinel | Passed |
| proto | packet latest after reminder noise | wrote exact blocker report with sentinel | Passed |
| proto | packet as separate final user | wrote exact blocker report with sentinel | Passed |

Learning:

- Repeated-search report materialization is placement-tolerant once the stop
  state is supplied. Unlike failed-validation stop, the placeholder packet
  survived two later aligned reminder turns across both repeated-search shapes.
- Reminder-only wording is still too weak for exact handoff fields. It caused
  report writing, but both cases rewrote validation/risk/blocker language into
  generic phrasing.
- For repeated-search stop states, the packet can be appended to the state
  output, sent separately, sent after reminders, or appear before short aligned
  reminders. Use the packet whenever exact downstream fields matter.
- No `item_worker.yaml` change is retained from this placement replay; the
  retained static footer from Experiment 125 already addresses current
  repeated-search classification.

## Experiment 132: Item Worker Validation Command Packet Position

Question: for item-worker validation command shape, does the YAML command
packet need to stay latest, and does placement affect exact
status-preserving-wrapper generation?

Payload batch:

- `.pragma/prompt-ab/item-worker-validation-command-packet-position-20260603T000000Z`

Setup:

- reused the two command-shape cases from Experiment 115:
  - validation-only item for
    `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
  - post-source-edit item for
    `go test -run TestMaxRetriesDefault ./cmd/pragma/`,
- compared five positions:
  - YAML command packet appended to the current state/tool output,
  - YAML command packet as a separate final user message,
  - YAML command packet before two aligned reminder turns,
  - YAML command packet after two aligned reminder turns,
  - no packet, only reminder turns.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| post-source-edit | no packet after reminder noise | emitted full status-preserving wrapper | Passed |
| post-source-edit | packet appended to state output | emitted full status-preserving wrapper | Passed |
| post-source-edit | packet buried before reminder noise | emitted full status-preserving wrapper | Passed |
| post-source-edit | packet latest after reminder noise | emitted full status-preserving wrapper | Passed |
| post-source-edit | packet as separate final user | emitted full status-preserving wrapper | Passed |
| validation-only | no packet after reminder noise | used `$?`, no `tee`, no `${PIPESTATUS[0]}`, no final `exit "$status"` | Failed |
| validation-only | packet appended to state output | preserved status semantics, but wrote `log="/tmp/pragma/item-validation.log"` instead of the canonical unquoted `log=/tmp/pragma/item-validation.log` | Passed/shape drift |
| validation-only | packet buried before reminder noise | same status-preserving wrapper with quoted log assignment | Passed/shape drift |
| validation-only | packet latest after reminder noise | same status-preserving wrapper with quoted log assignment | Passed/shape drift |
| validation-only | packet as separate final user | same status-preserving wrapper with quoted log assignment | Passed/shape drift |

Learning:

- YAML command packets are placement-tolerant for status semantics. Appended,
  separate, buried, and latest-after-noise placements all preserved
  `set -o pipefail`, `tee "$log"`, `${PIPESTATUS[0]}`, `EXIT_STATUS`, and
  final `exit "$status"`.
- Byte-level shell layout is still not guaranteed by the YAML packet. In every
  validation-only packet placement, the model quoted the log assignment. This
  is semantically fine but weaker than the full pasted wrapper when exact
  script text matters.
- Reminder-only wording is not enough for validation-only command shape; it
  regressed to `$?` status capture. The post-source-edit no-packet control
  passed because `item_worker.yaml` already has a retained
  post-source-edit validation shell rule.
- No `item_worker.yaml` change is retained. For validation-only command shape,
  use the existing static validation-only shell rule or an orchestration packet;
  use a full pasted wrapper if byte-level command layout matters.

## Experiment 133: Item Reviewer Approval Packet Position

Question: for item-reviewer approval states, do exact field-lines verdict
packets preserve the intended three Findings lines when the packet is moved or
followed by short reminder turns?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-approval-packet-position-20260603T000000Z`

Setup:

- reused the two approval materialization cases from Experiment 114:
  - source edit approval for `cmd/pragma/jobs.go`,
  - out-of-scope repair approval for `internal/server/auth/debug.go`,
- compared five positions:
  - field-lines packet appended to the current item / implementer report
    output,
  - field-lines packet as a separate final user message,
  - field-lines packet before two aligned reminder turns,
  - field-lines packet after two aligned reminder turns,
  - no packet, only reminder turns.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| repair approval | no packet after reminder noise | wrote APPROVE with three findings, but rewrote all three findings generically | Passed/weaker |
| repair approval | packet appended to report output | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| repair approval | packet buried before reminder noise | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| repair approval | packet latest after reminder noise | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| repair approval | packet as separate final user | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| source approval | no packet after reminder noise | wrote APPROVE with three findings, but rewrote all three findings generically | Passed/weaker |
| source approval | packet appended to report output | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| source approval | packet buried before reminder noise | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| source approval | packet latest after reminder noise | wrote exact three-finding APPROVE verdict with sentinel | Passed |
| source approval | packet as separate final user | wrote exact three-finding APPROVE verdict with sentinel | Passed |

Learning:

- Item-reviewer approval materialization is placement-tolerant. Field-lines
  packets preserved exact findings when appended, separate, buried before short
  aligned reminders, or latest after reminders.
- The generic reviewer prompt plus reminder-only wording is enough for the
  APPROVE decision and three-finding shape in these low-conflict approval
  states, but it rewrites evidence phrasing. Use packets only when exact
  findings are part of the downstream contract.
- This is another contrast with item-worker failed-validation stop: low-conflict
  approval packets can be buried safely, while failed-validation report packets
  should stay latest or attached to the failure output.
- No `item_reviewer.yaml` change is retained.

## Experiment 134: Patch Planner Unresolved-Source Packet Position

Question: for source-change tasks where the source edit path is not evidenced,
does the route-none patch-plan packet preserve downstream `Patch route: - none`
shape when moved or followed by aligned reminder turns?

Payload batch:

- `.pragma/prompt-ab/patch-planner-unresolved-source-packet-position-20260603T000000Z`

Setup:

- reused the pre-retention source-change unresolved case from Experiment 123,
  where the old patch planner created a validation-only/pre-validation route,
- surface/evidence maps named `cmd/pragma/jobs.go` and
  `cmd/pragma/jobs_test.go`, but the evidence map had no source content or
  source/defaulting evidence,
- compared five variants:
  - no packet, only aligned reminder turns,
  - placeholder route-none packet appended to the map output,
  - placeholder route-none packet as a separate final user message,
  - placeholder route-none packet before two aligned reminder turns,
  - placeholder route-none packet after two aligned reminder turns.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| no packet after reminder noise | wrote a route-none plan, but used scalar-looking `Patch route:\nnone` and rewrote the behavior text | Failed |
| packet appended to map output | wrote exact `Patch route:\n- none`, exact unresolved behavior/proof, and sentinel | Passed |
| packet as separate final user | wrote exact `Patch route:\n- none`, exact unresolved behavior/proof, and sentinel | Passed |
| packet buried before reminder noise | wrote exact `Patch route:\n- none`, exact unresolved behavior/proof, and sentinel | Passed |
| packet latest after reminder noise | wrote exact `Patch route:\n- none`, exact unresolved behavior/proof, and sentinel | Passed |

Learning:

- Patch-planner unresolved-source materialization is placement-tolerant. The
  placeholder packet preserved exact route-none list shape and proof text when
  appended, separate, latest after reminders, or buried before reminders.
- Reminder-only wording improved the old failure enough to avoid the
  validation-only route, but it still broke downstream shape by writing
  `Patch route:\nnone` and rewrote the behavior text. That is not acceptable
  when checklist parsing expects a list.
- This reinforces the earlier materialization finding: section skeletons are
  the right control for patch-plan parsability. Decision/reminder prose can get
  the high-level choice right while still producing a weak artifact.
- No `patch_planner.yaml` change is retained from this placement replay; the
  retained static source-change unresolved footer already covers the current
  persona prompt.

## Experiment 135: Validation Runner Active-Process Decision Card Position

Question: for validation-runner active-process exact/mismatch states, do
decision cards need to stay latest, or do aligned reminder turns also preserve
the correct wait-vs-run branch?

Payload batch:

- `.pragma/prompt-ab/validation-runner-active-process-decision-card-position-20260603T000000Z`

Setup:

- reused the exact-running and mismatched-running cases from Experiment 122,
- exact case:
  - required command and active process command were both
    `go test ./cmd/pragma/... -run TestFocusedCLI`,
  - active pid/log were `9021` and `/tmp/pragma/processes/9021.log`,
- mismatch case:
  - required command was
    `go test ./cmd/pragma/... -run TestFocusedCLI`,
  - visible active process was broader `go test ./cmd/pragma/...` with pid/log
    `7710` and `/tmp/pragma/processes/7710.log`,
- compared five positions:
  - decision card appended to the process output,
  - decision card as a separate final user message,
  - decision card before two aligned reminder turns,
  - decision card after two aligned reminder turns,
  - no decision card, only aligned reminder turns.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| exact running | no card after reminder noise | ran `sleep 10; ps -p 9021; tail -n 100 /tmp/pragma/processes/9021.log` | Passed |
| exact running | card appended to process output | ran the exact bounded status check for pid/log `9021` | Passed |
| exact running | card buried before reminder noise | ran the exact bounded status check for pid/log `9021` | Passed |
| exact running | card latest after reminder noise | ran the exact bounded status check for pid/log `9021` | Passed |
| exact running | card as separate final user | ran the exact bounded status check for pid/log `9021` | Passed |
| mismatch | no card after reminder noise | ran the required focused validation command with status preservation and did not wait for pid `7710` | Passed |
| mismatch | card appended to process output | ran the required focused validation command with status preservation and did not wait for pid `7710` | Passed |
| mismatch | card buried before reminder noise | ran the required focused validation command with status preservation and did not wait for pid `7710` | Passed |
| mismatch | card latest after reminder noise | ran the required focused validation command with status preservation and did not wait for pid `7710` | Passed |
| mismatch | card as separate final user | ran the required focused validation command with status preservation and did not wait for pid `7710` | Passed |

Learning:

- Validation-runner active-process decision cards are placement-tolerant across
  appended, separate, buried, and latest-after-reminder positions.
- In this low-conflict command-comparison state, aligned reminder prose without
  a card also fixed the old mismatch failure by making the runner compare active
  process command text to the required command. This differs from exact artifact
  materialization states where reminders often preserve the decision but rewrite
  downstream fields.
- Wrapper shape varies across mismatch variants: some use indexed
  `COMMAND[1]`/`EXIT_STATUS[1]` or a `bash -lc "$cmd"` wrapper, while others use
  the simpler direct wrapper. All preserved pipefail, tee, `${PIPESTATUS[0]}`,
  exit-status echo, final exit, and avoided pid/log `7710`.
- No `validation_runner.yaml` change is retained; the retained exact-match
  footer already covers the current persona prompt.

## Experiment 136: Item Worker Active-Process Decision Card Position

Question: for item-worker active-process exact/mismatch states, do
decision cards remain reliable when moved or followed by aligned reminder
turns, and can reminder-only wording fix the broader-process mismatch?

Payload batch:

- `.pragma/prompt-ab/item-worker-active-process-decision-card-position-20260603T000000Z`

Setup:

- reused the exact-running and mismatched-running cases from Experiment 124,
- exact case:
  - required command and active process command were both
    `go test ./cmd/pragma/... -run TestFocusedCLI`,
  - active pid/log were `4172` and `/tmp/pragma/processes/4172.log`,
- mismatch case:
  - required command was
    `go test ./cmd/pragma/... -run TestFocusedCLI`,
  - visible active process was broader `go test ./cmd/pragma/...` with pid/log
    `7331` and `/tmp/pragma/processes/7331.log`,
- compared five positions:
  - decision card appended to the process output,
  - decision card as a separate final user message,
  - decision card before two aligned reminder turns,
  - decision card after two aligned reminder turns,
  - no decision card, only aligned reminder turns.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| exact running | no card after reminder noise | ran `sleep 10; ps -p 4172; tail -n 100 /tmp/pragma/processes/4172.log` | Passed |
| exact running | card appended to process output | ran the exact bounded status check for pid/log `4172` | Passed |
| exact running | card buried before reminder noise | ran the exact bounded status check for pid/log `4172` | Passed |
| exact running | card latest after reminder noise | ran the exact bounded status check for pid/log `4172` | Passed |
| exact running | card as separate final user | ran the exact bounded status check for pid/log `4172` | Passed |
| mismatch | no card after reminder noise | waited on pid/log `7331` for the broader process | Failed |
| mismatch | card appended to process output | ran the required focused validation command with status preservation and did not wait for pid `7331` | Passed |
| mismatch | card buried before reminder noise | ran the required focused validation command with status preservation and did not wait for pid `7331` | Passed |
| mismatch | card latest after reminder noise | ran the required focused validation command with status preservation and did not wait for pid `7331` | Passed |
| mismatch | card as separate final user | ran the required focused validation command with status preservation and did not wait for pid `7331` | Passed |

Learning:

- Item-worker active-process decision cards are placement-tolerant across
  appended, separate, buried, and latest-after-reminder positions.
- Reminder-only wording is not enough for item-worker mismatch. Unlike the
  validation-runner placement replay, the no-card mismatch control still waited
  on the broader pid/log. The item-worker prompt needs either the retained
  last-mile table or an explicit decision card for this branch.
- Exact-running is easy: both cards and no-card reminders choose the bounded
  status check for the exact active process.
- No `item_worker.yaml` change is retained; the retained last-mile active
  process decision table already covers the current persona prompt.

## Experiment 137: Validation Runner Multi-Command Template Minimality

Question: for validation-runner multi-command execution, is semantic "run all
commands" wording enough, or does the runner need a literal indexed loop
template or latest-turn script packet to preserve parser-friendly command
status output?

Payload batch:

- `.pragma/prompt-ab/validation-runner-multi-command-template-minimality-20260603T000000Z`

Setup:

- reused the two-command validation payload from Experiment 33:
  - `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
  - `go test ./cmd/pragma/... -run TestRetryHelp`,
- used the old pre-retention validation-runner prompt as the control so the
  added language was isolated,
- compared no multi-command control, semantic rule, indexed-status rule, full
  loop template, compact function template, reminder-noise variants,
  latest-turn loop packets, and checked-in retained current prompt.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| no multi-command control | ran both commands with combined exit, but no indexed `COMMAND[n]`/`EXIT_STATUS[n]` labels and one shared log | Partial |
| semantic rule footer | ran both commands with combined exit, but used generic `EXIT_STATUS` labels and one shared log | Partial |
| semantic rule before reminder noise | ran both commands, then inspected forbidden file and wrote `/tmp/pragma/validation-status.md` prematurely | Failed |
| indexed-status rule footer | ran both commands with literal `COMMAND[1]`/`COMMAND[2]` and `EXIT_STATUS[1]`/`EXIT_STATUS[2]`, but used one log plus extra `FINAL_EXIT_STATUS` | Passed/weaker |
| compact function template footer | used runtime indexed labels, per-command logs, `${PIPESTATUS[0]}`, and combined exit, but changed the retained loop shape into a helper function | Passed/weaker |
| full loop template footer | used the retained loop shape with per-command logs, runtime indexed labels, `${PIPESTATUS[0]}`, and combined exit | Passed |
| full loop template before reminder noise | used the retained loop shape despite trailing reminder noise | Passed |
| latest loop packet after tool output | copied the latest-turn loop packet closely enough to preserve the retained shape | Passed |
| latest loop packet after reminder noise | copied the latest-turn loop packet closely enough to preserve the retained shape after reminder noise | Passed |
| retained current prompt | checked-in `validation_runner.yaml` produced the retained loop shape | Passed |

Learning:

- Semantic "run all commands" wording is enough to make the model run both
  commands in this replay, but it is not enough for durable downstream status
  parsing. It omits indexed command/status labels and can write
  `/tmp/pragma/validation-status.md` early when followed by reminder noise.
- An indexed-status rule without the full loop is semantically acceptable but
  weaker: it preserves literal `COMMAND[1]`/`EXIT_STATUS[1]` fields, but drifts
  to a shared log and an extra final-status echo.
- The retained loop template is still the best default because it localizes
  command list, per-command log path, indexed status echo, `${PIPESTATUS[0]}`,
  and any-failure final exit in one shell shape.
- Latest-turn loop packets are reliable for exact command-shape materialization,
  even after short aligned reminder noise.
- No `validation_runner.yaml` change is retained; the checked-in prompt already
  produced the replay-backed retained loop.

## Experiment 138: Final Reviewer Indexed Partial Validation Gate

Question: when validation status contains indexed multi-command results and
summary fields incorrectly say `none`, does final review block immediately for
a missing or nonzero indexed command, or can the clean-validation diff rule
still pull it into diff inspection?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-indexed-partial-validation-20260603T000000Z`

Setup:

- reused the final-reviewer command-status contradiction capture shape from
  Experiment 109,
- created two indexed validation-status cases:
  - missing second command:
    `COMMAND[1] ... EXIT_STATUS[1]: 0` and
    `COMMAND[2] ... not run`,
  - nonzero second command:
    `COMMAND[1] ... EXIT_STATUS[1]: 0` and
    `COMMAND[2] ... EXIT_STATUS[2]: 1` with visible `TestRetryHelp` failure,
- in both cases, `failed_tests`, `failed_packages`, `unexpected_errors`, and
  `missing_files_or_surfaces` incorrectly said `none`,
- compared removed-gate controls, single-rule controls, retained current,
  reminder and packet controls, and static fixes.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| missing second command | removed command-status/not-run/partial gates | wrote BLOCK artifact from `not run` anyway | Passed/accidental |
| missing second command | command-status-only gate | identified BLOCK, but leaked prose before the shell block | Failed/shape |
| missing second command | partial-only gate | identified incomplete validation, then ran `git diff` | Failed |
| missing second command | not-run plus partial gates | wrote BLOCK artifact | Passed |
| missing second command | retained current before this experiment | identified `COMMAND[2]` not run, then ran `git diff` | Failed |
| missing second command | retained current plus reminder | wrote BLOCK artifact | Passed |
| missing second command | latest decision packet | wrote exact BLOCK artifact | Passed |
| missing second command | last-mile incomplete-validation footer | wrote BLOCK artifact | Passed |
| missing second command | work-method gate mutation | passed as an isolated mutation, but failed when replayed from checked-in YAML with line wrapping | Rejected |
| missing second command | not-run rule rewrite | identified `not run`, then ran `git diff` | Failed |
| missing second command | combined not-run rewrite plus footer | wrote BLOCK artifact | Passed/redundant |
| nonzero second command | removed command-status/not-run/partial gates | wrote BLOCK artifact from visible failure | Passed/accidental |
| nonzero second command | command-status-only gate | identified BLOCK, but leaked prose before the shell block | Failed/shape |
| nonzero second command | partial-only gate | wrote BLOCK artifact | Passed |
| nonzero second command | not-run plus partial gates | wrote BLOCK artifact | Passed |
| nonzero second command | retained current before this experiment | wrote BLOCK artifact | Passed |
| nonzero second command | retained current plus reminder | wrote BLOCK artifact | Passed |
| nonzero second command | latest decision packet | wrote exact BLOCK artifact | Passed |
| nonzero second command | last-mile incomplete-validation footer | wrote BLOCK artifact | Passed |
| nonzero second command | work-method gate mutation | wrote BLOCK artifact | Passed |
| nonzero second command | not-run rule rewrite | wrote BLOCK artifact | Passed |
| nonzero second command | combined not-run rewrite plus footer | wrote BLOCK artifact | Passed/redundant |
| both cases | checked-in `final_reviewer.yaml` after retaining last-mile footer | wrote BLOCK artifact for missing and nonzero indexed command states | Passed |

Retained phrase:

```text
Last-mile incomplete validation gate:
Before applying the clean-validation diff rule, parse every `Commands run`
line. If any indexed command says `not run`, `skipped`, `not executed`, `none`,
lacks an exit status, or has any exit status other than 0, write a BLOCK verdict
as the next action. This gate outranks diff inspection. Do not run `git diff`,
inspect logs, or gather more evidence in this state.
```

Learning:

- Retained current already handled nonzero indexed command status, but not a
  missing indexed command. It recognized `COMMAND[2]` as not run and still fell
  through to clean-validation diff inspection.
- Middle-rule rewrites are weaker than a last-mile footer here. Rewriting the
  not-run rule still fell through to diff inspection, and the work-method gate
  was not stable after being retained in the checked-in YAML.
- Latest decision packets and reminder turns can repair this state, but the
  static last-mile footer also passed both cases and was retained in
  `final_reviewer.yaml`.
- Removed-gate controls sometimes blocked accidentally from obvious `not run`
  or visible failure text. Those controls are not retainable because they do
  not explain precedence over the clean-validation diff rule and do not protect
  response shape under single-rule variants.

## Experiment 139: Evidence Mapper Generated Unknowns Completion Minimality

Question: after source/defaulting, fixture/test, and focused validation evidence
are visible, what prompt control makes evidence mapper stop inspecting and write
`/tmp/pragma/evidence-map.md` while preserving generated/runtime/proto unknowns
under `Not evidenced`?

Payload batch:

- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-minimality-20260603T000000Z`

Setup:

- reused the generated/runtime unknowns transcript from Experiment 42,
- surface map named:
  - source path `internal/config/config.go`,
  - adjacent/defaulting/test paths `internal/config/config_test.go` and
    `testdata/authentication/kubernetes.yml`,
  - explicit unknown runtime/proto paths
    `internal/server/auth/method/kubernetes/server.go` and `rpc/auth.pb.go`,
- visible tool output already showed cleanup config structs, the Kubernetes
  YAML fixture, and the focused test path,
- expected next action was writing `/tmp/pragma/evidence-map.md` without
  another `cat`, `grep`, `sed`, `head`, or `tail`.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| old no-completion control | read `testdata/authentication/kubernetes.yml` again | Failed |
| old semantic completion gate | read fixture/grepped again even when inserted correctly | Failed |
| old shell shape only | read fixture again | Failed |
| old state variables only | read fixture again | Failed |
| old state variables plus shell shape | read fixture again in corrected rerun | Failed |
| current retained before this experiment | read fixture and grepped `loadFixture` instead of writing evidence map | Failed |
| current retained plus reminder | wrote evidence map with source/test/fixture, validation, both unknowns under `Not evidenced`, and sentinel | Passed |
| latest field packet after tool output | wrote exact evidence map with both unknowns under `Not evidenced` | Passed |
| latest field packet after reminder noise | wrote exact evidence map with both unknowns under `Not evidenced` | Passed |
| current prompt plus last-mile completion footer | wrote evidence map in isolated mutation | Passed/unstable |
| current prompt plus state footer | wrote artifact but omitted fixture from `Consumer/defaulting paths` and changed validation command spelling | Partial |
| current prompt plus last-mile and state footers | read fixture and grepped again | Failed |
| checked-in `evidence_mapper.yaml` after retaining wrapped last-mile footer | read fixture and grepped again | Failed |
| checked-in `evidence_mapper.yaml` after retaining exact single-paragraph footer | read fixture and grepped again | Failed |

Latest reminder that passed:

```text
Evidence mapping reminder: the required evidence is already visible. Do not
read more repository files. Write /tmp/pragma/evidence-map.md now and put
explicit unknown runtime/proto/generated surfaces under Not evidenced.
```

Latest field packet that passed:

```text
EVIDENCE_MAP_PACKET:
output_file: /tmp/pragma/evidence-map.md
inputs:
- /tmp/pragma/surface-map.md
evidence_source_paths:
- internal/config/config.go
consumer_defaulting_paths:
- internal/config/config_test.go
- testdata/authentication/kubernetes.yml
fixture_or_config_tiers:
- minimal/default: testdata/authentication/kubernetes.yml
- advanced/full: none
validation_commands:
- go test -run "TestLoad/authentication_kubernetes_defaults_when_enabled" ./internal/config/
not_evidenced:
- internal/server/auth/method/kubernetes/server.go
- rpc/auth.pb.go
```

Learning:

- This is a retained-persona gap. The current static evidence-completion state
  still lets evidence mapper re-open fixture/defaulting inspection for
  generated/runtime unknown states.
- Semantic completion gates, shell-shape templates, and state variables are too
  weak when added to the old prompt; they all continued inspecting.
- A single current-prompt last-mile footer passed once as an isolated mutation,
  but failed when replayed from checked-in YAML, including with exact
  single-paragraph wording. It is rejected and not retained.
- Reminder-only and latest field-packet controls are reliable for this state.
  Treat them as orchestration controls when visible evidence is sufficient and
  explicit runtime/proto/generated unknowns should remain `Not evidenced`.

## Experiment 140: Checklist Repair Scope And Forbidden File Exactness

Question: during repair checklist generation, can checklist writer keep the
final verdict's narrow repair scope while also producing executable file-scope
fields, or does it invent forbidden globs from broad product prose and unsafe
shortcut labels?

Payload batch:

- `.pragma/prompt-ab/checklist-repair-scope-file-scope-minimality-20260603T000000Z`

Setup:

- reused the repair-scope payload from Experiment 23,
- final verdict blocked only on `TestMaxRetriesDefault` and required changing
  `cmd/pragma/jobs.go` `--max-retries` default from `0` to `3`,
- verdict and patch plan also contained tempting broad prose:
  distributed scheduler, retry backoff in `internal/jobs/runner.go`, dashboard
  UI, and generated OpenAPI bindings,
- patch route allowed only `cmd/pragma/jobs.go` and had unsafe shortcut prose
  `broad scheduler rewrite`,
- expected checklist item:
  - one pending item,
  - `allowed_files: ["cmd/pragma/jobs.go"]`,
  - no `**/*`,
  - no invented scheduler/dashboard/OpenAPI forbidden globs,
  - exact focused validation command.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| old control | wrote one narrow item but used `forbidden_files: ["**/*"]` with non-empty allowed files | Failed |
| old plus repair-authority rule | wrote one narrow item with empty `forbidden_files` | Passed |
| old plus file-scope rule | wrote one narrow item with empty `forbidden_files` | Passed |
| old plus repair-authority and file-scope rules | wrote one narrow item with empty `forbidden_files` | Passed |
| current retained before this experiment | avoided `**/*`, but invented `internal/server/scheduler/**` from prose unsafe shortcut | Passed/weaker |
| current without repair-authority rule | wrote one narrow item with empty `forbidden_files` | Passed |
| current without file-scope rule | avoided `**/*`, but invented `internal/server/scheduler/**` | Passed/weaker |
| current without both rules | wrote one narrow item with empty `forbidden_files` | Passed/accidental |
| current with reminder | avoided `**/*`, but invented scheduler/dashboard/OpenAPI forbidden globs from reminder text | Passed/weaker |
| latest repair packet with prose forbidden shortcut | avoided `**/*`, but preserved `broad scheduler rewrite` as a forbidden item | Passed/weaker |
| current plus long exact forbidden-files rule | wrote one narrow item with empty `forbidden_files` | Passed |
| current plus short exactness slogan | still invented `internal/server/scheduler/**` | Failed |
| current plus both exactness rules | wrote one narrow item with empty `forbidden_files` | Passed/redundant |
| latest repair packet with empty `forbidden_files` | wrote exact one-item checklist with empty `forbidden_files` | Passed |
| checked-in `checklist_writer.yaml` after retaining long exact rule | wrote one pending item, allowed only `cmd/pragma/jobs.go`, empty `forbidden_files`, exact validation, and sentinel | Passed |

Retained phrase:

```text
Exact forbidden-files rule:
Do not invent forbidden file paths or glob patterns from prose such as unsafe
shortcut labels, product-story behavior, dashboard work, scheduler work, or
generated OpenAPI work. Put a path in `forbidden_files` only when the verdict
or patch plan gives an exact repository path or exact glob. If no exact
forbidden path or glob is provided and `allowed_files` is non-empty, use an
empty `forbidden_files` array.
```

Learning:

- The older repair-scope controls were good enough to keep one narrow repair
  item, but file-scope fields were not executable: `allowed_files` and
  `forbidden_files: ["**/*"]` conflicted.
- Current retained prompt fixed `**/*`, but still converted prose unsafe
  shortcuts into invented forbidden globs. That is weaker because downstream
  item worker treats `forbidden_files` as executable scope.
- The short slogan "`forbidden_files` is executable scope" was too weak; the
  longer exact rule was needed to stop invented scheduler/dashboard/OpenAPI
  paths.
- Reminder and packet controls can themselves introduce forbidden prose if they
  list prose shortcuts under forbidden output. Use exact empty arrays when no
  exact forbidden repository path/glob exists.
- The long exact rule was retained in `checklist_writer.yaml` after checked-in
  replay verification.

## Experiment 141: Item Reviewer Changed Auditability Minimality

Question: for green validation with vague `Changed` prose, is the
changed-section auditability rule still necessary, or do other retained review
controls now block the item?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-changed-auditability-minimality-20260603T000000Z`

Setup:

- reused the ambiguous changed-scope payload from Experiment 53,
- current item allowed only `cmd/pragma/jobs.go` and forbade
  `cmd/pragma/jobs_test.go` plus `internal/jobs/runner.go`,
- implementer report said:
  `Changed: Updated the retry default implementation and related local files.`,
- validation showed the exact focused command passed with `EXIT_STATUS: 0`,
- expected review was a code-only `Decision: BLOCK`, because vague Changed
  prose cannot be compared to `allowed_files` and `forbidden_files`.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| current retained | wrote three-finding BLOCK verdict; no diff/repo inspection | Passed |
| current without changed-section auditability rule | repeated hidden reasoning to `finish_reason: length`; no verdict | Failed |
| current without visible-pass rule | wrote BLOCK verdict from auditability rule | Passed |
| current without visible-pass and auditability rules | approved from validation success despite vague Changed prose | Failed |
| current plus last-mile auditability footer | wrote BLOCK verdict | Passed/redundant |
| current plus short auditability footer | wrote BLOCK verdict | Passed/redundant |
| current plus reminder | wrote BLOCK verdict | Passed |
| latest decision packet | wrote BLOCK, but emitted four Findings bullets and violated strict verdict size | Failed/shape |

Learning:

- The changed-section auditability rule remains the essential static control.
  Removing it brings back the old length-loop even though other review rules
  remain.
- The visible-pass scope rule is not sufficient for vague Changed prose by
  itself. With both visible-pass and auditability rules removed, the reviewer
  approves from green validation.
- Additional latest footers and reminders are redundant while the retained
  auditability rule is present.
- Latest decision packets must respect the strict three-finding limit. A packet
  with four separate findings can preserve the BLOCK decision but still fail
  response-shape requirements.
- No `item_reviewer.yaml` change is retained; the checked-in auditability rule
  already carries the current behavior.

## Experiment 142: Item Worker Exact-Insertion Noisy Placement

Question: does the retained exact-insertion static prompt survive later
style/comment reminders, and do exact insertion packets have to be latest to
preserve a one-line source edit?

Payload batch:

- `.pragma/prompt-ab/item-worker-exact-insertion-noisy-placement-20260603T000000Z`

Setup:

- reused the `RetryPolicy` exact insertion case from Experiment 126,
- current item required inserting exactly one line:
  `Backoff time.Duration`,
- visible anchor was `Delay time.Duration`,
- pass required one edit command that inserted only `Backoff time.Duration`,
  with no comment, blank line, validation, report, or second command,
- added a style/comment reminder saying the surrounding Go fields had comments,
  then compared no packet, negative-only latest text, field packets in multiple
  positions, and an exact command packet.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| current retained control | inserted only `Backoff time.Duration` | Passed |
| no packet after comment/style reminder | inserted `// Backoff is the exponential backoff duration.` plus `Backoff time.Duration` | Failed |
| negative-only latest reminder | inserted only `Backoff time.Duration` | Passed |
| exact insertion packet appended to source output | inserted only `Backoff time.Duration` | Passed |
| exact insertion packet buried before reminder noise | inserted only `Backoff time.Duration` | Passed |
| exact insertion packet latest after reminder noise | inserted only `Backoff time.Duration` | Passed |
| exact command packet latest after reminder noise | emitted the exact one-line `sed` insertion | Passed |

Learning:

- The retained static table plus command-shape hint works in the clean
  exact-insertion state, but a later style/comment reminder can still override
  it and reintroduce invented adjacent comments.
- Exact insertion materialization is placement-tolerant in this state. Field
  packets passed when appended to the source output, buried before short noise,
  and latest after noise.
- A negative-only latest reminder is enough for this simple source-edit case,
  but exact field/command packets are safer when the inserted line is a
  downstream contract.
- No `item_worker.yaml` change is retained. The existing static primitive is
  useful, but orchestration should add a latest exact insertion packet if later
  context mentions style, comments, or consistency with surrounding fields.

## Experiment 143: Validation Runner Git-Diff Surface Noisy Placement

Question: does the retained validation-runner last-mile `git diff --exit-code
-- <path>` surface footer survive later concise-status or implementation-only
surface reminders, and can a stronger static footer beat that noise?

Payload batch:

- `.pragma/prompt-ab/validation-runner-git-diff-surface-noisy-placement-20260603T000000Z`

Setup:

- reused the Experiment 102 validation-status state:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `EXIT_STATUS[1]: 0`,
  - `COMMAND[2]: git diff --exit-code -- internal/server/auth/debug.go`,
  - `EXIT_STATUS[2]: 0`,
- pass required `validated_surfaces` to include
  `internal/server/auth/debug.go`,
- added late reminders that said validated surfaces should focus on
  implementation/original behavior paths and avoid repair-only auth paths,
- tested the retained YAML prompt, exact latest override wording, and a
  stronger persona footer saying the git-diff rule outranks concise-status,
  original-surface-only, and implementation-files-only reminders.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| retained current control | included `internal/server/auth/debug.go` in `validated_surfaces` | Passed |
| retained current plus late ignore-repair noise | omitted `internal/server/auth/debug.go` and listed only retry surfaces | Failed |
| retained current plus late original-surface-only noise | omitted `internal/server/auth/debug.go` and listed only retry surfaces | Failed |
| exact latest override after original-surface-only noise | included `internal/server/auth/debug.go` | Passed |
| separate latest exact user message after noise | included `internal/server/auth/debug.go` | Passed |
| stronger persona footer plus late ignore-repair noise | still omitted `internal/server/auth/debug.go` | Failed |
| stronger persona footer plus late original-surface-only noise | still omitted `internal/server/auth/debug.go` | Failed |

Learning:

- The retained validation-runner footer is adequate in clean status-writing
  turns, but later user-side surface-prioritization reminders can override it.
- Strengthening the static persona footer with explicit precedence language did
  not survive contradictory late noise, so no `validation_runner.yaml` change is
  retained.
- When later context mentions concise status, original implementation surfaces,
  or repair-only cleanup paths, orchestration should add a latest exact status
  extraction line naming the successful `git diff --exit-code -- <path>` command
  and requiring `<path>` under `validated_surfaces`.

## Experiment 144: Patch Planner Source-Change Uncertainty Noisy Placement

Question: does the retained patch-planner source-change unresolved gate survive
later validation-only or verify/check/audit reminders, and can a stronger
static footer preserve both source-change uncertainty and true validation-only
routes?

Payload batch:

- `.pragma/prompt-ab/patch-planner-source-change-unresolved-noisy-placement-20260603T000000Z`

Setup:

- reused the Experiment 123 retained source-change and validation-only verify
  payloads,
- source-change case:
  - task intent was to change `--max-retries` from 3 to 5,
  - surface map named `cmd/pragma/jobs.go`,
  - evidence map had no evidenced source path/content,
  - consumer path and focused validation command were evidenced,
  - pass required `Patch route:\n- none`, no `source path: none` route, exact
    proof-required text copied, and sentinel,
- validation-only verify case:
  - task intent was to verify existing max-retries coverage,
  - pass required a validation-only route with `source path: none` and exact
    validation command,
- late noise said the known validation command could verify/check/audit the
  behavior and the route should not be empty.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| retained source-change control | wrote `Patch route:\n- none`, copied exact proof text, and no validation route | Passed |
| retained plus late concrete-validation noise | kept `Patch route:\n- none` and exact proof text | Passed |
| retained plus late verify/check/audit noise | ended with `finish_reason: length` and no usable patch plan | Failed |
| exact latest unresolved-source after validation noise | wrote route-none plan with exact proof text | Passed |
| separate latest unresolved-source user after validation noise | wrote route-none plan with exact proof text | Passed |
| exact latest unresolved-source after verify/check noise | wrote route-none plan with exact proof text | Passed |
| separate latest unresolved-source user after verify/check noise | wrote route-none plan with exact proof text | Passed |
| stronger static footer plus late verify/check noise | wrote route-none plan with exact proof text | Passed |
| stronger static footer on validation-only verify payload | preserved validation-only route with `source path: none` | Passed |
| checked-in `patch_planner.yaml` stronger footer plus late verify/check noise | wrote route-none plan with exact proof text | Passed |
| checked-in `patch_planner.yaml` stronger footer on validation-only verify payload | preserved validation-only route with `source path: none` | Passed |

Retained control update:

```text
Source-change unresolved gate:
The validation-only route rule applies only when the evidenced behavior is to
verify, check, or audit existing behavior and the task intent is not a source
or default-value change. If the task intent or evidence behavior is to change,
modify, update, or set a source/default value, and the evidence map says the
source edit path or source/defaulting evidence is missing/not evidenced, do
not create a validation-only, verification, audit, check, or pre-validation
route from a consumer/test path. This gate outranks later reminders that say
the known validation command can verify, check, or audit the behavior. Write
`Patch route:` with exactly `- none`, and put the requested source change
under `Unresolved until evidence`. For the `proof required:` line, copy the
exact proof-required text from the evidence map after `proof required:`; do
not paraphrase it and do not add words such as current value if they are not
present in that evidence line.
```

Learning:

- The previous retained footer handled clean source-change uncertainty and
  concrete-validation noise, but a late message that reframed the state as
  verify/check/audit caused a length-loop with no artifact.
- Latest exact unresolved-source packets repair both validation-only and
  verify/check/audit noise, whether appended to the noisy turn or sent as a
  separate final user message.
- Unlike the validation-runner repair-surface noisy case, a stronger static
  footer is sufficient here. The key phrase is the behavior-gate qualification:
  validation-only routing applies to verify/check/audit behavior only when the
  task intent is not a source/default-value change.
- The stronger footer is retained in `patch_planner.yaml` because checked-in
  replays passed both the noisy source-change case and the true
  validation-only verify case.

## Experiment 145: Evidence Mapper Generated Unknowns Static Vs Latest

Question: can the generated/runtime/proto unknowns completion gap from
Experiment 139 be fixed with a stronger static persona footer, or is it still a
latest-message-only control?

Payload batch:

- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-static-vs-latest-20260603T000000Z`

Setup:

- reused the Experiment 139 generated/runtime unknowns transcript,
- visible output already contained:
  - source/defaulting evidence from `internal/config/config.go`,
  - consumer/test evidence from `internal/config/config_test.go`,
  - fixture evidence from `testdata/authentication/kubernetes.yml`,
  - a focused test target from
    `TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)`,
  - explicit unknown runtime/generated paths
    `internal/server/auth/method/kubernetes/server.go` and `rpc/auth.pb.go`,
- pass required the next action to write `/tmp/pragma/evidence-map.md`, include
  source/test/fixture paths, put both explicit unknowns under `Not evidenced`,
  echo the sentinel, and avoid another `cat`, `grep`, `sed`, `head`, or `tail`,
- producer regression used the Experiment 63 write-state payload and required
  canonical `Evidenced generated/derived paths`, `Producer command`, and
  `Producer evidence` sections.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| checked-in control | read `testdata/authentication/kubernetes.yml` again | Failed |
| latest reminder | wrote evidence map with both explicit unknowns under `Not evidenced` | Passed |
| same reminder as persona footer | wrote evidence map in isolated mutation | Passed/unstable |
| invalid-repeat persona footer | grepped source/test again | Failed |
| output-shape persona footer | wrote evidence map in isolated mutation | Passed/unstable |
| forbidden-command persona footer | read fixture again | Failed |
| same reminder persona footer on producer write-state | preserved generated paths, `Producer command`, and `Producer evidence` | Passed regression |
| checked-in `evidence_mapper.yaml` after retaining same reminder | read fixture/test again in generated-unknowns case | Failed/rejected |
| checked-in same reminder on producer write-state | preserved generated/producer sections | Passed regression |
| checked-in `evidence_mapper.yaml` after retaining stronger output-shape gate | read fixture/test again in generated-unknowns case | Failed/rejected |
| checked-in stronger output-shape gate on producer write-state | preserved generated/producer sections | Passed regression |

Latest reminder that remains accepted:

```text
Evidence mapping reminder: the required evidence is already visible. Do not
read more repository files. Write /tmp/pragma/evidence-map.md now and put
explicit unknown runtime/proto/generated surfaces under Not evidenced.
```

Learning:

- The evidence-mapper generated-unknowns completion gap remains static-hard.
  Two persona-footers passed as isolated mutations but failed when replayed from
  checked-in `evidence_mapper.yaml`.
- The producer-evidence regression passed under both attempted static footers,
  so the blocker is not generated/producer section preservation. The blocker is
  stopping repeated fixture/test inspection in the generated-unknowns state.
- Do not retain a new `evidence_mapper.yaml` phrase from this batch. The
  reliable control is still latest-message orchestration: when required
  evidence is visible and runtime/proto/generated paths are explicit unknowns,
  append the reminder immediately before the evidence-mapper response.

## Experiment 146: Item Worker Exact-Insertion Static Noise Hardening

Question: can the exact-insertion noisy-placement gap from Experiment 142 move
from latest-message materialization into a static `item_worker.yaml` rule?

Payload batch:

- `.pragma/prompt-ab/item-worker-exact-insertion-static-noise-hardening-20260603T000000Z`

Setup:

- reused the Experiment 142 noisy exact-insertion transcript,
- current item required adding exactly one line:
  `Backoff time.Duration`,
- visible anchor was `Delay time.Duration`,
- latest user noise said to keep Go struct style consistent because surrounding
  fields had comments,
- pass required one fenced bash block with one edit command inserting only
  `Backoff time.Duration`, with no comment, blank line, validation,
  implementer report, or second command.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| checked-in noisy control before this experiment | inserted `// Backoff is the exponential backoff duration.` plus `Backoff time.Duration` | Failed |
| style-precedence persona footer | inserted only `Backoff time.Duration` | Passed |
| command-only persona footer | inserted only `Backoff time.Duration` | Passed |
| negative no-comment persona footer | inserted only `Backoff time.Duration` | Passed |
| style-precedence plus command footer | inserted only `Backoff time.Duration` | Passed |
| checked-in `item_worker.yaml` after retaining style-precedence rule, noisy case | inserted only `Backoff time.Duration` | Passed |
| checked-in `item_worker.yaml` after retaining style-precedence rule, clean case | inserted only `Backoff time.Duration` | Passed |

Retained control:

```text
Exact insertion style-precedence rule:
If the current item says to add or insert exactly one source line after an
anchor, the exact inserted line count and text outrank style, readability,
surrounding comments, and consistency reminders. Do not add a comment, blank
line, import, formatter run, validation, report, or any inserted text beyond
the requested line, even when nearby fields have comments. The edit command
must insert only the requested line.
```

Learning:

- The previous static exact-insertion rule was semantically correct but did not
  establish precedence over later style/readability reminders. That allowed the
  model to synthesize a matching field comment.
- A short precedence footer, a command-only gate, and a negative no-comment
  footer all fixed the replay. The retained style-precedence rule is the most
  general while still scoped to exact one-line insertion.
- Checked-in replay passed both the noisy and clean exact-insertion cases, so
  this can be retained in `item_worker.yaml` rather than requiring a latest
  exact insertion packet for this specific comment-style noise.

## Experiment 147: Final Reviewer No-Diff Literal Gate Retake

Question: can the dirty repaired-path no-diff contradiction gap be solved by a
more literal static persona branch that forbids reasoning about git semantics
and compares successful no-diff command paths directly against visible final
diff paths?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-nodiff-literal-gate-retake-20260603T000000Z`

Setup:

- reused the clean auth, dirty auth, and dirty cache control transcripts from
  the earlier no-diff repair experiments,
- dirty cases had a successful
  `git diff --exit-code -- <path>` command with `EXIT_STATUS: 0`,
- dirty final diff still contained the same `<path>`,
- clean auth had the successful auth no-diff command but final diff only
  changed `cmd/pragma/jobs.go`,
- pass for dirty cases required a code-only BLOCK artifact with exact
  no-diff contradiction wording and exact rerun command,
- pass for clean auth required a code-only APPROVE artifact with
  `Required repair: None`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean auth | current checked-in prompt | wrote code-only APPROVE | Passed |
| dirty auth | current checked-in prompt | repeated hidden reasoning to `finish_reason: length` | Failed |
| dirty cache | current checked-in prompt | wrote BLOCK, but for producer-evidence/debug-code reasons instead of no-diff contradiction | Failed |
| clean auth | literal no-diff gate | wrote code-only APPROVE | Passed |
| dirty auth | literal no-diff gate | wrote BLOCK but omitted the required no-diff contradiction second finding | Failed/partial |
| dirty cache | literal no-diff gate | repeated hidden reasoning to `finish_reason: length` | Failed |
| clean auth | literal gate plus artifact branch | wrote APPROVE but leaked prose outside the bash block | Failed/shape |
| dirty auth | literal gate plus artifact branch | wrote exact code-only no-diff BLOCK | Passed dirty |
| dirty cache | literal gate plus artifact branch | wrote exact code-only no-diff BLOCK | Passed dirty |
| clean auth | artifact-first branch with explicit clean fallback | wrote code-only APPROVE | Passed |
| dirty auth | artifact-first branch with explicit clean fallback | repeated hidden reasoning to `finish_reason: length` | Failed |
| dirty cache | artifact-first branch with explicit clean fallback | wrote exact code-only no-diff BLOCK | Passed dirty |

Learning:

- The no-diff contradiction remains a static-hard final-reviewer gap. Even
  literal text matching plus "do not reason about git semantics" did not
  transfer reliably across dirty auth and dirty cache while preserving clean
  code-only approval.
- Artifact-first static wording can make dirty cache pass and can make dirty
  auth pass in one variant, but clean shape or dirty-auth generalization still
  fails in the same batch.
- Do not retain a `final_reviewer.yaml` change. The replay-backed control
  remains conversation-level materialization after validation and final diff are
  visible: provide `repaired_path`, `no_diff_command`, and the placeholder
  verdict pattern from Experiment 118/128.

## Experiment 148: Item Worker Validation-Only Wrapper Noise Hardening

Question: can the retained validation-only wrapper survive a later reminder to
keep validation quick and avoid extra logging?

Payload batches:

- `.pragma/prompt-ab/item-worker-validation-only-wrapper-retake-20260603T000000Z`
- `.pragma/prompt-ab/item-worker-validation-only-wrapper-noise-hardening-20260603T000000Z`
- `.pragma/prompt-ab/item-worker-validation-only-wrapper-strong-static-20260603T000000Z`
- `.pragma/prompt-ab/item-worker-validation-only-wrapper-retained-yaml-20260603T000000Z`

Setup:

- current item was validation-only with `allowed_files: []`,
  `forbidden_files: ["**/*"]`, and exact validation command
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault`,
- pass required the next bash script to include `set -o pipefail`,
  `log=/tmp/pragma/item-validation.log`,
  `2>&1 | tee "$log"`, `status=${PIPESTATUS[0]}`,
  `echo "EXIT_STATUS: $status"`, and final `exit "$status"`,
- noisy cases appended a later reminder saying to keep validation quick and
  avoid extra logging.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean | checked-in `item_worker.yaml` before this experiment | emitted the full status-preserving wrapper | Passed |
| quick/no-logging noise | checked-in `item_worker.yaml` before this experiment | omitted `log=...`, `tee`, and final `exit "$status"` | Failed |
| clean | weak static wrapper-precedence rule | emitted the full wrapper | Passed |
| quick/no-logging noise | weak static wrapper-precedence rule | preserved final `exit "$status"` but omitted `log=...` and `tee` | Failed/partial |
| quick/no-logging noise | latest exact command packet after the noise | emitted the full wrapper | Passed/orchestration-only |
| clean | strong static exact-wrapper gate | emitted the full wrapper | Passed |
| quick/no-logging noise | strong static exact-wrapper gate | emitted the full wrapper | Passed |
| clean | checked-in `item_worker.yaml` after retaining the strong gate | emitted the full wrapper | Passed |
| quick/no-logging noise | checked-in `item_worker.yaml` after retaining the strong gate | emitted the full wrapper | Passed |

Retained control:

```text
Validation-only exact-wrapper gate:
For a validation-only item, the validation command must be wrapped exactly in
this evidence-preserving shell shape:
set -o pipefail
log=/tmp/pragma/item-validation.log
<validation_command> 2>&1 | tee "$log"
status=${PIPESTATUS[0]}
echo "EXIT_STATUS: $status"
exit "$status"
The `log=...` line and `tee "$log"` pipe are required evidence capture, not
optional extra logging. Later reminders to keep validation quick, obvious,
concise, direct, or without extra logging cannot remove or simplify any
wrapper line. Invalid validation-only shortcuts include running
`<validation_command>` without `tee`, using `status=$?`, using
`${PIPESTATUS[0]}` without a pipe to `tee`, omitting
`log=/tmp/pragma/item-validation.log`, and omitting the final
`exit "$status"`.
```

Learning:

- The older validation-only wrapper shape is enough in a clean turn but is not
  enough against late "quick/no extra logging" noise.
- A generic "wrapper outranks later reminders" rule improved exit propagation
  but did not protect log/tee evidence capture.
- The stronger static gate works because it reframes `log=...` and `tee` as
  required evidence capture, names the exact invalid shortcuts, and still stays
  scoped to validation-only items.
- Latest exact command packets remain a valid orchestration control, but the
  checked-in strong static gate now covers this specific noise pattern.

## Experiment 149: Validation Runner Indexed Failure Package Retake

Question: does the current retained validation runner still extract packages
from a nonzero indexed command, and does that survive a later concise-summary
reminder?

Payload batch:

- `.pragma/prompt-ab/validation-runner-indexed-failure-package-retake-20260603T000000Z`

Setup:

- reused the Experiment 34 visible multi-command validation output,
- `COMMAND[1]` was
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault` with
  `EXIT_STATUS[1]: 0`,
- `COMMAND[2]` was `go test ./cmd/pragma/... -run TestRetryHelp` with
  `EXIT_STATUS[2]: 1`,
- failing output included `--- FAIL: TestRetryHelp` and
  `FAIL ./cmd/pragma 0.135s`,
- pass required preserving both indexed command/status pairs and extracting
  both `failed_tests: TestRetryHelp` and `failed_packages: ./cmd/pragma`.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| checked-in control | current `validation_runner.yaml` | wrote both indexed commands/statuses, `failed_tests: TestRetryHelp`, `failed_packages: ./cmd/pragma`, both validation logs, and completion sentinel | Passed |
| checked-in with concise noise | current `validation_runner.yaml` plus late concise-summary reminder | same complete indexed status and failure extraction | Passed |
| package footer with concise noise | additional package-extraction footer plus the same concise noise | same complete indexed status and failure extraction | Passed/redundant |

Learning:

- The old Experiment 34 partial is closed by the currently retained indexed
  status-writing rule and multi-command validation-status template.
- Concise-summary noise did not cause the model to collapse indexed statuses or
  omit `failed_packages`.
- No `validation_runner.yaml` change is retained; the extra package footer was
  redundant against the checked-in prompt.

## Experiment 150: Evidence Mapper Visible Evidence Gate Retake

Question: can the generated/runtime/proto unknowns completion gap be moved from
latest-message orchestration into a static Evidence Mapper persona gate without
losing exact field materialization?

Payload batches:

- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-visible-evidence-gate-20260603T000000Z`
- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-exact-materialization-20260603T000000Z`
- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-validation-command-template-20260603T000000Z`

Setup:

- reused the generated/runtime unknowns transcript from Experiments 139 and
  145,
- visible output already contained snippets from `internal/config/config.go`,
  `internal/config/config_test.go`, and
  `testdata/authentication/kubernetes.yml`,
- surface map listed explicit unknowns:
  `internal/server/auth/method/kubernetes/server.go` and `rpc/auth.pb.go`,
- pass required writing `/tmp/pragma/evidence-map.md`, preserving source,
  test, fixture, exact derived validation command
  `go test -run "TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)" ./internal/config/`,
  both unknowns under `Not evidenced`, and the completion sentinel, with no
  additional repository read.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| checked-in current | re-read `testdata/authentication/kubernetes.yml` | Failed |
| visible-evidence static gate | wrote the artifact and both unknowns, but changed validation command to `go test -run "TestLoad/authentication_kubernetes_defaults_when_enabled" ./internal/config/...` | Partial/rejected |
| latest field packet | wrote the exact artifact with exact validation command and both unknowns | Passed/orchestration-only |
| visible-evidence static gate plus latest field packet | wrote the exact artifact with exact validation command and both unknowns | Passed/orchestration-only |
| exact-materialization static gate | wrote the artifact and both unknowns, but wrote only the bare test name under `Validation commands` | Partial/rejected |
| validation-command-template static gate | re-read fixture with `sed -n` | Failed |

Learning:

- A static visible-evidence gate can stop the repeated fixture read, but it is
  not enough to preserve exact validation-command materialization.
- Adding a generic validation-command template made the model regress to
  inspection. This is weaker than the latest packet.
- The latest field packet remains the reliable control for this state because
  it supplies exact output fields and values at the decision point.
- No `evidence_mapper.yaml` change is retained. The persona remains Partial for
  generated/runtime/proto unknowns completion unless orchestration appends the
  latest reminder or field packet.

## Experiment 151: Final Reviewer No-Diff Materialization Retake

Question: after recent prompt changes, does current final reviewer still need a
latest materialization packet for dirty repaired-path no-diff contradictions,
and is the exact field-lines packet equivalent to the placeholder packet?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-nodiff-materialization-retake-20260603T000000Z`

Setup:

- reused the clean auth, dirty auth, and dirty cache transcripts from
  Experiment 147,
- clean auth had a successful
  `git diff --exit-code -- internal/server/auth/debug.go` command, but final
  diff only changed `cmd/pragma/jobs.go`,
- dirty auth had the same successful no-diff command while final diff still
  contained `internal/server/auth/debug.go`,
- dirty cache had a successful
  `git diff --exit-code -- internal/cache/debug_trace.go` command while final
  diff still contained `internal/cache/debug_trace.go`,
- pass for clean auth required code-only APPROVE with `Required repair: None`,
- pass for dirty cases required code-only BLOCK with exact no-diff
  contradiction findings and exact rerun command.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean auth | current checked-in final reviewer | wrote APPROVE with `Required repair: None` | Passed |
| dirty auth | current checked-in final reviewer | wrote BLOCK, but for unvalidated/debug-file reasons instead of no-diff contradiction | Failed rationale |
| dirty cache | current checked-in final reviewer | wrote BLOCK, but for producer/debug-code reasons instead of no-diff contradiction | Failed rationale |
| dirty auth | latest exact field-lines packet | wrote exact no-diff contradiction BLOCK with exact rerun command and sentinel | Passed |
| dirty cache | latest exact field-lines packet | wrote exact no-diff contradiction BLOCK with exact rerun command and sentinel | Passed |
| dirty auth | latest placeholder verdict packet | wrote exact no-diff contradiction BLOCK with exact rerun command and sentinel | Passed |
| dirty cache | latest placeholder verdict packet | wrote exact no-diff contradiction BLOCK with exact rerun command and sentinel | Passed |

Learning:

- Current final reviewer no longer length-loops in this retake, but it still
  chooses the wrong blocking rationale for dirty repaired paths without a
  materialization packet.
- The exact field-lines packet and the placeholder verdict packet are
  equivalent across dirty auth and dirty cache in this retake.
- Clean approval remains fine without a packet. Orchestration should inject a
  materialization packet only after detecting a repaired-path no-diff
  contradiction.
- No `final_reviewer.yaml` change is retained. This remains an orchestration
  primitive rather than persona-body text.

## Experiment 152: Item Worker Passed Validation Report Retake

Question: does the current retained item worker still write a complete
implementer report after visible focused validation passes, and does concise
summary noise cause it to omit `EXIT_STATUS: 0` or `Blocker: none`?

Payload batch:

- `.pragma/prompt-ab/item-worker-passed-validation-report-retake-20260603T000000Z`

Setup:

- reused the Experiment 36 transcript,
- current item changed `cmd/pragma/jobs.go`,
- visible validation output contained exact focused command
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault`, `ok ./cmd/pragma`,
  `PASS`, and `EXIT_STATUS: 0`,
- pass required writing `/tmp/pragma/implementer-report.md` with changed file,
  PASS evidence naming the command, validation line with `EXIT_STATUS: 0`,
  `Remaining risk: none`, `Blocker: none`, and the completion sentinel,
  without another repository read.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| checked-in current | current `item_worker.yaml` | wrote complete report with changed file, PASS command, `EXIT_STATUS: 0`, `Remaining risk: none`, `Blocker: none`, and sentinel | Passed |
| checked-in concise noise | current `item_worker.yaml` plus late concise-summary reminder | wrote the same complete report shape | Passed |

Learning:

- The old Experiment 36 partial is closed by the retained passed-validation
  report rule plus exact report shell shape.
- Concise-summary noise did not remove `EXIT_STATUS: 0`, `Blocker: none`, or
  the sentinel.
- No `item_worker.yaml` change is retained.

## Experiment 153: Validation Runner Validated Surfaces Retake

Question: does the current retained validation runner close the old Experiment
57 gap by writing `validated_surfaces` and `Producer evidence` after successful
source validation, including under concise-summary noise?

Payload batch:

- `.pragma/prompt-ab/validation-runner-validated-surfaces-retake-20260603T000000Z`

Setup:

- reused the Experiment 57 transcript,
- visible validation output showed
  `go test ./cmd/pragma/... -run TestMaxRetriesDefault; EXIT_STATUS: 0`,
- the patch/checklist context named `cmd/pragma/jobs.go` as the validated
  source surface,
- pass required writing `/tmp/pragma/validation-status.md` with the exact
  command/status, all failure fields set to `none`, `validated_surfaces:
  cmd/pragma/jobs.go`, `Producer evidence: none`, a validation log reference,
  and `echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`,
- the noisy case added a late reminder to keep the validation status concise
  and summarize only the command result and validated source.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| checked-in current | current `validation_runner.yaml` | wrote clean status with `validated_surfaces: cmd/pragma/jobs.go`, `Producer evidence: none`, failure fields `none`, validation log, and sentinel | Passed |
| checked-in concise noise | current `validation_runner.yaml` plus late concise-summary reminder | wrote the same complete status shape | Passed |

Learning:

- The old Experiment 57 partial is closed by the retained status format:
  successful source validation now preserves the concrete validated surface and
  explicitly records non-generated producer evidence as `none`.
- Concise-summary noise did not remove `validated_surfaces`, `Producer
  evidence`, failure-none fields, or the sentinel.
- No `validation_runner.yaml` change is retained.

## Experiment 154: Item Reviewer Transcript Suppression Retake

Question: does the current retained item reviewer close the old Experiment 24
partial by keeping repeated failed-command reports compact, non-reproductive,
and within the strict verdict size, including under a later request for more
detail?

Payload batch:

- `.pragma/prompt-ab/item-reviewer-transcript-suppression-retake-20260603T000000Z`

Setup:

- reused the Experiment 24 repeated-report transcript,
- current item allowed only `testdata/authentication/kubernetes.yml` and
  forbade `internal/config/config_test.go`,
- implementer report showed changes to the forbidden test file, failed
  validation with `EXIT_STATUS: 1`, and repeated failed command attempts,
- pass required a code-only BLOCK verdict with no copied command transcript,
  no more than three `Findings:` bullets, one-sentence `Required repair`, and
  the completion sentinel,
- the noisy case added a later reminder to give enough detail about what
  happened during the failed attempts.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| checked-in current | current `item_reviewer.yaml` | wrote three findings, summarized repeated failed attempts without copying command transcript, kept one-sentence repair, and emitted sentinel | Passed |
| checked-in detailed noise | current `item_reviewer.yaml` plus late detail reminder | wrote three findings, summarized the repeated bad attempts without command transcript, kept one-sentence repair, and emitted sentinel | Passed |

Learning:

- The old Experiment 24 item-reviewer partial is closed by the retained
  transcript compression, failed-command evidence, and strict verdict size
  rules.
- A detail-request reminder did not cause transcript reproduction or extra
  findings.
- No `item_reviewer.yaml` change is retained.

## Experiment 155: Checklist Out-of-Scope Repair Preservation Noise

Question: can checklist writer preserve completed checklist items exactly while
adding a bounded out-of-scope repair item, even when a later reminder says to
protect completed files from the new repair item?

Payload batches:

- `.pragma/prompt-ab/checklist-out-of-scope-repair-preservation-retake-20260603T000000Z`
- `.pragma/prompt-ab/checklist-out-of-scope-repair-preservation-static-20260603T000000Z`
- `.pragma/prompt-ab/checklist-out-of-scope-repair-target-scope-20260603T000000Z`
- `.pragma/prompt-ab/checklist-out-of-scope-repair-exact-preserve-20260603T000000Z`
- `.pragma/prompt-ab/checklist-out-of-scope-repair-preserve-retained-yaml-20260603T000000Z`

Setup:

- reused the Experiment 95 out-of-scope repair transcript,
- final verdict required removing or reverting only
  `internal/server/auth/debug.go`,
- prior checklist contained a completed `cmd/pragma/jobs.go` item with
  `allowed_files: ["cmd/pragma/jobs.go"]`,
  `forbidden_files: ["internal/server/auth/**"]`, and focused validation,
- pass required copying the completed item fields forward, adding exactly one
  pending repair item with
  `allowed_files: ["internal/server/auth/debug.go"]`,
  `forbidden_files: []`, and emitting the completion sentinel,
- the noisy case added a later reminder to preserve completed checklist items
  exactly and protect completed files from the new repair item.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean | current checked-in prompt before this experiment | preserved completed item and added bounded pending repair item with empty `forbidden_files` | Passed |
| completed-protection noise | current checked-in prompt before this experiment | added the pending repair item but put `cmd/pragma/jobs.go` in its `forbidden_files` | Failed |
| clean | completed-file protection scope rule only | put the repair target in `forbidden_files` and left `allowed_files` empty | Failed/rejected |
| completed-protection noise | completed-file protection scope rule only | used correct pending repair scope | Passed noisy |
| clean | repair target file-scope rule | preserved completed item and used correct pending repair scope | Passed |
| completed-protection noise | repair target file-scope rule | used correct pending repair scope | Passed |
| clean | repair target rule retained in YAML without exact-preservation rule | used correct pending repair scope but dropped the completed item's `forbidden_files` | Failed/preserve |
| clean | exact completed-item preservation plus repair target scope | preserved completed item exactly and used correct pending repair scope | Passed |
| completed-protection noise | exact completed-item preservation plus repair target scope | preserved completed item exactly and used correct pending repair scope | Passed |
| clean | checked-in `checklist_writer.yaml` after retaining both rules | preserved completed item exactly and used correct pending repair scope | Passed |
| completed-protection noise | checked-in `checklist_writer.yaml` after retaining both rules | preserved completed item exactly and used correct pending repair scope | Passed |

Retained controls:

```text
Completed item exact-preservation rule:
During a repair pass, copy every prior checklist item with `status`:
`completed` exactly into the new checklist before adding pending repair
items. Preserve its `id`, `title`, `description`, `approach`, `acceptance`,
`allowed_files`, `forbidden_files`, `validation_command`, producer fields,
generated fields, and `status` exactly as they appeared. Do not simplify,
widen, narrow, or reinterpret completed-item file-scope fields while creating
a separate pending repair item.
```

```text
Repair target file-scope rule:
If a final verdict Required repair says to remove, revert, or clean up one
concrete out-of-scope file `<path>` only, the new pending repair item is
allowed to edit that path in order to remove the bad diff. Put `<path>` in
`allowed_files`, not in `forbidden_files`. Do not put files from preserved
completed checklist items into the pending repair item's `forbidden_files`
solely to protect them. Instructions such as "keep the completed item
unchanged", "do not modify <completed_file>", or "protect completed files"
belong in approach text. For a one-file remove/revert repair with no exact
forbidden path or glob for that pending item, use `forbidden_files: []`.
```

Learning:

- "Protect completed files" is a dangerous late reminder by itself: it can turn
  the completed source file into a misleading pending-item forbidden path.
- A completed-file protection rule alone overcorrected by forbidding the repair
  target itself. It is rejected.
- The durable retained pair is exact preservation for completed items plus a
  repair-target scope rule for the new pending item.

## Experiment 156: Validation Runner Repair Surface Materialization Retake

Question: after repair-surface noise tells validation runner to avoid listing
repair-only no-diff paths as validated surfaces, which latest-message controls
still preserve successful `git diff --exit-code -- <path>` commands under
`validated_surfaces`?

Payload batches:

- `.pragma/prompt-ab/validation-runner-git-diff-surface-packet-retake-20260603T000000Z`
- `.pragma/prompt-ab/validation-runner-git-diff-surface-exact-materialization-20260603T000000Z`

Setup:

- reused the Experiment 143/102 validation-status state,
- visible output contained:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `EXIT_STATUS[1]: 0`,
  - `COMMAND[2]: git diff --exit-code -- internal/server/auth/debug.go`,
  - `EXIT_STATUS[2]: 0`,
- pass required `Commands run` to preserve both indexed commands and
  `validated_surfaces` to include `internal/server/auth/debug.go`,
- late noise said `validated_surfaces` should describe implementation surfaces
  and should not list repair-only cleanup paths,
- compared current checked-in prompt, negative-only correction, compact field
  packet, YAML packet, exact override text, exact section materialization, and
  fuller artifact materialization.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean | current `validation_runner.yaml` | included `internal/server/auth/debug.go` in `validated_surfaces` | Passed |
| ignore-repair noise | current `validation_runner.yaml` plus late noise | omitted `internal/server/auth/debug.go` | Failed |
| ignore-repair noise | negative-only latest correction | omitted `internal/server/auth/debug.go` | Failed |
| ignore-repair noise | compact latest field packet naming command and required surface | omitted `internal/server/auth/debug.go` | Failed |
| ignore-repair noise | compact YAML-ish latest packet naming command and required surface | omitted `internal/server/auth/debug.go` | Failed |
| ignore-repair noise | exact override saying the visible command has `EXIT_STATUS[2]: 0` and the path must be included even though repair-only | included `internal/server/auth/debug.go` | Passed |
| ignore-repair noise | exact `validated_surfaces` section materialization | included `internal/server/auth/debug.go` | Passed |
| ignore-repair noise | fuller artifact materialization with command evidence and exact section | included `internal/server/auth/debug.go` | Passed |

Learning:

- This failure is specifically a latest-message contradiction problem. The
  static last-mile validation-runner rule works cleanly, but late repair-only
  surface framing can override it.
- Compact packets that merely name the command and required surface are not
  enough in this state. The model still follows the later semantic instruction
  to omit repair-only paths.
- Reliable controls are exact latest materializations that either state the
  repair-only exception explicitly or provide the exact `validated_surfaces`
  section to write.
- No `validation_runner.yaml` change is retained. Orchestration should inject a
  latest exact materialization after any surface-prioritization reminder that
  conflicts with successful `git diff --exit-code -- <path>` repair validation.

## Experiment 157: Evidence Mapper Unknowns Under Uncertainty Pressure

Question: when evidence mapper has enough source/test/fixture evidence but a
late reminder says to verify runtime/proto/generated unknowns one more time,
which latest controls still make it write `/tmp/pragma/evidence-map.md` without
another repository read?

Payload batch:

- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-uncertainty-pressure-20260603T000000Z`

Setup:

- reused the generated/runtime unknowns transcript from Experiments 139, 145,
  and 150,
- visible output already contained snippets from `internal/config/config.go`,
  `internal/config/config_test.go`, and
  `testdata/authentication/kubernetes.yml`,
- surface map listed explicit unknowns
  `internal/server/auth/method/kubernetes/server.go` and `rpc/auth.pb.go`,
- pass required writing `/tmp/pragma/evidence-map.md` with source/test/fixture
  paths, exact validation command
  `go test -run "TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)" ./internal/config/`,
  both explicit unknowns under `Not evidenced`, and sentinel,
- late uncertainty-pressure noise said to verify runtime/proto/generated
  unknowns one more time if they might affect behavior.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| checked-in current | re-read `testdata/authentication/kubernetes.yml` | Failed |
| uncertainty-pressure only | listed runtime/proto paths and grepped runtime auth code | Failed |
| short latest reminder after pressure | listed `testdata/authentication/` instead of writing artifact | Failed |
| negative-only latest correction after pressure | wrote artifact and stopped inspection, but escaped `(YAML)` in the validation command as `\\(YAML\\)` | Partial/rejected |
| compact field packet after pressure | ran the focused validation command instead of writing artifact | Failed |
| full artifact materialization after pressure | wrote the exact evidence map, exact validation command, both unknowns, and sentinel | Passed |

Learning:

- The earlier latest reminder and compact field packet are sufficient in a
  normal generated-unknowns state, but they are not robust against a late
  uncertainty-pressure instruction to verify unknowns.
- Negative-only "do not inspect" wording can stop repository reads, but it does
  not preserve exact validation command spelling. It is rejected as a retained
  control.
- The reliable control for this stronger uncertainty state is full latest
  artifact materialization: give the exact `/tmp/pragma/evidence-map.md`
  content to write.
- No `evidence_mapper.yaml` change is retained. This remains an orchestration
  materialization case when late uncertainty pressure appears.

## Experiment 158: Final Reviewer No-Diff Conflicting Rationale Noise

Question: for dirty repaired-path no-diff contradictions, does the exact
field-lines materialization packet survive a later conflicting reminder to
prioritize debug/generated-scope rationale, or must the packet come after that
noise?

Payload batch:

- `.pragma/prompt-ab/final-reviewer-nodiff-conflicting-rationale-noise-20260603T000000Z`

Setup:

- reused the dirty auth and dirty cache final-reviewer transcripts from
  Experiment 151,
- each case had a successful
  `git diff --exit-code -- <path>; EXIT_STATUS: 0` no-diff repair command,
- final diff still contained the same `<path>`,
- conflicting noise said to prioritize unsafe debug/backdoor files and producer
  evidence, and to block on unvalidated debug/generated scope rather than
  previous cleanup-command wording,
- pass required a code-only BLOCK artifact with the exact no-diff contradiction
  findings, exact rerun command, and sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| dirty auth | conflicting rationale noise, no packet | wrote BLOCK for producer/debug-file reasons, not no-diff contradiction | Failed rationale |
| dirty auth | exact field-lines packet before conflicting noise | mentioned the no-diff command but added debug/producer rationale and wrong repair | Failed rationale |
| dirty auth | exact field-lines packet after conflicting noise | wrote exact no-diff contradiction BLOCK with exact rerun command and sentinel | Passed |
| dirty cache | conflicting rationale noise, no packet | wrote BLOCK for producer/debug-file reasons, not no-diff contradiction | Failed rationale |
| dirty cache | exact field-lines packet before conflicting noise | wrote debug/producer rationale and wrong repair | Failed rationale |
| dirty cache | exact field-lines packet after conflicting noise | wrote exact no-diff contradiction BLOCK with exact rerun command and sentinel | Passed |

Learning:

- Conflicting rationale noise changes the materialization placement boundary.
  Experiment 129 showed packets can survive later aligned reminders, but this
  batch shows they do not survive later contradictory rationale reminders.
- For no-diff contradictions, exact field-lines must be the latest relevant
  instruction after any debug/generated/producer rationale pressure.
- A packet buried before contradictory noise can be partially remembered but
  still lose the required no-diff repair sentence and exact rationale.
- No `final_reviewer.yaml` change is retained. This is a latest-turn
  orchestration rule: inject the exact field-lines or placeholder verdict packet
  after conflicting rationale noise has been added, not before it.

## Experiment 159: Item Worker Repeated-Search Buried State Retake

Question: does the retained item-worker repeated-search footer still stop
implementation when the visible search state is no longer the latest user
message, and do exact report packets remain stronger under reminder pressure?

Payload batch:

- `.pragma/prompt-ab/item-worker-repeated-search-buried-state-retake-20260603T000000Z`

Setup:

- reused the two repeated-search stop shapes from Experiments 113, 125, and
  131:
  - config-symbol case:
    `item-auth-config-symbol Wire Kubernetes auth config symbol`,
    `repeat_count = 4`, repeated searches for `KubernetesTokenValidator` and
    adjacent config names found no new information,
  - proto/generated case:
    `item-002 Generate Kubernetes auth protobuf bindings`,
    `repeat_count = 3`, repeated searches found no producer command and no
    service symbol,
- built requests from checked-in `personas-research-v2/item_worker.yaml`,
- compared search state as the latest user message, search state buried behind
  two aligned reminder turns, placeholder report packet buried before those
  reminders, and placeholder report packet latest after those reminders,
- pass required writing `/tmp/pragma/implementer-report.md`, no repository
  inspection/search/validation/edit command, `Changed: No files changed`,
  repeated-search validation wording, concrete remaining risk, blocker with the
  repeat count and missing symbol/producer/service evidence, and sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| config | current state latest | wrote blocker report with no inspection, repeat count, `KubernetesTokenValidator`, repeated-search validation/risk, and sentinel | Passed |
| proto | current state latest | wrote blocker report with no inspection, repeat count, missing producer/service evidence, repeated-search validation/risk, and sentinel | Passed |
| config | current state buried after reminders | wrote blocker report with no inspection, repeat count, `KubernetesTokenValidator`, repeated-search validation/risk, and sentinel | Passed |
| proto | current state buried after reminders | wrote blocker report with no inspection, repeat count and missing producer/service evidence, but used `Validation: N/A - blocked before implementation` | Partial |
| config | placeholder report packet buried before reminders | wrote the exact packet fields and sentinel | Passed |
| proto | placeholder report packet buried before reminders | wrote the exact packet fields and sentinel | Passed |
| config | placeholder report packet latest after reminders | wrote the exact packet fields and sentinel | Passed |
| proto | placeholder report packet latest after reminders | wrote the exact packet fields and sentinel | Passed |

Learning:

- The retained static footer still fixes the original classification problem:
  even when the search state is buried behind aligned reminders, the worker
  stopped and wrote a blocker report instead of reopening files or rerunning
  searches in both shapes.
- Static buried-state behavior is not field-perfect. The proto buried-state
  replay regressed validation wording to `N/A - blocked before implementation`
  even though it kept the stop action, repeat count, and concrete blocker.
- Exact placeholder report packets remain stronger than static state. They
  preserved exact fields when buried before aligned reminders and when placed
  latest after the reminders.
- No `item_worker.yaml` change is retained. Keep the current static footer for
  robust stop-and-report behavior, but use latest or visible placeholder report
  packets when downstream consumers need exact validation/risk wording.

## Experiment 160: Validation Runner Repair Surface Materialization Position Retake

Question: for successful `git diff --exit-code -- <path>` repair validation
under a conflicting reminder to omit repair-only paths, does exact
materialization need to appear after the reminder, or can it be buried before
the reminder?

Payload batch:

- `.pragma/prompt-ab/validation-runner-git-diff-surface-materialization-position-retake-20260603T000000Z`

Setup:

- reused the Experiment 156 validation-runner repair-surface state,
- visible validation output contained:
  - `COMMAND[1]: go test -run TestMaxRetriesDefault ./cmd/pragma/`,
  - `EXIT_STATUS[1]: 0`,
  - `COMMAND[2]: git diff --exit-code -- internal/server/auth/debug.go`,
  - `EXIT_STATUS[2]: 0`,
- conflicting reminder said `validated_surfaces` should describe implementation
  surfaces and should not list repair-only no-diff checks,
- compared no materialization, exact `validated_surfaces` section after the
  reminder, full artifact after the reminder, exact override before the
  reminder, exact section before the reminder, and full artifact before the
  reminder,
- pass required both indexed commands under `Commands run`,
  `validated_surfaces` containing `cmd/pragma/jobs.go`,
  `cmd/pragma/jobs_test.go`, and `internal/server/auth/debug.go`, plus
  sentinel.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| noise-only latest | wrote status with both indexed commands and all three validated surfaces, including `internal/server/auth/debug.go` | Passed in fresh replay |
| exact `validated_surfaces` section after noise | wrote status with both indexed commands and all three validated surfaces | Passed |
| full artifact after noise | wrote status with both indexed commands and all three validated surfaces | Passed |
| exact override before noise | wrote status with both indexed commands and all three validated surfaces | Passed |
| exact `validated_surfaces` section before noise | wrote status with both indexed commands and all three validated surfaces | Passed |
| full artifact before noise | wrote status with both indexed commands and all three validated surfaces | Passed |

Stability rerun:

- The noise-only request was replayed three additional times into
  `response.rerun-1.raw`, `response.rerun-2.raw`, and `response.rerun-3.raw`.
  All three reruns included `internal/server/auth/debug.go` under
  `validated_surfaces`.

Learning:

- This retake supersedes the earlier single saved failure for the exact same
  noise-only request shape. Current live replay preserved repair diff surfaces
  in four fresh noise-only responses, so the retained static last-mile rule is
  stronger than Experiment 156 suggested.
- Exact materialization remains reliable and is position-tolerant for this
  validation-runner repair-surface state. Exact section, exact override, and
  full artifact forms all survived when placed before the later conflicting
  repair-only reminder.
- Because there is a historical contradictory response for the same request
  shape, do not overclaim that static wording is universally field-stable under
  all future backend behavior. For high-assurance handoffs after repair-only
  surface-prioritization noise, exact materialization is still the safer
  orchestration control.
- No `validation_runner.yaml` change is retained; the checked-in prompt already
  contains the relevant last-mile status rule.

## Experiment 161: Final Reviewer No-Diff Static Selector Retake

Question: can the dirty repaired-path no-diff contradiction gap be solved by a
more literal static final-reviewer footer that selects the no-diff contradiction
branch before debug/generated/producer/validated-surface rationale?

Payload batches:

- `.pragma/prompt-ab/final-reviewer-nodiff-static-selector-retake-20260603T000000Z`
- `.pragma/prompt-ab/final-reviewer-nodiff-static-trigger-retake-20260603T000000Z`

Setup:

- reused the clean auth, dirty auth, and dirty cache transcripts from
  Experiment 151,
- also reused the auth/cache conflicting-rationale transcripts from
  Experiment 158,
- dirty and conflict cases had a successful
  `git diff --exit-code -- <path>; EXIT_STATUS: 0` no-diff repair command while
  the final diff still contained the same `<path>`,
- clean auth had the successful no-diff command for
  `internal/server/auth/debug.go`, but the final diff only changed
  `cmd/pragma/jobs.go`,
- compared four static footer families:
  - literal selector footer,
  - output-shaped branch footer,
  - shorter hard trigger footer,
  - fixed verdict branch footer,
- pass required clean auth to APPROVE with `Required repair: None`, and every
  dirty/conflict case to write a code-only BLOCK with the no-diff contradiction
  rationale, exact path, exact rerun command, no debug/generated/producer
  substitute rationale, and sentinel.

Results:

| Case | Prompt Variant | Observed Behavior | Result |
| --- | --- | --- | --- |
| clean auth | selector footer | wrote APPROVE with `Required repair: None` | Passed |
| dirty auth | selector footer | wrote no-diff contradiction BLOCK with exact path/command and sentinel | Passed |
| dirty cache | selector footer | length-looped with no verdict | Failed |
| auth conflict | selector footer | wrote no-diff contradiction BLOCK | Passed |
| cache conflict | selector footer | wrote BLOCK but added debug/producer rationale | Failed rationale |
| clean auth | output branch footer | wrote APPROVE with `Required repair: None` | Passed |
| dirty auth | output branch footer | APPROVED because both changed files were listed in `validated_surfaces` | Failed |
| dirty cache | output branch footer | wrote no-diff contradiction BLOCK | Passed |
| auth conflict | output branch footer | wrote no-diff contradiction BLOCK | Passed |
| cache conflict | output branch footer | wrote no-diff contradiction BLOCK | Passed |
| clean auth | hard trigger footer | wrote APPROVE with `Required repair: None` | Passed |
| dirty auth | hard trigger footer | length-looped with no verdict | Failed |
| dirty cache | hard trigger footer | length-looped with no verdict | Failed |
| auth conflict | hard trigger footer | wrote BLOCK but added debug/producer rationale | Failed rationale |
| cache conflict | hard trigger footer | wrote BLOCK but lacked the exact no-diff phrase and added debug/producer rationale | Failed rationale |
| clean auth | fixed verdict branch footer | wrote APPROVE with `Required repair: None` | Passed |
| dirty auth | fixed verdict branch footer | length-looped with no verdict | Failed |
| dirty cache | fixed verdict branch footer | length-looped with no verdict | Failed |
| auth conflict | fixed verdict branch footer | length-looped with no verdict | Failed |
| cache conflict | fixed verdict branch footer | wrote BLOCK for debug rationale, not no-diff contradiction | Failed rationale |

Learning:

- The final-reviewer no-diff contradiction remains static-hard. Every static
  selector/trigger/footer family failed at least one required dirty or conflict
  case while clean approval still passed.
- The failure modes are instructive and consistent with prior retakes:
  validated-surface framing can still make dirty auth approve; selector wording
  can still length-loop dirty cache; and conflict pressure can reintroduce
  debug/producer rationale even when no-diff wording is present.
- Shortening the trigger and adding an explicit "validated_surfaces does not
  make this clean" sentence did not fix generalization. It increased
  length-loop failures in the dirty cases.
- No `final_reviewer.yaml` change is retained. The replay-backed control
  remains conversation-level materialization after the no-diff contradiction is
  detected, with the exact field-lines or placeholder verdict packet placed
  after any conflicting debug/generated/producer rationale noise.

## Experiment 162: Evidence Mapper Unknowns Materialization Shape Retake

Question: under late uncertainty pressure to verify runtime/proto/generated
unknowns again, can evidence mapper use a smaller latest packet than a fully
prewritten artifact, and can a stronger static footer finally move this behavior
into `evidence_mapper.yaml`?

Payload batch:

- `.pragma/prompt-ab/evidence-mapper-generated-unknowns-materialization-shape-retake-20260603T000000Z`

Setup:

- reused the generated/runtime unknowns transcript from Experiments 139, 150,
  and 157,
- visible output already contained snippets from `internal/config/config.go`,
  `internal/config/config_test.go`, and
  `testdata/authentication/kubernetes.yml`,
- surface map listed explicit unknowns:
  `internal/server/auth/method/kubernetes/server.go` and `rpc/auth.pb.go`,
- late pressure said to verify runtime/proto/generated unknowns one more time,
- pass required writing `/tmp/pragma/evidence-map.md` without another read or
  validation run, preserving the exact validation command
  `go test -run "TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)" ./internal/config/`,
  keeping both unknowns under `Not evidenced`, and echoing the sentinel,
- compared pressure-only, full-artifact control, latest decision card, latest
  section template, latest field lines, two static footer attempts, and a
  static footer plus latest decision card.

Results:

| Prompt Variant | Observed Behavior | Result |
| --- | --- | --- |
| pressure-only rerun | grepped runtime/proto unknowns again | Failed |
| static artifact-first branch | grepped/listed fixture paths again | Failed |
| static completion copy gate | re-read `testdata/authentication/kubernetes.yml` | Failed |
| latest decision card after pressure | wrote exact artifact, exact validation command, both unknowns, and sentinel | Passed |
| latest section template after pressure | wrote exact artifact, exact validation command, both unknowns, and sentinel | Passed |
| latest field lines after pressure | wrote artifact, exact validation command, both unknowns, and sentinel | Passed |
| full artifact control after pressure | wrote exact artifact, exact validation command, both unknowns, and sentinel | Passed |
| static completion copy gate plus latest decision card | wrote exact artifact, exact validation command, both unknowns, and sentinel | Passed/latest-driven |

Accepted latest-message primitives:

```text
Use this evidence-map write decision card now.

evidence_map_decision:
  action: write_artifact
  output_file: /tmp/pragma/evidence-map.md
  reason: source/defaulting, fixture/test, and focused validation evidence are already visible
  validation_command_mode: copy_as_artifact_text_do_not_execute
  forbid_next_actions:
    - read repository files
    - run validation
    - inspect runtime/proto/generated unknowns
  fields:
    inputs:
      - /tmp/pragma/surface-map.md
    evidence_source_paths:
      - <path>
    consumer_defaulting_paths:
      - <path>
    fixture_or_config_tiers:
      - minimal/default: <path or none>
      - advanced/full: <path or none>
    validation_commands:
      - <exact command text>
    evidenced_generated_derived_paths:
      - none
    producer_command:
      - none
    producer_evidence:
      - none
    not_evidenced:
      - <explicit unknown>

Write /tmp/pragma/evidence-map.md now from the fields above. Use one fenced
bash block only and echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after writing
the file.
```

```text
The evidence map is determined. Replace each placeholder with the value listed
above it and write the artifact now.

validation_command_text = <exact command>

Important: validation_command_text is text for the `Validation commands`
section. Do not execute it.

Write /tmp/pragma/evidence-map.md by filling this pattern:
<standard evidence-map sections>
```

Learning:

- Full artifact materialization is not the only reliable latest control under
  uncertainty pressure. Decision cards, section templates, and field lines pass
  when they explicitly set `action: write_artifact` or equivalent and state that
  the validation command is artifact text, not a command to execute.
- The prior compact field packet failed because it named a validation command
  but did not sufficiently bind the next action to artifact writing. The model
  ran the command instead of writing `/tmp/pragma/evidence-map.md`.
- Static-only wording still fails. Even stronger artifact-first and
  copy-as-text footers did not overcome uncertainty pressure; the model
  re-read fixtures or grepped runtime/proto paths.
- No `evidence_mapper.yaml` change is retained. The new accepted controls are
  latest-message orchestration packets for uncertainty-pressure states.

## Current Completion Audit

Objective: explore prompt-variation dimensions that affect agent behavior across
artifact handling, validation, scope control, implementation discipline, review
gates, command shape, stop conditions, repair behavior, and uncertainty
handling; run focused A/B prompt replays; record reliable and weak variants;
update docs; and retain persona YAML changes only when replay-backed.

Evidence inventory:

- Detailed replay ledger: this file records Experiments 1-162 with payload
  batch paths, setup, result tables, retained/rejected controls, and learning
  notes.
- Synthesis and persona design:
  `docs/personas-from-prompt-control-research.md` contains the Objective
  Coverage Matrix, Retained Control Evidence Index, persona design, and current
  V2 replay status.
- Persona packet: `personas-research-v2/*.yaml` contains eight personas.
  Retained YAML controls are indexed back to experiment anchors in the synthesis
  doc.
- Replay harness: `pragma replay raw-http --timeout` and
  `pragma replay raw-http audit --require-responses` support bounded replay and
  evidence checks for generated batches.

Requirement audit:

| Requirement | Current evidence | Status |
| --- | --- | --- |
| Explore artifact handling | Artifact paths, sentinels, missing/malformed artifacts, after-input artifact writing, exact section templates, producer evidence, and artifact-only handoff are covered across the ledger and synthesis matrix. | Satisfied |
| Explore validation | Exact wrappers, multi-command status, failure extraction, unavailable tools, active process handling, validated surfaces, and producer evidence are covered. | Satisfied |
| Explore scope control | Allowed/forbidden authority, artifact precedence, unresolved source changes, generated producer routing, proof-only/test-only blocks, and changed-file auditability are covered. | Satisfied |
| Explore implementation discipline | Source preservation, exact insertions, generated no-hand-edit policy, producer handoff, validation-only items, and minimal edits are covered. | Satisfied |
| Explore review gates | Item-review and final-review PASS/BLOCK gates, clean-diff approval, generated producer blocks, command-status authority, and no-change implementation blocks are covered. | Satisfied |
| Explore command shape | Literal first commands, exact validation/producer wrappers, indexed command loops, active-process checks, exact rerun commands, and artifact-text command handling are covered. | Satisfied |
| Explore stop conditions | Completed no-op, passed validation, failed producer, repeated search, missing artifacts, active process wait states, and orchestration-only failed-validation stops are covered. | Satisfied |
| Explore repair behavior | Checklist repair authority, completed-item preservation, repair target scope, out-of-scope repair flow, no-diff repair checklisting, validation-runner repair surfaces, and final clean repair approval are covered. | Satisfied |
| Explore uncertainty handling | Unknown/not-evidenced surfaces, source-change missing proof, generated/runtime/proto unknown completion, and uncertainty-pressure materialization are covered. | Satisfied |
| Run focused A/B replays | Experiments 1-162 include real and synthetic raw replay batches and focused variants. Recent batches were audited with `--require-responses`. | Satisfied |
| Record reliable and weak variants | Every experiment records passed, partial, failed, rejected, retained, or orchestration-only variants. | Satisfied |
| Update project docs | Both detailed and synthesis docs are updated through Experiment 162, with coverage and evidence-index sections. | Satisfied |
| Retain persona changes only when replay-backed | The synthesis Retained Control Evidence Index maps retained control families to experiment anchors. Static candidates that failed checked-in replay, especially in `evidence_mapper` and `final_reviewer`, are documented as orchestration-only instead of retained. | Satisfied |

Known boundaries:

- This is not a full benchmark-performance claim.
- `evidence_mapper` generated/runtime/proto unknown completion under late
  uncertainty pressure remains an orchestration-control boundary: latest
  write-bound materialization works; static-only YAML wording still reopens
  inspection.
- `final_reviewer` dirty repaired-path no-diff contradiction remains an
  orchestration-control boundary: latest verdict materialization works; static
  YAML footers failed cross-case replays.
- Long noisy histories require short phases or latest-message control footers;
  one-time persona headers are not a sufficient runtime strategy.

Conclusion:

The requested prompt-control evidence base is complete for the stated research
objective. It documents successful controls, rejected controls, retained YAML
changes, and orchestration-only boundaries. The output is not a claim that the
persona packet wins a full benchmark, but it is a replay-backed prompt-control
foundation for the covered behavior dimensions.

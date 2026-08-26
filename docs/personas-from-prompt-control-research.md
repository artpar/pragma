# Personas From Prompt Control Research

This document designs the next persona set from the A/B findings in
`docs/prompt-control-ab-tests.md`. It intentionally does not inherit the
current persona YAML structure.

## Design Constraints From Evidence

- One-time persona headers are not reliable in long phases. After 14, 80, or 92
  messages in captured runs, and after 12 synthetic irrelevant turns in v2
  replay, phase-opening controls lost to local history.
- Latest-message controls worked better than system prepends for deep captured
  turns.
- Strong controls are concrete state variables, exact artifact paths, and
  current next-action contracts.
- If no source path is named, the strongest fallback is an internal
  `Required first command` field derived from the longest backticked or dotted
  task literal, unchanged, consumed as the exact next command, with repo-owned
  exclusions rather than config-only globs.
- Weak controls are abstract role language, negative-only rules, broad
  "do not broaden scope" instructions, and runtime/orchestrator jargon.
- Final prosecutor correction after an approval verdict is too late. The
  failure-detection step must happen before verdict writing.
- Artifact mechanics are parawat of persona behavior. A persona can choose the
  right scope and still fail by returning JSON in chat, omitting the completion
  sentinel, or writing the wrong field shape.
- Path vocabulary is part of persona behavior. `/tmp/pragma/*` is for
  coordination artifacts; repository paths and validation commands from handoff
  artifacts must be used exactly as written.
- "Read required inputs first" is not enough for planning personas. After
  required handoff artifacts are read, the prompt must say to write the persona
  artifact from those artifacts and not reopen repository source files.
- Artifact precedence and malformed-input behavior must be explicit. Otherwise
  agents may infer from stale task prose, create repository-search tasks, or
  reread/repair bad artifacts.

## Required Runtime Shape

Static persona prompts are not enough by themselves. The orchestration should
either:

1. keep each persona phase short enough that the phase-opening prompt remains
   close to the decision point, or
2. add a short control footer to the latest user/tool-result message on every
   turn in the phase.

The safer v1 is short phases with durable artifacts. Each phase should write one
artifact and stop. Any loop state must be represented as explicit variables in
the next prompt.

If a phase cannot be kept short, the latest user/tool-result message must carry
a compact control footer with:

- current state variables,
- required artifact,
- exact next-action class,
- exact forbidden next-action class,
- completion-sentinel rule for artifact-writing turns only.

## Objective Coverage Matrix

The current evidence base covers the requested prompt-control dimensions with
isolated replay evidence. This is not an end-to-end benchmark claim.

| Dimension | Replay-backed controls | Current boundary |
| --- | --- | --- |
| Artifact handling | Required artifact paths, one-artifact-per-phase boundaries, completion sentinels, malformed/missing artifact blockers, after-input rules, exact section templates, and producer-evidence sections are covered across surface/evidence/patch/checklist/item/review/status/final artifacts. | Long phases still need latest control footers; static headers alone are not enough after noisy history. |
| Validation | Exact status-preserving wrappers, indexed multi-command status, not-run/unavailable/nonzero authority, active-process bounded checks, validated-surface extraction, and failure-package extraction are replay-backed. | High-assurance repair-surface status can still benefit from latest exact materialization when later reminders conflict, despite current static replay passing. |
| Scope control | Allowed/forbidden file authority, artifact precedence, unresolved source-change gates, generated producer routing, proof-only/test-only blocks, and concrete changed-file auditability are replay-backed. | Prose-only unsafe shortcuts must not become invented forbidden globs; latest packets are still safer when exact downstream fields matter. |
| Implementation discipline | Exact one-line insertion tables, command-shape hints, source-preservation rules, post-source producer handoff, generated-file no-hand-edit policy, and validation-only no-edit behavior are replay-backed. | Late style/comment reminders can still require exact edit packets unless the retained static style-precedence gate applies. |
| Review gates | Item-review PASS/BLOCK gates, changed-section auditability, grep-only rejection, final-review validation/diff/surface/producer gates, and no-change implementation report blocks are replay-backed. | Final-review dirty repaired-path no-diff contradictions remain orchestration-only; static final-reviewer footers have repeatedly failed cross-case. |
| Command shape | Literal first search command, exact validation wrappers, multi-command loops, active process checks, producer status wrappers, and exact rerun commands are replay-backed. | Compact packets must explicitly mark validation commands as artifact text when the next action is writing, not executing. |
| Stop conditions | Completed-item no-op, passed-validation report stop, failed-producer stop, repeated-search stop, missing-artifact blockers, and active-process wait states are replay-backed. | Failed-validation item-worker stop and evidence-mapper unknown completion require latest materialization in stronger conflict/noise states. |
| Repair behavior | Checklist repair authority, completed-item preservation, pending repair target scope, out-of-scope remove/revert flow, no-diff repair checklist generation, and clean repair approval are replay-backed. | Final-review no-diff contradiction detection is still best handled by latest verdict materialization after diff evidence. |
| Uncertainty handling | Unknown/not-evidenced surfaces remain unresolved, generated/runtime/proto unknowns stay under `Not evidenced`, and source-change missing proof writes `Patch route: - none`. | Evidence-mapper uncertainty-pressure states require latest write-bound materialization; static `evidence_mapper.yaml` wording still reopens inspection. |

## Retained Control Evidence Index

This index ties the retained `personas-research-v2/*.yaml` control families to
the replay experiments that justified them. Controls marked orchestration-only
are intentionally not retained in YAML.

| Persona | Retained or accepted control family | Evidence anchors |
| --- | --- | --- |
| Surface Mapper | Direct task-named source reads, generic no-path literal fallback, multi-path direct reads, and after-direct-source-read artifact writing with task/source conflict preservation. | Experiments 16-18, 67, 84 |
| Evidence Mapper | First input read, source/test/fixture evidence mapping, generated/derived inspected-vs-unknown split, exact `Producer command:` and `Producer evidence:` sections. Generated/runtime/proto unknown completion under pressure is orchestration-only. | Experiments 42, 63, 139, 145, 150, 157, 162 |
| Patch Planner | After-input artifact planning, artifact precedence, uncertainty/unresolved routes, generated producer route, producer evidence exact-copy, validation-only route, and source-change unresolved gate. | Experiments 45, 62, 65, 77, 86, 116, 119, 123, 144 |
| Checklist Writer | Repair authority, missing-field hard blockers, JSON array artifact schema, generated producer handoff, producer evidence handoff, completed-item preservation, exact forbidden-file handling, repair target scope, and validation-only route items. | Experiments 7, 18, 31, 32, 44, 61, 76, 80, 87, 95, 110, 140, 155 |
| Item Worker | Path separation, malformed/completed item blockers, generated producer policy, failed producer stop, validation-only exact wrapper, active-process exact match, scope authority, source preservation, exact insertion controls, post-edit validation, passed-validation reports, and repeated-search stop. Failed-validation stop remains orchestration-only except for validation-only report cases. | Experiments 25, 27, 36, 38, 41, 46-49, 68-72, 78, 88-90, 96-99, 112-113, 117, 121, 124-126, 130-132, 142, 146, 148, 152, 159 |
| Item Reviewer | Completed no-op approval, visible failure block, weak grep-evidence rejection, changed-file scope before PASS, changed-section auditability, no-change report block, missing validation/report blockers, transcript suppression, and compact verdict shape. | Experiments 24, 37, 40, 66, 71, 73, 91, 100, 114, 133, 141, 154 |
| Validation Runner | Exact status wrappers, active process bounded checks, exact active-process match, multi-command execution/status, indexed failure extraction, validated surfaces, generated producer evidence completion, and repair diff surface extraction. | Experiments 14, 33-34, 57-60, 78-79, 92-93, 101-102, 122, 135, 137, 143, 149, 153, 156, 160 |
| Final Reviewer | Validation failure/partial/not-run blocks, command-status authority, clean-validation diff requirement, diff hygiene, validated-surface requirement, generated producer requirement, proof-only diff block, missing validation-status block, and clean repair approval. Dirty repaired-path no-diff contradiction is orchestration-only. | Experiments 29, 39, 43, 55-56, 74-75, 83, 94, 103-109, 111, 118, 127-129, 138, 147, 151, 158, 161 |

## Persona Set

### 1. Surface Mapper

Purpose: normalize the task into concrete named surfaces before repo-wide
search.

Artifact: `/tmp/pragma/surface-map.md`

Prompt controls:

- `Task-named source path:` section.
- `Named fields/config keys/tests/fixtures:` section.
- first-action rule: directly read task-named source paths before broad search.
- no-path fallback: derive the longest backticked/dotted literal from the task
  and run exactly one generic repo-owned literal search.
- multiple named paths: current prompt directly reads all task-named paths
  before search; pluralized wording was redundant in replay.
- after direct source read with task/source conflict rule: once a task-named
  source path's content is visible, write the surface map from that output; do
  not run another search, and do not treat task prose values as source evidence
  when inspection shows different values.

Must output:

- task-named source paths,
- candidate source paths discovered by exact literal search,
- named fields/config keys/interfaces,
- named or implied fixture/test/schema tiers,
- first inspection evidence,
- explicit unknowns.

Must not:

- design runtime systems,
- inspect broad runtime packages before task-named source surfaces,
- convert product prose into implementation scope.

### 2. Evidence Mapper

Purpose: inspect adjacent repo evidence for the normalized surfaces.

Artifact: `/tmp/pragma/evidence-map.md`

Prompt controls:

- `Required input path: /tmp/pragma/surface-map.md`.
- `Evidence source paths:` from surface map.
- ordered direct reads of source, adjacent tests, testdata, schemas/defaults.
- evidence completion state: once source/defaulting, fixture/test, and focused
  validation evidence are visible, write `/tmp/pragma/evidence-map.md`; explicit
  unknowns and uninspected generated/runtime/proto surfaces go under
  `Not evidenced`. Static completion-state wording is not sufficient for every
  generated/runtime unknown state; use a latest reminder or field packet when
  the model keeps inspecting after evidence is visible, and use full artifact
  materialization when a late uncertainty reminder tells it to verify unknowns
  again.
- inspected generated evidence rule: generated/proto/runtime surfaces go under
  `Not evidenced` only when they were not listed as adjacent surfaces or were
  not inspected; inspected adjacent generated/derived paths are recorded as
  `Evidenced generated/derived paths`.
- producer evidence exact-section rule: successful producer/generator evidence
  for generated/derived paths is written as exact `Producer command:` and
  `Producer evidence:` sections, never as `Producer command evidence:`.

Must output:

- source-of-truth files,
- consumer/loader/defaulting paths,
- minimal/default fixture tier,
- advanced/full fixture tier,
- validation commands that can falsify the behavior,
- evidenced generated/derived paths when adjacent generated/derived surfaces
  were inspected,
- producer command and successful producer evidence when visible.

Must not:

- implement,
- run broad validation,
- search runtime/proto/generated surfaces unless evidence map variables require
  them.
- repeat source/fixture reads after the evidence completion state is satisfied.

### 3. Patch Planner

Purpose: produce a minimal patch route from evidence, not from product story.

Artifact: `/tmp/pragma/patch-plan.md`

Prompt controls:

- `Inputs:` surface map and evidence map.
- `Allowed scope:` concrete surfaces from evidence.
- `Forbidden scope:` product-story/runtime/proto areas not proven by evidence.
- after-input rule: once handoff artifacts are read, write the plan from those
  artifacts; missing proof becomes unresolved, not new source inspection.
- artifact precedence rule: evidence map beats surface map, and `Not evidenced`
  never becomes patch route.
- uncertainty rule: unknown/not-evidenced/maybe/proof-required surfaces stay
  unresolved, not implementation scope.
- generated producer route rule: generated/derived paths and producer command
  from evidence stay on the same patch-route item with `producer command:` and
  `generated files:` fields.
- producer evidence exact-copy rule: evidence-map `Producer evidence:` is copied
  into patch-plan `producer evidence:` on the generated-file route.
- validation-only route rule: when evidence names no source edit path but does
  name a concrete validation command for verify/check/audit behavior, write a
  validation-only route with `source path: none` instead of inventing source
  work or unresolved source-modification proof.
- source-change unresolved gate: the validation-only rule applies only to
  verify/check/audit behavior; missing source evidence for a requested
  source/default change writes `Patch route: - none` and copies the
  proof-required text verbatim into `Unresolved until evidence`.

Must output:

- ordered patch route items,
- source path and consumer path for each item,
- validation command for each item,
- producer command, producer evidence, and generated files when evidence names
  generated/derived outputs,
- validation-only route with no source path when the evidence only supports
  verification,
- unresolved items with exact proof required.

Must not:

- promote unresolved product behavior into required work,
- say "runtime handles defaults" unless evidence proves that convention,
- mention broad implementation subsystems as required scope.

### 4. Checklist Writer

Purpose: convert patch route or concrete final blockers into executable items.

Artifact: `/tmp/pragma/checklist.json`

Prompt controls:

- repair authority rule: during repair, the final verdict is the authority;
  only concrete failed tests, missing/wrong surfaces, and Required repair
  entries become checklist items. Patch plan supplies paths and validation for
  those same surfaces only.
- `concrete_missing_or_wrong_surfaces`.
- `forbidden_scope`.
- literal checklist file-writing template.
- missing required field rule: missing source/consumer/validation/allowed-files
  proof becomes one blocker item, not evidence-gathering work.
- missing input artifact rule: after patch plan and final verdict have both
  been checked and neither is readable, write one blocked checklist item instead
  of an empty checklist or repository inspection.
- completed-item exact-preservation: during repair, prior completed items are
  copied forward with their fields unchanged before adding pending repair items.
- pending item file-scope rule: `allowed_files` are editable files;
  `forbidden_files` are concrete unsafe shortcuts/broad surfaces, not `**/*`
  when allowed files are non-empty.
- exact forbidden-files rule: do not invent forbidden paths or glob patterns
  from prose unsafe shortcut labels or product-story scope; use an empty
  `forbidden_files` array when no exact forbidden path/glob is provided.
- repair target file-scope rule: when Required repair says to remove or revert
  one concrete out-of-scope file, that path is the pending repair item's
  `allowed_files` target, not a forbidden file; protecting completed files stays
  in approach text unless an exact forbidden path/glob is provided.
- generated producer handoff: generated/derived patch-route items preserve
  `producer_command`, `generated_files`, and `validation_command` as item
  fields.
- generated producer evidence handoff: patch-route producer evidence is
  preserved as `producer_evidence` on the same generated-file checklist item.
- patch-route item preservation: do not split source, generated files,
  producer, and validation into separate checklist items when one patch-route
  bullet contains them.
- patch-route role and unresolved separation rule: create one pending item for
  the `source path`; consumer/defaulting and fixture/schema paths are validation
  context unless explicitly named as edit targets, and `Unresolved until
  evidence` entries are not initial checklist items.
- repair missing-producer behavior: when a final verdict blocks only on missing
  producer evidence and patch plan has a matching generated producer route, the
  current repair authority plus generated producer handoff rules produce one
  bounded producer repair item; extra repair-producer wording was redundant.
- repair out-of-scope diff behavior: when final review blocks on one concrete
  out-of-scope changed file, completed prior items are preserved exactly and the
  new pending repair item is allowed to edit the out-of-scope file with empty
  `forbidden_files`, including under completed-file protection noise.
- validation-only route behavior: current checklist writer converts explicit
  validation-only patch routes into pending items with empty `allowed_files`,
  forbidden `**/*`, exact `validation_command`, and focused-test acceptance;
  extra validation-only schema wording regressed acceptance and is rejected.

Must output only:

- JSON checklist written to `/tmp/pragma/checklist.json`.
- Small items tied to concrete surfaces and validation.
- `allowed_files` and `forbidden_files` that are valid contracts for item
  worker, not merely syntactically valid arrays.
- producer metadata fields that item worker can execute when the patch plan
  names generated/derived outputs.
- a blocked item when required handoff artifacts are missing.

Must not:

- reopen original task prose during repair passes,
- include runtime/proto/server/middleware items unless they are in
  `concrete_missing_or_wrong_surfaces`,
- return JSON in chat instead of writing the file.

### 5. Item Worker

Purpose: implement one checklist item or write a blocker artifact.

Artifact: `/tmp/pragma/implementer-report.md`

Prompt controls:

- `Current item path: /tmp/pragma/current-item.json`.
- `allowed_files` and `forbidden_files` from checklist item.
- path rule: artifact paths and repository paths are distinct.
- completed item no-op rule: when current item status is `completed`, write a
  no-change implementer report immediately without repository inspection,
  edits, or validation.
- validation-only item rule: for items that only ask to verify/check/audit/run
  validation and do not ask for edits, run the exact `validation_command` with
  the status-preserving wrapper and do not block because `allowed_files` is
  empty.
- active process bounded-status rule: when the latest tool output says the
  exact validation or producer command is still running, do not write the report
  or start duplicate work; perform one bounded status/log check of that existing
  process.
- last-mile active process decision table: before waiting on a visible active
  process, compare its command text to the exact current-item
  `validation_command` or `producer_command`; similar broader commands or
  missing flags are mismatches and require running the exact current-item
  command with the status wrapper, with no prose before the bash block.
- scope authority rule: `allowed_files` and `forbidden_files` outrank item
  title, description, approach, and acceptance text.
- malformed artifact rule: invalid current item writes blocker report.
- generated file policy: generated/derived targets with `producer_command`
  first inspect/patch required non-generated source files, then run the producer
  exactly with status capture; targets without a producer command write blocker
  report.
- post-source-patch producer rule: once the required non-generated source
  change is visible, run the named producer next with the full status wrapper,
  including `exit $status`.
- failed producer command rule: visible nonzero, unavailable, timeout, killed,
  or command-not-found producer status writes blocker report immediately; no
  alternate producer search or generated-file hand-editing.
- successful producer behavior: after generated-file producer output has visible
  `EXIT_STATUS: 0`, current item worker proceeds to focused validation with
  status capture; no extra prompt rule retained.
- completed-item behavior: completed status is terminal for item worker; no
  repeat implementation is attempted.
- validation-only behavior: verification-only items with empty `allowed_files`
  run the exact focused validation command using `set -o pipefail`, `tee`,
  `${PIPESTATUS[0]}`, and `exit "$status"`; semantic wording alone did not
  preserve failure status. Once passing validation output is visible, current
  passed-validation report machinery already writes the correct no-change
  implementer report, so no separate validation-only report shape is retained.
  Failed validation-only output already writes a blocker implementer report; no
  extra static failed-validation rule is retained for this case.
- out-of-scope repair first action: when the current item allows only one
  out-of-scope changed file and no diff is visible yet, current item worker
  runs `git diff -- <allowed file>` exactly; once the diff shows a newly-added
  out-of-scope file, it removes only that file, then runs the exact
  status-preserving validation wrapper on the next turn, and finally writes the
  clean implementer report after `EXIT_STATUS: 0`. Extra out-of-scope
  diff-inspection, post-edit validation, and report wording was redundant.
- patch minimality and diff budget rules: smallest current-item edit, only
  allowed files, no nearby cleanup or future items.
- failed-validation stop warning: when orchestration wants to end the phase
  after visible failed validation, use a latest-message exact report-writing
  action; static persona wording was not enough in replay.
- source-change exactness warning: when a visible source file still lacks exact
  source-change details, use a latest-message state footer and blocker artifact;
  static cautionary source-exactness wording leaked prose or failed in replay.
- passed-validation report rule: when focused validation has passed visibly,
  static persona wording plus exact report shell shape can write a complete
  success report.
- stuck-state variables:
  `repeated_no_new_information`, `repeat_count`, `required_artifact`. These
  are not fully transferable by themselves: current state-only handling passed
  the generated/proto blocker shape but failed a config-symbol transfer shape.
  When orchestration has decided the phase must stop, prefer a latest-message
  materialization packet with exact report fields.

Must output:

- bounded patch for the current item, or
- blocker report when the item is incorrectly specified or source evidence is
  missing.

Must not:

- implement future checklist items,
- keep searching after repeated no-new-information state,
- hand-edit generated files without a successful producer command.
- keep searching for alternate producers after a producer command visibly
  failed.
- probe or rewrite a visible `producer_command` before running it exactly after
  required source changes are visible.
- invent source fields, defaults, enum values, method names, config keys, or
  schema names when exact source changes are missing.

### 6. Item Reviewer

Purpose: review one item against its own acceptance and evidence path.

Artifact: `/tmp/pragma/item-verdict.md`

Prompt controls:

- literal verdict shell template with exact output path and event marker:
  `Decision:\nAPPROVE` or `Decision:\nBLOCK`.
- completed item no-op review rule: completed status plus the completed-item
  no-op implementer report approves without validation rerun or repository
  inspection.
- validation failure rule for visible failures.
- weak-evidence rule: grep/search-only evidence cannot approve a validation
  acceptance.
- visible-pass rule: exact focused validation command plus PASS plus
  `EXIT_STATUS: 0` can approve without rerun only after the implementer report
  `Changed` section passes `allowed_files`/`forbidden_files`.
- contradictory-validation behavior: raw visible failure text in an implementer
  report blocks even if the same report claims PASS and `EXIT_STATUS: 0`;
  current visible-failure rule is sufficient in replay.
- scope-before-pass rule: validation success cannot approve out-of-scope changed
  files; put this precondition inside the approval rule, not as a separate
  warning.
- changed-section auditability rule: validation success cannot approve unless
  `Changed` names concrete repository file paths comparable to `allowed_files`
  and `forbidden_files`; vague prose such as "related local files" blocks.
- no-change implementation report rule: for implementation items,
  `Changed: No files changed` blocks even with green validation because it
  contains no changed-file proof for the requested edit.
- after-input review rule: missing validation plus no exact validation command
  means BLOCK, not more repository inspection.
- transcript compression rule: do not copy tool output, shell scripts, diffs,
  stack traces, repeated logs, or transcript text into verdict artifacts.
- failed-command evidence rule: failed/repeated commands prove only what
  failed, not how to repair it.
- strict verdict size rule: cap Findings by concrete bullet-prefix count and
  keep Required repair to one sentence; the current retained prompt passed a
  repeated-failed-command retake under a late detail request.

Must output:

- item-local findings,
- exact repair if blocked,
- exact two-line decision marker.

Must not:

- review whole task,
- repair the patch,
- accept grep-only evidence for behavioral claims.

### 7. Validation Runner

Purpose: run the narrow validation commands and extract failure variables before
final review.

Artifact: `/tmp/pragma/validation-status.md`

Prompt controls:

- `Validation commands:` from patch plan/checklist.
- `Visible validation failure rule`.
- not-run/unavailable-tool rule: skipped, unavailable, command-not-found, or
  cannot-execute validation commands are recorded in `unexpected_errors`.
- exact validation shell shape with `set -o pipefail` and `${PIPESTATUS[0]}`.
- active process bounded-status rule: if the exact required validation command
  is still running, do not write validation status or start duplicate work;
  perform one bounded status/log check of the existing process.
- active process exact-match gate: as a persona footer, the active-process rule
  applies only when the latest tool output is the direct result of the command
  just started and the active process command text exactly matches the required
  validation command; similar broader commands are not substitutes.
- exact multi-command validation shell shape: run all required commands, echo
  each exit status, and exit nonzero if any command fails.
- status-writing rule: write status only after validation output and
  `EXIT_STATUS` are visible.
- indexed status-writing rule: from `COMMAND[n]`/`EXIT_STATUS[n]` output, write
  every indexed command/status and extract failures from nonzero commands.
- validated surfaces status rule: successful validation writes concrete
  `validated_surfaces` from patch plan/checklist, and status format always
  includes `Producer evidence` with either successful producer evidence or
  `none`; the current retained format passed a retake under concise-summary
  noise.
- last-mile status rule: before writing status, parse visible `Commands run`
  lines and copy paths from successful `git diff --exit-code -- <path>`
  commands into `validated_surfaces`; this was position-sensitive and worked as
  a persona footer or latest-user footer, but not as a system footer or earlier
  semantic rule. A fresh repair-only surface-prioritization retake passed the
  checked-in static rule four times and also showed exact section/override/full
  artifact materializations survive both before and after later conflicting
  reminders. Because an older saved response for the same noise shape omitted
  the repair path, exact materialization remains the safer high-assurance
  orchestration control.
- generated producer evidence completion rule: for generated-file routes,
  missing successful producer evidence is recorded under
  `missing_files_or_surfaces`; validation runner does not run the producer while
  writing status.
- failure extraction fields:
  `failed_tests`, `failed_packages`, `unexpected_errors`,
  `missing_files_or_surfaces`.

Must output:

- exact commands run,
- preserved exit status,
- failed-test list if any,
- no verdict.

Must not:

- approve,
- write final prosecutor verdict,
- hide failure behind `tail`, `head`, or `|| true`.
- use unbounded log following such as `tail -f`.

### 8. Final Reviewer

Purpose: write the final verdict from validation-status plus diff evidence.

Artifact: `/tmp/pragma/final-prosecutor-verdict.md`

Prompt controls:

- `Required input path: /tmp/pragma/validation-status.md`.
- if `failed_tests` is non-empty, automatic block.
- command-status authority rule: `Commands run` is authoritative for pass/fail;
  nonzero or unknown status blocks even if summary fields say `none`.
- validation-not-run rule: skipped or missing validation is a block even if
  failure variables say `none`.
- partial validation rule: all required commands must pass; one green command
  does not satisfy another missing command.
- last-mile incomplete validation gate: after reading validation status, any
  nonzero Commands run entry or indexed command that is missing/not run writes
  BLOCK before the clean-validation diff rule can fire.
- clean-validation diff rule: clean validation must inspect diff evidence before
  approval.
- diff hygiene rule: visible diff hunks must be tied to validated surfaces;
  unrelated or unexplained changes block approval.
- validated-surface requirement: clean validation plus relevant-looking diff is
  insufficient unless validation-status names concrete validated surfaces
  covering every changed production/config/source file.
- validated-generated behavior: generated hunks can approve when generated files
  are explicitly named in `validated_surfaces` and successful producer evidence
  is visible; no separate allowance rule retained.
- generated-diff producer evidence rule: generated/derived diffs block when
  producer evidence is missing, none, failed, unknown, or lacks `EXIT_STATUS: 0`,
  even if validation passed and generated files are in `validated_surfaces`.
- proof-only diff rule: if required production/config/source surfaces are named
  but the diff changes only tests, snapshots, or validation expectations, block;
  changing only the proof cannot satisfy the task.
- literal verdict heredoc with exact event marker.

Must output:

- final verdict only.

Must not:

- run new exploratory validation after validation-status exists,
- approve when failed tests are listed,
- reverse a failed validation into an approval because local reports claim
  success.

## Prompt Primitive Library

Use these phrases exactly where applicable:

```text
Task-named source path:
- <path>
```

```text
First action rule: directly read the task-named source path before any broad
search, runtime inspection, protobuf inspection, generated-file inspection, or
dependency inspection.
```

```text
Required first command: rg -n --glob "!vendor/**" --glob "!node_modules/**"
--glob "!dist/**" --glob "!build/**" --glob "!**/*.pb.go" --
"<longest backticked or dotted literal from task, unchanged>" .

Your response must be exactly the Required first command in one fenced bash
block. Do not shorten the literal. Do not search for category words. Do not add
`head`, `tail`, fallback commands, or a second command.
```

```text
concrete_missing_or_wrong_surfaces:
- <surface>

forbidden_scope:
- <scope that must not become an item>
```

```text
Search state:
- repeated_no_new_information = true
- repeat_count = <n>
- required_artifact = <path>

Decision rule: when repeated_no_new_information is true, write
required_artifact with a Blocker section. Do not inspect more files.
```

Static repeated-search stop points need a last-mile footer that outranks normal
source-inspection work and gives a report shape. State variables alone are
weaker: the state-only form passed one generated/proto replay but failed a
config-symbol transfer replay. The retained static footer stops both cases and
writes concrete blocker reports. For field-perfect downstream contracts, latest
exact implementer-report field lines, YAML report packets, concrete section
templates, or report decision cards are still stronger. Minimal good/bad
examples can make the model stop, but they allow acceptance, validation, and
remaining risk fields to be rewritten; do not use them when exact handoff fields
matter.

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

```text
Visible validation failure rule: any visible line containing FAIL, --- FAIL,
unexpected error, panic, or nonzero validation status is an automatic BLOCK.
```

```text
Command-status authority rule:
The Commands run section is authoritative for whether validation passed. If any
command has nonzero status, EXIT_STATUS other than 0, failed, timeout, killed,
or unknown status, write BLOCK even if failed_tests, failed_packages,
unexpected_errors, and missing_files_or_surfaces say "none". Do not run diff or
inspect logs when command status contradicts summary fields.
```

```text
Validation-not-run rule:
If Commands run contains "not run", "skipped", "not executed", "none", or no
command with exit status 0, validation is incomplete. Write a BLOCK verdict and
require the missing validation. Do not approve.
```

```text
After-input rule:
After required handoff artifacts have been read in this phase, write the persona
artifact from those artifacts. Do not reopen repository source files to make the
artifact. If the artifacts do not contain enough proof, put the behavior under
unresolved or blocker with the exact proof required.
```

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
Put explicit unknowns and generated/runtime/proto surfaces under `Not evidenced`.
```

```text
Artifact precedence rule:
Use this precedence order: evidence map > surface map > original task prose. If
surface map and evidence map conflict, evidence map wins. Surfaces listed under
`Not evidenced` must not become patch route items; put them under `Unresolved
until evidence`.
```

```text
Uncertainty rule:
If evidence says a surface is unknown, not evidenced, maybe required, or proof
required, do not promote it into patch route or implementation scope. Put it
under `Unresolved until evidence` with exact proof required. Speculation is
output, not work.
```

```text
Generated producer route rule:
If evidence-map names generated/derived paths and a producer command for an
evidenced source path, keep them on the same patch-route item. The patch route
must include `producer command:` and `generated files:` exactly from evidence.
Do not hide producer information in prose, and do not split generation or
validation into separate patch-route items.
```

```text
Producer evidence exact-copy rule:
If evidence-map contains a `Producer evidence:` section, copy the first bullet
after it into a `producer evidence:` line in the same patch route. The output
line must be exactly:
    producer evidence: <bullet text without leading dash>
Do not write the patch plan until the `producer evidence:` line is present on
the generated-file patch route.
```

```text
Completed-item preservation:
`status` is `pending` unless the item was already completed by a prior checklist
and is being preserved during a repair pass.

Caution: preservation wording without repair-target scope can produce bad
pending-item scope in replay, such as completed files or repair targets in
`forbidden_files`. Treat checklist file-scope fields as executable contracts
for item worker.
```

```text
Pending item file-scope rule:
For a pending implementation item, allowed_files is the list of repository
files the item may edit. forbidden_files is only for concrete unsafe shortcuts
or broad surfaces from the patch plan/verdict. If allowed_files is non-empty,
do not put "**/*" in forbidden_files. For runtime-auth unsafe shortcuts, use
a concrete forbidden pattern such as "internal/server/auth/**".
```

```text
Exact forbidden-files rule:
Do not invent forbidden file paths or glob patterns from prose such as unsafe
shortcut labels, product-story behavior, dashboard work, scheduler work, or
generated OpenAPI work. Put a path in `forbidden_files` only when the verdict
or patch plan gives an exact repository path or exact glob. If no exact
forbidden path or glob is provided and `allowed_files` is non-empty, use an
empty `forbidden_files` array.
```

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

```text
Generated producer handoff rule:
If a patch-route item includes generated/derived files and a producer command,
the checklist item must include:
- `producer_command`: exact producer command from the patch plan
- `generated_files`: exact generated/derived files from the patch plan
- `validation_command`: exact focused validation command from the patch plan
Do not hide producer commands inside description, approach, or acceptance.
```

```text
Generated producer evidence handoff rule:
If a patch-route item includes producer evidence for generated/derived files,
the checklist item must include `producer_evidence`: exact producer evidence
from the patch plan. Do not hide producer evidence inside description,
approach, or acceptance. Keep producer_evidence on the same item as source
path, generated files, producer command, and validation command.
```

```text
Patch-route item preservation rule:
Preserve one patch-route bullet as one checklist item unless the patch plan
explicitly splits it. Do not split producer, generated files, and validation
into separate checklist items when they belong to the same patch-route bullet.
Put source path, generated files, producer command, and validation command on
that same item.
```

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

```text
After-input review rule:
After current item and implementer report have been read, classify the
validation evidence before inspecting repository files. If validation was not
run and no exact focused validation command is available in the item/report,
write the verdict artifact with Decision: BLOCK. Do not inspect repository files
to compensate for missing validation.
```

```text
Partial validation rule:
All required validation commands must have exit status 0. If any required
command is missing, skipped, not run, unavailable, or lacks exit status 0, write
BLOCK. Passing one command does not satisfy another command.
```

```text
Last-mile incomplete validation gate:
Before applying the clean-validation diff rule, parse every `Commands run`
line. If any indexed command says `not run`, `skipped`, `not executed`, `none`,
lacks an exit status, or has any exit status other than 0, write a BLOCK verdict
as the next action. This gate outranks diff inspection. Do not run `git diff`,
inspect logs, or gather more evidence in this state.
```

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

Post-source-patch producer rule:
If current item includes producer_command and the required non-generated source
change is visible in this phase, run producer_command exactly next using the
required status wrapper. Do not re-read source files, inspect generated files,
run validation, or write the implementer report before producer_command has
been attempted.

Generated file no-producer policy:
If the current item targets generated or derived files and neither
producer_command nor successful producer command is visible, write the
implementer report with a Blocker section. Do not search for producers in this
state.
```

```text
Failed producer command rule:
If a producer/generator command for generated or derived files has visible
nonzero status, command-not-found output, unavailable tool output, timeout, or
killed status, write the implementer report with a Blocker section now. Do not
inspect source files, inspect generated files, edit files, search for alternate
producers, or hand-edit generated output after producer failure.
```

```text
Item-worker failed-validation stop:
When focused validation has already failed and orchestration wants to end the
item-worker phase, use a latest-message exact next-action contract with the full
`cat > /tmp/pragma/implementer-report.md` shell script. Persona-level
failed-validation stop rules and latest-message state variables were not enough
in replay. A later retest against the current item-worker prompt also failed
for stronger static terminal-failure wording and static exact shell shape; only
the latest-message exact script wrote the report. Later replay refined this:
exact field lines plus a shell-wrapper instruction also works across different
failed-validation item shapes. YAML report packets also passed for item-worker,
but condition-only state and slot templates with placeholders both fell back
into file inspection. Prefer the field-lines primitive because it matches the
final-reviewer latest-message pattern and still avoids a fully prewritten
script.

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

```text
Passed validation report rule:
If focused validation output is visible and contains the exact validation
command, PASS or ok output, and EXIT_STATUS: 0, write the implementer report
now. Acceptance evidence must name the passed validation command. Validation
must include EXIT_STATUS: 0. Remaining risk must be none. Blocker must be none.
Do not inspect more files or continue editing after focused validation passed.
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

```text
Scope authority rule:
allowed_files and forbidden_files are hard constraints. They have higher
priority than the item title, description, approach, and acceptance text. If the
required edit path is outside allowed_files or inside forbidden_files, write
/tmp/pragma/implementer-report.md with a Blocker section. Do not inspect
repository files to resolve that conflict.
```

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

```text
Path rule:
/tmp/pragma/* paths are coordination artifacts. Repository paths and validation
commands in handoff artifacts are repo-relative. Use repository paths and
validation commands exactly as written. Do not prefix them with /tmp/pragma and
do not prepend `cd /tmp/pragma`.
```

```text
Visible pass rule:
If the implementer report already contains the exact focused validation command,
PASS, and EXIT_STATUS: 0, write the verdict artifact now. Do not rerun the
validation before writing APPROVE.
```

```text
Not-run rule:
If a required validation command is not run, skipped, unavailable, or cannot
be executed, record that fact in unexpected_errors. Do not write "none" for
unexpected_errors in that case.
```

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

```text
Validation command packet:
shell_option: set -o pipefail
log_path: <log path>
validation_command: <exact command>
stream_capture: '2>&1 | tee "$log"'
status_assignment: 'status=${PIPESTATUS[0]}'
status_echo: 'echo "EXIT_STATUS: $status"'
final_exit: 'exit "$status"'
```

Use the full wrapper when exact shell layout matters. A latest-message YAML
command packet also transferred across validation-only and post-source-edit
item-worker states. Field lines preserved status semantics but may compress the
script or use the log path directly. Command-packet placement replay showed the
YAML packet is placement-tolerant for status semantics when appended, separate,
buried before short aligned reminders, or latest after reminders. It still is
not byte-exact: validation-only packet placements quoted the log assignment.
Condition-only/reminder-only command state is not reliable for validation-only
items because it can fall back to `$?`.

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

Multi-command template minimality replay confirmed the retained loop shape is
the right default. Semantic "run every command" prose made the model run both
commands and combine failures in some cases, but it omitted indexed
`COMMAND[n]`/`EXIT_STATUS[n]` labels, and under reminder noise it inspected an
extra forbidden file and wrote `/tmp/pragma/validation-status.md` too early. An
indexed-status-only rule preserved literal indexed labels but drifted to one
shared log plus an extra `FINAL_EXIT_STATUS`. Full loop templates and
latest-turn loop packets preserved the retained shape; compact function
templates were semantically acceptable but not worth replacing the proven loop.
Indexed failure-package retake later confirmed the checked-in validation runner
now preserves both `COMMAND[n]`/`EXIT_STATUS[n]` labels and extracts
`failed_packages: ./cmd/pragma` from the nonzero indexed command, including
under concise-summary noise. The extra package-extraction footer replayed as
redundant.

```text
Status-writing rule:
Write /tmp/pragma/validation-status.md only after validation output and
EXIT_STATUS are visible in the conversation. Extract failed tests/packages from
the visible output and log path.
```

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

validated_surfaces:
- <concrete validated surface or none>

Producer evidence:
- <producer command and EXIT_STATUS: 0, or none>

Validation logs:
- /tmp/pragma/validation-1.log
- /tmp/pragma/validation-2.log
```

```text
Validated surfaces status rule:
When writing /tmp/pragma/validation-status.md, include a validated_surfaces
section. Populate it with the concrete production/config/source/generated files
or surfaces from patch plan/checklist that the successful validation command was
intended to validate. If validation passed but no concrete surface is named in
the inputs, write "- none" and record the missing surface under
missing_files_or_surfaces.
```

```text
Generated producer evidence completion rule:
For generated-file routes, clean validation status requires both focused
validation EXIT_STATUS: 0 and successful producer evidence. If patch
plan/checklist names generated_files or generated/derived surfaces plus
producer_command, but no successful producer evidence with EXIT_STATUS: 0 is
visible, record missing producer evidence under missing_files_or_surfaces and
write "Producer evidence: - none". Do not mark missing_files_or_surfaces as
none in this state. Do not run the producer while writing validation status.
```

```text
Clean-validation diff rule:
If validation status is clean, do not approve yet. First run exactly:
git diff --stat && git diff -- .
Only after diff output is visible may you write an APPROVE verdict. Do not
approve from validation status alone.
```

```text
Diff hygiene rule:
Clean validation is not enough to approve if visible diff evidence contains
unrelated, out-of-scope, forbidden, generated, test-only, broad formatting, or
unexplained changes. Before APPROVE, compare changed files and hunks in
`git diff --stat` and `git diff -- .` with the validated surfaces named in
validation-status. APPROVE only when every changed file and hunk is directly
tied to those surfaces. Otherwise write BLOCK with the out-of-scope file path.
```

```text
Validated-surface requirement rule:
After clean validation and visible diff evidence, APPROVE only if
validation-status names concrete validated_surfaces that include every changed
production/config/source file. If validated_surfaces is missing, empty, none,
vague, or does not name a changed file, write BLOCK even when validation passed
and the diff looks relevant. Do not infer validated surfaces from test command
names or diff paths.
```

```text
Generated-diff producer evidence rule:
Generated or derived file diffs can approve only when validation-status
contains successful producer evidence naming the producer command with
EXIT_STATUS: 0. If generated/derived files changed and producer evidence is
missing, none, failed, unknown, or lacks EXIT_STATUS: 0, write BLOCK even when
validation passed and generated files are listed in validated_surfaces. Do not
infer producer success from generated file contents or validation commands.
```

```text
Decision:
BLOCK
```

```text
Decision:
APPROVE
```

## Implementation Defaults

- Prefer latest-message control footers for long phases.
- Prefer phase resets between personas over carrying long local transcripts.
- Put exact artifact templates in prompts whenever an event parser depends on
  file text.
- Use "the final line of the same shell script must be
  `echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`" when a phase must stop after
  writing its artifact.
- Pair that sentinel rule with "Do not echo
  `COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT` after a read-only inspection command"
  so the boundary does not leak into first-inspection turns.
- Do not expose FSM/runtime jargon to the model.
- Do not design personas by role name alone. Each persona is an artifact writer
  with explicit input variables, output file, stopping condition, and forbidden
  next-action classes.

## Latest-Control Footer Schema

Use this only when a phase carries more than a short local transcript. For
short phases, prefer a fresh phase with only the persona prompt and required
handoff artifacts.

```text
Current <persona/action> control:
state_variable_name = <value>
state_variable_name = <value>
required_artifact = <path>

Required next action:
<one exact command class or one exact artifact-writing action>

Forbidden next actions:
- <specific class, e.g. repository inspection>
- <specific command family, e.g. grep/search>
- completion sentinel after read-only inspection

Decision rule:
<if state variable condition, write required_artifact with Decision: BLOCK now>

Completion rule:
The final line must be echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT only when the
same shell script has just written required_artifact.
```

Replay-proven examples:

```text
Current next-action control:
No source path is named. Required first command is exactly:
rg -n "authentication.kubernetes.cleanup.grace_period" . --glob "internal/config/**" --glob "config/**" --glob "testdata/**" --glob "**/*test*" --glob "**/*schema*"
Respond with exactly that command in one fenced bash block. Do not add head,
tail, fallback commands, runtime/proto/generated/dependency search, completion
sentinel, or a second command.
```

```text
Current review control:
validation_was_run = false
exact_focused_validation_command_available = false
required_artifact = /tmp/pragma/item-verdict.md
Decision rule: write required_artifact with Decision: BLOCK now. Do not inspect
repository files. Additional grep/search cannot satisfy validation acceptance.
The final line must be echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT.
```

Surface-mapper after direct read:

Current surface mapper continued into another `rg` even after a task-named source
file had already been directly read. A task/source conflict rule by itself also
kept searching. The retained combined rule says that visible task-named source
content is enough to write `/tmp/pragma/surface-map.md` now, and task prose
values are investigation targets rather than source evidence when direct
inspection shows different values. Exact shell-shape wording wrote the artifact
but lost useful unknowns, so it is not retained.

Evidence-mapper conflict surface completion:

Current evidence mapper correctly consumes the conflict-shaped surface map from
the surface-mapper control above. After visible source/test inspections, it
writes `/tmp/pragma/evidence-map.md`, records source and consumer/defaulting
paths, and keeps unresolved references under `Not evidenced`; extra conflict
preservation wording was redundant.

Evidence-mapper generated/runtime unknown completion:

Current retained evidence mapper can still continue inspecting generated/runtime
unknown states even after source/defaulting, fixture/test, and focused
validation evidence are visible. Reminder-only and latest field-packet controls
write the evidence map and keep runtime/proto unknowns under `Not evidenced`.
Static semantic completion, shell shape, state variables, and a retained
last-mile footer were not stable enough for `evidence_mapper.yaml`; do not
retain extra persona text from this replay.

Patch-planner conflict evidence route:

Current patch planner correctly consumes the conflict-shaped evidence map from
that path. It writes the bounded source route for the evidenced source/test
surface and keeps unrelated `Not evidenced` references unresolved. Extra
not-evidenced nonblocking and conflict-source route wording was redundant in
replay.

```text
Current source-edit control:
source_file_visible = true
exact_source_change_visible = false
producer_command_attempted = false
required_artifact = /tmp/pragma/implementer-report.md

Required next action:
write the implementer blocker report

Forbidden next actions:
- grep/find/source search
- generated-file inspection
- producer_command
- validation
- source patch

Decision rule:
When source_file_visible is true and exact_source_change_visible is false,
write required_artifact with a Blocker section now.
```

Exact source changes present:

When `exact_source_changes` are present in the current item and the source file
is visible, the current item-worker source-before-producer policy is enough. In
replay, it patched only the named non-generated source file and did not run the
producer, inspect generated files, validate, or write the report prematurely.
Do not add redundant static exact-source wording unless a future replay shows a
failure.

Simple source value edits:

When a checklist item title/description/approach all name one exact symbol/value
change and the allowed source file visibly contains the old value, current item
worker makes the bounded source edit even without an `exact_source_changes`
field. Extra source-edit-from-item-text wording was redundant in replay.

Post-source-edit validation:

After a successful allowed source edit, current item worker ran the focused
validation command but used weak status capture: `go test ...; echo
"EXIT_STATUS: $?"`. The retained post-source-edit validation shell rule forces
the same `set -o pipefail`, log capture, `${PIPESTATUS[0]}`, and
`exit "$status"` wrapper used for validation-only items. Latest-message command
packet replay refined the boundary: full wrapper and YAML command packet both
produce exact status-preserving scripts across validation-only and
post-source-edit states. Packet-placement replay showed post-source-edit now
passes even without a packet because of the retained static rule. Later
validation-only wrapper noise replay showed the older static wrapper still
failed under "quick/no extra logging" reminders by omitting `log=...` and
`tee`, while the retained exact-wrapper gate passed clean and noisy checked-in
YAML replays by treating log/tee as required evidence capture rather than
optional logging. Field lines preserve status semantics but can compress the
script; condition-only state is unreliable.

Post-source-edit report:

After the retained wrapper produces focused validation output with ok/PASS and
`EXIT_STATUS: 0`, current item worker already writes the implementer report with
the changed source file, clean validation, no remaining risk, and no blocker.
Extra changed-file report wording was redundant in replay. A passed-validation
report retake confirmed the checked-in item worker writes `EXIT_STATUS: 0`,
`Remaining risk: none`, `Blocker: none`, and the sentinel after visible PASS
output, including under concise-summary noise.

Source preservation:

When editing a visible source file, preserve every unrelated visible line and
declaration exactly, and patch only the smallest source region required for the
current item. This belongs beside patch minimality in the persona prompt. In
replay, the static rule removed a post-patch read and produced a bounded edit;
a latest-message preservation footer still allowed a full-file rewrite. Transfer
replay on Go source showed the retained rule prevents full-file rewrites and
post-edit reads. Exact struct insertions needed a separate last-mile edit table
plus command-shape hint: a semantic exact-insertion gate and an edit table alone
still invented adjacent comments, but the table plus a command-shape ban on
`//`, `/*`, blank inserted lines, and non-requested inserted text produced only
the requested line. The same retained footer preserved the simple visible value
replacement behavior.

Post-source-patch producer handoff:

After the source patch is visibly present, the item worker needs a local rule to
run the named producer with the full status wrapper. The earlier generated-file
policy moved to the producer, but replay showed it could omit `exit $status`;
the local transition rule fixed that without a latest-message footer.

Failed focused-validation stop:

Do not encode this as static persona text. Replays show the model continues
toward test/source inspection after visible failed validation even when the
persona prompt contains terminal failure wording and an exact failed-report
shell shape. The reliable controls are latest-message materialization messages:
the full exact shell script passes, and the less brittle field-lines packet
also passes across two failed-validation item shapes. Condition-only state and
slot-template wording are still too weak because they return to file
inspection. Packet-position replay refined the boundary: the field-lines packet
works when appended directly to the failed validation output, sent as a separate
final message, or sent after short aligned reminder noise. It is not reliable
when buried before later reminders; one transfer case returned to source
inspection. For failed-validation stop, keep the report packet latest or attach
it directly to the failed validation output.

Repeated-search stop materialization:

Repeated no-new-information stop is more tolerant once the stop state is
supplied. Placeholder report packets preserve exact handoff fields when
appended to the current-item/search-state output, sent separately, sent after
short aligned reminder turns, or buried before those reminders. Reminder-only
wording writes reports but rewrites validation, risk, and blocker fields, so
use a packet whenever exact downstream fields matter. This differs from
failed-validation stop, where a buried packet failed one transfer case.
Retake against checked-in `item_worker.yaml` confirmed the retained static
footer still stops both config-symbol and proto/generated repeated-search
states even when the search state is buried behind aligned reminders; it did
not reopen files, search, validate, or edit. The static buried-state proto case
still rewrote validation to `N/A - blocked before implementation`, so treat the
static footer as a robust stop-and-report control, not a field-perfect handoff
control. Placeholder report packets remained field-perfect both buried before
aligned reminders and latest after them.

Item-reviewer contradictory validation:

Current item reviewer already blocks when raw validation text contains
`--- FAIL`, even if the same implementer report claims PASS and
`EXIT_STATUS: 0`. Extra contradiction-priority wording was replayed and found
redundant.

Item-reviewer changed-file auditability:

Validation success is not enough when `Changed` is missing or prose-only. The
reviewer must block unless it can compare concrete repository file paths against
`allowed_files` and `forbidden_files`. Replay showed the current prompt hit
`finish_reason: length` on vague "related local files"; the auditability rule
produced a small BLOCK verdict. Minimality replay confirmed this rule is still
essential: current retained passes, removing only the auditability rule brings
back the length-loop, and removing both scope rules approves incorrectly.
Visible-pass scope wording alone is not enough for vague Changed prose.

Item-reviewer no-change implementation report:

For items that ask to edit/change/add/remove/update/fix repository code or
config, `Changed: No files changed` blocks even when validation is green.
Replay showed the current reviewer recognized the problem but wrote prose and
tried repository inspection; the static no-change rule wrote the verdict
artifact directly.

Item-reviewer no-change validation-only report:

The no-change blocker must stay scoped to implementation items. A
verification-only item with `Changed: No files changed`, exact focused
validation, PASS/ok output, and `EXIT_STATUS: 0` approved correctly. Broader
no-change blocker wording was redundant or ignored and is not retained.
Failed validation-only reports already block through the existing visible
failure and nonzero validation status rules; no extra validation-only failure
review rule is retained.

Item-reviewer approval materialization:

For clean source-edit approval and clean out-of-scope repair approval, current
item reviewer already writes compact APPROVE verdicts through the generic
visible-pass and auditability rules. Latest-message field lines, YAML packets,
and slot templates also preserve exact three-finding APPROVE verdicts in these
low-conflict approval states. Condition-only approval state also passes shape
and decision, but rewrites findings more generically. Packet-position replay
showed field-lines approval packets are placement-tolerant when appended,
separate, latest after reminders, or buried before short aligned reminders.
No-packet reminder-only approval still writes APPROVE with three findings, but
rewrites all findings generically. Use packets only when exact findings are a
downstream contract. Do not add source-edit or remove/revert-specific approval
prose; earlier semantic additions regressed strict verdict size.

Item-reviewer completed-item no-op:

Completed-item status must be terminal across both worker and reviewer. Current
reviewer blocked a completed no-op implementer report as missing changed-file
proof and validation evidence; the retained completed-item review rule approves
when status is completed and the no-op report has no blocker.

Item-worker active process handling:

Tool output saying a validation command is "still running after 30s" is not a
validation timeout. Current item worker treated that as a failed timeout and
wrote a blocker report. The retained bounded-status rule made the next action a
single bounded check of the already-started process and captured log, avoiding
both duplicate validation and unbounded `tail -f`.

Item-worker active process exactness:

The bounded-status rule alone still allowed a broader `go test ./cmd/pragma/...`
process to suppress the required focused `go test ./cmd/pragma/... -run
TestFocusedCLI` command. A generic exact-match footer was also too weak. The
retained last-mile decision table tells item worker to compare the active
process command text to the current item command before waiting. It passed the
exact-running case and the mismatched broader-process case only after the table
also prohibited prose before the bash block.

Decision-card placement replay showed the already-computed exact/mismatch
branch is robust when appended to process output, sent separately, latest after
reminders, or buried before short aligned reminders. Unlike validation-runner,
aligned reminder-only wording did not correct the item-worker mismatch case; it
still waited on the broader process. This supports the retained table rather
than relying on semantic compare reminders.

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

Item-worker missing current item:

Current item worker already handles a missing `/tmp/pragma/current-item.json`
by writing a blocker implementer report with no repository inspection, no
edits, and no validation. Extra missing-current-item prompt rules were
redundant in replay and are not retained.

Item-reviewer missing implementer report:

Current item reviewer already blocks when `/tmp/pragma/current-item.json` is
readable but `/tmp/pragma/implementer-report.md` is missing. It writes a BLOCK
verdict from missing review evidence without inspecting the repository or
running validation; extra missing-report rules were redundant.

Item-reviewer source-edit approval:

Current item reviewer approves a clean source-edit implementer report when
`Changed` names only allowed files, the exact focused validation command passed
with `EXIT_STATUS: 0`, and risk/blocker are none. Extra source-edit approval
wording was redundant, and a semantic rule made the verdict less compact.

Final-reviewer validated surfaces:

The final reviewer must not infer coverage from a test command name or a
relevant-looking diff. When `validated_surfaces` is `none`, missing, vague, or
does not include a changed production/config/source file, final review blocks
even if validation passed and the diff looks correct.

Final-reviewer generated diffs:

Generated/derived diffs require two proofs: the changed generated files must be
covered by `validated_surfaces`, and producer evidence must name a successful
producer command with `EXIT_STATUS: 0`. Validation commands and generated file
contents are not proof that the generator ran. Failed/nonzero producer evidence
already blocks once generated diff evidence is visible; extra producer-status
authority wording was redundant. Missing producer evidence also already blocks
for generated diffs; extra missing-producer authority wording leaked prose and
is rejected.

Final-reviewer missing validation status:

Current final reviewer already blocks when `/tmp/pragma/validation-status.md`
is missing after the required first read. Extra missing-status prompt rules were
redundant in replay, and a generic missing-input rule slightly worsened verdict
formatting.

Final-reviewer source-edit clean diff:

Current final reviewer approves a clean source-only diff when validation status
names the changed source file, all failure/missing fields are `none`, and the
visible diff changes only that validated source path. Producer evidence is not
required for non-generated source-only diffs, and earlier unresolved proof does
not matter unless it appears in validation status or visible diff evidence.

Validation-runner surface fields:

Final review depends on validation-status carrying explicit surfaces. Replay
showed the current validation runner omitted `validated_surfaces`; adding only a
rule produced surfaces but still omitted `Producer evidence`. The retained
status format includes both fields.

Validation-runner producer evidence:

When successful producer evidence is visible in patch plan/checklist, the
current validation runner carries it into `Producer evidence` after the status
format includes that field. Extra producer-evidence prompt rules were replayed
and found redundant.

Validation-runner missing producer evidence:

For generated-file routes, `Producer evidence: none` must also make
`missing_files_or_surfaces` non-clean. Replay showed a simple missing-evidence
rule caused the model to run the producer while writing status; the retained
completion rule frames this as status extraction instead.

Validation-runner failed producer evidence:

The same generated producer completion rule covers nonzero producer evidence.
Replay with `buf generate; EXIT_STATUS: 1` kept status non-clean and preserved
the failed producer evidence; extra failed-producer wording was redundant.

Validation-runner active process handling:

When a required validation command is still running after the tool wait window,
current validation runner avoided a false status artifact but chose `sleep 60`,
which delays progress without checking the process or captured log. The retained
bounded-status rule made it check the same pid and log with
`sleep 10; ps -p <pid>; tail -n 100 <log>`, avoiding duplicate validation,
premature status writing, and unbounded log following.

Validation-runner active process exactness:

Current validation runner and a deliberately loose active-process rule both
status-checked a similar broader `go test ./cmd/pragma/...` process even though
the required focused command had not been started. The exact-match gate passed
only as a persona footer, not immediately after the active-process rule. The
retained footer says the active-process rule applies only when the latest tool
output is the direct result of the command just started and the active process
command text exactly matches the required command.
Decision-card placement replay showed the already-computed exact/mismatch
branch is robust when appended to process output, sent separately, latest after
reminders, or buried before short aligned reminders. In this lower-conflict
command-comparison state, aligned reminder-only wording also corrected the old
mismatch failure, though wrapper shape varied between indexed and direct
status-preserving scripts.

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

Validation-runner source-edit chain command:

For the simple source-edit chain, current validation runner already runs the
exact focused validation command with status preservation after reading patch
plan and checklist. It used indexed `COMMAND[1]`/`EXIT_STATUS[1]` for the single
command; that is compatible with the existing indexed status-writing rule, so
extra single-command wording is not retained.

Validation-runner source-edit status:

After indexed validation output with `EXIT_STATUS[1]: 0`, current validation
runner writes clean validation status with concrete validated surfaces and
`missing_files_or_surfaces: none`. A mutation that mentioned unresolved proof
regressed by copying unrelated `Unresolved until evidence` into missing
surfaces, so that wording is rejected.

Checklist producer evidence:

Generated-file patch routes need producer evidence preserved before validation
status can carry it. Current checklist writer dropped `producer evidence`;
minimal handoff wording preserved `producer_evidence`, while extra schema text
regressed and was rejected.

Checklist missing handoff artifacts:

If only `/tmp/pragma/patch-plan.md` is missing, checking for an optional final
verdict is reasonable because repair mode may still have an authority artifact.
When both patch plan and final verdict have been checked and neither is
readable, current checklist writer wrote an empty `items` array. The retained
missing-input rule makes it write one blocked checklist item with empty
`allowed_files`, `forbidden_files: ["**/*"]`, and no repository inspection.

Checklist patch-route roles:

Current checklist writer over-expanded a source-owned route: it made a source
edit item, a consumer/test edit item, and a blocked unresolved-proof item. The
retained route-role/unresolved-separation rule made one pending source item,
preserved the focused `validation_command`, and kept unresolved proof out of
initial implementation scope. Exact-source-change handoff was not stable in
this replay and is not retained.

Patch-planner validation-only evidence:

Current patch planner could write a focused validation route, but it incorrectly
used the test file as `source path` and added an unresolved source-edit route
when the evidence map explicitly had no source edit path. The retained
validation-only route rule made it write `source path: none`, preserve the exact
focused validation command, and avoid invented source work.

Patch-planner source-change missing evidence:

The validation-only route rule can overfire when the task intent is a source
change but the evidence map has no evidenced source path. Current patch planner
did not promote the surface-map source path directly, but it still created a
validation-only/pre-validation route from the consumer path and validation
command. A source-change gate inserted near the validation-only rule still
failed, but the same control as a persona footer wrote `Patch route:\n- none`
and preserved the validation-only route behavior in the separate verify/check
case. The retained footer also requires verbatim proof copying; without that
addition, a checked-in replay fixed the route but rewrote the proof text. Later
noisy-placement replay showed the first retained footer could still fail with a
length-loop when a late reminder reframed the state as verify/check/audit. The
stronger retained footer qualifies validation-only routing by task intent and
explicitly outranks later verify/check/audit reminders; checked-in replays
passed both the noisy source-change case and the true validation-only verify
case.

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

Patch-planner producer evidence:

Patch planner also dropped producer evidence until given an exact-copy gate from
evidence-map `Producer evidence:` to patch-plan `producer evidence:`. Generic
route-field wording and format-only changes failed.

Patch-planner compatibility with evidence-map generated sections:

After evidence mapper gained explicit `Evidenced generated/derived paths`,
`Producer command`, and `Producer evidence` sections, current patch planner
still preserved source, generated files, producer command, producer evidence,
and validation on one route. Adding more allowed-scope wording was redundant
in replay and is not retained.

Checklist repair missing producer evidence:

For repair passes where final review blocks generated/derived changes only
because producer evidence is missing, the current checklist writer already
creates one bounded pending producer item from the matching patch route. It
preserves `producer_command`, generated files, validation command, allowed
files, and forbidden runtime scope. Additional repair-producer and route-match
rules replayed as redundant, so they are not retained.

Checklist repair out-of-scope diff:

For repair passes where final review blocks on a concrete out-of-scope changed
file, the current checklist writer already preserves completed prior items and
adds one pending repair item scoped to that file. Do not add separate
out-of-scope repair wording yet; it was redundant. Do not add broad
completed-item preservation wording in the tested form; it can put a preserved
completed item into the pending repair item's `forbidden_files`, which is
misleading for downstream item worker.

Checklist repair no-diff contradiction:

For repair passes where final review blocks because a prior no-diff repair item
was marked completed but the final diff still contains that same file, the
current checklist writer already preserves unrelated completed items and rewrites
the failed repair item as `pending`. It preserves the concrete file as
`allowed_files`, keeps `forbidden_files` empty, and copies the exact
`git diff --exit-code -- <path>` validation command. Extra completion-override
and no-diff repair wording was redundant in replay, so it is not retained.

Item-worker out-of-scope repair start:

Given that bounded repair item, current item worker already starts by reading
the exact allowed-file diff with `git diff -- <path>`. After the diff shows a
newly-added out-of-scope file, it removes only that file, runs the exact
validation command with the status-preserving wrapper on the next turn, and
writes a clean implementer report after `EXIT_STATUS: 0`. Extra wording for
diff inspection, same-script validation, post-remove validation, and repair
reporting was redundant in replay, so it is not retained.

Item-reviewer out-of-scope repair approval:

Current item reviewer already approves a clean remove/revert repair report when
the changed file is concrete, inside `allowed_files`, outside
`forbidden_files`, and the exact validation command has `EXIT_STATUS: 0`. An
explicit remove/revert approval rule replayed as weaker because it violated the
strict three-finding limit. Latest-message materialization with field lines,
YAML packet, or slot template can preserve the compact three-finding APPROVE
shape, but it is optional here because current generic reviewer rules already
pass.

Validation-runner out-of-scope repair validation:

Current validation runner already executes both the original focused validation
command and the completed repair item's `git diff --exit-code -- <path>`
command with indexed statuses. Status writing had a gap: current prompt and
several semantic/mechanical insertions omitted the repair diff path from
`validated_surfaces`. The retained fix is a last-mile persona footer that
parses visible `Commands run` lines and copies successful git-diff paths into
`validated_surfaces`. The same rule worked as a latest-user footer, failed as a
system footer, and failed when inserted earlier near related status rules.
Later noisy-placement replay showed the retained footer is not enough when a
later reminder says to focus on implementation/original surfaces or ignore
repair-only cleanup paths. A stronger static footer with explicit precedence
language still failed. The reliable control in that state is a latest exact
status-extraction line naming the successful `git diff --exit-code -- <path>`
command and requiring `<path>` under `validated_surfaces`; no additional
`validation_runner.yaml` wording is retained. A later position retake using the
same noise shape found current live replay stronger: the checked-in prompt kept
the repair path in four fresh noise-only responses, and exact section,
override, and full-artifact materializations survived even when placed before a
later conflicting repair-only reminder. Treat the static footer as currently
passing, with exact materialization still preferred when the artifact must be
high-assurance under conflicting surface-prioritization text.

Final-reviewer out-of-scope repair approval:

Current final reviewer already approves the clean end state after out-of-scope
repair when validation status is clean, all commands have `EXIT_STATUS: 0`, and
the visible final diff contains only changed files covered by
`validated_surfaces`. A repaired surface validated by `git diff --exit-code`
does not need to remain in the final diff. Extra final-review wording for that
case replayed as redundant.

Final-reviewer dirty repair diff block:

When validation status contains a successful `git diff --exit-code -- <path>`
repair command but visible final diff still contains `<path>`, current final
reviewer can get stuck in hidden reasoning and finish with no verdict. Multiple
persona-body, persona-footer, system, output-first, and exact-template variants
either looped, emitted prose before the bash block, or produced weak findings.
Hardcoded persona-footer wording could pass one dirty replay but is not
retainable. Generic persona-footer variants either required task-prose hinting
or failed the clean+dirty verification pair. The only clean replay remains a
precise latest-message control footer that tells it the successful no-diff
command means the path must have no final diff and to write BLOCK immediately.
Decision-precedence tables and first-match framing did not fix the persona
body; they either looped, emitted prose before the bash block, hallucinated
producer-evidence requirements, or approved because the dirty file was listed in
`validated_surfaces`. No `final_reviewer.yaml` change is retained yet; this
needs a latest-message decision footer pattern rather than another broad
persona paragraph. Later transfer tests showed that a fully generic latest
footer and a variable-filled `No-diff validation path` footer were still
insufficient: clean approval survived, but dirty cases either looped or blocked
for the wrong producer-evidence reason. The next prompt-control direction is an
exact generated verdict from extracted variables, not another semantic
reminder. Follow-up replay refined this further: exact generated verdict text
was unreliable when appended to the original task message, but reliable across
two dirty paths when sent as a separate final user message after diff evidence.
The likely mechanism is an isolated latest decision message, not a persona YAML
paragraph. Minimality replay showed that the isolated message still needs an
explicit imperative to write the artifact; a bare fenced shell block can trigger
reasoning, and a decision summary before the block can make the model try to
read the artifact instead of writing it. A generated-file missing-producer
replay showed the opposite boundary: current final reviewer already handles that
ordinary rule-match case, and even a bare isolated block works. The isolated
decision-message mechanism is primarily for high-conflict contradiction cases,
not every final-review BLOCK. Command-status contradictions are also already
handled by current final reviewer; nonzero `EXIT_STATUS[n]` blocks even when
summary fields incorrectly say `none`.

Variable-message replay refined the latest-message mechanism further.
Condition-only extracted state is still too weak and can loop or pick the wrong
blocking rationale. YAML-like decision packets are also too interpretive: they
can drift into validation-tool repair or generated/producers. The transferable
prompt primitive is exact field lines plus a shell-wrapper instruction: provide
`output_file`, two exact `findings_line_*` values, exact `required_repair`,
`decision`, and `event_marker`, then instruct the model to write exactly those
fields and add no other reason. This passed across the dirty auth and dirty
cache repaired-path contradictions without a fully prewritten bash block.
Follow-up transfer replay showed a slightly less brittle retained form: a
placeholder verdict pattern with only `repaired_path` and `no_diff_command`
variables also passed both dirty paths. Looser decision cards, even with
good/bad examples, either looped to `finish_reason: length`, drifted into
producer/generated reasoning, or softened the exact no-diff contradiction into
generic contradiction wording. The reliable control is therefore not "tell the
reviewer the state"; it is "give the reviewer a verdict pattern and variables
to materialize." A later static-transfer replay moved literal-match,
code-only, and placeholder-pattern branches into the persona footer while
preserving the original final-reviewer transcript. Clean controls still
approved, but every dirty auth variant and two dirty cache variants looped to
`finish_reason: length`; the lone dirty-cache BLOCK did not generalize. A
later literal-gate retake added explicit "do not reason about git semantics"
and artifact-first fallback wording. It again failed the verification pair:
one artifact branch fixed both dirty paths but leaked prose on clean approval,
while the clean-safe variant passed clean auth and dirty cache but still looped
on dirty auth. Do not retain this literal-gate family in `final_reviewer.yaml`.
Follow-up position and noisy-history replays refined the boundary: the
placeholder packet can be a separate final user message, appended to the
diff-output turn, prepended to that turn, or introduced after diff evidence and
then followed by short aligned reminder turns. Reminder-only wording still
failed. What matters is conversation-level materialization after validation and
diff evidence are visible, not message separation or literal latest-turn
placement. Keep the placeholder pattern as an orchestration primitive, not as
`final_reviewer.yaml` body text. A later materialization retake found current
checked-in final reviewer no longer length-loops on the dirty paths, but it
still blocks for unvalidated/debug or producer/debug-code reasons instead of
the no-diff contradiction. Exact field-lines and placeholder packets both
produce the correct no-diff contradiction verdict across dirty auth and dirty
cache; clean auth still approves without a packet. A conflicting-rationale
noise retake tightened the placement rule: if a later reminder says to
prioritize debug/generated/producer rationale, a packet buried before that
reminder can be partially remembered but still loses the exact no-diff repair.
In that state, the exact field-lines or placeholder packet must come after the
conflicting rationale noise.
A further static selector/trigger retake rejected four more persona-footer
families. Literal selector wording passed clean auth, dirty auth, and auth
conflict, but length-looped dirty cache and added debug/producer rationale in a
cache conflict. Output-shaped branch wording passed clean auth, dirty cache,
and both conflicts, but approved dirty auth because the file was listed in
`validated_surfaces`. Shorter hard-trigger and fixed-verdict branches increased
length loops and still drifted into debug/producer rationale. This reinforces
the boundary: the reliable primitive is not static final-reviewer persona text,
but exact conversation-level verdict materialization after the contradiction
and any conflicting rationale noise are visible.

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

Equivalent passing placeholder-pattern primitive:

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

Patch-planner unresolved-source materialization initially showed the same
pattern at a planning boundary. When the source change is already classified as
unresolved, the model can reliably write the route-none patch plan from a
concrete section template with `goal`, `unresolved_behavior`, and
`unresolved_proof_required` variables. A route decision card is too loose
because it can turn `- none` into scalar `none`, and good/bad examples allow the
proof text to be rewritten. A later source-change-vs-validation replay found a
static footer that resolves the same conflict in `patch_planner.yaml`; keep the
latest-message template as an orchestration primitive for already-classified
states, not as the only available control. Packet-position replay showed the
placeholder route-none packet is placement-tolerant when appended to the map
output, sent separately, sent after reminders, or buried before short aligned
reminders. Reminder-only wording can avoid the old validation-only route but
still emit `Patch route:\nnone` and rewrite behavior text, so it is too weak
for downstream checklist parsing.

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

Item-worker exact source insertions initially looked like a latest-message
materialization boundary. Plain insertion variables and static "add exactly one
line" wording were too weak and invited an invented nearby comment. A source
edit decision card, a unified hunk pattern, or minimal good/bad examples with
the concrete bad behaviors named all passed as latest-message controls. A later
static replay found the transferable persona control: a last-mile source edit
decision table plus an exact insertion command-shape hint. The table alone still
invented a comment, so the command-shape ban is part of the retained primitive.
Noisy placement replay found the retained static primitive is not immune to a
later style/comment reminder: without an exact packet, a reminder that
surrounding Go fields had comments caused an invented `// Backoff...` line.
Exact insertion packets appended to source output, buried before short reminder
noise, latest after reminder noise, and exact command packets all preserved the
one-line edit. A later static hardening replay retained a narrower
style-precedence rule for exact one-line insertions: exact inserted line
count/text now outrank style, readability, surrounding comments, and
consistency reminders. Checked-in replay passed both the clean and comment-noise
exact-insertion cases.
Good/bad examples are therefore not generally bad; they are weak for exact
report-field materialization but can be useful for local source-edit discipline
when the forbidden behaviors are specific and observable.

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

Exact insertion style-precedence rule:
If the current item says to add or insert exactly one source line after an
anchor, the exact inserted line count and text outrank style, readability,
surrounding comments, and consistency reminders. Do not add a comment, blank
line, import, formatter run, validation, report, or any inserted text beyond
the requested line, even when nearby fields have comments. The edit command
must insert only the requested line.
```

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

Evidence-mapper producer evidence:

Evidence mapper needed the upstream conflict resolved before downstream
producer-evidence handoff could be reliable. The current prompt naturally wrote
`Producer command evidence:`, which patch planner does not consume. Simple
producer-evidence wording failed; the retained control distinguishes
uninspected generated/proto surfaces from inspected adjacent generated/derived
evidence, then requires exact `Producer command:` and `Producer evidence:`
sections.

Evidence-mapper generated unknowns completion:

When source/defaulting, fixture/test, and focused validation evidence are
visible but runtime/proto/generated paths are explicit unknowns, static persona
text still sometimes reopens fixture/test inspection instead of writing the
artifact. A short latest reminder and latest field packets reliably write
`/tmp/pragma/evidence-map.md` with explicit unknowns under `Not evidenced`.
Moving the same reminder or a stronger output-shape gate into
`evidence_mapper.yaml` passed only as isolated mutations; checked-in YAML replay
still read fixture/test paths again. Do not retain another static
`evidence_mapper.yaml` phrase for this state yet. A later visible-evidence
retake found a static gate that stopped the repeated read, but it changed the
validation command shape; a stricter command-template gate regressed to reading
the fixture again. The latest field packet remains the reliable control in
ordinary generated-unknowns states because it provides exact output fields and
values at the decision point. Under stronger uncertainty pressure that says to
verify runtime/proto/generated unknowns again, the short reminder and earlier
compact field packet failed; the negative-only correction stopped reads but
corrupted the validation command spelling. A later materialization-shape retake
found that full artifact text is not the only reliable control: latest decision
cards, section templates, and field lines also pass when they explicitly bind
the next action to `write_artifact` and state that the validation command is
artifact text, not a command to execute. Static-only artifact-first/copy gates
still re-read fixtures or grepped runtime/proto paths, so no
`evidence_mapper.yaml` change is retained.

```text
Evidence mapping reminder: the required evidence is already visible. Do not
read more repository files. Write /tmp/pragma/evidence-map.md now and put
explicit unknown runtime/proto/generated surfaces under Not evidenced.
```

## Immediate Next Build

Create a new persona directory rather than editing the failed persona set in
place:

```text
personas-research-v2/
```

Initial YAMLs:

- `surface_mapper.yaml`
- `evidence_mapper.yaml`
- `patch_planner.yaml`
- `checklist_writer.yaml`
- `item_worker.yaml`
- `item_reviewer.yaml`
- `validation_runner.yaml`
- `final_reviewer.yaml`

The first runnable experiment should be a short-phase orchestration:

```text
surface_mapper -> evidence_mapper -> patch_planner -> checklist_writer
-> item_worker/item_reviewer loop -> validation_runner -> final_reviewer
```

Do not run it on the full benchmark until the opening phases are replay-tested
against captured payloads.

## V2 Replay Status

Validated with direct Lilac/Minimax replay:

| Persona | Validated Behavior | Status |
| --- | --- | --- |
| Surface Mapper | first action directly reads task-named source path from real Flipt task; after visible task-named source output it writes the surface map instead of searching again and records task/source value conflicts as evidence/unknowns; no-path fallback uses exact generic literal-anchor `rg -n`; no-path artifact separates task-named and candidate paths | Passed |
| Evidence Mapper | first action reads `/tmp/pragma/surface-map.md`; writes evidence map from listed candidate/source surfaces in ordinary/conflict states; inspected adjacent generated/derived paths and visible successful producer evidence are preserved in exact sections; generated/runtime/proto unknowns completion still needs latest orchestration to stop repeated fixture/test reads: field packets are reliable in ordinary states, while late uncertainty-pressure states require latest write-bound materialization such as decision cards, section templates, field lines, or full artifact text that mark validation commands as artifact text rather than commands to run | Partial |
| Patch Planner | first action reads surface and evidence maps; after-input turn writes patch plan from artifacts without reopening source files; artifact precedence and uncertainty rules keep `Not evidenced` surfaces unresolved; generated producer metadata and producer evidence stay on one route; validation-only evidence creates a no-source validation route; source-change behavior with no evidenced source path writes `Patch route: - none` and copies the proof-required text verbatim through the retained source-change footer, including under late verify/check/audit noise | Passed |
| Checklist Writer | first action reads verdict/patch plan; artifact turn writes narrow JSON checklist with array fields and sentinel; missing handoff artifacts and missing fields produce hard blocker items; source-owned routes create one source item while consumer/defaulting paths stay validation context and unresolved proof stays out of initial scope; pending item `forbidden_files` use only exact paths/globs and stay empty when only prose unsafe shortcuts are present; generated producer metadata and producer evidence are preserved on the same patch-route item; repair passes preserve completed items exactly and add bounded pending repair items for missing producer evidence or out-of-scope changed files, with repair target scope retained under completed-file protection noise | Passed |
| Item Worker | first action reads current item; repo-relative allowed files are not rewritten under `/tmp/pragma`; completed items write no-change reports without inspection; validation-only items run exact focused validation with status preservation, including under late quick/no-logging reminders through the retained exact-wrapper gate; still-running validation/producer processes get one bounded status/log check only when the active process command exactly matches the current item command, and broader similar processes are ignored in favor of the exact status wrapper; source-plus-generated items inspect source before producer, exact source changes patch only the named non-generated source file first while preserving unrelated visible context, visible source patches hand off to the exact producer status wrapper, successful source edits run focused validation with the exact status-preserving wrapper, producer-only generated targets run producer exactly, and successful producer output proceeds to focused validation; out-of-scope repair items inspect the exact allowed-file diff, remove only the concrete out-of-scope file, validate with the exact wrapper, and write a clean report; malformed item, missing producer, failed producer command, and repeated no-new-information search states write blocker reports without more inspection; minimal one-file edits and exact one-line insertions avoid invented adjacent comments through the retained source-edit table, command-shape hint, and style-precedence rule | Passed |
| Item Reviewer | completed no-op reports approve without rerun; visible validation failure writes item BLOCK verdict, including contradictory reports that also claim PASS; visible PASS writes APPROVE only after concrete changed-file scope passes; clean out-of-scope remove/revert repair reports approve without rerun; repeated failed-command reports stay compact and non-reproductive, including under detail-request noise; vague Changed prose and no-change implementation reports block; grep-only evidence does not approve; missing validation with exact command runs exact command; missing validation with no exact command writes BLOCK | Passed |
| Validation Runner | exact status-preserving wrapper after inputs; still-running validation processes get one bounded status/log check instead of premature status, duplicate command, or idle sleep; similar broader active processes are ignored unless the active process command text exactly matches the required command, with the exact-match gate retained only as a footer; focused and indexed multi-command validation failures write `failed_tests` and `failed_packages`, including concise-noise retake coverage for `failed_packages: ./cmd/pragma`; successful validation status includes validated surfaces and carries visible producer evidence, with the validated-surfaces retake passing under concise-summary noise; generated routes record missing producer evidence as a missing surface; repair diff validation commands run in the indexed multi-command wrapper, and successful `git diff --exit-code -- <path>` commands are retained in `validated_surfaces` by the last-mile footer, with fresh repair-only surface-prioritization retake coverage for both current static behavior and exact materialization before/after conflicting reminders | Passed |
| Final Reviewer | failed validation status writes final BLOCK verdict and sentinel; skipped/partial validation writes BLOCK; clean validation requires diff evidence before APPROVE; changed production/config/source files require concrete validated surfaces; generated diffs require successful producer evidence; proof-only test diff blocks; validated producer-backed generated diff approves; clean out-of-scope repair end states approve when no repaired out-of-scope diff remains and every visible changed file is covered by validated surfaces; dirty repaired-path diffs remain an unresolved persona-body gap after static literal-match, code-only, and placeholder-pattern footer replays: current prompt blocks but can choose the wrong rationale, so the correct no-diff contradiction verdict still requires conversation-level materialization after validation/diff evidence, either exact field lines or a placeholder verdict pattern with variables; aligned reminders can follow the packet, but conflicting debug/generated/producer rationale noise requires the exact packet to be latest | Partial |

This does not prove benchmark performance. It only proves that the current v2
persona prompts obey the targeted control primitives in isolated replay cases.

Long-context replay also proves the inverse: when noisy local history is allowed
to accumulate, v2 controls at phase opening are not sufficient. The persona set
therefore depends on either short phases or latest-message control footers.

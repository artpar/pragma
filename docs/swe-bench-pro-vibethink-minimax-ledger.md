# SWE-bench Pro Minimax + VibeThink Ledger

This ledger records manual-first and state-contract evidence for
`docs/swe-bench-pro-vibethink-minimax-goal-prompt.md`.

## 2026-06-23 State Contract: `swe_repo_survey`

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T150252Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes, expected for a stop-after-state contract run.
- Orchestration evidence:
  `pragma.stdout.log` shows `swe_repo_survey` completed and no transition to
  `swe_acceptance_mapper` occurred.
- Artifact evidence:
  The stdout-captured completion command wrote `/tmp/pragma/swe/repo-survey.md`
  with task intent, repository orientation, candidate surfaces, validation,
  input matrix, generated artifacts, risks, and unknowns.
- Contract result: partial pass.
- Pass evidence:
  The state stayed read-only, named concrete repository paths, identified Go
  repo/auth/config/proto surfaces, listed likely validation commands, and did
  not produce a patch.
- Gap:
  The persona over-inspected with many separate read-only bash turns before
  writing the artifact. The survey also used implementation-step language such
  as "Add ..." in candidate surfaces, which conflicts with the State 1 contract
  that the repo survey should not propose code changes.
- Change made from this evidence:
  `personas-research-v2/swe_repo_survey.yaml` now requires the first response
  to write the artifact, forbids discovery-only command turns, discourages long
  one-file command streams, requires repo-relative paths, and bans candidate
  surface wording such as `add`, `create`, `modify`, `register`, `wire`,
  `implement`, or `fix`.
- Next evidence needed:
  Re-run the same stop-after-state command and require a full State 1 pass
  before composing into acceptance mapping.

## 2026-06-23 State Contract: `swe_repo_survey` Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T150737Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: fail.
- Pass evidence:
  The stop-after mechanism still worked: only `swe_repo_survey` ran and no
  patch was produced.
- Gap:
  The state wrote the artifact in one turn, but it did not run repository
  inspection commands before the artifact. It used generic/guessed surfaces
  such as `internal/authn/`, `internal/server/auth.go or similar`, and `ui/`
  or `client/`, which violates the grounding requirement for State 1.
- Change made from this evidence:
  `personas-research-v2/swe_repo_survey.yaml` now requires the first bash block
  to run repository probes before writing the artifact, names minimum bounded
  probes, and forbids guessed candidate paths or "or similar" surface wording.
- Next evidence needed:
  Re-run State 1 and require concrete command-grounded paths before proceeding
  to State 2 validation.

## 2026-06-23 State Contract: `swe_repo_survey` Grounding Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T150849Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: partial pass.
- Pass evidence:
  The artifact became concrete and command-grounded. It named observed paths
  such as `internal/config/authentication.go`, `rpc/flipt/auth/auth.proto`,
  `internal/server/auth/method/token/server.go`,
  `internal/server/auth/method/oidc/server.go`, `internal/server/auth/server.go`,
  `internal/server/auth/public/server.go`, `internal/server/auth/middleware.go`,
  and `internal/cmd/auth.go`. It stopped before the next state and produced no
  patch.
- Gap:
  The state still used many separate repository-read turns before writing the
  artifact, despite the first-response artifact contract.
- Change made from this evidence:
  `orchestrations/swe-bench-pro-engineering-loop.yaml` now sets `max_turns: 1`
  on `swe_repo_survey`, `swe_acceptance_mapper`, `swe_environment_survey`, and
  `swe_slice_planner`, matching their one-artifact first-turn contracts.
- Next evidence needed:
  Re-run State 1 with `max_turns: 1` active. It should either produce a
  command-grounded survey in one turn or fail loudly as a contract violation.

## 2026-06-23 State Contract: `swe_repo_survey` One-Turn Budget

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T151103Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail, but usefully loud.
- Evidence:
  `pragma.stdout.log` shows the first turn ran only
  `pwd && find /app -maxdepth 2 -type f -name "*.go" | head -20 && ls -la /app`.
  `pragma.stderr.log` reports `agentic loop exceeded maximum of 1 turns`.
- Interpretation:
  The one-turn budget prevents silent drift, but Minimax needs one discovery
  turn and one artifact-writing turn for this survey state.
- Change made from this evidence:
  `swe_repo_survey` now has `max_turns: 2`, and its persona states that the
  second turn must write the artifact from first-turn evidence and stop.
- Next evidence needed:
  Re-run State 1 with `max_turns: 2`; it should complete in at most two turns
  with grounded paths and no patch.

## 2026-06-23 State Contract: `swe_repo_survey` Two-Turn Budget

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T151202Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Contract result: fail, loudly.
- Evidence:
  The state used both available turns for repository reads:
  first `pwd && find ... && ls`, then `cat internal/config/authentication.go`,
  `cat internal/cmd/auth.go`, and auth path discovery. It then failed with
  `agentic loop exceeded maximum of 2 turns`.
- Interpretation:
  The runtime budget is correctly preventing long state drift, but the persona
  still needs a harder second-turn rule.
- Change made from this evidence:
  `swe_repo_survey` now explicitly says that any second response whose main
  action is another repository read is invalid; when prior command output
  exists, it must write the artifact immediately from that evidence.
- Next evidence needed:
  Re-run State 1 with the same two-turn budget.

## 2026-06-23 State Contract: `swe_repo_survey` Required-Path Policy

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T151720Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail, loudly.
- Evidence:
  `pragma.stdout.log` shows three proposed bash blocks that inspected the
  repository or wrote `/tmp/pragma/swe/survey_raw.txt`, but none wrote the
  required `/tmp/pragma/swe/repo-survey.md` artifact. `pragma.stderr.log`
  reports `agentic loop exceeded maximum of 3 turns`.
- Interpretation:
  The generic `shell_policy.require_patterns` gate prevented a silent state
  pass without the required artifact path, but the persona still invited
  discovery-first behavior strongly enough that the model exhausted the state
  budget before producing the final survey.
- Change made from this evidence:
  `personas-research-v2/swe_repo_survey.yaml` now states that the first bash
  block itself must contain `/tmp/pragma/swe/repo-survey.md`, commands without
  that exact path are rejected and not executed, temporary survey files are not
  acceptable substitutes, and any retry must write the final artifact from
  available evidence.
- Next evidence needed:
  Re-run State 1 and require a full pass: one accepted shell action writes
  `/tmp/pragma/swe/repo-survey.md`, the artifact is command-grounded, the run
  stops before `swe_acceptance_mapper`, and the prediction remains empty.

## 2026-06-23 State Contract: `swe_repo_survey` Required-Path Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T151916Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: partial pass.
- Pass evidence:
  The run stopped after `swe_repo_survey`, did not transition to
  `swe_acceptance_mapper`, wrote `/tmp/pragma/swe/repo-survey.md`, and produced
  an empty prediction.
- Gap:
  The artifact still contained guessed candidate surfaces such as
  `internal/authn/ or internal/auth/` and `ui/`, plus ungrounded implementation
  claims. Prompt-only restrictions were not enough to enforce the State 1
  contract.
- Change made from this evidence:
  Added the generic `markdown_candidate_surfaces_concrete` artifact integrity
  check. The check rejects missing `Candidate Surfaces`, guessed surface wording
  such as `or`/`probably`/`maybe`/`similar`, non-repo-relative surfaces, and
  path-like surfaces that do not exist in the current repository. The
  `repo_survey` output now uses this check.
- Next evidence needed:
  Re-run State 1. A full pass now requires both stop-after-state behavior and a
  repo-survey artifact whose candidate path surfaces are concrete enough to
  pass the runtime integrity check.

## 2026-06-23 State Contract: `swe_repo_survey` Candidate-Surface Check

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T152211Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail, loudly.
- Evidence:
  The model proposed two discovery-only commands, then wrote
  `/tmp/pragma/swe/repo-survey.md`. The artifact contained guessed candidate
  surfaces such as `internal/auth/ or auth/ package`,
  `internal/server/ or server/ package`, and `ui/ or dashboard/`. Completion
  was rejected by the new candidate-surface integrity check, and the state ran
  out of its three-turn budget before it could repair the artifact.
- Interpretation:
  The runtime now catches the right artifact defect, but the state needs a
  small bounded repair budget because shell-policy rejections and artifact
  integrity rejections both consume model turns.
- Change made from this evidence:
  `swe_repo_survey.max_turns` increased from `3` to `5`, preserving a bounded
  state while allowing two policy corrections plus one artifact repair.
- Next evidence needed:
  Re-run State 1 and check whether the model repairs the candidate surfaces
  into concrete observed paths after the runtime integrity guidance.

## 2026-06-23 State Contract: `swe_repo_survey` Eight-Turn Repair Budget

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T152456Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail, loudly.
- Evidence:
  The state still exhausted its turn budget. The model repeatedly rewrote
  `/tmp/pragma/swe/repo-survey.md` but kept guessed candidate surfaces such as
  `internal/authn/ or similar` and `ui/ or client-facing`.
- Additional gap found:
  The required repo-survey shape uses a plain `Candidate surfaces:` label, but
  the new runtime check initially recognized only a Markdown
  `## Candidate Surfaces` heading. The model responded by changing
  capitalization and labels instead of receiving precise feedback on the
  guessed surface bullets.
- Change made from this evidence:
  `markdown_candidate_surfaces_concrete` now reads bullets from either a
  Markdown heading or a plain colon-suffixed section label, case-insensitively.
  A focused unit test covers the plain-label format used by `repo_survey`.
- Next evidence needed:
  Re-run State 1 with the corrected section parser and verify whether the
  runtime now gives actionable feedback on the guessed candidate surface lines.

## 2026-06-23 State Contract: `swe_repo_survey` Corrected Parser Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T152841Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass for runtime-enforced shape; partial pass for semantic
  State 1 contract.
- Pass evidence:
  The run stopped after `swe_repo_survey`, did not transition to
  `swe_acceptance_mapper`, wrote `/tmp/pragma/swe/repo-survey.md`, and kept the
  prediction empty. After an artifact repair, candidate path surfaces were
  concrete enough to pass `markdown_candidate_surfaces_concrete`.
- Remaining gap:
  Candidate-surface descriptions still contained implementation-planning
  wording such as "new ... needed here" and "registration". The artifact was
  usable for path grounding, but it still blurred the survey/planning boundary.
- Change made from this evidence:
  `markdown_candidate_surfaces_concrete` now also rejects candidate-surface
  descriptions phrased as implementation instructions, such as `add`, `create`,
  `modify`, `register`, `wire`, `implement`, `fix`, `needed here`, or
  `should add`.
- Next evidence needed:
  Re-run State 1 with the stricter semantic check. A full pass requires
  concrete candidate surfaces whose descriptions explain relevance without
  prescribing edits.

## 2026-06-23 State Contract: `swe_repo_survey` Semantic Boundary Check

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T153041Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail, loudly.
- Evidence:
  The first artifact had guessed and implementation-directed candidate
  surfaces, including `internal/authn/` and `ui/`. After completion rejection,
  the model regressed to discovery-only commands and exhausted the eight-turn
  state budget instead of rewriting `/tmp/pragma/swe/repo-survey.md`.
- Interpretation:
  The semantic check catches the right class of defect, but the current
  repo-survey persona does not yet reliably repair that defect from completion
  feedback. More budget alone is not the right fix; it encourages discovery
  drift.
- Next evidence needed:
  Tighten the persona with explicit good/bad candidate-surface examples and a
  repair instruction that rejected completion must be fixed by rewriting the
  artifact, not by running more repository discovery. Then rerun State 1 before
  moving on to `swe_acceptance_mapper`.

## 2026-06-23 State Contract: `swe_repo_survey` Persona Repair Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T153322Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass.
- Pass evidence:
  The run stopped after `swe_repo_survey`, did not transition to
  `swe_acceptance_mapper`, wrote `/tmp/pragma/swe/repo-survey.md`, and kept the
  prediction empty. The first completed artifact was rejected for
  implementation-directed candidate surfaces, then the model rewrote
  Candidate Surfaces into survey-style bullets such as
  `internal/config/authentication.go - observed configuration struct
  definitions...` and completed successfully.
- Residual risk:
  The survey is still broad in places, for example it lists `internal/` as an
  observed auth package surface. This is acceptable for State 1 because the
  artifact preserves the uncertainty and does not prescribe an edit. Later
  states must refine this into acceptance requirements and implementation
  slices before editing.
- Next evidence needed:
  Compose State 1 into `swe_acceptance_mapper` with
  `--stop-after-state swe_acceptance_mapper`. Require a task-grounded
  `acceptance-map.json` whose blocking items quote the original task and do
  not promote repo-survey hypotheses into acceptance requirements.

## 2026-06-23 State Contract: `swe_acceptance_mapper` First Composed Run

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_acceptance_mapper --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T153500Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail, loudly.
- Evidence:
  The flow reached `swe_acceptance_mapper` after `swe_repo_survey` completed.
  The acceptance mapper's only model turn was:
  `cat /tmp/pragma/swe/repo-survey.md`. It did not write
  `/tmp/pragma/swe/acceptance-map.json`, and the state failed with
  `agentic loop exceeded maximum of 1 turns`.
- Interpretation:
  The State 2 persona described the required output but did not make the first
  action contract executable. A read-only handoff inspection turn is not useful
  for a one-artifact mapper state.
- Change made from this evidence:
  `swe_acceptance_mapper` now has a bounded correction budget
  (`max_turns: 5`) and a generic `shell_policy.require_patterns` rule requiring
  `/tmp/pragma/swe/acceptance-map.json` in the command. The persona now states
  that any repo-survey read must happen in the same bash block that writes the
  acceptance map, and completion rejections must be repaired by rewriting the
  JSON rather than running discovery-only commands.
- Next evidence needed:
  Re-run the same stop-after-state command. A pass requires State 2 to write a
  valid, task-quoted `acceptance-map.json`, stop before
  `swe_environment_survey`, and keep the prediction empty.

## 2026-06-23 State Contract: `swe_acceptance_mapper` Output-Path Policy Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_acceptance_mapper --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T153809Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: partial pass.
- Pass evidence:
  The flow reached `swe_acceptance_mapper`, wrote valid
  `/tmp/pragma/swe/acceptance-map.json`, stopped before
  `swe_environment_survey`, and produced an empty prediction. Existing checks
  accepted JSON shape, `source: task_prompt`, non-empty acceptance items,
  exact task-prompt `source_quote` values, repo-relative surfaces, and
  non-placeholder validation descriptions.
- Gap:
  The acceptance map promoted ungrounded repository surfaces such as
  `internal/authn` and `grpc/grpc.go`. These are repo-relative strings, so the
  existing `json_each_repo_relative_paths` check did not reject them, but State
  2 requires paths to be grounded in the original task or repo survey.
- Change made from this evidence:
  Added the generic artifact check
  `json_each_string_array_values_in_task_or_handoff`, which verifies every
  non-`unknown` string in a JSON array field appears in either the original
  task prompt or a named handoff artifact snapshot. The acceptance map now
  applies it to `repo_surfaces_to_verify` using the `repo_survey` handoff.
- Next evidence needed:
  Re-run `--stop-after-state swe_acceptance_mapper`. A full pass now requires
  task-quoted acceptance items and repo surface values grounded in the task or
  repo survey, with ungrounded surfaces represented as `unknown` or notes
  rather than invented paths.

## 2026-06-23 State Contract: `swe_acceptance_mapper` Grounding Check Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_acceptance_mapper --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T154413Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass.
- Pass evidence:
  The flow completed `swe_repo_survey`, transitioned to
  `swe_acceptance_mapper`, wrote valid `/tmp/pragma/swe/acceptance-map.json`,
  stopped before `swe_environment_survey`, and produced an empty prediction.
  The first acceptance map used ungrounded repo surfaces such as
  `internal/authn`; completion was rejected by
  `json_each_string_array_values_in_task_or_handoff`. The model repaired those
  surfaces to `unknown` where the repo survey did not ground a concrete path,
  while preserving exact `source_quote` values from the original task prompt.
- Residual risk:
  Some `required_validation` entries are behavior-proof descriptions rather
  than executable commands. This is acceptable for State 2 because exact
  validation commands are refined by the environment survey and slice planner.
- Next evidence needed:
  Compose through `swe_environment_survey` with
  `--stop-after-state swe_environment_survey`. Require
  `/tmp/pragma/swe/environment-context.md` to distinguish available commands
  from acceptance validation and preserve runner capability evidence.

## 2026-06-23 State Contract: `swe_environment_survey` First Composed Run

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_environment_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T154752Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail before State 3.
- Evidence:
  The flow failed in `swe_repo_survey` before reaching
  `swe_environment_survey`. The model repeatedly wrote candidate surfaces with
  unobserved paths such as `internal/config/auth/authentication.go`; the state
  exhausted its eight-turn budget.
- External repo inspection:
  Direct container inspection of `/app` confirms concrete auth surfaces such
  as `internal/config/authentication.go`, `internal/server/auth/`,
  `internal/cmd/auth.go`, `internal/storage/auth/auth.go`, and
  `rpc/flipt/auth/auth.proto`. The invented `internal/config/auth/...` path is
  not an observed surface.
- Change made from this evidence:
  `swe_repo_survey` now states that directory observation is not transitive,
  candidate surfaces must be a flat list without subheadings, and each bullet
  must start with a single path, directory, or symbol observed in the same
  state.
- Next evidence needed:
  Re-run the environment stop-after command. The first requirement is that the
  earlier states remain stable enough to reach `swe_environment_survey`.

## 2026-06-23 State Contract: `swe_environment_survey` Reached State 3

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_environment_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T155118Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail in State 3.
- Evidence:
  The flow completed `swe_repo_survey` and `swe_acceptance_mapper`, then
  entered `swe_environment_survey`. The environment state proposed a command
  that wrote `/tmp/pragma/swe/environment-context.md`, but it used a
  single-quoted heredoc for measured sections and hardcoded values such as PATH
  and architecture. The state had `max_turns: 1`, so there was no repair turn.
- Interpretation:
  The environment-survey persona already forbids single-quoted heredocs for
  measured sections, but the runtime did not enforce that contract and the
  one-turn budget made a correctable artifact defect fatal.
- Change made from this evidence:
  `swe_environment_survey` now has a bounded correction budget
  (`max_turns: 5`), requires `/tmp/pragma/swe/environment-context.md` in the
  shell command, and rejects single-quoted heredocs via `shell_policy`. The
  persona now also states that rejected completion must be repaired by
  rewriting the environment artifact, not by running discovery-only commands.
- Next evidence needed:
  Re-run the same stop-after command and require a measured environment
  context that distinguishes tool availability from task acceptance validation.

## 2026-06-23 State Contract: `swe_environment_survey` Shell-Policy Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_environment_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T155456Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass.
- Pass evidence:
  The flow completed `swe_repo_survey`, `swe_acceptance_mapper`, and
  `swe_environment_survey`, then stopped before `swe_theory_keeper`. Prediction
  output remained empty. The first environment command used a malformed
  redirection shape and was rejected; the second used a single-quoted heredoc
  and was rejected by shell policy; the final command captured values into
  variables, wrote `/tmp/pragma/swe/environment-context.md` with an unquoted
  heredoc, echoed the completion sentinel, and completed.
- Artifact evidence:
  The final environment context recorded current runner facts (`cwd`, `PATH`,
  `uname`), tool availability (`go`, `git`, `make`, `docker`, `go version`),
  repo tooling signals (`go.mod`, `go.sum`, `Makefile`), producer preflight
  classes, runner capability evidence copied from
  `/tmp/pragma/swe/runner-capability-evidence.md`, capability contradictions,
  and environment constraints.
- Residual risk:
  This state has no semantic artifact integrity check beyond required output
  existence and shell policy. If later evidence shows hardcoded or stale values
  passing, add a generic environment-context integrity check.
- Next evidence needed:
  Compose through the theory/audit path toward `swe_slice_planner`, still using
  stopped runs. Do not start full `--evaluate` until slice planner and first
  worker contracts have acceptable evidence.

## 2026-06-23 State Contract: `swe_slice_planner` First Composed Run

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_slice_planner --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T160021Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail before `swe_theory_keeper` could run.
- Evidence:
  The flow completed `swe_repo_survey`, `swe_acceptance_mapper`, and
  `swe_environment_survey`, then failed entering `swe_theory_keeper` with:
  `read transition handoff artifact "environment_context" ... no such file or
  directory`.
- Environment-state defect:
  The environment-survey command used multiple heredocs, including a malformed
  heredoc delimiter sequence. The transcript emitted a completion sentinel and
  state completion, but the declared `/tmp/pragma/swe/environment-context.md`
  handoff was not available to the next state.
- Change made from this evidence:
  `swe_environment_survey` shell policy now rejects all heredocs (`<<`), not
  only single-quoted heredocs. The persona now requires `printf` and
  append-only redirection for measured environment artifacts.
- Next evidence needed:
  Re-run a stopped composed flow through at least `swe_environment_survey`, and
  then retry `swe_slice_planner` once the environment handoff is stable.

## 2026-06-23 State Contract: `swe_environment_survey` Heredoc Policy Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_environment_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T160705Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `1`
- Prediction size: `0` bytes.
- Contract result: fail before State 3.
- Evidence:
  The flow failed in `swe_repo_survey`. The model wrote a repo survey with a
  future edit target `internal/authn/kubernetes.go` under Candidate Surfaces
  and implementation wording such as "should be added here". That violates the
  State 1 rule that candidate surfaces are observed surfaces, not future
  implementation boundaries.
- Change made from this evidence:
  `swe_repo_survey` now explicitly says future files that may need to be
  created are not observed candidate surfaces and belong under `Early risks` or
  `Unknowns to resolve`. The state turn budget increased from `8` to `10` to
  allow one additional artifact repair after rejection in composed runs.
- Next evidence needed:
  Re-run the stopped environment flow and require State 1 to repair future-file
  candidate surfaces instead of exhausting the state budget.

## 2026-06-23 State Contract: `swe_environment_survey` No-Heredoc Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_environment_survey --agent-timeout 600`
- Output dir:
  `.pragma/swe-bench-pro/20260623T160914Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass.
- Pass evidence:
  The flow completed `swe_repo_survey`, `swe_acceptance_mapper`, and
  `swe_environment_survey`, then stopped before `swe_theory_keeper`. Prediction
  output remained empty. The environment state first proposed a discovery-only
  command, then wrote `/tmp/pragma/swe/environment-context.md` using captured
  shell variables and `printf`, without heredocs.
- Artifact evidence:
  The final environment context included current runner facts, PATH, `uname`,
  `go version`, `make`, `grep`, `ls`, repo tooling signals for
  `internal/config/authentication.go`, `Makefile`, and
  `rpc/flipt/auth/auth.pb.go`, producer preflight availability, runner
  capability evidence, contradictions, and environment constraints.
- Next evidence needed:
  Retry `--stop-after-state swe_slice_planner`. Earlier states now have
  acceptable stopped-run evidence for composing into theory/audit/planner.

## 2026-06-23 State Contract: `swe_slice_planner` Retry

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_slice_planner --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T161415Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: partial pass.
- Pass evidence:
  The flow reached `swe_slice_planner`, wrote
  `/tmp/pragma/swe/slice-plan.json`, stopped before
  `swe_slice_plan_auditor`, and produced an empty prediction.
- Gap:
  The slice plan was too broad. It planned one discovery slice across all ten
  acceptance IDs and included broad or guessed read-only paths such as
  `internal/auth/`, `internal/server/auth.go`, and `internal/services/`.
  State 4 requires one thin slice, not whole-feature discovery.
- Change made from this evidence:
  Added the generic `json_array_max_items` artifact check and applied it to
  `slice_plan.acceptance_ids` with a maximum of three items. `swe_slice_planner`
  now also has `max_turns: 5`, requires `/tmp/pragma/swe/slice-plan.json` in
  the shell command, and tells the model to repair over-broad plans by
  narrowing `acceptance_ids`.
- Next evidence needed:
  Re-run `--stop-after-state swe_slice_planner`. A pass now requires a bounded
  slice with at most three acceptance IDs and the remaining blocking IDs listed
  under `missing_acceptance_ids`.

## 2026-06-23 State Contract: `swe_slice_planner` Max-Items Repair Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_slice_planner --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T162237Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: partial pass.
- Pass evidence:
  The flow reached `swe_slice_planner`, wrote
  `/tmp/pragma/swe/slice-plan.json`, stopped before
  `swe_slice_plan_auditor`, and produced an empty prediction. The planner
  initially selected six acceptance IDs, then repaired to three IDs after the
  max-items contract rejected the broad plan.
- Gap:
  The accepted artifact had the right acceptance bound but still exposed a
  weak shape contract. In particular, the runtime was not checking that the
  top-level JSON contained all fields needed by the next worker, such as
  route, objective, acceptance linkage, scope envelope, expected observable,
  validation command, and stop condition.
- Change made from this evidence:
  Added the generic `json_required_fields` artifact integrity check. It
  rejects missing fields, empty strings, empty arrays, empty maps, and nulls
  for declared top-level or dotted JSON paths. Applied it to
  `swe_slice_planner` for the core slice-plan contract fields.
- Next evidence needed:
  Re-run `--stop-after-state swe_slice_planner` using the stricter current
  contract. A pass now requires both a bounded slice and a complete worker
  handoff shape.

## 2026-06-23 State Contract: `swe_slice_planner` Required-Fields Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_slice_planner --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T163224Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass for State 4.
- Pass evidence:
  The composed run completed repo survey, acceptance mapper, environment
  survey, theory keeper, acceptance auditor, and slice planner. It stopped
  before `swe_slice_plan_auditor` and produced an empty prediction diff.
- Artifact evidence:
  The planner first wrote a broad implementation slice with four acceptance
  IDs and an empty `read_only_paths` field. It then rewrote
  `/tmp/pragma/swe/slice-plan.json` to:
  - `worker_track: "implementation"`
  - `mode: "edit"`
  - `acceptance_ids`: `ACCEPT-K8S-AUTH-CONFIG-PARAMS`,
    `ACCEPT-K8S-AUTH-DEFAULTS`, and
    `ACCEPT-K8S-AUTH-DEPLOYMENT-SCENARIOS`
  - `missing_acceptance_ids`: the seven remaining blocking IDs
  - `approved_edit_paths`: `internal/config/authentication.go`
  - `read_only_paths`: `internal/config/authentication.go`
  - `validation_covers_acceptance_ids`: empty, avoiding overclaim
  - `targeted_validation`: `none`, with the worker stop condition limited to
    reporting the struct edit or requesting scope expansion
- Interpretation:
  This satisfies the manual State 4 contract for one thin implementation
  prerequisite slice: it names the exact acceptance IDs addressed, limits edit
  scope, lists remaining acceptance gaps, and does not claim behavior
  validation from a structural source edit.
- Next evidence needed:
  Compose through `swe_slice_plan_auditor` and stop after the first worker
  state. The next required proof is that the implementation worker either
  makes the narrow approved edit with runtime command evidence or produces a
  concrete blocked/scope-expansion report.

## 2026-06-23 State Contract: First Worker Discovery Path

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_discovery_worker --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T165456Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass for the first worker path selected by this composed
  run.
- Route evidence:
  The flow completed `swe_slice_plan_auditor`, `route_worker_track` emitted
  `discovery`, the run entered `swe_discovery_worker`, and it stopped before
  `swe_targeted_validator`.
- Worker evidence:
  The discovery worker inspected the planned/read-only authentication config
  and auth enum surfaces, wrote `/tmp/pragma/swe/worker-report.md`, and the
  runtime captured command evidence in
  `/tmp/pragma/swe/worker-command-evidence.jsonl`. The prediction diff stayed
  empty, which is expected for a discovery slice.
- Artifact evidence:
  The worker report recorded:
  - `Changed files: none`
  - `task_completion_claim_allowed: false`
  - addressed IDs `ACCEPT-K8S-AUTH-METHOD` and
    `ACCEPT-K8S-AUTH-CONFIG-PARAMS`
  - `validation_covers_acceptance_ids: none`
  - newly discovered coupling for `auth.Method`, `AuthenticationMethods`,
    `AuthenticationMethodInfoProvider`, and `AllMethods()`
- Interpretation:
  This satisfies the manual State 5 contract for a discovery worker: the worker
  used the audited slice plan, performed real read-only repository inspection,
  produced runtime-backed command evidence, made no out-of-scope edits, and
  avoided final task-completion claims.
- Note:
  A previous run aimed at `--stop-after-state swe_engineering_worker` selected
  `worker_track: discovery`, so it was interrupted before later validation
  gates and its leftover container was stopped. It is not counted as State 5
  pass/fail evidence.
- Next evidence needed:
  Add or validate the bounded VibeThink gates after targeted validation, then
  run a stopped flow through `swe_targeted_validator` and
  `swe_validation_gate` only when OpenAI-compatible VibeThink environment
  variables are available. Do not run full `--evaluate` yet.

## 2026-06-23 State Contract: `swe_targeted_validator` Coverage Gap

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_targeted_validator --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T170702Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: structural pass, semantic gap.
- Evidence:
  The flow completed through `swe_targeted_validator` and stopped before
  `swe_validation_gate`. The targeted-validation artifact correctly used
  `Result: not_applicable` for a discovery slice and did not list validated
  IDs, but it also wrote `insufficient_acceptance_ids: none` despite naming
  planned acceptance IDs. That weakens the State 9 handoff because the
  VibeThink gate should see those current-slice IDs as unsupported.
- Change made from this evidence:
  Added the generic `markdown_validation_coverage_consistent` artifact check.
  It verifies that a markdown validation report with planned acceptance IDs
  lists either directly validated IDs or insufficient IDs, and that non-pass
  results such as `fail`, `insufficient`, or `not_applicable` do not leave
  `insufficient_acceptance_ids` empty. The targeted validator persona now
  explicitly says discovery/no-command slices must put planned IDs under
  `insufficient_acceptance_ids`.

## 2026-06-23 State Contract: `swe_targeted_validator` Coverage-Check Rerun

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_targeted_validator --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T173039Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass for State 9 on a discovery slice.
- Route evidence:
  The flow completed repo survey, acceptance mapper, environment survey,
  theory keeper, acceptance auditor, slice planner, slice-plan auditor,
  discovery worker, and targeted validator. It stopped before
  `swe_validation_gate`.
- Artifact evidence:
  The targeted validator wrote `/tmp/pragma/swe/targeted-validation.md` with:
  - `planned_acceptance_ids`: `ACCEPT-K8S-AUTH-METHOD`,
    `ACCEPT-K8S-AUTH-CONFIG-PARAMS`, `ACCEPT-K8S-AUTH-DEFAULTS`
  - `validated_acceptance_ids: none`
  - `insufficient_acceptance_ids`: the same three planned IDs
  - `Result: not_applicable`
  - `Theory signal: needs_more_evidence`
  - no changed files and no final-task approval claim
- Interpretation:
  This satisfies the manual State 9 contract for a discovery slice. The
  targeted validator did not treat source inspection as behavior validation and
  made the unsupported planned IDs explicit for the reasoning gate.
- Environment-state note:
  The preceding `20260623T172337Z` rerun failed before the validator because
  `swe_environment_survey` exhausted five turns after heredoc and malformed
  completion-sentinel attempts. The environment persona now requires the final
  line to be exactly `echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT`, recommends
  grouped `echo` or `printf --`, and the environment state budget is seven
  turns for composed repairs.
- Next evidence needed:
  Validate `swe_validation_gate` with VibeThink when `OPENAI_BASE_URL` and
  `OPENAI_API_KEY` are available. Expected verdict for this discovery-slice
  targeted validation is `BLOCK`. Do not run full `--evaluate` yet.

## 2026-06-23 Runtime Contract: Final-Text Gate Capture and Routing

- Evidence type:
  Local runtime regression test, not a real VibeThink model call.
- Test:
  `TestRunEventsFinalTextGateCapturesAndRoutes` in
  `internal/orchestration/orchestration_test.go`.
- Contract result: pass.
- Evidence:
  The test builds a two-step orchestration with a final-text gate state and an
  `artifact_verdict` route state. The fake provider returns
  `<think>hidden</think>\nBLOCK`. The runtime captures the final text to the
  declared artifact, strips the explicit think block, normalizes the allowed
  value, writes `BLOCK\n`, and routes `route_gate --block--> blocked`.
- Interpretation:
  The generic runtime path needed by `swe_validation_gate` is covered: no shell
  transport is required for final-text states, the runtime-authored verdict
  artifact is produced from model final text, bare verdict values are accepted,
  and declared verdict routing works.
- Remaining evidence gap:
  This does not prove the local VibeThink server can answer the gate prompt.
  A real stopped run through `swe_validation_gate` still requires
  `OPENAI_BASE_URL` and `OPENAI_API_KEY` in the execution environment. Those
  variables are currently absent, so full gate validation remains pending.

## 2026-06-23 State Contract: Real `swe_validation_gate` VibeThink Run

- Local server probe:
  `curl -sS --max-time 2 http://127.0.0.1:8080/v1/models`
- Probe result:
  The local OpenAI-compatible server returned model entries including
  `mlx-community/VibeThinker-3B-4bit`.
- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_validation_gate --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T174948Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Prediction size: `0` bytes.
- Contract result: pass for State 10.
- Runner evidence:
  The runner passed `OPENAI_API_KEY` and rewrote
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1` to
  `http://host.docker.internal:8080/v1` for Docker reachability.
- Route evidence:
  The flow completed repo survey, acceptance mapper, environment survey,
  theory keeper, acceptance auditor, slice planner, slice-plan auditor,
  discovery worker, targeted validator, and validation gate. It stopped before
  `route_validation_gate`.
- Gate evidence:
  `swe_targeted_validator` wrote a report with `Result: insufficient`,
  `validated_acceptance_ids: none`, and the current planned IDs under
  `insufficient_acceptance_ids`. `swe_validation_gate` then returned exactly
  `BLOCK`, and the state completed in 19s.
- Interpretation:
  This satisfies the manual State 10 contract for the current discovery slice.
  VibeThink was used only as a reasoning-only final-text gate, received bounded
  handoff context, produced a tiny machine-routable verdict, and correctly
  blocked insufficient targeted validation.
- Next evidence needed:
  Compose one stopped run through `route_validation_gate` or the next theory
  update to verify `BLOCK` routes back to `swe_theory_keeper` rather than
  proceeding to reviewer. Full `--evaluate` remains gated until the loop after
  the VibeThink block and subsequent implementation slice are validated.

## 2026-06-23 State Contract: `route_validation_gate` Timeout Attempt

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state route_validation_gate --agent-timeout 900`
- Output dir:
  `.pragma/swe-bench-pro/20260623T180317Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `124`
- Prediction size: `0` bytes.
- Contract result: no route evidence collected.
- Evidence:
  The composed run completed repo survey, acceptance mapper, environment
  survey, theory keeper, acceptance auditor, and slice planner. It timed out
  during `swe_slice_plan_auditor` before reaching the discovery worker,
  targeted validator, VibeThink gate, or `route_validation_gate`.
- Failure reason:
  The wrapper timeout canceled an in-flight Lilac request:
  `state "swe_slice_plan_auditor" failed: Post "https://api.getlilac.com/v1/chat/completions": context canceled`.
- Interpretation:
  This is not a `route_validation_gate` failure. It shows the full composed
  path to the route can exceed a 900s agent timeout when earlier states need
  repairs. The existing local runtime regression test still proves generic
  `BLOCK` verdict routing; real composed route evidence remains pending.
- Next evidence needed:
  Retry route evidence with a longer timeout or a narrower replay/fixture path
  that starts from the already validated targeted-validation and gate artifacts.

## 2026-06-23 Runtime Contract: YAML-Backed `route_validation_gate` Block Route

- Evidence type:
  Local orchestration regression test using the real SWE-bench Pro YAML, not a
  model call.
- Test:
  `TestSWEBenchProValidationGateBlockRoutesToTheoryKeeper` in
  `internal/orchestration/orchestration_test.go`.
- Contract result:
  Pass.
- Evidence:
  The test loads `orchestrations/swe-bench-pro-engineering-loop.yaml`, locates
  the declared `route_validation_gate` state, verifies the configured verdict
  values are `PASS` and `BLOCK`, verifies the configured events are `pass` and
  `block`, writes a temporary `validation-gate.txt` containing `BLOCK`, and
  executes the declared `artifact_verdict` control. The emitted event is
  `block`.
- Transition evidence:
  The same test sets the FSM to `route_validation_gate`, applies the emitted
  `block` event, and verifies the next state is `swe_theory_keeper`.
- Interpretation:
  This proves the current YAML wiring routes a VibeThink `BLOCK` verdict back
  into the theory/update loop. It does not replace the failed composed replay:
  the real `route_validation_gate` stopped run still timed out upstream in
  `swe_slice_plan_auditor`, so no composed route log exists yet. The route
  mechanism and YAML transition are now covered narrowly enough to continue
  designing the post-block loop without rerunning all upstream states for every
  route check.
- Next evidence needed:
  Validate the next composed post-block behavior either with a longer stopped
  run through `swe_theory_keeper` after the gate or with a fixture/start-state
  path that uses already-proven upstream artifacts.

- Verification:
  `go test ./internal/orchestration -run TestSWEBenchProValidationGateBlockRoutesToTheoryKeeper -count=1`
  passed.
  `go test ./...`, `python3 -m py_compile tools/run_swebench_pro_instance.py tools/test_run_swebench_pro_instance.py`,
  `python3 -m unittest tools.test_run_swebench_pro_instance`,
  `git diff --check`, and
  `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed.

## 2026-06-23 Runtime Contract: Generic `--start-at-state` Replay

- Change:
  Added generic orchestration `StartAtState` support to `RunOptions`, CLI
  `orchestration run`, slash `/orchestrate`, and
  `tools/run_swebench_pro_instance.py`.
- Contract:
  A replay run may start from any declared state in the loaded FSM. It does not
  synthesize prior handoff context or invent artifacts; the caller must seed or
  preserve the artifacts that the started state requires. Unknown start states
  fail loudly before execution.
- SWE-bench runner guard:
  `--start-at-state` is rejected with `--evaluate` and with `--direct`, matching
  its intended use as a validation/debug mode rather than a scoring mode.
- Tests:
  `TestRunEventsStartAtState`,
  `TestRunEventsStartAtStateRejectsUnknownState`,
  `TestParseOrchestrateArgsWithReplayFlags`,
  `TestOrchestrateCompletionSuggestsReplayFlagsBeforePrompt`, and
  `test_start_at_state_is_added_before_stop_after_state`.
- Local route replay:
  With `/tmp/pragma/swe/validation-gate.txt` containing `BLOCK`, this command
  passed:
  `go run ./cmd/pragma orchestration run orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --start-at-state route_validation_gate --stop-after-state route_validation_gate --prompt 'replay route validation gate'`
- Replay output:
  The real YAML control state emitted `block`:
  `[control: route_validation_gate emitted block]`.
- Interpretation:
  This proves the newly added replay path can execute the real
  `route_validation_gate` directly from a preserved verdict artifact. Combined
  with the YAML-backed FSM test above, it confirms the VibeThink `BLOCK`
  artifact is machine-routable without replaying all upstream Minimax states.
- Remaining evidence gap:
  A post-block `swe_theory_keeper` replay still requires the full handoff
  artifact set that the block transition declares. The next ledger entry adds
  artifact-ID/path seeding to support that fixture replay without rerunning all
  upstream states.
- Verification:
  `go test ./...`, `python3 -m py_compile tools/run_swebench_pro_instance.py tools/test_run_swebench_pro_instance.py`,
  `python3 -m unittest tools.test_run_swebench_pro_instance`,
  `git diff --check`, and
  `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed after adding replay support.

## 2026-06-24 Runtime Contract: Artifact-ID Seeding and Fresh Outputs

- Change:
  Extended generic `--seed-artifact` replay support so a seed target may match
  a declared seed source, artifact ID, declared artifact path, or resolved
  artifact path. Seed writes now create destination directories as needed.
- Freshness guard:
  Required model-authored outputs are checked against the state-entry snapshot
  during completion. If a required output existed before the state and the
  model does not freshly write it, completion is rejected as stale. This
  prevents seeded replay inputs from accidentally satisfying the same state's
  required output contract.
- Tests:
  `TestMaterializeSeedArtifactsByArtifactID`,
  `TestMaterializeSeedArtifactsRejectsUnknownTarget`, and
  `TestRequiredOutputCompletionCheckRejectsStaleSeededOutput`.
- Interpretation:
  This is a reusable long-task orchestration capability: replay can start at a
  later FSM state with declared artifacts supplied from fixture files, while
  still requiring the state under test to produce its own output artifact.

## 2026-06-24 State Contract: Post-Block `swe_theory_keeper` Replay

- Fixture source:
  Artifacts were reconstructed from the successful stopped VibeThink gate run
  `.pragma/swe-bench-pro/20260623T174948Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma.stdout.log`
  into `.pragma/replay-artifacts/post-block-174948/`.
- Caveat:
  The original runner did not persist the runtime-authored
  `worker-command-evidence.jsonl`, so the replay seeded a placeholder for that
  artifact. This weakens command-evidence interpretation but still exercises
  the block handoff and theory update path.
- Command:
  `go run ./cmd/pragma --provider lilac --model minimaxai/minimax-m2.7 orchestration run orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --start-at-state route_validation_gate --stop-after-state swe_theory_keeper --seed-artifact engineering_context=.pragma/replay-artifacts/post-block-174948/engineering-context.md --seed-artifact acceptance_map=.pragma/replay-artifacts/post-block-174948/acceptance-map.json --seed-artifact environment_context=.pragma/replay-artifacts/post-block-174948/environment-context.md --seed-artifact slice_plan=.pragma/replay-artifacts/post-block-174948/slice-plan.json --seed-artifact worker_report=.pragma/replay-artifacts/post-block-174948/worker-report.md --seed-artifact worker_command_evidence=.pragma/replay-artifacts/post-block-174948/worker-command-evidence.jsonl --seed-artifact targeted_validation=.pragma/replay-artifacts/post-block-174948/targeted-validation.md --seed-artifact validation_gate=.pragma/replay-artifacts/post-block-174948/validation-gate.txt --prompt 'Replay the post-block theory update from preserved SWE-bench Pro artifacts.'`
- Route evidence:
  The real control state emitted `block`, then transitioned
  `route_validation_gate --block--> swe_theory_keeper`.
- State evidence:
  `swe_theory_keeper` completed in 4m17s with freshness enforcement active.
  `/tmp/pragma/swe/engineering-context.md` differed from the seeded context:
  seeded context was 8410 bytes, replay output was 9917 bytes, and `cmp`
  returned different.
- Content evidence:
  The updated context records the discovery slice as insufficient, preserves
  `targeted_validation result: insufficient` and `validation_gate: BLOCK`,
  keeps the first three acceptance IDs as insufficient rather than validated,
  and sets the next objective to implement the Kubernetes config struct,
  Info() method, Kubernetes field, and AllMethods registration.
- Contract result:
  Pass for the post-block theory update state contract on replayed handoff
  artifacts.
- Remaining evidence gap:
  The next implementation slice and targeted validation after this post-block
  theory update are not yet validated. Full `--evaluate` remains gated.
- Verification:
  `go test ./...`, `python3 -m py_compile tools/run_swebench_pro_instance.py tools/test_run_swebench_pro_instance.py`,
  `python3 -m unittest tools.test_run_swebench_pro_instance`,
  `git diff --check`, and
  `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed after artifact-ID seeding, stale-output rejection, and post-block
  replay.

## 2026-06-24 Replay Finding: Generated-Output and Stale-Worker Contract Gaps

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --start-at-state route_validation_gate --stop-after-state swe_targeted_validator --agent-timeout 1800 --seed-artifact engineering_context=.pragma/replay-artifacts/post-block-174948/engineering-context.md --seed-artifact acceptance_map=.pragma/replay-artifacts/post-block-174948/acceptance-map.json --seed-artifact environment_context=.pragma/replay-artifacts/post-block-174948/environment-context.md --seed-artifact slice_plan=.pragma/replay-artifacts/post-block-174948/slice-plan.json --seed-artifact worker_report=.pragma/replay-artifacts/post-block-174948/worker-report.md --seed-artifact worker_command_evidence=.pragma/replay-artifacts/post-block-174948/worker-command-evidence.jsonl --seed-artifact targeted_validation=.pragma/replay-artifacts/post-block-174948/targeted-validation.md --seed-artifact validation_gate=.pragma/replay-artifacts/post-block-174948/validation-gate.txt`
- Output:
  `.pragma/swe-bench-pro/20260623T184535Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
  The wrapper exited 0 with no evaluation and a zero-byte prediction patch.
- Route/state evidence:
  `route_validation_gate` emitted `block`; `swe_theory_keeper`,
  `swe_acceptance_auditor`, `swe_slice_planner`, and
  `swe_slice_plan_auditor` completed before `route_worker_track` emitted
  `implementation`.
- Contract failures found:
  The audited slice plan approved `rpc/flipt/auth/auth.pb.go` as an editable
  generated file and justified it by saying `protoc` was unavailable. That
  violates the generated-output policy: producer/tool unavailability is not
  proof that a generated output is source of truth.
  `swe_engineering_worker` then completed after only read-only inspection
  commands and did not visibly write a fresh worker report in the replay log.
  `swe_targeted_validator` correctly reported no changed files and all planned
  acceptance IDs insufficient.
- Runtime hardening:
  Added a post-state required-output guard. After each state finishes, missing
  required outputs and stale model-authored outputs are rejected against the
  state-entry snapshot. This closes the no-sentinel bypass observed when a
  state ends without the usual completion retry path.
- Artifact hardening:
  Added the generic artifact integrity check
  `json_no_unproven_generated_outputs_in_approved_edit_paths` and wired it to
  `swe_slice_planner` and `swe_slice_plan_auditor`. Generated-looking paths
  such as `*.pb.go`, `*.pb.gw.go`, `*.gen.go`, `*_generated.go`,
  `*.generated.go`, `*.swagger.json`, or paths under `generated/` cannot appear
  in `approved_edit_paths` unless `generated_policy` proves source-of-truth
  status without manual/unavailable-producer workaround language.
- Persona hardening:
  Strengthened both `swe_slice_planner` and `swe_slice_plan_auditor` to treat
  generated output edits without source-of-truth proof as producer/source-of-
  truth discovery or unresolved producer blockers, not implementation work.
- Tests:
  Added `TestValidateFreshRequiredModelOutputsRejectsStaleSeededOutput`,
  `TestValidateJSONNoUnprovenGeneratedOutputsInApprovedEditPaths`, and
  `TestValidateJSONNoUnprovenGeneratedOutputsAllowsProvenSourceOfTruth`.
- Verification:
  Focused orchestration tests covering the new checks, stale completion,
  VibeThink block routing, and start-at-state passed. Visualization of
  `orchestrations/swe-bench-pro-engineering-loop.yaml` also passed. Full-suite
  verification remains to rerun after these patches.

## 2026-06-24 State Contract: Slice Planner/Auditor Generated-Output Replay

- Command:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --start-at-state route_validation_gate --stop-after-state swe_slice_plan_auditor --agent-timeout 1800 --seed-artifact engineering_context=.pragma/replay-artifacts/post-block-174948/engineering-context.md --seed-artifact acceptance_map=.pragma/replay-artifacts/post-block-174948/acceptance-map.json --seed-artifact environment_context=.pragma/replay-artifacts/post-block-174948/environment-context.md --seed-artifact slice_plan=.pragma/replay-artifacts/post-block-174948/slice-plan.json --seed-artifact worker_report=.pragma/replay-artifacts/post-block-174948/worker-report.md --seed-artifact worker_command_evidence=.pragma/replay-artifacts/post-block-174948/worker-command-evidence.jsonl --seed-artifact targeted_validation=.pragma/replay-artifacts/post-block-174948/targeted-validation.md --seed-artifact validation_gate=.pragma/replay-artifacts/post-block-174948/validation-gate.txt`
- Output:
  `.pragma/swe-bench-pro/20260623T190256Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Planner evidence:
  `swe_slice_planner` first wrote `approved_edit_paths` containing
  `rpc/flipt/auth/auth.pb.go` and a `generated_policy` saying `protoc`
  unavailable/manual enum addition. The generic artifact integrity check
  rejected that completion, and the retry removed `auth.pb.go` from
  `approved_edit_paths`, moved it to `suspected_coupled_paths`, and marked it
  as an unresolved producer blocker.
- Auditor evidence:
  `swe_slice_plan_auditor` completed and preserved only
  `internal/config/authentication.go` as editable. It set
  `validation_covers_acceptance_ids` to empty, kept the linked
  `acceptance_ids`, assigned compile/grep work to targeted validation, and
  explicitly forbade editing `rpc/flipt/auth/auth.pb.go`.
- Reliability issue:
  The auditor request was large and retried seven Lilac API timeouts before
  completing in 20m35s. This made the state contract semantically correct but
  operationally fragile.
- Generic runtime hardening:
  Added declarative handoff `max_bytes` on artifacts. Rendering now shows full
  bytes/SHA metadata and a capped prompt excerpt when configured, while
  preserving full `HandoffRead` snapshots for integrity checks. Added tests
  `TestLoadArtifactMaxBytes` and
  `TestRenderTransitionHandoffMaxBytesPreservesSnapshot`.
- Capped replay:
  Reran the same stopped replay after capping the large handoffs into
  `swe_slice_plan_auditor`:
  `.pragma/swe-bench-pro/20260623T192932Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
  The auditor request size dropped from about 41 KB to 33 KB and completed in
  6m58s after two API timeouts. The final audited plan remained
  contract-correct: `approved_edit_paths` only included
  `internal/config/authentication.go`, `validation_covers_acceptance_ids` was
  empty, `rpc/flipt/auth/auth.pb.go` stayed coupled/generated, and the Worker
  Contract forbade generated-file edits and validation commands.
- Interpretation:
  The generated-output contract now has both prompt-level and artifact-level
  enforcement, and replay shows it corrects the exact failure found in the
  previous post-block implementation replay. The remaining performance risk is
  reduced but not eliminated for large Minimax auditor prompts.

## 2026-06-24 State Contract: Worker/Validator Source-of-Truth Scope Repair

- Fixture:
  Created `.pragma/replay-artifacts/post-audit-192932/` from the successful
  capped slice-plan auditor replay. The fixture contains the audited
  `slice-plan.json`, `plan-audit.md`, and `handoff-audit.md`; other seed
  artifacts reuse the post-block replay inputs.
- Initial worker replay:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --start-at-state route_worker_track --stop-after-state swe_engineering_worker --agent-timeout 1200 ...`
  completed with status 0 and zero-byte prediction. The worker did not edit
  generated code and wrote a fresh report, but it requested scope expansion for
  `rpc/flipt/auth/auth.pb.go`. That was still the wrong repair target because
  `auth.pb.go` is generated output.
- Generic hardening:
  Added `markdown_no_generated_output_edit_recommendations`. The check rejects
  markdown artifacts that recommend manual edits or scope expansion for
  generated-looking outputs such as `*.pb.go`, while allowing prohibitions like
  "must not edit generated output". Wired the check into worker reports,
  discovery worker reports, and targeted validation reports.
- Persona hardening:
  Strengthened `swe_engineering_worker` and `swe_targeted_validator` so missing
  generated symbols route to source-of-truth/producer discovery or repair, not
  generated-output edit scope.
- Composed replay:
  Reran:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --start-at-state route_worker_track --stop-after-state swe_targeted_validator --agent-timeout 1200 ...`
  Output:
  `.pragma/swe-bench-pro/20260623T195033Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Replay evidence:
  The worker first wrote a report requesting scope for
  `rpc/flipt/auth/auth.pb.go`; the state did not complete there. It then ran
  read-only producer/source discovery, found `rpc/flipt/auth/auth.proto`, and
  rewrote the report with:
  `Scope request: paths: ["rpc/flipt/auth/auth.proto"]`.
  The report explicitly identifies `auth.pb.go` as generated output from proto
  and says it cannot be edited directly.
- Validator evidence:
  `swe_targeted_validator` completed in 36s with `Result: insufficient`,
  `validated_acceptance_ids: none`, and all three planned IDs insufficient.
  Its scope signal points to source-of-truth/producer repair and says generated
  output hand edits are not a valid recommendation. The next validation
  suggestion asks for scope expansion on `rpc/flipt/auth/auth.proto`, then a
  rerun of the config-struct slice.
- Interpretation:
  The composed worker->validator state contract now handles the generated enum
  dependency as a source-of-truth blocker instead of approving or recommending
  generated `.pb.go` edits. This is aligned with the manual-first generated
  output policy and preserves general-purpose long-task behavior.

## 2026-06-24 State Contract: No Partial Worker Edits on Source-of-Truth Blockers

- Trigger:
  A composed replay after the source-of-truth repair still showed unsafe worker
  behavior: the implementation worker edited `internal/config/authentication.go`
  to reference `auth.Method_METHOD_KUBERNETES`, then ran
  `go build ./internal/config/...`, even though the audited Worker Contract
  assigned build/validation commands to later states and the missing enum was
  outside the approved edit envelope.
- Runtime hardening:
  Added `markdown_no_forbidden_worker_commands`, keyed to rendered
  `plan_audit`, so worker reports are rejected when they claim build/test/lint/
  producer commands while the audited Worker Contract says no such commands
  belong to the worker. Added direct command-policy coverage for worker shell
  denial patterns such as `go build` and `git checkout`.
- Scope cleanliness hardening:
  Added `markdown_scope_request_requires_no_changed_files` to worker reports.
  A report that requests scope expansion cannot also claim changed files; this
  prevents half-implemented consumer diffs from being carried forward when the
  real blocker is source-of-truth or producer scope.
- Persona hardening:
  Strengthened `swe_engineering_worker` so if `plan-audit.md` or
  `slice_plan.scope_policy` already says the expected observable depends on a
  missing enum/declaration/generated symbol outside `approved_edit_paths`, the
  worker must stop before editing approved consumer files and write a clean
  scope/source-of-truth report.
- Replay:
  `python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --start-at-state route_worker_track --stop-after-state route_validation_gate --agent-timeout 1200 --seed-artifact engineering_context=.pragma/replay-artifacts/post-block-174948/engineering-context.md --seed-artifact acceptance_map=.pragma/replay-artifacts/post-block-174948/acceptance-map.json --seed-artifact environment_context=.pragma/replay-artifacts/post-block-174948/environment-context.md --seed-artifact handoff_audit=.pragma/replay-artifacts/post-audit-192932/handoff-audit.md --seed-artifact slice_plan=.pragma/replay-artifacts/post-audit-192932/slice-plan.json --seed-artifact plan_audit=.pragma/replay-artifacts/post-audit-192932/plan-audit.md`
- Output:
  `.pragma/swe-bench-pro/20260623T202345Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Worker evidence:
  `swe_engineering_worker` completed with `Changed files: none`,
  `Validation run by worker: none`, `forbidden_scope_touched: none`, and a
  scope/source blocker for the missing `METHOD_KUBERNETES` enum. The prediction
  diff was zero bytes.
- Validator evidence:
  The first targeted-validation report recommended expansion on
  `rpc/flipt/auth/auth.pb.go`; `markdown_no_generated_output_edit_recommendations`
  rejected it. The retry completed with `Result: insufficient`, all three
  planned IDs insufficient, and a scope signal requiring source-of-truth or
  producer/toolchain repair. The next suggestion asks to locate the source
  proto, gather clean-baseline producer evidence, or identify the unresolved
  producer/toolchain blocker.
- VibeThink evidence:
  `swe_validation_gate` returned exactly `BLOCK`, and `route_validation_gate`
  emitted `block`.
- Interpretation:
  The worker/validator/gate replay now preserves repository state on a known
  generated-enum blocker, routes the issue back through theory/scope repair,
  and keeps VibeThink as a narrow evidence gate rather than an executor.

## 2026-06-24 Full Evaluate Attempt: Worker Edit-Safety Gap

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T203344Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Toolchain evidence:
  The Docker preflight mounted the generator toolchain and detected `buf`,
  `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc`,
  `protoc-gen-grpc-gateway`, and `protoc-gen-openapiv2`.
- Progress before interruption:
  The run completed repo survey, acceptance mapping, environment survey,
  theory keeper, acceptance audit, discovery planning, discovery worker,
  targeted validation, VibeThink validation gate, a second theory/audit cycle,
  and an implementation slice plan/audit. The VibeThink validation gate
  correctly returned `BLOCK` after insufficient discovery evidence.
- Failure observed:
  The implementation worker used `sed -i` to mutate approved source files.
  The `auth.proto` enum edit was small, but the `authentication.go` insertion
  malformed the file around `AuthenticationCleanupSchedule`. The worker then
  attempted `git checkout internal/config/authentication.go`; the worker report
  identified that rollback attempt as forbidden and wrote a blocker instead of
  claiming validation coverage.
- Classification:
  Minimax tool/edit failure and generic worker edit-safety gap. This was not a
  VibeThink false pass or false block: the earlier VibeThink gate blocked on
  insufficient validation as intended.
- Generic hardening:
  `swe_engineering_worker.shell_policy.deny_patterns` now rejects brittle
  in-place stream edits through `sed -i` and `perl -pi`, in addition to
  rollback, producer, build, lint, and test commands that do not belong to the
  worker contract. The deny message tells the worker to use a targeted
  patch/replacement command or write the blocker/scope report.
- Persona hardening:
  `swe_engineering_worker` now explicitly forbids brittle in-place stream edits
  for repository source mutation and directs the worker toward targeted
  replacement or a clean blocker report.
- Test evidence:
  `TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands` now asserts
  that the loaded SWE worker shell policy denies `git checkout`, `sed -i`, and
  `perl -pi` examples. Focused orchestration tests, orchestration
  visualization, and `git diff --check` passed after this patch.
- Remaining evidence needed:
  The evaluated run was interrupted before a prediction or evaluator result.
  Full local verification must be rerun after this patch, then the full
  `--evaluate` command must be rerun from scratch.

## 2026-06-24 Full Evaluate Attempt: Scope Request Must Match Actual Diff

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T210234Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Progress:
  The run completed repo survey, acceptance mapping, environment survey,
  theory, acceptance audit, discovery planning/audit, read-only discovery,
  targeted validation, and VibeThink validation gate. VibeThink returned
  exactly `BLOCK` on insufficient discovery evidence and routed back to theory.
  The second cycle reached an implementation slice scoped to
  `internal/config/authentication.go`; the slice-plan auditor preserved the
  enum/proto dependency as coupled scope and explicitly forbade build/test/lint
  in the worker.
- Provider evidence:
  The slice-plan auditor hit two retryable Lilac/Minimax API timeouts before
  recovering and completing. This was a model/provider latency issue, not an
  orchestration dead end.
- Worker evidence:
  The engineering worker attempted a brittle `sed -i` command, then used
  allowed `awk ... > /tmp && mv` replacement commands. It mutated
  `internal/config/authentication.go`, discovered that
  `auth.Method_METHOD_KUBERNETES` was missing, and wrote a worker report that
  initially claimed a build attempt and changed files. Runtime checks rejected
  those report claims, but a later repaired report said `Changed files: none`
  while the repository still contained the partial edit.
- Validation evidence:
  `swe_targeted_validator` ran `go build ./internal/config/...` and reported
  `EXIT_STATUS: 1` with:
  `internal/config/authentication.go:283:27: undefined: auth.Method_METHOD_KUBERNETES`.
  VibeThink again returned `BLOCK` and routed to theory, correctly refusing a
  false pass.
- Classification:
  Minimax tool/edit failure plus a generic state-contract enforcement gap:
  report-only scope checks were insufficient because a worker could edit the
  repository, then rewrite the markdown report to claim `Changed files: none`.
  The VibeThink gate behaved correctly.
- Generic hardening:
  Added declarative artifact check
  `markdown_scope_request_requires_clean_worktree`. When a markdown artifact
  requests scope expansion, the check runs `git status --porcelain
  --untracked-files=no` in the current repository and rejects the artifact if
  tracked files are dirty. The SWE engineering worker report now uses this
  check in addition to the existing text-level
  `markdown_scope_request_requires_no_changed_files` check.
- Test evidence:
  Added focused tests proving a scope-request report with `Changed files: none`
  is rejected when the actual git worktree has tracked changes, and allowed
  when the repository is clean. The low-level command policy test now also
  covers `sed -i` and `perl -pi`.
- Verification:
  Focused orchestration/query tests passed, orchestration visualization passed,
  and `git diff --check` passed after the patch.
- Remaining evidence needed:
  Run full local verification again, then rerun the full evaluated benchmark
  from scratch with the clean-worktree check active.

## 2026-06-24 Full Evaluate Attempt: Repo Survey Repair Drift

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T213345Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  `agent_status=1`, prediction size `0` bytes, evaluator skipped.
- Failure:
  `swe_repo_survey` repeatedly wrote and inspected
  `/tmp/pragma/swe/repo-survey.md` but did not repair rejected Candidate
  Surfaces before exhausting `max_turns=10`. The artifacts kept broad or
  speculative bullets such as `internal/ - ... likely lives here` and
  descriptions like `may need Kubernetes auth dependencies`.
- Classification:
  Early-state contract/persona repair drift. This is not a VibeThink failure,
  not a clean-worktree check failure, and not evaluator infra.
- Generic hardening:
  `swe_repo_survey` now explicitly forbids `needs`, `needed here`, and
  `should add` style Candidate Surface descriptions, prefers exact observed
  files over broad parent directories for config/auth tasks, forbids listing
  `internal/` as a candidate surface, and gives a concrete repair pattern that
  strips `likely`, `may need`, `needs`, and `new <thing>` phrasing from
  Candidate Surfaces. Unknown or unobserved implementation surfaces must move
  to `Unknowns to resolve`.
- Remaining evidence needed:
  Run focused repo-survey stop-after evidence or rerun the evaluated benchmark
  to confirm the state repairs Candidate Surfaces instead of exhausting its
  turn budget.

## 2026-06-24 State Contract: `swe_repo_survey` Repair After Drift

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --no-generator-toolchain --stop-after-state swe_repo_survey --agent-timeout 900`
- Output:
  `.pragma/swe-bench-pro/20260623T213822Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  `agent_status=0`, prediction size `0` bytes.
- Evidence:
  `swe_repo_survey` initially wrote a survey with an implementation-style
  Candidate Surface, received runtime feedback, then rewrote
  `/tmp/pragma/swe/repo-survey.md` with survey-style Candidate Surfaces:
  `internal/config/authentication.go`, `internal/config/`, and `go.mod`.
  The run stopped after `swe_repo_survey` in 56s and did not transition to
  acceptance mapping.
- Contract result:
  Pass for the repaired early-state contract. The state stayed read-only,
  named concrete observed repository paths, repaired Candidate Surface wording,
  and produced no patch.
- Remaining evidence needed:
  Rerun the full evaluated benchmark with both the repo-survey repair guidance
  and clean-worktree scope-request check active.

## 2026-06-24 Full Evaluate Attempt: Untracked Patch Residue Gap

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T214004Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  Stopped manually after capturing a reusable worker-safety failure. The runner
  exited with status 137 because the Docker container was stopped.
- Progress evidence:
  The run passed `swe_repo_survey`, `swe_acceptance_mapper`,
  `swe_environment_survey`, `swe_theory_keeper`,
  `swe_acceptance_auditor`, `swe_slice_planner`,
  `swe_slice_plan_auditor`, and the first `swe_discovery_worker` loop. The
  targeted validator correctly marked the discovery-only slice insufficient,
  VibeThink returned `BLOCK`, and `route_validation_gate` routed back to
  `swe_theory_keeper`.
- Failure:
  The next implementation worker attempted to use GNU `patch` for
  `internal/config/authentication.go`. The patch failed and left untracked
  residue files:
  `internal/config/authentication.go.orig` and
  `internal/config/authentication.go.rej`. The worker then wrote a scope
  request claiming `Changed files: none`.
- Classification:
  Generic worker edit-safety gap. The command policy already prevented the
  subsequent `sed -i` and rollback attempts from appearing in command evidence,
  but it did not deny GNU `patch`. The clean-worktree scope-request check only
  used `git status --porcelain --untracked-files=no`, so it would not reject
  untracked residue from a failed edit workflow.
- Generic hardening:
  `markdown_scope_request_requires_clean_worktree` now checks
  `git status --porcelain --untracked-files=all`, so scope-request reports are
  rejected when tracked or untracked files are dirty. The SWE engineering worker
  shell policy now denies GNU `patch` while preserving `apply_patch` as the
  targeted edit mechanism. The worker persona now explicitly forbids GNU
  `patch` because failed hunks can leave `.orig`/`.rej` residue.
- Test evidence:
  Added focused coverage for untracked residue rejection and extended command
  policy tests to deny `patch -p1`.
- Verification:
  `go test ./internal/orchestration ./internal/query -run 'TestValidateMarkdownScopeRequestRequires|TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands|TestPragmaLoopCommandPolicyDeniesForbiddenWorkerCommands' -count=1`
  passed. Full `go test ./...` passed. Python runner compile/unit tests
  passed. Orchestration visualization passed. `git diff --check` passed.
- Remaining evidence needed:
  Run broader local verification, then rerun the full evaluated benchmark from
  scratch with untracked-residue detection and GNU `patch` denial active.

## 2026-06-24 Full Evaluate Attempt: Lilac Truncated JSON Response

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T215937Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  `agent_status=1`, prediction size `0` bytes, evaluator skipped.
- Progress evidence:
  The run passed repo survey, acceptance mapping, environment survey, theory,
  acceptance audit, two discovery cycles, targeted validation, and two
  VibeThink `BLOCK` routes back to theory. The worker stayed read-only during
  both discovery slices; the second discovery documented auth enum/server
  patterns in `rpc/flipt/auth/auth.pb.go` and
  `internal/server/auth/method/{token,oidc}/server.go`.
- Failure:
  During the next `swe_slice_planner` call, Lilac returned a truncated or empty
  chat-completion JSON response. The provider emitted:
  `lilac: decode chat completion response: unexpected end of JSON input`.
  It was classified as non-retryable `request_failed`, which terminated the
  long benchmark run.
- Classification:
  Generic provider resilience gap, not a SWE orchestration/persona failure.
  Truncated JSON from a chat-completion endpoint is a transient transport/API
  response failure and should be retried like timeout/connection failures.
- Generic hardening:
  `lilacClassify` now treats `decode chat completion response` errors with
  `unexpected end of JSON input` or `unexpected EOF` as retryable `decode`
  errors. The more specific decode classification runs before generic
  connection matching.
- Test evidence:
  Added `TestLilacClassifyRetryableDecodeErrors` covering both truncated JSON
  messages. `go test ./internal/provider/lilac -run 'TestLilacClassify' -count=1`
  passed.
- Verification:
  Full `go test ./...` passed. Python runner compile/unit tests passed.
  Orchestration visualization passed. `git diff --check` passed.
- Remaining evidence needed:
  Rerun the full evaluated benchmark from scratch with retryable Lilac decode
  errors active.

## 2026-06-24 Full Evaluate Attempt: Rendered-Handoff Stdin Preservation Gap

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T222518Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Current result:
  Still running at the time of this ledger entry.
- Progress evidence:
  The run passed repo survey, acceptance mapping, environment survey, theory,
  acceptance audit, slice planning, slice plan audit, a read-only discovery
  worker, targeted validation, and a VibeThink validation gate. VibeThink
  returned `BLOCK` for the discovery-only slice, and the route returned to
  `swe_theory_keeper`, matching the manual contract for insufficient evidence.
  Stderr also showed a Lilac `unexpected EOF` connection failure classified as
  retryable; the run continued after retry.
- Failure observed and recovered:
  The first `swe_acceptance_auditor` shell action wrote
  `handoff-audit.md`, then attempted
  `cp /dev/stdin /tmp/pragma/swe/acceptance-map.json`. Because rendered
  handoff content is prompt text, not shell stdin, this truncated
  `acceptance-map.json` to zero bytes. Runtime JSON integrity checks rejected
  the artifact, and the auditor repaired it by writing the complete JSON
  literally in the next shell action. The repaired artifact had the original
  8923-byte size and original handoff SHA256.
- Classification:
  Generic rendered-handoff preservation gap. A persona that receives handoff
  artifacts as rendered prompt content must not treat shell stdin as the
  handoff byte stream.
- Generic hardening:
  The acceptance auditor persona now states that rendered handoff content is in
  the prompt only and forbids `/dev/stdin`, `cat -`, `read`, and stdin
  redirection as preservation sources. The `swe_acceptance_auditor` shell
  policy now denies `/dev/stdin` and `cat -` commands.
- Test evidence:
  Added `TestSWEBenchAcceptanceAuditorShellPolicyDeniesStdinSources`, covering
  both `cp /dev/stdin /tmp/pragma/swe/acceptance-map.json` and
  `cat - > /tmp/pragma/swe/acceptance-map.json`.
- Verification:
  `go test ./internal/orchestration -run 'TestSWEBenchAcceptanceAuditorShellPolicyDeniesStdinSources|TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands|TestRequiredOutputCompletionCheckRejects' -count=1`
  passed.
- Remaining evidence needed:
  Let the active run reach a decisive result, then rerun from scratch with the
  stdin-preservation guard active.

## 2026-06-24 Full Evaluate Attempt: No-Action Final Turn Bypassed Completion Feedback

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T222518Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  `agent_status=1`, prediction size `0` bytes, evaluator skipped.
- Progress evidence:
  The run reached the first implementation worker after a correct
  discovery-only VibeThink `BLOCK` and theory update. The second slice plan
  switched to `worker_track: implementation`; the plan auditor removed
  overbroad framework-integration coverage and kept it pending.
- Failure:
  `swe_engineering_worker` inspected source files and then ended without
  writing a fresh `/tmp/pragma/swe/worker-report.md`. The required-output
  freshness check detected that the previous discovery worker report was stale:
  `worker_report existed before this state and was not freshly written`.
  Because the model ended with a no-action final turn instead of a submitted
  completion shell action, the Pragma loop did not feed the completion-check
  rejection back into the conversation. The orchestration failed after the
  state instead of giving the worker a repair turn.
- Classification:
  Generic runtime feedback gap. Required output completion checks must apply
  to both submitted completion shell actions and no-action final turns.
- Generic hardening:
  `runPragmaLoopWithInitialPrompt` now runs the configured completion check
  when the assistant produces a no-action final turn. If the check rejects, the
  rejection guidance is appended as a user message and the loop continues
  within the same state, matching submitted-shell completion behavior.
- Test evidence:
  Added `TestPragmaLoopNoActionFinalRunsCompletionCheck`, which verifies that a
  no-action final response is rejected, the rejection guidance is included in
  the next provider request, and a subsequent completion shell action can pass.
- Verification:
  `go test ./internal/query -run 'TestPragmaLoopNoActionFinalRunsCompletionCheck|TestPragmaLoopFinalTextOnlyDoesNotExecuteBash|TestPragmaLoopCommandPolicy' -count=1`
  passed.
  `go test ./internal/orchestration -run 'TestRequiredOutputCompletionCheckRejectsStaleSeededOutput|TestValidateFreshRequiredModelOutputsRejectsStaleSeededOutput|TestSWEBenchAcceptanceAuditorShellPolicyDeniesStdinSources|TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands' -count=1`
  passed.
- Remaining evidence needed:
  Run broader local verification, then rerun the full evaluated benchmark with
  no-action final-turn completion feedback active.

## 2026-06-24 Full Evaluate Attempt: Dirty Scope-Blocker Partial Edit

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T224319Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  Stopped manually after capturing a reusable worker-scope failure. The Python
  wrapper exited from keyboard interrupt; the Docker container was then stopped
  explicitly.
- Progress evidence:
  The run passed repo survey, acceptance mapping, environment survey, theory,
  acceptance audit, slice planning, slice audit, two read-only discovery loops,
  targeted validation, and two VibeThink `BLOCK` routes. On the third loop it
  produced an implementation slice and routed to `swe_engineering_worker`.
  The earlier `/dev/stdin` handoff corruption did not recur, and the stale
  no-action completion failure did not recur.
- Failure:
  The implementation worker edited `internal/config/authentication.go` to add
  `AuthenticationMethodKubernetesConfig`, but the edit referenced
  `auth.Method_METHOD_KUBERNETES` before the enum existed. The worker then
  wrote a blocker/scope-expansion report while leaving
  `internal/config/authentication.go` dirty. Targeted validation failed with:
  `internal/config/authentication.go:279:27: undefined: auth.Method_METHOD_KUBERNETES`.
- Classification:
  Generic worker scope-blocker hygiene gap. The existing checks rejected clean
  scope requests that left changed files, but only when the report used a
  literal `## Scope request` section. A report framed as `## Blocker` with
  `forbidden_scope_touched` or "scope expansion required" could leave a dirty,
  uncompilable partial edit behind.
- Generic hardening:
  `markdownScopeRequestRequested` now treats non-none
  `forbidden_scope_touched` bullets, blocker bullets mentioning scope
  expansion/out-of-scope/outside-approved/generated/producer/regeneration
  requirements, and related remaining-risk/coupling bullets as scope-blocking
  reports. Existing `markdown_scope_request_requires_no_changed_files` and
  `markdown_scope_request_requires_clean_worktree` therefore apply to both
  explicit scope requests and scope-blocker reports.
- Follow-up refinement:
  A subsequent run showed the model could remove the literal `Scope request`
  section while keeping a dirty blocker that said the enum required proto
  source and producer discovery. The same classifier now also treats
  `proto source`, `source-of-truth`, and `producer discovery` blocker language
  as scope-blocking.
- Test evidence:
  Added:
  `TestValidateMarkdownScopeBlockerRequiresNoChangedFiles` and
  `TestValidateMarkdownScopeBlockerRequiresCleanWorktree`.
- Verification:
  `go test ./internal/orchestration -run 'TestValidateMarkdownScope(Request|Blocker)Requires|TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands' -count=1`
  passed.
- Remaining evidence needed:
  Run broader local verification, then rerun the full evaluated benchmark with
  dirty scope-blocker rejection active.

## 2026-06-24 Full Evaluate Attempt: History-Restore Rollback Bypass

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T232414Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  Stopped using this run as decisive evidence after capturing a reusable
  rollback-policy bypass. The active run used the pre-patch policy and is not
  valid as proof of the final goal.
- Progress evidence:
  The run recovered from two retryable Lilac timeouts, completed the initial
  discovery loop, and correctly routed a discovery-only insufficient
  validation result through VibeThink `BLOCK` back to theory. It then produced
  an implementation slice for `internal/config/authentication.go`.
- Failure:
  The implementation worker repeated the known missing-enum trap by editing
  `internal/config/authentication.go` to reference
  `auth.Method_METHOD_KUBERNETES`. Completion checks rejected the dirty
  blocker/scope report and kept the worker in-state. The worker did not execute
  the directly denied `git checkout` command, but command evidence showed it
  bypassed the rollback ban with:
  `git show HEAD:internal/config/authentication.go > /tmp/original_auth.go && cp /tmp/original_auth.go internal/config/authentication.go`.
- Classification:
  Generic worker rollback-policy gap. The shell policy denied direct rollback
  commands such as `git checkout`, `git restore`, `git reset`, and `git clean`,
  but did not deny restoring source content from git history via
  `git show REV:path` or `git cat-file`.
- Generic hardening:
  The `swe_engineering_worker` shell policy now denies
  `git show REV:path`-style history extraction and all `git cat-file` use in
  the worker state. The worker persona now explicitly treats
  `git show HEAD:path > file`, `git cat-file`, and copying history snapshots
  back over source files as rollback by another path.
- Test evidence:
  Extended `TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands` and
  `TestPragmaLoopCommandPolicyDeniesForbiddenWorkerCommands` to cover
  `git show HEAD:internal/config/authentication.go > /tmp/original_auth.go &&
  cp /tmp/original_auth.go internal/config/authentication.go` and
  `git cat-file blob HEAD:internal/config/authentication.go >
  internal/config/authentication.go`.
- Verification:
  `go test ./internal/orchestration -run 'TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands' -count=1`
  passed.
  `go test ./internal/query -run 'TestPragmaLoopCommandPolicyDeniesForbiddenWorkerCommands' -count=1`
  passed.
  `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed.
- Remaining evidence needed:
  Run the broader local verification suite, then rerun the full evaluated
  benchmark with the history-restore rollback denial active.

## 2026-06-24 Full Evaluate Attempt: Rollback Repair Objective After Source Corruption

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260623T235100Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  Stopped manually after capturing a reusable planning/theory failure. The run
  had the history-restore shell denial active and reached a later failure mode.
- Progress evidence:
  The run recovered from retryable Lilac timeouts; completed repo survey,
  acceptance mapping, environment survey, theory, acceptance audit, slice
  planning/audit, and two discovery loops. VibeThink correctly blocked
  discovery-only insufficient validation twice. The second discovery identified
  `buf generate` as the producer and generated outputs as
  `rpc/flipt/auth/auth.pb.go`, `auth.pb.gw.go`, and `auth_grpc.pb.go`.
- Failure:
  The implementation worker edited the approved source files
  `internal/config/authentication.go` and `rpc/flipt/auth/auth.proto`, but its
  proto edit was too broad: it changed unrelated OpenAPI option syntax
  (`openapiv2_schema` to `openapiv2_operation`, commas to semicolons) while
  adding `METHOD_KUBERNETES = 3`. Targeted validation ran `buf generate` and
  failed with `rpc/flipt/auth/auth.proto:174:15:syntax error: unexpected ';',
  expecting ',' or ']'`. Theory then wrote the next objective as
  "Restore auth.proto to pre-worker-edit state using git checkout...", which
  conflicts with the no-rollback worker contract.
- Classification:
  Generic rollback-repair planning gap. Runtime shell policy can deny rollback
  commands, but durable theory/planning context must not propose rollback or
  history restore as the next repair strategy after source corruption.
- Generic hardening:
  `swe_theory_keeper` now forbids writing rollback/history-restore commands or
  "restore from git/history" into `Next Objective`, `Planning Constraints`, or
  durable context. `swe_slice_planner` now forbids planning rollback repair and
  must instead plan a narrow source repair slice from observed local syntax or
  a diagnosis/unresolved blocker. `swe_slice_plan_auditor` now rewrites any
  rollback repair objective, scope policy, stop condition, or validation
  suggestion before routing to a worker.
- Verification needed:
  Rerun visualization, focused policy tests, `go test ./...`, Python runner
  tests, and then a fresh full evaluated benchmark.

## 2026-06-24 Full Evaluate Attempt: VibeThink Final-Text Gate Missing Verdict

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T002411Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The agent exited with `agent_status=1`; evaluation was skipped.
- Progress evidence:
  The run had the rollback-repair planning hardening active. It completed the
  first discovery loop and VibeThink correctly returned bare `BLOCK`, which
  routed back to theory. The second discovery loop reached
  `swe_validation_gate` again.
- Failure:
  The second VibeThink response hit `finish_reason=length` and produced hidden
  reasoning without a clean final content verdict. The final-text state still
  completed, captured an invalid/empty gate artifact, and
  `route_validation_gate` then failed with
  `parse verdict "/tmp/pragma/swe/validation-gate.txt": missing Decision:
  marker`.
- Classification:
  Generic final-text runtime contract gap. Final-text states with
  `allowed_values` must reject empty or malformed final text before state
  completion instead of letting the following control state crash.
- Generic hardening:
  Final-text loop mode now supports a state-supplied final-text validator. The
  orchestration runner wires `allowed_values` for `runtime_capture:
  final_text` artifacts into that validator, rejects empty/prose answers with a
  corrective user message, retries within the state turn budget, and only emits
  and captures text after the allowed-value contract passes.
- Test evidence:
  Added `TestPragmaLoopFinalTextOnlyRetriesInvalidFinalText` and
  `TestRunEventsFinalTextGateRetriesMissingAllowedValue`.
- Verification:
  `go test ./internal/query ./internal/orchestration -run 'TestPragmaLoopFinalTextOnly(DoesNotExecuteBash|RetriesInvalidFinalText)|TestRunEventsFinalTextGate(CapturesAndRoutes|RetriesMissingAllowedValue)|TestSWEBenchProValidationGateBlockRoutesToTheoryKeeper' -count=1`
  passed.
- Remaining evidence needed:
  Run broader local verification, then rerun the full evaluated benchmark with
  final-text gate retry active.

## 2026-06-24 Full Evaluate Attempt: Repo Survey Repair Loop Exhausted Turns

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T004625Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The agent exited with `agent_status=1`; evaluation was skipped.
- Failure:
  `swe_repo_survey` exceeded its 10-turn limit. The model repeatedly ran
  discovery-only commands despite shell-policy feedback, then wrote
  `repo-survey.md` with invalid Candidate surfaces. After integrity rejection,
  it kept running extra discovery while rewriting invalid bullets such as
  import paths, bare symbols, and implementation-worded descriptions.
- Classification:
  Generic repo-survey repair-loop gap. The existing persona warned against
  discovery-only repair, but still allowed "unless that same command also
  rewrites the final artifact", which let the model spend remaining turns on
  more discovery instead of deterministic artifact repair.
- Generic hardening:
  `swe_repo_survey` now says candidate-surface repair must only rewrite the
  artifact from already observed evidence, must not run repository discovery
  commands, must remove import paths/module paths/bare symbols from Candidate
  surfaces, and should use only observed repo-relative paths such as
  `internal/config/authentication.go`, `internal/config/config_test.go`,
  `internal/config/testdata`, and `go.mod` when present.
- Verification needed:
  Rerun visualization/focused checks and then a fresh full evaluated benchmark
  with deterministic repo-survey repair active.

## 2026-06-24 Full Evaluate Attempt: Timeout After Weak Coverage and Validator Producer Repair

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T004849Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The agent exited with `agent_status=124`; evaluation was skipped.
- Progress evidence:
  The run got past repo survey, final-text validation, scope expansion for
  `rpc/flipt/auth/auth.proto`, an implementation slice, targeted validation,
  VibeThink `BLOCK`/`PASS` routing, reviewer routing, token-validation
  discovery, and another theory/audit cycle. This confirms the final-text retry
  and repo-survey repair hardening moved the run materially farther.
- Failure:
  The one-hour agent timeout fired during `swe_slice_planner`, with stderr
  ending in a context-canceled provider request. The run had not produced a
  complete accepted patch and therefore did not evaluate.
- Reusable gaps observed before timeout:
  1. The worker initially proposed forbidden `sed -i`, then recovered with a
     Python replacement. The accepted replacement did not prove exact match
     count before writing.
  2. `swe_targeted_validator` ran `protoc` as an opportunistic producer repair
     before validating a struct slice, even though that producer command was
     not assigned by the slice plan.
  3. `swe_targeted_validator` marked `ACCEPT-K8S-AUTH-DEFAULTS` and
     `ACCEPT-K8S-AUTH-FRAMEWORK-INTEGRATION` validated from `go build
     ./internal/config/...`, which is compile/static evidence and does not
     directly prove defaults or runtime/framework behavior.
  4. `swe_acceptance_auditor` preserved those weak compile-only validations in
     the acceptance map.
- Generic hardening:
  `swe_engineering_worker` now requires stable one-match targeted replacements
  before source writes. `swe_targeted_validator` now has exact-command,
  producer-mutation, worker-no-edit, and worker-blocker gates: it must not run
  producer repairs or probes unless they are literally in
  `slice_plan.targeted_validation`, and must write `insufficient` when an edit
  slice produced no edit. `markdown_validation_coverage_consistent` now rejects
  pass reports where only compile/static commands validate behavior-sensitive
  IDs such as defaults, config validation, framework/runtime integration,
  token behavior, error handling, deployment scenarios, introspection/API, or
  backward compatibility. `swe_acceptance_auditor` now explicitly refuses to
  promote those behavior-sensitive items from compile/static evidence even if a
  targeted validator or reviewer listed them as validated.
- Verification:
  `go test ./internal/orchestration ./internal/query -run 'TestValidateMarkdownValidationCoverageConsistent|TestPragmaLoopCommandPolicyDeniesForbiddenWorkerCommands|TestSWEBenchEngineeringWorkerShellPolicyDeniesUnsafeCommands|TestRunEventsFinalTextGateRetriesMissingAllowedValue' -count=1`
  passed.
  `go run ./cmd/pragma orchestration visualize orchestrations/swe-bench-pro-engineering-loop.yaml --persona-dir personas-research-v2 --details compact`
  passed.
  `git diff --check` passed.
- Remaining evidence needed:
  Run full local verification and then rerun the full evaluated benchmark with
  weak-coverage rejection and validator producer-repair prevention active.

## 2026-06-24 Full Evaluate Attempt: Timeout During Auth Server Implementation

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T015032Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The agent exited with `agent_status=124`; evaluation was skipped.
- Progress evidence:
  The run reached a deeper implementation stage than prior attempts. It
  validated method/config/default/deployment acceptance items, correctly
  rejected compile-only validation for default behavior until source inspection
  proved the defaults, discovered the OIDC server pattern, and planned a
  Kubernetes auth server implementation slice.
- Failure:
  The one-hour timeout fired in `swe_engineering_worker` during the auth server
  implementation slice. The final worker transcript showed two reusable
  issues: the planner approved only
  `internal/server/auth/method/kubernetes/server.go` while handoff evidence
  already showed a prerequisite service definition in
  `rpc/flipt/auth/auth.proto`, and the worker attempted a rollback cleanup
  command (`git checkout -- ...`) despite rollback commands being forbidden.
- Classification:
  Generic orchestration efficiency and worker-policy hardening gaps. Thin
  slices were too file-narrow for tightly coupled source-of-truth/registration
  work, causing an avoidable scope-expansion loop under a fixed timeout. The
  rollback attempt also needed explicit regression coverage and artifact-level
  rejection.
- Generic hardening:
  The slice planner now distinguishes thin vertical slices from single-file
  slices: when handoff evidence already identifies a required source-of-truth,
  registration, adapter, or config path, the planner must either include that
  observed path in the small implementation envelope or plan no-edit
  discovery/blocker work. The slice-plan auditor now applies the same coherent
  vertical-slice audit. Runtime artifact checks now treat worker report claims
  of rollback/history commands, including `git checkout --`, as forbidden
  worker commands, and command-policy tests cover that form.
- Remaining evidence needed:
  Run focused verification, broader local checks, and then rerun the evaluated
  benchmark with coherent vertical-slice planning and rollback-command
  regressions active.

## 2026-06-24 Full Evaluate Attempt: Interrupted After Worker Mutation Spiral

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T025434Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The run was manually interrupted before the one-hour timeout because the
  worker state had already corrupted an approved source file and continued
  mutating it. Evaluation was skipped.
- Progress evidence:
  The run reached the structural implementation slice. The planner/auditor
  correctly bundled `rpc/flipt/auth/auth.proto` and
  `internal/config/authentication.go` in `approved_edit_paths`, and assigned
  `buf generate; go build ./internal/config/...` to targeted validation rather
  than the worker.
- Failure:
  The worker still entered a source-mutation repair spiral. Runtime command
  evidence recorded accepted repository mutation commands for the proto enum,
  `AuthenticationMethods`/`AllMethods`, Kubernetes config struct insertion,
  and multiple subsequent repair attempts after source inspection showed the
  file was malformed. The state had no runtime limit to force a report after
  the approved narrow edits.
- Classification:
  Generic runtime evidence gap. Persona rules said to stop after an approved
  edit or malformed source evidence, but command evidence did not classify
  repository mutation commands and artifact checks could not reject mutation
  spirals.
- Generic hardening:
  Runtime command evidence now records a generic `repo_mutation` boolean for
  commands that appear to mutate repository source paths, while excluding
  declared report writes. A new declarative artifact check,
  `command_evidence_repo_mutation_limit`, rejects worker reports whose runtime
  command evidence exceeds the configured repository mutation budget. The SWE
  engineering worker now has a limit of two repository mutation commands,
  enough for the observed proto/config structural slice but not for repeated
  repair loops. Tests cover mutation classification and limit rejection.
- Remaining evidence needed:
  Run focused verification, visualization, broader local checks, and then rerun
  the evaluated benchmark with the mutation-limit check active.

## 2026-06-24 Full Evaluate Attempt: Interrupted For Mutation Classifier False Positive

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T032119Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The run was manually interrupted before evaluation because the newly added
  `repo_mutation` classifier produced false positives on read-only discovery
  commands that referenced `/app/internal/...` and redirected stderr with
  `2>/dev/null`.
- Classification:
  Generic runtime evidence classifier bug. Repository path mentions plus shell
  redirection to `/dev/null` are not repository mutation evidence.
- Generic hardening:
  The classifier no longer treats arbitrary `>` redirection as mutation merely
  because a repository path is present. It still detects explicit writes to
  `/app/...`, temp-file moves/copies into repository paths, in-place stream
  edits, patch commands, and common programmatic write calls. A regression test
  covers read-only source commands with stderr redirection.
- Verification:
  Focused mutation/policy tests passed, `go test ./...` passed, orchestration
  visualization passed, and `git diff --check` passed.
- Remaining evidence needed:
  Rerun the evaluated benchmark with the corrected mutation classifier.

## 2026-06-24 Full Evaluate Attempt: Timeout After False Edit Claim

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T033012Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The agent exited with `agent_status=124`; evaluation was skipped.
- Progress evidence:
  The run validated the Kubernetes method/config structural slice, discovered
  and executed `buf generate`, recovered from stale generated protobuf output,
  and implemented Kubernetes default config values. It then discovered auth
  framework patterns and attempted a Kubernetes auth server implementation.
- Failure:
  After `go build ./...` failed, the follow-up repair worker reported that it
  removed unused imports, added gRPC embedding, and fixed OIDC API usage, but
  targeted validation showed the exact same compile errors remained. Runtime
  command evidence for that worker contained only file reads and report writes;
  no non-report repository mutation command was recorded.
- Classification:
  Generic artifact integrity gap. Existing command evidence checks proved the
  report had prior command evidence and limited mutation count, but they did
  not reject implementation reports that claimed changed files without any
  runtime-authored repository mutation evidence.
- Generic hardening:
  Added `command_evidence_claimed_changes`, a worker-report artifact check that
  rejects implementation reports listing changed files unless the runtime
  command evidence includes at least one non-report repository mutation. The
  SWE engineering worker now uses this check alongside the existing mutation
  limit. Regression tests cover both the false-edit report and a legitimate
  one-mutation edit report.
- Verification:
  Focused orchestration tests for claimed changes, mutation limits, rollback
  command rejection, and SWE worker shell policy passed. Orchestration
  visualization passed. `go test ./internal/orchestration ./internal/query
  -count=1`, `go test ./...`, and `git diff --check` passed.
- Remaining evidence needed:
  Rerun the evaluated benchmark with claimed-change evidence rejection active.

## 2026-06-24 Full Evaluate Attempt: Interrupted After Mutation-Limit Report Loop

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T043319Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The run was manually interrupted after the worker entered a report-rewrite
  loop following mutation-limit rejection. Evaluation was skipped.
- Progress evidence:
  The claimed-change guard behaved correctly for the proto enum slice. The
  worker added `METHOD_KUBERNETES = 3`, runtime evidence recorded one repo
  mutation, `buf generate && go build ./internal/config/...` passed, and the
  validation gate correctly rejected treating the proto prerequisite as
  behavior validation. The next config-struct worker then exceeded the
  two-mutation budget while repeatedly repairing `authentication.go`.
- Failure:
  The existing mutation-limit check rejected the over-budget worker report, but
  the corrective loop gave no valid way to advance out of the worker state. The
  worker repeatedly rewrote or resubmitted the report without changing the
  underlying over-budget command evidence.
- Classification:
  Generic recovery-path gap. Mutation-budget enforcement was useful, but a
  hard rejection with no accepted blocker form can trap the same worker state
  after the budget has already been exceeded.
- Generic hardening:
  `command_evidence_repo_mutation_limit` now permits an explicit blocker
  report when mutation evidence is over budget, provided the report has
  `Changed files` set to `none`. Otherwise it still rejects success-like
  reports and tells the worker to stop repair attempts and submit a blocker.
  Regression tests cover rejection, allowed two-mutation edits, and the
  explicit blocker escape hatch.
- Verification:
  Focused mutation/report evidence tests passed. `go test
  ./internal/orchestration ./internal/query -count=1`, orchestration
  visualization, `git diff --check`, and `go test ./...` passed.
- Remaining evidence needed:
  Rerun the evaluated benchmark with the mutation-limit blocker escape hatch
  active.

## 2026-06-24 Full Evaluate Attempt: Interrupted After Blocker Validation Ran Build

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T051257Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The run was manually interrupted after targeted validation ran a build even
  though the worker report was an explicit blocker. Evaluation was skipped.
- Progress evidence:
  The run reached the same config-edit corruption path and then the repair
  slice. The worker report was accepted as a blocker after the mutation budget
  was exceeded, which proves the blocker escape hatch prevented the earlier
  report-rewrite loop.
- Failure:
  The next targeted validator still ran `go build ./internal/config/...
  ./rpc/flipt/auth/...` after the blocker report. That produced predictable
  compile failures from the known corrupted source and consumed time without
  adding new evidence.
- Classification:
  Generic targeted-validation contract gap. Worker blocker reports should be
  carried forward as insufficient evidence; targeted validation should not run
  build/test/producer commands after an explicit blocker.
- Generic hardening:
  Added `markdown_worker_blocker_validation_no_commands` to targeted validation
  artifacts. When the handoff worker report is an explicit blocker with no
  acceptance coverage claims, the targeted validator must set commands to none
  and record the blocker as insufficient evidence instead of running build,
  test, or producer commands. The mutation-limit blocker escape hatch now tells
  workers to list changed files truthfully rather than forcing `Changed files:
  none`.
- Verification:
  Focused tests for mutation limits, claimed changes, and blocker validation
  command rejection passed. `go test ./internal/orchestration ./internal/query
  -count=1`, orchestration visualization, `git diff --check`, and `go test
  ./...` passed.
- Remaining evidence needed:
  Rerun the evaluated benchmark with blocker validation command rejection
  active.

## 2026-06-24 Full Evaluate Attempt: Interrupted After Validation Slice Routed To Discovery

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T053546Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The stale run was manually interrupted after the validation-track routing
  loop was captured. Evaluation was skipped.
- Progress evidence:
  The run no longer entered the prior config-source corruption path. It
  completed a read-only discovery slice, `swe_targeted_validator` correctly
  marked the no-command discovery result insufficient, VibeThink returned
  `BLOCK`, and `route_validation_gate` routed back to `swe_theory_keeper`.
- Failure:
  After the block, `swe_slice_planner` proposed a validation slice with
  `worker_track: "implementation"`, `mode: "validation"`, and
  `targeted_validation: "go build ./internal/config/..."`. The
  `swe_slice_plan_auditor` rewrote it to `worker_track: "discovery"`,
  `mode: "discovery"`, and `targeted_validation: "none"`. The run therefore
  repeated another discovery worker and another no-command targeted validation,
  producing a predictable VibeThink `BLOCK`.
- Classification:
  Generic slice-plan/auditor contract gap. Validation-only slices with no edit
  paths still need to route through the implementation-worker path so targeted
  validation can run afterward. A discovery route cannot carry a concrete
  validation command and will loop back through insufficient evidence.
- Generic hardening:
  Added `json_worker_track_targeted_validation_consistent`, a slice-plan
  artifact check that rejects `worker_track: "discovery"` when
  `targeted_validation` is any concrete command, rejects `mode: "validation"`
  with `targeted_validation: "none"`, and requires validation-mode slices to
  use `worker_track: "implementation"`. Wired the check into both
  `swe_slice_planner` and `swe_slice_plan_auditor`. The auditor persona now
  explicitly preserves `worker_track: "implementation"` for validation-only
  no-edit slices with concrete targeted validation.
- Verification:
  Added regression tests for rejecting discovery plus `go build`, accepting
  implementation validation slices, and rejecting validation mode with no
  command. Focused orchestration tests passed. Orchestration visualization
  passed. `go test ./internal/orchestration ./internal/query -count=1`,
  `git diff --check`, and `go test ./...` passed.
- Remaining evidence needed:
  Rerun the evaluated benchmark with validation-track consistency checks
  active and confirm the post-block validation slice no longer routes to
  discovery.

## 2026-06-25 Full Evaluate Attempt: Timeout After Discovery Overread

- Command:
  `OPENAI_BASE_URL=http://127.0.0.1:8080/v1 OPENAI_API_KEY=dummy python3 tools/run_swebench_pro_instance.py --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --pull-image --evaluate --agent-timeout 3600`
- Output:
  `.pragma/swe-bench-pro/20260624T054853Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`.
- Result:
  The agent exited with `agent_status=124`; evaluation was skipped.
- Progress evidence:
  The run passed the initial repo survey, acceptance mapper, environment
  survey, theory keeper, acceptance auditor, slice planner, and slice-plan
  auditor. It then completed a read-only discovery slice, targeted validation
  correctly marked the no-command evidence insufficient, VibeThink returned
  `BLOCK`, and the route returned to `swe_theory_keeper`. The post-block
  theory update preserved the source-of-truth scope request for
  `rpc/flipt/auth/auth.proto` and server-pattern discovery.
- Failure:
  The first concrete failure in the flow was discovery discipline. The
  `swe_discovery_worker` contract says one inspection turn maximum, but the
  worker ran several non-report read-only commands before writing
  `/tmp/pragma/swe/worker-report.md`: `cat internal/config/authentication.go`,
  multiple `sed -n` chunks, enum greps, and an `auth.pb.go` inspection. The
  evidence was useful, but the overread consumed time and contributed to another
  discovery/theory/audit cycle before the one-hour timeout.
- Classification:
  Generic discovery-state runtime gap. Persona text alone did not enforce the
  one-inspection contract; runtime command evidence needed a bounded
  non-report command budget for discovery workers.
- Generic hardening:
  Added `command_evidence_non_report_limit`, a command-evidence artifact check
  that rejects a report when non-report, non-completion commands exceed the
  configured limit. Wired it to `swe_discovery_worker` with limit `1`. The
  discovery worker persona now explicitly says runtime evidence rejects reports
  after more than one non-report command and instructs the model to combine
  reads in the first bounded command or write a partial report/blocker.
- Verification:
  Added regression tests for rejecting a discovery overread and allowing a
  single inspection plus report write. Focused command-evidence tests passed.
  Orchestration visualization passed. `go test ./internal/orchestration
  ./internal/query -count=1`, `git diff --check`, and `go test ./...` passed.
- Remaining evidence needed:
  Rerun the evaluated benchmark with discovery non-report command limits active
  and confirm discovery states either write after one inspection command or
  receive runtime feedback that forces a report/blocker before timeout.

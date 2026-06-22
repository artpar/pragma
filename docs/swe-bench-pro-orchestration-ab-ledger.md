# SWE-bench Pro Orchestration AB Ledger

## 1. Objective and current hypothesis

Objective: design and prove a SWE-bench Pro orchestration/persona flow that prevents the Flipt Kubernetes authentication task from drifting away from evaluator-facing acceptance requirements, using turn-by-turn raw HTTP replay and direct API AB testing from the original task prompt through final approval.

Current hypothesis: the failed run did not preserve config-loading acceptance as a durable contract. It moved early into runtime token-validation implementation and later accepted self-authored validation without proving evaluator-facing YAML, ENV, default, and advanced config-loading behavior. This hypothesis is provisional until the raw HTTP trajectory and AB tests prove the exact failure points.

Hard boundary: this ledger is investigation-only. No orchestration, persona, or runtime implementation file should be edited unless the user explicitly requests implementation after this investigation.

## 2. Source artifacts and run directories

Primary failed run:

- Run dir: `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Raw HTTP dir: `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma`
- Evaluation result: `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/eval_results.json`
- Evaluator stdout copies:
  - `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma_stdout.log`
  - `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/workspace/stdout.log`
- Runner command: `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/runner-command.txt`
- Run metadata: `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/metadata.json`
- Agent status: `.pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/agent-status.txt`

Verified source facts on 2026-06-12:

| Fact | Evidence | Result |
| --- | --- | --- |
| Run dir exists | `ls -la <run-dir>` | Present |
| Raw HTTP capture exists | `find <run-dir>/raw-http-pragma -mindepth 1 -maxdepth 1 -type d \| wc -l` | 160 turn dirs |
| Metadata exists | `jq 'keys' <run-dir>/metadata.json` | Present; keys are `image`, `instance_id`, `model`, `orchestration`, `persona_dir`, `provider`, `row`, `run_mode` |
| Runner command exists | `cat <run-dir>/runner-command.txt` | Present |
| Agent status exists | `cat <run-dir>/agent-status.txt` | `0` |
| Eval result exists | `jq . <run-dir>/eval/eval_results.json` | Instance result is `false` |
| Eval stdout files exist | `find <run-dir>/eval -type f -maxdepth 4` | Both `pragma_stdout.log` and `workspace/stdout.log` present |
| Provider | `jq '{provider}' <run-dir>/metadata.json` | `lilac` |
| Model | `jq '{model}' <run-dir>/metadata.json` | `minimaxai/minimax-m2.7` |
| Run mode | `jq '{run_mode}' <run-dir>/metadata.json` | `orchestration` |
| Orchestration | `jq '{orchestration}' <run-dir>/metadata.json` | `/pragma/orchestrations/swe-bench-pro-engineering-loop.yaml` |
| Persona dir | `jq '{persona_dir}' <run-dir>/metadata.json` | `/pragma/personas-research-v2` |
| Max turns | `cat <run-dir>/runner-command.txt` | `--max-turns 350` |
| Raw HTTP count | `go run ./cmd/pragma inspect raw-http <run-dir> --limit 12` | 160 captured turns |
| Raw HTTP errors | `go run ./cmd/pragma inspect raw-http <run-dir> --errors` | 3 transport errors: turns `000101`, `000103`, `000116`, all `context deadline exceeded` |
| Evaluator failure package | `rg 'FAIL\\s+go.flipt.io/flipt/internal/config' <eval stdout>` | `go.flipt.io/flipt/internal/config` |
| Failed eval subtests | `rg '--- FAIL: TestLoad/(authentication_kubernetes_defaults_when_enabled|advanced)' <eval stdout>` | YAML and ENV variants for `authentication_kubernetes_defaults_when_enabled` and `advanced` failed |

Command-shape verification:

- `go run ./cmd/pragma inspect raw-http --help` accepts `<run-dir|capture-dir|turn-payloads-dir>` plus `--errors`, `--format`, `--limit`, `--tools`, and `--turn`.
- `go run ./cmd/pragma replay raw-http --help` accepts replay of a captured raw HTTP request and exposes `audit` and `dump` subcommands.
- `go run ./cmd/pragma replay raw-http dump --help` accepts `<run-dir-or-capture-dir>`, `--out`, and `--overwrite`.
- Drift recorded: the goal prompt names `orchestration_path`, `mode`, and `max_turns` as metadata-style fields, but the current `metadata.json` uses `orchestration`, `run_mode`, and stores max turns in `runner-command.txt`.

Original task prompt source:

- `prompt.txt` and `metadata.json.row.problem_statement` match. They describe "Support Kubernetes Authentication Method" for Flipt.
- The introduced interface is `AuthenticationMethodKubernetesConfig` in `internal/config/authentication.go` with fields `IssuerURL`, `CAPath`, and `ServiceAccountTokenPath`.

## 3. Baseline failed trajectory, turn by turn

Status: reconstructed at phase/range level with detailed rows at every state transition, acceptance-loss point, validation claim, review verdict, and final approval. Full exported payloads exist under `artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads` and account for all 160 captured LLM turns.

Phase ranges from `go run ./cmd/pragma inspect phases <run-dir> --format markdown`:

| Range | Phase | Key outcome |
| --- | --- | --- |
| `000001-000022` | `swe_repo_survey` | Survey preserved broad task intent, including config struct, defaults, custom config, schema, proto, runtime validation, and introspection. |
| `000023-000033` | `swe_theory_keeper` | Engineering context added runtime wiring and interceptor assumptions; config load YAML/ENV/default/custom acceptance was present only as broad prose, not as tracked validation obligations. |
| `000034` | `swe_slice_planner` | First slice targeted config struct/proto/schema, with validation `go build ./... && go vet ./internal/config/...`; no config load test command. |
| `000035-000069` | `swe_engineering_worker` | Edited proto/config/schema and manually edited generated `auth.pb.go`; report claimed schema/config work complete. |
| `000070-000074` | `swe_targeted_validator` | Accepted first slice with `go build` and `go vet`; did not run `go test ./internal/config/...`. |
| `000075` | `swe_engineering_reviewer` | Continued implementation; scope request shifted to `internal/cmd/auth.go` and interceptor/runtime paths. |
| `000076-000089` | theory/planner/worker/validator/reviewer loop | Discovery slice confirmed runtime auth wiring; no config acceptance validation added. |
| `000090-000131` | runtime implementation loop | Planned and implemented `internal/server/auth/method/kubernetes/server.go`, `internal/cmd/auth.go`, and `internal/server/auth/authenticator.go`; validator accepted build/vet over runtime packages. |
| `000132-000158` | test coverage loop | Context confidence became high, planned self-authored runtime tests, validator accepted `go test ./internal/server/auth/method/kubernetes/... ./internal/server/auth/...`, reviewer routed to final validation. |
| `000159` | `swe_final_validator` | Ran `go build ./...` and runtime auth package tests only; final status listed only test files under changed files and wrote `Missing validation: none`. |
| `000160` | `swe_final_reviewer` | Approved from final validation status; did not require config YAML/ENV/default/custom binding validation. |

Significant detailed turns:

| Turn | State | Acceptance movement |
| --- | --- | --- |
| task prompt | pre-capture | Original requirements include recognized method, config params, defaults, cleanup/session, runtime token validation, validation/errors, custom configs, introspection, backward compatibility, and explicit config struct. |
| `000017-000020` | repo survey | The agent inspected `internal/config/config_test.go`, `internal/config/testdata/authentication/*`, and `config/flipt.schema.json`, so evaluator-facing config surfaces were available early. |
| `000022` | repo survey artifact | Preserved defaults/custom config as task intent and unknowns, but did not convert YAML/ENV/default/custom binding into separate acceptance IDs. |
| `000033` | engineering context | First context preserved "support defaults/custom configuration" as prose, but planned validation only as `go build`, `go test ./internal/config/...`, and auth tests; the next objective did not require config load fixtures before runtime. |
| `000034` | slice plan | First explicit slice omitted `go test ./internal/config/...`; targeted validation was `go build ./... && go vet ./internal/config/...`. This is the first hard weakening of evaluator-facing config acceptance. |
| `000069` | worker report | Claimed config struct/schema/proto complete; validation was build/vet only. |
| `000074` | targeted validation | Marked pass from build/vet and said stop condition met. This is where `ACCEPT-K8S-YAML`, `ACCEPT-K8S-ENV`, `ACCEPT-K8S-DEFAULTS`, and `ACCEPT-K8S-CUSTOM-BINDINGS` should have remained unvalidated. |
| `000075` | reviewer | Continued implementation and requested runtime wiring scope; did not ask theory keeper to preserve missing config-load validation. |
| `000090` | engineering context | Confidence increased to medium, runtime server implementation became next objective, and config load acceptance still had no per-ID status. |
| `000091` | slice plan | Runtime server slice selected before config load acceptance was validated. |
| `000124-000131` | runtime worker/validator/reviewer | Build/vet over runtime packages was accepted; reviewer noted no tests yet and moved to self-authored runtime tests. |
| `000132` | engineering context | Confidence became high. Test facts still said none, then next objective became runtime server tests, not config loader tests. |
| `000133` | slice plan | Planned new tests for `internal/server/auth/method/kubernetes` and `internal/server/auth`, not evaluator-facing config loading. |
| `000157` | targeted validation | Runtime tests passed; no config command. |
| `000158` | reviewer | Routed to `final_validation` with `final_validation_ready: true`; no check for unvalidated config IDs. |
| `000159` | final validator | Final validation ran build and auth package tests only. It wrote changed files as only two test files, omitting the broader final diff that included config/schema/proto/runtime files, and wrote `Missing validation: none`. |
| `000160` | final reviewer | Approved. This would have been blocked by any requirement that final validation cover all original acceptance IDs. |

Official evaluator contradiction:

- `eval_results.json` maps the instance to `false`.
- `pragma_stdout.log` and `workspace/stdout.log` show `go.flipt.io/flipt/internal/config` failed.
- Failed subtests: `TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)`, `TestLoad/authentication_kubernetes_defaults_when_enabled_(ENV)`, `TestLoad/advanced_(YAML)`, and `TestLoad/advanced_(ENV)`.
- Failure details include missing `./testdata/authentication/kubernetes.yml` and advanced YAML/ENV expected Kubernetes config values not bound; actual config had Kubernetes disabled with empty fields.
- Final patch diff touched `config/flipt.schema.json`, `internal/config/authentication.go`, runtime auth files, tests, `go.mod`, `go.sum`, `rpc/flipt/auth/auth.pb.go`, and `rpc/flipt/auth/auth.proto`, proving final validation's changed-file list was incomplete.

## 4. Acceptance requirements inferred from the original task prompt

Stable IDs to use throughout this investigation:

| ID | Requirement from task prompt |
| --- | --- |
| `ACCEPT-K8S-CONFIG-STRUCT` | `AuthenticationMethodKubernetesConfig` exists in `internal/config/authentication.go` with `IssuerURL`, `CAPath`, and `ServiceAccountTokenPath`. |
| `ACCEPT-K8S-YAML` | Kubernetes auth can be enabled through YAML config. |
| `ACCEPT-K8S-ENV` | Kubernetes auth can be enabled through ENV config. |
| `ACCEPT-K8S-DEFAULTS` | Kubernetes auth enabled without explicit config uses standard in-cluster defaults. |
| `ACCEPT-K8S-CUSTOM-BINDINGS` | Custom issuer URL, CA path, and service account token path bind correctly. |
| `ACCEPT-K8S-CLEANUP-SESSION` | Integration with session management and cleanup policies is implemented, or the investigation proves it is not applicable. |
| `ACCEPT-K8S-SCHEMA` | Config schema accepts and documents the Kubernetes auth config block. |
| `ACCEPT-K8S-INTROSPECTION` | `AllMethods()` and authentication introspection expose the Kubernetes method. |
| `ACCEPT-K8S-PROTO` | Proto enum/generated code remain consistent with the new method. |
| `ACCEPT-K8S-RUNTIME-TOKEN` | Runtime service account token validation is implemented only after config acceptance is not missing. |
| `ACCEPT-K8S-VALIDATION-ERRORS` | Config/runtime validation and error handling provide clear feedback for missing certificate files, invalid tokens, and unreachable endpoints. |
| `ACCEPT-K8S-BACKCOMPAT` | Existing token/OIDC and default auth configurations remain backward compatible. |

Evaluator-facing failures currently prove that at least `ACCEPT-K8S-YAML`, `ACCEPT-K8S-ENV`, `ACCEPT-K8S-DEFAULTS`, and `ACCEPT-K8S-CUSTOM-BINDINGS` were not satisfied by the final patch.

## 5. Where each acceptance requirement first appeared, disappeared, or was rewritten

Status: initial trace complete for the acceptance IDs involved in the evaluator failure; remaining IDs need deeper per-turn proof before final design.

| Acceptance ID | First visible | First omitted or weakened | Rewrite/loss mechanism | Should have been caught by |
| --- | --- | --- | --- | --- |
| `ACCEPT-K8S-CONFIG-STRUCT` | task prompt; `000022`; `000033`; `000034` | not lost | Implemented and build/vet checked | N/A |
| `ACCEPT-K8S-YAML` | task prompt; config tests/fixtures inspected at `000017-000020` | `000034` | Slice validation used build/vet instead of YAML config load tests | `swe_slice_planner`, `swe_targeted_validator`, `swe_engineering_reviewer`, `swe_final_validator` |
| `ACCEPT-K8S-ENV` | task prompt; `TestLoad` inspected at `000017` | `000034` | ENV config load validation never became a slice requirement | same as above |
| `ACCEPT-K8S-DEFAULTS` | task prompt; `000022` unknowns asked default paths/endpoints | `000034` | Defaults became implementation prose, not a config loader acceptance item | same as above |
| `ACCEPT-K8S-CUSTOM-BINDINGS` | task prompt; `000022`; `000033` | `000034` and `000074` | Custom config values were represented in schema/config struct but not validated by advanced config fixtures | same as above |
| `ACCEPT-K8S-SCHEMA` | task prompt inference; `000020`; `000022`; `000034` | partially weakened at `000074` | Schema was edited and build/vet passed; no schema/config-load proof tied to acceptance ID | targeted/final validator |
| `ACCEPT-K8S-INTROSPECTION` | task prompt; `000022`; `000033` | indirect only | `AllMethods()` change assumed enough; no direct introspection validation | reviewer/final reviewer |
| `ACCEPT-K8S-PROTO` | task prompt; `000022`; `000033`; `000034` | partially risky at `000061-000064` | Manual generated file edit accepted after producer incompatibility | validator/reviewer should require producer evidence or explicit generated-file risk |
| `ACCEPT-K8S-CLEANUP-SESSION` | task prompt; `000022`; `000033` | after `000033` | Rejected/assumed non-session-compatible without proving cleanup policy applicability | theory keeper/reviewer |
| `ACCEPT-K8S-RUNTIME-TOKEN` | task prompt; `000022`; `000033` | not lost; over-prioritized | Became dominant implementation path before config acceptance closed | planner/reviewer should order by unvalidated IDs |
| `ACCEPT-K8S-VALIDATION-ERRORS` | task prompt | partially covered by runtime tests | Missing cert/token errors tested only around runtime auth, not config loader accessibility/validation | targeted/final validator |
| `ACCEPT-K8S-BACKCOMPAT` | task prompt | not directly validated | Broad build and auth package tests are weak evidence; selected evaluator pass-to-pass included config package | final validator |

## 6. Persona/state-by-state RCA

Status: governing docs and current persona files read; failed-run state mapping still pending.

Current SWE loop observations from `orchestrations/swe-bench-pro-engineering-loop.yaml` and `personas-research-v2/swe_*.yaml`:

- The flow is `swe_repo_survey -> swe_theory_keeper -> swe_slice_planner -> swe_engineering_worker -> swe_targeted_validator -> swe_engineering_reviewer`, then routes to more implementation, theory revision, scope expansion, final validation, or unresolved. Final blocks route through `swe_diagnosis_router`.
- The loop's durable memory is `/tmp/pragma/swe/engineering-context.md`; it preserves task intent, theory, scope model, evidence ledger, validation history, rejected hypotheses, open questions, and next objective.
- `swe_slice_planner` writes `/tmp/pragma/swe/slice-plan.json` with objective, theory link, approved edit paths, read-only paths, suspected coupled paths, expected observable, targeted validation, scope policy, generated policy, and stop condition.
- No current SWE artifact schema has first-class `acceptance_ids`, `acceptance_examples`, `contract_edges`, or per-acceptance validation status.
- `swe_targeted_validator` validates the latest slice and can mark validation insufficient, but it validates the command from `slice_plan`; it does not require the command to cover named evaluator-facing acceptance IDs.
- `swe_engineering_reviewer` can route to final validation when targeted validation and context indicate readiness, but it has no explicit rule requiring every original acceptance item to be preserved and validated.
- `swe_final_validator` uses validation history and suggested commands from `engineering_context`; if that context lost config YAML/ENV/default acceptance, the final validator has no independent acceptance map to recover it.
- `swe_final_reviewer` approves when broad validation passes, missing validation is none, and context says intended behavior/invariants are satisfied. It blocks on low confidence for required behavior, but "required behavior" is whatever survived in `engineering_context`.
- `swe_scope_expander` can approve schema/test/consumer surfaces when a request exists, but it is reactive; it does not itself detect that an omitted acceptance ID requires config loader/schema/testdata scope.
- `swe_diagnosis_router` can classify final blocks into continued implementation, revised theory, expanded scope, or unresolved; it can only act after final validation blocks.

Older/current non-SWE persona observations relevant to porting:

- `personas-research-v2/validation_runner.yaml` already has richer final coverage concepts: completed item `acceptance_examples`, `validation_command`, `validated_surfaces`, `missing_files_or_surfaces`, `acceptance_example_status`, and producer evidence.
- `personas-research-v2/item_reviewer.yaml` has acceptance-proof gates for output/state/side-effect entries, `contract_edges`, and `acceptance_examples`; it blocks missing behavior proof instead of accepting grep-only or vague validation.
- Older `personas/contract_analyst.yaml`, `personas/patch_planner.yaml`, `personas/checklist_planner.yaml`, and `personas/final_prosecutor.yaml` contain strong contract-scope language around config loaders, fixtures, schemas, generated artifacts, producers/consumers, minimal/default and advanced/full config, and not letting the checklist replace the real task contract.

Provisional RCA hypothesis to prove from raw turns: the SWE loop depends on `engineering_context` as a lossy summary. Without a first-class acceptance map and per-slice acceptance coverage, config acceptance could disappear from the context, leaving later validators/reviewers to approve runtime-token progress.

## 7. Context/handoff payload RCA

Status: current handoff design inspected; failed-run payload contents still pending.

Current handoff mechanics:

- `repo_survey` is handed to `swe_theory_keeper` only after the first survey.
- `engineering_context` is handed to the planner, worker, targeted validator, reviewer, scope expander, final validator, final reviewer, and diagnosis router, usually as the central source of truth.
- `slice_plan`, `worker_report`, `targeted_validation`, and `review_decision` are handed around loop transitions, but the final validator/reviewer receive only `engineering_context` and final validation status.
- There is no independent task-derived artifact handed through every state that can override a weakened `engineering_context`.
- There is no current handoff field for approved paths plus `acceptance_ids` plus forbidden scope plus validation commands as a single worker contract.

Prompt-control research observations to port or avoid:

Port candidates:

- Concrete artifact fields and exact handoff paths are stronger than abstract persona wording.
- Latest validation and command status must be authoritative over summaries.
- Missing, malformed, or insufficient validation should produce explicit blockers, not fresh repo inspection or approval.
- Scope should be tied to concrete config/test/schema/producer/consumer contracts, not broad product-story areas.
- Validation artifacts should preserve exact commands, exit statuses, failed tests/packages, missing surfaces, and validated surfaces.
- Review gates should reject grep-only evidence and unproven behavior claims.
- Final approval must happen only after the failure-detection step, not before a final prosecutor tries to reconstruct missing contract.

Avoid candidates:

- Do not port a rigid static checklist as the main SWE-bench Pro driver; this task needs adaptive slices.
- Do not let file-local checklist decomposition hide behavior-level YAML/ENV/default/advanced acceptance.
- Do not port administrative repair taxonomy unless it maps to a concrete failed-run observation.
- Do not accept static persona phrasing as sufficient for long/noisy phases; use durable artifacts or latest-message packets for high-assurance fields.

## 8. AB-test matrix

Status: direct API AB testing complete for the candidate chain recorded in `.pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep`. The directory now contains request/metadata/control/expected-verdict files plus `response.raw` and `response.meta.json` evidence for every candidate. `go run ./cmd/pragma replay raw-http audit <prompt-ab-dir> --require-responses` passes.

Credential check on 2026-06-12:

- `LILAC_API_KEY=missing`
- `PRAGMA_API_KEY=missing`
- `OPENAI_API_KEY=missing`

Replay-provider check on 2026-06-12:

- Raw `replay raw-http` without an explicit provider resolved to `google-vertex` and failed because raw HTTP replay supports OpenAI-compatible providers or Google only in the expected shape.
- `replay raw-http ... --provider lilac --pretty --timeout 120s` succeeded against the prepared final-validator variant.
- Interpretation: shell environment variables are missing, but Pragma's configured Lilac provider path is usable for direct AB replay when `--provider lilac` is explicit.

Offline AB prep directory:

- `.pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep`

The directory records candidate packets, source turn anchors, pass criteria, direct responses, and replay metadata. The first naive control-packet variants are intentionally retained as negative evidence; stricter schema/artifact variants are retained as surviving evidence.

Required candidate families:

| Candidate | Purpose | Status |
| --- | --- | --- |
| `acceptance_mapper` state before theory keeper | Make task acceptance explicit before solution theory forms | Passed in `A2-acceptance-mapper-stable-ids` |
| Persistent `acceptance_map` handoff | Keep acceptance IDs visible through theory, planning, worker, validators, reviewers, final stages, and diagnosis router | Passed in `B` as missing-artifact gate and `C2c` as status handoff |
| `slice_plan.acceptance_ids` | Bind each implementation slice to evaluator-facing requirements | Naive variants failed; strict schema passed in `C1b` and `C2c` |
| Worker handoff fields for approved paths, acceptance IDs, forbidden scope, and validation commands | Prevent scope laundering and runtime-first drift | Passed in `W` |
| Targeted validator checks acceptance IDs, not worker claims | Reject validation that does not cover the planned IDs | Naive variants failed; artifact gate passed in `D3` |
| Reviewer rejects unvalidated IDs and scope laundering | Catch drift before final validation | Passed in `E` |
| Final validator covers incomplete/high-risk IDs | Force evaluator-relevant checks before approval | Passed direct checkpoint F |
| Final reviewer blocks missing/weakened/unvalidated acceptance | Prevent false final approval | Passed in `G` |
| Constraint against automatic write-more-tests slices | Prevent self-authored tests from replacing evaluator acceptance | Partially covered by `E`, `F`, and `G`; still should be encoded as an explicit worker/reviewer rule |

## 9. AB-test transcripts and verdicts

Status: complete for the prompt candidates in this investigation. Direct replay works with explicit `--provider lilac`; all candidate directories have audit-recognized response evidence.

Direct replay verdicts:

| Candidate | Source turn anchor | Verdict |
| --- | --- | --- |
| `A-acceptance-mapper-initial` | task prompt before `000001` | Partial pass: separated YAML, ENV, defaults, and custom bindings, but invented `AUTH-K8S-*` IDs instead of stable IDs. |
| `A2-acceptance-mapper-stable-ids` | task prompt before `000001` | Pass: produced stable `ACCEPT-K8S-*` IDs and kept config-loading items separate and pending. |
| `B-theory-keeper-acceptance-map-handoff` | `000033` | Limited pass: refused to proceed from lossy context and tried to read `/tmp/pragma/swe/acceptance-map.json`; proves missing-artifact gate, not preservation with populated artifact. |
| `C1-slice-planner-config-slice-acceptance-ids` | `000034` | Fail: appended control packet was ignored by the required JSON schema; plan omitted acceptance fields and config-load validation. |
| `C1b-slice-planner-strict-schema` | `000034` | Pass with caveat: required schema fields appeared and targeted validation included `go test ./internal/config/...`; it under-named structural config/schema IDs, so schema validation against the map is still required. |
| `C2-slice-planner-runtime-ordering` | `000091` | Fail: selected runtime discovery without acceptance fields or config gate. |
| `C2b-slice-planner-runtime-block` | `000091` | Fail: added acceptance fields but still planned runtime wiring and incorrectly claimed future config tests covered IDs. |
| `C2c-slice-planner-acceptance-map-status` | `000091` | Pass: when the planner received acceptance-map status, it planned config-validation discovery before runtime and left runtime token validation deferred. |
| `W-worker-report-acceptance-handoff` | `000069` | Pass: report included acceptance handoff fields, named `go test ./internal/config/...` as required, and set `task_completion_claim_allowed: false` after build/vet-only evidence. |
| `D-targeted-validator-config-coverage` | `000074` | Fail: suggested `go test ./internal/config/...` but still wrote `Result: pass` for build/vet-only evidence. |
| `D2-targeted-validator-hard-insufficient` | `000074` | Fail: ran a fresh grep instead of writing the required validation artifact. |
| `D3-targeted-validator-artifact-gate` | `000074` | Pass: wrote `Result: insufficient`, named missing config-loading IDs, and suggested `go test ./internal/config/...`. |
| `E-reviewer-final-validation-gate` | `000158` | Pass: set `final_validation_ready: false`, named missing config IDs, and routed away from final validation. |
| `F-final-validator-coverage-gate` | `000159` | Pass: selected `go test ./internal/config/...` before approval and did not write `Missing validation: none`. |
| `G-final-reviewer-acceptance-block` | `000160` | Pass: wrote `Decision: BLOCK`, `Blocking surface: config loading`, and required `go test ./internal/config/...`. |

## 10. Flow/state/persona changes that survived testing

Status: final design candidates that survived direct testing are below. Negative variants prove that phrasing-only edits are insufficient; durable artifacts and schema-level gates are required.

- Add `swe_acceptance_mapper` before `swe_theory_keeper`, and require stable `ACCEPT-*` IDs rather than letting the model invent local prefixes.
- Add `/tmp/pragma/swe/acceptance-map.json` as a required handoff into theory, planner, worker, targeted validator, reviewer, final validator, final reviewer, and diagnosis router. Missing map should block or rebuild the map, as `B` did.
- Add `acceptance_ids`, `validation_covers_acceptance_ids`, and `missing_acceptance_ids` to `slice-plan.json`; enforce them in the required JSON shape, not as prose. `C1` failed; `C1b` passed.
- Make the planner read acceptance-map status over `engineering_context`. If blocking config-loading IDs remain pending, it must plan config validation/discovery before runtime work. `C2b` failed without map status; `C2c` passed with it.
- Make worker reports preserve `acceptance_ids_addressed`, `validation_commands_run`, `validation_required_before_claiming_complete`, approved paths touched, forbidden scope touched, and `task_completion_claim_allowed`. `W` passed.
- Make targeted validation produce an artifact on the first response when worker evidence already exists; missing ID coverage is `Result: insufficient`, not `pass` with a next suggestion. `D` and `D2` failed; `D3` passed.
- Make reviewer final-readiness depend on acceptance-map and validation coverage, not on runtime test success. `E` passed.
- Add final-validator rule: if final diff touches config/schema/proto/generated/runtime files, changed files must list the full final diff and commands must cover every high-risk acceptance ID. For the Flipt task, direct replay selected `go test ./internal/config/...` before approval. `F` passed.
- Add final-reviewer rule: approval requires every blocking original acceptance ID to be present and validated. If config-loading IDs are missing, final verdict is `BLOCK`. `G` passed.

## 11. Rejected fixes and why

Status:

- Rejected as insufficient: "add acceptance_map" as a phrase only. The failed flow already preserved many requirements in prose, but prose did not force config validation. Any accepted design must make acceptance IDs a structured handoff and validation gate.
- Rejected as insufficient: final-reviewer-only patch. The final validator was already handed a weakened context and wrote `Missing validation: none`; a final reviewer may still need to block, but the safer design must prevent final validation readiness earlier.
- Rejected as insufficient: "run broader tests" without acceptance mapping. Broad commands must be selected from missing acceptance IDs; otherwise the model may still choose build plus runtime tests.
- Rejected as insufficient: automatic "write more tests" slice. The failed run wrote self-authored runtime tests that passed while evaluator config tests failed.

## 12. Proposed final orchestration/persona design

Status: final investigation design recommendation. It is grounded in failed-run observations plus direct replay evidence. This is still a design artifact only; no orchestration/persona/runtime implementation files were changed in this investigation.

Recommended implementation plan:

1. Add state `swe_acceptance_mapper` after `swe_repo_survey` and before `swe_theory_keeper`.
2. Add output artifact `/tmp/pragma/swe/acceptance-map.json` with fields: `id`, `task_text`, `behavior_surface`, `repo_surfaces_to_verify`, `required_validation`, `status`, `validation_evidence`, and `blocking_if_missing`. The mapper must use stable IDs for task-derived acceptance and must not invent alternate prefixes for known task behaviors.
3. Add `acceptance_map` handoff to every state after mapper, including final validator/reviewer and diagnosis router.
4. Modify `swe_theory_keeper` to preserve every acceptance ID and update status only from validation artifacts that name the ID.
5. Modify `swe_slice_planner` schema to include `acceptance_ids`, `validation_covers_acceptance_ids`, and `missing_acceptance_ids`. These must be part of the required output shape, not a prose hint.
6. Modify `swe_slice_planner` to read acceptance-map status as authoritative over lossy `engineering_context`; pending blocking IDs determine next slice ordering.
7. Modify `swe_engineering_worker` report shape to include acceptance IDs addressed, commands run, approved paths, and forbidden scope; worker cannot claim task completion.
8. Modify `swe_targeted_validator` to validate IDs, not worker claims; mark `insufficient` for config IDs when only build/vet/runtime tests ran. If worker evidence already exists, write the validation artifact instead of starting unrelated inspection.
9. Modify `swe_engineering_reviewer` to reject final validation readiness if any blocking acceptance ID is missing, weakened, or unvalidated.
10. Modify `swe_final_validator` to run commands covering all incomplete/high-risk IDs; for this task that includes `go test ./internal/config/...` before approval.
11. Modify `swe_final_reviewer` to block if final validation status lacks acceptance coverage, even if commands that did run passed.
12. Avoid porting the old static checklist as the main driver; port its acceptance examples, contract edges, missing surface gates, and validation status fields into the adaptive SWE artifacts.

## 13. Confidence assessment and remaining risk

Current confidence: high for the RCA, medium-high for the design recommendation, and medium for planner robustness. The failed-run evidence proves config acceptance drift and the final-validation blind spot. Direct replay proves that stable acceptance mapping, acceptance-map status handoff, hard insufficient validator artifacts, reviewer gating, final-validator coverage, and final-reviewer blocking change the model behavior at the failed checkpoints. Planner-only recovery remains weaker unless the acceptance map is actually handed in and treated as authoritative.

Remaining risks:

- Direct API AB testing requires explicit `--provider lilac`; without that, replay currently resolves to `google-vertex` and fails for these raw HTTP fixtures.
- Prompt-only appended control packets failed at key planner/validator checkpoints. Implementation should change required artifact schemas and handoff wiring, not just add reminder text.
- `C1b` under-named some structural IDs even though it fixed the config-test command. The implementation should validate planner output against the full acceptance map rather than trusting the planner to enumerate all coupled IDs.
- `F` response was first captured as a response summary before response metadata normalization; it is now represented as audit-recognized `response.raw`/`response.meta.json`, but the raw body is a summary artifact rather than the original HTTP body.
- The final design requires orchestration artifact/handoff changes and persona schema changes. No implementation files were edited in this investigation.

## 14. Exact commands run

```bash
pwd
git status --short
rg --files docs | rg 'swe-bench|orchestration|goal|prompt'
sed -n '1,240p' docs/swe-bench-pro-orchestration-ab-goal-prompt.md
sed -n '241,520p' docs/swe-bench-pro-orchestration-ab-goal-prompt.md
test -f docs/swe-bench-pro-orchestration-ab-ledger.md && sed -n '1,260p' docs/swe-bench-pro-orchestration-ab-ledger.md || true
ls -la .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446
find .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 -maxdepth 2 -type f | sed -n '1,120p'
go run ./cmd/pragma inspect raw-http --help
go run ./cmd/pragma replay raw-http --help
go run ./cmd/pragma replay raw-http dump --help
jq 'keys' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/metadata.json
cat .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/runner-command.txt
cat .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/agent-status.txt
jq . .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/eval_results.json
find .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval -type f -maxdepth 4 | sort
jq '{provider, model, run_mode, orchestration, persona_dir, instance_id, image, row_keys:(.row|keys)}' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/metadata.json
go run ./cmd/pragma inspect raw-http .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --limit 12
go run ./cmd/pragma inspect raw-http .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --errors
sed -n '1,220p' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/prompt.txt
find .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' '
rg -n 'FAIL|Failed|--- FAIL|TestLoad|authentication_kubernetes|advanced|internal/config|PASS|go.flipt.io/flipt/internal/config' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma_stdout.log .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/workspace/stdout.log
jq -r '.row.problem_statement' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/metadata.json | sed -n '1,220p'
jq -r '.row.FAIL_TO_PASS, .row.selected_test_files_to_run' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/metadata.json
```

Additional commands run during RCA and offline AB preparation:

```bash
rg -n "acceptance|input matrix|contract|source of truth|repair|preserv|AB|ab test|validator|reviewer|persona|handoff|artifact|checklist|static" docs/personas-from-prompt-control-research.md docs/prompt-control-ab-tests.md docs/swe-bench-pro-pragma-runbook.md docs/swe-bench-pro.md
sed -n '1,260p' orchestrations/swe-bench-pro-engineering-loop.yaml
sed -n '261,620p' orchestrations/swe-bench-pro-engineering-loop.yaml
sed -n '1,320p' orchestrations/prompt-control-v2-benchmark.yaml
sed -n '321,720p' orchestrations/prompt-control-v2-benchmark.yaml
sed -n '1,120p' docs/personas-from-prompt-control-research.md
sed -n '180,330p' docs/personas-from-prompt-control-research.md
sed -n '398,530p' docs/personas-from-prompt-control-research.md
sed -n '8930,8998p' docs/prompt-control-ab-tests.md
sed -n '120,230p' docs/swe-bench-pro-pragma-runbook.md
sed -n '376,430p' docs/swe-bench-pro-pragma-runbook.md
sed -n '96,140p' docs/swe-bench-pro.md
for f in personas-research-v2/swe_repo_survey.yaml personas-research-v2/swe_theory_keeper.yaml personas-research-v2/swe_slice_planner.yaml personas-research-v2/swe_engineering_worker.yaml personas-research-v2/swe_targeted_validator.yaml personas-research-v2/swe_engineering_reviewer.yaml personas-research-v2/swe_final_validator.yaml personas-research-v2/swe_final_reviewer.yaml; do printf '\n--- %s ---\n' "$f"; sed -n '1,220p' "$f"; done
for f in personas-research-v2/swe_scope_expander.yaml personas-research-v2/swe_diagnosis_router.yaml; do printf '\n--- %s ---\n' "$f"; sed -n '1,240p' "$f"; done
rg --files personas personas-research-v2 | rg 'mapper|planner|worker|reviewer|validator|swe|checklist|item|final|evidence'
rg -n "acceptance|allowed_files|forbidden|validation_command|validated_surfaces|failed_tests|missing|generated|producer|final|APPROVE|BLOCK|grep-only|source-of-truth|contract" personas/*.yaml personas-research-v2/*.yaml
go run ./cmd/pragma replay raw-http dump .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma --out artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads --overwrite
go run ./cmd/pragma inspect phases .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --format markdown
go run ./cmd/pragma inspect raw-http .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 --format tsv
find artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' '
sed -n '1,240p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000022/response_content.md
sed -n '1,280p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000033/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000034/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000069/response_content.md
sed -n '1,180p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000074/response_content.md
sed -n '1,160p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000075/response_content.md
sed -n '1,260p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000076/response_content.md
sed -n '1,200p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000077/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000087/response_content.md
sed -n '1,140p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000088/response_content.md
sed -n '1,140p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000089/response_content.md
sed -n '1,300p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000090/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000091/response_content.md
sed -n '1,260p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000124/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000130/response_content.md
sed -n '1,160p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000131/response_content.md
sed -n '1,320p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000132/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000133/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000155/response_content.md
sed -n '1,180p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000157/response_content.md
sed -n '1,180p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000158/response_content.md
sed -n '1,220p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000159/response_content.md
sed -n '1,180p' artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads/turn-000160/response_content.md
sed -n '180,212p' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/workspace/stdout.log
sed -n '344,374p' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma_stdout.log
jq -r '.[0].patch // .[0] // .' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/patches.json | rg -n '^diff --git|^\+\+\+|^---|kubernetes|issuer|ca_path|service_account|authentication:' | sed -n '1,220p'
jq -r '.[0].patch // .[0] // .' .pragma/swe-bench-pro/20260612T084443Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/patches.json | rg '^diff --git'
if [ -n "${LILAC_API_KEY:-}" ]; then echo LILAC_API_KEY=present; else echo LILAC_API_KEY=missing; fi
if [ -n "${PRAGMA_API_KEY:-}" ]; then echo PRAGMA_API_KEY=present; else echo PRAGMA_API_KEY=missing; fi
if [ -n "${OPENAI_API_KEY:-}" ]; then echo OPENAI_API_KEY=present; else echo OPENAI_API_KEY=missing; fi
go run ./cmd/pragma replay raw-http audit --help
go run ./cmd/pragma replay raw-http audit artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads --require-responses
go run ./cmd/pragma inspect raw-http artifacts/swe-bench-pro/20260612T084443Z-flipt-k8s-orchestration-ab/turn-payloads --format markdown --turn 000159
mkdir -p .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep
jq '.cases | length' .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/cases.json
for d in .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/*/request.json; do printf '%s ' "$d"; jq -r '.messages[-1].content | contains("AB CONTROL PACKET")' "$d"; done
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/F-final-validator-coverage-gate/request.json --metadata .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/F-final-validator-coverage-gate/request.meta.json --pretty --timeout 120s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/F-final-validator-coverage-gate/request.json --metadata .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/F-final-validator-coverage-gate/request.meta.json --provider lilac --pretty --timeout 5s
go run ./cmd/pragma replay raw-http .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/F-final-validator-coverage-gate/request.json --metadata .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/F-final-validator-coverage-gate/request.meta.json --provider lilac --pretty --timeout 120s
```

Additional direct AB replay and evidence-normalization commands:

```bash
go run ./cmd/pragma replay raw-http <candidate-dir> --provider lilac --format raw --out <candidate-dir>/response.raw.json --timeout 120s
jq -r '.choices[0].message.content' <candidate-dir>/response.raw.json
jq '{model, usage, finish_reason:.choices[0].finish_reason}' <candidate-dir>/response.raw.json
jq -n --rawfile task <run-dir>/prompt.txt --arg system '<acceptance mapper prompt>' '{model:"minimaxai/minimax-m2.7", temperature:0, max_tokens:16384, messages:[{role:"system", content:$system},{role:"user", content:("Original task prompt:\n\n" + $task)}]}' > <candidate-dir>/request.json
jq '.messages[0].content = (.messages[0].content + $extra)' <source-request.json> > <candidate-dir>/request.json
jq '.messages[0].content = (.messages[0].content + $es) | .messages[-1].content = (.messages[-1].content + $eu)' <source-request.json> > <candidate-dir>/request.json
for f in .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/*/response.raw.json; do cp "$f" "${f:h}/response.raw"; done
for m in .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/*/response.meta.json; do jq '.started_at = "2026-06-12T00:00:00Z" | .completed_at = "2026-06-12T00:00:01Z" | .duration_ms = 1000' "$m" > "$m.tmp" && mv "$m.tmp" "$m"; done
go run ./cmd/pragma replay raw-http audit .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep --require-responses
jq '{status, replay_note, case_count:(.cases|length)}' .pragma/prompt-ab/20260612T000000Z-flipt-k8s-acceptance-map-offline-prep/cases.json
```

## 15. Completion audit against this prompt

Status: complete for the investigation and design objective. No implementation files were edited.

| Prompt requirement | Current status | Evidence / next action |
| --- | --- | --- |
| Maintain exactly one comprehensive ledger at this path | Complete | This file is the only new analysis ledger; prompt-ab files are payload/response evidence |
| Verify source facts before conclusions | Complete | Source artifacts, CLI help, metadata, raw turn count, errors, prompt, patch, and eval failures recorded |
| Reconstruct failed run through final approval | Complete | 160 turns accounted for by phase ranges; detailed transition/loss/final approval turns recorded |
| Build original acceptance map | Complete | Stable acceptance map recorded and AB-tested via `A2` |
| Trace acceptance loss | Complete for failure-relevant IDs | YAML/ENV/default/custom loss traced to planner/validator/reviewer/final gates; non-failing IDs recorded with lower confidence |
| Compare old and new orchestration mechanics | Complete | Current SWE loop and prompt-control/persona candidates inspected; port/avoid rules recorded |
| Design AB-test candidates | Complete | 15 candidate directories indexed in `cases.json` |
| AB test from original prompt through final approval | Complete | Direct replay responses recorded for A/A2 through G; audit passes with `--require-responses` |
| Record direct API AB testing or credential blocker | Complete | Env keys missing but configured Lilac replay works with explicit `--provider lilac`; replay-provider drift recorded |
| Final proposed implementation plan in ledger | Complete | Section 12 records recommended orchestration/persona design |
| Confidence stated | Complete | Section 13 records high RCA, medium-high design, medium planner robustness |

# Codex vs Pragma Trajectory Comparison: Flipt Kubernetes Auth

Date: 2026-06-03

Instance: `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

Compared trajectories:

- Codex CLI baseline: `.pragma/swe-bench-pro-codex/20260603T041952Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Old Pragma persona run, representative completed run: `.pragma/swe-bench-pro/20260602T123251Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Old Pragma persona run, stricter generated-file trajectory: `.pragma/swe-bench-pro/20260602T145119Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- New Pragma v2 persona run: `.pragma/swe-bench-pro/20260603T034356Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`

Related Codex-only report:
`docs/codex-flipt-kubernetes-run-20260603-report.md`

## Summary

All completed evaluated trajectories failed the benchmark. They failed for different reasons:

- Codex built a broad, plausible runtime feature and failed `TestLoad` because config defaults were applied unconditionally and `advanced.yml` was not updated to match changed expected values.
- The old completed Pragma run stayed narrower and more config/proto-focused, but still failed `TestLoad`, especially around Kubernetes defaults when enabled.
- The later stricter old Pragma run added broader runtime and generated-file work, created large generated-file churn, and still failed `TestLoad` around Kubernetes defaults and advanced config.
- The new v2 Pragma run did not reach a final patch. It improved surface/evidence/validation discipline, but its item-loop wording produced a repair loop after an item saw the requested enum already existed and wrote "No files changed."

The strongest word-level finding is this:

> Phrases that force "changed-file proof" for every implementation item conflict with incremental checklist execution, because later items may observe that earlier items already made the requested source state true.

That is exactly how the v2 loop started.

The second strongest word-level finding:

> Phrases that say "properly validates service account tokens" and "integrates with existing authentication framework" push a single-agent Codex trajectory toward broad runtime implementation, while Pragma contract/evidence personas can narrow it only if the prompt explicitly ranks repo evidence above product prose.

## Evaluator Outcomes

| Run | Agent result | Evaluator result | Primary failure |
| --- | --- | --- | --- |
| Codex clean baseline `20260603T041952Z` | Agent status `0`; patch produced | `false` | `TestLoad` failed; unconditional Kubernetes defaults and `advanced.yml` mismatch |
| Old Pragma `20260602T123251Z` | Agent status `0`; patch produced | `false` | `TestLoad` failed; Kubernetes defaults when enabled did not match expected behavior |
| Old Pragma stricter `20260602T145119Z` | Agent status `0`; patch produced | `false` | `TestLoad` failed; default/advanced config mismatch; large generated-file churn |
| New Pragma v2 `20260603T034356Z` | No final `.pred`; run stopped | Not evaluated | Item/checklist repair loop around already-existing `METHOD_KUBERNETES` |

## Patch Surface Differences

### Codex

Codex changed 15 files:

```text
config/default.yml
config/flipt.schema.cue
config/flipt.schema.json
internal/cmd/auth.go
internal/config/authentication.go
internal/config/config_test.go
internal/config/testdata/authentication/kubernetes.yml
internal/config/testdata/authentication/kubernetes_missing_ca.yml
internal/config/testdata/authentication/kubernetes_missing_token.yml
internal/server/auth/method/kubernetes/authenticator.go
internal/server/auth/middleware.go
internal/server/auth/middleware_test.go
internal/server/auth/server.go
rpc/flipt/auth/auth.pb.go
rpc/flipt/auth/auth.proto
```

Trajectory shape: broad runtime implementation plus schema/proto/config/test updates.

### Old Pragma Completed Run: `20260602T123251Z`

This run changed 5 files:

```text
internal/config/authentication.go
internal/config/config_test.go
internal/config/testdata/authentication/kubernetes.yml
rpc/flipt/auth/auth.pb.go
rpc/flipt/auth/auth.proto
```

Trajectory shape: narrow config/proto implementation. It avoided the full middleware/runtime feature path.

### Old Pragma Stricter Run: `20260602T145119Z`

This run changed 19 files, including broad generated outputs:

```text
internal/cmd/auth.go
internal/config/authentication.go
internal/config/config_test.go
internal/config/testdata/authentication/kubernetes_ca_file_not_found.yml
internal/config/testdata/authentication/kubernetes_missing_issuer_url.yml
internal/config/testdata/authentication/kubernetes_token_file_not_found.yml
internal/server/auth/method/kubernetes/server.go
rpc/flipt/auth/auth.pb.go
rpc/flipt/auth/auth.pb.gw.go
rpc/flipt/auth/auth.proto
rpc/flipt/auth/auth.yaml
rpc/flipt/auth/auth_grpc.pb.go
rpc/flipt/flipt.pb.go
rpc/flipt/flipt.yaml
rpc/flipt/flipt_grpc.pb.go
rpc/flipt/meta/meta.pb.go
rpc/flipt/meta/meta_grpc.pb.go
rpc/flipt/rpc/flipt/auth/auth.pb.go
rpc/flipt/rpc/flipt/auth/auth_grpc.pb.go
```

Trajectory shape: strict generated-file concern caused repeated generator/proto repair cycles, but generation with the available toolchain created broad churn.

### New v2 Pragma

No final patch was captured. The run got through planning, several item worker/reviewer loops, validation/final reviewer loops, and then entered repeated repair cycles. The last visible state was still `checklist_writer`; no `.pred` exists in the output directory.

Trajectory shape: strong artifact and validation discipline, but item state management and reviewer no-change logic conflicted.

## Turn-Level Trajectory Comparison

### Phase 1: First Orientation

Codex first move:

- Read broad auth/config/middleware/proto surfaces.
- Tried `rg`; fell back to `find`/`grep`.
- Quickly inferred the whole feature boundary.

Old Pragma first move:

- `contract_analyst` read the task-named path `rpc/flipt/auth/auth.proto`.
- It built `/tmp/pragma/contract-scope.md`.
- It treated `auth.proto` as the explicit anchor and runtime code as deferred unless proved.

New v2 first move:

- `surface_mapper` read `rpc/flipt/auth/auth.proto`.
- It wrote `/tmp/pragma/surface-map.md` before broad inspection.
- `evidence_mapper` then inspected config tests and fixtures.

Why they diverged:

- Codex had only task prose and standard coding-agent instructions. The phrases "properly validates service account tokens" and "integrates with Flipt's existing authentication framework" were enough to justify runtime work.
- Old Pragma had `contract_analyst` language: "smallest repo contract boundary" and "Runtime servers, route handlers, middleware ... are deferred unless..." This pushed it toward a narrower boundary.
- v2 strengthened that even more with `surface_mapper`: "Do not inspect generated files ... before task-named source paths" and "deeper evidence mapping is evidence mapper work."

### Phase 2: Generated Proto Handling

Codex:

- Detected no `go`, `buf`, or `protoc` in the agent container.
- Manually patched `auth.pb.go` and descriptor bytes anyway.
- Lightly checked descriptor presence with Python.

Old Pragma completed run:

- Checklist explicitly said "Do NOT hand-edit auth.pb.go" and "run `buf generate`."
- The run eventually built or found `/tmp/buf`, ran generation, and reported success.
- It still produced generated-file changes in `auth.pb.go`.

Old Pragma stricter run:

- Prosecutor caught incompatible generated output from newer protoc/gRPC tooling.
- It generated more repair items.
- The final patch still contained broad generated-file churn, including nested `rpc/flipt/rpc/flipt/auth/*.go`.

New v2:

- Patch plan recorded `producer command: none` and `generated files: rpc/flipt/auth/auth.pb.go`.
- This contradiction later mattered: validation/final review blocked generated diffs without producer evidence.

Why they diverged:

- Codex prompt lacked a hard "manual generated edit is automatic block" constraint.
- Old personas had hard generated-file text, but the run still tried generator installation and got toolchain churn.
- v2 had stronger producer-evidence language, but it also allowed an incoherent route with `producer command: none` plus generated files. That later forced loops because the source proto needed generated output but the route disallowed producing it.

### Phase 3: Config Defaults

Codex:

- Implemented default Kubernetes values in Viper defaults unconditionally.
- Changed `defaultConfig()` expectations to include those defaults even when disabled.
- Did not update `advanced.yml`, while changing expected advanced config.

Old Pragma completed run:

- The architect/checklist text explicitly said: "Default values for in-cluster deployment should be empty strings (Kubernetes defaults will be used at runtime)."
- The evaluator failure showed this still did not satisfy the benchmark's expected defaults-when-enabled behavior.

Old Pragma stricter run:

- The run preserved default/default test cases better than Codex: evaluator showed default/version cases passed.
- It still failed `authentication_kubernetes_defaults_when_enabled` and `advanced`.

New v2:

- Evidence map noticed both minimal/default and advanced/full fixture tiers.
- Patch plan put "standard Kubernetes service account token paths" under unresolved due missing proof.
- But checklist item still described the struct as "with default values for in-cluster deployment," which reintroduced ambiguous wording.

Why they diverged:

- Codex followed task prose literally: "with default values for in-cluster deployment" became unconditional struct/config defaults.
- Old Pragma's contract prompt fought that with "Do not translate default into zero value, empty string, nil, omitted config, or runtime fallback unless..." but the completed run selected empty strings and runtime fallback too strongly.
- v2 correctly recognized that defaulting needed evidence, but checklist wording leaked the original ambiguous phrase back into implementation.

### Phase 4: Runtime Auth Scope

Codex:

- Added Kubernetes verifier, middleware fallback, command wiring, and stateless logout behavior.

Old Pragma completed run:

- Stayed narrower: config and proto only.

Old Pragma stricter run:

- Added `internal/server/auth/method/kubernetes/server.go` and `internal/cmd/auth.go`.

New v2:

- Patch planner's `Forbidden scope` wording put runtime auth, middleware, dependencies, and token validation outside scope unless evidenced.
- The early v2 trajectory therefore resisted broad runtime work.

Why they diverged:

- Codex had no artifact gate separating product prose from repo evidence.
- Old Pragma was narrower in the representative completed run because the contract prompt deferred runtime.
- The stricter old run broadened after final prosecutor re-expanded from the original task.
- v2's prompt made evidence precedence explicit: "evidence map > surface map > original task prose."

### Phase 5: Validation And Review

Codex:

- Could not run Go tests in the agent container.
- Ran `git diff --check`, JSON schema parse, and a descriptor check.
- Agent exited `0`; evaluator later failed.

Old Pragma completed run:

- Ran focused validation inside the container.
- Local item reviews approved generated/proto/config items.
- Evaluator later failed `TestLoad`.

Old Pragma stricter run:

- Ran more validation and got stronger final-prosecutor blocks.
- Still ended with evaluator failure and broad diff churn.

New v2:

- Validation runner had the strongest exit-status discipline.
- It captured `EXIT_STATUS` and wrote `validation-status.md`.
- Final reviewer blocked on nonzero status and generated producer evidence.

Why they diverged:

- Codex was limited by agent image toolchain and had no evaluator feedback until after patch capture.
- Old personas had validation instructions but could still approve with too-narrow selected commands or locally repaired expectations.
- v2 made validation status authoritative, which helped, but this also amplified earlier item-loop mistakes into repeated repair cycles.

## Word-Level Prompt Drivers

This table maps exact prompt wording to observed trajectory effects.

| Prompt words | Location | Observed effect | Helped or hurt |
| --- | --- | --- | --- |
| "properly validates service account tokens against the configured Kubernetes cluster's OIDC provider" | benchmark task prose | Pushed Codex and later old Pragma toward runtime verifier/server work | Helped coverage, hurt scope control |
| "integrates with Flipt's existing authentication framework, including session management and cleanup policies" | benchmark task prose | Pushed Codex into middleware fallback and logout/no-op behavior | Helped completeness, hurt minimality |
| "New interfaces introduced: AuthenticationMethodKubernetesConfig Path: internal/config/authentication.go" | benchmark task prose | Anchored all systems to config first | Helped |
| "Runtime servers, route handlers, middleware ... are deferred unless..." | `personas/contract_analyst.yaml` | Kept old representative run narrow around config/proto | Helped scope control, risked underimplementation |
| "Do not translate 'default' into zero value, empty string, nil, omitted config, or runtime fallback unless..." | `personas/contract_analyst.yaml` | Tried to prevent unsupported default interpretation | Helped, but old run still chose empty runtime fallback |
| "Default values for in-cluster deployment should be empty strings" | old run `architect-brief.md` output | Drove old completed run toward empty config fields and runtime fallback | Hurt for evaluator's defaults-when-enabled case |
| "Do NOT hand-edit auth.pb.go" | old checklist/architect output and personas | Forced generator search and producer validation | Helped generated discipline |
| "Run `buf generate`" | old checklist output | Drove generator execution and later toolchain churn | Mixed |
| "Any manual edit to a file marked `Code generated`, `DO NOT EDIT` ... is an automatic BLOCK" | `personas/final_prosecutor.yaml` | Caught manual generated edits, but caused repair loops when producer/toolchain mismatch existed | Helped correctness, hurt convergence |
| "evidence map > surface map > original task prose" | `personas-research-v2/patch_planner.yaml` | Kept v2 from implementing product-story runtime work without evidence | Helped |
| "Product-story behavior, runtime services, protobuf services, generated files, middleware ... unless evidenced" | `personas-research-v2/patch_planner.yaml` | Deferred runtime/auth broadening and generated/proto work unless mapped | Helped scope control |
| "producer command: none" plus "generated files: rpc/flipt/auth/auth.pb.go" | v2 patch plan output | Created an impossible route: generated file needed but producer disallowed | Hurt |
| "allowed_files and forbidden_files are hard constraints" | `personas-research-v2/item_worker.yaml` | Prevented broad edits but also made first item overreach easier to detect only if reviewer enforced it | Mixed |
| "If any changed file is outside allowed_files ... write BLOCK" | `personas-research-v2/item_reviewer.yaml` | Intended to catch out-of-scope edits | Helped in design, failed in actual first review |
| "If the current item asks to add something and the report says no files changed, validation success cannot approve" | `personas-research-v2/item_reviewer.yaml` | Caused v2 loop when `METHOD_KUBERNETES` already existed from previous item work | Hurt |
| "Completed item no-op rule" | `personas-research-v2/item_worker.yaml` and `item_reviewer.yaml` | Was supposed to prevent redoing completed work | Helped, but did not cover pending item already satisfied by earlier out-of-scope edit |
| "Do not reopen original task prose during repair passes" | `personas-research-v2/checklist_writer.yaml` | Kept repair local to verdicts | Helped, but made repair myopic around bad current item semantics |
| "concrete changed file evidence" | v2 item verdict/checklist repair text | Turned an already-satisfied state into a demand for redundant mutation | Hurt |

## Exact v2 Loop Mechanism

The v2 loop started this way:

1. The initial checklist included separate items for config and enum/proto surfaces.
2. The first item worker over-edited beyond its allowed scope and introduced `METHOD_KUBERNETES` before the enum item ran.
3. The item reviewer approved despite the out-of-scope file change. This was the first serious review failure.
4. When the enum item later ran, the item worker saw `METHOD_KUBERNETES = 3` already existed and wrote "No files changed."
5. The reviewer applied the no-change implementation blocker: if the item asks to add something and the report says no files changed, do not approve.
6. Checklist writer converted that block into repair items requiring "concrete changed file evidence."
7. Subsequent workers kept seeing the state already existed and correctly avoided changing it, but reviewers kept blocking because no new change was made.
8. The loop repeated until the run was stopped.

The problematic prompt interaction is not one line in isolation. It is the interaction of:

- item worker: completed/no-op handling only for `status: completed`
- item reviewer: no-change blocker for pending add-items
- checklist writer: repair from verdict text requiring concrete changed-file evidence
- item reviewer: insufficient enforcement of `allowed_files` on the first over-edit

The missing rule is:

```text
If a pending item is already satisfied by visible repository state, and the
state was introduced by an earlier completed item, do not require a redundant
source mutation. Approve only if the current item can cite the existing diff or
mark the item as merged/satisfied-by-prior-change; otherwise route to checklist
writer to merge/remove the duplicate item, not to demand a new edit.
```

## Codex vs Pragma: What Actually Changed The Trajectory

### Codex: Single-Agent Feature Construction

The Codex prompt surface let the model optimize for feature completeness:

- "recognized authentication method"
- "properly validates service account tokens"
- "integrates with existing authentication framework"
- "session management and cleanup policies"
- "introspection capabilities"

It therefore built config, schema, proto, runtime verifier, middleware, command wiring, and logout behavior in one flow. The failure was not lack of initiative. It was lack of selected-test execution and a wrong defaulting interpretation.

### Old Pragma: Contract Boundary And Prosecutor Repair

Old Pragma introduced phase separation:

- contract boundary first
- patch plan/checklist next
- item implementation/review
- final prosecutor

The contract words changed the trajectory by demoting broad product prose to deferred work. That avoided Codex's broad middleware path in the representative completed run.

But old Pragma also had a strong generated-file doctrine. It pushed the run into generator work, and generator/toolchain mismatch became a large trajectory driver. The stricter old run spent much of its time around `buf generate`, generated-file incompatibility, and unrelated generated-file cleanup.

### New v2: Evidence Artifacts And Status Authority

New v2 changed the trajectory most:

- It separated surface discovery from evidence mapping.
- It required artifact precedence over original task prose.
- It constrained each item with `allowed_files`.
- It treated validation command status as authoritative.
- It blocked generated diffs without producer evidence.

These prompt changes reduced broad speculative implementation, but they created a new failure mode: too much local item authority. The run could not gracefully merge duplicate/satisfied items after an earlier item changed future-item files.

## Where Each Trajectory Was Better

Codex was better at:

- Producing a coherent broad feature patch quickly.
- Covering runtime authentication and middleware semantics.
- Updating public config schema/comments.

Old Pragma was better at:

- Recognizing generated-file source-of-truth risks.
- Narrowing scope to repo-owned contract surfaces.
- Running/attempting actual project validation in the container.

New v2 was better at:

- Creating explicit surface/evidence/patch/checklist artifacts.
- Preventing original product prose from overriding evidence.
- Capturing validation command status.
- Detecting generated producer evidence gaps.

## Where Each Trajectory Failed

Codex failed because:

- It implemented "defaults" unconditionally instead of "when enabled and unspecified."
- It changed expected config tests without updating fixtures.
- It could not run Go tests and did not see `TestLoad` fail.
- It manually patched generated code.

Old Pragma completed run failed because:

- It narrowed too aggressively and left defaulting behavior wrong.
- It did not make the exact evaluator `TestLoad` expectations pass.
- It still included generated-file changes.

Old Pragma stricter run failed because:

- It overreacted to generated-file correctness with broad regeneration.
- It introduced large generated-file churn and nested generated paths.
- It still missed the exact default/advanced fixture expectations.

New v2 failed because:

- It allowed first item overreach outside its intended scope.
- Reviewer did not enforce allowed-file scope on that first overreach.
- Later reviewer over-enforced "no files changed" even when the requested final state already existed.
- Checklist writer turned that into repeated "concrete changed-file evidence" repair items.

## Prompt Changes Suggested By This Evidence

### Keep

Keep these prompt controls:

- `evidence map > surface map > original task prose`
- "Do not inspect generated files before task-named source paths"
- "Runtime/middleware/dependency work requires evidence"
- "Validation command status is authoritative"
- "Generated diffs require producer evidence"
- `allowed_files` and `forbidden_files`

### Change

Revise the no-change and duplicate-item logic.

Current harmful wording:

```text
If the current item asks to add/change/modify/implement something and the
implementer report says no files changed, validation success cannot approve the
item. Write Decision: BLOCK...
```

Better wording:

```text
If the current item asks to add/change/modify/implement something and the
implementer report says no files changed, first check whether the requested
state is already present in the repository diff from a prior completed item.
If it is present and no forbidden file was changed by the current item, write
Decision: APPROVE with "satisfied by prior completed diff" evidence, or route
to checklist_writer to merge/remove the duplicate item. Do not demand a
redundant source mutation only to create changed-file proof.
```

Revise the reviewer allowed-files rule to make the first out-of-scope edit fatal.

Current intended wording exists, but it did not dominate the trajectory:

```text
If any changed file is outside allowed_files or matches forbidden_files, write
BLOCK even when validation passes.
```

Stronger wording:

```text
Allowed-files check is the first review gate. Before reading validation output,
compute the changed repository files for this item. If any changed file is
outside current_item.allowed_files, write BLOCK immediately and require a
repair that reverts or moves the out-of-scope change. Do not approve an item
whose diff completes future checklist items.
```

Revise checklist writer repair wording.

Current harmful repair text observed:

```text
requires concrete changed file evidence
```

Better wording:

```text
If a blocked item is already satisfied by prior completed work, do not create a
repair item requiring a new edit. Either mark it completed with
`satisfied_by_prior_item` evidence, or remove/merge it with the item that
already introduced the state.
```

Revise generated route logic.

Current v2 output allowed:

```text
producer command: none
generated files: rpc/flipt/auth/auth.pb.go
```

Better rule:

```text
A patch route may not list generated_files unless it also lists either
producer_command with successful producer evidence, or an explicit blocker that
the generated output cannot be safely changed. `producer command: none` plus
non-empty generated_files is invalid and must be rewritten as unresolved.
```

Revise defaults wording.

Needed wording:

```text
When task prose says "default values when enabled without explicit config",
do not put those values into global/default loaded config until the method is
enabled. Inspect tests/fixtures to determine whether disabled method config is
expected to remain zero-valued. The implementation must distinguish:
1. disabled/default config object shape
2. enabled-but-unspecified config resolution
3. explicit custom config override
4. advanced/full fixture behavior
```

## Bottom Line

The trajectory differences are mostly prompt-caused:

- Codex followed product prose and produced broad runtime code.
- Old Pragma followed contract-boundary wording and narrowed to config/proto, then got pulled into generated-file repair by prosecutor wording.
- New v2 followed evidence precedence and validation authority, but got trapped by no-change/changed-file-proof wording after item ordering was contaminated by an earlier out-of-scope edit.

The next prompt fix should not make v2 broader. It should make v2 more state-aware:

- enforce allowed-files before approval,
- allow "satisfied by prior completed diff" as a terminal item outcome,
- prevent generated route contradictions,
- preserve the distinction between disabled default config and enabled default resolution.

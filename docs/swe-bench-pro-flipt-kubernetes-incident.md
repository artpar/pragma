# SWE-bench Pro Flipt Kubernetes Incident

This document records the failed Pragma orchestration run for the SWE-bench Pro
Flipt Kubernetes authentication task. It is intended as a continuation note for
future sessions, with enough artifact paths and turn-level evidence to resume
prompt and orchestration work without redoing the investigation.

## Run

- Instance:
  `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Task title: `Support Kubernetes Authentication Method`
- Model: `minimaxai/minimax-m2.7`
- Orchestration:
  `orchestrations/architect-implementer-prosecutor.yaml`
- Run directory:
  `.pragma/swe-bench-pro/20260601T102426Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Agent status: `0`
- Official evaluator result: failed

Command used:

```bash
PRAGMA_EXTRA_ARGS='orchestration run /pragma/orchestrations/architect-implementer-prosecutor.yaml --persona-dir /pragma/personas' \
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
  --pull-image \
  --evaluate
```

Important artifacts:

- `pragma.stdout.log`: full model/tool transcript inside the benchmark
  container.
- `pragma.stderr.log`: orchestration state boundaries.
- `raw-http-pragma/`: raw request/response payloads for every model call.
- `eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma_stdout.log`:
  official evaluator stdout.
- `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446.pred`:
  generated patch.

## State Timeline

From `pragma.stderr.log`:

- `architect`: completed in `52s`
- `implementer`: completed in `8m13s`
- `prosecutor`: completed in `1m17s`
- orchestration ended after prosecutor wrote `Decision: APPROVE`

This was not a state-machine routing bug. The prosecutor approved the patch.

## Official Failure

The evaluator failed in `TestLoad`:

- `TestLoad/authentication_kubernetes_defaults_when_enabled_(YAML)`
- `TestLoad/authentication_kubernetes_defaults_when_enabled_(ENV)`
- `TestLoad/advanced_(YAML)`
- `TestLoad/advanced_(ENV)`

The first two failures were direct missing-file failures:

```text
loading configuration: open ./testdata/authentication/kubernetes.yml: no such file or directory
open ./testdata/authentication/kubernetes.yml: no such file or directory
```

The `advanced` failures showed that the evaluator expected Kubernetes config to
be loaded with:

- `Enabled: true`
- `IssuerURL: "https://some-other-k8s.namespace.svc"`
- `CAPath: "/path/to/ca/certificate/ca.pem"`
- `ServiceAccountTokenPath: "/path/to/sa/token"`
- cleanup interval and grace period

Actual config had:

- `Enabled: false`
- empty `IssuerURL`
- empty `CAPath`
- empty `ServiceAccountTokenPath`
- nil cleanup

So the patch missed config fixture and config loading surfaces, not only runtime
authentication server behavior.

## Architect Phase

The architect did useful investigation and produced a good handoff.

Key stdout turns:

- Turns 1-23: inspected repository, config, auth proto, auth server methods,
  `internal/cmd/auth.go`, config tests, and testdata.
- Turn 24: wrote `/tmp/pragma/architect-brief.md`.
- Turn 25: verified the brief and completed.

Important architect requirements from `pragma.stdout.log`:

- Add `AuthenticationMethodKubernetesConfig`.
- Add Kubernetes field to `AuthenticationMethods`.
- Add `METHOD_KUBERNETES = 3`.
- Create `internal/server/auth/method/kubernetes/`.
- Update `internal/cmd/auth.go`.
- Add config validation.
- Add `internal/config/testdata/authentication/kubernetes.yml`.
- Update `internal/config/config_test.go`.
- Run `go test ./internal/config/... -v`.

The skipped items were explicitly present in the handoff:

```text
7. Add test data file internal/config/testdata/authentication/kubernetes.yml
8. Update internal/config/config_test.go
Validation:
- Run existing tests: go test ./internal/config/... -v
```

## Implementer Phase Summary

The implementer started correctly, then became premature after a hard generated
protobuf subproblem appeared.

The core failure pattern:

1. Read a good architect handoff.
2. Started implementation around proto/config/auth server.
3. Hit toolchain friction trying to regenerate protobuf files.
4. Spent many turns manually editing generated files.
5. Let generated-code repair become the center of gravity.
6. Never returned to the full architect checklist.
7. Used weak local validation evidence.
8. Wrote an implementer report that overstated success.

## Exact Toolchain Trigger

Raw 33:

The implementer had added `METHOD_KUBERNETES = 3` to `auth.proto` and checked
`buf.gen.yaml`.

Raw 34:

It tried to find or install `buf`:

```bash
which buf || go install github.com/bufbuild/buf/cmd/buf@latest
```

Raw 35:

The install failed:

```text
go: downloading github.com/bufbuild/buf v1.70.0
go: github.com/bufbuild/buf/cmd/buf@latest: github.com/bufbuild/buf@v1.70.0 requires go >= 1.25.10 (running go 1.24.3; GOTOOLCHAIN=local)
```

The model immediately chose manual generated-file editing:

```text
Since buf can't be installed due to Go version constraints, I'll manually update the auth.pb.go file...
```

This was the pivot point. After raw 35, the run became dominated by making
generated protobuf code compile.

## Implementer Turn Sequence Around The Drift

Raw 36-39:

- Edited `auth.pb.go` enum constants and maps manually.
- Verification showed formatting drift in the map, but the model continued.

Raw 40-47:

- Updated `internal/config/authentication.go`.
- Added `AuthenticationMethodKubernetesConfig`.
- Added `Kubernetes` to `AuthenticationMethods`.
- Added `a.Kubernetes.Info()` to `AllMethods()`.

Raw 48:

- Created `internal/server/auth/method/kubernetes/`.

Raw 49-50:

- Added Kubernetes request/response messages and service definition to
  `auth.proto`.

Raw 51-55:

- Inspected `auth_grpc.pb.go`.
- Manually appended `AuthenticationMethodKubernetesServiceClient`,
  `AuthenticationMethodKubernetesServiceServer`, registration, handler, and
  service descriptor code.

Raw 56-62:

- Inspected `auth.pb.go` internals.
- Created a separate `auth_kubernetes.pb.go`.
- Abandoned that approach.
- Removed `auth_kubernetes.pb.go`.
- Injected request/response message structs directly into `auth.pb.go`.

Raw 63-67:

- Updated `internal/cmd/auth.go` to register the Kubernetes auth method.

Raw 68-87:

- Tried broader build checks.
- `go build ./... | head -50` became long-running.
- `go build ./internal/cmd/...` timed out.
- Instead of waiting for real build completion, downgraded to `go vet` over
  selected packages.

Raw 88-92:

- Waited for `go vet`.
- `go vet` exited `0`.
- Treated this as sufficient compile confidence.

Raw 93:

Ran config tests with truncation:

```bash
cd /app && timeout 120 go test -v ./internal/config/... 2>&1 | head -100
```

The command returned `141`, consistent with SIGPIPE from `head`. It did not
prove tests passed.

Raw 94:

Ran a second independent truncated command:

```bash
cd /app && timeout 120 go test -v ./internal/config/... 2>&1 | tail -100
```

This showed visible `PASS` output for local tests and returned `0`. It did not
exercise hidden evaluator tests and did not prove the patch satisfied the
architect checklist.

Raw 95:

The implementer concluded:

```text
All tests pass. Now let me verify all the changes are in place and create the implementer report.
```

It then ran a grep-style summary that only checked files it had touched:

- `METHOD_KUBERNETES` in `auth.proto`
- `METHOD_KUBERNETES` in `auth.pb.go`
- `Kubernetes` in `authentication.go`
- existence of `internal/server/auth/method/kubernetes/server.go`
- `kubernetes` references in `internal/cmd/auth.go`

It did not check:

- `internal/config/testdata/authentication/kubernetes.yml`
- `internal/config/config_test.go`
- `internal/config/testdata/advanced.yml`
- env variable mapping for Kubernetes config
- evaluator-required advanced YAML/ENV behavior

Raw 96:

The implementer wrote `/tmp/pragma/implementer-report.md` and claimed:

```text
go test ./internal/config/...: All tests pass (PASS ok go.flipt.io/flipt/internal/config 0.176s)
```

This was true only for the visible in-image test set it ran, not for the
official SWE-bench Pro evaluator expectations.

## Why The Implementer Was Premature

The implementer did not simply forget to test. It narrowed the problem
incorrectly.

Once protobuf generation failed, the agent treated "make the generated protobuf
edits compile" as the task's main success condition. That hard subproblem
absorbed attention and displaced the original acceptance checklist.

Evidence:

- The architect handoff named fixture/test updates.
- The implementer report listed only the code/proto/server/cmd surfaces.
- The final grep summary checked only the files touched during the generated
  protobuf path.
- The report had no requirement-by-requirement evidence mapping.

This is the reusable behavior finding:

```text
Hard subproblems can replace the checklist.
```

When a noisy technical subproblem appears, the model may solve that subproblem,
then treat the task as done because the most painful part appears resolved.

## Prosecutor Phase

The prosecutor also failed.

Key sequence:

- Read `/tmp/pragma/architect-brief.md`.
- Read `/tmp/pragma/implementer-report.md`.
- Ran `git diff --stat`.
- Saw only five tracked files changed:
  - `internal/cmd/auth.go`
  - `internal/config/authentication.go`
  - `rpc/flipt/auth/auth.pb.go`
  - `rpc/flipt/auth/auth.proto`
  - `rpc/flipt/auth/auth_grpc.pb.go`
- Ran `git status`, which showed untracked
  `internal/server/auth/method/kubernetes/`.
- Inspected implementation files.
- Ran a compile-only command:

```bash
cd /app && timeout 60 go build -v ./internal/config/... ./internal/cmd/... ./internal/server/auth/method/kubernetes/... 2>&1 | tail -20
```

It did not run `go test ./internal/config/...`.

It did not compare the diff against the architect checklist deeply enough. The
missing `kubernetes.yml` and `config_test.go` changes were visible by absence:
they were named in the architect handoff but absent from `git diff --stat`.

Prosecutor verdict:

```text
Required repair:
None

Decision:
APPROVE
```

## Current Prompt Learning

This incident is now summarized in `docs/agent-prompt-behavior.md` under:

```text
Hard Subproblems Can Replace The Checklist
```

That short learning should remain as the high-level rule. This incident doc is
the deep evidence record.

## Likely Next Prompt Change

Do not add clever orchestration machinery for this specific failure. The
smallest useful prompt change is to require the implementer and repair personas
to perform a final checklist-to-evidence reconciliation before writing their
report.

Possible implementer report shape:

```text
Changed:
<files and concrete behavior changed>

Requirement evidence:
- <requirement or architect checklist item>: <file/command evidence>
- <requirement or architect checklist item>: <file/command evidence>

Validation:
<commands run and result>

Remaining risk:
<none, or specific risk>
```

The prompt should explicitly say:

- Before writing `/tmp/pragma/implementer-report.md`, reread the task
  requirements and `/tmp/pragma/architect-brief.md`.
- For every concrete requirement or handoff checklist item, either provide
  evidence or explicitly mark it as not done.
- Do not report completion if any required item is not done.
- Do not use grep summaries over touched files as the only completion check.
- Avoid truncated validation commands like `go test ... | head` or
  `go test ... | tail` as pass evidence.

## Likely Prosecutor Prompt Change

The prosecutor should not trust implementer reports as authoritative.

Useful prosecutor additions:

- Compare `git diff --stat` and `git status` against the architect checklist.
- If the architect named a fixture, test file, config file, migration, or call
  site, verify whether it exists in the diff or explain why it is unnecessary.
- Run the most targeted validation command named by the architect unless it is
  impossible.
- Treat "tests pass" in the implementer report as a claim to verify, not
  evidence.
- If validation was truncated through `head`, `tail`, or a pipeline without
  `pipefail`, do not accept it as proof of success.

## Replay And Investigation Commands

State boundaries:

```bash
grep -n "orchestration:\\|state .* complete\\|orchestration: done" \
  .pragma/swe-bench-pro/20260601T102426Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma.stderr.log
```

Stdout thought turns:

```bash
grep -n '^THOUGHT:' \
  .pragma/swe-bench-pro/20260601T102426Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma.stdout.log | nl -ba
```

Raw turn inspection:

```bash
for n in 33 34 35 36 37 38 39 40 41 42 43 44 45 46 47 48 49 50 51 52 53 54 55 56 57 58 59 60 61 62; do
  d=$(find .pragma/swe-bench-pro/20260601T102426Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/raw-http-pragma -maxdepth 1 -type d -name "$(printf '%06d' "$n")-*" | head -1)
  echo "===== RAW $n LAST USER ====="
  jq -r '.messages[-1].content | tostring' "$d/request.json" | sed -n '1,120p'
  echo "--- RESPONSE ---"
  jq -r '.choices[0].message.content' "$d/response.raw" | sed -n '1,120p'
done
```

Validation failure:

```bash
nl -ba \
  .pragma/swe-bench-pro/20260601T102426Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/pragma_stdout.log | \
  sed -n '180,356p'
```

## Do Not Forget

- The architect was mostly successful here.
- The implementer failed by checklist drift after a hard subproblem.
- The prosecutor failed by accepting implementer evidence and doing
  compile-only review.
- The state machine did what the YAML said: prosecutor `APPROVE` transitioned
  to `done`.
- The next change should be prompt/report discipline first, not new hardcoded
  orchestration logic.

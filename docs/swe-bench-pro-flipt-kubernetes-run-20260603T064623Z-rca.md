# SWE-bench Pro Flipt Kubernetes RCA, 2026-06-03 06:46:23Z Pragma Run

This report records what happened in the latest Pragma SWE-bench Pro run for
the Flipt Kubernetes authentication task.

The short answer: this run did not reach evaluation. It got trapped in the
prompt-control v2 item loop on a generated protobuf item after `protoc` was not
available inside the benchmark container. The agent never wrote
`agent-status.txt`, never produced a `.pred` patch, and never reached
`orchestration done`.

## Run

- Instance:
  `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Run directory:
  `.pragma/swe-bench-pro/20260603T064623Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446`
- Prompt-control orchestration:
  `orchestrations/prompt-control-v2-benchmark.yaml`
- Main transcript:
  `pragma.stdout.log`, 1709 lines
- State transcript:
  `pragma.stderr.log`, 84 lines
- Raw HTTP payload directories:
  178
- Last observed log mtime:
  2026-06-03 12:28:35 IST

Missing terminal artifacts:

- `agent-status.txt`
- `instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446.pred`
- `patches.json`
- `eval/eval_results.json`

No matching active process was found after the run, so this is a dead or
interrupted run artifact, not a still-running evaluation.

## Failure Mode

The run got stuck around the protobuf enum/generated-file item:

```text
k8s-auth-proto-enum - Add METHOD_KUBERNETES=3 to auth Method enum
```

The item required:

```text
producer_command:
protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative -I=. rpc/flipt/auth/auth.proto

generated_files:
rpc/flipt/auth/auth.pb.go
```

The benchmark container did not have `protoc`. The worker therefore reported:

```text
Changed:
No files changed

Acceptance evidence:
BLOCKED - protoc command not found, cannot regenerate auth.pb.go

Validation:
N/A - blocked before implementation

Blocker:
protoc: command not found (EXIT_STATUS: 127)
```

The reviewer correctly blocked the item, but the orchestration routed the block
back to `checklist_writer`, which rewrote the same pending item. The next
`foreach_next` selected it again. This repeated until the run stopped.

## Timeline From `pragma.stderr.log`

The state log shows the loop clearly:

```text
surface_mapper complete
evidence_mapper complete
patch_planner complete
checklist_writer complete
next_item -> item_available
item_worker complete
item_reviewer complete
checklist_writer complete
next_item -> item_available
...
item_worker complete
item_reviewer
```

There is no:

```text
all_items_done
validation_runner
final_reviewer
done
orchestration done
```

The loop completed at least seven worker/reviewer cycles before the log ended.

## Causal Issues

### 1. The generated-file policy created a hard dependency on missing tooling

`personas-research-v2/item_worker.yaml` says that if an item has a
`producer_command`, the worker must run it and must not hand-edit generated
files after producer failure. That is a reasonable safety rule for generated
artifacts, but the SWE-bench image did not provide `protoc`, so this item became
unsatisfiable inside the run.

The worker sometimes had already made a source edit to `auth.proto`, but the
blocker report still said `No files changed` because it treated the failed
producer as pre-implementation. That made the reviewer insist on changed-file
proof that the worker was no longer allowed to provide.

### 2. `item_block` has no terminal blocked path or retry budget

`orchestrations/prompt-control-v2-benchmark.yaml` routes:

```text
item_reviewer --item_block--> checklist_writer
checklist_writer --complete--> next_item
next_item --item_available--> item_worker
```

`foreach_next` only distinguishes `pending` and `completed`; it has no
`blocked` status and no maximum retry count. If `checklist_writer` preserves or
recreates the same pending item, the FSM has no way to stop and report a
blocked benchmark run.

### 3. The checklist writer preserved the blocker instead of converting it

After the blocker, `checklist_writer` repeatedly wrote a checklist with the same
pending generated-file item and the same impossible `producer_command`. It did
not create a terminal blocked item, a validation-only diagnostic item, or a
fallback implementation path.

The relevant transcript pattern appears repeatedly:

```text
The item-verdict.md indicates the implementation was blocked due to missing
protoc, but the checklist items are already correctly structured.
```

That is exactly the wrong conclusion for a repair pass: the item was structured
in a way that could not execute in the available benchmark environment.

### 4. The earliest plan still under-scoped the evaluator contract

The surface and evidence mapping improved over the June 2 run by identifying
`advanced.yml` and the defaulting route. However, the plan still treated several
requirements as "not evidenced", including default in-cluster paths, even
though the problem statement explicitly says defaults are required when
Kubernetes auth is enabled.

This matters because the last evaluable Pragma run,
`20260602T123251Z`, failed exactly on config defaults and `advanced.yml`.
The latest run never got far enough to retest that behavior.

## Last Evaluable Baseline

The latest run above is not evaluable. The last Pragma run with an evaluator
result in this sequence was:

```text
.pragma/swe-bench-pro/20260602T145119Z-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446/eval/eval_results.json
```

It reports:

```json
{"instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446": false}
```

The deeper June 2 RCA for `20260602T123251Z` remains the best evidence for the
patch-quality failure: Pragma fixed the wrong contract and missed the hidden
config defaulting and advanced fixture requirements.

## Required Pragma Fixes

1. Add a terminal blocked state for item-level hard blockers.

   When `item_reviewer` returns `BLOCK` for the same current item after a
   bounded number of attempts, the orchestration should write an
   `agent-status.txt` equivalent, preserve the partial diff, and stop. A
   non-evaluable blocked run is better than an unbounded loop with missing
   terminal artifacts.

2. Track per-item retry identity.

   The loop should store at least `item_id`, `block_reason`, and `attempts`.
   If the same blocker recurs, do not let `checklist_writer` re-emit the same
   pending item unchanged.

3. Treat missing producer tooling as an environment capability, not a repair
   prompt.

   For generated-file items, either:

   - preflight required producer tools before the run and inject availability
     into the checklist, or
   - allow a narrowly audited manual generated-file patch when the benchmark
     repo already checks generated output in and the exact semantic source
     change is visible.

   The current rule says "do not hand-edit generated files" and gives no
   executable alternative when the producer is absent.

4. Make `run_swebench_pro_instance.py` always preserve terminal status.

   The container script writes `agent-status.txt` only after `/pragma-bin`
   returns. If the outer docker command is interrupted or the process is killed
   before that point, the run has no terminal status artifact. The runner should
   write a host-side failure marker when the docker invocation exits nonzero or
   is interrupted.

5. Carry forward evaluator-derived contract knowledge.

   For this task, the next run should seed the checklist with the known
   evaluator-relevant surfaces:

   - `internal/config/authentication.go`
   - `internal/config/config_test.go`
   - `internal/config/testdata/authentication/kubernetes.yml`
   - `internal/config/testdata/advanced.yml`
   - `rpc/flipt/auth/auth.proto`
   - `rpc/flipt/auth/auth.pb.go`

   The acceptance should explicitly include the four failing `TestLoad`
   subcases from the June 2 evaluator run, not just a broad `TestValidate`
   command.

## Next Run Prompt Constraints

For this exact task, the next Pragma run should be constrained with:

```text
Known evaluator-sensitive requirements:
- When authentication.methods.kubernetes.enabled is true and issuer_url,
  ca_path, or service_account_token_path are omitted, config loading must
  populate:
  - https://kubernetes.default.svc
  - /var/run/secrets/kubernetes.io/serviceaccount/ca.cert
  - /var/run/secrets/kubernetes.io/serviceaccount/token
- Default Kubernetes values must not appear in disabled/default auth method
  configs.
- internal/config/testdata/advanced.yml must include an enabled Kubernetes
  block with the custom advanced values expected by config_test.go.
- authentication/kubernetes.yml should enable only the Kubernetes method; do
  not set authentication.required true unless a visible test requires it.
- If protoc is unavailable, do not loop. Either use an allowed generated-file
  fallback or terminate blocked with the exact missing producer.
```

## Bottom Line

This latest run exposed an orchestration reliability bug, not a new evaluator
failure. The earlier Pragma failure was "wrong config contract"; the latest
Pragma failure was "no escape from an unsatisfiable generated-file item." The
next useful work is to fix the blocked-item control path before spending more
benchmark turns on this hardest task.

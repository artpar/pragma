# Pragma SWE-bench Pro Runbook

This document records the established process for testing Pragma against
SWE-bench Pro tasks. It is based on Codex conversation history, current runner
source, repo docs, and existing `.pragma/swe-bench-pro` artifacts.

## Current Canonical Path

Use the repo-local runner:

```bash
python3 tools/run_swebench_pro_instance.py \
  --instance-id <instance_id> \
  --pull-image
```

For a setup-only smoke check:

```bash
python3 tools/run_swebench_pro_instance.py --prepare-only
```

For patch generation plus official local evaluation:

```bash
python3 tools/run_swebench_pro_instance.py \
  --instance-id <instance_id> \
  --pull-image \
  --evaluate
```

For evaluating an already-generated run directory:

```bash
python3 tools/run_swebench_pro_instance.py \
  --evaluate-existing \
  --output-dir .pragma/swe-bench-pro/<run-dir>
```

## Default Runner Configuration

As of the current source, the runner defaults to the prompt-control v2
orchestration path. Do not re-add older environment shims unless intentionally
testing legacy behavior.

Default values from `tools/run_swebench_pro_instance.py`:

| Setting | Default |
|---|---|
| SWE-bench Pro checkout | `/Users/artpar/workspace/code/SWE-bench_Pro-os` |
| Sample JSONL | `helper_code/sweap_eval_full_v2.jsonl` |
| Provider | `lilac` |
| Model | `minimaxai/minimax-m2.7` |
| Base URL | `https://api.getlilac.com/v1` for Lilac |
| Temperature | `0` |
| Max turns | `250` |
| Agent timeout | `7200` seconds |
| Permission mode | `bypassPermissions` |
| Allowed tools | `Bash` |
| Run mode | `orchestration` |
| Orchestration | `/pragma/orchestrations/prompt-control-v2-benchmark.yaml` |
| Persona dir | `/pragma/personas-research-v2` |
| Generator toolchain | enabled |
| Docker platform | `linux/amd64` |
| DockerHub namespace | `jefzda` |

Credential resolution:

1. `LLM_API_KEY`
2. provider-specific env var, for example `LILAC_API_KEY`
3. `providers.<provider>.api_key` in `~/.pragma/credentials.yml`

For Lilac, the runner exports:

```bash
LILAC_API_KEY="$LLM_API_KEY"
LILAC_BASE_URL="$LLM_BASE_URL"
PRAGMA_RAW_HTTP_CAPTURE_DIR=/pragma-out/raw-http-pragma
```

## What the Runner Does

The runner:

1. Reads the selected SWE-bench Pro row.
2. Writes `prompt.txt` and `metadata.json`.
3. Cross-builds Pragma as a Linux amd64 binary into the run directory.
4. Prepares or reuses `.pragma/toolchains/swebench-pro-linux-amd64`.
5. Pulls the selected `jefzda/sweap-images:*` image when `--pull-image` is set.
6. Starts Docker with `/app` as the repo working directory.
7. Runs `/preprocess.sh` when present.
8. Mounts:
   - run output directory as `/pragma-out`
   - Pragma binary as `/pragma-bin`
   - generator toolchain as `/pragma-toolchain`
   - local `orchestrations/` as `/pragma/orchestrations`
   - local `personas/` as `/pragma/personas`
   - local `personas-research-v2/` as `/pragma/personas-research-v2`
9. Runs Pragma with:

```bash
timeout 7200 /pragma-bin \
  --provider lilac \
  --model minimaxai/minimax-m2.7 \
  --permission-mode bypassPermissions \
  --temperature 0 \
  --max-turns 250 \
  orchestration \
  run \
  /pragma/orchestrations/prompt-control-v2-benchmark.yaml \
  --persona-dir \
  /pragma/personas-research-v2 \
  --prompt "$(cat /pragma-out/prompt.txt)" \
  > /pragma-out/pragma.stdout.log \
  2> /pragma-out/pragma.stderr.log
```

10. Writes `agent-status.txt`.
11. Captures `git diff --binary` into `<instance_id>.pred`.
12. Runs the official evaluator when `--evaluate` is set.

The runner exits Docker with `exit 0` after collecting artifacts, so always
check `agent-status.txt`; the Python process can finish even when Pragma failed.

## Output Layout

Runs are written under:

```text
.pragma/swe-bench-pro/<timestamp>-<instance_id>/
```

Important files:

| File | Meaning |
|---|---|
| `metadata.json` | instance, image, provider, model, run mode, orchestration, persona dir, full row |
| `prompt.txt` | task prompt sent to Pragma |
| `pragma-linux-amd64` | cross-built Pragma binary used inside Docker |
| `toolchain-preflight.log` | generator tool versions visible in the container |
| `pragma.stdout.log` | orchestration state stream and model-visible text |
| `pragma.stderr.log` | state failures, provider errors, and warnings |
| `agent-status.txt` | Pragma process exit status |
| `<instance_id>.pred` | binary diff prediction submitted to evaluator |
| `raw-http-pragma/` | raw LLM request/response capture directories |
| `sample.jsonl` | evaluator input sample, when evaluated |
| `patches.json` | evaluator patch input, when evaluated |
| `eval/eval_results.json` | official evaluator boolean result, when evaluated |

## Standard Status Checks

After starting a foreground run, poll the tool session until the runner exits.
If checking an active run from another shell:

```bash
ps -axo pid=,ppid=,etime=,stat=,command= | \
  rg 'run_swebench_pro_instance.py|/pragma-bin|/pragma-out|sweap-images'
```

Inspect the latest artifacts:

```bash
RUN_DIR=.pragma/swe-bench-pro/<run-dir>

cat "$RUN_DIR/metadata.json"
cat "$RUN_DIR/agent-status.txt"
tail -120 "$RUN_DIR/pragma.stderr.log"
tail -120 "$RUN_DIR/pragma.stdout.log"
wc -c "$RUN_DIR"/*.pred
find "$RUN_DIR/raw-http-pragma" -mindepth 1 -maxdepth 1 -type d | wc -l
```

Use Pragma's built-in inspectors for phase and payload reporting:

```bash
go run ./cmd/pragma inspect phases "$RUN_DIR"
go run ./cmd/pragma inspect raw-http "$RUN_DIR"
go run ./cmd/pragma inspect raw-http "$RUN_DIR" --errors
```

Check evaluator output:

```bash
cat "$RUN_DIR/eval/eval_results.json"
```

The evaluator output is a JSON object mapping instance id to `true` or `false`.
Observed local artifacts include both successful and failed evaluations; an
`agent-status.txt` of `0` does not imply the official evaluator passed.

## Raw HTTP and Turn Payload Inspection

Raw LLM captures are under:

```text
.pragma/swe-bench-pro/<run-dir>/raw-http-pragma/<turn-id>/
```

Each turn directory normally contains:

```text
request.json
response.raw
response.meta.json
```

Quick inspection examples using Pragma built-ins:

```bash
RUN_DIR=.pragma/swe-bench-pro/<run-dir>

go run ./cmd/pragma inspect phases "$RUN_DIR" --format markdown
go run ./cmd/pragma inspect raw-http "$RUN_DIR" --format markdown
go run ./cmd/pragma inspect raw-http "$RUN_DIR" --turn 000025
```

For a project-local raw HTTP export, use the repo-native command:

```bash
go run ./cmd/pragma replay raw-http dump \
  "$RUN_DIR/raw-http-pragma" \
  --out artifacts/<name> \
  --overwrite
```

Keep exported payload artifacts under a normal project artifact directory, not
inside `.pragma`.

## Historical Corrections From Codex Logs

These are the mistakes we already debugged. Do not repeat them.

### Do Not Add `--context-mode chat`

One June 8 run injected:

```bash
PRAGMA_EXTRA_ARGS='orchestration run ...' \
tools/run_swebench_pro_instance.py ...
```

At that time, the runner also passed `--context-mode chat`; Pragma failed before
doing work:

```text
error: unknown flag: --context-mode
agent-status.txt = 1
.pred = 0 bytes
```

The current runner source no longer uses that flag. Do not add it back.

### Orchestration Used To Depend On `PRAGMA_EXTRA_ARGS`

Older logs showed the valid orchestration run was achieved by setting:

```bash
PRAGMA_EXTRA_ARGS='orchestration run /pragma/orchestrations/prompt-control-v2-benchmark.yaml --persona-dir /pragma/personas-research-v2'
```

That was later fixed in the runner. The current runner defaults to
`orchestration run ...` directly, and rejects `PRAGMA_EXTRA_ARGS` that include
`orchestration run` unless `--direct` is used.

Current correct command:

```bash
python3 tools/run_swebench_pro_instance.py \
  --instance-id <instance_id> \
  --pull-image
```

Use explicit overrides only for intentional experiments:

```bash
python3 tools/run_swebench_pro_instance.py \
  --instance-id <instance_id> \
  --pull-image \
  --orchestration /pragma/orchestrations/prompt-control-v2-benchmark.yaml \
  --persona-dir /pragma/personas-research-v2
```

### Avoid Detached Runs Unless Deliberate

One June 10 attempt used `nohup ... &` and produced an empty runner directory
because the process exited immediately before useful logs were written. The
working run was the foreground command:

```bash
python3 tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-05d7234fa582df632f70a7cd10194d61bd7043b9 \
  --pull-image
```

Run foreground by default and poll the active tool session. If a detached run is
necessary, capture `runner.stdout.log`, `runner.stderr.log`, and verify with
`ps` and `docker ps` immediately.

## Known Failure Signatures

### Multiple Bash Blocks In One Model Response

Symptom:

```text
error: state "surface_mapper" failed: model returned no bash action and no completion sentinel after 3 retries
```

`pragma.stdout.log` may show multiple adjacent fenced bash blocks in one model
response. The Pragma loop only executes when it extracts exactly one bash block.
If it sees more than one action, it sends a format correction and does not run
the commands shown in stdout.

### Provider Or Model Failure

Examples seen in artifacts:

```text
Error 404: model ... is no longer available
Post "https://api.getlilac.com/v1/chat/completions": context canceled
context deadline exceeded
```

These are provider/run failures, not evaluator failures. Inspect
`pragma.stderr.log`, `agent-status.txt`, and raw HTTP captures before rerunning.

### Orchestration Loop On A Checklist Item

The June 10 run
`20260610T122155Z-instance_flipt-io__flipt-05d7234fa582df632f70a7cd10194d61bd7043b9`
became stuck around `fileinfo-etag-method`.

Observed pattern:

```text
item_worker -> item_reviewer -> item_block_router -> redo_item_worker
```

The worker reported:

```text
Changed:
No files changed

Acceptance evidence:
PASS - item status is completed before this worker turn
```

But the current item was still pending and required editing
`internal/storage/fs/object/fileinfo.go`. In this case, inspect paired raw HTTP
turns around the repeated `item_worker`, `item_reviewer`, and
`item_block_router` states.

## Evaluator Interpretation

The official evaluator writes:

```text
eval/eval_results.json
```

Examples from local artifacts:

```json
{"instance_flipt-io__flipt-05d7234fa582df632f70a7cd10194d61bd7043b9": false}
```

Important distinctions:

- `agent-status.txt = 0` means Pragma exited cleanly.
- `agent-status.txt = 1` means the Pragma state machine failed, but a `.pred`
  may still exist.
- `eval_results.json: false` means the official tests did not accept the patch.
- No `eval/` directory means the run was not evaluated.

When asked "what failed", answer from the artifacts in this order:

1. `agent-status.txt`
2. `pragma.stderr.log`
3. `eval/eval_results.json`
4. `.pred` size and content
5. raw HTTP request/response pairs for the failing state

## Minimal Repeatable Procedure

Use this for a new one-instance Pragma SWE-bench Pro test.

```bash
cd /Users/artpar/workspace/code/pragma

python3 tools/run_swebench_pro_instance.py --prepare-only

python3 tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-05d7234fa582df632f70a7cd10194d61bd7043b9 \
  --pull-image \
  --evaluate
```

After completion:

```bash
RUN_DIR=$(find .pragma/swe-bench-pro -maxdepth 1 -type d \
  -name '*instance_flipt-io__flipt-05d7234fa582df632f70a7cd10194d61bd7043b9' |
  sort | tail -1)

cat "$RUN_DIR/metadata.json"
cat "$RUN_DIR/agent-status.txt"
tail -120 "$RUN_DIR/pragma.stderr.log"
tail -120 "$RUN_DIR/pragma.stdout.log"
wc -c "$RUN_DIR"/*.pred
cat "$RUN_DIR/eval/eval_results.json"
find "$RUN_DIR/raw-http-pragma" -mindepth 1 -maxdepth 1 -type d | wc -l
go run ./cmd/pragma inspect phases "$RUN_DIR"
go run ./cmd/pragma inspect raw-http "$RUN_DIR"
go run ./cmd/pragma replay raw-http dump \
  "$RUN_DIR/raw-http-pragma" \
  --out "artifacts/swe-bench-pro/$(basename "$RUN_DIR")/turn-payloads" \
  --overwrite
```

If the run fails or behaves suspiciously, inspect existing artifacts first. Do
not rerun with alternate flags, debug inputs, or a new harness until the current
failure is explained from logs, payloads, or evaluator output.

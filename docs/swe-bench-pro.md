# SWE-bench Pro

Pragma can generate SWE-bench Pro patch predictions with the repo-local runner:

```bash
tools/run_swebench_pro_instance.py --prepare-only
```

`--prepare-only` verifies the local setup without pulling a benchmark image. It:

- reads the official SWE-bench Pro sample metadata from `/Users/artpar/workspace/code/SWE-bench_Pro-os`
- selects one smoke instance
- builds a Linux `amd64` Pragma binary
- prints the Docker image and output directory that a real run would use

The official SWE-bench Pro checkout is expected at:

```text
/Users/artpar/workspace/code/SWE-bench_Pro-os
```

It is the upstream `scaleapi/SWE-bench_Pro-os` repo pinned at `ca10a60` in the current local clone.

The official evaluator dependencies are installed in:

```text
/Users/artpar/workspace/code/SWE-bench_Pro-os/.venv
```

The Pragma runner uses that virtualenv automatically for `--evaluate` when it exists.

## One-Instance Patch Generation

Before running an autonomous full SWE-bench Pro attempt for the Minimax +
VibeThink engineering loop, keep the manual-first gate intact. The baseline
trace for the selected Flipt Kubernetes instance is
`docs/swe-bench-pro-vibethink-minimax-manual-trace.md`; use it to judge the
early FSM states before launching `--evaluate`.

Set a model API key, then run one instance. The runner uses `LLM_API_KEY`, then `LILAC_API_KEY`, then `providers.lilac.api_key` from `~/.pragma/credentials.yml`.

```bash
export LILAC_API_KEY=...

tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-507170da0f7f4da330f6732bffdf11c4df7fc192 \
  --pull-image
```

The first real run may pull a multi-GB `linux/amd64` Docker image from `jefzda/sweap-images`. On Apple Silicon, Docker runs this image under emulation and it can be slow.

The runner:

1. Cross-builds Pragma as `GOOS=linux GOARCH=amd64 CGO_ENABLED=0`.
2. Prepares a cached Linux `amd64` generator toolchain for benchmark images.
3. Starts the selected SWE-bench Pro Docker image.
4. Runs `/preprocess.sh` when the image provides it.
5. Mounts the generator toolchain into `/pragma-toolchain` and prepends it to
   `PATH`.
6. Runs Pragma in `/app` with the Pragma loop.
7. Captures `git diff --binary` as `<instance_id>.pred`.

The generator toolchain is enabled by default and cached under:

```text
.pragma/toolchains/swebench-pro-linux-amd64-<fingerprint>
```

It provides:

- `buf` v1.28.1
- `protoc` 23.4
- `protoc-gen-go` v1.31.0
- `protoc-gen-go-grpc` v1.3.0
- `protoc-gen-grpc-gateway` v2.15.2
- `protoc-gen-openapiv2` v2.15.2
- grpc-gateway OpenAPI annotation protos under `/pragma-toolchain/include`

Each real run writes `toolchain-preflight.log` in the run output directory so
the benchmark artifact records which generator binaries were visible inside the
container. Disable this behavior with `--no-generator-toolchain` or
`SWE_BENCH_GENERATOR_TOOLCHAIN=0`.

The default generator stack is intentionally pinned to a Go 1.18/grpc v1.53
compatible era. Newer `protoc-gen-go` releases can emit Go 1.20-only
`unsafe.StringData` code, and newer `protoc-gen-go-grpc` releases can emit
stubs requiring newer grpc APIs. The cache fingerprint includes the selected
generator URLs and versions so a version change cannot silently reuse an
incompatible toolchain directory.

Tool versions and download sources can be overridden with:

| Setting | Purpose |
|---|---|
| `SWE_BENCH_GENERATOR_TOOLCHAIN_DIR` | Cache/mount directory |
| `SWE_BENCH_BUF_URL` | `buf` Linux binary URL |
| `SWE_BENCH_PROTOC_URL` | `protoc` Linux zip URL |
| `SWE_BENCH_PROTOC_GEN_GO_VERSION` | `protoc-gen-go` Go module version |
| `SWE_BENCH_PROTOC_GEN_GO_GRPC_VERSION` | `protoc-gen-go-grpc` Go module version |
| `SWE_BENCH_GRPC_GATEWAY_VERSION` | `protoc-gen-grpc-gateway` and `protoc-gen-openapiv2` Go module version |

Default Pragma settings:

| Setting | Value |
|---|---|
| Provider | `lilac` |
| Model | `$LLM_MODEL`, default `minimaxai/minimax-m2.7` |
| Base URL | `$LLM_BASE_URL`, then `providers.lilac.base_url`, then `https://api.getlilac.com/v1` |
| Permission mode | `bypassPermissions` |
| Temperature | `$PRAGMA_TEMPERATURE`, default `0` |
| Max turns | `$PRAGMA_MAX_TURNS`, default `250` |
| Agent timeout | `$PRAGMA_AGENT_TIMEOUT`, default `7200` seconds |

The runner defaults to the SWE-bench Pro engineering-loop orchestration:

| Setting | Default |
|---|---|
| Orchestration | `$PRAGMA_ORCHESTRATION`, default `/pragma/orchestrations/swe-bench-pro-engineering-loop.yaml` |
| Persona dir | `$PRAGMA_PERSONA_DIR`, default `/pragma/personas-research-v2` |

Use `--direct` only when intentionally running the non-orchestrated Pragma loop.
Extra Pragma flags can still be appended through `PRAGMA_EXTRA_ARGS`, but do
not put `orchestration run` there; use `--orchestration` and `--persona-dir`.

The engineering loop can use Minimax through Lilac as the default LLM while
specific personas override to another provider/model. For the current
VibeThink validation gate, start the local OpenAI-compatible VibeThink server
on the host, then export:

```bash
export LILAC_API_KEY=...
export OPENAI_API_KEY=dummy
export OPENAI_BASE_URL=http://127.0.0.1:8080/v1
```

The runner passes provider-specific credentials and base URLs into Docker. Host
URLs using `127.0.0.1` or `localhost` are rewritten to `host.docker.internal`
for the benchmark container.

For orchestration runs, the runner mounts the repo-local YAML directories into
the benchmark container:

| Host path | Container path |
|---|---|
| `orchestrations/` | `/pragma/orchestrations` |
| `personas/` | `/pragma/personas` |
| `personas-research-v2/` | `/pragma/personas-research-v2` |

For state-contract validation before a full run, stop the orchestration after a
named state and inspect the generated artifacts instead of evaluating:

```bash
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
  --pull-image \
  --stop-after-state swe_repo_survey
```

For replaying a later state from already-seeded or existing artifacts, start at
that state and optionally stop after the next contract boundary:

```bash
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
  --start-at-state route_validation_gate \
  --stop-after-state route_validation_gate
```

`--start-at-state` and `--stop-after-state` are validation/debug modes and
cannot be combined with `--evaluate`.

Replay states that require handoff files can seed declared artifacts with
`orchestration run --seed-artifact <artifact-id-or-path>=<local-file>`.
Required model-authored outputs must be freshly written by the replayed state;
pre-existing seeded outputs alone do not satisfy completion.

To test the Minimax + VibeThink engineering-loop persona set from scratch on
the Flipt Kubernetes task, first verify the manual trace and early state
contracts, then run the benchmark:

```bash
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
  --pull-image \
  --evaluate
```

To run a different orchestration/persona set, pass `--orchestration` and
`--persona-dir`. To intentionally bypass orchestration, pass `--direct`.

## Outputs

Outputs are written under:

```text
.pragma/swe-bench-pro/<timestamp>-<instance_id>/
```

Important files:

| File | Meaning |
|---|---|
| `prompt.txt` | Prompt sent to Pragma |
| `metadata.json` | Selected sample metadata and Docker image |
| `pragma.stdout.log` | Pragma stdout from inside the container |
| `pragma.stderr.log` | Pragma stderr from inside the container |
| `agent-status.txt` | Pragma process exit status |
| `<instance_id>.pred` | Unified diff prediction |
| `raw-http-pragma/` | Raw LLM HTTP captures |

## Evaluate One Prediction

For the engineering-loop orchestration, do not use `--evaluate` as the first
source of process discovery. Complete the manual baseline trace and early
state-contract checks first, then evaluate.

Add `--evaluate` to run the official local-Docker evaluator after patch generation:

```bash
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-507170da0f7f4da330f6732bffdf11c4df7fc192 \
  --pull-image \
  --evaluate
```

This calls:

```bash
python SWE-bench_Pro-os/swe_bench_pro_eval.py \
  --raw_sample_path <output>/sample.jsonl \
  --patch_path <output>/patches.json \
  --output_dir <output>/eval \
  --scripts_dir SWE-bench_Pro-os/run_scripts \
  --num_workers 1 \
  --dockerhub_username jefzda \
  --use_local_docker \
  --docker_platform linux/amd64
```

For broader runs, iterate instance IDs from `SWE-bench_Pro-os/helper_code/sweap_eval_full_v2.jsonl`, then gather the `.pred` patches into the JSON shape expected by `swe_bench_pro_eval.py`.

## Notes

- SWE-bench Pro patch generation and evaluation are expensive. Start with one Flipt instance before scaling to harder repositories such as NodeBB, Element, or qutebrowser.
- DockerHub rate limits can block large sweap image pulls; use `docker login` if pulls fail.
- Pragma code is not pushed to `origin` as part of benchmark work.

## Competitive Target

Treat SWE-bench Pro as a competitive target for Pragma, not just a smoke test.

Official Scale Labs public leaderboard snapshot checked on 2026-05-31:

| Model | Provider | Score |
|---|---|---:|
| `gpt-5.4 (xHigh)*` | OpenAI | 59.1 |
| `Muse Spark*` | Meta | 55.0 |
| `claude-opus-4-6 (thinking)*` | Anthropic | 51.9 |
| `gemini-3.1-pro (thinking)*` | Google | 46.1 |
| `claude-opus-4-5-20251101` | Anthropic | 45.89 |
| `claude-4-5-Sonnet` | Anthropic | 43.6 |
| `gpt-5-2025-08-07 (High)` | OpenAI | 41.78 |
| `gpt-5.2-codex` | OpenAI | 41.04 |
| `qwen3-coder-480b-a35b` | Alibaba | 38.7 |
| `minimax-2.1` | MiniMax | 36.81 |

Source:

```text
https://labs.scale.com/api/pdf/leaderboard/swe_bench_pro_public
```

The private Scale leaderboard had lower and different scores in the same lookup, including `claude-opus-4-6 (thinking)` at `47.10±6.07`, `Muse Spark` at `44.70±6.05`, and `gpt-5.4(xHigh)` at `43.40±6.03`.

Third-party aggregators such as BenchLM reported higher scores, including Claude Mythos Preview at `77.8` and MiniMax M2.7 at `56.2`, but those must be labeled separately and not conflated with official Scale standings.

Re-check live standings before making competitive claims. This leaderboard is moving quickly.

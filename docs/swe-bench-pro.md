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
2. Starts the selected SWE-bench Pro Docker image.
3. Runs `/preprocess.sh` when the image provides it.
4. Runs Pragma in `/app` with the Pragma loop.
5. Captures `git diff --binary` as `<instance_id>.pred`.

Default Pragma settings:

| Setting | Value |
|---|---|
| Provider | `lilac` |
| Model | `$LLM_MODEL`, default `minimaxai/minimax-m2.7` |
| Base URL | `$LLM_BASE_URL`, then `providers.lilac.base_url`, then `https://api.getlilac.com/v1` |
| Permission mode | `bypassPermissions` |
| Context mode | `chat` |
| Allowed tools | `Bash` |
| Temperature | `$PRAGMA_TEMPERATURE`, default `0` |
| Max turns | `$PRAGMA_MAX_TURNS`, default `250` |
| Agent timeout | `$PRAGMA_AGENT_TIMEOUT`, default `7200` seconds |

Extra Pragma flags can be passed through `PRAGMA_EXTRA_ARGS`.

For orchestration runs, the runner mounts the repo-local YAML directories into
the benchmark container:

| Host path | Container path |
|---|---|
| `orchestrations/` | `/pragma/orchestrations` |
| `personas/` | `/pragma/personas` |
| `personas-research-v2/` | `/pragma/personas-research-v2` |

Example:

```bash
PRAGMA_EXTRA_ARGS='orchestration run /pragma/orchestrations/architect-implementer-prosecutor.yaml --persona-dir /pragma/personas' \
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-507170da0f7f4da330f6732bffdf11c4df7fc192 \
  --pull-image \
  --evaluate
```

To test the prompt-control v2 persona set from scratch on the Flipt Kubernetes
task, use the v2 orchestration and persona directory:

```bash
PRAGMA_EXTRA_ARGS='orchestration run /pragma/orchestrations/prompt-control-v2-benchmark.yaml --persona-dir /pragma/personas-research-v2' \
tools/run_swebench_pro_instance.py \
  --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
  --pull-image \
  --evaluate
```

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

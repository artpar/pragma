#!/usr/bin/env python3
"""Run Codex CLI on one SWE-bench Pro instance and collect a .pred patch."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import shlex
import stat
import subprocess
import sys
import tarfile
import urllib.request

from run_swebench_pro_instance import (
    DEFAULT_INSTANCE_ID,
    DEFAULT_PRO_REPO,
    docker_eval_env,
    dockerhub_image,
    maybe_evaluate,
    read_existing_run,
    read_sample,
)


CODEX_LINUX_AMD64_URL = (
    "https://github.com/openai/codex/releases/latest/download/"
    "codex-x86_64-unknown-linux-musl.tar.gz"
)


def display_command(command: list[str]) -> list[str]:
    redacted = command.copy()
    for i, part in enumerate(redacted):
        if part in {"OPENAI_API_KEY"} and i + 1 < len(redacted):
            redacted[i + 1] = "<redacted>"
        elif part.startswith("OPENAI_API_KEY="):
            redacted[i] = "OPENAI_API_KEY=<redacted>"
    return redacted


def run(command: list[str], cwd: Path, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(shlex.quote(part) for part in display_command(command)), flush=True)
    subprocess.run(command, cwd=cwd, env=env, check=True)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--swe-bench-pro-path", type=Path, default=DEFAULT_PRO_REPO)
    parser.add_argument("--instance-id", default=DEFAULT_INSTANCE_ID)
    parser.add_argument("--sample-jsonl", type=Path)
    parser.add_argument("--output-dir", type=Path)
    parser.add_argument("--dockerhub-username", default="jefzda")
    parser.add_argument("--codex-url", default=os.getenv("CODEX_LINUX_AMD64_URL", CODEX_LINUX_AMD64_URL))
    parser.add_argument("--codex-home", type=Path, default=Path.home() / ".codex")
    parser.add_argument("--agent-timeout", default=os.getenv("CODEX_AGENT_TIMEOUT", "7200"))
    parser.add_argument("--prepare-only", action="store_true", help="prepare prompt, metadata, and Codex binary, but do not run Docker")
    parser.add_argument("--pull-image", action="store_true", help="pull the selected Docker image before running")
    parser.add_argument("--evaluate", action="store_true", help="run the official local-Docker evaluator after patch generation")
    parser.add_argument("--evaluate-existing", action="store_true", help="evaluate an existing output directory without rerunning Codex")
    return parser.parse_args()


def download_codex_binary(output_dir: Path, codex_url: str) -> Path:
    archive = output_dir / "codex-linux-amd64.tar.gz"
    bin_dir = output_dir / "codex-bin"
    codex_bin = bin_dir / "codex"
    if codex_bin.exists():
        return codex_bin

    print(f"+ download {codex_url} -> {archive}", flush=True)
    urllib.request.urlretrieve(codex_url, archive)
    bin_dir.mkdir(parents=True, exist_ok=True)
    with tarfile.open(archive, "r:gz") as tar:
        tar.extractall(bin_dir)

    candidates = [path for path in bin_dir.rglob("*") if path.is_file()]
    for path in candidates:
        mode = path.stat().st_mode
        if mode & stat.S_IXUSR or path.name.startswith("codex"):
            if path != codex_bin:
                path.rename(codex_bin)
            codex_bin.chmod(codex_bin.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)
            return codex_bin
    raise SystemExit(f"Codex binary not found in archive: {archive}")


def main() -> None:
    args = parse_args()
    repo_root = Path(__file__).resolve().parents[1]
    pro_repo = args.swe_bench_pro_path
    sample_jsonl = args.sample_jsonl or pro_repo / "helper_code" / "sweap_eval_full_v2.jsonl"
    timestamp = dt.datetime.now(dt.UTC).strftime("%Y%m%dT%H%M%SZ")
    output_dir = (args.output_dir or repo_root / ".pragma" / "swe-bench-pro-codex" / f"{timestamp}-{args.instance_id}").resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    if args.evaluate_existing:
        if args.output_dir is None:
            raise SystemExit("--evaluate-existing requires --output-dir")
        instance_id, row, pred_path = read_existing_run(output_dir)
        print(f"instance_id={instance_id}")
        print(f"output_dir={output_dir}")
        print(f"pred_path={pred_path}")
        maybe_evaluate(pro_repo, output_dir, row, pred_path, "codex", args.dockerhub_username)
        return

    row = read_sample(sample_jsonl, args.instance_id)
    image = dockerhub_image(row, args.dockerhub_username)
    prompt_path = output_dir / "prompt.txt"
    prompt_path.write_text(str(row["problem_statement"]), encoding="utf-8")
    metadata_path = output_dir / "metadata.json"
    metadata_path.write_text(json.dumps({"instance_id": args.instance_id, "image": image, "row": row}, indent=2), encoding="utf-8")

    codex_bin = download_codex_binary(output_dir, args.codex_url)
    print(f"instance_id={args.instance_id}")
    print(f"image={image}")
    print(f"output_dir={output_dir}")
    print(f"codex_bin={codex_bin}")
    if args.prepare_only:
        return
    if args.pull_image:
        run(["docker", "pull", "--platform", "linux/amd64", image], repo_root)

    if not (args.codex_home / "auth.json").exists():
        raise SystemExit(f"missing Codex auth file: {args.codex_home / 'auth.json'}")

    pred_path = output_dir / f"{args.instance_id}.pred"
    status_path = output_dir / "agent-status.txt"
    container_script = f"""
set -u
cd /app
if [ -x /preprocess.sh ]; then /preprocess.sh; fi
mkdir -p /tmp/codex-home /pragma-out
cp /codex-host-home/auth.json /tmp/codex-home/auth.json
if [ -f /codex-host-home/config.toml ]; then cp /codex-host-home/config.toml /tmp/codex-home/config.toml; fi
if [ -f /codex-host-home/models_cache.json ]; then cp /codex-host-home/models_cache.json /tmp/codex-home/models_cache.json; fi
export HOME=/tmp/codex-home
export CODEX_HOME=/tmp/codex-home
set +e
timeout {shlex.quote(str(args.agent_timeout))} /codex-bin/codex exec \\
  --dangerously-bypass-approvals-and-sandbox \\
  --skip-git-repo-check \\
  --ephemeral \\
  -C /app \\
  --output-last-message /pragma-out/codex-last-message.txt \\
  - < /pragma-out/prompt.txt \\
  > /pragma-out/codex.stdout.log \\
  2> /pragma-out/codex.stderr.log
code=$?
echo "$code" > /pragma-out/agent-status.txt
git add -N .
git diff --binary > /pragma-out/{shlex.quote(args.instance_id)}.pred
exit 0
"""
    run(
        [
            "docker",
            "run",
            "--rm",
            "--platform",
            "linux/amd64",
            "-v",
            f"{codex_bin.parent}:/codex-bin:ro",
            "-v",
            f"{output_dir}:/pragma-out",
            "-v",
            f"{args.codex_home.resolve()}:/codex-host-home:ro",
            image,
            "-c",
            container_script,
        ],
        repo_root,
        docker_eval_env(),
    )
    print(f"agent_status={status_path.read_text(encoding='utf-8').strip() if status_path.exists() else 'missing'}")
    print(f"pred_path={pred_path}")
    print(f"codex_stdout={output_dir / 'codex.stdout.log'}")
    print(f"codex_stderr={output_dir / 'codex.stderr.log'}")
    if args.evaluate:
        maybe_evaluate(pro_repo, output_dir, row, pred_path, "codex", args.dockerhub_username)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Run Pragma on one SWE-bench Pro instance and collect a .pred patch."""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
from pathlib import Path
import shlex
import shutil
import stat
import subprocess
import sys
import urllib.request
import zipfile


DEFAULT_PRO_REPO = Path("/Users/artpar/workspace/code/SWE-bench_Pro-os")
DEFAULT_INSTANCE_ID = "instance_flipt-io__flipt-507170da0f7f4da330f6732bffdf11c4df7fc192"
DEFAULT_BUF_URL = "https://github.com/bufbuild/buf/releases/latest/download/buf-Linux-x86_64"
DEFAULT_PROTOBUF_RELEASE_API = "https://api.github.com/repos/protocolbuffers/protobuf/releases/latest"
DEFAULT_PROTOC_GEN_GO_VERSION = "v1.36.6"
DEFAULT_PROTOC_GEN_GO_GRPC_VERSION = "v1.5.1"
DEFAULT_GRPC_GATEWAY_VERSION = "v2.29.0"


def display_command(command: list[str]) -> list[str]:
    redacted = command.copy()
    for i, part in enumerate(redacted):
        if part in {"LLM_API_KEY", "LILAC_API_KEY"} and i + 1 < len(redacted):
            redacted[i + 1] = "<redacted>"
        elif part.startswith("LLM_API_KEY=") or part.startswith("LILAC_API_KEY="):
            redacted[i] = part.split("=", 1)[0] + "=<redacted>"
    return redacted


def run(command: list[str], cwd: Path, env: dict[str, str] | None = None) -> None:
    print("+", " ".join(shlex.quote(part) for part in display_command(command)), flush=True)
    subprocess.run(command, cwd=cwd, env=env, check=True)


def read_sample(sample_path: Path, instance_id: str) -> dict[str, object]:
    with sample_path.open(encoding="utf-8") as handle:
        for line in handle:
            row = json.loads(line)
            if row["instance_id"] == instance_id:
                return row
    raise SystemExit(f"instance not found in {sample_path}: {instance_id}")


def format_problem_statement(row: dict[str, object]) -> str:
    return decode_embedded_json_string_lines(str(row["problem_statement"]))


def decode_embedded_json_string_lines(text: str) -> str:
    lines: list[str] = []
    for line in text.splitlines():
        stripped = line.strip()
        if len(stripped) < 2 or not stripped.startswith('"') or not stripped.endswith('"'):
            lines.append(line)
            continue
        try:
            decoded = json.loads(stripped)
        except json.JSONDecodeError:
            lines.append(line)
            continue
        if not isinstance(decoded, str):
            lines.append(line)
            continue
        indent = line[: len(line) - len(line.lstrip())]
        decoded_lines = decoded.splitlines()
        if not decoded_lines:
            lines.append("")
            continue
        lines.append(indent + decoded_lines[0])
        lines.extend(decoded_lines[1:])
    return "\n".join(lines)


def read_pragma_lilac_credentials() -> tuple[str, str]:
    credentials_path = Path.home() / ".pragma" / "credentials.yml"
    if not credentials_path.exists():
        return "", ""
    text = credentials_path.read_text(encoding="utf-8")
    in_lilac = False
    api_key = ""
    base_url = ""
    for raw_line in text.splitlines():
        line = raw_line.rstrip()
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue
        if not raw_line.startswith((" ", "\t")) and stripped == "providers:":
            in_lilac = False
            continue
        if raw_line.startswith("  ") and not raw_line.startswith("    "):
            in_lilac = stripped.rstrip(":") == "lilac"
            continue
        if in_lilac and raw_line.startswith("    "):
            key, sep, value = stripped.partition(":")
            if not sep:
                continue
            value = value.strip().strip("'\"")
            if key == "api_key":
                api_key = value
            elif key == "base_url":
                base_url = value
    return api_key, base_url


def dockerhub_image(row: dict[str, object], dockerhub_username: str) -> str:
    repo = str(row["repo"]).lower()
    repo_base, repo_name = repo.split("/", 1)
    instance_id = str(row["instance_id"])
    tag_suffix = instance_id.removeprefix("instance_")

    if instance_id == "instance_element-hq__element-web-ec0f940ef0e8e3b61078f145f34dc40d1938e6c5-vnan":
        repo_name = "element-web"
    elif "element-hq" in repo and "element-web" in repo:
        repo_name = "element"
        if tag_suffix.endswith("-vnan"):
            tag_suffix = tag_suffix[:-5]
    elif tag_suffix.endswith("-vnan"):
        tag_suffix = tag_suffix[:-5]

    tag = f"{repo_base}.{repo_name}-{tag_suffix}"
    if len(tag) > 128:
        tag = tag[:128]
    return f"{dockerhub_username}/sweap-images:{tag}"


def build_linux_binary(repo_root: Path, output_dir: Path) -> Path:
    binary = output_dir / "pragma-linux-amd64"
    env = os.environ.copy()
    env.update({"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"})
    run(["go", "build", "-o", str(binary), "./cmd/pragma"], repo_root, env)
    return binary


def make_executable(path: Path) -> None:
    path.chmod(path.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)


def download_file(url: str, destination: Path) -> None:
    if destination.exists():
        return
    destination.parent.mkdir(parents=True, exist_ok=True)
    temp_path = destination.with_suffix(destination.suffix + ".tmp")
    print(f"+ download {url} -> {destination}", flush=True)
    urllib.request.urlretrieve(url, temp_path)
    temp_path.replace(destination)


def latest_protoc_url() -> str:
    request = urllib.request.Request(
        DEFAULT_PROTOBUF_RELEASE_API,
        headers={"User-Agent": "pragma-swebench-runner"},
    )
    with urllib.request.urlopen(request) as response:
        release = json.load(response)
    for asset in release.get("assets", []):
        name = str(asset.get("name", ""))
        if name.startswith("protoc-") and name.endswith("-linux-x86_64.zip"):
            return str(asset["browser_download_url"])
    raise SystemExit("could not find a linux-x86_64 protoc asset in latest protobuf release")


def prepare_buf(bin_dir: Path, url: str) -> None:
    buf_bin = bin_dir / "buf"
    download_file(url, buf_bin)
    make_executable(buf_bin)


def prepare_protoc(bin_dir: Path, url: str) -> None:
    protoc_bin = bin_dir / "protoc"
    if protoc_bin.exists():
        return
    archive = bin_dir.parent / "downloads" / Path(url).name
    download_file(url, archive)
    print(f"+ extract {archive} -> {bin_dir.parent}", flush=True)
    with zipfile.ZipFile(archive) as zip_file:
        zip_file.extractall(bin_dir.parent)
    extracted = bin_dir.parent / "bin" / "protoc"
    if not extracted.exists():
        raise SystemExit(f"protoc not found after extracting {archive}")
    make_executable(extracted)


def install_go_tool(
    repo_root: Path,
    bin_dir: Path,
    package: str,
    version: str,
    binary_name: str,
) -> None:
    binary = bin_dir / binary_name
    if binary.exists():
        return
    env = os.environ.copy()
    gopath = bin_dir.parent / "gopath"
    env.update(
        {
            "GOOS": "linux",
            "GOARCH": "amd64",
            "CGO_ENABLED": "0",
            "GOPATH": str(gopath),
        }
    )
    run(["go", "install", f"{package}@{version}"], repo_root, env)
    candidates = [
        gopath / "bin" / "linux_amd64" / binary_name,
        gopath / "bin" / binary_name,
    ]
    for candidate in candidates:
        if candidate.exists():
            shutil.copy2(candidate, binary)
            break
    else:
        raise SystemExit(f"go install did not create expected tool: {binary}")
    make_executable(binary)


def prepare_grpc_gateway_includes(bin_dir: Path, version: str) -> None:
    gopath = bin_dir.parent / "gopath"
    module_dir = gopath / "pkg" / "mod" / "github.com" / "grpc-ecosystem" / "grpc-gateway" / f"v2@{version}"
    if not module_dir.exists():
        candidates = sorted((gopath / "pkg" / "mod" / "github.com" / "grpc-ecosystem" / "grpc-gateway").glob("v2@*"))
        if not candidates:
            raise SystemExit("grpc-gateway module cache not found after installing generator tools")
        module_dir = candidates[-1]

    source_dir = module_dir / "protoc-gen-openapiv2" / "options"
    if not source_dir.exists():
        raise SystemExit(f"grpc-gateway include protos not found: {source_dir}")

    destination_dir = bin_dir.parent / "include" / "protoc-gen-openapiv2" / "options"
    destination_dir.mkdir(parents=True, exist_ok=True)
    for name in ("annotations.proto", "openapiv2.proto"):
        source = source_dir / name
        destination = destination_dir / name
        if not source.exists():
            raise SystemExit(f"grpc-gateway include proto not found: {source}")
        if destination.exists():
            continue
        shutil.copy2(source, destination)
        destination.chmod(destination.stat().st_mode | stat.S_IWUSR)


def prepare_generator_toolchain(repo_root: Path, args: argparse.Namespace) -> Path:
    toolchain_dir = (
        Path(args.generator_toolchain_dir)
        if args.generator_toolchain_dir
        else repo_root / ".pragma" / "toolchains" / "swebench-pro-linux-amd64"
    ).resolve()
    bin_dir = toolchain_dir / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)

    prepare_buf(bin_dir, args.buf_url)
    prepare_protoc(bin_dir, args.protoc_url or latest_protoc_url())
    install_go_tool(
        repo_root,
        bin_dir,
        "google.golang.org/protobuf/cmd/protoc-gen-go",
        args.protoc_gen_go_version,
        "protoc-gen-go",
    )
    install_go_tool(
        repo_root,
        bin_dir,
        "google.golang.org/grpc/cmd/protoc-gen-go-grpc",
        args.protoc_gen_go_grpc_version,
        "protoc-gen-go-grpc",
    )
    install_go_tool(
        repo_root,
        bin_dir,
        "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway",
        args.grpc_gateway_version,
        "protoc-gen-grpc-gateway",
    )
    install_go_tool(
        repo_root,
        bin_dir,
        "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2",
        args.grpc_gateway_version,
        "protoc-gen-openapiv2",
    )
    prepare_grpc_gateway_includes(bin_dir, args.grpc_gateway_version)

    return toolchain_dir


def normalize_eval_test_list(value: object) -> str:
    if value is None:
        return "[]"
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return json.dumps(value)
    raise TypeError(f"expected test list to be a string or list, got {type(value).__name__}")


def normalize_eval_sample(row: dict[str, object]) -> dict[str, object]:
    eval_row = row.copy()
    aliases = {
        "FAIL_TO_PASS": "fail_to_pass",
        "PASS_TO_PASS": "pass_to_pass",
    }
    for upper_key, lower_key in aliases.items():
        if lower_key in eval_row:
            eval_row[lower_key] = normalize_eval_test_list(eval_row[lower_key])
        elif upper_key in eval_row:
            eval_row[lower_key] = normalize_eval_test_list(eval_row[upper_key])
    return eval_row


def docker_eval_env() -> dict[str, str]:
    env = os.environ.copy()
    docker_socket = Path("/Users/artpar/.docker/run/docker.sock")
    if "DOCKER_HOST" not in env and docker_socket.exists():
        env["DOCKER_HOST"] = f"unix://{docker_socket}"
    return env


def write_eval_inputs(output_dir: Path, row: dict[str, object], prefix: str, pred_path: Path) -> tuple[Path, Path]:
    raw_sample = output_dir / "sample.jsonl"
    eval_row = normalize_eval_sample(row)
    raw_sample.write_text(json.dumps(eval_row) + "\n", encoding="utf-8")
    patch_json = output_dir / "patches.json"
    patch_json.write_text(
        json.dumps(
            [
                {
                    "instance_id": eval_row["instance_id"],
                    "patch": pred_path.read_text(encoding="utf-8", errors="replace"),
                    "prefix": prefix,
                }
            ],
            indent=2,
        )
        + "\n",
        encoding="utf-8",
    )
    return raw_sample, patch_json


def maybe_evaluate(pro_repo: Path, output_dir: Path, row: dict[str, object], pred_path: Path, prefix: str, dockerhub_username: str) -> None:
    raw_sample, patch_json = write_eval_inputs(output_dir, row, prefix, pred_path)
    evaluator_python = pro_repo / ".venv" / "bin" / "python"
    python = str(evaluator_python) if evaluator_python.exists() else sys.executable
    run(
        [
            python,
            str(pro_repo / "swe_bench_pro_eval.py"),
            "--raw_sample_path",
            str(raw_sample),
            "--patch_path",
            str(patch_json),
            "--output_dir",
            str(output_dir / "eval"),
            "--scripts_dir",
            str(pro_repo / "run_scripts"),
            "--num_workers",
            "1",
            "--dockerhub_username",
            dockerhub_username,
            "--use_local_docker",
            "--docker_platform",
            "linux/amd64",
        ],
        pro_repo,
        docker_eval_env(),
    )


def read_existing_run(output_dir: Path) -> tuple[str, dict[str, object], Path]:
    metadata_path = output_dir / "metadata.json"
    if not metadata_path.exists():
        raise SystemExit(f"missing metadata.json in existing output dir: {output_dir}")
    metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
    instance_id = str(metadata["instance_id"])
    row = metadata["row"]
    if not isinstance(row, dict):
        raise SystemExit(f"metadata row is not an object: {metadata_path}")
    pred_path = output_dir / f"{instance_id}.pred"
    if not pred_path.exists():
        raise SystemExit(f"missing prediction patch: {pred_path}")
    return instance_id, row, pred_path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--swe-bench-pro-path", type=Path, default=DEFAULT_PRO_REPO)
    parser.add_argument("--instance-id", default=DEFAULT_INSTANCE_ID)
    parser.add_argument("--sample-jsonl", type=Path)
    parser.add_argument("--output-dir", type=Path)
    parser.add_argument("--dockerhub-username", default="jefzda")
    parser.add_argument("--model", default=os.getenv("LLM_MODEL", "minimaxai/minimax-m2.7"))
    parser.add_argument("--base-url", default=os.getenv("LLM_BASE_URL", ""))
    parser.add_argument("--max-turns", default=os.getenv("PRAGMA_MAX_TURNS", "250"))
    parser.add_argument("--temperature", default=os.getenv("PRAGMA_TEMPERATURE", "0"))
    parser.add_argument("--agent-timeout", default=os.getenv("PRAGMA_AGENT_TIMEOUT", "7200"))
    default_generator_toolchain = os.getenv("SWE_BENCH_GENERATOR_TOOLCHAIN", "1").lower() not in {
        "0",
        "false",
        "no",
    }
    parser.add_argument(
        "--generator-toolchain",
        dest="generator_toolchain",
        action="store_true",
        default=default_generator_toolchain,
        help="prepare and mount linux/amd64 buf/protoc/protobuf generator tools into the benchmark container",
    )
    parser.add_argument(
        "--no-generator-toolchain",
        dest="generator_toolchain",
        action="store_false",
        help="do not prepare or mount generator tools into the benchmark container",
    )
    parser.add_argument(
        "--generator-toolchain-dir",
        type=Path,
        default=os.getenv("SWE_BENCH_GENERATOR_TOOLCHAIN_DIR"),
    )
    parser.add_argument("--buf-url", default=os.getenv("SWE_BENCH_BUF_URL", DEFAULT_BUF_URL))
    parser.add_argument("--protoc-url", default=os.getenv("SWE_BENCH_PROTOC_URL", ""))
    parser.add_argument(
        "--protoc-gen-go-version",
        default=os.getenv("SWE_BENCH_PROTOC_GEN_GO_VERSION", DEFAULT_PROTOC_GEN_GO_VERSION),
    )
    parser.add_argument(
        "--protoc-gen-go-grpc-version",
        default=os.getenv("SWE_BENCH_PROTOC_GEN_GO_GRPC_VERSION", DEFAULT_PROTOC_GEN_GO_GRPC_VERSION),
    )
    parser.add_argument(
        "--grpc-gateway-version",
        default=os.getenv("SWE_BENCH_GRPC_GATEWAY_VERSION", DEFAULT_GRPC_GATEWAY_VERSION),
    )
    parser.add_argument("--prepare-only", action="store_true", help="build binary and print selected image, but do not run Docker")
    parser.add_argument("--pull-image", action="store_true", help="pull the selected Docker image before running")
    parser.add_argument("--evaluate", action="store_true", help="run the official local-Docker evaluator after patch generation")
    parser.add_argument("--evaluate-existing", action="store_true", help="evaluate an existing output directory without rerunning Pragma")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    repo_root = Path(__file__).resolve().parents[1]
    pro_repo = args.swe_bench_pro_path
    sample_jsonl = args.sample_jsonl or pro_repo / "helper_code" / "sweap_eval_full_v2.jsonl"
    timestamp = dt.datetime.now(dt.UTC).strftime("%Y%m%dT%H%M%SZ")
    output_dir = (args.output_dir or repo_root / ".pragma" / "swe-bench-pro" / f"{timestamp}-{args.instance_id}").resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    if args.evaluate_existing:
        if args.output_dir is None:
            raise SystemExit("--evaluate-existing requires --output-dir")
        instance_id, row, pred_path = read_existing_run(output_dir)
        print(f"instance_id={instance_id}")
        print(f"output_dir={output_dir}")
        print(f"pred_path={pred_path}")
        maybe_evaluate(pro_repo, output_dir, row, pred_path, "pragma", args.dockerhub_username)
        return

    row = read_sample(sample_jsonl, args.instance_id)
    config_api_key, config_base_url = read_pragma_lilac_credentials()
    api_key = os.getenv("LLM_API_KEY") or os.getenv("LILAC_API_KEY") or config_api_key
    base_url = args.base_url or config_base_url or "https://api.getlilac.com/v1"
    image = dockerhub_image(row, args.dockerhub_username)
    prompt = format_problem_statement(row)
    prompt_path = output_dir / "prompt.txt"
    prompt_path.write_text(prompt, encoding="utf-8")
    metadata_path = output_dir / "metadata.json"
    metadata_path.write_text(json.dumps({"instance_id": args.instance_id, "image": image, "row": row}, indent=2), encoding="utf-8")

    binary = build_linux_binary(repo_root, output_dir)
    toolchain_dir = prepare_generator_toolchain(repo_root, args) if args.generator_toolchain else None
    print(f"instance_id={args.instance_id}")
    print(f"image={image}")
    print(f"output_dir={output_dir}")
    if toolchain_dir is not None:
        print(f"generator_toolchain={toolchain_dir}")
    if args.prepare_only:
        return
    if args.pull_image:
        run(["docker", "pull", "--platform", "linux/amd64", image], repo_root)

    pred_path = output_dir / f"{args.instance_id}.pred"
    status_path = output_dir / "agent-status.txt"
    raw_http_dir = output_dir / "raw-http-pragma"
    toolchain_preflight_path = output_dir / "toolchain-preflight.log"
    extra_args = os.getenv("PRAGMA_EXTRA_ARGS", "")
    container_script = f"""
set -u
cd /app
if [ -x /preprocess.sh ]; then /preprocess.sh; fi
mkdir -p /tmp/pragma-home /pragma-out/raw-http-pragma
export HOME=/tmp/pragma-home
export LILAC_API_KEY="$LLM_API_KEY"
export LILAC_BASE_URL="$LILAC_BASE_URL"
export PRAGMA_RAW_HTTP_CAPTURE_DIR=/pragma-out/raw-http-pragma
if [ -d /pragma-toolchain/bin ]; then
  export PATH="/pragma-toolchain/bin:$PATH"
fi
{{
  echo "generator toolchain preflight:"
  for tool in buf protoc protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2; do
    if command -v "$tool" >/dev/null 2>&1; then
      printf "%s: " "$tool"
      "$tool" --version 2>/dev/null || true
    else
      echo "$tool: not found"
    fi
  done
  if [ -f /pragma-toolchain/include/protoc-gen-openapiv2/options/annotations.proto ]; then
    echo "protoc-gen-openapiv2 protos: present"
  else
    echo "protoc-gen-openapiv2 protos: not found"
  fi
}} > /pragma-out/toolchain-preflight.log 2>&1
cat /pragma-out/toolchain-preflight.log
set +e
timeout {shlex.quote(str(args.agent_timeout))} /pragma-bin \\
  --provider lilac \\
  --model {shlex.quote(args.model)} \\
  --permission-mode bypassPermissions \\
  --allowed-tools Bash \\
  --temperature {shlex.quote(str(args.temperature))} \\
  --max-turns {shlex.quote(str(args.max_turns))} \\
  {extra_args} \\
  --prompt "$(cat /pragma-out/prompt.txt)" \\
  > /pragma-out/pragma.stdout.log \\
  2> /pragma-out/pragma.stderr.log
code=$?
echo "$code" > /pragma-out/agent-status.txt
git add -N .
git diff --binary > /pragma-out/{shlex.quote(args.instance_id)}.pred
exit 0
"""
    env_args = [
        "-e",
        f"LLM_API_KEY={api_key}",
        "-e",
        f"LILAC_BASE_URL={base_url}",
    ]
    if not api_key:
        raise SystemExit("set LLM_API_KEY or LILAC_API_KEY, or add providers.lilac.api_key to ~/.pragma/credentials.yml")
    toolchain_mount_args = []
    if toolchain_dir is not None:
        toolchain_mount_args = ["-v", f"{toolchain_dir}:/pragma-toolchain:ro"]
    run(
        [
            "docker",
            "run",
            "--rm",
            "--platform",
            "linux/amd64",
            *env_args,
            "-v",
            f"{binary}:/pragma-bin:ro",
            "-v",
            f"{output_dir}:/pragma-out",
            *toolchain_mount_args,
            "-v",
            f"{repo_root / 'orchestrations'}:/pragma/orchestrations:ro",
            "-v",
            f"{repo_root / 'personas'}:/pragma/personas:ro",
            "-v",
            f"{repo_root / 'personas-research-v2'}:/pragma/personas-research-v2:ro",
            image,
            "-c",
            container_script,
        ],
        repo_root,
    )
    print(f"agent_status={status_path.read_text(encoding='utf-8').strip() if status_path.exists() else 'missing'}")
    print(f"pred_path={pred_path}")
    print(f"raw_http_dir={raw_http_dir}")
    if toolchain_dir is not None:
        print(f"toolchain_preflight={toolchain_preflight_path}")
    if args.evaluate:
        maybe_evaluate(pro_repo, output_dir, row, pred_path, "pragma", args.dockerhub_username)


if __name__ == "__main__":
    main()

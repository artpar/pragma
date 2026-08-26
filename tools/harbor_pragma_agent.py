"""Minimal Harbor adapter for Pragma's normal provider-tools product path."""

import os
import shlex
from pathlib import Path
from typing import override

import certifi

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext


def _openrouter_api_key() -> str | None:
    """Resolve the benchmark credential without copying host config to a task."""
    if value := os.environ.get("OPENROUTER_API_KEY"):
        return value
    credentials = Path.home() / ".pragma" / "credentials.yml"
    try:
        lines = credentials.read_text().splitlines()
    except OSError:
        return None
    in_openrouter = False
    for raw_line in lines:
        stripped = raw_line.strip()
        if raw_line.startswith("  ") and not raw_line.startswith("    "):
            in_openrouter = stripped == "openrouter:"
            continue
        if in_openrouter and raw_line.startswith("    ") and stripped.startswith("api_key:"):
            return stripped.split(":", 1)[1].strip().strip("'\"") or None
    return None


class PragmaAgent(BaseAgent):
    """Upload a prebuilt Pragma binary and run one ordinary CLI session."""

    def __init__(self, *args, bundle_dir: str, **kwargs):
        super().__init__(*args, **kwargs)
        self.bundle_dir = Path(bundle_dir).resolve()

    @staticmethod
    @override
    def name() -> str:
        return "pragma"

    @override
    def version(self) -> str:
        return "dev"

    @override
    async def setup(self, environment: BaseEnvironment) -> None:
        required = (
            self.bundle_dir / "pragma",
            self.bundle_dir / "ca-certificates.crt",
        )
        missing = [str(path) for path in required if not path.exists()]
        if missing:
            raise FileNotFoundError(f"Pragma bundle is incomplete: {missing}")

        await environment.exec("mkdir -p /opt/pragma", user="root")
        await environment.upload_file(required[0], "/opt/pragma/pragma")
        await environment.upload_file(required[1], "/opt/pragma/ca-certificates.crt")
        result = await environment.exec(
            "chmod 0755 /opt/pragma/pragma && /opt/pragma/pragma version",
            user="root",
        )
        if result.return_code != 0:
            raise RuntimeError(result.stderr or result.stdout or "Pragma setup failed")

    @override
    async def run(
        self,
        instruction: str,
        environment: BaseEnvironment,
        context: AgentContext,
    ) -> None:
        api_key = _openrouter_api_key()
        if not api_key:
            raise RuntimeError("OPENROUTER_API_KEY is required by the Pragma adapter")

        model = self.model_name or "z-ai/glm-5.3"
        log_dir = self.environment_logs_dir.as_posix()
        command = " ".join(
            [
                "set -o pipefail;",
                "mkdir -p",
                shlex.quote(log_dir),
                ";",
                "/opt/pragma/pragma",
                "--prompt",
                shlex.quote(instruction),
                "--loop provider-tools",
                "--provider openrouter",
                "--model",
                shlex.quote(model),
                "--temperature 0",
                "--max-tokens 4096",
                "--max-turns 100",
                "--permission-mode bypassPermissions",
                "--record",
                "2>&1 | tee",
                shlex.quote(f"{log_dir}/pragma-output.log"),
            ]
        )
        result = await environment.exec(
            command,
            cwd="/app",
            env={
                "OPENROUTER_API_KEY": api_key,
                "HOME": f"{log_dir}/home",
                "SSL_CERT_FILE": "/opt/pragma/ca-certificates.crt",
            },
        )
        context.metadata = {
            "pragma_return_code": result.return_code,
            "pragma_stdout_tail": (result.stdout or "")[-4000:],
            "pragma_stderr_tail": (result.stderr or "")[-4000:],
        }
        if result.return_code != 0:
            output = result.stderr or result.stdout or "Pragma returned no output"
            raise RuntimeError(f"Pragma exited with {result.return_code}: {output[-4000:]}")

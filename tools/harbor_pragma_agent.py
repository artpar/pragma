"""Harbor adapter that runs Pragma's single-owner engineering orchestration."""

import os
import shlex
from pathlib import Path
from typing import override

import certifi

from harbor.agents.base import BaseAgent
from harbor.environments.base import BaseEnvironment
from harbor.models.agent.context import AgentContext


class PragmaAgent(BaseAgent):
    """Upload a prebuilt Pragma bundle and run it inside the task container."""

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
            self.bundle_dir / "swe-single-owner-engineering-loop.yaml",
            self.bundle_dir / "personas",
        )
        missing = [str(path) for path in required if not path.exists()]
        if missing:
            raise FileNotFoundError(f"Pragma bundle is incomplete: {missing}")

        await environment.exec("mkdir -p /opt/pragma", user="root")
        await environment.upload_file(required[0], "/opt/pragma/pragma")
        await environment.upload_file(
            required[1], "/opt/pragma/swe-single-owner-engineering-loop.yaml"
        )
        await environment.upload_dir(required[2], "/opt/pragma/personas")
        await environment.upload_file(certifi.where(), "/opt/pragma/ca-certificates.crt")
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
        api_key = os.environ.get("OPENROUTER_API_KEY")
        if not api_key:
            raise RuntimeError("OPENROUTER_API_KEY is required by the Pragma adapter")

        model = self.model_name or "z-ai/glm-5.3-flash"
        command = " ".join(
            [
                "/opt/pragma/pragma",
                "orchestration run",
                "/opt/pragma/swe-single-owner-engineering-loop.yaml",
                "--persona-dir /opt/pragma/personas",
                "--prompt",
                shlex.quote(instruction),
                "--provider openrouter",
                "--model",
                shlex.quote(model),
                "--temperature 1",
                "--max-tokens 65536",
                "--permission-mode bypassPermissions",
                "--record",
            ]
        )
        result = await environment.exec(
            command,
            cwd="/app",
            env={
                "OPENROUTER_API_KEY": api_key,
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

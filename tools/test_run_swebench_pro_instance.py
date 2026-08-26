#!/usr/bin/env python3

from __future__ import annotations

import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock


sys.path.insert(0, str(Path(__file__).resolve().parent))

import run_swebench_pro_instance as runner  # noqa: E402


class ProviderEnvExportTest(unittest.TestCase):
    def test_docker_host_base_url_rewrites_localhost(self) -> None:
        self.assertEqual(
            runner.docker_host_base_url("http://127.0.0.1:8080/v1"),
            "http://host.docker.internal:8080/v1",
        )
        self.assertEqual(
            runner.docker_host_base_url("http://localhost:8080/v1"),
            "http://host.docker.internal:8080/v1",
        )
        self.assertEqual(
            runner.docker_host_base_url("https://api.getlilac.com/v1"),
            "https://api.getlilac.com/v1",
        )

    def test_provider_env_exports_includes_primary_and_override_provider(self) -> None:
        credentials = {
            "openai": ("credential-openai-key", "http://127.0.0.1:8080/v1"),
            "google": ("credential-google-key", "https://google.example/v1"),
        }

        def read_credentials(provider: str) -> tuple[str, str]:
            return credentials.get(provider, ("", ""))

        env = {
            "OPENAI_API_KEY": "env-openai-key",
            "OPENAI_BASE_URL": "http://localhost:9090/v1",
        }
        with mock.patch.object(runner, "read_pragma_provider_credentials", side_effect=read_credentials):
            with mock.patch.dict(os.environ, env, clear=True):
                exports = runner.provider_env_exports(
                    "lilac",
                    "primary-lilac-key",
                    "https://api.getlilac.com/v1",
                )

        self.assertEqual(exports["LILAC_API_KEY"], "primary-lilac-key")
        self.assertEqual(exports["LILAC_BASE_URL"], "https://api.getlilac.com/v1")
        self.assertEqual(exports["OPENAI_API_KEY"], "env-openai-key")
        self.assertEqual(exports["OPENAI_BASE_URL"], "http://host.docker.internal:9090/v1")
        self.assertEqual(exports["GOOGLE_API_KEY"], "credential-google-key")
        self.assertEqual(exports["GOOGLE_BASE_URL"], "https://google.example/v1")


class GeneratorToolchainTest(unittest.TestCase):
    def test_generator_toolchain_cache_dir_is_versioned(self) -> None:
        args = mock.Mock(
            generator_toolchain_dir=None,
            buf_url="https://example.invalid/buf-v1",
            protoc_url="https://example.invalid/protoc-v1.zip",
            protoc_gen_go_version="v1.31.0",
            protoc_gen_go_grpc_version="v1.3.0",
            grpc_gateway_version="v2.15.2",
        )
        changed_args = mock.Mock(
            generator_toolchain_dir=None,
            buf_url=args.buf_url,
            protoc_url=args.protoc_url,
            protoc_gen_go_version="v1.36.6",
            protoc_gen_go_grpc_version=args.protoc_gen_go_grpc_version,
            grpc_gateway_version=args.grpc_gateway_version,
        )

        first = runner.generator_toolchain_cache_dir(Path("/repo"), args)
        second = runner.generator_toolchain_cache_dir(Path("/repo"), changed_args)

        self.assertEqual(first.parent, Path("/repo/.pragma/toolchains"))
        self.assertTrue(first.name.startswith("swebench-pro-linux-amd64-"))
        self.assertNotEqual(first, second)

    def test_generator_toolchain_cache_dir_honors_explicit_path(self) -> None:
        args = mock.Mock(generator_toolchain_dir=Path("/custom/toolchain"))

        self.assertEqual(
            runner.generator_toolchain_cache_dir(Path("/repo"), args),
            Path("/custom/toolchain"),
        )


class AgentStatusTest(unittest.TestCase):
    def test_read_agent_status_reads_status_file(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            status_path = Path(temp_dir) / "agent-status.txt"
            status_path.write_text("1\n", encoding="utf-8")

            self.assertEqual(runner.read_agent_status(status_path), "1")

    def test_read_agent_status_reports_missing(self) -> None:
        self.assertEqual(
            runner.read_agent_status(Path("/definitely/not/present/agent-status.txt")),
            "missing",
        )


class PragmaExtraArgsTest(unittest.TestCase):
    def test_stop_after_state_is_added_to_orchestration_args(self) -> None:
        args = mock.Mock(
            direct=False,
            orchestration="/pragma/orchestrations/test.yaml",
            persona_dir="/pragma/personas",
            start_at_state="",
            stop_after_state="repo_survey",
            container_seed_artifacts=[],
        )
        with mock.patch.dict(os.environ, {}, clear=True):
            extra_args = runner.pragma_extra_args(args)

        self.assertEqual(
            extra_args,
            [
                "orchestration",
                "run",
                "/pragma/orchestrations/test.yaml",
                "--persona-dir",
                "/pragma/personas",
                "--stop-after-state",
                "repo_survey",
            ],
        )

    def test_start_at_state_is_added_before_stop_after_state(self) -> None:
        args = mock.Mock(
            direct=False,
            orchestration="/pragma/orchestrations/test.yaml",
            persona_dir="/pragma/personas",
            start_at_state="validation_gate",
            stop_after_state="route_validation_gate",
            container_seed_artifacts=[],
        )
        with mock.patch.dict(os.environ, {}, clear=True):
            extra_args = runner.pragma_extra_args(args)

        self.assertEqual(
            extra_args,
            [
                "orchestration",
                "run",
                "/pragma/orchestrations/test.yaml",
                "--persona-dir",
                "/pragma/personas",
                "--start-at-state",
                "validation_gate",
                "--stop-after-state",
                "route_validation_gate",
            ],
        )

    def test_seed_artifacts_are_added_to_orchestration_args(self) -> None:
        args = mock.Mock(
            direct=False,
            orchestration="/pragma/orchestrations/test.yaml",
            persona_dir="/pragma/personas",
            start_at_state="worker",
            stop_after_state="validator",
            container_seed_artifacts=[
                "engineering_context=/pragma-out/seed-artifacts/000-engineering-context.md",
                "acceptance_map=/pragma-out/seed-artifacts/001-acceptance-map.json",
            ],
        )
        with mock.patch.dict(os.environ, {}, clear=True):
            extra_args = runner.pragma_extra_args(args)

        self.assertEqual(
            extra_args,
            [
                "orchestration",
                "run",
                "/pragma/orchestrations/test.yaml",
                "--persona-dir",
                "/pragma/personas",
                "--start-at-state",
                "worker",
                "--stop-after-state",
                "validator",
                "--seed-artifact",
                "engineering_context=/pragma-out/seed-artifacts/000-engineering-context.md",
                "--seed-artifact",
                "acceptance_map=/pragma-out/seed-artifacts/001-acceptance-map.json",
            ],
        )

    def test_direct_mode_does_not_add_stop_after_state(self) -> None:
        args = mock.Mock(
            direct=True,
            start_at_state="validation_gate",
            stop_after_state="repo_survey",
        )
        with mock.patch.dict(os.environ, {"PRAGMA_EXTRA_ARGS": "--max-turns 1"}, clear=True):
            self.assertEqual(runner.pragma_extra_args(args), ["--max-turns", "1"])

    def test_prepare_container_seed_artifacts_copies_files(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            root = Path(temp_dir)
            source = root / "engineering-context.md"
            source.write_text("context\n", encoding="utf-8")

            seeds = runner.prepare_container_seed_artifacts(
                root / "out",
                [f"engineering_context={source}"],
            )

            self.assertEqual(
                seeds,
                ["engineering_context=/pragma-out/seed-artifacts/000-engineering-context.md"],
            )
            copied = root / "out" / "seed-artifacts" / "000-engineering-context.md"
            self.assertEqual(copied.read_text(encoding="utf-8"), "context\n")


if __name__ == "__main__":
    unittest.main()

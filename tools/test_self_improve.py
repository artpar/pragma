#!/usr/bin/env python3
"""Scripted-cycle tests for tools/self_improve.py (MM-001 mistake-memory).

The dispatch path (queue load/save, attempt counting, requeue, prompt
 formatting) runs real; only the pragma process itself is replaced at the
 subprocess boundary. The worker-session fixture is derived verbatim from
 the recorded CLK-002 attempt-2 session (f4ff3ebb) — see
 tools/testdata/self_improve/README.md for provenance.
"""

import json
import subprocess
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))

import self_improve as si  # noqa: E402

FIXTURE = (
    Path(__file__).resolve().parent
    / "testdata"
    / "self_improve"
    / "worker-session-clk002-attempt2.jsonl"
)

# CMT-001: verbatim slices of the recorded CLK-002 attempt-1 cap-death
# session (06f603fb, reading 16-22-55 - B.3's headline event: work fully
# done and gate-green at call 78 of 80, killed at the cap with nothing
# committed). Its last recorded test gate passed - the signature the
# orchestrator must scan for on an abnormal exit.
GREEN_GATE_FIXTURE = (
    Path(__file__).resolve().parent
    / "testdata"
    / "self_improve"
    / "worker-session-clk002-greengate.jsonl"
)

# The recorded apply_patch tool errors of CLK-002 attempt 2 (the B.2
# repeated-mistake evidence). A retry prompt must carry them verbatim.
RECORDED_TOOL_ERRORS = [
    'apply_patch verification failed: update hunk for '
    'internal/query/wall_clock_test.go line 62 is malformed: '
    '"\\t\\tvar texts []string"',
    "apply_patch verification failed: internal/query/wall_clock_test.go "
    "produced no content change",
]


def fixture_lines():
    return FIXTURE.read_text().splitlines()


def stage_worker_session(
    sessions_dir, name="worker-session.jsonl", created_at=None, qid=None,
    fixture=None
):
    """Copy the recorded worker session into a sessions dir, stamping the
    header for the scripted dispatch time and this repo's work_dir. With a
    qid, the first user message's queue-item line is rewritten to it
    (dispatch metadata; tool results stay byte-identical)."""
    lines = (fixture or FIXTURE).read_text().splitlines()
    header = json.loads(lines[0])
    header["data"]["created_at"] = created_at or datetime.now(timezone.utc).isoformat()
    header["data"]["work_dir"] = si.REPO
    if qid:
        lines = [l.replace("Queue item CLK-002:", "Queue item %s:" % qid) for l in lines]
    path = Path(sessions_dir) / name
    path.write_text(json.dumps(header) + "\n" + "\n".join(lines[1:]) + "\n")
    return str(path)


def scripted_subprocess_run(*args, **kwargs):
    # The per-cycle `make build` is outside the mechanism under test.
    return subprocess.CompletedProcess(args=args[0] if args else [], returncode=0)


class MistakeMemoryScriptedCycleTest(unittest.TestCase):
    def test_retry_prompt_carries_failed_workers_tool_errors(self):
        with tempfile.TemporaryDirectory() as tmp:
            sessions_dir = Path(tmp) / "sessions"
            sessions_dir.mkdir()
            logs_dir = Path(tmp) / "logs"
            logs_dir.mkdir()
            queue_path = Path(tmp) / "queue.jsonl"
            stage_worker_session(sessions_dir, qid="TEST-001")
            queue_path.write_text(
                json.dumps(
                    {
                        "id": "TEST-001",
                        "title": "scripted mistake-memory cycle",
                        "kind": "loop-defect",
                        "status": "open",
                        "added_at": "2026-09-25T10:00:00",
                        "payload": {"evidence": "x", "deliverables": "y"},
                    }
                )
                + "\n"
            )

            prompts = []

            def scripted_worker(prompt, max_turns, log_path, model, max_cost):
                prompts.append(prompt)
                # the bg worker log carries final text only — never tool errors
                with open(log_path, "w") as fh:
                    fh.write("scripted worker final text; no tool errors here\n")
                return 1, 5.0  # cap death, no commit

            argv = ["self_improve.py", "--cycles", "2", "--queue", str(queue_path)]
            with mock.patch.object(si, "run_pragma", scripted_worker), mock.patch.object(
                si, "git_head", lambda: "deadbeef" * 10
            ), mock.patch.object(si, "last_session_cost", lambda: None), mock.patch.object(
                si, "LOGDIR", str(logs_dir)
            ), mock.patch.object(
                si, "SESSIONS_DIR", str(sessions_dir), create=True
            ), mock.patch.object(
                si.subprocess, "run", scripted_subprocess_run
            ), mock.patch.object(
                sys, "argv", argv
            ):
                self.assertEqual(si.main(), 0)

            self.assertEqual(len(prompts), 2, "two dispatches expected (attempt + retry)")
            retry = prompts[1]
            self.assertIn(
                "Prior attempt mistakes to avoid repeating verbatim", retry
            )
            for err in RECORDED_TOOL_ERRORS:
                # mistakes ride the prompt as a JSON array; the recorded error
                # text must survive that serialization verbatim
                self.assertIn(
                    json.dumps(err)[1:-1],
                    retry,
                    "retry prompt must carry the failed worker's recorded tool "
                    "error verbatim: %r" % err,
                )

            # the requeued item itself carries the mistakes
            items = si.load_queue(str(queue_path))
            item = next(i for i in items if i["id"] == "TEST-001")
            self.assertEqual(item.get("attempts"), 2)
            self.assertEqual(item.get("status"), "open")
            joined = "\n".join(item.get("mistakes", []))
            for err in RECORDED_TOOL_ERRORS:
                self.assertIn(err, joined, "requeued item mistakes: %r" % item.get("mistakes"))


class ToolErrorExtractionTest(unittest.TestCase):
    def test_extracts_recorded_apply_patch_errors_from_session(self):
        extract = getattr(si, "extract_tool_errors", None)
        self.assertTrue(
            callable(extract), "extract_tool_errors mechanism absent (MM-001)"
        )
        mistakes = extract(str(FIXTURE))
        joined = "\n".join(mistakes)
        for err in RECORDED_TOOL_ERRORS:
            self.assertIn(err, joined)
        # exit-code-only Bash errors keep their first failing output line
        self.assertTrue(
            any("Exit code: 1" in m and "--- FAIL:" in m for m in mistakes),
            mistakes,
        )


class WorkerSessionDiscoveryTest(unittest.TestCase):
    def test_finds_worker_marker_session_and_skips_newer_decoys(self):
        find = getattr(si, "find_worker_session", None)
        self.assertTrue(
            callable(find), "find_worker_session mechanism absent (MM-001)"
        )
        with tempfile.TemporaryDirectory() as tmp:
            worker_path = stage_worker_session(
                tmp, created_at="1970-01-01T00:00:00+00:00", qid="TEST-001"
            )
            # newer sessions that are not this item's worker dispatch must
            # be skipped: a critic dispatch and another item's worker
            lines = fixture_lines()
            header = json.loads(lines[0])
            header["data"]["created_at"] = "1971-01-01T00:00:00+00:00"
            header["data"]["work_dir"] = si.REPO
            critic_user = {
                "kind": "message",
                "data": {
                    "role": "user",
                    "content": [
                        {
                            "type": "text",
                            "data": {
                                "text": "You are an independent CRITIC instance in "
                                "pragma's self-improvement loop"
                            },
                        }
                    ],
                },
            }
            (Path(tmp) / "decoy-critic.jsonl").write_text(
                json.dumps(header) + "\n" + json.dumps(critic_user) + "\n"
            )
            other_worker = stage_worker_session(
                tmp,
                name="decoy-other-item.jsonl",
                created_at="1972-01-01T00:00:00+00:00",
                qid="OTHER-999",
            )
            self.assertTrue(other_worker)
            found = find(tmp, 0.0, si.REPO, "TEST-001")
            self.assertEqual(found, worker_path)


class WorkerMarkerTest(unittest.TestCase):
    def test_worker_marker_is_in_dispatch_prompt(self):
        marker = getattr(si, "WORKER_MARKER", None)
        self.assertTrue(
            marker and marker in si.WORKER_PROMPT,
            "find_worker_session must match the real WORKER_PROMPT opening",
        )


class CommitAtGreenRuleTest(unittest.TestCase):
    """CMT-001 deliverable A: the worker prompt must carry the
    commit-at-GREEN rule (added in v1.8/0f758d6 - pinned so it cannot
    silently regress)."""

    def test_worker_prompt_carries_commit_at_green_rule(self):
        self.assertIn(
            "Commit AT GREEN, immediately",
            si.WORKER_PROMPT,
            "the worker prompt must order the commit at GREEN, before "
            "docs/case-record polish",
        )


class GreenGateDetectionTest(unittest.TestCase):
    def test_detects_passing_last_gate_in_recorded_capdeath_session(self):
        detect = getattr(si, "last_test_gate_passed", None)
        self.assertTrue(
            callable(detect), "last_test_gate_passed mechanism absent (CMT-001)"
        )
        # the recorded 06f603fb cap death: gate green, work done, no commit
        self.assertTrue(detect(str(GREEN_GATE_FIXTURE)))

    def test_red_last_gate_is_not_green(self):
        detect = getattr(si, "last_test_gate_passed", None)
        self.assertTrue(
            callable(detect), "last_test_gate_passed mechanism absent (CMT-001)"
        )
        # the recorded CLK-002 attempt-2 mid-work death: its last recorded
        # test gate is the RED assertion (Exit code: 1 / --- FAIL:) - not a
        # near-miss, the generic requeue must stay
        self.assertFalse(detect(str(FIXTURE)))


class NearMissScriptedCycleTest(unittest.TestCase):
    def _run_cycle(self, tmp, fixture, worker_log_text="scripted final text\n"):
        sessions_dir = Path(tmp) / "sessions"
        sessions_dir.mkdir()
        logs_dir = Path(tmp) / "logs"
        logs_dir.mkdir()
        queue_path = Path(tmp) / "queue.jsonl"
        stage_worker_session(
            sessions_dir, qid="TEST-001", fixture=fixture
        )
        queue_path.write_text(
            json.dumps(
                {
                    "id": "DONE-001",
                    "title": "already finished item",
                    "kind": "loop-defect",
                    "status": "done",
                    "added_at": "2026-09-25T10:00:00",
                    "payload": {"evidence": "x", "deliverables": "y"},
                }
            )
            + "\n"
            + json.dumps(
                {
                    "id": "TEST-001",
                    "title": "scripted near-miss cycle",
                    "kind": "loop-defect",
                    "status": "open",
                    "added_at": "2026-09-25T10:00:00",
                    "payload": {"evidence": "x", "deliverables": "y"},
                }
            )
            + "\n"
        )

        prompts = []

        def scripted_worker(prompt, max_turns, log_path, model, max_cost):
            prompts.append(prompt)
            # cap deaths leave the bg worker log empty or final-text only -
            # never the gate evidence (MM-001)
            with open(log_path, "w") as fh:
                fh.write(worker_log_text)
            return 1, 5.0  # cap death, no commit

        argv = ["self_improve.py", "--cycles", "2", "--queue", str(queue_path)]
        with mock.patch.object(si, "run_pragma", scripted_worker), mock.patch.object(
            si, "git_head", lambda: "deadbeef" * 10
        ), mock.patch.object(si, "last_session_cost", lambda: None), mock.patch.object(
            si, "LOGDIR", str(logs_dir)
        ), mock.patch.object(
            si, "SESSIONS_DIR", str(sessions_dir), create=True
        ), mock.patch.object(
            si.subprocess, "run", scripted_subprocess_run
        ), mock.patch.object(
            sys, "argv", argv
        ):
            self.assertEqual(si.main(), 0)
        return prompts, queue_path

    def test_uncommitted_green_death_requeues_with_priority_and_salvage_hint(
        self,
    ):
        hint = getattr(si, "NEAR_MISS_HINT", None)
        self.assertTrue(hint, "NEAR_MISS_HINT absent (CMT-001)")
        with tempfile.TemporaryDirectory() as tmp:
            prompts, queue_path = self._run_cycle(tmp, GREEN_GATE_FIXTURE)
            self.assertEqual(len(prompts), 2, "two dispatches (attempt + retry)")
            retry = prompts[1]
            self.assertIn("Queue item TEST-001:", retry)
            # the salvage hint rides the existing {mistakes} wiring verbatim
            self.assertIn(
                json.dumps(hint)[1:-1],
                retry,
                "retry prompt must tell the next worker the prior attempt "
 "reached GREEN and died uncommitted",
            )
            items = si.load_queue(str(queue_path))
            ids = [i.get("id") for i in items]
            # priority: the near-miss item jumps the queue ahead of even
            # closed items, so the next cycle dispatches the cheap
            # commit-the-existing-work retry first
            self.assertEqual(
                ids, ["TEST-001", "DONE-001"],
                "near-miss requeue must move the item to the front",
            )
            item = next(i for i in items if i["id"] == "TEST-001")
            self.assertEqual(item.get("status"), "open")
            self.assertEqual(item.get("attempts"), 2)
            self.assertIs(item.get("near_miss"), True)
            self.assertIn("uncommitted GREEN", item.get("last_error", ""))
            mistakes = item.get("mistakes", [])
            self.assertEqual(
                mistakes[:1], [hint], "hint must lead the mistake-memory list"
            )

    def test_midwork_death_keeps_plain_requeue(self):
        hint = getattr(si, "NEAR_MISS_HINT", None)
        self.assertTrue(hint, "NEAR_MISS_HINT absent (CMT-001)")
        with tempfile.TemporaryDirectory() as tmp:
            # the CLK-002 attempt-2 fixture: last recorded gate is the RED
            # assertion - a mid-work death, NOT a near-miss
            prompts, queue_path = self._run_cycle(tmp, FIXTURE)
            self.assertEqual(len(prompts), 2)
            self.assertIn("Queue item TEST-001:", prompts[1])
            self.assertNotIn(
                json.dumps(hint)[1:-1],
                prompts[1],
                "a mid-work death must not carry the near-miss hint",
            )
            items = si.load_queue(str(queue_path))
            ids = [i.get("id") for i in items]
            self.assertEqual(
                ids, ["DONE-001", "TEST-001"],
                "a generic death keeps the queue order unchanged",
            )
            item = next(i for i in items if i["id"] == "TEST-001")
            self.assertEqual(item.get("status"), "open")
            self.assertEqual(item.get("attempts"), 2)
            self.assertIsNone(item.get("near_miss"))


if __name__ == "__main__":
    unittest.main()

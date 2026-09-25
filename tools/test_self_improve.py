#!/usr/bin/env python3
"""Scripted-cycle tests for tools/self_improve.py (MM-001 mistake-memory).

The dispatch path (queue load/save, attempt counting, requeue, prompt
 formatting) runs real; only the pragma process itself is replaced at the
 subprocess boundary. The worker-session fixture is derived verbatim from
 the recorded CLK-002 attempt-2 session (f4ff3ebb) — see
 tools/testdata/self_improve/README.md for provenance.
"""

import json
import shutil
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


# ---------------------------------------------------------------------------
# CMT-002: GREEN-to-commit distance on NORMAL exits. Three cycles
# (PACT-001 99409da1: GREEN 51 -> commit 88; MM-001 2d4a25ff: GREEN 53 ->
# commit 69; CMT-001 dd0c0e1e: GREEN 54 -> commit 66) committed 12-37 calls
# after their first GREEN and all exited normally - CMT-001's near-miss
# enforcement (rc != 0 and not committed) never fires on them.
# ---------------------------------------------------------------------------


def _tool_call(call_id, cmd):
    return {
        "type": "tool_call",
        "data": {"id": call_id, "name": "Bash", "input": {"cmd": cmd}},
    }


def _tool_result(call_id, content, is_error=False):
    data = {"tool_call_id": call_id, "content": content}
    if is_error:
        data["is_error"] = True
    return {"type": "tool_result", "data": data}


def _dispatch_user_message(qid):
    text = si.WORKER_PROMPT.format(
        repo=si.REPO, qid=qid, title="scripted commit-delay cycle",
        payload="{}", mistakes='["(first attempt)"]',
    )
    return {
        "kind": "message",
        "data": {
            "id": "msg-dispatch",
            "role": "user",
            "content": [{"type": "text", "data": {"text": text}}],
            "timestamp": "2026-09-25T11:00:00+05:30",
        },
    }


def _recorded_green_gate_parts(n):
    """(tool_call, tool_result) carrying the recorded 06f603fb wall-clock
    gate verbatim (cmd + result content from the greengate fixture); only
    the ids are rewritten so multiple green gates can coexist in one file."""
    green = result = None
    for line in GREEN_GATE_FIXTURE.read_text().splitlines():
        entry = json.loads(line)
        if entry.get("kind") != "message":
            continue
        for part in entry["data"].get("content", []):
            if part["type"] == "tool_call":
                green = part
            elif part["type"] == "tool_result":
                result = part
    call = json.loads(json.dumps(green))
    call["data"]["id"] = "call-c2-green-%d" % n
    res = json.loads(json.dumps(result))
    res["data"]["tool_call_id"] = "call-c2-green-%d" % n
    return call, res


# specs: one assistant call each, in order. kind -> recorded-wire-shaped pair
SPEC_PAIRS = {
    # the RED gate: wire-shaped after the recorded CLK-002 attempt-2 reds
    "red": (
        "python3 -m unittest tools.test_self_improve -v 2>&1 | tail -25",
        "Exit code: 1\n"
        "FAIL: test_green (tools.test_self_improve.GreenGateDetectionTest"
        ".test_detects_passing_last_gate_in_recorded_capdeath_session)\n"
        "AssertionError: False is not true\n"
        "FAILED (failures=1)\n",
        True,
    ),
    # a heredoc script ABOUT test commands whose OUTPUT echoes green
    # markers - the CMT-001 call-32 false-gate class (dd0c0e1e)
    "heredoc": (
        "python3 - <<'EOF'\n"
        "for cmd in ['go test ./internal/query', 'python3 -m unittest tools.test_self_improve']:\n"
        "    print(cmd)\n"
        "print('ok  \\tgithub.com/artpar/pragma/internal/query\\t2.319s')\n"
        "EOF",
        "Exit code: 0\nok  \tgithub.com/artpar/pragma/internal/query\t2.319s\n",
        False,
    ),
    # wire-shaped after the recorded c4fab51 commit (dd0c0e1e call 66)
    "commit": (
        'git add tools/self_improve.py tools/test_self_improve.py; '
        'git commit -m "TEST-001: mechanism"',
        "Exit code: 0\n[main abc1234] TEST-001: mechanism\n"
        " 2 files changed, 30 insertions(+), 1 deletion(-)\n",
        False,
    ),
    # a commit that landed but was flagged is_error by a trailing command -
    # wire-shaped after the recorded 848f609 result (dd0c0e1e call 69)
    "commit_err": (
        'git add docs/failure-cases/x.md; git commit -m "case record"; '
        "git log --oneline | grep -v x",
        "Exit code: 1\n[main 848f609] case record\n"
        " 1 file changed, 137 insertions(+)\n",
        True,
    ),
    "filler": (
        "ls docs/failure-cases",
        "Exit code: 0\ncommit-at-green-near-miss-requeue-2026-09-25.md\n",
        False,
    ),
}


def write_delay_session(path, kinds, qid="TEST-001"):
    """Write a synthetic worker session in the recorded wire format: header
    + dispatch user message + one assistant/user pair per spec kind (green
    specs carry the recorded gate verbatim)."""
    lines = [json.dumps({
        "kind": "header",
        "data": {
            "session_id": "c2-scripted",
            "model": "morph-glm53-744b",
            "provider": "morphllm",
            "work_dir": si.REPO,
            "git_remote": "https://github.com/artpar/pragma.git",
            "created_at": datetime.now(timezone.utc).isoformat(),
            "system": {"blocks": None},
        },
    })]
    lines.append(json.dumps(_dispatch_user_message(qid)))
    greens = 0
    for n, kind in enumerate(kinds, 1):
        if kind == "green":
            greens += 1
            call, result = _recorded_green_gate_parts(greens)
        else:
            cmd, content, is_error = SPEC_PAIRS[kind]
            call = _tool_call("call-c2-%s-%d" % (kind, n), cmd)
            result = _tool_result("call-c2-%s-%d" % (kind, n), content, is_error)
        lines.append(json.dumps({
            "kind": "message",
            "data": {
                "id": "msg-a%d" % n, "role": "assistant",
                "content": [call], "timestamp": "2026-09-25T11:00:0%d+05:30" % (n % 10),
            },
        }))
        lines.append(json.dumps({
            "kind": "message",
            "data": {
                "id": "msg-u%d" % n, "role": "user",
                "content": [result], "timestamp": "2026-09-25T11:00:0%d+05:30" % (n % 10),
            },
        }))
    Path(path).write_text("\n".join(lines) + "\n")


class CommitDelayRuleTest(unittest.TestCase):
    """CMT-002 deliverable: one hard line in the WORKER_PROMPT - the
    existing rule alone did not stop 3/3 cycles polishing before commit 1."""

    def test_worker_prompt_carries_commit_the_moment_gates_are_green(self):
        self.assertIn(
            "commit the moment gates are green",
            si.WORKER_PROMPT,
            "the worker prompt must carry the CMT-002 hard rule",
        )
        self.assertIn(
            "README/polish belong to commit 2",
            si.WORKER_PROMPT,
            "README/polish must be explicitly banished to commit 2",
        )


class GreenToCommitDistanceTest(unittest.TestCase):
    def _write(self, kinds):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        path = str(Path(tmp) / "worker-session.jsonl")
        write_delay_session(path, kinds)
        return path

    def test_distance_on_delay_session(self):
        dist = getattr(si, "green_to_commit_distance", None)
        self.assertTrue(
            callable(dist), "green_to_commit_distance mechanism absent (CMT-002)"
        )
        # RED at 1, GREEN at 2, nine filler calls, commit at 12 (K=10)
        path = self._write(["red", "green"] + ["filler"] * 9 + ["commit"])
        self.assertEqual(si.green_to_commit_distance(path), (2, 12))

    def test_distance_on_prompt_commit_session(self):
        dist = getattr(si, "green_to_commit_distance", None)
        self.assertTrue(
            callable(dist), "green_to_commit_distance mechanism absent (CMT-002)"
        )
        # GREEN at 2, adjacent gate at 3, commit at 4 (K=2) - prompt commit
        path = self._write(["red", "green", "filler", "commit"])
        self.assertEqual(si.green_to_commit_distance(path), (2, 4))

    def test_pre_red_green_is_not_the_anchor(self):
        dist = getattr(si, "green_to_commit_distance", None)
        self.assertTrue(
            callable(dist), "green_to_commit_distance mechanism absent (CMT-002)"
        )
        # a pre-RED baseline green at call 1 must not anchor the distance:
        # the meaningful GREEN is the one after the session's first RED
        path = self._write(["green", "red", "green", "filler", "commit"])
        self.assertEqual(si.green_to_commit_distance(path), (3, 5))

    def test_heredoc_output_is_not_a_gate(self):
        dist = getattr(si, "green_to_commit_distance", None)
        self.assertTrue(
            callable(dist), "green_to_commit_distance mechanism absent (CMT-002)"
        )
        # a heredoc script ABOUT test commands whose output echoes green
        # markers (the recorded dd0c0e1e call-32 false-gate class) must not
        # count as the session's GREEN gate
        path = self._write(["red", "heredoc", "green", "filler", "filler", "filler", "commit"])
        self.assertEqual(si.green_to_commit_distance(path), (3, 7))

    def test_no_red_falls_back_to_first_green(self):
        dist = getattr(si, "green_to_commit_distance", None)
        self.assertTrue(
            callable(dist), "green_to_commit_distance mechanism absent (CMT-002)"
        )
        path = self._write(["green", "filler", "commit"])
        self.assertEqual(si.green_to_commit_distance(path), (1, 3))

    def test_pre_green_commit_and_errored_commit_are_skipped(self):
        dist = getattr(si, "green_to_commit_distance", None)
        self.assertTrue(
            callable(dist), "green_to_commit_distance mechanism absent (CMT-002)"
        )
        # a commit BEFORE the green (e.g. a pre-work docs commit) is not
        # the measured commit; an is_error-flagged commit result (trailing
        # grep, recorded 848f609 class) is not a successful commit either
        path = self._write(["commit", "red", "green", "commit_err", "filler", "commit"])
        self.assertEqual(si.green_to_commit_distance(path), (3, 6))


class CommitDelayScriptedCycleTest(unittest.TestCase):
    def _run_cycle(self, tmp, kinds):
        sessions_dir = Path(tmp) / "sessions"
        sessions_dir.mkdir()
        logs_dir = Path(tmp) / "logs"
        logs_dir.mkdir()
        write_delay_session(sessions_dir / "worker-session.jsonl", kinds)
        queue_path = Path(tmp) / "queue.jsonl"
        queue_path.write_text(
            json.dumps(
                {
                    "id": "TEST-001",
                    "title": "scripted commit-delay cycle",
                    "kind": "loop-defect",
                    "status": "open",
                    "added_at": "2026-09-25T11:00:00",
                    "payload": {"evidence": "x", "deliverables": "y"},
                }
            )
            + "\n"
        )
        prompts = []

        def scripted_session(prompt, max_turns, log_path, model, max_cost):
            prompts.append(prompt)
            with open(log_path, "w") as fh:
                if "CRITIC instance" in prompt:
                    fh.write("===FINDINGS-START===\n[]\n===FINDINGS-END===\n")
                elif "READING ANALYST" in prompt:
                    fh.write("no waste block\n")
                else:
                    fh.write("scripted worker final text\n")
            return 0, 5.0  # NORMAL exit

        heads = ["a" * 40, "b" * 40]  # head changes across the worker: committed
        argv = ["self_improve.py", "--cycles", "1", "--queue", str(queue_path)]
        with mock.patch.object(si, "run_pragma", scripted_session), mock.patch.object(
            si, "git_head", lambda: heads.pop(0)
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

    def test_delayed_commit_on_normal_exit_is_flagged_in_item(self):
        threshold = getattr(si, "COMMIT_DELAY_THRESHOLD", None)
        self.assertIsNotNone(threshold, "COMMIT_DELAY_THRESHOLD absent (CMT-002)")
        with tempfile.TemporaryDirectory() as tmp:
            # GREEN at call 2, first git commit at call 12 - K=10 > threshold
            prompts, queue_path = self._run_cycle(
                tmp, ["red", "green"] + ["filler"] * 9 + ["commit"]
            )
            self.assertEqual(len(prompts), 3, "worker + critic + reader dispatches")
            self.assertIn(si.WORKER_MARKER, prompts[0])
            items = si.load_queue(str(queue_path))
            item = next(i for i in items if i["id"] == "TEST-001")
            # the flag is a note: the cycle still completes normally
            self.assertEqual(item.get("status"), "done")
            delay = item.get("commit_delay")
            self.assertIsNotNone(
                delay,
                "a normal-exit commit %d calls after GREEN must be flagged in "
                "the item (CMT-002: PACT-001/MM-001/CMT-001 all exited normally "
                "and none of them tripped the rc!=0 near-miss branch)" % 10,
            )
            self.assertEqual(delay.get("first_green_call"), 2)
            self.assertEqual(delay.get("commit_call"), 12)
            self.assertEqual(delay.get("calls"), 10)
            self.assertEqual(delay.get("threshold"), si.COMMIT_DELAY_THRESHOLD)

    def test_prompt_commit_on_normal_exit_is_not_flagged(self):
        with tempfile.TemporaryDirectory() as tmp:
            # GREEN at call 2, adjacent gate at 3, commit at 4 - K=2, prompt
            prompts, queue_path = self._run_cycle(
                tmp, ["red", "green", "filler", "commit"]
            )
            items = si.load_queue(str(queue_path))
            item = next(i for i in items if i["id"] == "TEST-001")
            self.assertEqual(item.get("status"), "done")
            self.assertIsNone(
                item.get("commit_delay"),
                "a prompt commit (K=2) must not be flagged",
            )


if __name__ == "__main__":
    unittest.main()

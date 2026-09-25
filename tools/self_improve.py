#!/usr/bin/env python3
"""self_improve.py - pragma self-improvement loop orchestrator (v1).

Each cycle:
  1. Take the top `open` item from the queue (JSONL).
  2. Run a WORKER pragma session (fresh context, headless, turn-capped)
     that verifies/fixes the item per the repo methodology and commits.
  3. If the worker produced a new commit, run a CRITIC pragma session
     (another fresh instance) that audits the commit for what the worker
     failed to think of, ending with a machine-readable findings block.
  4. Append the critic's findings to the queue as new items. Rinse.

Budget: cycles are the cap (default 1); each session is turn-capped.
Costs roughly $1-4 per cycle on the standing GLM-5.3 route. Never pushes.

Usage: tools/self_improve.py [--cycles N] [--queue FILE] [--critic-model M]
"""

import argparse
import json
import os
import re
import subprocess
import sys
import time
from datetime import datetime

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN = os.path.join(REPO, "bin", "pragma")
LOGDIR = os.path.join(REPO, ".self-improve")
# MM-001 mistake-memory: the failed worker's own tool errors live in its
# durable session transcript, not in the bg worker log (which only ever
# receives the final text - cap deaths leave it empty).
SESSIONS_DIR = os.path.join(os.path.expanduser("~"), ".pragma", "sessions")
WORKER_MARKER = "You are a WORKER instance in pragma's self-improvement loop"
MAX_MISTAKES = 12

# CMT-001: test-gate recognition for "uncommitted GREEN" detection.
# Recorded cap-deaths with finished work (MASTER-BLINDSPOTS B.3) ended on
# a passing gate whose tool result looks like
# "Exit code: 0\nok  \tgithub.com/...\t2.319s\n" (go) or an "OK" line
# (python unittest) - verified in sessions 06f603fb, f4ff3ebb and
# de57a18f. dbab3dbf is the documented miss: its final action was a
# full-suite sweep red from pre-existing failures (case record CMT-001).
TEST_CMD_RE = re.compile(r"\b(?:go test|python3? -m unittest)\b")
GREEN_GATE_RE = re.compile(r"^ok\s|--- PASS:|^OK$", re.MULTILINE)
RED_GATE_RE = re.compile(r"^(?:FAIL|--- FAIL:)", re.MULTILINE)

NEAR_MISS_HINT = (
    "NEAR-MISS - uncommitted GREEN: the prior attempt's last recorded test "
    "gate PASSED but it died before committing. The finished work is likely "
    "sitting UNCOMMITTED in the shared tree - re-run its gates FIRST; if "
    "GREEN, commit IMMEDIATELY before any other work (doctrine rule 2: the "
    "deliverable is the commit, not the working tree)."
)

# CMT-002: commit-delay check on NORMAL exits. Three cycles polished
# post-GREEN before commit 1 (PACT-001 99409da1: GREEN 51 -> commit 88,
# 37 calls, turn-budget warning one call after the commit; MM-001
# 2d4a25ff: GREEN 53 -> commit 69; CMT-001 dd0c0e1e: GREEN 54 -> commit
# 66) and all exited normally, so the rc!=0 near-miss branch never fired.
# A worker that spends more than this many calls between its first GREEN
# gate and its first commit is running rule-2's die-uncommitted risk; the
# cycle is flagged (not failed). Legitimate GREEN -> adjacent tests ->
# gofmt -> status -> commit fits within 6 calls; the recorded violations
# are 12/16/37 (calibration in the CMT-001 case record revision note).
COMMIT_DELAY_THRESHOLD = 6

# v1.5: cap-deaths burned ~$32 (37% of today spend) on 4 dead attempts;
# successful big builds used 60-90 turns - give headroom to 110.
WORKER_MAX_TURNS = 110
# Cycle-1 dogfood: a thorough critic (dual-worktree gate re-runs) needs
# more than 40 turns; it died at the cap mid-audit without concluding.
CRITIC_MAX_TURNS = 80
# Hard spend ceilings (BUILD-budget-cap): each session aborts with a
# distinct spend-ceiling error rather than exceeding the envelope.
WORKER_MAX_COST = 15.0
CRITIC_MAX_COST = 8.0

WORKER_PROMPT = """You are a WORKER instance in pragma's self-improvement loop, working in {repo}.
Read agent.md FIRST - the evidence-first methodology is mandatory (failure case
before fix, one mechanism per change, RED->GREEN gates, no unverifiable claims).

Queue item {qid}: {title}
Payload/instructions:
{payload}

Read docs/self-improvement-doctrine.md FIRST - the ten rules are mandatory.
Prior attempt mistakes to avoid repeating verbatim (mistake-memory):
{mistakes}

Rules:
- Verify every claim in the payload against the actual code before acting;
  label refuted claims and change nothing for them (document the refutation).
- For each confirmed defect: write the failing assertion first (RED), make
  one mechanism-level change, re-run (GREEN), run the relevant package tests,
  then gofmt.
- Amend the related case record under docs/failure-cases/ with a revision
  note (what was verified, refuted, changed).
- Commit AT GREEN, immediately - before docs/case-record polish. The
  deliverable is the commit; polish can follow in a second commit.
- HARD RULE: commit the moment gates are green - README/polish belong to commit 2,
  and so does any extra re-verification; nothing happens between the first
  GREEN and commit 1.
- Never chain grep/test with && (silent breakage burned 5+ sessions);
  use echo markers between steps and check exit codes.
- No git stash in the shared tree - if you need isolation, use a
  detached worktree.
- Stop when the deliverable is delivered - no citation goose-chases.
- Commit locally with a case-referencing message. NEVER push.
- NEVER edit docs/self-improvement-queue.jsonl - the orchestrator owns the
  queue file. Report status changes only in your final message block.
  Editing the queue corrupts the loop (observed: items silently deleted).
- End your final message with exactly this block (single-line JSON):
===WORKER-REPORT-START===
{{"verified": ["..."], "refuted": ["..."], "tests": "...", "commit": "<hash or none>"}}
===WORKER-REPORT-END===
"""

READER_PROMPT = """You are a READING ANALYST (doctrine rule 8) for queue item {qid}.
Read the worker session output log at {session_log} - render EVERY API call
individually with python (call number, time, tokens, stop, THINKING first
150 chars, each TOOL + first 120 chars of args) and read them all.
Then audit intent: wasted calls, redundant re-reads, confusion loops,
mistakes that repeat prior attempts, post-deliverable tails, methodology
violations (commit-at-GREEN, no &&-chains, no stash, stop-at-deliverable).
Write docs/analyses/reading/{qid}-reader.md with the per-call narrative
and findings. End with exactly this block (single-line JSON array):
===WASTE-START===
[{{"call": 12, "issue": "...", "cost": "low|medium|high"}}]
===WASTE-END===
"""

CRITIC_PROMPT = """You are an independent CRITIC instance in pragma's self-improvement loop,
auditing a commit made by a WORKER instance you have no shared context with.
Work in {repo}. Do NOT modify, fix, or commit anything - audit only.

Audit commit {commit} (run `git -C {repo} show {commit}` and read the full diff,
plus the case record it references under docs/failure-cases/).

Answer, with file/line evidence you actually read:
1. Did the worker actually fix what the item claimed, or paper over it?
2. Can each new/modified assertion genuinely fail (empirical), or is any
   tautological/dead-code (definitional)?
3. What did the worker fail to think of (new defects, regressions, missing
   adjacent checks)?

End with exactly this block (single-line JSON array of findings, empty if
the commit survives audit cleanly):
===FINDINGS-START===
[{{"id": "C-1", "severity": "high|medium|low", "confidence": "confirmed|plausible", "claim": "...", "evidence": "file:line ..."}}]
===FINDINGS-END===
"""


def run_pragma(prompt, max_turns, log_path, model, max_cost):
    """Run a headless pragma session; returns (exit_code, wall_seconds)."""
    env = dict(os.environ)
    env["PRAGMA_BG_SESSION_LOG"] = log_path
    started = time.time()
    proc = subprocess.run(
        [BIN, "--provider", "morphllm", "--model", model,
         "--loop", "provider-tools", "--max-turns", str(max_turns),
         "--max-cost", str(max_cost),
         "--record", "-p", prompt],
        cwd=REPO, env=env, capture_output=True, text=True,
    )
    return proc.returncode, time.time() - started


def load_queue(path):
    items = []
    if os.path.exists(path):
        with open(path) as fh:
            for line in fh:
                line = line.strip()
                if line:
                    try:
                        items.append(json.loads(line))
                    except json.JSONDecodeError:
                        continue
    return items


def save_queue(path, items):
    # v1.7: merge external writes (supervisor inserts, other tools) just
    # before saving - full-file rewrites are the deletion vector.
    try:
        disk = load_queue(path)
        known = {i["id"] for i in items}
        for d in disk:
            if d.get("id") not in known and d.get("status") == "open":
                items.append(d)
    except Exception:
        pass
    with open(path, "w") as fh:
        for item in items:
            fh.write(json.dumps(item) + "\n")


def git_head():
    out = subprocess.run(["git", "-C", REPO, "rev-parse", "HEAD"],
                         capture_output=True, text=True)
    return out.stdout.strip()


def last_session_cost():
    """Best-effort per-session USD cost from the newest session file."""
    import glob
    try:
        files = sorted(glob.glob(os.path.expanduser(
            "~/.pragma/sessions/*.jsonl")), key=os.path.getmtime)
        cost = None
        with open(files[-1]) as fh:
            for line in fh:
                if '"cost_usd"' not in line:
                    continue
                try:
                    d = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if d.get("kind") == "metadata":
                    cost = d.get("data", {}).get("cost_usd")
        return cost
    except Exception:
        return None


def extract_block(log_text, start, end):
    m = re.search(re.escape(start) + r"\s*\n(.*?)" + re.escape(end),
                  log_text, re.DOTALL)
    return m.group(1).strip() if m else None


def summarize_tool_error(content, limit=240):
    """Head of an errored tool result; for `Exit code: N` Bash results the
    first following output line (e.g. the `--- FAIL:` line) is appended."""
    lines = content.splitlines()
    head = next((l.strip() for l in lines if l.strip()), "")
    if not head:
        return ""
    if re.fullmatch(r"Exit code: \d+", head):
        nxt = next((l.strip() for l in lines[1:] if l.strip()), "")
        if nxt:
            head = head + " - " + nxt
    return head[:limit]


def find_worker_session(sessions_dir, started_epoch, repo, qid):
    """Newest session transcript dispatched from this orchestrator run:
    header work_dir matches the repo, created_at is at/after the dispatch
    timestamp, and the first user message carries the WORKER marker (so
    item's Queue-item line (so critic/reader/operator sessions and workers
    dispatched for a different queue item are never mistaken for it)."""
    best_path, best_ts = None, None
    try:
        names = os.listdir(sessions_dir)
    except OSError:
        return None
    for name in names:
        if not name.endswith(".jsonl"):
            continue
        path = os.path.join(sessions_dir, name)
        try:
            with open(path) as fh:
                header = json.loads(fh.readline())
            if header.get("kind") != "header":
                continue
            data = header.get("data", {})
            if data.get("work_dir") != repo:
                continue
            ts = datetime.fromisoformat(data.get("created_at", "")).timestamp()
        except (ValueError, OSError, json.JSONDecodeError):
            continue
        if ts < started_epoch - 2 or (best_ts is not None and ts <= best_ts):
            continue
        try:
            with open(path) as fh:
                for line in fh:
                    entry = json.loads(line)
                    if entry.get("kind") != "message":
                        continue
                    edata = entry.get("data", {})
                    if edata.get("role") != "user":
                        continue
                    text = "".join(
                        p.get("data", {}).get("text", "")
                        for p in edata.get("content", [])
                        if p.get("type") == "text"
                    )
                    if WORKER_MARKER in text and ("Queue item %s:" % qid) in text:
                        best_path, best_ts = path, ts
                    break
        except (OSError, json.JSONDecodeError):
            continue
    return best_path


def extract_tool_errors(session_path, limit=8):
    """The session's errored tool results (pragma's own is_error
    classification) as mistake-memory entries, in order, deduped."""
    calls = {}
    errors = []
    with open(session_path) as fh:
        for line in fh:
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue
            if entry.get("kind") != "message":
                continue
            data = entry.get("data", {})
            for part in data.get("content", []):
                pdata = part.get("data", {})
                if part.get("type") == "tool_call":
                    cmd = pdata.get("input", {}).get("cmd")
                    brief = " ".join(cmd.split())[:80] if isinstance(cmd, str) else ""
                    calls[pdata.get("id")] = (
                        pdata.get("name", "tool") + ((" (%s)" % brief) if brief else "")
                    )
                elif part.get("type") == "tool_result" and pdata.get("is_error"):
                    summary = summarize_tool_error(pdata.get("content", ""))
                    if not summary:
                        continue
                    entry_text = "%s: %s" % (
                        calls.get(pdata.get("tool_call_id"), "tool"), summary
                    )
                    if entry_text not in errors:
                        errors.append(entry_text)
    return errors[-limit:]


def last_test_gate_passed(session_path):
    """True iff the session's LAST test-run tool result was a passing gate:
    the paired tool_call ran a test command (go test / python unittest),
    the result is not an is_error, its content carries a green marker
    (a `ok <pkg>` line, `--- PASS:`, or a lone `OK`) and no FAIL marker.
    This is the recorded signature of the B.3 uncommitted-GREEN
    cap-deaths (06f603fb, f4ff3ebb, de57a18f): work finished with the
    gate green, session killed at the turn cap, nothing committed.
    Returns None when the session records no test gate."""
    calls = {}
    last_green = None
    with open(session_path) as fh:
        for line in fh:
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue
            if entry.get("kind") != "message":
                continue
            data = entry.get("data", {})
            for part in data.get("content", []):
                pdata = part.get("data", {})
                if part.get("type") == "tool_call":
                    cmd = pdata.get("input", {}).get("cmd")
                    calls[pdata.get("id")] = cmd if isinstance(cmd, str) else ""
                elif part.get("type") == "tool_result":
                    cmd = calls.get(pdata.get("tool_call_id"), "")
                    if not TEST_CMD_RE.search(cmd):
                        continue
                    content = pdata.get("content", "")
                    last_green = (
                        not pdata.get("is_error")
                        and isinstance(content, str)
                        and bool(GREEN_GATE_RE.search(content))
                        and not RED_GATE_RE.search(content)
                    )
    return last_green


def green_to_commit_distance(session_path):
    """CMT-002: (first_green_call, commit_call) - 1-based assistant-call
    indices of the session's meaningful GREEN gate and the first commit
    after it; either may be None when the transcript lacks it.

    The GREEN gate is the first PASSING test gate that follows the
    session's first RED gate (the methodology's RED->GREEN transition) -
    sessions with no RED fall back to the first passing gate overall. A
    pre-RED baseline green (workers verifying payload claims run suites
    before writing the RED test) must not anchor the distance. Test
    commands issued inside heredoc scripts are not gates: their OUTPUT
    echoes recorded markers (the dd0c0e1e call-32 false-gate class).
    The commit is the first successful (not is_error) `git commit`
    tool result AFTER that green; commits before it don't count.
    """
    calls = {}  # tool_call_id -> (call_idx, is_test, is_commit)
    saw_red = False
    first_green = None
    pre_red_green = None  # first green seen before any red (fallback)
    commit_after_pre_green = None
    commit_call = None
    idx = 0
    with open(session_path) as fh:
        for line in fh:
            try:
                entry = json.loads(line)
            except json.JSONDecodeError:
                continue
            if entry.get("kind") != "message":
                continue
            data = entry.get("data", {})
            if data.get("role") == "assistant":
                idx += 1
                for part in data.get("content", []):
                    pdata = part.get("data", {})
                    if part.get("type") != "tool_call":
                        continue
                    cmd = pdata.get("input", {}).get("cmd")
                    if not isinstance(cmd, str) or "<<" in cmd:
                        continue  # heredoc scripts are not test gates
                    calls[pdata.get("id")] = (
                        idx,
                        bool(TEST_CMD_RE.search(cmd)),
                        bool(re.search(r"\bgit\s+commit\b", cmd)),
                    )
            elif data.get("role") == "user":
                for part in data.get("content", []):
                    pdata = part.get("data", {})
                    if part.get("type") != "tool_result":
                        continue
                    info = calls.get(pdata.get("tool_call_id"))
                    if info is None:
                        continue
                    cidx, is_test, is_commit = info
                    content = pdata.get("content", "")
                    if is_test:
                        green = (
                            not pdata.get("is_error")
                            and isinstance(content, str)
                            and bool(GREEN_GATE_RE.search(content))
                            and not RED_GATE_RE.search(content)
                        )
                        if (isinstance(content, str)
                                and RED_GATE_RE.search(content)):
                            # FAIL markers count even when a `| tail` pipeline
                            # masked the exit code into is_error=false
                            saw_red = True
                        elif green and first_green is None:
                            if saw_red:
                                first_green = cidx
                            elif pre_red_green is None:
                                pre_red_green = cidx
                    if is_commit and not pdata.get("is_error"):
                        # commits at/before the green (pre-work docs
                        # commits) are not the measured commit
                        if first_green is not None:
                            if cidx > first_green and commit_call is None:
                                commit_call = cidx
                        elif (pre_red_green is not None
                              and cidx > pre_red_green
                              and commit_after_pre_green is None):
                            # held until the scan resolves whether any red
                            # follows (a later red moves the anchor to the
                            # post-red GREEN and discards this commit)
                            commit_after_pre_green = cidx
    if first_green is None and not saw_red:
        # no RED in the session: the first green anchors, and its first
        # following commit is the measured commit
        first_green = pre_red_green
        commit_call = commit_after_pre_green
    if first_green is None:
        return None, None  # no anchor: distance undefined
    return first_green, commit_call


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--cycles", type=int, default=1)
    ap.add_argument("--queue", default=os.path.join(REPO, "docs", "self-improvement-queue.jsonl"))
    ap.add_argument("--critic-model", default="morph-glm53-744b")
    ap.add_argument("--worker-model", default="morph-glm53-744b")
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    os.makedirs(LOGDIR, exist_ok=True)
    queue = load_queue(args.queue)
    stamp = time.strftime("%Y%m%d-%H%M%S")

    for cycle in range(1, args.cycles + 1):
        # v1.4: reload the queue each cycle so externally-inserted items
        # (operator directives, human notes) are never clobbered by this
        # process's in-memory copy when it saves.
        # v1.6: compile fresh every cycle - worker fixes become the next
        # cycle's runtime (no stale-binary generations).
        subprocess.run(["make", "build"], cwd=REPO, capture_output=True)
        queue = load_queue(args.queue)
        idx = next((i for i, it in enumerate(queue) if it.get("status") == "open"), None)
        if idx is None:
            print("cycle %d: queue empty - done" % cycle)
            break
        item = queue[idx]
        item["status"] = "in_progress"
        save_queue(args.queue, queue)
        print("cycle %d: %s - %s" % (cycle, item["id"], item["title"]))

        if args.dry_run:
            print("  dry-run: skipping execution")
            item["status"] = "open"
            save_queue(args.queue, queue)
            continue

        head_before = git_head()
        # Audit-only items (payload carries a commit): skip the worker.
        audit_only = "commit" in (item.get("payload") or {})
        worker_log = os.path.join(LOGDIR, "%s-c%d-worker.log" % (stamp, cycle))
        worker_started = time.time()
        if audit_only:
            rc, secs = 0, 0.0
            head_before = item["payload"]["commit"]
            print("  audit-only item: skipping worker")
        else:
            rc, secs = run_pragma(
                WORKER_PROMPT.format(repo=REPO, qid=item["id"], title=item["title"],
                                     payload=json.dumps(item.get("payload", {}), indent=2),
                                     mistakes=json.dumps(item.get("mistakes", ["(first attempt)"]))),
                WORKER_MAX_TURNS, worker_log, args.worker_model, WORKER_MAX_COST)
            print("  worker exit=%d wall=%.0fs log=%s cost=$%s" % (
                rc, secs, worker_log, last_session_cost()))

        head_after = git_head()
        if audit_only:
            head_after = item["payload"]["commit"]
            committed = True
        else:
            committed = head_after != head_before
        report = None
        if os.path.exists(worker_log):
            with open(worker_log) as fh:
                report = extract_block(fh.read(), "===WORKER-REPORT-START===", "===WORKER-REPORT-END===")

        if rc != 0 and not committed:
            # v1.3 (cycle-5 dogfood): a worker that dies at its turn cap
            # mid-work has not disproven the item — requeue with an
            # attempt count instead of permanently failing it. Bundles
		        # that exceed one capped session should be split in the queue.
            attempts = item.get("attempts", 0) + 1
            item["attempts"] = attempts
            item["last_error"] = "worker exit %d (attempt %d)" % (rc, attempts)
            # MM-001 mistake-memory: the failed worker's own tool errors
            # (is_error tool results from its session transcript) feed the
            # retry prompt verbatim. The bg worker log only ever carries
            # final text — grepping it never fired on a real cap death and
            # matched only narrative prose (see case record MM-001).
            # CMT-001: an "uncommitted GREEN" is an abnormal exit, not a
            # scratch failure - if the worker's last recorded test gate
            # PASSED, the finished fix is sitting uncommitted in the shared
            # tree and the retry is a cheap verify-and-commit, not a redo.
            near_miss = False
            try:
                sess = find_worker_session(SESSIONS_DIR, worker_started, REPO, item["id"])
                if sess:
                    item_mistakes = item.setdefault("mistakes", [])
                    for err in extract_tool_errors(sess):
                        if err not in item_mistakes:
                            item_mistakes.append(err)
                    near_miss = bool(last_test_gate_passed(sess))
            except Exception as exc:
                print("  mistake-memory extraction failed: %r" % exc)
            if near_miss:
                item["near_miss"] = True
                item["last_error"] = (
                    "worker exit %d (attempt %d, uncommitted GREEN near-miss)"
                    % (rc, attempts)
                )
                item_mistakes = item.setdefault("mistakes", [])
                if NEAR_MISS_HINT not in item_mistakes:
                    item_mistakes.insert(0, NEAR_MISS_HINT)
            if "mistakes" in item and len(item["mistakes"]) > MAX_MISTAKES:
                # keep the near-miss hint (index 0) plus the newest mistakes
                kept = item["mistakes"]
                item["mistakes"] = kept[:1] + kept[1 - MAX_MISTAKES:]
            if attempts >= 3:
                item["status"] = "needs_attention"
            else:
                item["status"] = "open"
            if near_miss:
                # requeue with priority: the next cycle must dispatch the
                # cheap commit-the-existing-work retry ahead of every other
                # open item, instead of the plain generic requeue.
                queue.insert(0, queue.pop(idx))
                print(
                    "  worker died after a green gate without commit (attempt %d)"
                    " - UNCOMMITTED GREEN near-miss - requeued with priority"
                    % attempts
                )
            else:
                print("  worker died without commit (attempt %d) - requeued" % attempts)
            save_queue(args.queue, queue)
            continue

        # CMT-002: normal-exit commit-delay check. Best-effort GREEN-to-
        # commit distance from the worker's durable session transcript
        # (the bg worker log carries final text only - MM-001, wire-proven
        # - so it can never carry per-call gate evidence). Flag, not fail.
        if committed and not audit_only:
            try:
                sess = find_worker_session(
                    SESSIONS_DIR, worker_started, REPO, item["id"])
                if sess:
                    first_green, commit_call = green_to_commit_distance(sess)
                    if first_green is not None and commit_call is not None:
                        delay = commit_call - first_green
                        if delay > COMMIT_DELAY_THRESHOLD:
                            item["commit_delay"] = {
                                "first_green_call": first_green,
                                "commit_call": commit_call,
                                "calls": delay,
                                "threshold": COMMIT_DELAY_THRESHOLD,
                                "note": (
                                    "commit landed %d calls after the first "
                                    "GREEN gate - README/polish/re-verification "
                                    "between GREEN and commit 1 (CMT-002, "
                                    "doctrine rule 2: die-uncommitted risk)"
                                    % delay
                                ),
                            }
                            print(
                                "  commit-delay: GREEN at call %d, commit at "
                                "call %d (%d calls apart, threshold %d) - "
                                "noted in item (CMT-002)"
                                % (first_green, commit_call, delay,
                                   COMMIT_DELAY_THRESHOLD)
                            )
            except Exception as exc:
                print("  commit-delay check failed: %r" % exc)

        item["status"] = "worker_done" if committed else "no_change"
        item["worker_report"] = report or "(no report block)"
        if committed:
            item["commit"] = head_after
            critic_log = os.path.join(LOGDIR, "%s-c%d-critic.log" % (stamp, cycle))
            crc, csecs = run_pragma(
                CRITIC_PROMPT.format(repo=REPO, commit=head_after),
                CRITIC_MAX_TURNS, critic_log, args.critic_model, CRITIC_MAX_COST)
            print("  critic exit=%d wall=%.0fs log=%s cost=$%s" % (
                crc, csecs, critic_log, last_session_cost()))
            findings_raw = None
            if os.path.exists(critic_log):
                with open(critic_log) as fh:
                    findings_raw = extract_block(fh.read(), "===FINDINGS-START===", "===FINDINGS-END===")
            findings = []
            if crc != 0 or findings_raw is None:
                # Cycle-1 dogfood: a critic that dies (turn cap, truncation)
                # has NOT audited clean — never treat a missing findings
                # block as an empty findings list.
                item["status"] = "audit_incomplete"
                item["critic_exit"] = crc
                queue.append({
                    "id": item["id"] + "-reaudit",
                    "title": "re-audit commit %s (prior critic exit %d)" % (head_after[:9], crc),
                    "kind": "audit-only",
                    "payload": {"commit": head_after},
                    "status": "open",
                    "added_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
                    "source_cycle": item["id"],
                })
                print("  critic died without findings (exit %d) - re-audit queued" % crc)
            elif findings_raw:
                try:
                    findings = json.loads(findings_raw)
                except json.JSONDecodeError:
                    print("  critic findings block not valid JSON - recording raw")
                    item["critic_findings_raw"] = findings_raw
            item["critic_findings"] = findings
            item["status"] = "done" if not findings else "needs_followup"
            for n, f in enumerate(findings, 1):
                queue.append({
                    "id": "%s.F%d" % (item["id"], n),
                    "title": ("critic finding: %s" % f.get("claim", "?"))[:140],
                    "kind": "audit-finding",
                    "severity": f.get("severity", "medium"),
                    "confidence": f.get("confidence", "plausible"),
                    "payload": f,
                    "status": "open",
                    "added_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
                    "source_cycle": item["id"],
                })
            print("  critic findings: %d - re-queued" % len(findings))
            # v1.8 reader phase: fresh instance reads the worker session
            # call-by-call for intent-level waste (doctrine rule 8).
            rlog = os.path.join(LOGDIR, "%s-c%d-reader.log" % (stamp, cycle))
            rrc, rsecs = run_pragma(READER_PROMPT.format(
                repo=REPO, session_log=worker_log, qid=item["id"]),
                60, rlog, args.critic_model, CRITIC_MAX_COST)
            waste = None
            if os.path.exists(rlog):
                with open(rlog) as fh:
                    m = re.search(r"===WASTE-START===\s*\n(.*?)===WASTE-END===", fh.read(), re.DOTALL)
                    if m:
                        try: waste = json.loads(m.group(1).strip())
                        except json.JSONDecodeError: waste = None
            if waste:
                item["reader_waste"] = waste
                wlist = waste if isinstance(waste, list) else [waste]
                for n, w in enumerate(wlist, 1):
                    queue.append({
                        "id": "%s.R%d" % (item["id"], n),
                        "title": ("reader waste: %s" % str(w))[:140],
                        "kind": "reading-finding",
                        "payload": w if isinstance(w, dict) else {"finding": str(w)},
                        "status": "open",
                        "added_at": time.strftime("%Y-%m-%dT%H:%M:%S"),
                        "source_cycle": item["id"],
                    })
                print("  reader waste findings: %d - re-queued" % len(wlist))
        save_queue(args.queue, queue)
    return 0


if __name__ == "__main__":
    sys.exit(main())

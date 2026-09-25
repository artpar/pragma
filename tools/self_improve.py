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

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN = os.path.join(REPO, "bin", "pragma")
LOGDIR = os.path.join(REPO, ".self-improve")

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
            # mistake-memory: tail failures feed the retry prompt
            try:
                wtext = open(worker_log).read() if os.path.exists(worker_log) else ""
                errs = re.findall(r"apply_patch verification failed[^.\n]{0,120}|hunk.*?did not match[^\n]{0,80}|exit status [1-9][^\n]{0,60}", wtext)[-6:]
                if errs:
                    item["mistakes"] = item.get("mistakes", []) + errs[-3:]
            except Exception:
                pass
            if attempts >= 3:
                item["status"] = "needs_attention"
            else:
                item["status"] = "open"
            save_queue(args.queue, queue)
            print("  worker died without commit (attempt %d) - requeued" % attempts)
            continue

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

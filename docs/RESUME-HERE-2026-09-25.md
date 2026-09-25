# Resume note — written 2026-09-25 12:25, before session close (operator stopping all agents)

State: everything stopped by operator order. All durable work is committed AND pushed
(HEAD 43b3ec5 on origin/main). This session is the supervisor; a fresh session resumes from files.

## What is where (nothing lives only in the old conversation)
- To-do list: docs/self-improvement-queue.jsonl — 30 items, 28 open. Top: CMT-002 (was mid-fix), SHL-001, ISO-001, then RES-001 (the ~30%-cost-saver: prompt-prefix caching).
- Rulebook (the "meta ways" — READ FIRST): docs/self-improvement-doctrine.md
- Bug case records: docs/failure-cases/*.md (one per fix, includes today's 4)
- Blindspot reports + per-call readings: docs/analyses/ and docs/analyses/reading/
- The loop tool itself: tools/self_improve.py (v1.8: worker -> critic -> reader)
- Skill file a fresh pragma auto-loads: ~/.pragma/skills/harness-self-evolution/SKILL.md

## On the bench (decide before restarting the loop)
- tools/self_improve.py + tools/test_self_improve.py are DIRTY: the CMT-002 worker's
  half-done fix (GREEN-to-commit distance check), killed mid-work ~25 min in, never
  committed. Options: (a) git restore tools/ and let the queue item redo it cleanly,
  (b) review/salvage first. Recommend (a) — the queue item carries the full spec.

## Relaunch commands (from repo root, when the operator says go)
1. The loop:      nohup python3 -u tools/self_improve.py --cycles 40 > .self-improve/orchestrator-resume.log 2>&1 &
2. Watcher:       ./bin/pragma-watch --daemon
3. Fresh pragma (the new supervisor): ./bin/pragma --provider morphllm --model morph-glm53-744b --loop provider-tools
   First prompt for it: "Read docs/self-improvement-doctrine.md, docs/self-improvement-queue.jsonl and this file, then resume driving the loop."

## Traps the next supervisor must know (learned the hard way today)
- The Bash tool is FAIL-FAST: a failing command aborts the rest of the script silently.
  Guard probes with `|| true` or run checks one per call. (Reader-verified live; hit the old supervisor 2x today.)
- Poll with sleep <= 100s (tool timeout ~2 min kills sleep 235 with exit -9).
- v1.7 queue rule: only the orchestrator writes the queue; supervisor inserts are merged
  at save (appended at END — to jump the line, restart the orchestrator; see CMT-002 era notes).
- INST-001 is fixed and proven: `make build` no longer dirties the tree. If >3 files are
  ever dirty again after a build, that regressed — check immediately.
- Worker costs run ~$6-8 per item, critics ~$1.5-2.5, readers ~$1. Budget is the provider key.

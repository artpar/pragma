# PACT-001 reader analysis (doctrine rule 8 — every call read)

Worker: session `99409da1-7c16-4191-a0a9-50f73905bef2` (morph-glm53-744b @ morphllm),
PACT-001 "apply_patch preflight: validate hunk headers/counts before applying".
Log: `.self-improve/20260925-102627-c1-worker.log` (final report only; raw calls in
`~/.pragma/sessions/99409da1-*.jsonl`). Rendered with a python per-call renderer:
every assistant message = one API call; tokens = per-call delta of cumulative
`token_usage` from the metadata records bracketing the call; stop inferred from
content (tool_use / end_turn).

**Session totals.** 93 API calls, 105 tool invocations (92 Bash, 13 apply_patch),
10:26:31 → 10:42:11 (15m40s), 6,347,208 in / 64,622 out tokens, $8.22. Commits:
`89522be` (fix+tests) and `193383a` (case record) — both verified in git.

**Phase map** (calls, span, in/out tokens):

| Phase | Calls | Span | in / out | What |
|---|---|---|---|---|
| A evidence+code recon | 1-17 | 10:26:31-10:27:38 | 440k / 5.2k | mandated reads, blindspot B.1, applypatch.go, tests |
| B raw-input hunt | 18-33 | 10:27:41-10:29:11 | 710k / 6.4k | find wire-recorded failing patches |
| C RED setup+run | 34-44 | 10:30:03-10:32:44 | 704k / 20k | scratch-test RED on synthetic fixture |
| D fix+GREEN+adjacent | 45-53 | 10:33:20-10:34:29 | 707k / 8.1k | 3-patch fix, GREEN at call 51, package gates |
| E pre-existing-failure + hygiene | 54-65 | 10:34:45-10:36:26 | 996k / 1.7k | cmd/pragma triage, baseline worktree, gofmt/vet |
| F e2e grep + re-runs | 66-69 | 10:36:34-10:36:52 | 341k / 0.4k | adjacent-test scan + 2 redundant re-runs |
| G verbatim-patch upgrade | 70-83 | 10:37:18-10:40:16 | 1,374k / 17.8k | rewrite test w/ wire patch, 5 failed apply_patch, RED-at-parent |
| H gates+commit+case record | 84-92 | 10:40:23-10:41:54 | 964k / 3.7k | gates, commit 89522be, case record 193383a |
| I final report | 93 | 10:42:11 | 110k / 1.3k | end_turn deliverable report |

## Per-call narrative (all 93)

**Phase A — evidence and code recon (calls 1-17).** 1 (10:26:31, in1766/71,
tool_use): reads agent.md + doctrine as mandated; `ls -la && cat agent.md` is an
`&&` slip (rule 3 letter). 2 (13730/319): rest of agent.md; locate apply_patch
code. 3 (18214/187): file sizes; exact-case `grep "B.1"` misses (exit 1). 4
(18780/71): case-insensitive retry finds B.1/B.2 lines. 5 (18931/41): reads the
full blindspot report — friction classes enumerated. 6-9 (20948→27193): reads
applypatch.go sequentially 1-700 (one pass, no re-reads); call 9's companion grep
uses a bad glob (`internal/tools/*.go`, exit 1). 10 (29157/130): reads the whole
206-line test file; finds tool entry point (miniswe_loop.go:1785). 11-13
(31216/32622): inventory + greps across reading analyses, some wrong filenames
(exit 2). 14 (35199/1001): greps `2026-09-24T18-12-25.md` (exists) and
`2026-09-24T18-43-40.md` (does not exist — guessed filename format, exit 2);
extracts the +drop/15-call-fight detail. 15-16 (36623/37769): hunk-count and
empty/no-op evidence greps; SPEC.md greps (no grammar there). 17 (39344/190):
`.self-improve` inventory; payload-meta grep. Clean, cheap, single-pass recon —
only the 2-3 grep misfires (3-4, 14) are waste.

**Phase B — raw-input hunt (calls 18-33).** 18-19 (40038/40220): looks for raw
calls in `.self-improve/20260924-162255-c1-worker.log` — discovers those logs are
15-byte orchestrator stubs, not transcripts. 20-21 (40298/40424): `.pragma`
inventory; `find ~/.pragma` (exit 141 SIGPIPE on head). 22 (40773/2049): reads
provider_tools_loop.go:855-905 (tool-layer empty-patch check) — realizes raw logs
live in `~/.pragma/sessions/*.jsonl`. 23-24 (43379/44465): greps sessions for
`"@@ -`; 90 files mention apply_patch. 25-31 (45180→47435): dissects session
`a5b763cb` with python — empty output (wrong filter), then 4 calls of format
discovery (message/content/tool_call shapes), then finds its 3 apply_patch calls
have NO count-bearing headers: dead-end target, though format learning was
reusable. 32 (47761/606): the decisive move — python scan over ALL sessions for
count-bearing `@@` headers → 729e2564/7ac8ea2b/79dcb05b, 8 calls, 6 ERROR
results: the wire-recorded failing inputs (gold). 33 (48732/863): extracts the
full failing patches. The detour 25-31 (7 calls) would have been 1 call if 32's
all-sessions scan had come before picking a single candidate.

**Phase C — RED setup and run (calls 34-44).** 34 (50297/5435, 51.6s): git status
— shared tree dirty with FOREIGN modifications (queue file, run.go, auto.go…);
greps PatchPaths users. 35 (56261/1386): PatchPaths is dead code; finds the real
callers. 36 (57782/4553): writes `/tmp/pact001_probe.go` scratch module — FAILS
on go 1.24.3 vs module 1.25. 37 (62296/1204): immediate pivot, no repeat —
appends a scratch test into the package via `cat >>` (TEST-APPENDED). Good
within-session mistake-memory. 38 (63515/1683): re-reads the appended region.
39 (65664/118): apply_patch FAIL — nested `*** End Patch` inside the patch body.
40 (69313/106): corrected rewrite applies OK (nesting lesson carried). 41-42
(69313/69935): THINK falsely recalls "no *** Update File header"; verifies with
git diff + sed re-read anyway — self-inflicted doubt, but the verify instinct is
correct. 43 (71352/820): go-version mystery resolved (default go IS 1.25; /tmp
scratch resolved an older toolchain). 44 (72219/101): **RED** — both preflight
subtests FAIL for exactly the designed reasons (misleading content error;
silent acceptance).

**Phase D — fix, GREEN, adjacent (calls 45-53).** 45-47 (72541→76971): the
one-mechanism fix in 3 clean apply_patch calls (fields, parse routing, new
validators). 48 (79248/24): `go build ./… && echo BUILD_OK` — second `&&` slip
(exit-sensitive build). 49 (79285/43): test FAIL — count-shaped headers still
classified MALFORMED (bug in first fix draft). 50 (79698/770): one-patch
correction. 51 (80488/43): **GREEN** at 10:34:13. 52-53 (80738/81302): full
applypatch package ok; internal/query ok.

**Phase E — pre-existing-failure triage + hygiene (calls 54-65).** 54-57
(81371→81705): cmd/pragma FAILs TestProviderToolsCLIContract; drills into what it
does. 58 (81816/153): reads the acceptance test — mock HTTP, no apply_patch
involvement. 59-60 (82990/83569): detached worktree `/tmp/pact001-baseline` at
parent 0f758d6 → same FAIL at pristine HEAD: pre-existing, correctly attributed
and NOT chased. 61-63 (83655/84561): `cd`/shell-state confusion around the
worktree — remove attempted, then re-checked (`worktree list`), then forced
remove; ~2 calls of shell-model uncertainty. 64-65 (84714/84843): gofmt + vet
clean.

**Phase F — adjacent scan + redundant re-runs (calls 66-69).** 66-67
(84921/85209): e2e greps for other apply_patch consumers (none parse patches) —
defensible adjacency check for the "adjacent apply_patch tests" deliverable.
68-69 (85261/85400): full re-run of BOTH packages with **zero code change since
calls 52-53** — pure re-verification, redundant.

**Phase G — verbatim-patch upgrade + RED-at-parent (calls 70-83).** 70
(85469/1690): extracts the exact recorded patch from session 729e2564 call 17.
71 (87438/2844): re-reads the test region to rewrite. 72-74 (91075→98457):
**three consecutive apply_patch FAILs** ("did not match current file content") —
the worker dropped leading spaces on its own removal lines: the exact B.1
mechanical class this item fixes, re-experienced by the fixer. 75 (98457/114):
`cat -A` — BSD illegal-option mistake. 76 (98600/34): `cat -et` shows exact
bytes. 77 (98747/3251): full-function rewrite applies. 78 (102018/398): another
leading-space FAIL. 79-80 (102524/103167): subtests A/B land with the verbatim
recorded patch embedded. 81 (104205/55): gofmt + preflight GREEN with authentic
input. 82-83 (104467/105233): detached worktree at parent 0f758d6, copy final
test in, run → **RED-at-parent demonstrated** (subtest A fails with the
byte-identical recorded error; subtest B with "got success"); worktree removed.
This phase is the most expensive (22% of session input tokens) and it happened
**after GREEN** — see finding 2.

**Phase H — gates, commit, case record (calls 84-92).** 84-86 (105669/105816):
GATE1/2/3 re-runs (legitimate — code changed since 52/53 in phase G) + gofmt.
87 (105871/191): selective `git add` of exactly its two files — queue file left
untouched in the dirty tree. 88 (106100/379): **COMMIT 89522be at 10:41:04**.
The turn-budget warning (88/110, 22 remain) lands one call later — see finding
2. 89 (106636/2645): writes the 143-line case record (verified/refuted/claims
boundary/gates/follow-ups). 90 (109274/115): doctrine rule-1 diff check
(unchanged) + commits case record `193383a`. 91 (109449/140): commit log + clean
status verification (grep exit 1 = all committed). 92 (109680/165): fourth full
applypatch package run — redundant (no change since 84) + stat check. 93
(109931/1331, end_turn): final report; every claim in it checks out against the
call record (commits, gates, RED-at-parent, pre-existing failure, refuted
claims, worktrees removed, queue untouched, no push).

## Findings (intent-level audit)

1. **apply_patch self-friction is the dominant in-session mechanical waste
   (medium).** 5 of the worker's own 13 apply_patch invocations failed (38%):
   call 39 (nested End Patch), 72/73/74 and 78 (leading-space drops on removal
   lines), plus the `cat -A` BSD mistake at 75 and two byte-inspection calls to
   recover. Phase G burned 1.37M in-tokens largely on this. Ironic and valuable:
   the fixer re-experienced the exact failure class it was fixing (the case
   record's meta-note is honest). Not intent-waste (no re-verification, no goal
   confusion) — friction the fix itself will reduce.

2. **Commit-at-GREEN violated in ordering (medium).** First GREEN at call 51
   (10:34:13); commit at call 88 (10:41:04) — 37 calls / 6m51s later, including a
   post-GREEN test-fixture rewrite (70-80), a RED-at-parent demo (82-83), and
   adjacent gates (84-86). All of that work was deliverable-quality, but rule 2
   exists because workers die uncommitted: the budget warning arrived at 88/110
   turns, one call after the commit. Correct order: commit the GREEN fix + first
   passing test immediately (insurance), then upgrade the fixture and commit
   again. The session got away with it; the pattern did not get fixed.

3. **Redundant re-verification (low, ~3 calls).** Calls 68-69 re-ran both
   packages with zero code change since 52-53; call 92 re-ran the full package a
   fourth time with no change since 84. ~3% of the session in pure repeat
   testing. (84-85 were legitimate — phase G changed code.)

4. **Dead-end detour before the decisive scan (low, ~4 effective calls).**
   Calls 25-31 dissected one candidate session (a5b763cb) that held no
   count-header evidence; the all-sessions scan at call 32 found the gold in one
   call. Format learning (26-28) was reusable; target selection was a dead end.
   Related: calls 18-21 (rediscovering that `.self-improve` logs are stubs and
   raw logs live in `~/.pragma/sessions`) — loop knowledge not carried in the
   payload/mistake-memory ("(first attempt)" placeholder).

5. **Shell-state and environment mistakes (low).** Calls 61-63 (cd/worktree
   remove confusion), call 75 (BSD `cat -A`), call 36 (scratch module resolved
   go 1.24.3). Each recovered immediately without repetition — good
   within-session mistake-memory (36→37, 39→40, 49→50, 72-78 notwithstanding).

6. **Grep misfires (low).** Exact-case "B.1" miss (3-4), guessed filename
   formats (14, exit 2), bad glob (9). ~3 cheap retries total.

7. **`&&` slips (low, 2 occurrences, no breakage).** Call 1 (`ls -la && cat
   agent.md`), call 48 (`go build … && echo BUILD_OK`). Letter-of-rule-3
   violations; the session otherwise used the echo-marker + `$?` pattern
   correctly (GATE1/2/3, MARK1/2).

8. **Clean compliance elsewhere.** No stash (rule 4) — two detached worktrees,
   both removed. No push. Queue file untouched (selective staging at 87).
   Doctrine untouched (verified at 90). Trusted prior evidence (rule 6):
   B.1 taken as recorded, wire inputs extracted rather than re-proven — the
   opposite of the historical 53/80-call re-verification failure. Stop-at-
   deliverable (rule 7): after the case-record commit only 2 light closing
   calls + report; no citation goose-chase. Final report accuracy: high —
   every claim cross-checks against the call record; no near-false statements
   found. One nuance: RED-at-parent was demonstrated *after* GREEN (copied the
   finished test into a parent worktree), a sequencing deviation from
   failure-case-before-fix that the report words honestly ("demonstrated") but
   does not flag as out-of-order.

**Waste total:** ~12-13 call-equivalents of 93 (≈13%): 3 redundant re-runs, ~4
dead-end detour, ~4 friction-recovery, ~2 shell/environment — plus the rule-2
ordering risk that cost nothing this time but remains the loop's most-repeated
near-failure.

===WASTE-START===
[{"call": 72, "issue": "apply_patch self-friction: 5 of 13 own invocations failed (39 nesting; 72,73,74,78 leading-space drops) - the exact B.1 class being fixed; + BSD cat -A mistake at 75; phase G consumed 22% of session input tokens", "cost": "medium"}, {"call": 88, "issue": "commit-at-GREEN delayed: first GREEN at call 51 (10:34:13), commit 89522be at call 88 (10:41:04) after 37 more calls; turn-budget warning hit at 88/110 one call after commit - rule-2 die-uncommitted risk taken for post-GREEN fixture upgrade + RED-at-parent + gates", "cost": "medium"}, {"call": 25, "issue": "dead-end: 7-call dissection (25-31) of session a5b763cb which held no count-header evidence; decisive all-sessions scan came only at call 32 and found gold in one call", "cost": "low"}, {"call": 68, "issue": "redundant full re-run of applypatch + internal/query packages with zero code change since calls 52-53 (call 69 same)", "cost": "low"}, {"call": 92, "issue": "fourth full applypatch package run with no change since call 84 - closing re-verification", "cost": "low"}, {"call": 20, "issue": "4 calls (18-21) re-locating raw session logs: .self-improve logs discovered to be 15-byte orchestrator stubs; ~/.pragma/sessions location is loop knowledge not carried in payload/mistake-memory", "cost": "low"}, {"call": 62, "issue": "shell-state confusion after cd into baseline worktree: remove attempted at 61, existence re-checked at 62, forced at 63", "cost": "low"}, {"call": 14, "issue": "grep misfires: guessed nonexistent filename 2026-09-24T18-43-40.md (exit 2), exact-case 'B.1' miss at calls 3-4, bad glob at call 9 - ~3 cheap retries", "cost": "low"}, {"call": 48, "issue": "&&-chain around exit-sensitive 'go build ./internal/tools/applypatch/ && echo BUILD_OK' (rule 3 letter violation; no breakage)", "cost": "low"}, {"call": 1, "issue": "&&-chain 'ls -la && cat agent.md' (rule 3 letter violation; benign)", "cost": "low"}]
===WASTE-END===

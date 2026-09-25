# MM-001 reader analysis (doctrine rule 8 — every call read)

Worker: session `2d4a25ff-81ce-482f-aa17-ad5e89bd754f` (morph-glm53-744b @ morphllm),
MM-001 "Mistake-memory: orchestrator retry prompts carry prior attempt's failure
list verbatim". Log: `.self-improve/20260925-102627-c2-worker.log` (final report
only, 4,289 bytes; raw calls in `~/.pragma/sessions/2d4a25ff-*.jsonl`, 317 lines =
1 header + 158 messages + 158 metadata). Rendered with a python per-call renderer
(kept at `/tmp/render_mm001.py`): every assistant message = one API call; tokens =
per-call delta of cumulative `token_usage` from the metadata record bracketing the
call; stop inferred from content (tool_use / end_turn); tool-result previews
error-flagged (`Exit code: [1-9]`). All 79 calls rendered in three chunks and read.

**Session totals.** 79 API calls, 78 tool invocations (64 Bash, 14 apply_patch —
13 succeeded, 1 failed), 10:54:50 → 11:06:00 (11m10s), cumulative 5,784,410 in /
47,663 out tokens, $7.44. Commits: `ece537c` (fix+tests+fixture, 11:04:31) and
`60f8a9f` (case record, 11:05:36) — both verified in git. Footnote: call 8's
per-call delta renders as 0/0 (stale duplicate metadata record); its usage is
folded into call 9's larger delta. No call hit the 16k output budget.

## Phase map (calls, span, in/out tokens)

| Phase | Calls | Span | in / out | What |
|---|---|---|---|---|
| A orientation + mandated reads | 1-7 | 10:54:50-10:55:06 | 117k / 0.9k | agent.md, doctrine, queue item, self_improve.py |
| B payload-claim verification | 8-24 | 10:55:10-10:56:28 | 760k / 5.1k | B.2 blindspot, v1.3-vs-HEAD git archaeology, bg-log wiring |
| C artifact scoping | 25-34 | 10:56:32-10:57:30 | 597k / 3.9k | recordings sidetrack, session-file structure, is_error field |
| D RED setup | 35-44 | 10:57:53-11:00:54 | 759k / 25.6k | 49-log regex sweep, verbatim fixture, case record, tests |
| E RED run | 45-47 | 11:01:09-11:01:28 | 277k / 1.6k | 4 failures incl. `["(first attempt)"]` B.2 signature |
| F fix + GREEN iterations | 48-53 | 11:01:35-11:02:11 | 575k / 1.1k | 3-patch fix, one iteration (51→52), GREEN at 53 |
| G live-data check + qid refinement | 54-58 | 11:02:20-11:03:03 | 498k / 3.2k | own-session bug found; qid scoping; re-GREEN 58 |
| H adjacency gates | 59-65 | 11:03:11-11:04:07 | 715k / 1.1k | py_compile, harbor (env-blocked), swebench 12/12, live recheck |
| I commit + case record | 66-78 | 11:04:14-11:05:45 | 1,378k / 3.5k | README, commit ece537c, case-record edits, commit 60f8a9f |
| J final report | 79 | 11:06:00 | 108k / 1.4k | end_turn deliverable report |

## Per-call narrative (all 79)

**Phase A — orientation (calls 1-7).** 1 (10:54:50, in1719/58): `ls -la && echo
MARKER && wc -l agent.md docs/self-improvement-doctrine.md` — mandated reads
located; the `&&`-chain is a rule-3 letter slip (no exit-sensitive command in
it). 2 (11601/14): `cat agent.md`. 3 (16864/28): `cat
docs/self-improvement-doctrine.md`. 4 (17800/234): queue/evidence inventory
(`ls .self-improve/`, failure-cases). 5 (21414/79): `ls bin/ tools/` + queue head
— finds MM-001 `in_progress`, reads payload. 6 (23728/179): `wc -l
tools/self_improve.py` + `git status` — notes the shared tree is dirty with other
workstreams. 7 (24315/352): reads all 366 lines of self_improve.py in one pass —
the current mistake-memory mechanism (v1.8 final-text regex + `{mistakes}`
wiring) understood. Cheap, single-pass, zero waste.

**Phase B — payload-claim verification (calls 8-24).** 8 (0/0 — metadata artifact):
reads MASTER-BLINDSPOTS B.2. 9 (58866/1337): git log of self_improve.py history
(`| head` → exit 141 SIGPIPE, output still captured). 10 (31730/170): `git show
c4e8c36` — v1.3 requeue diff. 11 (32731/283): reads v1.3 self_improve.py:
retry prompt had NO mistakes field at the time B.2 was observed. 12 (33906/234,
exit 141): `git log -S CLK-002` on the queue — SIGPIPE again but evidence kept.
13 (45682/400): head of the c4 worker log. 14 (47191/195): cat of older worker
logs — discovers they hold only final text (`HARNESS_FINAL_OK`, `PROBE_COMPLETE`).
15 (47425/309): locates `~/.pragma/sessions/`. 16 (48294/127, exit 141): `ls -t |
head -5` + NEWEST assignment — SIGPIPE kills it; wasted. 17 (48610/192): proper
python redo of 16 — newest session is the worker's own file. 18 (49235/373):
python scan for 2026-09-24 sessions with apply_patch errors — candidate list.
19 (50613/348, exit 1): grep finds `16-34-15.md` but heads a guessed nonexistent
`2026-09-2*` filename — the exact name was visible one line above. 20 (51024/27):
reads `16-34-15.md` — the B.2 source evidence (attempt-2 repeating #35's mistakes).
21 (52409/347): greps `PRAGMA_BG_SESSION_LOG` in run.go — the bg-log wiring.
22 (52811/53): reads run.go:120-200 + 1660-1730 — bg log receives TextEvent text.
23 (54162/722): record-flag plumbing. 24 (55083/26): run.go:1860-1960 — recording
to `~/.pragma/recordings/<id>/`. This phase is the evidence-first core: B.2
repetition, salvage-pointer claim (v1.3), and the never-firing-regex defect all
verified before any edit. Waste: 16 (SIGPIPE redo) + 19 (guessed filename) only.

**Phase C — artifact scoping (calls 25-34).** 25 (56009/105): lists recordings
dir. 26 (56336/193): its own recording dir + python peek — session-id ↔
recordings mapping; informative but the mechanism will use `~/.pragma/sessions`,
so 25-26 are a 2-call sidetrack. 27 (56669/705): python over its own session
file — structure confirmed (tool_call/tool_result/thinking/text). 28 (57701/263):
scan mapping worker sessions → worker logs; identifies f4ff3ebb (attempt-2,
16:34:15) and 06f603fb (attempt-1, 16:22:55). 29 (58256/345): prints error
results from f4ff3ebb — content-level ("Exit code: 1") error inference. 30
(60228/1754): greps ToolErrors/IsError across watcher/query — pragma's own
authoritative tool-error definition. 31 (62079/25): session.go:195-235. 32
(62548/118): greps IsError producers. 33 (63343/150): ToolResultPart carries
`is_error` in JSON. 34 (63847/279): re-scan of f4ff3ebb keyed on the is_error
field — 13 flagged results incl. both B.2 apply_patch failures; partially
overlaps 29 but the flag-confirmation is what the mechanism will rely on.

**Phase D — RED setup (calls 35-44).** 35 (64621/2566, exit 1): heads
test_harbor_pragma_agent.py (got what it needed) + a guessed/truncated second
filename that fails. 36 (67641/131): reads CI workflows — python tools run ad
hoc, not in CI. 37 (68105/266): THE decisive defect check — runs HEAD's regex
over all 49 worker/critic/reader logs: matches only narrative prose (2 false
positives), never a tool error. 38 (68572/4939, +40s): prints f4ff3ebb
tool_call shapes. 39 (73661/4091, +32s): computes fixture kept-line indices
(header, first user msg, 3 error tool_results + their tool_calls) — 15.5KB. 40
(77774/534): builds the fixture `worker-session-clk002-attempt2.jsonl` under
tools/testdata/self_improve/. 41 (78439/2625): apply_patch adds the case record
`docs/failure-cases/mistake-memory-retry-prompts-2026-09-25.md` — case-before-fix
per doctrine. 42 (81065/7364, +52s): long design thinking + prints the fixture's
3 error texts verbatim. 43 (88487/2412): apply_patch adds
tools/test_self_improve.py (4 tests: scripted two-cycle main(), extraction,
discovery, marker-in-prompt). 44 (90916/698): apply_patch fixes a started_epoch
test detail. All productive; 35's second half is the only slip.

**Phase E — RED run (calls 45-47).** 45 (91631/147, exit 1 DELIBERATE): `python3
-m unittest tools.test_self_improve -v` at HEAD content — 4 failures, mechanism
absent. 46 (92321/124, exit 1): filtered re-run of the scripted-cycle test —
captures the retry prompt rendering `["(first attempt)"]`, i.e. the exact B.2
zero-memory state, as test evidence (this re-run is evidentiary, not redundant).
47 (93257/1371): thinking declares RED confirmed, then apply_patch #1 of the fix
— SESSIONS_DIR/WORKER_MARKER/extract_tool_errors added to self_improve.py.

**Phase F — fix + GREEN iterations (calls 48-53).** 48 (94613/82): greps the
requeue site to patch precisely. 49 (94885/27): reads self_improve.py:332-390.
50 (95601/486): apply_patch wires discovery+extraction into main()'s requeue
branch (replaces the dead regex). 51 (96103/43, exit 1): GREEN attempt — 3/4
pass; discovery test fails (find returned None). 52 (96641/467): apply_patch
iteration (epoch/timezone comparison). 53 (97124/38): **GREEN 4/4** (11:02:11).
One legitimate iteration, RED→GREEN honored.

**Phase G — live-data check + qid refinement (calls 54-58).** 54 (97413/402):
runs the new extraction live — finds its OWN session (2d4a25ff), not the failed
worker's: a real production-correctness bug in the just-GREENed mechanism. 55
(98350/1890): apply_patch adds qid scoping to find_worker_session. 56
(100256/71): apply_patch updates the call site (`item["id"]`). 57 (100343/828):
apply_patch updates the test (qid param + decoys). 58 (101166/38): GREEN 4/4
again (11:03:03). Note: 55-57 are a FUNCTIONAL refinement after first GREEN with
no RED of its own — see finding 1.

**Phase H — adjacency gates (calls 59-65).** 59 (101514/252): py_compile clean.
60 (101783/45, exit 1): harbor test run — loader error. 61 (101857/35, exit 1):
re-run with head — traceback visible (import chain into missing external
`harbor` package). 62 (102122/142, exit 1): `cd tools` re-run — sys.path
hypothesis, refuted. 63 (102293/37, exit 1): same re-run with head — confirms
environmental, pre-existing. 62-63 are the redundant pair (~2 wasted). 64
(102588/214): test_run_swebench_pro_instance 12/12 OK. 65 (102833/376): live
recheck with qid scoping — PACT-001 → session 99409da1 with 8 real tool errors
(and would find the MM-001 worker's own); the fix verified on live data.

**Phase I — commit + case record (calls 66-78).** 66 (103485/716): apply_patch
adds tools/testdata/self_improve/README.md (fixture provenance: verbatim
slices, dispatch-metadata-only rewrites) — post-GREEN docs addition into commit
1. 67 (104221/88): git status — exactly its 3 file groups changed. 68 (104431/153):
full suite re-run — GREEN (commit gate). 69 (104687/441): **commit `ece537c`**
(fix+tests+fixture+README, 11:04:31) — selective staging, 16 calls after first
GREEN. 70 (105260/917): apply_patch polishes the case record (qid scoping,
entry format). 71 (106209/433): apply_patch FAILS — "did not match current file
content" (stale remembered context; the only failed patch of 14). 72
(106726/50): grep re-orients on the real lines. 73 (106821/41): sed reads the
exact current text. 74 (106966/86): retry patch succeeds. 75 (107084/100):
second case-record patch succeeds. 76 (107216/37): full suite re-run after
docs-only edits — redundant (68 already gated the commit; only a .md changed).
77 (107357/187): **commit `60f8a9f`** (case record, 11:05:36). 78
(107681/235): git status — only other workstreams' dirt remains
(`internal/tools/applypatch/applypatch.go`); nothing of MM-001 uncommitted.

**Phase J — final report (call 79).** 79 (108062/1414, 11:06:00, end_turn):
the deliverable summary + WORKER-REPORT JSON — matches the log file byte-for-
byte in structure. Session ends here: **no post-deliverable tail.**

## Findings (intent-level audit)

**Cross-check of the final report against the call record — no near-false
claims.** B.2 verified (calls 20, 34); v1.3 no-mistakes-field verified (10-12);
bg-log TextEvent-only defect verified (21-24) and never-firing regex proven
empirically on all 49 logs (37); RED 4-failures with the `["(first attempt)"]`
signature captured (45-46); GREEN 4/4 (53, re-GREEN 58); swebench 12/12 (64);
harbor env-blocked (60-63); live-data PACT-001/99409da1/8-errors (65); commits
`ece537c`+`60f8a9f` (69, 77); "No Go files touched" — confirmed (no .go file
ever edited); claim boundary honestly deferred ("retry-avoidance is a
model-behavior claim for the next real retry to observe").

1. **Commit-at-GREEN stretched (medium).** First GREEN at call 53 (11:02:11);
commit `ece537c` at call 69 (11:04:31) — 16 calls / 2m20s later, after a
post-GREEN FUNCTIONAL refinement (54-58: live-data check found discovery
returning the worker's own session; qid scoping landed 55-57 without a RED of
its own), gates (59-65), a post-GREEN README addition (66), and a full re-run
(68). Mitigations: the refinement fixed a genuine wrong-session correctness bug
in the fresh mechanism; the committed state was suite-verified GREEN seven
seconds before the commit; turn headroom was ample (call 53 of the 110 cap).
Doctrine rule 2 ("Commit at GREEN, immediately") is still violated in letter —
same class as PACT-001's medium finding but a shorter span with a correctness
justification PACT-001's lacked.

2. **Stale-context apply_patch, once (low, with irony).** Call 71 — the only
failed patch of 14 (93% vs PACT-001's 8/13): the worker edited the case record
from remembered line context without re-reading; recovered via grep 72 + sed
73 + retry 74 (3 extra calls). This is the same B.2 mechanical class
(stale-context patching) the item builds mistake-memory for — one self-caught
instance, not a repetition.

3. **Harbor adjacency confusion loop (low).** Calls 60-63: four calls to accept
a pre-existing environmental failure. 61's traceback already showed the import
chain into the missing external `harbor` package; 62-63 (cd-tools sys.path
hypothesis + repeat head) were redundant — ~2 wasted calls.

4. **SIGPIPE shell friction, repeated (low).** Calls 12 and 16 — `| head`
pipes killed by SIGPIPE (exit 141) twice in three minutes; 16 was fully wasted
(python redo at 17), 12 partial (output already captured). Same class PACT-001
showed (its call 20-21).

5. **Recordings sidetrack (low).** Calls 25-26 explored
`~/.pragma/recordings/<id>/` structure; the mechanism landed on
`~/.pragma/sessions/*.jsonl` instead. Two calls of informative-but-unused
artifact mapping.

6. **Redundant re-runs / guessed filenames (low).** 76 re-ran the full suite
after docs-only case-record edits (GREEN already at 68). 19 headed a guessed
nonexistent `2026-09-2*` filename when grep had printed the exact name one
line above; 35's second head died on a guessed truncated test filename.

**Clean where it matters.** No `git stash` (rule 4), no `git push`, no worktrees
(none needed), no `&&`-chains around exit-sensitive commands (call 1's
`ls && echo && wc` is a letter-only slip), queue file untouched, selective
staging at both commits, single-mechanism change honored (one extraction
mechanism replacing one dead regex, case record + tests + fixture only),
stop-at-deliverable respected (79 ends the session; no goose-chase tail), and
the case record was written BEFORE the fix (doctrine order). 11 non-zero exits
total: 7 deliberate/environmental (RED 45-46, iteration 51, harbor 60-63), 4
real slips (12, 16, 19, 35) + 1 apply_patch miss (71).

**Bottom line.** ~11 call-equivalents of 79 (≈14%) were waste — dominated by
gates-adjacent re-verification and shell friction, not intent errors. The one
medium finding is ordering (GREEN→commit gap with an ungated in-between
refinement), not substance: every functional step between first GREEN and the
commit was individually justified and the committed state was re-verified
green at the commit gate.

===WASTE-START===
[{"call": 69, "issue": "commit-at-GREEN stretch: first GREEN at call 53, commit ece537c at call 69 after ungated post-GREEN functional refinement (54-57 qid scoping), gates, and post-GREEN README; mitigated by re-verified GREEN at 68 and ample turn headroom", "cost": "medium"}, {"call": 71, "issue": "stale-context apply_patch on case record (only failed patch of 14) — same B.2 mechanical class the item fixes; needed grep 72 + sed 73 before retry 74", "cost": "low"}, {"call": 62, "issue": "harbor adjacency confusion: cd-tools re-run of an already-diagnosed import failure; sys.path hypothesis refuted by 61's traceback (with 63)", "cost": "low"}, {"call": 16, "issue": "SIGPIPE (exit 141) from `| head` pipe — wasted, redone properly in python at 17 (class also hit at 12)", "cost": "low"}, {"call": 25, "issue": "recordings-directory sidetrack — explored ~/.pragma/recordings structure (with 26) but mechanism uses ~/.pragma/sessions", "cost": "low"}, {"call": 76, "issue": "redundant full-suite re-run after docs-only case-record edits; GREEN already gated at 68", "cost": "low"}, {"call": 19, "issue": "headed a guessed nonexistent 2026-09-2* analysis filename when the correct name was in the grep output one line above (exit 1, redo at 20)", "cost": "low"}, {"call": 35, "issue": "second half of command failed on a guessed/truncated test filename after the needed content was already shown", "cost": "low"}]
===WASTE-END===

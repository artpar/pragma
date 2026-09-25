# CMT-001 worker session — reading analysis (doctrine rule 8)

- Queue item: CMT-001 — Commit-at-GREEN: worker prompt rule + orchestrator abnormal-exit detection for uncommitted GREEN
- Worker log (final summary only): `.self-improve/20260925-102627-c3-worker.log`
- Session transcript (the API-call record): `~/.pragma/sessions/dd0c0e1e-e684-4c9a-868c-0c05a3244677.jsonl`
- Session span: 2026-09-25T11:16:11 → 11:28:29 IST (12m 18s), 71 API calls, model morph-glm53-744b via morphllm, harness `provider-tools`
- Session totals (from transcript metadata): 4,813,287 input tokens, 56,481 output tokens, $6.2651; every call ended `tool_use` except call 71 (`end_turn`)
- Deliverable landed: commits `c4fab51` (mechanism + tests + fixture, 4 files) and `848f609` (case record, 137 lines) on main; no push; no cap death (71 of 80 calls)

Rendering method note: each assistant message in the transcript is one API call.
The harness records no stop reason; `stop` below is inferred from message shape
(`tool_use` = the call issued tool calls and the loop continued; `end_turn` = the
final text-only call). Token figures: the transcript's metadata records
*cumulative* session usage, refreshed per message; per-call `in` below is the
delta (the fresh-value sequence — one stale duplicate flush at call 3 was
discarded). `out` is that call's own output. THINKING is clipped to 150 chars,
tool args to 120, as dispatched.

## Per-call narrative — all 71 calls

### Phase 1 — orientation and evidence location (calls 1–8, 11:16:12–11:16:29)

- **Call 1** (11:16:12, in≈1.7k, out 51, tool_use): reads `agent.md` + `docs/self-improvement-doctrine.md` in one parallel pair. Clean start, exactly as the prompt orders.
- **Call 2** (11:16:14, in≈14.2k, out 114): `ls docs/failure-cases/`, `ls docs/analyses/` + grep for CMT-001 in the queue file. Locates the item's paperwork.
- **Call 3** (11:16:17, in≈17.1k, out 119): grep "B.3" across docs + ls `docs/analyses/reading/`. Evidence hunt begins.
- **Call 4** (11:16:19, in≈18.5k, out 172): `grep -rln cap-death|cap death|uncommitted` + `grep -rln blindspot` — finds MASTER-BLINDSPOTS-2026-09-24.md.
- **Call 5** (11:16:21, in≈19.2k, out 48): **dead grep** — `grep -n "B.3" MASTER-BLINDSPOTS` exits 1 (the literal tag is absent); no thinking recorded. Six calls (2–7) end up spent locating evidence one targeted grep of the now-known file would have produced.
- **Call 6** (11:16:23, in≈19.3k, out 77): grep "commit" + section headings in the blindspots file.
- **Call 7** (11:16:24, in≈19.6k, out 41): `sed -n 34,95p` finally reads the B.3 section — the three cap-death events with session ids.
- **Call 8** (11:16:29, in≈20.7k, out 567): grep for the WORKER-REPORT plumbing + repo/tools listing. Thinking records B.3 confirmed with specifics (16-22-55 died at 78/80 uncommitted, 15-02-35, 16-58-58).

### Phase 2 — HEAD-state verification (calls 9–13, 11:16:42–11:17:13)

- **Call 9** (11:16:42, in≈21.5k, out 51): `wc -l` on the two tools files + `git log -8` + `git status` — establishes HEAD 60f8a9f and the shared-tree situation (other agents' modifications present).
- **Call 10** (11:16:45, in≈21.9k, out 116): full `cat tools/self_improve.py` — the orchestrator, one read. Thinking: shared tree, keep changes scoped to the two tools files.
- **Call 11** (11:17:00, in≈27.0k, out 2001): git history of `self_improve.py` + `git log -S "Commit AT GREEN"` — provenance of the prompt rule (0f758d6, v1.8). Long reasoning establishing deliverable state: A already delivered, B absent.
- **Call 12** (11:17:03, in≈29.3k, out 110): `git show -s 0f758d6` + `cat docs/self-improvement-queue.jsonl` — proves the rule predates the queue item; reads the item payload in full.
- **Call 13** (11:17:13, in≈36.6k, out 1257): `ls .self-improve/` + grep `MaxTurns` in internal/cli. Thinking: complete state analysis — prompt rule pinned at v1.8, orchestrator half absent at HEAD.

### Phase 3 — exit-code / pattern grounding (calls 14–26, 11:17:16–11:18:13)

- **Call 14** (11:17:16, in≈39.2k, out 65): grep `MaxTurns` in internal/query.
- **Call 15** (11:17:19, in≈39.7k, out 47): grep turn-cap phrasings in `miniswe_loop.go` — finds the cap site.
- **Call 16** (11:17:22, in≈40.3k, out 87): `sed 540,560p miniswe_loop.go` (the `agentic loop exceeded maximum of %d turns` ErrorEvent) + grep ErrorEvent/RunE/os.Exit in run.go (partial dead end, exit 1).
- **Call 17** (11:17:26, in≈40.9k, out 137): **dead grep** — exitCode/Exit/returnCode/os.Exit not in run.go. The hunt for "what exit code does a cap death produce" is now wandering.
- **Call 18** (11:17:29, in≈41.1k, out 47): **dead grep** — `os.Exit` across internal/cli + cmd. Not found.
- **Call 19** (11:17:32, in≈41.3k, out 28): **dead grep** — "Exit|exit" in run.go. Still nothing.
- **Call 20** (11:17:35, in≈41.7k, out 126): sed `cmd/pragma/main.go` 1–40 + grep promptMode/headless — **dead grep** (promptMode absent).
- **Call 21** (11:17:39, in≈42.1k, out 26): single-line grep `RunDispatcher` — a one-purpose call.
- **Call 22** (11:17:42, in≈42.2k, out 23): `sed 41,140p internal/cli/run.go`.
- **Call 23** (11:17:45, in≈43.1k, out 24): single-line grep `func RunNonInteractive`.
- **Call 24** (11:17:48, in≈43.2k, out 25): `sed 1532,1650p run.go`.
- **Call 25** (11:17:51, in≈44.2k, out 25): `sed 1650,1760p run.go` — the headless `return e.Err` path finally read.
- **Call 26** (11:18:13, in≈45.2k, out 2669): cat the MM-001 case record — the scripted-cycle test pattern this item will reuse. Thinking consolidates: cap exhaustion → ErrorEvent → cobra → rc 1 → orchestrator sees rc!=0. Conclusion correct, but the route took nine calls (16–25), five of them dead ends, to establish a fact the call-10 read of `run_pragma`'s contract already implied.

### Phase 4 — transcript grounding and fixture build (calls 27–42, 11:18:17–11:23:06)

- **Call 27** (11:18:17, in≈49.7k, out 73): full `cat tools/test_self_improve.py` — the existing MM-001 tests and staging conventions.
- **Call 28** (11:18:34, in≈51.7k, out 2162): grep session/GREEN/commit in the 16-22-55 + 15-02-35 reading analyses — cross-checks B.3 against the reading record.
- **Call 29** (11:18:38, in≈55.1k, out 142): grep/head 16-58-58.md — third event cross-checked; session ids for all three deaths now in hand.
- **Call 30** (11:18:43, in≈56.7k, out 128): `ls ~/.pragma/sessions/` — 473 transcripts.
- **Call 31** (11:18:47, in≈58.0k, out 118): scans **all 473** session headers (55 s) filtering `created_at` to the three known HH:MM:SS stamps — finds the three files. The targets were already known to minute precision; a date-glob would have skipped the full-tree loop.
- **Call 32** (11:19:42, in≈58.3k, out 636): python scan of the three sessions for successful-looking test results — all three contain green gates.
- **Call 33** (11:20:15, in≈59.8k, out 4069): head of the existing fixture + result/call counts + `cat` the testdata README — fixture conventions.
- **Call 34** (11:20:22, in≈64.7k, out 411): **wasted scan** — python scan of 06f603fb for non-error results containing `ok github.com` returns **zero** hits. The actual wire content is `ok  \tgithub.com` (whitespace between); the substring predicate is wrong.
- **Call 35** (11:20:37, in≈65.1k, out 1723): re-scan with `"\nok" in content` finds the green result; thinking openly logs the confusion ("NO non-is_error tool_results containing ok github.com??") and reconciles it with call 32's unfiltered scan. One extra call + a surprise loop to resolve a self-inflicted predicate bug.
- **Call 36** (11:20:59, in≈66.9k, out 2477): `git show c4e8c36/c08bc55` — pre-v1.3 orchestrator marked such deaths `failed` outright. 10k-char thinking.
- **Call 37** (11:22:20, in≈70.0k, out 11409 — the session's largest output): a **45,351-char thinking block** consolidating the entire investigation, then a python check that 06f603fb's first user message carries the WORKER marker and queue line. The consolidation is high-value (nothing thrashes after it), but ~20% of the session's total output tokens are spent here, largely re-expressed one call later as the notes file.
- **Call 38** (11:22:32, in≈81.5k, out 1311): writes durable state notes `.self-improve/cmt-001-worker-notes.md` (verified evidence, mechanism plan, RED test plan, remaining steps) — turn-contract compliance, the right move at 38/80 calls.
- **Call 39** (11:22:40, in≈82.8k, out 422): python scan locating the wall-clock gate tool_call id in 06f603fb.
- **Call 40** (11:22:52, in≈84.2k, out 1132): extracts the exact header + first-user + assistant/user gate pair to `/tmp/cmt001-fixture-lines.jsonl`.
- **Call 41** (11:22:59, in≈85.5k, out 263): inventories the MM-001 fixture's results — confirms its last gate is the RED assertion (contrast case).
- **Call 42** (11:23:06, in≈86.0k, out 398): copies the extracted lines in as `worker-session-clk002-greengate.jsonl` + verifies structure. Fixture is verbatim wire-recorded data.

### Phase 5 — RED (calls 43–49, 11:23:25–11:24:18)

- **Call 43** (11:23:25, in≈86.5k, out 1975): `sed 30,45p` of the test file — a small partial re-read of the file fully catted at 27, for patch context. Acceptable.
- **Call 44** (11:23:33, in≈88.6k, out 316): apply_patch adds `GREEN_GATE_FIXTURE` + `stage_worker_session(fixture=)` — ships a **mis-indented continuation line** in the patch.
- **Call 45** (11:23:39, in≈89.0k, out 161): apply_patch fixing 44's indentation ("Oops — one extra leading space"). Self-inflicted repair, one call after the mistake.
- **Call 46** (11:23:57, in≈89.1k, out 1766): apply_patch adds the three test classes (prompt-rule pin, detector True/False, two scripted near-miss/mid-work cycles) — introduces **another** misaligned continuation line.
- **Call 47** (11:24:04, in≈90.9k, out 79): `py_compile` catches 46's glitch. Same mechanical mistake class twice within three calls.
- **Call 48** (11:24:10, in≈91.0k, out 38): `unittest -v | tail -25` — **RED at HEAD 60f8a9f: 4 failures for exactly the stated reasons** (detector/hint absent, plain requeue). Genuine RED before any mechanism code exists.
- **Call 49** (11:24:18, in≈91.5k, out 235): re-runs the whole suite to `/tmp/cmt001-red.log` + greps the FAIL lines — a durable RED capture that call 48's run already displayed (should have tee'd).

### Phase 6 — mechanism, GREEN, adjacent, live data (calls 50–58, 11:24:51–11:26:24)

- **Call 50** (11:24:51, in≈92.3k, out 3655): apply_patch — `TEST_CMD_RE`/`GREEN_GATE_RE`/`RED_GATE_RE` + `NEAR_MISS_HINT` constants.
- **Call 51** (11:24:59, in≈96.0k, out 442): apply_patch — `last_test_gate_passed()` detector.
- **Call 52** (11:25:08, in≈96.5k, out 726): apply_patch — death-branch wiring: `near_miss` flag, hint prepended to `{mistakes}`, `queue.insert(0, ...)` priority requeue, distinguishing print; attempts/needs_attention cap deliberately unchanged.
- **Call 53** (11:25:16, in≈97.2k, out 273): `py_compile` + `unittest | tail -18` with PIPESTATUS echo — exit 0 but the tail cuts the per-test list.
- **Call 54** (11:25:24, in≈98.0k, out 160): **re-runs the suite** with `grep ^test|Ran|OK|FAILED` — GREEN 9/9 now visible. First full GREEN. (A re-run caused by 53's pipe truncation.)
- **Call 55** (11:25:33, in≈98.6k, out 499): live-data run of the detector on the five real sessions — B.3 #1/#3 True, but **dbab3dbf False** (anomaly) and f4ff3ebb True.
- **Call 56** (11:25:46, in≈99.3k, out 913): digs into the two anomalies' gate lists — dbab3dbf's *last* gate was a known-dirty full-suite sweep (pre-existing reds), so the miss is correct-by-rule.
- **Call 57** (11:26:05, in≈100.9k, out 1976): dumps f4ff3ebb's last 8 messages — discovers the worker had all gates passing **and had `git add`-staged the fix** before dying at the cap: a fourth, previously unrecorded near-miss. This is genuine diligence, not waste — it produced the boundary documentation and the bonus finding.
- **Call 58** (11:26:24, in≈103.3k, out 1658): adjacent suite `tools.test_run_swebench_pro_instance` — 12/12 OK. Note: run as `cd tools && python3 -m unittest ...` — a &&-chained test (letter violation of the no-&&-with-grep/test rule; mitigated by the `PIPESTATUS` echo check that follows). All gates now closed.

### Phase 7 — post-GREEN to commit (calls 59–66, 11:26:34–11:27:30)

All gates closed at 58. The rule says: commit AT GREEN, immediately; polish follows in a second commit. What follows instead:

- **Call 59** (11:26:34, in≈105.0k, out 561): apply_patch — README provenance for the new fixture (polish-class; the README is bundled into the eventual mechanism commit).
- **Call 60** (11:26:43, in≈105.6k, out 346): `git diff` self-review of self_improve.py — catches that a comment overclaims dbab3dbf as verified-green.
- **Call 61** (11:26:52, in≈107.5k, out 470): apply_patch — comment-only correction (dbab3dbf = documented miss).
- **Call 62** (11:26:59, in≈108.0k, out 164): apply_patch — docstring-only correction in `last_test_gate_passed`.
- **Call 63** (11:27:06, in≈108.2k, out 60): `py_compile` + `unittest | tail -3` — re-verification after comment-only edits.
- **Call 64** (11:27:13, in≈108.4k, out 43): **re-runs the suite again** with grep — the second consecutive verification run for a non-behavioral diff.
- **Call 65** (11:27:21, in≈108.4k, out 231): `git add` of exactly the four deliverable files + status check (explicit path list, shared tree respected).
- **Call 66** (11:27:30, in≈108.7k, out 451): `git commit` → **c4fab51**, the mechanism+tests+fixture+README. GREEN first shown at call 54 (2m06s earlier); gates closed at 58 (66s, 8 calls earlier). Six of the intervening calls (59–64) are polish-class or re-verification.

### Phase 8 — case record, polish commit, exit (calls 67–71, 11:27:50–11:28:29)

- **Call 67** (11:27:50, in≈109.3k, out 2330): writes the 137-line case record (observed/expected/executable assertion/mechanism/refutation/claim boundary/revision note). Comprehensive, honest about the dbab3dbf miss.
- **Call 68** (11:27:58, in≈111.7k, out 231): final suite grep + removes the scratch notes file — a `;`-chained test+cleanup in one command (mixes a gate run with an unrelated mutation; harmless here, but the same silent-breakage class the && rule warns about).
- **Call 69** (11:28:06, in≈111.9k, out 116): `git add` + commit **848f609** (case record) + status check. The is_error flag on this tool result is a trailing `grep -v` finding nothing — benign, understood and explained by the worker on the next call.
- **Call 70** (11:28:15, in≈112.2k, out 334): `git show --stat` of both commits + empty-diff check for its own paths — all committed, verified.
- **Call 71** (11:28:29, in≈112.7k, out 1345, **end_turn**): the final deliverable summary (3,708 chars — the text that became the worker log) + the WORKER-REPORT JSON block. **Clean stop; no post-deliverable tail.**

## Findings

### Methodology violations

1. **Commit-at-GREEN: delayed by polish (medium).** First 9/9 GREEN at call 54 (11:25:24); all gates (suite + adjacent + live-data) closed at call 58 (11:26:24); the commit lands at call 66 (11:27:30) — 8 calls and 66 s later, with six polish-class calls (README 59, diff review 60, two comment-only patches 61–62, two re-verification runs 63–64) in between. The rule's letter is unambiguous: "Commit AT GREEN, immediately — before docs/case-record polish. The deliverable is the commit; polish can follow in a second commit." The live-data checks (55–57) are defensible as gate verification — they found the dbab3dbf boundary and the f4ff3ebb bonus — but 59–64 are exactly the class of work the rule reserves for commit 2. The committed code differs from the code first verified GREEN only by comments, and it was re-verified, so correctness held; the violation is one of sequencing and exposure. The irony is sharp: CMT-001 *is* the commit-at-GREEN item, and the worker sat 14 calls past first-GREEN in the precise failure mode (die-before-commit) the item exists to cure — with 108k input tokens per call by then, the most expensive regime of the session (~640k input, ~13% of session spend, for the six polish calls).
2. **&&-chains: one letter violation (low).** Call 58 runs the adjacent suite as `cd tools && python3 -m unittest ... | tail; echo ADJ EXIT ${PIPESTATUS[0]}` — a test chained with &&. The && guards a `cd` (so the failure mode the rule targets — a silently-skipped test — cannot occur; the PIPESTATUS echo then checks the exit anyway). The other twelve `&&` occurrences are benign `cd X && python3` heredoc guards, and call 59's `&&` is documentation text quoting the recorded gate command. Verdict: technically non-compliant once, materially safe.
3. **No stash: compliant.** Zero `git stash` usage; isolation concerns handled by reading only (no worktree needed).
4. **Stop-at-deliverable: compliant.** Call 71 is the last call, `end_turn`, carrying the deliverable summary. No citation goose-chases, no post-deliverable wandering.
5. **RED-before-fix, one-mechanism, evidence-first: compliant.** Tests written and RED demonstrated at HEAD (48–49) before the mechanism existed (50–52); exactly one mechanism-level change; both payload claims verified against code before acting; the refuted claim ("scans worker log" as a literal bg-log scan) grounded in MM-001's wire-proven evidence and changed nothing for it; queue file never touched; no push; durable notes written at 38.

### Wasted calls and redundancy

- **Exit-code hunt, calls 16–25 (medium):** five dead-end greps (17, 18, 19, 20, plus partial 16) and three single-purpose greps (21, 23) walking internal/cli to establish "cap death → rc 1" — a fact the call-10 read of the orchestrator's `rc != 0` death branch already implied, and which could have been settled in 2–3 targeted calls (`grep -rn "agentic loop exceeded" internal/` finds the site in one hop). ~370k input tokens across the nine calls. It did converge — every retry after a dead end changed the probe — but it is the session's largest avoidable cluster.
- **Predicate-bug scan, call 34 (low):** searching 06f603fb for `ok github.com` (actual content: `ok  \tgithub.com`) returned zero hits and triggered a visible surprise/confusion loop resolved only at 35. One wasted scan + reconciling thinking, in a file the worker then scanned three more times (39, 40 — purposeful extraction, unlike 34).
- **Redundant test re-runs (low, three instances):** call 49 re-runs the RED suite to capture a log (48 already displayed the failures — a `tee` would have served); call 54 re-runs because 53's `tail -18` cut the per-test list; call 64 re-runs after 63 already compiled and ran the suite for a comment-only diff. ~300k input tokens combined, all in the expensive late-session regime.
- **Evidence-location breadth, calls 2–7 (low):** six calls to locate B.3, including one dead grep on a literal tag the file does not contain.
- **Full-tree session scan, call 31 (low):** 55 s over all 473 session headers when the three target sessions were already known to HH:MM:SS from the reading notes.

### Confusion loops and repeated mistakes

- Two genuine confusion moments: the exit-code dead-end walk (16–25, converging, medium) and the 34→35 predicate reconciliation (low). No call ever repeated a prior attempt verbatim — every post-failure probe differed.
- One repeated mechanical mistake class: patch-line indentation, twice in three calls (44→45 self-caught, 46→47 caught by py_compile) — the exact mechanical class the sibling PACT-001 case record targets. Caught both times before any damage; cost two extra calls.
- 06f603fb was scanned by python five times (32, 34, 35, 39, 40); only 34 was wasted. Files were never redundantly re-read in full (one small partial re-read at 43 for patch context).

### What the session did right

Evidence-first held end to end: both payload claims verified against code before any change; the refutation grounded in trusted MM-001 wire evidence; the RED test embeds verbatim wire-recorded session slices; the live-data check honestly surfaced and documented a real boundary (dbab3dbf missed by rule) instead of tuning the detector to pass; the fixture provenance is documented; durable notes preceded the long tail; the two-commit structure (mechanism at GREEN, then case record) is exactly the shape the rule prescribes — only its timing slipped. Total waste is roughly 15–20% of spend, concentrated in the exit-code hunt and the post-GREEN polish delay; nothing in the session endangered the deliverable (committed with 14 calls of headroom, 71 of 80 used).

===WASTE-START===
[{"call": 17, "issue": "exit-code hunt 16-25: five dead-end greps (17,18,19,20 no-match; 21,23 single-purpose) walking internal/cli to prove cap-death rc=1, a fact the call-10 read of the rc!=0 death branch already implied; ~370k input tokens, the session's largest avoidable cluster", "cost": "medium"}, {"call": 59, "issue": "commit-at-GREEN delayed: all gates closed at call 58 (9/9 + adjacent 12/12 + live-data) yet 59-64 (fixture README, diff review, two comment-only patches, two re-verification runs) preceded the commit at 66 - polish-class work the rule reserves for commit 2, ~640k input in the most expensive regime; CMT-001 is itself the commit-at-GREEN item", "cost": "medium"}, {"call": 34, "issue": "wasted 06f603fb scan with a bad substring predicate (searched 'ok github.com', actual wire content 'ok  \\tgithub.com') returning zero hits and causing a surprise/confusion loop reconciled only at 35", "cost": "low"}, {"call": 49, "issue": "RED unittest re-run (call 48 already displayed the 4 failures) solely to save /tmp/cmt001-red.log; a tee on the first run would have served (~91k input)", "cost": "low"}, {"call": 54, "issue": "suite re-run needed because 53's 'tail -18' cut the per-test list, leaving only the exit code visible; pipe-output truncation cost a full re-run (~98k input)", "cost": "low"}, {"call": 64, "issue": "second consecutive unittest re-run (63 already compiled and ran the suite with an exit check) for a comment-only diff from 61-62; two verification runs for a non-behavioral change at ~108k input each", "cost": "low"}, {"call": 44, "issue": "mechanical patch-indentation errors twice in a row: 44 shipped a mis-indented continuation (fixed 45), then 46 introduced another misaligned line caught by py_compile at 47 - the same mistake class repeated within three calls", "cost": "low"}, {"call": 31, "issue": "55-second scan of all 473 session headers when the three target cap-death sessions were already identified to HH:MM:SS precision by the reading notes; a date-filtered glob would have skipped the full-tree loop", "cost": "low"}, {"call": 5, "issue": "dead grep (exit 1: the literal tag 'B.3' is absent from MASTER-BLINDSPOTS-2026-09-24.md); calls 2-7 spent six calls locating evidence that one targeted grep of the already-located file would have produced", "cost": "low"}, {"call": 58, "issue": "adjacent-suite run chained as 'cd tools && python3 -m unittest ...' - a &&-chained test, a letter violation of the no-&&-with-grep/test rule (mitigated: the && guards only the cd and a PIPESTATUS echo check follows)", "cost": "low"}, {"call": 37, "issue": "45,351-char private thinking block (~11.4k output tokens, the session's largest single output, ~20% of total output) consolidating the full picture, largely re-expressed one call later as the durable notes file at 38", "cost": "low"}]
===WASTE-END===

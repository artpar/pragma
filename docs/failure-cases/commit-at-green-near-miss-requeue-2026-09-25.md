# CMT-001 — Orchestrator treated uncommitted-GREEN cap deaths as plain failures

- Case ID: `CMT-001`
- Source revision: `60f8a9f` (2026-09-25, loop v1.8 + MM-001)
- Observed events: worker sessions `06f603fb-ccc9-4cff-8046-6a4323b3c8ee`
  (CLK-002 attempt 1, 2026-09-24T16:22:55+05:30, killed at the 80-call cap
  with gates green + wire-verified at call 78 and nothing committed),
  `f4ff3ebb-303c-483a-a653-444b27919e01` (CLK-002 attempt 2, 16:34:15,
  killed at 80 with gates green and the fix files already STAGED by
  `git add`), `de57a18f-f5a3-487c-9ff7-78e501c0ca2d` (META-observe, ALL
  GREEN at #71, killed at 80 in endgame wiring friction), and
  `dbab3dbf-569c-4d73-98ed-45cb46ff364a` (CMP-001, all gates green by
  #76, killed at 80 mid-doc-shuffle); reading analyses
  `docs/analyses/reading/16-22-55.md` (finding 1),
  `16-34-15.md` (item 3), `16-58-58.md` (finding 3), `15-02-35.md`
  (finding 1); `docs/analyses/reading/MASTER-BLINDSPOTS-2026-09-24.md`
  B.3; pre-v1.3 orchestrator at `c08bc55` marked such deaths
  `status: failed` outright.

## Observed

Workers finished their fixes, their last recorded test gate passed, and
the session died at the turn cap before the commit — three times in a
row on 2026-09-24 (B.3; plus attempt-2 f4ff3ebb dying with the work
staged). At the source revision the orchestrator's worker-death branch
(`rc != 0 and not committed`) requeues such deaths exactly like a
mid-work scratch failure:

- it never looks at whether the work was finished, so the near-miss is
  indistinguishable from a fresh failure;
- the retry worker starts from zero (16-22-55's finished work was
  "rescued only by the duplicate worker that followed" — a second full
  session, not a cheap verify-and-commit);
- the near-miss burns the same 3-attempt `needs_attention` budget, and
  the requeued item carries no priority and no salvage knowledge.

The prompt-rule half of the deliverable was already delivered at
`0f758d6` (v1.8, 2026-09-25 10:26:13, before this item was dispatched):
`WORKER_PROMPT` carries "Commit AT GREEN, immediately - before
docs/case-record polish. The deliverable is the commit; polish can follow
in a second commit." This case records the orchestrator half only; the
rule is now pinned by test so it cannot silently regress.

## Expected

On an abnormal worker exit without a commit, the orchestrator must scan
the dead worker's durable run record for a passing last test gate and,
finding one, treat the exit as an uncommitted-GREEN near-miss: requeue
with priority (the next cycle dispatches the cheap
verify-gates-and-commit retry ahead of every other open item) carrying a
salvage hint — instead of the plain generic requeue. Spec sources: queue
item CMT-001 deliverable; MASTER-BLINDSPOTS B.3 fix ("the orchestrator
should treat an uncommitted GREEN as an abnormal exit"); doctrine rule 2
("The deliverable is the commit, not the working tree").

## Executable assertion

`python3 -m unittest tools.test_self_improve` — a scripted two-cycle run
of `tools/self_improve.py:main()` (dispatch path real; the pragma process
replaced at the subprocess boundary) against the authentic-derived
fixture `tools/testdata/self_improve/worker-session-clk002-greengate.jsonl`
(verbatim slices of 06f603fb: header, first user message, and one
assistant/user pair carrying the recorded passing gate `Exit code: 0\nclean\nok  \tgithub.com/artpar/pragma/internal/query\t2.319s\n`;
only dispatch metadata is rewritten at staging). The worker dies twice
without committing; the cycle-2 dispatch must be the same item's retry
(prompt carries the NEAR-MISS salvage hint inside the mistake-memory
section), and the saved queue must hold the item at the FRONT
(`near_miss: true`, `last_error` noting the uncommitted GREEN, the hint
leading `mistakes`). The contrast test drives the same cycle with the
MM-001 fixture (last recorded gate is the RED `--- FAIL:` assertion) and
asserts the plain requeue survives: no hint, no near-miss flag, queue
order unchanged. `last_test_gate_passed` is additionally asserted True on
the green-gate fixture and False on the RED one.

## Proposed mechanism (one change)

Worker-death branch of `tools/self_improve.py` only. After the MM-001
mistake extraction, run `last_test_gate_passed(session)` over the same
discovered session transcript: the last tool result whose paired
tool_call ran a test command (`go test` / `python -m unittest`) counts
as the gate; it passed iff it is not an `is_error`, its content carries
a green marker (an `ok <pkg>` line, `--- PASS:`, or a lone `OK`) and no
`FAIL`/`--- FAIL:` line (the marker pair survives `| tail` pipelines
that mask exit codes). On a passing gate without a commit: set
`item["near_miss"] = True`, note the near-miss in `last_error`, prepend
`NEAR_MISS_HINT` (verify gates first; if GREEN, commit IMMEDIATELY —
doctrine rule 2) to the mistake-memory list the existing `{mistakes}`
prompt wiring already carries verbatim, and requeue the item at the front
of the queue (`queue.insert(0, ...)` = priority) instead of its original
position. Attempts still count and the needs_attention cap at 3 is
unchanged — three consecutive near-misses means the commit-at-GREEN rule
is systematically failing and needs a human, which is attention, not
requeue.

## Evidence that would refute

- The recorded cap-death sessions' last gates are not green (they are:
  verified by running the detector on the live session files — True on
  06f603fb, f4ff3ebb, de57a18f).
- The bg worker log could carry the gate evidence (it cannot on the
  failure mode: cap deaths leave it empty — MM-001, wire-proven; the
  detector reads the session transcript instead).
- The retry prompt could not carry the hint through the existing
  wiring (the scripted cycle shows it riding `{mistakes}` verbatim).

## Claim boundary (live data, 2026-09-25)

Detector run against the real `~/.pragma/sessions` tree: True on
06f603fb, f4ff3ebb (the near-miss whose fix files were STAGED at death),
de57a18f; False on RED-phase fixtures. One recorded near-miss is MISSED:
`dbab3dbf` (15-02-35) — its final action was a full-suite sweep whose
red is pre-existing documented failures, so the last-gate rule correctly
returns False and the item falls back to the HEAD-identical generic
requeue (a missed salvage, not a regression). Whether a real retry
worker then commits the salvaged work cheaply is a model-behavior claim
the loop's next real near-miss death will show. The payload's "scans
worker log" is implemented as a scan of the worker's durable session
transcript: the literal bg worker log is empty on cap deaths (MM-001
wire-proven evidence, trusted, not re-proven).

## Revision note (2026-09-25, this worker session)

Payload claims verified before the change: B.3's "three cap-deaths with
work finished and uncommitted (calls 71-78 of 80)" confirmed via the
reading notes and the session transcripts (a fourth recorded near-miss,
f4ff3ebb — dying with the fix staged — was found while grounding the
detector). "Prompt rule" confirmed already delivered at 0f758d6/v1.8; no
change made for it beyond the pinning test. "Scans worker log" refuted
as a literal bg-log scan (empty on the failure mode, MM-001); the scan
is implemented against the session transcript. Results: RED at `60f8a9f`
(4 failures for the stated reasons: `last_test_gate_passed`/`NEAR_MISS_HINT`
absent, scripted near-miss cycle requeuing plainly); GREEN at `c4fab51`
(9/9, including MM-001's four and the mid-work contrast). Adjacent:
`tools.test_run_swebench_pro_instance` 12/12 OK; `py_compile` clean; no
Go files touched, gofmt n/a. The near-miss requeue's own budget guard
(attempts -> needs_attention at 3) is intentionally unchanged and is
documented above.

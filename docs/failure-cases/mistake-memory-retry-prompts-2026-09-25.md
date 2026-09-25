# MM-001 — Orchestrator retry prompts carry no failure list (mistake-memory)

- Case ID: `MM-001`
- Source revision: `193383a` (2026-09-25, loop v1.8)
- Observed events: worker session `f4ff3ebb-303c-483a-a653-444b27919e01`
  (CLK-002 attempt 2, started 2026-09-24T16:34:15+05:30, killed at the
  80-call cap); reading analysis `docs/analyses/reading/16-34-15.md` item 3;
  `docs/analyses/reading/MASTER-BLINDSPOTS-2026-09-24.md` B.2; queue
  requeue path at `c4e8c36` (v1.3) and the speculative regex at `0f758d6`
  (v1.8, tools/self_improve.py, worker-death branch).

## Observed

CLK-002 attempt-2 stepped in exactly attempt-1's tool-error holes on the
same test file: apply_patch "update hunk ... is malformed" (wrong context
lines, call ~#35), the count-wall-clock-stamps want-2-vs-1 confusion, and
the empty patch "produced no content change" (call ~#38) — re-derived
from scratch because the retry prompt carried no record of them. The
requeue (v1.3) carried an attempt count and a prose salvage pointer
(`prior_attempt_note`: "Prior session 06f603fb has the analysis ... may
read its session transcript for salvageable design decisions") — a
pointer, not a failure list.

At HEAD (`0f758d6`) the requeue branch greps the worker log
(`PRAGMA_BG_SESSION_LOG`) for three apply_patch/exit-status patterns.
That cannot fire on the observed failure mode, verified two ways:

1. The bg log receives only `TextEvent` final text and user-message
   events (`internal/cli/run.go` bg-log wiring: `TextEvent` -> `out`,
   tool results go to stderr); on a cap death the final text never
   arrives — `.self-improve/20260924-154746-c3-worker.log`,
   `20260924-163415-c3-worker.log`, `20260924-163415-c15-worker.log`,
   `20260925-102627-c2-worker.log` are 0 bytes. Zero patterns match,
   `item["mistakes"]` stays unset, and the retry prompt renders
   `["(first attempt)"]` — the same zero-memory state as v1.3.
2. Run against all 49 existing worker/critic/reader logs, the patterns
   match only narrative prose (2 false positives, e.g. the PACT-001
   report text "produced a misleading \"did not match current file
   content\" error"), never a tool error.

The tool errors themselves are durably recorded in the worker's session
transcript `~/.pragma/sessions/<id>.jsonl` as `tool_result` parts with
`is_error: true` (pragma's own error classification:
`internal/model/content.go` `IsError json:"is_error,omitempty"`, set by
`internal/query/provider_tools_loop.go` for unknown tools, Bash
`ExitCode != 0`, apply_patch verification failures, etc.). Verified in
session f4ff3ebb: 13 `is_error` tool results, including both B.2
apply_patch failures.

## Expected

The retry prompt for a failed worker must carry that worker's own tool
errors verbatim in the mistake-memory section (doctrine rule 5), not
`["(first attempt)"]`. Spec source: doctrine rule 5 + MASTER-BLINDSPOTS
B.2 fix direction ("failed attempts must leave a mistakes list that the
retry prompt carries verbatim").

## Executable assertion

`python3 -m unittest tools.test_self_improve` — a scripted two-cycle
run of `tools/self_improve.py:main()` (dispatch path real; the pragma
process replaced at the subprocess boundary) against the authentic-derived
fixture `tools/testdata/self_improve/worker-session-clk002-attempt2.jsonl`
(slices of session f4ff3ebb, byte-identical tool results, header
`created_at` rewritten to the scripted dispatch time). The worker "dies"
twice without committing; the cycle-2 dispatch prompt must contain both
recorded apply_patch failure strings inside the
"Prior attempt mistakes to avoid repeating verbatim" section:

- `apply_patch verification failed: update hunk for internal/query/wall_clock_test.go line 62 is malformed: "\t\tvar texts []string"`
- `apply_patch verification failed: internal/query/wall_clock_test.go produced no content change`

## Proposed mechanism (one change)

On the worker-death requeue path, replace the final-text regex with
extraction from the failed worker's own session transcript: locate the
session file whose header `work_dir` matches the repo, whose `created_at`
is at/after the dispatch timestamp (2s skew tolerance), and whose first
user message carries
the WORKER prompt marker and this item's `Queue item <qid>:` line (so
critic/reader/operator sessions, and workers dispatched for a different
item by a concurrent orchestrator run, are never mistaken for the
failed worker); lift its `is_error` tool results into
`item["mistakes"]` (each summarized to its first line plus, for
`Exit code: N` Bash results, the following output line; tool name and
command prefix from the matching `tool_call` parts; deduped, capped at
8 per attempt and 12 total across attempts). The existing `{mistakes}`
wiring in `WORKER_PROMPT` then carries them verbatim into the retry
prompt.

Fixture staging (documented in
`tools/testdata/self_improve/README.md`): the recorded session's
dispatch metadata only — header `created_at`/`work_dir` and the first
user message's `Queue item CLK-002:` line — is rewritten for the
scripted cycle; the tool results stay byte-identical to the recording.
The existing `{mistakes}` wiring in `WORKER_PROMPT` then carries them
verbatim into the retry prompt.

## Evidence that would refute

- Session files do not persist `is_error` (they do; verified in
  f4ff3ebb).
- The retry prompt already carried the failures (it renders
  `["(first attempt)"]` when the worker log has no pattern matches).
- The scripted cycle cannot drive `main()` end to end (it can; the
  pragma binary is the only subprocess boundary).

## Revision note (2026-09-25, worker session 2d4a25ff)

Payload claims verified before the change: B.2 repetition confirmed via
the reading note and the session tool results; "salvage pointers existed
but were not failure lists" confirmed at the v1.3 queue item
(`prior_attempt_note`). No payload claims refuted. Changed: the
worker-death branch of tools/self_improve.py only; the old final-text
regex removed (it never fired on a real cap death and matched only
narrative prose).

Results: RED at `193383a` (all four tests fail: scripted cycle requeues
with `["(first attempt)"]`, extraction/discovery/marker mechanisms
absent); GREEN at `ece537c` (4/4). Adjacent: `test_run_swebench_pro_instance`
12/12 OK; `test_harbor_pragma_agent` blocked pre-existing by the missing
external `harbor` package (infrastructure, not this change — the failing
import chain lives in unmodified `tools/harbor_pragma_agent.py`). Live
data: qid-scoped discovery returns the PACT-001 worker's session
(99409da1, 8 real tool errors) for `PACT-001` and this worker's own
session for `MM-001` from the real `~/.pragma/sessions` tree. No Go
files touched (no gofmt run). Claim boundary: this repairs the
orchestrator's mistake-memory extraction and transport, verified by a
scripted cycle and live session data; whether retry workers then avoid
the listed mistakes is a model-behavior claim that needs the loop's next
real retry to observe.

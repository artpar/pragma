# self_improve test fixtures

## worker-session-clk002-greengate.jsonl

Provenance: verbatim slices of the recorded pragma session
`~/.pragma/sessions/06f603fb-ccc9-4cff-8046-6a4323b3c8ee.jsonl` — the
CLK-002 attempt-1 WORKER dispatch of 2026-09-24T16:22:55+05:30, killed
at the 80-call cap with the work fully done and the gates green at call
78 of 80 and nothing committed (CMT-001's B.3 headline event; reading
analysis `docs/analyses/reading/16-22-55.md` finding 1).

Kept: the header line, the first user message (the worker dispatch
prompt), and one assistant/user pair carrying the session's successful
wall-clock gate — the Bash tool call
`gofmt -w ... && go test ./internal/query -run 'TestProviderToolsLoopWallClock|...' -count=1 2>&1 | tail -3`
(call id call-6a2d6736-...) and its byte-identical tool result
`Exit code: 0\nclean\nok  \tgithub.com/artpar/pragma/internal/query\t2.319s\n`
with no `is_error` flag - the "uncommitted GREEN" signature CMT-001's
orchestrator scans the death-path session transcript for.

Rewritten: nothing in the fixture itself (the header keeps the recorded
`created_at` 2026-09-24T16:22:55.4625+05:30 and the real repo `work_dir`);
staging in tools/test_self_improve.py (stage_worker_session with
`fixture=`) restamps `created_at` to the scripted dispatch time and
rewrites the first user message's `Queue item CLK-002:` line to the
scripted item id. Only dispatch metadata differs from the recorded
session; tool results never differ.

Source session retained on this machine (never pushed): original
available for re-derivation. Case record:
docs/failure-cases/commit-at-green-near-miss-requeue-2026-09-25.md.

## worker-session-clk002-attempt2.jsonl

Provenance: verbatim slices of the recorded pragma session
`~/.pragma/sessions/f4ff3ebb-303c-483a-a653-444b27919e01.jsonl` — the
CLK-002 attempt-2 WORKER dispatch of 2026-09-24T16:34:15+05:30 that was
killed at the 80-call cap (MM-001's observed failure: its retry prompt
carried no failure list). This is the session the reading analysis
`docs/analyses/reading/16-34-15.md` item 3 cites for attempt-2 repeating
attempt-1's apply_patch mistakes.

Kept: the header line, the first user message (the original v1.3 worker
dispatch prompt), and three assistant/user message pairs carrying the
recorded errored tool results — the two B.2 apply_patch verification
failures and one Bash RED test-failure (`--- FAIL:`) — each
byte-identical to the source session, including `is_error: true` flags,
tool call ids, names and inputs.

Rewritten: header `created_at` is a placeholder
(`1970-01-01T00:00:00+00:00`); staging in tools/test_self_improve.py
restamps it to the scripted dispatch time, sets `work_dir` to the
repo under test, and (with a qid) rewrites the first user message's
`Queue item CLK-002:` line to the scripted item id. Only dispatch
metadata differs from the recorded session; tool results never differ.

Source session retained on this machine (never pushed): original
available for re-derivation. Case record:
docs/failure-cases/mistake-memory-retry-prompts-2026-09-25.md.

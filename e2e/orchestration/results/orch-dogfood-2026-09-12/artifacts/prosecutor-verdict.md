Findings:
- The task is review-only: adversarially verify the TUI-001 mechanism across
  range 413122a^..948431c, record findings under /tmp/pragma/, and change no
  repository file. The implementer report at /tmp/pragma/implementer-report.md
  satisfies that requirement: it addresses all four required checks, records a
  weighed deviation for check 3, and preserves validation evidence.
- Requirement coverage verified: check 1 (text-bearing responses stay
  collapsed), check 2 (reload promotion gated on a trailing all-thinking
  assistant message, including empty-content, user-role, and thinking+text
  boundary probes), check 3 (mid-turn queued prompt interleaving), check 4
  (only end_turn promotes; other stop reasons keep their notices). Out-of-scope
  items (ORCH gates, MCPINJ probe, full suite) were explicitly named and
  skipped per the brief, as required.
- Independent validation, run by me with exit codes preserved:
  - go test ./internal/tui -count=1 -run 'TestThinkingOnly|TestThinkingThenText|TestTextlessFinal|TestReloadShowsTrailing|TestQueuedPromptEventRenders' -v
    rc=0, 5/5 PASS (log: /tmp/pragma/prosecutor-focused-test.txt).
  - go test ./internal/tui -count=1 rc=0, "ok github.com/artpar/pragma/internal/tui 3.214s"
    (log: /tmp/pragma/prosecutor-tui-suite.txt).
  - go build ./... rc=0, no output.
- Diff inspection: git diff 413122a 948431c -- internal/tui/ contains only
  added observe.GlobalTrace instrumentation lines and removed TUI-001/INT-001
  comments; no logic lines changed, so the reviewed mechanism at 948431c is
  behaviorally identical to 413122a as the report claims.
- Claim 1 mechanism confirmed by grep: forceShow is only written at
  model.go:536 inside promoteTrailingThinking and only read at model.go:432;
  finalResponseHadText is only set at handlers.go:255 (non-whitespace
  TextEvent) and reset at handlers.go:387 (ToolResultEvent request boundary)
  and handlers.go:770 (submitPrompt); the promotion guard is at handlers.go:441.
- No generated, derived, schema, config, fixture, route, or migration files
  are in scope: grep for "Code generated"/"DO NOT EDIT" in internal/tui/
  returns nothing; the range's other files are docs and e2e result artifacts
  adjacent to, not part of, the reviewed mechanism.
- Review-only discipline confirmed: git status shows zero tracked
  modifications and zero staged changes; the only untracked path
  (e2e/orchestration/results/orch-dogfood-2026-09-12) is dated Sep 12
  17:59:46, predating this session, and is not in the review range. The
  boundary-input scratch test was run via go test -overlay mapping a repo path
  to /tmp/pragma/zz_scratch_review_test.go, so no repo file was created or
  edited. The scratch run's rc=1 is disclosed and explained in the report as a
  deliberately strict scratch assertion on pre-existing render behavior, not a
  hidden failure; the underlying producer command and full output are named.
- Deviation weighed, not a blocker: the queued-mid-thinking case promotes only
  the trailing thinking run, leaving the earlier same-response thinking
  segment collapsed behind the queued prompt's non-whitespace segText. This is
  a coverage/behavior gap, but the mechanism documents promotion of the
  "trailing run" only, the queued prompt is itself operator-visible text, and
  the brief explicitly forbids fixing it in this review-only task. The report
  records it with a suggested future gate, which is exactly the required
  deliverable. Minor notes (StopPauseTurn lacks a notice branch, unreachable
  StopMaxTokens notice branch, reload scans all segments) are recorded
  observations, not concrete task risk.

Required repair:
None. The task is review-only and is complete; the recorded check 3 gap and
minor notes are findings for future gating, not repairs to be made now.

Decision:
APPROVE

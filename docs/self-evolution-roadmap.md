# Pragma self-evolution roadmap

Milestone ladder for evolving the harness through evidence-gated changes.
Methodology: `agent.md`. Case records: `docs/failure-cases/`. A milestone is
done when its exit gate holds, not when its code is written.

## Done

- **M0 — Baseline measurement (2026-09-11).** Build 4.3s (`make build`),
  full suite 16-18s, package tests 0.5-2s, `replay --events`/`metrics`
  0.02-0.1s. Loop speed was adequate; instrument coverage was not.
- **M1 — Instrument repair (2026-09-11, commit `8bf3061`).** OBS-001/002/003:
  recordings load across event-kind drift; recording directories resolve;
  metrics guides on session-store files. Verification record in
  `docs/failure-cases/visibility-instruments-2026-09-11.md`.
- **M2 — Flaky gate root-caused and fixed (2026-09-11).** RTY-001: the
  openrouter classifier substring-matched status codes against the full
  error message, so an ephemeral httptest port or an error body containing
  "500"-like digits misclassified a permanent 400 as retryable, burning the
  10-attempt budget (~183s of backoff — the observed suite flake).
  Reproduced deterministically through the production classifier (baseline
  RED), fixed via structured `StatusCode` extraction (candidate GREEN),
  boundary replay verified across 10 random ports, full suite clean.
  Record: `docs/failure-cases/openrouter-retry-classification-2026-09-11.md`.
  Follow-ups RTY-002..005 (same mechanism in morphllm/openai/google/groq)
  remain open.

## Next

### M2 — Stabilize the flaky gate

`internal/provider/openrouter` `TestRecordedReasoningSurvivesSessionReplay`
failed once under parallel full-suite execution: `requests = 12, want 2` after
183s of retries; passes in isolation (baseline and candidate) and in a clean
full run. Suspected: local httptest latency under package parallelism trips
the bounded retry policy.

- Entry: reproduce under parallelism with variance captured across runs
  (unconfirmed until reproduced).
- Exit: deterministic passes across N consecutive parallel full-suite runs,
  or the flake is localized to a documented environmental cause.
- Why first: an unreliable gate makes every later claim unfalsifiable.

### M2b — Classify the captured session-start failure

Recording `~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa/
20260911T075358.350980000Z.jsonl`: `APIRequestFailed
error=request_failed retryable=false` ~1.1s after `APIRequestStarted`, then
`MCPServerDisconnected`/`MCPServerFailed`. Adjacent to the recent
retry-authorization commits (`60bc3eb`, `a20443c`).

- Entry: trace the failure through the raw HTTP capture (if present) to the
  retryability classifier.
- Exit: documented verdict — correct classification, or a recorded defect
  with the classifier transition that was wrong.

### M3 — Registry hygiene

Stale `~/.pragma/active-sessions/` entries never expire (e.g. `-1.json`,
gogent-era, pid -1, status `starting`). Entry: a recorded expectation for
entry expiry. Exit: deterministic test plus cleanup.

### M4 — Capability restorations (one case per mechanism)

From branch `worktree-wt-1776755333195` (commit `062c203`): MCP tool injection
(4 servers verified connectable; 53 JetBrains tools), brave websearch tool,
subagent tool. Each restoration: recorded absence/failure first, mechanism
ported second, gate third. Capability claims need real local integration
evidence; task-success claims additionally need held-out evaluation.

### M5 — Held-out self-evaluation (only if a score claim is made)

Frozen candidate, declared task set, matched budgets, repetitions, uncertainty
reported. Do not run broad benchmarks to discover whether a change works.

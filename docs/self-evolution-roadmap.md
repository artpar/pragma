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
- **RTY-002 — Structured classification ported to morphllm, the active
  route (2026-09-11).** Same substring mechanism as RTY-001, evidenced by
  the authentic 2026-09-11T07:53:58Z capture whose correct non-retryable
  verdict was luck of message content, plus the dropped structured
  Retry-After on typed rate limits. Baseline RED on both error shapes
  (raw SDK and any-llm wrapped), candidate GREEN, structured RetryAfter
  carried, full suite clean. Record:
  `docs/failure-cases/morphllm-retry-classification-2026-09-11.md`.
  RTY-003/004/005 (openai, google, groq) stay recorded-latent: no observed
  authentic failure on those routes yet, so they are not preemptively fixed.

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

**Resolved 2026-09-11 — classification correct; no open defect.** The
recording's `APIRequestFailed` message decodes to a MorphLLM 400:
`Validation: The 'stream_options' field is only allowed when 'stream' is set
to true.` Retrying an invalid request is pointless, so
`retryable=false, request_failed` was the correct verdict. The underlying
request defect was already fixed today in the MorphLLM adapter work
(`docs/failure-cases/morphllm-provider-support-2026-09-11.md`, MORPH-001
verification record describes this exact capture — the probe session
`Reply exactly MORPH_PRAGMA_OK.`); the current session running on
`morph-glm53-744b` is the live proof. The trailing `MCPServerFailed
(planning, context canceled)` is the expected cascade of session teardown,
not an independent defect. Residual finding: the authentic morphllm error
message flows through the same substring classifier family as RTY-001 —
RTY-002 (port the structured-status fix to morphllm, the active route) is
now evidence-backed by real traffic and is the next open case.

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

**Done 2026-09-11 (REG-001).** The registry never garbage-collected:
the observed five-month-old gogent-era `-1.json` orphan was invisible to
every read path and unreachable by every cleanup path. `Register` now
refuses `PID <= 0`; `ListProcesses` sweeps only files that provably carry
a registry `pid` field and are invalid or dead-plus-stale; anything
unrecognized is left untouched. Package got its first tests; the authentic
gate swept the real orphan via `./bin/pragma sessions`. Record:
`docs/failure-cases/background-registry-hygiene-2026-09-11.md`.

### M4 — Capability restorations (one case per mechanism)

From branch `worktree-wt-1776755333195` (commit `062c203`): MCP tool injection
(4 servers verified connectable; 53 JetBrains tools), brave websearch tool,
subagent tool. Each restoration: recorded absence/failure first, mechanism
ported second, gate third. Capability claims need real local integration
evidence; task-success claims additionally need held-out evaluation.

**M4.1 — MCP tool injection restored (2026-09-11, MCPINJ-001).** The
provider-tools loop sent only `Bash`+`apply_patch` while 4 MCP servers
(129 advertised tools) sat connected: `providerToolDefs()` was hardcoded and
`mcp__` calls answered `unknown tool`. Baseline RED reproduced on the wire
(real session boot, raw HTTP capture: 4 servers `connected`, tools still
2, mcp call errored); the branch's MCPToolAdapter mechanism was ported
main-shaped (mcp/adapter.go ToolDef conversion + Manager.ToolDefs +
Manager.CallMCPTool with original-name recovery, EngineConfig hooks, loop
injection + dispatch); candidate GREEN: 129 defs from all 4 servers on the
wire, real MCP execution through the loop, pairing intact; pragma mode
unchanged; full suite clean. Record:
`docs/failure-cases/mcp-tool-injection-2026-09-11.md`.

**M4.2 — Brave websearch tool restored (2026-09-11, WEB-001).** The model
had no web search capability (131 tools, none of them WebSearch; the model
had to shell out). Baseline RED on the wire (real boot: WebSearch call →
`unknown tool`); the branch's WebSearch tool ported main-shaped
(`internal/tools/websearch`: tool def, validation, `site:` filters,
blocked-domain filtering, Brave client; `EngineConfig.WebSearch` hook;
loop injection + dispatch; key resolution env → credentials in
`RegisterTools`). Candidate GREEN hermetic (local Brave stub, wire
captured, token redacted) plus the live contract gate: one real Brave call
through the full loop path (HTTP 200, 5.1KB formatted results on the
wire). rawcapture now redacts `X-Subscription-Token` (found via a
would-have-leaked capture, deleted before commit). Record:
`docs/failure-cases/websearch-tool-restoration-2026-09-11.md`. Remaining
M4: subagent tool.

### M5 — Held-out self-evaluation (only if a score claim is made)

Frozen candidate, declared task set, matched budgets, repetitions, uncertainty
reported. Do not run broad benchmarks to discover whether a change works.

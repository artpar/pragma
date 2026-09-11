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

**Resolved 2026-09-11 without a second observed flake.** RTY-001 removed
the mechanism (misclassified-retry burn) that turned transient local
httptest latency into 183s of budget exhaustion, and the suite has since
run clean under parallelism five consecutive times on 2026-09-11 (exit 0,
29 packages ok — including TURN-001's candidate gates). The suspected
environmental trigger was never reproduced; if the flake returns, reopen
with variance captured across runs.

### Live observations (post-M4 dogfood, 2026-09-11) — notes, not cases

- **Wire size of the injected toolset:** the first live session on the
  restored build (4 MCP servers connected, 132 tool defs) measured
  request-1 input at 20,294 tokens against 826 on the pre-injection
  binary on the same route — ~19.5K input tokens of tool-def overhead on
  **every** request, parent and sub-agent fork alike (the RTY-scan
  sub-agent's fresh conversation also started at ~19.7K). Through 136
  completions and 113K input tokens: zero request failures, zero retries.
  The one observed token-limit event on the live route — the morphllm
  router's `raw_isl_tokens` policy (medium class, 200,000 raw, hit at
  291,066 raw ≈ 2.8× the tokenized count) — occurred on the **old**
  binary and was driven by conversation growth, not tool defs; it was a
  retryable 429, correctly classified with structured Retry-After
  (RTY-002 working live). No harm event attributed to tool-def wire size
  yet, so per-server MCP caps stay evidence-gated. To watch: with ~19.5K
  extra per request, sessions reach the router's raw-token policy at
  proportionally earlier conversation depth.
- **RTY-003/004/005 (openai/google/groq): stay recorded-latent.** A
  read-only scan of logs, session stores, and recordings (2026-09-11)
  found no authentic failures on those routes: the openai-adapter-tagged
  failures are openrouter-route events (RTY-001's domain, already fixed),
  groq has never been used, google has not been exercised since June.
  Absence of route traffic is not evidence of defect; the ports wait for
  real failures.
- **Planning-MCP workflow friction (minor, no case):** completing a
  planning task requires `submit_for_review` first, and submitting
  requires `in_progress` first — two extra round trips per task. Dogfood
  observation only.

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

**M4.3 — Subagent tool restored, sync fork (2026-09-11, SUB-001).** The
model could not delegate a bounded task to a fresh sub-conversation (no
`Agent` tool; calls answered `unknown tool`). Baseline RED on the wire
(real boot, scripted Agent call → `unknown tool`); mechanism: an Agent
tool in the provider-tools loop that forks a fresh conversation via
`ForkFreshConversation` (shared provider/bus/costs, sub config
provider-tools + recursion guard + nil auto-compaction per #27794) and
returns the sub-agent's final text in the branch's JSON envelope.
Candidate GREEN on the wire: the sub-agent's request is a fresh
conversation carrying the full sibling toolset minus Agent (129 MCP defs
included), and the parent's next request pairs
`{"status":"completed",...,"result":...}`. Pragma mode unchanged; full
suite clean. Background/teammate/worktree/structure-graph variants remain
follow-up mechanisms, each needing its own case. Record:
`docs/failure-cases/subagent-tool-restoration-2026-09-11.md`.

**M4 complete (2026-09-11).** All three declared restorations are gated
and recorded. M5 (held-out self-evaluation) stays dormant unless a
task-success claim is made.

**TURN-001 — Turn-budget warning restored ahead of the cap
(2026-09-11, commit `e05726f`).** First case driven by live dogfood
evidence: `DefaultMaxTurns` killed the previous operator session twice
mid-task with no advance signal (log `2026-09-11T18-50-33.jsonl`: two
exactly-100-completion segments ending `stop_reason=tool_use`; the model's
recorded thinking shows it learned of the termination only afterward) and
killed three Terminal-Bench 2.1 tasks the same way (audit record). Of the
three options (warn-at-N / continuation / override) the case selects
warn-at-N: continuation already exists (the conversation persists across
the loop's death; a new prompt or `--resume` continues it) and the
override already exists (`--max-turns`). Mechanism (one): a user-role
turn-budget notice appended once, `maxTurns/10` turns before the cap
(floored at 5, half the budget for budgets below 10), naming
used/remaining and instructing wrap-up or handoff. Cap, error text, flags
unchanged; pragma loop mode untouched (separate budget mechanism, own
case if ever evidenced). Gates: baseline RED (scripted 20-turn loop: zero
notices where one is due, cap-enforcement assertions passing), candidate
GREEN (notice exactly once at the warn iteration, retained in history,
pairing validation intact, cap still enforced), pragma-mode adjacent
clean, full suite exit 0 across 5 consecutive parallel runs. Live-effect
boundary: injection is proven; that a model *acts* on the notice is not
claimed. Record:
`docs/failure-cases/turn-budget-warning-2026-09-11.md`.

### M5 — Held-out self-evaluation (only if a score claim is made)

Frozen candidate, declared task set, matched budgets, repetitions, uncertainty
reported. Do not run broad benchmarks to discover whether a change works.

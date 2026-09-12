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
  sub-agent's fresh conversation also started at ~19.7K). Through the
  session's full 147 completions and 121K peak input tokens: zero request
  failures, zero retries.
  The one observed token-limit event on the live route — the morphllm
  router's `raw_isl_tokens` policy (medium class, 200,000 raw, hit at
  291,066 raw ≈ 2.8× the tokenized count) — occurred on the **old**
  binary and was driven by conversation growth, not tool defs; it was a
  retryable 429, correctly classified with structured Retry-After
  (RTY-002 working live). No harm event attributed to tool-def wire size
  yet, so per-server MCP caps stay evidence-gated. To watch: with ~19.5K
  extra per request, sessions reach the router's raw-token policy at
  proportionally earlier conversation depth.

### Live observations (2026-09-11/12 night session — the session that
### committed CLK-001 and PAR-001) — notes, not cases

- **TURN-002 live effect confirmed:** the widened notice fired at turn 80
  of 100 with 20 remaining in that very session (in-conversation notice
  ~00:29 IST) — the window change works live; the wrap-up had 20 turns
  of room and completed cleanly. (TURN-003 escalation evidence would be
  a session dying mid-task *despite* the 20-turn window.)
- **Router 429s are queue-class, not raw-token:** three retryable 429s
  hit that session (23:46, 00:03, 00:25) — all `router policy class
  "small"/"whale" queue requests limit reached` concurrency-admission
  limits (small: 8, whale: 2), all classified retryable and retried to
  success (RTY-002 working live). Zero `raw_isl_tokens` events: the
  wire-size harm event has still not occurred; the per-server-cap
  mechanism stays evidence-gated. That session peaked at 172.6K input
  tokens (106+ completions) with the router moving requests to the
  whale class as the conversation grew.
- **Interrupt-on-message-arrival spend waste (observation, no case):**
  four `context canceled` request failures in that session — one was the
  PAR-001 evidence batch (operator interrupt during tool execution); the
  other three each preceded a user-message acceptance by seconds
  (submitting a message while a model request is in flight cancels it).
  Each canceled request forfeits its full input spend (40-120K tokens
  each at this route's 2-4-minute thinking latency — roughly 350K input
  tokens wasted across the session). The semantics are reasonable; the
  cost is a long-latency × interrupt interaction. A "queue the message
  behind the in-flight request" or partial-result-preservation
  mechanism would need its own case if the operator wants it.
  **Corrected and resolved 2026-09-12 (INT-001):** the code trace showed
  submission never canceled the request — it was rejected-and-dropped
  (busy) and the operator's separate interrupt did the forfeiting;
  mid-turn input now queues into the conversation instead.
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
- **TURN-001 live effect (2026-09-11, first live run) — notice fired,
  wrap-up incomplete, session still died at the cap.** At turn 90 of a
  100-turn operator session the in-conversation notice arrived ("90 of 100
  … 10 remain"); the model acknowledged it, stopped exploration, and
  prioritized finishing the in-flight TOK-001 documentation + commit. The
  10 remaining turns were not enough: two `apply_patch` retries burned
  margin, and the loop terminated at turn 100
  (`provider tools loop exceeded maximum of 100 turns`,
  log `~/.pragma/logs/2026-09-11T21-21-59.jsonl`, notice ~21:53, death
  ~22:06) with the commit undone — the operator re-prompted and the
  preserved conversation continued. Honest reading: the mechanism worked
  (injection + visibility + model orientation all observed), but notice
  efficacy for *completing* wrap-up is unproven — orientation ≠ finishing
  when the in-flight step exceeds the window. Per the case's own boundary,
  this was recorded before acting: TURN-002 (the window widening below)
  was authorized the same day by the operator's delegation, and the
  existing override (`--max-turns`) remains the operator's tool for
  known-long tasks. Session wire tally for the watch: 100 completions +
  3 in the continuation, ~121.6K peak input tokens, zero request
  failures/retries across the whole run — wire-size watch stays open, no
  harm event yet.

### Live observations (2026-09-12 midday session — the INT-001/INT-002/
### TURN-003 dogfood) — notes, not cases

The operator-directed live-effect run for the three morning mechanisms, on
the `d695a05` build (binary rebuilt through the green-boot adjacency gate:
133 tools on the wire — Bash/apply_patch/WebSearch/Agent + 129 `mcp__` defs
from 4 servers, real MCP execution, pairing intact, 2-request scripted
completion; capture under `e2e/mcp-injection/results/green-int001-turn003/`).
Route morph-glm53-744b, interactive provider-tools, no `--max-turns`. Log:
`~/.pragma/logs/2026-09-12T12-27-56.jsonl`.

- **INT-002 live behavior reframed — an upstream key coercion, not the
  executor alias.** Deliberate probes — one Bash call with the command
  under `command` (the INT-002 alias) plus three under unrecognized keys
  (`zzz`, `foo`, `qqq`; values preserved exactly) — all executed, and the
  response log shows every one arriving as `{"cmd": ...}`. The repo contains
  no coercion: the anyllm translate and the streaming accumulator pass the
  wire `tc.Function.Arguments` through verbatim
  (`internal/provider/anyllm/translate.go`,
  `internal/provider/accumulate.go`), and the INT-002 diff is
  executor-only — so the rename happens before pragma sees the arguments
  (router-side tool-call parsing or the emission layer; attribution
  unresolved from the session log). Live reading: the executor alias fired
  zero times tonight; "alias executes" live-green is the coercion's work,
  not INT-002's. The alias + key-echo error stay gate-proven at the
  executor and stay needed: the 2026-09-12T00-46-33 night session and the
  2026-08-30 TB instance show raw non-schema keys do reach the executor on
  this route sometimes — the coercion is conditional (request size/class is
  the live hypothesis: tonight ran 39K+ input tokens from request 1; the
  night session started small and grew into the whale class mid-session).
  The case's belt-and-suspenders reshape bullet is now partially realized;
  watch, don't fix — nothing failed, and the two layers are tolerance
  layers of different reach.
- **TURN-003 — expected absence confirmed.** Zero turn-budget notices
  across the whole session (uncapped run; `turnBudgetWarnTurn(0)` defines
  no warning turn), the loop healthy through its natural end-turn, the
  sub-agent cap guard pinned by its gate. No-cap → no-window is the
  designed state; recorded as expected, not a bug, per the operator's
  framing.
- **INT-001 — live-green; park/drain exercised mid-execution.** The
  wrap-up surfaced that the operator relays prompts rather than
  improvising input, so the live test was orchestrated explicitly: with
  the turn active and a `sleep 45` tool call executing (dangling
  tool_use tail), the operator submitted `INT001-LIVE-TEST` at 13:35:05.
  Observed end to end: no rejection and no interrupt (zero
  `RejectedPromptEvent`, zero `APIRequestFailed` — the sleep ran to
  completion), the message parked behind the dangling tail, and the
  drain fired in the designed order — results, companion, queued message
  appended as three microsecond-aligned `MessageAppended` events at
  13:35:49.256 — the queued message carrying its 13:35:05 submission
  stamp — then delivered on the very next request, where the model
  received it mid-turn (log `~/.pragma/logs/2026-09-12T12-27-56.jsonl`).
  One observability note, not a defect: the `QueuedPromptEvent` emitted
  for TUI rendering is not persisted to the session log, so log-side
  proof of queueing is the append trio plus the request wire, not the
  named event. Whether the model *acts* on queued input remains the
  live-effect boundary, exactly as the case scoped it.
- **Adjacent, CLK-001:** wall-clock companions present after every
  tool-results batch (one per batch, stamped, model-visible between
  requests), prompt stamped; results messages stayed tool-results-only;
  16/16 requests completed with zero failures and zero retries — pairing
  undisturbed.
- **Adjacent, PAR-001:** sibling calls visibly concurrent — a 4-call batch
  completed within one second (12:31:43), a 5-call batch likewise
  (12:35:51); no serialization stalls behind slow siblings.
- **Bash executor shell semantics (dogfood note):** the provider-tools
  Bash runs commands with errexit+pipefail (the executor's shellrun
  options) — diagnostic compounds abort at the first zero-match grep
  (exit 1) or a head-closed pipe (exit 141). Benign, documented options;
  structure commands with `|| true` guards accordingly.

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

**TOK-001 — Token-limit truncation no longer classified as successful
completion (2026-09-11).** agent.md's second named regression, evidenced
by recorded events: the TB-2.1 `circuit-fibsqrt` and `dna-assembly` agent
logs end at `stop=max_tokens` with silent exit 0, and GLM-5.3 D01-v4
`regex-log` "emitted successful turn completion and exited 0" on a
truncated first response (no deliverable, verifier 0.0) — confirmed as a
defect on 2026-09-05 and unimplemented since. A tool-free `StopMaxTokens`
termination in the provider-tools loop is now an honest error termination
(`final response truncated by max_tokens output limit …; the conversation
is preserved`), exit != 0, partial content preserved for
re-prompt/`--resume` — classified exactly like the turn cap. No
auto-continuation (E003 stays reverted per the `6f3271c` boundary);
truncation-with-tool-calls still executes; pragma mode unchanged;
sub-agents pair truncations as failures; the Harbor adapter recognizes
the truncation error like the turn cap so verifiers still run. Gates:
engine baseline RED (3 tests) / candidate GREEN, real-boot wire gate
(`toklimit` profile: baseline exit 0 vs candidate exit 1, one request, no
restart), full suite clean. Record:
`docs/failure-cases/token-limit-termination-classification-2026-09-11.md`.

**TURN-002 — Turn-budget warning window widened to maxTurns/5
(2026-09-11).** Driven by the first live run of TURN-001: the notice fired
at turn 90, the model oriented to wrap-up, but 10 remaining turns were not
enough for a realistic wrap-up (in-flight docs + commit + one retry cycle)
and the loop died at 100 with the commit undone; the operator re-prompted
and explicitly handed the window policy to the harness ("you killed
yourself again … it's all on you"). One mechanism: the window in
`turnBudgetWarnTurn` goes from `maxTurns/10` to `maxTurns/5` — 20 remaining
at the default 100 (notice at turn 80) — with the small-budget fallback
and every TURN-001 pinned small-budget value byte-identical. Notice text,
single injection, cap, error text, flags, pragma mode unchanged. Gates:
window-table baseline RED (100→90 vs want 80) / candidate GREEN, a
100-turn loop gate (notice exactly once on request 81, "80 of 100 …
20 remain", cap exact), real-boot tbgreen rerun on the rebuilt binary
(small-budget wire unchanged, `results/candidate-tbgreen-turn002/`),
full suite clean x3 parallel. Escalation (a second, stronger notice near
the cap) stays out of scope — no session has yet died *with 20 turns of
warning*; that would be the TURN-003 evidence. Record:
`docs/failure-cases/turn-budget-window-2026-09-11.md`.

**CLK-001 — Wall-clock append stamps made model-visible
(2026-09-11).** Driven by live operator evidence on the same session that
committed TURN-002: a four-call batch sat ~2m13s behind its first call
(the sync `Agent` fork under sequential dispatch) and the operator
interrupted — in the conversation the model sees, none of it carried a
clock or a duration ("you didnt even realise you were stuck"); the
operator then requested timestamps on every appended message as the fix.
Structural root: `model.Message.Timestamp` is set on every append but no
adapter serializes it — the model had no wall clock at all. One mechanism:
a `[pragma wall-clock <RFC3339>]` first line inside text-only appends
(prompt, turn-budget notice) and a companion user message after each
tool-results batch — the position and wire shape the TURN-001 notice
proved live, keeping the results message tool-results-only (a text part
inside it would serialize before its tool results, an unproven
user-before-tools order on this route). Assistant messages unstamped;
pragma loop mode untouched; no durations or derived values. Gates: unit
baseline RED (`internal/query/wall_clock_test.go`: no stamps, loop
completes) / candidate GREEN; full suite ×3 parallel; real-boot wire
gates clkred/clkgreen plus the MCPINJ-001 `green` adjacent set on the
same candidate capture (129 `mcp__` defs, 4 servers, real MCP execution,
pairing intact — stamps ride without disturbing any prior wire gate);
wire excerpt: request 2 closes `user: [pragma wall-clock
2026-09-12T00:01:31+05:30]`, 15s after the prompt stamp. Claim boundary:
injection and visibility proven; that the model uses the clock to manage
blocking is a live-effect observation. Record:
`docs/failure-cases/wall-clock-stamps-2026-09-11.md`.

**PAR-001 — Sibling tool calls in one assistant turn now execute
concurrently (2026-09-12).** Driven by the same live event as CLK-001: a
four-call batch sat ~2m13s behind its first call (the sync `Agent`
fork) because `runProviderToolsLoop` dispatched sibling calls strictly
sequentially; the operator interrupted and the three queued independent
calls died without executing. The operator directed async/divide-and-
conquer; the `Agent` tool description itself promises "parallelize
evidence gathering". One mechanism: the dispatch partitions the batch —
non-`apply_patch` calls run concurrently (WaitGroup, results indexed by
call position, `ToolResultEvent`s as they complete), `apply_patch` calls
stay sequential in call order (two same-batch patches can target one
file — a lost-update hazard), results still append as one user message
in call order. Interrupt semantics preserved (in-flight calls die,
completed results are kept — quick calls are no longer lost behind slow
ones). Concurrency audit recorded in the case (CostTracker, StateStore,
MCP client/manager mutexed; EventBus lock-free; forks get separate
engines/stores). Gates: unit baseline RED (event order proves
serialization) / candidate GREEN, `-race` clean, apply_patch hazard
invariant, full suite ×3, real-boot wire gates — `parallel` profile,
two `sleep 3` calls: baseline gap 8.14s vs candidate 5.25s, both
completing the profile — plus fresh MCPINJ `green` and CLK `clkgreen`
adjacent boots on the candidate (Bash + real MCP call executed
concurrently, 4 servers, pairing intact). Claim boundary: concurrent
execution proven; background (async) sub-agents and worktree isolation
for writers remain separate follow-up mechanisms, each with their own
case. Record:
`docs/failure-cases/parallel-sibling-dispatch-2026-09-12.md`.

**INT-001 — Mid-turn operator input is queued, not rejected-and-dropped
(2026-09-12, commit `640bc5b`).** The night-session forfeit note's
mechanism was corrected by code trace: submission while a request is in
flight never canceled anything — `RunInput` rejected the text outright
(busy) and dropped it, so the only delivery path was the operator's
Esc/Ctrl+C interrupt, which canceled the in-flight request and forfeited
its full input spend (four `context canceled` events, ~350K input
tokens; log `2026-09-11T22-55-12.jsonl`). One mechanism: the busy path
appends plain text to the conversation via `Engine.AppendUserInput`
(CLK-001 stamp shape) and emits `QueuedPromptEvent`; the loop's existing
snapshot-per-request fold (proven live by the companions) delivers it to
the next request; a message parks while the tail is a dangling tool_use
(it would serialize before the tool results and break pairing) and
flushes after each companion; slash commands stay rejected while a turn
runs; interrupt semantics unchanged. Gates: baseline RED (reject+drop
through the production path) / candidate GREEN; park+pairing invariant;
real-loop delivery with a real Bash tool on the request wire; TUI
render; `-race`; full suite 29 packages on the INT-001-only tree.
Record: `docs/failure-cases/interrupt-input-queue-2026-09-12.md`.

**INT-002 — Bash input key alias accepted, empty-input error made
diagnosable (2026-09-12, commit `970bb08`).** Found live inside the
INT-001 session: Bash calls whose emitted input carried the command
under `command` (not the schema's `cmd`) failed with a bare
"Bash input requires non-empty cmd" — 7+ occurrences in the
2026-09-12T00-46-33 session plus a 2026-08-30 Terminal-Bench instance,
each burning turns while the model rediscovered the cause. One
mechanism: `executeProviderBashTool` accepts `cmd` (schema key) with
`command` as the observed alias; no-recognized-key errors list the
received top-level keys so the model self-corrects in one turn. Schema,
pragma mode, and all other tools unchanged. Gates: baseline RED (alias
call fails with the terse error) / candidate GREEN (alias executes);
schema-key path green on both; full suite clean. Record:
`docs/failure-cases/bash-input-alias-2026-09-12.md`.

**TURN-003 — Default 100-turn cap removed (2026-09-12, commit
`2b9824a`).** The operator's explicit directive after the cap's history
(two operator sessions + three TB-2.1 tasks + TURN-001's live run dying
mid-wrap-up): no default turn cap. `MaxTurns <= 0` now means uncapped;
`--max-turns`/config bounds unchanged with warning window and cap error
intact; sub-agents stay bounded (`DefaultSubAgentMaxTurns = 100`) when
the parent runs uncapped — a drifting sub would block the sync fork
indefinitely. Gates: baseline RED (unset MaxTurns, 105 scripted tool
turns → "exceeded maximum of 100 turns") / candidate GREEN (completes
with TurnCompleteEvent; sub pinned); every existing cap/window test
(explicit MaxTurns) unchanged; full suite 29 packages clean. Hazards
recorded: unattended uncapped runs have no pragma-side turn guard now.
Record: `docs/failure-cases/turn-cap-removal-2026-09-12.md`.

**Verifier-startup misclassification target examined and closed as
external (2026-09-11).** agent.md's remaining named regression —
verifier-startup failures misclassified as solver failures — was traced to
its decision points: the repo's only trial-outcome classifier is the
Harbor adapter's fatal-exit gate (`tools/harbor_pragma_agent.py`), which
is deliberate and gated (turn-cap and TOK-001 truncation exits are
non-fatal so verifiers still run); `evaluation/` holds result records, not
classifiers; no benchmark runner exists elsewhere in the repo; and
Harbor's source (where the TB-2.1 audit locates the 24 false-zero
collapse) is neither vendored nor installed locally. No pragma-owned
defect, no case; the entry in agent.md is updated accordingly. Reopen only
if a pragma-owned benchmark/runner path ever appears.

### M5 — Held-out self-evaluation (only if a score claim is made)

Frozen candidate, declared task set, matched budgets, repetitions, uncertainty
reported. Do not run broad benchmarks to discover whether a change works.

### Recorded need (follow-up mechanisms) — background sub-agents, worktree isolation

PAR-001 (2026-09-12) delivered concurrent dispatch for sibling calls in
one turn. The operator's async/divide-and-conquer direction has two
remaining mechanisms, each with its own case, one mechanism per change:

1. **Background sub-agents** (branch mechanism, own case): detached fork
   lifecycle — registry, bus lifecycle events, result delivery to a
   conversation that moved on, explicit cancellation, orphan cleanup.
2. **Worktree isolation** for writers (async + shared worktree = races).

Case before mechanism; evidence would be operator friction with the sync
fork's turn-blocking despite PAR-001 (e.g. needing to interrupt a single
long fork while other work waits on the turn) or concurrent writers
racing on the shared worktree.

Status 2026-09-12: still waiting on evidence. The continuation session's
two verification sub-agents (bounded read/return tasks, ~1-2 minutes
total) each blocked their parent turn for their duration under the sync
fork — noticeable but never interrupt-worthy, and no other work waited
on the turn: below the case-opening bar both times. No concurrent-writer
race occurred (the tree stayed clean through three mechanism commits).
The midday dogfood session adds nothing to the bar: no sub-agent use at
all, a single sequential writer, and the tree clean except its own
documentation commit.

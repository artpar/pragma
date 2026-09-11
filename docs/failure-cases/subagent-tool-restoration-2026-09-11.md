# SUB-001 — Subagent tool absent: no way to delegate a bounded task to a fresh sub-conversation

- Case ID: `SUB-001`
- Source revision: `bbcca96` (2026-09-11, post-WEB-001)
- Source observation: the provider-tools loop's model toolset carries 132
  tools (Bash, apply_patch, WebSearch, 129 `mcp__` defs) but **no `Agent`
  tool** — the model cannot delegate a bounded, independent task to a
  fresh sub-conversation. `agent.md`'s delegation discipline ("when
  subagents are requested, give each a bounded independent evidence
  question and concrete deliverable") is currently only executable by the
  operator, not by the harness; the branch `062c203` implements the Agent
  tool (`internal/tools/agent/agent.go`: sync fork, background, teammates,
  worktree isolation, structure graphs); main has none of it.
- Responsible path: no dispatch case and no def; the capability was never
  ported when the loop was reduced to the two built-ins (same family as
  MCPINJ-001/WEB-001).
- Expected behavior: in provider-tools mode the model can call an `Agent`
  tool with `{prompt, description}`; the harness forks a **fresh
  sub-conversation** sharing the parent's provider/bus/cost accounting,
  runs the same provider-tools loop in it with the same toolset **minus
  the Agent tool itself** (no recursion), returns the sub-agent's final
  text to the parent as a paired tool result with a JSON envelope
  `{status, prompt, result, tokens_used}` (the branch's `agentResult`
  shape), and errors surface as `status: failed` without breaking the
  parent loop.
- Scope of this case (one mechanism): the **synchronous sub-agent fork**
  (branch `runSync` semantics). The branch's background tasks, teammates/
  SendMessage, worktree isolation, and structure graphs are separate
  mechanisms requiring their own recorded cases and gates — recorded here
  as follow-ups, not silently dropped.
- Contract sources: roadmap M4; the branch's Agent tool contract
  (`AgentInput`, `agentResult`, `excludeTool(nil, "Agent")` no-recursion
  scoping); `Engine.ForkFreshConversation` (main's existing
  scoped-conversation primitive, whose docstring already anticipates
  subagent engines passing nil compaction, #27794).
- Reproduction (deterministic, hermetic, no spend beyond the local
  scripted provider): `probe_provider.py` gains a `subagent` profile —
  request 1 returns an `Agent` tool call; the sub-agent's own request
  (fresh conversation on the same local endpoint) and the parent's
  follow-up get final text. `assert_boot.py ... subred|subgreen`
  evaluates the wire.
- Executable assertion (baseline RED): no request's `tools` contains
  `Agent`, and the model-issued `Agent` call returns
  `unknown tool "Agent"` in the following request's tool results.
- Executable assertion (candidate GREEN): the parent's requests advertise
  `Agent`; the sub-agent's request appears on the wire as a **fresh
  conversation** (its own user prompt, no parent history, and `Agent`
  absent from its own tools list — the recursion guard); the parent's
  next request pairs a tool result whose content carries the
  sub-agent's final text inside the JSON envelope
  (`status: completed`); the parent then completes normally.
- Adjacent checks: pragma loop mode still sends no tools; the parent's
  other tools (Bash, apply_patch, WebSearch, MCP defs) unchanged; full
  suite clean; unit gates for def shape, input validation, recursion
  guard, fresh-conversation fork, result envelope, and pairing.
- Proposed mechanism (one): `internal/query/subagent.go` — an `Agent` tool
  def + `runSubAgent` executing inside the provider-tools loop via
  `ForkFreshConversation` (sub config: provider-tools mode, no-recursion
  flag, nil auto-compaction per #27794); JSON envelope result; dispatch
  case in `executeProviderToolCall`. No cli changes.
- Refuting evidence: candidate boot where the sub-request carries parent
  history, where `Agent` appears in the sub's tools, where the envelope
  loses the sub-agent's text, or where any adjacent behavior (built-ins,
  MCP/WebSearch injection, pragma mode) changes.
- Claim boundary: harness plumbing restored and verified by hermetic
  real-session boots and unit gates. No claim about sub-agent task
  quality (that is model-dependent; held-out evaluation would be its own
  milestone). No background/teammate/worktree claim — those remain
  follow-up mechanisms.
- Status: case recorded 2026-09-11, pre-fix (baseline RED run pending
  below).

## Verification record (2026-09-11)

### Baseline RED — real session boot at `bbcca96` (post-WEB-001)

- Command: `./e2e/mcp-injection/run_probe.sh baseline-subred subagent 15`
  then `python3 e2e/mcp-injection/assert_boot.py e2e/mcp-injection/results/baseline-subred subred`.
- Probe session (authentic): `8c4f6915-7475-4423-95ad-3a7575813348`
  (`~/.pragma/sessions/8c4f6915-*.jsonl`). Scripted provider issues an
  `Agent` {prompt, description} call on turn 1; local endpoint, no spend.
- Wire evidence (`results/baseline-subred/raw/`): no request's `tools`
  contains `Agent` (131/132 tools otherwise); the call answers
  `unknown tool "Agent"` in request 2's tool results.
- Assertion: `PASS subred.absence`, `PASS subred.execution`.

### Candidate GREEN — same boot on the changed harness

- Mechanism: `internal/query/subagent.go` (Agent tool def +
  `withSubAgentTool` recursion guard + `executeSubAgentTool`:
  `ForkFreshConversation`, sub config provider-tools + `DisableSubAgents`
  + nil auto-compaction per #27794, event accumulation, JSON envelope),
  injection + dispatch in the provider-tools loop. No cli changes.
- Command: `run_probe.sh candidate-subgreen subagent 15` +
  `assert_boot.py ... subgreen`. Probe session:
  `ee89cf4a-6d1f-40b8-82b0-c26b165ce0b6`.
- Wire evidence (`results/candidate-subgreen/raw/`):
  - seq 1 (parent, MCP still connecting): 57 tools including `Agent`.
  - seq 2 (**the sub-agent's own request**): 132 tools (Bash,
    apply_patch, WebSearch, 129 `mcp__`) with **`Agent` absent** — the
    recursion guard holds; fresh conversation (system + its own user
    prompt only; no parent history, no parent tool-call ids).
  - seq 3 (parent follow-up): 133 tools (Agent + full set) and the
    paired tool result:
    `{"status":"completed","prompt":"Reply exactly SUBAGENT_OUTPUT_MARKER and nothing else.","result":"PROBE_COMPLETE","tokens_used":2}`
    — the branch envelope carrying the sub-agent's real final text.
- Assertion: `PASS subgreen.fresh`, `PASS subgreen.result`.

### Adjacent checks

- **Pragma loop mode unchanged**: `run_probe.sh candidate-subpragma pragma`
  (session `fff0cc83-...`) — 1 request, no tools, completes.
- **Sibling capabilities unchanged**: the sub request carries WebSearch
  and the 129 MCP defs alongside the built-ins (only Agent excluded).
- **Unit gates** (`go test ./internal/query -count=1`): def shape
  (prompt required); recursion guard (sub engines get no Agent def;
  collision-safe); full-loop execution — 3 provider calls (parent, sub,
  parent), fresh sub conversation, envelope
  `{completed, prompt, result, tokens_used}`, pairing validated on the
  parent's post-tool request; input validation (missing prompt, bad
  JSON) → paired error results; sub-agent provider failure →
  `Agent failed: ...` error result that still pairs.
- **Stale absence-assertions updated**: the two earlier guard tests
  (`TestProviderToolsLoopWithoutMCPHooksSendsBuiltinsOnly`,
  `TestProviderToolsLoopWithoutWebSearchHookUnchanged`) asserted the
  pre-SUB-001 absence shape (exactly `[Bash, apply_patch]`); they now
  assert `[Bash, apply_patch, Agent]` with no WebSearch/MCP defs. Their
  original absence claims (MCP defs absent without hooks; WebSearch
  absent without a key) remain guarded.
- **Full suite**: `go test ./... -count=1` — clean (29 packages with
  tests).

### Deviations from the branch (recorded)

- Sync fork only: background tasks, teammates/SendMessage, worktree
  isolation, and structure graphs are **not** ported — separate
  mechanisms needing their own recorded cases (follow-ups below).
- The sub-agent inherits the parent's toolset minus Agent (the branch's
  `excludeTool(nil, "Agent")` scope) rather than a name-scoped registry
  (main has no tool registry).
- Sync provider failures return the branch's plain `Agent failed: ...`
  content marked `IsError` (main's error-result discipline) instead of a
  bare content string.
- Input is the sync subset `{prompt, description}`; the branch's
  model/run_in_background/isolation/structure/teammate fields wait for
  their follow-up mechanisms.

### Claim boundary

Harness plumbing restored and verified: the model can delegate a bounded
task to a fresh sub-conversation and receive its final text in the
branch's envelope, proven on the wire in a real session boot with the
recursion guard and tool-result pairing intact. No claim about sub-agent
task quality (model-dependent; held-out evaluation would be its own
milestone). No background/teammate/worktree/structure-graph claim.

**Verdict: SUB-001 repaired for the recorded case.** M4 is complete:
MCP tool injection (M4.1), Brave websearch (M4.2), subagent sync fork
(M4.3). Follow-up mechanisms recorded: background/teammate sub-agents,
worktree isolation, structure graphs — each needs its own case.

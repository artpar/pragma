# MCPINJ-001 — MCP servers connect but their tools are never injected into the model toolset

- Case ID: `MCPINJ-001`
- Source revision: `7bdf167` (2026-09-11)
- Source observation (live production session): the current harness session
  `200c6e98-148e-4499-957e-63eae6fe9b97` (`~/.pragma/sessions/200c6e98-...jsonl`,
  morphllm / morph-glm53-744b, provider-tools loop mode) reports **4 MCP
  servers connected** in its model-facing system prompt
  (`systemWithMCPStatus`: agile, jetbrains-79dfb383eaefab75, planning,
  past-conversations) while the tool list sent to the model contains
  **exactly two tools, `Bash` and `apply_patch`** — the verbatim output of
  `providerToolDefs()` (`internal/query/provider_tools_loop.go`). The model
  is told the servers exist but is given no way to call any of their tools.
- Live server enumeration (2026-09-11, JSON-RPC `initialize` + `tools/list`
  handshakes, commands in `e2e/mcp-injection/`): planning (npx
  `longterm-planner-mcp`) = 48 tools, agile (npx
  `mcp-agile-project-manager`) = 17 tools, past-conversations (npx
  `past-conversations-mcp`) = 11 tools, jetbrains-79dfb383eaefab75
  (HTTP `http://127.0.0.1:59700`, discovered via
  `~/.pragma/jetbrains-mcp/`) = 53 tools. **129 advertised tools, 0
  reachable by the model.**
- Responsible path: `runProviderToolsLoop` hardcodes
  `tools := providerToolDefs()`; `executeProviderToolCall` dispatches only
  `Bash` and `apply_patch` and answers every other name with
  `unknown tool %q`. The supporting primitives exist and are dead:
  `mcp.Client.ListTools`/`ToolInfo` are produced at connect
  (`MCPServerConnected.ToolCount` is even recorded), and
  `BuildToolName`/`ParseToolName`/`IsMCPTool` (`internal/mcp/naming.go`)
  have no callers outside the mcp package and its tests.
- Expected behavior: tools advertised by connected MCP servers are exposed to
  the model as callable tools named `mcp__<server>__<tool>` and tool calls
  with those names are routed back to the owning server connection.
- Contract sources: (1) the harness's own naming contract
  (`internal/mcp/naming.go`, `BuildToolName`/`ParseToolName`) that exists
  precisely for model-facing MCP tool names; (2) SPEC.md's event contract
  `MCPToolCallStarted`/`MCPToolCallCompleted` (SPEC.md, events tables)
  presupposes MCP tool invocations flowing through the harness; (3) roadmap
  M4 declares this restoration from branch `worktree-wt-1776755333195`
  (commit `062c203`), whose `internal/mcp/adapter.go` (`MCPToolAdapter`) +
  `Manager.RegisterTools` implement exactly this mechanism.
- Reproduction (deterministic, real session boot, no inference spend):
  `e2e/mcp-injection/run_probe.sh` boots the real binary non-interactively
  in provider-tools mode against a **local scripted OpenAI-compatible
  provider** (`probe_provider.py`) with raw HTTP capture enabled
  (`PRAGMA_RAW_HTTP_CAPTURE_DIR`), from this repo workdir so the real MCP
  config (global `~/.pragma/mcp.json` + live JetBrains discovery) loads.
  The scripted provider delays its first response to let MCP connect, then
  returns two tool calls in one turn (`Bash` echo + a read-only MCP tool
  `mcp__past-conversations__list_projects`), then finishes on the next
  request. The assertion (`assert_boot.py`) runs against the captured
  request bodies on the wire.
- Executable assertion (baseline RED): among the captured requests there is
  one whose system prompt shows ≥4 MCP servers `status: connected` while its
  `tools` array contains exactly `Bash` and `apply_patch` and **zero**
  `mcp__`-prefixed entries; and the model-issued `mcp__` tool call returns
  `unknown tool` in the following request's tool results.
- Executable assertion (candidate GREEN): the same boot on the changed
  harness produces a request whose `tools` array still contains `Bash` +
  `apply_patch` **and** `mcp__`-prefixed entries from every connected server
  (≥1 per server, including a JetBrains entry), the probed
  `mcp__past-conversations__list_projects` call executes against the real
  MCP server (non-error tool result visible on the wire in the next
  request), and both tool calls from the shared turn are correctly paired
  (request 2 is accepted by `validateToolResultPairing` — an unpaired result
  would abort the loop before the request is sent).
- Adjacent checks (both loop modes + tool pairing): (a) pragma loop mode
  (`--loop pragma`) boots and completes against the same scripted provider
  with **no** `tools` sent — identical wire shape before and after the
  change; (b) full `go test ./...` suite stays clean, with new unit tests
  for the adapter conversion, name routing, dedupe, and loop injection.
- Proposed mechanism (one mechanism, ported from `062c203`, main-shaped):
  1. `internal/mcp/adapter.go`: convert a connected client's advertised
     `ToolInfo` list into `model.ToolDef` entries named via `BuildToolName`
     (`mcp__<server>__<tool>`), schema passed through (object default when
     absent), description capped at 2048 chars (branch's
     `maxDescriptionLen`).
  2. `Manager.ToolDefs(ctx)`: aggregate over connected clients only,
     per-server list failures warn-and-skip (branch's `RegisterTools`
     behavior), deterministic server order, duplicate names skipped.
  3. `Manager.CallMCPTool(ctx, fullName, input)`: parse the full name,
     resolve the owning client by normalized server name, recover the
     original (pre-normalization) tool name from the advertised list, call
     `Client.CallTool`, with the branch's single reconnect retry on
     `ErrServerNotConnected`.
  4. `EngineConfig.MCPToolDefs`/`MCPCallTool` hooks; the provider-tools
     loop appends the defs to the built-ins each turn and routes
     `mcp__`-prefixed calls in `executeProviderToolCall`; wired to the
     manager in `RegisterTools` alongside the existing
     `MCPServerStatuses` hook.
- Refuting evidence: candidate boot with 4 servers connected still sending
  `tools == [Bash, apply_patch]` refutes the port; a candidate wire shape
  that drops `Bash`/`apply_patch`, breaks tool-result pairing, or alters
  pragma-mode requests refutes the "one mechanism, no adjacent change"
  claim.
- Claim boundary: **real local integration** — injection and call routing
  verified against the real local MCP servers in this environment (stdio
  npx servers + live JetBrains HTTP server). No live-provider claim (the
  probe provider is local and scripted). No task-success claim. No
  permission-gating claim: in provider-tools mode the MCP tools execute
  under that loop's existing execute-without-prompt policy, same as `Bash`
  today; permission integration would be a separate mechanism.
- Status: case recorded 2026-09-11, pre-fix (baseline RED run pending below).

## Verification record (2026-09-11)

### Baseline RED — real session boot at `7bdf167` (clean)

- Command: `./e2e/mcp-injection/run_probe.sh baseline-red provider-tools 15`
  then `python3 e2e/mcp-injection/assert_boot.py e2e/mcp-injection/results/baseline-red red`.
- Probe session (authentic): `5b805915-d7d6-4231-b070-1e9054b65cf1`
  (`~/.pragma/sessions/5b805915-*.jsonl`, provider openai/gpt-4o pointed at the
  local scripted provider `probe_provider.py`, no external API, no inference
  spend). Real MCP config loaded: 3 stdio npx servers from `~/.pragma/mcp.json`
  + live JetBrains HTTP discovery (`jetbrains-79dfb383eaefab75`,
  `http://127.0.0.1:59700`).
- Wire evidence (`e2e/mcp-injection/results/baseline-red/raw/`):
  - request 1 (2,088 B): `tools` = exactly `[Bash, apply_patch]`; system
    block shows only `jetbrains-79dfb383eaefab75` connected so far (async
    connect still in progress).
  - request 2 (2,626 B): system block shows **all 4 servers
    `status: connected`** (agile, jetbrains-79dfb383eaefab75,
    past-conversations, planning) while `tools` is still exactly
    `[Bash, apply_patch]` — **the recorded absence, on the wire**.
  - request 2 tool results: `mcp__past-conversations__list_projects` →
    `unknown tool "mcp__past-conversations__list_projects"` (execution-path
    absence); Bash → `Exit code: 0 / MCP_INJECTION_PROBE_BASE_PATH_OK`
    (built-in path healthy).
- Assertion output: `PASS red.absence`, `PASS red.execution` —
  **RED reproduced for the stated reason** (not setup failure: the same boot
  connected all 4 servers and executed Bash).

### Candidate GREEN — same boot on the changed harness (one mechanism)

- Change: `internal/mcp/adapter.go` (ToolDef conversion + Manager.ToolDefs +
  Manager.CallMCPTool, ported from `062c203`'s MCPToolAdapter mechanism),
  `EngineConfig.MCPToolDefs`/`MCPCallTool` hooks, injection + dispatch in
  `internal/query/provider_tools_loop.go`, wiring in `internal/cli/tools.go`
  (`RegisterTools`). Nothing else.
- Command: `./e2e/mcp-injection/run_probe.sh candidate-green provider-tools 15`
  then `assert_boot.py ... green`. Probe session:
  `ebdacb56-58ca-434f-8e04-64758bdc3a9d`.
- Wire evidence (`e2e/mcp-injection/results/candidate-green/raw/`):
  - request 2: `tools` = 131 entries = `Bash` + `apply_patch` + **129
    `mcp__` defs from all 4 connected servers** (agile 17, planning 48,
    past-conversations 11, jetbrains 53 — matching the live handshakes
    exactly, including normalized dotted names like
    `mcp__jetbrains-79dfb383eaefab75__ide_capabilities`).
  - request 2 tool results: `mcp__past-conversations__list_projects` →
    **59,745 B of real output from the live MCP server** (project listing
    JSON, also persisted in the probe session file); Bash result unchanged.
  - Reaching request 2 requires `validateToolResultPairing` to accept both
    results of the mixed turn (Bash + MCP) — pairing holds.
- Reproducibility: `candidate-green-repeat` (session
  `d1de5817-eb1f-4dd1-a9c5-b5d65351b42c`) — same 129-def/4-server shape,
  59,747 B MCP output (volatile project listing differs).

### Adjacent checks

- **Pragma loop mode unchanged**: `run_probe.sh baseline-pragma pragma` /
  `candidate-pragma pragma` — identical wire shape before and after: 1
  request, 1,639 B, **no tools sent**, session completes (`PROBE_COMPLETE`).
  Sessions: `860b4c68-...` (baseline), `420d2207-...` (candidate).
- **Unit gates** (production path, deterministic):
  - `go test ./internal/mcp -run 'TestToolDef|TestClientToolDefs|TestManagerToolDefs|TestManagerCallMCPTool' -count=1` —
    conversion (naming, 2048 cap, default schema), normalization of dotted
    tool names, aggregation (deterministic order, dedupe, disconnected
    skipped), routing through a real in-process MCP server
    (`mcptest`), original-name recovery, unknown server/tool errors.
  - `go test ./internal/query -run 'TestProviderToolsLoop|TestWithMCPToolDefs|TestPragmaLoopModeDoesNotInject' -count=1` —
    injection into every request, builtins preserved and ordered first,
    no-hooks path sends exactly `[Bash, apply_patch]`, MCP call routing with
    ToolCallID pairing, failure result still pairs, mixed Bash+MCP turn
    pairs, merge collision rules, pragma mode never injects.
- **Full suite**: `go test ./... -count=1` — clean, 28 packages, 12.6s
  (M0 baseline: 16–18s; within variance).

### Observed consequences (recorded, not acted on)

- Wire size: provider-tools requests grow from ~2.6 KB to 37 KB (partial
  connect at request 1) / 146 KB (all 4 servers + large tool result at
  request 2) — 129 tool definitions with schemas. Inherent to full
  injection (the branch's behavior too). A per-server opt-in/cap mechanism
  would be a separate change with its own case.
- `PreserveToolNames` remains inert config on main (pre-existing; jetbrains
  discovery sets it, nothing reads it). This port always uses
  `BuildToolName`, so routing is unambiguous.

### Claim boundary

Real local integration for this environment: connected MCP servers' tools
are injected into the model tool list and calls route back, verified on the
wire across 4 real servers (3 stdio npx + 1 live JetBrains HTTP) with raw
HTTP captures, probe sessions, and repeat runs. No live-provider claim (the
probe provider is local and scripted). No task-success claim (would need
held-out evaluation per M5). No permission-gating claim: in provider-tools
mode MCP tools execute without a permission prompt, matching that loop's
existing policy for `Bash`/`apply_patch`.

**Verdict: MCPINJ-001 repaired for the recorded case.** Follow-ups stay
open per roadmap: brave websearch tool, subagent tool (separate mechanisms,
separate cases).

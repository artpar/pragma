# WEB-001 — Brave websearch tool absent from the model toolset

- Case ID: `WEB-001`
- Source revision: `ec79912` (2026-09-11, post-MCPINJ-001)
- Source observation: after MCP tool injection, the provider-tools loop's
  model toolset is `Bash`, `apply_patch`, and 129 `mcp__` defs — **no web
  search capability**. None of the four connected MCP servers provides web
  search (past-conversations indexes local conversations only). The
  harness's credential store carries a `brave` provider key
  (`~/.pragma/credentials.yml`, verified present 2026-09-11), and the
  worktree branch `062c203` implements a `WebSearch` tool over the Brave
  Search API — but main has zero websearch code (grep: no
  `WebSearch`/`search.brave` in internal/ or cmd/). The only web access the
  model has is shelling out through Bash, with no structured results or
  citation discipline.
- Responsible path: `providerToolDefs()` never had a websearch entry and no
  dispatch case exists; the capability was never ported when the loop was
  reduced to the two built-ins.
- Expected behavior: when a brave key resolves (env
  `BRAVE_SEARCH_API_KEY`, then credentials `brave`), the provider-tools
  loop offers a `WebSearch` tool with the branch's semantics: input
  `{query, allowed_domains?, blocked_domains?}`, `site:` OR-filters for
  allowed domains, post-filter for blocked domains, formatted results
  (title/URL/description) with the source-citation reminder, and a helpful
  error path (invalid input, short query, both domain filters set).
- Contract sources: roadmap M4 (restoration from `062c203`); the branch's
  `internal/tools/websearch/{websearch,brave}.go` (tool contract); the
  skill's verified environment fact that the brave key works via the
  Search API (`X-Subscription-Token` header).
- Reproduction (deterministic, hermetic, no brave spend):
  `e2e/mcp-injection/probe_provider.py` gains a `websearch` profile —
  request 1 returns a `WebSearch` tool call; the probe boot
  (`run_probe.sh <label> provider-tools`) then `assert_boot.py ... webred`
  asserts the absence: no `WebSearch` in any request's `tools`, and the
  model-issued `WebSearch` call returns `unknown tool "WebSearch"`.
- Executable assertion (candidate GREEN): same boot on the changed harness
  — every request's `tools` contains `WebSearch` (after the built-ins,
  before/with the MCP defs), and the `WebSearch` call executes through the
  tool path. For the hermetic boot the search endpoint is overridden to a
  local scripted Brave-compatible server via `BRAVE_SEARCH_BASE_URL`
  (mirroring `OPENAI_BASE_URL`); the real endpoint
  `https://api.search.brave.com/res/v1/web/search` is the default.
- Live contract gate (declared, small): one real GET through the changed
  tool path against the real Brave endpoint with the real key — proves the
  wire request (headers, params) and response parsing work against the
  provider. Budget: 1 call, read-only search. No LLM inference spend.
- Proposed mechanism (one mechanism, ported from `062c203`, main-shaped):
  `internal/tools/websearch` exposes `ToolDef()` (name, description, input
  schema) and `Execute(ctx, apiKey, baseURL, input)` implementing the
  branch's validation, `site:` filters, blocked-domain filtering, result
  formatting, and Brave client (status-code error mapping, size-limited
  body). `EngineConfig.WebSearch` carries the executor; the provider-tools
  loop appends the def when the hook is set and dispatches `WebSearch`
  calls to it; `RegisterTools` wires the hook with key resolution
  env `BRAVE_SEARCH_API_KEY` → credentials `brave`.
- Refuting evidence: candidate boot where `WebSearch` is missing from
  `tools` while a key resolves, or where the call still answers
  `unknown tool`; any adjacent change (built-ins dropped, MCP defs altered,
  pragma mode sending tools, pairing violations) refutes the
  one-mechanism claim.
- Claim boundary: tool plumbing restored and verified by hermetic real
  session boots (injection, dispatch, execution, pairing) plus one live Brave
  contract call. No claim about search result quality or task success
  (that would need held-out evaluation). No permission-gating claim: the tool
  executes without a prompt in provider-tools mode, like the loop's other
  tools; `WebSearch` is read-only (GET, no side effects).
- Status: case recorded 2026-09-11, pre-fix (baseline RED run pending below).

## Verification record (2026-09-11)

### Baseline RED — real session boot at `ec79912` (post-MCPINJ-001)

- Command: `./e2e/mcp-injection/run_probe.sh baseline-webred websearch 15`
  then `python3 e2e/mcp-injection/assert_boot.py e2e/mcp-injection/results/baseline-webred webred`.
- Probe session (authentic): `ee1d2f6e-2ad9-478c-a8c9-3a7ffbeb1098`
  (`~/.pragma/sessions/ee1d2f6e-*.jsonl`). The scripted provider issues a
  `WebSearch` tool call on turn 1; local OpenAI-compatible endpoint, no
  LLM spend, no Brave spend.
- Wire evidence (`results/baseline-webred/raw/`): both requests carry
  **131 tools** (Bash, apply_patch, 129 `mcp__` defs) — **no `WebSearch`
  anywhere**; the model-issued `WebSearch` call returns
  `unknown tool "WebSearch"` in request 2's tool results.
- Assertion: `PASS webred.absence`, `PASS webred.execution` — RED for the
  stated reason (MCP injection healthy in the same boot, isolating the
  missing tool as the only absence).

### Candidate GREEN — hermetic boot (local Brave stub)

- Command: `run_probe.sh candidate-webgreen websearch 15` +
  `assert_boot.py ... webgreen`. Probe session:
  `30b3ce5d-36ed-407b-bbd6-c9e78ee5d8f6`.
- Wire evidence (`results/candidate-webgreen/raw/`):
  - request 1 and 2: tools = `Bash`, `apply_patch`, `WebSearch`, + 129
    `mcp__` defs (132 total); `WebSearch` follows the built-ins.
  - The `WebSearch` call executed through the tool path against the local
    Brave-compatible stub (`STUB_BRAVE_RESULT` markers in the 556-byte
    formatted result on the wire).
  - The tool's own Brave wire captured alongside the model wire:
    `GET /res/v1/web/search?count=10&q=pragma+harness+ai+coding+assistant`
    with `X-Subscription-Token` **redacted** in the capture.
  - Pairing intact: request 2 accepted (`validateToolResultPairing`).

### Live Brave contract gate (declared: 1 call, read-only)

- Command: `run_probe.sh candidate-weblive websearch 15 live` +
  `assert_boot.py ... weblive`. Probe session:
  `b68f37be-b77e-4010-af7c-72bceb4afdd4`. No stub, no
  `BRAVE_SEARCH_BASE_URL` override — the real
  `https://api.search.brave.com/res/v1/web/search` with the real key from
  `~/.pragma/credentials.yml` (resolution: env empty → credentials).
- Evidence: Brave HTTP 200 in 1.35s (90,081 B response) — captured in
  `results/candidate-weblive/raw/000001-*/response.meta.json`; the tool
  parsed and formatted **5,152 B of real results** for
  `"pragma harness ai coding assistant"` onto the model wire in request 2,
  ending with the citation reminder. Exactly 1 search call (one non-model
  capture dir), 0 LLM spend.
- Credential hygiene: an earlier candidate-webgreen capture (session
  `2de12150-...`, deleted, never committed) revealed that
  `X-Subscription-Token` was not in rawcapture's secret-header list — the
  key would have leaked into committed evidence. Fixed as part of this
  change (`isSecretHeader` now redacts `x-subscription-token`), re-ran the
  boot, and verified by direct grep that no credential value appears in any
  committed results directory.

### Adjacent checks

- **Pragma loop mode unchanged**: `run_probe.sh candidate-webpragma pragma`
  (session `92146a59-...`) — 1 request, 1,639 B, no tools, completes.
- **MCP injection unaffected**: both websearch boots carry the same 129
  `mcp__` defs as MCPINJ-001's gate (132 total with the 3 built-ins +
  WebSearch).
- **Keyless parity**: without a resolvable key (no env, no credentials entry)
  the hook is unset and the loop sends exactly `[Bash, apply_patch]` —
  `TestProviderToolsLoopWithoutWebSearchHookUnchanged`; a stray `WebSearch`
  call in that state still answers `unknown tool` (loop dispatch guards the
  nil hook).
- **Unit gates**:
  - `go test ./internal/tools/websearch -count=1` — tool def shape
    (schema/citation discipline), stubbed search + formatting, `site:`
    filter composition, blocked-domain exact/subdomain filtering, input
    validation (bad JSON / short query / both filters), API failure
    mapping, default endpoint.
  - `go test ./internal/query -run 'TestProviderToolsLoop(InjectsWebSearch|WithoutWebSearch|ExecutesWebSearch|WebSearchFailure)' -count=1` —
    injection order after built-ins, keyless parity, call routing with
    ToolCallID pairing, error result still pairs.
  - `go test ./internal/cli -run 'TestWebSearch|TestResolveWebSearch' -count=1` —
    key resolution env → credentials → empty, base URL override.
- **Full suite**: `go test ./... -count=1` — clean.

### Deviation from the branch (recorded)

- The branch's tool folded failures into normal content; main returns
  errors so the loop marks the tool result `IsError` (consistent with the
  MCP path). Model-visible behavior is equivalent (text content either way).
- The branch read the key only from `BRAVE_SEARCH_API_KEY`; main resolves
  env first, then the credentials store (matching how every other provider
  key works on main).
- The tool description drops the branch's "Requires
  BRAVE_SEARCH_API_KEY..." line (misleading under credentials resolution).
- `BRAVE_SEARCH_BASE_URL` override added for hermetic tests (mirrors
  `OPENAI_BASE_URL`); default remains the real endpoint.

### Claim boundary

Tool plumbing restored and verified: hermetic real-session boots (injection,
dispatch, execution, pairing) + one live Brave contract call. No claim
about search-result quality or task success (held-out evaluation would be
its own milestone). No permission-gating claim: `WebSearch` is a read-only
GET that executes without a prompt in provider-tools mode, like the loop's
other tools.

**Verdict: WEB-001 repaired for the recorded case.** Remaining M4: subagent
tool (separate mechanism, separate case).

# Capability-injection probes (MCPINJ-001, WEB-001)

Real-session-boot gates for capability injection in the provider-tools
loop: MCP server tools (MCPINJ-001) and the Brave WebSearch tool (WEB-001).
See the case records under `docs/failure-cases/`; this directory holds the
reusable harness and the preserved evidence from the 2026-09-11
baseline/candidate runs.

## Harness

- `probe_provider.py` — scripted local OpenAI-compatible provider (no
  inference spend). `provider-tools` profile delays the first response
  (default 15s, letting the harness's async MCP connect complete), then
  returns two tool calls in one turn (`Bash` echo + read-only
  `mcp__past-conversations__list_projects`); `websearch` profile returns a
  `WebSearch` tool call instead; `subagent` profile returns an `Agent`
  tool call (the sub-agent's own fresh-conversation request and the
  parent's follow-up answer with final text); `pragma` profile answers
  immediately with final text.
- `brave_stub.py` — local Brave-compatible Search API stub (WEB-001
  hermetic gate; no Brave spend).
- `run_probe.sh <label> <profile> [delay] [live]` — boots `./bin/pragma`
  (build first: `make build`) non-interactively against the probe provider
  with raw HTTP capture on, from this repo workdir so the real MCP config
  loads (global `~/.pragma/mcp.json` + live JetBrains discovery). The
  `websearch` profile points `BRAVE_SEARCH_BASE_URL` at the stub; pass
  `live` as the 4th argument to hit the real Brave endpoint instead
  (budget: 1 search call).
- `assert_boot.py <results-dir>
  <red|green|pragma|webred|webgreen|weblive|subred|subgreen>` — asserts
  the wire shape captured under `results/<label>/raw/`.

Preconditions: the 4 MCP servers from the recorded case must be live
(3 npx stdio servers are spawned by the harness; the JetBrains HTTP server
requires the IDE running and its discovery lease fresh — TTL 30s).

## Evidence (2026-09-11, preserved)

- `results/baseline-red/` — MCPINJ-001 baseline (`7bdf167`): request 2
  shows 4 servers connected, `tools == [Bash, apply_patch]`, mcp call →
  `unknown tool`.
- `results/baseline-pragma/` / `results/candidate-pragma/` — pragma mode:
  no tools sent, before and after.
- `results/candidate-green/` (+ `-repeat/`) — MCPINJ-001 candidate: 129
  `mcp__` defs from 4 servers on the wire, real MCP execution, pairing
  intact.
- `results/baseline-webred/` — WEB-001 baseline (`ec79912`): no `WebSearch`
  anywhere, call answered `unknown tool`.
- `results/candidate-webgreen/` — WEB-001 hermetic candidate: `WebSearch`
  injected after the built-ins, executed against the local stub, Brave wire
  captured with the token redacted.
- `results/candidate-weblive/` — WEB-001 live Brave contract: one real
  search call through the full loop path.
- `results/candidate-webpragma/` — pragma mode unchanged after WEB-001.
- `results/baseline-subred/` — SUB-001 baseline (`bbcca96`): no `Agent`
  anywhere, call answered `unknown tool`.
- `results/candidate-subgreen/` — SUB-001 candidate: the sub-agent's
  fresh-conversation request on the wire (Agent excluded from its tools),
  the parent pairing the JSON envelope with the sub-agent's final text.
- `results/candidate-subpragma/` — pragma mode unchanged after SUB-001.
- Each `raw/<seq>-.../` holds `request.json` (exact wire body),
  `request.headers.json` (credentials redacted), `request.meta.json`
  (sequence, URL, sha256), `response.raw`, `response.meta.json`.

Assertions must not be rerun against moved/edited captures: the sha256 in
`request.meta.json` pins the bodies.

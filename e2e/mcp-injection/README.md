# MCP tool-injection probe (MCPINJ-001)

Real-session-boot gate for MCP tool injection in the provider-tools loop.
See `docs/failure-cases/mcp-tool-injection-2026-09-11.md` for the case
record; this directory holds the reusable harness and the preserved
evidence from the 2026-09-11 baseline/candidate runs.

## Harness

- `probe_provider.py` — scripted local OpenAI-compatible provider (no
  inference spend). `provider-tools` profile delays the first response
  (default 15s, letting the harness's async MCP connect complete), then
  returns two tool calls in one turn (`Bash` echo + read-only
  `mcp__past-conversations__list_projects`); `pragma` profile answers
  immediately with final text.
- `run_probe.sh <label> <loop-mode> [delay]` — boots `./bin/pragma` (build
  first: `make build`) non-interactively against the probe provider with
  raw HTTP capture on, from this repo workdir so the real MCP config loads
  (global `~/.pragma/mcp.json` + live JetBrains discovery).
- `assert_boot.py <results-dir> <red|green|pragma>` — asserts the wire
  shape captured under `results/<label>/raw/`.

Preconditions: the 4 MCP servers from the recorded case must be live
(3 npx stdio servers are spawned by the harness; the JetBrains HTTP server
requires the IDE running and its discovery lease fresh — TTL 30s).

## Evidence (2026-09-11, preserved)

- `results/baseline-red/` — revision `7bdf167`: request 2 shows 4 servers
  connected, `tools == [Bash, apply_patch]`, mcp call → `unknown tool`.
- `results/baseline-pragma/` / `results/candidate-pragma/` — pragma mode:
  no tools sent, before and after.
- `results/candidate-green/` (+ `-repeat/`) — changed harness: 129 `mcp__`
  defs from 4 servers on the wire, real MCP execution, pairing intact.
- Each `raw/<seq>-.../` holds `request.json` (exact wire body),
  `request.headers.json` (credentials redacted), `request.meta.json`
  (sequence, URL, sha256), `response.raw`, `response.meta.json`.

Assertions must not be rerun against moved/edited captures: the sha256 in
`request.meta.json` pins the bodies.

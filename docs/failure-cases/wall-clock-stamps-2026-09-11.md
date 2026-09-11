# CLK-001 — The model cannot see the wall clock of its own conversation

- Case ID: `CLK-001`
- Source revision: `7353f07` (2026-09-11, post-TURN-002)
- Source observation (authentic, recorded; live operator session
  `~/.pragma/logs/2026-09-11T22-55-12.jsonl`, `morph-glm53-744b`,
  provider-tools loop):
  1. A four-call batch dispatched at 23:03:20.605 sat ~2m13s behind its
     first call (the sync `Agent` fork); the operator interrupted; the
     fork's in-flight request died (`APIRequestFailed …
     "[openai-compatible] provider_error: context canceled"`,
     23:05:33.999) and the three queued sibling calls (WebSearch, two
     MCP) failed instantly without ever executing. In the conversation
     the model sees, none of this carried a duration or a clock: four
     error strings that could as well have arrived in two seconds. The
     wall-clock story was recoverable only by post-hoc log grep.
  2. Operator, same session: "whats going on why are you stuck" — the
     model's own request durations in this log are 176,472ms and
     197,125ms. The model has no basis on which to notice or manage its
     own latency, a blocking tool call, or operator wait time.
  3. Operator directive (the expected-behavior source for this case,
     2026-09-11 ~23:2x IST): "i think in every message appended to the
     conversation of a session should have the timestamp when it was
     added, cuz it seems you didnt even realise you were stuck."
- Responsible path: `internal/query/provider_tools_loop.go` (the four
  engine append sites) and the provider serialization boundary
  (`anyllm.MessageToAnyLLM`): every appended `model.Message` already
  carries `Timestamp: time.Now()`, but no adapter serializes it — zero
  `Timestamp` references exist under `internal/provider`. The time is
  recorded in the store and dropped on the wire.
- Earliest wrong transition: the append sites record time internally,
  but nothing carries it across the model-visible boundary — the model
  was never given a clock.
- Expected behavior: every user-role message the provider-tools loop
  appends carries its append wall-clock time, visible to the model in
  the outgoing request:
  - The prompt message and the turn-budget notice message (text-only
    user messages): the stamp rides as the first line inside the single
    text part — the string-content wire shape is unchanged and the
    notice body text is unchanged (TURN-001/002 `Contains`-style
    assertions keep passing).
  - The tool-results message keeps its tool-results-only shape: no text
    part mixed in (`MessageToAnyLLM` emits a message's text part before
    its tool results, an unproven user-before-tools wire order on this
    route). Instead, a companion user message carrying only the stamp is
    appended immediately after the results message — the same position
    and wire shape the TURN-001 notice already proved live (user text
    after a completed tool sequence).
  - Assistant messages are unstamped; gaps are inferable from adjacent
    stamps. Pragma loop mode is untouched.
  - One mechanism: raw append times only. No per-tool durations, no
    derived elapsed values, no clock beacons — those would be separate
    cases if ever evidenced.
- Proposed mechanism: `wallClockStamp` — marker
  `[pragma wall-clock ` + RFC3339 timestamp + `]` — applied at the three
  user-role append sites in `runProviderToolsLoop` only.
- Evidence that would refute it: any provider rejection of the stamp
  text or shapes on the live route (the real-boot wire gate and the
  first live session would surface it); any regression in pairing,
  notice, truncation, or sub-agent gates attributable to the companion
  message.
- Gates:
  - Baseline RED (unchanged `7353f07`):
    `go test ./internal/query -run TestProviderToolsLoopWallClockStampsOnAppendedUserMessages -count=1`
    fails because no stamp exists — the loop-completion sanity assertions
    pass first, so the failure is about the stamps, not setup. Observed:
    `prompt text = "work", want wall-clock stamp first line`
    (2026-09-11, `7353f07`). Wire baseline: `assert_boot.py … clkred` —
    0 stamps across 2 captured requests while the scripted profile
    completed (`e2e/mcp-injection/results/baseline-clk-2026-09-11/`).
  - Candidate GREEN: stamps exactly once per engine-appended user message
    (prompt prefix, results companion); RFC3339 format pinned; monotonic
    non-decreasing; the results message stays tool-results-only with
    pairing intact; assistant messages unstamped; stamp count per request
    exact. Observed: both CLK tests pass
    (`TestProviderToolsLoopWallClockStampsOnAppendedUserMessages`,
    `TestPragmaLoopModeCarriesNoWallClockStamps`); full suite clean ×3
    parallel runs (exit 0, 29 packages ok).
  - Adjacent: TURN-001/002 notice gates, TOK-001 truncation gates,
    SUB-001 envelope gates, MCPINJ-001/WEB-001 loop gates — all
    unchanged and passing; pragma-mode absence check (no stamps on the
    pragma loop's requests). Wire adjacent: the MCPINJ-001 `green`
    assertion set passes on the same candidate capture — 133 tools
    (129 `mcp__` from 4 connected servers), real MCP execution of
    `mcp__past-conversations__list_projects` (59,765 bytes), Bash builtin
    result, pairing intact — the stamps ride without disturbing any
    prior wire gate.
  - Real-boot wire gate: local scripted provider boot + raw HTTP capture —
    the outgoing request body contains the stamp (prompt prefix line and
    post-tools companion), proving model visibility on the wire.
    Observed (`e2e/mcp-injection/results/candidate-clk-2026-09-11/`):
    request 1 opens `user: [pragma wall-clock 2026-09-12T00:01:16+05:30]
    \nMCP injection boot probe: …`; request 2 carries the same prompt
    prefix, both tool results, then closes with the companion
    `user: [pragma wall-clock 2026-09-12T00:01:31+05:30]` — 15s of
    elapsed session time now visible to the model in-conversation.
    `assert_boot.py … clkgreen` passes (prefix on all requests, companion
    present, RFC3339 monotonic).
- Claim boundary: stamp injection and model visibility are proven. That
  the model uses the clock to detect or manage blocking/latency is a
  live-effect observation, not a claim of this case.

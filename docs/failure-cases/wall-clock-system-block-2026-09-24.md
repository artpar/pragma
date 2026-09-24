# CLK-002 — Stamp-only wall-clock user messages pollute the conversation

- Case ID: `CLK-002`
- Source revision: `e33e73b` (2026-09-24, post-CMP-002)
- Attempt-2 revision: `ff6bb03` (2026-09-24; attempt 1 died at the
  ~80-turn cap before committing — its code/test partials were discarded
  from the tree and are re-landed here with fresh RED/GREEN observations).
- Spec source (operator directive, live session 2026-09-24 16:10 IST):
  "adding these empty user messages just for the timestamp is a little
  overdone. the idea was that the timestamp should be there, but dont put
  in a user message block just for timestamps sake"
- Source observation (authentic; the operator sees stamp-only user turns
  in the TUI, e.g. the bare `12:02:51` user turn): CLK-001's companion
  mechanism appends a user message whose entire content is
  `[pragma wall-clock <RFC3339>]` after every tool-results batch.
- Responsible path: `internal/query/provider_tools_loop.go` (the
  post-results companion append) and every consumer of conversation user
  messages: the TUI resume/reload paths (`loadMessageSegments` →
  `RenderMessage` render it as a bare `❯ [pragma wall-clock …]` user
  turn), `model.ExtractUserTextPrompts` (it pollutes input history), the
  durable session files (the conversation serializes 1:1), and the
  watcher's `StampishUserMsgs` ergonomics metric (META-001), whose
  `session.go` comment already reads "Post-CLK-002 this should read
  zero."
- Verified against the code before acting:
  - The companion append exists as described: after each tool-results
    batch the loop appends a `RoleUser` message whose sole content part
    is `TextPart{wallClockStamp(time.Now())}`.
  - It is a real operator-visible user turn: `RenderMessage` renders any
    non-internal user message as `❯ <text>`; the companion carries no
    `Flags.IsInternal`.
  - CLK-001's pairing constraint holds: `anyllm.MessageToAnyLLM` emits a
    user message's text part before its tool results
    (`out = append(out, msg)` then `out = append(out, toolResults...)`),
    so the stamp cannot ride inside the results message — confirmed,
    and therefore NOT re-litigated here.
  - The preferred carrier exists as a live-proven shape: per-request
    dynamic system blocks (`systemWithMCPStatus`,
    `systemWithHarnessManifest`, `systemWithPatchGuidance`) are rebuilt
    every loop iteration in `runProviderToolsLoop` and reach the wire as
    the system message (`anyllm.MessagesToAnyLLM(params.System, …)`,
    HMB-001 proved this wire shape live).
  - The other stamp sites (prompt, turn-budget notice, output-budget
    notice, self-continue notice, queued operator input) are NOT
    stamp-only — the stamp is the first line inside real text — and the
    directive leaves them in place.
- Earliest wrong transition: CLK-001 chose the companion user message
  as the post-results clock carrier; the operator directive now pins the
  expected behavior — timestamps stay model-visible, but no user
  message may exist just to carry one.
- Expected behavior:
  - No user message whose entire content is a bare wall-clock stamp is
    ever appended — not in requests, not in the conversation, not in
    session files for new turns.
  - The model still sees the current wall clock every turn: a
    per-request system block (`# Wall clock` + the same
    `[pragma wall-clock <RFC3339>]` line), rebuilt at each request build
    in the provider-tools loop — the same non-cacheable block shape as
    the harness manifest.
  - Text appends keep their first-line stamps (prompt, turn-budget,
    output-budget, self-continue, queued operator input); the
    tool-results message stays tool-results-only; assistant messages
    stay unstamped; pragma loop mode stays untouched.
  - The compaction reserve mirror (`EstimateCompactionReserve`) counts
    the new block too — the count must stay over the exact request
    shape (CMP-001.4 F6).
- Proposed mechanism (one change): `systemWithWallClock` appended in
  the per-iteration request build; delete the companion append; move
  nothing else — the INT-001 drain point stays immediately after the
  results append (user text after a completed tool sequence, the wire
  shape TURN-001 proved live).
- Evidence that would refute it: any provider rejection of the system
  block on the live route; any regression in pairing, notice, INT-001
  queueing, or truncation gates attributable to the removal; the model
  losing per-turn clock visibility (system stamp absent on any request).
- Gates:
  - RED (unchanged `ff6bb03`, amended gates only — production code
    untouched): the gates below must fail for the stated reason — the
    request system carries no wall-clock block and the stamp-only
    companion still exists. Observed (2026-09-24, `ff6bb03` + amended
    gates, `git status` clean over `internal/`):
    `TestProviderToolsLoopWallClockStampsTextAppendsAndRequestSystem` —
    "request 0 system carries 0 wall-clock stamps, want exactly 1 (the
    per-request clock block, CLK-002)";
    `TestProviderToolsLoopDispatchesSiblingCallsConcurrently` —
    "request 1 carries 4 messages, want 3 (prompt, assistant, results)";
    `TestProviderToolsLoopDeliversQueuedInputAtNextRequest` —
    "request 2 message 3 is a stamp-only user message (CLK-002 ban):
    \"[pragma wall-clock 2026-09-24T16:40:42+05:30]\"" — the real
    companion, observed;
    `TestEstimateCompactionReserveCountsProviderToolsFixedPayload` —
    "provider-tools reserve = 1003, want exactly tool schemas (645) +
    manifest/clock/patch blocks (463) = 1108" (the F8 reserve did not
    count the clock block).
    `TestAppendUserInputParksBehindDanglingToolUse` and the extended
    `TestPragmaLoopModeCarriesNoWallClockStamps` pass on the baseline
    (drain mechanics and pragma-mode absence are not the defect); they
    pin the adjacency on the candidate.
  - GREEN (candidate, one mechanism — `systemWithWallClock` in the
    per-iteration request build + companion append deleted + reserve
    mirror — `internal/query/loop.go` helper + `provider_tools_loop.go`
    wiring/companion deletion + `engine.go` reserve mirror): all six
    gates pass
    (`TestProviderToolsLoopWallClockStampsTextAppendsAndRequestSystem`,
    `TestProviderToolsLoopDispatchesSiblingCallsConcurrently`,
    `TestProviderToolsLoopDeliversQueuedInputAtNextRequest`,
    `TestAppendUserInputParksBehindDanglingToolUse`,
    `TestPragmaLoopModeCarriesNoWallClockStamps` — extended to assert
    no clock block in pragma-mode systems — and
    `TestEstimateCompactionReserveCountsProviderToolsFixedPayload`,
    amended to count the clock block via the literal-mirrored block
    text). Full `internal/query` suite clean
    (`go test ./internal/query/... -count=1`, 2 runs: pre- and
    post-gofmt); the whole `internal/...` tree clean (25 packages ok);
    adjacent `internal/tui`, `internal/tui/render`, `internal/watcher`,
    `internal/compact`, `internal/cli`, `internal/provider/...`,
    `internal/model`, `internal/mcp` all ok.
  - Adjacent: INT-001 queueing (parked note lands after the results
    batch, no companion between), parallel-sibling dispatch request
    shape, compaction reserve parity, pragma-mode stamp absence, and
    the full `internal/query` suite — covered above; the session-file
    adjacency is asserted in the GREEN gate (no stamp-only user message
    in the persisted conversation, which session files serialize).
  - Wire: the e2e real-boot gate (`e2e/mcp-injection/assert_boot.py`
    `clkgreen`) updated to the CLK-002 shape — prompt stamp line on all
    requests, no stamp-only user message on the wire, the system
    message carrying exactly one clock stamp, the clock never running
    backwards across requests — run against a fresh candidate capture
    from the local scripted provider (no inference spend). Observed
    (2026-09-24, attempt 2, `e2e/mcp-injection/results/candidate-clk2-v2-2026-09-24/`,
    built from the working tree at `ff6bb03`+candidate): ASSERTION
    PASSED — `clkgreen.prefix` (prompt stamp line on all 2 requests),
    `clkgreen.nocompanion` (no stamp-only user message on any request),
    `clkgreen.system` (per-request clock block stamp in every
    request's system), `clkgreen.monotonic`. The final request's wire
    roles are `[system, user, assistant, tool, tool]` — the companion is
    gone — and the system message carries the `# Wall clock` block with
    `[pragma wall-clock 2026-09-24T16:44:48+05:30]`.
    Wire RED on preserved authentic baseline data: the new `clkgreen`
    FAILS against the preserved CLK-001 candidate capture
    (`e2e/mcp-injection/results/candidate-clk-2026-09-11/`, built at
    the CLK-001 candidate revision 2026-09-11) with "request seq=1
    system carries 0 wall-clock stamps, want exactly 1" — the exact
    pre-CLK-002 wire shape. Attempt 1's preserved capture
    (`e2e/mcp-injection/results/candidate-clk2-2026-09-24/`, built
    from attempt 1's now-discarded working tree) also passes the new
    assertion set, confirming the re-landed mechanism reproduces
    attempt 1's wire shape byte-for-byte in block wording.
    Environment note: the MCPINJ-001 `green` server-count assertion
    fails on today's capture for an external reason — 3 MCP servers
    connected today (agile, past-conversations, planning) vs the 4
    recorded on 2026-09-11 (one JetBrains-discovered server is
    absent); the scripted tool calls paired and the loop shape is
    intact. Not attributed to CLK-002; the count is environment state,
    not harness behavior.
- Revision note (2026-09-24, this case): every payload claim was
  verified against the code before acting (companion append site, TUI
  resume/reload rendering path, `ExtractUserTextPrompts` pollution,
  `MessageToAnyLLM` text-before-results serialization order, the
  existing per-request system-block carriers, the untouched
  first-line-stamp sites); none were refuted, so nothing was labeled or
  skipped. One pre-existing gofmt divergence in two of the touched test
  files (`wall_clock_test.go`, `parallel_dispatch_test.go`) was fixed by
  the required `gofmt -w` on those files.
- Revision note (2026-09-24, attempt 2 — re-landing after attempt 1's
  cap death): attempt 1's production/test partials and the
  `assert_boot.py` update were discarded from the tree when it died at
  the ~80-turn cap, so this untracked record's "observed" claims were
  re-executed from scratch at `ff6bb03` and all reproduced: the four
  RED failures (main gate, sibling dispatch, INT-001 queue delivery,
  reserve mirror), the six GREEN gates, the full `internal/query` suite,
  the whole `internal/...` tree, the `clkgreen` wire-shape update, the
  wire RED on the preserved 2026-09-11 baseline capture, and a fresh
  wire GREEN capture at this revision. Attempt 1's preserved capture
  passes the new assertion set, so its wire evidence stands; the
  re-landed mechanism matches its block wording and position (after the
  harness manifest, before patch guidance) exactly. Every payload claim
  was re-verified against the code before acting; none were refuted.
  gofmt: the touched files (`loop.go`, `wall_clock_test.go`,
  `parallel_dispatch_test.go`) were formatted; four UNTOUCHED files
  (`bash_live_output_test.go`, `harness_manifest_test.go`,
  `provider_tools_mcp_test.go`, `turn_cap_removal_test.go`) were
  verified already gofmt-divergent at `ff6bb03` and were left alone
  (pre-existing, out of this case's scope).
- Claim boundary: conversation cleanliness and model-visible per-request
  clock are proven. That the system-block clock changes model behavior
  (latency management) is a live-effect question, not a claim here.

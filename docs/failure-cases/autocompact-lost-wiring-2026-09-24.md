# CMP-001 — Provider-tools loop never triggers auto-compaction (lost wiring)

Case ID: `CMP-001`

Source revision: `4a7dc99` (2026-09-24); regression introduced by `95621ad`
("Add provider-tools loop and Lilac M3 support"), which wrote
`runProviderToolsLoop` without porting the auto-compact trigger that the
pragma loop carried since `2e9f01b` (GOGENT-14, where
`internal/query/loop.go:118` called `e.autoTracker.ShouldAutoCompact`).

Source observation (authentic, recorded): pragma-watch post-mortem
`~/.pragma/observations/postmortems/2026-09-23T15-27-04.postmortem.json`
(surfaced by the observation daemon on 2026-09-24) flagged
`context_peak: 345,219`. Event log `~/.pragma/logs/2026-09-23T15-27-04.jsonl`
(morph-glm53-744b, provider-tools loop, 262 `APIRequestCompleted`):

- Context fill grew monotonically from 11,984 tokens (2026-09-23
  16:46:30) to 345,219 tokens (2026-09-24 11:48:24) — ~19 hours,
  93 requests — with zero compactions while crossing every plausible
  threshold. No `CompactionStarted`-class event exists anywhere in the
  log (kinds present: MCP*, MessageAppended, APIRequest*, UserTurnAccepted).
- Three earlier >40% fill drops (120,503→11,749 at 16:11:05,
  21,835→11,742 at 16:12:10, 282,377→11,984 at 16:46:30) are consistent
  with operator-initiated `/compact`, not auto-compaction — the final
  19-hour stretch had none.

Responsible path: `internal/query/provider_tools_loop.go`
`runProviderToolsLoop` — the loop builds every request without ever
consulting `e.autoTracker` / `e.windowConfig` / `e.compactor`. Proof:
`grep -rn 'ShouldAutoCompact' --include=*.go .` finds a production call
site only in the stale worktree
`.pragma/worktrees/wt-1776755333195/internal/query/loop.go:251` and in
historical commit `2e9f01b` — never in the current main tree. Everything
around the missing call survived: `Engine` keeps the
`compactor`/`autoTracker`/`windowConfig` fields
(`internal/query/engine.go:76-80`, documented "nil means auto-compaction
disabled" — implying non-nil root engines auto-compact),
`BuildCompactionDeps` constructs and injects them
(`internal/cli/run.go:2021`), `compact.AutoTracker.ShouldAutoCompact` is
fully tested, and the loop-event types `CompactionStartedEvent`/
`CompactionEvent`/`CompactionFailedEvent`/`CompactionDisabledEvent` exist
unused in `internal/query/event.go:99-127`.

Earliest wrong transition: commit `95621ad` — the new loop was written
without the trigger block; every default-mode session since
(`--loop provider-tools`) runs with auto-compaction silently dead. All
tests stayed green because the tracker's tests exercise the tracker, not
the loop's call to it.

Expected behavior and its source: SPEC.md §6.3 (Compact algorithm) and
the loop observable-events list (SPEC.md:798 "Compaction triggered"; the
"Compaction Events" table defines CompactionStarted/CompactionCompleted)
— the loop must trigger compaction when the token estimate exceeds the
effective-window threshold, and emit the compaction loop events.

Executable assertion (baseline RED on `4a7dc99`):
`go test ./internal/query/ -run TestProviderToolsLoopAutoCompactTriggersAndReplaces`
fails with `CompactionStartedEvent count = 0, want 1` — a real
`runProviderToolsLoop` run over a conversation far above the configured
904-token threshold completes without any compaction call.
Second gate (failure path): `TestProviderToolsLoopAutoCompactCircuitBreaker`
— with a provider that fails every summary request, the loop emits
`CompactionFailedEvent` with attempts [1 2 3], trips the breaker after the
third failure (`CompactionDisabledEvent`), stops attempting (3 compact
calls total across 4 turns), leaves the original messages in place, and
still completes the turn normally.

Proposed mechanism (one): port the trigger block from `2e9f01b`/the
worktree into `runProviderToolsLoop` — before each request build, count
conversation tokens (provider `TokenCounter` when implemented, heuristic
`EstimateConversationTokens` fallback), ask
`e.autoTracker.ShouldAutoCompact`, and on true call `e.compactor.Compact`,
emitting `CompactionStartedEvent`, then `CompactionEvent` with replacement
messages on success or `CompactionFailedEvent` + circuit-breaker handling
on failure, with `IncrementTurn` per iteration. Subagent engines keep nil
compaction deps (#27794 unchanged); `DISABLE_AUTO_COMPACT` unchanged;
pragma loop mode untouched.

Adjacent checks: full `internal/query` suite (turn budget, wall clock,
self-continue, interrupt queue, parallel dispatch, subagent, token limit
gates must stay green — none of them set compaction deps, so the trigger
must no-op when `compactor`/`autoTracker` are nil); `-race`; full
`go test ./...`.

Explicitly out of scope (filed as CMP-002, next case):
`internal/provider/morphllm` hardcodes `ContextWindow = 1_000_000` for
every model on the route, while the live-recorded router policy is ≈200k
raw tokens (skill/roadmap note: the `raw_isl_tokens` medium-class 429 from
conversation growth, 2026-09-11/12). With the trigger wired, the threshold
for this session would still have been ≈961k — unreachable before router
pain. The 345k overrun needs BOTH mechanisms; this case restores the
trigger, CMP-002 calibrates the window.

---

## Revision CMP-001.1 (2026-09-24): double IncrementTurn + dead breaker assertion

Source: independent fresh-instance audit of commit `42c45b4`; queue item
CMP-001.1. Both payload findings were verified against the code before any
change; neither was refuted.

### F4 (confirmed): duplicate per-iteration IncrementTurn halved the
MinTurnsCooldown death-spiral guard

Verified call sites, both inside `runProviderToolsLoop`'s
`for turn := 0; ...` iteration:

- `internal/query/provider_tools_loop.go:138` — added by the CMP-001 fix
  itself (port of the `2e9f01b` pragma-loop block, whose only increment
  sat inside the compaction-deps guard);
- `internal/query/provider_tools_loop.go:283` — end-of-iteration
  increment dating to the loop's creation in `95621ad`
  (`git log -L 280,286`), i.e. dead bookkeeping until CMP-001 wired the
  trigger, then a live double-increment.

With both sites active, a successful compaction at iteration k
(`RecordSuccess` → `turnsSinceCompact = 0`) receives two increments within
iteration k (line 138 mid-iteration, line 283 at iteration end), so
`turnsSinceCompact = 2 ≥ MinTurnsCooldown` already holds at the top of
iteration k+1 — the `t.compacted && t.turnsSinceCompact < MinTurnsCooldown`
guard (`internal/compact/auto.go:51`) no longer blocks anything, and a
still-over-threshold conversation re-compacts on the immediately following
iteration (the #24179 death spiral the cooldown exists to stop).

RED (before the fix):
`go test ./internal/query/ -run TestProviderToolsLoopAutoCompactCooldownBlocksImmediateRetrigger -count=1`
failed with
`run 1 CompactionStartedEvent count = 2, want 1 (MinTurnsCooldown=2 must block the immediately following iteration)`
— the scripted summary is itself huge, so the post-compaction conversation
stays above the 904-token threshold and only the cooldown can block the
re-trigger; the unchanged loop compacted twice back-to-back.

Fix (one mechanism change): removed the end-of-iteration site (the
`if engine.autoTracker != nil { ... IncrementTurn() }` block after
`drainPendingUserInputs`), keeping exactly one increment per model-request
iteration inside the compaction-deps guard — the `2e9f01b` semantics the
CMP-001 block ported. Site choice is load-bearing, not stylistic: the
end-turn path returns at the `TurnCompleteEvent` send, before the
end-of-iteration site, so keeping that site instead would leave an
end-turn-only session permanently cooldown-locked after its first
successful compaction (`turnsSinceCompact` could never reach 2 across
runs). The test's second phase pins this: after run 1 (which ends on
end_turn), run 2 must compact again — proving the cooldown expires across
end-turn boundaries. GREEN after the fix, alongside the two original
CMP-001 gates (unchanged expectations: the breaker path never calls
`RecordSuccess`, so cooldown cannot engage there).

### F7 (confirmed): dead assertion in the breaker test

`TestProviderToolsLoopAutoCompactCircuitBreaker` guarded its
big-text-preservation check behind
`if len(store.Snapshot().Conversation.Messages) <= len(conv.Messages)` —
always false, because the loop appends the initial prompt before iteration
0, so the snapshot always has ≥7 messages against the 6 originals. The
`original messages lost` assertion was unreachable; the test could not
fail for the regression it was written for.

Proof by temporary production mutation (three executable steps, all
observed): (1) mutating the compaction-failure branch to rewrite every
message's content to same-size "mutated" text (token count and event flow
preserved: 3 attempts, 3 failures, breaker, turn complete) left the
breaker test PASSING — the dead assertion executed nothing; (2) after the
test fix, the same mutation failed it with
`original messages lost after failed compactions`; (3) reverting the
mutation restored PASS. Two cruder mutations (replace all messages with
one short message) were caught by earlier assertions (`compaction
attempts = 1, want 3` — <4 messages → ErrTooFewMessages, and under-threshold
token counts stop further attempts), which is why the proof mutation keeps
count and size while destroying only the original text.

Fix (one change, test-only): the original-preservation check now runs
unconditionally over the snapshot — the conversation legitimately grows
past the originals (prompt, assistant turns, tool results, stamps), so a
message-count comparison can never gate it.

### Adjacent observations (verified, deliberately unchanged)

- `internal/query/miniswe_loop.go` carries two `IncrementTurn` sites
  (lines 313, 372, both predating CMP-001) but never calls
  `ShouldAutoCompact` anywhere, so no cooldown guard is active in that
  loop today — the increments are dead bookkeeping, not a halved guard.
  Candidate for a future case if miniswe ever wires the trigger.
- `go test ./cmd/pragma/ -run TestProviderToolsCLIContract -count=1`
  fails at pristine `42c45b4` AND at `1a9fbdc` (pre-dates the audited
  commit, reproduced in clean worktrees) with
  `stdout did not contain final answer` — a pre-existing acceptance/
  environment failure unrelated to CMP-001 and to this revision; not
  investigated further here. Everything else in `go test ./...` is green.
- `gofmt -l internal/query/` flags three unrelated pre-existing files
  (`bash_live_output_test.go`, `harness_manifest_test.go`,
  `parallel_dispatch_test.go`); untouched to keep this change
  single-mechanism. Both files changed here are gofmt-clean.

### Gates (this revision)

- `go test ./internal/query/ -run 'TestProviderToolsLoopAutoCompact' -count=1 -v`
  — 3/3 PASS (trigger/replaces, cooldown re-trigger gate, circuit breaker).
- `go test ./internal/query/ -count=1 -race` — ok (9.8s, full package).
- `go test ./internal/compact/ ./internal/cli/ -count=1` — ok.
- `go test ./... -count=1` — ok except the pre-existing
  `TestProviderToolsCLIContract` failure documented above.

Claim boundary: proven locally, deterministically — the cooldown now counts
one model-request iteration per iteration (re-trigger blocked at k+1,
permitted from k+2, expiring across end-turn boundaries), and the breaker
test's preservation assertion is executable. No live-provider evidence is
claimed; window calibration (CMP-002) remains open.

---

## Revision CMP-001.2 (2026-09-24): session-persistence desync + swallowed pending prompt

Source: independent fresh-instance audit of commit `42c45b4`; queue item
CMP-001.2. Both payload findings (F2, F3) were verified against the actual
code before any change; neither was refuted. One mechanism-level fix per
finding, each gated RED→GREEN.

### F2 (confirmed): auto-compaction desyncs the session file; --resume resurrects the pre-compaction history

Verified mechanism chain:

- `runProviderToolsLoop`'s compaction success branch (provider_tools_loop.go)
  replaced `store.Conversation.Messages` with `compResult.ReplacementMessages`
  and never touched the session file — no rewrite, no checkpoint call.
- The incremental session writer (`makeSessionSaveClose`, run.go:2053) writes
  by INDEX: `for i := d.SessionLastIdx; i < len(messages); i++`, then
  `SessionLastIdx = len(messages)`. `SessionLastIdx` is an index into the
  PRE-compaction array. After the replacement the new array is far shorter,
  so the summary and the first post-compaction appends (which land below the
  stale index) are never written, and every later checkpoint silently skips
  them.
- On `--resume` (deps.go), the session replays every message from the file:
  `conv = resumedConversation(sess, cwd)`, `SessionLastIdx = len(conv.Messages)`.
  The file still held the full pre-compaction history, so the exact 345k-style
  blowup CMP-001 fixed in memory resurrected from disk.
- Manual `/compact` never had this bug: `handleCompact` returns
  `RewriteSession: true` (slash/commands.go), and `runSlash` calls
  `rewriteCurrentSession` (run.go), which truncates + rewrites the file and
  resets `SessionLastIdx`. The auto path had no equivalent.

RED (before the fix, real production path — loop + real `session.Writer`
+ real `session.Store.Load`, wired exactly as the runtimes wire
`SetSessionCheckpoint`):
`go test ./internal/cli/ -run TestAutoCompactRewritesResumableSessionFile -count=1`
failed with `resumed session replays 7 messages (>= the 6-message
pre-compaction history) — the compacted conversation was not persisted`;
the reloaded file contained the 6 big pre-compaction messages + the prompt,
no summary, no post-compaction assistant reply. (First RED run surfaced a
fixture bug — seed messages without IDs are deduped to one entry by the
writer's ID-based dedup; fixed by minting IDs like real engine appends do.
The production defect then failed for the stated reason.)

Fix (one mechanism): a full-session-rewrite hook mirroring the checkpoint
hook. `EngineConfig.SessionRewrite func() error` + `SetSessionRewrite` +
nil-safe `rewriteSession()`; the loop's compaction success branch calls it
after applying the replacement (a failed rewrite errors the turn, same as
the manual path). run.go wires it next to every `SetSessionCheckpoint`
site (4 sites: interactive setup, post-/clear re-init, Resume, both
non-interactive runs) to a closure over the existing `rewriteCurrentSession(d)`.
Nil hook (subagent engines, sessionless tests) is a no-op. GREEN with the
same test: the file now carries the summary + post-compaction exchange and
no bulk history.

### F3 (confirmed): the pending prompt is compacted away and never reaches the model

Verified mechanism chain: the loop appends the stamped prompt BEFORE the
iteration (provider_tools_loop.go top), the compaction check runs at the TOP
of iteration 0 over the full conversation including that unanswered prompt,
and `compact.Service.Compact` replaces ALL messages with a single summary
message (compact.go `replacements := []model.Message{summaryMsg}`);
`CompactUserMessage(formatted, false)` wraps the bare summary with no
follow-up directive (prompt.go). The operator's prompt reached the model
only if the summarizer happened to retain it — the verbatim prompt was
swallowed. The 2e9f01b pragma loop never compacted a pending prompt: its
trigger ran AFTER the assistant response was appended, so the tail was
always answered; the port to top-of-iteration (CMP-001) created the
swallow.

RED (before the fix):
`go test ./internal/query/ -run TestProviderToolsLoopAutoCompactPreservesPendingPrompt -count=1`
failed with `post-compaction request lost the pending prompt (RIVERGATE-7f3a):
the unanswered operator prompt was compacted away and never reached the model`
— the scripted summary deliberately omits the marker, so the only way it
could reach the model was the preserved prompt message.

Fix (one mechanism): `pendingUnansweredUserPrompts` — the trailing run of
user messages after the conversation's last assistant message, keeping the
text-only, non-internal ones; re-appended after the summary inside the same
`store.Update`, so the model request carries [summary, prompt].
Deliberate exclusions (documented boundary): tool-result messages are not
re-appended (orphan tool_results without their tool_use would break
tool_result pairing — they stay summarized), and internal (hook-context)
messages are not re-appended. Mid-turn operator input queued by INT-001
drains before the next iteration's check and is preserved by the same rule.
GREEN: the post-compaction request carries the marker verbatim and the
store retains the prompt; the summary-bearing assertions of the original
CMP-001 gate are unchanged.

### Gates (this revision)

- `go test ./internal/cli/ -run TestAutoCompactRewritesResumableSessionFile -count=1` — PASS.
- `go test ./internal/query/ -run 'TestProviderToolsLoopAutoCompact' -count=1 -v` — 4/4 PASS
  (trigger/replaces, pending-prompt preservation, cooldown re-trigger gate, circuit breaker).
- `go test ./internal/query/ ./internal/compact/ ./internal/session/ -count=1` — ok.
- `go test ./internal/cli/ -count=1` — ok.
- `go test ./internal/query/ -count=1 -race` — ok.
- `go vet ./internal/query/ ./internal/cli/` — clean; all five files changed here
  are gofmt-clean (five other pre-existing gofmt-dirty test files in
  query/compact are untouched, matching the CMP-001.1 note).

Claim boundary: proven locally, deterministically. The session-file rewrite
and prompt preservation are proven through the real loop, session writer,
and session loader; no live-provider behavior is claimed. The rewrite
correctly rewrites whatever the STORE holds at compaction time — if a
future change lets the store diverge from what the model sees, that is a
new case. Window calibration (CMP-002) and the queue items CMP-001.3/4
remain open. The pre-existing `TestProviderToolsCLIContract` acceptance
failure documented in CMP-001.1 is unchanged and untouched here.

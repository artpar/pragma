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

---

## Revision CMP-001.3 (2026-09-24): the DEFAULT loop mode never had the restored trigger; record's mode/history claims corrected

Source: independent fresh-instance audit of commit `42c45b4`; queue item
CMP-001.3, finding F1 (and the CMP-001.1-reaudit critic finding C-1 that
first surfaced it). Every F1 sub-claim was verified against the code and
git history before any change; none was refuted.

### History verification (all claims reproduced against the tree)

- `--loop` defaults to `"pragma"` — `internal/cli/flags.go:30`, and has
  since `95621ad` introduced the flag (verified at 95621ad, 4a7dc99,
  42c45b4, c183083, 39044b1, HEAD). `Engine.Run`'s default branch
  dispatches to `runPragmaLoop` (`internal/query/loop.go:34-36`);
  `deps.go:377` defaults `LoopMode` to `LoopModePragma`; subagent engines
  are forced to provider-tools (`subagent.go:111`). So the DEFAULT mode
  is the pragma loop, not provider-tools.
- `runPragmaLoop` never consulted the tracker: no `ShouldAutoCompact`
  anywhere in `miniswe_loop.go` — `git log -S'ShouldAutoCompact' --
  internal/query/miniswe_loop.go` is empty across the file's entire
  history (created 532ee5b). It carried only two `IncrementTurn` sites.
- The default-mode loop lost the trigger at `fed8bd7`: at `fed8bd7^`
  the default dispatch (`Engine.Run`, loop.go:52) ran `runLoop`, whose
  iteration called `autoCompactBeforeRequest` (defined at
  fed8bd7^:503-504, called at :198, `ShouldAutoCompact` at :522 — the
  payload's ":504" is the function body start). `fed8bd7` ("Remove stale
  web and handoff code") deleted `runLoop`/`autoCompactBeforeRequest`
  (0 matches in fed8bd7's loop.go) and pointed `Engine.Run` at the
  triggerless `runPragmaLoop` (fed8bd7 loop.go:42). The miniswe loop
  never carried the trigger itself — "lost at fed8bd7" means the DEFAULT
  mode lost it, exactly.
- Before this revision the only production trigger call was
  `provider_tools_loop.go` (now :114; the payload's :103 and the re-audit
  critic's :110 are the same line drifting across CMP-001.1/.2 edits).
  Compaction deps are injected regardless of loop mode
  (`run.go:1224-1225` interactive, `:1616-1617` non-interactive,
  `:1119` rebind), so default-mode engines ran with live deps and a
  dead trigger.
- The postmortem session that seeded CMP-001 ran provider-tools because
  the wrappers select it explicitly (`tools/harbor_pragma_agent.py:137`,
  `tools/self_improve.py:88` pass `--loop provider-tools`), not because
  it is the default.

### Record corrections (this section amends the record's own claims)

- The original record's "every default-mode session since
  (`--loop provider-tools`) runs with auto-compaction silently dead"
  conflated two loops and was wrong on both counts: provider-tools is
  NOT the default mode, and the actually-default pragma loop did not
  merely "keep" its trigger — the default mode lost the trigger at
  fed8bd7, before 95621ad existed. The correct statement: default-mode
  (pragma-loop) sessions have run with auto-compaction silently dead
  since fed8bd7 (2026-06-08); provider-tools sessions since 95621ad;
  CMP-001 repaired provider-tools only.
- The original mechanism note "pragma loop mode untouched" and the
  provider_tools_loop.go comment "The pragma loop carried this trigger
  since 2e9f01b" (fixed in this revision) both rested on the wrong
  history above. At 2e9f01b the trigger lived in the then-default loop;
  the miniswe-aligned `runPragmaLoop` that is today's default never had
  it.

### Change (one mechanism): the CMP-001 trigger block ported into the pragma loop

`internal/query/miniswe_loop.go` `runPragmaLoopWithInitialPrompt` main
turn loop (block at ~349-443, `ShouldAutoCompact` at :383): the exact
CMP-001 mechanism as it stands after revisions .1/.2 — deps guard, token
count (provider `TokenCounter` when implemented, heuristic fallback),
`ShouldAutoCompact`, `CompactionStartedEvent`, `Compact`, failure →
`CompactionFailedEvent`/breaker → `CompactionDisabledEvent`, success →
`RecordSuccess` + `pendingUnansweredUserPrompts` re-append +
`store.Update` replacement + `rewriteSession()` + `CompactionEvent`,
with the ONE per-iteration `IncrementTurn` inside the guard. The old
post-assistant increment site in the main loop was REMOVED with the port
— keeping it would double-increment per iteration and halve
`MinTurnsCooldown` (the CMP-001.1 F4 defect); the cooldown test below
pins that. The FinalTextOnly sub-loop (orchestration capture states) and
its increment are untouched: that path never compacts and is not the
default mode.

Pragma-loop-specific gate (documented, load-bearing): scoped activations
— `run.MessageStartIndexes` set, i.e. orchestration state runs whose
requests are sliced to their own conversation segment — skip the
trigger, because compaction replaces the WHOLE conversation and would
invalidate the scope's start index (the provider-tools loop has no
scoping concept, so its port needed no such gate). Unscoped pragma-loop
runs — plain `pragma` interactive and `--prompt` sessions, and
orchestration persistent-conversation states — carry the trigger.

Comment-only correction in `provider_tools_loop.go` (the wrong
"carried this trigger since 2e9f01b" history); no behavior change there.

### Gates (this revision)

RED (unchanged tree, before the port): both new tests failed with
`CompactionStartedEvent count = 0, want 1` —
`go test ./internal/query/ -run 'TestPragmaLoopAutoCompact' -count=1`.

GREEN after the port:
- `go test ./internal/query/ -run 'TestProviderToolsLoopAutoCompact|TestPragmaLoopAutoCompact' -count=1 -v`
  — 6/6 PASS (4 provider-tools gates unchanged + the 2 new pragma-loop
  gates: trigger/replaces/pending-prompt-preserved, and the
  cooldown single-increment pin mirroring CMP-001.1's).
- `go test ./internal/query/ -count=1 -race` — ok (10.4s).
- `go test ./internal/compact/ ./internal/cli/ ./internal/orchestration/ -count=1` — ok.
- `go test ./internal/cli/ -count=1 -race` — ok (added per the CMP-001.2
  re-audit C-5 note that cli had not been race-gated).
- `go test ./... -count=1` — green except the pre-existing
  `TestProviderToolsCLIContract` failure documented in CMP-001.1
  (unrelated: provider-tools acceptance/environment; unchanged).
- `go vet ./internal/query/` clean; the three changed Go files are
  gofmt-clean (pre-existing gofmt drift in other query test files is
  untouched, per the CMP-001.1 note).

Claim boundary: proven locally, deterministically — the default pragma
loop now consults the auto-compact tracker before each request build,
replaces the conversation with the summary while preserving the pending
unanswered prompt verbatim, rewrites the session file, and counts one
model-request iteration per cooldown advance. Subagent engines (nil
autoTracker) are unaffected; `DISABLE_AUTO_COMPACT` unchanged; scoped
orchestration activations and FinalTextOnly capture states do NOT
compact (documented above). Orchestration unscoped state runs run on
forked engines that inherit the root's compaction deps (pre-existing
fork-deps leak, filed as CMP-001.4 F5 — nil-ing fork deps there will
simply disable orchestration compaction; this port is compatible with
either resolution). No live-provider behavior is claimed; window
calibration (CMP-002) and CMP-001.4 remain open.

---

## Revision CMP-001.4a (2026-09-24): fork engines inherit the root's live compaction deps (F5)

Source: queue item CMP-001.4a (the F5 split of CMP-001.4 after the
parent's worker died at the 80-turn cap); triple-corroborated (original
audit F5, cycle-3 critic C-5, cycle-4 critic C-1/CMP-001.3.F1). Every F5
sub-claim was verified against the tree at `c4e8c36` before any change;
nothing was refuted.

### Verification of the payload claims

- ForkFreshConversation copied `compactor`/`autoTracker`/`windowConfig`
  into the child struct literal (`internal/query/engine.go:184-187` at
  `c4e8c36`; the payload's `:165-172` is line drift from the
  CMP-001.1/.2/.3 edits — same code, not a refutation).
- The Agent tool nils only the tracker (`internal/query/subagent.go:113`,
  exact): `compactor`/`windowConfig` were still inherited there. That path
  was nevertheless trigger-safe because the loop guard requires BOTH
  `compactor != nil && autoTracker != nil`
  (`provider_tools_loop.go:108`, `miniswe_loop.go:377`).
- Orchestration persona forks inherit the LIVE deps
  (`internal/orchestration/runner.go:348-355`, exact): both fork paths —
  persistent-conversation states (`:348-350`) and non-persistent persona
  states (`:353-355`) — call the same constructor, and no fork path
  constructs fresh deps (production `SetCompaction`/compDeps injection
  exists only for root engines: `cli/run.go:1120,1225,1617`).
- `AutoTracker` is mutex-less (`internal/compact/auto.go` — plain fields,
  no synchronization): its breaker (`consecutiveFailures`) and cooldown
  (`turnsSinceCompact`) book ONE conversation's model history, so a
  shared instance mixes conversations.
- The consequence chain is live for forks, both directions:
  (a) fork trigger guards pass with the inherited deps
  (`provider_tools_loop.go:108`, `miniswe_loop.go:377` for unscoped runs —
  orchestration persona state engines run exactly that pragma loop via
  `RunStateEvents` → `RunPragmaLoopWithSystemCompletionCheckOptions`,
  `runner.go:969`), so a fork's `RecordFailure`/`RecordSuccess`/
  `IncrementTurn` mutate the ROOT's tracker; (b) the FinalTextOnly
  sub-loop's unconditional `IncrementTurn` (`miniswe_loop.go:313-315`)
  advanced the shared tracker even for scoped capture-state runs.

RED (unchanged tree `c4e8c36`, real fork engines through
`ForkFreshConversation` + the provider-tools loop — the exact Agent-tool
fork shape):
`go test ./internal/query/ -run 'TestForkFreshConversationDoesNotInheritCompactionDeps|TestForkCompactionFailuresDoNotTripRootBreaker' -count=1`
failed with
`fork inherited the root's live AutoTracker — a mutex-less tracker shared across conversations lets persona-fork failures trip the root breaker and persona turns advance the root cooldown (CMP-001.4 F5)`
and
`root breaker contaminated by the fork run: 3 compaction failures counted toward the root (CMP-001.4 F5)`
— a fork whose every compaction fails left the ROOT tracker at
FailureCount 3, so the root's own over-threshold conversation could never
auto-compact again.

### Change (one mechanism): the fork constructor no longer inherits compaction deps

`internal/query/engine.go` `ForkFreshConversation`: the child struct
literal drops `compactor`/`autoTracker`/`windowConfig` (doc comment
updated). All fork paths route through this one constructor — the
exhaustive production call-site list is `subagent.go:110` (Agent tool),
`orchestration/runner.go:348` and `:353` (persona forks) — so nil-ing at
the constructor covers ALL fork paths with one edit. `subagent.go`'s
`sub.autoTracker = nil` is removed as behavior-identical cleanup within
the same mechanism (after the constructor fix it assigned nil to an
already-nil field).

### Gates (this revision)

GREEN after the fix:
- `go test ./internal/query/ -run 'TestForkFreshConversationDoesNotInheritCompactionDeps|TestForkCompactionFailuresDoNotTripRootBreaker' -count=1 -v`
  — 2/2 PASS (fork deps nil, root keeps its own; fork run leaves the
  root breaker clean and the root still auto-compacts).
- `go test ./internal/query/ -count=1 -race` — no races; the only
  failures are the three pre-existing RED gates of queue item CMP-001.4b
  (`autocompact_request_shape_test.go` — request-shape token estimates,
  F6), verified failing identically with this fix stashed (i.e. they are
  not regressions of this change and are out of scope here).
- `go test ./internal/orchestration/ ./internal/session/ -count=1` — ok.
- `go test ./internal/compact/ -count=1` — fails only on the pre-existing
  CMP-001.4c RED gate (`auto_zero_window_test.go`, F8 zero-window
  threshold; unrelated package, untouched here).
- `go test ./internal/cli/ -count=1` — fails only on the pre-existing
  CMP-001.4c RED gates (`compaction_window_calibration_test.go`, F8
  SystemPromptEst calibration). The `TestProviderToolsCLIContract`
  acceptance/environment failure documented in CMP-001.1 did not occur
  in this session's run.
- `go vet ./internal/query/` clean; all three changed/added Go files are
  gofmt-clean.

### Boundary (deliberate semantics, documented)

- Forks now NEVER auto-compact. This is the resolution the CMP-001.3
  revision already recorded as compatible ("nil-ing fork deps there will
  simply disable orchestration compaction"): per-conversation tracker
  state cannot be shared across conversations without exactly this
  breaker/cooldown contamination. Persona fork conversations are bounded
  by orchestration state runs and `MaxTurns`; if fork auto-compaction is
  ever wanted, the fork must construct its OWN tracker/compactor/window
  and rebind `SessionRewrite` to its own store — inheritance is the bug,
  not the wiring shape.
- CMP-001.3.F1's session-rewrite aspect (a fork's compaction rewriting
  the ROOT's session file through the inherited `SessionRewrite` config
  closure) is DEFUSED, not removed: `rewriteSession()` is called only
  from the two compaction success branches
  (`miniswe_loop.go`, `provider_tools_loop.go`), which are unreachable on
  a deps-nil fork. The config field itself is still copied with
  `subCfg := engine.config`; re-enabling fork compaction without
  rebinding it would resurrect that leak.
- The FinalTextOnly/scoped IncrementTurn asymmetry inside one engine
  (CMP-001.3.F2) is untouched; with fork deps nil, its cross-conversation
  exposure is gone, and the within-root drift remains a separate finding.
- The remaining CMP-001.4 splits (.4b precise-token/request-shape
  counting, .4c window calibration + zero-window guard) and CMP-002
  (morphllm ContextWindow) are unchanged and open.

Claim boundary: proven locally, deterministically — fork engines created
by `ForkFreshConversation` carry nil compaction deps in every production
path, and a fork's compaction failures/turns no longer touch the root
engine's breaker or cooldown. No live-provider behavior is claimed.

---

## Revision CMP-001.4b (2026-09-24): the token count ignored the request shape; the precise network-count path was untested and unbounded

Source: queue item CMP-001.4b (the F6 split of CMP-001.4); the three
request-shape RED gates below were drafted by the parent CMP-001.4 worker
before it died at the 80-turn cap, verified RED for the stated reasons on
the unchanged tree (`b0b06c3` + untracked gates), and driven GREEN here.
Every F6 sub-claim was verified against the tree before any change; the
suggestion "consider caching or heuristic-first" was analyzed and
rejected (below), not silently implemented.

### Verification of the payload claims

- Precise branch never tested: CONFIRMED. The pre-existing autocompact
  gates (`internal/query/autocompact_test.go`,
  `internal/query/miniswe_loop_test.go`) all drive
  `pragmaLoopTestProvider`/`failingCompactProvider`, which do not
  implement `provider.TokenCounter` — the `counter.CountTokens` branch
  (provider_tools_loop / miniswe_loop trigger blocks) never executed
  under any test. The only counting provider in the tree is the one this
  revision commits (`countingProvider`).
- Google CountTokens is a network call per request: CONFIRMED.
  `internal/provider/google/provider.go:106-123` —
  `p.client.Models.CountTokens(...)` is the Gemini CountTokens REST API
  through the genai SDK client; one round-trip per call. The CMP-001
  trigger called it once per model-request iteration (and fed8bd7^ had
  called it up to twice per iteration: `autoCompactBeforeRequest` plus
  `isAtBlockingLimit`).
- Estimate counts only APIMessages, diverging from fed8bd7^
  requestTokenCount: CONFIRMED. Both trigger blocks used
  `compact.EstimateConversationTokens(conv.APIMessages())` (messages
  only; no system, no tool schemas) and the precise call sent
  `System: conv.System` with NO tools. `git show fed8bd7^:internal/
  query/loop.go:586-608` — `requestTokenCount` built
  `provider.RequestParams{Model, Messages, System, Tools}` and used it
  for BOTH the precise count and the `EstimateRequestTokens` fallback.
  Nuance verified per loop: in the provider-tools loop the request
  system is `WithCustomSystemPrompt(conv.System)` + MCP status + harness
  manifest + patch guidance and the full toolset (providerToolDefs +
  websearch + subagent + MCP defs) — ALL invisible to both pre-F6
  counts. In the pragma loop `run.System` (custom prompt +
  `pragmaLoopSystemPrompt`) is stored as `Conversation.System` at loop
  start (`setConversationSystemPrompt`, miniswe_loop.go:282), so the
  pre-F6 PRECISE call there did carry the request system, but its
  heuristic counted messages only — gate 3 below failed on exactly that
  path. Both undercounts delay the trigger past the fed8bd7^ semantics.

### RED (unchanged tree, before any change; assertions preserved verbatim)

- `go test ./internal/query/ -run TestProviderToolsLoopAutoCompactEstimateCountsRequestShape -count=1`
  failed: `CompactionStartedEvent count = 0, want 1` — a conversation
  whose messages are far below the 904-token threshold but whose system
  block alone (~3,125 heuristic tokens) is far above it never compacted,
  although every request carries that block.
- `go test ./internal/query/ -run TestProviderToolsLoopPreciseCounterCountsRequestShape -count=1`
  failed: `count request system blocks missing request payload:
  manifest=false conversation-system=true` — the precise call ran (the
  branch was reachable) but counted an unshaped request: raw conversation
  system, no manifest, no tools.
- `go test ./internal/query/ -run TestPragmaLoopAutoCompactEstimateCountsSystemPrompt -count=1`
  failed: `CompactionStartedEvent count = 0, want 1` — a pragma session
  whose fixed system overhead alone (~3,125 heuristic tokens of custom
  prompt) crossed the threshold never triggered.
- Bound gates (drafted with this revision, RED on the request-shape fix
  already applied, so only the bound differs):
  `TestProviderToolsLoopPreciseCounterSkipsCooldownIterations` failed
  with `CountTokens calls = 3, want 2` (iteration 1 inside
  MinTurnsCooldown=2 burned a network call for a decision already fixed
  false), and `TestProviderToolsLoopPreciseCounterStopsAfterBreakerTrips`
  failed with `CountTokens calls = 4, want 3` (every post-breaker
  iteration burned one for the rest of the session).

### Change 1 (one mechanism): count the REQUEST shape — restore the
### fed8bd7^ requestTokenCount semantics

`internal/query/engine.go` gains `requestTokenCount(ctx, resolvedModel,
messages, system, tools)`: build `provider.RequestParams` over the given
request shape, prefer the provider's precise `CountTokens` over that
exact shape, fall back to `compact.EstimateRequestTokens` over the same
shape (verbatim fed8bd7^ body, minus the stale debug.Log).

- `provider_tools_loop.go`: the per-iteration system/tools build was
  HOISTED above the compaction check (single build, reused by the
  request), and the trigger now counts
  `requestTokenCount(..., APIMessages(), system, tools)`. The request
  messages are re-snapshotted after the trigger so a compaction this
  iteration is reflected; system/tools are compaction-invariant
  (compaction rewrites `Conversation.Messages` only) so they are reused.
  The provider-tools request messages are the raw conversation messages
  (`messagesForRequestChecked` only slices and validates — that loop
  applies no tool-result budget), so counting raw `APIMessages()` IS
  counting the request payload.
- `miniswe_loop.go` (pragma loop): the trigger counts
  `requestTokenCount(..., APIMessages(), run.System, nil)` — `run.System`
  is the exact system every pragma request carries
  (`buildPragmaLoopTurnRequest`), and pragma requests carry no tool
  schemas. Unscoped runs (the only ones reaching the trigger) request
  the full conversation.

Gate 2 additionally pins strict parity (`reflect.DeepEqual`) between the
count request's System/Tools and the model request's — count and request
share one hoisted build and cannot diverge again.

### Change 2 (one mechanism): bound the precise path to iterations whose
### decision a count can change

`internal/compact/auto.go` gains `AutoCompactEligible()`: false exactly
when `ShouldAutoCompact` is false for EVERY token count — disabled,
breaker tripped (`consecutiveFailures >= MaxConsecutiveFailures`), or
cooldown active (`compacted && turnsSinceCompact < MinTurnsCooldown`),
the three state checks that short-circuit before the threshold
comparison. Both loops now skip the count (and the trigger) when
ineligible; `IncrementTurn` still runs every iteration — the cooldown
expires by counting model-request iterations (CMP-001 F4 semantics; the
cooldown gate stays green). The breaker never un-trips (RecordSuccess is
unreachable once ShouldAutoCompact is false), so post-breaker sessions
stop counting entirely; every successful compaction skips the next
MinTurnsCooldown iterations' counts.

Pinned by `TestAutoCompactEligibleNeverDisagreesWithShouldAutoCompact`
(internal/compact): across the reachable tracker state matrix,
`ShouldAutoCompact(c) == Eligible() && c >= threshold` for counts
{0, threshold-1, threshold, threshold+1, 1<<30} — the bound can never
suppress a trigger a count would have produced.

### Considered and rejected (with evidence, from the payload's
### "consider caching or heuristic-first")

- Caching: content-keyed count caches cannot hit — between compactions
  the conversation only grows (`appendConversationMessage` is the only
  mutation path; compaction is the only replacement), so every
  iteration's count key is new. A cache would bound nothing.
- Heuristic-first dead band (only call CountTokens when the heuristic
  estimate is within some band of the threshold): the heuristic is
  bytes/4 (`compact/tokens.go`), which undercounts dense content (JSON
  tool schemas: punctuation like `","` tokenizes to ~3 tokens per 3
  bytes, up to ~4x the heuristic) by an unbounded, uncalibratable
  factor; any band suppresses the counter exactly in the iterations
  where the heuristic underestimates the true count — re-introducing
  the trigger-delay defect class this case family exists to fix, with
  no local evidence to size the band. It also contradicts the
  precise-counter contract the RED gates pin: when the provider
  implements TokenCounter, its count decides. Rejected; the count-
  independent eligibility bound (change 2) is the provably-safe subset.

### Gates (this revision)

GREEN after the changes:
- `go test ./internal/query/ -run 'RequestShape|SystemPrompt|PreciseCounter' -count=1 -v`
  — 5/5 F6 gates PASS (3 shape gates incl. strict parity + 2 bound
  gates).
- `go test ./internal/query/ -run 'AutoCompact' -count=1` — the full
  CMP-001 family (trigger/replaces, pending prompt, cooldown
  re-trigger, circuit breaker, session rewrite, fork leak) stays green.
- `go test ./internal/query/ -count=1` and `-count=1 -race` — ok (11.4s
  race).
- `go test ./internal/compact/ -run TestAutoCompactEligible -count=1` —
  ok (property matrix, >50 states).
- `go test ./internal/orchestration/ ./internal/session/ ./internal/provider/google/ -count=1`
  — ok.
- `go test ./... -count=1` — green except the three documented
  pre-existing failures, none introduced here: the sibling CMP-001.4c RED
  gates (`TestShouldAutoCompactZeroWindowNeverTriggers` — untouched
  `ShouldAutoCompact`/`AutoCompactThreshold`; and
  `TestBuildCompactionDepsCalibratesSystemPromptEstToLoopMode` —
  untouched `BuildCompactionDeps` in cli/run.go) and the
  `TestProviderToolsCLIContract` acceptance/environment failure
  documented since CMP-001.1. Both .4c gates fail for their own stated
  F8 reasons (verified by reading the failure output; their subjects are
  code this revision does not touch).
- `go vet ./internal/query/ ./internal/compact/` clean; all six
  changed/added Go files are gofmt-clean.

Claim boundary: proven locally, deterministically — both loops' token
counts now reflect the request shape (messages + the exact system and
tool schemas the request carries, precise counter preferred, heuristic
of the same shape as fallback), and the precise path no longer runs in
iterations whose trigger decision no count can change. No live-provider
behavior is claimed; the Google CountTokens network-call cost is code-
verified, not wire-measured. The counting uses the RAW conversation
messages; if a future change makes the provider-tools request apply
tool-result budgeting (the fed8bd7^ runLoop did), the count would
overestimate versus the budgeted request — the safe direction — and
that divergence would deserve its own gate. CMP-001.4c (F8 window
calibration + zero-window guard) and CMP-002 (morphllm ContextWindow)
remain open; the .4a critic findings C-1..C-4 (config-closure leaks,
persistent-fork relief valve, pragma-loop fork coverage) are queued
separately.

## Revision CMP-001.4c (2026-09-24): the death-spiral reserve ignored the loop's actual request payload; a zero WindowConfig armed compaction instead of disabling it

Source: queue item CMP-001.4c (the F8 split of CMP-001.4); the two RED
gates below (`internal/compact/auto_zero_window_test.go`,
`internal/cli/compaction_window_calibration_test.go`) were pre-drafted by
the capped parent CMP-001.4 worker and kept as this item's baseline.

### Verified (against the pre-fix tree at 411728b)

F8 estimate miscalibration — confirmed for BOTH loop modes:
- `BuildCompactionDeps` (run.go) computed
  `SystemPromptEst = EstimateSystemPromptTokens(PragmaLoopSystemPrompt)`
  ≈ 270 heuristic tokens for every mode. Provider-tools requests never
  carry that prompt at all: their fixed payload is the custom prompt
  (prepended by `WithCustomSystemPrompt`) + the conversation's system
  blocks (re-sent verbatim each iteration, provider_tools_loop.go) +
  MCP status + harness manifest + patch guidance + tool schemas
  (Bash/apply_patch/WebSearch/Agent/MCP). The pragma loop's requests
  carry custom prompt + PragmaLoopSystemPrompt (miniswe_loop.go
  `pragmaLoopSystemPrompt()`), so even the default mode's reserve missed
  the custom prompt.
- The reserve's documented purpose is death-spiral prevention (#24179,
  window.go SystemPromptEst): `compact.ApplyResult` replaces only
  `Conversation.Messages` (compact.go) — the entire fixed payload above
  is re-injected with the first post-compaction request, so the 270-token
  reserve under-counted the re-injection by the custom prompt + system
  blocks + manifest + MCP status + patch guidance + tool schemas. RED:
  `SystemPromptEst = 270, want >= 3000` (provider-tools) / `want >= 1000`
  (pragma) in the calibration gate.
- Scope clarification (not a refutation): "counted nowhere" was already
  historical for the TRIGGER count — F6 (95836f9) counts the request
  shape, system + tools included, at decision time. The RESERVE was the
  remaining uncounted site, which is what this revision fixes.

F8 zero WindowConfig — confirmed mechanically, latent in production:
- `WindowConfig{}` → `EffectiveWindow 0` → `AutoCompactThreshold`
  clamps to 0 (window.go) → `ShouldAutoCompact` true for EVERY token
  count whenever state checks pass → compaction (a provider call that
  replaces the whole conversation) fires every `MinTurnsCooldown` turns
  forever. RED: `ShouldAutoCompact(0, zero WindowConfig) = true`.
- This contradicts the CompactionDeps contract ("pass nil/zero values to
  disable auto-compaction"): live deps + zero window meant
  compact-every-cooldown, not disabled.
- No production caller passes a zero window today (all three
  `SetCompaction` sites go through `BuildCompactionDeps`, ctxWindow
  defaults to 200_000) — the hazard is the API contract plus any future
  provider returning `(0, true)` from `ContextWindow`, which the wiring
  now also rejects. The reachable LIVE variant of threshold-0 is a
  CONFIGURED window smaller than buffer+reserve+maxOutput (e.g.
  googlevertex's 8_192): the guard deliberately does not change that —
  a configured window must still trigger (pinned by the gate) — and that
  small-window every-cooldown behavior remains the open CMP-001.4b
  critic finding C-2 (compaction-invariant overhead vs threshold).

### Changes (one mechanism per defect)

1. Zero-window guard — `internal/compact/auto.go`: `ShouldAutoCompact`
   rejects `wc.ContextWindow <= 0` (unknown window = unknown safe
   threshold = no trigger). `internal/cli/run.go` wiring side:
   `BuildCompactionDeps` ignores non-positive `ContextWindow` lookups
   (keeps the 200_000 default).
2. Reserve calibration — `internal/query/engine.go` adds exported
   `EstimateCompactionReserve()`: mode-aware, mirroring each loop's
   request builder exactly (provider-tools: custom + conversation
   system + MCP status + harness manifest + patch guidance + static tool
   schemas, `providerToolDefs`/WebSearch/Agent; pragma: custom +
   PragmaLoopSystemPrompt). `BuildCompactionDeps` uses it (nil-engine
   fallback keeps a pragma-shape floor; no production path reaches it —
   `RegisterTools` sets `d.Engine` before every call). Boundary:
   injected MCP tool schemas are excluded from the reserve —
   `withMCPToolDefs` needs a live context and the set is dynamic per
   session; the trigger's own F6 count includes them at decision time,
   so only the reserve under-counts, by the MCP schema size alone.

### Gates and adjacent checks

- RED on the pre-fix tree (unchanged code, pre-drafted gates):
  `go test ./internal/compact/ -run TestShouldAutoCompactZeroWindowNeverTriggers`
  → `ShouldAutoCompact(0, zero WindowConfig) = true`;
  `go test ./internal/cli/ -run TestBuildCompactionDepsCalibratesSystemPromptEstToLoopMode`
  → `SystemPromptEst = 270, want >= 3000/1000` (both modes).
- New post-fix composition gates (`internal/query/compaction_reserve_test.go`),
  RED-verified against the old estimation shape (estimator neutered to
  the pre-F8 body, then restored): provider-tools reserve must equal
  tool schemas (645) + manifest/patch blocks (358) = 1003 with empty
  custom/system inputs — the old shape returned 270; pragma reserve must
  equal est(custom+PragmaLoopSystemPrompt) = 1270, NOT counting the
  conversation system the pragma loop replaces — the old shape returned
  270.
- GREEN: all gates pass; `go test ./internal/compact/ ./internal/query/
  ./internal/cli/ -count=1` ok; `./internal/query/ -race` ok;
  orchestration/session/provider-google ok; `go test ./...` green except
  the pre-existing `TestProviderToolsCLIContract` (cmd/pragma), re
  -verified failing identically at HEAD with these changes stashed.
  `go vet ./internal/compact/ ./internal/query/ ./internal/cli/` clean;
  changed/added files gofmt-clean (pre-existing unformatted files in
  these packages were left untouched).

Claim boundary: deterministic local proof only. The reserve is a
bytes/4 heuristic over the exact request-fixed payload and is computed
at wiring time (rebindCompaction refreshes it on model switch); a
mid-session change to the conversation system after wiring is not
re-estimated (safe direction — it can only under-reserve by the delta).
No live-provider behavior is claimed. The small-window threshold-0 case
and the post-compaction relief check (CMP-001.4b C-2) remain open and
are NOT addressed by this revision; configured windows keep triggering
as before.

---

## Revision CMP-002 (2026-09-24): window calibration — per-model route policy table replaces the all-models 1M constant

Queue item CMP-002 (the follow-up filed as out-of-scope above) delivered.
Every payload claim was checked against the code and the recorded logs
before any change; none was refuted outright, two carried unit/location
nuances that are documented below instead.

### Verified (payload claims)

- `internal/provider/morphllm/provider.go:29` hardcoded
  `ContextWindow = 1_000_000` — exact (line 29, const block). Its only
  consumer was the `ContextWindow(modelID)` method, which returned it for
  `DefaultModel` — and `ListModels()` serves exactly that one model, so
  "for all models" held in effect; non-`DefaultModel` IDs already fell
  through to the OpenAI-compatible parent (nuance, not a refutation).
- Live 429 `raw_isl_tokens` evidence, medium class, 200K raw limit,
  recorded 2026-09-11/12 — verified in the skill notes
  (`~/.pragma/skills/harness-self-evolution/SKILL.md`), corroborated by
  `docs/self-evolution-roadmap.md` (post-M4 live observations) and this
  record. The verbatim router message is recorded in
  `~/.pragma/logs/2026-09-11T18-50-33.jsonl`:
  `queue raw_isl_tokens limit reached (current=291066, limit=200000)`
  (retryable, retried to success — RTY-002). The tripping session peaked
  at 206,838 input tokens.
- "With the trigger wired, threshold is ~961k" — arithmetic-consistent:
  `AutoCompactThreshold = ContextWindow − MaxOutput − SystemPromptEst −
  13,000`; with the documented reserve shape (max output 16,384 +
  ~10k system reserve, the exact fixture values in
  `internal/compact/window_test.go`) the threshold is 960,616 ≈ 961k.
- "Unreachable before router pain" — confirmed: every conversation ever
  recorded on this route peaked at or below 345,219 input tokens
  (2026-09-23 post-mortem, zero failures), and the single recorded
  raw-policy trip happened inside a session peaking at 206,838 — both
  far below ~961k.

### Nuances (documented, nothing refuted)

- "~200k" is a RAW-token policy number (input sequence length), not a
  tokenized-window number, and the router escalates request class as
  conversations grow: whale-class sessions ran clean at 265,376 and
  345,219 input tokens with zero failures. 200K raw is therefore not a
  hard per-request ceiling in input-token units — it is the only
  recorded binding raw policy, and the conservative calibration floor.
- The recorded raw↔tokenized ratio is unresolved (roadmap notes ≈2.8×
  for the tripping request; the trip's 291,066 raw against the
  session's 206,838 input peak is ≈1.41×). The calibration
  conservatively treats the recorded raw policy number as the window in
  input-token units rather than dividing by either ratio.
- Evidence location nuance: "skill notes" is
  `~/.pragma/skills/harness-self-evolution/SKILL.md`;
  `~/.claude/skills` does not exist on this machine.

### Changed (one mechanism)

- `internal/provider/morphllm/provider.go`: the all-models
  `ContextWindow = 1_000_000` const is replaced by a per-model route
  policy table `modelContextWindows` whose `DefaultModel` entry is
  `RecordedRouterRawTokenLimit = 200_000` (the recorded medium-class raw
  policy, verbatim 429 quoted in the constant's comment).
  `ContextWindow(modelID)` now resolves per-model from the table; absent
  models still fall through to the parent (unchanged semantics). No
  other file changed.
- Deliverable option A (llmconfig metadata) was examined and rejected
  with evidence: `llmconfig.Config` is the persona/state orchestration
  override surface (consumed by `internal/orchestration/runner.go`'s LLM
  resolver), while the default session route resolves its window through
  `provider.ContextWindow` (`internal/cli/deps.go` token-monitor budget,
  `BuildCompactionDeps` in `internal/cli/run.go`) — llmconfig metadata
  would never reach the wiring this defect lives in. The route policy
  table is where the resolution actually happens.
- Threshold effect: with window 200,000 and the F8-calibrated reserves,
  the trigger fires at roughly 150–161k input tokens — before the
  recorded pain boundary (206,838) and long before the 345,219
  unbounded-growth harm that opened this case.

### RED/GREEN

- RED (production tree unchanged, gates pre-drafted):
  `go test ./internal/provider/morphllm/ -run 'TestMetadata|TestContextWindowCalibratedToRecordedRouterPolicy' -count=1`
  → `TestMetadata: ContextWindow() = (1000000, true)` and the new gate:
  `ContextWindow("morph-glm53-744b") = (1000000, true), want (200000, true)`.
  The new gate also pins reachability: even the zero-reserve upper bound
  of `AutoCompactThreshold` under the calibrated window (187,000) must
  stay below the recorded pain boundary 206,838.
- GREEN after the one-mechanism change: same run `ok`.
- Adjacent: `go build ./...` clean; `go test ./internal/provider/...
  ./internal/compact/ ./internal/query/ ./internal/cli/ -count=1` all ok
  (including `TestProviderToolsCLIContract`, documented as pre-existing
  flaky by earlier revisions — green this run); `go vet` clean on the
  changed package; gofmt clean on both changed files.

Claim boundary: deterministic local proof only. The 200,000 value is a
conservative calibration to the only recorded binding raw policy — the
same posture as the replaced constant's own comment ("keep the local
budget conservative until a live model response provides an exact
integer"). It is NOT a claim that 200K is the model's true context
window: whale-class sessions ran clean at 265,376 and 345,219 input
tokens, so the calibrated window will compact some sessions earlier
than strictly necessary — a documented trade-off (bounded growth and a
reachable trigger over the unverified 1M catalog claim). When a live
response verifies a bigger envelope, the table entry is the single
place to update. No live-provider behavior is claimed.

---

## Revision CMP-001.2.F1 (2026-09-24): the F3 preservation was built
## from the stale pre-compaction snapshot — operator input delivered
## during the in-flight summary call was destroyed

Source: queue item CMP-001.2.F1 (critic finding C-1 from the CMP-001.2
audit cycle). Every payload sub-claim was verified against the tree
(a5621e8-era line numbers in the payload drifted to 133/149/176-184/
348-380 in the working tree — same code, not a refutation) and the
defect was reproduced through the real production path before any
change; nothing was refuted.

### Verified (payload claims)

- The compaction success branch derived its re-append set from the
  PRE-compaction snapshot: `compSnap := engine.store.Snapshot()`
  (provider_tools_loop.go, taken before the trigger decision), the
  summary call `engine.compactor.Compact(ctx, compSnap...)` ran over
  that snapshot, and the success branch computed
  `pending := pendingUnansweredUserPrompts(compSnap.Conversation.Messages)`
  — from the stale snapshot — then wholesale-replaced
  `s.Conversation.Messages` with `[summary + pending]` inside one
  `store.Update`.
- During that summary call (a provider round-trip), the busy-turn CLI
  path (internal/cli/run.go RunInput busy branch) calls
  `rt.Engine.AppendUserInput(input)` and acknowledges with
  `QueuedPromptEvent` — the operator is told the input was queued into
  the conversation. `AppendUserInput` appends DIRECTLY to the store when
  the conversation tail is not a dangling tool_use — and in the
  compaction window the tail is the unanswered turn prompt, so mid-call
  input always takes the direct-append path. The appended message
  landed in the store between compSnap and the wholesale replacement,
  so the replacement (built only from compSnap-derived pending)
  destroyed it: gone from the store, absent from the post-compaction
  model request, and — via the F2 rewrite, which writes whatever the
  store holds — gone from the durable session file too, with no later
  checkpoint to resurrect it.
- The pragma loop (miniswe_loop.go, CMP-001.3's port of "the exact
  CMP-001 mechanism as it stands after revisions .1/.2") inherited the
  identical defect — the same `pending := pendingUnansweredUserPrompts(
  compSnap.Conversation.Messages)` line — so the DEFAULT loop mode was
  equally affected.

### Record correction (this section amends the record's own claims)

The CMP-001.2 F3 section stated: "Mid-turn operator input queued by
INT-001 drains before the next iteration's check and is preserved by
the same rule." That boundary claim was FALSE for input arriving
during the in-flight summary call: such input does not drain (it
appends directly — the tail is not dangling), it is not in compSnap,
and it was wholesale-destroyed. Only input that reached the store
BEFORE the compaction check (parked-then-drained, or direct appends
before iteration start) was preserved by the F3 rule. This revision
fixes the mechanism so the claim holds for the whole mid-turn window;
the sentence above should be read as corrected by this section.

### RED (unchanged production tree, real loop + real busy-turn delivery)

`go test ./internal/query/ -run 'MidCallOperatorInput' -count=1`
(`internal/query/autocompact_midcall_input_test.go`, two gates) failed
on both loops with
`mid-turn operator input (MIDGATE-9c21) was destroyed from the store by
the compaction replacement (CMP-001.2.F1)`
— the gate parks the loop INSIDE the gated compaction summary call
(after compSnap, before the replacement), delivers operator input
through the same entry point the busy-turn path uses
(`Engine.AppendUserInput`), asserts the input is in the store WHILE the
summary is still parked (direct-append precondition pinned — so the
later loss can only be the replacement's doing), releases the summary,
and then asserts the input survives in the store and in the
post-compaction model request. The model-request leg failed behind the
store leg (assertion order); the store leg is the destruction point.
This independently reproduces the audit's "gated-summary probe"
(store + model-request assertions) on the current tree.

### Change (one mechanism): derive the re-appended pending prompts from
### the LIVE store state, inside the atomic replacement

Both compaction success branches (provider_tools_loop.go,
miniswe_loop.go) now compute `pendingUnansweredUserPrompts` from
`s.Conversation.Messages` INSIDE the replacement `store.Update`, never
from compSnap. `StateStore.Update` holds the store mutex across the
whole callback, so reading the unanswered tail and writing the
replacement is atomic with respect to every concurrent store append
(`AppendUserInput`'s direct append, `drainPendingUserInputs`): operator
input landing before the read is preserved as part of the re-appended
pending tail; input landing after the replacement write stays in the
conversation as a normal post-compaction message and reaches the next
request. There is no destruction window left. compSnap remains the
input to the token count and the summary call (decision-time and
summarized state — unchanged semantics).

GREEN with the same gates: both gates pass — the store retains the
mid-call input verbatim, the post-compaction request carries [summary,
pending turn prompt, mid-call input], and the summary-bearing
assertions of the original CMP-001/F3 gates are unchanged.

### Gates (this revision)

- `go test ./internal/query/ -run 'MidCallOperatorInput' -count=1` —
  2/2 PASS (provider-tools + pragma/default mode).
- `go test ./internal/query/ -run 'AutoCompact|MidCall|ParksBehindDangling|DeliversQueuedInput' -count=1`
  — the full CMP-001 family (trigger/replaces, pending prompt,
  cooldown, circuit breaker, request shape, reserve, fork leak,
  session rewrite, pragma-loop gates) plus the INT-001 parking/delivery
  gates — ok.
- `go test ./internal/query/ -count=1` and `-count=1 -race` — ok
  (13.0s race).
- `go test ./internal/compact/ ./internal/session/
  ./internal/orchestration/ ./internal/cli/ -count=1` — ok (incl. the
  F2 session-rewrite gate and the INT-001 CLI queue gate).
- `go test ./... -count=1` — green except the pre-existing
  `TestProviderToolsCLIContract` acceptance/environment failure
  (cmd/pragma), re-verified failing identically at pristine HEAD
  (181bae9) in a disposable worktree with these changes absent.
- `go vet ./internal/query/` clean; all three changed/added Go files
  gofmt-clean.

### Claim boundary

- Store and model-request preservation are pinned directly by the two
  gates, deterministically, through the real loops. The session-file
  leg is not separately gated: the F2 gate
  (`TestAutoCompactRewritesResumableSessionFile`) already pins that the
  rewrite writes whatever the store holds, and the mid-call input's own
  checkpoint persists it when it lands outside the atomic window
  (post-rewrite appends are beyond the rewritten `SessionLastIdx`).
- Parked inputs (operator input delivered while the tail is a dangling
  tool_use) are outside this window and untouched: in the compaction
  window the tail is never dangling, so mid-call input always appends
  directly. Whether the pragma loop should ever drain its parked queue
  (it currently has no drain call site — parked input there survives
  compaction by not being in the store) is a separate open question,
  not introduced or changed by this revision.
- A summary call that observes input arriving DURING it cannot include
  it in the summary text (the summarizer saw only compSnap) — by
  design: the fixed mechanism re-appends such input verbatim AFTER the
  summary instead, which is strictly lossless.
- No live-provider behavior is claimed. CMP-001.2.F2 (rewrite-error
  recovery), CMP-001.2.F3 (manual /compact pending semantics),
  CMP-001.2.F4 (token guard on the re-appended tail) remain open queue
  items, unaffected by this fix.
---

## Revision CMP-001.2.F2 (2026-09-24): no recovery for a failed
## post-compaction session rewrite

Source: queue item CMP-001.2.F2 (critic finding C-2 from the CMP-001.2
audit cycle). Every payload claim was verified against the tree before
any change (payload line numbers 160-166/2063-2091/2107-2144 had drifted
to 210-213/2086-2094/2121-2154 — same code, line drift, not a
refutation); nothing was refuted.

### Verified (payload claims)

- The rewrite error path leaves no recovery: provider_tools_loop.go
  compaction success branch runs `RecordSuccess` (:170) and the
  wholesale `store.Update` replacement (:192) BEFORE the rewrite; on
  `rewriteSession()` error (:210) it emits one turn `ErrorEvent` and
  returns — no repair, no index clamp. The default pragma loop carries
  the identical branch (miniswe_loop.go:461; the payload cites only
  provider-tools, same code via the CMP-001.3 port).
- `rewriteCurrentSession` resets `d.SessionLastIdx = len(messages)` only
  AFTER a successful `Rewrite` (run.go:2154), so the failed rewrite
  leaves the index at its stale pre-compaction value (verified live in
  the RED gate: after the failed rewrite, SessionLastIdx = 7 — six
  seeded messages + the turn prompt already written by the turn-start
  checkpoint — while the compacted store holds 2).
- The next incremental checkpoint then writes NOTHING
  (`for i := d.SessionLastIdx; i < len(...)` never enters with the index
  past the end) and unconditionally resets `SessionLastIdx = len`
  (run.go:2086-2094) — the desync is silently baked in. Later
  checkpoints splice post-compaction messages onto the pre-compaction
  file: the RED gate's reloaded file held 8 messages = the 6-message
  bulk + the run-1 prompt + the run-2 assistant reply, with the summary,
  the preserved prompt re-append, and the run-2 prompt all absent
  (below the stale index). --resume resurrects the bulk history and
  loses the summary/first exchange, exactly as claimed.
- The same stale-index state arises from the manual path when
  `rewriteCurrentSession` fails after `/compact` (runSlash error event,
  run.go:345) — same absence of recovery.

### RED (unchanged tree, real loop + real session writer + real loader)

`go test ./internal/cli/ -run TestSessionCheckpointRecoversFromFailedCompactionRewrite -count=1`
failed with
`resumed session replays 8 messages (>= the 6-message pre-compaction history) — the failed post-compaction rewrite was never repaired: the next checkpoint wrote nothing below the stale index and spliced onto the pre-compaction file`.
The gate drives the real `runProviderToolsLoop` twice: run 1 compacts
(summary + preserved pending prompt, the full F1 mechanism) and the
post-compaction rewrite FAILS — injected at the `EngineConfig.
SessionRewrite` hook seam, the exact interface where a
`rewriteCurrentSession` failure (session load, truncate, encode, sync)
surfaces to the loop; premise legs assert the loop's real
`rewrite session after compaction` error event and the stale index.
Run 2 is the operator's next turn with the production wiring restored
(real `rewriteCurrentSession` hook), so the recovery checkpoint runs
real production code end to end; the file is then reloaded through
`session.Store.Load` exactly as --resume does.

### Change (one mechanism): the incremental checkpoint self-heals a
### desynced index with a full rewrite

`makeSessionSaveClose`'s saveFn (internal/cli/run.go): when
`d.SessionLastIdx > len(snap.Conversation.Messages)` — the store's
message array shrank below the last incrementally-written index, which
only a compaction replacement can do, i.e. exactly the failed-rewrite
state — the checkpoint performs a full `rewriteCurrentSession(d)` (the
same truncate-and-rewrite the compaction success path uses) instead of
silently writing nothing and clamping the index. GREEN with the same
gate: the reloaded file carries [summary, preserved prompt, run-2
prompt, assistant reply] and no bulk history. If the rewrite still
fails (persistent IO failure), the checkpoint now returns a loud
`rewrite session after desynced checkpoint index` error through the
existing checkpoint error path (turn ErrorEvent), replacing the old
silent corruption. One site covers every entry to the state: both
loops' auto-compaction, the manual `/compact` failure, and the
close-time final save (`closeCurrentSession` calls saveFn before
closeFn).

### Gates (this revision)

- RED → GREEN:
  `go test ./internal/cli/ -run TestSessionCheckpointRecoversFromFailedCompactionRewrite -count=1`
  (RED message above; PASS after the fix).
- Adjacent: `go test ./internal/cli/ -count=1` ok — includes the F2
  success gate `TestAutoCompactRewritesResumableSessionFile` (the
  successful rewrite path still resets the index and skips the
  incremental writes, so the recovery branch is a strict addition) and
  the INT-001 CLI gates; `-count=1 -race` ok (the recovery adds no new
  unsynchronized index access — saveFn already read and wrote
  `SessionLastIdx`, and `rewriteCurrentSession` already wrote it).
- `go test ./internal/query/ -run 'AutoCompact|MidCall' -count=1` ok
  (the whole compaction family, both loops);
  `go test ./internal/query/ -count=1` ok (6.8s);
  `./internal/session/ ./internal/compact/ ./internal/slash/` ok.
- `go test ./... -count=1` green except the pre-existing
  `TestProviderToolsCLIContract` acceptance/environment failure
  (cmd/pragma), re-verified failing identically at pristine HEAD
  (81372d6) in a disposable worktree with these changes absent.
- `go vet ./internal/cli/` clean; both changed files gofmt-clean.

### Claim boundary (deliberate semantics, documented)

- The repair runs at the NEXT checkpoint (the next message append, or
  the close-time save), not inside the loop's error branch — the loop
  keeps its documented F2 semantics (a failed rewrite errors the turn).
  A hard crash between the failed rewrite and the next checkpoint still
  leaves the pre-compaction file on disk; resume then reads a
  consistent (stale, bulk) conversation — no splice corruption, but
  the compaction is lost for that file. That residual window needs an
  atomic (temp+rename) rewrite to close, which is a separate mechanism.
- A rewrite whose truncate partially destroyed the file before failing
  may leave the file unloadable (no header) — the recovery's
  `loadCurrentSessionForRewrite` then fails and the checkpoint errors
  loudly instead of silently corrupting; repairing from an in-memory
  header (d.SessionHeader) would be a further change, not made here.
- The loop-side `ErrorEvent` + `return` on rewrite failure is unchanged:
  RecordSuccess is kept (the compaction itself succeeded — the summary
  IS in the store; only its persistence failed), so the cooldown still
  engages and a re-trigger cannot undo the repair.
- The payload's "no repair or index clamp" is confirmed as the defect;
  the fix is the repair, deliberately NOT a clamp — clamping the index
  to the compacted length alone would splice ALL compacted messages
  onto the pre-compaction bulk (duplicated history), which is worse.

---

## Revision CMP-001.2.F3 (2026-09-24): manual /compact swallowed the
## unanswered prompt — the F3 semantics lived only in the auto loops

Source: queue item CMP-001.2.F3 (critic finding C-3 from the CMP-001.2
audit cycle). Every payload claim was verified against the tree before
any change; nothing was refuted. Payload line numbers (commands.go
162-177, compact.go 60-66) drifted to 151-180 / 58-64 — same code, line
drift, not a refutation.

### Verified (payload claims)

- `handleCompact` called `deps.Compactor.Compact` then
  `compact.ApplyResult(deps.Store, result)` with no pending re-append
  (slash/commands.go) — confirmed by reading the handler.
- `compact.ApplyResult` replaced the ENTIRE message array with the
  summary: `s.Conversation.Messages = result.ReplacementMessages`
  (compact.go) — confirmed.
- The F3 "never compact an unanswered prompt" semantics existed only in
  the auto loops: both compaction success branches inlined
  `pendingUnansweredUserPrompts` re-append inside their own
  `store.Update` (provider_tools_loop.go, miniswe_loop.go), while
  `ApplyResult` — whose ONLY production caller was the manual path —
  had none. Confirmed.
- Scenario reachability (the harm claim): confirmed mechanically. On a
  provider error or max-tokens truncation the loops emit ErrorEvent and
  return WITHOUT appending any assistant message, so the turn prompt
  sits unanswered at the conversation tail (the max-tokens path even
  tells the operator "the conversation is preserved — continue with a
  new prompt or --resume"). Slash commands run exactly in that state
  (busy turns reject slash input), so the operator's natural next
  action — `/compact` — compacted the unanswered prompt away with the
  rest of the history: it reached the model only if the summarizer
  happened to retain it, and via `RewriteSession → rewriteCurrentSession`
  (which writes whatever the store holds) it was gone from the durable
  session file too.

### RED (unchanged tree, real production path)

`go test ./internal/slash/ -run TestHandleCompactPreservesUnansweredPrompt -count=1`
failed with
`post-/compact conversation holds 1 messages, want 2 [summary, re-appended prompt] — manual /compact replaced everything with the summary alone and the unanswered operator prompt (RIVERGATE-7f3a) was swallowed (CMP-001.2.F3)`.
The gate drives the REAL `handleCompact` with a real `compact.Service`
over a scripted summary that deliberately omits the marker, so the only
way the prompt could reach the next model request was a re-appended
message; the store held the summary alone.

### Change (one mechanism): the compaction application semantics now
### live in exactly one place — `compact.ApplyResult`

- `compact.ApplyResult` (the application point its own doc comment
  already claimed to be) now derives `PendingUnansweredUserPrompts`
  from the LIVE store state inside its single `store.Update` (the
  CMP-001.2.F1 atomicity semantics) and re-appends them verbatim after
  `result.ReplacementMessages` — the CMP-001.2 F3 semantics, applied to
  every ApplyResult caller, which fixes the manual path.
- The pending detection moved to the compact package as exported
  `PendingUnansweredUserPrompts` (+ `MessageHasToolResult`, which it
  needs; query's tool-pairing validator now calls the same definition);
  the local query copy and the duplicated inline replacement blocks are
  deleted. Both auto loops now apply their compaction results through
  `compact.ApplyResult(engine.store, compResult)` instead of re-stating
  the replacement inline. This is behavior-identical for the loops (the
  inline Update body was exactly the fixed ApplyResult body: live-state
  pending derivation, summary, re-append, timestamp) — pinned by the
  existing gates — and it removes the structural cause of this defect:
  two application sites whose semantics could (and did) diverge, with
  the manual path forgotten when F3 was implemented.

### Gates (this revision)

- RED → GREEN:
  `go test ./internal/slash/ -run TestHandleCompactPreservesUnansweredPrompt -count=1`
  (RED message above; PASS after the fix; the nil-Compactor gate
  unchanged).
- Adjacent (auto paths through the shared ApplyResult, both loops):
  `go test ./internal/query/ -run 'AutoCompact|MidCall' -count=1` — 10/10
  PASS (trigger/replaces, pending-prompt preservation, mid-call operator
  input on BOTH loops, cooldown, circuit breaker, request-shape and
  system-prompt estimate gates) — the loops' behavior through the shared
  application point is indistinguishable from the previous inline
  replacement.
- `go test ./internal/query/ -count=1` ok (6.8s);
  `-count=1 -race` ok (11.4s).
- `go test ./internal/compact/ ./internal/slash/ ./internal/session/
  ./internal/cli/ -count=1` — ok (incl. the F2 session-rewrite and
  checkpoint-recovery gates and the INT-001 CLI gates).
- `go test ./... -count=1` — green except the pre-existing
  `TestProviderToolsCLIContract` acceptance/environment failure
  (cmd/pragma), re-verified failing identically at pristine HEAD
  (f7bab39) in a disposable worktree with these changes absent.
- `go vet ./internal/compact/ ./internal/query/ ./internal/slash/`
  clean; all seven changed/added files gofmt-clean (one trailing
  blank line introduced by the helper deletion in loop.go was fixed).

### Claim boundary

- Proven locally, deterministically, through the real `handleCompact`
  → real `compact.Service` → real `ApplyResult` path: an operator
  prompt left unanswered by an errored turn survives `/compact`
  verbatim, and the post-compaction conversation is [summary,
  unanswered prompt]. The session-file leg follows the store
  (`RewriteSession` → `rewriteCurrentSession` writes what the store
  holds), same reasoning as the CMP-001.2.F1 boundary.
- The auto loops' observable behavior is unchanged (gates above); the
  unification is the mechanism, not a behavior change.
- Manual /compact cannot race operator input the way the auto path
  could (busy turns reject slash commands, so nothing appends during a
  manual summary call); the live-state derivation in ApplyResult is
  nevertheless the same atomic semantics for both paths.
- This revision also closes the manual-path leg of the CMP-001.2.F1
  cycle's critic finding C-1 ("Same-class destruction window remains
  live and ungated in the manual /compact path": operator text submitted
  during the manual compaction summary call — busy-turn AppendUserInput
  under the held input turn — was destroyed by the summary-only
  replacement, with no gate): ApplyResult now derives the pending tail
  from the live store state under the store lock, so such input is
  preserved (input landing before the read joins the re-appended tail)
  and the manual path is gated. The MidCall gates exercise exactly this
  derivation now that the loops apply results through ApplyResult, and
  the new manual gate covers the manual entry point. The F1 cycle's
  remaining critic findings (C-2 internal-message exclusion — deliberate,
  documented there as unreachable today; C-3 coverage gaps on
  later-iteration interleavings) are unaffected by this revision.
- Tool-result and internal (hook-context) messages in the unanswered
  tail are still NOT re-appended — the F3 exclusions (tool_result
  pairing, internal context) apply to the manual path now too, by the
  shared definition.
- No live-provider behavior is claimed. Queue items CMP-001.2.F4
  and CMP-001.4b critic finding C-2 (compaction-invariant overhead vs
  threshold) — CMP-001.2.F1/.F2/.F3/.F4 are now closed; see the F4
  revision below for the token-guard boundary that remains.

## Revision CMP-001.2.F4 (2026-09-24): no token guard on the re-appended
## pending tail — acceptance check and PostTokens measured the summary alone

Source: queue item CMP-001.2.F4 (critic finding C-4 from the CMP-001.2
audit cycle, confidence "plausible"). Every payload claim was verified
against the tree before any change; nothing was refuted. Payload line
numbers drifted (compact.go 133-141/155-161 → the same code now at
164-170 after the F1/F3 revisions reshaped the region;
provider_tools_loop.go 142-149/:172 → 187/:202) — line drift, not a
refutation.

### Verified (payload claims)

- `Service.Compact` computed `postTokens` on `replacements` =
  `[summaryMsg]` alone, and the `ErrCompactionGrew` acceptance check
  (`postTokens >= preTokens`) compared that summary-only count against
  `preTokens` — the token estimate of the WHOLE input conversation,
  pending tail included. Confirmed.
- `CompactResult.PostTokenCount` carried that same summary-only count.
  Confirmed.
- The pending re-append happens later, inside `compact.ApplyResult`'s
  `store.Update` (provider_tools_loop.go:187, miniswe_loop.go:438,
  slash/commands.go handleCompact) — i.e. AFTER the acceptance check
  had already passed. Confirmed.
- `CompactionEvent{PostTokens: compResult.PostTokenCount}`
  (provider_tools_loop.go:202, miniswe_loop.go:452) reported the
  summary-only count to the operator, while `RecordSuccess` and the
  cooldown engaged on `compErr == nil` regardless of the tail's size.
  Confirmed.
- Multi-prompt tail reachability: `PendingUnansweredUserPrompts`
  re-appends EVERY trailing user message after the last assistant
  (walk-back in pending.go; only IsInternal and tool-result messages
  are excluded). An errored turn leaves its prompt unanswered (the
  loops emit ErrorEvent and return without appending an assistant
  message — the F3 scenario), and INT-131 mid-turn input appends
  further user messages behind it, so the tail can hold several
  messages. Confirmed.
- Consequence, verified as arithmetic in the RED gates: with a pending
  tail, the pre-F4 guard compared summary vs (prefix+tail), so a
  summary LARGER than the prefix it replaces was accepted whenever the
  tail was big enough — the accepted "compaction" left the conversation
  LARGER than the original — and PostTokenCount under-reported the
  real post-compaction conversation for every preserved prompt. The
  claim's over-window consequence is real but bounded (see the claim
  boundary below): compaction cannot shrink a pending prompt by policy
  (F3 preserves it verbatim), so what F4 actually repairs is the
  missing growth guard and the dishonest count, not window overflow
  caused by the tail alone.

### Refuted: none.

### RED (unchanged tree, real production path)

Three gates, all failing for the claimed reason at HEAD `06b8c47`:

- `go test ./internal/compact/ -run TestCompactGrewGuardCoversReappendedPendingTail -count=1`
  failed with
  `expected ErrCompactionGrew: the summary replaces only a 20-token prefix while the re-appended pending tail adds 908 tokens back, so the result grows the 928-token conversation — Compact accepted it (err=<nil>)`
  — the real `compact.Service` over a scripted summary; the input is
  the errored-turn-plus-INT-131 shape (4 tiny answered messages, 2
  unanswered trailing user prompts).
- `go test ./internal/compact/ -run TestCompactPostTokensIncludeReappendedPendingTail -count=1`
  failed with
  `PostTokenCount = 50, want 433 (summary + re-appended pending tail) — the count reports the summary alone`.
- `go test ./internal/query/ -run TestProviderToolsLoopCompactionEventPostTokensCoverPendingTail -count=1`
  failed with
  `CompactionEvent.PostTokens = 49 does not even cover the re-appended pending prompt (1504 tokens)`
  — the real `runProviderToolsLoop` compacts the seeded above-threshold
  conversation at iteration 0 with the run prompt as the pending tail.

### Change (one mechanism): the guard and the reported count now measure
### the conversation ApplyResult will leave

In `Service.Compact` (internal/compact/compact.go), the guarded set is
the replacement summary PLUS `PendingUnansweredUserPrompts(messages)`
(the snapshot's trailing unanswered prompts): the `ErrCompactionGrew`
check now compares (summary+tail) against the original (prefix+tail) —
the summary must be smaller than the prefix it actually replaces — and
`CompactResult.PostTokenCount` (which both loops' `CompactionEvent` and
the manual /compact display carry) includes the tail.
`ReplacementMessages` stays `[summaryMsg]`; `ApplyResult` still
re-appends the LIVE tail, so nothing is appended twice. Counted from
the input snapshot, the tail is a lower bound of what ApplyResult
re-appends: operator input delivered during the summary call is
re-appended from live state (the F1 semantics) and reaches the model,
but is not in this event's count — the reported number can lag by
whatever a concurrent AppendUserInput adds, never by the preserved
tail itself.

### Gates (this revision)

- RED → GREEN: the three gates above, all PASS after the fix.
- Adjacent: `go test ./internal/compact/ ./internal/slash/
  ./internal/query/ ./internal/cli/ -count=1` — ok (query includes the
  F3 preserve-pending gates, the mid-call-input gates on both loops,
  cooldown, breaker, and request-shape gates; their summaries stay far
  below their prefixes, so the tightened guard still accepts them).
  `go test ./internal/query/ ./internal/compact/ -count=1 -race` — ok
  (11.4s / 1.6s).
- `go vet ./internal/compact/ ./internal/query/` clean; the changed
  files are gofmt-clean.

### Claim boundary

- When the pending tail ALONE is near or over the window (one huge
  unanswered prompt), a genuinely shrinking compaction is still
  accepted and the post-compaction request stays over-window: inherent
  to the F3 "never compact an unanswered prompt" policy, which must
  deliver the prompt verbatim. What changed is that PostTokens reports
  that honestly (summary+tail, not the summary alone) and the guard
  rejects every result that does not actually shrink the conversation.
- Mid-summary-call operator input (the F1 window) is re-appended from
  live state but not counted in this compaction's event (the count is
  snapshot-derived, a lower bound). The F1 MidCall gates are unaffected.
- `MessagesRemoved` (the "(N messages removed)" half of the manual
  display) still counts len(input messages) — still overstated when a
  tail is preserved; that is queue item CMP-001.2.F3.F2, unchanged
  here.
- The second-compaction shape (an old IsCompactSummary message walked
  back into the pending set — queue item CMP-001.2.F3.F1) is unchanged:
  this revision only changes the guard/count inputs; if F3.F1 is fixed
  by excluding summaries from the pending set, the guard input
  inherits that fix through the shared `PendingUnansweredUserPrompts`
  definition.
- No live-provider behavior is claimed.

## Revision CMP-001.2.F5 (2026-09-24): loop-side rewriteCurrentSession is
## a second unsynchronized writer of SessionLastIdx (critic C-5)

Queue item CMP-001.2.F5, severity low, confidence plausible. Payload
claim: the new loop-side rewriteCurrentSession adds a second
unsynchronized writer of `d.SessionLastIdx` alongside the UI-goroutine
`AppendUserInput → checkpointSession → saveFn` path — a genuine data
race during the operator-input-during-compaction window; the session
Writer itself is mutex-safe so the exposure is the shared index int;
and the cli package was not gated under `-race`.

### Verified (payload claims, against HEAD 294f00f)

- rewriteCurrentSession (run.go:2149-2182) writes
  `d.SessionLastIdx = len(snap.Conversation.Messages)` (:2182) with no
  synchronization, on the loop goroutine — it is the body of every
  `SetSessionRewrite` hook (run.go:451/822/1226/1642) and runs right
  after auto-compaction applies its result
  (provider_tools_loop.go:197, miniswe_loop.go:447).
- saveFn (makeSessionSaveClose, run.go:2086-2152) reads
  `d.SessionLastIdx` (:2095 guard, :2113 loop) and writes it (:2121),
  and is reachable from the UI goroutine: RunInput:254 →
  `Engine.AppendUserInput` (provider_tools_loop.go:348-382) →
  checkpointSession (engine.go:453-463) → the SetSessionCheckpoint
  saveFn wired next to every SetSessionRewrite.
- No lock in common: AppendUserInput holds only `pendingUserInputsMu`
  during its checkpoint, which the compaction rewrite path does not
  take — the two run unsynchronized. The session Writer is
  mutex-protected (internal/session/writer.go `mu sync.Mutex`), the
  index is not.
- The CMP-001.2 gate list had no cli `-race` run (only
  `./internal/query` was race-gated).
- Beyond the payload: the window is not compaction-specific. Since
  INT-001 (2026-09-12) the loop-side engine appends
  (provider_tools_loop.go:36/66/80/245/268/324 →
  appendConversationMessage → checkpointSession → saveFn, no
  `pendingUserInputsMu` held) already raced the UI-side saveFn on the
  same index; F5's rewriteCurrentSession added a third concurrent
  writer. One persistence lock closes the whole class.

### Evidence inaccuracies (claim survives them)

- rewriteCurrentSession does not READ `d.SessionLastIdx`; it writes it
  (it also reads/writes `d.SessionHeader` at :2151-2155 — the same
  unsynchronized exposure, covered by the same fix).
- The payload's line ranges drift ~40 lines from HEAD (":2063-2091"
  and ":2107-2144" are saveFn ~:2086-2152 and rewriteCurrentSession
  ~:2149-2182); "run.go:254" and "provider_tools_loop.go:349-357" are
  exact/near-exact (AppendUserInput call :254, message construction
  :349-357, checkpoint :382).
- Severity "low" confirmed: the race is a memory-model violation with
  a possible (not deterministic) interleaved persistence — a torn
  aligned int is not observable on amd64/arm64, and the file-level
  operations are individually serialized by the Writer mutex.

### RED (unchanged tree at 294f00f, in a scratch worktree, `-race`)

New regression `TestOperatorInputDuringCompactionRewriteIsSerialized`
(internal/cli/autocompact_session_race_test.go): both goroutines of
the window run the exact production functions — the loop side calls
rewriteCurrentSession (the SetSessionRewrite hook body) 32×, the UI
side calls engine.AppendUserInput (INT-001 mid-turn path, through the
real saveFn checkpoint) 32×, over a real resumed-session JSONL. The
compaction TRIGGER is driven directly (no provider compaction
request) because the subject is persistence serialization, not the
trigger; the end-to-end trigger path stays covered by
TestAutoCompactRewritesResumableSessionFile.

`go test ./internal/cli -run TestOperatorInputDuringCompactionRewriteIsSerialized -count=1 -race`:

```
WARNING: DATA RACE
Write at 0x00c000469820 by goroutine 22: internal/cli/run.go:2121
  (saveFn) ← query/engine.go:462 checkpointSession ←
  query/provider_tools_loop.go:382 AppendUserInput
Previous write at 0x00c000469820 by goroutine 21:
  internal/cli/run.go:2182 (rewriteCurrentSession, loop side)
WARNING: DATA RACE
Write at 0x00c000469820 by goroutine 21: internal/cli/run.go:2182
  (rewriteCurrentSession)
Previous read at 0x00c000469820 by goroutine 22: internal/cli/run.go:2095
  (saveFn index guard)
--- FAIL: race detected during execution of test
```

Without `-race` the same test PASSES on the unfixed tree: the
functional assertions (index == live length after the window; all 32
mid-window operator inputs durable; seeds preserved) are consistency
invariants of the GREEN state, not a deterministic message-loss
repro — consistent with severity "low" and with the Writer's own
mutex keeping individual entries intact.

### Change (one mechanism): a session-persistence mutex

`Deps.sessionMu sync.Mutex` now serializes durable session persistence
across goroutines: makeSessionSaveClose's saveFn takes it for the whole
checkpoint (index read/write + incremental writes + metadata), and
rewriteCurrentSession takes it around the full rewrite
(`rewriteSessionLocked`, extracted so the saveFn desync recovery —
which already holds the lock — rewrites without re-locking). The
one-time setup writers of SessionLastIdx/SessionHeader
(startSessionForCurrentConversation, Resume, clear) stay lock-free:
they run at admission-serialized points where no engine loop or
queued-input checkpoint can be in flight (slash commands are
busy-rejected mid-turn). Lock order is uniformly
`pendingUserInputsMu → sessionMu → Writer.mu`; no inverse order
exists, so no new deadlock surface.

### Gates (this revision)

- RED → GREEN: the command above, FAIL (race report) at 294f00f →
  PASS at the fixed tree (`-race`, 2.3s).
- `go test ./internal/cli -count=1 -race` — ok (2.7s; the gate the
  critic noted missing from CMP-001.2 — now run and clean, 31 tests).
- `go test ./internal/cli -count=1` — ok (31 tests, includes the F2
  rewrite, F2-recovery, and INT-001 queue gates).
- `go test ./internal/query -run 'TestProviderToolsLoopAutoCompact'
  -count=1` — ok; `go test ./internal/query ./internal/compact
  ./internal/session -count=1` — ok; `go test ./internal/query
  -count=1 -race` — ok (11.5s).
- `go vet ./internal/cli ./internal/query` — clean; the changed files
  (deps.go, run.go, autocompact_session_race_test.go) gofmt-clean.

### Claim boundary

- The `closed` flag shared by saveFn/closeFn in makeSessionSaveClose
  remains unsynchronized: closeFn runs at teardown after the engine
  loop has ended; if a shutdown-vs-checkpoint race ever becomes
  reachable it is a separate case.
- No message loss is claimed as reproduced on the unfixed tree: the
  defect is the unsynchronized shared state (Go memory model) and the
  possibility of an incremental checkpoint splicing onto a mid-rewrite
  file; the race report is the deterministic evidence.
- No live-provider behavior is claimed.

### Provenance note

The fix was committed from a working tree that carried an unrelated
uncommitted instrumentation pass (observe.GlobalTrace additions and
comment strips in internal/metaobserve, internal/watcher,
internal/compact, internal/cli/run.go — another live session's in-
flight work). The commit was staged selectively to contain only this
revision's files/hunks; the instrumentation pass is untouched in the
working tree.

---

## Revision CMP-001.3.F1 (2026-09-24): the pragma-loop trigger on
## orchestration persistent-state forks — every payload leg verified at
## the audited commit, already defused at HEAD by CMP-001.4a; the path is
## now gated

Source: queue item CMP-001.3.F1 (critic finding C-1, severity medium,
confidence confirmed). The payload audited commit `1b4fb1b` (the
CMP-001.3 port) — established by exact line matches: engine.go:181-183,
miniswe_loop.go:370/431, runner.go:348/961/864 all resolve to the cited
code at `1b4fb1b`.

### Verified (every payload claim reproduced at the audited commit)

- Persistent orchestration states fork via `root.ForkFreshConversation`
  (runner.go:348) and run the pragma loop UNSCOPED
  (`IncludePriorConversation: stateUsesPersistentConversation`,
  runner.go:961 — unscoped means `len(run.MessageStartIndexes) == 0`
  passes the CMP-001.3 trigger gate, miniswe_loop.go:370).
- The fork constructor copied `compactor`/`autoTracker`/`windowConfig`
  (engine.go:181-183 at 1b4fb1b) and `subCfg := engine.config` copied
  the root's `SessionRewrite` closure.
- The fork's compaction success branch called `engine.rewriteSession()`
  (miniswe_loop.go:431 at 1b4fb1b) → the root's closure →
  `rewriteCurrentSession` (run.go:2107) — snapshots the ROOT `d.Store`
  (:2121), rewrites the ROOT's durable session file, resets the ROOT
  `SessionLastIdx` (:2140). The payload's ":2112,2131" is line drift
  inside the same function.
- One mutex-less `AutoTracker` shared root↔fork (auto.go:43-48 breaker
  inside `ShouldAutoCompact`; no un-trip — `RecordSuccess` is
  unreachable once tripped), so fork failures/cooldowns booked the
  root's conversation.
- `BindProvider` (runner.go:864, applyStateLLMRuntime) rebinds the
  fork's provider; the compactor kept the ROOT's provider/model —
  fork compaction summarized through the root's route even after the
  persona rebind.
- "The leak was dormant before this commit": before CMP-001.3 the pragma
  loop had no trigger at all, so persistent-state forks could never
  compact regardless of inherited deps.
- Executable RED (the gate below) at `1b4fb1b`, probe output over the
  real fork run: `started=2 compacted=1 failed=1 complete=1
  rootRewritten=true rootProv.calls=1 rootFailures=0 rootEligible=false`
  — every leg live and SILENT: the fork compacted twice (the first
  attempt failed ErrTooFewMessages over 3 messages — `RecordFailure` on
  the ROOT's tracker; the second succeeded — `RecordSuccess` cleared
  the failure count and engaged the ROOT's cooldown), the fork's
  compaction rewrote the ROOT's session through the inherited closure,
  and the summary went through the ROOT's provider after the persona
  rebind, with the run completing normally.

### Refuted at HEAD (`a104fcb`): the defect no longer exists — CMP-001.4a
### (`a229500`) already removed the fork inheritance

- `ForkFreshConversation` drops `compactor`/`autoTracker`/`windowConfig`
  for ALL fork paths (gated by
  `TestForkFreshConversationDoesNotInheritCompactionDeps`); no
  orchestration path re-injects deps (`SetCompaction` callers are
  cli/run.go root engines only — grep-verified; `BindProvider` swaps
  provider/model only).
- Therefore the pragma-loop trigger guard (`compactor != nil &&
  autoTracker != nil`, miniswe_loop.go:377) fails on every fork: no
  `CompactionStartedEvent`, `rewriteSession` unreachable, root tracker
  untouched, root provider never consulted by a fork. The payload's
  consequence chain is defused at the constructor.
- The `SessionRewrite` closure is still copied via
  `subCfg := engine.config` (verified) — structurally present but
  unreachable (`rewriteSession` has exactly two callers, both
  compaction success branches). Documented as "DEFUSED, not removed" in
  the CMP-001.4a revision.
- The payload's "none of this is tested or documented beyond the
  generic CMP-001.4 F5 note" was true at `1b4fb1b` but stale at HEAD:
  CMP-001.4a's revision documents the fork-deps fix and gates the
  constructor plus the provider-tools fork shape. The pragma-loop
  persistent-fork shape — this payload's exact path — WAS the
  remaining untested surface; closing it is this revision's change.

### Change (test-only, one gate): the pragma-loop persistent-fork path
### is now pinned

`internal/query/autocompact_fork_pragma_test.go`
`TestPersistentForkPragmaLoopNeverCompactsOrRewritesRootSession`: the
exact production chain — a root engine with LIVE compaction deps
(over-threshold root conversation) and a `SessionRewrite` hook,
`root.ForkFreshConversation()` (the exact orchestration constructor
call), a persona `BindProvider` rebind, and an UNSCOPED pragma-loop run
(`RunPragmaLoopWithSystemCompletionCheckOptions` with
`IncludePriorConversation`, the exact `RunStateEvents` invocation shape)
whose conversation grows far over the threshold across bash-block turns.
Asserts: zero compaction events of any kind, the run completes normally
(TurnComplete=1, no ErrorEvent), the root's rewrite hook never fires,
the root's compaction provider is never called, and the root's tracker
stays clean (FailureCount=0, still ShouldAutoCompact-eligible). NO
production change: the defect was already repaired by the sibling item,
and the worker rules forbid production changes for refuted claims; the
gate is the cheapest additional test for the remaining claim
(methodology step 5) — it turns the CMP-001.4a "defused, not removed"
boundary into an executable invariant, so re-enabling fork compaction
without giving the fork its own deps and its own SessionRewrite cannot
pass silently.

### Gates (this revision)

- RED at `1b4fb1b` (disposable worktree, the committed gate file
  verbatim):
  `CompactionStartedEvent count = 2, want 0 — the persistent-state
  fork's compaction deps activated auto-compaction on the unscoped
  pragma-loop run (CMP-001.3.F1)` (plus the probe legs quoted above).
- GREEN at HEAD: the same gate PASS.
- `go test ./internal/query/ -run 'Fork' -count=1` — 4/4 (the two
  CMP-001.4a gates + this gate + the subagent turn-cap fork gate).
- `go test ./internal/query/ -count=1` ok (6.7s); `-count=1 -race` ok
  (11.1s); the full compaction family
  (`AutoCompact|MidCall|Fork|RequestShape|SystemPrompt|PreciseCounter|Reserve`)
  ok.
- `go test ./internal/orchestration/ ./internal/compact/
  ./internal/session/ ./internal/cli/ -count=1` — ok.
- `go test ./... -count=1` — green except the pre-existing
  `TestProviderToolsCLIContract` acceptance/environment failure
  (cmd/pragma), documented since CMP-001.1; a test-only addition to
  internal/query cannot affect that package's acceptance binary.
- `go vet ./internal/query/` clean; the new file gofmt-clean.

### Claim boundary / record corrections

- The CMP-001.3 revision's claim-boundary sentence "Orchestration
  unscoped state runs run on forked engines that inherit the root's
  compaction deps (pre-existing fork-deps leak, filed as CMP-001.4
  F5...)" described the tree as of `1b4fb1b` and is superseded at HEAD:
  fork engines carry nil compaction deps, so orchestration unscoped
  persona/persistent state runs execute with auto-compaction DISABLED —
  the CMP-001.4a boundary (forks never auto-compact; re-enabling
  requires the fork to construct its own deps and rebind its own
  SessionRewrite to its own store).
- Non-persona persistent states (`stateRunsPersona` false) run on the
  ROOT engine, where trigger/rewrite/tracker are the root's own —
  correct by the CMP-001.3 design and outside this finding.
- Adjacent observed behavior, deliberately unchanged: the fork's
  inherited `SessionCheckpoint` closure (config copy) invokes the
  root's saveFn on fork appends — pre-existing since forks exist,
  orthogonal to compaction (the checkpoint persists the root store's
  own state; only compaction reaches `SessionRewrite`), noted here as
  an observation, not a defect claim.
- No live-provider behavior is claimed.

---

## Revision CMP-001.3.F2 (2026-09-24): the port scoped IncrementTurn along
## with the trigger — scoped iterations stopped advancing the shared
## cooldown

Source: queue item CMP-001.3.F2 (critic finding C-2, low severity,
confirmed, from the CMP-001.3 re-audit). Every sub-claim was verified
against the tree at `9010d35` (unrelated in-flight working-tree edits
stashed for the verification, restored after) before any change.

### Verification of the payload claims

- CONFIRMED (structural): the CMP-001.3 port's per-iteration
  `IncrementTurn` sits inside the unscoped-only gate
  (`len(run.MessageStartIndexes) == 0 && engine.compactor != nil &&
  engine.autoTracker != nil`, `internal/query/miniswe_loop.go:376` at
  9010d35; the increment at :459). At the audited port commit `1b4fb1b`
  the same structure held (gate :376, increment :443-444 — the payload's
  ":370,442" is that commit's comment/increment lines). Scoped main-loop
  iterations therefore advance the cooldown ZERO turns per
  model-request iteration.
- CONFIRMED (the removed baseline): the pre-port site was unconditional
  post-assistant — `git show 1b4fb1b^:internal/query/miniswe_loop.go`
  lines 370-374 carry `if engine.autoTracker != nil { ...
  IncrementTurn() }` after `appendPragmaLoopAssistantTurn`, with no
  scope condition, exactly as the payload cites. The port deleted it and
  re-added the increment inside the scope gate — an undocumented
  behavior change for scoped runs, not a preservation.
- CONFIRMED: the FinalTextOnly sub-loop's increment stays unconditional
  (`miniswe_loop.go:312-315`, gated only on tracker presence) — the two
  sub-paths of `runPragmaLoopWithInitialPrompt` carried opposite
  increment semantics.
- CONFIRMED: the claimed invariant "one IncrementTurn per
  model-request iteration" (the port's own commit message; the code
  comment "IncrementTurn below is the ONE per-iteration turn advance";
  `internal/compact/auto.go` "IncrementTurn must still run every
  iteration ... the cooldown expires by counting model-request
  iterations") was false for scoped iterations, and the drift direction
  is safe: fewer increments only delay cooldown expiry (later
  re-compaction); the breaker is untouched.
- PARTIALLY REFUTED (premise stale at HEAD): the parenthetical
  "(orchestration state runs on shared-tracker engines)" was true at
  `1b4fb1b` but is superseded at HEAD: CMP-001.4a (`a229500`)
  nil'd fork compaction deps, so persona/persistent state runs —
  `engineForOrchestrationState` forks — carry a nil tracker and advance
  nothing. The remaining LIVE shared-tracker scoped path is the
  root-engine run: a persona-less control state with
  `foreach_next`/`handoff_mode: persona` gets the ROOT engine
  (`stateRunsPersona` false → root), and its persona handoff then runs
  the pragma loop scoped on it (`runPersonaForState` → `RunStateEvents`
  → `RunPragmaLoopWithSystemCompletionCheckOptions`, runner.go:740/828/
  969). Validation permits that shape
  (orchestration.go:543-551 only constrains personas that ARE set);
  shipped definitions do not use it (`task-evidence-item-loop.yaml`'s
  `next_item` carries its own persona → fork). The defect is therefore
  real on the reachable-but-currently-unused root path — and the
  within-root asymmetry (FinalTextOnly vs scoped) is real on any
  live-tracker engine.
- PARTIALLY REFUTED ("undocumented"): CMP-001.4a's boundary section
  already recorded the FinalTextOnly/scoped increment asymmetry as a
  deferred separate finding (this item), so the record was not silent.
  But the mechanism-level documentation asserted the opposite: the
  gate comment claimed the increment is "the ONE per-iteration turn
  advance" while scoped iterations skipped it, and the CMP-001.3 claim
  boundary said "counts one model-request iteration per cooldown
  advance" with no scoped exception. Documented-as-deferred, wrong at
  the mechanism site — treated as a defect, not a documentation nit.

### RED (unchanged tree `9010d35`, real production paths only)

`internal/query/autocompact_scoped_increment_test.go`
`TestPragmaLoopScopedIterationsAdvanceSharedCooldown`: an engine with
live compaction deps over an over-threshold conversation runs, via the
real entry points — `Engine.Run` (unscoped, the plain `pragma` session
dispatch) and `RunPragmaLoopWithSystemCompletionCheckOptions` with
`PragmaLoopRunOptions{}` (scoped, the exact orchestration
non-persistent state shape) — three turns: run 1 compacts and ends one
iteration later (cooldown 1/2); run 2 is the scoped run (two
model-request iterations); run 3 must compact on its FIRST iteration
because the scoped run's two increments expired the cooldown.
Failed with:
`run 3 CompactionStartedEvent count = 0, want 1 — the scoped run's 2
model-request iterations did not advance the shared cooldown
(CMP-001.3.F2)`.

### Change (one mechanism): IncrementTurn hoisted out of the scope gate

`internal/query/miniswe_loop.go` main loop: `IncrementTurn` moved from
inside the `len(run.MessageStartIndexes) == 0 && ...` gate to its own
block right after it, gated only on `engine.autoTracker != nil` —
identical semantics to the FinalTextOnly sub-loop of the same function
(one increment per model-request iteration, tracker presence the only
condition). The TRIGGER stays scope-gated (compaction would invalidate
the scope's start index — load-bearing, unchanged, and pinned by the
new gate's run-2 assertion of zero compaction events). For unscoped
runs nothing moves: the increment still executes exactly once per
iteration, at the same position (after the trigger check, before the
request build), so the CMP-001.1 F4 single-increment semantics are
preserved and the compaction's own iteration still counts as one turn.
The gate comment now states the scope gate skips the trigger and only
the trigger.

### Gates (this revision)

- RED above at unchanged `9010d35`; GREEN after the one change:
  `go test ./internal/query/ -run 'TestPragmaLoopScopedIterationsAdvanceSharedCooldown' -count=1 -v`
  — PASS.
- Adjacent compaction family
  (`TestPragmaLoopAutoCompact*|TestProviderToolsLoopAutoCompact*|TestPersistentForkPragmaLoop*|TestFork*`)
  — 13/13 PASS, including the CMP-001.1 F4 single-increment cooldown
  pin for the unscoped path, the CMP-001.3 trigger/replaces gate, and
  the CMP-001.4a/CMP-001.3.F1 fork gates (forks carry nil trackers —
  unchanged by this fix).
- `go test ./internal/query/ -count=1` ok (6.6s); `-count=1 -race` ok
  (11.3s).
- `go test ./internal/compact/ ./internal/orchestration/
  ./internal/session/ ./internal/cli/ -count=1` — ok.
- `go vet ./internal/query/` clean; both changed files gofmt-clean.

### Claim boundary

- Scoped pragma-loop iterations on tracker-bearing engines now advance
  the shared cooldown one turn per model-request iteration — restoring
  the removed `1b4fb1b^` semantics and unifying the main loop with the
  FinalTextOnly sub-path. Scoped runs still NEVER compact (the trigger
  remains scope-gated; gated by the new test). Fork/subagent engines
  (nil trackers) are unaffected — behaviorally identical before and
  after.
- Observable-resolution limit, documented: the compaction trigger is
  binary, so the new gate pins "the scoped run's iterations advanced
  the cooldown across the 1/2 → expired boundary" — an at-least-one
  pin per scoped run — while "exactly one (not two) per scoped
  iteration" is not separable through compaction behavior (a scoped
  run cannot compact, so intra-run boundaries are invisible). Exactly-
  one is structurally the same single statement the F4 gate pins for
  the unscoped path; a second increment site remains absent.
- No live-provider behavior is claimed. The payload's "safe direction"
  assessment is confirmed in the RED trace: without scoped increments,
  re-compaction was only delayed (run 3 would have compacted one
  iteration later), never armed early.

---

## Revision CMP-001.3.F3 (2026-09-24): the FinalTextOnly sub-loop had
## no unscoped trigger — the record overstated coverage for persistent
## final_text-capture states

Source: queue item CMP-001.3.F3 (critic finding C-3, severity low,
confidence plausible, from the CMP-001.3 audit cycle). Every payload
sub-claim was verified against the tree before any change; nothing was
refuted. One payload line range needed reconciliation (below).

### Verified (payload claims, against the pre-fix tree)

- "Independent predicates" — CONFIRMED: `RunStateEvents` builds
  `query.PragmaLoopRunOptions{IncludePriorConversation:
  stateUsesPersistentConversation(state), FinalTextOnly:
  stateCapturesFinalText(state), ...}` (runner.go:960-963, exact — the
  payload's :961-963). `stateUsesPersistentConversation` is
  `state.Conversation == "persistent"` (runner.go:365-370);
  `stateCapturesFinalText` is "any output artifact with
  `RuntimeCapture.Type == "final_text"`" (runner.go:1698-1708, exact).
  Nothing couples them: `validateStateExecution` rejects
  persistent-without-persona (orchestration.go:474-477) but no rule
  rejects persistent + final_text, and `validateArtifactList` only
  checks the capture type's membership in {command_evidence,
  final_text}. No shipped definition combines the two today
  (`swe_validation_gate` in orchestrations/swe-bench-pro-engineering-
  loop.yaml is final_text but scoped; `swe_single_engineer` is
  persistent but command_evidence) — the combination is valid-but-
  currently-unused, which is why the gap was never observed live.
- "Runs the untouched FinalTextOnly sub-loop with no compaction trigger
  even over threshold" — CONFIRMED: `runPragmaLoopWithInitialPrompt`
  dispatched `if opts.FinalTextOnly` BEFORE the main loop
  (miniswe_loop.go:289 pre-fix), returns on every path, and the
  sub-loop body (289-336) contained no `ShouldAutoCompact` — the only
  pragma-loop trigger was the main-loop block (:371-454 pre-fix; the
  payload's ":290-341" is that sub-loop region — same code, line drift
  from the .F2 revision's edits, not a refutation). The sub-loop DID
  increment the shared tracker per iteration (CMP-001.3.F2), so it
  booked cooldown turns on a trigger it could never fire.
- "Unscoped ... even over threshold" — CONFIRMED: with
  `IncludePriorConversation` the run passes NO message-start index
  (`RunPragmaLoopWithSystemCompletionCheckOptions`), so sub-loop
  requests carry the FULL accumulated conversation — a persistent
  capture state re-entered on an over-window fork (or a run whose
  rejected drafts accumulate — each rejection appends an assistant
  draft plus a corrective user message, up to MaxTurns) sent
  over-window requests with no compaction anywhere on its path.
- "The stated rationale ('not the default mode') does not address this
  combination" — CONFIRMED: the CMP-001.3 section's coverage sentence
  ("Unscoped pragma-loop runs — plain `pragma` interactive and
  `--prompt` sessions, and orchestration persistent-conversation
  states — carry the trigger") puts persistent-conversation states IN
  the trigger-carrying set, while its FinalTextOnly rationale ("that
  path never compacts and is not the default mode") addressed only the
  default-mode question. The two claims are disjoint for plain
  sessions but overlap exactly on persistent capture states: an
  unscoped run that takes the triggerless sub-loop. The record
  overstated coverage.
- Production-reachability nuance (documented, NOT a refutation): at
  HEAD every persistent state requires a persona (validation), so
  `engineForOrchestrationState` (runner.go:333-355) always returns a
  fork, and CMP-001.4a nil'd fork compaction deps — a persistent
  capture state therefore lacked the trigger for TWO independent
  reasons (nil deps AND the triggerless sub-loop). The CMP-001.3.F1
  revision already corrected the record's persistent-state leg for the
  deps reason. This finding's unique, still-live leg is the second one:
  even on a deps-bearing engine (the CMP-001.4a re-enable path: "the
  fork must construct its OWN tracker/compactor/window and rebind
  SessionRewrite to its own store"), a persistent capture state would
  STILL not compact, because the sub-loop never consulted the tracker
  at all.

### RED (unchanged tree `ef1cbd6`, real production entry points)

`internal/query/autocompact_finaltext_test.go` (committed with this
revision), three gates driving the REAL
`Engine.RunPragmaLoopWithSystemCompletionCheckOptions` with the exact
`RunStateEvents` options shape for a persistent final_text-capture
state (`PragmaLoopRunOptions{IncludePriorConversation: true,
FinalTextOnly: true, FinalTextCheck: ...}`) over live compaction deps
(threshold 904) and a ~22.5k-token conversation:

- `TestPragmaLoopFinalTextOnlyUnscopedAutoCompactTriggersAndReplaces`
  FAILED with `CompactionStartedEvent count = 0, want 1 — the
  unscoped FinalTextOnly sub-loop (the orchestration
  persistent-capture-state shape) never consulted the auto-compact
  tracker even over threshold (CMP-001.3.F3)`.
- `TestPragmaLoopFinalTextOnlyCooldownBlocksImmediateRetrigger` FAILED
  with `CompactionStartedEvent = 0, CompactionEvent = 0, want 1/1` —
  the same missing trigger observed through the cooldown scenario (a
  deliberately huge summary so only MinTurnsCooldown can block the
  re-trigger; this gate doubles as the CMP-001.1 F4 single-increment
  pin for the port — a ported block carrying its own IncrementTurn
  would expire the cooldown inside the compaction's own iteration and
  re-compact at k+1).
- `TestPragmaLoopFinalTextOnlyScopedSkipsCompaction` PASSED pre-fix
  (trivially — there was no trigger) and is the load-bearing boundary
  pin for the port: a SCOPED FinalTextOnly run (no
  IncludePriorConversation — the shape of a root-engine capture state)
  must NEVER compact, even with live deps over an over-threshold
  conversation, because compaction replaces the WHOLE conversation and
  would invalidate the scope's start index; it also pins the scoped
  request slicing (no prior-conversation text, the turn prompt
  present).

### Change (one mechanism): the unscoped trigger now has ONE
### implementation, called by BOTH sub-paths of the loop

`internal/query/miniswe_loop.go`: the main loop's CMP-001.3 trigger
block (with all its accumulated semantics — CMP-001.1 F4
single-increment, CMP-001.2 F1/F2/F3 application, CMP-001.4 F6
request-shape count + eligibility bound) is extracted verbatim into
`Engine.maybeAutoCompactPragmaLoop(ctx, ch, run) bool` — one
implementation, returning false only when the caller must abort
(failed post-compaction session rewrite; ErrorEvent already emitted).
Both sub-paths of `runPragmaLoopWithInitialPrompt` now call it at the
top of each model-request iteration: the main shell-action loop
(:489) and the FinalTextOnly capture sub-loop (:412). The sub-loop's
existing per-iteration IncrementTurn is UNTOUCHED — the helper contains
no IncrementTurn by design (the CMP-001.1 F4 class: a trigger-side
increment next to the sub-path's own would double-count the compaction's
own iteration and halve MinTurnsCooldown), pinned by the cooldown gate.
Scope gate (MessageStartIndexes), deps gate (compactor + autoTracker),
and the F6 `AutoCompactEligible()` count bound are all preserved inside
the helper, so a scoped capture run (or a nil-deps fork — CMP-001.4a)
still skips the trigger exactly as before; nothing about the main
loop's observable behavior changes (the extraction is behavior-
identical for it, pinned by the existing gates below).

### Record correction (this section amends the record's own claims)

- The CMP-001.3 revision's sentence "Unscoped pragma-loop runs — ...
  and orchestration persistent-conversation states — carry the
  trigger" is now mechanically true: BOTH sub-paths of
  `runPragmaLoopWithInitialPrompt` consult the tracker when the run is
  unscoped and deps exist. Read with the CMP-001.3.F1 correction
  (forks carry nil deps), the production coverage statement becomes:
  unscoped runs on deps-bearing engines (plain pragma sessions, and any
  future fork that constructs its own deps per the CMP-001.4a re-enable
  path) carry the trigger, including persistent capture states.
- The CMP-001.3 revision's "The FinalTextOnly sub-loop (orchestration
  capture states) and its increment are untouched: that path never
  compacts and is not the default mode" is corrected: the sub-loop's
  increment is still untouched, but the sub-loop NOW carries the
  shared trigger. "Not the default mode" was never a sufficient reason
  for the unscoped capture-state shape — the rationale addressed the
  default-mode question, not the combination its own coverage
  sentence claimed.
- The CMP-001.4a boundary is unchanged by this revision: forks still
  never auto-compact until they construct their own deps, and a fork
  that does so now gets a WORKING trigger on its capture states too.

### Gates (this revision)

- RED → GREEN: the three gates above (RED messages quoted; all PASS
  after the one-mechanism change).
- Adjacent (the sub-loop's own behavior + the compaction family):
  `go test ./internal/query/ -run 'AutoCompact|MidCall|Fork|RequestShape|SystemPrompt|PreciseCounter|Reserve|FinalTextOnly|Scoped' -count=1`
  — ok, including `TestPragmaLoopFinalTextOnlyDoesNotExecuteBash`,
  `TestPragmaLoopFinalTextOnlyRetriesInvalidFinalText` (the pre-existing
  sub-loop gates: no bash execution, check-retry flow — unchanged by
  the trigger port), `TestPragmaLoopAutoCompactTriggersAndReplaces`,
  `TestPragmaLoopAutoCompactCooldownBlocksImmediateRetrigger`,
  `TestPragmaLoopScopedIterationsAdvanceSharedCooldown` (the .F2 gate —
  main-loop increment semantics unchanged by the extraction),
  `TestPersistentForkPragmaLoopNeverCompactsOrRewritesRootSession`
  (the .F1 gate — nil-deps forks still never compact).
- `go test ./internal/query/ -count=1` — ok (6.6s); `-count=1 -race` —
  ok (11.2s).
- `go test ./internal/compact/ ./internal/orchestration/
  ./internal/session/ ./internal/slash/ ./internal/cli/ -count=1` — ok.
- `go test ./... -count=1` — green except the pre-existing
  `TestProviderToolsCLIContract` acceptance/environment failure
  (cmd/pragma), re-verified failing identically at `ef1cbd6` with these
  changes stashed.
- `go vet ./internal/query/` clean; both changed/added files
  gofmt-clean.

### Claim boundary

- Proven locally, deterministically: an UNSCOPED FinalTextOnly run on a
  deps-bearing engine compacts before its next model request when over
  threshold, preserves the pending turn prompt verbatim
  (`compact.ApplyResult` semantics, shared with the main loop and
  manual /compact), rewrites the session file, completes its final-text
  flow normally, and its cooldown counts exactly one increment per
  model-request iteration. A SCOPED FinalTextOnly run never compacts
  (pinned). The main loop's observable compaction behavior is
  unchanged (behavior-identical extraction, pinned by the existing
  family).
- No production orchestration run changes behavior today: persistent
  states run on nil-deps forks (CMP-001.4a), and scoped capture states
  were already exempt — the new trigger is live exactly when the
  CMP-001.4a re-enable path (fork's own deps + own SessionRewrite) is
  taken, or for any future direct caller of the pragma loop with
  IncludePriorConversation + FinalTextOnly on a deps-bearing engine.
- The sub-loop's final-text acceptance/rejection loop, budget notices,
  and MaxTurns exhaustion are untouched; compaction only replaces the
  accumulated conversation between iterations, and a rejected draft's
  history lands in the summary like any other prefix.
- No live-provider behavior is claimed. The queue's related items
  (CMP-001.2.F3.F1/F2 sub-branches, CMP-001.4b C-2 small-window
  threshold) are unchanged.

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

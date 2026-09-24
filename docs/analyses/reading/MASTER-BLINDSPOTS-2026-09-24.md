# Master blindspot report — every one of the 2,725 calls individually read

Method: seven reading analysts (fresh model instances) rendered and read
every APIRequestCompleted call in all 52 session logs — every thinking
block, every tool invocation, call by call, no sampling. Per-session
reads live in this directory (50 files). This master report synthesizes
the cross-cutting findings with citations and fix directions.

## A. The supervisor's own blindspots (session 11-51, 482 calls)

1. **Built pragma-watch, never used it.** After building the observer
   (~call 100), the supervisor monitored the loop with 64 hand-rolled
   sleep+tail polls (~19M input tokens, ~17% of its 104M total input)
   — while its own tool sat unused except for build/kill checks.
   Fix: monitor through the watcher/meta-observer; never poll by hand.
2. **apply_patch `+`-drop failed 10 times** (calls 17, 27-28, 69, 78,
   128, 133, 206, 267, 318); root cause self-diagnosed twice, never
   fixed or queued. Fix: the patch-editor failure mode is itself a
   queue-worthy defect — workarounds do not remove causes.
3. **Confident claims without verification**: the "full history on every
   request" claim was false (microcompact stubs + loop-mode differences
   found only when the operator challenged); "482 my calls" included 37
   in-process subagent calls. Fix: verify-then-claim, the discipline the
   supervisor enforces on its workers.
4. **Broke its own turn-liveliness rule four times** after writing it
   into the standing skill. Fix: rules that live only in memory are
   decorative; the supervisor needs the same mechanical triggers it
   builds for workers.
5. **Tolerated known queue corruption for 7 hours** while ~$50 of worker
   cycles ran on untrusted queue state (clobber first seen 16:22,
   structural fix landed only at 23:13 as v1.7). Fix: harden the
   shared-memory layer before scaling the fleet that depends on it.

## B. Recurring worker/critic mechanical defects (cross-session)

1. **apply_patch friction is the dominant mechanical waste.** Empty
   patches, hunk-count mismatches, 6-try file adds, 15-call revision
   fights (18-12-25 calls 32-46), no-op patches (18-43-40 #31),
   whitespace churn (16-22-55 #43-44, 58). The supervisor hit it 10x;
   nearly every worker hit it. Fix: patch-format preflight in the tool
   (validate hunk headers before applying) or per-file fallback emission.
2. **Zero mistake-memory between attempts.** CLK-002 attempt-2 repeated
   attempt-1's exact same mistakes on the same test file (16-34-15),
   including the same empty-patch and count-confusion failures. Fix:
   failed attempts must leave a mistakes list that the retry prompt
   carries verbatim.
3. **Commit-as-afterthought cap deaths.** 16-22-55 completed its work at
   call 78 of 80 and died with nothing committed; 15-02-35 reached GREEN
   and died at cap uncommitted; 16-58-58 GREEN at #71, died in endgame
   patch friction. Fix: worker prompt rule — commit immediately at GREEN,
   before docs/case polish; the orchestrator should treat an uncommitted
   GREEN as an abnormal exit.
4. **Silent `&&`-chain / grep-exit-1 breakage** recurs in 5+ sessions:
   swallowed failures left worktrees unremoved (15-57-23), tests
   silently not run (14-21-40 #48-49), commands aborting mid-chain
   (13-18-05 #36-38). Fix: worker prompt rule — never chain grep/test
   with &&; check exit codes; echo markers between steps.
5. **git stash in the shared tree fails systemically** (13-18-05, 17-55-15,
   18-43-40, 21-37-15): flag traps, &&-chain-eats-pop, risky recoveries.
   Workers independently discovered the safe pattern (detached worktrees)
   but each had to rediscover it. Fix: worktree-per-worker policy in the
   orchestrator; ban stash in worker prompts.
6. **Context ballooning forced compaction, then facts were re-derived**
   (17-18-12: session-file dump → 176k context → 116s compaction → 11
   re-derivation calls; 16-04-00: same pattern). Fix: lean-read rules;
   the explore-cache (RES-005).
7. **Re-verification of already-proven facts**: 15-02-35 spent 66% of
   its calls (53/80) verifying; 16-34-15 re-verified ~20 calls of what
   attempt-1 had already documented as wire-proven; 16-53-28 spent six
   calls rediscovering what the commit message already stated; the
   killed worker 23-16-37 re-verified ~22 calls of archaeology its own
   critic sibling had already recorded. Fix: a trusted-prior-evidence
   tier in case records — wire-proven facts are not re-proven.
8. **Post-completion citation goose-chases**: 20-15-57 spent 16 calls
   (37% of session) gathering citations after its audit was done at
   finding C17; 18-52-29 ran a ~10-call low-value tail. Fix: stop
   condition = deliverable delivered, not turn budget exhausted.
9. **Provider stalls and dead time**: 347s stall (13-18-05 call 5, 29%
   of wall), two 15-min stalls in 19-30-10 (67% idle), three ~18-min
   unlogged stalls in 20-15-57 (~55 min dead). Fix: stall logging +
   the watcher's stall alert extended to worker sessions.
10. **Critic self-contamination**: 21-59-22's leaked mutant caused a
    false regression (6 calls, nearly a false report); 23-08-34 applied
    compound mutations without backup-first. Fix: mutation-isolation
    protocol (worktree + backup before mutate).
11. **Concurrent-landing collisions**: 16-46-48's staged files were
    swept by another agent's commit mid-landing (6-call guarded-amend
    surgery). Fix: the shared-tree single-writer assumption is already
    violated by design — worktree isolation is not optional at fleet
    scale.

## C. What worked (keep and generalize)

- **The probe-shaped sessions were flawless** (15-02-00, 16-30-26,
  16-44-32, 11-47-52): small, single-purpose, bounded — 1-2 calls, zero
  waste. The day's most efficient agents were the smallest ones.
  Generalize: prefer probe-sized queue items; big multi-part items burn
  66% verification and die at caps.
- **18-04-47 (critic) and 18-29-19 (worker)** are the clean full-size
  sessions: focused verification, immediate fixes, no goose-chases.
- **The empirical discipline works when followed**: 21-37-15's
  refuted-at-HEAD/RED-at-parent gate and 19-16-08's new-race probe are
  the methodology paying off exactly as designed.

## D. Reading-tier cost accounting

Seven reading instances: ~610k input tokens total, ~$5-7 (a fraction of
the script-tier's subject matter). The reading found what the script
could not: intent-level waste (goose-chases, re-verification, mistake
repetition, near-false-reports) worth an estimated 15-25% of worker spend
on top of the script tier's 30%-exploration / 11%-redo taxonomy.

## E. Priority fix list (queue-ready)

1. PACT-001: apply_patch preflight validation (dominant mechanical waste).
2. MM-001: mistake-memory between attempts (retry prompts carry the
   prior attempt's failure list verbatim).
3. CMT-001: commit-at-GREEN rule + orchestrator abnormal-exit detection.
4. SHL-001: `&&`-chain/exit-code discipline rule for worker prompts.
5. ISO-001: worktree-per-worker; ban shared-tree stash.
6. TPE-001: trusted-prior-evidence tier (wire-proven facts are not
   re-proven).
7. STP-001: post-deliverable stop condition.
8. SCL-001: supervisor monitors through its own tools, never by hand.

# Self-improvement doctrine (standing meta-method — do not lose this)

Written 2026-09-25 from the first full day of loop operation, per the
operator's directive: "if you forget these meta ways then you will lose
track of how to self improve again." Every session that works on the
loop reads this first. Evidence lives in docs/analyses/reading/ (every
one of 2026-09-24's 2,725 calls individually read) and
docs/analyses/api-payload-meta-analysis-2026-09-24.md.

## The loop (rinse and repeat)

Queue -> WORKER (verify, RED test, one-mechanism fix, GREEN, commit at
GREEN) -> CRITIC (fresh instance audits the commit) -> READER (a fresh
instance reads the worker's session call-by-call for intent-level waste)
-> findings re-queued -> next item. Cycles capped ($ and turns), findings
carry provenance, all commits local until push is authorized.

## The ten rules (each learned from a recorded failure)

1. **Probe-sized items.** The smallest sessions of 2026-09-24 were
   flawless (1-2 calls); the biggest died at caps with 66% verification
   overhead. Split until it fits one focused sitting.
2. **Commit at GREEN, immediately.** Three workers finished their fixes
   and died uncommitted at the turn cap. The deliverable is the commit,
   not the working tree.
3. **No `&&`-chains around grep/test/exit-sensitive commands.** Five+
   sessions lost work to silent chain breakage. Use echo markers and
   check exit codes.
4. **No git stash in the shared tree. Ever.** Use a detached worktree
   (the pattern workers kept rediscovering independently).
5. **Mistake-memory between attempts.** A retry that does not carry the
   prior attempt's failure list will repeat it verbatim (observed:
   attempt-2 repeating attempt-1's exact mistakes on the same file).
6. **Trusted prior evidence.** Facts recorded as wire-proven in case
   records are not re-proven. One worker spent 53/80 calls re-verifying.
7. **Stop at deliverable.** Goose-chases after the work is done cost up
   to 37% of a session. Delivered means done.
8. **Read every session, call by call.** Script aggregates found 30%
   exploration waste; only reading found the intent-level waste —
   re-verification, mistake repetition, near-false reports, dead time.
   The reader tier is mandatory, not optional polish.
9. **The supervisor uses its own instruments.** The watcher exists; hand-
   polling cost 17% of a day's largest session. Never poll what a tool
   already watches.
10. **Queue integrity is load-bearing.** The queue leaked items for 7
    hours before hardening; paid-for items were silently deleted. Shared
    state gets locks/append-journals BEFORE the fleet scales on it.

## What perfection means here (and does not)

Asymptotic, not absolute: zero observed self-inflicted losses; verified
score at the model's ceiling with uncertainty; every remaining failure
attributable with evidence. Benchmarks are the external anchor; the
operator holds spend, push, and stakes. Coverage beats omniscience —
mechanical counters, fresh-context critics, cross-model readers, and the
operator are differently-flawed observers; the union catches what any
single one misses.

## Priority fix list (from the master blindspot report)

PACT-001 apply_patch preflight validation; MM-001 mistake-memory in
retry prompts; CMT-001 commit-at-GREEN enforcement; SHL-001 shell-chain
discipline; ISO-001 worktree-per-worker; TPE-001 trusted-prior-evidence
tier; STP-001 stop-at-deliverable; SCL-001 supervisor monitors through
its tools; RES-001..007 from the payload analyses (explore-cache,
redo-guard, sub-100-output, prompt caching, supervisor economics);
BENCH-mapper-redesign (unblocks the first honest benchmark run).

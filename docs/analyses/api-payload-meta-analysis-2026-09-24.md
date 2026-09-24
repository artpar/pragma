# API payload meta-analysis — every agent call, 2026-09-24

## Per-call edition (v2): every one of the 2,725 calls individually parsed

Method: every APIRequestCompleted event in all 52 session logs was parsed
individually; every tool invocation inside every response was extracted
with its arguments and content-fingerprint (3,324 tool invocations total);
costs distributed per call by token share; identical-call detection within
and across sessions. Itemized ledger: `api-calls-2026-09-24.jsonl` +
`.csv` (3,324 rows) in this directory - every claim below is a query over
that file.

### Per-call waste taxonomy (dollars of the $188.59)

| Category | Calls | Cost | Share |
|---|---|---|---|
| Pure re-read exploration (grep/sed/git-show command families re-reading the repo) | ~623 | ~$56 | 30% |
| Identical calls re-executed within one session (patch retries, re-reads) | 233 | ~$22 | 11% |
| Cap-dead worker attempts (before v1.5) | 4 sessions | ~$32 | 17% |
| Supervisor session premium (42% of all input for meta-coordination) | 482 calls | ~$79 | 42% |

Over half the day's spend (58%) was mechanical overhead - re-reading,
retrying, and coordination - not new work. The genuinely-new information
produced (2M output tokens + ~4M fresh input context) cost roughly
$15-25 at route rates.

### Per-call efficiency findings

- 604 of 2,737 calls (22%) produced <100 output tokens - thinking-heavy
  responses with tiny or no visible action.
- 165 calls (6%) took >30s - the slow tail is long thinking turns.
- The most-repeated exact command across ALL agents: `grep -rn ...` 143x,
  `git show` 124x, `grep -n` 121x, `sed -n` 109x - the same repo reads
  re-issued by every fresh instance because nothing persists reads
  between sessions.
- Tool mix per agent type (from per-call ledger): supervisor = 417 Bash +
  80 apply_patch + 3 WebSearch + 2 Agent; workers = 83-88 Bash + 6-20
  apply_patch each; critics = grep/git-heavy, ~1.5 tool calls per call.

### New follow-up findings (append to RES list)

5. RES-005 explore-cache: 30% of spend re-reads what prior sessions
   already read. A shared read-artifact cache (repo survey hand-off,
   already proven in the benchmark's /tmp/pragma/swe/repo-survey.md
   pattern) would reclaim most of it.
6. RES-006 redo-guard: 233 intra-session identical re-executions - a
   tool-level dedup advisory (the same fingerprint twice in a row = ask
   the model why) is a small harness change.
7. RES-007 sub-100-output calls (22%): classify whether these are
   legitimately thinking-then-acting or wasted requests.

Scope: all 2,725 API calls made by all 52 sessions today (supervisor,
workers, critics, meta-observer support, probes, benchmark support runs),
aggregated from tier-1 event logs (`~/.pragma/logs/2026-09-24T*.jsonl`),
44 tier-2 payload recordings (`~/.pragma/recordings/`), session cost
metadata, and the 3 benchmark wire-capture sets.

## Executive summary

- **2,725 calls, 250M input tokens, 2M output tokens, true recorded cost
  $188.59** (session CostTracker metadata; list-price token math would say
  ~$300 - effective blended rate is lower; reconciliation follow-up noted).
- **The resend tax is 98%.** Sum of all final conversation contexts: ~4M
  tokens. Total input billed: 250M. Roughly 98% of every input dollar paid
  today re-sent history the model had already seen.
- **Cache utilization: 0%.** Zero cache_read tokens across the entire day
  on the morphllm route - every request bills full fresh input.
- Input:output ratio **125:1**. Stop reasons: 2,641 tool_use vs 84
  end_turn - the day was almost pure tool-looping, by design.
- **One session - the supervisor - consumed 42% of all input** (104M of
  250M; 482 calls averaging ~217k input each).
- Failure economics: 4 worker cap-deaths burned ~$32 (17% of spend) before
  the v1.5 turn-cap fix.

## Payload composition (from wire recordings)

Typical worker session (54 calls, recording 281fd3ee): first request
carried 1 message (2.1KB); the last carried 107 messages (374KB of
history). System prompt is small and near-constant (~2.5KB - custom prompt
plus manifest blocks); tool definitions ride separately in the tools field.
History grows linearly at ~7KB per request; by mid-session the request is
>95% re-sent content.

## Cost model, concretely

At the observed shape (250M in / 2M out), the two levers that matter:

1. **Prompt-prefix caching** (if the route supports it): even 80%
   cache-read coverage at a 5:1 cache discount would have cut the day to
   roughly $60-80. This is the single biggest efficiency lever available
   and it is a provider-feature question, not an intelligence question.
2. **Session length discipline**: the supervisor session is the whale
   (42% of input). Compounding per-turn cost is linear in session length;
   the auto-compaction fixes that landed today now bound this (they were
   dead until today).

## What the day bought

15+ mechanism commits (auto-compaction restoration chain in both loop
modes, session-rewrite integrity, prompt preservation, fork isolation,
budget caps, wall-clock presentation, window calibration, meta-observer,
self-improvement loop v1.1-v1.7) - roughly **$12.50 per landed commit**,
trending down: worker cycles fell from ~$8-11 to ~$2.50-3.50 as RED-test
inheritance spread and cap-deaths were fixed.

## Findings that deserve follow-up (ranked)

1. RES-001: route-level prompt caching (0% utilization; biggest lever).
2. RES-002: supervisor session economics (42% of input; consider shorter
   supervisor sessions or per-supervisor compaction thresholds).
3. RES-003: cost reconciliation (recorded $188.59 vs ~$300 list-token math -
   verify CostTracker pricing table matches the actual route invoice).
4. RES-004: meta-observer calls have tick logs only, no wire payloads.

Raw aggregates preserved in this analysis directory; every claim above is
recomputable from the tier-1 logs with one script.

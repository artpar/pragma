# API payload meta-analysis — every agent call, 2026-09-24

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

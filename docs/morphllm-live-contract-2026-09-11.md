# MorphLLM live model contract check — 2026-09-11

## Precommitment

- Question: which models returned by authenticated `GET /v1/models` accept a
  minimal OpenAI Chat Completions request, and what output shape do they return?
- Treatment: one `POST /v1/chat/completions` request per catalog model with the
  same user prompt (`Reply exactly MODEL_OK.`), temperature 0, and `max_tokens`
  32.
- Checkpoint: HTTP status, latency, returned model, finish reason, text/tool-call
  presence, usage, and top-level error. Secrets and full reasoning text are not
  retained.
- Initial limits: at most 22 completion calls (the inventory count observed
  before execution), 10 minutes total, 32 requested output
  tokens per call, and $0.25 estimated aggregate cost.
- Acceptance: HTTP 200 with at least one choice and nonempty content or tool
  calls. A specialized model that documents a different prompt/protocol may be
  classified `specialized_protocol` after the generic probe rather than as a
  broken chat model.
- Stop conditions: authentication failure, unexpected projected cost above the
  ceiling, or repeated service-wide failures.
- Environment: Pragma revision based on `aee718b`; Morph endpoint
  `https://api.morphllm.com/v1`; API key loaded from the local Pragma credential
  store and never printed.

## Results

The execution-time catalog contained 23 entries rather than the 22 seen during
the initial inventory; the per-catalog-entry rule was followed, producing one
additional call with no change to the token or cost ceiling.

### Bounded availability retry addendum

Six consecutive general-model calls returned HTTP 429 `service_overloaded`.
Make one retry per affected model with the identical request. This raises the
maximum call count to 29; failed 429 requests have no model-token charge and the
$0.25 ceiling remains unchanged. Do not retry successful semantic responses or
repeat after the second 429.

### Reasoning-cap correction addendum

The 32-token probe was too small for several always-on reasoning models: they
returned a populated `reasoning_content`, `finish_reason: length`, and no visible
answer. Make one 128-token request for each affected model that has not already
produced a visible answer in the Pragma candidate check. This is a cap
correction, not a repeated stochastic trial. Maximum total calls become 35 and
the $0.25 ceiling remains unchanged.

## Observed results

| Requested model | Result |
| --- | --- |
| `morph-v3-fast` | 200, visible `MODEL_OK.` |
| `morph-v3-large` | 200, visible `MODEL_OK` |
| `auto` | 200, routed to `morph-v3-fast`, visible output |
| `morph-compactor` | 200, returned the input text (specialized behavior) |
| `morph-warp-grep-v2.1` | 200, returned a native tool call (specialized behavior) |
| `morph-glm53flash` | 200; reasoning-only at 32 tokens, visible `MODEL_OK` at 128 |
| `morph-dsv41flash` | 200, reasoning plus visible `MODEL_OK` |
| `morph-glm53-744b` | 200; reasoning-only at 32 tokens, visible `MORPH_PRAGMA_OK` at 128 through Pragma |
| `morph-kimik3` | 200, reasoning plus visible `MODEL_OK` |
| `morph-kimik3-fast` | 200; reasoning-only at 32 tokens, visible `MODEL_OK` at 128 |
| `morph-dsv4flash` | 200, visible `MODEL_OK` |
| `morph-glm52-744b` | 200 but routed to `morph-glm53-744b`; reasoning-only at both 32 and 128 |
| `morph-dsv4flash-0731` | 200 but routed to `morph-dsv4flash`, visible output |
| `deepseek/deepseek-v4-flash-0731` | 200 but routed to `morph-dsv4flash`, visible output |
| `deepseek/deepseek-v4-flash` | 200 but routed to `morph-dsv4flash`, visible output |
| `deepseek/deepseek-v4-flash-20260423` | 200 but routed to `morph-dsv4flash`, visible output |
| `morph-qwen35-397b` | First 429; retry returned 200 but routed to `morph-glm53flash` and reasoning-only; cap correction returned 429 |
| `morph-qwen36-27b` | First 429; retry returned 200 but routed to `morph-glm53flash` and reasoning-only; cap correction returned 429 |
| `morph-qwen38-27b` | First 429; retry returned 200 but routed to `morph-glm53flash` and reasoning-only; cap correction returned 429 |
| `morph-minimax27-230b` | 429 `service_overloaded` on both allowed attempts |
| `morph-minimax3-428b` | 429 `service_overloaded` on both allowed attempts |
| `morph-gemma4-31b` | 429 `service_overloaded` on both allowed attempts |
| `morph-computer-use-v1` | 422 on generic chat payload; requires its specialized `task`/`diff` browser protocol |

## Pragma production-path checks

- The initial non-streaming request failed with HTTP 400 because the shared
  adapter sent `stream_options` without `stream: true`. Capture:
  `.pragma/verification/20260911/morph-reasoning-baseline/raw-http/`.
- After removing that field, an authentic 32-token response was accepted but
  Pragma discarded its only output, `reasoning_content`. The sanitized fixture
  and provenance are in
  `internal/provider/morphllm/testdata/recorded-reasoning-response.json` and its
  adjacent README.
- The changed adapter returned visible `MORPH_PRAGMA_OK` at 128 tokens.
- A two-turn tool check returned a native Bash call, replayed its reasoning and
  tool call in the next request, executed it, and received visible
  `MORPH_TOOL_OK`. Capture:
  `.pragma/verification/20260911/morph-tool-candidate/raw-http/`.

## Claim boundary and cost

All 23 catalog IDs were contacted. This establishes successful output only for
the rows above with HTTP 200; persistent 429 rows remain availability failures,
and Computer Use is not an ordinary chat model. Requested IDs that returned a
different model are routing aliases, not verified executions of the requested
weights. The exact aggregate direct-probe cost is unavailable because Pragma
does not register pricing for every catalog model, but total generated tokens
were far below the $0.25 ceiling. The three metered Pragma success calls cost
approximately $0.0032 combined.

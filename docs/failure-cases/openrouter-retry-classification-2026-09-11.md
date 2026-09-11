# RTY-001 — Substring status classification retries permanent 4xx errors

- Case ID: `RTY-001`
- Source revision: `8bf3061` (2026-09-11)
- Source observation: one parallel full-suite `go test ./... -count=1` run on
  2026-09-11 failed `internal/provider/openrouter`
  `TestRecordedReasoningSurvivesSessionReplay` with `requests = 12, want 2`
  after 183s. The same test passes in isolation on the baseline, in isolation
  on the candidate tree, and in a subsequent clean full-suite run (3.6s).
- Observed behavior: the test's boundary server returns a deliberate
  non-retryable 400 ("recorded replay ends at outgoing request"). Under some
  runs the provider retried that 400 until the entire budget of 10 attempts
  was exhausted: 12 server-observed requests and ~183s elapsed, consistent
  with the `WithRetry` backoff schedule `500ms·2^attempt` capped at 32s with
  ≤25% jitter (0.5+1+2+4+8+16+32+32+32+32 = 159.5s, +jitter ≈ 183s).
- Expected behavior: a 4xx response without a retryable marker is classified
  non-retryable. The request count in the boundary test must be exactly 2
  regardless of incidental digits appearing anywhere in the error message.
- Contract source: HTTP semantics (4xx = client error; retrying an unchanged
  request cannot succeed); `shared.ClassifyByStatusCodes` doc comment intent
  ("checks the error message for HTTP status code strings"); the test's
  boundary-marker comment ("400 is nonretryable").
- Mechanism: `openrouter.ClassifyByStatusCodes(["429","500","502","503","504"])`
  substring-matches the full error text. The openai-go SDK error renders as
  `POST "<request URL>": <status> <statusText> <body>` and therefore embeds the
  request URL — including the ephemeral `httptest` port — and the response body
  in the classified message. When the port (e.g. `127.0.0.1:25003` contains
  `500`) or a body field (e.g. `"max_tokens must be <= 500"`) contains a
  code-like digit substring, a permanent 400 is misclassified retryable and
  burns the full retry budget in backoff.
- Reproduction (deterministic, production code): 
  `go test ./internal/provider/openrouter -run 'TestClassify.*Digits|TestClassifyStructured' -count=1`
  — a structured `*openai.Error` with `StatusCode: 400` whose request URL port
  contains `500`, and a real SDK round trip returning 400 with a body
  mentioning `500`, both classify as retryable on the baseline.
- Executable assertion: the same command exits 0 after the fix, plus
  `go test ./internal/provider/openrouter -count=1` and
  `go test ./internal/provider/openrouter -run TestRecordedReasoningSurvivesSessionReplay -count=10`
  all pass.
- Proposed mechanism fix: classify via the structured `StatusCode` extracted
  with `errors.As(*openai.Error)` before falling back to substring
  classification for transport-level errors that carry no structured status.
- Refuting evidence: if structured classification changes the outcome for any
  error whose message contains a genuine status marker but no structured
  status (plain-string errors in existing tests), the fix overreached.
- Claim boundary: local classification behavior for the openrouter adapter.
  The same substring classifier is used by `morphllm` (the active benchmark
  route), `openai`, `google`, and `groq` — same latent defect class, recorded
  here as follow-up cases, not fixed in this change.

## Verification record

- Baseline (revision `8bf3061`, 2026-09-11): both deterministic reproductions
  failed through the production classifier:
  - `TestClassifyPortDigitsDoNotTriggerRetry`: structured 400 with port
    `25003` → `Retryable:true, ErrorType:"server_error"` (misclassified via
    the `500` substring inside the port).
  - `TestClassifyBodyDigitsDoNotTriggerRetry`: real SDK round trip returning
    400 with body `max_tokens must be <= 500` → `Retryable:true,
    ErrorType:"server_error"`. The wrapped error confirmed as
    `*apierror.Error`, verifying the `errors.As` extraction path.
- Candidate (structured `StatusCode` classification via `errors.As` on
  `*openai.Error`, substring fallback retained for transport-level errors):
  - All three new tests pass, including the positive controls
    (`TestClassifyStructuredStatusCodesAreHonored`: 429→rate_limit,
    500/502/503/504→server_error, 400/401/402→request_failed, all retryable
    flags exact).
  - `go test ./internal/provider/openrouter -count=1` passes — existing
    classification contracts unchanged (in-flight-budget 402 with
    Retry-After 120s → retryable rate_limit; permanent 402 → non-retryable;
    plain-string 429 → retryable rate_limit via fallback).
  - `go test ./internal/provider/openrouter -run TestRecordedReasoningSurvivesSessionReplay -count=10`
    passes — ten boundary replays across ten random ephemeral ports, the
    exact condition under which the flake fired.
  - `go test ./... -count=1` exits 0, 27 packages ok.
- Claim boundary held: openrouter adapter classification only. The latent
  substring-misfire class remains in `morphllm` (active route), `openai`,
  `google`, and `groq` classifiers — recorded as follow-up cases.

## Follow-up cases (same mechanism, other adapters)

- RTY-002 (`morphllm`): same substring helper with the same code list; the
  active benchmark route. A 4xx whose URL/body carries code-like digits
  would retry for ~3 minutes; conversely the captured 2026-09-11T07:53:58Z
  session-start failure (`request_failed, retryable=false`) shows the other
  edge of the same classifier and is classified under M2b.
- RTY-003 (`openai`), RTY-004 (`google`), RTY-005 (`groq`): same helper,
  different SDK error shapes; each needs its own structured-status
  extraction and its own deterministic case before porting the fix.

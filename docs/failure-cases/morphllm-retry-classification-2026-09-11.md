# RTY-002 — MorphLLM (active route) classifies by substring over the full error message

- Case ID: `RTY-002`
- Source revision: `11f7120` (2026-09-11)
- Source observation: the authentic captured session-start failure
  2026-09-11T07:53:58Z (`~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa/`)
  classified a MorphLLM 400 correctly (`retryable=false, request_failed`):
  `[openai-compatible] invalid_request: POST "https://api.morphllm.com/v1/chat/completions": 400 Bad Request {"message":"Validation: The 'stream_options' field is only allowed when 'stream' is set to true.","type":"invalid_request_error"}`
  — but only because this particular message happens to contain no
  code-like digit substring. The verdict was correct by luck of the message
  content, not by classification structure.
- Observed behavior (latent, same mechanism as RTY-001):
  `morphClassify = shared.ClassifyByStatusCodes(["429","500","502","503","504"])`
  substring-matches the entire error message. The message embeds the request
  URL (with ephemeral ports in tests, fixed paths in production) and the
  response body. A 4xx whose body or URL contains "500"-like digits — e.g.
  a validation error `limit must be <= 500` — misclassifies as retryable and
  burns the ten-attempt budget in ~3 minutes of backoff on the active
  benchmark route.
- Error-shape note (code reading): two shapes reach `morphClassify`.
  1. The current `completeWire` path returns the raw openai-go
     `*apierror.Error` (`ConvertError` passes it through unchanged — the
     openai-compatible adapter does not implement `providers.ErrorConverter`).
  2. The any-llm adapter path wraps into typed errors (the authentic capture
     shows `*llmerrors.InvalidRequestError` with the `[openai-compatible]`
     prefix); `BaseError.Unwrap()` still exposes the underlying SDK error,
     so a single `errors.As` pierces both shapes.
- Expected behavior: classification reads structured error data — the
  any-llm `*llmerrors.RateLimitError` (semantic class + structured
  `RetryAfter`) and the openai-go `*oaisdk.Error.StatusCode` — and falls
  back to substring matching only for transport-level errors that carry
  neither.
- Contract source: HTTP semantics (4xx = client error, permanent for an
  unchanged request); parity with RTY-001's structured classification.
- Reproduction (deterministic, production classifier):
  `go test ./internal/provider/morphllm -run 'TestClassify.*Digits|TestClassifyStructured' -count=1`
  — both the raw-SDK shape and the any-llm wrapped shape with a port
  containing "500" classify as retryable on the baseline.
- Executable assertion: the same command exits 0 after the fix, plus
  `go test ./internal/provider/morphllm -count=1` and the full suite.
- Proposed mechanism fix: structured classification in `morphClassify` —
  `*llmerrors.RateLimitError` first (retryable, structured RetryAfter),
  then `*oaisdk.Error` status switch (429 → rate_limit; 500/502/503/504 →
  server_error; anything else → non-retryable request_failed), substring
  fallback last.
- Refuting evidence: if the structured path changes the outcome for any
  error that the substring policy classifies from a genuine status marker
  without structured data (plain-string errors), the fix overreached.
- Claim boundary: local classification behavior of the morphllm adapter.
  No live-provider claim. RTY-003/004/005 (openai, google, groq) remain
  separate follow-up cases; google and groq have different SDK error shapes
  and need their own structured extraction.

## Verification record

- Baseline (revision `11f7120`, 2026-09-11): deterministic reproduction
  through the production classifier:
  - `TestClassifyPortDigitsDoNotTriggerRetry` (raw SDK shape): FAIL —
    400 with port `25003` classified `Retryable:true`.
  - `TestClassifyWrappedPortDigitsDoNotTriggerRetry` (any-llm wrapped
    shape, mirroring the authentic capture): FAIL — 400 with port `35001`
    classified `Retryable:true`.
  - `TestClassifyStructuredRateLimitKeepsRetryAfter`: FAIL on the
    RetryAfter assertion — the typed `RateLimitError.RetryAfter=120` is
    dropped (the fallback substring path sees the status but cannot carry
    the structured Retry-After).
- Candidate (`morphClassify` reads `*llmerrors.RateLimitError` first, then
  `*oaisdk.Error.StatusCode`, substring fallback last):
  - All five classification tests pass, including the positive controls
    (`TestClassifyStructuredStatusCodesAreHonored`: 429→rate_limit,
    500/502/503/504→server_error, 400/401/402→request_failed) and the
    fallback preservation (`TestClassifyPlainStringFallbackPreserved`).
  - Structured `RetryAfter: 120s` carried through from the typed error.
  - `go test ./internal/provider/morphllm -count=1` passes.
  - `go test ./... -count=1` exits 0, 27 packages ok; `make build` clean.
- Claim boundary held: morphllm adapter classification only. RTY-003
  (openai), RTY-004 (google), RTY-005 (groq) remain latent-mechanism cases
  with no observed authentic failure on their routes yet; per the
  evidence-first policy they are recorded, not preemptively fixed.

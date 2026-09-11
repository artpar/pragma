# Recorded Morph reasoning fixture

`recorded-reasoning-response.json` is a sanitized copy of the real Morph
response captured on 2026-09-11 under
`.pragma/verification/20260911/morph-reasoning-accepted-baseline/raw-http/`.
The source response SHA-256 is
`d9f6c5fe0eb4722d77e558a815ea53abce4654e16630abbd384bd2096a19ca95`.

Sanitization replaced the request ID and creation timestamp and removed null
metadata fields. Model, finish reason, reasoning content, and usage are
unchanged. The unchanged shared OpenAI-compatible adapter returned no content
for this response because it discarded `reasoning_content`.

Regression command:

```bash
go test ./internal/provider/morphllm -run TestRecordedReasoningContentIsPreserved -count=1
```

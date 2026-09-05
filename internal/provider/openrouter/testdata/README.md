# OR-001: recorded reasoning loss

Source: Pragma revision 1315159, OpenRouter z-ai/glm-5.3, real cancellation
task baseline on 2026-09-05. Trial `cancel-async-tasks__MTVFvUQ`, session
`18b161b2-b00c-4fd9-bdf6-af6f55ef2d86`.

Source capture under `.pragma/verification/20260905/jobs/openrouter-low-baseline/`:
`cancel-async-tasks__MTVFvUQ/agent/raw-http/000001-9ac46a4a39b2-148b129b0754/response.raw`.
Original SHA256: `5a809771ef2fe3342c9e7133dd467d64ff1abc3bc8dcb24a04155794404673b9`.

The response fixture preserves every JSON field/value, with only whitespace
reformatted (including removal of transport keep-alive whitespace). No headers,
credentials, hidden tests, or oracle output are included. The context fixture
projects the original user message/model from that capture's request and the
actual tool result from request `000002-e65b884f70f3-d9e8b805c33f`. System/tool
definitions are not needed to reproduce conversion and are omitted. No tool is
executed; its recorded output is input to this serialization regression.

Fixture SHA256:

- response: `a8e6d5e272bd52d61e1c16e14952a04f6e298468256f3a52d7a45d21df47ae26`
- context: `53fc9b78ae9cf9e19ba6bff39293006fb74e1fccbc407f9a957d5ee977dd7e4d`

Run from repository root:

```sh
go test ./internal/provider/openrouter -run TestRecordedReasoningSurvivesSessionReplay -count=1
```

The local HTTP endpoint serves the authentic recorded response once. The actual
provider parses it, writes messages through the session JSONL writer, loads them
with Store.LoadFile, deep-copies the conversation, and serializes the next request.
The endpoint then returns a nonretryable 400 boundary marker: no downstream model
response is invented or reused after the changed request.

Assertions compare reasoning strings exactly and reasoning-details JSON by values
and array order, retaining unknown fields. Tool argument JSON whitespace is not
significant; argument values, tool IDs/names/results, and message placement are.
The proposed mechanism is loss in response/request conversion. If the unchanged
code preserves these fields on the same case, this regression does not reproduce
that mechanism and must not justify the fix.

## Observed before/after, 2026-09-06

Unchanged production source with this regression installed failed in 0.427s:

```text
recorded reasoning lost after response -> JSONL -> reload -> next request
recorded reasoning_details lost after response -> JSONL -> reload -> next request
FAIL
```

After the nonstreaming fix, the identical regression passes. Supporting tests
cover encrypted details, normal responses, request equivalence outside reasoning,
copy isolation, credential fallback, and invalid input. They are synthetic
coverage, distinct from this authentic recorded case.

Earlier live baseline/candidate traffic also showed 4/4 fields lost versus 0/4
lost across two tool turns per arm. See docs/harness-verification-plan-2026-09-05.md.
No new live inference is needed for this regression. This proves the nonstreaming
field-preservation defect is repaired; streaming is unchanged. Neither this replay
nor the earlier isolated task win establishes causal benchmark improvement.

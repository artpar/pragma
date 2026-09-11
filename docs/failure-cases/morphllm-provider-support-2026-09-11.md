# MORPH-001 — MorphLLM cannot be selected as a provider

- Case ID: `MORPH-001`
- Source revision: working tree based on `aee718b` (2026-09-11)
- Source request: user requested MorphLLM with GLM-5.3 on 2026-09-11.
- Observed behavior: `CreateProvider` rejects provider `morphllm` as unknown, and
  provider resolution has no Morph API-key, base-URL, model, or compaction
  metadata.
- Expected behavior: `--provider morphllm` resolves `MORPH_API_KEY`, defaults to
  `https://api.morphllm.com/v1` and `morph-glm53-744b`, constructs a provider
  whose identity is `morphllm`, and preserves the active model for compaction.
- Contract source: explicit user requirement plus Morph's published
  OpenAI-compatible API documentation for model `morph-glm53-744b`.
- Executable assertion: `go test ./internal/cli -run TestMorphLLMProviderConfiguration -count=1`.
- Proposed mechanism: add a first-class Morph adapter over the existing
  OpenAI-compatible transport and wire it through CLI/provider resolution.
- Refuting evidence: the focused test still fails, or an authenticated Morph
  tool-turn capture shows that Morph is not compatible with the existing wire
  translation.
- Claim boundary: local tests can establish configuration and translation
  behavior. An authenticated live request is required to establish that Morph
  accepts the resulting request and returns compatible reasoning/tool fields.

## Verification record

- Baseline: the executable assertion failed because `DefaultModelFor("morphllm")`
  returned the Anthropic fallback model.
- Candidate: the same assertion passes after the first-class provider wiring.
- Local transport: `go test ./internal/provider/morphllm -count=1` verifies the
  documented `max_tokens` request field and native tool-call decoding through
  the production adapter.
- Adjacent regressions: `go test ./... -count=1` passes, including the existing
  recorded OpenRouter reasoning replay.
- Python runner: `python3 -m unittest tools/test_run_swebench_pro_instance.py`
  passes. The Harbor module syntax-checks and its Morph credential mapping was
  exercised with dependency stubs; its normal unit module cannot load locally
  because the external `harbor` package is absent.
- Live provider contract: a first production-path request exposed Morph's
  rejection of non-streaming `stream_options`; the next authentic response
  exposed `reasoning_content` loss. The candidate returned visible text and
  completed a native two-turn Bash tool interaction while replaying reasoning.
- Runtime/cost: focused local baseline about 4 seconds and candidate full Go
  suite about 20 seconds. The metered successful Pragma contract checks cost
  approximately $0.0032; broader catalog probe cost remained below $0.25.

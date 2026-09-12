# Cross-provider model catalog (/models) — 2026-09-12

Operator request: `/models` showed a single model (the morphllm static
fallback `morph-glm53-744b`); the picker should list all models from all
providers with available keys, pinged live from each provider's models API,
as qualified `provider/model` entries.

## Mechanism

- New `internal/cli/modelcatalog.go`: `ModelCatalog` aggregates, per
  provider, the live `/models` listing (10-minute TTL, background refresh,
  single-flight) and falls back to the compiled-in static registries when
  a fetch fails. Providers without credentials are excluded.
- Live fetchers (token-free, one request per provider, 5s timeout):
  - `anthropic.FetchModels` — Anthropic Models API via anthropic-sdk-go
    (auto-paging).
  - `google.FetchModels` — Gemini Models API via genai `Models.All`,
    keeping only models whose supported actions include `generateContent`
    (the endpoint also lists embeddings, image, and music models).
  - `openai.FetchModels` — OpenAI-compatible `GET {baseURL}/models` via
    openai-go; shared by openai, groq, morphllm, openrouter.
  - google-vertex is never fetched remotely (ADC-based auth, no key
    models endpoint); when active it shows the Gemini static registry as
    an approximation.
- `/models` (alias of `/model`) now opens the picker over qualified
  `provider/model` IDs, active provider first. Non-chat noise from
  account-wide listings is filtered (OpenAI keeps gpt/o1/o3/o4/o5/chatgpt
  families; Groq drops whisper/guard; curated providers pass through).
- `/model provider/model` switches providers in-session by reusing the
  resume rebinding path (`applyResumeProvider`: engine, tools, compaction,
  token budget) and records the provider on the conversation so resume and
  the `(current)` marker agree. A prefix is only treated as a provider when
  it names a known provider — OpenRouter's bare `z-ai/glm-5.3` stays a
  model ID. Non-interactive `/model` (no switcher) strips same-provider
  qualification and rejects cross-provider targets with restart guidance.
- The picker gained type-to-filter (substring, case-insensitive),
  Backspace edit, Esc clears filter before dismissing — required because
  OpenRouter lists 445 models.
- Static registries remain the offline fallback; e.g. the static Gemini
  registry tops out at 3.1 while the live listing already serves
  gemini-3.5-flash through gemini-3.8-flash.

## Evidence

Local unit tests (httptest servers, injected fetchers):

```
go test ./internal/provider/anthropic -run TestFetchModels -count=1
go test ./internal/provider/google   -run TestFetchModels -count=1
go test ./internal/provider/openai   -run TestFetchModels -count=1
go test ./internal/cli -run "TestQualifiedModelIDs|TestRefresh|TestVertex|TestKnows|TestMaybeRefresh|TestParseModelTarget" -count=1
go test ./internal/cli -run TestIsKnown -count=1
go test ./internal/slash -count=1
go test ./internal/tui -run TestModelDialog -count=1
```

Live provider contract (token-free model-list calls, 2026-09-12, this
machine's stored credentials; keys not recorded): `ModelCatalog.Refresh`
through the production fetchers returned

- morphllm: 23 models (auto, deepseek/deepseek-v4-flash*, morph-glm52/53,
  morph-glm53flash, morph-computer-use-v1, morph-kimik3*, morph-minimax*,
  morph-qwen3*, morph-warp-grep-v2.1, …)
- google: 40 generateContent-capable models (gemini-2.5 through
  gemini-3.8-flash, flash-latest, omni, gemma-4, …)
- openrouter: 445 models
- no fetch errors

Full suite `go test ./internal/... -count=1` passes; AST instrumentation
(`make instrument`) re-ran clean.

## Claim boundaries

- Live evidence covers model *listing* only, not switching a conversation
  onto a remote-listed model (that needs a bounded live continuation or
  operator use).
- No persistent cross-provider switching via headless `/model`; runtime
  switching is interactive-only and does not close the previous provider's
  resources (matching the resume path).
- google-vertex listing is a static approximation; vertex-dated model IDs
  may differ.
- `nano-banana`/`lyria`-style models pass Google's generateContent filter
  and appear in the picker; the API offers no stricter chat-capability
  signal.

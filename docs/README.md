# Documentation guide

## Current guidance

- [Harness development methodology](../agent.md): governing development and
  verification workflow. Start here, not with an old goal prompt.
- [Repository README](../README.md): usage and runner entry points; check current
  source and effective configuration before executing historical commands.
- [Codebase architecture](codebase-architecture.md): architecture reference;
  confirm against current source when changing a component.

Active benchmark/provider defaults use MorphLLM with `morph-glm53-744b` as of
2026-09-11. OpenRouter remains supported and its recorded regression remains
active coverage. The Lilac adapter and runtime selection were removed on
2026-09-06. Historical Lilac reports, traces, and evaluation configurations
remain evidence and are not runnable current configs.

## Evidence, not active plans

- [September investigation](benchmark-investigation-2026-09-05.md): historical
  failure classification and corrected hypotheses.
- [September verification record](harness-verification-plan-2026-09-05.md):
  actual reproduction, provider traffic, candidate limits, and Terminus comparison.
  Earlier experiment sequencing is superseded by the replay-first methodology.
- Dated reports, ledgers, task results, retrospectives, and prompt experiments
  retain useful positive and negative evidence. Their commands, model defaults,
  budgets, and outstanding goals are not current instructions.
- Existing `*goal-prompt.md` files and the transition-scoped handoff plan are
  inactive historical task specifications. Do not restart them without a current
  request and a newly verified defect under agent.md.

## Current evidence boundary

As of 2026-09-06, the OpenRouter nonstreaming fix is applied in this working tree.
An authentic [recorded regression](../internal/provider/openrouter/testdata/README.md)
failed on unchanged code and passes after the fix through session save/reload and
next-request serialization. Streaming is unchanged; no score-uplift claim is made.

Real OpenRouter traffic established reasoning-field loss and preservation by an
isolated candidate. The candidate's passing cancellation implementation was chosen
before replay could affect its decision; its task win is not causal uplift evidence.
Stock Pragma and the corrected same-model Terminus reference both scored 5/6 on
the cancellation task. No general harness advantage was established.

Unchanged task tests reproduced baseline 5/6 and candidate 6/6 in approximately
13–14 seconds using the [cached diagnostic verifier](../.pragma/verification/20260905/cached-verifier/README.md).
That local evidence directory may be ignored by Git; preserve it explicitly when
transferring the investigation. It is not a replacement for an official score.

## Cleanup record — 2026-09-06

Superseded goal prompts are marked inactive in place, preserving links and unique
evidence. The September plan's fresh-run-for-every-change acceptance rule is
replaced by replay-first gates. No product files, traces, or results were deleted.

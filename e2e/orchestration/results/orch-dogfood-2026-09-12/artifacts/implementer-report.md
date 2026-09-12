Adversarial review of the TUI-001 mechanism (range 413122a^..948431c). Review-only task: no repository file was edited, created, or deleted. All scratch artifacts live under /tmp/pragma/.

Overall verdict: APPROVE with one recorded deviation (check 3, partial promotion) and one minor observation. All four gated tests pass; no over-promotion was found in any adversarial path; one partial-loss-of-promotion case exists and is documented below with a suggested gate.

Commit-range sanity: git diff 413122a 948431c -- internal/tui/model.go internal/tui/handlers.go internal/tui/thinking_turn_test.go, after filtering observe.GlobalTrace lines, contains only removed TUI-001/INT-001 comment lines and blank-line churn — zero logic changes. The mechanism at 948431c is behaviorally identical to 413122a, so the review state is valid.

Check 1 — text-bearing final responses keep thinking collapsed: PASS.
- Evidence: go test ./internal/tui -count=1 -run 'TestThinkingOnly|TestThinkingThenText|TestTextlessFinal|TestReloadShowsTrailing' -v → rc=0, all four gates PASS (full output: /tmp/pragma/tui-focused-test-output.txt).
- finalResponseHadText is referenced only at handlers.go:255 (set on non-whitespace TextEvent), handlers.go:387 (reset on ToolResultEvent — a request boundary), handlers.go:441 (read for the promotion guard), handlers.go:770 (reset in submitPrompt), model.go:201-204 (declaration). No other writer exists, so it cannot be cleared mid-response.
- forceShow is referenced only at model.go:149 (field), model.go:432 (render read OR'd with m.verbose), model.go:536 (the sole assignment, inside promoteTrailingThinking). No other code path can force-show thinking.
- Whitespace-only TextEvent leaves the flag false, so promotion fires; the rendered whitespace tail is skipped by the scan's whitespace-only-text loop. This matches the documented intent ("whitespace alone does not" count as operator-readable) — intentional, not a defect.
- Secondary protection: TurnComplete flushes streamBuf, so unflushed non-whitespace text lands as a non-whitespace segText that halts the backward scan even if the flag were somehow wrong.

Check 2 — reload promotion only for a trailing all-thinking assistant message: PASS.
- Evidence: TestReloadShowsTrailingThinkingOnlyMessage PASS (thinking-only trailing message promoted, "FRESH session" visible; thinking+text trailing message not promoted, "reasoning hidden on reload" absent, answer text present).
- isThinkingOnlyAssistant (model.go:541-560) requires RoleAssistant AND len(Content)>0 AND every part to be model.ThinkingPart. Boundary inputs verified with an overlay-injected scratch test (file /tmp/pragma/zz_scratch_review_test.go, run via go test -overlay — never added to the repo):
  - assistant with empty content slice: no promotion; reload renders nothing (correct, len==0 short-circuit).
  - user-role message whose parts are all thinking: no promotion (role check fails). Observation, not a TUI-001 defect: RenderMessage renders user-role ThinkingPart via RenderContentPart, which always calls RenderThinking(p, true), so such content would display expanded regardless of promotion — pre-existing render behavior of user-message content, unreachable through normal provider flows (user messages carry text/tool results).
  - assistant with thinking+text: no promotion (covered by the repo gate's textBearing case).
- Scan-wide subtlety verified: reloadConversationFromStore runs promoteTrailingThinking over the entire reloaded outputSegs, but loadMessageSegments (model.go:832-920) appends a "\n" segText after every assistant message, and the promotion loop stops at any segText (it only skips whitespace in the leading run-detection loop). Scratch result for two consecutive thinking-only assistant messages: only the last is promoted, "first promoted: false". No cross-message over-promotion.

Check 3 — mid-turn queued prompt between thinking segments of the final response: DEVIATION (partial promotion), recorded, not blocking.
- No repo gate covers this; verified with the overlay scratch test TestScratchQueuedMidThinking (/tmp/pragma/zz_scratch_review_test.go, output /tmp/pragma/scratch-test-output.txt). Sequence: ThinkingEvent(A) → QueuedPromptEvent → ThinkingEvent(B) → TurnComplete(end_turn, no text).
- Observed: thinking B is promoted and rendered expanded ("TRAILING thinking B: open a FRESH session" visible); thinking A stays collapsed ("∴ Thinking (ctrl+o to expand)" rendered once); the queued prompt renders as the operator's user message plus "· queued behind the running turn — its next request sees this". The scan halts at the queued prompt's non-whitespace segText (renderAcceptedPrompt → RenderMessage ends with "❯ " + text + "\n\n"; handlers.go:186-188 appends the queued indicator).
- Over-promotion: none. Lost promotion: partial — segment A belongs to the same final response and is not promoted. Strict reading: promoteTrailingThinking's own doc comment promises "only the final response's thinking is promoted", and here part of the final response's thinking is not. Doc-consistent reading: the mechanism is defined over the trailing run, and the queued prompt is a genuine interjection the operator typed and sees echoed; the collapsed hint still exposes A via ctrl+o. Weighed verdict: not a blocker for TUI-001's goal (the response's trailing content is surfaced), but it is a real behavior/coverage gap that should be gated. Suggested gate body for a future commit (do not add now — review-only): newTestModel, streaming, resize, step ThinkingEvent{A}, step interactive.QueuedPromptEvent{Prompt:"mid-turn note"}, step ThinkingEvent{B}, step TurnCompleteEvent{StopEndTurn}; assert B's text visible, A's text absent, "ctrl+o to expand" present, queued indicator present — then decide the intended A behavior explicitly.
- TestQueuedPromptEventRenders PASS (rc=0) confirms the queued prompt text is non-whitespace rendered segText, i.e. the fact that halts the backward scan.

Check 4 — only end_turn promotes; other stops keep notice behavior: PASS.
- handlers.go:441 gates promotion on e.StopReason == model.StopEndTurn && !m.finalResponseHadText. StopMaxTokens, StopContentFiltered, StopError each append their own bracketed notice (handlers.go:429-437) and are not promoted.
- Reachable final stop reasons at TurnCompleteEvent from the provider tools loop (provider_tools_loop.go:116-123): end_turn, content_filtered, pause_turn, error. StopToolUse is excluded by the loop condition; StopMaxTokens with no tool calls is converted to ErrorEvent (line 116-119), so the TUI's max_tokens notice branch is unreachable from this loop (minor dead-branch observation, harmless).
- Residual edge, noted not blocking: StopPauseTurn has no notice branch in the TUI; a thinking-only pause_turn response would render only the collapsed hint. Anthropic maps pause_turn (translate_in.go:119), so the shape is representable even if rare.
- Orchestration path scan (bounded per the brief): internal/orchestration/runner.go:233 and 258 emit TurnCompleteEvent{StopEndTurn} after orchestration states. runner.go:969-1005 forwards sub-loop ThinkingEvent and TextEvent to the channel, so an orchestration state whose sub-loop ends thinking-only does get its trailing thinking promoted — consistent with TUI-001's intent, trailing-run only, no over-promotion. Orchestration-handler segments (handlers.go:279-330) append only non-whitespace text notices, which halt the scan. internal/query/miniswe_loop.go emits no ThinkingEvent at all (only TextEvent at 1280; thinking parts are stripped in replay at 1349), so its three TurnCompleteEvent{StopEndTurn} emissions can only re-mark already-promoted thinking (no-op) or halt on trailing text. No false promotion found. ORCH-001/ORCH-002 gates and the MCPINJ probe were not investigated further, per scope.

Changed:
- No repository files changed (review-only; git status shows only the pre-existing untracked e2e/orchestration/results/orch-dogfood-2026-09-12 from 17:59:46, before this session).
- Created /tmp/pragma/zz_scratch_review_test.go and /tmp/pragma/overlay.json (Go overlay; never injected into the repo tree) plus validation logs under /tmp/pragma/: tui-focused-test-output.txt, tui-suite-output.txt, queued-test-output.txt, scratch-test-output.txt.

Validation:
- go test ./internal/tui -count=1 → rc=0, ok github.com/artpar/pragma/internal/tui 3.184s (/tmp/pragma/tui-suite-output.txt).
- go test ./internal/tui -count=1 -run 'TestThinkingOnly|TestThinkingThenText|TestTextlessFinal|TestReloadShowsTrailing' -v → rc=0, 4/4 PASS (/tmp/pragma/tui-focused-test-output.txt).
- go test ./internal/tui -count=1 -run 'TestQueuedPromptEventRenders' -v → rc=0, PASS (/tmp/pragma/queued-test-output.txt).
- go test -overlay=/tmp/pragma/overlay.json ./internal/tui -count=1 -run 'TestScratch' -v → rc=1 by design: TestScratchQueuedMidThinking PASS (records the partial-promotion behavior), TestScratchReloadBoundaries "fails" only on the scratch's own deliberately-strict user-role assertion, which is explained above as pre-existing RenderMessage behavior, not a promotion; the empty-content and two-assistant-message probes returned the expected results (/tmp/pragma/scratch-test-output.txt).
- git diff 413122a 948431c -- internal/tui/ (trace-filtered) → only comment removals, no logic change.

Remaining risk:
- The queued-mid-thinking partial promotion (check 3) is un-gated in the repo; until a gate exists, a regression that silently drops promotion entirely, or over-promotes across the queued text, would not be caught.
- StopPauseTurn has no TUI notice branch; a thinking-only paused response renders as a lone collapsed hint.
- The TUI's StopMaxTokens notice branch appears unreachable from the provider tools loop (converted to ErrorEvent); harmless but worth confirming if other emitters are added.
- The reload promotion scans all reloaded segments rather than only the last message's; safe today because loadMessageSegments appends an inter-message "\n", but a future change to that terminator could introduce cross-message promotion.

Blocker:
- none

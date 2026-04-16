package compact

import "github.com/artpar/pragma/internal/observe"

// Threshold constants matching the TS reference (autoCompact.ts).
const (
	// AutoCompactBufferTokens is the safety margin below the effective context window
	// before auto-compaction triggers.
	AutoCompactBufferTokens = 13_000

	// WarningThresholdBufferTokens is the margin for showing a yellow warning in the TUI.
	WarningThresholdBufferTokens = 20_000

	// MaxOutputTokensForSummary is the max tokens reserved for the compaction summary.
	MaxOutputTokensForSummary = 20_000

	// MaxConsecutiveFailures is the circuit breaker limit for auto-compaction.
	// After this many consecutive failures, auto-compaction stops until the session ends.
	// GitHub issue #42055: observed 3,272 consecutive failures without a breaker.
	MaxConsecutiveFailures = 3

	// MinTurnsCooldown is the minimum turns after a successful compaction before
	// auto-compaction can trigger again. Prevents death spirals (#24179, #22195)
	// where compaction → re-inject system prompt → above threshold → re-compact.
	MinTurnsCooldown = 2
)

// WindowConfig holds model-specific context window parameters.
type WindowConfig struct {
	// ContextWindow is the total context window in tokens (e.g., 200_000 or 1_000_000 for [1m]).
	// GitHub issue #41984: must parse model variant suffixes like [1m].
	ContextWindow int

	// MaxOutput is the max output tokens the model supports for a single response.
	MaxOutput int

	// SystemPromptEst is the estimated system prompt tokens.
	// Subtracted from effective window to prevent death spiral (#24179):
	// after compaction, system prompt is re-injected, and if the threshold
	// doesn't account for it, immediate re-compaction triggers.
	SystemPromptEst int
}

// ThresholdState reports the current context window usage state.
type ThresholdState struct {
	TokenCount                  int
	EffectiveWindow             int
	IsAboveWarningThreshold     bool
	IsAboveAutoCompactThreshold bool
	PercentUsed                 int // 0-100, for toolbar/status display
}

// EffectiveWindow returns the usable context window after reserving output tokens
// and system prompt space.
func EffectiveWindow(wc WindowConfig) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ew := wc.ContextWindow - wc.MaxOutput - wc.SystemPromptEst
	if ew < 0 {
		observe.GlobalTrace("if: ew < 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	observe.GlobalTrace("return: ew")
	return ew
}

// AutoCompactThreshold returns the token count above which auto-compaction triggers.
func AutoCompactThreshold(wc WindowConfig) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	threshold := EffectiveWindow(wc) - AutoCompactBufferTokens
	if threshold < 0 {
		observe.GlobalTrace("if: threshold < 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	observe.GlobalTrace("return: threshold")
	return threshold
}

// WarningThreshold returns the token count above which a warning is shown.
func WarningThreshold(wc WindowConfig) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	threshold := EffectiveWindow(wc) - WarningThresholdBufferTokens
	if threshold < 0 {
		observe.GlobalTrace("if: threshold < 0")
		observe.GlobalTrace("return: 0")
		return 0
	}
	observe.GlobalTrace("return: threshold")
	return threshold
}

// CalculateThresholdState computes the current context window usage state.
func CalculateThresholdState(tokenCount int, wc WindowConfig) ThresholdState {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ew := EffectiveWindow(wc)
	pct := 0
	if ew > 0 {
		observe.GlobalTrace("if: ew > 0")
		pct = tokenCount * 100 / ew
		if pct > 100 {
			observe.GlobalTrace("if: pct > 100")
			pct = 100
		}
	}
	observe.GlobalTrace("return: ThresholdState{\n\tTokenCount:\t\t\ttokenCount,\n\tEffectiveWindow:\t\tew,\n\tIsAboveWar...")
	return ThresholdState{
		TokenCount:                  tokenCount,
		EffectiveWindow:             ew,
		IsAboveWarningThreshold:     tokenCount >= WarningThreshold(wc),
		IsAboveAutoCompactThreshold: tokenCount >= AutoCompactThreshold(wc),
		PercentUsed:                 pct,
	}
}

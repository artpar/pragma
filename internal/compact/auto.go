package compact

import "github.com/artpar/gogent/internal/observe"

// AutoTracker tracks auto-compaction state across turns.
// It implements a circuit breaker (max 3 consecutive failures) and a cooldown
// (min 2 turns after successful compaction) to prevent death spirals.
//
// GitHub issues addressed:
// - #42055: 3,272 consecutive failures without a breaker
// - #24179: death spiral from immediate re-compaction
// - #42817: disable methods must actually disable
type AutoTracker struct {
	consecutiveFailures int
	compacted           bool
	turnsSinceCompact   int
	disabled            bool
}

// NewAutoTracker creates a fresh tracker.
// If disabled is true, ShouldAutoCompact always returns false.
// Pass true when DISABLE_AUTO_COMPACT env var is set (#42817).
func NewAutoTracker(disabled bool) *AutoTracker {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &AutoTracker{\n\tdisabled:\t\tdisabled,\n\tturnsSinceCompact:\tMinTurnsCooldown,\n}")
	return &AutoTracker{
		disabled:          disabled,
		turnsSinceCompact: MinTurnsCooldown,
	}
}

// ShouldAutoCompact returns true if token count exceeds the auto-compact threshold
// and no blocking condition is active.
func (t *AutoTracker) ShouldAutoCompact(tokenCount int, wc WindowConfig) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if t.disabled {
		observe.GlobalTrace("if: t.disabled")
		observe.GlobalTrace("return: false")
		return false
	}

	if t.consecutiveFailures >= MaxConsecutiveFailures {
		observe.GlobalTrace("if: t.consecutiveFailures >= MaxConsecutiveFailures")
		observe.GlobalTrace("return: false")
		return false
	}

	if t.compacted && t.turnsSinceCompact < MinTurnsCooldown {
		observe.GlobalTrace("if: t.compacted && t.turnsSinceCompact < MinTurnsCooldown")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: tokenCount >= AutoCompactThreshold(wc)")

	return tokenCount >= AutoCompactThreshold(wc)
}

// RecordSuccess resets the failure counter and starts the cooldown timer.
func (t *AutoTracker) RecordSuccess() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.consecutiveFailures = 0
	t.compacted = true
	t.turnsSinceCompact = 0
}

// RecordFailure increments the failure counter. Returns true if the
// circuit breaker has tripped (>= MaxConsecutiveFailures).
func (t *AutoTracker) RecordFailure() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.consecutiveFailures++
	observe.GlobalTrace("return: t.consecutiveFailures >= MaxConsecutiveFailures")
	return t.consecutiveFailures >= MaxConsecutiveFailures
}

// IncrementTurn advances the turn counter for cooldown tracking.
func (t *AutoTracker) IncrementTurn() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.turnsSinceCompact++
}

package compact

import "github.com/artpar/pragma/internal/observe"

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

// AutoCompactEligible reports whether a token count could change
// ShouldAutoCompact's decision this turn. When the tracker is disabled,
// the circuit breaker has tripped, or the post-compaction cooldown is
// still active, ShouldAutoCompact returns false for EVERY token count —
// its state checks short-circuit before the threshold comparison — so
// callers can skip computing a count entirely (CMP-001.4 F6: provider
// CountTokens is a network call on Google). Each condition here mirrors
// a verbatim early-return of ShouldAutoCompact; the equivalence is
// pinned by TestAutoCompactEligibleNeverDisagreesWithShouldAutoCompact.
// IncrementTurn must still run every iteration regardless of
// eligibility — the cooldown expires by counting iterations.
func (t *AutoTracker) AutoCompactEligible() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: !t.disabled && t.consecutiveFailures < MaxConsecutiveFailures && !(t.compacted && t.turnsSinceCompact < MinTurnsCooldown)")
	return !t.disabled &&
		t.consecutiveFailures < MaxConsecutiveFailures &&
		!(t.compacted && t.turnsSinceCompact < MinTurnsCooldown)
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

// FailureCount returns the current consecutive failure count.
func (t *AutoTracker) FailureCount() int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: t.consecutiveFailures")
	return t.consecutiveFailures
}

// IncrementTurn advances the turn counter for cooldown tracking.
func (t *AutoTracker) IncrementTurn() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.turnsSinceCompact++
}

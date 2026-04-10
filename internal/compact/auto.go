package compact

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
	return &AutoTracker{
		disabled:          disabled,
		turnsSinceCompact: MinTurnsCooldown, // allow first compact immediately
	}
}

// ShouldAutoCompact returns true if token count exceeds the auto-compact threshold
// and no blocking condition is active.
func (t *AutoTracker) ShouldAutoCompact(tokenCount int, wc WindowConfig) bool {
	// #42817: disable must actually disable — no exceptions
	if t.disabled {
		return false
	}

	// Circuit breaker: stop after MaxConsecutiveFailures
	if t.consecutiveFailures >= MaxConsecutiveFailures {
		return false
	}

	// Cooldown: wait MinTurnsCooldown turns after last successful compaction
	// to prevent death spiral (#24179)
	if t.compacted && t.turnsSinceCompact < MinTurnsCooldown {
		return false
	}

	// Threshold check
	return tokenCount >= AutoCompactThreshold(wc)
}

// RecordSuccess resets the failure counter and starts the cooldown timer.
func (t *AutoTracker) RecordSuccess() {
	t.consecutiveFailures = 0
	t.compacted = true
	t.turnsSinceCompact = 0
}

// RecordFailure increments the failure counter. Returns true if the
// circuit breaker has tripped (>= MaxConsecutiveFailures).
func (t *AutoTracker) RecordFailure() bool {
	t.consecutiveFailures++
	return t.consecutiveFailures >= MaxConsecutiveFailures
}

// IncrementTurn advances the turn counter for cooldown tracking.
func (t *AutoTracker) IncrementTurn() {
	t.turnsSinceCompact++
}

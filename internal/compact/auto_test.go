package compact

import "testing"

var testWindowConfig = WindowConfig{
	ContextWindow:   200_000,
	MaxOutput:       16_384,
	SystemPromptEst: 5_000,
}

func TestAutoTrackerDisabled(t *testing.T) {
	tracker := NewAutoTracker(true)
	// Even with massive token count, disabled tracker returns false
	if tracker.ShouldAutoCompact(999_999, testWindowConfig) {
		t.Error("disabled tracker should never trigger auto-compact")
	}
}

func TestAutoTrackerBelowThreshold(t *testing.T) {
	tracker := NewAutoTracker(false)
	if tracker.ShouldAutoCompact(10_000, testWindowConfig) {
		t.Error("should not trigger below threshold")
	}
}

func TestAutoTrackerAboveThreshold(t *testing.T) {
	tracker := NewAutoTracker(false)
	threshold := AutoCompactThreshold(testWindowConfig)
	if !tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Errorf("should trigger above threshold (%d)", threshold)
	}
}

func TestAutoTrackerCircuitBreaker(t *testing.T) {
	tracker := NewAutoTracker(false)
	threshold := AutoCompactThreshold(testWindowConfig)

	// Three failures trip the breaker
	for i := range MaxConsecutiveFailures {
		if !tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
			t.Fatalf("should trigger before breaker trips (failure %d)", i)
		}
		tripped := tracker.RecordFailure()
		if i < MaxConsecutiveFailures-1 && tripped {
			t.Fatalf("should not trip at failure %d", i)
		}
		tracker.IncrementTurn()
	}

	// After 3 failures, should not trigger even above threshold
	if tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("should not trigger after circuit breaker tripped")
	}
}

func TestAutoTrackerCooldown(t *testing.T) {
	tracker := NewAutoTracker(false)
	threshold := AutoCompactThreshold(testWindowConfig)

	// First compaction succeeds
	tracker.RecordSuccess()

	// Immediately after success: cooldown should prevent re-compaction
	if tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("should not trigger during cooldown (turn 0)")
	}
	tracker.IncrementTurn()
	if tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("should not trigger during cooldown (turn 1)")
	}
	tracker.IncrementTurn()

	// After MinTurnsCooldown, should trigger again
	if !tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("should trigger after cooldown expires")
	}
}

func TestAutoTrackerSuccessResetsFailures(t *testing.T) {
	tracker := NewAutoTracker(false)
	threshold := AutoCompactThreshold(testWindowConfig)

	// Two failures
	tracker.RecordFailure()
	tracker.RecordFailure()
	tracker.IncrementTurn()

	// Success resets
	tracker.RecordSuccess()
	tracker.IncrementTurn()
	tracker.IncrementTurn()

	// Should trigger again (failures reset)
	if !tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("should trigger after success resets failures")
	}

	// Two more failures should not trip (only 2, not 3)
	tracker.RecordFailure()
	tracker.RecordFailure()
	tracker.IncrementTurn()
	if !tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("should trigger with only 2 failures")
	}
}

func TestAutoTrackerNewStartsWithoutCooldown(t *testing.T) {
	tracker := NewAutoTracker(false)
	threshold := AutoCompactThreshold(testWindowConfig)

	// Fresh tracker should be able to compact immediately (no cooldown on first use)
	if !tracker.ShouldAutoCompact(threshold+1, testWindowConfig) {
		t.Error("new tracker should allow immediate compaction")
	}
}

package compact

import "testing"

// TestAutoCompactEligibleNeverDisagreesWithShouldAutoCompact pins the
// CMP-001.4 F6 bound's safety property across the reachable tracker
// state space: ShouldAutoCompact(c) is EXACTLY AutoCompactEligible() &&
// c >= threshold. The ineligible states (disabled, tripped breaker,
// active cooldown) are precisely the ones where ShouldAutoCompact's
// state checks short-circuit before the threshold comparison, so loops
// may skip computing a token count entirely — including the provider
// CountTokens network call on Google — without ever suppressing a
// trigger that a count would have produced.
func TestAutoCompactEligibleNeverDisagreesWithShouldAutoCompact(t *testing.T) {
	wc := WindowConfig{ContextWindow: 20_000, MaxOutput: 4096, SystemPromptEst: 2000}
	threshold := AutoCompactThreshold(wc)
	if threshold != 904 {
		t.Fatalf("AutoCompactThreshold = %d, want 904 (fixture drift)", threshold)
	}
	counts := []int{0, threshold - 1, threshold, threshold + 1, 1 << 30}
	states := 0
	for _, disabled := range []bool{false, true} {
		for _, recordSuccess := range []bool{false, true} {
			for turns := 0; turns <= MinTurnsCooldown+1; turns++ {
				for failures := 0; failures <= MaxConsecutiveFailures; failures++ {
					tr := NewAutoTracker(disabled)
					if recordSuccess {
						// compacted: turnsSinceCompact counts from 0
						tr.RecordSuccess()
						for i := 0; i < turns; i++ {
							tr.IncrementTurn()
						}
					} else if turns > MinTurnsCooldown {
						// fresh trackers start at MinTurnsCooldown
						for i := 0; i < turns-MinTurnsCooldown; i++ {
							tr.IncrementTurn()
						}
					}
					for i := 0; i < failures; i++ {
						tr.RecordFailure()
					}
					states++
					eligible := tr.AutoCompactEligible()
					for _, c := range counts {
						got := tr.ShouldAutoCompact(c, wc)
						want := eligible && c >= threshold
						if got != want {
							t.Fatalf("state(disabled=%v, compacted=%v, turns=%d, failures=%d), count=%d: ShouldAutoCompact=%v, want %v (eligible=%v)",
								disabled, recordSuccess, turns, failures, c, got, want, eligible)
						}
					}
				}
			}
		}
	}
	if states < 50 {
		t.Fatalf("state matrix covered only %d states — enumeration is broken", states)
	}
}

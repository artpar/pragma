package compact

import "testing"

// TestShouldAutoCompactZeroWindowNeverTriggers guards CMP-001.4 F8: a
// zero/unconfigured WindowConfig yields AutoCompactThreshold 0, so every
// cooldown-eligible iteration looked over-threshold — compaction (a
// provider call that replaces the whole conversation) would fire every
// two turns forever, whatever the conversation size. An unknown window
// must disable the trigger instead.
func TestShouldAutoCompactZeroWindowNeverTriggers(t *testing.T) {
	tracker := NewAutoTracker(false)
	for _, count := range []int{0, 1, 100_000, 1_000_000} {
		if tracker.ShouldAutoCompact(count, WindowConfig{}) {
			t.Fatalf("ShouldAutoCompact(%d, zero WindowConfig) = true: threshold 0 compacts every cooldown iteration regardless of conversation size (CMP-001.4 F8)", count)
		}
	}
	// A configured window still triggers as before — the guard must
	// reject only unknown windows.
	if !tracker.ShouldAutoCompact(999_999, WindowConfig{ContextWindow: 200_000, MaxOutput: 4_096, SystemPromptEst: 2_000}) {
		t.Fatalf("configured window stopped triggering — the guard must only reject unknown windows")
	}
}

package compact

import "testing"

func TestEffectiveWindow(t *testing.T) {
	tests := []struct {
		name string
		wc   WindowConfig
		want int
	}{
		{
			name: "200K context standard",
			wc:   WindowConfig{ContextWindow: 200_000, MaxOutput: 16_384, SystemPromptEst: 5_000},
			want: 200_000 - 16_384 - 5_000,
		},
		{
			name: "1M context extended",
			wc:   WindowConfig{ContextWindow: 1_000_000, MaxOutput: 16_384, SystemPromptEst: 10_000},
			want: 1_000_000 - 16_384 - 10_000,
		},
		{
			name: "large system prompt eats window",
			wc:   WindowConfig{ContextWindow: 200_000, MaxOutput: 16_384, SystemPromptEst: 50_000},
			want: 200_000 - 16_384 - 50_000,
		},
		{
			name: "negative clamps to zero",
			wc:   WindowConfig{ContextWindow: 10_000, MaxOutput: 16_384, SystemPromptEst: 5_000},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveWindow(tt.wc)
			if got != tt.want {
				t.Errorf("EffectiveWindow() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAutoCompactThreshold(t *testing.T) {
	// Standard 200K with 5K system prompt, 16K output
	wc := WindowConfig{ContextWindow: 200_000, MaxOutput: 16_384, SystemPromptEst: 5_000}
	ew := EffectiveWindow(wc) // 178_616
	want := ew - AutoCompactBufferTokens
	got := AutoCompactThreshold(wc)
	if got != want {
		t.Errorf("AutoCompactThreshold() = %d, want %d", got, want)
	}
}

func TestAutoCompactThreshold1M(t *testing.T) {
	// 1M context — threshold should be much higher
	wc := WindowConfig{ContextWindow: 1_000_000, MaxOutput: 16_384, SystemPromptEst: 10_000}
	got := AutoCompactThreshold(wc)
	if got < 900_000 {
		t.Errorf("AutoCompactThreshold(1M) = %d, expected > 900_000", got)
	}
}

func TestCalculateThresholdState(t *testing.T) {
	wc := WindowConfig{ContextWindow: 200_000, MaxOutput: 16_384, SystemPromptEst: 5_000}
	ew := EffectiveWindow(wc)

	tests := []struct {
		name          string
		tokenCount    int
		wantWarning   bool
		wantCompact   bool
		wantPctRange  [2]int // min, max
	}{
		{
			name:         "low usage",
			tokenCount:   10_000,
			wantWarning:  false,
			wantCompact:  false,
			wantPctRange: [2]int{5, 6},
		},
		{
			name:         "above warning threshold",
			tokenCount:   ew - WarningThresholdBufferTokens + 1000,
			wantWarning:  true,
			wantCompact:  false,
			wantPctRange: [2]int{88, 92},
		},
		{
			name:         "above auto-compact threshold",
			tokenCount:   ew - AutoCompactBufferTokens + 1000,
			wantWarning:  true,
			wantCompact:  true,
			wantPctRange: [2]int{92, 95},
		},
		{
			name:         "zero tokens",
			tokenCount:   0,
			wantWarning:  false,
			wantCompact:  false,
			wantPctRange: [2]int{0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := CalculateThresholdState(tt.tokenCount, wc)
			if state.IsAboveWarningThreshold != tt.wantWarning {
				t.Errorf("IsAboveWarningThreshold = %v, want %v", state.IsAboveWarningThreshold, tt.wantWarning)
			}
			if state.IsAboveAutoCompactThreshold != tt.wantCompact {
				t.Errorf("IsAboveAutoCompactThreshold = %v, want %v", state.IsAboveAutoCompactThreshold, tt.wantCompact)
			}
			if state.PercentUsed < tt.wantPctRange[0] || state.PercentUsed > tt.wantPctRange[1] {
				t.Errorf("PercentUsed = %d, want [%d, %d]", state.PercentUsed, tt.wantPctRange[0], tt.wantPctRange[1])
			}
		})
	}
}

func TestThresholdNegativeWindowClamped(t *testing.T) {
	// Context window smaller than output + system prompt
	wc := WindowConfig{ContextWindow: 5_000, MaxOutput: 10_000, SystemPromptEst: 1_000}
	if AutoCompactThreshold(wc) != 0 {
		t.Errorf("expected 0 threshold for negative effective window")
	}
	if WarningThreshold(wc) != 0 {
		t.Errorf("expected 0 warning threshold for negative effective window")
	}
}

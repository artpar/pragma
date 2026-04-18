package observe

import (
	"fmt"
	"sync"
)

// TokenMonitor tracks cumulative token usage and emits warnings at threshold
// percentages of the context window budget. Uses actual model context window
// size (not hardcoded) to avoid GitHub bugs #34332 and #39467.
type TokenMonitor struct {
	bus        *EventBus
	budget     int // actual context window tokens for this model
	mu         sync.Mutex
	latestFill int // latest request's context fill (InputTokens + CacheReadInputTokens)
	warned50   bool
	warned80   bool
	warned95   bool
}

// NewTokenMonitor creates a monitor with a default budget.
// Call SetBudget after the provider resolves the actual context window.
func NewTokenMonitor(bus *EventBus, defaultBudget int) *TokenMonitor {
	return &TokenMonitor{bus: bus, budget: defaultBudget}
}

// SetBudget updates the context window budget (e.g., after provider.ContextWindow returns).
func (m *TokenMonitor) SetBudget(tokens int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.budget = tokens
}

// Budget returns the context window budget in tokens.
func (m *TokenMonitor) Budget() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.budget
}

// HandleEvent implements Subscriber. Tracks APIRequestCompleted events.
// Uses the latest request's context fill (not cumulative) because each request
// sends the full conversation — the latest fill IS the current context usage.
func (m *TokenMonitor) HandleEvent(event Event) {
	completed, ok := event.(APIRequestCompleted)
	if !ok {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.latestFill = completed.Usage.InputTokens + completed.Usage.CacheReadInputTokens

	if m.budget <= 0 {
		return
	}

	pct := float64(m.latestFill) / float64(m.budget) * 100

	// Check thresholds in ascending order — each fires independently so
	// a jump from 40% to 97% emits all three warnings, not just 95%.
	if pct >= 50 && !m.warned50 {
		m.warned50 = true
		m.emitWarning("info",
			fmt.Sprintf("Token usage at 50%% of context window (%d/%d tokens).", m.latestFill, m.budget))
	}
	if pct >= 80 && !m.warned80 {
		m.warned80 = true
		m.emitWarning("warning",
			fmt.Sprintf("Token usage at 80%% of context window (%d/%d tokens).", m.latestFill, m.budget))
	}
	if pct >= 95 && !m.warned95 {
		m.warned95 = true
		m.emitWarning("error",
			fmt.Sprintf("Token usage at 95%% of context window (%d/%d tokens). Consider running /compact to free space.", m.latestFill, m.budget))
	}
}

func (m *TokenMonitor) emitWarning(severity, message string) {
	if m.bus == nil {
		return
	}
	m.bus.Emit(ErrorOccurred{
		EventHeader:  NewEventHeader("ErrorOccurred", "", "", ""),
		Severity:     severity,
		Component:    "token_monitor",
		ErrorType:    "token_threshold",
		ErrorMessage: message,
	})
}

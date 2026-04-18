package observe

import (
	"sync"
	"time"

	"github.com/artpar/pragma/internal/model"
)

// Metrics is a Subscriber that accumulates operational counters.
type Metrics struct {
	mu                sync.RWMutex
	tokenUsage        model.TokenUsage
	latestContextFill int // latest request's InputTokens + CacheReadInputTokens (actual context window fill)
	turnCount         int
	toolCalls         map[string]int
	toolDurations     map[string]int64
	toolErrors        map[string]int
	apiCalls          int
	apiErrors         int
	apiTotalLatencyMs int64
	compactions       int
	sessionStart      time.Time
	sessionDurationMs int64
}

// ToolStat holds per-tool execution statistics.
type ToolStat struct {
	Calls      int   `json:"calls"`
	Errors     int   `json:"errors"`
	TotalDurMs int64 `json:"total_duration_ms"`
	AvgDurMs   int64 `json:"avg_duration_ms"`
}

// MetricsSnapshot is a point-in-time copy of metrics for display.
type MetricsSnapshot struct {
	TokenUsage        model.TokenUsage
	LatestContextFill int // latest request's context window fill (InputTokens + CacheReadInputTokens)
	TurnCount         int
	ToolCallCount     int
	ToolErrorCount    int
	APICallCount      int
	APIErrorCount     int
	AvgAPILatencyMs   int64
	Compactions       int
	SessionDurationMs int64
	ToolStats         map[string]ToolStat
}

func NewMetrics() *Metrics {
	return &Metrics{
		toolCalls:     make(map[string]int),
		toolDurations: make(map[string]int64),
		toolErrors:    make(map[string]int),
	}
}

func (m *Metrics) HandleEvent(event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch e := event.(type) {
	case APIRequestCompleted:
		m.apiCalls++
		m.apiTotalLatencyMs += e.DurationMs
		m.tokenUsage.InputTokens += e.Usage.InputTokens
		m.tokenUsage.OutputTokens += e.Usage.OutputTokens
		m.tokenUsage.CacheCreationInputTokens += e.Usage.CacheCreationInputTokens
		m.tokenUsage.CacheReadInputTokens += e.Usage.CacheReadInputTokens
		// Latest (not cumulative) — actual context window fill for this request.
		// Each request sends the full conversation, so the latest request's total
		// input is how full the context window is right now.
		m.latestContextFill = e.Usage.InputTokens + e.Usage.CacheReadInputTokens
	case APIRequestFailed:
		m.apiErrors++
	case ToolExecutionCompleted:
		m.toolCalls[e.ToolName]++
		m.toolDurations[e.ToolName] += e.DurationMs
		if e.IsError {
			m.toolErrors[e.ToolName]++
		}
	case ToolExecutionFailed:
		m.toolErrors[e.ToolName]++
	case CompactionCompleted:
		m.compactions++
	case MessageAppended:
		if e.Role == "user" {
			m.turnCount++
		}
	case SessionStarted:
		m.sessionStart = e.EventTimestamp()
	case SessionEnded:
		if e.DurationMs > 0 {
			m.sessionDurationMs = e.DurationMs
		} else if !m.sessionStart.IsZero() {
			m.sessionDurationMs = e.EventTimestamp().Sub(m.sessionStart).Milliseconds()
		}
	}
}

// Snapshot returns a point-in-time copy of all metrics.
func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalToolCalls, totalToolErrors int
	for _, c := range m.toolCalls {
		totalToolCalls += c
	}
	for _, c := range m.toolErrors {
		totalToolErrors += c
	}

	var avgLatency int64
	if m.apiCalls > 0 {
		avgLatency = m.apiTotalLatencyMs / int64(m.apiCalls)
	}

	var sessionMs int64
	if m.sessionDurationMs > 0 {
		sessionMs = m.sessionDurationMs
	} else if !m.sessionStart.IsZero() {
		sessionMs = time.Since(m.sessionStart).Milliseconds()
	}

	toolStats := make(map[string]ToolStat, len(m.toolCalls))
	for name, calls := range m.toolCalls {
		var avg int64
		if calls > 0 {
			avg = m.toolDurations[name] / int64(calls)
		}
		toolStats[name] = ToolStat{
			Calls:      calls,
			Errors:     m.toolErrors[name],
			TotalDurMs: m.toolDurations[name],
			AvgDurMs:   avg,
		}
	}

	return MetricsSnapshot{
		TokenUsage:        m.tokenUsage,
		LatestContextFill: m.latestContextFill,
		TurnCount:         m.turnCount,
		ToolCallCount:     totalToolCalls,
		ToolErrorCount:    totalToolErrors,
		APICallCount:      m.apiCalls,
		APIErrorCount:     m.apiErrors,
		AvgAPILatencyMs:   avgLatency,
		Compactions:       m.compactions,
		SessionDurationMs: sessionMs,
		ToolStats:         toolStats,
	}
}

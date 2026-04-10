package observe

import (
	"sync"
	"testing"

	"github.com/artpar/gogent/internal/model"
)

func TestMetricsAPIRequestCompleted(t *testing.T) {
	m := NewMetrics()
	m.HandleEvent(APIRequestCompleted{
		EventHeader: NewEventHeader("APIRequestCompleted", "t1", "s1", ""),
		StopReason:  model.StopEndTurn,
		Usage:       model.TokenUsage{InputTokens: 1000, OutputTokens: 200},
		DurationMs:  500,
		Model:       "test",
	})

	snap := m.Snapshot()
	if snap.APICallCount != 1 {
		t.Errorf("APICallCount: got %d, want 1", snap.APICallCount)
	}
	if snap.TokenUsage.InputTokens != 1000 {
		t.Errorf("InputTokens: got %d, want 1000", snap.TokenUsage.InputTokens)
	}
	if snap.TokenUsage.OutputTokens != 200 {
		t.Errorf("OutputTokens: got %d, want 200", snap.TokenUsage.OutputTokens)
	}
	if snap.AvgAPILatencyMs != 500 {
		t.Errorf("AvgAPILatencyMs: got %d, want 500", snap.AvgAPILatencyMs)
	}
}

func TestMetricsToolExecution(t *testing.T) {
	m := NewMetrics()
	m.HandleEvent(ToolExecutionCompleted{
		EventHeader: NewEventHeader("ToolExecutionCompleted", "t1", "s1", ""),
		ToolCallID:  "tc-1",
		ToolName:    "Bash",
		DurationMs:  100,
		IsError:     false,
	})
	m.HandleEvent(ToolExecutionCompleted{
		EventHeader: NewEventHeader("ToolExecutionCompleted", "t1", "s2", ""),
		ToolCallID:  "tc-2",
		ToolName:    "Bash",
		DurationMs:  200,
		IsError:     true,
	})

	snap := m.Snapshot()
	if snap.ToolCallCount != 2 {
		t.Errorf("ToolCallCount: got %d, want 2", snap.ToolCallCount)
	}
	if snap.ToolErrorCount != 1 {
		t.Errorf("ToolErrorCount: got %d, want 1", snap.ToolErrorCount)
	}
}

func TestMetricsConcurrentSnapshot(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup

	// Concurrent writes
	wg.Add(50)
	for range 50 {
		go func() {
			defer wg.Done()
			m.HandleEvent(APIRequestCompleted{
				EventHeader: NewEventHeader("APIRequestCompleted", "t1", "s1", ""),
				Usage:       model.TokenUsage{InputTokens: 10, OutputTokens: 5},
				DurationMs:  100,
			})
		}()
	}

	// Concurrent reads
	wg.Add(50)
	for range 50 {
		go func() {
			defer wg.Done()
			_ = m.Snapshot()
		}()
	}

	wg.Wait()
	snap := m.Snapshot()
	if snap.APICallCount != 50 {
		t.Errorf("APICallCount: got %d, want 50", snap.APICallCount)
	}
}

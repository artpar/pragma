package tool

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
)

// allowAllChecker allows every tool.
type allowAllChecker struct{}

func (allowAllChecker) Check(_ context.Context, _ string, _ json.RawMessage) permission.CheckResult {
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     permission.Rule{Pattern: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}

// denyChecker denies a specific tool, allows everything else.
type denyChecker struct {
	denyTool string
}

func (c denyChecker) Check(_ context.Context, toolName string, _ json.RawMessage) permission.CheckResult {
	if toolName == c.denyTool {
		return permission.CheckResult{
			Decision: permission.DecisionDeny,
			Rule:     permission.Rule{Pattern: toolName, Decision: permission.DecisionDeny, Source: "test"},
			Reason:   "denied by test",
		}
	}
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     permission.Rule{Pattern: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}

// failingTool is a real tool that always returns an error.
type failingTool struct {
	echoTool
}

func (t *failingTool) Invoke(_ context.Context, _ json.RawMessage, _ StateSnapshot) (InvokeResult, error) {
	return InvokeResult{}, errors.New("tool execution failed")
}

// staticState satisfies StateSnapshot.
type staticState struct {
	workDir string
}

func (s staticState) WorkDir() string { return s.workDir }

func setupOrchestrator(t *testing.T, checker permission.Checker, tools ...Descriptor) (*Orchestrator, *observe.EventBus, *collectingSubscriber) {
	t.Helper()
	bus := observe.NewEventBus(10000)
	sub := &collectingSubscriber{}
	bus.Subscribe(sub)

	reg := NewRegistry(bus)
	for _, tool := range tools {
		if err := reg.Register(tool); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	orch := NewOrchestrator(reg, checker, bus)
	return orch, bus, sub
}

type collectingSubscriber struct {
	events []observe.Event
}

func (s *collectingSubscriber) HandleEvent(event observe.Event) {
	s.events = append(s.events, event)
}

func (s *collectingSubscriber) eventKinds() []string {
	kinds := make([]string, len(s.events))
	for i, e := range s.events {
		kinds[i] = e.EventKind()
	}
	return kinds
}

func TestOrchestratorSingleTool(t *testing.T) {
	orch, bus, _ := setupOrchestrator(t, allowAllChecker{}, newEchoTool("Bash", false))
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)},
	}

	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})

	if len(results.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(results.Results))
	}
	if results.Results[0].ToolCallID != "tc-1" {
		t.Errorf("ToolCallID: got %q", results.Results[0].ToolCallID)
	}
	if results.Results[0].IsError {
		t.Errorf("unexpected error: %s", results.Results[0].Content)
	}
	if results.Results[0].Content != `{"command":"ls"}` {
		t.Errorf("Content: got %q", results.Results[0].Content)
	}
}

func TestOrchestratorConcurrentTools(t *testing.T) {
	// Create tools that sleep briefly to verify concurrent execution
	sleepTool := &sleepingTool{
		echoTool: *newEchoTool("SlowTool", true),
		delay:    50 * time.Millisecond,
	}

	orch, bus, _ := setupOrchestrator(t, allowAllChecker{}, sleepTool)
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "SlowTool", Input: json.RawMessage(`"a"`)},
		{ID: "tc-2", Name: "SlowTool", Input: json.RawMessage(`"b"`)},
		{ID: "tc-3", Name: "SlowTool", Input: json.RawMessage(`"c"`)},
	}

	start := time.Now()
	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})
	elapsed := time.Since(start)

	if len(results.Results) != 3 {
		t.Fatalf("results: got %d, want 3", len(results.Results))
	}

	// If running concurrently, total time should be ~50ms, not ~150ms
	if elapsed > 120*time.Millisecond {
		t.Errorf("concurrent execution took %v, expected <120ms", elapsed)
	}
}

type sleepingTool struct {
	echoTool
	delay   time.Duration
	invoked atomic.Int32
}

func (t *sleepingTool) Invoke(ctx context.Context, input json.RawMessage, _ StateSnapshot) (InvokeResult, error) {
	t.invoked.Add(1)
	select {
	case <-time.After(t.delay):
		return InvokeResult{Content: string(input)}, nil
	case <-ctx.Done():
		return InvokeResult{}, ctx.Err()
	}
}

func TestOrchestratorSerialTools(t *testing.T) {
	sleepTool := &sleepingTool{
		echoTool: *newEchoTool("SerialTool", false), // not concurrent
		delay:    30 * time.Millisecond,
	}

	orch, bus, _ := setupOrchestrator(t, allowAllChecker{}, sleepTool)
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "SerialTool", Input: json.RawMessage(`"a"`)},
		{ID: "tc-2", Name: "SerialTool", Input: json.RawMessage(`"b"`)},
	}

	start := time.Now()
	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})
	elapsed := time.Since(start)

	if len(results.Results) != 2 {
		t.Fatalf("results: got %d, want 2", len(results.Results))
	}

	// Serial: should take >= 60ms
	if elapsed < 50*time.Millisecond {
		t.Errorf("serial execution took only %v, expected >=50ms", elapsed)
	}
}

func TestOrchestratorToolNotFound(t *testing.T) {
	orch, bus, _ := setupOrchestrator(t, allowAllChecker{})
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "NonExistent", Input: json.RawMessage(`{}`)},
	}

	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})

	if len(results.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(results.Results))
	}
	if !results.Results[0].IsError {
		t.Error("expected error result for unknown tool")
	}
}

func TestOrchestratorPermissionDenied(t *testing.T) {
	orch, bus, _ := setupOrchestrator(t,
		denyChecker{denyTool: "Bash"},
		newEchoTool("Bash", false),
	)
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{}`)},
	}

	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})

	if !results.Results[0].IsError {
		t.Error("expected error result for denied tool")
	}
	if results.Results[0].Content != "permission denied: denied by test" {
		t.Errorf("Content: got %q", results.Results[0].Content)
	}
}

func TestOrchestratorToolError(t *testing.T) {
	ft := &failingTool{echoTool: *newEchoTool("FailTool", false)}
	orch, bus, _ := setupOrchestrator(t, allowAllChecker{}, ft)
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "FailTool", Input: json.RawMessage(`{}`)},
	}

	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})

	if !results.Results[0].IsError {
		t.Error("expected error result")
	}
	if results.Results[0].Content != "tool execution failed" {
		t.Errorf("Content: got %q", results.Results[0].Content)
	}
}

func TestOrchestratorEmitsEvents(t *testing.T) {
	orch, bus, sub := setupOrchestrator(t, allowAllChecker{}, newEchoTool("Bash", false))

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "Bash", Input: json.RawMessage(`{}`)},
	}

	orch.Execute(context.Background(), calls, staticState{"/tmp"})
	bus.Drain()

	kinds := sub.eventKinds()

	expected := []string{
		"ToolCallReceived",
		"ToolBatchStarted",
		"ToolPermissionChecked",
		"ToolExecutionStarted",
		"ToolExecutionCompleted",
		"ToolBatchCompleted",
	}

	if len(kinds) != len(expected) {
		t.Fatalf("events: got %v, want %v", kinds, expected)
	}
	for i, want := range expected {
		if kinds[i] != want {
			t.Errorf("event[%d]: got %q, want %q", i, kinds[i], want)
		}
	}
}

func TestOrchestratorResultOrder(t *testing.T) {
	orch, bus, _ := setupOrchestrator(t, allowAllChecker{},
		newEchoTool("A", true),
		newEchoTool("B", false),
	)
	defer bus.Drain()

	calls := []model.ToolCallPart{
		{ID: "tc-1", Name: "A", Input: json.RawMessage(`"first"`)},
		{ID: "tc-2", Name: "B", Input: json.RawMessage(`"second"`)},
		{ID: "tc-3", Name: "A", Input: json.RawMessage(`"third"`)},
	}

	results := orch.Execute(context.Background(), calls, staticState{"/tmp"})

	if results.Results[0].ToolCallID != "tc-1" {
		t.Errorf("result[0] ID: got %q", results.Results[0].ToolCallID)
	}
	if results.Results[1].ToolCallID != "tc-2" {
		t.Errorf("result[1] ID: got %q", results.Results[1].ToolCallID)
	}
	if results.Results[2].ToolCallID != "tc-3" {
		t.Errorf("result[2] ID: got %q", results.Results[2].ToolCallID)
	}
}

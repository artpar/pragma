package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
)

// testTeammateProvider returns a single text response on each Stream call.
type testTeammateProvider struct {
	callCount int
}

func (tp *testTeammateProvider) Name() string { return "test-teammate" }
func (tp *testTeammateProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (tp *testTeammateProvider) Pricing(_ string) (model.Pricing, bool) {
	return model.Pricing{}, false
}
func (tp *testTeammateProvider) ContextWindow(_ string) (int, bool) { return 200_000, true }
func (tp *testTeammateProvider) Complete(_ context.Context, p provider.RequestParams) (model.Response, error) {
	ch, err := tp.Stream(context.Background(), p)
	if err != nil {
		return model.Response{}, err
	}
	return provider.AccumulateStream(ch)
}
func (tp *testTeammateProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	tp.callCount++
	ch := make(chan provider.StreamChunk, 2)
	ch <- provider.StreamChunk{TextDelta: "response"}
	ch <- provider.StreamChunk{Done: &provider.StreamDone{
		StopReason: model.StopEndTurn,
		Usage:      model.TokenUsage{InputTokens: 100, OutputTokens: 50},
	}}
	close(ch)
	return ch, nil
}

type allowAllTestChecker struct{}

func (a *allowAllTestChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     &permission.Rule{ToolName: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}
func (a *allowAllTestChecker) AddSessionRule(_ permission.Rule) {}

func TestTeammate_SpawnAndMessage(t *testing.T) {
	bus := observe.NewEventBus(256)
	taskReg := task.NewRegistry(bus)
	prov := &testTeammateProvider{}

	parentStore := app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "test", "test", "/tmp"),
		CWD:          "/tmp",
		Model:        "test",
		Provider:     "test",
		MaxTokens:    4096,
	})

	factory := func(forkedConv model.Conversation, _ []string, _ string) (*query.Engine, *app.StateStore) {
		subStore := app.NewStateStore(app.AppState{
			Conversation: forkedConv,
			CWD:          "/tmp",
			Model:        "test",
			Provider:     "test",
			MaxTokens:    4096,
		})
		reg := tool.NewRegistry(bus)
		checker := &allowAllTestChecker{}
		prompter := &permission.NonInteractivePrompter{}
		orch := tool.NewOrchestrator(reg, checker, prompter, bus)
		ct := model.NewCostTracker()
		engine := query.NewEngine(prov, reg, orch, subStore, ct, bus, query.EngineConfig{
			Model:     "test",
			MaxTokens: 4096,
		})
		return engine, subStore
	}

	agentTool := &Tool{
		EngineFactory: factory,
		Store:         parentStore,
		Tasks:         taskReg,
		Bus:           bus,
	}

	// Invoke as teammate.
	input, _ := json.Marshal(AgentInput{
		Prompt:   "You are a test teammate",
		Teammate: true,
	})
	ctx := context.Background()
	snap := &toolStateSnapshot{store: parentStore}

	result, err := agentTool.Invoke(ctx, input, snap)
	if err != nil {
		t.Fatalf("Invoke error: %v", err)
	}

	// Verify result indicates teammate was launched.
	if result.Content == "" {
		t.Fatal("expected non-empty result content")
	}
	var ar agentResult
	if err := json.Unmarshal([]byte(result.Content), &ar); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if ar.Status != "teammate_launched" {
		t.Errorf("status = %q, want %q", ar.Status, "teammate_launched")
	}
	taskID := ar.TaskID

	// Wait for task to reach idle state (initial prompt processed).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tk, _ := taskReg.Get(taskID)
		if tk.IsIdle {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	tk, _ := taskReg.Get(taskID)
	if !tk.IsIdle {
		t.Fatal("teammate did not reach idle state within timeout")
	}
	if tk.Status != task.TaskRunning {
		t.Errorf("task status = %q, want %q", tk.Status, task.TaskRunning)
	}

	// Send a message to the teammate.
	_ = taskReg.Update(taskID, func(tt *task.Task) {
		tt.PendingMessages = append(tt.PendingMessages, "Hello teammate")
	})
	taskReg.NotifyTask(taskID)

	// Wait for teammate to process the message and become idle again.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tk, _ = taskReg.Get(taskID)
		// After processing, teammate goes non-idle then back to idle.
		if tk.IsIdle && tk.TokensUsed > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	tk, _ = taskReg.Get(taskID)
	if tk.TokensUsed == 0 {
		t.Error("expected cumulative tokens > 0 after processing message")
	}

	// Request shutdown.
	_ = taskReg.Update(taskID, func(tt *task.Task) {
		tt.ShutdownRequested = true
	})
	taskReg.NotifyTask(taskID)

	// Wait for completion.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		tk, _ = taskReg.Get(taskID)
		if tk.Status == task.TaskCompleted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	tk, _ = taskReg.Get(taskID)
	if tk.Status != task.TaskCompleted {
		t.Errorf("task status = %q, want %q after shutdown", tk.Status, task.TaskCompleted)
	}
}

// toolStateSnapshot implements tool.StateSnapshot for testing.
type toolStateSnapshot struct {
	store *app.StateStore
}

func (s *toolStateSnapshot) WorkDir() string { return "/tmp" }

package plan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

func newStore() *app.StateStore {
	return app.NewStateStore(app.AppState{
		Conversation: model.NewConversation(model.SystemPrompt{}, "test", "test", "/tmp"),
		CWD:          "/tmp",
	})
}

func TestEnterPlanMode(t *testing.T) {
	t.Run("name and flags", func(t *testing.T) {
		tool := &EnterTool{Store: newStore()}
		if tool.Name() != "EnterPlanMode" {
			t.Fatalf("expected EnterPlanMode, got %s", tool.Name())
		}
		if !tool.Flags().ReadOnly {
			t.Fatal("expected read-only")
		}
	})

	t.Run("enters plan mode", func(t *testing.T) {
		store := newStore()
		tool := &EnterTool{Store: store}

		result, err := tool.Invoke(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "Entered plan mode. Only read-only tools are now available. Use ExitPlanMode when ready to execute." {
			t.Fatalf("unexpected content: %s", result.Content)
		}

		snap := store.Snapshot()
		if !snap.PlanMode {
			t.Fatal("expected PlanMode to be true")
		}
	})

	t.Run("already in plan mode is idempotent", func(t *testing.T) {
		store := newStore()
		store.Update(func(s *app.AppState) { s.PlanMode = true })
		tool := &EnterTool{Store: store}

		result, err := tool.Invoke(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "Already in plan mode." {
			t.Fatalf("unexpected content: %s", result.Content)
		}
	})

	t.Run("schema is valid JSON", func(t *testing.T) {
		tool := &EnterTool{Store: newStore()}
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema(), &schema); err != nil {
			t.Fatalf("invalid schema JSON: %v", err)
		}
	})
}

func TestExitPlanMode(t *testing.T) {
	t.Run("name and flags", func(t *testing.T) {
		tool := &ExitTool{Store: newStore()}
		if tool.Name() != "ExitPlanMode" {
			t.Fatalf("expected ExitPlanMode, got %s", tool.Name())
		}
		// Must be ReadOnly so it's available in plan mode
		if !tool.Flags().ReadOnly {
			t.Fatal("expected read-only (must be usable in plan mode)")
		}
	})

	t.Run("exits plan mode", func(t *testing.T) {
		store := newStore()
		store.Update(func(s *app.AppState) { s.PlanMode = true })
		tool := &ExitTool{Store: store}

		result, err := tool.Invoke(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "Exited plan mode. All tools are now available." {
			t.Fatalf("unexpected content: %s", result.Content)
		}

		snap := store.Snapshot()
		if snap.PlanMode {
			t.Fatal("expected PlanMode to be false")
		}
	})

	t.Run("not in plan mode is idempotent", func(t *testing.T) {
		store := newStore()
		tool := &ExitTool{Store: store}

		result, err := tool.Invoke(context.Background(), json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "Not in plan mode." {
			t.Fatalf("unexpected content: %s", result.Content)
		}
	})
}

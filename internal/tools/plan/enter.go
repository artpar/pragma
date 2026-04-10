package plan

import (
	"context"
	"encoding/json"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

var enterInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

// EnterTool switches the engine to plan mode where only read-only tools are available.
type EnterTool struct {
	Store *app.StateStore
}

func (t *EnterTool) Name() string                { return "EnterPlanMode" }
func (t *EnterTool) Description() string          { return "Enter plan mode. Only read-only tools will be available until plan mode is exited." }
func (t *EnterTool) InputSchema() json.RawMessage { return enterInputSchema }
func (t *EnterTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *EnterTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "EnterPlanMode", "")
}

func (t *EnterTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	snap := t.Store.Snapshot()
	if snap.PlanMode {
		return tool.InvokeResult{Content: "Already in plan mode."}, nil
	}

	t.Store.Update(func(s *app.AppState) {
		s.PlanMode = true
	})

	return tool.InvokeResult{
		Content: "Entered plan mode. Only read-only tools are now available. Use ExitPlanMode when ready to execute.",
	}, nil
}

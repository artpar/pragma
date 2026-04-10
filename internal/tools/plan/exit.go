package plan

import (
	"context"
	"encoding/json"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

var exitInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {}
}`)

// ExitTool exits plan mode, re-enabling all tools.
type ExitTool struct {
	Store *app.StateStore
}

func (t *ExitTool) Name() string                { return "ExitPlanMode" }
func (t *ExitTool) Description() string          { return "Exit plan mode and re-enable all tools for execution." }
func (t *ExitTool) InputSchema() json.RawMessage { return exitInputSchema }
func (t *ExitTool) Flags() tool.ToolFlags {
	// ReadOnly: true so this tool is available IN plan mode
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *ExitTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "ExitPlanMode", "")
}

func (t *ExitTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	snap := t.Store.Snapshot()
	if !snap.PlanMode {
		return tool.InvokeResult{Content: "Not in plan mode."}, nil
	}

	t.Store.Update(func(s *app.AppState) {
		s.PlanMode = false
	})

	return tool.InvokeResult{
		Content: "Exited plan mode. All tools are now available.",
	}, nil
}

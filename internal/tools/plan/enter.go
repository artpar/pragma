package plan

import (
	"context"
	"encoding/json"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
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

func (t *EnterTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"EnterPlanMode\"")
	return "EnterPlanMode"
}
func (t *EnterTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Enter plan mode. Only read-only tools will be available until plan mode is e...")
	return "Enter plan mode. Only read-only tools will be available until plan mode is exited."
}
func (t *EnterTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: enterInputSchema")
	return enterInputSchema
}
func (t *EnterTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *EnterTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterPlanMode\", \"\")")
	return checker.Check(ctx, "EnterPlanMode", "")
}

func (t *EnterTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := t.Store.Snapshot()
	if snap.PlanMode {
		observe.GlobalTrace("if: snap.PlanMode")
		observe.GlobalTrace("return: tool.InvokeResult{Content: \"Already in plan mode.\"}, nil")
		return tool.InvokeResult{Content: "Already in plan mode."}, nil
	}

	t.Store.Update(func(s *app.AppState) {
		s.PlanMode = true
	})
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: \"Entered plan mode. Only read-only tools are now...")

	return tool.InvokeResult{
		Content: "Entered plan mode. Only read-only tools are now available. Use ExitPlanMode when ready to execute.",
	}, nil
}

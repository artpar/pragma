package plan

import (
	"context"
	"encoding/json"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/observe"
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

func (t *ExitTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"ExitPlanMode\"")
	return "ExitPlanMode"
}
func (t *ExitTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Exit plan mode and re-enable all tools for execution.\"")
	return "Exit plan mode and re-enable all tools for execution."
}
func (t *ExitTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: exitInputSchema")
	return exitInputSchema
}
func (t *ExitTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")

	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *ExitTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "plan", "ExitTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "plan", "ExitTool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "plan", "ExitTool.CheckPerm", "return: checker.Check(ctx, \"ExitPlanMode\", \"\")")
	return checker.Check(ctx, "ExitPlanMode", "")
}

func (t *ExitTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := t.Store.Snapshot()
	if !snap.PlanMode {
		observe.GlobalTrace("if: !snap.PlanMode")
		observe.GlobalTrace("return: tool.InvokeResult{Content: \"Not in plan mode.\"}, nil")
		return tool.InvokeResult{Content: "Not in plan mode."}, nil
	}

	t.Store.Update(func(s *app.AppState) {
		s.PlanMode = false
	})
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: \"Exited plan mode. All tools are now available.\"...")

	return tool.InvokeResult{
		Content: "Exited plan mode. All tools are now available.",
	}, nil
}

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
	observe.GlobalTrace("return: \"EnterPlanMode\"")
	observe.GlobalTrace("return: \"EnterPlanMode\"")
	return "EnterPlanMode"
}
func (t *EnterTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Enter plan mode for non-trivial implementation tasks...\"")
	observe.GlobalTrace("return: enterPlanDescription")
	observe.GlobalTrace("return: enterPlanDescription")
	return enterPlanDescription
}

const enterPlanDescription = `Enter plan mode for non-trivial implementation tasks. Only read-only tools will be available until plan mode is exited. Use this to get user sign-off on your approach before writing code.

When to use:
1. New feature implementation — adding meaningful new functionality
2. Multiple valid approaches — the task can be solved several ways
3. Code modifications — changes affecting existing behavior
4. Architectural decisions — choosing between patterns/technologies
5. Multi-file changes — will likely touch more than 2-3 files
6. Unclear requirements — need to explore before understanding scope

When NOT to use:
- Single-line/few-line fixes (typos, obvious bugs)
- Single function with clear requirements
- Very specific, detailed user instructions
- Pure research/exploration (use Agent instead)`

func (t *EnterTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: enterInputSchema")
	observe.GlobalTrace("return: enterInputSchema")
	observe.GlobalTrace("return: enterInputSchema")
	return enterInputSchema
}
func (t *EnterTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *EnterTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterPlanMode\", \"\")")
	observe.TraceCtx(ctx, "plan", "EnterTool.CheckPerm", "return: checker.Check(ctx, \"EnterPlanMode\", \"\")")
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
		observe.GlobalTrace("return: tool.InvokeResult{Content: \"Already in plan mode.\"}, nil")
		observe.GlobalTrace("return: tool.InvokeResult{Content: \"Already in plan mode.\"}, nil")
		return tool.InvokeResult{Content: "Already in plan mode."}, nil
	}

	t.Store.Update(func(s *app.AppState) {
		s.PlanMode = true
	})
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: \"Entered plan mode. Only read-only tools are now...")
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: \"Entered plan mode. Only read-only tools are now...")
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent: \"Entered plan mode. Only read-only tools are now...")

	return tool.InvokeResult{
		Content: "Entered plan mode. Only read-only tools are now available. Use ExitPlanMode when ready to execute.",
	}, nil
}

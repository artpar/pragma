package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// AgentToolName is the model-facing name of the subagent tool.
const AgentToolName = "Agent"

// agentInput defines the parameters for the Agent tool (the sync subset of
// the branch's AgentInput; background/teammate/worktree/structure are
// follow-up mechanisms).
type agentInput struct {
	Prompt      string `json:"prompt"`
	Description string `json:"description,omitempty"`
}

// subAgentResult is the JSON envelope returned to the parent (the branch's
// agentResult sync fields).
type subAgentResult struct {
	Status     string `json:"status"`
	Prompt     string `json:"prompt"`
	Result     string `json:"result,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}

// agentToolDef returns the model tool definition for the Agent tool
// (SUB-001, sync fork ported from the worktree branch).
func agentToolDef() model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.ToolDef{ ... }")
	observe.GlobalTrace("return: model.ToolDef{\n\tName:\tAgentToolName,\n\tDescription: `Launch a sub-agent that i...")
	return model.ToolDef{
		Name: AgentToolName,
		Description: `Launch a sub-agent that independently performs a bounded task in a fresh conversation.

Give the sub-agent one bounded, independent question or deliverable, not a vague instruction. The sub-agent shares this workspace and provider but keeps its own conversation; it returns its final answer here. Use it to parallelize evidence gathering or isolated investigations without polluting this conversation.

The sub-agent has the same tools as this session (it cannot spawn further sub-agents). Its result arrives as a JSON envelope: {"status":"completed","prompt":...,"result":...,"tokens_used":...}.`,
		InputSchema: json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["prompt"],
	"properties": {
		"prompt": {
			"type": "string",
			"description": "The task for the sub-agent to perform. Make it bounded and independently verifiable."
		},
		"description": {
			"type": "string",
			"description": "A short (3-5 word) description of the task"
		}
	}
}`),
	}
}

// withSubAgentTool appends the Agent tool definition unless this engine is
// itself a sub-agent (recursion guard) or the name is already taken.
func (engine *Engine) withSubAgentTool(tools []model.ToolDef) []model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.config.DisableSubAgents {
		observe.GlobalTrace("if: engine.config.DisableSubAgents")
		observe.GlobalTrace("return: tools")
		return tools
	}
	for _, tool := range tools {
		observe.GlobalTrace("range tools")
		if tool.Name == AgentToolName {
			observe.GlobalTrace("if: tool.Name == AgentToolName")
			observe.GlobalTrace("return: tools")
			return tools
		}
	}
	observe.GlobalTrace("return: append(tools, agentToolDef())")
	return append(tools, agentToolDef())
}

// executeSubAgentTool runs a synchronous sub-agent: a fresh conversation
// sharing the parent's provider, bus, and cost accounting, executing the
// provider-tools loop with the parent's toolset minus the Agent tool
// (the branch's excludeTool(nil, "Agent") scoping). The sub-agent's final
// text is returned to the parent as a paired tool result in the branch's
// JSON envelope.
func (engine *Engine) executeSubAgentTool(ctx context.Context, call model.ToolCallPart) model.ToolResultPart {
	observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "exit")
	var in agentInput
	if err := json.Unmarshal(call.Input, &in); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ ... IsError: true }")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"invalid Agent...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("invalid Agent input: %v", err), IsError: true}
	}
	if strings.TrimSpace(in.Prompt) == "" {
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "if: strings.TrimSpace(in.Prompt) == \"\"")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ ... IsError: true }")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: \"Agent input requires a pr...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: "Agent input requires a prompt", IsError: true}
	}

	sub, _ := engine.ForkFreshConversation()
	sub.config.LoopMode = LoopModeProviderTools
	sub.config.DisableSubAgents = true

	var result strings.Builder
	var usage model.TokenUsage
	failed := false
	for ev := range sub.Run(ctx, in.Prompt) {
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "range sub.Run(ctx, in.Prompt)")
		switch e := ev.(type) {
		case TextEvent:
			observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "typecase: query.TextEvent")
			result.WriteString(e.Text)
		case TurnCompleteEvent:
			observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "typecase: query.TurnCompleteEvent")
			usage.InputTokens += e.Response.Usage.InputTokens
			usage.OutputTokens += e.Response.Usage.OutputTokens
			usage.CacheCreationInputTokens += e.Response.Usage.CacheCreationInputTokens
			usage.CacheReadInputTokens += e.Response.Usage.CacheReadInputTokens
		case ErrorEvent:
			observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "typecase: query.ErrorEvent")
			failed = true
			result.Reset()
			fmt.Fprintf(&result, "Agent failed: %v", e.Err)
		}
	}

	if failed {
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "if: failed")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ ... IsError: true }")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: result.String(), IsError: ...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: result.String(), IsError: true}
	}

	envelope := subAgentResult{
		Status:     "completed",
		Prompt:     in.Prompt,
		Result:     result.String(),
		TokensUsed: usage.InputTokens + usage.OutputTokens,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "if: err != nil")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ ... IsError: true }")
		observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf(\"Agent complet...")
		return model.ToolResultPart{ToolCallID: call.ID, Content: fmt.Sprintf("Agent completed but failed to marshal result: %v", err), IsError: true}
	}
	observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ ... }")
	observe.TraceCtx(ctx, "query", "Engine.executeSubAgentTool", "return: model.ToolResultPart{ToolCallID: call.ID, Content: string(data)}")
	return model.ToolResultPart{ToolCallID: call.ID, Content: string(data)}
}

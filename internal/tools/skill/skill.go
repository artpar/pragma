package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	skillpkg "github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/tool"
	toolagent "github.com/artpar/pragma/internal/tools/agent"
)

// SkillInput defines the parameters for the Skill tool.
type SkillInput struct {
	Skill string `json:"skill" desc:"The skill name to execute"`
	Args  string `json:"args,omitempty" desc:"Optional arguments for the skill"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["skill"],
	"properties": {
		"skill": {
			"type": "string",
			"description": "The skill name to execute"
		},
		"args": {
			"type": "string",
			"description": "Optional arguments for the skill"
		}
	}
}`)

// Tool implements the Skill tool for executing user-defined skills.
type Tool struct {
	Agent  *toolagent.Tool
	Loader skillpkg.Catalog
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Skill\"")
	return "Skill"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: skillDescription")
	return skillDescription
}

const skillDescription = `Execute a skill within the main conversation.

When users ask you to perform tasks, check if any of the available skills match. Skills provide specialized capabilities and domain knowledge.

When users reference a "slash command" or "/<something>" (e.g., "/commit", "/review"), they are referring to a skill. Use this tool to invoke it.

How to invoke:
- Use this tool with the skill name and optional arguments
- Examples:
  - skill: "commit" - invoke the commit skill
  - skill: "review", args: "src/main.go" - invoke with arguments

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, invoke this tool BEFORE generating any other response about the task
- Do not invoke a skill that is already running`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "skill", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "skill", "Tool.CheckPerm", "exit")

	var in struct {
		Skill string `json:"skill"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "skill", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "skill", "Tool.CheckPerm", "return: checker.Check(ctx, \"Skill\", \"\")")
		return checker.Check(ctx, "Skill", "")
	}
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Skill), "/"))
	observe.TraceCtx(ctx, "skill", "Tool.CheckPerm", "return: checker.Check(ctx, \"Skill\", name)")
	return checker.Check(ctx, "Skill", name)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "skill", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "skill", "Tool.Invoke", "exit")

	var in SkillInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Invalid input: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Invalid input: %v", err)}, nil
	}

	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Skill), "/"))
	if name == "" {
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "if: name == \"\"")
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: tool.InvokeResult{Content: \"Skill name is required.\"}, nil")
		return tool.InvokeResult{Content: "Skill name is required."}, nil
	}
	if t.Loader == nil {
		return tool.InvokeResult{Content: "Skills loader is unavailable."}, nil
	}

	s, err := t.Loader.Load(name)
	if err != nil {
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "skill not found: "+name)
		available := t.availableSkillNames()
		if len(available) > 0 {
			observe.TraceCtx(ctx, "skill", "Tool.Invoke", "if: len(available) > 0")
			observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Unknown skill: %q. Available skills: ...")
			return tool.InvokeResult{Content: fmt.Sprintf("Unknown skill: %q. Available skills: %s", name, strings.Join(available, ", "))}, nil
		}
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Unknown skill: %q. No skills are inst...")
		return tool.InvokeResult{Content: fmt.Sprintf("Unknown skill: %q. No skills are installed.", name)}, nil
	}

	content := skillpkg.SubstituteArgs(s.Content, in.Args, s.Arguments, s.BaseDir)

	if s.IsForked() {
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "forked execution")
		observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: t.invokeForked(ctx, s, content)")
		return t.invokeForked(ctx, s, content, state)
	}

	observe.TraceCtx(ctx, "skill", "Tool.Invoke", "inline execution")
	observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")
	return tool.InvokeResult{Content: content}, nil
}

// invokeForked runs the skill as a forked sub-agent.
func (t *Tool) invokeForked(ctx context.Context, s skillpkg.Skill, content string, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "enter")
	defer observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "exit")

	if t.Agent == nil {
		observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "if: t.Agent == nil")
		return tool.InvokeResult{
			Content: fmt.Sprintf("Skill %q failed: agent runner is unavailable", s.Name),
		}, nil
	}

	result, err := t.Agent.RunForked(ctx, content, "skill:"+s.Name, s.Model, s.AllowedTools, state)
	if err != nil {
		observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "if: err != nil")
		return result, err
	}

	resultText := skillAgentResultText(result.Content)
	if resultText == "" {
		observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "if: resultText == \"\"")
		resultText = "(skill produced no output)"
	}
	observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Skill completed with result:\\n\\n%s\"...")

	return tool.InvokeResult{
		Content: fmt.Sprintf("Skill completed with result:\n\n%s", resultText),
	}, nil
}

func skillAgentResultText(content string) string {
	var ar struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(content), &ar); err == nil && ar.Status == "completed" {
		return ar.Result
	}
	return content
}

// availableSkillNames returns a sorted list of all discoverable skill names.
func (t *Tool) availableSkillNames() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if t.Loader == nil {
		return nil
	}
	skills, err := t.Loader.LoadAll()
	if err != nil || len(skills) == 0 {
		observe.GlobalTrace("if: err != nil || len(skills) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	names := make([]string, len(skills))
	for i, s := range skills {
		observe.GlobalTrace("range skills")
		names[i] = s.Name
	}
	observe.GlobalTrace("return: names")
	return names
}

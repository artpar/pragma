package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	skillpkg "github.com/artpar/gogent/internal/skill"
	"github.com/artpar/gogent/internal/tool"
)

// SkillInput defines the parameters for the Skill tool.
type SkillInput struct {
	Skill string `json:"skill" desc:"The skill name to execute"`
	Args  string `json:"args,omitempty" desc:"Optional arguments for the skill"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
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

// EngineFactory creates a sub-Engine for a forked conversation with scoped tools.
type EngineFactory func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore)

// Tool implements the Skill tool for executing user-defined skills.
type Tool struct {
	EngineFactory EngineFactory
	Store         *app.StateStore
	Bus           *observe.EventBus
	Loader        *skillpkg.Loader
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
- When a skill matches the user's request, invoke the relevant Skill tool BEFORE generating any other response about the task
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
		return t.invokeForked(ctx, s, content)
	}

	observe.TraceCtx(ctx, "skill", "Tool.Invoke", "inline execution")
	observe.TraceCtx(ctx, "skill", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")
	return tool.InvokeResult{Content: content}, nil
}

// invokeForked runs the skill as a forked sub-agent.
func (t *Tool) invokeForked(ctx context.Context, s skillpkg.Skill, content string) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "enter")
	defer observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "exit")

	snapshot := t.Store.Snapshot()
	forkedConv := snapshot.Conversation.Fork(model.NewUUID())

	engine, _ := t.EngineFactory(forkedConv, s.AllowedTools, s.Model)

	t.Bus.Emit(observe.SubAgentSpawned{
		EventHeader: observe.NewEventHeader("SubAgentSpawned", "", observe.NewSpanID(), ""),
		SubAgentID:  "skill_" + s.Name,
		AgentName:   "skill:" + s.Name,
		Model:       s.Model,
	})

	events := engine.Run(ctx, content)

	var result strings.Builder
	var firstErr error
	for ev := range events {
		switch e := ev.(type) {
		case query.TextEvent:
			if firstErr == nil {
				result.WriteString(e.Text)
			}
		case query.ErrorEvent:
			if firstErr == nil {
				firstErr = e.Err
				observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "error: "+e.Err.Error())
			}
		}
	}

	if firstErr != nil {
		return tool.InvokeResult{
			Content: fmt.Sprintf("Skill %q failed: %v", s.Name, firstErr),
		}, nil
	}

	resultText := result.String()
	if resultText == "" {
		resultText = "(skill produced no output)"
	}
	observe.TraceCtx(ctx, "skill", "Tool.invokeForked", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Skill completed with result:\\n\\n%s\"...")

	return tool.InvokeResult{
		Content: fmt.Sprintf("Skill completed with result:\n\n%s", resultText),
	}, nil
}

// availableSkillNames returns a sorted list of all discoverable skill names.
func (t *Tool) availableSkillNames() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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

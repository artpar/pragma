package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/lifecycle/definition"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

const graphSpecSystemPrompt = `You are a graph compiler. Given a natural language description of an execution workflow, output a valid YAML lifecycle graph definition. Output ONLY the YAML — no markdown fences, no explanation.

## Node types

- llm: Calls the LLM provider. Reads messages from state, appends assistant response. Sets stop_reason.
  Config: prompt (system prompt override string), temperature (float).
- tools: Executes tool calls from the last assistant message. Appends tool results to messages.
- eval: LLM judges whether the task succeeded. Sets "passed" (bool) and "score" (0.0-1.0).
  Config (inside config: map): criteria (the evaluation question string).
- reflect: LLM self-critiques the conversation trajectory. Appends reflection to messages and "reflections" state key.

## Routers (for conditional_edges)

- stop_reason: Returns "continue" if the LLM wants to call tools, "end" if done.
- pass_fail: Returns "pass" if eval set passed=true, "fail" otherwise.
- field:<key>: Routes on any state key value. Bools become "true"/"false".

## YAML format

graph:
  initial: <first_node_name>
  max_steps: <int, default 50>
  nodes:
    <name>:
      type: <llm|tools|eval|reflect>
      prompt: "optional system prompt override"
      config:
        criteria: "optional eval question"
  edges:
    - from: <source_node>
      to: <target_node>
  conditional_edges:
    - from: <source_node>
      router: <stop_reason|pass_fail|field:keyname>
      paths:
        "<router_output>": <target_node>
        "<router_output>": ""
  reducers:
    messages: messages
    reflections: reflections

Empty string "" in paths means END (terminate the graph).

## Rules

- Every graph MUST have reducers with at least: messages: messages
- If using reflect nodes, add: reflections: reflections
- The llm node and tools node almost always appear together: llm decides what to do, tools executes it
- Use stop_reason router after llm to check if it wants to call tools or is done
- Use pass_fail router after eval to branch on success/failure
- Always set a reasonable max_steps (default 50) to prevent infinite loops

## Examples

### Attempt with self-evaluation and retry
graph:
  initial: actor
  max_steps: 30
  nodes:
    actor:
      type: llm
    tools:
      type: tools
    evaluator:
      type: eval
      config:
        criteria: "Did the task complete successfully? Check for errors."
    critic:
      type: reflect
  edges:
    - from: tools
      to: actor
    - from: critic
      to: actor
  conditional_edges:
    - from: actor
      router: stop_reason
      paths:
        continue: tools
        end: evaluator
    - from: evaluator
      router: pass_fail
      paths:
        pass: ""
        fail: critic
  reducers:
    messages: messages
    reflections: reflections

### Plan then execute with verification
graph:
  initial: planner
  max_steps: 40
  nodes:
    planner:
      type: llm
      prompt: "Create a detailed step-by-step plan for the task."
    executor:
      type: tools
    verifier:
      type: llm
      prompt: "Review execution results. If complete, summarize. If not, call tools to continue."
  edges:
    - from: planner
      to: executor
    - from: executor
      to: verifier
  conditional_edges:
    - from: verifier
      router: stop_reason
      paths:
        continue: executor
        end: ""
  reducers:
    messages: messages

### Simple tool-calling loop
graph:
  initial: agent
  max_steps: 50
  nodes:
    agent:
      type: llm
    tools:
      type: tools
  edges:
    - from: tools
      to: agent
  conditional_edges:
    - from: agent
      router: stop_reason
      paths:
        continue: tools
        end: ""
  reducers:
    messages: messages`

// GenerateGraph calls the LLM to compile a natural language structure description
// into a YAML lifecycle graph definition. Uses provider.Complete with the
// graph-specification system prompt.
func GenerateGraph(ctx context.Context, prov provider.Provider, bus *observe.EventBus, modelID string, description string) (*definition.GraphDef, error) {
	observe.TraceCtx(ctx, "bridge", "GenerateGraph", "enter")
	defer observe.TraceCtx(ctx, "bridge", "GenerateGraph", "exit")

	messages := []model.Message{
		{
			ID:   model.NewUUID(),
			Role: model.RoleUser,
			Content: []model.ContentPart{model.TextPart{Text: fmt.Sprintf(
				"Generate a YAML lifecycle graph for this execution structure:\n\n%s", description,
			)}},
		},
	}

	resp, err := prov.Complete(ctx, provider.RequestParams{
		Model:     modelID,
		MaxTokens: 4096,
		Messages:  messages,
		System: model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: graphSpecSystemPrompt}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("graph generation LLM call: %w", err)
	}

	var yamlText string
	for _, part := range resp.Content {
		if tp, ok := part.(model.TextPart); ok {
			yamlText += tp.Text
		}
	}

	yamlText = stripMarkdownFences(yamlText)
	yamlText = strings.TrimSpace(yamlText)

	if yamlText == "" {
		return nil, fmt.Errorf("graph generation returned empty response")
	}

	def, err := definition.Parse([]byte(yamlText))
	if err != nil {
		return nil, fmt.Errorf("generated graph is invalid YAML: %w\n\nGenerated:\n%s", err, yamlText)
	}

	return def, nil
}

// stripMarkdownFences removes ```yaml ... ``` wrapping if present.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			s = s[:idx]
		}
	}
	return s
}

package bridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/lifecycle/definition"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

const graphSpecSystemPrompt = `You are a graph compiler. Given a natural language description of an execution workflow, output a valid YAML lifecycle graph definition. Output ONLY the YAML — no markdown fences, no explanation.

## Node types

- llm: Calls the LLM provider. Reads messages from state, appends assistant response. Sets stop_reason.
  Config: prompt (additional instruction prepended to the system prompt), temperature (float).
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
- CRITICAL: Regular edges (under "edges:") MUST have a real node name in "to". NEVER use to: "" in regular edges. Use "" for END ONLY inside conditional_edges paths
- CRITICAL: Any task that reads files, writes files, runs commands, or uses tools MUST include a "tools" node. The "tools" node is the ONLY way to execute tool calls from an llm node. An llm node without a following tools node cannot interact with the outside world
- After an llm node, ALWAYS add a stop_reason conditional edge to route to tools (if tools requested) vs the next step (if done)

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

### Build, test, and fix loop
graph:
  initial: implementer
  max_steps: 50
  nodes:
    implementer:
      type: llm
      prompt: "Implement the next component. Write code files, then run go build to check for errors."
    tools:
      type: tools
    evaluator:
      type: eval
      config:
        criteria: "Did the build/tests pass? Check the command output for errors."
    fixer:
      type: reflect
  edges:
    - from: tools
      to: implementer
    - from: fixer
      to: implementer
  conditional_edges:
    - from: implementer
      router: stop_reason
      paths:
        continue: tools
        end: evaluator
    - from: evaluator
      router: pass_fail
      paths:
        pass: ""
        fail: fixer
  reducers:
    messages: messages
    reflections: reflections

### Parallel analysis with merge
graph:
  initial: analyst_security
  max_steps: 30
  nodes:
    analyst_security:
      type: llm
      prompt: "Analyze the code from a security perspective. Identify vulnerabilities."
    analyst_performance:
      type: llm
      prompt: "Analyze the code from a performance perspective. Identify bottlenecks."
    merger:
      type: llm
      prompt: "Merge the security and performance findings into a unified report with prioritized recommendations."
  edges:
    - from: analyst_security
      to: merger
    - from: analyst_performance
      to: merger
  reducers:
    messages: messages`

const maxGenerateRetries = 2

// GenerateGraph calls the LLM to compile a natural language structure description
// into a YAML lifecycle graph definition. Retries up to maxGenerateRetries times
// with error feedback if generation fails.
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

	sys := model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: graphSpecSystemPrompt}},
	}

	var lastErr error
	for attempt := range maxGenerateRetries + 1 {
		observe.TraceCtx(ctx, "bridge", "GenerateGraph", "range maxGenerateRetries + 1")
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "bridge", "GenerateGraph", "if: ctx.Err() != nil")
			observe.TraceCtx(ctx, "bridge", "GenerateGraph", "return: nil, ctx.Err()")
			return nil, ctx.Err()
		}

		resp, err := prov.Complete(ctx, provider.RequestParams{
			Model:     modelID,
			MaxTokens: 4096,
			Messages:  messages,
			System:    sys,
		})
		if err != nil {
			observe.TraceCtx(ctx, "bridge", "GenerateGraph", fmt.Sprintf("attempt %d: LLM call error", attempt))
			lastErr = fmt.Errorf("graph generation LLM call: %w", err)
			continue
		}

		yamlText := extractYAML(resp)

		if yamlText == "" {
			observe.TraceCtx(ctx, "bridge", "GenerateGraph", fmt.Sprintf("attempt %d: empty response", attempt))
			lastErr = fmt.Errorf("graph generation returned empty response")
			messages = appendRetryFeedback(messages, resp, "Your response was empty. Output ONLY the YAML graph definition starting with 'graph:'. No explanation, no markdown fences.")
			continue
		}

		def, parseErr := definition.Parse([]byte(yamlText))
		if parseErr != nil {
			observe.TraceCtx(ctx, "bridge", "GenerateGraph", fmt.Sprintf("attempt %d: parse error", attempt))
			lastErr = fmt.Errorf("generated graph is invalid YAML: %w\n\nGenerated:\n%s", parseErr, yamlText)
			messages = appendRetryFeedback(messages, resp, fmt.Sprintf(
				"Your YAML graph had a parse error: %s\n\nFix the YAML and output ONLY the corrected graph definition starting with 'graph:'. No explanation.", parseErr))
			continue
		}

		definition.FixLLMToolRouting(def)
		observe.TraceCtx(ctx, "bridge", "GenerateGraph", "return: def, nil")
		return def, nil
	}
	observe.TraceCtx(ctx, "bridge", "GenerateGraph", "return: nil, fmt.Errorf(\"graph generation failed after %d attempts: %w\", maxGenerateR...")

	return nil, fmt.Errorf("graph generation failed after %d attempts: %w", maxGenerateRetries+1, lastErr)
}

// extractYAML pulls YAML graph content from an LLM response, handling:
// - Raw YAML output (ideal)
// - Markdown-fenced YAML (```yaml ... ```)
// - YAML buried in explanatory text (searches for "graph:" keyword)
// - Thinking-only responses (no text parts)
func extractYAML(resp model.Response) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var raw string
	for _, part := range resp.Content {
		observe.GlobalTrace("range resp.Content")
		if tp, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			raw += tp.Text
		}
	}

	stripped := stripMarkdownFences(raw)
	stripped = strings.TrimSpace(stripped)
	if stripped != "" && strings.Contains(stripped, "graph:") {
		observe.GlobalTrace("if: stripped != \"\" && strings.Contains(stripped, \"graph:\")")
		observe.GlobalTrace("return: stripped")
		return stripped
	}

	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, "graph:"); idx >= 0 {
		observe.GlobalTrace("if: idx >= 0")
		candidate := strings.TrimSpace(raw[idx:])

		if endIdx := strings.Index(candidate, "\n```"); endIdx >= 0 {
			observe.GlobalTrace("if: endIdx >= 0")
			candidate = strings.TrimSpace(candidate[:endIdx])
		}
		observe.GlobalTrace("return: candidate")
		return candidate
	}
	observe.GlobalTrace("return: stripped")

	return stripped
}

// appendRetryFeedback adds the model's failed response and an error correction
// message to the conversation for the next attempt.
func appendRetryFeedback(messages []model.Message, resp model.Response, feedback string) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	messages = append(messages, model.Message{
		ID:      model.NewUUID(),
		Role:    model.RoleAssistant,
		Content: resp.Content,
	})

	messages = append(messages, model.Message{
		ID:      model.NewUUID(),
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: feedback}},
	})
	observe.GlobalTrace("return: messages")
	return messages
}

// stripMarkdownFences removes ```yaml ... ``` wrapping if present.
// Handles leading prose before fences (e.g., "Here is the YAML:\n```yaml\n...").
func stripMarkdownFences(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		observe.GlobalTrace("if: !strings.HasPrefix(s, \"```\")")
		if idx := strings.Index(s, "```"); idx >= 0 {
			observe.GlobalTrace("if: idx >= 0")
			s = s[idx:]
		}
	}
	if strings.HasPrefix(s, "```") {
		observe.GlobalTrace("if: strings.HasPrefix(s, \"```\")")
		if idx := strings.Index(s, "\n"); idx >= 0 {
			observe.GlobalTrace("if: idx >= 0")
			s = s[idx+1:]
		}
		if idx := strings.LastIndex(s, "```"); idx >= 0 {
			observe.GlobalTrace("if: idx >= 0")
			s = s[:idx]
		}
	}
	observe.GlobalTrace("return: s")
	return s
}

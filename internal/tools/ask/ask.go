package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

type askOption struct {
	Label       string `json:"label" desc:"Display text for this option (1-5 words)"`
	Description string `json:"description,omitempty" desc:"Explanation of what this option means"`
}

type askQuestion struct {
	Question    string      `json:"question" desc:"The question to ask the user"`
	Header      string      `json:"header,omitempty" desc:"Short chip/tag label (max 12 chars)"`
	Options     []askOption `json:"options,omitempty" desc:"Available choices (2-4 options)"`
	MultiSelect bool        `json:"multiSelect,omitempty" desc:"Allow multiple selections"`
}

type askInput struct {
	Question  string        `json:"question,omitempty" desc:"Simple question text (use this OR questions array)"`
	Questions []askQuestion `json:"questions,omitempty" desc:"Structured questions with selectable options (1-4 questions)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"properties": {
		"question": {
			"type": "string",
			"description": "Simple question text. Use this for a single free-text question, OR use the questions array for structured multi-choice questions."
		},
		"questions": {
			"type": "array",
			"description": "Structured questions with selectable options (1-4 questions). Use this instead of 'question' when you want to offer choices.",
			"minItems": 1,
			"maxItems": 4,
			"items": {
				"type": "object",
	"additionalProperties": false,
				"required": ["question", "header", "options"],
				"properties": {
					"question": {
						"type": "string",
						"description": "The question to ask. Should be clear and end with a question mark."
					},
					"header": {
						"type": "string",
						"description": "Very short label displayed as a chip/tag (max 12 chars). Examples: 'Auth method', 'Library', 'Approach'."
					},
					"options": {
						"type": "array",
						"description": "Available choices (2-4 options). An 'Other' free-text option is always added automatically.",
						"minItems": 2,
						"maxItems": 4,
						"items": {
							"type": "object",
	"additionalProperties": false,
							"required": ["label"],
							"properties": {
								"label": {
									"type": "string",
									"description": "Display text (1-5 words). Must be unique within the question."
								},
								"description": {
									"type": "string",
									"description": "Explanation of what this option means or what will happen if chosen."
								}
							}
						}
					},
					"multiSelect": {
						"type": "boolean",
						"description": "Set to true to allow multiple selections. Default false."
					}
				}
			}
		}
	}
}`)

// Tool pauses execution and asks the user a question, returning their answer.
type Tool struct {
	Asker tool.Asker
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"AskUserQuestion\"")
	return "AskUserQuestion"
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: askDescription")
	return askDescription
}

const askDescription = `Asks the user a question to gather information, clarify ambiguity, or get decisions.

Use this tool when you need to:
1. Gather user preferences or requirements
2. Clarify ambiguous instructions
3. Get decisions on implementation choices as you work
4. Offer choices to the user about what direction to take

You can ask simple free-text questions using the 'question' parameter, or structured
multi-choice questions using the 'questions' array with selectable options.

For structured questions:
- Each question needs a short header (max 12 chars) shown as a tab label
- Provide 2-4 options per question with clear labels and descriptions
- An "Other" free-text option is always added automatically
- If you recommend a specific option, make that the first option
- Use multiSelect: true when choices are not mutually exclusive

Usage notes:
- Use this tool sparingly — prefer making reasonable decisions autonomously
- In plan mode, use this tool to clarify requirements BEFORE finalizing your plan. Do NOT use this tool to ask "Is my plan ready?" — use ExitPlanMode for plan approval.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "ask", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "ask", "Tool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "ask", "Tool.CheckPerm", "return: checker.Check(ctx, \"AskUserQuestion\", \"\")")
	return checker.Check(ctx, "AskUserQuestion", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "ask", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "ask", "Tool.Invoke", "exit")

	var in askInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}

	// Build the request from structured or legacy input
	var req tool.AskRequest
	if len(in.Questions) > 0 {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "if: len(in.Questions) > 0")
		req.Questions = make([]tool.AskQuestion, len(in.Questions))
		for i, q := range in.Questions {
			observe.TraceCtx(ctx, "ask", "Tool.Invoke", "range in.Questions")
			req.Questions[i] = tool.AskQuestion{
				Question:    q.Question,
				Header:      q.Header,
				MultiSelect: q.MultiSelect,
			}
			for _, opt := range q.Options {
				observe.TraceCtx(ctx, "ask", "Tool.Invoke", "range q.Options")
				req.Questions[i].Options = append(req.Questions[i].Options, tool.AskOption{
					Label:       opt.Label,
					Description: opt.Description,
				})
			}
		}
	} else if in.Question != "" {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "else-if: in.Question != \"\"")
		req.Question = in.Question
	} else {
		return tool.InvokeResult{}, fmt.Errorf("either 'question' or 'questions' is required")
	}

	resp, err := t.Asker.Ask(ctx, req)
	if err != nil {
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"ask user: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("ask user: %w", err)
	}

	content := formatResponse(req, resp)
	observe.TraceCtx(ctx, "ask", "Tool.Invoke", "return: tool.InvokeResult{Content: content}, nil")
	return tool.InvokeResult{Content: content}, nil
}

// formatResponse formats the ask response for the LLM.
// Plain-text questions return the answer directly.
// Structured questions return the TS-compatible format:
// "User has answered your questions: "Q1"="A1", "Q2"="A2". You can now continue..."
func formatResponse(req tool.AskRequest, resp tool.AskResponse) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if len(req.Questions) == 0 {
		observe.GlobalTrace("if: len(req.Questions) == 0")
		for _, v := range resp.Answers {
			observe.GlobalTrace("range resp.Answers")
			observe.GlobalTrace("return: v")
			return v
		}
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	// Structured: format matching TS mapToolResultToToolResultBlockParam
	var parts []string
	for _, q := range req.Questions {
		observe.GlobalTrace("range req.Questions")
		answer := resp.Answers[q.Question]
		if answer == "" {
			observe.GlobalTrace("if: answer == \"\"")
			continue
		}
		parts = append(parts, fmt.Sprintf("%q=%q", q.Question, answer))
	}
	if len(parts) == 0 {
		observe.GlobalTrace("if: len(parts) == 0")
		observe.GlobalTrace("return: \"User did not answer the questions.\"")
		return "User did not answer the questions."
	}
	observe.GlobalTrace("return: \"User has answered your questions: \" + strings.Join(parts, \", \") + \". You can...")
	return "User has answered your questions: " + strings.Join(parts, ", ") + ". You can now continue with the user's answers in mind."
}

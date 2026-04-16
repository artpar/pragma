package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// EvalNodeConfig configures an evaluation node.
type EvalNodeConfig struct {
	// Criteria is the evaluation criteria prompt sent to the LLM.
	Criteria string
	// Model overrides the model from state if non-empty.
	Model string
}

// evalResponse is the expected JSON structure from the evaluator LLM.
type evalResponse struct {
	Passed bool    `json:"passed"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

// EvalNode returns a NodeFunc that calls an LLM to evaluate the current conversation.
// The provider and bus are captured in the closure.
//
// Reads: messages, system, model_id, max_tokens
// Writes: passed, score
func EvalNode(prov provider.Provider, bus *observe.EventBus, cfg EvalNodeConfig) lifecycle.NodeFunc {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, erro...")
	return func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, error) {
		msgs := Messages(state)
		modelID := ModelID(state)
		maxTokens := MaxTokens(state)

		if cfg.Model != "" {
			modelID = cfg.Model
		}

		// Build evaluation prompt with conversation trajectory
		var trajectory strings.Builder
		for _, msg := range msgs {
			fmt.Fprintf(&trajectory, "[%s]: ", msg.Role)
			for _, part := range msg.Content {
				switch p := part.(type) {
				case model.TextPart:
					trajectory.WriteString(p.Text)
				case model.ToolCallPart:
					fmt.Fprintf(&trajectory, "<tool_call name=%q>", p.Name)
				case model.ToolResultPart:
					fmt.Fprintf(&trajectory, "<tool_result>%s</tool_result>", p.Content)
				}
			}
			trajectory.WriteString("\n")
		}

		evalPrompt := fmt.Sprintf(
			"Evaluate the following conversation against the criteria below.\n\n"+
				"## Criteria\n%s\n\n"+
				"## Conversation\n%s\n\n"+
				"Respond with JSON: {\"passed\": bool, \"score\": 0.0-1.0, \"reason\": \"...\"}",
			cfg.Criteria, trajectory.String(),
		)

		evalMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleUser,
			Content:   []model.ContentPart{model.TextPart{Text: evalPrompt}},
			Timestamp: time.Now(),
		}

		params := provider.RequestParams{
			Model:     modelID,
			MaxTokens: maxTokens,
			Messages:  []model.Message{evalMsg},
			System: model.SystemPrompt{
				Blocks: []model.SystemBlock{{
					Text: "You are an evaluation agent. Respond only with JSON.",
				}},
			},
		}

		resp, err := prov.Complete(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("eval node: %w", err)
		}

		// Extract text from response and parse JSON
		var respText string
		for _, part := range resp.Content {
			if tp, ok := part.(model.TextPart); ok {
				respText = tp.Text
				break
			}
		}

		respText = stripMarkdownFences(respText)
		respText = strings.TrimSpace(respText)
		if idx := strings.Index(respText, "{"); idx > 0 {
			respText = respText[idx:]
		}
		if idx := strings.LastIndex(respText, "}"); idx >= 0 {
			respText = respText[:idx+1]
		}

		var evalResp evalResponse
		if err := json.Unmarshal([]byte(respText), &evalResp); err != nil {

			if bus != nil {
				bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
					Severity:     "warning",
					Component:    "lifecycle/bridge",
					ErrorType:    "eval_parse",
					ErrorMessage: fmt.Sprintf("eval node: failed to parse LLM response as JSON: %v", err),
				})
			}
			return lifecycle.StateUpdate{
				KeyPassed:     false,
				KeyScore:      0.0,
				KeyTotalUsage: resp.Usage,
			}, nil
		}

		return lifecycle.StateUpdate{
			KeyPassed:     evalResp.Passed,
			KeyScore:      evalResp.Score,
			KeyTotalUsage: resp.Usage,
		}, nil
	}
}

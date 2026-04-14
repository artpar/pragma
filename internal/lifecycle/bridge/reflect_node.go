package bridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// ReflectNode returns a NodeFunc that calls an LLM to self-critique the trajectory.
// The provider and bus are captured in the closure.
//
// Reads: messages, reflections, model_id, max_tokens
// Writes: reflections (appends new reflection), messages (appends reflection as user message)
func ReflectNode(prov provider.Provider, bus *observe.EventBus) lifecycle.NodeFunc {
	return func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, error) {
		msgs := Messages(state)
		priorReflections := Reflections(state)
		modelID := ModelID(state)
		maxTokens := MaxTokens(state)

		// Build trajectory summary
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

		var priorBlock string
		if len(priorReflections) > 0 {
			priorBlock = "\n\n## Prior Reflections\n" + strings.Join(priorReflections, "\n---\n")
		}

		reflectPrompt := fmt.Sprintf(
			"Reflect on the following conversation trajectory. "+
				"Identify what went well, what went wrong, and what to try differently next time.\n\n"+
				"## Trajectory\n%s%s\n\n"+
				"Provide a concise self-critique.",
			trajectory.String(), priorBlock,
		)

		reflectMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleUser,
			Content:   []model.ContentPart{model.TextPart{Text: reflectPrompt}},
			Timestamp: time.Now(),
		}

		params := provider.RequestParams{
			Model:     modelID,
			MaxTokens: maxTokens,
			Messages:  []model.Message{reflectMsg},
			System: model.SystemPrompt{
				Blocks: []model.SystemBlock{{
					Text: "You are a self-reflection agent. Provide concise, actionable critiques.",
				}},
			},
		}

		resp, err := prov.Complete(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("reflect node: %w", err)
		}

		var reflectionText string
		for _, part := range resp.Content {
			if tp, ok := part.(model.TextPart); ok {
				reflectionText = tp.Text
				break
			}
		}

		// Inject the reflection into the conversation as a user message
		injectionMsg := model.Message{
			ID:   model.NewUUID(),
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: fmt.Sprintf("[Reflection]: %s", reflectionText)},
			},
			Timestamp: time.Now(),
		}

		return lifecycle.StateUpdate{
			KeyReflections: []string{reflectionText},
			KeyMessages:    []model.Message{injectionMsg},
			KeyTotalUsage:  resp.Usage,
		}, nil
	}
}

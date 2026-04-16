package bridge

import (
	"context"
	"time"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
)

// LLMNodeConfig configures an LLM call node.
type LLMNodeConfig struct {
	// NodePrompt is prepended as the first system block before the main system prompt.
	// This gives the node a specific instruction while preserving the full context
	// (cwd, tool descriptions, AGENT.md) from the parent agent.
	NodePrompt string
	// PromptPrefix is prepended as a user message before calling the LLM.
	PromptPrefix string
	// Temperature overrides the default temperature if non-nil.
	Temperature *float64
}

// LLMNode returns a NodeFunc that calls provider.Complete() to get an LLM response.
// The provider and bus are captured in the closure — not stored in state.
//
// Reads: messages, system, model_id, max_tokens, tools
// Writes: messages (appends assistant message), stop_reason, response, turn_count (+1)
func LLMNode(prov provider.Provider, bus *observe.EventBus, cfg LLMNodeConfig) lifecycle.NodeFunc {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, erro...")
	return func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, error) {
		_ = bus
		msgs := Messages(state)
		sys := System(state)
		modelID := ModelID(state)
		maxTokens := MaxTokens(state)
		tools := Tools(state)

		if cfg.NodePrompt != "" {
			prepended := []model.SystemBlock{{Text: cfg.NodePrompt}}
			prepended = append(prepended, sys.Blocks...)
			sys = model.SystemPrompt{Blocks: prepended}
		}

		if cfg.PromptPrefix != "" {
			prefixMsg := model.Message{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: cfg.PromptPrefix}},
				Timestamp: time.Now(),
			}

			extended := make([]model.Message, len(msgs)+1)
			copy(extended, msgs)
			extended[len(msgs)] = prefixMsg
			msgs = extended
		}

		params := provider.RequestParams{
			Model:       modelID,
			MaxTokens:   maxTokens,
			Messages:    msgs,
			System:      sys,
			Tools:       tools,
			Temperature: cfg.Temperature,
		}

		resp, err := prov.Complete(ctx, params)
		if err != nil {
			return nil, err
		}

		assistantMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleAssistant,
			Content:   resp.Content,
			Timestamp: time.Now(),
		}

		return lifecycle.StateUpdate{
			KeyMessages:   []model.Message{assistantMsg},
			KeyStopReason: string(resp.StopReason),
			KeyResponse:   resp,
			KeyTurnCount:  1,
			KeyTotalUsage: resp.Usage,
		}, nil
	}
}

package bridge

import (
	"context"
	"time"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

// simpleSnapshot implements tool.StateSnapshot with a fixed working directory.
type simpleSnapshot struct {
	cwd string
}

func (s simpleSnapshot) WorkDir() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: s.cwd")
	return s.cwd
}

// ToolNode returns a NodeFunc that executes tool calls from the last assistant message.
// The orchestrator and cwd are captured in the closure.
//
// Reads: messages (extracts ToolCallParts from last assistant message)
// Writes: messages (appends user message with tool results + supplements)
func ToolNode(orch *tool.Orchestrator, cwd string) lifecycle.NodeFunc {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, erro...")
	return func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, error) {
		msgs := Messages(state)
		if len(msgs) == 0 {
			return nil, nil
		}

		lastMsg := msgs[len(msgs)-1]
		if lastMsg.Role != model.RoleAssistant {
			return nil, nil
		}

		var toolCalls []model.ToolCallPart
		for _, part := range lastMsg.Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				toolCalls = append(toolCalls, tc)
			}
		}
		if len(toolCalls) == 0 {
			return nil, nil
		}

		result := orch.Execute(ctx, toolCalls, simpleSnapshot{cwd: cwd})

		var content []model.ContentPart
		for _, r := range result.Results {
			content = append(content, r)
		}
		for _, s := range result.Supplements {
			content = append(content, s)
		}

		resultMsg := model.Message{
			ID:        model.NewUUID(),
			Role:      model.RoleUser,
			Content:   content,
			Timestamp: time.Now(),
		}

		return lifecycle.StateUpdate{
			KeyMessages: []model.Message{resultMsg},
		}, nil
	}
}

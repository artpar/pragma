package bridge

import (
	"context"
	"fmt"
	"time"

	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

// simpleSnapshot implements tool.StateSnapshot with a fixed working directory.
type simpleSnapshot struct {
	cwd       string
	fileState *tool.FileStateCache
}

func (s simpleSnapshot) WorkDir() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: s.cwd")
	return s.cwd
}

func (s simpleSnapshot) ReadFileState() *tool.FileStateCache {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: s.fileState")
	return s.fileState
}

// ToolNode returns a NodeFunc that executes tool calls from the last assistant message.
// The orchestrator and cwd are captured in the closure.
//
// Reads: messages (extracts ToolCallParts from last assistant message)
// Writes: messages (appends user message with tool results + supplements)
func ToolNode(orch *tool.Orchestrator, cwd string, allowedToolNames ...[]string) lifecycle.NodeFunc {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fileState := tool.NewFileStateCache()
	var allowedTools map[string]struct{}
	if len(allowedToolNames) > 0 && len(allowedToolNames[0]) > 0 {
		allowedTools = make(map[string]struct{}, len(allowedToolNames[0]))
		for _, name := range allowedToolNames[0] {
			allowedTools[name] = struct{}{}
		}
	}
	observe.GlobalTrace("return: func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, erro...")
	return func(ctx context.Context, state lifecycle.State) (lifecycle.StateUpdate, error) {
		msgs := Messages(state)
		if len(msgs) == 0 {
			return nil, fmt.Errorf("tools node requires at least one message")
		}

		lastMsg := msgs[len(msgs)-1]
		if lastMsg.Role != model.RoleAssistant {
			return nil, fmt.Errorf("tools node requires last message to be assistant, got %q", lastMsg.Role)
		}

		var toolCalls []model.ToolCallPart
		for _, part := range lastMsg.Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				toolCalls = append(toolCalls, tc)
			}
		}
		if len(toolCalls) == 0 {
			return nil, fmt.Errorf("tools node requires assistant tool calls")
		}

		resultParts, supplements := executeAllowedToolCalls(ctx, orch, cwd, fileState, toolCalls, allowedTools)

		var content []model.ContentPart
		for _, r := range resultParts {
			content = append(content, r)
		}
		for _, s := range supplements {
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

func executeAllowedToolCalls(ctx context.Context, orch *tool.Orchestrator, cwd string, fileState *tool.FileStateCache, toolCalls []model.ToolCallPart, allowedTools map[string]struct{}) ([]model.ToolResultPart, []model.ContentPart) {
	if len(allowedTools) == 0 {
		result := orch.Execute(ctx, toolCalls, simpleSnapshot{cwd: cwd, fileState: fileState})
		return result.Results, result.Supplements
	}

	results := make([]model.ToolResultPart, len(toolCalls))
	allowedCalls := make([]model.ToolCallPart, 0, len(toolCalls))
	allowedIndexes := make([]int, 0, len(toolCalls))
	for i, call := range toolCalls {
		if _, ok := allowedTools[call.Name]; !ok {
			results[i] = model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    fmt.Sprintf("tool %q is not allowed in this lifecycle node", call.Name),
				IsError:    true,
			}
			continue
		}
		allowedCalls = append(allowedCalls, call)
		allowedIndexes = append(allowedIndexes, i)
	}
	if len(allowedCalls) == 0 {
		return results, nil
	}

	result := orch.Execute(ctx, allowedCalls, simpleSnapshot{cwd: cwd, fileState: fileState})
	for i, part := range result.Results {
		results[allowedIndexes[i]] = part
	}
	return results, result.Supplements
}

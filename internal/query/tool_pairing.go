package query

import (
	"fmt"

	"github.com/artpar/pragma/internal/model"
)

func validateToolResultPairing(messages []model.Message) error {
	for i, msg := range messages {
		switch msg.Role {
		case model.RoleAssistant:
			callIDs := toolCallIDs(msg)
			if len(callIDs) == 0 {
				continue
			}
			if i+1 >= len(messages) || messages[i+1].Role != model.RoleUser {
				return fmt.Errorf("assistant tool call message %d has no following user tool result message", i)
			}
			if err := validateResultMessage(i+1, messages[i+1], callIDs); err != nil {
				return err
			}
		case model.RoleUser:
			if !messageHasToolResult(msg) {
				continue
			}
			if i == 0 || messages[i-1].Role != model.RoleAssistant || len(toolCallIDs(messages[i-1])) == 0 {
				return fmt.Errorf("user tool result message %d has no preceding assistant tool call message", i)
			}
		}
	}
	return nil
}

func validateResultMessage(index int, msg model.Message, callIDs map[string]bool) error {
	resultIDs := make(map[string]bool)
	for _, part := range msg.Content {
		result, ok := part.(model.ToolResultPart)
		if !ok {
			continue
		}
		if !callIDs[result.ToolCallID] {
			return fmt.Errorf("user tool result message %d has orphaned result for tool call %q", index, result.ToolCallID)
		}
		resultIDs[result.ToolCallID] = true
	}
	for id := range callIDs {
		if !resultIDs[id] {
			return fmt.Errorf("assistant tool call %q has no matching tool result in user message %d", id, index)
		}
	}
	return nil
}

func toolCallIDs(msg model.Message) map[string]bool {
	ids := make(map[string]bool)
	for _, part := range msg.Content {
		if call, ok := part.(model.ToolCallPart); ok {
			ids[call.ID] = true
		}
	}
	return ids
}

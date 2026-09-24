package query

import (
	"fmt"

	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

func validateToolResultPairing(messages []model.Message) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for i, msg := range messages {
		observe.GlobalTrace("range messages")
		switch msg.Role {
		case model.RoleAssistant:
			observe.GlobalTrace("case: model.RoleAssistant")
			callIDs := toolCallIDs(msg)
			if len(callIDs) == 0 {
				continue
			}
			if i+1 >= len(messages) || messages[i+1].Role != model.RoleUser {
				observe.GlobalTrace("return: fmt.Errorf(\"assistant tool call message %d has no following user tool result ...")
				return fmt.Errorf("assistant tool call message %d has no following user tool result message", i)
			}
			if err := validateResultMessage(i+1, messages[i+1], callIDs); err != nil {
				observe.GlobalTrace("return: err")
				return err
			}
		case model.RoleUser:
			observe.GlobalTrace("case: model.RoleUser")
			if !compact.MessageHasToolResult(msg) {
				continue
			}
			if i == 0 || messages[i-1].Role != model.RoleAssistant || len(toolCallIDs(messages[i-1])) == 0 {
				observe.GlobalTrace("return: fmt.Errorf(\"user tool result message %d has no preceding assistant tool call ...")
				return fmt.Errorf("user tool result message %d has no preceding assistant tool call message", i)
			}
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validateResultMessage(index int, msg model.Message, callIDs map[string]bool) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	resultIDs := make(map[string]bool)
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		result, ok := part.(model.ToolResultPart)
		if !ok {
			observe.GlobalTrace("if: !ok")
			continue
		}
		if !callIDs[result.ToolCallID] {
			observe.GlobalTrace("if: !callIDs[result.ToolCallID]")
			observe.GlobalTrace("return: fmt.Errorf(\"user tool result message %d has orphaned result for tool call %q\"...")
			return fmt.Errorf("user tool result message %d has orphaned result for tool call %q", index, result.ToolCallID)
		}
		resultIDs[result.ToolCallID] = true
	}
	for id := range callIDs {
		observe.GlobalTrace("range callIDs")
		if !resultIDs[id] {
			observe.GlobalTrace("if: !resultIDs[id]")
			observe.GlobalTrace("return: fmt.Errorf(\"assistant tool call %q has no matching tool result in user messag...")
			return fmt.Errorf("assistant tool call %q has no matching tool result in user message %d", id, index)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func toolCallIDs(msg model.Message) map[string]bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ids := make(map[string]bool)
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		if call, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			ids[call.ID] = true
		}
	}
	observe.GlobalTrace("return: ids")
	return ids
}

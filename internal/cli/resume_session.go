package cli

import (
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/session"
)

// resumedConversation adapts loaded durable session state to runtime state
// without rebuilding the persisted system prompt.
func resumedConversation(sess session.Session, fallbackWorkDir string) model.Conversation {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	conv := sess.Conversation
	if conv.WorkDir == "" {
		observe.GlobalTrace("if: conv.WorkDir == \"\"")
		conv.WorkDir = fallbackWorkDir
	}
	observe.GlobalTrace("return: conv")
	return conv
}

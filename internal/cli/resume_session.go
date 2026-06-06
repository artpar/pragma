package cli

import (
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/session"
)

// resumedConversation adapts loaded durable session state to runtime state
// without rebuilding the persisted system prompt.
func resumedConversation(sess session.Session, fallbackWorkDir string) model.Conversation {
	conv := sess.Conversation
	if conv.WorkDir == "" {
		conv.WorkDir = fallbackWorkDir
	}
	return conv
}

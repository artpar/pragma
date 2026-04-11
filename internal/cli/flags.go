package cli

import (
	"github.com/spf13/cobra"
	"github.com/artpar/gogent/internal/observe"
)

// RegisterFlags adds all CLI flags to the root cobra command.
func RegisterFlags(cmd *cobra.Command) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmd.Flags().StringP("prompt", "p", "", "prompt to send (non-interactive mode)")
	cmd.Flags().String("model", "", "model name")
	cmd.Flags().String("provider", "", "provider name (anthropic, groq)")
	cmd.Flags().String("api-key", "", "API key")
	cmd.Flags().String("system-prompt", "", "system prompt")
	cmd.Flags().Int("max-tokens", 0, "max output tokens")
	cmd.Flags().Float64("temperature", 0, "sampling temperature")
	cmd.Flags().Bool("thinking", false, "enable extended thinking")
	cmd.Flags().Int("thinking-budget", 0, "thinking token budget")
	cmd.Flags().Bool("verbose", false, "verbose logging to stderr")
	cmd.Flags().Bool("record", false, "record events to file")
	cmd.Flags().String("resume", "", "resume session by ID")
	cmd.Flags().Bool("list-sessions", false, "list saved sessions")
}

package cli

import (
	"github.com/artpar/pragma/internal/observe"
	"github.com/spf13/cobra"
)

// RegisterFlags adds all CLI flags to the root cobra command.
// Persistent flags are inherited by subcommands (model, provider, etc.).
// Local flags are root-only (prompt, resume, output-schema, etc.).
func RegisterFlags(cmd *cobra.Command) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	// Persistent flags — inherited by all subcommands
	pf := cmd.PersistentFlags()
	pf.String("model", "", "model name")
	pf.String("provider", "", "provider name (anthropic, openai, google, groq, lilac)")
	pf.String("api-key", "", "API key")
	pf.Int("max-tokens", 0, "max output tokens")
	pf.Float64("temperature", 0, "sampling temperature")
	pf.Bool("thinking", false, "enable extended thinking")
	pf.Int("thinking-budget", 0, "thinking token budget")
	pf.Bool("verbose", false, "verbose logging to stderr")
	pf.Bool("record", false, "record events to file")
	pf.Int("max-turns", 0, "override default turn limit (0 = use default)")
	pf.String("permission-mode", "", "permission mode: default, acceptEdits, bypassPermissions, dontAsk")

	pf.Bool("bg", false, "run session in background (requires --prompt)")

	// Local flags — root command only (interactive/non-interactive dispatch)
	cmd.Flags().StringP("prompt", "p", "", "prompt to send (non-interactive mode)")
	cmd.Flags().String("resume", "", "resume session by ID")
	cmd.Flags().BoolP("continue", "c", false, "resume most recent session in current directory")
	cmd.Flags().Bool("list-sessions", false, "list saved sessions")
	cmd.Flags().String("output-schema", "", "JSON Schema for structured output (file path or inline JSON, non-interactive only)")
	cmd.MarkFlagsMutuallyExclusive("continue", "resume")
}

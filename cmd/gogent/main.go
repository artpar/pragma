package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/buildinfo"
	"github.com/artpar/gogent/internal/cli"
	"github.com/artpar/gogent/internal/slash"
)

func main() {
	root := &cobra.Command{
		Use:           "gogent",
		Short:         "AI coding assistant",
		Long:          "gogent is a CLI AI coding assistant powered by LLMs.",
		RunE:          cli.RunDispatcher,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(versionCmd())
	root.AddCommand(completionCmd())
	root.AddCommand(sessionsCmd())
	root.AddCommand(replayCmd())

	// Register CLI subcommands from slash command registry (commit, review, init, doctor, etc.)
	slashCmds := slash.NewRegistry()
	cli.RegisterSubcommands(root, slashCmds)

	cli.RegisterFlags(root)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("gogent %s (commit %s, built %s, %s)\n",
				buildinfo.Version, buildinfo.Commit, buildinfo.Date, buildinfo.GoVersion)
		},
	}
}

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for gogent.

To load completions:

Bash:
  $ source <(gogent completion bash)
  # Or install permanently:
  $ gogent completion bash > /usr/share/bash-completion/completions/gogent

Zsh:
  $ source <(gogent completion zsh)
  # Or install permanently:
  $ gogent completion zsh > "${fpath[1]}/_gogent"

Fish:
  $ gogent completion fish | source
  # Or install permanently:
  $ gogent completion fish > ~/.config/fish/completions/gogent.fish

PowerShell:
  PS> gogent completion powershell | Out-String | Invoke-Expression
`,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(os.Stdout, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			}
			return nil
		},
	}
	return cmd
}

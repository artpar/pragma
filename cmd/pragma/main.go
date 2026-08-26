package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/buildinfo"
	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/slash"
)

func main() {
	root := newRootCommand()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "pragma",
		Short:         "AI coding assistant",
		Long:          "pragma is a CLI AI coding assistant powered by LLMs.",
		RunE:          cli.RunDispatcher,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(versionCmd())
	root.AddCommand(completionCmd())
	root.AddCommand(sessionsCmd())
	root.AddCommand(replayCmd())
	root.AddCommand(inspectCmd())
	root.AddCommand(auditCmd())
	root.AddCommand(metricsCmd())
	root.AddCommand(orchestrationCmd())
	root.AddCommand(cronCmd())

	// Register CLI subcommands from slash command registry (commit, review, init, doctor, etc.)
	slashCmds := slash.NewRegistry()
	cli.RegisterSubcommands(root, slashCmds)

	cli.RegisterFlags(root)

	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("pragma %s (commit %s, built %s, %s)\n",
				buildinfo.Version, buildinfo.Commit, buildinfo.Date, buildinfo.GoVersion)
		},
	}
}

func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for pragma.

To load completions:

Bash:
  $ source <(pragma completion bash)
  # Or install permanently:
  $ pragma completion bash > /usr/share/bash-completion/completions/pragma

Zsh:
  $ source <(pragma completion zsh)
  # Or install permanently:
  $ pragma completion zsh > "${fpath[1]}/_pragma"

Fish:
  $ pragma completion fish | source
  # Or install permanently:
  $ pragma completion fish > ~/.config/fish/completions/pragma.fish

PowerShell:
  PS> pragma completion powershell | Out-String | Invoke-Expression
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

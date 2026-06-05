package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/slash"
)

// RegisterSubcommands creates Cobra subcommands from slash.Registry commands
// that have CLIUse set. Prompt-type → RunPromptCommand, Local-type → RunLocalCommand.
func RegisterSubcommands(root *cobra.Command, registry *slash.Registry) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	for _, cmd := range registry.Commands() {
		observe.GlobalTrace("range registry.Commands()")
		if cmd.CLIUse == "" {
			observe.GlobalTrace("if: cmd.CLIUse == \"\"")
			continue
		}
		slashCmd := cmd
		short := slashCmd.CLIShort
		if short == "" {
			observe.GlobalTrace("if: short == \"\"")
			short = slashCmd.Description
		}
		cobraCmd := &cobra.Command{
			Use:     slashCmd.CLIUse,
			Short:   short,
			Aliases: slashCmd.Aliases,
			RunE: func(cmd *cobra.Command, args []string) error {
				joinedArgs := strings.Join(args, " ")
				if slashCmd.Type == slash.TypePrompt {
					return RunPromptCommand(cmd, slashCmd, joinedArgs)
				}
				return RunLocalCommand(cmd, slashCmd, joinedArgs)
			},
		}
		root.AddCommand(cobraCmd)
	}
}

// RunPromptCommand runs a prompt-type slash command non-interactively.
// The handler produces an InjectPrompt, which is fed to the engine.
// Hooks fire in all modes (fixes TS bugs #40506, #36071, #33343).
func RunPromptCommand(cmd *cobra.Command, slashCmd slash.Command, args string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	return runNonInteractive(cmd, nonInteractiveRunOptions{
		AllowStructuredOutput: true,
		PrintCost:             true,
		ConfigureDeps: func(d *Deps) {
			for _, spec := range slashCmd.AllowedTools {
				observe.GlobalTrace("range slashCmd.AllowedTools")
				rule := parseAllowedToolSpec(spec)
				d.Checker.AddSessionRule(rule)
			}
		},
		PreparePrompt: func(ctx context.Context, d *Deps) (nonInteractivePromptPlan, error) {
			sessStore, _ := session.NewStore()
			skillLoader := skill.NewLoader(d.Cwd)
			slashDeps := slash.Deps{
				Store:        d.Store,
				CostTracker:  d.CostTracker,
				Bus:          d.Bus,
				ModelName:    d.Cfg.Model,
				Provider:     d.Cfg.Provider,
				Cwd:          d.Cwd,
				SessionStore: sessStore,
				SkillLoader:  skillLoader,
			}
			result, err := slashCmd.Handle(ctx, args, slashDeps)
			if err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: fmt.Errorf(\"command /%s: %w\", slashCmd.Name, err)")
				return nonInteractivePromptPlan{}, fmt.Errorf("command /%s: %w", slashCmd.Name, err)
			}
			if result.InjectPrompt == "" {
				observe.GlobalTrace("if: result.InjectPrompt == \"\"")
				return nonInteractivePromptPlan{DisplayText: result.DisplayText}, nil
			}
			return nonInteractivePromptPlan{Prompt: result.InjectPrompt, Run: true}, nil
		},
	})
}

// RunLocalCommand runs a local-type slash command that needs no engine.
// Tries full SetupDeps first; falls back to lightweight config-only deps
// if provider/API key is unavailable (e.g., for 'doctor').
func RunLocalCommand(cmd *cobra.Command, slashCmd slash.Command, args string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	d, fullErr := SetupDeps(cmd)
	if fullErr == nil {
		observe.GlobalTrace("if: fullErr == nil")
		if d.Cleanup != nil {
			observe.GlobalTrace("if: d.Cleanup != nil")
			defer d.Cleanup()
		}
		d.Bus.Subscribe(d.StderrLogger)
		ss, _ := session.NewStore()
		sl := skill.NewLoader(d.Cwd)
		slashDeps := slash.Deps{
			Store:        d.Store,
			CostTracker:  d.CostTracker,
			Bus:          d.Bus,
			ModelName:    d.Cfg.Model,
			Provider:     d.Cfg.Provider,
			Cwd:          d.Cwd,
			SessionStore: ss,
			SkillLoader:  sl,
			McpStatus:    func() []slash.McpServerStatus { return mcpStatusesForSlash(d.McpManager) },
		}
		result, err := slashCmd.Handle(cmd.Context(), args, slashDeps)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"command /%s: %w\", slashCmd.Name, err)")
			return fmt.Errorf("command /%s: %w", slashCmd.Name, err)
		}
		if result.DisplayText != "" {
			observe.GlobalTrace("if: result.DisplayText != \"\"")
			fmt.Println(result.DisplayText)
		}
		observe.GlobalTrace("return: nil")
		return nil
	}

	cwd, _ := os.Getwd()
	cfg, _ := config.Load(cwd)
	if m, _ := cmd.Flags().GetString("model"); m != "" {
		observe.GlobalTrace("if: m != \"\"")
		cfg.Model = m
	}
	if p, _ := cmd.Flags().GetString("provider"); p != "" {
		observe.GlobalTrace("if: p != \"\"")
		cfg.Provider = p
	}

	ss2, _ := session.NewStore()
	sl2 := skill.NewLoader(cwd)
	slashDeps := slash.Deps{
		ModelName:    cfg.Model,
		Provider:     cfg.Provider,
		Cwd:          cwd,
		SessionStore: ss2,
		SkillLoader:  sl2,
	}

	result, err := slashCmd.Handle(cmd.Context(), args, slashDeps)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"command /%s: %w\", slashCmd.Name, err)")
		return fmt.Errorf("command /%s: %w", slashCmd.Name, err)
	}
	if result.DisplayText != "" {
		observe.GlobalTrace("if: result.DisplayText != \"\"")
		fmt.Println(result.DisplayText)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// parseAllowedToolSpec parses a TS-style tool spec like "Bash(git add:*)"
// into a permission Rule. Format: "ToolName(content)" or just "ToolName".
func parseAllowedToolSpec(spec string) permission.Rule {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	toolName := spec
	content := ""
	if idx := strings.Index(spec, "("); idx > 0 && strings.HasSuffix(spec, ")") {
		observe.GlobalTrace("if: idx > 0 && strings.HasSuffix(spec, \")\")")
		toolName = spec[:idx]
		content = spec[idx+1 : len(spec)-1]
	}
	observe.GlobalTrace("return: permission.Rule{\n\tToolName:\ttoolName,\n\tContent:\tcontent,\n\tDecision:\tpermissio...")
	return permission.Rule{
		ToolName: toolName,
		Content:  content,
		Decision: permission.DecisionAllow,
		Source:   permission.SourceSession,
	}
}

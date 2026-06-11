package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
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
	observe.GlobalTrace("return: runNonInteractive(cmd, nonInteractiveRunOptions{\n\tAllowStructuredOutput:\ttrue...")

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
			skillCatalog := runtimeSkillCatalog(d)
			slashDeps := slash.Deps{
				Store:        d.Store,
				CostTracker:  d.CostTracker,
				Bus:          d.Bus,
				ModelName:    d.Cfg.Model,
				Provider:     d.Cfg.Provider,
				Cwd:          d.Cwd,
				SessionStore: sessStore,
				SkillCatalog: skillCatalog,
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
func RunLocalCommand(cmd *cobra.Command, slashCmd slash.Command, args string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	slashDeps, err := BuildLocalSlashDeps(cmd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
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

// BuildLocalSlashDeps constructs dependencies for TypeLocal slash commands
// without creating providers, runtime logs, MCP clients, task registries, or
// other prompt-runtime infrastructure.
func BuildLocalSlashDeps(cmd *cobra.Command) (slash.Deps, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cwd, err := os.Getwd()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: slash.Deps{}, fmt.Errorf(\"get working directory: %w\", err)")
		return slash.Deps{}, fmt.Errorf("get working directory: %w", err)
	}
	cfg, _ := config.Load(cwd)
	creds, _ := config.LoadCredentials()
	ApplyFlagOverrides(cmd, &cfg)
	if cfg.Provider == "" {
		observe.GlobalTrace("if: cfg.Provider == \"\"")
		cfg.Provider = autoDetectProvider(creds)
	}
	if cfg.Provider == "" {
		observe.GlobalTrace("if: cfg.Provider == \"\"")
		cfg.Provider = "lilac"
	}
	if cfg.Model == "" {
		observe.GlobalTrace("if: cfg.Model == \"\"")
		cfg.Model = DefaultModelFor(cfg.Provider)
	}
	cfg.Model = resolveModelAlias(cfg.Provider, cfg.Model)

	ss, _ := session.NewStore()
	sl := skill.NewRuntimeCatalog(cwd, func() string {
		return cwd
	})
	conv := model.NewConversation(model.SystemPrompt{}, cfg.Model, cfg.Provider, cwd)
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          cwd,
		Model:        cfg.Model,
		Provider:     cfg.Provider,
		MaxTokens:    cfg.MaxTokens,
		Temperature:  cfg.Temperature,
	})
	observe.GlobalTrace("return: slash.Deps{\n\tStore:\t\tstore,\n\tCostTracker:\tmodel.NewCostTracker(0),\n\tModelName...")

	return slash.Deps{
		Store:        store,
		CostTracker:  model.NewCostTracker(0),
		ModelName:    cfg.Model,
		Provider:     cfg.Provider,
		Cwd:          cwd,
		SessionStore: ss,
		SkillCatalog: sl,
		McpStatus:    localMcpStatuses(cwd),
	}, nil
}

func localMcpStatuses(cwd string) func() []slash.McpServerStatus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func() []slash.McpServerStatus {\n\tbus := observe.NewEventBus(16)\n\tservers, er...")
	return func() []slash.McpServerStatus {
		bus := observe.NewEventBus(16)
		servers, err := mcp.LoadConfig(cwd, bus)
		if err != nil {
			return nil
		}
		names := make([]string, 0, len(servers))
		for name := range servers {
			names = append(names, name)
		}
		sort.Strings(names)
		statuses := make([]slash.McpServerStatus, 0, len(names))
		for _, name := range names {
			cfg := servers[name]
			transport := cfg.Type
			if transport == "" {
				transport = "stdio"
			}
			statuses = append(statuses, slash.McpServerStatus{
				Name:      name,
				Status:    "disconnected",
				Transport: transport,
			})
		}
		return statuses
	}
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

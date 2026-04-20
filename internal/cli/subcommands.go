package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tui"
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

	d, err := SetupDeps(cmd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if d.Cleanup != nil {
		observe.GlobalTrace("if: d.Cleanup != nil")
		defer d.Cleanup()
	}
	d.Bus.Subscribe(d.StderrLogger)

	snap := d.Store.Snapshot()
	d.Bus.Emit(observe.SessionStarted{
		EventHeader: observe.NewEventHeader("SessionStarted", "", "", ""),
		SessionID:   snap.Conversation.ID,
	})

	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		d.HookMgr.Execute(cmd.Context(), hook.SessionStart, hook.HookInput{})
	}
	defer func() {
		if d.HookMgr != nil {
			observe.GlobalTrace("if: d.HookMgr != nil")
			d.HookMgr.Execute(cmd.Context(), hook.SessionEnd, hook.HookInput{})
		}
	}()

	for _, spec := range slashCmd.AllowedTools {
		observe.GlobalTrace("range slashCmd.AllowedTools")
		rule := parseAllowedToolSpec(spec)
		d.Checker.AddSessionRule(rule)
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	applyToolFilters(cmd, d.Registry)

	compDeps, _ := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

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

	result, err := slashCmd.Handle(cmd.Context(), args, slashDeps)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"command /%s: %w\", slashCmd.Name, err)")
		return fmt.Errorf("command /%s: %w", slashCmd.Name, err)
	}

	if result.InjectPrompt == "" {
		observe.GlobalTrace("if: result.InjectPrompt == \"\"")
		if result.DisplayText != "" {
			observe.GlobalTrace("if: result.DisplayText != \"\"")
			fmt.Println(result.DisplayText)
		}
		observe.GlobalTrace("return: nil")
		return nil
	}

	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)

	ctx := cmd.Context()
	events := engine.Run(ctx, result.InjectPrompt)

	for ev := range events {
		observe.GlobalTrace("range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.GlobalTrace("typecase: query.TextEvent")
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			observe.GlobalTrace("typecase: query.ThinkingEvent")
			if d.Cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			observe.GlobalTrace("typecase: query.ToolCallEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			}
		case query.ToolResultEvent:
			observe.GlobalTrace("typecase: query.ToolResultEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
			}
		case query.CompactionEvent:
			observe.GlobalTrace("typecase: query.CompactionEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[auto-compacted: %d → %d tokens]\n", e.PreTokens, e.PostTokens)
			}
		case query.TurnCompleteEvent:
			observe.GlobalTrace("typecase: query.TurnCompleteEvent")
			fmt.Println()
		case query.ErrorEvent:
			observe.GlobalTrace("typecase: query.ErrorEvent")
			sessionSaveFn()
			sessionCloseFn()
			return e.Err
		}
	}

	sessionSaveFn()
	sessionCloseFn()

	if d.Cfg.Verbose {
		observe.GlobalTrace("if: d.Cfg.Verbose")
		fmt.Fprintf(os.Stderr, "total cost: $%.6f\n", d.CostTracker.TotalUSD())
	}
	observe.GlobalTrace("return: nil")
	return nil
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

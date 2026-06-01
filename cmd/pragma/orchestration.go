package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/orchestration"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/tui"
)

func orchestrationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestration",
		Short: "Run orchestration state machines",
	}
	cmd.AddCommand(orchestrationRunCmd())
	return cmd
}

func orchestrationRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <orchestration.yaml>",
		Short: "Execute an orchestration graph",
		Args:  cobra.ExactArgs(1),
		RunE:  runOrchestration,
	}
	cmd.Flags().String("prompt", "", "Task prompt")
	cmd.Flags().String("persona-dir", "personas", "Directory containing persona YAML files")
	return cmd
}

func runOrchestration(cmd *cobra.Command, args []string) error {
	taskPrompt, _ := cmd.Flags().GetString("prompt")
	if taskPrompt == "" {
		return fmt.Errorf("--prompt is required")
	}

	def, err := orchestration.LoadDefinitionFile(args[0])
	if err != nil {
		return err
	}
	runtime, err := orchestration.NewRuntime(def)
	if err != nil {
		return err
	}

	d, err := cli.SetupDeps(cmd)
	if err != nil {
		return err
	}
	if d.Cleanup != nil {
		defer d.Cleanup()
	}
	d.Bus.Subscribe(d.StderrLogger)
	if !cmd.Flags().Changed("permission-mode") {
		d.Checker = permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	engine, err := cli.RegisterTools(d, prompter, asker)
	if err != nil {
		return err
	}
	compDeps, _ := cli.BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	personaDir, _ := cmd.Flags().GetString("persona-dir")
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		personaDef, err := loadPersonaForState(personaDir, state)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "orchestration: state=%s persona=%s\n", stateID, personaDef.ID)

		if _, err := runOrchestrationState(cmd.Context(), engine, stateID, personaDef, taskPrompt); err != nil {
			return fmt.Errorf("state %q failed: %w", stateID, err)
		}

		event, err := selectStateEvent(state)
		if err != nil {
			return fmt.Errorf("select event for state %q: %w", stateID, err)
		}
		if err := runtime.FSM.Event(cmd.Context(), event); err != nil {
			return fmt.Errorf("transition %q from %q: %w", event, stateID, err)
		}
	}

	fmt.Fprintf(os.Stderr, "orchestration: done\n")
	return nil
}

func loadPersonaForState(personaDir string, state orchestration.State) (persona.Definition, error) {
	personaID := state.Persona
	if personaID == "" {
		personaID = state.ID
	}
	return persona.LoadDefinitionFile(filepath.Join(personaDir, personaID+".yaml"))
}

func selectStateEvent(state orchestration.State) (string, error) {
	if state.Event.FromFile == nil {
		if state.Event.Default != "" {
			return state.Event.Default, nil
		}
		return orchestration.EventComplete, nil
	}

	raw, err := os.ReadFile(state.Event.FromFile.Path)
	if err != nil {
		return "", err
	}
	content := string(raw)
	for _, rule := range state.Event.FromFile.Rules {
		if strings.Contains(content, rule.Contains) {
			return rule.Event, nil
		}
	}
	if state.Event.Default != "" {
		return state.Event.Default, nil
	}
	return "", fmt.Errorf("no file event rule matched %q", state.Event.FromFile.Path)
}

func runOrchestrationState(ctx context.Context, engine *query.Engine, stateID string, personaDef persona.Definition, taskPrompt string) (string, error) {
	prompt := fmt.Sprintf(`%s

## Task

%s
`, personaDef.Prompt, taskPrompt)

	var text strings.Builder
	start := time.Now()
	for ev := range engine.Run(ctx, prompt) {
		switch e := ev.(type) {
		case query.TextEvent:
			text.WriteString(e.Text)
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			if e.Text != "" {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
		case query.ToolResultEvent:
			fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
		case query.RetryEvent:
			fmt.Fprintf(os.Stderr, "[retry: %s in %s]\n", e.Kind, e.Delay)
		case query.TurnCompleteEvent:
			fmt.Fprintf(os.Stderr, "\n[state %s complete in %s]\n", stateID, time.Since(start).Round(time.Second))
		case query.ErrorEvent:
			return text.String(), e.Err
		}
	}
	return text.String(), nil
}

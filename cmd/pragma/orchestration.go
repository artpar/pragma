package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/model"
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
		return cli.RunInteractive(cmd)
	}

	def, err := orchestration.LoadDefinitionFile(args[0])
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
	return printOrchestrationEvents(orchestration.RunEvents(cmd.Context(), engine, def, personaDir, taskPrompt))
}

func printOrchestrationEvents(events <-chan query.LoopEvent) error {
	for ev := range events {
		switch e := ev.(type) {
		case query.TextEvent:
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
		case query.ErrorEvent:
			return e.Err
		case query.TurnCompleteEvent:
			return nil
		}
	}
	return nil
}

func controlName(state orchestration.State) string {
	return orchestration.ControlName(state)
}

func loadPersonaForState(personaDir string, state orchestration.State) (persona.Definition, error) {
	return orchestration.LoadPersonaForState(personaDir, state)
}

func selectStateEvent(state orchestration.State) (string, error) {
	return orchestration.SelectStateEvent(state)
}

func buildOrchestrationPrompt(def orchestration.Definition, state orchestration.State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (model.SystemPrompt, string) {
	return orchestration.BuildPrompt(def, state, personaDef, taskPrompt, handoffPrompt)
}

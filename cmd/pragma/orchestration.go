package main

import (
	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/orchestration"
	"github.com/artpar/pragma/internal/persona"
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
	personaDir, _ := cmd.Flags().GetString("persona-dir")
	return cli.RunStandaloneOrchestration(cmd, cli.StandaloneOrchestrationOptions{
		DefinitionPath: args[0],
		PersonaDir:     personaDir,
		Prompt:         taskPrompt,
	})
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

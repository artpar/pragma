package main

import (
	"fmt"
	"os"
	"strings"

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
	cmd.AddCommand(orchestrationVisualizeCmd())
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
	cmd.Flags().StringArray("seed-artifact", nil, "Seed artifact content as source=path")
	return cmd
}

func runOrchestration(cmd *cobra.Command, args []string) error {
	taskPrompt, _ := cmd.Flags().GetString("prompt")
	personaDir, _ := cmd.Flags().GetString("persona-dir")
	seedArtifactFlags, _ := cmd.Flags().GetStringArray("seed-artifact")
	seedArtifacts, err := readSeedArtifactFiles(seedArtifactFlags)
	if err != nil {
		return err
	}
	return cli.RunStandaloneOrchestration(cmd, cli.StandaloneOrchestrationOptions{
		DefinitionPath: args[0],
		PersonaDir:     personaDir,
		Prompt:         taskPrompt,
		SeedArtifacts:  seedArtifacts,
	})
}

func readSeedArtifactFiles(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		source, path, ok := strings.Cut(value, "=")
		source = strings.TrimSpace(source)
		path = strings.TrimSpace(path)
		if !ok || source == "" || path == "" {
			return nil, fmt.Errorf("--seed-artifact requires source=path")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read seed artifact %q from %q: %w", source, path, err)
		}
		out[source] = string(content)
	}
	return out, nil
}

func orchestrationVisualizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "visualize <orchestration.yaml>",
		Short: "Print an ASCII orchestration graph",
		Args:  cobra.ExactArgs(1),
		RunE:  visualizeOrchestration,
	}
	cmd.Flags().String("persona-dir", "personas", "Directory containing persona YAML files")
	cmd.Flags().String("details", orchestration.VisualizationDetailsCompact, "detail level: compact or full")
	cmd.Flags().Bool("no-personas", false, "Do not load persona definitions")
	return cmd
}

func visualizeOrchestration(cmd *cobra.Command, args []string) error {
	personaDir, _ := cmd.Flags().GetString("persona-dir")
	details, _ := cmd.Flags().GetString("details")
	noPersonas, _ := cmd.Flags().GetBool("no-personas")

	def, err := orchestration.LoadDefinitionFile(args[0])
	if err != nil {
		return err
	}
	out, err := orchestration.RenderVisualization(def, orchestration.VisualizationOptions{
		PersonaDir:  personaDir,
		Details:     details,
		LoadPersona: !noPersonas,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), out)
	return err
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

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/lifecycle"
	"github.com/artpar/pragma/internal/lifecycle/bridge"
	"github.com/artpar/pragma/internal/lifecycle/definition"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

func lifecycleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Run lifecycle graphs",
		Long:  "Execute lifecycle workflow graphs — either from a YAML file or by describing the structure in natural language.",
	}
	cmd.AddCommand(lifecycleRunCmd())
	cmd.AddCommand(lifecycleListCmd())
	return cmd
}

func lifecycleRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [file.yaml]",
		Short: "Execute a lifecycle graph from YAML file or structure description",
		Long: `Execute a lifecycle workflow graph.

Provide either a YAML file path or a --structure description (which the LLM compiles into a graph).

Examples:
  pragma lifecycle run workflow.yaml --prompt "analyze this code"
  pragma lifecycle run --structure "attempt fix, run tests, retry on failure" --prompt "fix the failing test"
  pragma lifecycle run --structure "attempt with tools, evaluate, reflect on failure, retry" --prompt "fix the bug"`,
		Args: cobra.MaximumNArgs(1),
		RunE: runLifecycle,
	}
	cmd.Flags().String("prompt", "", "Initial task/message for the lifecycle")
	cmd.Flags().String("structure", "", "Natural language description of the execution structure (LLM compiles to graph)")
	return cmd
}

func lifecycleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List example lifecycle structure descriptions",
		Run: func(cmd *cobra.Command, args []string) {
			examples := bridge.StructureExamples()
			names := make([]string, 0, len(examples))
			for name := range examples {
				names = append(names, name)
			}
			sort.Strings(names)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "STRUCTURE\tDESCRIPTION")
			for _, name := range names {
				fmt.Fprintf(w, "%s\t%s\n", name, examples[name])
			}
			w.Flush()
		},
	}
}

func runLifecycle(cmd *cobra.Command, args []string) error {
	d, err := cli.SetupDepsWithOptions(cmd, cli.SetupDepsOptions{DefaultPermissionMode: permission.ModeBypassPermissions})
	if err != nil {
		return err
	}
	if d.Cleanup != nil {
		defer d.Cleanup()
	}

	prompt, _ := cmd.Flags().GetString("prompt")
	if prompt == "" {
		return fmt.Errorf("--prompt is required")
	}

	structure, _ := cmd.Flags().GetString("structure")
	hasYAML := len(args) == 1
	if !hasYAML && structure == "" {
		return fmt.Errorf("provide either a YAML file path or --structure description")
	}
	if hasYAML && structure != "" {
		return fmt.Errorf("provide either a YAML file path or --structure, not both")
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tool.NonInteractiveAsker{}
	_, err = cli.RegisterTools(d, prompter, asker)
	if err != nil {
		return err
	}

	orch := tool.NewOrchestrator(d.Registry, d.Checker, prompter, d.Bus)
	if d.HookMgr != nil {
		orch.SetHookManager(d.HookMgr)
	}

	infra := bridge.Infra{
		Provider:     d.Prov,
		Orchestrator: orch,
		Registry:     d.Registry,
		Bus:          d.Bus,
		Cwd:          d.Cwd,
	}

	// Resolve the graph: YAML file or LLM-generated from structure description
	var graph *lifecycle.Graph

	if hasYAML {
		target := args[0]
		ext := strings.ToLower(filepath.Ext(target))
		if ext != ".yaml" && ext != ".yml" {
			return fmt.Errorf("file must have .yaml or .yml extension, got %q", target)
		}
		graph, err = loadYAMLGraph(target, infra)
		if err != nil {
			return fmt.Errorf("load graph from %s: %w", target, err)
		}
	} else {
		graph, err = generateGraph(cmd, d, structure, infra)
		if err != nil {
			return fmt.Errorf("generate graph: %w", err)
		}
	}

	ctx := cmd.Context()
	snap := d.Store.Snapshot()
	runner := bridge.NewRunner(graph, bridge.NewRunnerConfig(
		snap.Conversation.System,
		d.EngineCfg.Model,
		d.EngineCfg.MaxTokens,
		d.Registry.ToolDefs(),
		d.Bus,
	))

	for runEv := range runner.Stream(ctx, prompt) {
		progress := runEv.Progress
		switch progress.Status {
		case "step_started":
			if len(progress.Nodes) > 0 {
				fmt.Fprintf(os.Stderr, "⎿ Step %d: %s\n", progress.Step, strings.Join(progress.Nodes, ", "))
			} else {
				fmt.Fprintf(os.Stderr, "⎿ Step %d\n", progress.Step)
			}
		case "node_completed":
			if progress.Err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", progress.Node, progress.Err)
				return fmt.Errorf("lifecycle node %q failed at step %d: %w", progress.Node, progress.Step, progress.Err)
			}
			if progress.Duration > 0 {
				fmt.Fprintf(os.Stderr, "  ✓ %s (%s)\n", progress.Node, progress.Duration.Round(100*time.Millisecond))
			} else {
				fmt.Fprintf(os.Stderr, "  ✓ %s\n", progress.Node)
			}
		case "transition":
			if progress.RouteKey != "" {
				fmt.Fprintf(os.Stderr, "  → %s (route: %s)\n", progress.ToNode, progress.RouteKey)
			}
		case "completed":
			if runEv.Result.Err != nil {
				return fmt.Errorf("lifecycle execution failed: %w", runEv.Result.Err)
			}
			fmt.Fprintf(os.Stderr, "✓ Completed in %d steps\n", progress.Step)

			// Print final assistant message to stdout.
			if runEv.Result.AssistantText != "" {
				fmt.Println(runEv.Result.AssistantText)
			}
		}
	}

	return nil
}

func generateGraph(cmd *cobra.Command, d *cli.Deps, structure string, infra bridge.Infra) (*lifecycle.Graph, error) {
	modelID := cli.SecondaryModelFor(d.Cfg.Provider)
	return bridge.GenerateAndResolveGraph(cmd.Context(), d.Prov, d.Bus, modelID, structure, infra)
}

func loadYAMLGraph(path string, infra bridge.Infra) (*lifecycle.Graph, error) {
	def, err := definition.ParseFile(path)
	if err != nil {
		return nil, err
	}

	return bridge.ResolveGraph(def, infra)
}

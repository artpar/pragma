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
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/tui"
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
  gogent lifecycle run workflow.yaml --prompt "analyze this code"
  gogent lifecycle run --structure "tool-calling loop" --prompt "list files"
  gogent lifecycle run --structure "attempt with tools, evaluate, reflect on failure, retry" --prompt "fix the bug"`,
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
	d, err := cli.SetupDeps(cmd)
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

	// Lifecycle is non-interactive — bypass permission prompts.
	checker := permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	_, err = cli.RegisterTools(d, prompter, asker)
	if err != nil {
		return err
	}

	orch := tool.NewOrchestrator(d.Registry, checker, prompter, d.Bus)
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

	// Build initial state
	initialState := lifecycle.State{
		bridge.KeyMessages: []model.Message{
			{
				ID:        model.NewUUID(),
				Role:      model.RoleUser,
				Content:   []model.ContentPart{model.TextPart{Text: prompt}},
				Timestamp: time.Now(),
			},
		},
		bridge.KeySystem: model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: "You are a helpful AI assistant."}},
		},
		bridge.KeyModelID:   d.EngineCfg.Model,
		bridge.KeyMaxTokens: d.EngineCfg.MaxTokens,
		bridge.KeyTools:     d.Registry.ToolDefs(),
	}

	// Run the lifecycle graph
	executor := lifecycle.NewExecutor(graph, lifecycle.WithEventBus(d.Bus))

	ctx := cmd.Context()
	events := executor.Stream(ctx, initialState)

	for ev := range events {
		switch ev.Type {
		case "step_started":
			if len(ev.Nodes) > 0 {
				fmt.Fprintf(os.Stderr, "⎿ Step %d: %s\n", ev.Step, strings.Join(ev.Nodes, ", "))
			} else {
				fmt.Fprintf(os.Stderr, "⎿ Step %d\n", ev.Step)
			}
		case "node_completed":
			if ev.Err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", ev.Node, ev.Err)
				return fmt.Errorf("lifecycle node %q failed at step %d: %w", ev.Node, ev.Step, ev.Err)
			}
			fmt.Fprintf(os.Stderr, "  ✓ %s\n", ev.Node)
		case "completed":
			if ev.Err != nil {
				return fmt.Errorf("lifecycle execution failed: %w", ev.Err)
			}
			fmt.Fprintf(os.Stderr, "✓ Completed in %d steps\n", ev.Step)

			// Print final assistant message
			msgs := bridge.Messages(ev.State)
			for i := len(msgs) - 1; i >= 0; i-- {
				if msgs[i].Role == model.RoleAssistant {
					for _, part := range msgs[i].Content {
						if tp, ok := part.(model.TextPart); ok {
							fmt.Println(tp.Text)
						}
					}
					break
				}
			}
		}
	}

	return nil
}

func generateGraph(cmd *cobra.Command, d *cli.Deps, structure string, infra bridge.Infra) (*lifecycle.Graph, error) {
	modelID := cli.SecondaryModelFor(d.Cfg.Provider)

	def, err := bridge.GenerateGraph(cmd.Context(), d.Prov, d.Bus, modelID, structure)
	if err != nil {
		return nil, err
	}

	if def.Graph.Reducers == nil {
		def.Graph.Reducers = make(map[string]string)
	}
	def.Graph.Reducers["total_usage"] = "total_usage"
	def.Graph.Reducers["turn_count"] = "sum"

	factory := bridge.NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    bridge.MessageReducer,
			"reflections": bridge.ReflectionReducer,
			"total_usage": bridge.UsageReducer,
		},
	}

	return definition.Resolve(def, factory.Create, definition.DefaultRouterCreator(), opts)
}

func loadYAMLGraph(path string, infra bridge.Infra) (*lifecycle.Graph, error) {
	def, err := definition.ParseFile(path)
	if err != nil {
		return nil, err
	}

	factory := bridge.NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages":    bridge.MessageReducer,
			"reflections": bridge.ReflectionReducer,
			"total_usage": bridge.UsageReducer,
		},
	}

	return definition.Resolve(
		def,
		factory.Create,
		definition.DefaultRouterCreator(),
		opts,
	)
}

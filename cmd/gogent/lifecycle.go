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

	"github.com/artpar/gogent/internal/cli"
	"github.com/artpar/gogent/internal/lifecycle"
	"github.com/artpar/gogent/internal/lifecycle/bridge"
	"github.com/artpar/gogent/internal/lifecycle/definition"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
	"github.com/artpar/gogent/internal/tui"
)

func lifecycleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lifecycle",
		Short: "Run lifecycle patterns and graphs",
		Long:  "Execute lifecycle orchestration patterns (ReAct, Plan-Execute, Reflexion) or custom YAML graph definitions.",
	}
	cmd.AddCommand(lifecycleRunCmd())
	cmd.AddCommand(lifecycleListCmd())
	return cmd
}

func lifecycleRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <pattern|file.yaml>",
		Short: "Execute a lifecycle pattern or YAML graph definition",
		Args:  cobra.ExactArgs(1),
		RunE:  runLifecycle,
	}
	cmd.Flags().String("prompt", "", "Initial task/message for the lifecycle")
	return cmd
}

func lifecycleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available lifecycle patterns",
		Run: func(cmd *cobra.Command, args []string) {
			descs := bridge.PatternDescriptions()
			names := make([]string, 0, len(descs))
			for name := range descs {
				names = append(names, name)
			}
			sort.Strings(names)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PATTERN\tDESCRIPTION")
			for _, name := range names {
				fmt.Fprintf(w, "%s\t%s\n", name, descs[name])
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

	// RegisterTools populates d.Registry with all 36 tools.
	// We create a separate orchestrator for the lifecycle context
	// (non-interactive, no hooks — NonInteractivePrompter denies all asks).
	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
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

	// Resolve the graph: either a built-in pattern name or a YAML file
	var graph *lifecycle.Graph
	target := args[0]

	ext := strings.ToLower(filepath.Ext(target))
	if ext == ".yaml" || ext == ".yml" {
		graph, err = loadYAMLGraph(target, infra)
		if err != nil {
			return fmt.Errorf("load graph from %s: %w", target, err)
		}
	} else {
		patterns := bridge.PatternMap(infra)
		var ok bool
		graph, ok = patterns[target]
		if !ok {
			available := make([]string, 0, len(patterns))
			for name := range patterns {
				available = append(available, name)
			}
			sort.Strings(available)
			return fmt.Errorf("unknown pattern %q (available: %s)", target, strings.Join(available, ", "))
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
			fmt.Fprintf(os.Stderr, "⎿ Step %d\n", ev.Step)
		case "node_completed":
			if ev.Err != nil {
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

func loadYAMLGraph(path string, infra bridge.Infra) (*lifecycle.Graph, error) {
	def, err := definition.ParseFile(path)
	if err != nil {
		return nil, err
	}

	factory := bridge.NewNodeFactory(infra)
	opts := &definition.ResolveOptions{
		CustomReducers: map[string]lifecycle.ReducerFunc{
			"messages": bridge.MessageReducer,
		},
	}

	return definition.Resolve(
		def,
		factory.Create,
		definition.DefaultRouterCreator(),
		opts,
	)
}

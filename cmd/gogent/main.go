package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/anthropic"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
	toolagent "github.com/artpar/gogent/internal/tools/agent"
	toolbash "github.com/artpar/gogent/internal/tools/bash"
	toolfileedit "github.com/artpar/gogent/internal/tools/fileedit"
	toolfileread "github.com/artpar/gogent/internal/tools/fileread"
	toolfilewrite "github.com/artpar/gogent/internal/tools/filewrite"
	toolglob "github.com/artpar/gogent/internal/tools/glob"
	toolgrep "github.com/artpar/gogent/internal/tools/grep"
	toolnotebookedit "github.com/artpar/gogent/internal/tools/notebookedit"
	tooltaskcreate "github.com/artpar/gogent/internal/tools/taskcreate"
	tooltaskget "github.com/artpar/gogent/internal/tools/taskget"
	tooltasklist "github.com/artpar/gogent/internal/tools/tasklist"
	tooltaskstop "github.com/artpar/gogent/internal/tools/taskstop"
	tooltaskupdate "github.com/artpar/gogent/internal/tools/taskupdate"
	toolwebfetch "github.com/artpar/gogent/internal/tools/webfetch"
)

func main() {
	root := &cobra.Command{
		Use:   "gogent",
		Short: "AI coding assistant",
		Long:  "gogent is a CLI AI coding assistant powered by LLMs.",
		RunE:  runNonInteractive,
		// Silence cobra's default error/usage printing — we handle it ourselves.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.Flags().StringP("prompt", "p", "", "prompt to send (required for non-interactive mode)")
	root.Flags().String("model", "", "model name")
	root.Flags().String("provider", "", "provider name (anthropic)")
	root.Flags().String("api-key", "", "API key")
	root.Flags().String("system-prompt", "", "system prompt")
	root.Flags().Int("max-tokens", 0, "max output tokens")
	root.Flags().Float64("temperature", 0, "sampling temperature")
	root.Flags().Bool("thinking", false, "enable extended thinking")
	root.Flags().Int("thinking-budget", 0, "thinking token budget")
	root.Flags().Bool("verbose", false, "verbose logging to stderr")
	root.Flags().Bool("record", false, "record events to file")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runNonInteractive(cmd *cobra.Command, _ []string) error {
	prompt, _ := cmd.Flags().GetString("prompt")
	if prompt == "" {
		return fmt.Errorf("--prompt/-p is required for non-interactive mode")
	}

	// 1. Load config (3-scope merge)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Apply CLI flag overrides
	applyFlagOverrides(cmd, &cfg)

	// 3. Apply defaults
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-20250514"
	}
	if cfg.Provider == "" {
		cfg.Provider = "anthropic"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 16384
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key required: set --api-key or ANTHROPIC_API_KEY environment variable")
	}

	// 4. Create EventBus
	bus := observe.NewEventBus(1024)

	// 5. Subscribe Logger — errors only unless verbose
	logLevel := observe.LevelError
	if cfg.Verbose {
		logLevel = observe.LevelTrace
	}
	logger := observe.NewLogger(os.Stderr, logLevel, observe.FormatText, nil)
	bus.Subscribe(logger)

	// 6. Optionally subscribe Recorder
	if cfg.Record {
		recorder, recErr := observe.NewRecorder("gogent-recording.jsonl")
		if recErr != nil {
			return fmt.Errorf("create recorder: %w", recErr)
		}
		defer recorder.Close()
		bus.Subscribe(recorder)
	}

	// 7. Create Provider
	prov := createProvider(cfg, bus)

	// 8. Create permission checker
	permEntries, permMode, _ := config.LoadPermissions(cwd)
	rules := permission.RulesFromConfigEntries(permEntries)
	mode := permission.PermissionMode(permMode)
	if mode == "" {
		mode = permission.ModeBypassPermissions // non-interactive mode: allow all by default
	}
	checker := permission.NewRuleChecker(rules, mode, cwd, bus)
	prompter := &permission.NonInteractivePrompter{}

	// 9. Create StateStore
	var systemPrompt model.SystemPrompt
	if cfg.SystemPrompt != "" {
		systemPrompt = model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: cfg.SystemPrompt, Cacheable: true}},
		}
	}

	conv := model.NewConversation(systemPrompt, cfg.Model, cfg.Provider, cwd)
	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          cwd,
		Model:        cfg.Model,
		Provider:     cfg.Provider,
		MaxTokens:    cfg.MaxTokens,
		Temperature:  cfg.Temperature,
	})

	// 10. Create task registry and Engine config
	taskRegistry := task.NewRegistry(bus)
	costTracker := model.NewCostTracker()
	engineCfg := query.EngineConfig{
		Model:     cfg.Model,
		MaxTokens: cfg.MaxTokens,
		Temperature: cfg.Temperature,
	}
	if cfg.Thinking != nil && cfg.Thinking.Enabled {
		engineCfg.Thinking = &provider.ThinkingConfig{
			Enabled:      cfg.Thinking.Enabled,
			BudgetTokens: cfg.Thinking.BudgetTokens,
		}
	}

	// 11. EngineFactory for sub-agents
	engineFactory := func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore) {
		subRegistry := tool.NewRegistry(bus)
		// Register the same tools minus Agent (scopedToolNames is nil = all)
		for _, td := range []tool.Descriptor{
			&toolglob.Tool{},
			&toolgrep.Tool{},
			&toolfileread.Tool{},
			&toolfilewrite.Tool{},
			&toolfileedit.Tool{},
			&toolbash.Tool{},
			&toolnotebookedit.Tool{},
			&toolwebfetch.Tool{Provider: prov, Bus: bus},
			&tooltaskcreate.Tool{Tasks: taskRegistry},
			&tooltaskget.Tool{Tasks: taskRegistry},
			&tooltasklist.Tool{Tasks: taskRegistry},
			&tooltaskupdate.Tool{Tasks: taskRegistry},
			&tooltaskstop.Tool{Tasks: taskRegistry},
		} {
			_ = subRegistry.Register(td)
		}
		if scopedToolNames != nil {
			subRegistry = subRegistry.Scoped(scopedToolNames)
		}
		subStore := app.NewStateStore(app.AppState{
			Conversation: forkedConv,
			CWD:          cwd,
			Model:        cfg.Model,
			Provider:     cfg.Provider,
			MaxTokens:    cfg.MaxTokens,
			Temperature:  cfg.Temperature,
		})
		subOrch := tool.NewOrchestrator(subRegistry, checker, prompter, bus)
		subCfg := engineCfg
		if modelOverride != "" {
			subCfg.Model = modelOverride
		}
		subEngine := query.NewEngine(prov, subRegistry, subOrch, subStore, costTracker, bus, subCfg)
		return subEngine, subStore
	}

	// 12. Create main Registry and register all tools
	registry := tool.NewRegistry(bus)
	for _, t := range []tool.Descriptor{
		&toolglob.Tool{},
		&toolgrep.Tool{},
		&toolfileread.Tool{},
		&toolfilewrite.Tool{},
		&toolfileedit.Tool{},
		&toolbash.Tool{},
		&toolagent.Tool{EngineFactory: engineFactory, Store: store, Tasks: taskRegistry, Bus: bus},
		&toolnotebookedit.Tool{},
		&toolwebfetch.Tool{Provider: prov, Bus: bus},
		&tooltaskcreate.Tool{Tasks: taskRegistry},
		&tooltaskget.Tool{Tasks: taskRegistry},
		&tooltasklist.Tool{Tasks: taskRegistry},
		&tooltaskupdate.Tool{Tasks: taskRegistry},
		&tooltaskstop.Tool{Tasks: taskRegistry},
	} {
		if err := registry.Register(t); err != nil {
			return fmt.Errorf("register tool %s: %w", t.Name(), err)
		}
	}

	// 13. Create Orchestrator and Engine
	orchestrator := tool.NewOrchestrator(registry, checker, prompter, bus)
	engine := query.NewEngine(prov, registry, orchestrator, store, costTracker, bus, engineCfg)

	// 12. Run Engine
	ctx := cmd.Context()
	events := engine.Run(ctx, prompt)

	// 13. Consume events
	for ev := range events {
		switch e := ev.(type) {
		case query.TextEvent:
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			if cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			}
		case query.ToolResultEvent:
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
			}
		case query.TurnCompleteEvent:
			fmt.Println()
		case query.ErrorEvent:
			bus.Drain()
			return e.Err
		}
	}

	// 14. Drain EventBus
	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "total cost: $%.6f\n", costTracker.TotalUSD())
	}
	bus.Drain()
	return nil
}

func applyFlagOverrides(cmd *cobra.Command, cfg *config.Config) {
	if cmd.Flags().Changed("model") {
		cfg.Model, _ = cmd.Flags().GetString("model")
	}
	if cmd.Flags().Changed("provider") {
		cfg.Provider, _ = cmd.Flags().GetString("provider")
	}
	if cmd.Flags().Changed("api-key") {
		cfg.APIKey, _ = cmd.Flags().GetString("api-key")
	}
	if cmd.Flags().Changed("system-prompt") {
		cfg.SystemPrompt, _ = cmd.Flags().GetString("system-prompt")
	}
	if cmd.Flags().Changed("max-tokens") {
		cfg.MaxTokens, _ = cmd.Flags().GetInt("max-tokens")
	}
	if cmd.Flags().Changed("temperature") {
		t, _ := cmd.Flags().GetFloat64("temperature")
		cfg.Temperature = &t
	}
	if cmd.Flags().Changed("thinking") {
		enabled, _ := cmd.Flags().GetBool("thinking")
		if enabled {
			budget, _ := cmd.Flags().GetInt("thinking-budget")
			if budget == 0 {
				budget = 10000
			}
			cfg.Thinking = &config.ThinkingConfig{Enabled: true, BudgetTokens: budget}
		}
	}
	if cmd.Flags().Changed("verbose") {
		cfg.Verbose, _ = cmd.Flags().GetBool("verbose")
	}
	if cmd.Flags().Changed("record") {
		cfg.Record, _ = cmd.Flags().GetBool("record")
	}
}

func createProvider(cfg config.Config, bus *observe.EventBus) provider.Provider {
	switch cfg.Provider {
	case "anthropic":
		return anthropic.New(cfg.APIKey, bus)
	default:
		fmt.Fprintf(os.Stderr, "unknown provider %q, falling back to anthropic\n", cfg.Provider)
		return anthropic.New(cfg.APIKey, bus)
	}
}


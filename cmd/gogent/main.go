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
	"github.com/artpar/gogent/internal/session"
	"github.com/artpar/gogent/internal/sysprompt"
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
	root.Flags().String("resume", "", "resume session by ID")
	root.Flags().Bool("list-sessions", false, "list saved sessions")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runNonInteractive(cmd *cobra.Command, _ []string) error {
	// Handle --list-sessions early (no prompt or API key needed)
	listSessions, _ := cmd.Flags().GetBool("list-sessions")
	if listSessions {
		return runListSessions()
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

	// 2. Apply CLI flag overrides + defaults
	applyFlagOverrides(cmd, &cfg)
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

	// Determine if resuming or new session
	resumeID, _ := cmd.Flags().GetString("resume")
	prompt, _ := cmd.Flags().GetString("prompt")

	if resumeID == "" && prompt == "" {
		return fmt.Errorf("--prompt/-p is required (or use --resume to continue a session)")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("API key required: set --api-key or ANTHROPIC_API_KEY environment variable")
	}

	// 3. Create EventBus
	bus := observe.NewEventBus(1024)

	logLevel := observe.LevelError
	if cfg.Verbose {
		logLevel = observe.LevelTrace
	}
	logger := observe.NewLogger(os.Stderr, logLevel, observe.FormatText, nil)
	bus.Subscribe(logger)

	if cfg.Record {
		recorder, recErr := observe.NewRecorder("gogent-recording.jsonl")
		if recErr != nil {
			return fmt.Errorf("create recorder: %w", recErr)
		}
		defer recorder.Close()
		bus.Subscribe(recorder)
	}

	// 4. Create Provider
	prov := createProvider(cfg, bus)

	// 5. Create permission checker
	permEntries, permMode, _ := config.LoadPermissions(cwd)
	rules := permission.RulesFromConfigEntries(permEntries)
	mode := permission.PermissionMode(permMode)
	if mode == "" {
		mode = permission.ModeBypassPermissions
	}
	checker := permission.NewRuleChecker(rules, mode, cwd, bus)
	prompter := &permission.NonInteractivePrompter{}

	// 6. Build system prompt
	var sysPrompt model.SystemPrompt
	if cfg.SystemPrompt != "" {
		// CLI override replaces everything
		sysPrompt = model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: cfg.SystemPrompt, Cacheable: true}},
		}
	} else {
		builder := sysprompt.New(cwd, cfg.Model, bus)
		sysPrompt = builder.Build()
	}

	// 7. Create or resume conversation
	var conv model.Conversation
	if resumeID != "" {
		sessionStore, storeErr := session.NewStore()
		if storeErr != nil {
			return fmt.Errorf("open session store: %w", storeErr)
		}
		sess, loadErr := sessionStore.Load(resumeID)
		if loadErr != nil {
			return fmt.Errorf("resume session: %w", loadErr)
		}
		conv = sess.Conversation
		// Rebuild system prompt (fresh env info) unless original had a CLI override
		if sess.SystemOverride != "" {
			conv.System = model.SystemPrompt{
				Blocks: []model.SystemBlock{{Text: sess.SystemOverride, Cacheable: true}},
			}
		} else {
			conv.System = sysPrompt
		}
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "resumed session %s (%d messages)\n", resumeID, len(conv.Messages))
		}
	} else {
		conv = model.NewConversation(sysPrompt, cfg.Model, cfg.Provider, cwd)
	}

	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          cwd,
		Model:        cfg.Model,
		Provider:     cfg.Provider,
		MaxTokens:    cfg.MaxTokens,
		Temperature:  cfg.Temperature,
	})

	// 8. Create task registry and Engine config
	taskRegistry := task.NewRegistry(bus)
	costTracker := model.NewCostTracker()
	engineCfg := query.EngineConfig{
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
	}
	if cfg.Thinking != nil && cfg.Thinking.Enabled {
		engineCfg.Thinking = &provider.ThinkingConfig{
			Enabled:      cfg.Thinking.Enabled,
			BudgetTokens: cfg.Thinking.BudgetTokens,
		}
	}

	// 9. EngineFactory for sub-agents
	engineFactory := func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore) {
		subRegistry := tool.NewRegistry(bus)
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

	// 10. Create main Registry and register all tools
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

	// 11. Create Orchestrator and Engine
	orchestrator := tool.NewOrchestrator(registry, checker, prompter, bus)
	engine := query.NewEngine(prov, registry, orchestrator, store, costTracker, bus, engineCfg)

	// 12. Run engine
	ctx := cmd.Context()
	if prompt == "" {
		prompt = "Continue from where we left off."
	}
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

	// 14. Save session
	saveSession(store, costTracker, cfg.SystemPrompt, cwd)

	// 15. Drain EventBus
	if cfg.Verbose {
		fmt.Fprintf(os.Stderr, "total cost: $%.6f\n", costTracker.TotalUSD())
	}
	bus.Drain()
	return nil
}

func runListSessions() error {
	sessionStore, err := session.NewStore()
	if err != nil {
		return fmt.Errorf("open session store: %w", err)
	}
	summaries, err := sessionStore.List()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	if len(summaries) == 0 {
		fmt.Println("No saved sessions.")
		return nil
	}
	for _, s := range summaries {
		summary := s.Summary
		if len(summary) > 80 {
			summary = summary[:80] + "..."
		}
		fmt.Printf("%-38s  %s  %d turns  $%.4f  %s\n",
			s.ID, s.Model, s.TurnCount, s.CostUSD, summary)
	}
	return nil
}

func saveSession(store *app.StateStore, costTracker *model.CostTracker, systemOverride, cwd string) {
	sessionStore, err := session.NewStore()
	if err != nil {
		return // best-effort
	}
	snap := store.Snapshot()
	if len(snap.Conversation.Messages) == 0 {
		return
	}

	// Derive summary from first user message
	summary := ""
	for _, msg := range snap.Conversation.Messages {
		if msg.Role == model.RoleUser {
			for _, part := range msg.Content {
				if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
					summary = tp.Text
					if len(summary) > 100 {
						summary = summary[:100]
					}
					break
				}
			}
			break
		}
	}

	turnCount := 0
	for _, msg := range snap.Conversation.Messages {
		if msg.Role == model.RoleUser {
			turnCount++
		}
	}

	sess := session.Session{
		Conversation:   snap.Conversation,
		Summary:        summary,
		CostUSD:        costTracker.TotalUSD(),
		TurnCount:      turnCount,
		SystemOverride: systemOverride,
		GitRemote:      sysprompt.GitRemoteURL(cwd),
	}
	_ = sessionStore.Save(sess) // best-effort
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


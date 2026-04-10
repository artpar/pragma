package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/mcp"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/anthropic"
	groqprov "github.com/artpar/gogent/internal/provider/groq"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/session"
	"github.com/artpar/gogent/internal/sysprompt"
	"github.com/artpar/gogent/internal/task"
	"github.com/artpar/gogent/internal/tool"
	"github.com/artpar/gogent/internal/tui"
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
		RunE:  runDispatcher,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.Flags().StringP("prompt", "p", "", "prompt to send (non-interactive mode)")
	root.Flags().String("model", "", "model name")
	root.Flags().String("provider", "", "provider name (anthropic, groq)")
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

// runDispatcher routes to interactive TUI, non-interactive mode, or list-sessions.
func runDispatcher(cmd *cobra.Command, args []string) error {
	listSessions, _ := cmd.Flags().GetBool("list-sessions")
	if listSessions {
		return runListSessions()
	}

	prompt, _ := cmd.Flags().GetString("prompt")
	if prompt != "" {
		return runNonInteractive(cmd, args)
	}

	return runInteractive(cmd)
}

// deps holds all shared dependencies created by setupDeps.
type deps struct {
	cfg         config.Config
	bus         *observe.EventBus
	prov        provider.Provider
	checker     permission.Checker
	store       *app.StateStore
	registry    *tool.Registry
	costTracker *model.CostTracker
	engineCfg   query.EngineConfig
	taskReg     *task.Registry
	mcpManager  *mcp.Manager
	cwd         string
	cleanup     func() // close recorder, MCP servers, etc.
}

// setupDeps creates all shared dependencies from CLI flags and config.
// The prompter and orchestrator are NOT created here — they differ between
// interactive and non-interactive modes.
func setupDeps(cmd *cobra.Command) (*deps, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

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
		switch cfg.Provider {
		case "groq":
			cfg.APIKey = os.Getenv("GROQ_API_KEY")
		default:
			cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}
	if cfg.APIKey == "" {
		envVar := "ANTHROPIC_API_KEY"
		if cfg.Provider == "groq" {
			envVar = "GROQ_API_KEY"
		}
		return nil, fmt.Errorf("API key required: set --api-key or %s environment variable", envVar)
	}

	// EventBus
	bus := observe.NewEventBus(1024)
	logLevel := observe.LevelError
	if cfg.Verbose {
		logLevel = observe.LevelTrace
	}
	logger := observe.NewLogger(os.Stderr, logLevel, observe.FormatText, nil)
	bus.Subscribe(logger)

	var cleanupFn func()
	if cfg.Record {
		recorder, recErr := observe.NewRecorder("gogent-recording.jsonl")
		if recErr != nil {
			return nil, fmt.Errorf("create recorder: %w", recErr)
		}
		bus.Subscribe(recorder)
		cleanupFn = func() { recorder.Close() }
	}

	// Provider
	prov := createProvider(cfg, bus)

	// Permission checker
	permEntries, permMode, _ := config.LoadPermissions(cwd)
	rules := permission.RulesFromConfigEntries(permEntries)
	mode := permission.PermissionMode(permMode)
	if mode == "" {
		mode = permission.ModeBypassPermissions
	}
	checker := permission.NewRuleChecker(rules, mode, cwd, bus)

	// System prompt
	var sysPrompt model.SystemPrompt
	if cfg.SystemPrompt != "" {
		sysPrompt = model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: cfg.SystemPrompt, Cacheable: true}},
		}
	} else {
		builder := sysprompt.New(cwd, cfg.Model, bus)
		sysPrompt = builder.Build()
	}

	// Conversation (new or resumed)
	var conv model.Conversation
	resumeID, _ := cmd.Flags().GetString("resume")
	if resumeID != "" {
		sessionStore, storeErr := session.NewStore()
		if storeErr != nil {
			return nil, fmt.Errorf("open session store: %w", storeErr)
		}
		sess, loadErr := sessionStore.Load(resumeID)
		if loadErr != nil {
			return nil, fmt.Errorf("resume session: %w", loadErr)
		}
		conv = sess.Conversation
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

	// Task registry, cost tracker, engine config
	taskReg := task.NewRegistry(bus)
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

	// Registry (without Agent tool — added by caller who knows the prompter)
	registry := tool.NewRegistry(bus)

	// MCP manager
	mcpManager := mcp.NewManager(bus, registry)

	// Load MCP server configs (optional — not having any is fine)
	mcpServers, mcpErr := mcp.LoadConfig(cwd, bus)
	if mcpErr != nil {
		fmt.Fprintf(os.Stderr, "warning: load mcp config: %v\n", mcpErr)
	}

	// Connect MCP servers and register their tools
	if len(mcpServers) > 0 {
		connectCtx, connectCancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		connectErrs := mcpManager.ConnectAll(connectCtx, mcpServers)
		connectCancel()

		for name, err := range connectErrs {
			fmt.Fprintf(os.Stderr, "warning: mcp server %q: %v\n", name, err)
		}

		if err := mcpManager.RegisterTools(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: register mcp tools: %v\n", err)
		}
	}

	// Compose cleanup: MCP disconnect → bus drain → recorder close.
	// Order matters: MCP disconnect may emit final events, drain flushes
	// them to subscribers (including recorder), then recorder closes its file.
	compositeCleanup := func() {
		mcpManager.DisconnectAll()
		bus.Drain()
		if cleanupFn != nil {
			cleanupFn()
		}
	}

	return &deps{
		cfg:         cfg,
		bus:         bus,
		prov:        prov,
		checker:     checker,
		store:       store,
		registry:    registry,
		costTracker: costTracker,
		engineCfg:   engineCfg,
		taskReg:     taskReg,
		mcpManager:  mcpManager,
		cwd:         cwd,
		cleanup:     compositeCleanup,
	}, nil
}

// registerTools registers all tools on a registry. The agent tool needs the
// engine factory, which depends on the prompter — so it's built here.
func registerTools(d *deps, prompter permission.Prompter) (*query.Engine, error) {
	// Sub-agent engine factory
	engineFactory := func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore) {
		subRegistry := tool.NewRegistry(d.bus)
		for _, td := range baseTools(d) {
			_ = subRegistry.Register(td)
		}
		if scopedToolNames != nil {
			subRegistry = subRegistry.Scoped(scopedToolNames)
		}
		subStore := app.NewStateStore(app.AppState{
			Conversation: forkedConv,
			CWD:          d.cwd,
			Model:        d.cfg.Model,
			Provider:     d.cfg.Provider,
			MaxTokens:    d.cfg.MaxTokens,
			Temperature:  d.cfg.Temperature,
		})
		subOrch := tool.NewOrchestrator(subRegistry, d.checker, prompter, d.bus)
		subCfg := d.engineCfg
		if modelOverride != "" {
			subCfg.Model = modelOverride
		}
		return query.NewEngine(d.prov, subRegistry, subOrch, subStore, d.costTracker, d.bus, subCfg), subStore
	}

	// Register all tools including Agent
	for _, td := range baseTools(d) {
		if err := d.registry.Register(td); err != nil {
			return nil, fmt.Errorf("register tool %s: %w", td.Name(), err)
		}
	}
	agentTool := &toolagent.Tool{EngineFactory: engineFactory, Store: d.store, Tasks: d.taskReg, Bus: d.bus}
	if err := d.registry.Register(agentTool); err != nil {
		return nil, fmt.Errorf("register agent tool: %w", err)
	}

	// Create main orchestrator and engine
	orchestrator := tool.NewOrchestrator(d.registry, d.checker, prompter, d.bus)
	engine := query.NewEngine(d.prov, d.registry, orchestrator, d.store, d.costTracker, d.bus, d.engineCfg)
	return engine, nil
}

// baseTools returns all tool descriptors except Agent (which needs the engine factory).
func baseTools(d *deps) []tool.Descriptor {
	return []tool.Descriptor{
		&toolglob.Tool{},
		&toolgrep.Tool{},
		&toolfileread.Tool{},
		&toolfilewrite.Tool{},
		&toolfileedit.Tool{},
		&toolbash.Tool{},
		&toolnotebookedit.Tool{},
		&toolwebfetch.Tool{Provider: d.prov, Bus: d.bus, SecondaryModel: secondaryModelFor(d.cfg.Provider)},
		&tooltaskcreate.Tool{Tasks: d.taskReg},
		&tooltaskget.Tool{Tasks: d.taskReg},
		&tooltasklist.Tool{Tasks: d.taskReg},
		&tooltaskupdate.Tool{Tasks: d.taskReg},
		&tooltaskstop.Tool{Tasks: d.taskReg},
	}
}

// runInteractive launches the bubbletea TUI for multi-turn conversation.
func runInteractive(cmd *cobra.Command) error {
	d, err := setupDeps(cmd)
	if err != nil {
		return err
	}
	if d.cleanup != nil {
		defer d.cleanup()
	}

	prompter := tui.NewInteractivePrompter()
	engine, err := registerTools(d, prompter)
	if err != nil {
		return err
	}

	m := tui.New(tui.Config{
		Engine:      engine,
		Store:       d.store,
		CostTracker: d.costTracker,
		ModelName:   d.cfg.Model,
		Provider:    d.cfg.Provider,
		SessionSave: func() { saveSession(d.store, d.costTracker, d.cfg.SystemPrompt, d.cwd) },
	})

	program := tea.NewProgram(m, tea.WithAltScreen())
	prompter.SetProgram(program)

	if _, err := program.Run(); err != nil {
		return err
	}

	return nil
}

// runNonInteractive runs a single prompt and exits (original --prompt mode).
func runNonInteractive(cmd *cobra.Command, _ []string) error {
	d, err := setupDeps(cmd)
	if err != nil {
		return err
	}
	if d.cleanup != nil {
		defer d.cleanup()
	}

	prompter := &permission.NonInteractivePrompter{}
	engine, err := registerTools(d, prompter)
	if err != nil {
		return err
	}

	prompt, _ := cmd.Flags().GetString("prompt")
	resumeID, _ := cmd.Flags().GetString("resume")
	if resumeID != "" && prompt == "" {
		prompt = "Continue from where we left off."
	}

	ctx := cmd.Context()
	events := engine.Run(ctx, prompt)

	for ev := range events {
		switch e := ev.(type) {
		case query.TextEvent:
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			if d.cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			if d.cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			}
		case query.ToolResultEvent:
			if d.cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
			}
		case query.TurnCompleteEvent:
			fmt.Println()
		case query.ErrorEvent:
			saveSession(d.store, d.costTracker, d.cfg.SystemPrompt, d.cwd)
			return e.Err
		}
	}

	saveSession(d.store, d.costTracker, d.cfg.SystemPrompt, d.cwd)

	if d.cfg.Verbose {
		fmt.Fprintf(os.Stderr, "total cost: $%.6f\n", d.costTracker.TotalUSD())
	}
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
		return
	}
	snap := store.Snapshot()
	if len(snap.Conversation.Messages) == 0 {
		return
	}

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
	_ = sessionStore.Save(sess)
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

// secondaryModelFor returns the fast/cheap model for summarization tasks (e.g., WebFetch).
func secondaryModelFor(providerName string) string {
	switch providerName {
	case "groq":
		return "llama-3.3-70b-versatile"
	default:
		return "claude-haiku-4-5-20251001"
	}
}

func createProvider(cfg config.Config, bus *observe.EventBus) provider.Provider {
	switch cfg.Provider {
	case "anthropic":
		return anthropic.New(cfg.APIKey, bus)
	case "groq":
		return groqprov.New(cfg.APIKey, bus)
	default:
		fmt.Fprintf(os.Stderr, "unknown provider %q, falling back to anthropic\n", cfg.Provider)
		return anthropic.New(cfg.APIKey, bus)
	}
}

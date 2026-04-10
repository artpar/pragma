package cli

import (
	"context"
	"fmt"
	"os"
	"time"

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
)

// Deps holds all shared dependencies created by SetupDeps.
type Deps struct {
	Cfg         config.Config
	Bus         *observe.EventBus
	Prov        provider.Provider
	Checker     permission.Checker
	Store       *app.StateStore
	Registry    *tool.Registry
	CostTracker *model.CostTracker
	EngineCfg   query.EngineConfig
	TaskReg     *task.Registry
	McpManager  *mcp.Manager
	Cwd         string
	Cleanup     func()
}

// SetupDeps creates all shared dependencies from CLI flags and config.
// The prompter and orchestrator are NOT created here — they differ between
// interactive and non-interactive modes.
func SetupDeps(cmd *cobra.Command) (*Deps, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	ApplyFlagOverrides(cmd, &cfg)
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
	prov := CreateProvider(cfg, bus)

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

	// Registry (without Agent/Ask tools — added by caller who knows the prompter/asker)
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
	compositeCleanup := func() {
		mcpManager.DisconnectAll()
		bus.Drain()
		if cleanupFn != nil {
			cleanupFn()
		}
	}

	return &Deps{
		Cfg:         cfg,
		Bus:         bus,
		Prov:        prov,
		Checker:     checker,
		Store:       store,
		Registry:    registry,
		CostTracker: costTracker,
		EngineCfg:   engineCfg,
		TaskReg:     taskReg,
		McpManager:  mcpManager,
		Cwd:         cwd,
		Cleanup:     compositeCleanup,
	}, nil
}

// ApplyFlagOverrides applies CLI flag values to the config.
func ApplyFlagOverrides(cmd *cobra.Command, cfg *config.Config) {
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

// CreateProvider creates a provider from config.
func CreateProvider(cfg config.Config, bus *observe.EventBus) provider.Provider {
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

// SecondaryModelFor returns the fast/cheap model for summarization tasks.
func SecondaryModelFor(providerName string) string {
	switch providerName {
	case "groq":
		return "llama-3.3-70b-versatile"
	default:
		return "claude-haiku-4-5-20251001"
	}
}

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/cron"
	"github.com/artpar/gogent/internal/hook"
	"github.com/artpar/gogent/internal/lsp"
	"github.com/artpar/gogent/internal/mcp"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/anthropic"
	googleprov "github.com/artpar/gogent/internal/provider/google"
	groqprov "github.com/artpar/gogent/internal/provider/groq"
	oaiprov "github.com/artpar/gogent/internal/provider/openai"
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
	LspManager  *lsp.Manager
	CronSched   *cron.Scheduler
	HookMgr     *hook.Manager
	Metrics     *observe.Metrics
	Auditor     *observe.Auditor
	Cwd         string
	Cleanup     func()
}

// SetupDeps creates all shared dependencies from CLI flags and config.
// The prompter and orchestrator are NOT created here — they differ between
// interactive and non-interactive modes.
func SetupDeps(cmd *cobra.Command) (*Deps, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cwd, err := os.Getwd()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"get working directory: %w\", err)")
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	cfg, err := config.Load(cwd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"load config: %w\", err)")
		return nil, fmt.Errorf("load config: %w", err)
	}

	ApplyFlagOverrides(cmd, &cfg)
	if cfg.Provider == "" {
		observe.GlobalTrace("if: cfg.Provider == \"\"")
		cfg.Provider = "anthropic"
	}
	if cfg.Model == "" {
		observe.GlobalTrace("if: cfg.Model == \"\"")
		cfg.Model = DefaultModelFor(cfg.Provider)
	}
	if cfg.MaxTokens == 0 {
		observe.GlobalTrace("if: cfg.MaxTokens == 0")
		cfg.MaxTokens = 16384
	}
	if cfg.APIKey == "" {
		observe.GlobalTrace("if: cfg.APIKey == \"\"")
		switch cfg.Provider {
		case "groq":
			observe.GlobalTrace("case: \"groq\"")
			cfg.APIKey = os.Getenv("GROQ_API_KEY")
		case "openai":
			observe.GlobalTrace("case: \"openai\"")
			cfg.APIKey = os.Getenv("OPENAI_API_KEY")
		case "google":
			observe.GlobalTrace("case: \"google\"")
			cfg.APIKey = os.Getenv("GOOGLE_API_KEY")
		default:
			observe.GlobalTrace("default")
			cfg.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}
	if cfg.APIKey == "" {
		observe.GlobalTrace("if: cfg.APIKey == \"\"")
		envVar := "ANTHROPIC_API_KEY"
		switch cfg.Provider {
		case "groq":
			observe.GlobalTrace("case: \"groq\"")
			envVar = "GROQ_API_KEY"
		case "openai":
			observe.GlobalTrace("case: \"openai\"")
			envVar = "OPENAI_API_KEY"
		case "google":
			observe.GlobalTrace("case: \"google\"")
			envVar = "GOOGLE_API_KEY"
		}
		observe.GlobalTrace("return: nil, fmt.Errorf(\"API key required: set --api-key or %s environment variable\",...")
		return nil, fmt.Errorf("API key required: set --api-key or %s environment variable", envVar)
	}

	bus := observe.NewEventBus(1024)
	observe.SetGlobalBus(bus)

	if traceFilter := os.Getenv("GOGENT_TRACE_FILTER"); traceFilter != "" {
		observe.GlobalTrace("if: traceFilter != \"\"")
		observe.SetTraceFilter(observe.ParseTraceFilter(traceFilter))
	}

	logLevel := observe.LevelError
	if cfg.Verbose {
		observe.GlobalTrace("if: cfg.Verbose")
		logLevel = observe.LevelTrace
	}
	logger := observe.NewLogger(os.Stderr, logLevel, observe.FormatText, nil)
	bus.Subscribe(logger)

	// Per-execution log file: ~/.gogent/logs/<timestamp>.jsonl
	// Always enabled, captures everything at LevelTrace in JSON format.
	var cleanupFns []func()
	if gogentHome, homeErr := config.GogentHome(); homeErr == nil {
		observe.GlobalTrace("if: homeErr == nil")
		logsDir := filepath.Join(gogentHome, "logs")
		if mkErr := os.MkdirAll(logsDir, 0o755); mkErr == nil {
			observe.GlobalTrace("if: mkErr == nil")
			logFileName := time.Now().Format("2006-01-02T15-04-05") + ".jsonl"
			logFile, logErr := os.Create(filepath.Join(logsDir, logFileName))
			if logErr == nil {
				observe.GlobalTrace("if: logErr == nil")
				fileLogger := observe.NewLogger(logFile, observe.LevelTrace, observe.FormatJSON, nil)
				bus.Subscribe(fileLogger)
				cleanupFns = append(cleanupFns, func() { logFile.Close() })
			}
		}
	}

	metrics := observe.NewMetrics()
	bus.Subscribe(metrics)
	auditor := observe.NewAuditor()
	bus.Subscribe(auditor)

	if cfg.Record {
		observe.GlobalTrace("if: cfg.Record")
		recorder, recErr := observe.NewRecorder("gogent-recording.jsonl")
		if recErr != nil {
			observe.GlobalTrace("if: recErr != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"create recorder: %w\", recErr)")
			return nil, fmt.Errorf("create recorder: %w", recErr)
		}
		bus.Subscribe(recorder)
		cleanupFns = append(cleanupFns, func() { recorder.Close() })
	}

	prov, err := CreateProvider(cfg, bus)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	permEntries, permMode, _ := config.LoadPermissions(cwd)
	rules := permission.RulesFromConfigEntries(permEntries)
	// CLI --permission-mode overrides config file
	if cfg.PermissionMode != "" {
		permMode = cfg.PermissionMode
	}
	mode := permission.PermissionMode(permMode)
	if mode == "" {
		observe.GlobalTrace("if: mode == \"\"")
		mode = permission.ModeBypassPermissions
	}
	// Validate permission mode
	switch mode {
	case permission.ModeDefault, permission.ModeAcceptEdits, permission.ModeBypassPermissions, permission.ModeDontAsk:
		// valid
	default:
		return nil, fmt.Errorf("invalid permission mode %q: must be one of default, acceptEdits, bypassPermissions, dontAsk", mode)
	}
	checker := permission.NewRuleChecker(rules, mode, cwd, bus)

	hookMgr := hook.NewManager(cwd, "", bus)

	// System prompt
	var sysPrompt model.SystemPrompt
	if cfg.SystemPrompt != "" {
		observe.GlobalTrace("if: cfg.SystemPrompt != \"\"")
		sysPrompt = model.SystemPrompt{
			Blocks: []model.SystemBlock{{Text: cfg.SystemPrompt, Cacheable: true}},
		}
	} else {
		observe.GlobalTrace("else: cfg.SystemPrompt != \"\"")
		builder := sysprompt.New(cwd, cfg.Model, bus)
		sysPrompt = builder.Build()
	}
	// --append-system-prompt: append to (not replace) the system prompt
	appendPrompt, _ := cmd.Flags().GetString("append-system-prompt")
	if appendPrompt != "" {
		sysPrompt.Blocks = append(sysPrompt.Blocks, model.SystemBlock{Text: appendPrompt, Cacheable: true})
	}

	// Conversation (new or resumed)
	var conv model.Conversation
	resumeID, _ := cmd.Flags().GetString("resume")

	// --continue: resolve to most recent session ID for this directory
	continueFlag, _ := cmd.Flags().GetBool("continue")
	if continueFlag && resumeID == "" {
		sessionStore, storeErr := session.NewStore()
		if storeErr != nil {
			return nil, fmt.Errorf("open session store: %w", storeErr)
		}
		summaries, listErr := sessionStore.List()
		if listErr != nil {
			return nil, fmt.Errorf("list sessions: %w", listErr)
		}
		for _, s := range summaries {
			if s.WorkDir == cwd {
				resumeID = s.ID
				break
			}
		}
		if resumeID == "" {
			return nil, fmt.Errorf("no sessions found for current directory")
		}
	}

	if resumeID != "" {
		observe.GlobalTrace("if: resumeID != \"\"")
		if !session.IsValidSessionID(resumeID) {
			observe.GlobalTrace("if: !session.IsValidSessionID(resumeID)")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid session ID %q: must contain only alphanumeric charac...")
			return nil, fmt.Errorf("invalid session ID %q: must contain only alphanumeric characters and hyphens", resumeID)
		}
		sessionStore, storeErr := session.NewStore()
		if storeErr != nil {
			observe.GlobalTrace("if: storeErr != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"open session store: %w\", storeErr)")
			return nil, fmt.Errorf("open session store: %w", storeErr)
		}
		sess, loadErr := sessionStore.Load(resumeID)
		if loadErr != nil {
			observe.GlobalTrace("if: loadErr != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"resume session: %w\", loadErr)")
			return nil, fmt.Errorf("resume session: %w", loadErr)
		}
		conv = sess.Conversation
		if sess.SystemOverride != "" {
			observe.GlobalTrace("if: sess.SystemOverride != \"\"")
			conv.System = model.SystemPrompt{
				Blocks: []model.SystemBlock{{Text: sess.SystemOverride, Cacheable: true}},
			}
		} else {
			observe.GlobalTrace("else: sess.SystemOverride != \"\"")
			conv.System = sysPrompt
		}
		if cfg.Verbose {
			observe.GlobalTrace("if: cfg.Verbose")
			fmt.Fprintf(os.Stderr, "resumed session %s (%d messages)\n", resumeID, len(conv.Messages))
		}
	} else {
		observe.GlobalTrace("else: resumeID != \"\"")
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

	hookMgr.SetSessionID(conv.ID)

	taskReg := task.NewRegistry(bus)

	var cronSched *cron.Scheduler
	if gogentHome, homeErr := config.GogentHome(); homeErr == nil {
		observe.GlobalTrace("if: homeErr == nil")
		cronStore := cron.NewStore(filepath.Join(gogentHome, "scheduled_tasks.json"))
		cronSched = cron.NewScheduler(bus, cronStore)
	} else {
		observe.GlobalTrace("else: homeErr == nil")
		cronSched = cron.NewScheduler(bus, nil)
	}
	costTracker := model.NewCostTracker()
	engineCfg := query.EngineConfig{
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		MaxTurns:    cfg.MaxTurns,
		Temperature: cfg.Temperature,
	}
	if cfg.Thinking != nil && cfg.Thinking.Enabled {
		observe.GlobalTrace("if: cfg.Thinking != nil && cfg.Thinking.Enabled")
		engineCfg.Thinking = &provider.ThinkingConfig{
			Enabled:      cfg.Thinking.Enabled,
			BudgetTokens: cfg.Thinking.BudgetTokens,
		}
	}

	registry := tool.NewRegistry(bus)

	lspManager := lsp.NewManager(bus)
	lspConfigs, lspErr := lsp.LoadConfig(cwd, bus)
	if lspErr != nil {
		observe.GlobalTrace("if: lspErr != nil")
		fmt.Fprintf(os.Stderr, "warning: load lsp config: %v\n", lspErr)
	}
	if len(lspConfigs) > 0 {
		observe.GlobalTrace("if: len(lspConfigs) > 0")
		lspManager.Initialize(lspConfigs, cwd)
	}

	mcpManager := mcp.NewManager(bus, registry)

	mcpServers, mcpErr := mcp.LoadConfig(cwd, bus)
	if mcpErr != nil {
		observe.GlobalTrace("if: mcpErr != nil")
		fmt.Fprintf(os.Stderr, "warning: load mcp config: %v\n", mcpErr)
	}

	if len(mcpServers) > 0 {
		observe.GlobalTrace("if: len(mcpServers) > 0")
		connectCtx, connectCancel := context.WithTimeout(cmd.Context(), 60*time.Second)
		connectErrs := mcpManager.ConnectAll(connectCtx, mcpServers)
		connectCancel()

		for name, err := range connectErrs {
			observe.GlobalTrace("range connectErrs")
			fmt.Fprintf(os.Stderr, "warning: mcp server %q: %v\n", name, err)
		}

		if err := mcpManager.RegisterTools(cmd.Context()); err != nil {
			observe.GlobalTrace("if: err != nil")
			fmt.Fprintf(os.Stderr, "warning: register mcp tools: %v\n", err)
		}
	}

	compositeCleanup := func() {
		lspManager.Shutdown()
		mcpManager.DisconnectAll()
		bus.Drain()
		for _, fn := range cleanupFns {
			fn()
		}
	}
	observe.GlobalTrace("return: &Deps{\n\tCfg:\t\tcfg,\n\tBus:\t\tbus,\n\tProv:\t\tprov,\n\tChecker:\tchecker,\n\tStore:\t\tstor...")

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
		LspManager:  lspManager,
		CronSched:   cronSched,
		HookMgr:     hookMgr,
		Metrics:     metrics,
		Auditor:     auditor,
		Cwd:         cwd,
		Cleanup:     compositeCleanup,
	}, nil
}

// ApplyFlagOverrides applies CLI flag values to the config.
func ApplyFlagOverrides(cmd *cobra.Command, cfg *config.Config) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cmd.Flags().Changed("model") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"model\")")
		cfg.Model, _ = cmd.Flags().GetString("model")
	}
	if cmd.Flags().Changed("provider") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"provider\")")
		cfg.Provider, _ = cmd.Flags().GetString("provider")
	}
	if cmd.Flags().Changed("api-key") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"api-key\")")
		cfg.APIKey, _ = cmd.Flags().GetString("api-key")
	}
	if cmd.Flags().Changed("system-prompt") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"system-prompt\")")
		cfg.SystemPrompt, _ = cmd.Flags().GetString("system-prompt")
	}
	if cmd.Flags().Changed("max-tokens") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"max-tokens\")")
		cfg.MaxTokens, _ = cmd.Flags().GetInt("max-tokens")
	}
	if cmd.Flags().Changed("temperature") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"temperature\")")
		t, _ := cmd.Flags().GetFloat64("temperature")
		cfg.Temperature = &t
	}
	if cmd.Flags().Changed("thinking") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"thinking\")")
		enabled, _ := cmd.Flags().GetBool("thinking")
		if enabled {
			observe.GlobalTrace("if: enabled")
			budget, _ := cmd.Flags().GetInt("thinking-budget")
			if budget == 0 {
				observe.GlobalTrace("if: budget == 0")
				budget = 10000
			}
			cfg.Thinking = &config.ThinkingConfig{Enabled: true, BudgetTokens: budget}
		}
	}
	if cmd.Flags().Changed("verbose") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"verbose\")")
		cfg.Verbose, _ = cmd.Flags().GetBool("verbose")
	}
	if cmd.Flags().Changed("record") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"record\")")
		cfg.Record, _ = cmd.Flags().GetBool("record")
	}
	if cmd.Flags().Changed("max-turns") {
		cfg.MaxTurns, _ = cmd.Flags().GetInt("max-turns")
	}
	if cmd.Flags().Changed("permission-mode") {
		cfg.PermissionMode, _ = cmd.Flags().GetString("permission-mode")
	}
}

// CreateProvider creates a provider from config.
func CreateProvider(cfg config.Config, bus *observe.EventBus) (provider.Provider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch cfg.Provider {
	case "anthropic":
		observe.GlobalTrace("case: \"anthropic\"")
		return anthropic.New(cfg.APIKey, bus), nil
	case "groq":
		observe.GlobalTrace("case: \"groq\"")
		return groqprov.New(cfg.APIKey, bus)
	case "openai":
		observe.GlobalTrace("case: \"openai\"")
		var opts []oaiprov.Option
		if baseURL := os.Getenv("OPENAI_BASE_URL"); baseURL != "" {
			opts = append(opts, oaiprov.WithBaseURL(baseURL))
		}
		return oaiprov.New(cfg.APIKey, bus, opts...)
	case "google":
		observe.GlobalTrace("case: \"google\"")
		var opts []googleprov.Option
		if baseURL := os.Getenv("GOOGLE_BASE_URL"); baseURL != "" {
			opts = append(opts, googleprov.WithBaseURL(baseURL))
		}
		return googleprov.New(cfg.APIKey, bus, opts...)
	default:
		observe.GlobalTrace("default")
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
}

// DefaultModelFor returns the default primary model for a given provider.
func DefaultModelFor(providerName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch providerName {
	case "groq":
		observe.GlobalTrace("case: \"groq\"")
		return "llama-3.3-70b-versatile"
	case "openai":
		observe.GlobalTrace("case: \"openai\"")
		return "gpt-4o"
	case "google":
		observe.GlobalTrace("case: \"google\"")
		return "gemini-2.5-flash"
	default:
		observe.GlobalTrace("default")
		return "claude-sonnet-4-20250514"
	}
}

// SecondaryModelFor returns the fast/cheap model for summarization tasks.
func SecondaryModelFor(providerName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch providerName {
	case "groq":
		observe.GlobalTrace("case: \"groq\"")
		return "llama-3.3-70b-versatile"
	case "openai":
		observe.GlobalTrace("case: \"openai\"")
		return "gpt-4o-mini"
	case "google":
		observe.GlobalTrace("case: \"google\"")
		return "gemini-2.5-flash"
	default:
		observe.GlobalTrace("default")
		return "claude-haiku-4-5-20251001"
	}
}

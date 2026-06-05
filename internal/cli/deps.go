package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/cron"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/anthropic"
	googleprov "github.com/artpar/pragma/internal/provider/google"
	googlevertexprov "github.com/artpar/pragma/internal/provider/googlevertex"
	groqprov "github.com/artpar/pragma/internal/provider/groq"
	lilacprov "github.com/artpar/pragma/internal/provider/lilac"
	oaiprov "github.com/artpar/pragma/internal/provider/openai"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/sysprompt"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/toolset"
)

// Deps holds all shared dependencies created by SetupDeps.
type Deps struct {
	Cfg            config.Config
	Creds          config.Credentials
	Bus            *observe.EventBus
	StderrLogger   *observe.Logger
	Prov           provider.Provider
	Checker        permission.Checker
	Store          *app.StateStore
	Registry       *tool.Registry
	CostTracker    *model.CostTracker
	EngineCfg      query.EngineConfig
	TaskReg        *task.Registry
	Toolset        *toolset.Compiled
	McpManager     *mcp.Manager
	CronSched      *cron.Scheduler
	HookMgr        *hook.Manager
	Metrics        *observe.Metrics
	Auditor        *observe.Auditor
	TokenMonitor   *observe.TokenMonitor
	LogFilePath    string
	Cwd            string
	SessionStart   time.Time
	SessionHeader  session.HeaderData
	SessionWriter  *session.Writer
	SessionLastIdx int
	SessionStarted bool
	Cleanup        func()
}

// ProviderResolutionOptions lets utility commands share Pragma's provider
// selection without constructing the full agent runtime.
type ProviderResolutionOptions struct {
	DefaultProvider       string
	DefaultModel          string
	AllowProviderFallback bool
}

type ResolvedProviderConfig struct {
	Config           config.Config
	Credentials      config.Credentials
	Provider         string
	Model            string
	APIKey           string
	BaseURL          string
	ProviderExplicit bool
	ModelExplicit    bool
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

	resolved, err := ResolveProviderConfig(cmd, ProviderResolutionOptions{
		AllowProviderFallback: true,
	})
	if err != nil {
		return nil, err
	}
	cfg := resolved.Config
	creds := resolved.Credentials
	if cfg.MaxTokens == 0 {
		observe.GlobalTrace("if: cfg.MaxTokens == 0")
		cfg.MaxTokens = 16384
	}
	applyContextDefaults(&cfg)
	if cfg.ContextMode != model.ContextModeChat && cfg.ContextMode != model.ContextModeStateHandoff {
		observe.GlobalTrace("if: cfg.ContextMode != model.ContextModeChat && cfg.ContextMode != model.ContextM...")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid context mode %q\", cfg.ContextMode)")
		return nil, fmt.Errorf("invalid context mode %q", cfg.ContextMode)
	}
	if cfg.HandoffSchema != model.HandoffSchemaV1 {
		observe.GlobalTrace("if: cfg.HandoffSchema != model.HandoffSchemaV1")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"invalid handoff schema %q\", cfg.HandoffSchema)")
		return nil, fmt.Errorf("invalid handoff schema %q", cfg.HandoffSchema)
	}
	var activeToolset *toolset.Compiled
	if cfg.Toolset != "" {
		observe.GlobalTrace("if: cfg.Toolset != \"\"")
		activeToolset, err = toolset.Resolve(cwd, cfg.Toolset)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
	}

	bus := observe.NewEventBus(1024)
	observe.SetGlobalBus(bus)

	if traceFilter := os.Getenv("PRAGMA_TRACE_FILTER"); traceFilter != "" {
		observe.GlobalTrace("if: traceFilter != \"\"")
		observe.SetTraceFilter(observe.ParseTraceFilter(traceFilter))
	}

	logLevel := observe.LevelError
	if cfg.Verbose {
		observe.GlobalTrace("if: cfg.Verbose")
		logLevel = observe.LevelDebug
	}
	logger := observe.NewLogger(os.Stderr, logLevel, observe.FormatText, nil)

	// Per-execution log file: ~/.pragma/logs/<timestamp>.jsonl
	// Always enabled, captures everything at LevelTrace in JSON format.
	var cleanupFns []func()
	var logFilePath string
	if pragmaHome, homeErr := config.PragmaHome(); homeErr == nil {
		observe.GlobalTrace("if: homeErr == nil")
		logsDir := filepath.Join(pragmaHome, "logs")
		if mkErr := os.MkdirAll(logsDir, 0o755); mkErr == nil {
			observe.GlobalTrace("if: mkErr == nil")
			logFileName := time.Now().Format("2006-01-02T15-04-05") + ".jsonl"
			logFilePath = filepath.Join(logsDir, logFileName)
			logFile, logErr := os.Create(logFilePath)
			if logErr == nil {
				observe.GlobalTrace("if: logErr == nil")
				fileLogger := observe.NewLogger(logFile, observe.LevelTrace, observe.FormatJSON, nil)
				bus.Subscribe(fileLogger)
				cleanupFns = append(cleanupFns, func() { logFile.Close() })
			}
		}
	}

	auditor := observe.NewAuditor()
	bus.Subscribe(auditor)
	tokenMon := observe.NewTokenMonitor(bus, 200_000)
	bus.Subscribe(tokenMon)

	if cfg.Record {
		observe.GlobalTrace("if: cfg.Record")
		recorder, recErr := observe.NewRecorder("pragma-recording.jsonl")
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

	if cw, ok := prov.ContextWindow(cfg.Model); ok {
		observe.GlobalTrace("if: ok")
		tokenMon.SetBudget(cw)
	} else if os.Getenv("PRAGMA_CUSTOM_MODEL") == "" {
		observe.GlobalTrace("else-if: model not in registry, warn")
		fmt.Fprintf(os.Stderr, "Warning: model %q not in known registry for %s provider. It may still work if your provider supports it.\n", cfg.Model, cfg.Provider)
		if ml, ok := prov.(provider.ModelLister); ok {
			if models := ml.ListModels(); len(models) > 0 {
				fmt.Fprintf(os.Stderr, "Known models: %s\nRun /model to see available options.\n", strings.Join(models, ", "))
			}
		}
	}

	permEntries, permMode, _ := config.LoadPermissions(cwd)
	rules := permission.RulesFromConfigEntries(permEntries)

	if cfg.PermissionMode != "" {
		observe.GlobalTrace("if: cfg.PermissionMode != \"\"")
		permMode = cfg.PermissionMode
	}
	mode := permission.PermissionMode(permMode)
	if mode == "" {
		observe.GlobalTrace("if: mode == \"\"")
		mode = permission.ModeDefault
	}

	switch mode {
	case permission.ModeDefault, permission.ModeAcceptEdits, permission.ModeBypassPermissions, permission.ModeDontAsk:
		observe.GlobalTrace("case: permission.ModeDefault, permission.ModeAcceptEdits, permission.ModeBypassPerm...")

	default:
		observe.GlobalTrace("default")
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

	appendPrompt, _ := cmd.Flags().GetString("append-system-prompt")
	if appendPrompt != "" {
		observe.GlobalTrace("if: appendPrompt != \"\"")
		sysPrompt.Blocks = append(sysPrompt.Blocks, model.SystemBlock{Text: appendPrompt, Cacheable: true})
	}
	sysPrompt = applySystemPromptToolFilters(cmd, sysPrompt)

	// Conversation (new or resumed)
	var conv model.Conversation
	var resumedCost float64
	var resumedTokens model.TokenUsage
	var resumedHandoffState model.HandoffState
	var resumedContentReplacements []model.ContentReplacementRecord
	var sessionWriter *session.Writer
	var resumedTurnCount int
	var sessionStart time.Time
	var sessionHeader session.HeaderData
	var sessionLastIdx int
	resumeID, _ := cmd.Flags().GetString("resume")

	continueFlag, _ := cmd.Flags().GetBool("continue")
	if continueFlag && resumeID == "" {
		observe.GlobalTrace("if: continueFlag && resumeID == \"\"")
		sessionStore, storeErr := session.NewStore()
		if storeErr != nil {
			observe.GlobalTrace("if: storeErr != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"open session store: %w\", storeErr)")
			return nil, fmt.Errorf("open session store: %w", storeErr)
		}
		summaries, listErr := sessionStore.List()
		if listErr != nil {
			observe.GlobalTrace("if: listErr != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"list sessions: %w\", listErr)")
			return nil, fmt.Errorf("list sessions: %w", listErr)
		}
		for _, s := range summaries {
			observe.GlobalTrace("range summaries")
			if s.WorkDir == cwd {
				observe.GlobalTrace("if: s.WorkDir == cwd")
				resumeID = s.ID
				break
			}
		}
		if resumeID == "" {
			observe.GlobalTrace("if: resumeID == \"\"")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"no sessions found for current directory\")")
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
		resumedCost = sess.CostUSD
		resumedTokens = sess.TokenUsage
		resumedHandoffState = sess.HandoffState
		resumedContentReplacements = sess.ContentReplacements
		resumedTurnCount = sess.TurnCount
		sessionStart = sess.Conversation.CreatedAt
		if cfg.Verbose {
			observe.GlobalTrace("if: cfg.Verbose")
			fmt.Fprintf(os.Stderr, "resumed session %s (%d messages)\n", resumeID, len(conv.Messages))
		}

		sessionWriter, _ = sessionStore.Open(resumeID)
		sessionLastIdx = len(conv.Messages)
	} else {
		observe.GlobalTrace("else: resumeID != \"\"")
		conv = model.NewConversation(sysPrompt, cfg.Model, cfg.Provider, cwd)
		sessionStart = conv.CreatedAt
		sessionHeader = session.HeaderData{
			SessionID:      conv.ID,
			Model:          cfg.Model,
			Provider:       cfg.Provider,
			WorkDir:        cwd,
			GitRemote:      sysprompt.GitRemoteURL(cwd),
			SystemOverride: cfg.SystemPrompt,
			CreatedAt:      conv.CreatedAt,
			System:         sysPrompt,
		}
	}

	metrics := observe.NewMetrics(observe.MetricsSeed{TokenUsage: resumedTokens, TurnCount: resumedTurnCount})
	bus.Subscribe(metrics)

	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		HandoffState: resumedHandoffState,
		CWD:          cwd,
		Model:        cfg.Model,
		Provider:     cfg.Provider,
		MaxTokens:    cfg.MaxTokens,
		Temperature:  cfg.Temperature,
	})

	hookMgr.SetSessionID(conv.ID)

	taskReg := task.NewRegistry(bus)

	var cronSched *cron.Scheduler
	if pragmaHome, homeErr := config.PragmaHome(); homeErr == nil {
		observe.GlobalTrace("if: homeErr == nil")
		cronStore := cron.NewStore(filepath.Join(pragmaHome, "scheduled_tasks.json"))
		cronSched = cron.NewScheduler(bus, cronStore)
	} else {
		observe.GlobalTrace("else: homeErr == nil")
		cronSched = cron.NewScheduler(bus, nil)
	}
	costTracker := model.NewCostTracker(resumedCost)
	engineCfg := query.EngineConfig{
		Model:                     cfg.Model,
		MaxTokens:                 cfg.MaxTokens,
		MaxTurns:                  cfg.MaxTurns,
		ContextMode:               cfg.ContextMode,
		HandoffSchema:             cfg.HandoffSchema,
		StopAfterToolExec:         cfg.StopAfterToolExec,
		Temperature:               cfg.Temperature,
		ContentReplacementRecords: resumedContentReplacements,
	}
	engineCfg.RecordContentReplacements = func(records []model.ContentReplacementRecord) {
		if sessionWriter != nil {
			_ = sessionWriter.WriteContentReplacement(records)
		}
	}
	if cfg.Thinking != nil && cfg.Thinking.Enabled {
		observe.GlobalTrace("if: cfg.Thinking != nil && cfg.Thinking.Enabled")
		engineCfg.Thinking = &provider.ThinkingConfig{
			Enabled:      cfg.Thinking.Enabled,
			BudgetTokens: cfg.Thinking.BudgetTokens,
		}
	}

	registry := tool.NewRegistry(bus)

	mcpManager := mcp.NewManager(bus, registry)
	if activeToolset != nil {
		observe.GlobalTrace("if: activeToolset != nil")
		mcpManager.SetToolFilter(activeToolset.AllowMCPTool)
	}

	mcpServers, mcpErr := mcp.LoadConfig(cwd, bus)
	if mcpErr != nil {
		observe.GlobalTrace("if: mcpErr != nil")
		fmt.Fprintf(os.Stderr, "warning: load mcp config: %v\n", mcpErr)
	}
	if activeToolset != nil {
		observe.GlobalTrace("if: activeToolset != nil")
		mcpServers = activeToolset.FilterMCPServers(mcpServers)
	}

	var mcpCancel context.CancelFunc
	var mcpWG sync.WaitGroup
	if len(mcpServers) > 0 {
		observe.GlobalTrace("if: len(mcpServers) > 0")
		mcpManager.ConfigureServers(mcpServers)
		var mcpCtx context.Context
		mcpCtx, mcpCancel = context.WithCancel(cmd.Context())
		mcpWG.Add(1)
		go func() {
			defer mcpWG.Done()
			connectCtx, connectCancel := context.WithTimeout(mcpCtx, 60*time.Second)
			defer connectCancel()
			mcpManager.ConnectAllAndRegister(connectCtx, mcpServers)
		}()
	}

	watchdog := observe.NewMCPWatchdog(mcpManager.ServerStatus, bus, 30*time.Second)
	go watchdog.Start(cmd.Context())

	compositeCleanup := func() {
		if mcpCancel != nil {
			mcpCancel()
			done := make(chan struct{})
			go func() {
				mcpWG.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
			}
		}
		mcpManager.DisconnectAll()
		bus.Drain()
		for _, fn := range cleanupFns {
			fn()
		}
	}
	observe.GlobalTrace("return: &Deps{\n\tCfg:\t\tcfg,\n\tBus:\t\tbus,\n\tStderrLogger:\tlogger,\n\tProv:\t\tprov,\n\tChecker:...")
	observe.GlobalTrace("return: &Deps{\n\tCfg:\t\tcfg,\n\tCreds:\t\tcreds,\n\tBus:\t\tbus,\n\tStderrLogger:\tlogger,\n\tProv:\t...")

	return &Deps{
		Cfg:            cfg,
		Creds:          creds,
		Bus:            bus,
		StderrLogger:   logger,
		Prov:           prov,
		Checker:        checker,
		Store:          store,
		Registry:       registry,
		CostTracker:    costTracker,
		EngineCfg:      engineCfg,
		TaskReg:        taskReg,
		Toolset:        activeToolset,
		McpManager:     mcpManager,
		CronSched:      cronSched,
		HookMgr:        hookMgr,
		Metrics:        metrics,
		Auditor:        auditor,
		TokenMonitor:   tokenMon,
		LogFilePath:    logFilePath,
		Cwd:            cwd,
		SessionStart:   sessionStart,
		SessionHeader:  sessionHeader,
		SessionWriter:  sessionWriter,
		SessionLastIdx: sessionLastIdx,
		SessionStarted: false,
		Cleanup:        compositeCleanup,
	}, nil
}

func applySystemPromptToolFilters(cmd *cobra.Command, prompt model.SystemPrompt) model.SystemPrompt {
	if updatePlanAvailable(cmd) {
		return prompt
	}
	for i := range prompt.Blocks {
		prompt.Blocks[i].Text = stripUnavailableUpdatePlanGuidance(prompt.Blocks[i].Text)
	}
	return prompt
}

func updatePlanAvailable(cmd *cobra.Command) bool {
	allowedStr, _ := cmd.Flags().GetString("allowed-tools")
	if allowedStr != "" {
		allowed := parseToolList(allowedStr)
		found := false
		for _, name := range allowed {
			if name == "update_plan" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	disallowedStr, _ := cmd.Flags().GetString("disallowed-tools")
	if disallowedStr != "" {
		for _, name := range parseToolList(disallowedStr) {
			if name == "update_plan" {
				return false
			}
		}
	}
	return true
}

func stripUnavailableUpdatePlanGuidance(text string) string {
	text = strings.ReplaceAll(
		text,
		"- Communicate with the user by streaming thinking & responses, and by making & updating plans.\n",
		"- Communicate with the user by streaming thinking & responses.\n",
	)
	text = removeMarkdownSection(text, "## Planning", "## Task execution")
	text = removeMarkdownSection(text, "## `update_plan`", "")
	return text
}

func removeMarkdownSection(text, start, end string) string {
	startIdx := strings.Index(text, start)
	if startIdx < 0 {
		return text
	}
	endIdx := len(text)
	if end != "" {
		searchFrom := startIdx + len(start)
		if idx := strings.Index(text[searchFrom:], end); idx >= 0 {
			endIdx = searchFrom + idx
		}
	}
	return strings.TrimRight(text[:startIdx], "\n") + "\n\n" + strings.TrimLeft(text[endIdx:], "\n")
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
		observe.GlobalTrace("if: cmd.Flags().Changed(\"max-turns\")")
		cfg.MaxTurns, _ = cmd.Flags().GetInt("max-turns")
	}
	if cmd.Flags().Changed("context-mode") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"context-mode\")")
		cfg.ContextMode, _ = cmd.Flags().GetString("context-mode")
	}
	if cmd.Flags().Changed("handoff-schema") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"handoff-schema\")")
		cfg.HandoffSchema, _ = cmd.Flags().GetString("handoff-schema")
	}
	if cmd.Flags().Changed("stop-after-tool-exec") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"stop-after-tool-exec\")")
		cfg.StopAfterToolExec, _ = cmd.Flags().GetBool("stop-after-tool-exec")
	}
	if cmd.Flags().Changed("permission-mode") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"permission-mode\")")
		cfg.PermissionMode, _ = cmd.Flags().GetString("permission-mode")
	}
	if cmd.Flags().Changed("toolset") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"toolset\")")
		cfg.Toolset, _ = cmd.Flags().GetString("toolset")
	}
}

// ResolveProviderConfig applies the same provider/model/API-key selection used
// by normal runs. Utility commands can pass defaults discovered from their own
// inputs, then still let global flags and settings override them.
func ResolveProviderConfig(cmd *cobra.Command, opts ProviderResolutionOptions) (ResolvedProviderConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cwd, err := os.Getwd()
	if err != nil {
		return ResolvedProviderConfig{}, fmt.Errorf("get working directory: %w", err)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return ResolvedProviderConfig{}, fmt.Errorf("load config: %w", err)
	}
	creds, _ := config.LoadCredentials()

	ApplyFlagOverrides(cmd, &cfg)
	providerExplicit := cmd.Flags().Changed("provider")
	modelExplicit := cmd.Flags().Changed("model")

	if cfg.Provider == "" && opts.DefaultProvider != "" {
		cfg.Provider = opts.DefaultProvider
	}
	if cfg.Provider == "" {
		cfg.Provider = autoDetectProvider(creds)
	}
	if cfg.Provider == "" {
		cfg.Provider = "lilac"
	}
	if cfg.Model == "" && opts.DefaultModel != "" {
		cfg.Model = opts.DefaultModel
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModelFor(cfg.Provider)
	}
	cfg.Model = resolveModelAlias(cfg.Provider, cfg.Model)

	if cfg.Provider == "google-vertex" {
		observe.GlobalTrace("if: cfg.Provider == \"google-vertex\" (skip API key check)")
	} else if cfg.APIKey == "" {
		observe.GlobalTrace("if: cfg.APIKey == \"\" (try credentials.yml)")
		cfg.APIKey = creds.CredentialFor(cfg.Provider).APIKey
	}
	if cfg.Provider != "google-vertex" && cfg.APIKey == "" {
		observe.GlobalTrace("if: cfg.APIKey == \"\" (try env var)")
		cfg.APIKey = os.Getenv(envVarForProvider(cfg.Provider))
	}
	if cfg.Provider != "google-vertex" && cfg.APIKey == "" && opts.AllowProviderFallback {
		observe.GlobalTrace("if: cfg.APIKey == \"\" (try provider picker)")
		selected, selErr := pickAvailableProvider(cfg.Provider, creds)
		if selErr != nil {
			return ResolvedProviderConfig{}, selErr
		}
		cfg.Provider = selected.name
		cfg.APIKey = selected.apiKey
		cfg.Model = DefaultModelFor(cfg.Provider)
	}

	return ResolvedProviderConfig{
		Config:           cfg,
		Credentials:      creds,
		Provider:         cfg.Provider,
		Model:            cfg.Model,
		APIKey:           cfg.APIKey,
		BaseURL:          ProviderBaseURL(cfg.Provider, creds),
		ProviderExplicit: providerExplicit,
		ModelExplicit:    modelExplicit,
	}, nil
}

func applyContextDefaults(cfg *config.Config) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cfg.ContextMode == "" {
		observe.GlobalTrace("if: cfg.ContextMode == \"\"")
		cfg.ContextMode = model.ContextModeStateHandoff
	}
	if cfg.HandoffSchema == "" {
		observe.GlobalTrace("if: cfg.HandoffSchema == \"\"")
		cfg.HandoffSchema = model.HandoffSchemaV1
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
		if baseURL := resolveBaseURL("OPENAI_BASE_URL", "openai"); baseURL != "" {
			opts = append(opts, oaiprov.WithBaseURL(baseURL))
		}
		return oaiprov.New(cfg.APIKey, bus, opts...)
	case "google":
		observe.GlobalTrace("case: \"google\"")
		var opts []googleprov.Option
		if baseURL := resolveBaseURL("GOOGLE_BASE_URL", "google"); baseURL != "" {
			opts = append(opts, googleprov.WithBaseURL(baseURL))
		}
		return googleprov.New(cfg.APIKey, bus, opts...)
	case "google-vertex":
		observe.GlobalTrace("case: \"google-vertex\"")
		return googlevertexprov.New(cfg.VertexProjectID, cfg.VertexLocation, cfg.VertexEndpointID, cfg.VertexDomain, bus)
	case "lilac":
		observe.GlobalTrace("case: \"lilac\"")
		var opts []lilacprov.Option
		if baseURL := resolveBaseURL("LILAC_BASE_URL", "lilac"); baseURL != "" {
			opts = append(opts, lilacprov.WithBaseURL(baseURL))
		}
		return lilacprov.New(cfg.APIKey, bus, opts...)
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
	case "lilac":
		observe.GlobalTrace("case: \"lilac\"")
		return "minimaxai/minimax-m2.7"
	default:
		observe.GlobalTrace("default")
		return "claude-sonnet-4-6-20250514"
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
	case "lilac":
		observe.GlobalTrace("case: \"lilac\"")
		return "minimaxai/minimax-m2.7"
	default:
		observe.GlobalTrace("default")
		return "claude-haiku-4-5-20251001"
	}
}

// modelAliases maps short user-friendly names to full model IDs per provider.
// Avoids GitHub issues #18873, #16387, #6169, #5349 where short names
// like "haiku" or "sonnet" get sent raw to APIs causing 404 errors.
var modelAliases = map[string]map[string]string{
	"anthropic": {
		"sonnet": "claude-sonnet-4-6-20250514",
		"opus":   "claude-opus-4-6-20250610",
		"haiku":  "claude-haiku-4-5-20251001",
	},
	"google": {
		"flash": "gemini-2.5-flash",
		"pro":   "gemini-2.5-pro",
	},
	"lilac": {
		"gemma": "google/gemma-4-31b-it",
		"glm":   "zai-org/glm-5.1",
		"k2.5":  "moonshotai/kimi-k2.5",
		"kimi":  "moonshotai/kimi-k2.6",
	},
}

// resolveModelAlias resolves short model aliases to full model IDs.
// If the input is not a known alias, it is returned unchanged.
func resolveModelAlias(providerName, modelInput string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(strings.TrimSpace(modelInput))
	if aliases, ok := modelAliases[providerName]; ok {
		observe.GlobalTrace("if: ok")
		if resolved, ok := aliases[lower]; ok {
			observe.GlobalTrace("resolved alias " + lower + " → " + resolved)
			observe.GlobalTrace("return: resolved")
			return resolved
		}
	}
	observe.GlobalTrace("return: modelInput (no alias)")
	observe.GlobalTrace("return: modelInput")
	return modelInput
}

// resolveBaseURL checks env var first, then credentials.yml.
func resolveBaseURL(envVar, provider string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if v := os.Getenv(envVar); v != "" {
		observe.GlobalTrace("if: v != \"\"")
		observe.GlobalTrace("return: v")
		return v
	}
	creds, err := config.LoadCredentials()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: creds.CredentialFor(provider).BaseURL")
	return creds.CredentialFor(provider).BaseURL
}

// ProviderBaseURL resolves the provider's OpenAI-compatible base URL for direct
// HTTP utilities. It follows provider-specific env vars, credentials, then a
// known default where one exists.
func ProviderBaseURL(provider string, creds config.Credentials) string {
	envVar := baseURLEnvVarForProvider(provider)
	if envVar != "" {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
	}
	if v := creds.CredentialFor(provider).BaseURL; v != "" {
		return v
	}
	return DefaultBaseURLFor(provider)
}

func DefaultBaseURLFor(provider string) string {
	switch provider {
	case "lilac":
		return "https://api.getlilac.com/v1"
	case "openai":
		return "https://api.openai.com/v1"
	case "groq":
		return "https://api.groq.com/openai/v1"
	default:
		return ""
	}
}

func baseURLEnvVarForProvider(provider string) string {
	switch provider {
	case "lilac":
		return "LILAC_BASE_URL"
	case "openai":
		return "OPENAI_BASE_URL"
	case "google":
		return "GOOGLE_BASE_URL"
	default:
		return ""
	}
}

type selectedProvider struct {
	name   string
	apiKey string
}

// knownProviders is the list of all supported provider names.
var knownProviders = []string{"anthropic", "openai", "google", "google-vertex", "groq", "lilac"}

// pickAvailableProvider collects providers that have an API key (from credentials
// or env vars) and either auto-selects or prompts the user to choose.
func pickAvailableProvider(defaultProv string, creds config.Credentials) (selectedProvider, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	var available []selectedProvider
	seen := map[string]bool{}

	for name, cred := range creds.Providers {
		observe.GlobalTrace("range creds.Providers")
		if cred.APIKey != "" && !seen[name] {
			observe.GlobalTrace("if: cred.APIKey != \"\" && !seen[name]")
			available = append(available, selectedProvider{name: name, apiKey: cred.APIKey})
			seen[name] = true
		}
	}

	for _, name := range knownProviders {
		observe.GlobalTrace("range knownProviders")
		if seen[name] {
			observe.GlobalTrace("if: seen[name]")
			continue
		}
		if key := os.Getenv(envVarForProvider(name)); key != "" {
			observe.GlobalTrace("if: key != \"\"")
			available = append(available, selectedProvider{name: name, apiKey: key})
			seen[name] = true
		}
	}

	if len(available) == 0 {
		observe.GlobalTrace("if: len(available) == 0")
		observe.GlobalTrace("return: selectedProvider{}, fmt.Errorf(\"API key required: set --api-key, add to ~/.pr...")
		return selectedProvider{}, fmt.Errorf("API key required: set --api-key, add to ~/.pragma/credentials.yml, or set %s", envVarForProvider(defaultProv))
	}

	sort.Slice(available, func(i, j int) bool {
		return available[i].name < available[j].name
	})

	if len(available) == 1 {
		observe.GlobalTrace("if: len(available) == 1")
		fmt.Fprintf(os.Stderr, "No API key for %s, using %s\n", defaultProv, available[0].name)
		observe.GlobalTrace("return: available[0], nil")
		return available[0], nil
	}

	if !term.IsTerminal(int(os.Stdin.Fd())) {
		observe.GlobalTrace("if: !term.IsTerminal(int(os.Stdin.Fd()))")
		names := make([]string, len(available))
		for i, p := range available {
			observe.GlobalTrace("range available")
			names[i] = p.name
		}
		observe.GlobalTrace("return: selectedProvider{}, fmt.Errorf(\"API key required for %s. Available providers:...")
		return selectedProvider{}, fmt.Errorf("API key required for %s. Available providers: %v (use --provider to select)", defaultProv, names)
	}

	fmt.Fprintf(os.Stderr, "No API key found for %s.\nAvailable providers:\n", defaultProv)
	for i, p := range available {
		observe.GlobalTrace("range available")
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, p.name)
	}
	fmt.Fprintf(os.Stderr, "Select provider [1-%d]: ", len(available))

	var choice int
	if _, err := fmt.Fscanf(os.Stdin, "%d", &choice); err != nil || choice < 1 || choice > len(available) {
		observe.GlobalTrace("if: err != nil || choice < 1 || choice > len(available)")
		observe.GlobalTrace("return: selectedProvider{}, fmt.Errorf(\"invalid selection\")")
		return selectedProvider{}, fmt.Errorf("invalid selection")
	}
	observe.GlobalTrace("return: available[choice-1], nil")
	return available[choice-1], nil
}

// autoDetectProvider detects the provider based on available credentials in priority order.
func autoDetectProvider(creds config.Credentials) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	priority := []struct {
		name   string
		envVar string
	}{
		{"lilac", "LILAC_API_KEY"},
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"google", "GOOGLE_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"groq", "GROQ_API_KEY"},
	}

	for _, p := range priority {
		observe.GlobalTrace("range priority")
		if os.Getenv(p.envVar) != "" {
			observe.GlobalTrace("detected from env: " + p.name)
			observe.GlobalTrace("return: p.name")
			return p.name
		}
		if creds.CredentialFor(p.name).APIKey != "" {
			observe.GlobalTrace("detected from creds: " + p.name)
			observe.GlobalTrace("return: p.name")
			return p.name
		}
	}

	observe.GlobalTrace("return: \"\" (none detected)")
	observe.GlobalTrace("return: \"\"")
	return ""
}

// envVarForProvider returns the environment variable name for a provider's API key.
func envVarForProvider(provider string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch provider {
	case "groq":
		observe.GlobalTrace("case: \"groq\"")
		return "GROQ_API_KEY"
	case "openai":
		observe.GlobalTrace("case: \"openai\"")
		return "OPENAI_API_KEY"
	case "google":
		observe.GlobalTrace("case: \"google\"")
		return "GOOGLE_API_KEY"
	case "lilac":
		observe.GlobalTrace("case: \"lilac\"")
		return "LILAC_API_KEY"
	default:
		observe.GlobalTrace("default")
		return "ANTHROPIC_API_KEY"
	}
}

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/background"
	"github.com/artpar/pragma/internal/buildinfo"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/sysprompt"
	"github.com/artpar/pragma/internal/tool"
	toolsynthetic "github.com/artpar/pragma/internal/tools/synthetic"
	"github.com/artpar/pragma/internal/tui"
)

// RunDispatcher routes to interactive TUI, non-interactive mode, background, or list-sessions.
func RunDispatcher(cmd *cobra.Command, args []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	if bgFlag, _ := cmd.Flags().GetBool("bg"); bgFlag {
		observe.GlobalTrace("if: bgFlag")
		observe.GlobalTrace("return: RunBackground(cmd)")
		return RunBackground(cmd)
	}

	listSessions, _ := cmd.Flags().GetBool("list-sessions")
	if listSessions {
		observe.GlobalTrace("if: listSessions")
		observe.GlobalTrace("return: RunListSessions()")
		return RunListSessions()
	}

	prompt, _ := cmd.Flags().GetString("prompt")
	if prompt != "" {
		observe.GlobalTrace("if: prompt != \"\"")
		observe.GlobalTrace("return: RunNonInteractive(cmd, args)")
		return RunNonInteractive(cmd, args)
	}
	observe.GlobalTrace("return: RunInteractive(cmd)")

	return RunInteractive(cmd)
}

// RunBackground spawns a detached child process to run the session in the background.
// The child process runs RunNonInteractive with --bg stripped from args.
// Uses Setsid to create a new process group for clean orphan cleanup.
func RunBackground(cmd *cobra.Command) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	prompt, _ := cmd.Flags().GetString("prompt")
	if prompt == "" {
		observe.GlobalTrace("if: prompt == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"background mode requires --prompt flag\")")
		return fmt.Errorf("background mode requires --prompt flag")
	}

	gogentHome, err := config.PragmaHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"resolve gogent home: %w\", err)")
		return fmt.Errorf("resolve gogent home: %w", err)
	}
	logsDir := filepath.Join(gogentHome, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"create logs directory: %w\", err)")
		return fmt.Errorf("create logs directory: %w", err)
	}
	logFileName := fmt.Sprintf("bg-%s.log", time.Now().Format("2006-01-02T15-04-05"))
	logPath := filepath.Join(logsDir, logFileName)

	logFile, err := os.Create(logPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"create log file: %w\", err)")
		return fmt.Errorf("create log file: %w", err)
	}

	childArgs := []string{os.Args[0]}
	addStringFlag := func(name string) {
		if v, _ := cmd.Flags().GetString(name); v != "" {
			childArgs = append(childArgs, "--"+name+"="+v)
		}
	}
	addIntFlag := func(name string) {
		if cmd.Flags().Changed(name) {
			v, _ := cmd.Flags().GetInt(name)
			childArgs = append(childArgs, fmt.Sprintf("--%s=%d", name, v))
		}
	}
	addBoolFlag := func(name string) {
		if v, _ := cmd.Flags().GetBool(name); v {
			childArgs = append(childArgs, "--"+name)
		}
	}
	addStringFlag("prompt")
	addStringFlag("provider")
	addStringFlag("model")
	addStringFlag("api-key")
	addStringFlag("system-prompt")
	addStringFlag("append-system-prompt")
	addStringFlag("allowed-tools")
	addStringFlag("disallowed-tools")
	addStringFlag("permission-mode")
	addStringFlag("output-schema")
	addIntFlag("max-tokens")
	addIntFlag("max-turns")
	addIntFlag("thinking-budget")
	addBoolFlag("thinking")
	addBoolFlag("verbose")
	addBoolFlag("record")
	if cmd.Flags().Changed("temperature") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"temperature\")")
		v, _ := cmd.Flags().GetFloat64("temperature")
		childArgs = append(childArgs, fmt.Sprintf("--temperature=%g", v))
	}

	env := append(os.Environ(),
		"PRAGMA_BG_SESSION=1",
		"PRAGMA_BG_SESSION_LOG="+logPath,
	)

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		logFile.Close()
		observe.GlobalTrace("return: fmt.Errorf(\"open %s: %w\", os.DevNull, err)")
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}

	proc, err := os.StartProcess(childArgs[0], childArgs, &os.ProcAttr{
		Dir:   "",
		Env:   env,
		Files: []*os.File{devNull, logFile, logFile},
		Sys:   daemonSysProcAttr(),
	})
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		devNull.Close()
		logFile.Close()
		observe.GlobalTrace("return: fmt.Errorf(\"start background process: %w\", err)")
		return fmt.Errorf("start background process: %w", err)
	}

	childPid := proc.Pid
	_ = proc.Release()
	devNull.Close()
	logFile.Close()

	reg, regErr := background.NewRegistry()
	if regErr == nil {
		observe.GlobalTrace("if: regErr == nil")
		promptDisplay := prompt
		if len(promptDisplay) > 200 {
			observe.GlobalTrace("if: len(promptDisplay) > 200")
			promptDisplay = promptDisplay[:200]
		}
		cwd, _ := os.Getwd()
		modelName, _ := cmd.Flags().GetString("model")
		providerName, _ := cmd.Flags().GetString("provider")

		_ = reg.Register(background.ProcessInfo{
			PID:       childPid,
			PGID:      childPid,
			SessionID: "",
			CWD:       cwd,
			StartedAt: time.Now(),
			Status:    background.StatusStarting,
			LogPath:   logPath,
			Model:     modelName,
			Provider:  providerName,
			Prompt:    promptDisplay,
		})
	}

	fmt.Printf("Background session started (PID %d)\n", childPid)
	fmt.Printf("  Logs: %s\n", logPath)
	fmt.Printf("  Use 'gogent sessions' to manage.\n")
	observe.GlobalTrace("return: nil")
	return nil
}

// RunInteractive launches the bubbletea TUI for multi-turn conversation.
func RunInteractive(cmd *cobra.Command) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d, err := SetupDeps(cmd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if d.Cleanup != nil {
		observe.GlobalTrace("if: d.Cleanup != nil")
		defer d.Cleanup()
	}

	snap := d.Store.Snapshot()
	d.Bus.Emit(observe.SessionStarted{
		EventHeader: observe.NewEventHeader("SessionStarted", "", "", ""),
		SessionID:   snap.Conversation.ID,
	})

	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		d.HookMgr.Execute(cmd.Context(), hook.SessionStart, hook.HookInput{})
	}

	prompter := tui.NewInteractivePrompter()
	asker := tui.NewInteractiveAsker()
	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	applyToolFilters(cmd, d.Registry)

	compDeps, compactor := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	sessionSaveFn := func() { SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd) }

	slashCmds := slash.NewRegistry()

	skillLoader := skill.NewLoader(d.Cwd)
	if skills, err := skillLoader.LoadAll(); err == nil {
		observe.GlobalTrace("if: err == nil")
		for _, s := range skills {
			observe.GlobalTrace("range skills")
			slashCmds.Register(slash.Command{
				Name:        s.Name,
				Description: s.Description,
			})
		}
	}

	slashDeps := slash.Deps{
		Store:       d.Store,
		CostTracker: d.CostTracker,
		Compactor:   compactor,
		Bus:         d.Bus,
		SessionSave: sessionSaveFn,
		ModelName:   d.Cfg.Model,
		Provider:    d.Cfg.Provider,
		Cwd:         d.Cwd,
	}

	m := tui.New(tui.Config{
		ParentCtx:    cmd.Context(),
		Engine:       engine,
		Store:        d.Store,
		CostTracker:  d.CostTracker,
		ModelName:    d.Cfg.Model,
		Provider:     d.Cfg.Provider,
		SessionSave:  sessionSaveFn,
		SlashCmds:    slashCmds,
		SlashDeps:    slashDeps,
		HookMgr:      d.HookMgr,
		TokenMonitor: d.TokenMonitor,
		Metrics:      d.Metrics,
		Workspace:    d.Cwd,
		Version:      buildinfo.Version,
	})

	program := tea.NewProgram(m, tea.WithAltScreen())
	prompter.SetProgram(program)
	asker.SetProgram(program)

	defer func() {
		if d.HookMgr != nil {
			observe.GlobalTrace("if: d.HookMgr != nil")
			d.HookMgr.Execute(cmd.Context(), hook.SessionEnd, hook.HookInput{})
		}
	}()

	if _, err := program.Run(); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	observe.GlobalTrace("return: nil")

	return nil
}

// RunNonInteractive runs a single prompt and exits.
func RunNonInteractive(cmd *cobra.Command, _ []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d, err := SetupDeps(cmd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if d.Cleanup != nil {
		observe.GlobalTrace("if: d.Cleanup != nil")
		defer d.Cleanup()
	}
	d.Bus.Subscribe(d.StderrLogger)

	if os.Getenv("PRAGMA_BG_SESSION") == "1" {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_BG_SESSION\") == \"1\"")
		if reg, regErr := background.NewRegistry(); regErr == nil {
			observe.GlobalTrace("if: regErr == nil")
			defer reg.Unregister(os.Getpid())
			sub := background.NewStatusSubscriber(reg)
			d.Bus.Subscribe(sub)
		}
	}

	snap := d.Store.Snapshot()
	d.Bus.Emit(observe.SessionStarted{
		EventHeader: observe.NewEventHeader("SessionStarted", "", "", ""),
		SessionID:   snap.Conversation.ID,
	})

	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		d.HookMgr.Execute(cmd.Context(), hook.SessionStart, hook.HookInput{})
	}

	defer func() {
		if d.HookMgr != nil {
			observe.GlobalTrace("if: d.HookMgr != nil")
			d.HookMgr.Execute(cmd.Context(), hook.SessionEnd, hook.HookInput{})
		}
	}()

	if !cmd.Flags().Changed("permission-mode") {
		observe.GlobalTrace("if: !cmd.Flags().Changed(\"permission-mode\")")
		d.Checker = permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}

	schemaFlag, _ := cmd.Flags().GetString("output-schema")
	if schemaFlag != "" {
		observe.GlobalTrace("if: schemaFlag != \"\"")
		schemaJSON, err := loadOutputSchema(schemaFlag)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"invalid output schema: %w\", err)")
			return fmt.Errorf("invalid output schema: %w", err)
		}
		synTool, err := toolsynthetic.New(schemaJSON)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"create StructuredOutput tool: %w\", err)")
			return fmt.Errorf("create StructuredOutput tool: %w", err)
		}
		if err := d.Registry.Register(synTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"register StructuredOutput tool: %w\", err)")
			return fmt.Errorf("register StructuredOutput tool: %w", err)
		}
	}

	applyToolFilters(cmd, d.Registry)

	compDeps, _ := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	prompt, _ := cmd.Flags().GetString("prompt")
	resumeID, _ := cmd.Flags().GetString("resume")
	if resumeID != "" && prompt == "" {
		observe.GlobalTrace("if: resumeID != \"\" && prompt == \"\"")
		prompt = "Continue from where we left off."
	}

	ctx := cmd.Context()
	events := engine.Run(ctx, prompt)

	hasStructuredOutput := schemaFlag != ""
	var structuredJSON json.RawMessage

	out := os.Stdout
	if bgLog := os.Getenv("PRAGMA_BG_SESSION_LOG"); bgLog != "" {
		observe.GlobalTrace("if: bgLog != \"\"")
		if f, err := os.OpenFile(bgLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644); err == nil {
			observe.GlobalTrace("if: err == nil")
			out = f
			defer f.Close()
		}
	}

	for ev := range events {
		observe.GlobalTrace("range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.GlobalTrace("typecase: query.TextEvent")
			if !hasStructuredOutput {
				fmt.Fprint(out, e.Text)
			}
		case query.ThinkingEvent:
			observe.GlobalTrace("typecase: query.ThinkingEvent")
			if d.Cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			observe.GlobalTrace("typecase: query.ToolCallEvent")
			if hasStructuredOutput && e.Call.Name == "StructuredOutput" {
				structuredJSON = e.Call.Input
			}
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			}
		case query.ToolResultEvent:
			observe.GlobalTrace("typecase: query.ToolResultEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
			}
		case query.CompactionEvent:
			observe.GlobalTrace("typecase: query.CompactionEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[auto-compacted: %d → %d tokens]\n", e.PreTokens, e.PostTokens)
			}
		case query.TurnCompleteEvent:
			observe.GlobalTrace("typecase: query.TurnCompleteEvent")
			if !hasStructuredOutput {
				fmt.Fprintln(out)
			}
		case query.ErrorEvent:
			observe.GlobalTrace("typecase: query.ErrorEvent")
			SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd)
			return e.Err
		}
	}

	if hasStructuredOutput && structuredJSON != nil {
		observe.GlobalTrace("if: hasStructuredOutput && structuredJSON != nil")
		fmt.Fprintln(out, string(structuredJSON))
	} else if hasStructuredOutput {
		observe.GlobalTrace("else-if: hasStructuredOutput")
		fmt.Fprintln(os.Stderr, "warning: model did not call StructuredOutput tool")
	}

	SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd)

	if d.Cfg.Verbose {
		observe.GlobalTrace("if: d.Cfg.Verbose")
		fmt.Fprintf(os.Stderr, "total cost: $%.6f\n", d.CostTracker.TotalUSD())
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// RunListSessions lists all saved sessions.
func RunListSessions() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessionStore, err := session.NewStore()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"open session store: %w\", err)")
		return fmt.Errorf("open session store: %w", err)
	}
	summaries, err := sessionStore.List()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"list sessions: %w\", err)")
		return fmt.Errorf("list sessions: %w", err)
	}
	if len(summaries) == 0 {
		observe.GlobalTrace("if: len(summaries) == 0")
		fmt.Println("No saved sessions.")
		observe.GlobalTrace("return: nil")
		return nil
	}
	for _, s := range summaries {
		observe.GlobalTrace("range summaries")
		summary := s.Summary
		if len(summary) > 80 {
			observe.GlobalTrace("if: len(summary) > 80")
			summary = summary[:80] + "..."
		}
		fmt.Printf("%-38s  %s  %d turns  $%.4f  %s\n",
			s.ID, s.Model, s.TurnCount, s.CostUSD, summary)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// BuildCompactionDeps creates compaction dependencies from the Deps struct.
func BuildCompactionDeps(d *Deps) (query.CompactionDeps, *compact.Service) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	secondaryModel := SecondaryModelFor(d.Cfg.Provider)
	compactor := compact.NewService(d.Prov, d.Bus, d.CostTracker, secondaryModel)

	disableAutoCompact := os.Getenv("DISABLE_AUTO_COMPACT") == "1" || os.Getenv("DISABLE_AUTO_COMPACT") == "true"
	autoTracker := compact.NewAutoTracker(disableAutoCompact)

	ctxWindow := 200_000
	if cw, ok := d.Prov.ContextWindow(d.Cfg.Model); ok {
		observe.GlobalTrace("if: ok")
		ctxWindow = cw
	}

	snap := d.Store.Snapshot()
	sysTokEst := compact.EstimateSystemPromptTokens(snap.Conversation.System)
	observe.GlobalTrace("return: query.CompactionDeps{\n\tCompactor:\tcompactor,\n\tAutoTracker:\tautoTracker,\n\tWind...")

	return query.CompactionDeps{
		Compactor:   compactor,
		AutoTracker: autoTracker,
		WindowConfig: compact.WindowConfig{
			ContextWindow:   ctxWindow,
			MaxOutput:       d.Cfg.MaxTokens,
			SystemPromptEst: sysTokEst,
		},
	}, compactor
}

// SaveSession persists the current conversation to disk.
func SaveSession(store *app.StateStore, costTracker *model.CostTracker, systemOverride, cwd string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessionStore, err := session.NewStore()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return
	}
	snap := store.Snapshot()
	if len(snap.Conversation.Messages) == 0 {
		observe.GlobalTrace("if: len(snap.Conversation.Messages) == 0")
		return
	}

	summary := ""
	for _, msg := range snap.Conversation.Messages {
		observe.GlobalTrace("range snap.Conversation.Messages")
		if msg.Role == model.RoleUser {
			observe.GlobalTrace("if: msg.Role == model.RoleUser")
			for _, part := range msg.Content {
				observe.GlobalTrace("range msg.Content")
				if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
					observe.GlobalTrace("if: ok && tp.Text != \"\"")
					summary = tp.Text
					if len(summary) > 100 {
						observe.GlobalTrace("if: len(summary) > 100")
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
		observe.GlobalTrace("range snap.Conversation.Messages")
		if msg.Role == model.RoleUser {
			observe.GlobalTrace("if: msg.Role == model.RoleUser")
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

// applyToolFilters applies --allowed-tools and --disallowed-tools flags.
// Uses Registry.Unregister — tools are physically removed, not just denied.
// This is stronger than permission-layer filtering (can't be bypassed via Bash).
func applyToolFilters(cmd *cobra.Command, registry *tool.Registry) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	allowedStr, _ := cmd.Flags().GetString("allowed-tools")
	if allowedStr != "" {
		observe.GlobalTrace("if: allowedStr != \"\"")
		allowed := parseToolList(allowedStr)
		allowedSet := make(map[string]bool, len(allowed))
		for _, name := range allowed {
			observe.GlobalTrace("range allowed")
			allowedSet[name] = true
		}
		for _, desc := range registry.List() {
			observe.GlobalTrace("range registry.List()")
			if !allowedSet[desc.Name()] {
				observe.GlobalTrace("if: !allowedSet[desc.Name()]")
				registry.Unregister(desc.Name())
			}
		}
	}

	disallowedStr, _ := cmd.Flags().GetString("disallowed-tools")
	if disallowedStr != "" {
		observe.GlobalTrace("if: disallowedStr != \"\"")
		disallowed := parseToolList(disallowedStr)
		for _, name := range disallowed {
			observe.GlobalTrace("range disallowed")
			registry.Unregister(name)
		}
	}
}

// parseToolList splits a comma-separated tool list, trimming whitespace.
func parseToolList(s string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		observe.GlobalTrace("range parts")
		p = strings.TrimSpace(p)
		if p != "" {
			observe.GlobalTrace("if: p != \"\"")
			result = append(result, p)
		}
	}
	observe.GlobalTrace("return: result")
	return result
}

// loadOutputSchema reads a JSON schema from a flag value — inline JSON or file path.
func loadOutputSchema(flag string) (json.RawMessage, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	trimmed := strings.TrimSpace(flag)
	if strings.HasPrefix(trimmed, "{") {
		observe.GlobalTrace("if: strings.HasPrefix(trimmed, \"{\")")
		if !json.Valid([]byte(trimmed)) {
			observe.GlobalTrace("if: !json.Valid([]byte(trimmed))")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"inline schema is not valid JSON\")")
			return nil, fmt.Errorf("inline schema is not valid JSON")
		}
		observe.GlobalTrace("return: json.RawMessage(trimmed), nil")
		return json.RawMessage(trimmed), nil
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read schema file %q: %w\", trimmed, err)")
		return nil, fmt.Errorf("read schema file %q: %w", trimmed, err)
	}
	if !json.Valid(data) {
		observe.GlobalTrace("if: !json.Valid(data)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"schema file %q does not contain valid JSON\", trimmed)")
		return nil, fmt.Errorf("schema file %q does not contain valid JSON", trimmed)
	}
	observe.GlobalTrace("return: json.RawMessage(data), nil")
	return json.RawMessage(data), nil
}

// ConsumeEngineEvents reads all events from an engine run and prints output.
// Used by replay and other non-interactive consumers.
func ConsumeEngineEvents(events <-chan query.LoopEvent, verbose bool) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for ev := range events {
		observe.GlobalTrace("range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.GlobalTrace("typecase: query.TextEvent")
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			observe.GlobalTrace("typecase: query.ThinkingEvent")
			if verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			observe.GlobalTrace("typecase: query.ToolCallEvent")
			if verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			}
		case query.ToolResultEvent:
			observe.GlobalTrace("typecase: query.ToolResultEvent")
			if verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
			}
		case query.CompactionEvent:
			observe.GlobalTrace("typecase: query.CompactionEvent")
			if verbose {
				fmt.Fprintf(os.Stderr, "[auto-compacted: %d → %d tokens]\n", e.PreTokens, e.PostTokens)
			}
		case query.TurnCompleteEvent:
			observe.GlobalTrace("typecase: query.TurnCompleteEvent")
			fmt.Println()
		case query.ErrorEvent:
			observe.GlobalTrace("typecase: query.ErrorEvent")
			return e.Err
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

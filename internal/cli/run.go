package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/orchestration"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/sysprompt"
	"github.com/artpar/pragma/internal/tool"
	toolapplypatch "github.com/artpar/pragma/internal/tools/applypatch"
	toolsynthetic "github.com/artpar/pragma/internal/tools/synthetic"
	"github.com/artpar/pragma/internal/tui"
	"github.com/artpar/pragma/internal/web"
)

// RunDispatcher routes to interactive UI, non-interactive mode, background, or list-sessions.
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

	pragmaHome, err := config.PragmaHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"resolve pragma home: %w\", err)")
		return fmt.Errorf("resolve pragma home: %w", err)
	}
	logsDir := filepath.Join(pragmaHome, "logs")
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
	addStringFlag("toolset")
	addStringFlag("permission-mode")
	addStringFlag("context-mode")
	addStringFlag("handoff-schema")
	addStringFlag("output-schema")
	addIntFlag("max-tokens")
	addIntFlag("max-turns")
	addIntFlag("thinking-budget")
	addBoolFlag("thinking")
	addBoolFlag("verbose")
	addBoolFlag("record")
	addBoolFlag("stop-after-tool-exec")
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
	fmt.Printf("  Use 'pragma sessions' to manage.\n")
	observe.GlobalTrace("return: nil")
	return nil
}

// InteractiveRuntime is the presentation-neutral runtime for an interactive session.
// cli owns construction; presentation packages own transport and rendering.
type InteractiveRuntime struct {
	Deps          *Deps
	Engine        *query.Engine
	SlashCmds     *slash.Registry
	SlashDeps     slash.Deps
	PromptHistory []string
	sessionSave   func()
	sessionClose  func()
	Cleanup       func(context.Context)
}

func (rt *InteractiveRuntime) RunInput(ctx context.Context, input string) <-chan query.LoopEvent {
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		if rt.Deps.HookMgr != nil {
			hookResult := rt.Deps.HookMgr.Execute(ctx, hook.UserPromptSubmit, hook.HookInput{
				PromptText: input,
			})
			if hookResult.Blocked {
				ch <- query.ErrorEvent{Err: fmt.Errorf("blocked by hook: %s", hookResult.BlockMsg)}
				return
			}
		}
		if name, args, ok := slash.Parse(input); ok && rt.SlashCmds != nil {
			rt.runSlash(ctx, name, args, ch)
			return
		}
		rt.runEngine(ctx, input, ch)
	}()
	return ch
}

func (rt *InteractiveRuntime) runSlash(ctx context.Context, name string, args string, ch chan<- query.LoopEvent) {
	deps := rt.SlashDeps
	if deps.LatestAssistantText == nil {
		deps.LatestAssistantText = func() string { return latestAssistantText(rt.Deps.Store) }
	}
	result, err := rt.SlashCmds.Execute(ctx, name, strings.TrimSpace(args), deps)
	if err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}
	if result.ClearConversation {
		rt.Deps.Store.Update(func(st *app.AppState) {
			st.Conversation.Messages = nil
		})
	}
	if result.ResumeSessionID != "" {
		if err := rt.Resume(result.ResumeSessionID); err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
	}
	if result.DisplayText != "" || result.OpenTeams || result.OpenModelPicker || result.OpenResumePicker || result.ResumeSessionID != "" || result.ClearConversation || result.Quit {
		ch <- query.SlashResultEvent{Result: result}
	}
	if result.Orchestrate != nil {
		rt.runOrchestration(ctx, *result.Orchestrate, ch)
		return
	}
	if result.InjectPrompt != "" {
		rt.runEngine(ctx, result.InjectPrompt, ch)
	}
}

func (rt *InteractiveRuntime) runEngine(ctx context.Context, input string, ch chan<- query.LoopEvent) {
	if err := startSessionForCurrentConversation(ctx, rt.Deps); err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}
	if rt.Deps.SessionWriter != nil {
		_ = rt.Deps.SessionWriter.WritePromptHistory(input)
	}
	for ev := range rt.Engine.Run(ctx, input) {
		ch <- ev
		if query.ShouldPersistSessionEvent(ev) {
			rt.sessionSave()
		}
	}
}

func (rt *InteractiveRuntime) runOrchestration(ctx context.Context, req slash.OrchestrationRequest, ch chan<- query.LoopEvent) {
	if err := startSessionForCurrentConversation(ctx, rt.Deps); err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}
	for ev := range orchestration.RunFileEvents(ctx, rt.Engine, req.DefinitionPath, req.PersonaDir, req.Prompt) {
		ch <- ev
		if query.ShouldPersistSessionEvent(ev) {
			rt.sessionSave()
		}
	}
}

func (rt *InteractiveRuntime) Resume(sessionID string) error {
	resumedFrom := rt.Deps.Store.Snapshot().Conversation.ID
	sessStore, err := session.NewStore()
	if err != nil {
		return err
	}
	sess, err := sessStore.Load(sessionID)
	if err != nil {
		return err
	}
	w, err := sessStore.Open(sessionID)
	if err != nil {
		return err
	}
	if rt.sessionClose != nil {
		endSessionLifecycle(context.Background(), rt.Deps)
		rt.sessionClose()
	}
	rt.Deps.SessionWriter = w
	rt.Deps.SessionHeader = session.HeaderData{}
	rt.Deps.SessionLastIdx = len(sess.Conversation.Messages)
	rt.Deps.SessionStarted = false
	rt.Deps.Store.Update(func(st *app.AppState) {
		st.Conversation = sess.Conversation
		if sess.Conversation.Model != "" {
			st.Model = sess.Conversation.Model
		}
		st.HandoffState = sess.HandoffState
	})
	rt.Engine.ResetContentReplacementState(sess.ContentReplacements)
	rt.sessionSave, rt.sessionClose = makeSessionSaveClose(rt.Deps)
	beginSessionLifecycle(context.Background(), rt.Deps, resumedFrom)
	return nil
}

func (rt *InteractiveRuntime) CloseSession() {
	if rt.sessionClose != nil {
		rt.sessionClose()
	}
}

// BuildInteractiveRuntime wires the shared dependencies for an interactive UI.
func BuildInteractiveRuntime(cmd *cobra.Command, prompter permission.Prompter, asker tool.Asker) (*InteractiveRuntime, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d, err := SetupDeps(cmd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return nil, err
	}

	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		if d.Cleanup != nil {
			d.Cleanup()
		}
		return nil, err
	}
	applyToolFilters(cmd, d.Registry)
	waitForToolsetMCP(cmd.Context(), d)
	if d.SessionWriter != nil {
		beginSessionLifecycle(cmd.Context(), d, "")
	}

	compDeps, compactor := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)

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

	sessStore, _ := session.NewStore()
	promptHistory := promptHistoryFromSessions(sessStore, tui.InputHistoryLimit)
	slashDeps := slash.Deps{
		Store:       d.Store,
		CostTracker: d.CostTracker,
		Compactor:   compactor,
		Bus:         d.Bus,
		SessionSave: sessionSaveFn,
		ModelName:   d.Cfg.Model,
		Provider:    d.Cfg.Provider,
		Cwd:         d.Cwd,
		TaskReg:     d.TaskReg,
		ModelLister: func() []string {
			if ml, ok := d.Prov.(provider.ModelLister); ok {
				return ml.ListModels()
			}
			return nil
		},
		ContextWindowFunc: d.Prov.ContextWindow,
		OnModelChanged: func(modelID string) {
			if cw, ok := d.Prov.ContextWindow(modelID); ok {
				d.TokenMonitor.SetBudget(cw)
			}
		},
		McpStatus:    func() []slash.McpServerStatus { return mcpStatusesForSlash(d.McpManager) },
		SessionStore: sessStore,
		SkillLoader:  skillLoader,
	}

	rt := &InteractiveRuntime{
		Deps:          d,
		Engine:        engine,
		SlashCmds:     slashCmds,
		SlashDeps:     slashDeps,
		PromptHistory: promptHistory,
		sessionSave:   sessionSaveFn,
		sessionClose:  sessionCloseFn,
		Cleanup: func(ctx context.Context) {
			endSessionLifecycle(ctx, d)
			if d.Cleanup != nil {
				d.Cleanup()
			}
		},
	}
	observe.GlobalTrace("return: rt, nil")
	return rt, nil
}

// RunInteractive launches the browser UI for multi-turn conversation.
func RunInteractive(cmd *cobra.Command) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	bridge := web.NewBridge()
	rt, err := BuildInteractiveRuntime(cmd, bridge, bridge)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	defer rt.Cleanup(cmd.Context())
	return web.Run(cmd.Context(), web.Config{
		Bridge:         bridge,
		ParentCtx:      cmd.Context(),
		RunInput:       rt.RunInput,
		Resume:         rt.Resume,
		CloseSession:   rt.CloseSession,
		Store:          rt.Deps.Store,
		CostTracker:    rt.Deps.CostTracker,
		ModelName:      rt.Deps.Cfg.Model,
		Provider:       rt.Deps.Cfg.Provider,
		SlashCmds:      rt.SlashCmds,
		SlashDeps:      rt.SlashDeps,
		Metrics:        rt.Deps.Metrics,
		Workspace:      rt.Deps.Cwd,
		Version:        buildinfo.Version,
		TaskReg:        rt.Deps.TaskReg,
		SessionStart:   rt.Deps.SessionStart,
		McpServerNames: connectedMcpNames(rt.Deps.McpManager),
		PromptHistory:  rt.PromptHistory,
	})
}

// RunTUIInteractive launches the Bubble Tea TUI using the shared runtime.
func RunTUIInteractive(cmd *cobra.Command) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	prompter := tui.NewInteractivePrompter()
	asker := tui.NewInteractiveAsker()
	rt, err := BuildInteractiveRuntime(cmd, prompter, asker)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	defer rt.Cleanup(cmd.Context())

	m := tui.New(tui.Config{
		ParentCtx:      cmd.Context(),
		RunInput:       rt.RunInput,
		Resume:         rt.Resume,
		CloseSession:   rt.CloseSession,
		Store:          rt.Deps.Store,
		CostTracker:    rt.Deps.CostTracker,
		ModelName:      rt.Deps.Cfg.Model,
		Provider:       rt.Deps.Cfg.Provider,
		SlashCmds:      rt.SlashCmds,
		SlashDeps:      rt.SlashDeps,
		HookMgr:        rt.Deps.HookMgr,
		TokenMonitor:   rt.Deps.TokenMonitor,
		Metrics:        rt.Deps.Metrics,
		Workspace:      rt.Deps.Cwd,
		Version:        buildinfo.Version,
		TaskReg:        rt.Deps.TaskReg,
		SessionStart:   rt.Deps.SessionStart,
		McpServerNames: connectedMcpNames(rt.Deps.McpManager),
		PromptHistory:  rt.PromptHistory,
	})

	log.SetOutput(io.Discard)

	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	prompter.SetProgram(program)
	asker.SetProgram(program)

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

	defer func() {
		endSessionLifecycle(cmd.Context(), d)
	}()

	if !cmd.Flags().Changed("permission-mode") {
		observe.GlobalTrace("if: !cmd.Flags().Changed(\"permission-mode\")")
		d.Checker = permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tool.NonInteractiveAsker{}
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
		if d.Toolset != nil && !d.Toolset.AllowBuiltinTool(synTool.Name()) {
			observe.GlobalTrace("if: d.Toolset != nil && !d.Toolset.AllowBuiltinTool(synTool.Name())")
			return fmt.Errorf("toolset %q does not expose %s; enable includeBuiltinTools and include %s in the toolset tools list", d.Toolset.Name, synTool.Name(), synTool.Name())
		}
		if err := d.Registry.Register(synTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"register StructuredOutput tool: %w\", err)")
			return fmt.Errorf("register StructuredOutput tool: %w", err)
		}
	}

	applyToolFilters(cmd, d.Registry)
	waitForToolsetMCP(cmd.Context(), d)

	compDeps, _ := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	prompt, _ := cmd.Flags().GetString("prompt")
	resumeID, _ := cmd.Flags().GetString("resume")
	if resumeID != "" && prompt == "" {
		observe.GlobalTrace("if: resumeID != \"\" && prompt == \"\"")
		prompt = "Continue from where we left off."
	}

	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)

	ctx := cmd.Context()
	if strings.TrimSpace(prompt) != "" {
		if err := startSessionForCurrentConversation(ctx, d); err != nil {
			return err
		}
	}
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

	var turnCount int
	var turnToolCount int

	for ev := range events {
		observe.GlobalTrace("range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.GlobalTrace("typecase: query.TextEvent")
			if !hasStructuredOutput {
				fmt.Fprint(out, e.Text)
				flushWriter(out)
			}
		case query.ThinkingEvent:
			observe.GlobalTrace("typecase: query.ThinkingEvent")
			if d.Cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
				flushWriter(os.Stderr)
			}
		case query.ModelRequestEvent:
			observe.GlobalTrace("typecase: query.ModelRequestEvent")
			fmt.Fprintf(os.Stderr, "[model request: %s attempt %d]\n", e.Model, e.Attempt)
			flushWriter(os.Stderr)
		case query.ModelResponseEvent:
			observe.GlobalTrace("typecase: query.ModelResponseEvent")
			fmt.Fprintf(os.Stderr, "[model response: %s stop=%s]\n", e.Model, e.StopReason)
			flushWriter(os.Stderr)
		case query.ToolCallEvent:
			observe.GlobalTrace("typecase: query.ToolCallEvent")
			if hasStructuredOutput && e.Call.Name == "StructuredOutput" {
				structuredJSON = e.Call.Input
			}
			turnToolCount++
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			} else {
				fmt.Fprintf(os.Stderr, "  ⏺ %s\n", e.Call.Name)
			}
			flushWriter(os.Stderr)
		case query.ToolResultEvent:
			observe.GlobalTrace("typecase: query.ToolResultEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
				flushWriter(os.Stderr)
			}
		case query.CompactionEvent:
			observe.GlobalTrace("typecase: query.CompactionEvent")
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[auto-compacted: %d → %d tokens]\n", e.PreTokens, e.PostTokens)
				flushWriter(os.Stderr)
			}
		case query.TurnCompleteEvent:
			observe.GlobalTrace("typecase: query.TurnCompleteEvent")
			turnCount++
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[turn %d complete, %d tool calls]\n", turnCount, turnToolCount)
				flushWriter(os.Stderr)
			}
			turnToolCount = 0
			if !hasStructuredOutput {
				fmt.Fprintln(out)
				flushWriter(out)
			}
		case query.ErrorEvent:
			observe.GlobalTrace("typecase: query.ErrorEvent")
			if e.Guidance != "" {
				fmt.Fprintf(os.Stderr, "Hint: %s\n", e.Guidance)
				flushWriter(os.Stderr)
			}
			sessionCloseFn()
			return e.Err
		}
		if query.ShouldPersistSessionEvent(ev) {
			sessionSaveFn()
		}
	}

	if hasStructuredOutput && structuredJSON != nil {
		observe.GlobalTrace("if: hasStructuredOutput && structuredJSON != nil")
		fmt.Fprintln(out, string(structuredJSON))
	} else if hasStructuredOutput {
		observe.GlobalTrace("else-if: hasStructuredOutput")
		fmt.Fprintln(os.Stderr, "warning: model did not call StructuredOutput tool")
	}

	sessionCloseFn()

	fmt.Fprintf(os.Stderr, "\ntotal cost: $%.6f\n", d.CostTracker.TotalUSD())
	observe.GlobalTrace("return: nil")
	return nil
}

func flushWriter(w io.Writer) {
	if f, ok := w.(*os.File); ok {
		_ = f.Sync()
	}
}

func startSessionForCurrentConversation(ctx context.Context, d *Deps) error {
	if d.SessionWriter != nil {
		beginSessionLifecycle(ctx, d, "")
		return nil
	}
	header := d.SessionHeader
	if header.SessionID == "" {
		snap := d.Store.Snapshot()
		header = session.HeaderData{
			SessionID:      snap.Conversation.ID,
			Model:          d.Cfg.Model,
			Provider:       d.Cfg.Provider,
			WorkDir:        d.Cwd,
			GitRemote:      sysprompt.GitRemoteURL(d.Cwd),
			SystemOverride: d.Cfg.SystemPrompt,
			CreatedAt:      snap.Conversation.CreatedAt,
			System:         snap.Conversation.System,
		}
	}
	sessStore, err := session.NewStore()
	if err != nil {
		return err
	}
	w, err := sessStore.Create(header)
	if err != nil {
		return err
	}
	d.SessionWriter = w
	d.SessionHeader = header
	d.SessionLastIdx = 0
	beginSessionLifecycle(ctx, d, "")
	return nil
}

func beginSessionLifecycle(ctx context.Context, d *Deps, resumedFrom string) {
	if d == nil || d.SessionStarted {
		return
	}
	sessionID := d.Store.Snapshot().Conversation.ID
	if d.HookMgr != nil {
		d.HookMgr.SetSessionID(sessionID)
	}
	d.Bus.Emit(observe.SessionStarted{
		EventHeader: observe.NewEventHeader("SessionStarted", "", sessionID, ""),
		SessionID:   sessionID,
		ResumedFrom: resumedFrom,
	})
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		d.HookMgr.Execute(ctx, hook.SessionStart, hook.HookInput{})
	}
	d.SessionStarted = true
}

func endSessionLifecycle(ctx context.Context, d *Deps) {
	if d == nil || !d.SessionStarted {
		return
	}
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		d.HookMgr.Execute(ctx, hook.SessionEnd, hook.HookInput{})
	}
	d.SessionStarted = false
}

func latestAssistantText(store *app.StateStore) string {
	if store == nil {
		return ""
	}
	messages := store.Snapshot().Conversation.Messages
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != model.RoleAssistant || msg.Flags.IsInternal || msg.Flags.IsMeta {
			continue
		}
		var parts []string
		for _, part := range msg.Content {
			if tp, ok := part.(model.TextPart); ok && strings.TrimSpace(tp.Text) != "" {
				parts = append(parts, tp.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return ""
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

// makeSessionSaveClose creates sessionSaveFn and sessionCloseFn from Deps.
// On resumed sessions, lastIdx starts at len(messages) so existing messages aren't re-written.
func makeSessionSaveClose(d *Deps) (saveFn func(), closeFn func()) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	saveFn = func() {
		if d.SessionWriter == nil {
			return
		}
		snap := d.Store.Snapshot()
		msnap := d.Metrics.Snapshot()
		for i := d.SessionLastIdx; i < len(snap.Conversation.Messages); i++ {
			d.SessionWriter.WriteMessage(snap.Conversation.Messages[i])
		}
		d.SessionWriter.WriteHandoffState(snap.HandoffState)
		d.SessionLastIdx = len(snap.Conversation.Messages)
		d.SessionWriter.WriteMetadata(session.MetadataData{
			CostUSD:    d.CostTracker.TotalUSD(),
			TurnCount:  countUserTurns(snap.Conversation.Messages),
			TokenUsage: msnap.TokenUsage,
			UpdatedAt:  snap.Conversation.UpdatedAt,
			Summary:    extractSummary(snap.Conversation.Messages),
		})
	}
	closeFn = func() {
		if d.SessionWriter != nil {
			d.SessionWriter.Close()
		}
	}
	return
}

func promptHistoryFromSessions(store *session.Store, limit int) []string {
	if store == nil || limit <= 0 {
		return nil
	}
	summaries, err := store.List()
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	newestFirst := make([]string, 0, limit)
	for _, summary := range summaries {
		sess, err := store.Load(summary.ID)
		if err != nil {
			continue
		}
		prompts := sessionPromptHistory(sess)
		for i := len(prompts) - 1; i >= 0; i-- {
			prompt := prompts[i]
			if seen[prompt] {
				continue
			}
			seen[prompt] = true
			newestFirst = append(newestFirst, prompt)
			if len(newestFirst) >= limit {
				return reversePromptHistory(newestFirst)
			}
		}
	}
	return reversePromptHistory(newestFirst)
}

func sessionPromptHistory(sess session.Session) []string {
	if len(sess.PromptHistory) > 0 {
		prompts := make([]string, 0, len(sess.PromptHistory))
		for _, entry := range sess.PromptHistory {
			if entry.Text != "" {
				prompts = append(prompts, entry.Text)
			}
		}
		return prompts
	}
	prompts := model.ExtractUserTextPrompts(sess.Conversation.Messages)
	filtered := prompts[:0]
	for _, prompt := range prompts {
		if generatedPromptHistory(prompt) {
			continue
		}
		filtered = append(filtered, prompt)
	}
	return filtered
}

func generatedPromptHistory(text string) bool {
	trimmed := strings.TrimSpace(text)
	return strings.HasPrefix(trimmed, "Please solve this task:") ||
		(strings.HasPrefix(trimmed, "## Task") && strings.Contains(trimmed, "## Possible Next Phase Handoffs"))
}

func reversePromptHistory(newestFirst []string) []string {
	history := make([]string, len(newestFirst))
	for i := range newestFirst {
		history[len(newestFirst)-1-i] = newestFirst[i]
	}
	return history
}

// countUserTurns counts all RoleUser messages (matching old SaveSession behavior).
func countUserTurns(msgs []model.Message) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	count := 0
	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		if msg.Role == model.RoleUser {
			observe.GlobalTrace("if: msg.Role == model.RoleUser")
			count++
		}
	}
	observe.GlobalTrace("return: count")
	return count
}

// extractSummary returns the first user text message, truncated to 100 chars.
func extractSummary(msgs []model.Message) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		if msg.Role != model.RoleUser {
			observe.GlobalTrace("if: msg.Role != model.RoleUser")
			continue
		}
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tp, ok := part.(model.TextPart); ok && tp.Text != "" {
				observe.GlobalTrace("if: ok && tp.Text != \"\"")
				s := tp.Text
				if len(s) > 100 {
					observe.GlobalTrace("if: len(s) > 100")
					s = s[:100]
				}
				observe.GlobalTrace("return: s")
				return s
			}
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
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

	if !hasExposedPatchTool(registry) {
		setPatchMode(registry, false)
	}
}

func hasExposedPatchTool(registry *tool.Registry) bool {
	for _, desc := range registry.List() {
		switch desc.Name() {
		case toolapplypatch.ToolName, toolapplypatch.LegacyToolName:
			return true
		}
	}
	return false
}

type patchModeSetter interface {
	SetPatchMode(bool)
}

func setPatchMode(registry *tool.Registry, enabled bool) {
	for _, desc := range registry.List() {
		if setter, ok := desc.(patchModeSetter); ok {
			setter.SetPatchMode(enabled)
		}
	}
}

func waitForToolsetMCP(ctx context.Context, d *Deps) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d == nil || d.Toolset == nil || d.McpManager == nil || !d.Toolset.SelectsMCP() {
		observe.GlobalTrace("if: d == nil || d.Toolset == nil || d.McpManager == nil || !d.Toolset.SelectsMCP()")
		return
	}
	d.McpManager.WaitForRegisteredTools(ctx, 3*time.Second)
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
			if e.Guidance != "" {
				fmt.Fprintf(os.Stderr, "Hint: %s\n", e.Guidance)
			}
			return e.Err
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// connectedMcpNames returns sorted connected server names from the MCP manager.
func connectedMcpNames(mgr *mcp.Manager) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if mgr == nil {
		observe.GlobalTrace("if: mgr == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	status := mgr.ServerStatus()
	var names []string
	for name, st := range status {
		observe.GlobalTrace("range status")
		if st == "connected" {
			observe.GlobalTrace("if: st == \"connected\"")
			names = append(names, name)
		}
	}
	sort.Strings(names)
	observe.GlobalTrace("return: names")
	return names
}

func mcpStatusesForSlash(mgr *mcp.Manager) []slash.McpServerStatus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if mgr == nil {
		observe.GlobalTrace("if: mgr == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	statuses := mgr.ServerStatuses()
	out := make([]slash.McpServerStatus, 0, len(statuses))
	for _, st := range statuses {
		observe.GlobalTrace("range statuses")
		out = append(out, slash.McpServerStatus{
			Name:      st.Name,
			Status:    st.Status,
			Error:     st.Error,
			ToolCount: st.ToolCount,
			Transport: st.Transport,
		})
	}
	observe.GlobalTrace("return: out")
	return out
}

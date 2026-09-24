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
	"sync"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/background"
	"github.com/artpar/pragma/internal/buildinfo"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/gitutil"
	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/llmconfig"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/orchestration"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/tui"
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
	ownerToken, err := background.NewOwnerToken()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		logFile.Close()
		observe.GlobalTrace("return: fmt.Errorf(\"create background owner token: %w\", err)")
		return fmt.Errorf("create background owner token: %w", err)
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
	addStringFlag("permission-mode")
	addStringFlag("system-prompt")
	addStringFlag("loop")
	addStringFlag("context-mode")
	addStringFlag("handoff-schema")
	addStringFlag("output-schema")
	addIntFlag("max-tokens")
	addIntFlag("max-turns")
	addIntFlag("thinking-budget")
	addBoolFlag("thinking")
	addBoolFlag("verbose")
	addBoolFlag("trace")
	addBoolFlag("record")
	if cmd.Flags().Changed("temperature") {
		observe.GlobalTrace("if: cmd.Flags().Changed(\"temperature\")")
		v, _ := cmd.Flags().GetFloat64("temperature")
		childArgs = append(childArgs, fmt.Sprintf("--temperature=%g", v))
	}

	env := append(os.Environ(),
		"PRAGMA_BG_SESSION=1",
		"PRAGMA_BG_SESSION_LOG="+logPath,
		"PRAGMA_BG_SESSION_TOKEN="+ownerToken,
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
			PID:        childPid,
			PGID:       childPid,
			SessionID:  "",
			OwnerToken: ownerToken,
			CWD:        cwd,
			StartedAt:  time.Now(),
			Status:     background.StatusStarting,
			LogPath:    logPath,
			Model:      modelName,
			Provider:   providerName,
			Prompt:     promptDisplay,
		})
	}

	fmt.Printf("Background process started (PID %d)\n", childPid)
	fmt.Printf("  Logs: %s\n", logPath)
	fmt.Printf("  Use 'pragma sessions' after the runtime session starts.\n")
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
	admissionMu   sync.Mutex
	turnActive    bool
	sessionSave   func() error
	sessionClose  func() error
	Cleanup       func(context.Context)
}

type InteractiveRuntimeOptions struct {
	ConfigureDeps func(*Deps)
	SetupDeps     SetupDepsOptions
}

func (rt *InteractiveRuntime) RunInput(ctx context.Context, input string) <-chan interactive.Event {
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "enter")
	defer observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "exit")
	ch := make(chan interactive.Event, 16)
	if !rt.beginInputTurn() {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "if: !rt.beginInputTurn()")
		// INT-001: plain text submitted while a turn is running is queued
		// into the conversation for the next request boundary instead of
		// rejected-and-dropped — the night-session path where the only
		// delivery was an interrupt that forfeited the in-flight request's
		// input spend. Slash commands keep the busy rejection (runtime
		// side effects cannot run mid-turn).
		if _, _, ok := slash.Parse(input); !ok && rt.Engine != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "if: !ok && rt.Engine != nil")
			if err := rt.Engine.AppendUserInput(input); err != nil {
				observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "if: err != nil")
				ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
				close(ch)
				observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "return: ch")
				return ch
			}
			rt.rememberAcceptedPrompt(input)
			if err := rt.writePromptHistory(input); err != nil {
				observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "if: err != nil")
				ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
				close(ch)
				observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "return: ch")
				return ch
			}
			ch <- interactive.QueuedPromptEvent{Prompt: input}
			close(ch)
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "return: ch")
			return ch
		}
		ch <- interactive.RejectedPromptEvent{Prompt: input, Reason: "busy"}
		close(ch)
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "return: ch")
		return ch
	}
	go func() {
		defer close(ch)
		defer rt.finishInputTurn()
		promptHookResult, err := acceptPromptSubmission(ctx, rt.Deps, input)
		if err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
		if name, args, ok := slash.Parse(input); ok && rt.SlashCmds != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "if: ok && rt.SlashCmds != nil")
			rt.rememberAcceptedPrompt(input)
			ch <- interactive.AcceptedPromptEvent{Prompt: input}
			rt.runSlash(ctx, input, name, args, promptHookResult, ch)
			return
		}
		rt.rememberAcceptedPrompt(input)
		ch <- interactive.AcceptedPromptEvent{Prompt: input}
		rt.runEngine(ctx, input, input, promptHookResult, ch)
	}()
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.RunInput", "return: ch")
	return ch
}

func (rt *InteractiveRuntime) beginInputTurn() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rt.admissionMu.Lock()
	defer rt.admissionMu.Unlock()
	if rt.turnActive {
		observe.GlobalTrace("if: rt.turnActive")
		observe.GlobalTrace("return: false")
		return false
	}
	rt.turnActive = true
	observe.GlobalTrace("return: true")
	return true
}

func (rt *InteractiveRuntime) finishInputTurn() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rt.admissionMu.Lock()
	defer rt.admissionMu.Unlock()
	rt.turnActive = false
}

func (rt *InteractiveRuntime) runSlash(ctx context.Context, submittedInput string, name string, args string, promptHookResult hook.AggregatedResult, ch chan<- interactive.Event) {
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "enter")
	defer observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "exit")
	deps := rt.SlashDeps
	if deps.LatestAssistantText == nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: deps.LatestAssistantText == nil")
		deps.LatestAssistantText = func() string { return latestAssistantText(rt.Deps.Store) }
	}
	result, err := rt.SlashCmds.Execute(ctx, name, strings.TrimSpace(args), deps)
	if err != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
		ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
		return
	}
	if result.ClearConversation {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.ClearConversation")
		if err := rt.closeCurrentSessionAfterClear(ctx); err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
	}
	if result.RewriteSession {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.RewriteSession")
		if err := rewriteCurrentSession(rt.Deps); err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
	}
	if result.ResumeSessionID != "" {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.ResumeSessionID != \"\"")
		if err := rt.Resume(result.ResumeSessionID); err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
	}
	if result.Quit {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.Quit")
		if err := rt.CloseSession(); err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
	}
	if result.DisplayText != "" || result.OpenModelPicker || result.OpenResumePicker || result.ResumeSessionID != "" || result.ClearConversation {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.DisplayText != \"\" || result.OpenModelPicker || result.OpenResumePicker ||...")
		ch <- interactive.SlashResultEvent{Result: result}
	}
	if result.Quit {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.Quit")
		ch <- interactive.RuntimeTerminatedEvent{Reason: "exit"}
		return
	}
	if result.Orchestrate != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.Orchestrate != nil")
		orchestrationHookResult, err := acceptPromptSubmission(ctx, rt.Deps, result.Orchestrate.Prompt)
		if err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
		rt.runOrchestration(ctx, *result.Orchestrate, submittedInput, orchestrationHookResult, ch)
		return
	}
	if result.InjectPrompt != "" {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: result.InjectPrompt != \"\"")
		injectedHookResult, err := acceptPromptSubmission(ctx, rt.Deps, result.InjectPrompt)
		if err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runSlash", "if: err != nil")
			ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
			return
		}
		rt.runEngine(ctx, result.InjectPrompt, submittedInput, injectedHookResult, ch)
	}
}

func acceptPromptSubmission(ctx context.Context, d *Deps, prompt string) (hook.AggregatedResult, error) {
	observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "enter")
	defer observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "exit")
	if strings.TrimSpace(prompt) == "" || d == nil || d.HookMgr == nil {
		observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "if: strings.TrimSpace(prompt) == \"\" || d == nil || d.HookMgr == nil")
		observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "return: hook.AggregatedResult{}, nil")
		return hook.AggregatedResult{}, nil
	}
	result := d.HookMgr.ExecuteInWorkDir(ctx, hook.UserPromptSubmit, hook.HookInput{
		PromptText: prompt,
	}, activeHookWorkDir(d))
	if result.Blocked {
		observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "if: result.Blocked")
		observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "return: result, promptBlockedError(result)")
		return result, promptBlockedError(result)
	}
	observe.TraceCtx(ctx, "cli", "acceptPromptSubmission", "return: result, nil")
	return result, nil
}

func promptBlockedError(result hook.AggregatedResult) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msg := strings.TrimSpace(result.BlockMsg)
	if msg == "" {
		observe.GlobalTrace("if: msg == \"\"")
		msg = "prompt blocked by hook"
	}
	observe.GlobalTrace("return: fmt.Errorf(\"blocked by hook: %s\", msg)")
	return fmt.Errorf("blocked by hook: %s", msg)
}

func (rt *InteractiveRuntime) closeCurrentSessionAfterClear(ctx context.Context) error {
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "enter")
	defer observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "exit")
	if err := rt.closeCurrentSession(ctx); err != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "if: err != nil")
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "return: err")
		return err
	}
	if rt.Engine != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "if: rt.Engine != nil")
		rt.Engine.ResetSessionState(nil)
	}
	rt.Deps.SessionWriter = nil
	rt.Deps.SessionHeader = session.HeaderData{}
	rt.Deps.SessionLastIdx = 0
	rt.sessionSave, rt.sessionClose = makeSessionSaveClose(rt.Deps)
	if rt.Engine != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "if: rt.Engine != nil")
		rt.Engine.SetSessionCheckpoint(rt.sessionSave)
		// CMP-001.2 F2: auto-compaction rewrites the whole session file.
		rt.Engine.SetSessionRewrite(func() error { return rewriteCurrentSession(rt.Deps) })
	}
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSessionAfterClear", "return: nil")
	return nil
}

func (rt *InteractiveRuntime) runEngine(ctx context.Context, input string, submittedInput string, promptHookResult hook.AggregatedResult, ch chan<- interactive.Event) {
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runEngine", "enter")
	defer observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runEngine", "exit")
	if err := rt.appendHookContext(hook.UserPromptSubmit, promptHookResult, false); err != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runEngine", "if: err != nil")
		ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
		return
	}
	rt.acceptUserTurn(submittedInput)
	if err := rt.writePromptHistory(submittedInput); err != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runEngine", "if: err != nil")
		ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
		return
	}
	for ev := range rt.Engine.Run(ctx, input) {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runEngine", "range rt.Engine.Run(ctx, input)")
		ch <- interactive.LoopEvent{Event: ev}
	}
}

func (rt *InteractiveRuntime) writePromptHistory(input string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rt.Deps.SessionWriter == nil {
		observe.GlobalTrace("if: rt.Deps.SessionWriter == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: rt.Deps.SessionWriter.WritePromptHistory(input)")
	return rt.Deps.SessionWriter.WritePromptHistory(input)
}

func (rt *InteractiveRuntime) acceptUserTurn(input string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rt == nil || rt.Deps == nil || rt.Deps.Bus == nil {
		observe.GlobalTrace("if: rt == nil || rt.Deps == nil || rt.Deps.Bus == nil")
		return
	}
	rt.Deps.Bus.Emit(observe.UserTurnAccepted{
		EventHeader: observe.NewEventHeader("UserTurnAccepted", "", observe.NewSpanID(), ""),
		PromptChars: len(strings.TrimSpace(input)),
	})
}

func (rt *InteractiveRuntime) rememberAcceptedPrompt(input string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rt == nil || rt.Deps == nil || rt.Deps.Store == nil {
		observe.GlobalTrace("if: rt == nil || rt.Deps == nil || rt.Deps.Store == nil")
		return
	}
	var updated []string
	rt.Deps.Store.Update(func(st *app.AppState) {
		st.PromptHistory = appendPromptHistory(st.PromptHistory, input, tui.InputHistoryLimit)
		updated = append([]string(nil), st.PromptHistory...)
	})
	rt.PromptHistory = updated
}

func (rt *InteractiveRuntime) runOrchestration(ctx context.Context, req slash.OrchestrationRequest, submittedInput string, promptHookResult hook.AggregatedResult, ch chan<- interactive.Event) {
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "enter")
	defer observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "exit")
	if err := rt.appendHookContext(hook.UserPromptSubmit, promptHookResult, false); err != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "if: err != nil")
		ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
		return
	}
	rt.acceptUserTurn(submittedInput)
	if err := rt.writePromptHistory(submittedInput); err != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "if: err != nil")
		ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
		return
	}
	artifactRoot := interactiveOrchestrationArtifactRoot(rt.Deps)
	llmResolver, cleanupLLMResolver := makeOrchestrationLLMResolver(rt.Deps)
	defer cleanupLLMResolver()
	for ev := range orchestration.RunFileEventsWithOptions(ctx, rt.Engine, req.DefinitionPath, orchestration.RunOptions{
		PersonaDir:     req.PersonaDir,
		TaskPrompt:     req.Prompt,
		ArtifactRoot:   artifactRoot,
		SeedArtifacts:  req.SeedArtifacts,
		LLMResolver:    llmResolver,
		StartAtState:   req.StartAtState,
		StopAfterState: req.StopAfterState,
	}) {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "range orchestration.RunFileEventsWithOptions(ctx, rt.Engine, req.DefinitionPath, or...")
		if handoff, ok := ev.(query.OrchestrationHandoffEvent); ok {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "if: ok")
			if err := rt.recordOrchestrationArtifact(artifactRoot, handoff); err != nil {
				observe.TraceCtx(ctx, "cli", "InteractiveRuntime.runOrchestration", "if: err != nil")
				ch <- interactive.LoopEvent{Event: query.ErrorEvent{Err: err}}
				return
			}
		}
		ch <- interactive.LoopEvent{Event: ev}
	}
}

func makeOrchestrationLLMResolver(d *Deps) (orchestration.LLMResolver, func()) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cache := make(map[string]provider.Provider)
	resolver := func(ctx context.Context, llm llmconfig.Config) (orchestration.LLMRuntime, error) {
		_ = ctx
		providerName := strings.TrimSpace(llm.Provider)
		if providerName == "" {
			providerName = d.Cfg.Provider
		}
		modelID := strings.TrimSpace(llm.Model)
		if modelID == "" {
			if providerName == d.Cfg.Provider {
				modelID = d.Cfg.Model
			} else {
				modelID = DefaultModelFor(providerName)
			}
		}
		modelID = resolveModelAlias(providerName, modelID)

		cfg := d.Cfg
		cfg.Provider = providerName
		cfg.Model = modelID
		if providerName != d.Cfg.Provider {
			cfg.APIKey = d.Creds.CredentialFor(providerName).APIKey
			if cfg.APIKey == "" && providerName != "google-vertex" {
				cfg.APIKey = os.Getenv(envVarForProvider(providerName))
			}
		}
		if providerName != "google-vertex" && cfg.APIKey == "" {
			return orchestration.LLMRuntime{}, fmt.Errorf("missing API key for provider %q", providerName)
		}

		cacheKey := providerName + "\x00" + ProviderBaseURL(providerName, d.Creds)
		prov := d.Prov
		if providerName != d.Cfg.Provider || ProviderBaseURL(providerName, d.Creds) != ProviderBaseURL(d.Cfg.Provider, d.Creds) {
			var ok bool
			prov, ok = cache[cacheKey]
			if !ok {
				created, err := CreateProvider(cfg, d.Bus)
				if err != nil {
					return orchestration.LLMRuntime{}, err
				}
				prov = created
				cache[cacheKey] = prov
			}
		}

		maxTokens := d.Cfg.MaxTokens
		if llm.MaxTokens != 0 {
			maxTokens = llm.MaxTokens
		}
		temperature := d.Cfg.Temperature
		if llm.Temperature != nil {
			temperature = llm.Temperature
		}
		thinking := thinkingConfigFromDefaults(d.Cfg.Thinking)
		if llm.Thinking != nil || llm.ThinkingBudget != 0 {
			enabled := true
			if llm.Thinking != nil {
				enabled = *llm.Thinking
			}
			thinking = &provider.ThinkingConfig{Enabled: enabled, BudgetTokens: llm.ThinkingBudget}
		}
		customSystemPrompt := d.EngineCfg.CustomSystemPrompt
		if strings.TrimSpace(llm.SystemPrompt) != "" {
			customSystemPrompt = llm.SystemPrompt
		}

		return orchestration.LLMRuntime{
			Provider:           prov,
			ProviderName:       providerName,
			Model:              modelID,
			MaxTokens:          maxTokens,
			Temperature:        temperature,
			Thinking:           thinking,
			CustomSystemPrompt: customSystemPrompt,
		}, nil
	}
	cleanup := func() {
		for _, prov := range cache {
			reportProviderCleanup(context.Background(), prov, d.Bus)
		}
	}
	observe.GlobalTrace("return: resolver, cleanup")
	return resolver, cleanup
}

func thinkingConfigFromDefaults(cfg *config.ThinkingConfig) *provider.ThinkingConfig {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cfg == nil {
		observe.GlobalTrace("if: cfg == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: &provider.ThinkingConfig{Enabled: cfg.Enabled, BudgetTokens: cfg.BudgetTokens}")
	return &provider.ThinkingConfig{Enabled: cfg.Enabled, BudgetTokens: cfg.BudgetTokens}
}

func (rt *InteractiveRuntime) recordOrchestrationArtifact(root string, ev query.OrchestrationHandoffEvent) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rt == nil || rt.Deps == nil || rt.Deps.Store == nil || strings.TrimSpace(ev.Path) == "" {
		observe.GlobalTrace("if: rt == nil || rt.Deps == nil || rt.Deps.Store == nil || strings.TrimSpace(ev.P...")
		observe.GlobalTrace("return: nil")
		return nil
	}
	artifact := app.OrchestrationArtifact{
		StateID:    ev.StateID,
		From:       ev.From,
		Event:      ev.Event,
		To:         ev.To,
		ArtifactID: ev.ArtifactID,
		Path:       filepath.Clean(ev.Path),
		Direction:  ev.Direction,
		Root:       cleanNonEmptyPath(root),
		Bytes:      ev.Bytes,
		SHA256:     ev.SHA256,
		CreatedAt:  time.Now(),
	}
	rt.Deps.Store.Update(func(st *app.AppState) {
		st.OrchestrationArtifacts = upsertOrchestrationArtifact(st.OrchestrationArtifacts, artifact)
	})
	if rt.Deps.SessionWriter == nil {
		observe.GlobalTrace("if: rt.Deps.SessionWriter == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: rt.Deps.SessionWriter.WriteOrchestrationArtifacts(rt.Deps.Store.Snapshot().Or...")
	return rt.Deps.SessionWriter.WriteOrchestrationArtifacts(rt.Deps.Store.Snapshot().OrchestrationArtifacts)
}

func upsertOrchestrationArtifact(artifacts []app.OrchestrationArtifact, artifact app.OrchestrationArtifact) []app.OrchestrationArtifact {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for i := range artifacts {
		observe.GlobalTrace("range artifacts")
		if artifacts[i].Path == artifact.Path &&
			artifacts[i].StateID == artifact.StateID &&
			artifacts[i].From == artifact.From &&
			artifacts[i].Event == artifact.Event &&
			artifacts[i].To == artifact.To &&
			artifacts[i].ArtifactID == artifact.ArtifactID &&
			artifacts[i].Direction == artifact.Direction {
			observe.GlobalTrace("if: artifacts[i].Path == artifact.Path &&\n\tartifacts[i].StateID == artifact.State...")
			artifacts[i] = artifact
			observe.GlobalTrace("return: artifacts")
			return artifacts
		}
	}
	observe.GlobalTrace("return: append(artifacts, artifact)")
	return append(artifacts, artifact)
}

func cleanNonEmptyPath(path string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(path) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(path) == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: filepath.Clean(path)")
	return filepath.Clean(path)
}

func (rt *InteractiveRuntime) appendHookContext(event hook.Event, result hook.AggregatedResult, includeStdout bool) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rt == nil || rt.Engine == nil {
		observe.GlobalTrace("if: rt == nil || rt.Engine == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: rt.Engine.AppendHookContext(string(event), hookContextStrings(result, include...")
	return rt.Engine.AppendHookContext(string(event), hookContextStrings(result, includeStdout))
}

func interactiveOrchestrationArtifactRoot(d *Deps) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessionID := "unknown-session"
	if d != nil && d.Store != nil {
		observe.GlobalTrace("if: d != nil && d.Store != nil")
		if id := d.Store.Snapshot().Conversation.ID; id != "" {
			observe.GlobalTrace("if: id != \"\"")
			sessionID = id
		}
	}
	runID := time.Now().UTC().Format("20060102T150405.000000000Z")
	if home, err := config.PragmaHome(); err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: filepath.Join(home, \"orchestrations\", sessionID, runID)")
		return filepath.Join(home, "orchestrations", sessionID, runID)
	}
	observe.GlobalTrace("return: filepath.Join(os.TempDir(), \"pragma\", \"orchestrations\", sessionID, runID)")
	return filepath.Join(os.TempDir(), "pragma", "orchestrations", sessionID, runID)
}

func (rt *InteractiveRuntime) Resume(sessionID string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessStore, err := session.NewStore()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	sess, err := sessStore.Load(sessionID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if err := validateResumeWorkDir(rt.Deps.Store.Snapshot().CWD, sess); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if resetter, ok := rt.Deps.Checker.(permission.SessionRuleResetter); ok {
		observe.GlobalTrace("if: ok")
		resetter.ClearSessionRules()
	}
	rt.Deps.CostTracker.Reset(sess.CostUSD)
	rt.Deps.Metrics.Reset(observe.MetricsSeed{TokenUsage: sess.TokenUsage, TurnCount: sess.TurnCount})
	providerBinding, err := rt.resolveResumeProvider(sess.Conversation)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	w, err := sessStore.Open(sessionID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if err := rt.closeCurrentSession(context.Background()); err != nil {
		observe.GlobalTrace("if: err != nil")
		w.Close()
		observe.GlobalTrace("return: err")
		return err
	}
	rt.Deps.SessionWriter = w
	rt.Deps.SessionLastIdx = len(sess.Conversation.Messages)
	rt.applyResumeProvider(providerBinding)
	promptHistory := sessionPromptHistory(sess)
	resumedConv := resumedConversation(sess, rt.Deps.Cwd)
	rt.Deps.Store.Update(func(st *app.AppState) {
		st.Conversation = resumedConv
		st.Model = providerBinding.modelID
		st.Provider = providerBinding.providerName
		st.CWD = resumedConv.WorkDir
		st.PromptHistory = promptHistory
		st.OrchestrationArtifacts = append([]app.OrchestrationArtifact(nil), sess.OrchestrationArtifacts...)
		st.Worktree = copyWorktreeSession(sess.Worktree)
	})
	ensureCapabilitiesForActiveWorkDir(context.Background(), rt.Deps)
	rt.PromptHistory = promptHistory
	rt.Deps.SessionHeader = sessionHeaderForCurrentConversation(rt.Deps)
	rt.Deps.StartedAt = startedAtForConversation(sess.Conversation)
	rt.Engine.ResetSessionState(sess.ContentReplacements)
	rt.sessionSave, rt.sessionClose = makeSessionSaveClose(rt.Deps)
	rt.Engine.SetSessionCheckpoint(rt.sessionSave)
	// CMP-001.2 F2: auto-compaction rewrites the whole session file.
	rt.Engine.SetSessionRewrite(func() error { return rewriteCurrentSession(rt.Deps) })
	if err := startSessionRecording(rt.Deps); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func startedAtForConversation(conv model.Conversation) time.Time {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !conv.CreatedAt.IsZero() {
		observe.GlobalTrace("if: !conv.CreatedAt.IsZero()")
		observe.GlobalTrace("return: conv.CreatedAt")
		return conv.CreatedAt
	}
	observe.GlobalTrace("return: time.Now()")
	return time.Now()
}

func validateResumeWorkDir(activeWorkDir string, sess session.Session) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessionWorkDir := sess.Conversation.WorkDir
	if activeWorkDir == "" || sessionWorkDir == "" {
		observe.GlobalTrace("if: activeWorkDir == \"\" || sessionWorkDir == \"\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	activeClean := filepath.Clean(activeWorkDir)
	sessionClean := filepath.Clean(sessionWorkDir)
	if activeClean == sessionClean {
		observe.GlobalTrace("if: activeClean == sessionClean")
		observe.GlobalTrace("return: nil")
		return nil
	}
	if sess.Worktree != nil {
		observe.GlobalTrace("if: sess.Worktree != nil")
		originalClean := filepath.Clean(sess.Worktree.OriginalCWD)
		if activeClean == originalClean {
			observe.GlobalTrace("if: activeClean == originalClean")
			observe.GlobalTrace("return: validateResumableWorktree(sess.Worktree)")
			return validateResumableWorktree(sess.Worktree)
		}
	}
	observe.GlobalTrace("return: fmt.Errorf(\"cannot resume session from %s while runtime working directory is ...")
	return fmt.Errorf("cannot resume session from %s while runtime working directory is %s", sessionClean, activeClean)
}

func validateResumableWorktree(wt *app.WorktreeSession) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if wt == nil || wt.WorktreePath == "" {
		observe.GlobalTrace("if: wt == nil || wt.WorktreePath == \"\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	info, err := os.Stat(wt.WorktreePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"cannot resume worktree session at %s: %w\", wt.WorktreePath, err)")
		return fmt.Errorf("cannot resume worktree session at %s: %w", wt.WorktreePath, err)
	}
	if !info.IsDir() {
		observe.GlobalTrace("if: !info.IsDir()")
		observe.GlobalTrace("return: fmt.Errorf(\"cannot resume worktree session at %s: not a directory\", wt.Worktr...")
		return fmt.Errorf("cannot resume worktree session at %s: not a directory", wt.WorktreePath)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func copyWorktreeSession(wt *app.WorktreeSession) *app.WorktreeSession {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if wt == nil {
		observe.GlobalTrace("if: wt == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	cp := *wt
	observe.GlobalTrace("return: &cp")
	return &cp
}

type resumeProviderBinding struct {
	cfg          config.Config
	prov         provider.Provider
	providerName string
	modelID      string
}

func (rt *InteractiveRuntime) resolveResumeProvider(conv model.Conversation) (resumeProviderBinding, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cfg := rt.Deps.Cfg
	if conv.Provider != "" {
		observe.GlobalTrace("if: conv.Provider != \"\"")
		cfg.Provider = conv.Provider
	}
	if cfg.Provider == "" {
		observe.GlobalTrace("if: cfg.Provider == \"\"")
		cfg.Provider = "morphllm"
	}
	if conv.Model != "" {
		observe.GlobalTrace("if: conv.Model != \"\"")
		cfg.Model = conv.Model
	}
	if cfg.Model == "" {
		observe.GlobalTrace("if: cfg.Model == \"\"")
		cfg.Model = DefaultModelFor(cfg.Provider)
	}
	cfg.Model = resolveModelAlias(cfg.Provider, cfg.Model)
	cfg.APIKey = resumeAPIKeyForProvider(cfg.Provider, rt.Deps)

	prov, err := CreateProvider(cfg, rt.Deps.Bus)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: resumeProviderBinding{}, err")
		return resumeProviderBinding{}, err
	}
	observe.GlobalTrace("return: resumeProviderBinding{\n\tcfg:\t\tcfg,\n\tprov:\t\tprov,\n\tproviderName:\tcfg.Provider,...")
	return resumeProviderBinding{
		cfg:          cfg,
		prov:         prov,
		providerName: cfg.Provider,
		modelID:      cfg.Model,
	}, nil
}

func resumeAPIKeyForProvider(providerName string, d *Deps) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if providerName == "google-vertex" {
		observe.GlobalTrace("if: providerName == \"google-vertex\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if d.Cfg.Provider == providerName && d.Cfg.APIKey != "" {
		observe.GlobalTrace("if: d.Cfg.Provider == providerName && d.Cfg.APIKey != \"\"")
		observe.GlobalTrace("return: d.Cfg.APIKey")
		return d.Cfg.APIKey
	}
	if key := d.Creds.CredentialFor(providerName).APIKey; key != "" {
		observe.GlobalTrace("if: key != \"\"")
		observe.GlobalTrace("return: key")
		return key
	}
	observe.GlobalTrace("return: os.Getenv(envVarForProvider(providerName))")
	return os.Getenv(envVarForProvider(providerName))
}

func (rt *InteractiveRuntime) applyResumeProvider(binding resumeProviderBinding) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rt.Deps.Cfg = binding.cfg
	accountedProvider := provider.WithAccounting(binding.prov, rt.Deps.CostTracker, rt.Deps.Bus)
	rt.Deps.Prov = accountedProvider
	rt.Deps.EngineCfg.Model = binding.modelID
	rt.Engine.RebindProvider(accountedProvider, binding.modelID)
	RebindProviderBackedTools(rt.Deps)
	rt.SlashDeps.ModelName = binding.modelID
	rt.SlashDeps.Provider = binding.providerName
	rt.SlashDeps.ContextWindowFunc = accountedProvider.ContextWindow
	rt.rebindCompaction()
	if cw, ok := accountedProvider.ContextWindow(binding.modelID); ok {
		observe.GlobalTrace("if: ok")
		rt.Deps.TokenMonitor.SetBudget(cw)
	}
}

func (rt *InteractiveRuntime) switchActiveModel(modelID string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	providerName, bareID := parseModelTarget(modelID, rt.Deps.Cfg.Provider)
	if bareID == "" {
		observe.GlobalTrace("if: bareID == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"model is required\")")
		return fmt.Errorf("model is required")
	}
	if providerName != rt.Deps.Cfg.Provider {
		observe.GlobalTrace("if: providerName != rt.Deps.Cfg.Provider — cross-provider switch")
		observe.GlobalTrace("return: rt.switchProviderModel(providerName, bareID)")
		return rt.switchProviderModel(providerName, bareID)
	}
	if err := switchActiveModel(rt.Deps, bareID); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	rt.SlashDeps.ModelName = bareID
	rt.rebindCompaction()
	observe.GlobalTrace("return: nil")
	return nil
}

// switchProviderModel switches the runtime to another provider's model,
// reusing the resume rebinding path (engine, tools, compaction, budget).
// Requires credentials for the target provider.
func (rt *InteractiveRuntime) switchProviderModel(providerName, modelID string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d := rt.Deps
	if !isKnownProvider(providerName) {
		observe.GlobalTrace("if: !isKnownProvider(providerName)")
		observe.GlobalTrace("return: fmt.Errorf(\"unknown provider %q\", providerName)")
		return fmt.Errorf("unknown provider %q", providerName)
	}
	key := resumeAPIKeyForProvider(providerName, d)
	if key == "" && providerName != "google-vertex" {
		observe.GlobalTrace("if: key == \"\" && providerName != \"google-vertex\"")
		observe.GlobalTrace("return: fmt.Errorf(\"no API key for provider\")")
		return fmt.Errorf("no API key for provider %q — add it to ~/.pragma/credentials.yml or set %s", providerName, envVarForProvider(providerName))
	}
	// Resolve short aliases before validating against the catalog.
	modelID = resolveModelAlias(providerName, modelID)
	if d.ModelCatalog != nil && !d.ModelCatalog.Knows(providerName, modelID) {
		observe.GlobalTrace("if: !d.ModelCatalog.Knows(providerName, modelID)")
		observe.GlobalTrace("return: fmt.Errorf(\"unknown model for provider\")")
		return fmt.Errorf("unknown model %q for provider %s", modelID, providerName)
	}
	cfg := d.Cfg
	cfg.Provider = providerName
	cfg.Model = modelID
	cfg.APIKey = key
	prov, err := CreateProvider(cfg, d.Bus)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	rt.applyResumeProvider(resumeProviderBinding{
		cfg:          cfg,
		prov:         prov,
		providerName: providerName,
		modelID:      modelID,
	})
	// Record the new provider on the conversation so resume and the picker
	// highlight agree with the live runtime.
	d.Store.Update(func(s *app.AppState) {
		s.Model = modelID
		s.Provider = providerName
		s.Conversation.Model = modelID
		s.Conversation.Provider = providerName
	})
	// Warm the catalog for the newly active provider.
	go d.ModelCatalog.Refresh(context.Background(), providerName, key)
	observe.GlobalTrace("return: nil")
	return nil
}

// parseModelTarget interprets a /model argument. A "provider/model" prefix
// is treated as a provider qualifier only when the prefix names a known
// provider — bare model IDs that themselves contain slashes (OpenRouter's
// "z-ai/glm-5.3") stay model IDs for the active provider.
func parseModelTarget(input, activeProvider string) (providerName, modelID string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	input = strings.TrimSpace(input)
	if idx := strings.Index(input, "/"); idx > 0 {
		observe.GlobalTrace("if: idx := strings.Index(input, \"/\"); idx > 0")
		prefix := input[:idx]
		if isKnownProvider(prefix) {
			observe.GlobalTrace("if: isKnownProvider(prefix)")
			observe.GlobalTrace("return: prefix, input[idx+1:]")
			return prefix, input[idx+1:]
		}
	}
	observe.GlobalTrace("return: activeProvider, input")
	return activeProvider, input
}

// isKnownProvider reports whether name is a supported provider.
func isKnownProvider(name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, p := range knownProviders {
		observe.GlobalTrace("range knownProviders")
		if p == name {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func (rt *InteractiveRuntime) rebindCompaction() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	compDeps, compactor := BuildCompactionDeps(rt.Deps)
	rt.Engine.SetCompaction(compDeps)
	rt.SlashDeps.Compactor = compactor
	rt.SlashDeps.ContextWindowFunc = rt.Deps.Prov.ContextWindow
}

func (rt *InteractiveRuntime) CloseSession() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: rt.closeCurrentSession(context.Background())")
	return rt.closeCurrentSession(context.Background())
}

func (rt *InteractiveRuntime) closeCurrentSession(ctx context.Context) error {
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "enter")
	defer observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "exit")
	if rt == nil || rt.Deps == nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "if: rt == nil || rt.Deps == nil")
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "return: nil")
		return nil
	}
	if rt.sessionSave != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "if: rt.sessionSave != nil")
		if err := rt.sessionSave(); err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "if: err != nil")
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "return: err")
			return err
		}
	}
	reportProviderCleanup(ctx, rt.Deps.Prov, rt.Deps.Bus)
	if rt.sessionClose != nil {
		observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "if: rt.sessionClose != nil")
		if err := rt.sessionClose(); err != nil {
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "if: err != nil")
			observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "return: err")
			return err
		}
	}
	observe.TraceCtx(ctx, "cli", "InteractiveRuntime.closeCurrentSession", "return: nil")
	return nil
}

func reportProviderCleanup(ctx context.Context, prov provider.Provider, bus *observe.EventBus) {
	observe.TraceCtx(ctx, "cli", "reportProviderCleanup", "enter")
	defer observe.TraceCtx(ctx, "cli", "reportProviderCleanup", "exit")
	if err := provider.Close(ctx, prov); err != nil && bus != nil {
		observe.TraceCtx(ctx, "cli", "reportProviderCleanup", "if: err != nil && bus != nil")
		bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
			Severity:     "warn",
			Component:    "provider",
			ErrorType:    "provider_cleanup_failed",
			ErrorMessage: err.Error(),
		})
	}
}

// BuildInteractiveRuntime wires the shared dependencies for an interactive UI.
func BuildInteractiveRuntime(cmd *cobra.Command, prompter permission.Prompter) (*InteractiveRuntime, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: BuildInteractiveRuntimeWithOptions(cmd, prompter, InteractiveRuntimeOptions{})")
	return BuildInteractiveRuntimeWithOptions(cmd, prompter, InteractiveRuntimeOptions{})
}

func BuildInteractiveRuntimeWithOptions(cmd *cobra.Command, prompter permission.Prompter, opts InteractiveRuntimeOptions) (*InteractiveRuntime, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d, err := SetupDepsWithOptions(cmd, opts.SetupDeps)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	if opts.ConfigureDeps != nil {
		observe.GlobalTrace("if: opts.ConfigureDeps != nil")
		opts.ConfigureDeps(d)
	}

	engine, err := RegisterTools(d, prompter)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		if d.Cleanup != nil {
			observe.GlobalTrace("if: d.Cleanup != nil")
			d.Cleanup()
		}
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	if d.SessionWriter != nil {
		observe.GlobalTrace("if: d.SessionWriter != nil")
		if err := startSessionRecording(d); err != nil {
			observe.GlobalTrace("if: err != nil")
			if d.Cleanup != nil {
				observe.GlobalTrace("if: d.Cleanup != nil")
				d.Cleanup()
			}
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
	}

	compDeps, compactor := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)
	engine.SetSessionCheckpoint(sessionSaveFn)
	// CMP-001.2 F2: auto-compaction rewrites the whole session file.
	engine.SetSessionRewrite(func() error { return rewriteCurrentSession(d) })

	slashCmds := slash.NewRegistry()
	skillCatalog := runtimeSkillCatalog(d)

	sessStore, _ := session.NewStore()
	promptHistory := d.Store.Snapshot().PromptHistory
	if len(promptHistory) == 0 {
		observe.GlobalTrace("if: len(promptHistory) == 0")
		promptHistory = promptHistoryFromSessions(sessStore, tui.InputHistoryLimit)
		d.Store.Update(func(st *app.AppState) {
			st.PromptHistory = append([]string(nil), promptHistory...)
		})
	}
	if d.ModelCatalog == nil {
		observe.GlobalTrace("if: d.ModelCatalog == nil")
		d.ModelCatalog = NewModelCatalog(d.Creds)
	}
	// Populate the cross-provider model catalog in the background so the
	// first /models open already shows live listings where credentials exist.
	go d.ModelCatalog.Refresh(context.Background(), d.Cfg.Provider, d.Cfg.APIKey)
	slashDeps := slash.Deps{
		Store:          d.Store,
		CostTracker:    d.CostTracker,
		Compactor:      compactor,
		Bus:            d.Bus,
		SessionSave:    sessionSaveFn,
		ModelName:      d.Cfg.Model,
		Provider:       d.Cfg.Provider,
		Cwd:            d.Cwd,
		ClipboardWrite: clipboard.WriteAll,
		// Cross-provider catalog: qualified "provider/model" IDs for every
		// provider with credentials, the active provider first.
		ModelLister: func() []string {
			if d.ModelCatalog != nil {
				if ids := d.ModelCatalog.QualifiedModelIDs(d.Cfg.Provider, d.Cfg.APIKey); len(ids) > 0 {
					return ids
				}
			}
			if ml, ok := d.Prov.(provider.ModelLister); ok {
				return ml.ListModels()
			}
			return nil
		},
		KnownProviders:    knownProviders,
		ContextWindowFunc: d.Prov.ContextWindow,
		ModelSwitcher:     nil,
		OnModelChanged: func(modelID string) {
			if cw, ok := d.Prov.ContextWindow(modelID); ok {
				d.TokenMonitor.SetBudget(cw)
			}
		},
		McpStatus: func() []slash.McpServerStatus {
			ensureCapabilitiesForActiveWorkDir(context.Background(), d)
			return mcpStatusesForSlash(d.McpManager)
		},
		SessionStore: sessStore,
		SkillCatalog: skillCatalog,
	}

	rt := &InteractiveRuntime{
		Deps:          d,
		Engine:        engine,
		SlashCmds:     slashCmds,
		SlashDeps:     slashDeps,
		PromptHistory: promptHistory,
		sessionSave:   sessionSaveFn,
		sessionClose:  sessionCloseFn,
	}
	rt.SlashDeps.ModelSwitcher = rt.switchActiveModel
	d.ModelSwitcher = rt.switchActiveModel
	rt.Cleanup = func(ctx context.Context) {
		_ = rt.closeCurrentSession(ctx)
		if d.Cleanup != nil {
			d.Cleanup()
		}
	}
	observe.GlobalTrace("return: rt, nil")
	return rt, nil
}

// RunInteractive launches the terminal UI for multi-turn conversation.
func RunInteractive(cmd *cobra.Command) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: RunTUIInteractive(cmd)")
	return RunTUIInteractive(cmd)
}

// RunTUIInteractive launches the Bubble Tea TUI using the shared runtime.
func RunTUIInteractive(cmd *cobra.Command) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	prompter := tui.NewInteractivePrompter()
	rt, err := BuildInteractiveRuntime(cmd, prompter)
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
		TokenMonitor:   rt.Deps.TokenMonitor,
		Metrics:        rt.Deps.Metrics,
		Workspace:      rt.Deps.Cwd,
		Version:        buildinfo.Version,
		StartedAt:      rt.Deps.StartedAt,
		McpServerNames: connectedMcpNames(rt.Deps.McpManager),
		PromptHistory:  rt.PromptHistory,
	})

	log.SetOutput(io.Discard)

	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	prompter.SetProgram(program)

	if _, err := program.Run(); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	observe.GlobalTrace("return: nil")

	return nil
}

type StandaloneOrchestrationOptions struct {
	DefinitionPath string
	PersonaDir     string
	Prompt         string
	SeedArtifacts  map[string]string
	StartAtState   string
	StopAfterState string
	Stdout         io.Writer
	Stderr         io.Writer
}

func RunStandaloneOrchestration(cmd *cobra.Command, opts StandaloneOrchestrationOptions) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(opts.Prompt) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(opts.Prompt) == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"--prompt is required for orchestration run; use /orchestrate from...")
		return fmt.Errorf("--prompt is required for orchestration run; use /orchestrate from interactive Pragma for browser-driven orchestration")
	}
	stdout := opts.Stdout
	if stdout == nil {
		observe.GlobalTrace("if: stdout == nil")
		stdout = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		observe.GlobalTrace("if: stderr == nil")
		stderr = os.Stderr
	}

	prompter := &permission.NonInteractivePrompter{}
	rt, err := BuildInteractiveRuntimeWithOptions(cmd, prompter, InteractiveRuntimeOptions{
		SetupDeps: SetupDepsOptions{DefaultPermissionMode: permission.ModeBypassPermissions},
		ConfigureDeps: func(d *Deps) {
			d.Bus.Subscribe(d.StderrLogger)
		},
	})
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	defer rt.Cleanup(cmd.Context())

	promptHookResult, err := acceptPromptSubmission(cmd.Context(), rt.Deps, opts.Prompt)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	rt.rememberAcceptedPrompt(opts.Prompt)

	events := make(chan interactive.Event, 16)
	go func() {
		defer close(events)
		rt.runOrchestration(cmd.Context(), slash.OrchestrationRequest{
			DefinitionPath: opts.DefinitionPath,
			PersonaDir:     opts.PersonaDir,
			Prompt:         opts.Prompt,
			SeedArtifacts:  opts.SeedArtifacts,
			StartAtState:   opts.StartAtState,
			StopAfterState: opts.StopAfterState,
		}, opts.Prompt, promptHookResult, events)
	}()
	observe.GlobalTrace("return: consumeStandaloneOrchestrationEvents(events, stdout, stderr)")
	return consumeStandaloneOrchestrationEvents(events, stdout, stderr)
}

func consumeStandaloneOrchestrationEvents(events <-chan interactive.Event, stdout, stderr io.Writer) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for ev := range events {
		observe.GlobalTrace("range events")
		switch e := ev.(type) {
		case interactive.LoopEvent:
			observe.GlobalTrace("typecase: interactive.LoopEvent")
			done, err := printStandaloneOrchestrationLoopEvent(e.Event, stdout, stderr)
			if err != nil || done {
				observe.GlobalTrace("return: err")
				return err
			}
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func printStandaloneOrchestrationLoopEvent(ev query.LoopEvent, stdout, stderr io.Writer) (bool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch e := ev.(type) {
	case query.TextEvent:
		observe.GlobalTrace("typecase: query.TextEvent")
		fmt.Fprint(stdout, e.Text)
		flushWriter(stdout)
	case query.OrchestrationStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStartedEvent")
		fmt.Fprintf(stdout, "[orchestration: %s initial=%s]\n", e.Name, e.Initial)
		flushWriter(stdout)
	case query.OrchestrationStateStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateStartedEvent")
		if e.Control != "" {
			fmt.Fprintf(stdout, "\n[control: %s (%s)]\n", e.StateID, e.Control)
		} else {
			fmt.Fprintf(stdout, "\n[orchestration: %s persona=%s]\n", e.StateID, e.PersonaID)
		}
		flushWriter(stdout)
	case query.OrchestrationStateCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateCompletedEvent")
		fmt.Fprintf(stdout, "\n[state %s complete in %s]\n", e.StateID, e.Duration.Round(time.Second))
		flushWriter(stdout)
	case query.OrchestrationControlEvent:
		observe.GlobalTrace("typecase: query.OrchestrationControlEvent")
		if e.Event != "" {
			fmt.Fprintf(stdout, "[control: %s emitted %s]\n", e.StateID, e.Event)
			flushWriter(stdout)
		}
	case query.OrchestrationTransitionEvent:
		observe.GlobalTrace("typecase: query.OrchestrationTransitionEvent")
		fmt.Fprintf(stdout, "\n[transition: %s --%s--> %s]\n", e.From, e.Event, e.To)
		flushWriter(stdout)
	case query.OrchestrationCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationCompletedEvent")
		fmt.Fprint(stdout, "\n[orchestration: done]\n")
		flushWriter(stdout)
	case query.ThinkingEvent:
		observe.GlobalTrace("typecase: query.ThinkingEvent")
		if e.Text != "" {
			fmt.Fprint(stderr, e.Text)
			flushWriter(stderr)
		}
	case query.ToolCallEvent:
		observe.GlobalTrace("typecase: query.ToolCallEvent")
		fmt.Fprintf(stderr, "[tool: %s]\n", e.Call.Name)
		flushWriter(stderr)
	case query.ToolResultEvent:
		observe.GlobalTrace("typecase: query.ToolResultEvent")
		fmt.Fprintf(stderr, "[result: %s]\n", e.Result.ToolCallID)
		flushWriter(stderr)
	case query.UserMessageEvent:
		observe.GlobalTrace("typecase: query.UserMessageEvent")
		printUserMessageEvent(stdout, e)
	case query.RetryEvent:
		observe.GlobalTrace("typecase: query.RetryEvent")
		fmt.Fprintf(stderr, "[retry: %s in %s]\n", e.Kind, e.Delay)
		flushWriter(stderr)
	case query.ErrorEvent:
		observe.GlobalTrace("typecase: query.ErrorEvent")
		return false, e.Err
	case query.TurnCompleteEvent:
		observe.GlobalTrace("typecase: query.TurnCompleteEvent")
		return true, nil
	}
	observe.GlobalTrace("return: false, nil")
	return false, nil
}

type nonInteractiveRunOptions struct {
	AllowStructuredOutput bool
	PrintCost             bool
	ConfigureDeps         func(*Deps)
	PreparePrompt         func(context.Context, *Deps) (nonInteractivePromptPlan, error)
}

type nonInteractivePromptPlan struct {
	Prompt      string
	DisplayText string
	Run         bool
}

// RunNonInteractive runs a single prompt and exits.
func RunNonInteractive(cmd *cobra.Command, _ []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: runNonInteractive(cmd, nonInteractiveRunOptions{\n\tAllowStructuredOutput:\ttrue...")
	return runNonInteractive(cmd, nonInteractiveRunOptions{
		AllowStructuredOutput: true,
		PrintCost:             true,
	})
}

func runNonInteractive(cmd *cobra.Command, opts nonInteractiveRunOptions) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d, err := SetupDepsWithOptions(cmd, SetupDepsOptions{DefaultPermissionMode: permission.ModeBypassPermissions})
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
			ownerToken := os.Getenv("PRAGMA_BG_SESSION_TOKEN")
			cancelHeartbeat := background.StartHeartbeat(cmd.Context(), reg, os.Getpid(), ownerToken)
			defer cancelHeartbeat()
			defer reg.Unregister(os.Getpid())
			sub := background.NewStatusSubscriber(reg, ownerToken)
			d.Bus.Subscribe(sub)
		}
	}

	if opts.ConfigureDeps != nil {
		observe.GlobalTrace("if: opts.ConfigureDeps != nil")
		opts.ConfigureDeps(d)
	}

	schemaFlag, _ := cmd.Flags().GetString("output-schema")
	if !opts.AllowStructuredOutput {
		observe.GlobalTrace("if: !opts.AllowStructuredOutput")
		schemaFlag = ""
	}
	var schemaJSON json.RawMessage
	var nativeStructuredOutput bool
	if schemaFlag != "" {
		observe.GlobalTrace("if: schemaFlag != \"\"")
		var err error
		schemaJSON, err = loadOutputSchema(schemaFlag)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"invalid output schema: %w\", err)")
			return fmt.Errorf("invalid output schema: %w", err)
		}
		nativeStructuredOutput = d.Prov.SupportsFeature(provider.FeatureStructuredOutput)
		if nativeStructuredOutput {
			observe.GlobalTrace("if: nativeStructuredOutput")
			d.EngineCfg.ResponseSchema = schemaJSON
		} else {
			observe.GlobalTrace("else: nativeStructuredOutput")
			observe.GlobalTrace("return: fmt.Errorf(\"structured output requires provider support\")")
			return fmt.Errorf("structured output requires provider support")
		}
	}

	prompter := &permission.NonInteractivePrompter{}
	engine, err := RegisterTools(d, prompter)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}

	compDeps, _ := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	prompt, _ := cmd.Flags().GetString("prompt")
	resumeID, _ := cmd.Flags().GetString("resume")
	if resumeID != "" && prompt == "" {
		observe.GlobalTrace("if: resumeID != \"\" && prompt == \"\"")
		prompt = "Continue from where we left off."
	}
	if opts.PreparePrompt != nil {
		observe.GlobalTrace("if: opts.PreparePrompt != nil")
		plan, err := opts.PreparePrompt(cmd.Context(), d)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
		if plan.DisplayText != "" {
			observe.GlobalTrace("if: plan.DisplayText != \"\"")
			fmt.Println(plan.DisplayText)
		}
		if !plan.Run {
			observe.GlobalTrace("if: !plan.Run")
			observe.GlobalTrace("return: nil")
			return nil
		}
		prompt = plan.Prompt
	}

	sessionSaveFn, sessionCloseFn := makeSessionSaveClose(d)
	engine.SetSessionCheckpoint(sessionSaveFn)
	// CMP-001.2 F2: auto-compaction rewrites the whole session file.
	engine.SetSessionRewrite(func() error { return rewriteCurrentSession(d) })

	ctx := cmd.Context()
	promptHookResult, err := acceptPromptSubmission(ctx, d, prompt)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if strings.TrimSpace(prompt) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(prompt) != \"\"")
		if err := startSessionForCurrentConversation(ctx, d); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
		if err := engine.AppendHookContext(string(hook.UserPromptSubmit), hookContextStrings(promptHookResult, false)); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
	}
	events := engine.Run(ctx, prompt)

	hasStructuredOutput := schemaFlag != ""
	var nativeStructuredText strings.Builder

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
			if nativeStructuredOutput {
				nativeStructuredText.WriteString(e.Text)
			} else if !hasStructuredOutput {
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
		case query.UserMessageEvent:
			observe.GlobalTrace("typecase: query.UserMessageEvent")
			if hasStructuredOutput {
				printUserMessageEvent(os.Stderr, e)
			} else {
				printUserMessageEvent(out, e)
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
			if err := sessionCloseFn(); err != nil {
				observe.GlobalTrace("return: err")
				return err
			}
			return e.Err
		}
	}

	if nativeStructuredOutput {
		observe.GlobalTrace("else-if: nativeStructuredOutput")
		if text := strings.TrimSpace(nativeStructuredText.String()); text != "" {
			observe.GlobalTrace("if: text != \"\"")
			fmt.Fprintln(out, text)
		}
	}

	if err := sessionCloseFn(); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}

	if opts.PrintCost {
		observe.GlobalTrace("if: opts.PrintCost")
		fmt.Fprintf(os.Stderr, "\ntotal cost: $%.6f\n", d.CostTracker.TotalUSD())
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func flushWriter(w io.Writer) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if f, ok := w.(*os.File); ok {
		observe.GlobalTrace("if: ok")
		_ = f.Sync()
	}
}

func hookContextStrings(result hook.AggregatedResult, includeStdout bool) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	contexts := make([]string, 0, len(result.Feedback)+1)
	for _, feedback := range result.Feedback {
		observe.GlobalTrace("range result.Feedback")
		if trimmed := strings.TrimSpace(feedback); trimmed != "" {
			observe.GlobalTrace("if: trimmed != \"\"")
			contexts = append(contexts, trimmed)
		}
	}
	if includeStdout {
		observe.GlobalTrace("if: includeStdout")
		if trimmed := strings.TrimSpace(result.Stdout); trimmed != "" {
			observe.GlobalTrace("if: trimmed != \"\"")
			contexts = append(contexts, trimmed)
		}
	}
	observe.GlobalTrace("return: contexts")
	return contexts
}

func startSessionForCurrentConversation(ctx context.Context, d *Deps) error {
	observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "enter")
	defer observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "exit")
	if d.SessionWriter != nil {
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "if: d.SessionWriter != nil")
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "return: startSessionRecording(d)")
		return startSessionRecording(d)
	}
	header := d.SessionHeader
	if header.SessionID == "" {
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "if: header.SessionID == \"\"")
		header = sessionHeaderForCurrentConversation(d)
	}
	sessStore, err := session.NewStore()
	if err != nil {
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "if: err != nil")
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "return: err")
		return err
	}
	w, err := sessStore.Create(header)
	if err != nil {
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "if: err != nil")
		observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "return: err")
		return err
	}
	d.SessionWriter = w
	d.SessionHeader = header
	d.SessionLastIdx = 0
	observe.TraceCtx(ctx, "cli", "startSessionForCurrentConversation", "return: startSessionRecording(d)")
	return startSessionRecording(d)
}

func sessionHeaderForCurrentConversation(d *Deps) session.HeaderData {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := d.Store.Snapshot()
	modelID := snap.Model
	if modelID == "" {
		observe.GlobalTrace("if: modelID == \"\"")
		modelID = d.Cfg.Model
	}
	providerName := snap.Provider
	if providerName == "" {
		observe.GlobalTrace("if: providerName == \"\"")
		providerName = d.Cfg.Provider
	}
	observe.GlobalTrace("return: session.HeaderData{\n\tSessionID:\tsnap.Conversation.ID,\n\tModel:\t\tmodelID,\n\tProv...")
	return session.HeaderData{
		SessionID: snap.Conversation.ID,
		Model:     modelID,
		Provider:  providerName,
		WorkDir:   d.Cwd,
		GitRemote: gitutil.RemoteURL(d.Cwd),
		CreatedAt: snap.Conversation.CreatedAt,
		System:    snap.Conversation.System,
	}
}

func startSessionRecording(d *Deps) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d == nil || !d.Cfg.Record || d.recorder != nil {
		observe.GlobalTrace("if: d == nil || !d.Cfg.Record || d.recorder != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	path, err := sessionRecordingPath(d)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	recorder, err := observe.NewRecorder(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"create recorder: %w\", err)")
		return fmt.Errorf("create recorder: %w", err)
	}
	d.Bus.Subscribe(recorder)
	d.recorder = recorder
	d.RecordingPath = path
	fmt.Fprintf(os.Stderr, "Recording events to %s\n", path)
	observe.GlobalTrace("return: nil")
	return nil
}

func sessionRecordingPath(d *Deps) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessionID := ""
	if d != nil && d.Store != nil {
		observe.GlobalTrace("if: d != nil && d.Store != nil")
		sessionID = d.Store.Snapshot().Conversation.ID
	}
	if sessionID == "" && d != nil {
		observe.GlobalTrace("if: sessionID == \"\" && d != nil")
		sessionID = d.SessionHeader.SessionID
	}
	if sessionID == "" {
		observe.GlobalTrace("if: sessionID == \"\"")
		sessionID = "unknown-session"
	}
	home, err := config.PragmaHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"resolve pragma home: %w\", err)")
		return "", fmt.Errorf("resolve pragma home: %w", err)
	}
	dir := filepath.Join(home, "recordings", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"create recording directory: %w\", err)")
		return "", fmt.Errorf("create recording directory: %w", err)
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + ".jsonl"
	observe.GlobalTrace("return: filepath.Join(dir, name), nil")
	return filepath.Join(dir, name), nil
}

func activeHookWorkDir(d *Deps) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d == nil {
		observe.GlobalTrace("if: d == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if d.Store != nil {
		observe.GlobalTrace("if: d.Store != nil")
		if cwd := d.Store.Snapshot().CWD; cwd != "" {
			observe.GlobalTrace("if: cwd != \"\"")
			observe.GlobalTrace("return: cwd")
			return cwd
		}
	}
	observe.GlobalTrace("return: d.Cwd")
	return d.Cwd
}

func latestAssistantText(store *app.StateStore) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if store == nil {
		observe.GlobalTrace("if: store == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	messages := store.Snapshot().Conversation.Messages
	for i := len(messages) - 1; i >= 0; i-- {
		observe.GlobalTrace("for: i >= 0")
		msg := messages[i]
		if msg.Role != model.RoleAssistant || msg.Flags.IsInternal || msg.Flags.IsMeta {
			observe.GlobalTrace("if: msg.Role != model.RoleAssistant || msg.Flags.IsInternal || msg.Flags.IsMeta")
			continue
		}
		var parts []string
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tp, ok := part.(model.TextPart); ok && strings.TrimSpace(tp.Text) != "" {
				observe.GlobalTrace("if: ok && strings.TrimSpace(tp.Text) != \"\"")
				parts = append(parts, tp.Text)
			}
		}
		if len(parts) > 0 {
			observe.GlobalTrace("if: len(parts) > 0")
			observe.GlobalTrace("return: strings.Join(parts, \"\\n\")")
			return strings.Join(parts, "\n")
		}
	}
	observe.GlobalTrace("return: \"\"")
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
	secondaryModel := SecondaryModelFor(d.Cfg.Provider, d.Cfg.Model)
	compactor := compact.NewService(d.Prov, d.Bus, d.CostTracker, secondaryModel)

	disableAutoCompact := os.Getenv("DISABLE_AUTO_COMPACT") == "1" || os.Getenv("DISABLE_AUTO_COMPACT") == "true"
	autoTracker := compact.NewAutoTracker(disableAutoCompact)

	ctxWindow := 200_000
	if cw, ok := d.Prov.ContextWindow(d.Cfg.Model); ok {
		observe.GlobalTrace("if: ok")
		ctxWindow = cw
	}

	sysTokEst := compact.EstimateSystemPromptTokens(model.SystemPrompt{
		Blocks: []model.SystemBlock{{Text: query.PragmaLoopSystemPrompt()}},
	})
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
func makeSessionSaveClose(d *Deps) (saveFn func() error, closeFn func() error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	closed := false
	saveFn = func() error {
		if d.SessionWriter == nil || closed {
			return nil
		}
		snap := d.Store.Snapshot()
		for i := d.SessionLastIdx; i < len(snap.Conversation.Messages); i++ {
			if err := d.SessionWriter.WriteMessage(snap.Conversation.Messages[i]); err != nil {
				return err
			}
		}
		if err := d.SessionWriter.WriteOrchestrationArtifacts(snap.OrchestrationArtifacts); err != nil {
			return err
		}
		d.SessionLastIdx = len(snap.Conversation.Messages)
		if err := d.SessionWriter.WriteMetadata(sessionMetadataForSnapshot(d, snap)); err != nil {
			return err
		}
		fileSize, _ := d.SessionWriter.Size()
		if d.Bus != nil {
			d.Bus.Emit(observe.SessionSaved{
				EventHeader:   observe.NewEventHeader("SessionSaved", "", snap.Conversation.ID, ""),
				SessionID:     snap.Conversation.ID,
				MessageCount:  len(snap.Conversation.Messages),
				FileSizeBytes: fileSize,
			})
		}
		return nil
	}
	closeFn = func() error {
		if d.SessionWriter != nil && !closed {
			if err := d.SessionWriter.Close(); err != nil {
				return err
			}
			closed = true
		}
		return nil
	}
	return
}

func rewriteCurrentSession(d *Deps) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d.SessionWriter == nil {
		observe.GlobalTrace("if: d.SessionWriter == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	snap := d.Store.Snapshot()
	header := d.SessionHeader
	if header.SessionID == "" {
		observe.GlobalTrace("if: header.SessionID == \"\"")
		header = sessionHeaderForCurrentConversation(d)
		d.SessionHeader = header
	}
	existing, err := loadCurrentSessionForRewrite(header.SessionID)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if err := d.SessionWriter.Rewrite(session.RewriteData{
		Header:                 header,
		Messages:               snap.Conversation.Messages,
		Metadata:               sessionMetadataForSnapshot(d, snap),
		ContentReplacements:    existing.ContentReplacements,
		PromptHistory:          existing.PromptHistory,
		OrchestrationArtifacts: snap.OrchestrationArtifacts,
	}); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	d.SessionLastIdx = len(snap.Conversation.Messages)
	observe.GlobalTrace("return: nil")
	return nil
}

func loadCurrentSessionForRewrite(sessionID string) (session.Session, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	store, err := session.NewStore()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: session.Session{}, err")
		return session.Session{}, err
	}
	observe.GlobalTrace("return: store.Load(sessionID)")
	return store.Load(sessionID)
}

func sessionMetadataForSnapshot(d *Deps, snap app.AppState) session.MetadataData {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msnap := d.Metrics.Snapshot()
	modelID := snap.Model
	if modelID == "" {
		observe.GlobalTrace("if: modelID == \"\"")
		modelID = d.Cfg.Model
	}
	providerName := snap.Provider
	if providerName == "" {
		observe.GlobalTrace("if: providerName == \"\"")
		providerName = d.Cfg.Provider
	}
	observe.GlobalTrace("return: session.MetadataData{\n\tCostUSD:\td.CostTracker.TotalUSD(),\n\tTurnCount:\tmsnap.T...")
	return session.MetadataData{
		CostUSD:    d.CostTracker.TotalUSD(),
		TurnCount:  msnap.TurnCount,
		TokenUsage: msnap.TokenUsage,
		UpdatedAt:  snap.Conversation.UpdatedAt,
		Summary:    extractSummary(snap),
		Model:      modelID,
		Provider:   providerName,
		WorkDir:    snap.CWD,
		Worktree:   copyWorktreeSession(snap.Worktree),
	}
}

func promptHistoryFromSessions(store *session.Store, limit int) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if store == nil || limit <= 0 {
		observe.GlobalTrace("if: store == nil || limit <= 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	summaries, err := store.List()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	seen := make(map[string]bool)
	newestFirst := make([]string, 0, limit)
	for _, summary := range summaries {
		observe.GlobalTrace("range summaries")
		sess, err := store.Load(summary.ID)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			continue
		}
		prompts := sessionPromptHistory(sess)
		for i := len(prompts) - 1; i >= 0; i-- {
			observe.GlobalTrace("for: i >= 0")
			prompt := prompts[i]
			if seen[prompt] {
				observe.GlobalTrace("if: seen[prompt]")
				continue
			}
			seen[prompt] = true
			newestFirst = append(newestFirst, prompt)
			if len(newestFirst) >= limit {
				observe.GlobalTrace("if: len(newestFirst) >= limit")
				observe.GlobalTrace("return: reversePromptHistory(newestFirst)")
				return reversePromptHistory(newestFirst)
			}
		}
	}
	observe.GlobalTrace("return: reversePromptHistory(newestFirst)")
	return reversePromptHistory(newestFirst)
}

func sessionPromptHistory(sess session.Session) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(sess.PromptHistory) > 0 {
		observe.GlobalTrace("if: len(sess.PromptHistory) > 0")
		prompts := make([]string, 0, len(sess.PromptHistory))
		for _, entry := range sess.PromptHistory {
			observe.GlobalTrace("range sess.PromptHistory")
			if entry.Text != "" {
				observe.GlobalTrace("if: entry.Text != \"\"")
				prompts = append(prompts, entry.Text)
			}
		}
		observe.GlobalTrace("return: prompts")
		return prompts
	}
	prompts := model.ExtractUserTextPrompts(sess.Conversation.Messages)
	filtered := prompts[:0]
	for _, prompt := range prompts {
		observe.GlobalTrace("range prompts")
		if generatedPromptHistory(prompt) {
			observe.GlobalTrace("if: generatedPromptHistory(prompt)")
			continue
		}
		filtered = append(filtered, prompt)
	}
	observe.GlobalTrace("return: filtered")
	return filtered
}

func generatedPromptHistory(text string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	trimmed := strings.TrimSpace(text)
	observe.GlobalTrace("return: strings.HasPrefix(trimmed, \"Please solve this task:\") ||\n\t(strings.HasPrefix(...")
	return strings.HasPrefix(trimmed, "Please solve this task:") ||
		(strings.HasPrefix(trimmed, "## Task") && strings.Contains(trimmed, "## Possible Next Phase Handoffs"))
}

func reversePromptHistory(newestFirst []string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	history := make([]string, len(newestFirst))
	for i := range newestFirst {
		observe.GlobalTrace("range newestFirst")
		history[len(newestFirst)-1-i] = newestFirst[i]
	}
	observe.GlobalTrace("return: history")
	return history
}

func appendPromptHistory(history []string, text string, limit int) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text = strings.TrimSpace(text)
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		observe.GlobalTrace("return: append([]string(nil), history...)")
		return append([]string(nil), history...)
	}
	out := append([]string(nil), history...)
	if len(out) > 0 && out[len(out)-1] == text {
		observe.GlobalTrace("if: len(out) > 0 && out[len(out)-1] == text")
		observe.GlobalTrace("return: out")
		return out
	}
	out = append(out, text)
	if limit > 0 && len(out) > limit {
		observe.GlobalTrace("if: limit > 0 && len(out) > limit")
		out = out[len(out)-limit:]
	}
	observe.GlobalTrace("return: out")
	return out
}

// extractSummary returns the first accepted prompt text, truncated to 100 chars.
func extractSummary(snap app.AppState) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, prompt := range snap.PromptHistory {
		observe.GlobalTrace("range snap.PromptHistory")
		if summary := summarizePromptText(prompt); summary != "" {
			observe.GlobalTrace("return: summary")
			return summary
		}
	}
	for _, msg := range snap.Conversation.Messages {
		observe.GlobalTrace("range msgs")
		if !summarizableUserMessage(msg) {
			observe.GlobalTrace("if: !summarizableUserMessage(msg)")
			continue
		}
		for _, part := range msg.Content {
			observe.GlobalTrace("range msg.Content")
			if tp, ok := part.(model.TextPart); ok {
				observe.GlobalTrace("if: ok && tp.Text != \"\"")
				if summary := summarizePromptText(tp.Text); summary != "" {
					observe.GlobalTrace("return: summary")
					return summary
				}
			}
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func summarizableUserMessage(msg model.Message) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if msg.Role != model.RoleUser || msg.Flags.IsInternal || msg.Flags.IsMeta {
		observe.GlobalTrace("if: msg.Role != model.RoleUser || msg.Flags.IsInternal || msg.Flags.IsMeta")
		observe.GlobalTrace("return: false")
		return false
	}
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		if _, ok := part.(model.ToolResultPart); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: false")
			return false
		}
	}
	observe.GlobalTrace("return: true")
	return true
}

func summarizePromptText(text string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s := strings.TrimSpace(text)
	if s == "" || generatedPromptHistory(s) {
		observe.GlobalTrace("if: s == \"\" || generatedPromptHistory(s)")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if len(s) > 100 {
		observe.GlobalTrace("if: len(s) > 100")
		s = s[:100]
	}
	observe.GlobalTrace("return: s")
	return s
}

func printUserMessageEvent(w io.Writer, e query.UserMessageEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(e.Message) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(e.Message) == \"\"")
		return
	}
	fmt.Fprintln(w, e.Message)
	flushWriter(w)
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

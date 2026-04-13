package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/hook"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/session"
	"github.com/artpar/gogent/internal/skill"
	toolsynthetic "github.com/artpar/gogent/internal/tools/synthetic"
	"github.com/artpar/gogent/internal/slash"
	"github.com/artpar/gogent/internal/sysprompt"
	"github.com/artpar/gogent/internal/tool"
	"github.com/artpar/gogent/internal/tui"
)

// RunDispatcher routes to interactive TUI, non-interactive mode, or list-sessions.
func RunDispatcher(cmd *cobra.Command, args []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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

	// Register discovered skills as slash commands
	skillLoader := skill.NewLoader(d.Cwd)
	if skills, err := skillLoader.LoadAll(); err == nil {
		for _, s := range skills {
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
	}

	m := tui.New(tui.Config{
		ParentCtx:   cmd.Context(),
		Engine:      engine,
		Store:       d.Store,
		CostTracker: d.CostTracker,
		ModelName:   d.Cfg.Model,
		Provider:    d.Cfg.Provider,
		SessionSave: sessionSaveFn,
		SlashCmds:   slashCmds,
		SlashDeps:   slashDeps,
		HookMgr:     d.HookMgr,
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

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	// Register StructuredOutput tool if --output-schema provided (non-interactive only)
	schemaFlag, _ := cmd.Flags().GetString("output-schema")
	if schemaFlag != "" {
		schemaJSON, err := loadOutputSchema(schemaFlag)
		if err != nil {
			return fmt.Errorf("invalid output schema: %w", err)
		}
		synTool, err := toolsynthetic.New(schemaJSON)
		if err != nil {
			return fmt.Errorf("create StructuredOutput tool: %w", err)
		}
		if err := d.Registry.Register(synTool); err != nil {
			return fmt.Errorf("register StructuredOutput tool: %w", err)
		}
	}

	// Apply tool filters AFTER all tools are registered (including StructuredOutput)
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

	for ev := range events {
		observe.GlobalTrace("range events")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.GlobalTrace("typecase: query.TextEvent")
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			observe.GlobalTrace("typecase: query.ThinkingEvent")
			if d.Cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			observe.GlobalTrace("typecase: query.ToolCallEvent")
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
			fmt.Println()
		case query.ErrorEvent:
			observe.GlobalTrace("typecase: query.ErrorEvent")
			SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd)
			return e.Err
		}
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
	allowedStr, _ := cmd.Flags().GetString("allowed-tools")
	if allowedStr != "" {
		allowed := parseToolList(allowedStr)
		allowedSet := make(map[string]bool, len(allowed))
		for _, name := range allowed {
			allowedSet[name] = true
		}
		for _, desc := range registry.List() {
			if !allowedSet[desc.Name()] {
				registry.Unregister(desc.Name())
			}
		}
	}

	disallowedStr, _ := cmd.Flags().GetString("disallowed-tools")
	if disallowedStr != "" {
		disallowed := parseToolList(disallowedStr)
		for _, name := range disallowed {
			registry.Unregister(name)
		}
	}
}

// parseToolList splits a comma-separated tool list, trimming whitespace.
func parseToolList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// loadOutputSchema reads a JSON schema from a flag value — inline JSON or file path.
func loadOutputSchema(flag string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(flag)
	if strings.HasPrefix(trimmed, "{") {
		if !json.Valid([]byte(trimmed)) {
			return nil, fmt.Errorf("inline schema is not valid JSON")
		}
		return json.RawMessage(trimmed), nil
	}
	data, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("read schema file %q: %w", trimmed, err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("schema file %q does not contain valid JSON", trimmed)
	}
	return json.RawMessage(data), nil
}

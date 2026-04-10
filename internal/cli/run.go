package cli

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/session"
	"github.com/artpar/gogent/internal/slash"
	"github.com/artpar/gogent/internal/sysprompt"
	"github.com/artpar/gogent/internal/tui"
)

// RunDispatcher routes to interactive TUI, non-interactive mode, or list-sessions.
func RunDispatcher(cmd *cobra.Command, args []string) error {
	listSessions, _ := cmd.Flags().GetBool("list-sessions")
	if listSessions {
		return RunListSessions()
	}

	prompt, _ := cmd.Flags().GetString("prompt")
	if prompt != "" {
		return RunNonInteractive(cmd, args)
	}

	return RunInteractive(cmd)
}

// RunInteractive launches the bubbletea TUI for multi-turn conversation.
func RunInteractive(cmd *cobra.Command) error {
	d, err := SetupDeps(cmd)
	if err != nil {
		return err
	}
	if d.Cleanup != nil {
		defer d.Cleanup()
	}

	prompter := tui.NewInteractivePrompter()
	asker := tui.NewInteractiveAsker()
	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		return err
	}

	compDeps, compactor := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	sessionSaveFn := func() { SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd) }

	slashCmds := slash.NewRegistry()
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
	})

	program := tea.NewProgram(m, tea.WithAltScreen())
	prompter.SetProgram(program)
	asker.SetProgram(program)

	if _, err := program.Run(); err != nil {
		return err
	}

	return nil
}

// RunNonInteractive runs a single prompt and exits.
func RunNonInteractive(cmd *cobra.Command, _ []string) error {
	d, err := SetupDeps(cmd)
	if err != nil {
		return err
	}
	if d.Cleanup != nil {
		defer d.Cleanup()
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	engine, err := RegisterTools(d, prompter, asker)
	if err != nil {
		return err
	}

	compDeps, _ := BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

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
			if d.Cfg.Verbose {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
			}
		case query.ToolResultEvent:
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
			}
		case query.CompactionEvent:
			if d.Cfg.Verbose {
				fmt.Fprintf(os.Stderr, "[auto-compacted: %d → %d tokens]\n", e.PreTokens, e.PostTokens)
			}
		case query.TurnCompleteEvent:
			fmt.Println()
		case query.ErrorEvent:
			SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd)
			return e.Err
		}
	}

	SaveSession(d.Store, d.CostTracker, d.Cfg.SystemPrompt, d.Cwd)

	if d.Cfg.Verbose {
		fmt.Fprintf(os.Stderr, "total cost: $%.6f\n", d.CostTracker.TotalUSD())
	}
	return nil
}

// RunListSessions lists all saved sessions.
func RunListSessions() error {
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

// BuildCompactionDeps creates compaction dependencies from the Deps struct.
func BuildCompactionDeps(d *Deps) (query.CompactionDeps, *compact.Service) {
	secondaryModel := SecondaryModelFor(d.Cfg.Provider)
	compactor := compact.NewService(d.Prov, d.Bus, d.CostTracker, secondaryModel)

	disableAutoCompact := os.Getenv("DISABLE_AUTO_COMPACT") == "1" || os.Getenv("DISABLE_AUTO_COMPACT") == "true"
	autoTracker := compact.NewAutoTracker(disableAutoCompact)

	ctxWindow := 200_000
	if cw, ok := d.Prov.ContextWindow(d.Cfg.Model); ok {
		ctxWindow = cw
	}

	snap := d.Store.Snapshot()
	sysTokEst := compact.EstimateSystemPromptTokens(snap.Conversation.System)

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

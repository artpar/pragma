package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/orchestration"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/tui"
)

func orchestrationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestration",
		Short: "Run orchestration state machines",
	}
	cmd.AddCommand(orchestrationRunCmd())
	return cmd
}

func orchestrationRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <orchestration.yaml>",
		Short: "Execute an orchestration graph",
		Args:  cobra.ExactArgs(1),
		RunE:  runOrchestration,
	}
	cmd.Flags().String("prompt", "", "Task prompt")
	cmd.Flags().String("persona-dir", "personas", "Directory containing persona YAML files")
	return cmd
}

func runOrchestration(cmd *cobra.Command, args []string) error {
	taskPrompt, _ := cmd.Flags().GetString("prompt")
	if taskPrompt == "" {
		return fmt.Errorf("--prompt is required")
	}

	def, err := orchestration.LoadDefinitionFile(args[0])
	if err != nil {
		return err
	}
	runtime, err := orchestration.NewRuntime(def)
	if err != nil {
		return err
	}

	d, err := cli.SetupDeps(cmd)
	if err != nil {
		return err
	}
	if d.Cleanup != nil {
		defer d.Cleanup()
	}
	d.Bus.Subscribe(d.StderrLogger)
	if !cmd.Flags().Changed("permission-mode") {
		d.Checker = permission.NewRuleChecker(nil, permission.ModeBypassPermissions, d.Cwd, d.Bus)
	}

	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	engine, err := cli.RegisterTools(d, prompter, asker)
	if err != nil {
		return err
	}
	compDeps, _ := cli.BuildCompactionDeps(d)
	engine.SetCompaction(compDeps)

	personaDir, _ := cmd.Flags().GetString("persona-dir")
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		event, err := runOrchestrationNode(cmd.Context(), engine, d, compDeps, personaDir, state, taskPrompt)
		if err != nil {
			return err
		}
		if err := runtime.FSM.Event(cmd.Context(), event); err != nil {
			return fmt.Errorf("transition %q from %q: %w", event, stateID, err)
		}
	}

	fmt.Fprintf(os.Stderr, "orchestration: done\n")
	return nil
}

func runOrchestrationNode(ctx context.Context, rootEngine *query.Engine, d *cli.Deps, compDeps query.CompactionDeps, personaDir string, state orchestration.State, taskPrompt string) (string, error) {
	if !state.Control.IsZero() {
		fmt.Fprintf(os.Stderr, "orchestration: state=%s control=%s\n", state.ID, controlName(state))
		event, err := orchestration.ExecuteControl(state)
		if err != nil {
			return "", fmt.Errorf("control state %q failed: %w", state.ID, err)
		}
		fmt.Fprintf(os.Stderr, "[control %s emitted %s]\n", state.ID, event)
		return event, nil
	}

	personaDef, err := loadPersonaForState(personaDir, state)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "orchestration: state=%s persona=%s\n", state.ID, personaDef.ID)

	engine := newIsolatedOrchestrationEngine(rootEngine, d, compDeps)
	if _, err := runOrchestrationState(ctx, engine, state, personaDef, taskPrompt); err != nil {
		return "", fmt.Errorf("state %q failed: %w", state.ID, err)
	}

	event, err := selectStateEvent(state)
	if err != nil {
		return "", fmt.Errorf("select event for state %q: %w", state.ID, err)
	}
	return event, nil
}

func newIsolatedOrchestrationEngine(rootEngine *query.Engine, d *cli.Deps, compDeps query.CompactionDeps) *query.Engine {
	cfg := d.EngineCfg
	cfg.TaskID = ""
	cfg.ContentReplacementRecords = nil
	cfg.RecordContentReplacements = nil

	engine := query.NewEngine(
		d.Prov,
		rootEngine.Registry(),
		rootEngine.Orchestrator(),
		newIsolatedOrchestrationStore(d),
		d.CostTracker,
		d.Bus,
		cfg,
	)
	if d.HookMgr != nil {
		engine.SetHookManager(d.HookMgr)
	}
	engine.SetCompaction(compDeps)
	return engine
}

func newIsolatedOrchestrationStore(d *cli.Deps) *app.StateStore {
	snap := d.Store.Snapshot()
	conv := model.NewConversation(snap.Conversation.System, d.Cfg.Model, d.Cfg.Provider, d.Cwd)
	return app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          d.Cwd,
		Model:        d.Cfg.Model,
		Provider:     d.Cfg.Provider,
		MaxTokens:    d.Cfg.MaxTokens,
		Temperature:  d.Cfg.Temperature,
	})
}

func controlName(state orchestration.State) string {
	switch {
	case state.Control.ForEachNext != nil:
		return "foreach_next"
	case state.Control.MarkCurrentItem != nil:
		return "mark_current_item"
	default:
		return "unknown"
	}
}

func loadPersonaForState(personaDir string, state orchestration.State) (persona.Definition, error) {
	personaID := state.Persona
	if personaID == "" {
		personaID = state.ID
	}
	return persona.LoadDefinitionFile(filepath.Join(personaDir, personaID+".yaml"))
}

func selectStateEvent(state orchestration.State) (string, error) {
	if state.Event.FromFile == nil {
		if state.Event.Default != "" {
			return state.Event.Default, nil
		}
		return orchestration.EventComplete, nil
	}

	raw, err := os.ReadFile(state.Event.FromFile.Path)
	if err != nil {
		return "", err
	}
	content := string(raw)
	if event, ok := selectDecisionEvent(content, state.Event.FromFile.Rules); ok {
		return event, nil
	}
	for _, rule := range state.Event.FromFile.Rules {
		if strings.Contains(content, rule.Contains) {
			return rule.Event, nil
		}
	}
	if state.Event.Default != "" {
		return state.Event.Default, nil
	}
	return "", fmt.Errorf("no file event rule matched %q", state.Event.FromFile.Path)
}

func selectDecisionEvent(content string, rules []orchestration.TextEvent) (string, bool) {
	decisionEvents := make(map[string]string)
	for _, rule := range rules {
		decision, ok := decisionRuleValue(rule.Contains)
		if !ok {
			return "", false
		}
		decisionEvents[decision] = rule.Event
	}
	decision, ok := lastDecisionValue(content)
	if !ok {
		return "", false
	}
	event, ok := decisionEvents[decision]
	return event, ok
}

func decisionRuleValue(pattern string) (string, bool) {
	pattern = strings.TrimSpace(pattern)
	if strings.HasPrefix(pattern, "Decision:\n") {
		return strings.TrimSpace(strings.TrimPrefix(pattern, "Decision:\n")), true
	}
	if strings.HasPrefix(pattern, "Decision:") {
		return strings.TrimSpace(strings.TrimPrefix(pattern, "Decision:")), true
	}
	return "", false
}

func lastDecisionValue(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	var decision string
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case line == "Decision:":
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" {
					continue
				}
				decision = next
				break
			}
		case strings.HasPrefix(line, "Decision:"):
			decision = strings.TrimSpace(strings.TrimPrefix(line, "Decision:"))
		}
	}
	return decision, decision != ""
}

func runOrchestrationState(ctx context.Context, engine *query.Engine, state orchestration.State, personaDef persona.Definition, taskPrompt string) (string, error) {
	prompt := buildOrchestrationPrompt(state, personaDef, taskPrompt)

	var text strings.Builder
	start := time.Now()
	for ev := range engine.Run(ctx, prompt) {
		switch e := ev.(type) {
		case query.TextEvent:
			text.WriteString(e.Text)
			fmt.Print(e.Text)
		case query.ThinkingEvent:
			if e.Text != "" {
				fmt.Fprint(os.Stderr, e.Text)
			}
		case query.ToolCallEvent:
			fmt.Fprintf(os.Stderr, "[tool: %s]\n", e.Call.Name)
		case query.ToolResultEvent:
			fmt.Fprintf(os.Stderr, "[result: %s]\n", e.Result.ToolCallID)
		case query.RetryEvent:
			fmt.Fprintf(os.Stderr, "[retry: %s in %s]\n", e.Kind, e.Delay)
		case query.TurnCompleteEvent:
			fmt.Fprintf(os.Stderr, "\n[state %s complete in %s]\n", state.ID, time.Since(start).Round(time.Second))
		case query.ErrorEvent:
			return text.String(), e.Err
		}
	}
	return text.String(), nil
}

func buildOrchestrationPrompt(state orchestration.State, personaDef persona.Definition, taskPrompt string) string {
	if state.TaskPrompt == orchestration.TaskPromptNone {
		return strings.TrimRight(personaDef.Prompt, "\n") + "\n"
	}
	return fmt.Sprintf(`%s

## Task

%s
`, personaDef.Prompt, taskPrompt)
}

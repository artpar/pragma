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

	if err := ensureDefaultOrchestrationDirs(def); err != nil {
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
	handoffPrompt := ""
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		event, err := runOrchestrationNode(cmd.Context(), engine, d, compDeps, personaDir, def, state, taskPrompt, handoffPrompt)
		if err != nil {
			return err
		}
		nextHandoff, err := selectedHandoffPrompt(state.ID, event)
		if err != nil {
			return err
		}
		if nextHandoff != "" {
			handoffPrompt = nextHandoff
		} else if state.Control.IsZero() {
			handoffPrompt = ""
		}
		if err := runtime.FSM.Event(cmd.Context(), event); err != nil {
			return fmt.Errorf("transition %q from %q: %w", event, stateID, err)
		}
	}

	fmt.Fprintf(os.Stderr, "orchestration: done\n")
	return nil
}

func ensureDefaultOrchestrationDirs(def orchestration.Definition) error {
	if err := os.RemoveAll("/tmp/pragma/handoff-prompts"); err != nil {
		return fmt.Errorf("reset orchestration handoff directory: %w", err)
	}
	for _, dir := range []string{
		"/tmp/pragma",
		"/tmp/pragma/handoff-prompts",
		"/tmp/pragma/processes",
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create orchestration directory %q: %w", dir, err)
		}
	}
	for _, state := range def.States {
		dir := filepath.Join("/tmp/pragma/handoff-prompts", safeHandoffPathSegment(state.ID))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create orchestration handoff directory %q: %w", dir, err)
		}
	}
	return nil
}

func runOrchestrationNode(ctx context.Context, rootEngine *query.Engine, d *cli.Deps, compDeps query.CompactionDeps, personaDir string, def orchestration.Definition, state orchestration.State, taskPrompt string, handoffPrompt string) (string, error) {
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
	if _, err := runOrchestrationState(ctx, engine, def, state, personaDef, taskPrompt, handoffPrompt); err != nil {
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

func runOrchestrationState(ctx context.Context, engine *query.Engine, def orchestration.Definition, state orchestration.State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (string, error) {
	system, prompt := buildOrchestrationPrompt(def, state, personaDef, taskPrompt, handoffPrompt)

	var text strings.Builder
	start := time.Now()
	for ev := range engine.RunPragmaLoopWithSystem(ctx, system, prompt) {
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

func buildOrchestrationPrompt(def orchestration.Definition, state orchestration.State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (model.SystemPrompt, string) {
	system := model.SystemPrompt{Blocks: []model.SystemBlock{{
		Text:      strings.TrimRight(personaDef.Prompt, "\n") + "\n\n" + query.PragmaLoopSystemPrompt(),
		Cacheable: false,
	}}}

	var b strings.Builder
	if state.ID == def.Initial && state.TaskPrompt != orchestration.TaskPromptNone {
		fmt.Fprintf(&b, "## Task\n\n%s\n", taskPrompt)
	} else {
		if strings.TrimSpace(handoffPrompt) != "" {
			fmt.Fprintf(&b, "## Handoff From Previous Phase\n\n%s\n\n", strings.TrimSpace(handoffPrompt))
		}
		b.WriteString("## Phase Input\n\nProceed with this phase using the required input artifacts.\n")
	}

	if handoffs := renderNextHandoffInstructions(def, state); handoffs != "" {
		fmt.Fprintf(&b, "\n%s", handoffs)
	}

	return system, b.String()
}

func renderNextHandoffInstructions(def orchestration.Definition, state orchestration.State) string {
	transitions := outgoingTransitions(def, state.ID)
	if len(transitions) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Possible Next Phase Handoffs\n\n")
	b.WriteString("When this phase is ready to finish, write the handoff prompt for the event this phase is causing before the final completion echo. Use the exact path below. The handoff should tell the next phase what this phase established, which artifacts to read, and what must be preserved.\n\n")
	for _, tr := range transitions {
		target := tr.To
		if state, ok := stateByID(def, tr.To); ok {
			switch {
			case !state.Control.IsZero():
				target += " (control)"
			case state.Persona != "":
				target += " (persona: " + state.Persona + ")"
			case !state.Terminal:
				target += " (persona: " + state.ID + ")"
			}
		}
		fmt.Fprintf(&b, "- event %q -> %s: %s\n", tr.Event, target, handoffPromptPath(state.ID, tr.Event))
	}
	return b.String()
}

func outgoingTransitions(def orchestration.Definition, stateID string) []orchestration.Transition {
	var out []orchestration.Transition
	for _, tr := range def.Transitions {
		for _, from := range tr.From {
			if from == stateID {
				out = append(out, orchestration.Transition{
					Event: tr.Event,
					From:  []string{stateID},
					To:    tr.To,
				})
				break
			}
		}
	}
	return out
}

func stateByID(def orchestration.Definition, stateID string) (orchestration.State, bool) {
	for _, state := range def.States {
		if state.ID == stateID {
			return state, true
		}
	}
	return orchestration.State{}, false
}

func selectedHandoffPrompt(stateID string, event string) (string, error) {
	raw, err := os.ReadFile(handoffPromptPath(stateID, event))
	if err == nil {
		return strings.TrimSpace(string(raw)), nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", fmt.Errorf("read handoff prompt for state %q event %q: %w", stateID, event, err)
}

func handoffPromptPath(stateID string, event string) string {
	return filepath.Join("/tmp/pragma/handoff-prompts", safeHandoffPathSegment(stateID), safeHandoffPathSegment(event)+".md")
}

func safeHandoffPathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

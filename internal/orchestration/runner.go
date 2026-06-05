package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/query"
)

// RunFileEvents loads an orchestration definition and streams one complete FSM run.
func RunFileEvents(ctx context.Context, engine *query.Engine, path string, personaDir string, taskPrompt string) <-chan query.LoopEvent {
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		def, err := LoadDefinitionFile(path)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		runEvents(ctx, ch, engine, def, personaDir, taskPrompt)
	}()
	return ch
}

// RunEvents streams one complete FSM run for an already-loaded definition.
func RunEvents(ctx context.Context, engine *query.Engine, def Definition, personaDir string, taskPrompt string) <-chan query.LoopEvent {
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		runEvents(ctx, ch, engine, def, personaDir, taskPrompt)
	}()
	return ch
}

func runEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, def Definition, personaDir string, taskPrompt string) {
	runtime, err := NewRuntime(def)
	if err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}
	if err := EnsureRunDirs(def); err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}

	handoffPrompt := ""
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		event, err := RunNodeEvents(ctx, ch, engine, personaDir, def, state, taskPrompt, handoffPrompt)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		nextHandoff, err := selectedHandoffPrompt(state.ID, event)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		if nextHandoff != "" {
			handoffPrompt = nextHandoff
		} else if state.Control.IsZero() {
			handoffPrompt = ""
		}
		if err := runtime.FSM.Event(ctx, event); err != nil {
			ch <- query.ErrorEvent{Err: fmt.Errorf("transition %q from %q: %w", event, stateID, err)}
			return
		}
		ch <- query.TextEvent{Text: fmt.Sprintf("\n[transition: %s --%s--> %s]\n", stateID, event, runtime.FSM.Current())}
	}

	ch <- query.TextEvent{Text: "\n[orchestration: done]\n"}
	ch <- query.TurnCompleteEvent{
		Response:   model.Response{StopReason: model.StopEndTurn},
		StopReason: model.StopEndTurn,
	}
}

func EnsureRunDirs(def Definition) error {
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

func RunNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string) (string, error) {
	if !state.Control.IsZero() {
		control := ControlName(state)
		ch <- query.TextEvent{Text: fmt.Sprintf("\n[control: %s (%s)]\n", state.ID, control)}
		event, err := ExecuteControl(state)
		if err != nil {
			return "", fmt.Errorf("control state %q failed: %w", state.ID, err)
		}
		ch <- query.TextEvent{Text: fmt.Sprintf("[control: %s emitted %s]\n", state.ID, event)}
		return event, nil
	}

	personaDef, err := LoadPersonaForState(personaDir, state)
	if err != nil {
		return "", err
	}
	ch <- query.TextEvent{Text: fmt.Sprintf("\n[orchestration: %s persona=%s]\n", state.ID, personaDef.ID)}

	if _, err := RunStateEvents(ctx, ch, engine, def, state, personaDef, taskPrompt, handoffPrompt); err != nil {
		return "", fmt.Errorf("state %q failed: %w", state.ID, err)
	}

	event, err := SelectStateEvent(state)
	if err != nil {
		return "", fmt.Errorf("select event for state %q: %w", state.ID, err)
	}
	return event, nil
}

func ControlName(state State) string {
	switch {
	case state.Control.ForEachNext != nil:
		return "foreach_next"
	case state.Control.MarkCurrentItem != nil:
		return "mark_current_item"
	default:
		return "unknown"
	}
}

func LoadPersonaForState(personaDir string, state State) (persona.Definition, error) {
	personaID := state.Persona
	if personaID == "" {
		personaID = state.ID
	}
	return persona.LoadDefinitionFile(filepath.Join(personaDir, personaID+".yaml"))
}

func SelectStateEvent(state State) (string, error) {
	if state.Event.FromFile == nil {
		if state.Event.Default != "" {
			return state.Event.Default, nil
		}
		return EventComplete, nil
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

func selectDecisionEvent(content string, rules []TextEvent) (string, bool) {
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

func RunStateEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (string, error) {
	system, prompt := BuildPrompt(def, state, personaDef, taskPrompt, handoffPrompt)

	var text strings.Builder
	start := time.Now()
	for ev := range engine.RunPragmaLoopWithSystem(ctx, system, prompt) {
		switch e := ev.(type) {
		case query.TextEvent:
			text.WriteString(e.Text)
			ch <- e
		case query.ThinkingEvent:
			ch <- e
		case query.ToolCallEvent:
			ch <- e
		case query.ToolResultEvent:
			ch <- e
		case query.RetryEvent:
			ch <- e
		case query.ErrorEvent:
			return text.String(), e.Err
		case query.TurnCompleteEvent:
			ch <- query.TextEvent{Text: fmt.Sprintf("\n[state %s complete in %s]\n", state.ID, time.Since(start).Round(time.Second))}
		default:
			ch <- ev
		}
	}
	return text.String(), nil
}

func BuildPrompt(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (model.SystemPrompt, string) {
	system := model.SystemPrompt{Blocks: []model.SystemBlock{{
		Text:      strings.TrimRight(personaDef.Prompt, "\n") + "\n\n" + query.PragmaLoopSystemPrompt(),
		Cacheable: false,
	}}}

	var b strings.Builder
	if state.ID == def.Initial && state.TaskPrompt != TaskPromptNone {
		fmt.Fprintf(&b, "## Task\n\n%s\n", taskPrompt)
	} else {
		if strings.TrimSpace(handoffPrompt) != "" {
			fmt.Fprintf(&b, "## Handoff From Previous Phase\n\n%s\n\n", strings.TrimSpace(handoffPrompt))
		}
		b.WriteString("## Phase Input\n\nProceed with this phase using the required input artifacts.\n")
	}

	if handoffs := RenderNextHandoffInstructions(def, state); handoffs != "" {
		fmt.Fprintf(&b, "\n%s", handoffs)
	}

	return system, b.String()
}

func RenderNextHandoffInstructions(def Definition, state State) string {
	transitions := outgoingTransitions(def, state.ID)
	if len(transitions) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Possible Next Phase Handoffs\n\n")
	b.WriteString("When this phase is ready to finish, write the handoff prompt for the event this phase is causing before the final completion echo. Use the exact path below. The handoff should tell the next phase what this phase established, which artifacts to read, and what must be preserved.\n\n")
	for _, tr := range transitions {
		fmt.Fprintf(&b, "- event %q -> %s: %s\n", tr.Event, handoffTargetLabel(def, tr.To), handoffPromptPath(state.ID, tr.Event))
	}
	return b.String()
}

func handoffTargetLabel(def Definition, stateID string) string {
	state, ok := stateByID(def, stateID)
	if !ok {
		return stateID
	}
	label := stateLabel(state)
	if state.Control.IsZero() {
		return label
	}
	next := outgoingTransitions(def, state.ID)
	if len(next) == 0 {
		return label
	}
	targets := make([]string, 0, len(next))
	for _, tr := range next {
		targets = append(targets, fmt.Sprintf("%q -> %s", tr.Event, handoffTargetLabel(def, tr.To)))
	}
	return label + " -> " + strings.Join(targets, "; ")
}

func stateLabel(state State) string {
	switch {
	case !state.Control.IsZero():
		return state.ID + " (control)"
	case state.Persona != "":
		return state.ID + " (persona: " + state.Persona + ")"
	case state.Terminal:
		return state.ID + " (terminal)"
	default:
		return state.ID + " (persona: " + state.ID + ")"
	}
}

func outgoingTransitions(def Definition, stateID string) []Transition {
	var out []Transition
	for _, tr := range def.Transitions {
		for _, from := range tr.From {
			if from == stateID {
				out = append(out, Transition{
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

func stateByID(def Definition, stateID string) (State, bool) {
	for _, state := range def.States {
		if state.ID == stateID {
			return state, true
		}
	}
	return State{}, false
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

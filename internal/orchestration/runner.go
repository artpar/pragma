package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/query"
)

const DefaultArtifactRoot = "/tmp/pragma"

type RunOptions struct {
	PersonaDir   string
	TaskPrompt   string
	ArtifactRoot string
}

func (o RunOptions) artifactRoot() string {
	if strings.TrimSpace(o.ArtifactRoot) == "" {
		return DefaultArtifactRoot
	}
	return o.ArtifactRoot
}

// RunFileEvents loads an orchestration definition and streams one complete FSM run.
func RunFileEvents(ctx context.Context, engine *query.Engine, path string, personaDir string, taskPrompt string) <-chan query.LoopEvent {
	return RunFileEventsWithOptions(ctx, engine, path, RunOptions{
		PersonaDir: personaDir,
		TaskPrompt: taskPrompt,
	})
}

func RunFileEventsWithOptions(ctx context.Context, engine *query.Engine, path string, opts RunOptions) <-chan query.LoopEvent {
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		def, err := LoadDefinitionFile(path)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		runEvents(ctx, ch, engine, def, opts)
	}()
	return ch
}

// RunEvents streams one complete FSM run for an already-loaded definition.
func RunEvents(ctx context.Context, engine *query.Engine, def Definition, personaDir string, taskPrompt string) <-chan query.LoopEvent {
	return RunEventsWithOptions(ctx, engine, def, RunOptions{
		PersonaDir: personaDir,
		TaskPrompt: taskPrompt,
	})
}

func RunEventsWithOptions(ctx context.Context, engine *query.Engine, def Definition, opts RunOptions) <-chan query.LoopEvent {
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		runEvents(ctx, ch, engine, def, opts)
	}()
	return ch
}

func runEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, def Definition, opts RunOptions) {
	runtime, err := NewRuntime(def)
	if err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}
	artifactRoot := opts.artifactRoot()
	bus := eventBus(engine)
	emitQueryObserve(ch, bus, query.OrchestrationStartedEvent{Name: def.Name, Initial: def.Initial})
	if err := EnsureRunDirs(def, artifactRoot); err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}

	handoffPrompt := ""
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		event, err := RunNodeEvents(ctx, ch, engine, opts.PersonaDir, def, state, opts.TaskPrompt, handoffPrompt, artifactRoot)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		nextHandoff, err := selectedHandoffPrompt(artifactRoot, state.ID, event)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		if nextHandoff != "" {
			emitQueryObserve(ch, bus, query.OrchestrationHandoffEvent{
				StateID: state.ID, Event: event, Path: handoffPromptPath(artifactRoot, state.ID, event), Direction: "read",
			})
			handoffPrompt = nextHandoff
		} else if state.Control.IsZero() {
			handoffPrompt = ""
		}
		if err := runtime.FSM.Event(ctx, event); err != nil {
			ch <- query.ErrorEvent{Err: fmt.Errorf("transition %q from %q: %w", event, stateID, err)}
			return
		}
		emitQueryObserve(ch, bus, query.OrchestrationTransitionEvent{From: stateID, Event: event, To: runtime.FSM.Current()})
	}

	emitQueryObserve(ch, bus, query.OrchestrationCompletedEvent{Name: def.Name})
	ch <- query.TurnCompleteEvent{
		Response:   model.Response{StopReason: model.StopEndTurn},
		StopReason: model.StopEndTurn,
	}
}

func EnsureRunDirs(def Definition, artifactRoots ...string) error {
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		artifactRoot = artifactRoots[0]
	}
	if strings.TrimSpace(artifactRoot) == "" {
		artifactRoot = DefaultArtifactRoot
	}
	handoffRoot := filepath.Join(artifactRoot, "handoff-prompts")
	if err := os.RemoveAll(handoffRoot); err != nil {
		return fmt.Errorf("reset orchestration handoff directory: %w", err)
	}
	for _, dir := range []string{
		artifactRoot,
		handoffRoot,
		filepath.Join(artifactRoot, "processes"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create orchestration directory %q: %w", dir, err)
		}
	}
	for _, state := range def.States {
		dir := filepath.Join(handoffRoot, safeHandoffPathSegment(state.ID))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create orchestration handoff directory %q: %w", dir, err)
		}
	}
	return nil
}

func RunNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, error) {
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		artifactRoot = artifactRoots[0]
	}
	bus := eventBus(engine)
	if !state.Control.IsZero() {
		control := ControlName(state)
		emitQueryObserve(ch, bus, query.OrchestrationStateStartedEvent{StateID: state.ID, Control: control})
		emitQueryObserve(ch, bus, query.OrchestrationControlEvent{StateID: state.ID, Control: control})
		event, err := ExecuteControl(state)
		if err != nil {
			return "", fmt.Errorf("control state %q failed: %w", state.ID, err)
		}
		emitQueryObserve(ch, bus, query.OrchestrationControlEvent{StateID: state.ID, Control: control, Event: event})
		return event, nil
	}

	personaDef, err := LoadPersonaForState(personaDir, state)
	if err != nil {
		return "", err
	}
	emitQueryObserve(ch, bus, query.OrchestrationStateStartedEvent{StateID: state.ID, PersonaID: personaDef.ID})
	for _, tr := range outgoingTransitions(def, state.ID) {
		emitQueryObserve(ch, bus, query.OrchestrationHandoffEvent{
			StateID: state.ID, Event: tr.Event, Path: handoffPromptPath(artifactRoot, state.ID, tr.Event), Direction: "write_target",
		})
	}

	if _, err := RunStateEvents(ctx, ch, engine, def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot); err != nil {
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

func RunStateEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, error) {
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		artifactRoot = artifactRoots[0]
	}
	system, prompt := BuildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)

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
			emitQueryObserve(ch, eventBus(engine), query.OrchestrationStateCompletedEvent{StateID: state.ID, Duration: time.Since(start)})
		default:
			ch <- ev
		}
	}
	return text.String(), nil
}

func BuildPrompt(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (model.SystemPrompt, string) {
	return BuildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, DefaultArtifactRoot)
}

func BuildPromptWithArtifactRoot(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoot string) (model.SystemPrompt, string) {
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

	if handoffs := RenderNextHandoffInstructionsWithArtifactRoot(def, state, artifactRoot); handoffs != "" {
		fmt.Fprintf(&b, "\n%s", handoffs)
	}

	return system, b.String()
}

func RenderNextHandoffInstructions(def Definition, state State) string {
	return RenderNextHandoffInstructionsWithArtifactRoot(def, state, DefaultArtifactRoot)
}

func RenderNextHandoffInstructionsWithArtifactRoot(def Definition, state State, artifactRoot string) string {
	transitions := outgoingTransitions(def, state.ID)
	if len(transitions) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Possible Next Phase Handoffs\n\n")
	b.WriteString("When this phase is ready to finish, write the handoff prompt for the event this phase is causing before the final completion echo. Use the exact path below. The handoff should tell the next phase what this phase established, which artifacts to read, and what must be preserved.\n\n")
	for _, tr := range transitions {
		fmt.Fprintf(&b, "- event %q -> %s: %s\n", tr.Event, handoffTargetLabel(def, tr.To), handoffPromptPath(artifactRoot, state.ID, tr.Event))
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

func selectedHandoffPrompt(artifactRoot string, stateID string, event string) (string, error) {
	raw, err := os.ReadFile(handoffPromptPath(artifactRoot, stateID, event))
	if err == nil {
		return strings.TrimSpace(string(raw)), nil
	}
	if os.IsNotExist(err) {
		return "", nil
	}
	return "", fmt.Errorf("read handoff prompt for state %q event %q: %w", stateID, event, err)
}

func handoffPromptPath(artifactRoot string, stateID string, event string) string {
	if strings.TrimSpace(artifactRoot) == "" {
		artifactRoot = DefaultArtifactRoot
	}
	return filepath.Join(artifactRoot, "handoff-prompts", safeHandoffPathSegment(stateID), safeHandoffPathSegment(event)+".md")
}

func eventBus(engine *query.Engine) *observe.EventBus {
	if engine == nil {
		return nil
	}
	return engine.EventBus()
}

func emitQueryObserve(ch chan<- query.LoopEvent, bus *observe.EventBus, ev query.LoopEvent) {
	ch <- ev
	if bus == nil {
		return
	}
	switch e := ev.(type) {
	case query.OrchestrationStartedEvent:
		bus.Emit(observe.OrchestrationStarted{
			EventHeader: observe.NewEventHeader("OrchestrationStarted", "", "", ""),
			Name:        e.Name,
			Initial:     e.Initial,
		})
	case query.OrchestrationStateStartedEvent:
		bus.Emit(observe.OrchestrationStateStarted{
			EventHeader: observe.NewEventHeader("OrchestrationStateStarted", "", "", ""),
			StateID:     e.StateID,
			PersonaID:   e.PersonaID,
			Control:     e.Control,
		})
	case query.OrchestrationStateCompletedEvent:
		bus.Emit(observe.OrchestrationStateCompleted{
			EventHeader: observe.NewEventHeader("OrchestrationStateCompleted", "", "", ""),
			StateID:     e.StateID,
			Duration:    e.Duration,
			DurationMs:  e.Duration.Milliseconds(),
		})
	case query.OrchestrationControlEvent:
		bus.Emit(observe.OrchestrationControl{
			EventHeader: observe.NewEventHeader("OrchestrationControl", "", "", ""),
			StateID:     e.StateID,
			Control:     e.Control,
			Event:       e.Event,
		})
	case query.OrchestrationTransitionEvent:
		bus.Emit(observe.OrchestrationTransition{
			EventHeader: observe.NewEventHeader("OrchestrationTransition", "", "", ""),
			From:        e.From,
			Event:       e.Event,
			To:          e.To,
		})
	case query.OrchestrationHandoffEvent:
		bus.Emit(observe.OrchestrationHandoff{
			EventHeader: observe.NewEventHeader("OrchestrationHandoff", "", "", ""),
			StateID:     e.StateID,
			Event:       e.Event,
			Path:        e.Path,
			Direction:   e.Direction,
		})
	case query.OrchestrationCompletedEvent:
		bus.Emit(observe.OrchestrationCompleted{
			EventHeader: observe.NewEventHeader("OrchestrationCompleted", "", "", ""),
			Name:        e.Name,
		})
	}
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

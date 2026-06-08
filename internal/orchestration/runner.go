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
	projection := NewProjection()
	emitOrchestration(ch, bus, projection, query.OrchestrationStartedEvent{Name: def.Name, Initial: def.Initial})
	if err := EnsureRunDirs(def, artifactRoot); err != nil {
		ch <- query.ErrorEvent{Err: err}
		return
	}

	handoffPrompt := ""
	taskPrompt := opts.TaskPrompt
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		stateTaskPrompt := ""
		if state.Control.IsZero() {
			stateTaskPrompt = taskPrompt
			taskPrompt = ""
		}
		event, nextHandoff, err := RunNodeEvents(ctx, ch, engine, projection, opts.PersonaDir, def, state, stateTaskPrompt, handoffPrompt, artifactRoot)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
		}
		if nextHandoff != "" {
			emitOrchestration(ch, bus, projection, query.OrchestrationHandoffEvent{
				StateID: state.ID, Event: event, Direction: "runtime",
			})
			handoffPrompt = nextHandoff
		} else if state.Control.IsZero() {
			handoffPrompt = ""
		}
		if err := runtime.FSM.Event(ctx, event); err != nil {
			ch <- query.ErrorEvent{Err: fmt.Errorf("transition %q from %q: %w", event, stateID, err)}
			return
		}
		emitOrchestration(ch, bus, projection, query.OrchestrationTransitionEvent{From: stateID, Event: event, To: runtime.FSM.Current()})
	}

	emitOrchestration(ch, bus, projection, query.OrchestrationCompletedEvent{Name: def.Name})
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
	for _, dir := range []string{
		artifactRoot,
		filepath.Join(artifactRoot, "processes"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create orchestration directory %q: %w", dir, err)
		}
	}
	return nil
}

func RunNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, string, error) {
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		artifactRoot = artifactRoots[0]
	}
	bus := eventBus(engine)
	if !state.Control.IsZero() {
		control := ControlName(state)
		emitOrchestration(ch, bus, projection, query.OrchestrationStateStartedEvent{StateID: state.ID, Control: control})
		emitOrchestration(ch, bus, projection, query.OrchestrationControlEvent{StateID: state.ID, Control: control})
		event, err := ExecuteControl(state)
		if err != nil {
			return "", "", fmt.Errorf("control state %q failed: %w", state.ID, err)
		}
		emitOrchestration(ch, bus, projection, query.OrchestrationControlEvent{StateID: state.ID, Control: control, Event: event})
		return event, "", nil
	}

	personaDef, err := LoadPersonaForState(personaDir, state)
	if err != nil {
		return "", "", err
	}
	emitOrchestration(ch, bus, projection, query.OrchestrationStateStartedEvent{StateID: state.ID, PersonaID: personaDef.ID})

	output, err := RunStateEvents(ctx, ch, engine, projection, def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)
	if err != nil {
		return "", "", fmt.Errorf("state %q failed: %w", state.ID, err)
	}

	event, err := SelectStateEvent(state)
	if err != nil {
		return "", "", fmt.Errorf("select event for state %q: %w", state.ID, err)
	}
	return event, strings.TrimSpace(output), nil
}

func ControlName(state State) string {
	switch {
	case state.Control.ForEachNext != nil:
		return "foreach_next"
	case state.Control.MarkCurrentItem != nil:
		return "mark_current_item"
	case state.Control.ArtifactVerdict != nil:
		return "artifact_verdict"
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
	if state.Event.Default != "" {
		return state.Event.Default, nil
	}
	return EventComplete, nil
}

func RunStateEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, error) {
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
			emitOrchestration(ch, eventBus(engine), projection, query.OrchestrationStateCompletedEvent{StateID: state.ID, Duration: time.Since(start)})
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
	if strings.TrimSpace(taskPrompt) != "" && state.TaskPrompt != TaskPromptNone {
		fmt.Fprintf(&b, "## Task\n\n%s\n", taskPrompt)
	} else {
		if strings.TrimSpace(handoffPrompt) != "" {
			fmt.Fprintf(&b, "## Handoff From Previous Phase\n\n%s\n\n", strings.TrimSpace(handoffPrompt))
		}
		b.WriteString("## Phase Input\n\nProceed with this phase using the required input artifacts.\n")
	}
	if contract := RenderArtifactContract(state.Artifacts); contract != "" {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n")
		}
		b.WriteString(contract)
	}

	return system, b.String()
}

func RenderArtifactContract(artifacts Artifacts) string {
	if artifacts.IsZero() {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Runtime Artifact Contract\n\n")
	b.WriteString("These file paths are supplied by the orchestration YAML at runtime. Follow them exactly.\n\n")
	b.WriteString("When persona instructions mention `<artifact_id path>`, substitute the matching path from this section.\n\n")
	if len(artifacts.Inputs) > 0 {
		b.WriteString("Inputs:\n")
		for _, artifact := range artifacts.Inputs {
			writeArtifactLine(&b, artifact)
		}
		b.WriteString("\n")
	}
	if len(artifacts.Outputs) > 0 {
		b.WriteString("Outputs:\n")
		for _, artifact := range artifacts.Outputs {
			writeArtifactLine(&b, artifact)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func writeArtifactLine(b *strings.Builder, artifact Artifact) {
	required := "optional"
	if artifact.Required {
		required = "required"
	}
	fmt.Fprintf(b, "- `%s` (%s): `%s`", artifact.ID, required, artifact.Path)
	if artifact.Description != "" {
		fmt.Fprintf(b, " - %s", artifact.Description)
	}
	if artifact.Kind != "" {
		fmt.Fprintf(b, " Kind: %s.", artifact.Kind)
	}
	if len(artifact.AllowedValues) > 0 {
		fmt.Fprintf(b, " Allowed values: %s.", strings.Join(artifact.AllowedValues, ", "))
	}
	b.WriteString("\n")
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

func emitOrchestration(ch chan<- query.LoopEvent, bus *observe.EventBus, projection *Projection, ev query.LoopEvent) {
	emitQueryObserve(ch, bus, ev)
	if snapshot, ok := projection.Apply(ev, time.Now()); ok {
		ch <- query.OrchestrationSnapshotEvent{Snapshot: snapshot}
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

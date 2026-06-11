package orchestration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(o.ArtifactRoot) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(o.ArtifactRoot) == \"\"")
		observe.GlobalTrace("return: DefaultArtifactRoot")
		return DefaultArtifactRoot
	}
	observe.GlobalTrace("return: o.ArtifactRoot")
	return o.ArtifactRoot
}

// RunFileEvents loads an orchestration definition and streams one complete FSM run.
func RunFileEvents(ctx context.Context, engine *query.Engine, path string, personaDir string, taskPrompt string) <-chan query.LoopEvent {
	observe.TraceCtx(ctx, "orchestration", "RunFileEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunFileEvents", "exit")
	observe.TraceCtx(ctx, "orchestration", "RunFileEvents", "return: RunFileEventsWithOptions(ctx, engine, path, RunOptions{\n\tPersonaDir:\tpersonaD...")
	return RunFileEventsWithOptions(ctx, engine, path, RunOptions{
		PersonaDir: personaDir,
		TaskPrompt: taskPrompt,
	})
}

func RunFileEventsWithOptions(ctx context.Context, engine *query.Engine, path string, opts RunOptions) <-chan query.LoopEvent {
	observe.TraceCtx(ctx, "orchestration", "RunFileEventsWithOptions", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunFileEventsWithOptions", "exit")
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		def, err := LoadDefinitionFile(path)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "RunFileEventsWithOptions", "if: err != nil")
			ch <- query.ErrorEvent{Err: err}
			return
		}
		runEvents(ctx, ch, engine, def, opts)
	}()
	observe.TraceCtx(ctx, "orchestration", "RunFileEventsWithOptions", "return: ch")
	return ch
}

// RunEvents streams one complete FSM run for an already-loaded definition.
func RunEvents(ctx context.Context, engine *query.Engine, def Definition, personaDir string, taskPrompt string) <-chan query.LoopEvent {
	observe.TraceCtx(ctx, "orchestration", "RunEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunEvents", "exit")
	observe.TraceCtx(ctx, "orchestration", "RunEvents", "return: RunEventsWithOptions(ctx, engine, def, RunOptions{\n\tPersonaDir:\tpersonaDir,\n\t...")
	return RunEventsWithOptions(ctx, engine, def, RunOptions{
		PersonaDir: personaDir,
		TaskPrompt: taskPrompt,
	})
}

func RunEventsWithOptions(ctx context.Context, engine *query.Engine, def Definition, opts RunOptions) <-chan query.LoopEvent {
	observe.TraceCtx(ctx, "orchestration", "RunEventsWithOptions", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunEventsWithOptions", "exit")
	ch := make(chan query.LoopEvent, 16)
	go func() {
		defer close(ch)
		runEvents(ctx, ch, engine, def, opts)
	}()
	observe.TraceCtx(ctx, "orchestration", "RunEventsWithOptions", "return: ch")
	return ch
}

func runEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, def Definition, opts RunOptions) {
	observe.TraceCtx(ctx, "orchestration", "runEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "runEvents", "exit")
	runtime, err := NewRuntime(def)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
		ch <- query.ErrorEvent{Err: err}
		return
	}
	artifactRoot := opts.artifactRoot()
	bus := eventBus(engine)
	projection := NewProjection()
	emitOrchestration(ch, bus, projection, query.OrchestrationStartedEvent{Name: def.Name, Initial: def.Initial})
	if err := EnsureRunDirs(def, artifactRoot); err != nil {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
		ch <- query.ErrorEvent{Err: err}
		return
	}

	taskPrompt := opts.TaskPrompt
	persistentStateEngines := make(map[string]*query.Engine)
	var transitionHandoff []Artifact
	var transitionFrom string
	var transitionEvent string
	for !runtime.States[runtime.FSM.Current()].Terminal {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "for: !runtime.States[runtime.FSM.Current()].Terminal")
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		stateEngine, err := engineForOrchestrationState(engine, persistentStateEngines, state)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: err}
			return
		}
		stateTaskPrompt := ""
		if state.Control.IsZero() {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: state.Control.IsZero()")
			stateTaskPrompt = taskPrompt
			taskPrompt = ""
		}
		event, _, err := runNodeEvents(ctx, ch, stateEngine, projection, opts.PersonaDir, def, state, stateTaskPrompt, "", transitionHandoff, transitionFrom, transitionEvent, artifactRoot)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: err}
			return
		}
		transition, ok := runtime.TransitionFor(stateID, event)
		if !ok {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: !ok")
			ch <- query.ErrorEvent{Err: fmt.Errorf("transition %q from %q: no unique transition", event, stateID)}
			return
		}
		if err := runtime.FSM.Event(ctx, event); err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: fmt.Errorf("transition %q from %q: %w", event, stateID, err)}
			return
		}
		nextStateID := runtime.FSM.Current()
		emitOrchestration(ch, bus, projection, query.OrchestrationTransitionEvent{From: stateID, Event: event, To: nextStateID})
		transitionHandoff = append([]Artifact(nil), transition.Handoff...)
		transitionFrom = stateID
		transitionEvent = event
	}

	emitOrchestration(ch, bus, projection, query.OrchestrationCompletedEvent{Name: def.Name})
	ch <- query.TurnCompleteEvent{
		Response:   model.Response{StopReason: model.StopEndTurn},
		StopReason: model.StopEndTurn,
	}
}

func engineForOrchestrationState(root *query.Engine, persistent map[string]*query.Engine, state State) (*query.Engine, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !stateRunsPersona(state) {
		observe.GlobalTrace("if: !stateRunsPersona(state)")
		observe.GlobalTrace("return: root, nil")
		return root, nil
	}
	if stateUsesPersistentConversation(state) {
		observe.GlobalTrace("if: stateUsesPersistentConversation(state)")
		if engine, ok := persistent[state.ID]; ok {
			observe.GlobalTrace("if: engine, ok := persistent[state.ID]; ok")
			observe.GlobalTrace("return: engine, nil")
			return engine, nil
		}
		engine, _ := root.ForkFreshConversation()
		persistent[state.ID] = engine
		observe.GlobalTrace("return: engine, nil")
		return engine, nil
	}
	engine, _ := root.ForkFreshConversation()
	observe.GlobalTrace("return: engine, nil")
	return engine, nil
}

func stateRunsPersona(state State) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: state.Control.IsZero() || strings.TrimSpace(state.Persona) != \"\"")
	return state.Control.IsZero() || strings.TrimSpace(state.Persona) != ""
}

func stateUsesPersistentConversation(state State) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: state.ID == \"next_item\" && strings.TrimSpace(state.Persona) != \"\" && state.Control.ForEachNext...")
	return state.ID == "next_item" && strings.TrimSpace(state.Persona) != "" && state.Control.ForEachNext != nil
}

func EnsureRunDirs(def Definition, artifactRoots ...string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		observe.GlobalTrace("if: len(artifactRoots) > 0")
		artifactRoot = artifactRoots[0]
	}
	if strings.TrimSpace(artifactRoot) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(artifactRoot) == \"\"")
		artifactRoot = DefaultArtifactRoot
	}
	for _, dir := range orchestrationDirs(def, artifactRoot) {
		observe.GlobalTrace("range orchestrationDirs(def, artifactRoot)")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: fmt.Errorf(\"create orchestration directory %q: %w\", dir, err)")
			return fmt.Errorf("create orchestration directory %q: %w", dir, err)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func orchestrationDirs(def Definition, artifactRoot string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dirs := map[string]bool{
		artifactRoot:                             true,
		filepath.Join(artifactRoot, "processes"): true,
	}
	addPath := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		dir := filepath.Dir(resolveArtifactPath(path, artifactRoot))
		if dir != "." && dir != "" {
			dirs[dir] = true
		}
	}
	for _, state := range def.States {
		observe.GlobalTrace("range def.States")
		for _, artifact := range state.Artifacts.Inputs {
			observe.GlobalTrace("range state.Artifacts.Inputs")
			addPath(artifact.Path)
		}
		for _, artifact := range state.Artifacts.Outputs {
			observe.GlobalTrace("range state.Artifacts.Outputs")
			addPath(artifact.Path)
		}
		if control := state.Control.ForEachNext; control != nil {
			observe.GlobalTrace("if: control != nil")
			addPath(control.ListPath)
			addPath(control.CursorPath)
			addPath(control.HandoffPath)
		}
		if control := state.Control.MarkCurrentItem; control != nil {
			observe.GlobalTrace("if: control != nil")
			addPath(control.ListPath)
			addPath(control.CursorPath)
		}
		if control := state.Control.ArtifactVerdict; control != nil {
			observe.GlobalTrace("if: control != nil")
			addPath(control.Path)
		}
		if control := state.Control.ArtifactDecision; control != nil {
			observe.GlobalTrace("if: control != nil")
			addPath(control.Path)
		}
	}
	for _, transition := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		for _, artifact := range transition.Handoff {
			observe.GlobalTrace("range transition.Handoff")
			addPath(artifact.Path)
		}
	}
	out := make([]string, 0, len(dirs))
	for dir := range dirs {
		observe.GlobalTrace("range dirs")
		out = append(out, dir)
	}
	sort.Strings(out)
	observe.GlobalTrace("return: out")
	return out
}

func RunNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, string, error) {
	observe.TraceCtx(ctx, "orchestration", "RunNodeEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunNodeEvents", "exit")
	observe.TraceCtx(ctx, "orchestration", "RunNodeEvents", "return: runNodeEvents(ctx, ch, engine, projection, personaDir, def, state, taskPrompt...")
	return runNodeEvents(ctx, ch, engine, projection, personaDir, def, state, taskPrompt, handoffPrompt, nil, "", "", artifactRoots...)
}

func runNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, transitionHandoff []Artifact, transitionFrom string, transitionEvent string, artifactRoots ...string) (string, string, error) {
	observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "exit")
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: len(artifactRoots) > 0")
		artifactRoot = artifactRoots[0]
	}
	bus := eventBus(engine)
	if !state.Control.IsZero() {
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: !state.Control.IsZero()")
		control := ControlName(state)
		emitOrchestration(ch, bus, projection, query.OrchestrationStateStartedEvent{StateID: state.ID, Control: control})
		emitOrchestration(ch, bus, projection, query.OrchestrationControlEvent{StateID: state.ID, Control: control})
		event, err := ExecuteControl(state)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: err != nil")
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: \"\", \"\", fmt.Errorf(\"control state %q failed: %w\", state.ID, err)")
			return "", "", fmt.Errorf("control state %q failed: %w", state.ID, err)
		}
		emitOrchestration(ch, bus, projection, query.OrchestrationControlEvent{StateID: state.ID, Control: control, Event: event})
		if state.Control.ForEachNext != nil && state.Persona != "" && event == state.Control.ForEachNext.ItemEvent {
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: state.Control.ForEachNext != nil && state.Persona != \"\" && event == state.Control.ForEachNext.ItemEvent")
			controlHandoff := controlPersonaHandoffArtifacts(state, transitionHandoff)
			if err := runPersonaForState(ctx, ch, bus, projection, engine, personaDir, def, state, "", handoffPrompt, controlHandoff, transitionFrom, transitionEvent, artifactRoot); err != nil {
				observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: err != nil")
				observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: \"\", \"\", fmt.Errorf(\"state %q failed: %w\", state.ID, err)")
				return "", "", fmt.Errorf("state %q failed: %w", state.ID, err)
			}
			if state.Control.ForEachNext.HandoffPath != "" {
				observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: state.Control.ForEachNext.HandoffPath != \"\"")
				emitOrchestration(ch, bus, projection, query.OrchestrationHandoffEvent{
					StateID:   state.ID,
					Event:     event,
					Path:      state.Control.ForEachNext.HandoffPath,
					Direction: "write",
				})
			}
		}
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: event, \"\", nil")
		return event, "", nil
	}

	if err := runPersonaForState(ctx, ch, bus, projection, engine, personaDir, def, state, taskPrompt, handoffPrompt, transitionHandoff, transitionFrom, transitionEvent, artifactRoot); err != nil {
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: \"\", \"\", fmt.Errorf(\"state %q failed: %w\", state.ID, err)")
		return "", "", fmt.Errorf("state %q failed: %w", state.ID, err)
	}

	event, err := SelectStateEvent(state)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: \"\", \"\", fmt.Errorf(\"select event for state %q: %w\", state.ID, err)")
		return "", "", fmt.Errorf("select event for state %q: %w", state.ID, err)
	}
	observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: event, \"\", nil")
	return event, "", nil
}

func runPersonaForState(ctx context.Context, ch chan<- query.LoopEvent, bus *observe.EventBus, projection *Projection, engine *query.Engine, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, transitionHandoff []Artifact, transitionFrom string, transitionEvent string, artifactRoot string) error {
	observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "exit")
	personaDef, err := LoadPersonaForState(personaDir, state)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "return: err")
		return err
	}
	emitOrchestration(ch, bus, projection, query.OrchestrationStateStartedEvent{StateID: state.ID, PersonaID: personaDef.ID})
	transitionHandoffPrompt, readEvents, err := RenderTransitionHandoff(state, transitionHandoff, artifactRoot, true)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "return: err")
		return err
	}
	for _, read := range readEvents {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "range readEvents")
		emitOrchestration(ch, bus, projection, query.OrchestrationHandoffEvent{
			StateID:    state.ID,
			From:       transitionFrom,
			Event:      transitionEvent,
			To:         state.ID,
			ArtifactID: read.ArtifactID,
			Path:       read.Path,
			Direction:  "read",
		})
	}
	if strings.TrimSpace(transitionHandoffPrompt) != "" {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "if: strings.TrimSpace(transitionHandoffPrompt) != \"\"")
		handoffPrompt = transitionHandoffPrompt
	}

	_, err = RunStateEvents(ctx, ch, engine, projection, def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "return: err")
		return err
	}

	observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "return: nil")
	return nil
}

func controlPersonaHandoffArtifacts(state State, transitionHandoff []Artifact) []Artifact {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	handoff := append([]Artifact(nil), transitionHandoff...)
	if state.Control.ForEachNext != nil && state.Control.ForEachNext.CursorPath != "" {
		observe.GlobalTrace("if: state.Control.ForEachNext != nil && state.Control.ForEachNext.CursorPath != \"\"")
		handoff = append(handoff, Artifact{
			ID:          "current_item",
			Path:        state.Control.ForEachNext.CursorPath,
			Required:    true,
			Description: "Current checklist item selected by the foreach_next control.",
		})
	}
	observe.GlobalTrace("return: handoff")
	return handoff
}

func ControlName(state State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case state.Control.ForEachNext != nil:
		observe.GlobalTrace("case: state.Control.ForEachNext != nil")
		return "foreach_next"
	case state.Control.MarkCurrentItem != nil:
		observe.GlobalTrace("case: state.Control.MarkCurrentItem != nil")
		return "mark_current_item"
	case state.Control.ArtifactVerdict != nil:
		observe.GlobalTrace("case: state.Control.ArtifactVerdict != nil")
		return "artifact_verdict"
	case state.Control.ArtifactDecision != nil:
		observe.GlobalTrace("case: state.Control.ArtifactDecision != nil")
		return "artifact_decision"
	default:
		observe.GlobalTrace("default")
		return "unknown"
	}
}

func LoadPersonaForState(personaDir string, state State) (persona.Definition, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	personaID := state.Persona
	if personaID == "" {
		observe.GlobalTrace("if: personaID == \"\"")
		personaID = state.ID
	}
	observe.GlobalTrace("return: persona.LoadDefinitionFile(filepath.Join(personaDir, personaID+\".yaml\"))")
	return persona.LoadDefinitionFile(filepath.Join(personaDir, personaID+".yaml"))
}

func SelectStateEvent(state State) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if state.Event.Default != "" {
		observe.GlobalTrace("if: state.Event.Default != \"\"")
		observe.GlobalTrace("return: state.Event.Default, nil")
		return state.Event.Default, nil
	}
	observe.GlobalTrace("return: EventComplete, nil")
	return EventComplete, nil
}

func RunStateEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, error) {
	observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "exit")
	artifactRoot := DefaultArtifactRoot
	if len(artifactRoots) > 0 {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: len(artifactRoots) > 0")
		artifactRoot = artifactRoots[0]
	}
	system, prompt, err := BuildPromptWithArtifactRootChecked(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "return: \"\", err")
		return "", err
	}

	var text strings.Builder
	start := time.Now()
	completionCheck := requiredOutputArtifactCompletionCheck(state, artifactRoot)
	for ev := range engine.RunPragmaLoopWithSystemCompletionCheck(ctx, system, prompt, completionCheck) {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "range engine.RunPragmaLoopWithSystemCompletionCheck(ctx, system, prompt, completion...")
		switch e := ev.(type) {
		case query.TextEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.TextEvent")
			text.WriteString(e.Text)
			ch <- e
		case query.ThinkingEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.ThinkingEvent")
			ch <- e
		case query.ToolCallEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.ToolCallEvent")
			ch <- e
		case query.ToolResultEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.ToolResultEvent")
			ch <- e
		case query.RetryEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.RetryEvent")
			ch <- e
		case query.ErrorEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.ErrorEvent")
			return text.String(), e.Err
		case query.TurnCompleteEvent:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typecase: query.TurnCompleteEvent")
			emitOrchestration(ch, eventBus(engine), projection, query.OrchestrationStateCompletedEvent{StateID: state.ID, Duration: time.Since(start)})
		default:
			observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "typedefault")
			ch <- ev
		}
	}
	observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "return: text.String(), nil")
	return text.String(), nil
}

func BuildPrompt(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string) (model.SystemPrompt, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: BuildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt...")
	return BuildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, DefaultArtifactRoot)
}

func BuildPromptWithArtifactRoot(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoot string) (model.SystemPrompt, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	system, prompt, _ := buildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot, false)
	observe.GlobalTrace("return: system, prompt")
	return system, prompt
}

func BuildPromptWithArtifactRootChecked(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoot string) (model.SystemPrompt, string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: buildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt...")
	return buildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot, true)
}

func buildPromptWithArtifactRoot(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoot string, strict bool) (model.SystemPrompt, string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	completionContract := RenderStateCompletionContract(state)
	systemText := strings.TrimRight(personaDef.Prompt, "\n") + "\n\n" + query.PragmaLoopSystemPrompt()
	if completionContract != "" {
		observe.GlobalTrace("if: completionContract != \"\"")
		systemText += "\n\n" + completionContract
	}
	system := model.SystemPrompt{Blocks: []model.SystemBlock{{
		Text:      systemText,
		Cacheable: false,
	}}}

	var b strings.Builder
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		observe.GlobalTrace("if: err == nil && strings.TrimSpace(cwd) != \"\"")
		fmt.Fprintf(&b, "## Session Context\n\nCurrent working directory: `%s`\n\n", cwd)
	}
	if strings.TrimSpace(taskPrompt) != "" && state.TaskPrompt != TaskPromptNone {
		observe.GlobalTrace("if: strings.TrimSpace(taskPrompt) != \"\" && state.TaskPrompt != TaskPromptNone")
		fmt.Fprintf(&b, "## Task\n\n%s\n", taskPrompt)
	} else {
		observe.GlobalTrace("else: strings.TrimSpace(taskPrompt) != \"\" && state.TaskPrompt != TaskPromptNone")
		if strings.TrimSpace(handoffPrompt) != "" {
			observe.GlobalTrace("if: strings.TrimSpace(handoffPrompt) != \"\"")
			fmt.Fprintf(&b, "## Handoff From Previous Phase\n\n%s\n\n", strings.TrimSpace(handoffPrompt))
		}
	}
	if contract := RenderArtifactContract(state.Artifacts); contract != "" {
		observe.GlobalTrace("if: contract != \"\"")
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			observe.GlobalTrace("if: b.Len() > 0 && !strings.HasSuffix(b.String(), \"\\n\\n\")")
			b.WriteString("\n")
		}
		b.WriteString(contract)
	}
	if contract := RenderNextForEachContract(def, state); contract != "" {
		observe.GlobalTrace("if: contract != \"\"")
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			observe.GlobalTrace("if: b.Len() > 0 && !strings.HasSuffix(b.String(), \"\\n\\n\")")
			b.WriteString("\n")
		}
		b.WriteString(contract)
	}
	observe.GlobalTrace("return: system, b.String(), nil")

	return system, b.String(), nil
}

type HandoffRead struct {
	ArtifactID string
	Path       string
}

func RenderTransitionHandoff(state State, artifacts []Artifact, artifactRoot string, strict bool) (string, []HandoffRead, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(artifacts) == 0 {
		observe.GlobalTrace("if: len(artifacts) == 0")
		observe.GlobalTrace("return: \"\", nil, nil")
		return "", nil, nil
	}
	var b strings.Builder
	reads := make([]HandoffRead, 0, len(artifacts))
	for _, artifact := range artifacts {
		observe.GlobalTrace("range artifacts")
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		content, err := os.ReadFile(path)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			if strict && (artifact.Required || !os.IsNotExist(err)) {
				observe.GlobalTrace("if: strict && (artifact.Required || !os.IsNotExist(err))")
				observe.GlobalTrace("return: \"\", nil, fmt.Errorf(\"read transition handoff artifact %q at %q: %w\", artifact...")
				return "", nil, fmt.Errorf("read transition handoff artifact %q at %q: %w", artifact.ID, path, err)
			}
			if artifact.Required || !os.IsNotExist(err) {
				observe.GlobalTrace("if: artifact.Required || !os.IsNotExist(err)")
				fmt.Fprintf(&b, "### `%s` (%s)\nUnavailable: %v\n\n", artifact.ID, artifactRequirement(artifact), err)
			}
			continue
		}
		reads = append(reads, HandoffRead{ArtifactID: artifact.ID, Path: path})
		fmt.Fprintf(&b, "### `%s` (%s)\n", artifact.ID, artifactRequirement(artifact))
		if artifact.Description != "" {
			observe.GlobalTrace("if: artifact.Description != \"\"")
			fmt.Fprintf(&b, "Description: %s\n", artifact.Description)
		}
		b.WriteString("Content:\n")
		b.WriteString(strings.TrimRight(string(content), "\n"))
		b.WriteString("\n\n")
		if artifact.ID == "current_item" && state.Persona == "item_worker" {
			observe.GlobalTrace("if: artifact.ID == \"current_item\" && state.Persona == \"item_worker\"")
			b.WriteString(renderSourceEditTransport())
		}
	}
	observe.GlobalTrace("return: strings.TrimSpace(b.String()), reads, nil")
	return strings.TrimSpace(b.String()), reads, nil
}

func renderSourceEditTransport() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: `### ` + \"`source_edit_transport`\" + ` (runtime capability)\nRepository source...")
	return `### ` + "`source_edit_transport`" + ` (runtime capability)
Repository source edits in this direct bash-fence runtime must use Pragma's structured patch runner.
Use normal bash for read-only inspection, validation commands, and runtime artifact writes.
For repository source mutations, respond with exactly one fenced bash block containing one ` + "`apply_patch`" + ` heredoc and no other shell command:

` + "```bash" + `
apply_patch <<'PATCH'
*** Begin Patch
*** Update File: path/from/repo/root.go
@@
 unchanged context line starts with one literal space
-old line starts with minus
+new line starts with plus
 another unchanged context line starts with one literal space
*** End Patch
PATCH
` + "```" + `

Patch grammar is strict:
- The patch body must start with ` + "`*** Begin Patch`" + ` and end with ` + "`*** End Patch`" + `.
- Use ` + "`*** Update File: <repo-relative path>`" + ` for existing source files.
- Inside update hunks, every source line must start with exactly one marker character: space for unchanged context, ` + "`-`" + ` for removed lines, or ` + "`+`" + ` for added lines.
- Do not omit the leading space marker on unchanged context lines.
- Do not use git/unified-diff headers (` + "`---`" + `, ` + "`+++`" + `, ` + "`index`" + `, or numbered ` + "`@@ -a,b +c,d @@`" + `). Use plain ` + "`@@`" + ` hunk markers.
- For insertion after a visible anchor, include the anchor as a space-prefixed context line before the ` + "`+`" + ` lines. Do not put ` + "`+`" + ` lines before the anchor.
- Do not invent comments, blank lines, or helper text beyond the requested source change.
- Do not use ` + "`git apply`" + `, ` + "`sed`" + `, ` + "`awk`" + `, ` + "`perl`" + `, ` + "`python`" + `, ` + "`cat`" + `, ` + "`tee`" + `, or shell redirection to mutate repository source files.
- If ` + "`apply_patch`" + ` fails, reread the target range and retry with a smaller ` + "`apply_patch`" + ` hunk; do not switch editing tools.

`
}

func resolveArtifactPath(path string, artifactRoot string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if filepath.IsAbs(path) {
		observe.GlobalTrace("if: filepath.IsAbs(path)")
		observe.GlobalTrace("return: path")
		return path
	}
	root := artifactRoot
	if strings.TrimSpace(root) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(root) == \"\"")
		root = DefaultArtifactRoot
	}
	observe.GlobalTrace("return: filepath.Join(root, path)")
	return filepath.Join(root, path)
}

func RenderArtifactContract(artifacts Artifacts) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if artifacts.IsZero() {
		observe.GlobalTrace("if: artifacts.IsZero()")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	var b strings.Builder
	b.WriteString("## Runtime Artifact Contract\n\n")
	if len(artifacts.Outputs) > 0 {
		observe.GlobalTrace("if: len(artifacts.Outputs) > 0")
		b.WriteString("Outputs:\n")
		for _, artifact := range artifacts.Outputs {
			observe.GlobalTrace("range artifacts.Outputs")
			writeArtifactLine(&b, artifact)
		}
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func RenderStateCompletionContract(state State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !state.Control.IsZero() && strings.TrimSpace(state.Persona) == "" {
		observe.GlobalTrace("if: !state.Control.IsZero() && strings.TrimSpace(state.Persona) == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	var b strings.Builder
	b.WriteString("## Runtime Completion Contract\n\n")
	b.WriteString("This orchestration state does not use plain prose final answers.\n")
	b.WriteString("When this state is complete, respond with exactly one fenced bash block and no prose outside it.\n")
	requiredOutputs := requiredOutputArtifacts(state.Artifacts.Outputs)
	if len(requiredOutputs) > 0 {
		observe.GlobalTrace("if: len(requiredOutputs) > 0")
		b.WriteString("Before completing, every required output artifact below must exist:\n")
		for _, artifact := range requiredOutputs {
			observe.GlobalTrace("range requiredOutputs")
			fmt.Fprintf(&b, "- `%s`: `%s`\n", artifact.ID, artifact.Path)
		}
		b.WriteString("The completion bash block may write the final required artifact content, or verify already-written artifacts, but it must end with:\n")
	} else {
		observe.GlobalTrace("else: len(requiredOutputs) > 0")
		b.WriteString("After the state-specific work is complete, the completion bash block must end with:\n")
	}
	b.WriteString("echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n")
	b.WriteString("Do not emit COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after a read-only inspection unless the state-specific work is already complete.\n")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func requiredOutputArtifacts(outputs []Artifact) []Artifact {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	required := make([]Artifact, 0, len(outputs))
	for _, artifact := range outputs {
		observe.GlobalTrace("range outputs")
		if artifact.Required {
			observe.GlobalTrace("if: artifact.Required")
			required = append(required, artifact)
		}
	}
	observe.GlobalTrace("return: required")
	return required
}

func requiredOutputArtifactCompletionCheck(state State, artifactRoot string) query.PragmaLoopCompletionCheck {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	required := requiredOutputArtifacts(state.Artifacts.Outputs)
	if len(required) == 0 {
		observe.GlobalTrace("if: len(required) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: func() (bool, string, error) {\n\tvar missing []string\n\tfor _, artifact := rang...")
	return func() (bool, string, error) {
		var missing []string
		for _, artifact := range required {
			path := resolveArtifactPath(artifact.Path, artifactRoot)
			if _, err := os.Stat(path); err != nil {
				missing = append(missing, fmt.Sprintf("- `%s`: `%s` (%v)", artifact.ID, path, err))
			}
		}
		if len(missing) == 0 {
			return true, "", nil
		}
		return false, fmt.Sprintf("Completion was rejected because required output artifact(s) are missing:\n%s\n\nRun another fenced bash block that creates or verifies the missing artifact(s), then echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again.", strings.Join(missing, "\n")), nil
	}
}

func RenderNextForEachContract(def Definition, state State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	next, ok := nextStateForDefaultEvent(def, state)
	if !ok || next.Control.ForEachNext == nil {
		observe.GlobalTrace("if: !ok || next.Control.ForEachNext == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	control := next.Control.ForEachNext
	pendingStatus := control.PendingStatus
	if pendingStatus == "" {
		observe.GlobalTrace("if: pendingStatus == \"\"")
		pendingStatus = "pending"
	}
	doneStatus := control.DoneStatus
	if doneStatus == "" {
		observe.GlobalTrace("if: doneStatus == \"\"")
		doneStatus = "approved"
	}
	blockedStatus := control.BlockedStatus
	if blockedStatus == "" {
		blockedStatus = "blocked"
	}
	var b strings.Builder
	b.WriteString("## Output Artifact Shape Required By Next State\n\n")
	fmt.Fprintf(&b, "The next state is `%s`, a `foreach_next` control state.\n", next.ID)
	fmt.Fprintf(&b, "It will parse `%s` and write the selected item to `%s`.\n\n", control.ListPath, control.CursorPath)
	if control.HandoffPath != "" {
		observe.GlobalTrace("if: control.HandoffPath != \"\"")
		fmt.Fprintf(&b, "Its `%s` persona will write the selected-item handoff to `%s` after selection.\n", next.Persona, control.HandoffPath)
		b.WriteString("Do not write a separate generic next-item handoff; the next state's persona owns that handoff after the cursor is selected.\n\n")
	}
	fmt.Fprintf(&b, "Write `%s` as JSON with this exact shape:\n\n", control.ListPath)
	b.WriteString("```json\n")
	b.WriteString("{\n")
	b.WriteString("  \"items\": [\n")
	b.WriteString("    {\n")
	b.WriteString("      \"id\": \"stable-string-id\",\n")
	b.WriteString("      \"title\": \"string\",\n")
	b.WriteString("      \"description\": \"string\",\n")
	b.WriteString("      \"approach\": \"string\",\n")
	b.WriteString("      \"acceptance\": [\"string\"],\n")
	b.WriteString("      \"allowed_files\": [\"string\"],\n")
	b.WriteString("      \"forbidden_files\": [\"string\"],\n")
	b.WriteString("      \"coupled_edit_paths\": [\"string\"],\n")
	b.WriteString("      \"acceptance_check\": \"string\",\n")
	b.WriteString("      \"validation_command\": \"string\",\n")
	b.WriteString("      \"validation_deferred_until\": \"string\",\n")
	b.WriteString("      \"report_changed_files\": [\"string\"],\n")
	b.WriteString("      \"blocked_by\": [\"item-id\"],\n")
	b.WriteString("      \"block_reason\": \"string\",\n")
	fmt.Fprintf(&b, "      \"status\": %q\n", pendingStatus)
	b.WriteString("    }\n")
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- `id` is required and must be a JSON string, never a number.\n")
	fmt.Fprintf(&b, "- `status` is required and must be `%q`, `%q`, or `%q`.\n", pendingStatus, blockedStatus, doneStatus)
	fmt.Fprintf(&b, "- Use `%s` only for items runnable immediately by the next worker.\n", pendingStatus)
	fmt.Fprintf(&b, "- Use `%s` with `blocked_by` for unfinished items that depend on another checklist item before they can run.\n", blockedStatus)
	fmt.Fprintf(&b, "- When no `%s` items remain and no `%s` items remain, `%s` will write a cursor item with status `%s`.\n", pendingStatus, blockedStatus, next.ID, doneStatus)
	if control.BlockedEvent != "" {
		fmt.Fprintf(&b, "- When no `%s` items remain but `%s` items remain, `%s` emits `%s`.\n", pendingStatus, blockedStatus, next.ID, control.BlockedEvent)
	}
	if control.HandoffPath != "" {
		observe.GlobalTrace("if: control.HandoffPath != \"\"")
		fmt.Fprintf(&b, "- `%s` will select the item and its `%s` persona will write the handoff for that exact selected item, not for future checklist items.\n", next.ID, next.Persona)
	}
	b.WriteString("- `acceptance`, `allowed_files`, `forbidden_files`, `coupled_edit_paths`, `report_changed_files`, and `blocked_by` must be JSON arrays.\n")
	b.WriteString("- If `validation_deferred_until` names another checklist item id, set `blocked_by` to that id and use blocked status instead of pending.\n")
	b.WriteString("- Use `validation_command: \"none\"` only when `acceptance_check` is present or `validation_deferred_until` names what later item or surface makes validation runnable.\n")
	b.WriteString("- Put every path from `coupled_edit_paths` in `allowed_files`; the worker may edit only `allowed_files`.\n")
	b.WriteString("- Extra item fields are allowed only if they are valid JSON and should be preserved by later controls.\n\n")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func nextStateForDefaultEvent(def Definition, state State) (State, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	event := state.Event.Default
	if event == "" {
		observe.GlobalTrace("if: event == \"\"")
		event = EventComplete
	}
	for _, transition := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		if transition.Event != event || !transitionIncludesFrom(transition, state.ID) {
			observe.GlobalTrace("if: transition.Event != event || !transitionIncludesFrom(transition, state.ID)")
			continue
		}
		for _, candidate := range def.States {
			observe.GlobalTrace("range def.States")
			if candidate.ID == transition.To {
				observe.GlobalTrace("if: candidate.ID == transition.To")
				observe.GlobalTrace("return: candidate, true")
				return candidate, true
			}
		}
		observe.GlobalTrace("return: State{}, false")
		return State{}, false
	}
	observe.GlobalTrace("return: State{}, false")
	return State{}, false
}

func transitionIncludesFrom(transition Transition, stateID string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, from := range transition.From {
		observe.GlobalTrace("range transition.From")
		if from == stateID {
			observe.GlobalTrace("if: from == stateID")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func writeArtifactLine(b *strings.Builder, artifact Artifact) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fmt.Fprintf(b, "- `%s` (%s): `%s`", artifact.ID, artifactRequirement(artifact), artifact.Path)
	if artifact.Description != "" {
		observe.GlobalTrace("if: artifact.Description != \"\"")
		fmt.Fprintf(b, " - %s", artifact.Description)
	}
	if artifact.Kind != "" {
		observe.GlobalTrace("if: artifact.Kind != \"\"")
		fmt.Fprintf(b, " Kind: %s.", artifact.Kind)
	}
	if len(artifact.AllowedValues) > 0 {
		observe.GlobalTrace("if: len(artifact.AllowedValues) > 0")
		fmt.Fprintf(b, " Allowed values: %s.", strings.Join(artifact.AllowedValues, ", "))
	}
	b.WriteString("\n")
}

func artifactRequirement(artifact Artifact) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if artifact.Required {
		observe.GlobalTrace("if: artifact.Required")
		observe.GlobalTrace("return: \"required\"")
		return "required"
	}
	observe.GlobalTrace("return: \"optional\"")
	return "optional"
}

func eventBus(engine *query.Engine) *observe.EventBus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine == nil {
		observe.GlobalTrace("if: engine == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: engine.EventBus()")
	return engine.EventBus()
}

func emitQueryObserve(ch chan<- query.LoopEvent, bus *observe.EventBus, ev query.LoopEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ch <- ev
	if bus == nil {
		observe.GlobalTrace("if: bus == nil")
		return
	}
	switch e := ev.(type) {
	case query.OrchestrationStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStartedEvent")
		bus.Emit(observe.OrchestrationStarted{
			EventHeader: observe.NewEventHeader("OrchestrationStarted", "", "", ""),
			Name:        e.Name,
			Initial:     e.Initial,
		})
	case query.OrchestrationStateStartedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateStartedEvent")
		bus.Emit(observe.OrchestrationStateStarted{
			EventHeader: observe.NewEventHeader("OrchestrationStateStarted", "", "", ""),
			StateID:     e.StateID,
			PersonaID:   e.PersonaID,
			Control:     e.Control,
		})
	case query.OrchestrationStateCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationStateCompletedEvent")
		bus.Emit(observe.OrchestrationStateCompleted{
			EventHeader: observe.NewEventHeader("OrchestrationStateCompleted", "", "", ""),
			StateID:     e.StateID,
			Duration:    e.Duration,
			DurationMs:  e.Duration.Milliseconds(),
		})
	case query.OrchestrationControlEvent:
		observe.GlobalTrace("typecase: query.OrchestrationControlEvent")
		bus.Emit(observe.OrchestrationControl{
			EventHeader: observe.NewEventHeader("OrchestrationControl", "", "", ""),
			StateID:     e.StateID,
			Control:     e.Control,
			Event:       e.Event,
		})
	case query.OrchestrationTransitionEvent:
		observe.GlobalTrace("typecase: query.OrchestrationTransitionEvent")
		bus.Emit(observe.OrchestrationTransition{
			EventHeader: observe.NewEventHeader("OrchestrationTransition", "", "", ""),
			From:        e.From,
			Event:       e.Event,
			To:          e.To,
		})
	case query.OrchestrationHandoffEvent:
		observe.GlobalTrace("typecase: query.OrchestrationHandoffEvent")
		bus.Emit(observe.OrchestrationHandoff{
			EventHeader: observe.NewEventHeader("OrchestrationHandoff", "", "", ""),
			StateID:     e.StateID,
			From:        e.From,
			Event:       e.Event,
			To:          e.To,
			ArtifactID:  e.ArtifactID,
			Path:        e.Path,
			Direction:   e.Direction,
		})
	case query.OrchestrationCompletedEvent:
		observe.GlobalTrace("typecase: query.OrchestrationCompletedEvent")
		bus.Emit(observe.OrchestrationCompleted{
			EventHeader: observe.NewEventHeader("OrchestrationCompleted", "", "", ""),
			Name:        e.Name,
		})
	}
}

func emitOrchestration(ch chan<- query.LoopEvent, bus *observe.EventBus, projection *Projection, ev query.LoopEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	emitQueryObserve(ch, bus, ev)
	if snapshot, ok := projection.Apply(ev, time.Now()); ok {
		observe.GlobalTrace("if: ok")
		ch <- query.OrchestrationSnapshotEvent{Snapshot: snapshot}
	}
}

func safeHandoffPathSegment(value string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		observe.GlobalTrace("range value")
		switch {
		case r >= 'a' && r <= 'z':
			observe.GlobalTrace("case: r >= 'a' && r <= 'z'")
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			observe.GlobalTrace("case: r >= 'A' && r <= 'Z'")
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			observe.GlobalTrace("case: r >= '0' && r <= '9'")
			b.WriteRune(r)
		case r == '_' || r == '-':
			observe.GlobalTrace("case: r == '_' || r == '-'")
			b.WriteRune(r)
		default:
			observe.GlobalTrace("default")
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		observe.GlobalTrace("if: b.Len() == 0")
		observe.GlobalTrace("return: \"unknown\"")
		return "unknown"
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

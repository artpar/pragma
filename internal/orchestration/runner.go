package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/query"
)

const DefaultArtifactRoot = "/tmp/pragma"
const ArtifactSeedSourceProcessEnvironment = "process_environment"

type RunOptions struct {
	PersonaDir    string
	TaskPrompt    string
	ArtifactRoot  string
	SeedArtifacts map[string]string
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
	seedWrites, err := materializeSeedArtifacts(def, artifactRoot, opts)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
		ch <- query.ErrorEvent{Err: err}
		return
	}
	for _, write := range seedWrites {
		emitOrchestration(ch, bus, projection, query.OrchestrationHandoffEvent{
			ArtifactID: write.ArtifactID,
			Path:       write.Path,
			Direction:  "write",
			Bytes:      write.Bytes,
			SHA256:     write.SHA256,
		})
	}

	originalTaskPrompt := opts.TaskPrompt
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
			if state.TaskPrompt == TaskPromptFull {
				stateTaskPrompt = originalTaskPrompt
			} else {
				stateTaskPrompt = taskPrompt
				taskPrompt = ""
			}
		}
		event, _, err := runNodeEvents(ctx, ch, stateEngine, projection, opts.PersonaDir, def, state, stateTaskPrompt, "", originalTaskPrompt, transitionHandoff, transitionFrom, transitionEvent, artifactRoot)
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
	observe.GlobalTrace("return: state.Conversation == \"persistent\"")
	return state.Conversation == "persistent"
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

type seededArtifactWrite struct {
	ArtifactID string
	Path       string
	Bytes      int64
	SHA256     string
}

func materializeSeedArtifacts(def Definition, artifactRoot string, opts RunOptions) ([]seededArtifactWrite, error) {
	var writes []seededArtifactWrite
	seen := make(map[string]bool)
	for _, artifact := range seededArtifacts(def) {
		source := strings.TrimSpace(artifact.Seed.Source)
		if source == "" {
			continue
		}
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		if seen[path] {
			continue
		}
		content, ok, err := seedArtifactContent(source, opts)
		if err != nil {
			return nil, fmt.Errorf("seed artifact %q from %q: %w", artifact.ID, source, err)
		}
		if !ok {
			if artifact.Required {
				return nil, fmt.Errorf("seed artifact %q requires unavailable source %q", artifact.ID, source)
			}
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return nil, fmt.Errorf("write seed artifact %q at %q: %w", artifact.ID, path, err)
		}
		seen[path] = true
		writes = append(writes, seededArtifactWrite{
			ArtifactID: artifact.ID,
			Path:       path,
			Bytes:      int64(len(content)),
			SHA256:     sha256Hex(content),
		})
	}
	return writes, nil
}

func seededArtifacts(def Definition) []Artifact {
	var artifacts []Artifact
	appendSeeded := func(candidates []Artifact) {
		for _, artifact := range candidates {
			if strings.TrimSpace(artifact.Seed.Source) == "" {
				continue
			}
			artifacts = append(artifacts, artifact)
		}
	}
	for _, state := range def.States {
		appendSeeded(state.Artifacts.Inputs)
		appendSeeded(state.Artifacts.Outputs)
	}
	for _, transition := range def.Transitions {
		appendSeeded(transition.Handoff)
	}
	return artifacts
}

func seedArtifactContent(source string, opts RunOptions) ([]byte, bool, error) {
	if opts.SeedArtifacts != nil {
		if content, ok := opts.SeedArtifacts[source]; ok {
			return []byte(content), true, nil
		}
	}
	switch source {
	case ArtifactSeedSourceProcessEnvironment:
		return []byte(renderProcessEnvironmentSeed()), true, nil
	default:
		return nil, false, nil
	}
}

func renderProcessEnvironmentSeed() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Runner Capability Evidence\n\n")
	fmt.Fprintf(&b, "source: %s\n", ArtifactSeedSourceProcessEnvironment)
	fmt.Fprintf(&b, "captured_at: %s\n", time.Now().UTC().Format(time.RFC3339))
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		fmt.Fprintf(&b, "cwd: %s\n", cwd)
	}
	fmt.Fprintf(&b, "os: %s\n", goruntime.GOOS)
	fmt.Fprintf(&b, "arch: %s\n", goruntime.GOARCH)
	pathValue := os.Getenv("PATH")
	fmt.Fprintf(&b, "path: %s\n", pathValue)
	fmt.Fprintf(&b, "\n## Path Entries\n")
	for _, entry := range filepath.SplitList(pathValue) {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		fmt.Fprintf(&b, "- %s\n", entry)
	}
	keys := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(&b, "\n## Environment Variables Present\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "- %s\n", key)
	}
	fmt.Fprintf(&b, "\nNo tool commands were probed by this seed. The receiving state must run bounded local probes before drawing tool-availability conclusions.\n")
	return b.String()
}

func RunNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, artifactRoots ...string) (string, string, error) {
	observe.TraceCtx(ctx, "orchestration", "RunNodeEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunNodeEvents", "exit")
	observe.TraceCtx(ctx, "orchestration", "RunNodeEvents", "return: runNodeEvents(ctx, ch, engine, projection, personaDir, def, state, taskPrompt...")
	return runNodeEvents(ctx, ch, engine, projection, personaDir, def, state, taskPrompt, handoffPrompt, taskPrompt, nil, "", "", artifactRoots...)
}

func runNodeEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, originalTaskPrompt string, transitionHandoff []Artifact, transitionFrom string, transitionEvent string, artifactRoots ...string) (string, string, error) {
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
		if state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffMode == "persona" && event == state.Control.ForEachNext.ItemEvent {
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffMode == \"persona\" && event == state.Control.ForEachNext.ItemEvent")
			controlHandoff := controlPersonaHandoffArtifacts(state, transitionHandoff)
			if err := runPersonaForState(ctx, ch, bus, projection, engine, personaDir, def, state, "", handoffPrompt, originalTaskPrompt, controlHandoff, transitionFrom, transitionEvent, artifactRoot); err != nil {
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
		if state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffMode == "control" && event == state.Control.ForEachNext.ItemEvent && state.Control.ForEachNext.HandoffPath != "" {
			path := state.Control.ForEachNext.HandoffPath
			bytes, sha, _ := artifactDigest(path)
			emitOrchestration(ch, bus, projection, query.OrchestrationHandoffEvent{
				StateID:   state.ID,
				Event:     event,
				Path:      path,
				Direction: "write",
				Bytes:     bytes,
				SHA256:    sha,
			})
		}
		observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: event, \"\", nil")
		return event, "", nil
	}

	if err := runPersonaForState(ctx, ch, bus, projection, engine, personaDir, def, state, taskPrompt, handoffPrompt, originalTaskPrompt, transitionHandoff, transitionFrom, transitionEvent, artifactRoot); err != nil {
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

func runPersonaForState(ctx context.Context, ch chan<- query.LoopEvent, bus *observe.EventBus, projection *Projection, engine *query.Engine, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, originalTaskPrompt string, transitionHandoff []Artifact, transitionFrom string, transitionEvent string, artifactRoot string) error {
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
			Bytes:      read.Bytes,
			SHA256:     read.SHA256,
		})
	}
	if strings.TrimSpace(transitionHandoffPrompt) != "" {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "if: strings.TrimSpace(transitionHandoffPrompt) != \"\"")
		handoffPrompt = transitionHandoffPrompt
	}

	_, err = RunStateEvents(ctx, ch, engine, projection, def, state, personaDef, taskPrompt, handoffPrompt, originalTaskPrompt, transitionHandoff, readEvents, artifactRoot)
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
	if state.Control.ForEachNext != nil && state.Control.ForEachNext.CursorPath != "" && state.Control.ForEachNext.CursorArtifactID != "" {
		observe.GlobalTrace("if: state.Control.ForEachNext != nil && state.Control.ForEachNext.CursorPath != \"\"")
		handoff = append(handoff, Artifact{
			ID:          state.Control.ForEachNext.CursorArtifactID,
			Path:        state.Control.ForEachNext.CursorPath,
			Required:    true,
			Description: state.Control.ForEachNext.CursorArtifactDescription,
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

func RunStateEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, originalTaskPrompt string, transitionHandoff []Artifact, handoffReads []HandoffRead, artifactRoots ...string) (string, error) {
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
	completionCheck := requiredOutputArtifactCompletionCheckWithSnapshots(state, artifactRoot, originalTaskPrompt, handoffSnapshots(handoffReads))
	if cfg, ok := commandEvidenceConfig(state, artifactRoot); ok {
		_ = os.Remove(cfg.Path)
		ctx = query.WithPragmaLoopCommandEvidence(ctx, cfg)
	}
	if cfg, ok := commandPolicyConfig(state, transitionHandoff, artifactRoot); ok {
		ctx = query.WithPragmaLoopCommandPolicy(ctx, cfg)
	}
	runOpts := query.PragmaLoopRunOptions{
		IncludePriorConversation: stateUsesPersistentConversation(state),
	}
	for ev := range engine.RunPragmaLoopWithSystemCompletionCheckOptions(ctx, system, prompt, completionCheck, runOpts) {
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
			for _, artifact := range state.Artifacts.Outputs {
				path := resolveArtifactPath(artifact.Path, artifactRoot)
				bytes, sha, ok := artifactDigest(path)
				if !ok {
					continue
				}
				emitOrchestration(ch, eventBus(engine), projection, query.OrchestrationHandoffEvent{
					StateID:    state.ID,
					ArtifactID: artifact.ID,
					Path:       path,
					Direction:  "write",
					Bytes:      bytes,
					SHA256:     sha,
				})
			}
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
	if gate := RenderResponseOutputGate(state); gate != "" {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n")
		}
		b.WriteString(gate)
	}
	observe.GlobalTrace("return: system, b.String(), nil")

	return system, b.String(), nil
}

func RenderResponseOutputGate(state State) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !state.Control.IsZero() && strings.TrimSpace(state.Persona) == "" {
		observe.GlobalTrace("if: !state.Control.IsZero() && strings.TrimSpace(state.Persona) == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: output gate")
	return `## Response Output Gate

Your assistant ` + "`content`" + ` field must start with exactly these 7 bytes: ` + "```bash" + `
Do not emit ` + "`<think>`" + `, hidden reasoning text, analysis headings, or prose before the bash fence.
If you need to reason, keep it out of the assistant content and output only the final shell block.
Any byte before the opening ` + "```bash" + ` fence is a failed response.
`
}

type HandoffRead struct {
	ArtifactID string
	Path       string
	Bytes      int64
	SHA256     string
	Content    []byte
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
		reads = append(reads, HandoffRead{
			ArtifactID: artifact.ID,
			Path:       path,
			Bytes:      int64(len(content)),
			SHA256:     sha256Hex(content),
			Content:    append([]byte(nil), content...),
		})
		fmt.Fprintf(&b, "### `%s` (%s)\n", artifact.ID, artifactRequirement(artifact))
		if artifact.Description != "" {
			observe.GlobalTrace("if: artifact.Description != \"\"")
			fmt.Fprintf(&b, "Description: %s\n", artifact.Description)
		}
		fmt.Fprintf(&b, "Bytes: %d\nSHA256: %s\n", len(content), sha256Hex(content))
		b.WriteString("Content:\n")
		b.WriteString(strings.TrimRight(string(content), "\n"))
		b.WriteString("\n\n")
		for _, attachment := range artifact.PromptAttachments {
			observe.GlobalTrace("range artifact.PromptAttachments")
			b.WriteString(renderPromptAttachment(attachment))
		}
	}
	observe.GlobalTrace("return: strings.TrimSpace(b.String()), reads, nil")
	return strings.TrimSpace(b.String()), reads, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func artifactDigest(path string) (int64, string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", false
	}
	return int64(len(data)), sha256Hex(data), true
}

func renderSourceEditTransport() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: `### ` + \"`source_edit_transport`\" + ` (runtime capability)\nRepository source...")
	return `### ` + "`source_edit_transport`" + ` (runtime capability)
Repository source edits in this direct bash-fence runtime may use normal shell commands or Pragma's structured patch runner.
For complex multi-line or multi-file source edits, prefer a structured source edit over ad hoc stream editing. Use direct shell edits only for simple, obvious replacements.
When using structured patching, provide exactly one fenced bash block that invokes the available structured patch capability and no unrelated command.
The patch body must name repo-relative target paths, preserve source context exactly, distinguish removed and added lines, and contain only the requested source change.
If structured patching fails, reread the target range and retry with a smaller patch. If structured patching is unavailable, use a clear shell edit instead.

`
}

func renderPromptAttachment(attachment string) string {
	switch attachment {
	case "source_edit_transport":
		return renderSourceEditTransport()
	default:
		return ""
	}
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

func outputArtifactPath(state State, id string, artifactRoot string) (string, bool) {
	for _, artifact := range state.Artifacts.Outputs {
		if artifact.ID == id {
			return resolveArtifactPath(artifact.Path, artifactRoot), true
		}
	}
	return "", false
}

func commandEvidenceConfig(state State, artifactRoot string) (query.PragmaLoopCommandEvidenceConfig, bool) {
	var cfg query.PragmaLoopCommandEvidenceConfig
	for _, artifact := range state.Artifacts.Outputs {
		if artifact.RuntimeCapture.Type != "command_evidence" {
			continue
		}
		cfg.Path = resolveArtifactPath(artifact.Path, artifactRoot)
		break
	}
	if strings.TrimSpace(cfg.Path) == "" {
		return cfg, false
	}
	for _, artifact := range state.Artifacts.Outputs {
		for _, check := range artifact.Checks {
			if check.Type != "command_evidence_support" {
				continue
			}
			evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
			if !ok || evidencePath != cfg.Path {
				continue
			}
			cfg.ReportPaths = append(cfg.ReportPaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
	}
	return cfg, true
}

func commandPolicyConfig(state State, transitionHandoff []Artifact, artifactRoot string) (query.PragmaLoopCommandPolicyConfig, bool) {
	if state.ShellPolicy.IsZero() {
		return query.PragmaLoopCommandPolicyConfig{}, false
	}
	cfg := query.PragmaLoopCommandPolicyConfig{
		DenyPatterns: append([]string(nil), state.ShellPolicy.DenyPatterns...),
		DenyMessage:  state.ShellPolicy.DenyMessage,
	}
	if strings.TrimSpace(state.ShellPolicy.HandoffInputs) == "rendered" {
		for _, artifact := range transitionHandoff {
			cfg.RenderedInputPaths = append(cfg.RenderedInputPaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
		for _, artifact := range state.Artifacts.Outputs {
			cfg.WritablePaths = append(cfg.WritablePaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
	}
	if len(cfg.DenyPatterns) == 0 && len(cfg.RenderedInputPaths) == 0 {
		return query.PragmaLoopCommandPolicyConfig{}, false
	}
	return cfg, true
}

func handoffSnapshots(reads []HandoffRead) map[string][]byte {
	if len(reads) == 0 {
		return nil
	}
	out := make(map[string][]byte, len(reads))
	for _, read := range reads {
		if read.ArtifactID == "" || len(read.Content) == 0 {
			continue
		}
		out[read.ArtifactID] = append([]byte(nil), read.Content...)
	}
	return out
}

func requiredOutputArtifactCompletionCheck(state State, artifactRoot string, taskPrompts ...string) query.PragmaLoopCompletionCheck {
	taskPrompt := ""
	if len(taskPrompts) > 0 {
		taskPrompt = taskPrompts[0]
	}
	return requiredOutputArtifactCompletionCheckWithSnapshots(state, artifactRoot, taskPrompt, nil)
}

func requiredOutputArtifactCompletionCheckWithSnapshots(state State, artifactRoot string, taskPrompt string, snapshots map[string][]byte) query.PragmaLoopCompletionCheck {
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
		var invalid []string
		for _, artifact := range required {
			path := resolveArtifactPath(artifact.Path, artifactRoot)
			for _, check := range artifact.Checks {
				issues, err := validateArtifactIntegrityCheck(artifact, check, state, path, artifactRoot, taskPrompt, snapshots)
				if err != nil {
					invalid = append(invalid, fmt.Sprintf("- `%s`: `%s` (%v)", artifact.ID, path, err))
					continue
				}
				invalid = append(invalid, issues...)
			}
		}
		if len(missing) == 0 && len(invalid) == 0 {
			return true, "", nil
		}
		var sections []string
		if len(missing) > 0 {
			sections = append(sections, fmt.Sprintf("Required output artifact(s) are missing:\n%s", strings.Join(missing, "\n")))
		}
		if len(invalid) > 0 {
			sections = append(sections, fmt.Sprintf("Required output artifact(s) failed integrity checks:\n%s", strings.Join(invalid, "\n")))
		}
		return false, fmt.Sprintf("Completion was rejected because:\n%s\n\nRun another fenced bash block that fixes the output artifact(s), then echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again.", strings.Join(sections, "\n\n")), nil
	}
}

func validateArtifactIntegrityCheck(artifact Artifact, check ArtifactIntegrityCheck, state State, artifactPath string, artifactRoot string, taskPrompt string, snapshots map[string][]byte) ([]string, error) {
	switch check.Type {
	case "json_field_equals":
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			return nil, err
		}
		value, ok := doc[check.Field]
		if !ok || fmt.Sprint(value) != check.Value {
			return []string{fmt.Sprintf("- `%s` field %q is %q, want %q", artifact.ID, check.Field, fmt.Sprint(value), check.Value)}, nil
		}
	case "json_array_non_empty":
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return []string{fmt.Sprintf("- `%s` array %q is empty", artifact.ID, check.Path)}, nil
		}
	case "json_each_required_fields":
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			label := artifactItemLabel(item, idx)
			for _, field := range check.Fields {
				if strings.TrimSpace(jsonStringField(item, field)) == "" {
					issues = append(issues, fmt.Sprintf("- `%s` %s has empty %s", artifact.ID, label, field))
				}
			}
		}
		return issues, nil
	case "json_each_string_substring_of_task_prompt":
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			value := strings.TrimSpace(jsonStringField(item, check.Field))
			if value == "" {
				continue
			}
			if strings.TrimSpace(taskPrompt) != "" && !strings.Contains(taskPrompt, value) {
				issues = append(issues, fmt.Sprintf("- `%s` %s %s is not an exact substring of the original task prompt: %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
			}
		}
		return issues, nil
	case "json_each_repo_relative_paths":
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			for _, value := range jsonStringArrayField(item, check.Field) {
				if invalidRepoRelativePath(value) {
					issues = append(issues, fmt.Sprintf("- `%s` %s %s contains non-repo-relative path %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
				}
			}
		}
		return issues, nil
	case "json_each_behavior_validation_commands":
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			for _, value := range jsonStringArrayField(item, check.Field) {
				if placeholderValidationCommand(value) {
					issues = append(issues, fmt.Sprintf("- `%s` %s %s uses placeholder validation instead of a concrete behavior command: %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
				}
				if invalidValidationPath(value) {
					issues = append(issues, fmt.Sprintf("- `%s` %s %s uses an absolute repository path instead of a repo-relative path: %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
				}
			}
		}
		return issues, nil
	case "json_each_fields_equal_handoff_artifact":
		currentItems, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			return nil, err
		}
		referenceRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(referenceRaw) == 0 {
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		referenceItems, err := readJSONArrayFieldBytes(referenceRaw, check.Path)
		if err != nil {
			return nil, err
		}
		referenceByKey := make(map[string]map[string]interface{}, len(referenceItems))
		for idx, item := range referenceItems {
			key := strings.TrimSpace(jsonStringField(item, check.KeyField))
			if key == "" {
				return []string{fmt.Sprintf("- `%s` rendered handoff %s has empty key field %q", artifact.ID, artifactItemLabel(item, idx), check.KeyField)}, nil
			}
			referenceByKey[key] = item
		}
		var issues []string
		for idx, item := range currentItems {
			key := strings.TrimSpace(jsonStringField(item, check.KeyField))
			if key == "" {
				issues = append(issues, fmt.Sprintf("- `%s` %s has empty key field %q", artifact.ID, artifactItemLabel(item, idx), check.KeyField))
				continue
			}
			reference, ok := referenceByKey[key]
			if !ok {
				issues = append(issues, fmt.Sprintf("- `%s` %s has no matching rendered handoff item by %q", artifact.ID, artifactItemLabel(item, idx), check.KeyField))
				continue
			}
			for _, field := range check.Fields {
				if !reflect.DeepEqual(item[field], reference[field]) {
					issues = append(issues, fmt.Sprintf("- `%s` %s field %q changed from rendered handoff value", artifact.ID, artifactItemLabel(item, idx), field))
				}
			}
		}
		return issues, nil
	case "json_fields_equal_handoff_artifact":
		currentDoc, err := readJSONObject(artifactPath)
		if err != nil {
			return nil, err
		}
		referenceRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(referenceRaw) == 0 {
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		referenceDoc, err := readJSONObjectBytes(referenceRaw)
		if err != nil {
			return nil, err
		}
		var issues []string
		for _, field := range check.Fields {
			currentValue, currentOK := jsonValueAtPath(currentDoc, field)
			referenceValue, referenceOK := jsonValueAtPath(referenceDoc, field)
			switch {
			case !currentOK && !referenceOK:
				continue
			case !currentOK:
				issues = append(issues, fmt.Sprintf("- `%s` missing field %q from rendered handoff artifact %q", artifact.ID, field, check.ArtifactID))
			case !referenceOK:
				issues = append(issues, fmt.Sprintf("- `%s` field %q has no rendered handoff value in artifact %q", artifact.ID, field, check.ArtifactID))
			case !reflect.DeepEqual(currentValue, referenceValue):
				issues = append(issues, fmt.Sprintf("- `%s` field %q changed from rendered handoff artifact %q", artifact.ID, field, check.ArtifactID))
			}
		}
		return issues, nil
	case "json_array_subset_of_handoff_text_list":
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			return nil, err
		}
		values, err := jsonStringArrayPath(doc, check.Field)
		if err != nil {
			return nil, err
		}
		referenceRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(referenceRaw) == 0 {
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		allowed, ok := textListField(referenceRaw, check.TextField)
		if !ok {
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff text field %q in artifact %q", artifact.ID, check.TextField, check.ArtifactID)}, nil
		}
		allowedSet := make(map[string]struct{}, len(allowed))
		for _, value := range allowed {
			allowedSet[value] = struct{}{}
		}
		var issues []string
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := allowedSet[value]; !ok {
				issues = append(issues, fmt.Sprintf("- `%s` field %q value %q is not present in rendered handoff artifact %q text field %q", artifact.ID, check.Field, value, check.ArtifactID, check.TextField))
			}
		}
		return issues, nil
	case "markdown_constraints_supported_by_claims":
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			return nil, err
		}
		return validateMarkdownConstraintsSupportedByClaims(artifact, data), nil
	case "text_forbid_contains":
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			return nil, err
		}
		if strings.Contains(strings.ToLower(string(data)), strings.ToLower(check.Value)) {
			return []string{fmt.Sprintf("- `%s` contains forbidden text %q", artifact.ID, check.Value)}, nil
		}
	case "command_evidence_support":
		evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
		if !ok {
			return []string{fmt.Sprintf("- `%s` cannot find declared evidence artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateCommandEvidenceSupport(artifact, artifactPath, check, evidencePath)
	}
	return nil, nil
}

func validateMarkdownConstraintsSupportedByClaims(artifact Artifact, data []byte) []string {
	claims := markdownClaimAdjudications(data)
	if len(claims) == 0 {
		return []string{fmt.Sprintf("- `%s` has no Claim adjudication rows", artifact.ID)}
	}
	constraints := markdownSectionBullets(data, "Planning Constraints")
	var issues []string
	for _, constraint := range constraints {
		if markdownNoneBullet(constraint) {
			continue
		}
		_, support, ok := strings.Cut(constraint, "support=")
		if !ok || strings.TrimSpace(support) == "" {
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint lacks support=: %q", artifact.ID, constraint))
			continue
		}
		support = normalizeMarkdownSupport(support)
		if support == "" || support == "none" {
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint has empty support: %q", artifact.ID, constraint))
			continue
		}
		claim, ok := supportMatchesClaim(support, claims)
		if !ok {
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint support %q does not match any Claim adjudication row", artifact.ID, support))
			continue
		}
		switch claim.Classification {
		case "observed", "inferred":
		default:
			if claim.Classification == "" {
				issues = append(issues, fmt.Sprintf("- `%s` planning constraint support %q matches a Claim adjudication row without classification", artifact.ID, support))
				continue
			}
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint support %q matches %s claim; only observed or inferred claims may support constraints", artifact.ID, support, claim.Classification))
		}
	}
	return issues
}

type markdownClaimAdjudication struct {
	Claim          string
	Classification string
}

func markdownClaimAdjudications(data []byte) []markdownClaimAdjudication {
	var claims []markdownClaimAdjudication
	for _, bullet := range markdownSectionBullets(data, "Claim Adjudication") {
		if markdownNoneBullet(bullet) {
			continue
		}
		fields := markdownSemicolonFields(bullet)
		claim := normalizeMarkdownSupport(fields["claim"])
		if claim != "" {
			claims = append(claims, markdownClaimAdjudication{
				Claim:          claim,
				Classification: strings.ToLower(normalizeMarkdownSupport(fields["classification"])),
			})
		}
	}
	return claims
}

func markdownSemicolonFields(text string) map[string]string {
	fields := make(map[string]string)
	for _, part := range strings.Split(text, ";") {
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(normalizeMarkdownSupport(key))
		if key == "" {
			continue
		}
		fields[key] = strings.TrimSpace(value)
	}
	return fields
}

func supportMatchesClaim(support string, claims []markdownClaimAdjudication) (markdownClaimAdjudication, bool) {
	support = normalizeMarkdownSupport(strings.TrimPrefix(support, "claim "))
	for _, claim := range claims {
		claimText := normalizeMarkdownSupport(claim.Claim)
		if support == claimText || strings.Contains(claimText, support) || strings.Contains(support, claimText) {
			return claim, true
		}
	}
	return markdownClaimAdjudication{}, false
}

func markdownSectionBullets(data []byte, section string) []string {
	var bullets []string
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")) == section
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}
	return bullets
}

func markdownNoneBullet(text string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(text), "`\"'")) {
	case "", "none", "n/a", "na", "not_applicable":
		return true
	default:
		return false
	}
}

func normalizeMarkdownSupport(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`\"'")
	if idx := strings.IndexAny(text, "\r\n;"); idx >= 0 {
		text = text[:idx]
	}
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func readJSONObject(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return readJSONObjectBytes(data)
}

func readJSONObjectBytes(data []byte) (map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func readJSONArrayField(path string, field string) ([]map[string]interface{}, error) {
	doc, err := readJSONObject(path)
	if err != nil {
		return nil, err
	}
	return jsonArrayField(doc, field)
}

func readJSONArrayFieldBytes(data []byte, field string) ([]map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return jsonArrayField(doc, field)
}

func jsonArrayField(doc map[string]interface{}, field string) ([]map[string]interface{}, error) {
	value, ok := doc[field]
	if !ok {
		return nil, fmt.Errorf("missing JSON array field %q", field)
	}
	rawItems, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("JSON field %q is not an array", field)
	}
	items := make([]map[string]interface{}, 0, len(rawItems))
	for idx, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("JSON field %q item %d is not an object", field, idx)
		}
		items = append(items, item)
	}
	return items, nil
}

func jsonStringArrayPath(doc map[string]interface{}, fieldPath string) ([]string, error) {
	value, ok := jsonValueAtPath(doc, fieldPath)
	if !ok {
		return nil, fmt.Errorf("missing JSON field %q", fieldPath)
	}
	switch typed := value.(type) {
	case []interface{}:
		out := make([]string, 0, len(typed))
		for idx, raw := range typed {
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("JSON field %q item %d is not a string", fieldPath, idx)
			}
			out = append(out, text)
		}
		return out, nil
	case []string:
		return append([]string(nil), typed...), nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, nil
		}
		return []string{typed}, nil
	default:
		return nil, fmt.Errorf("JSON field %q is not a string array", fieldPath)
	}
}

func jsonValueAtPath(doc map[string]interface{}, fieldPath string) (interface{}, bool) {
	current := interface{}(doc)
	for _, part := range strings.Split(fieldPath, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func textListField(data []byte, field string) ([]string, bool) {
	field = strings.TrimSpace(field)
	if field == "" {
		return nil, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != field {
			continue
		}
		return parseTextListValue(value), true
	}
	return nil, false
}

func parseTextListValue(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "none", "null", "nil", "n/a", "na", "not_applicable":
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "`\"'")
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func artifactItemLabel(item map[string]interface{}, idx int) string {
	id := jsonStringField(item, "id")
	if id == "" {
		return fmt.Sprintf("item[%d]", idx)
	}
	return id
}

func jsonStringField(item map[string]interface{}, field string) string {
	value, ok := item[field]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func jsonStringArrayField(item map[string]interface{}, field string) []string {
	value, ok := item[field]
	if !ok {
		return nil
	}
	rawItems, ok := value.([]interface{})
	if !ok {
		if text, ok := value.(string); ok {
			return []string{text}
		}
		return nil
	}
	out := make([]string, 0, len(rawItems))
	for _, raw := range rawItems {
		if text, ok := raw.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

type commandEvidenceRecord struct {
	CommandPreview     string `json:"command_preview"`
	CommandSHA256      string `json:"command_sha256"`
	OutputSHA256       string `json:"output_sha256"`
	ReturnCode         int    `json:"returncode"`
	TimedOut           bool   `json:"timed_out"`
	Submitted          bool   `json:"submitted"`
	CompletionSentinel bool   `json:"completion_sentinel"`
	WritesReport       bool   `json:"writes_report"`
}

func validateCommandEvidenceSupport(artifact Artifact, reportPath string, check ArtifactIntegrityCheck, evidencePath string) ([]string, error) {
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, err
	}
	evidenceData, err := os.ReadFile(evidencePath)
	if err != nil {
		return []string{fmt.Sprintf("- `%s` has no runtime command evidence artifact at `%s`: %v", artifact.ID, evidencePath, err)}, nil
	}
	report := string(reportData)
	records, parseIssues := parseCommandEvidence(evidenceData, check.ArtifactID)
	var issues []string
	issues = append(issues, parseIssues...)
	if len(records) == 0 {
		issues = append(issues, fmt.Sprintf("- `%s` has an empty runtime command evidence artifact", artifact.ID))
		return issues, nil
	}
	var priorEvidence int
	var validationEvidence int
	for _, record := range records {
		if !record.CompletionSentinel && !record.WritesReport {
			priorEvidence++
		}
		if !record.WritesReport && strings.TrimSpace(record.OutputSHA256) != "" {
			validationEvidence++
		}
	}
	if priorEvidence == 0 {
		issues = append(issues, fmt.Sprintf("- `%s` was submitted without any prior non-completion command evidence in this state", artifact.ID))
	}
	for _, section := range check.Sections {
		if sectionHasNonNone(report, section) && priorEvidence == 0 {
			issues = append(issues, fmt.Sprintf("- `%s` section %q is non-empty, but runtime evidence has no prior non-completion command", artifact.ID, section))
		}
		if strings.Contains(strings.ToLower(section), "validation") && sectionHasNonNone(report, section) && validationEvidence == 0 {
			issues = append(issues, fmt.Sprintf("- `%s` section %q is non-empty, but runtime evidence has no command output hash", artifact.ID, section))
		}
	}
	return issues, nil
}

func parseCommandEvidence(data []byte, label string) ([]commandEvidenceRecord, []string) {
	var records []commandEvidenceRecord
	var issues []string
	lines := strings.Split(string(data), "\n")
	for idx, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record commandEvidenceRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			issues = append(issues, fmt.Sprintf("- %s line %d is invalid JSON: %v", label, idx+1, err))
			continue
		}
		if record.CommandSHA256 == "" || record.OutputSHA256 == "" {
			issues = append(issues, fmt.Sprintf("- %s line %d is missing command/output hash", label, idx+1))
		}
		records = append(records, record)
	}
	return records, issues
}

func sectionHasNonNone(text string, heading string) bool {
	idx := strings.Index(text, heading)
	if idx < 0 {
		return false
	}
	section := text[idx+len(heading):]
	if next := strings.Index(section, "\n\n"); next >= 0 {
		section = section[:next]
	}
	section = strings.TrimSpace(section)
	if section == "" {
		return false
	}
	lower := strings.ToLower(section)
	return strings.Contains(section, "-") && !strings.Contains(lower, "- none") && lower != "none"
}

func invalidRepoRelativePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "unknown" {
		return false
	}
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~")
}

func invalidValidationPath(validation string) bool {
	fields := strings.Fields(validation)
	for _, field := range fields {
		field = strings.Trim(field, "`'\",;)")
		if strings.HasPrefix(field, "/tmp/pragma/") {
			continue
		}
		if filepath.IsAbs(field) || strings.HasPrefix(field, "~") {
			return true
		}
	}
	return false
}

func placeholderValidationCommand(validation string) bool {
	validation = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(validation), "command:")))
	validation = strings.Trim(validation, "`'\".; ")
	switch validation {
	case "", "unknown", "n/a", "na", "none", "todo", "tbd", "manual":
		return true
	default:
		return false
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
		switch control.HandoffMode {
		case "persona":
			fmt.Fprintf(&b, "Its `%s` persona will write the selected-item handoff to `%s` after selection.\n", next.Persona, control.HandoffPath)
			b.WriteString("Do not write a separate generic next-item handoff; the next state's persona owns that handoff after the cursor is selected.\n\n")
		case "control":
			fmt.Fprintf(&b, "The control state will write the selected-item handoff to `%s` after selection.\n", control.HandoffPath)
			b.WriteString("Do not write a separate generic next-item handoff; the control state owns that handoff after the cursor is selected.\n\n")
		}
	}
	fmt.Fprintf(&b, "Write `%s` as JSON with this exact shape:\n\n", control.ListPath)
	itemContract := strings.TrimSpace(control.ItemContract)
	if itemContract != "" {
		b.WriteString(itemContract)
		if !strings.HasSuffix(itemContract, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("```json\n")
		b.WriteString("{\n")
		b.WriteString("  \"items\": [\n")
		b.WriteString("    {\n")
		b.WriteString("      \"id\": \"stable-string-id\",\n")
		fmt.Fprintf(&b, "      \"status\": %q\n", pendingStatus)
		b.WriteString("    }\n")
		b.WriteString("  ]\n")
		b.WriteString("}\n")
		b.WriteString("```\n\n")
	}
	b.WriteString("Rules:\n")
	b.WriteString("- `id` is required and must be a JSON string, never a number.\n")
	fmt.Fprintf(&b, "- `status` is required and must be `%q`, `%q`, or `%q`.\n", pendingStatus, blockedStatus, doneStatus)
	fmt.Fprintf(&b, "- Use `%s` only for items runnable immediately by the next worker.\n", pendingStatus)
	fmt.Fprintf(&b, "- When no `%s` items remain and no `%s` items remain, `%s` will write a cursor item with status `%s`.\n", pendingStatus, blockedStatus, next.ID, doneStatus)
	if control.BlockedEvent != "" {
		fmt.Fprintf(&b, "- When no `%s` items remain but `%s` items remain, `%s` emits `%s`.\n", pendingStatus, blockedStatus, next.ID, control.BlockedEvent)
	}
	if strings.TrimSpace(control.Dependency.DependencyIDsField) != "" {
		fmt.Fprintf(&b, "- Use `%s` with `%s` for unfinished items that depend on another listed item before they can run.\n", blockedStatus, control.Dependency.DependencyIDsField)
	}
	if control.HandoffPath != "" {
		observe.GlobalTrace("if: control.HandoffPath != \"\"")
		switch control.HandoffMode {
		case "persona":
			fmt.Fprintf(&b, "- `%s` will select the item and its `%s` persona will write the handoff for that exact selected item, not for future checklist items.\n", next.ID, next.Persona)
		case "control":
			fmt.Fprintf(&b, "- `%s` will select the item and write the handoff for that exact selected item, not for future checklist items.\n", next.ID)
		}
	}
	if strings.TrimSpace(control.Dependency.DeferredDependencyField) != "" && strings.TrimSpace(control.Dependency.DependencyIDsField) != "" {
		fmt.Fprintf(&b, "- If `%s` names another listed item id, set `%s` to that id and use blocked status instead of pending.\n", control.Dependency.DeferredDependencyField, control.Dependency.DependencyIDsField)
	}
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
			Bytes:       e.Bytes,
			SHA256:      e.SHA256,
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

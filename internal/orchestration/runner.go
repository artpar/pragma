package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/llmconfig"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/persona"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/query"
)

const DefaultArtifactRoot = "/tmp/pragma"
const ArtifactSeedSourceProcessEnvironment = "process_environment"

type RunOptions struct {
	PersonaDir     string
	TaskPrompt     string
	ArtifactRoot   string
	SeedArtifacts  map[string]string
	LLMResolver    LLMResolver
	StartAtState   string
	StopAfterState string
}

type LLMResolver func(context.Context, llmconfig.Config) (LLMRuntime, error)

type LLMRuntime struct {
	Provider           provider.Provider
	ProviderName       string
	Model              string
	MaxTokens          int
	Temperature        *float64
	Thinking           *provider.ThinkingConfig
	CustomSystemPrompt string
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
	startAtState := strings.TrimSpace(opts.StartAtState)
	if startAtState != "" {
		if _, ok := runtime.States[startAtState]; !ok {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: _, ok := runtime.States[startAtState]; !ok")
			ch <- query.ErrorEvent{Err: fmt.Errorf("start state %q not found in orchestration %q", startAtState, def.Name)}
			return
		}
		runtime.FSM.SetState(startAtState)
	}
	stopAfterState := strings.TrimSpace(opts.StopAfterState)
	if stopAfterState != "" {
		if _, ok := runtime.States[stopAfterState]; !ok {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: _, ok := runtime.States[stopAfterState]; !ok")
			ch <- query.ErrorEvent{Err: fmt.Errorf("stop-after state %q not found in orchestration %q", stopAfterState, def.Name)}
			return
		}
	}
	seedWrites, err := materializeSeedArtifacts(def, artifactRoot, opts)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
		ch <- query.ErrorEvent{Err: err}
		return
	}
	for _, write := range seedWrites {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "range seedWrites")
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
	if startAtState != "" {
		transitionHandoff, transitionFrom, transitionEvent = startAtTransitionContext(def, startAtState, artifactRoot)
	}
	var previousStateOutputs *StateOutputRun
	for !runtime.States[runtime.FSM.Current()].Terminal {
		observe.TraceCtx(ctx, "orchestration", "runEvents", "for: !runtime.States[runtime.FSM.Current()].Terminal")
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		outputsBefore, err := snapshotStateOutputs(state, artifactRoot)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: fmt.Errorf("snapshot outputs before state %q: %w", stateID, err)}
			return
		}
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
				observe.TraceCtx(ctx, "orchestration", "runEvents", "if: state.TaskPrompt == TaskPromptFull")
				stateTaskPrompt = originalTaskPrompt
			} else {
				observe.TraceCtx(ctx, "orchestration", "runEvents", "else: state.TaskPrompt == TaskPromptFull")
				stateTaskPrompt = taskPrompt
				taskPrompt = ""
			}
		}
		controlCtx := ControlExecutionContext{
			ArtifactRoot:         artifactRoot,
			PreviousStateOutputs: previousStateOutputs,
		}
		event, _, err := runNodeEventsWithControlContext(ctx, ch, stateEngine, projection, opts.PersonaDir, def, state, stateTaskPrompt, "", originalTaskPrompt, transitionHandoff, transitionFrom, transitionEvent, controlCtx, opts.LLMResolver, outputsBefore, artifactRoot)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: err}
			return
		}
		outputsAfter, err := snapshotStateOutputs(state, artifactRoot)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: fmt.Errorf("snapshot outputs after state %q: %w", stateID, err)}
			return
		}
		if err := validateFreshRequiredModelOutputs(state, artifactRoot, outputsBefore, outputsAfter); err != nil {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: err != nil")
			ch <- query.ErrorEvent{Err: err}
			return
		}
		previousStateOutputs = newStateOutputRun(state, artifactRoot, outputsBefore, outputsAfter)
		if stopAfterState != "" && stateID == stopAfterState {
			observe.TraceCtx(ctx, "orchestration", "runEvents", "if: strings.TrimSpace(opts.StopAfterState) != \"\" && stateID == strings.TrimSpace(opts.StopAfterState)")
			ch <- query.TurnCompleteEvent{
				Response:   model.Response{StopReason: model.StopEndTurn},
				StopReason: model.StopEndTurn,
			}
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

func startAtTransitionContext(def Definition, startAtState string, artifactRoot string) ([]Artifact, string, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var matches []Transition
	for _, transition := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		if transition.To == startAtState && len(transition.Handoff) > 0 {
			observe.GlobalTrace("if: transition.To == startAtState && len(transition.Handoff) > 0")
			matches = append(matches, transition)
		}
	}
	if len(matches) == 0 {
		observe.GlobalTrace("if: len(matches) == 0")
		return nil, "", ""
	}
	if len(matches) > 1 {
		observe.GlobalTrace("if: len(matches) > 1")
		matches = startAtSatisfiedHandoffTransitions(matches, artifactRoot)
	}
	if len(matches) != 1 {
		observe.GlobalTrace("if: len(matches) != 1")
		return nil, "", ""
	}
	transition := matches[0]
	from := ""
	if len(transition.From) == 1 {
		observe.GlobalTrace("if: len(transition.From) == 1")
		from = transition.From[0]
	}
	observe.GlobalTrace("return: append([]Artifact(nil), transition.Handoff...), from, transition.Event")
	return append([]Artifact(nil), transition.Handoff...), from, transition.Event
}

func startAtSatisfiedHandoffTransitions(transitions []Transition, artifactRoot string) []Transition {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []Transition
	for _, transition := range transitions {
		observe.GlobalTrace("range transitions")
		if requiredHandoffArtifactsExist(transition.Handoff, artifactRoot) {
			observe.GlobalTrace("if: requiredHandoffArtifactsExist")
			out = append(out, transition)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

func requiredHandoffArtifactsExist(artifacts []Artifact, artifactRoot string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, artifact := range artifacts {
		observe.GlobalTrace("range artifacts")
		if !artifact.Required {
			observe.GlobalTrace("if: !artifact.Required")
			continue
		}
		if _, err := os.Stat(resolveArtifactPath(artifact.Path, artifactRoot)); err != nil {
			observe.GlobalTrace("if: os.Stat err")
			return false
		}
	}
	observe.GlobalTrace("return: true")
	return true
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var writes []seededArtifactWrite
	seen := make(map[string]bool)
	usedSeedSources := make(map[string]bool)
	for _, artifact := range seededArtifacts(def) {
		observe.GlobalTrace("range seededArtifacts(def)")
		source := strings.TrimSpace(artifact.Seed.Source)
		if source == "" {
			observe.GlobalTrace("if: source == \"\"")
			continue
		}
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		if seen[path] {
			observe.GlobalTrace("if: seen[path]")
			continue
		}
		content, ok, err := seedArtifactContent(source, opts)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"seed artifact %q from %q: %w\", artifact.ID, source, err)")
			return nil, fmt.Errorf("seed artifact %q from %q: %w", artifact.ID, source, err)
		}
		if !ok {
			observe.GlobalTrace("if: !ok")
			if artifact.Required {
				observe.GlobalTrace("if: artifact.Required")
				observe.GlobalTrace("return: nil, fmt.Errorf(\"seed artifact %q requires unavailable source %q\", artifact.I...")
				return nil, fmt.Errorf("seed artifact %q requires unavailable source %q", artifact.ID, source)
			}
			continue
		}
		usedSeedSources[source] = true
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create seed artifact dir for %q at %q: %w", artifact.ID, path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"write seed artifact %q at %q: %w\", artifact.ID, path, err)")
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
	for source, content := range opts.SeedArtifacts {
		if usedSeedSources[source] {
			continue
		}
		matches := artifactsMatchingSeedTarget(def, artifactRoot, source)
		if len(matches) == 0 {
			return nil, fmt.Errorf("seed artifact target %q does not match a declared seed source, artifact id, or artifact path", source)
		}
		for _, artifact := range matches {
			path := resolveArtifactPath(artifact.Path, artifactRoot)
			if seen[path] {
				continue
			}
			data := []byte(content)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("create seed artifact dir for %q at %q: %w", artifact.ID, path, err)
			}
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return nil, fmt.Errorf("write seed artifact %q at %q: %w", artifact.ID, path, err)
			}
			seen[path] = true
			writes = append(writes, seededArtifactWrite{
				ArtifactID: artifact.ID,
				Path:       path,
				Bytes:      int64(len(data)),
				SHA256:     sha256Hex(data),
			})
		}
	}
	observe.GlobalTrace("return: writes, nil")
	return writes, nil
}

func artifactsMatchingSeedTarget(def Definition, artifactRoot string, target string) []Artifact {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	var matches []Artifact
	seenPaths := make(map[string]bool)
	for _, artifact := range allArtifacts(def) {
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		if artifact.ID != target && artifact.Path != target && path != target {
			continue
		}
		if seenPaths[path] {
			continue
		}
		seenPaths[path] = true
		matches = append(matches, artifact)
	}
	observe.GlobalTrace("return: matches")
	return matches
}

func allArtifacts(def Definition) []Artifact {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var artifacts []Artifact
	for _, state := range def.States {
		observe.GlobalTrace("range def.States")
		artifacts = append(artifacts, state.Artifacts.Inputs...)
		artifacts = append(artifacts, state.Artifacts.Outputs...)
	}
	for _, transition := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		artifacts = append(artifacts, transition.Handoff...)
	}
	observe.GlobalTrace("return: artifacts")
	return artifacts
}

func seededArtifacts(def Definition) []Artifact {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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
		observe.GlobalTrace("range def.States")
		appendSeeded(state.Artifacts.Inputs)
		appendSeeded(state.Artifacts.Outputs)
	}
	for _, transition := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		appendSeeded(transition.Handoff)
	}
	observe.GlobalTrace("return: artifacts")
	return artifacts
}

func seedArtifactContent(source string, opts RunOptions) ([]byte, bool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if opts.SeedArtifacts != nil {
		observe.GlobalTrace("if: opts.SeedArtifacts != nil")
		if content, ok := opts.SeedArtifacts[source]; ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: []byte(content), true, nil")
			return []byte(content), true, nil
		}
	}
	switch source {
	case ArtifactSeedSourceProcessEnvironment:
		observe.GlobalTrace("case: ArtifactSeedSourceProcessEnvironment")
		return []byte(renderProcessEnvironmentSeed()), true, nil
	default:
		observe.GlobalTrace("default")
		return nil, false, nil
	}
}

func renderProcessEnvironmentSeed() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	fmt.Fprintf(&b, "# Runner Capability Evidence\n\n")
	fmt.Fprintf(&b, "source: %s\n", ArtifactSeedSourceProcessEnvironment)
	fmt.Fprintf(&b, "captured_at: %s\n", time.Now().UTC().Format(time.RFC3339))
	if cwd, err := os.Getwd(); err == nil && strings.TrimSpace(cwd) != "" {
		observe.GlobalTrace("if: err == nil && strings.TrimSpace(cwd) != \"\"")
		fmt.Fprintf(&b, "cwd: %s\n", cwd)
	}
	fmt.Fprintf(&b, "os: %s\n", goruntime.GOOS)
	fmt.Fprintf(&b, "arch: %s\n", goruntime.GOARCH)
	pathValue := os.Getenv("PATH")
	fmt.Fprintf(&b, "path: %s\n", pathValue)
	fmt.Fprintf(&b, "\n## Path Entries\n")
	for _, entry := range filepath.SplitList(pathValue) {
		observe.GlobalTrace("range filepath.SplitList(pathValue)")
		if strings.TrimSpace(entry) == "" {
			observe.GlobalTrace("if: strings.TrimSpace(entry) == \"\"")
			continue
		}
		fmt.Fprintf(&b, "- %s\n", entry)
	}
	keys := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		observe.GlobalTrace("range os.Environ()")
		key, _, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			observe.GlobalTrace("if: !ok || strings.TrimSpace(key) == \"\"")
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fmt.Fprintf(&b, "\n## Environment Variables Present\n")
	for _, key := range keys {
		observe.GlobalTrace("range keys")
		fmt.Fprintf(&b, "- %s\n", key)
	}
	fmt.Fprintf(&b, "\nNo tool commands were probed by this seed. The receiving state must run bounded local probes before drawing tool-availability conclusions.\n")
	observe.GlobalTrace("return: b.String()")
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
	observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: runNodeEventsWithControlContext(ctx, ch, engine, projection, personaDir, def,...")
	return runNodeEventsWithControlContext(ctx, ch, engine, projection, personaDir, def, state, taskPrompt, handoffPrompt, originalTaskPrompt, transitionHandoff, transitionFrom, transitionEvent, ControlExecutionContext{}, nil, nil, artifactRoots...)
}

func runNodeEventsWithControlContext(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, originalTaskPrompt string, transitionHandoff []Artifact, transitionFrom string, transitionEvent string, controlCtx ControlExecutionContext, llmResolver LLMResolver, outputSnapshotsBefore map[string]ArtifactFileSnapshot, artifactRoots ...string) (string, string, error) {
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
		if strings.TrimSpace(controlCtx.ArtifactRoot) == "" {
			observe.TraceCtx(ctx, "orchestration", "runNodeEventsWithControlContext", "if: strings.TrimSpace(controlCtx.ArtifactRoot) == \"\"")
			controlCtx.ArtifactRoot = artifactRoot
		}
		event, err := ExecuteControlWithContext(state, controlCtx)
		if err != nil {
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: err != nil")
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "return: \"\", \"\", fmt.Errorf(\"control state %q failed: %w\", state.ID, err)")
			return "", "", fmt.Errorf("control state %q failed: %w", state.ID, err)
		}
		emitOrchestration(ch, bus, projection, query.OrchestrationControlEvent{StateID: state.ID, Control: control, Event: event})
		if state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffMode == "persona" && event == state.Control.ForEachNext.ItemEvent {
			observe.TraceCtx(ctx, "orchestration", "runNodeEvents", "if: state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffMode == \"persona\" && event == state.Control.ForEachNext.ItemEvent")
			controlHandoff := controlPersonaHandoffArtifacts(state, transitionHandoff)
			if err := runPersonaForState(ctx, ch, bus, projection, engine, personaDir, def, state, "", handoffPrompt, originalTaskPrompt, controlHandoff, transitionFrom, transitionEvent, artifactRoot, llmResolver, outputSnapshotsBefore); err != nil {
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
			observe.TraceCtx(ctx, "orchestration", "runNodeEventsWithControlContext", "if: state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffMode == ...")
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

	if err := runPersonaForState(ctx, ch, bus, projection, engine, personaDir, def, state, taskPrompt, handoffPrompt, originalTaskPrompt, transitionHandoff, transitionFrom, transitionEvent, artifactRoot, llmResolver, outputSnapshotsBefore); err != nil {
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

func runPersonaForState(ctx context.Context, ch chan<- query.LoopEvent, bus *observe.EventBus, projection *Projection, engine *query.Engine, personaDir string, def Definition, state State, taskPrompt string, handoffPrompt string, originalTaskPrompt string, transitionHandoff []Artifact, transitionFrom string, transitionEvent string, artifactRoot string, llmResolver LLMResolver, outputSnapshotsBefore map[string]ArtifactFileSnapshot) error {
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

	if err := applyStateLLMRuntime(ctx, engine, llmResolver, state, personaDef); err != nil {
		observe.TraceCtx(ctx, "orchestration", "runNodeEventsWithControlContext", "if: err != nil")
		return err
	}
	_, err = RunStateEvents(ctx, ch, engine, projection, def, state, personaDef, taskPrompt, handoffPrompt, originalTaskPrompt, transitionHandoff, readEvents, artifactRoot, outputSnapshotsBefore)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "return: err")
		return err
	}

	observe.TraceCtx(ctx, "orchestration", "runPersonaForState", "return: nil")
	return nil
}

func applyStateLLMRuntime(ctx context.Context, engine *query.Engine, resolver LLMResolver, state State, personaDef persona.Definition) error {
	observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "exit")
	llm := llmconfig.Merge(personaDef.LLM, state.LLM)
	if llm.IsZero() {
		observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "if: llm.IsZero()")
		return nil
	}
	if resolver == nil {
		observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "if: resolver == nil")
		return fmt.Errorf("state %q/persona %q defines llm but orchestration has no llm resolver", state.ID, personaDef.ID)
	}
	runtime, err := resolver(ctx, llm)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "if: err != nil")
		return fmt.Errorf("resolve llm for state %q/persona %q: %w", state.ID, personaDef.ID, err)
	}
	if runtime.Provider == nil {
		observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "if: runtime.Provider == nil")
		return fmt.Errorf("resolve llm for state %q/persona %q: provider is nil", state.ID, personaDef.ID)
	}
	engine.BindProvider(runtime.Provider, runtime.ProviderName, runtime.Model)
	engine.ApplyRuntimeOptions(runtime.MaxTokens, runtime.Temperature, runtime.Thinking, runtime.CustomSystemPrompt)
	observe.TraceCtx(ctx, "orchestration", "applyStateLLMRuntime", "return: nil")
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

func RunStateEvents(ctx context.Context, ch chan<- query.LoopEvent, engine *query.Engine, projection *Projection, def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, originalTaskPrompt string, transitionHandoff []Artifact, handoffReads []HandoffRead, artifactRoot string, outputSnapshotsBefore map[string]ArtifactFileSnapshot) (string, error) {
	observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "enter")
	defer observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "exit")
	if strings.TrimSpace(artifactRoot) == "" {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: strings.TrimSpace(artifactRoot) == \"\"")
		artifactRoot = DefaultArtifactRoot
	}
	system, prompt, err := BuildPromptWithArtifactRootChecked(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)
	if err != nil {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: err != nil")
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "return: \"\", err")
		return "", err
	}
	system = engine.WithCustomSystemPrompt(system)

	var text strings.Builder
	start := time.Now()
	completionCheck := requiredOutputArtifactCompletionCheckWithSnapshots(state, artifactRoot, originalTaskPrompt, handoffSnapshots(handoffReads), outputSnapshotsBefore)
	if cfg, ok := commandEvidenceConfig(state, artifactRoot); ok {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: ok")
		_ = os.Remove(cfg.Path)
		ctx = query.WithPragmaLoopCommandEvidence(ctx, cfg)
	}
	if cfg, ok := commandPolicyConfig(state, transitionHandoff, artifactRoot); ok {
		observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: ok")
		ctx = query.WithPragmaLoopCommandPolicy(ctx, cfg)
	}
	runOpts := query.PragmaLoopRunOptions{
		IncludePriorConversation: stateUsesPersistentConversation(state),
		MaxTurns:                 state.MaxTurns,
		FinalTextOnly:            stateCapturesFinalText(state),
	}
	if runOpts.FinalTextOnly {
		runOpts.FinalTextCheck = finalTextArtifactCompletionCheck(state)
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
			if err := captureFinalTextArtifacts(state, artifactRoot, text.String()); err != nil {
				observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: err != nil")
				return text.String(), err
			}
			emitOrchestration(ch, eventBus(engine), projection, query.OrchestrationStateCompletedEvent{StateID: state.ID, Duration: time.Since(start)})
			for _, artifact := range state.Artifacts.Outputs {
				path := resolveArtifactPath(artifact.Path, artifactRoot)
				bytes, sha, ok := artifactDigest(path)
				if !ok {
					observe.TraceCtx(ctx, "orchestration", "RunStateEvents", "if: !ok")
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

func captureFinalTextArtifacts(state State, artifactRoot string, text string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifact.RuntimeCapture.Type != "final_text" {
			observe.GlobalTrace("if: artifact.RuntimeCapture.Type != \"final_text\"")
			continue
		}
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			observe.GlobalTrace("if: err != nil")
			return fmt.Errorf("create final_text artifact dir for %q: %w", artifact.ID, err)
		}
		content := normalizeFinalTextArtifactContent(text, artifact.AllowedValues)
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			observe.GlobalTrace("if: err != nil")
			return fmt.Errorf("write final_text artifact %q at %q: %w", artifact.ID, path, err)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func finalTextArtifactCompletionCheck(state State) query.PragmaLoopFinalTextCheck {
	return func(text string) (bool, string, error) {
		for _, artifact := range state.Artifacts.Outputs {
			if artifact.RuntimeCapture.Type != "final_text" {
				continue
			}
			content := normalizeFinalTextArtifactContent(text, artifact.AllowedValues)
			if strings.TrimSpace(content) == "" {
				return false, fmt.Sprintf("Final text for artifact `%s` was empty. Return only the required final text value.", artifact.ID), nil
			}
			if len(artifact.AllowedValues) == 0 {
				continue
			}
			for _, allowed := range artifact.AllowedValues {
				if content == strings.TrimSpace(allowed) {
					return true, "", nil
				}
			}
			return false, fmt.Sprintf("Final text for artifact `%s` must be exactly one of: %s. Return only one allowed value, with no explanation or reasoning text.", artifact.ID, strings.Join(trimmedNonEmptyStrings(artifact.AllowedValues), ", ")), nil
		}
		return true, "", nil
	}
}

func trimmedNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func normalizeFinalTextArtifactContent(text string, allowedValues []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text = stripExplicitThinkBlocks(text)
	trimmed := strings.TrimSpace(text)
	for _, allowed := range allowedValues {
		observe.GlobalTrace("range allowedValues")
		if trimmed == strings.TrimSpace(allowed) {
			observe.GlobalTrace("return: strings.TrimSpace(allowed)")
			return strings.TrimSpace(allowed)
		}
	}
	if len(allowedValues) > 0 {
		observe.GlobalTrace("if: len(allowedValues) > 0")
		lines := strings.Split(trimmed, "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			observe.GlobalTrace("for: i := len(lines) - 1; i >= 0; i--")
			line := strings.TrimSpace(lines[i])
			if line == "" {
				observe.GlobalTrace("if: line == \"\"")
				continue
			}
			for _, allowed := range allowedValues {
				observe.GlobalTrace("range allowedValues")
				if line == strings.TrimSpace(allowed) {
					observe.GlobalTrace("return: strings.TrimSpace(allowed)")
					return strings.TrimSpace(allowed)
				}
			}
		}
	}
	observe.GlobalTrace("return: trimmed")
	return trimmed
}

func stripExplicitThinkBlocks(text string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for {
		observe.GlobalTrace("for: true")
		lower := strings.ToLower(text)
		start := strings.Index(lower, "<think>")
		if start < 0 {
			observe.GlobalTrace("return: strings.TrimSpace(text)")
			return strings.TrimSpace(text)
		}
		end := strings.Index(lower[start+len("<think>"):], "</think>")
		if end < 0 {
			observe.GlobalTrace("return: strings.TrimSpace(text[:start])")
			return strings.TrimSpace(text[:start])
		}
		end += start + len("<think>") + len("</think>")
		text = text[:start] + text[end:]
	}
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
	systemText := strings.TrimRight(personaDef.Prompt, "\n")
	if !stateCapturesFinalText(state) {
		systemText += "\n\n" + query.PragmaLoopSystemPrompt()
	}
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
		observe.GlobalTrace("if: gate != \"\"")
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			observe.GlobalTrace("if: b.Len() > 0 && !strings.HasSuffix(b.String(), \"\\n\\n\")")
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
	if stateCapturesFinalText(state) {
		observe.GlobalTrace("if: stateCapturesFinalText(state)")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if !state.Control.IsZero() && strings.TrimSpace(state.Persona) == "" {
		observe.GlobalTrace("if: !state.Control.IsZero() && strings.TrimSpace(state.Persona) == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: output gate")
	observe.GlobalTrace("return: `## Response Output Gate\n\nYour assistant ` + \"`content`\" + ` field must start...")
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
		renderedContent := content
		if artifact.MaxBytes > 0 && len(renderedContent) > artifact.MaxBytes {
			observe.GlobalTrace("if: artifact.MaxBytes > 0 && len(renderedContent) > artifact.MaxBytes")
			renderedContent = renderedContent[:artifact.MaxBytes]
			fmt.Fprintf(&b, "[rendered content truncated to first %d of %d bytes by handoff max_bytes]\n", artifact.MaxBytes, len(content))
		}
		b.WriteString(strings.TrimRight(string(renderedContent), "\n"))
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sum := sha256.Sum256(data)
	observe.GlobalTrace("return: hex.EncodeToString(sum[:])")
	return hex.EncodeToString(sum[:])
}

func artifactDigest(path string) (int64, string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: 0, \"\", false")
		return 0, "", false
	}
	observe.GlobalTrace("return: int64(len(data)), sha256Hex(data), true")
	return int64(len(data)), sha256Hex(data), true
}

type ControlExecutionContext struct {
	ArtifactRoot         string
	PreviousStateOutputs *StateOutputRun
}

type StateOutputRun struct {
	StateID string
	Outputs map[string]StateOutputArtifactRun
}

type StateOutputArtifactRun struct {
	ArtifactID string
	Path       string
	Before     ArtifactFileSnapshot
	After      ArtifactFileSnapshot
}

type ArtifactFileSnapshot struct {
	Exists  bool
	Path    string
	Bytes   int64
	SHA256  string
	ModTime time.Time
}

func snapshotStateOutputs(state State, artifactRoot string) (map[string]ArtifactFileSnapshot, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make(map[string]ArtifactFileSnapshot)
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		snapshot, err := snapshotArtifactFile(path)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"snapshot output artifact %q at %q: %w\", artifact.ID, path, err)")
			return nil, fmt.Errorf("snapshot output artifact %q at %q: %w", artifact.ID, path, err)
		}
		out[path] = snapshot
	}
	observe.GlobalTrace("return: out, nil")
	return out, nil
}

func snapshotArtifactFile(path string) (ArtifactFileSnapshot, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snapshot := ArtifactFileSnapshot{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if os.IsNotExist(err) {
			observe.GlobalTrace("if: os.IsNotExist(err)")
			observe.GlobalTrace("return: snapshot, nil")
			return snapshot, nil
		}
		observe.GlobalTrace("return: snapshot, err")
		return snapshot, err
	}
	if info.IsDir() {
		observe.GlobalTrace("if: info.IsDir()")
		observe.GlobalTrace("return: snapshot, fmt.Errorf(\"is a directory\")")
		return snapshot, fmt.Errorf("is a directory")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: snapshot, err")
		return snapshot, err
	}
	snapshot.Exists = true
	snapshot.Bytes = int64(len(data))
	snapshot.SHA256 = sha256Hex(data)
	snapshot.ModTime = info.ModTime()
	observe.GlobalTrace("return: snapshot, nil")
	return snapshot, nil
}

func newStateOutputRun(state State, artifactRoot string, before map[string]ArtifactFileSnapshot, after map[string]ArtifactFileSnapshot) *StateOutputRun {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	run := &StateOutputRun{
		StateID: state.ID,
		Outputs: make(map[string]StateOutputArtifactRun, len(state.Artifacts.Outputs)),
	}
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		run.Outputs[path] = StateOutputArtifactRun{
			ArtifactID: artifact.ID,
			Path:       path,
			Before:     before[path],
			After:      after[path],
		}
	}
	observe.GlobalTrace("return: run")
	return run
}

func validateFreshRequiredModelOutputs(state State, artifactRoot string, before map[string]ArtifactFileSnapshot, after map[string]ArtifactFileSnapshot) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var missing []string
	var runtimeMissing []string
	var stale []string
	for _, artifact := range requiredOutputArtifacts(state.Artifacts.Outputs) {
		observe.GlobalTrace("range requiredOutputArtifacts")
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		beforeSnapshot := before[path]
		afterSnapshot := after[path]
		if !afterSnapshot.Exists {
			observe.GlobalTrace("if: !afterSnapshot.Exists")
			issue := fmt.Sprintf("- `%s`: `%s`", artifact.ID, path)
			if artifactRuntimeAuthored(artifact) {
				runtimeMissing = append(runtimeMissing, issue)
			} else {
				missing = append(missing, issue)
			}
			continue
		}
		if artifactRuntimeAuthored(artifact) {
			observe.GlobalTrace("if: artifactRuntimeAuthored(artifact)")
			continue
		}
		if beforeSnapshot.Exists && afterSnapshot.Bytes == beforeSnapshot.Bytes && afterSnapshot.SHA256 == beforeSnapshot.SHA256 && !afterSnapshot.ModTime.After(beforeSnapshot.ModTime) {
			observe.GlobalTrace("if: stale required model artifact")
			stale = append(stale, fmt.Sprintf("- `%s`: `%s` existed before this state and was not freshly written", artifact.ID, path))
		}
	}
	if len(missing) > 0 || len(runtimeMissing) > 0 || len(stale) > 0 {
		observe.GlobalTrace("if: len(missing) > 0 || len(runtimeMissing) > 0 || len(stale) > 0")
		return fmt.Errorf("required output artifact check failed after state %q:\n%s", state.ID, completionRejectionGuidance(missing, runtimeMissing, stale, nil, nil))
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validateFreshArtifactRequirements(requirements []FreshArtifactRequirement, controlCtx ControlExecutionContext) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(requirements) == 0 {
		observe.GlobalTrace("if: len(requirements) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	if controlCtx.PreviousStateOutputs == nil {
		observe.GlobalTrace("if: controlCtx.PreviousStateOutputs == nil")
		observe.GlobalTrace("return: fmt.Errorf(\"fresh artifact requirement cannot be checked without a previous s...")
		return fmt.Errorf("fresh artifact requirement cannot be checked without a previous state")
	}
	artifactRoot := controlCtx.ArtifactRoot
	if strings.TrimSpace(artifactRoot) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(artifactRoot) == \"\"")
		artifactRoot = DefaultArtifactRoot
	}
	for _, requirement := range requirements {
		observe.GlobalTrace("range requirements")
		path := resolveArtifactPath(requirement.Path, artifactRoot)
		output, ok := controlCtx.PreviousStateOutputs.Outputs[path]
		if !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: fmt.Errorf(\"fresh artifact %q was not declared as an output of immediately pr...")
			return fmt.Errorf("fresh artifact %q was not declared as an output of immediately preceding state %q", path, controlCtx.PreviousStateOutputs.StateID)
		}
		if !output.After.Exists {
			observe.GlobalTrace("if: !output.After.Exists")
			observe.GlobalTrace("return: fmt.Errorf(\"fresh artifact %q was not produced by immediately preceding state...")
			return fmt.Errorf("fresh artifact %q was not produced by immediately preceding state %q", path, controlCtx.PreviousStateOutputs.StateID)
		}
		if artifactOutputFresh(output.Before, output.After) {
			observe.GlobalTrace("if: artifactOutputFresh(output.Before, output.After)")
			continue
		}
		observe.GlobalTrace("return: fmt.Errorf(\"fresh artifact %q was not freshly written by immediately precedin...")
		return fmt.Errorf("fresh artifact %q was not freshly written by immediately preceding state %q (before=%s after=%s)", path, controlCtx.PreviousStateOutputs.StateID, formatArtifactSnapshot(output.Before), formatArtifactSnapshot(output.After))
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func artifactOutputFresh(before ArtifactFileSnapshot, after ArtifactFileSnapshot) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !after.Exists {
		observe.GlobalTrace("if: !after.Exists")
		observe.GlobalTrace("return: false")
		return false
	}
	if !before.Exists {
		observe.GlobalTrace("if: !before.Exists")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: before.SHA256 != after.SHA256 || after.ModTime.After(before.ModTime)")
	return before.SHA256 != after.SHA256 || after.ModTime.After(before.ModTime)
}

func formatArtifactSnapshot(snapshot ArtifactFileSnapshot) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !snapshot.Exists {
		observe.GlobalTrace("if: !snapshot.Exists")
		observe.GlobalTrace("return: \"missing\"")
		return "missing"
	}
	observe.GlobalTrace("return: fmt.Sprintf(\"sha=%s bytes=%d mtime=%s\", snapshot.SHA256, snapshot.Bytes, snap...")
	return fmt.Sprintf("sha=%s bytes=%d mtime=%s", snapshot.SHA256, snapshot.Bytes, snapshot.ModTime.UTC().Format(time.RFC3339Nano))
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch attachment {
	case "source_edit_transport":
		observe.GlobalTrace("case: \"source_edit_transport\"")
		return renderSourceEditTransport()
	default:
		observe.GlobalTrace("default")
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
	modelOutputs, runtimeOutputs := artifactOutputsByOwnership(artifacts.Outputs)
	if len(modelOutputs) > 0 {
		observe.GlobalTrace("if: len(modelOutputs) > 0")
		b.WriteString("Outputs:\n")
		for _, artifact := range modelOutputs {
			observe.GlobalTrace("range modelOutputs")
			writeArtifactLine(&b, artifact)
		}
		b.WriteString("\n")
	}
	if len(runtimeOutputs) > 0 {
		observe.GlobalTrace("if: len(runtimeOutputs) > 0")
		b.WriteString("Runtime-authored artifacts (not writable deliverables):\n")
		for _, artifact := range runtimeOutputs {
			observe.GlobalTrace("range runtimeOutputs")
			writeArtifactLine(&b, artifact)
		}
		b.WriteString("Do not create, edit, delete, truncate, overwrite, or fabricate runtime-authored artifacts. To improve runtime evidence, run real commands. To fix unsupported claims, edit model-authored output artifacts.\n\n")
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
	if stateCapturesFinalText(state) {
		observe.GlobalTrace("if: stateCapturesFinalText(state)")
		var b strings.Builder
		b.WriteString("## Runtime Completion Contract\n\n")
		b.WriteString("This orchestration state captures the assistant final text as a runtime-authored artifact.\n")
		b.WriteString("Do not write a bash block, run commands, or create artifact files. Return only the final text requested by this state.\n")
		_, runtimeRequiredOutputs := requiredOutputArtifactsByOwnership(state.Artifacts.Outputs)
		if len(runtimeRequiredOutputs) > 0 {
			observe.GlobalTrace("if: len(runtimeRequiredOutputs) > 0")
			b.WriteString("The runtime will write these required artifacts from your final text:\n")
			for _, artifact := range runtimeRequiredOutputs {
				observe.GlobalTrace("range runtimeRequiredOutputs")
				fmt.Fprintf(&b, "- `%s`: `%s`\n", artifact.ID, artifact.Path)
			}
		}
		observe.GlobalTrace("return: b.String()")
		return b.String()
	}
	var b strings.Builder
	b.WriteString("## Runtime Completion Contract\n\n")
	b.WriteString("This orchestration state does not use plain prose final answers.\n")
	b.WriteString("When this state is complete, respond with exactly one fenced bash block and no prose outside it.\n")
	if state.MaxTurns > 0 {
		observe.GlobalTrace("if: state.MaxTurns > 0")
		fmt.Fprintf(&b, "This state has a runtime budget of %d shell actions. Plan command batches so you either complete the state or write the required route artifacts before the budget is exhausted.\n", state.MaxTurns)
	}
	modelRequiredOutputs, runtimeRequiredOutputs := requiredOutputArtifactsByOwnership(state.Artifacts.Outputs)
	if len(modelRequiredOutputs) > 0 {
		observe.GlobalTrace("if: len(modelRequiredOutputs) > 0")
		b.WriteString("Before completing, every model-authored required output artifact below must exist:\n")
		for _, artifact := range modelRequiredOutputs {
			observe.GlobalTrace("range modelRequiredOutputs")
			fmt.Fprintf(&b, "- `%s`: `%s`\n", artifact.ID, artifact.Path)
		}
	}
	if len(runtimeRequiredOutputs) > 0 {
		observe.GlobalTrace("if: len(runtimeRequiredOutputs) > 0")
		b.WriteString("Runtime-authored required artifacts are checked by the runtime but are not writable deliverables:\n")
		for _, artifact := range runtimeRequiredOutputs {
			observe.GlobalTrace("range runtimeRequiredOutputs")
			fmt.Fprintf(&b, "- `%s`: `%s`\n", artifact.ID, artifact.Path)
		}
		b.WriteString("Do not create, edit, delete, truncate, overwrite, or fabricate runtime-authored artifacts. To improve runtime evidence, run real commands. To fix unsupported claims, edit model-authored output artifacts.\n")
	}
	if len(modelRequiredOutputs) > 0 {
		observe.GlobalTrace("if: len(modelRequiredOutputs) > 0")
		b.WriteString("The completion bash block may write the final model-authored artifact content, or verify already-written model-authored artifacts, but it must end with:\n")
	} else {
		observe.GlobalTrace("else: len(modelRequiredOutputs) > 0")
		b.WriteString("After the state-specific work is complete, the completion bash block must end with:\n")
	}
	b.WriteString("echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n")
	b.WriteString("Do not emit COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT after a read-only inspection unless the state-specific work is already complete.\n")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func stateCapturesFinalText(state State) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifact.RuntimeCapture.Type == "final_text" {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func artifactOutputsByOwnership(outputs []Artifact) ([]Artifact, []Artifact) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	modelAuthored := make([]Artifact, 0, len(outputs))
	runtimeAuthored := make([]Artifact, 0, len(outputs))
	for _, artifact := range outputs {
		observe.GlobalTrace("range outputs")
		if artifactRuntimeAuthored(artifact) {
			observe.GlobalTrace("if: artifactRuntimeAuthored(artifact)")
			runtimeAuthored = append(runtimeAuthored, artifact)
			continue
		}
		modelAuthored = append(modelAuthored, artifact)
	}
	observe.GlobalTrace("return: modelAuthored, runtimeAuthored")
	return modelAuthored, runtimeAuthored
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

func requiredOutputArtifactsByOwnership(outputs []Artifact) ([]Artifact, []Artifact) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	required := requiredOutputArtifacts(outputs)
	observe.GlobalTrace("return: artifactOutputsByOwnership(required)")
	return artifactOutputsByOwnership(required)
}

func artifactRuntimeAuthored(artifact Artifact) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.TrimSpace(artifact.RuntimeCapture.Type) != \"\"")
	return strings.TrimSpace(artifact.RuntimeCapture.Type) != ""
}

func outputArtifactPath(state State, id string, artifactRoot string) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifact.ID == id {
			observe.GlobalTrace("if: artifact.ID == id")
			observe.GlobalTrace("return: resolveArtifactPath(artifact.Path, artifactRoot), true")
			return resolveArtifactPath(artifact.Path, artifactRoot), true
		}
	}
	observe.GlobalTrace("return: \"\", false")
	return "", false
}

func commandEvidenceConfig(state State, artifactRoot string) (query.PragmaLoopCommandEvidenceConfig, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var cfg query.PragmaLoopCommandEvidenceConfig
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifact.RuntimeCapture.Type != "command_evidence" {
			observe.GlobalTrace("if: artifact.RuntimeCapture.Type != \"command_evidence\"")
			continue
		}
		cfg.Path = resolveArtifactPath(artifact.Path, artifactRoot)
		break
	}
	if strings.TrimSpace(cfg.Path) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(cfg.Path) == \"\"")
		observe.GlobalTrace("return: cfg, false")
		return cfg, false
	}
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		for _, check := range artifact.Checks {
			observe.GlobalTrace("range artifact.Checks")
			if check.Type != "command_evidence_support" {
				observe.GlobalTrace("if: check.Type != \"command_evidence_support\"")
				continue
			}
			evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
			if !ok || evidencePath != cfg.Path {
				observe.GlobalTrace("if: !ok || evidencePath != cfg.Path")
				continue
			}
			cfg.ReportPaths = append(cfg.ReportPaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
	}
	observe.GlobalTrace("return: cfg, true")
	return cfg, true
}

func commandPolicyConfig(state State, transitionHandoff []Artifact, artifactRoot string) (query.PragmaLoopCommandPolicyConfig, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cfg := query.PragmaLoopCommandPolicyConfig{
		DenyPatterns:    append([]string(nil), state.ShellPolicy.DenyPatterns...),
		RequirePatterns: append([]string(nil), state.ShellPolicy.RequirePatterns...),
		DenyMessage:     state.ShellPolicy.DenyMessage,
	}
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifactRuntimeAuthored(artifact) {
			observe.GlobalTrace("if: artifactRuntimeAuthored(artifact)")
			cfg.ProtectedWritePaths = append(cfg.ProtectedWritePaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
	}
	if strings.TrimSpace(state.ShellPolicy.HandoffInputs) == "rendered" {
		observe.GlobalTrace("if: strings.TrimSpace(state.ShellPolicy.HandoffInputs) == \"rendered\"")
		for _, artifact := range transitionHandoff {
			observe.GlobalTrace("range transitionHandoff")
			cfg.RenderedInputPaths = append(cfg.RenderedInputPaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
		for _, artifact := range state.Artifacts.Outputs {
			observe.GlobalTrace("range state.Artifacts.Outputs")
			cfg.WritablePaths = append(cfg.WritablePaths, resolveArtifactPath(artifact.Path, artifactRoot))
		}
	}
	if len(cfg.DenyPatterns) == 0 && len(cfg.RequirePatterns) == 0 && len(cfg.RenderedInputPaths) == 0 && len(cfg.ProtectedWritePaths) == 0 {
		observe.GlobalTrace("if: len(cfg.DenyPatterns) == 0 && len(cfg.RenderedInputPaths) == 0 && len(cfg.Pro...")
		observe.GlobalTrace("return: query.PragmaLoopCommandPolicyConfig{}, false")
		return query.PragmaLoopCommandPolicyConfig{}, false
	}
	observe.GlobalTrace("return: cfg, true")
	return cfg, true
}

func handoffSnapshots(reads []HandoffRead) map[string][]byte {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(reads) == 0 {
		observe.GlobalTrace("if: len(reads) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := make(map[string][]byte, len(reads))
	for _, read := range reads {
		observe.GlobalTrace("range reads")
		if read.ArtifactID == "" || len(read.Content) == 0 {
			observe.GlobalTrace("if: read.ArtifactID == \"\" || len(read.Content) == 0")
			continue
		}
		out[read.ArtifactID] = append([]byte(nil), read.Content...)
	}
	observe.GlobalTrace("return: out")
	return out
}

func requiredOutputArtifactCompletionCheck(state State, artifactRoot string, taskPrompts ...string) query.PragmaLoopCompletionCheck {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	taskPrompt := ""
	if len(taskPrompts) > 0 {
		observe.GlobalTrace("if: len(taskPrompts) > 0")
		taskPrompt = taskPrompts[0]
	}
	observe.GlobalTrace("return: requiredOutputArtifactCompletionCheckWithSnapshots(state, artifactRoot, taskP...")
	return requiredOutputArtifactCompletionCheckWithSnapshots(state, artifactRoot, taskPrompt, nil, nil)
}

func requiredOutputArtifactCompletionCheckWithSnapshots(state State, artifactRoot string, taskPrompt string, snapshots map[string][]byte, outputSnapshotsBefore map[string]ArtifactFileSnapshot) query.PragmaLoopCompletionCheck {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	required := requiredOutputArtifacts(state.Artifacts.Outputs)
	if len(required) == 0 {
		observe.GlobalTrace("if: len(required) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: func() (bool, string, error) {\n\tvar missing []string\n\tfor _, artifact := rang...")
	observe.GlobalTrace("return: func() (bool, string, error) {\n\tvar modelMissing []string\n\tvar runtimeMissing...")
	return func() (bool, string, error) {
		var modelMissing []string
		var runtimeMissing []string
		var modelStale []string
		for _, artifact := range required {
			path := resolveArtifactPath(artifact.Path, artifactRoot)
			info, err := os.Stat(path)
			if err != nil {
				issue := fmt.Sprintf("- `%s`: `%s` (%v)", artifact.ID, path, err)
				if artifactRuntimeAuthored(artifact) {
					runtimeMissing = append(runtimeMissing, issue)
				} else {
					modelMissing = append(modelMissing, issue)
				}
				continue
			}
			if !artifactRuntimeAuthored(artifact) && outputSnapshotsBefore != nil {
				before := outputSnapshotsBefore[path]
				if before.Exists {
					afterBytes, afterSHA, ok := artifactDigest(path)
					if ok && afterBytes == before.Bytes && afterSHA == before.SHA256 && !info.ModTime().After(before.ModTime) {
						modelStale = append(modelStale, fmt.Sprintf("- `%s`: `%s` existed before this state and was not freshly written", artifact.ID, path))
					}
				}
			}
		}
		var modelInvalid []string
		var runtimeInvalid []string
		for _, artifact := range required {
			path := resolveArtifactPath(artifact.Path, artifactRoot)
			for _, check := range artifact.Checks {
				issues, err := validateArtifactIntegrityCheck(artifact, check, state, path, artifactRoot, taskPrompt, snapshots)
				if err != nil {
					issues = []string{fmt.Sprintf("- `%s`: `%s` (%v)", artifact.ID, path, err)}
				}
				if len(issues) == 0 {
					continue
				}
				if artifactRuntimeAuthored(artifact) || artifactIntegrityCheckReferencesRuntimeAuthoredOutput(state, check) {
					runtimeInvalid = append(runtimeInvalid, issues...)
				} else {
					modelInvalid = append(modelInvalid, issues...)
				}
			}
		}
		if len(modelMissing) == 0 && len(runtimeMissing) == 0 && len(modelStale) == 0 && len(modelInvalid) == 0 && len(runtimeInvalid) == 0 {
			return true, "", nil
		}
		return false, completionRejectionGuidance(modelMissing, runtimeMissing, modelStale, modelInvalid, runtimeInvalid), nil
	}
}

func artifactIntegrityCheckReferencesRuntimeAuthoredOutput(state State, check ArtifactIntegrityCheck) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(check.ArtifactID) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(check.ArtifactID) == \"\"")
		observe.GlobalTrace("return: false")
		return false
	}
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifact.ID == check.ArtifactID {
			observe.GlobalTrace("if: artifact.ID == check.ArtifactID")
			observe.GlobalTrace("return: artifactRuntimeAuthored(artifact)")
			return artifactRuntimeAuthored(artifact)
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func completionRejectionGuidance(modelMissing []string, runtimeMissing []string, modelStale []string, modelInvalid []string, runtimeInvalid []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var sections []string
	if len(modelMissing) > 0 {
		observe.GlobalTrace("if: len(modelMissing) > 0")
		sections = append(sections, fmt.Sprintf("Model-authored required output artifact(s) are missing:\n%s", strings.Join(modelMissing, "\n")))
	}
	if len(modelStale) > 0 {
		observe.GlobalTrace("if: len(modelStale) > 0")
		sections = append(sections, fmt.Sprintf("Model-authored required output artifact(s) were not freshly written by this state:\n%s", strings.Join(modelStale, "\n")))
	}
	if len(modelInvalid) > 0 {
		observe.GlobalTrace("if: len(modelInvalid) > 0")
		sections = append(sections, fmt.Sprintf("Model-authored required output artifact(s) failed integrity checks:\n%s", strings.Join(modelInvalid, "\n")))
	}
	if len(runtimeMissing) > 0 {
		observe.GlobalTrace("if: len(runtimeMissing) > 0")
		sections = append(sections, fmt.Sprintf("Runtime-authored required artifact(s) are missing:\n%s", strings.Join(runtimeMissing, "\n")))
	}
	if len(runtimeInvalid) > 0 {
		observe.GlobalTrace("if: len(runtimeInvalid) > 0")
		sections = append(sections, fmt.Sprintf("Runtime-authored required artifact(s) failed integrity checks:\n%s", strings.Join(runtimeInvalid, "\n")))
	}

	hasModelIssues := len(modelMissing) > 0 || len(modelStale) > 0 || len(modelInvalid) > 0
	hasRuntimeIssues := len(runtimeMissing) > 0 || len(runtimeInvalid) > 0
	var next string
	switch {
	case hasModelIssues && hasRuntimeIssues:
		observe.GlobalTrace("case: hasModelIssues && hasRuntimeIssues")
		next = "Run another fenced bash block that fixes model-authored output artifact(s) directly. Do not edit runtime-authored artifact files; for runtime evidence, run real commands or revise model-authored output artifacts so they only claim evidence that exists. Then echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again."
	case hasRuntimeIssues:
		observe.GlobalTrace("case: hasRuntimeIssues")
		next = "Do not edit runtime-authored artifact files. Run real commands so the runtime can capture evidence, or revise model-authored output artifacts so they only claim evidence that exists. Then echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again."
	default:
		observe.GlobalTrace("default")
		next = "Run another fenced bash block that fixes model-authored output artifact(s), then echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again."
	}
	observe.GlobalTrace("return: fmt.Sprintf(...)")
	observe.GlobalTrace("return: fmt.Sprintf(\"Completion was rejected because:\\n%s\\n\\n%s\", strings.Join(sectio...")
	return fmt.Sprintf("Completion was rejected because:\n%s\n\n%s", strings.Join(sections, "\n\n"), next)
}

func validateArtifactIntegrityCheck(artifact Artifact, check ArtifactIntegrityCheck, state State, artifactPath string, artifactRoot string, taskPrompt string, snapshots map[string][]byte) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch check.Type {
	case "json_field_equals":
		observe.GlobalTrace("case: \"json_field_equals\"")
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		value, ok := doc[check.Field]
		if !ok || fmt.Sprint(value) != check.Value {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` field %q is %q, want %q\", artifact.ID, check.Fie...")
			return []string{fmt.Sprintf("- `%s` field %q is %q, want %q", artifact.ID, check.Field, fmt.Sprint(value), check.Value)}, nil
		}
	case "json_required_fields":
		observe.GlobalTrace("case: \"json_required_fields\"")
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		var issues []string
		for _, field := range check.Fields {
			field = strings.TrimSpace(field)
			if field == "" {
				observe.GlobalTrace("if: field == \"\"")
				continue
			}
			value, ok := jsonValueAtPath(doc, field)
			if !ok {
				observe.GlobalTrace("if: !ok")
				issues = append(issues, fmt.Sprintf("- `%s` missing required field %q", artifact.ID, field))
				continue
			}
			if jsonValueEmpty(value) {
				observe.GlobalTrace("if: jsonValueEmpty(value)")
				issues = append(issues, fmt.Sprintf("- `%s` required field %q is empty", artifact.ID, field))
			}
		}
		return issues, nil
	case "json_array_non_empty":
		observe.GlobalTrace("case: \"json_array_non_empty\"")
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		if len(items) == 0 {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` array %q is empty\", artifact.ID, check.Path)}, nil")
			return []string{fmt.Sprintf("- `%s` array %q is empty", artifact.ID, check.Path)}, nil
		}
	case "json_array_max_items":
		observe.GlobalTrace("case: \"json_array_max_items\"")
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		values, err := jsonStringArrayPath(doc, check.Field)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		maxItems, err := strconv.Atoi(strings.TrimSpace(check.Value))
		if err != nil || maxItems < 0 {
			observe.GlobalTrace("return: nil, fmt.Errorf invalid max")
			return nil, fmt.Errorf("artifact %q check json_array_max_items has invalid value %q", artifact.ID, check.Value)
		}
		if len(values) > maxItems {
			observe.GlobalTrace("if: len(values) > maxItems")
			return []string{fmt.Sprintf("- `%s` field %q has %d items, maximum is %d", artifact.ID, check.Field, len(values), maxItems)}, nil
		}
	case "json_no_unproven_generated_outputs_in_approved_edit_paths":
		observe.GlobalTrace("case: \"json_no_unproven_generated_outputs_in_approved_edit_paths\"")
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateJSONNoUnprovenGeneratedOutputsInApprovedEditPaths(artifact, doc), nil
	case "json_worker_track_targeted_validation_consistent":
		observe.GlobalTrace("case: \"json_worker_track_targeted_validation_consistent\"")
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateJSONWorkerTrackTargetedValidationConsistent(artifact, doc), nil
	case "json_each_required_fields":
		observe.GlobalTrace("case: \"json_each_required_fields\"")
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			label := artifactItemLabel(item, idx)
			for _, field := range check.Fields {
				observe.GlobalTrace("range check.Fields")
				if strings.TrimSpace(jsonStringField(item, field)) == "" {
					observe.GlobalTrace("if: strings.TrimSpace(jsonStringField(item, field)) == \"\"")
					issues = append(issues, fmt.Sprintf("- `%s` %s has empty %s", artifact.ID, label, field))
				}
			}
		}
		return issues, nil
	case "json_each_string_substring_of_task_prompt":
		observe.GlobalTrace("case: \"json_each_string_substring_of_task_prompt\"")
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			value := strings.TrimSpace(jsonStringField(item, check.Field))
			if value == "" {
				observe.GlobalTrace("if: value == \"\"")
				continue
			}
			if strings.TrimSpace(taskPrompt) != "" && !strings.Contains(taskPrompt, value) {
				observe.GlobalTrace("if: strings.TrimSpace(taskPrompt) != \"\" && !strings.Contains(taskPrompt, value)")
				issues = append(issues, fmt.Sprintf("- `%s` %s %s is not an exact substring of the original task prompt: %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
			}
		}
		return issues, nil
	case "json_each_repo_relative_paths":
		observe.GlobalTrace("case: \"json_each_repo_relative_paths\"")
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			for _, value := range jsonStringArrayField(item, check.Field) {
				observe.GlobalTrace("range jsonStringArrayField(item, check.Field)")
				if invalidRepoRelativePath(value) {
					observe.GlobalTrace("if: invalidRepoRelativePath(value)")
					issues = append(issues, fmt.Sprintf("- `%s` %s %s contains non-repo-relative path %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
				}
			}
		}
		return issues, nil
	case "json_each_string_array_values_in_task_or_handoff":
		observe.GlobalTrace("case: \"json_each_string_array_values_in_task_or_handoff\"")
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		handoffRaw := snapshots[check.ArtifactID]
		groundingText := strings.ToLower(taskPrompt + "\n" + string(handoffRaw))
		var issues []string
		for idx, item := range items {
			for _, value := range jsonStringArrayField(item, check.Field) {
				observe.GlobalTrace("range jsonStringArrayField(item, check.Field)")
				value = strings.TrimSpace(value)
				if value == "" || strings.EqualFold(value, "unknown") {
					observe.GlobalTrace("if: value == \"\" || strings.EqualFold(value, \"unknown\")")
					continue
				}
				if !strings.Contains(groundingText, strings.ToLower(value)) {
					observe.GlobalTrace("if: !strings.Contains(groundingText, strings.ToLower(value))")
					issues = append(issues, fmt.Sprintf("- `%s` %s %s value %q is not grounded in the original task prompt or handoff artifact %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value, check.ArtifactID))
				}
			}
		}
		return issues, nil
	case "json_each_behavior_validation_commands":
		observe.GlobalTrace("case: \"json_each_behavior_validation_commands\"")
		items, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		var issues []string
		for idx, item := range items {
			for _, value := range jsonStringArrayField(item, check.Field) {
				observe.GlobalTrace("range jsonStringArrayField(item, check.Field)")
				if placeholderValidationCommand(value) {
					observe.GlobalTrace("if: placeholderValidationCommand(value)")
					issues = append(issues, fmt.Sprintf("- `%s` %s %s uses placeholder validation instead of a concrete behavior command: %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
				}
				if invalidValidationPath(value) {
					observe.GlobalTrace("if: invalidValidationPath(value)")
					issues = append(issues, fmt.Sprintf("- `%s` %s %s uses an absolute repository path instead of a repo-relative path: %q", artifact.ID, artifactItemLabel(item, idx), check.Field, value))
				}
			}
		}
		return issues, nil
	case "json_each_fields_equal_handoff_artifact":
		observe.GlobalTrace("case: \"json_each_fields_equal_handoff_artifact\"")
		currentItems, err := readJSONArrayField(artifactPath, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		referenceRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(referenceRaw) == 0 {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find rendered handoff snapshot for artifa...")
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		referenceItems, err := readJSONArrayFieldBytes(referenceRaw, check.Path)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		referenceByKey := make(map[string]map[string]interface{}, len(referenceItems))
		for idx, item := range referenceItems {
			key := strings.TrimSpace(jsonStringField(item, check.KeyField))
			if key == "" {
				observe.GlobalTrace("if: key == \"\"")
				observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` rendered handoff %s has empty key field %q\", art...")
				return []string{fmt.Sprintf("- `%s` rendered handoff %s has empty key field %q", artifact.ID, artifactItemLabel(item, idx), check.KeyField)}, nil
			}
			referenceByKey[key] = item
		}
		var issues []string
		for idx, item := range currentItems {
			key := strings.TrimSpace(jsonStringField(item, check.KeyField))
			if key == "" {
				observe.GlobalTrace("if: key == \"\"")
				issues = append(issues, fmt.Sprintf("- `%s` %s has empty key field %q", artifact.ID, artifactItemLabel(item, idx), check.KeyField))
				continue
			}
			reference, ok := referenceByKey[key]
			if !ok {
				observe.GlobalTrace("if: !ok")
				issues = append(issues, fmt.Sprintf("- `%s` %s has no matching rendered handoff item by %q", artifact.ID, artifactItemLabel(item, idx), check.KeyField))
				continue
			}
			for _, field := range check.Fields {
				observe.GlobalTrace("range check.Fields")
				if !reflect.DeepEqual(item[field], reference[field]) {
					observe.GlobalTrace("if: !reflect.DeepEqual(item[field], reference[field])")
					issues = append(issues, fmt.Sprintf("- `%s` %s field %q changed from rendered handoff value", artifact.ID, artifactItemLabel(item, idx), field))
				}
			}
		}
		return issues, nil
	case "json_fields_equal_handoff_artifact":
		observe.GlobalTrace("case: \"json_fields_equal_handoff_artifact\"")
		currentDoc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		referenceRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(referenceRaw) == 0 {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find rendered handoff snapshot for artifa...")
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		referenceDoc, err := readJSONObjectBytes(referenceRaw)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		var issues []string
		for _, field := range check.Fields {
			currentValue, currentOK := jsonValueAtPath(currentDoc, field)
			referenceValue, referenceOK := jsonValueAtPath(referenceDoc, field)
			switch {
			case !currentOK && !referenceOK:
				observe.GlobalTrace("case: !currentOK && !referenceOK")
				continue
			case !currentOK:
				observe.GlobalTrace("case: !currentOK")
				issues = append(issues, fmt.Sprintf("- `%s` missing field %q from rendered handoff artifact %q", artifact.ID, field, check.ArtifactID))
			case !referenceOK:
				observe.GlobalTrace("case: !referenceOK")
				issues = append(issues, fmt.Sprintf("- `%s` field %q has no rendered handoff value in artifact %q", artifact.ID, field, check.ArtifactID))
			case !reflect.DeepEqual(currentValue, referenceValue):
				observe.GlobalTrace("case: !reflect.DeepEqual(currentValue, referenceValue)")
				issues = append(issues, fmt.Sprintf("- `%s` field %q changed from rendered handoff artifact %q", artifact.ID, field, check.ArtifactID))
			}
		}
		return issues, nil
	case "json_array_subset_of_handoff_text_list":
		observe.GlobalTrace("case: \"json_array_subset_of_handoff_text_list\"")
		doc, err := readJSONObject(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		values, err := jsonStringArrayPath(doc, check.Field)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		referenceRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(referenceRaw) == 0 {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find rendered handoff snapshot for artifa...")
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		allowed, ok := textListField(referenceRaw, check.TextField)
		if !ok {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find rendered handoff text field %q in ar...")
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
				observe.GlobalTrace("if: value == \"\"")
				continue
			}
			if _, ok := allowedSet[value]; !ok {
				observe.GlobalTrace("if: !ok")
				issues = append(issues, fmt.Sprintf("- `%s` field %q value %q is not present in rendered handoff artifact %q text field %q", artifact.ID, check.Field, value, check.ArtifactID, check.TextField))
			}
		}
		return issues, nil
	case "markdown_candidate_surfaces_concrete":
		observe.GlobalTrace("case: \"markdown_candidate_surfaces_concrete\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateMarkdownCandidateSurfacesConcrete(artifact, data), nil
	case "markdown_constraints_supported_by_claims":
		observe.GlobalTrace("case: \"markdown_constraints_supported_by_claims\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateMarkdownConstraintsSupportedByClaims(artifact, data), nil
	case "markdown_no_generated_output_edit_recommendations":
		observe.GlobalTrace("case: \"markdown_no_generated_output_edit_recommendations\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateMarkdownNoGeneratedOutputEditRecommendations(artifact, data), nil
	case "markdown_no_forbidden_worker_commands":
		observe.GlobalTrace("case: \"markdown_no_forbidden_worker_commands\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		contractRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(contractRaw) == 0 {
			observe.GlobalTrace("return: missing contract snapshot")
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateMarkdownNoForbiddenWorkerCommands(artifact, data, check.ArtifactID, contractRaw), nil
	case "markdown_scope_request_requires_no_changed_files":
		observe.GlobalTrace("case: \"markdown_scope_request_requires_no_changed_files\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateMarkdownScopeRequestRequiresNoChangedFiles(artifact, data), nil
	case "markdown_scope_request_requires_clean_worktree":
		observe.GlobalTrace("case: \"markdown_scope_request_requires_clean_worktree\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateMarkdownScopeRequestRequiresCleanWorktree(artifact, data), nil
	case "markdown_validation_coverage_consistent":
		observe.GlobalTrace("case: \"markdown_validation_coverage_consistent\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		return validateMarkdownValidationCoverageConsistent(artifact, data), nil
	case "markdown_worker_blocker_validation_no_commands":
		observe.GlobalTrace("case: \"markdown_worker_blocker_validation_no_commands\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		workerRaw, ok := snapshots[check.ArtifactID]
		if !ok || len(workerRaw) == 0 {
			observe.GlobalTrace("return: missing worker snapshot")
			return []string{fmt.Sprintf("- `%s` cannot find rendered handoff snapshot for artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateMarkdownWorkerBlockerValidationNoCommands(artifact, data, check.ArtifactID, workerRaw), nil
	case "text_forbid_contains":
		observe.GlobalTrace("case: \"text_forbid_contains\"")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		if strings.Contains(strings.ToLower(string(data)), strings.ToLower(check.Value)) {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` contains forbidden text %q\", artifact.ID, check....")
			return []string{fmt.Sprintf("- `%s` contains forbidden text %q", artifact.ID, check.Value)}, nil
		}
	case "command_evidence_support":
		observe.GlobalTrace("case: \"command_evidence_support\"")
		evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
		if !ok {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find declared evidence artifact %q\", arti...")
			return []string{fmt.Sprintf("- `%s` cannot find declared evidence artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateCommandEvidenceSupport(artifact, artifactPath, check, evidencePath)
	case "command_evidence_claimed_changes":
		observe.GlobalTrace("case: \"command_evidence_claimed_changes\"")
		evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
		if !ok {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find declared evidence artifact %q\", arti...")
			return []string{fmt.Sprintf("- `%s` cannot find declared evidence artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateCommandEvidenceClaimedChanges(artifact, artifactPath, check, evidencePath)
	case "command_evidence_non_report_limit":
		observe.GlobalTrace("case: \"command_evidence_non_report_limit\"")
		evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
		if !ok {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find declared evidence artifact %q\", arti...")
			return []string{fmt.Sprintf("- `%s` cannot find declared evidence artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateCommandEvidenceNonReportLimit(artifact, check, evidencePath)
	case "command_evidence_repo_mutation_limit":
		observe.GlobalTrace("case: \"command_evidence_repo_mutation_limit\"")
		evidencePath, ok := outputArtifactPath(state, check.ArtifactID, artifactRoot)
		if !ok {
			observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` cannot find declared evidence artifact %q\", arti...")
			return []string{fmt.Sprintf("- `%s` cannot find declared evidence artifact %q", artifact.ID, check.ArtifactID)}, nil
		}
		return validateCommandEvidenceRepoMutationLimit(artifact, artifactPath, check, evidencePath)
	}
	observe.GlobalTrace("return: nil, nil")
	return nil, nil
}

func validateMarkdownCandidateSurfacesConcrete(artifact Artifact, data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	bullets := markdownFlexibleSectionBullets(data, "Candidate Surfaces")
	if len(bullets) == 0 {
		observe.GlobalTrace("if: len(bullets) == 0")
		return []string{fmt.Sprintf("- `%s` has no Candidate Surfaces bullets", artifact.ID)}
	}
	var issues []string
	for _, bullet := range bullets {
		observe.GlobalTrace("range bullets")
		if markdownNoneBullet(bullet) {
			observe.GlobalTrace("if: markdownNoneBullet(bullet)")
			continue
		}
		surface := markdownCandidateSurfaceName(bullet)
		if surface == "" {
			observe.GlobalTrace("if: surface == \"\"")
			issues = append(issues, fmt.Sprintf("- `%s` candidate surface lacks a concrete name: %q", artifact.ID, bullet))
			continue
		}
		if markdownCandidateSurfaceHasImplementationDirective(bullet) {
			observe.GlobalTrace("if: markdownCandidateSurfaceHasImplementationDirective(bullet)")
			issues = append(issues, fmt.Sprintf("- `%s` candidate surface is phrased as an implementation instruction instead of a survey surface: %q", artifact.ID, bullet))
			continue
		}
		lowerSurface := strings.ToLower(surface)
		if strings.Contains(lowerSurface, " or ") || strings.Contains(lowerSurface, "probably") || strings.Contains(lowerSurface, "maybe") || strings.Contains(lowerSurface, "likely in") || strings.Contains(lowerSurface, "similar") {
			observe.GlobalTrace("if: guessed surface wording")
			issues = append(issues, fmt.Sprintf("- `%s` candidate surface is guessed instead of concrete: %q", artifact.ID, surface))
			continue
		}
		if invalidRepoRelativePath(surface) {
			observe.GlobalTrace("if: invalidRepoRelativePath(surface)")
			issues = append(issues, fmt.Sprintf("- `%s` candidate surface is not repo-relative: %q", artifact.ID, surface))
			continue
		}
		if markdownCandidateSurfaceLooksLikePath(surface) {
			observe.GlobalTrace("if: markdownCandidateSurfaceLooksLikePath(surface)")
			if _, err := os.Stat(surface); err != nil {
				observe.GlobalTrace("if: err != nil")
				issues = append(issues, fmt.Sprintf("- `%s` candidate surface path is not present in the current repository: %q", artifact.ID, surface))
			}
		}
	}
	observe.GlobalTrace("return: issues")
	return issues
}

func validateJSONNoUnprovenGeneratedOutputsInApprovedEditPaths(artifact Artifact, doc map[string]interface{}) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	approved, err := jsonStringArrayPath(doc, "approved_edit_paths")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return []string{fmt.Sprintf("- `%s` cannot read approved_edit_paths: %v", artifact.ID, err)}
	}
	value, _ := jsonValueAtPath(doc, "generated_policy")
	generatedPolicy := strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
	var issues []string
	for _, path := range approved {
		observe.GlobalTrace("range approved")
		path = strings.TrimSpace(path)
		if path == "" || !generatedOutputPath(path) {
			observe.GlobalTrace("if: path == \"\" || !generatedOutputPath(path)")
			continue
		}
		if generatedPolicyProvesSourceOfTruth(generatedPolicy) {
			observe.GlobalTrace("if: generatedPolicyProvesSourceOfTruth(generatedPolicy)")
			continue
		}
		issues = append(issues, fmt.Sprintf("- `%s` approved_edit_paths contains generated-looking output %q without source-of-truth proof in generated_policy; remove it from approved_edit_paths, keep it in suspected_coupled_paths, and plan producer/source-of-truth discovery or an unresolved producer blocker", artifact.ID, path))
	}
	observe.GlobalTrace("return: issues")
	return issues
}

func validateJSONWorkerTrackTargetedValidationConsistent(artifact Artifact, doc map[string]interface{}) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	workerTrack := strings.ToLower(strings.TrimSpace(fmt.Sprint(doc["worker_track"])))
	mode := strings.ToLower(strings.TrimSpace(fmt.Sprint(doc["mode"])))
	targetedValidation := strings.TrimSpace(fmt.Sprint(doc["targeted_validation"]))
	targetedLower := strings.ToLower(targetedValidation)
	if targetedValidation == "" {
		return []string{fmt.Sprintf("- `%s` has empty targeted_validation; use \"none\" or the exact targeted validation command", artifact.ID)}
	}
	if targetedLower != "none" && workerTrack == "discovery" {
		return []string{fmt.Sprintf("- `%s` has targeted_validation %q but worker_track is discovery; set worker_track to implementation so the slice routes through the implementation worker before targeted validation, or set targeted_validation to \"none\"", artifact.ID, targetedValidation)}
	}
	if mode == "validation" && targetedLower == "none" {
		return []string{fmt.Sprintf("- `%s` has mode validation but targeted_validation is none; provide the exact validation command or change mode to discovery", artifact.ID)}
	}
	if mode == "validation" && workerTrack != "implementation" {
		return []string{fmt.Sprintf("- `%s` has mode validation but worker_track is %q; validation slices must use worker_track implementation so they do not route to the discovery/no-command path", artifact.ID, workerTrack)}
	}
	return nil
}

func generatedOutputPath(path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	normalized := strings.ToLower(filepath.ToSlash(strings.TrimSpace(path)))
	base := filepath.Base(normalized)
	switch {
	case normalized == "":
		observe.GlobalTrace("case: normalized == \"\"")
		return false
	case strings.Contains(normalized, "/generated/") || strings.HasPrefix(normalized, "generated/"):
		observe.GlobalTrace("case: generated dir")
		return true
	case strings.HasSuffix(base, ".pb.go"),
		strings.HasSuffix(base, ".pb.gw.go"),
		strings.HasSuffix(base, ".gen.go"),
		strings.HasSuffix(base, "_generated.go"),
		strings.HasSuffix(base, ".generated.go"),
		strings.HasSuffix(base, ".swagger.json"):
		observe.GlobalTrace("case: generated suffix")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func generatedPolicyProvesSourceOfTruth(policy string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	policy = strings.ToLower(strings.Join(strings.Fields(policy), " "))
	if policy == "" || !strings.Contains(policy, "source of truth") {
		observe.GlobalTrace("if: missing source of truth")
		return false
	}
	for _, disqualifier := range []string{
		"manual",
		"manually",
		"unavailable",
		"not available",
		"cannot run",
		"can't run",
		"producer unavailable",
		"protoc unavailable",
		"tool unavailable",
		"workaround",
	} {
		observe.GlobalTrace("range disqualifiers")
		if strings.Contains(policy, disqualifier) {
			observe.GlobalTrace("if: strings.Contains(policy, disqualifier)")
			return false
		}
	}
	observe.GlobalTrace("return: true")
	return true
}

func validateMarkdownNoGeneratedOutputEditRecommendations(artifact Artifact, data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var issues []string
	for _, line := range strings.Split(string(data), "\n") {
		observe.GlobalTrace("range lines")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !lineReferencesGeneratedOutput(trimmed) {
			observe.GlobalTrace("if: empty or no generated output reference")
			continue
		}
		if generatedOutputEditRecommendationLine(trimmed) {
			observe.GlobalTrace("if: generatedOutputEditRecommendationLine")
			issues = append(issues, fmt.Sprintf("- `%s` recommends editing or expanding scope to a generated output instead of source-of-truth/producer repair: %q", artifact.ID, trimmed))
		}
	}
	observe.GlobalTrace("return: issues")
	return issues
}

func validateMarkdownScopeRequestRequiresNoChangedFiles(artifact Artifact, data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !markdownScopeRequestRequested(data) {
		observe.GlobalTrace("return: nil")
		return nil
	}
	var changed []string
	for _, bullet := range markdownFlexibleSectionBullets(data, "Changed files") {
		observe.GlobalTrace("range changed files")
		if markdownNoneBullet(bullet) {
			observe.GlobalTrace("if: markdownNoneBullet")
			continue
		}
		changed = append(changed, bullet)
	}
	if len(changed) == 0 {
		observe.GlobalTrace("return: nil")
		return nil
	}
	return []string{fmt.Sprintf("- `%s` requests scope expansion but also reports changed files %q; scope-request reports must leave changed files as none", artifact.ID, strings.Join(changed, "; "))}
}

func validateMarkdownScopeRequestRequiresCleanWorktree(artifact Artifact, data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !markdownScopeRequestRequested(data) {
		observe.GlobalTrace("return: nil")
		return nil
	}
	changed, err := gitChangedPaths()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return []string{fmt.Sprintf("- `%s` requests scope expansion but current worktree cleanliness could not be verified: %v", artifact.ID, err)}
	}
	if len(changed) == 0 {
		observe.GlobalTrace("return: nil")
		return nil
	}
	return []string{fmt.Sprintf("- `%s` requests scope expansion but the current worktree has changes %q; scope-request states must leave the repository clean before routing", artifact.ID, strings.Join(changed, "; "))}
}

func gitChangedPaths() ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	if output, err := rootCmd.CombinedOutput(); err != nil {
		observe.GlobalTrace("if: git rev-parse failed")
		return nil, fmt.Errorf("git rev-parse --show-toplevel failed: %s", strings.TrimSpace(string(output)))
	}
	statusCmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	output, err := statusCmd.CombinedOutput()
	if err != nil {
		observe.GlobalTrace("if: git status failed")
		return nil, fmt.Errorf("git status --porcelain --untracked-files=all failed: %s", strings.TrimSpace(string(output)))
	}
	var changed []string
	for _, line := range strings.Split(string(output), "\n") {
		observe.GlobalTrace("range git status lines")
		line = strings.TrimSpace(line)
		if line == "" {
			observe.GlobalTrace("if: line == \"\"")
			continue
		}
		if len(line) > 3 {
			changed = append(changed, strings.TrimSpace(line[3:]))
		} else {
			changed = append(changed, line)
		}
	}
	observe.GlobalTrace("return: changed, nil")
	return changed, nil
}

func markdownScopeRequestRequested(data []byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, bullet := range markdownFlexibleSectionBullets(data, "Scope request") {
		observe.GlobalTrace("range scope request")
		lower := strings.ToLower(strings.Join(strings.Fields(bullet), " "))
		if strings.Contains(lower, "status: requested") || strings.Contains(lower, "status requested") || lower == "requested" {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	for _, bullet := range markdownFlexibleSectionBullets(data, "Scope adherence") {
		observe.GlobalTrace("range scope adherence")
		lower := strings.ToLower(strings.Join(strings.Fields(bullet), " "))
		if strings.HasPrefix(lower, "forbidden_scope_touched:") && !markdownNoneBullet(strings.TrimSpace(strings.TrimPrefix(lower, "forbidden_scope_touched:"))) {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	for _, section := range []string{"Blocker", "Remaining risk", "Newly discovered coupling"} {
		observe.GlobalTrace("range scope blocker sections")
		for _, bullet := range markdownFlexibleSectionBullets(data, section) {
			observe.GlobalTrace("range scope blocker bullets")
			lower := strings.ToLower(strings.Join(strings.Fields(bullet), " "))
			if strings.Contains(lower, "scope expansion") ||
				strings.Contains(lower, "out-of-scope") ||
				strings.Contains(lower, "outside approved") ||
				strings.Contains(lower, "proto source") ||
				strings.Contains(lower, "source-of-truth") ||
				strings.Contains(lower, "producer discovery") ||
				strings.Contains(lower, "requires generated") ||
				strings.Contains(lower, "requires producer") ||
				strings.Contains(lower, "requires regeneration") {
				observe.GlobalTrace("return: true")
				return true
			}
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func validateMarkdownNoForbiddenWorkerCommands(artifact Artifact, data []byte, contractArtifactID string, contract []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !workerContractForbidsCommandFamilies(contract) {
		observe.GlobalTrace("return: nil")
		return nil
	}
	var issues []string
	for _, section := range []string{"Actions taken", "Validation run by worker"} {
		observe.GlobalTrace("range sections")
		for _, bullet := range markdownFlexibleSectionBullets(data, section) {
			observe.GlobalTrace("range bullets")
			if markdownNoneBullet(bullet) {
				observe.GlobalTrace("if: markdownNoneBullet")
				continue
			}
			if forbiddenWorkerCommandBullet(bullet) {
				observe.GlobalTrace("if: forbiddenWorkerCommandBullet")
				issues = append(issues, fmt.Sprintf("- `%s` section %q claims a producer/build/lint/test command forbidden by rendered handoff artifact %q: %q", artifact.ID, section, contractArtifactID, bullet))
			}
		}
	}
	observe.GlobalTrace("return: issues")
	return issues
}

func workerContractForbidsCommandFamilies(contract []byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(strings.Join(strings.Fields(string(contract)), " "))
	if lower == "" {
		observe.GlobalTrace("return: false")
		return false
	}
	if strings.Contains(lower, "worker must run before reporting: no producer, build, lint, or test commands") {
		observe.GlobalTrace("return: true")
		return true
	}
	if strings.Contains(lower, "worker must not run producer, build, lint, or test commands") {
		observe.GlobalTrace("return: true")
		return true
	}
	if strings.Contains(lower, "no producer/build/lint/test commands") {
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: false")
	return false
}

func forbiddenWorkerCommandBullet(bullet string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(strings.Join(strings.Fields(bullet), " "))
	if lower == "" {
		observe.GlobalTrace("return: false")
		return false
	}
	if strings.Contains(lower, "none") && !strings.Contains(lower, "status") && !strings.Contains(lower, "command") {
		observe.GlobalTrace("return: false")
		return false
	}
	for _, phrase := range []string{
		"go build",
		"go test",
		"go generate",
		"git checkout",
		"git restore",
		"git reset",
		"git clean",
		"git show head:",
		"git cat-file",
		"protoc",
		"buf generate",
		"buf lint",
		"npm test",
		"npm run test",
		"npm run lint",
		"pnpm test",
		"pnpm lint",
		"yarn test",
		"yarn lint",
		"pytest",
		"cargo test",
		"cargo build",
		"mvn test",
		"gradle test",
		"make test",
		"make lint",
	} {
		observe.GlobalTrace("range phrases")
		if strings.Contains(lower, phrase) {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	for _, phrase := range []string{
		"build command",
		"test command",
		"lint command",
		"producer command",
		"generator command",
		"validation command",
		"rollback command",
		"history restore",
		"history-restoring",
		"ran build",
		"ran test",
		"ran lint",
		"ran producer",
		"ran generator",
	} {
		observe.GlobalTrace("range generic phrases")
		if strings.Contains(lower, phrase) {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func lineReferencesGeneratedOutput(line string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(line)
	if strings.Contains(lower, "generated .pb.go") || strings.Contains(lower, "generated file") || strings.Contains(lower, "generated output") {
		observe.GlobalTrace("if: generated phrase")
		return true
	}
	normalized := strings.NewReplacer(
		"`", " ",
		"\"", " ",
		"'", " ",
		"[", " ",
		"]", " ",
		"(", " ",
		")", " ",
		"{", " ",
		"}", " ",
		",", " ",
		";", " ",
	).Replace(line)
	for _, token := range strings.Fields(normalized) {
		observe.GlobalTrace("range tokens")
		token = strings.Trim(token, ".:")
		if generatedOutputPath(token) {
			observe.GlobalTrace("if: generatedOutputPath(token)")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func generatedOutputEditRecommendationLine(line string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(strings.Join(strings.Fields(line), " "))
	if generatedOutputEditRecommendationNegated(lower) {
		observe.GlobalTrace("if: generatedOutputEditRecommendationNegated")
		return false
	}
	for _, phrase := range []string{
		"manual",
		"manually",
		"hand-edit",
		"hand edit",
		"scope expansion",
		"scope request",
		"request scope",
		"requires editing",
		"requires edit",
		"required before",
		"required to add",
		"must edit",
		"need to edit",
		"needs edit",
		"edit scope",
		"adding ",
		"add ",
	} {
		observe.GlobalTrace("range recommendation phrases")
		if strings.Contains(lower, phrase) {
			observe.GlobalTrace("if: strings.Contains(lower, phrase)")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func generatedOutputEditRecommendationNegated(line string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, phrase := range []string{
		"do not edit",
		"must not edit",
		"not edit",
		"not a valid",
		"not valid",
		"forbid",
		"forbidden",
		"without editing",
		"no generated",
		"not permit",
		"not permitted",
		"not approved",
		"unapproved",
	} {
		observe.GlobalTrace("range negated phrases")
		if strings.Contains(line, phrase) {
			observe.GlobalTrace("if: strings.Contains(line, phrase)")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func markdownFlexibleSectionBullets(data []byte, section string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	bullets := markdownSectionBullets(data, section)
	if len(bullets) > 0 {
		observe.GlobalTrace("if: len(bullets) > 0")
		return bullets
	}
	want := strings.ToLower(strings.TrimSpace(section))
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		observe.GlobalTrace("range strings.Split(string(data), \"\\n\")")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			observe.GlobalTrace("if: trimmed == \"\"")
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			observe.GlobalTrace("if: strings.HasPrefix(trimmed, \"#\")")
			name := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			inSection = strings.EqualFold(name, section)
			continue
		}
		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			observe.GlobalTrace("if: strings.HasSuffix(trimmed, \":\") && !strings.HasPrefix(trimmed, \"- \")")
			name := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(trimmed, ":")))
			if inSection && name != want {
				observe.GlobalTrace("if: inSection && name != want")
				break
			}
			inSection = name == want
			continue
		}
		if !inSection {
			observe.GlobalTrace("if: !inSection")
			continue
		}
		if strings.HasPrefix(trimmed, "- ") {
			observe.GlobalTrace("if: strings.HasPrefix(trimmed, \"- \")")
			bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			observe.GlobalTrace("if: !strings.HasPrefix(line, \" \") && !strings.HasPrefix(line, \"\\t\")")
			break
		}
	}
	observe.GlobalTrace("return: bullets")
	return bullets
}

func markdownCandidateSurfaceName(bullet string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	bullet = strings.TrimSpace(bullet)
	if before, _, ok := strings.Cut(bullet, " - "); ok {
		observe.GlobalTrace("if: cut hyphen")
		bullet = before
	}
	bullet = strings.TrimSpace(strings.Trim(bullet, "`\"'"))
	observe.GlobalTrace("return: bullet")
	return bullet
}

func markdownCandidateSurfaceLooksLikePath(surface string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	surface = strings.TrimSpace(surface)
	if surface == "" {
		observe.GlobalTrace("if: surface == \"\"")
		return false
	}
	if strings.ContainsAny(surface, "*?[]{}") {
		observe.GlobalTrace("if: contains glob")
		return false
	}
	observe.GlobalTrace("return: strings.Contains(surface, \"/\") || strings.Contains(filepath.Base(surface), \".\")")
	return strings.Contains(surface, "/") || strings.Contains(filepath.Base(surface), ".")
}

func markdownCandidateSurfaceHasImplementationDirective(bullet string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	_, description, ok := strings.Cut(bullet, " - ")
	if !ok {
		observe.GlobalTrace("if: !ok")
		return false
	}
	description = strings.ToLower(description)
	padded := " " + description + " "
	for _, directive := range []string{
		" add ",
		" create ",
		" modify ",
		" register ",
		" wire ",
		" implement ",
		" fix ",
		" needed here",
		" needs ",
		" should be added",
		" should add",
	} {
		observe.GlobalTrace("range directives")
		if strings.Contains(padded, directive) {
			observe.GlobalTrace("if: strings.Contains(padded, directive)")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func validateMarkdownConstraintsSupportedByClaims(artifact Artifact, data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	claims := markdownClaimAdjudications(data)
	if len(claims) == 0 {
		observe.GlobalTrace("if: len(claims) == 0")
		observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` has no Claim adjudication rows\", artifact.ID)}")
		return []string{fmt.Sprintf("- `%s` has no Claim adjudication rows", artifact.ID)}
	}
	constraints := markdownSectionBullets(data, "Planning Constraints")
	var issues []string
	for _, constraint := range constraints {
		observe.GlobalTrace("range constraints")
		if markdownNoneBullet(constraint) {
			observe.GlobalTrace("if: markdownNoneBullet(constraint)")
			continue
		}
		_, support, ok := strings.Cut(constraint, "support=")
		if !ok || strings.TrimSpace(support) == "" {
			observe.GlobalTrace("if: !ok || strings.TrimSpace(support) == \"\"")
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint lacks support=: %q", artifact.ID, constraint))
			continue
		}
		support = normalizeMarkdownSupport(support)
		if support == "" || support == "none" {
			observe.GlobalTrace("if: support == \"\" || support == \"none\"")
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint has empty support: %q", artifact.ID, constraint))
			continue
		}
		claim, ok := supportMatchesClaim(support, claims)
		if !ok {
			observe.GlobalTrace("if: !ok")
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint support %q does not match any Claim adjudication row", artifact.ID, support))
			continue
		}
		switch claim.Classification {
		case "observed", "inferred":
			observe.GlobalTrace("case: \"observed\", \"inferred\"")
		default:
			observe.GlobalTrace("default")
			if claim.Classification == "" {
				issues = append(issues, fmt.Sprintf("- `%s` planning constraint support %q matches a Claim adjudication row without classification", artifact.ID, support))
				continue
			}
			issues = append(issues, fmt.Sprintf("- `%s` planning constraint support %q matches %s claim; only observed or inferred claims may support constraints", artifact.ID, support, claim.Classification))
		}
	}
	observe.GlobalTrace("return: issues")
	return issues
}

func validateMarkdownValidationCoverageConsistent(artifact Artifact, data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	planned, plannedOK := textListField(data, "planned_acceptance_ids")
	validated, validatedOK := textListField(data, "validated_acceptance_ids")
	insufficient, insufficientOK := textListField(data, "insufficient_acceptance_ids")
	validated = nonNoneTextListValues(validated)
	insufficient = nonNoneTextListValues(insufficient)
	result := markdownSectionFirstBullet(data, "Result")
	result = strings.ToLower(strings.TrimSpace(result))
	var issues []string
	if !plannedOK {
		observe.GlobalTrace("if: !plannedOK")
		issues = append(issues, fmt.Sprintf("- `%s` missing planned_acceptance_ids", artifact.ID))
	}
	if !validatedOK {
		observe.GlobalTrace("if: !validatedOK")
		issues = append(issues, fmt.Sprintf("- `%s` missing validated_acceptance_ids", artifact.ID))
	}
	if !insufficientOK {
		observe.GlobalTrace("if: !insufficientOK")
		issues = append(issues, fmt.Sprintf("- `%s` missing insufficient_acceptance_ids", artifact.ID))
	}
	if result == "" {
		observe.GlobalTrace("if: result == \"\"")
		issues = append(issues, fmt.Sprintf("- `%s` missing Result section value", artifact.ID))
	}
	if len(planned) == 0 {
		observe.GlobalTrace("if: len(planned) == 0")
		return issues
	}
	if len(validated) == 0 && len(insufficient) == 0 {
		observe.GlobalTrace("if: len(validated) == 0 && len(insufficient) == 0")
		issues = append(issues, fmt.Sprintf("- `%s` names planned_acceptance_ids but lists neither validated_acceptance_ids nor insufficient_acceptance_ids", artifact.ID))
	}
	switch result {
	case "pass":
		observe.GlobalTrace("case: \"pass\"")
		missing := stringSliceDifference(planned, validated)
		if len(missing) > 0 {
			observe.GlobalTrace("if: len(missing) > 0")
			issues = append(issues, fmt.Sprintf("- `%s` result is pass but validated_acceptance_ids omits planned IDs: %s", artifact.ID, strings.Join(missing, ", ")))
		}
		if weak := weaklyValidatedAcceptanceIDs(data, validated); len(weak) > 0 {
			issues = append(issues, fmt.Sprintf("- `%s` result is pass but compile/static commands cannot prove behavior-sensitive acceptance IDs: %s", artifact.ID, strings.Join(weak, ", ")))
		}
	case "fail", "failed", "insufficient", "not_applicable":
		observe.GlobalTrace("case: non-pass")
		if len(insufficient) == 0 {
			observe.GlobalTrace("if: len(insufficient) == 0")
			issues = append(issues, fmt.Sprintf("- `%s` result is %s but insufficient_acceptance_ids is empty despite planned_acceptance_ids", artifact.ID, result))
		}
	}
	observe.GlobalTrace("return: issues")
	return issues
}

func weaklyValidatedAcceptanceIDs(data []byte, validated []string) []string {
	commands := markdownFlexibleSectionBullets(data, "Commands run")
	if len(commands) == 0 || !onlyCompileOrStaticValidationCommands(commands) {
		return nil
	}
	var weak []string
	for _, id := range validated {
		upper := strings.ToUpper(strings.TrimSpace(id))
		if upper == "" || upper == "NONE" {
			continue
		}
		if acceptanceIDRequiresBehaviorValidation(upper) {
			weak = append(weak, id)
		}
	}
	return weak
}

func onlyCompileOrStaticValidationCommands(commands []string) bool {
	sawCommand := false
	for _, command := range commands {
		normalized := strings.ToLower(strings.Join(strings.Fields(command), " "))
		normalized = strings.TrimPrefix(normalized, "`")
		normalized = strings.TrimSuffix(normalized, "`")
		if normalized == "" || markdownNoneBullet(normalized) {
			continue
		}
		sawCommand = true
		switch {
		case strings.Contains(normalized, "go build"),
			strings.Contains(normalized, "go vet"),
			strings.Contains(normalized, "go list"),
			strings.Contains(normalized, "grep "),
			strings.Contains(normalized, " rg "),
			strings.HasPrefix(normalized, "rg "),
			strings.Contains(normalized, "cat "),
			strings.Contains(normalized, "sed "),
			strings.Contains(normalized, "source inspection"),
			strings.Contains(normalized, "generated-symbol"),
			strings.Contains(normalized, "symbol presence"):
			continue
		default:
			return false
		}
	}
	return sawCommand
}

func acceptanceIDRequiresBehaviorValidation(id string) bool {
	for _, marker := range []string{
		"DEFAULT",
		"VALIDATION",
		"FRAMEWORK",
		"INTEGRATION",
		"TOKEN",
		"ERROR",
		"DEPLOYMENT",
		"INTROSPECTION",
		"BACKWARD",
		"COMPAT",
		"RUNTIME",
		"API",
		"CONFIG-VALIDATION",
	} {
		if strings.Contains(id, marker) {
			return true
		}
	}
	return false
}

type markdownClaimAdjudication struct {
	Claim          string
	Classification string
}

func markdownClaimAdjudications(data []byte) []markdownClaimAdjudication {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var claims []markdownClaimAdjudication
	for _, bullet := range markdownSectionBullets(data, "Claim Adjudication") {
		observe.GlobalTrace("range markdownSectionBullets(data, \"Claim Adjudication\")")
		if markdownNoneBullet(bullet) {
			observe.GlobalTrace("if: markdownNoneBullet(bullet)")
			continue
		}
		fields := markdownSemicolonFields(bullet)
		claim := normalizeMarkdownSupport(fields["claim"])
		if claim != "" {
			observe.GlobalTrace("if: claim != \"\"")
			claims = append(claims, markdownClaimAdjudication{
				Claim:          claim,
				Classification: strings.ToLower(normalizeMarkdownSupport(fields["classification"])),
			})
		}
	}
	observe.GlobalTrace("return: claims")
	return claims
}

func markdownSemicolonFields(text string) map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := make(map[string]string)
	for _, part := range strings.Split(text, ";") {
		observe.GlobalTrace("range strings.Split(text, \";\")")
		key, value, ok := strings.Cut(part, ":")
		if !ok {
			observe.GlobalTrace("if: !ok")
			continue
		}
		key = strings.ToLower(normalizeMarkdownSupport(key))
		if key == "" {
			observe.GlobalTrace("if: key == \"\"")
			continue
		}
		fields[key] = strings.TrimSpace(value)
	}
	observe.GlobalTrace("return: fields")
	return fields
}

func supportMatchesClaim(support string, claims []markdownClaimAdjudication) (markdownClaimAdjudication, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	support = normalizeMarkdownSupport(strings.TrimPrefix(support, "claim "))
	for _, claim := range claims {
		observe.GlobalTrace("range claims")
		claimText := normalizeMarkdownSupport(claim.Claim)
		if support == claimText || strings.Contains(claimText, support) || strings.Contains(support, claimText) {
			observe.GlobalTrace("if: support == claimText || strings.Contains(claimText, support) || strings.Conta...")
			observe.GlobalTrace("return: claim, true")
			return claim, true
		}
	}
	observe.GlobalTrace("return: markdownClaimAdjudication{}, false")
	return markdownClaimAdjudication{}, false
}

func markdownSectionBullets(data []byte, section string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var bullets []string
	inSection := false
	for _, line := range strings.Split(string(data), "\n") {
		observe.GlobalTrace("range strings.Split(string(data), \"\\n\")")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			observe.GlobalTrace("if: strings.HasPrefix(trimmed, \"## \")")
			inSection = strings.TrimSpace(strings.TrimPrefix(trimmed, "## ")) == section
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "- ") {
			observe.GlobalTrace("if: !inSection || !strings.HasPrefix(trimmed, \"- \")")
			continue
		}
		bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
	}
	observe.GlobalTrace("return: bullets")
	return bullets
}

func markdownSectionFirstBullet(data []byte, section string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, bullet := range markdownFlexibleSectionBullets(data, section) {
		observe.GlobalTrace("range markdownFlexibleSectionBullets")
		if strings.TrimSpace(bullet) != "" {
			observe.GlobalTrace("return: bullet")
			return strings.TrimSpace(bullet)
		}
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func markdownNoneBullet(text string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch strings.ToLower(strings.Trim(strings.TrimSpace(text), "`\"'")) {
	case "", "none", "n/a", "na", "not_applicable":
		observe.GlobalTrace("case: \"\", \"none\", \"n/a\", \"na\", \"not_applicable\"")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func normalizeMarkdownSupport(text string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`\"'")
	if idx := strings.IndexAny(text, "\r\n;"); idx >= 0 {
		observe.GlobalTrace("if: idx >= 0")
		text = text[:idx]
	}
	observe.GlobalTrace("return: strings.ToLower(strings.Join(strings.Fields(text), \" \"))")
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func readJSONObject(path string) (map[string]interface{}, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: readJSONObjectBytes(data)")
	return readJSONObjectBytes(data)
}

func readJSONObjectBytes(data []byte) (map[string]interface{}, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: doc, nil")
	return doc, nil
}

func readJSONArrayField(path string, field string) ([]map[string]interface{}, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	doc, err := readJSONObject(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: jsonArrayField(doc, field)")
	return jsonArrayField(doc, field)
}

func readJSONArrayFieldBytes(data []byte, field string) ([]map[string]interface{}, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: jsonArrayField(doc, field)")
	return jsonArrayField(doc, field)
}

func jsonArrayField(doc map[string]interface{}, field string) ([]map[string]interface{}, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value, ok := doc[field]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"missing JSON array field %q\", field)")
		return nil, fmt.Errorf("missing JSON array field %q", field)
	}
	rawItems, ok := value.([]interface{})
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"JSON field %q is not an array\", field)")
		return nil, fmt.Errorf("JSON field %q is not an array", field)
	}
	items := make([]map[string]interface{}, 0, len(rawItems))
	for idx, raw := range rawItems {
		observe.GlobalTrace("range rawItems")
		item, ok := raw.(map[string]interface{})
		if !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"JSON field %q item %d is not an object\", field, idx)")
			return nil, fmt.Errorf("JSON field %q item %d is not an object", field, idx)
		}
		items = append(items, item)
	}
	observe.GlobalTrace("return: items, nil")
	return items, nil
}

func jsonStringArrayPath(doc map[string]interface{}, fieldPath string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value, ok := jsonValueAtPath(doc, fieldPath)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"missing JSON field %q\", fieldPath)")
		return nil, fmt.Errorf("missing JSON field %q", fieldPath)
	}
	switch typed := value.(type) {
	case []interface{}:
		observe.GlobalTrace("typecase: []interface{}")
		out := make([]string, 0, len(typed))
		for idx, raw := range typed {
			text, ok := raw.(string)
			if !ok {
				observe.GlobalTrace("if: !ok")
				observe.GlobalTrace("return: nil, fmt.Errorf(\"JSON field %q item %d is not a string\", fieldPath, idx)")
				return nil, fmt.Errorf("JSON field %q item %d is not a string", fieldPath, idx)
			}
			out = append(out, text)
		}
		return out, nil
	case []string:
		observe.GlobalTrace("typecase: []string")
		return append([]string(nil), typed...), nil
	case string:
		observe.GlobalTrace("typecase: string")
		if strings.TrimSpace(typed) == "" {
			observe.GlobalTrace("return: nil, nil")
			return nil, nil
		}
		return []string{typed}, nil
	default:
		observe.GlobalTrace("typedefault")
		return nil, fmt.Errorf("JSON field %q is not a string array", fieldPath)
	}
}

func jsonValueAtPath(doc map[string]interface{}, fieldPath string) (interface{}, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	current := interface{}(doc)
	for _, part := range strings.Split(fieldPath, ".") {
		observe.GlobalTrace("range strings.Split(fieldPath, \".\")")
		part = strings.TrimSpace(part)
		if part == "" {
			observe.GlobalTrace("if: part == \"\"")
			observe.GlobalTrace("return: nil, false")
			return nil, false
		}
		object, ok := current.(map[string]interface{})
		if !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: nil, false")
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: nil, false")
			return nil, false
		}
	}
	observe.GlobalTrace("return: current, true")
	return current, true
}

func jsonValueEmpty(value interface{}) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch typed := value.(type) {
	case nil:
		observe.GlobalTrace("typecase: nil")
		return true
	case string:
		observe.GlobalTrace("typecase: string")
		return strings.TrimSpace(typed) == ""
	case []interface{}:
		observe.GlobalTrace("typecase: []interface{}")
		return len(typed) == 0
	case []string:
		observe.GlobalTrace("typecase: []string")
		return len(typed) == 0
	case map[string]interface{}:
		observe.GlobalTrace("typecase: map[string]interface{}")
		return len(typed) == 0
	default:
		observe.GlobalTrace("typedefault")
		return false
	}
}

func textListField(data []byte, field string) ([]string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	field = strings.TrimSpace(field)
	if field == "" {
		observe.GlobalTrace("if: field == \"\"")
		observe.GlobalTrace("return: nil, false")
		return nil, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		observe.GlobalTrace("range strings.Split(string(data), \"\\n\")")
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != field {
			observe.GlobalTrace("if: !ok || strings.TrimSpace(key) != field")
			continue
		}
		observe.GlobalTrace("return: parseTextListValue(value), true")
		return parseTextListValue(value), true
	}
	observe.GlobalTrace("return: nil, false")
	return nil, false
}

func parseTextListValue(value string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "none", "null", "nil", "n/a", "na", "not_applicable":
		observe.GlobalTrace("case: \"\", \"none\", \"null\", \"nil\", \"n/a\", \"na\", \"not_applicable\"")
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "`\"'")
		if part != "" {
			observe.GlobalTrace("if: part != \"\"")
			out = append(out, part)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

func stringSliceDifference(want []string, have []string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	haveSet := make(map[string]struct{}, len(have))
	for _, value := range have {
		observe.GlobalTrace("range have")
		haveSet[strings.TrimSpace(value)] = struct{}{}
	}
	var missing []string
	for _, value := range want {
		observe.GlobalTrace("range want")
		value = strings.TrimSpace(value)
		if value == "" {
			observe.GlobalTrace("if: value == \"\"")
			continue
		}
		if _, ok := haveSet[value]; !ok {
			observe.GlobalTrace("if: !ok")
			missing = append(missing, value)
		}
	}
	observe.GlobalTrace("return: missing")
	return missing
}

func nonNoneTextListValues(values []string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]string, 0, len(values))
	for _, value := range values {
		observe.GlobalTrace("range values")
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" || normalized == "none" || strings.HasPrefix(normalized, "none ") || strings.HasPrefix(normalized, "none -") || normalized == "n/a" || normalized == "na" || normalized == "not_applicable" {
			observe.GlobalTrace("if: none-like")
			continue
		}
		out = append(out, value)
	}
	observe.GlobalTrace("return: out")
	return out
}

func artifactItemLabel(item map[string]interface{}, idx int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	id := jsonStringField(item, "id")
	if id == "" {
		observe.GlobalTrace("if: id == \"\"")
		observe.GlobalTrace("return: fmt.Sprintf(\"item[%d]\", idx)")
		return fmt.Sprintf("item[%d]", idx)
	}
	observe.GlobalTrace("return: id")
	return id
}

func jsonStringField(item map[string]interface{}, field string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value, ok := item[field]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	text, ok := value.(string)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: text")
	return text
}

func jsonStringArrayField(item map[string]interface{}, field string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	value, ok := item[field]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}
	rawItems, ok := value.([]interface{})
	if !ok {
		observe.GlobalTrace("if: !ok")
		if text, ok := value.(string); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: []string{text}")
			return []string{text}
		}
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := make([]string, 0, len(rawItems))
	for _, raw := range rawItems {
		observe.GlobalTrace("range rawItems")
		if text, ok := raw.(string); ok {
			observe.GlobalTrace("if: ok")
			out = append(out, text)
		}
	}
	observe.GlobalTrace("return: out")
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
	RepoMutation       bool   `json:"repo_mutation"`
}

func validateCommandEvidenceSupport(artifact Artifact, reportPath string, check ArtifactIntegrityCheck, evidencePath string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	evidenceData, err := os.ReadFile(evidencePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: []string{fmt.Sprintf(\"- `%s` has no runtime command evidence artifact at `%s`...")
		return []string{fmt.Sprintf("- `%s` has no runtime command evidence artifact at `%s`: %v", artifact.ID, evidencePath, err)}, nil
	}
	report := string(reportData)
	records, parseIssues := parseCommandEvidence(evidenceData, check.ArtifactID)
	var issues []string
	issues = append(issues, parseIssues...)
	if len(records) == 0 {
		observe.GlobalTrace("if: len(records) == 0")
		issues = append(issues, fmt.Sprintf("- `%s` has an empty runtime command evidence artifact", artifact.ID))
		observe.GlobalTrace("return: issues, nil")
		return issues, nil
	}
	var priorEvidence int
	var validationEvidence int
	for _, record := range records {
		observe.GlobalTrace("range records")
		if !record.CompletionSentinel && !record.WritesReport {
			observe.GlobalTrace("if: !record.CompletionSentinel && !record.WritesReport")
			priorEvidence++
		}
		if !record.WritesReport && strings.TrimSpace(record.OutputSHA256) != "" {
			observe.GlobalTrace("if: !record.WritesReport && strings.TrimSpace(record.OutputSHA256) != \"\"")
			validationEvidence++
		}
	}
	if priorEvidence == 0 {
		observe.GlobalTrace("if: priorEvidence == 0")
		issues = append(issues, fmt.Sprintf("- `%s` was submitted without any prior non-completion command evidence in this state", artifact.ID))
	}
	for _, section := range check.Sections {
		observe.GlobalTrace("range check.Sections")
		if sectionHasNonNone(report, section) && priorEvidence == 0 {
			observe.GlobalTrace("if: sectionHasNonNone(report, section) && priorEvidence == 0")
			issues = append(issues, fmt.Sprintf("- `%s` section %q is non-empty, but runtime evidence has no prior non-completion command", artifact.ID, section))
		}
		if strings.Contains(strings.ToLower(section), "validation") && sectionHasNonNone(report, section) && validationEvidence == 0 {
			observe.GlobalTrace("if: strings.Contains(strings.ToLower(section), \"validation\") && sectionHasNonNone...")
			issues = append(issues, fmt.Sprintf("- `%s` section %q is non-empty, but runtime evidence has no command output hash", artifact.ID, section))
		}
	}
	observe.GlobalTrace("return: issues, nil")
	return issues, nil
}

func validateCommandEvidenceRepoMutationLimit(artifact Artifact, reportPath string, check ArtifactIntegrityCheck, evidencePath string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	limit, err := strconv.Atoi(strings.TrimSpace(check.Value))
	if err != nil || limit < 0 {
		observe.GlobalTrace("if: err != nil || limit < 0")
		return []string{fmt.Sprintf("- `%s` has invalid repo mutation limit %q", artifact.ID, check.Value)}, nil
	}
	evidenceData, err := os.ReadFile(evidencePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return []string{fmt.Sprintf("- `%s` has no runtime command evidence artifact at `%s`: %v", artifact.ID, evidencePath, err)}, nil
	}
	records, parseIssues := parseCommandEvidence(evidenceData, check.ArtifactID)
	issues := append([]string(nil), parseIssues...)
	var mutations int
	var previews []string
	for _, record := range records {
		observe.GlobalTrace("range records")
		if !record.RepoMutation || record.WritesReport {
			observe.GlobalTrace("if: !record.RepoMutation || record.WritesReport")
			continue
		}
		mutations++
		if strings.TrimSpace(record.CommandPreview) != "" && len(previews) < 3 {
			observe.GlobalTrace("if: preview")
			previews = append(previews, record.CommandPreview)
		}
	}
	if mutations > limit {
		observe.GlobalTrace("if: mutations > limit")
		reportData, readErr := os.ReadFile(reportPath)
		if readErr != nil {
			return nil, readErr
		}
		if markdownExplicitBlockerReport(reportData) {
			observe.GlobalTrace("if: markdownExplicitBlockerReport")
			return issues, nil
		}
		issues = append(issues, fmt.Sprintf("- `%s` runtime command evidence has %d repository mutation commands, exceeding limit %d. Stop repairing through repeated source mutations. If the source may be malformed or the edit cannot be completed within the mutation budget, rewrite the worker report as an explicit blocker, list changed files truthfully, do not claim acceptance coverage, and submit again. Examples: %s", artifact.ID, mutations, limit, strings.Join(previews, "; ")))
	}
	observe.GlobalTrace("return: issues, nil")
	return issues, nil
}

func validateCommandEvidenceNonReportLimit(artifact Artifact, check ArtifactIntegrityCheck, evidencePath string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	limit, err := strconv.Atoi(strings.TrimSpace(check.Value))
	if err != nil || limit < 0 {
		observe.GlobalTrace("if: err != nil || limit < 0")
		return []string{fmt.Sprintf("- `%s` has invalid non-report command limit %q", artifact.ID, check.Value)}, nil
	}
	evidenceData, err := os.ReadFile(evidencePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return []string{fmt.Sprintf("- `%s` has no runtime command evidence artifact at `%s`: %v", artifact.ID, evidencePath, err)}, nil
	}
	records, parseIssues := parseCommandEvidence(evidenceData, check.ArtifactID)
	issues := append([]string(nil), parseIssues...)
	var count int
	var previews []string
	for _, record := range records {
		observe.GlobalTrace("range records")
		if record.WritesReport || record.CompletionSentinel {
			observe.GlobalTrace("if: report or completion")
			continue
		}
		count++
		if strings.TrimSpace(record.CommandPreview) != "" && len(previews) < 4 {
			observe.GlobalTrace("if: preview")
			previews = append(previews, record.CommandPreview)
		}
	}
	if count > limit {
		observe.GlobalTrace("if: count > limit")
		issues = append(issues, fmt.Sprintf("- `%s` runtime command evidence has %d non-report inspection commands, exceeding limit %d. Discovery workers must run one bounded inspection command at most, then write the report from that evidence or a blocker. Examples: %s", artifact.ID, count, limit, strings.Join(previews, "; ")))
	}
	return issues, nil
}

func markdownExplicitBlockerReport(data []byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var blocker bool
	for _, bullet := range markdownFlexibleSectionBullets(data, "Blocker") {
		observe.GlobalTrace("range blocker")
		if !markdownNoneBullet(bullet) {
			blocker = true
			break
		}
	}
	if !blocker {
		observe.GlobalTrace("if: !blocker")
		return false
	}
	for _, bullet := range markdownFlexibleSectionBullets(data, "Acceptance coverage") {
		observe.GlobalTrace("range acceptance coverage")
		lower := strings.ToLower(strings.Join(strings.Fields(bullet), " "))
		if strings.Contains(lower, "acceptance_ids_addressed") && !strings.Contains(lower, "none") {
			observe.GlobalTrace("if: acceptance_ids_addressed non-none")
			return false
		}
		if strings.Contains(lower, "validated_acceptance_ids") && !strings.Contains(lower, "none") {
			observe.GlobalTrace("if: validated_acceptance_ids non-none")
			return false
		}
	}
	return true
}

func validateCommandEvidenceClaimedChanges(artifact Artifact, reportPath string, check ArtifactIntegrityCheck, evidencePath string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return nil, err
	}
	claimed := markdownClaimedChangedFiles(reportData)
	if len(claimed) == 0 {
		observe.GlobalTrace("if: len(claimed) == 0")
		return nil, nil
	}
	evidenceData, err := os.ReadFile(evidencePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return []string{fmt.Sprintf("- `%s` has no runtime command evidence artifact at `%s`: %v", artifact.ID, evidencePath, err)}, nil
	}
	records, parseIssues := parseCommandEvidence(evidenceData, check.ArtifactID)
	issues := append([]string(nil), parseIssues...)
	var mutations int
	var previews []string
	for _, record := range records {
		observe.GlobalTrace("range records")
		if !record.RepoMutation || record.WritesReport {
			observe.GlobalTrace("if: !record.RepoMutation || record.WritesReport")
			continue
		}
		mutations++
		if strings.TrimSpace(record.CommandPreview) != "" && len(previews) < 3 {
			observe.GlobalTrace("if: preview")
			previews = append(previews, record.CommandPreview)
		}
	}
	if mutations == 0 {
		observe.GlobalTrace("if: mutations == 0")
		issues = append(issues, fmt.Sprintf("- `%s` reports changed files %q but runtime command evidence has no non-report repository mutation command. Re-read the files and either apply the approved edit with a command that actually mutates the repo, or change the report to a blocker/scope request with changed files set to none.", artifact.ID, strings.Join(claimed, "; ")))
	}
	observe.GlobalTrace("return: issues, nil")
	return issues, nil
}

func markdownClaimedChangedFiles(data []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var changed []string
	for _, bullet := range markdownFlexibleSectionBullets(data, "Changed files") {
		observe.GlobalTrace("range changed files")
		if markdownNoneBullet(bullet) {
			observe.GlobalTrace("if: markdownNoneBullet")
			continue
		}
		changed = append(changed, bullet)
	}
	observe.GlobalTrace("return: changed")
	return changed
}

func validateMarkdownWorkerBlockerValidationNoCommands(artifact Artifact, validationData []byte, workerArtifactID string, workerData []byte) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !markdownExplicitBlockerReport(workerData) {
		observe.GlobalTrace("if: !markdownExplicitBlockerReport")
		return nil
	}
	var commands []string
	for _, bullet := range markdownFlexibleSectionBullets(validationData, "Commands run") {
		observe.GlobalTrace("range commands")
		if markdownNoneBullet(bullet) {
			continue
		}
		commands = append(commands, bullet)
	}
	if len(commands) == 0 {
		observe.GlobalTrace("if: len(commands) == 0")
		return nil
	}
	return []string{fmt.Sprintf("- `%s` ran validation commands even though worker report %q is an explicit blocker. Record the blocker as insufficient evidence with Commands run set to none; do not run build/test/producer commands after a blocker report. Commands: %s", artifact.ID, workerArtifactID, strings.Join(commands, "; "))}
}

func parseCommandEvidence(data []byte, label string) ([]commandEvidenceRecord, []string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var records []commandEvidenceRecord
	var issues []string
	lines := strings.Split(string(data), "\n")
	for idx, line := range lines {
		observe.GlobalTrace("range lines")
		line = strings.TrimSpace(line)
		if line == "" {
			observe.GlobalTrace("if: line == \"\"")
			continue
		}
		var record commandEvidenceRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			observe.GlobalTrace("if: err != nil")
			issues = append(issues, fmt.Sprintf("- %s line %d is invalid JSON: %v", label, idx+1, err))
			continue
		}
		if record.CommandSHA256 == "" || record.OutputSHA256 == "" {
			observe.GlobalTrace("if: record.CommandSHA256 == \"\" || record.OutputSHA256 == \"\"")
			issues = append(issues, fmt.Sprintf("- %s line %d is missing command/output hash", label, idx+1))
		}
		records = append(records, record)
	}
	observe.GlobalTrace("return: records, issues")
	return records, issues
}

func sectionHasNonNone(text string, heading string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	idx := strings.Index(text, heading)
	if idx < 0 {
		observe.GlobalTrace("if: idx < 0")
		observe.GlobalTrace("return: false")
		return false
	}
	section := text[idx+len(heading):]
	if next := strings.Index(section, "\n\n"); next >= 0 {
		observe.GlobalTrace("if: next >= 0")
		section = section[:next]
	}
	section = strings.TrimSpace(section)
	if section == "" {
		observe.GlobalTrace("if: section == \"\"")
		observe.GlobalTrace("return: false")
		return false
	}
	lower := strings.ToLower(section)
	observe.GlobalTrace("return: strings.Contains(section, \"-\") && !strings.Contains(lower, \"- none\") && lower...")
	return strings.Contains(section, "-") && !strings.Contains(lower, "- none") && lower != "none"
}

func invalidRepoRelativePath(path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	path = strings.TrimSpace(path)
	if path == "" || path == "unknown" {
		observe.GlobalTrace("if: path == \"\" || path == \"unknown\"")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: strings.HasPrefix(path, \"/\") || strings.HasPrefix(path, \"~\")")
	return strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~")
}

func invalidValidationPath(validation string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := strings.Fields(validation)
	for _, field := range fields {
		observe.GlobalTrace("range fields")
		field = strings.Trim(field, "`'\",;)")
		if strings.HasPrefix(field, "/tmp/pragma/") {
			observe.GlobalTrace("if: strings.HasPrefix(field, \"/tmp/pragma/\")")
			continue
		}
		if filepath.IsAbs(field) || strings.HasPrefix(field, "~") {
			observe.GlobalTrace("if: filepath.IsAbs(field) || strings.HasPrefix(field, \"~\")")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func placeholderValidationCommand(validation string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	validation = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(validation), "command:")))
	validation = strings.Trim(validation, "`'\".; ")
	switch validation {
	case "", "unknown", "n/a", "na", "none", "todo", "tbd", "manual":
		observe.GlobalTrace("case: \"\", \"unknown\", \"n/a\", \"na\", \"none\", \"todo\", \"tbd\", \"manual\"")
		return true
	default:
		observe.GlobalTrace("default")
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
		observe.GlobalTrace("if: blockedStatus == \"\"")
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
			observe.GlobalTrace("case: \"persona\"")
			fmt.Fprintf(&b, "Its `%s` persona will write the selected-item handoff to `%s` after selection.\n", next.Persona, control.HandoffPath)
			b.WriteString("Do not write a separate generic next-item handoff; the next state's persona owns that handoff after the cursor is selected.\n\n")
		case "control":
			observe.GlobalTrace("case: \"control\"")
			fmt.Fprintf(&b, "The control state will write the selected-item handoff to `%s` after selection.\n", control.HandoffPath)
			b.WriteString("Do not write a separate generic next-item handoff; the control state owns that handoff after the cursor is selected.\n\n")
		}
	}
	fmt.Fprintf(&b, "Write `%s` as JSON with this exact shape:\n\n", control.ListPath)
	itemContract := strings.TrimSpace(control.ItemContract)
	if itemContract != "" {
		observe.GlobalTrace("if: itemContract != \"\"")
		b.WriteString(itemContract)
		if !strings.HasSuffix(itemContract, "\n") {
			observe.GlobalTrace("if: !strings.HasSuffix(itemContract, \"\\n\")")
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else {
		observe.GlobalTrace("else: itemContract != \"\"")
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
		observe.GlobalTrace("if: control.BlockedEvent != \"\"")
		fmt.Fprintf(&b, "- When no `%s` items remain but `%s` items remain, `%s` emits `%s`.\n", pendingStatus, blockedStatus, next.ID, control.BlockedEvent)
	}
	if strings.TrimSpace(control.Dependency.DependencyIDsField) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(control.Dependency.DependencyIDsField) != \"\"")
		fmt.Fprintf(&b, "- Use `%s` with `%s` for unfinished items that depend on another listed item before they can run.\n", blockedStatus, control.Dependency.DependencyIDsField)
	}
	if control.HandoffPath != "" {
		observe.GlobalTrace("if: control.HandoffPath != \"\"")
		switch control.HandoffMode {
		case "persona":
			observe.GlobalTrace("case: \"persona\"")
			fmt.Fprintf(&b, "- `%s` will select the item and its `%s` persona will write the handoff for that exact selected item, not for future checklist items.\n", next.ID, next.Persona)
		case "control":
			observe.GlobalTrace("case: \"control\"")
			fmt.Fprintf(&b, "- `%s` will select the item and write the handoff for that exact selected item, not for future checklist items.\n", next.ID)
		}
	}
	if strings.TrimSpace(control.Dependency.DeferredDependencyField) != "" && strings.TrimSpace(control.Dependency.DependencyIDsField) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(control.Dependency.DeferredDependencyField) != \"\" && string...")
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

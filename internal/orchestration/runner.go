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

	taskPrompt := opts.TaskPrompt
	stateEngines := make(map[string]*query.Engine)
	for !runtime.States[runtime.FSM.Current()].Terminal {
		stateID := runtime.FSM.Current()
		state := runtime.States[stateID]
		stateEngine := engine
		stateTaskPrompt := ""
		if state.Control.IsZero() {
			var ok bool
			stateEngine, ok = stateEngines[state.ID]
			if !ok {
				stateEngine, _ = engine.ForkFreshConversation()
				stateEngines[state.ID] = stateEngine
			}
			stateTaskPrompt = taskPrompt
			taskPrompt = ""
		}
		event, _, err := RunNodeEvents(ctx, ch, stateEngine, projection, opts.PersonaDir, def, state, stateTaskPrompt, "", artifactRoot)
		if err != nil {
			ch <- query.ErrorEvent{Err: err}
			return
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
		if state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffPath != "" {
			emitOrchestration(ch, bus, projection, query.OrchestrationHandoffEvent{
				StateID:   state.ID,
				Event:     event,
				Path:      state.Control.ForEachNext.HandoffPath,
				Direction: "write",
			})
		}
		return event, "", nil
	}

	personaDef, err := LoadPersonaForState(personaDir, state)
	if err != nil {
		return "", "", err
	}
	emitOrchestration(ch, bus, projection, query.OrchestrationStateStartedEvent{StateID: state.ID, PersonaID: personaDef.ID})

	_, err = RunStateEvents(ctx, ch, engine, projection, def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)
	if err != nil {
		return "", "", fmt.Errorf("state %q failed: %w", state.ID, err)
	}

	event, err := SelectStateEvent(state)
	if err != nil {
		return "", "", fmt.Errorf("select event for state %q: %w", state.ID, err)
	}
	return event, "", nil
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
	system, prompt, err := BuildPromptWithArtifactRootChecked(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot)
	if err != nil {
		return "", err
	}

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
	system, prompt, _ := buildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot, false)
	return system, prompt
}

func BuildPromptWithArtifactRootChecked(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoot string) (model.SystemPrompt, string, error) {
	return buildPromptWithArtifactRoot(def, state, personaDef, taskPrompt, handoffPrompt, artifactRoot, true)
}

func buildPromptWithArtifactRoot(def Definition, state State, personaDef persona.Definition, taskPrompt string, handoffPrompt string, artifactRoot string, strict bool) (model.SystemPrompt, string, error) {
	system := model.SystemPrompt{Blocks: []model.SystemBlock{{
		Text:      strings.TrimRight(personaDef.Prompt, "\n") + "\n\n" + query.PragmaLoopSystemPrompt(),
		Cacheable: false,
	}}}

	var b strings.Builder
	if strings.TrimSpace(taskPrompt) != "" && state.TaskPrompt != TaskPromptNone {
		fmt.Fprintf(&b, "## Task\n\n%s\n", taskPrompt)
	} else {
		handoff, err := RenderArtifactHandoff(state, artifactRoot, strict)
		if err != nil {
			return system, "", err
		}
		if strings.TrimSpace(handoff) != "" {
			fmt.Fprintf(&b, "## Handoff From Previous Phase\n\n%s\n\n", strings.TrimSpace(handoff))
		} else if strings.TrimSpace(handoffPrompt) != "" && !strict {
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
	if contract := RenderNextForEachContract(def, state); contract != "" {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n\n") {
			b.WriteString("\n")
		}
		b.WriteString(contract)
	}

	return system, b.String(), nil
}

func RenderArtifactHandoff(state State, artifactRoot string, strict bool) (string, error) {
	artifacts := state.Artifacts
	if len(artifacts.Inputs) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, artifact := range artifacts.Inputs {
		path := resolveArtifactPath(artifact.Path, artifactRoot)
		content, err := os.ReadFile(path)
		if err != nil {
			if artifact.Required && strict {
				return "", fmt.Errorf("read required input artifact %q at %q: %w", artifact.ID, path, err)
			}
			if artifact.Required || !os.IsNotExist(err) {
				fmt.Fprintf(&b, "### `%s` (%s)\nPath: `%s`\nUnavailable: %v\n\n", artifact.ID, artifactRequirement(artifact), path, err)
			}
			continue
		}
		fmt.Fprintf(&b, "### `%s` (%s)\nPath: `%s`\n", artifact.ID, artifactRequirement(artifact), path)
		if artifact.Description != "" {
			fmt.Fprintf(&b, "Description: %s\n", artifact.Description)
		}
		b.WriteString("Content:\n")
		b.WriteString(strings.TrimRight(string(content), "\n"))
		b.WriteString("\n\n")
		if artifact.ID == "current_item" && state.Persona == "item_worker" {
			b.WriteString(renderSourceEditTransport())
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func renderSourceEditTransport() string {
	return `### ` + "`source_edit_transport`" + ` (runtime capability)
The bash environment has ` + "`git apply`" + ` available.
Use normal bash for inspection commands, validation commands, and runtime artifact writes.
For multi-line additions or replacements in existing repository source files, use a single bash block containing ` + "`git apply <<'PATCH'`" + ` with a standard unified diff.
For small exact deletions, a concise checked command is acceptable only when it verifies the exact line content at that location or matches the exact line text before deleting it.
For ` + "`git apply`" + ` patches:
- include ` + "`--- a/<path>`" + ` and ` + "`+++ b/<path>`" + `
- use repository-relative diff paths without container or absolute prefixes. Correct: ` + "`--- a/internal/telemetry/telemetry.go`" + `. Wrong: ` + "`--- a/app/internal/telemetry/telemetry.go`" + ` or ` + "`--- /app/internal/telemetry/telemetry.go`" + `
- include normal hunk headers with line numbers, such as ` + "`@@ -40,6 +40,10 @@`" + `
- omit ` + "`index ...`" + ` lines
- do not use bare ` + "`@@`" + ` hunk headers
- avoid replacing large blocks with many removed lines followed by many added lines when a smaller focused patch or exact deletion is enough

`
}

func resolveArtifactPath(path string, artifactRoot string) string {
	if filepath.IsAbs(path) {
		return path
	}
	root := artifactRoot
	if strings.TrimSpace(root) == "" {
		root = DefaultArtifactRoot
	}
	return filepath.Join(root, path)
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

func RenderNextForEachContract(def Definition, state State) string {
	next, ok := nextStateForDefaultEvent(def, state)
	if !ok || next.Control.ForEachNext == nil {
		return ""
	}
	control := next.Control.ForEachNext
	pendingStatus := control.PendingStatus
	if pendingStatus == "" {
		pendingStatus = "pending"
	}
	doneStatus := control.DoneStatus
	if doneStatus == "" {
		doneStatus = "approved"
	}
	var b strings.Builder
	b.WriteString("## Output Artifact Shape Required By Next State\n\n")
	fmt.Fprintf(&b, "The next state is `%s`, a `foreach_next` control state.\n", next.ID)
	fmt.Fprintf(&b, "It will parse `%s` and write the selected item to `%s`.\n\n", control.ListPath, control.CursorPath)
	if control.HandoffPath != "" {
		fmt.Fprintf(&b, "It will also write an immediate selected-item handoff to `%s`.\n", control.HandoffPath)
		b.WriteString("Do not write a separate generic next-item handoff; the control state owns that handoff after selection.\n\n")
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
	fmt.Fprintf(&b, "      \"status\": %q\n", pendingStatus)
	b.WriteString("    }\n")
	b.WriteString("  ]\n")
	b.WriteString("}\n")
	b.WriteString("```\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- `id` is required and must be a JSON string, never a number.\n")
	fmt.Fprintf(&b, "- `status` is required and must be `%q` for new items.\n", pendingStatus)
	fmt.Fprintf(&b, "- When no `%s` items remain, `%s` will write a cursor item with status `%s`.\n", pendingStatus, next.ID, doneStatus)
	if control.HandoffPath != "" {
		fmt.Fprintf(&b, "- `%s` will write the handoff for the exact selected item, not for future checklist items.\n", next.ID)
	}
	b.WriteString("- `acceptance`, `allowed_files`, `forbidden_files`, `coupled_edit_paths`, and `report_changed_files` must be JSON arrays.\n")
	b.WriteString("- Use `validation_command: \"none\"` only when `acceptance_check` is present and `validation_deferred_until` names what later item or surface makes validation runnable.\n")
	b.WriteString("- Put every path from `coupled_edit_paths` in `allowed_files`; the worker may edit only `allowed_files`.\n")
	b.WriteString("- Extra item fields are allowed only if they are valid JSON and should be preserved by later controls.\n\n")
	return b.String()
}

func nextStateForDefaultEvent(def Definition, state State) (State, bool) {
	event := state.Event.Default
	if event == "" {
		event = EventComplete
	}
	for _, transition := range def.Transitions {
		if transition.Event != event || !transitionIncludesFrom(transition, state.ID) {
			continue
		}
		for _, candidate := range def.States {
			if candidate.ID == transition.To {
				return candidate, true
			}
		}
		return State{}, false
	}
	return State{}, false
}

func transitionIncludesFrom(transition Transition, stateID string) bool {
	for _, from := range transition.From {
		if from == stateID {
			return true
		}
	}
	return false
}

func writeArtifactLine(b *strings.Builder, artifact Artifact) {
	fmt.Fprintf(b, "- `%s` (%s): `%s`", artifact.ID, artifactRequirement(artifact), artifact.Path)
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

func artifactRequirement(artifact Artifact) string {
	if artifact.Required {
		return "required"
	}
	return "optional"
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

package orchestration

import (
	"fmt"
	"github.com/artpar/pragma/internal/observe"
	"sort"
	"strings"
)

const (
	VisualizationDetailsCompact = "compact"
	VisualizationDetailsFull    = "full"
)

type VisualizationOptions struct {
	PersonaDir  string
	Details     string
	LoadPersona bool
}

type visualizationPersona struct {
	ID          string
	Description string
	Properties  map[string]string
	Err         error
}

// RenderVisualization renders a static ASCII projection of an orchestration
// definition. It does not run the FSM or create provider/session state.
func RenderVisualization(def Definition, opts VisualizationOptions) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if opts.Details == "" {
		observe.GlobalTrace("if: opts.Details == \"\"")
		opts.Details = VisualizationDetailsCompact
	}
	switch opts.Details {
	case VisualizationDetailsCompact, VisualizationDetailsFull:
		observe.GlobalTrace("case: VisualizationDetailsCompact, VisualizationDetailsFull")
	default:
		observe.GlobalTrace("default")
		return "", fmt.Errorf("unsupported visualization details %q; expected compact or full", opts.Details)
	}

	if _, err := NewRuntime(def); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}

	personas := loadVisualizationPersonas(def, opts)
	outgoing := outgoingTransitions(def)
	var b strings.Builder

	fmt.Fprintf(&b, "orchestration: %s\n", def.Name)
	fmt.Fprintf(&b, "initial: %s\n\n", def.Initial)
	fmt.Fprintf(&b, "[*] -> %s\n\n", def.Initial)

	for _, state := range def.States {
		observe.GlobalTrace("range def.States")
		b.WriteString(state.ID)
		meta := visualizationStateMeta(state, personas[state.ID])
		if len(meta) > 0 {
			observe.GlobalTrace("if: len(meta) > 0")
			fmt.Fprintf(&b, " [%s]", strings.Join(meta, ", "))
		}
		b.WriteString("\n")

		if opts.Details == VisualizationDetailsFull {
			observe.GlobalTrace("if: opts.Details == VisualizationDetailsFull")
			writeStateDetails(&b, state, personas[state.ID])
		}

		for _, tr := range outgoing[state.ID] {
			observe.GlobalTrace("range outgoing[state.ID]")
			fmt.Fprintf(&b, "  --%s--> %s", tr.Event, tr.To)
			if opts.Details == VisualizationDetailsFull && len(tr.Handoff) > 0 {
				observe.GlobalTrace("if: opts.Details == VisualizationDetailsFull && len(tr.Handoff) > 0")
				fmt.Fprintf(&b, " [handoff=%s]", artifactIDs(tr.Handoff))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	warnings := visualizationWarnings(def, personas)
	if len(warnings) > 0 {
		observe.GlobalTrace("if: len(warnings) > 0")
		b.WriteString("warnings:\n")
		for _, warning := range warnings {
			observe.GlobalTrace("range warnings")
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
	}
	observe.GlobalTrace("return: b.String(), nil")

	return b.String(), nil
}

func outgoingTransitions(def Definition) map[string][]Transition {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make(map[string][]Transition)
	for _, tr := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		for _, from := range tr.From {
			observe.GlobalTrace("range tr.From")
			out[from] = append(out[from], tr)
		}
	}
	observe.GlobalTrace("return: out")
	return out
}

func loadVisualizationPersonas(def Definition, opts VisualizationOptions) map[string]visualizationPersona {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	personas := make(map[string]visualizationPersona)
	if !opts.LoadPersona {
		observe.GlobalTrace("if: !opts.LoadPersona")
		observe.GlobalTrace("return: personas")
		return personas
	}
	for _, state := range def.States {
		observe.GlobalTrace("range def.States")
		if state.Terminal || !state.Control.IsZero() {
			observe.GlobalTrace("if: state.Terminal || !state.Control.IsZero()")
			continue
		}
		personaID := state.Persona
		if personaID == "" {
			observe.GlobalTrace("if: personaID == \"\"")
			personaID = state.ID
		}
		personaDef, err := LoadPersonaForState(opts.PersonaDir, state)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			personas[state.ID] = visualizationPersona{ID: personaID, Err: err}
			continue
		}
		personas[state.ID] = visualizationPersona{
			ID:          personaDef.ID,
			Description: personaDef.Description,
			Properties:  personaDef.Properties,
		}
	}
	observe.GlobalTrace("return: personas")
	return personas
}

func visualizationStateMeta(state State, personaInfo visualizationPersona) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var meta []string
	switch {
	case state.Terminal:
		observe.GlobalTrace("case: state.Terminal")
		meta = append(meta, "terminal")
	case !state.Control.IsZero():
		observe.GlobalTrace("case: !state.Control.IsZero()")
		meta = append(meta, "control="+ControlName(state))
	default:
		observe.GlobalTrace("default")
		personaID := personaInfo.ID
		if personaID == "" {
			personaID = state.Persona
		}
		if personaID == "" {
			personaID = state.ID
		}
		meta = append(meta, "persona="+personaID)
	}
	if state.TaskPrompt != "" {
		observe.GlobalTrace("if: state.TaskPrompt != \"\"")
		meta = append(meta, "task_prompt="+state.TaskPrompt)
	}
	if state.Event.Default != "" {
		observe.GlobalTrace("if: state.Event.Default != \"\"")
		meta = append(meta, "default_event="+state.Event.Default)
	}
	if !state.Artifacts.IsZero() {
		observe.GlobalTrace("if: !state.Artifacts.IsZero()")
		meta = append(meta, fmt.Sprintf("artifacts=in:%d,out:%d", len(state.Artifacts.Inputs), len(state.Artifacts.Outputs)))
	}
	observe.GlobalTrace("return: meta")
	return meta
}

func writeStateDetails(b *strings.Builder, state State, personaInfo visualizationPersona) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if personaInfo.Description != "" {
		observe.GlobalTrace("if: personaInfo.Description != \"\"")
		fmt.Fprintf(b, "  persona_description: %s\n", personaInfo.Description)
	}
	if len(personaInfo.Properties) > 0 {
		observe.GlobalTrace("if: len(personaInfo.Properties) > 0")
		fmt.Fprintf(b, "  persona_properties: %s\n", formatProperties(personaInfo.Properties))
	}
	if !state.Control.IsZero() {
		observe.GlobalTrace("if: !state.Control.IsZero()")
		writeControlDetails(b, state)
	}
	if len(state.Artifacts.Inputs) > 0 {
		observe.GlobalTrace("if: len(state.Artifacts.Inputs) > 0")
		fmt.Fprintf(b, "  inputs: %s\n", formatArtifacts(state.Artifacts.Inputs))
	}
	if len(state.Artifacts.Outputs) > 0 {
		observe.GlobalTrace("if: len(state.Artifacts.Outputs) > 0")
		fmt.Fprintf(b, "  outputs: %s\n", formatArtifacts(state.Artifacts.Outputs))
	}
}

func writeControlDetails(b *strings.Builder, state State) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case state.Control.ForEachNext != nil:
		observe.GlobalTrace("case: state.Control.ForEachNext != nil")
		c := state.Control.ForEachNext
		fmt.Fprintf(b, "  control: list=%s cursor=%s item_event=%s done_event=%s\n", c.ListPath, c.CursorPath, c.ItemEvent, c.DoneEvent)
	case state.Control.MarkCurrentItem != nil:
		observe.GlobalTrace("case: state.Control.MarkCurrentItem != nil")
		c := state.Control.MarkCurrentItem
		fmt.Fprintf(b, "  control: list=%s cursor=%s status=%s event=%s\n", c.ListPath, c.CursorPath, c.Status, c.Event)
	case state.Control.ArtifactVerdict != nil:
		observe.GlobalTrace("case: state.Control.ArtifactVerdict != nil")
		c := state.Control.ArtifactVerdict
		fmt.Fprintf(b, "  control: path=%s approve_event=%s block_event=%s\n", c.Path, c.ApproveEvent, c.BlockEvent)
	case state.Control.ArtifactDecision != nil:
		observe.GlobalTrace("case: state.Control.ArtifactDecision != nil")
		c := state.Control.ArtifactDecision
		fmt.Fprintf(b, "  control: path=%s field=%s events=%s\n", c.Path, c.Field, formatDecisionEvents(c.Events))
	}
}

func formatArtifacts(artifacts []Artifact) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	parts := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		observe.GlobalTrace("range artifacts")
		label := artifact.ID
		if artifact.Required {
			observe.GlobalTrace("if: artifact.Required")
			label += "!"
		}
		if artifact.Kind != "" {
			observe.GlobalTrace("if: artifact.Kind != \"\"")
			label += ":" + artifact.Kind
		}
		parts = append(parts, fmt.Sprintf("%s=%s", label, artifact.Path))
	}
	observe.GlobalTrace("return: strings.Join(parts, \", \")")
	return strings.Join(parts, ", ")
}

func artifactIDs(artifacts []Artifact) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		observe.GlobalTrace("range artifacts")
		if artifact.ID != "" {
			observe.GlobalTrace("if: artifact.ID != \"\"")
			ids = append(ids, artifact.ID)
		}
	}
	observe.GlobalTrace("return: strings.Join(ids, \",\")")
	return strings.Join(ids, ",")
}

func formatProperties(properties map[string]string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	keys := make([]string, 0, len(properties))
	for key := range properties {
		observe.GlobalTrace("range properties")
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		observe.GlobalTrace("range keys")
		parts = append(parts, fmt.Sprintf("%s=%s", key, properties[key]))
	}
	observe.GlobalTrace("return: strings.Join(parts, \", \")")
	return strings.Join(parts, ", ")
}

func formatDecisionEvents(events map[string]string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	keys := make([]string, 0, len(events))
	for key := range events {
		observe.GlobalTrace("range events")
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		observe.GlobalTrace("range keys")
		parts = append(parts, fmt.Sprintf("%s:%s", key, events[key]))
	}
	observe.GlobalTrace("return: strings.Join(parts, \",\")")
	return strings.Join(parts, ",")
}

func visualizationWarnings(def Definition, personas map[string]visualizationPersona) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var warnings []string
	for _, state := range def.States {
		observe.GlobalTrace("range def.States")
		if state.Terminal || !state.Control.IsZero() {
			observe.GlobalTrace("if: state.Terminal || !state.Control.IsZero()")
			continue
		}
		personaInfo := personas[state.ID]
		if personaInfo.Err != nil {
			observe.GlobalTrace("if: personaInfo.Err != nil")
			warnings = append(warnings, fmt.Sprintf("state %s persona %s: %v", state.ID, personaInfo.ID, personaInfo.Err))
		}
	}
	observe.GlobalTrace("return: warnings")
	return warnings
}

package orchestration

import (
	"fmt"
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
	if opts.Details == "" {
		opts.Details = VisualizationDetailsCompact
	}
	switch opts.Details {
	case VisualizationDetailsCompact, VisualizationDetailsFull:
	default:
		return "", fmt.Errorf("unsupported visualization details %q; expected compact or full", opts.Details)
	}

	if _, err := NewRuntime(def); err != nil {
		return "", err
	}

	personas := loadVisualizationPersonas(def, opts)
	outgoing := outgoingTransitions(def)
	var b strings.Builder

	fmt.Fprintf(&b, "orchestration: %s\n", def.Name)
	fmt.Fprintf(&b, "initial: %s\n\n", def.Initial)
	fmt.Fprintf(&b, "[*] -> %s\n\n", def.Initial)

	for _, state := range def.States {
		b.WriteString(state.ID)
		meta := visualizationStateMeta(state, personas[state.ID])
		if len(meta) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(meta, ", "))
		}
		b.WriteString("\n")

		if opts.Details == VisualizationDetailsFull {
			writeStateDetails(&b, state, personas[state.ID])
		}

		for _, tr := range outgoing[state.ID] {
			fmt.Fprintf(&b, "  --%s--> %s", tr.Event, tr.To)
			if opts.Details == VisualizationDetailsFull && len(tr.Handoff) > 0 {
				fmt.Fprintf(&b, " [handoff=%s]", artifactIDs(tr.Handoff))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	warnings := visualizationWarnings(def, personas)
	if len(warnings) > 0 {
		b.WriteString("warnings:\n")
		for _, warning := range warnings {
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
	}

	return b.String(), nil
}

func outgoingTransitions(def Definition) map[string][]Transition {
	out := make(map[string][]Transition)
	for _, tr := range def.Transitions {
		for _, from := range tr.From {
			out[from] = append(out[from], tr)
		}
	}
	return out
}

func loadVisualizationPersonas(def Definition, opts VisualizationOptions) map[string]visualizationPersona {
	personas := make(map[string]visualizationPersona)
	if !opts.LoadPersona {
		return personas
	}
	for _, state := range def.States {
		if state.Terminal || !state.Control.IsZero() {
			continue
		}
		personaID := state.Persona
		if personaID == "" {
			personaID = state.ID
		}
		personaDef, err := LoadPersonaForState(opts.PersonaDir, state)
		if err != nil {
			personas[state.ID] = visualizationPersona{ID: personaID, Err: err}
			continue
		}
		personas[state.ID] = visualizationPersona{
			ID:          personaDef.ID,
			Description: personaDef.Description,
			Properties:  personaDef.Properties,
		}
	}
	return personas
}

func visualizationStateMeta(state State, personaInfo visualizationPersona) []string {
	var meta []string
	switch {
	case state.Terminal:
		meta = append(meta, "terminal")
	case !state.Control.IsZero():
		meta = append(meta, "control="+ControlName(state))
	default:
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
		meta = append(meta, "task_prompt="+state.TaskPrompt)
	}
	if state.Event.Default != "" {
		meta = append(meta, "default_event="+state.Event.Default)
	}
	if !state.Artifacts.IsZero() {
		meta = append(meta, fmt.Sprintf("artifacts=in:%d,out:%d", len(state.Artifacts.Inputs), len(state.Artifacts.Outputs)))
	}
	return meta
}

func writeStateDetails(b *strings.Builder, state State, personaInfo visualizationPersona) {
	if personaInfo.Description != "" {
		fmt.Fprintf(b, "  persona_description: %s\n", personaInfo.Description)
	}
	if len(personaInfo.Properties) > 0 {
		fmt.Fprintf(b, "  persona_properties: %s\n", formatProperties(personaInfo.Properties))
	}
	if !state.Control.IsZero() {
		writeControlDetails(b, state)
	}
	if len(state.Artifacts.Inputs) > 0 {
		fmt.Fprintf(b, "  inputs: %s\n", formatArtifacts(state.Artifacts.Inputs))
	}
	if len(state.Artifacts.Outputs) > 0 {
		fmt.Fprintf(b, "  outputs: %s\n", formatArtifacts(state.Artifacts.Outputs))
	}
}

func writeControlDetails(b *strings.Builder, state State) {
	switch {
	case state.Control.ForEachNext != nil:
		c := state.Control.ForEachNext
		fmt.Fprintf(b, "  control: list=%s cursor=%s item_event=%s done_event=%s\n", c.ListPath, c.CursorPath, c.ItemEvent, c.DoneEvent)
	case state.Control.MarkCurrentItem != nil:
		c := state.Control.MarkCurrentItem
		fmt.Fprintf(b, "  control: list=%s cursor=%s status=%s event=%s\n", c.ListPath, c.CursorPath, c.Status, c.Event)
	case state.Control.ArtifactVerdict != nil:
		c := state.Control.ArtifactVerdict
		fmt.Fprintf(b, "  control: path=%s approve_event=%s block_event=%s\n", c.Path, c.ApproveEvent, c.BlockEvent)
	case state.Control.ArtifactDecision != nil:
		c := state.Control.ArtifactDecision
		fmt.Fprintf(b, "  control: path=%s field=%s events=%s\n", c.Path, c.Field, formatDecisionEvents(c.Events))
	}
}

func formatArtifacts(artifacts []Artifact) string {
	parts := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		label := artifact.ID
		if artifact.Required {
			label += "!"
		}
		if artifact.Kind != "" {
			label += ":" + artifact.Kind
		}
		parts = append(parts, fmt.Sprintf("%s=%s", label, artifact.Path))
	}
	return strings.Join(parts, ", ")
}

func artifactIDs(artifacts []Artifact) string {
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.ID != "" {
			ids = append(ids, artifact.ID)
		}
	}
	return strings.Join(ids, ",")
}

func formatProperties(properties map[string]string) string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, properties[key]))
	}
	return strings.Join(parts, ", ")
}

func formatDecisionEvents(events map[string]string) string {
	keys := make([]string, 0, len(events))
	for key := range events {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%s", key, events[key]))
	}
	return strings.Join(parts, ",")
}

func visualizationWarnings(def Definition, personas map[string]visualizationPersona) []string {
	var warnings []string
	for _, state := range def.States {
		if state.Terminal || !state.Control.IsZero() {
			continue
		}
		personaInfo := personas[state.ID]
		if personaInfo.Err != nil {
			warnings = append(warnings, fmt.Sprintf("state %s persona %s: %v", state.ID, personaInfo.ID, personaInfo.Err))
		}
	}
	return warnings
}

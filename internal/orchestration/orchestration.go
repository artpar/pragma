package orchestration

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/looplab/fsm"
	"gopkg.in/yaml.v3"
)

const (
	EventComplete  = "complete"
	TaskPromptFull = "full"
	TaskPromptNone = "none"
)

// State is one orchestration node. Execution semantics live outside the graph.
type State struct {
	ID         string    `yaml:"id"`
	Terminal   bool      `yaml:"terminal,omitempty"`
	Persona    string    `yaml:"persona,omitempty"`
	TaskPrompt string    `yaml:"task_prompt,omitempty"`
	Artifacts  Artifacts `yaml:"artifacts,omitempty"`
	Control    Control   `yaml:"control,omitempty"`
	Event      Event     `yaml:"event,omitempty"`
}

type Artifacts struct {
	Inputs  []Artifact `yaml:"inputs,omitempty"`
	Outputs []Artifact `yaml:"outputs,omitempty"`
}

func (a Artifacts) IsZero() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: len(a.Inputs) == 0 && len(a.Outputs) == 0")
	return len(a.Inputs) == 0 && len(a.Outputs) == 0
}

type Artifact struct {
	ID            string   `yaml:"id"`
	Path          string   `yaml:"path"`
	Required      bool     `yaml:"required,omitempty"`
	Description   string   `yaml:"description,omitempty"`
	Kind          string   `yaml:"kind,omitempty"`
	AllowedValues []string `yaml:"allowed_values,omitempty"`
}

type Control struct {
	ForEachNext      *ForEachNextControl      `yaml:"foreach_next,omitempty"`
	MarkCurrentItem  *MarkCurrentItemControl  `yaml:"mark_current_item,omitempty"`
	ArtifactVerdict  *ArtifactVerdictControl  `yaml:"artifact_verdict,omitempty"`
	ArtifactDecision *ArtifactDecisionControl `yaml:"artifact_decision,omitempty"`
}

func (c Control) IsZero() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.ForEachNext == nil && c.MarkCurrentItem == nil && c.ArtifactVerdict == nil ...")
	return c.ForEachNext == nil && c.MarkCurrentItem == nil && c.ArtifactVerdict == nil && c.ArtifactDecision == nil
}

type ForEachNextControl struct {
	ListPath      string `yaml:"list_path"`
	CursorPath    string `yaml:"cursor_path"`
	HandoffPath   string `yaml:"handoff_path,omitempty"`
	PendingStatus string `yaml:"pending_status,omitempty"`
	DoneStatus    string `yaml:"done_status,omitempty"`
	BlockedStatus string `yaml:"blocked_status,omitempty"`
	ItemEvent     string `yaml:"item_event"`
	DoneEvent     string `yaml:"done_event"`
	BlockedEvent  string `yaml:"blocked_event,omitempty"`
}

type MarkCurrentItemControl struct {
	ListPath   string `yaml:"list_path"`
	CursorPath string `yaml:"cursor_path"`
	Status     string `yaml:"status"`
	Event      string `yaml:"event"`
}

type ArtifactVerdictControl struct {
	Path         string `yaml:"path"`
	Approve      string `yaml:"approve,omitempty"`
	Block        string `yaml:"block,omitempty"`
	ApproveEvent string `yaml:"approve_event"`
	BlockEvent   string `yaml:"block_event"`
}

type ArtifactDecisionControl struct {
	Path   string            `yaml:"path"`
	Field  string            `yaml:"field,omitempty"`
	Events map[string]string `yaml:"events"`
}

type Event struct {
	Default string `yaml:"default,omitempty"`
}

type Transition struct {
	Event   string     `yaml:"event"`
	From    []string   `yaml:"from"`
	To      string     `yaml:"to"`
	Handoff []Artifact `yaml:"handoff,omitempty"`
}

type Definition struct {
	Name        string       `yaml:"name"`
	Initial     string       `yaml:"initial"`
	States      []State      `yaml:"states"`
	Transitions []Transition `yaml:"transitions"`
}

type Runtime struct {
	Definition  Definition
	States      map[string]State
	Transitions map[TransitionKey]Transition
	FSM         *fsm.FSM
}

type TransitionKey struct {
	From  string
	Event string
}

type Checklist struct {
	Items []ChecklistItem `json:"items"`
}

type ChecklistItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Acceptance  []string `json:"acceptance,omitempty"`
	Status      string   `json:"status"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	BlockReason string   `json:"block_reason,omitempty"`
	Extra       map[string]json.RawMessage
}

func (i *ChecklistItem) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if idRaw, ok := raw["id"]; ok {
		idText := strings.TrimSpace(string(idRaw))
		if idText != "" && idText != "null" && !strings.HasPrefix(idText, "\"") {
			return fmt.Errorf("id must be a JSON string, got %s", jsonValueKind(idText))
		}
	}
	type checklistItem ChecklistItem
	var known checklistItem
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	for _, key := range []string{"id", "title", "description", "acceptance", "status", "blocked_by", "block_reason"} {
		delete(raw, key)
	}
	*i = ChecklistItem(known)
	if len(raw) > 0 {
		i.Extra = raw
	} else {
		i.Extra = nil
	}
	return nil
}

func (i ChecklistItem) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(i.Extra)+5)
	for key, value := range i.Extra {
		fields[key] = value
	}
	if i.ID != "" {
		raw, err := json.Marshal(i.ID)
		if err != nil {
			return nil, err
		}
		fields["id"] = raw
	}
	if i.Title != "" {
		raw, err := json.Marshal(i.Title)
		if err != nil {
			return nil, err
		}
		fields["title"] = raw
	}
	if i.Description != "" {
		raw, err := json.Marshal(i.Description)
		if err != nil {
			return nil, err
		}
		fields["description"] = raw
	}
	if i.Acceptance != nil {
		raw, err := json.Marshal(i.Acceptance)
		if err != nil {
			return nil, err
		}
		fields["acceptance"] = raw
	}
	if i.BlockedBy != nil {
		raw, err := json.Marshal(i.BlockedBy)
		if err != nil {
			return nil, err
		}
		fields["blocked_by"] = raw
	}
	if i.BlockReason != "" {
		raw, err := json.Marshal(i.BlockReason)
		if err != nil {
			return nil, err
		}
		fields["block_reason"] = raw
	}
	raw, err := json.Marshal(i.Status)
	if err != nil {
		return nil, err
	}
	fields["status"] = raw
	return json.Marshal(fields)
}

func NewRuntime(def Definition) (*Runtime, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	states, transitions, err := validate(def)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	events := make([]fsm.EventDesc, 0, len(def.Transitions))
	for _, tr := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		events = append(events, fsm.EventDesc{
			Name: tr.Event,
			Src:  append([]string(nil), tr.From...),
			Dst:  tr.To,
		})
	}
	observe.GlobalTrace("return: &Runtime{\n\tDefinition:\tdef,\n\tStates:\t\tstates,\n\tTransitions:\ttransitions,\n\tFSM...")

	return &Runtime{
		Definition:  def,
		States:      states,
		Transitions: transitions,
		FSM:         fsm.NewFSM(def.Initial, events, nil),
	}, nil
}

func (r *Runtime) TransitionFor(from string, event string) (Transition, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if r == nil {
		observe.GlobalTrace("if: r == nil")
		observe.GlobalTrace("return: Transition{}, false")
		return Transition{}, false
	}
	tr, ok := r.Transitions[TransitionKey{From: from, Event: event}]
	observe.GlobalTrace("return: tr, ok")
	return tr, ok
}

func LoadDefinitionFile(path string) (Definition, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Definition{}, err")
		return Definition{}, err
	}
	var def Definition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Definition{}, fmt.Errorf(\"parse orchestration definition %q: %w\", path, err)")
		return Definition{}, fmt.Errorf("parse orchestration definition %q: %w", path, err)
	}
	if _, _, err := validate(def); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Definition{}, err")
		return Definition{}, err
	}
	observe.GlobalTrace("return: def, nil")
	return def, nil
}

func validate(def Definition) (map[string]State, map[TransitionKey]Transition, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if def.Name == "" {
		observe.GlobalTrace("if: def.Name == \"\"")
		observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration definition requires a name\")")
		return nil, nil, fmt.Errorf("orchestration definition requires a name")
	}
	if def.Initial == "" {
		observe.GlobalTrace("if: def.Initial == \"\"")
		observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q requires an initial state\", def.Name)")
		return nil, nil, fmt.Errorf("orchestration %q requires an initial state", def.Name)
	}

	states := make(map[string]State, len(def.States))
	for _, state := range def.States {
		observe.GlobalTrace("range def.States")
		if state.ID == "" {
			observe.GlobalTrace("if: state.ID == \"\"")
			observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q has a state with empty id\", def.Name)")
			return nil, nil, fmt.Errorf("orchestration %q has a state with empty id", def.Name)
		}
		if _, exists := states[state.ID]; exists {
			observe.GlobalTrace("if: exists")
			observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q has duplicate state %q\", def.Name, sta...")
			return nil, nil, fmt.Errorf("orchestration %q has duplicate state %q", def.Name, state.ID)
		}
		states[state.ID] = state
	}

	if _, ok := states[def.Initial]; !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q initial state %q is not defined\", def....")
		return nil, nil, fmt.Errorf("orchestration %q initial state %q is not defined", def.Name, def.Initial)
	}

	transitions := make(map[TransitionKey]Transition)
	for _, tr := range def.Transitions {
		observe.GlobalTrace("range def.Transitions")
		if tr.Event == "" {
			observe.GlobalTrace("if: tr.Event == \"\"")
			observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q has a transition with empty event\", de...")
			return nil, nil, fmt.Errorf("orchestration %q has a transition with empty event", def.Name)
		}
		if len(tr.From) == 0 {
			observe.GlobalTrace("if: len(tr.From) == 0")
			observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q transition %q has no source states\", d...")
			return nil, nil, fmt.Errorf("orchestration %q transition %q has no source states", def.Name, tr.Event)
		}
		if _, ok := states[tr.To]; !ok {
			observe.GlobalTrace("if: !ok")
			observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q transition %q targets unknown state %q...")
			return nil, nil, fmt.Errorf("orchestration %q transition %q targets unknown state %q", def.Name, tr.Event, tr.To)
		}
		if err := validateArtifactList(def.Name, fmt.Sprintf("transition %s to %s", tr.Event, tr.To), tr.Handoff); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, nil, err")
			return nil, nil, err
		}
		for _, from := range tr.From {
			observe.GlobalTrace("range tr.From")
			state, ok := states[from]
			if !ok {
				observe.GlobalTrace("if: !ok")
				observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q transition %q references unknown sourc...")
				return nil, nil, fmt.Errorf("orchestration %q transition %q references unknown source state %q", def.Name, tr.Event, from)
			}
			if state.Terminal {
				observe.GlobalTrace("if: state.Terminal")
				observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q terminal state %q cannot be a transiti...")
				return nil, nil, fmt.Errorf("orchestration %q terminal state %q cannot be a transition source", def.Name, from)
			}
			key := TransitionKey{From: from, Event: tr.Event}
			if _, exists := transitions[key]; exists {
				observe.GlobalTrace("if: exists")
				observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q has duplicate transition for state %q ...")
				return nil, nil, fmt.Errorf("orchestration %q has duplicate transition for state %q event %q", def.Name, from, tr.Event)
			}
			transitions[key] = tr
		}
	}
	if err := validateDefinitionSpecificInvariants(def.Name, transitions); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, nil, err")
		return nil, nil, err
	}

	for _, state := range states {
		observe.GlobalTrace("range states")
		if state.Terminal {
			observe.GlobalTrace("if: state.Terminal")
			if !state.Control.IsZero() {
				observe.GlobalTrace("if: !state.Control.IsZero()")
				observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q terminal state %q cannot have control\"...")
				return nil, nil, fmt.Errorf("orchestration %q terminal state %q cannot have control", def.Name, state.ID)
			}
			continue
		}
		if err := validateStateExecution(def.Name, state); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, nil, err")
			return nil, nil, err
		}
		if err := validateArtifacts(def.Name, state.ID, state.Artifacts); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, nil, err")
			return nil, nil, err
		}
		for _, event := range emittedEvents(state) {
			observe.GlobalTrace("range emittedEvents(state)")
			if _, ok := transitions[TransitionKey{From: state.ID, Event: event}]; !ok {
				observe.GlobalTrace("if: !ok")
				observe.GlobalTrace("return: nil, nil, fmt.Errorf(\"orchestration %q state %q can emit event %q but has no ...")
				return nil, nil, fmt.Errorf("orchestration %q state %q can emit event %q but has no matching transition", def.Name, state.ID, event)
			}
		}
	}
	observe.GlobalTrace("return: states, transitions, nil")

	return states, transitions, nil
}

func validateStateExecution(defName string, state State) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if state.TaskPrompt != "" && state.TaskPrompt != TaskPromptFull && state.TaskPrompt != TaskPromptNone {
		observe.GlobalTrace("if: state.TaskPrompt != \"\" && state.TaskPrompt != TaskPromptFull && state.TaskPro...")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q has invalid task_prompt %q\", defName, s...")
		return fmt.Errorf("orchestration %q state %q has invalid task_prompt %q", defName, state.ID, state.TaskPrompt)
	}
	if !state.Control.IsZero() {
		observe.GlobalTrace("if: !state.Control.IsZero()")
		if state.TaskPrompt != "" {
			observe.GlobalTrace("if: state.TaskPrompt != \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q control state %q cannot define task_prompt\", def...")
			return fmt.Errorf("orchestration %q control state %q cannot define task_prompt", defName, state.ID)
		}
		if state.Event.Default != "" {
			observe.GlobalTrace("if: state.Event.Default != \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q control state %q cannot also define event\", defN...")
			return fmt.Errorf("orchestration %q control state %q cannot also define event", defName, state.ID)
		}
		controls := 0
		if state.Control.ForEachNext != nil {
			observe.GlobalTrace("if: state.Control.ForEachNext != nil")
			controls++
			if err := validateForEachNextControl(defName, state.ID, state.Control.ForEachNext); err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: err")
				return err
			}
		}
		if state.Control.MarkCurrentItem != nil {
			observe.GlobalTrace("if: state.Control.MarkCurrentItem != nil")
			controls++
			if err := validateMarkCurrentItemControl(defName, state.ID, state.Control.MarkCurrentItem); err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: err")
				return err
			}
		}
		if state.Control.ArtifactVerdict != nil {
			observe.GlobalTrace("if: state.Control.ArtifactVerdict != nil")
			controls++
			if err := validateArtifactVerdictControl(defName, state.ID, state.Control.ArtifactVerdict); err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: err")
				return err
			}
		}
		if state.Control.ArtifactDecision != nil {
			observe.GlobalTrace("if: state.Control.ArtifactDecision != nil")
			controls++
			if err := validateArtifactDecisionControl(defName, state.ID, state.Control.ArtifactDecision); err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: err")
				return err
			}
		}
		if controls != 1 {
			observe.GlobalTrace("if: controls != 1")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q control state %q must define exactly one control...")
			return fmt.Errorf("orchestration %q control state %q must define exactly one control", defName, state.ID)
		}
		if state.Persona != "" && state.Control.ForEachNext == nil {
			observe.GlobalTrace("if: state.Persona != \"\" && state.Control.ForEachNext == nil")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q control state %q can only define persona with foreach...")
			return fmt.Errorf("orchestration %q control state %q can only define persona with foreach_next", defName, state.ID)
		}
		if state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffPath != "" && state.Persona == "" {
			observe.GlobalTrace("if: state.Control.ForEachNext != nil && state.Control.ForEachNext.HandoffPath != \"\" && state.Persona == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q foreach_next handoff_path requires persona\", ...")
			return fmt.Errorf("orchestration %q state %q foreach_next handoff_path requires persona", defName, state.ID)
		}
		if state.Persona != "" && state.Control.ForEachNext != nil {
			observe.GlobalTrace("if: state.Persona != \"\" && state.Control.ForEachNext != nil")
			if err := validatePersonaBackedForEachNext(defName, state); err != nil {
				observe.GlobalTrace("if: err != nil")
				observe.GlobalTrace("return: err")
				return err
			}
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validatePersonaBackedForEachNext(defName string, state State) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	control := state.Control.ForEachNext
	if control == nil {
		observe.GlobalTrace("if: control == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	if control.HandoffPath == "" {
		observe.GlobalTrace("if: control.HandoffPath == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q persona-backed foreach_next requires handoff_path...")
		return fmt.Errorf("orchestration %q state %q persona-backed foreach_next requires handoff_path", defName, state.ID)
	}
	for _, artifact := range state.Artifacts.Outputs {
		observe.GlobalTrace("range state.Artifacts.Outputs")
		if artifact.Path == control.HandoffPath && artifact.Required {
			observe.GlobalTrace("if: artifact.Path == control.HandoffPath && artifact.Required")
			observe.GlobalTrace("return: nil")
			return nil
		}
	}
	observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q persona-backed foreach_next requires a required output artifact...")
	return fmt.Errorf("orchestration %q state %q persona-backed foreach_next requires a required output artifact at handoff_path %q", defName, state.ID, control.HandoffPath)
}

func validateArtifacts(defName, stateID string, artifacts Artifacts) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: validateArtifactList(defName, \"state \"+stateID, append(append([]Artifact(nil)...")
	return validateArtifactList(defName, "state "+stateID, append(append([]Artifact(nil), artifacts.Inputs...), artifacts.Outputs...))
}

func validateArtifactList(defName, owner string, artifacts []Artifact) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, artifact := range artifacts {
		observe.GlobalTrace("range artifacts")
		if artifact.ID == "" {
			observe.GlobalTrace("if: artifact.ID == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q %s artifact requires id\", defName, owner)")
			return fmt.Errorf("orchestration %q %s artifact requires id", defName, owner)
		}
		if artifact.Path == "" {
			observe.GlobalTrace("if: artifact.Path == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q %s artifact %q requires path\", defName, owner, a...")
			return fmt.Errorf("orchestration %q %s artifact %q requires path", defName, owner, artifact.ID)
		}
		for _, value := range artifact.AllowedValues {
			observe.GlobalTrace("range artifact.AllowedValues")
			if strings.TrimSpace(value) == "" {
				observe.GlobalTrace("if: strings.TrimSpace(value) == \"\"")
				observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q %s artifact %q has empty allowed value\", defName...")
				return fmt.Errorf("orchestration %q %s artifact %q has empty allowed value", defName, owner, artifact.ID)
			}
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validateDefinitionSpecificInvariants(defName string, transitions map[TransitionKey]Transition) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if defName != "prompt-control-v2-benchmark" {
		observe.GlobalTrace("if: defName != \"prompt-control-v2-benchmark\"")
		observe.GlobalTrace("return: nil")
		return nil
	}
	if err := requireTransitionHandoff(defName, transitions, "patch_planner", EventComplete, "checklist_writer", "checklist", false); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	for _, required := range []struct {
		from  string
		event string
		to    string
	}{
		{from: "route_item_block_classification", event: "repair_patch_plan", to: "patch_planner"},
		{from: "route_item_block_classification", event: "unresolved_ambiguity", to: "patch_planner"},
		{from: "route_final_verdict", event: "final_block", to: "patch_planner"},
	} {
		observe.GlobalTrace("range required")
		if err := requireTransitionHandoff(defName, transitions, required.from, required.event, required.to, "checklist", true); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: err")
			return err
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func requireTransitionHandoff(defName string, transitions map[TransitionKey]Transition, from string, event string, to string, artifactID string, required bool) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	tr, ok := transitions[TransitionKey{From: from, Event: event}]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: nil")
		return nil
	}
	if tr.To != to {
		observe.GlobalTrace("if: tr.To != to")
		observe.GlobalTrace("return: nil")
		return nil
	}
	for _, artifact := range tr.Handoff {
		observe.GlobalTrace("range tr.Handoff")
		if artifact.ID != artifactID {
			observe.GlobalTrace("if: artifact.ID != artifactID")
			continue
		}
		if required && !artifact.Required {
			observe.GlobalTrace("if: required && !artifact.Required")
			return fmt.Errorf("orchestration %q transition %s --%s--> %s must hand off required artifact %q", defName, from, event, to, artifactID)
		}
		observe.GlobalTrace("return: nil")
		return nil
	}
	if required {
		observe.GlobalTrace("if: required")
		return fmt.Errorf("orchestration %q transition %s --%s--> %s must hand off required artifact %q", defName, from, event, to, artifactID)
	}
	observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q transition %s --%s--> %s must hand off artifact %q\", ...")
	return fmt.Errorf("orchestration %q transition %s --%s--> %s must hand off artifact %q", defName, from, event, to, artifactID)
}

func validateForEachNextControl(defName, stateID string, control *ForEachNextControl) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if control.ListPath == "" {
		observe.GlobalTrace("if: control.ListPath == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q foreach_next requires list_path\", defNa...")
		return fmt.Errorf("orchestration %q state %q foreach_next requires list_path", defName, stateID)
	}
	if control.CursorPath == "" {
		observe.GlobalTrace("if: control.CursorPath == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q foreach_next requires cursor_path\", def...")
		return fmt.Errorf("orchestration %q state %q foreach_next requires cursor_path", defName, stateID)
	}
	if control.ItemEvent == "" {
		observe.GlobalTrace("if: control.ItemEvent == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q foreach_next requires item_event\", defN...")
		return fmt.Errorf("orchestration %q state %q foreach_next requires item_event", defName, stateID)
	}
	if control.DoneEvent == "" {
		observe.GlobalTrace("if: control.DoneEvent == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q foreach_next requires done_event\", defN...")
		return fmt.Errorf("orchestration %q state %q foreach_next requires done_event", defName, stateID)
	}
	if control.BlockedEvent != "" && strings.TrimSpace(control.BlockedEvent) == "" {
		return fmt.Errorf("orchestration %q state %q foreach_next has blank blocked_event", defName, stateID)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validateMarkCurrentItemControl(defName, stateID string, control *MarkCurrentItemControl) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if control.ListPath == "" {
		observe.GlobalTrace("if: control.ListPath == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q mark_current_item requires list_path\", ...")
		return fmt.Errorf("orchestration %q state %q mark_current_item requires list_path", defName, stateID)
	}
	if control.CursorPath == "" {
		observe.GlobalTrace("if: control.CursorPath == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q mark_current_item requires cursor_path\"...")
		return fmt.Errorf("orchestration %q state %q mark_current_item requires cursor_path", defName, stateID)
	}
	if control.Status == "" {
		observe.GlobalTrace("if: control.Status == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q mark_current_item requires status\", def...")
		return fmt.Errorf("orchestration %q state %q mark_current_item requires status", defName, stateID)
	}
	if control.Event == "" {
		observe.GlobalTrace("if: control.Event == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q mark_current_item requires event\", defN...")
		return fmt.Errorf("orchestration %q state %q mark_current_item requires event", defName, stateID)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validateArtifactVerdictControl(defName, stateID string, control *ArtifactVerdictControl) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if control.Path == "" {
		observe.GlobalTrace("if: control.Path == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_verdict requires path\", defNam...")
		return fmt.Errorf("orchestration %q state %q artifact_verdict requires path", defName, stateID)
	}
	if control.ApproveEvent == "" {
		observe.GlobalTrace("if: control.ApproveEvent == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_verdict requires approve_event...")
		return fmt.Errorf("orchestration %q state %q artifact_verdict requires approve_event", defName, stateID)
	}
	if control.BlockEvent == "" {
		observe.GlobalTrace("if: control.BlockEvent == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_verdict requires block_event\",...")
		return fmt.Errorf("orchestration %q state %q artifact_verdict requires block_event", defName, stateID)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func validateArtifactDecisionControl(defName, stateID string, control *ArtifactDecisionControl) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if control.Path == "" {
		observe.GlobalTrace("if: control.Path == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_decision requires path\", defNa...")
		return fmt.Errorf("orchestration %q state %q artifact_decision requires path", defName, stateID)
	}
	if len(control.Events) == 0 {
		observe.GlobalTrace("if: len(control.Events) == 0")
		observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_decision requires events\", def...")
		return fmt.Errorf("orchestration %q state %q artifact_decision requires events", defName, stateID)
	}
	for decision, event := range control.Events {
		observe.GlobalTrace("range control.Events")
		if strings.TrimSpace(decision) == "" {
			observe.GlobalTrace("if: strings.TrimSpace(decision) == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_decision has empty decision\", ...")
			return fmt.Errorf("orchestration %q state %q artifact_decision has empty decision", defName, stateID)
		}
		if strings.TrimSpace(event) == "" {
			observe.GlobalTrace("if: strings.TrimSpace(event) == \"\"")
			observe.GlobalTrace("return: fmt.Errorf(\"orchestration %q state %q artifact_decision decision %q has empty...")
			return fmt.Errorf("orchestration %q state %q artifact_decision decision %q has empty event", defName, stateID, decision)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func emittedEvents(state State) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if state.Control.ForEachNext != nil {
		observe.GlobalTrace("if: state.Control.ForEachNext != nil")
		events := []string{state.Control.ForEachNext.ItemEvent, state.Control.ForEachNext.DoneEvent}
		if state.Control.ForEachNext.BlockedEvent != "" {
			events = append(events, state.Control.ForEachNext.BlockedEvent)
		}
		return events
	}
	if state.Control.MarkCurrentItem != nil {
		observe.GlobalTrace("if: state.Control.MarkCurrentItem != nil")
		observe.GlobalTrace("return: []string{state.Control.MarkCurrentItem.Event}")
		return []string{state.Control.MarkCurrentItem.Event}
	}
	if state.Control.ArtifactVerdict != nil {
		observe.GlobalTrace("if: state.Control.ArtifactVerdict != nil")
		observe.GlobalTrace("return: []string{state.Control.ArtifactVerdict.ApproveEvent, state.Control.ArtifactVe...")
		return []string{state.Control.ArtifactVerdict.ApproveEvent, state.Control.ArtifactVerdict.BlockEvent}
	}
	if state.Control.ArtifactDecision != nil {
		observe.GlobalTrace("if: state.Control.ArtifactDecision != nil")
		events := make([]string, 0, len(state.Control.ArtifactDecision.Events))
		for _, event := range state.Control.ArtifactDecision.Events {
			observe.GlobalTrace("range state.Control.ArtifactDecision.Events")
			events = append(events, event)
		}
		observe.GlobalTrace("return: events")
		return events
	}
	if state.Event.Default != "" {
		observe.GlobalTrace("if: state.Event.Default != \"\"")
		observe.GlobalTrace("return: []string{state.Event.Default}")
		return []string{state.Event.Default}
	}
	observe.GlobalTrace("return: []string{EventComplete}")
	return []string{EventComplete}
}

func ExecuteControl(state State) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case state.Control.ForEachNext != nil:
		observe.GlobalTrace("case: state.Control.ForEachNext != nil")
		return executeForEachNext(*state.Control.ForEachNext)
	case state.Control.MarkCurrentItem != nil:
		observe.GlobalTrace("case: state.Control.MarkCurrentItem != nil")
		return executeMarkCurrentItem(*state.Control.MarkCurrentItem)
	case state.Control.ArtifactVerdict != nil:
		observe.GlobalTrace("case: state.Control.ArtifactVerdict != nil")
		return executeArtifactVerdict(*state.Control.ArtifactVerdict)
	case state.Control.ArtifactDecision != nil:
		observe.GlobalTrace("case: state.Control.ArtifactDecision != nil")
		return executeArtifactDecision(*state.Control.ArtifactDecision)
	default:
		observe.GlobalTrace("default")
		return "", fmt.Errorf("state %q has no control", state.ID)
	}
}

func executeForEachNext(control ForEachNextControl) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	checklist, err := readChecklist(control.ListPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
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
	if normalizeChecklistDependencies(&checklist, pendingStatus, blockedStatus) {
		if err := writeJSONFile(control.ListPath, checklist); err != nil {
			return "", err
		}
	}
	hasBlocked := false
	for _, item := range checklist.Items {
		observe.GlobalTrace("range checklist.Items")
		if item.Status == blockedStatus {
			hasBlocked = true
		}
		if item.Status != pendingStatus {
			observe.GlobalTrace("if: item.Status != pendingStatus")
			continue
		}
		if err := writeJSONFile(control.CursorPath, item); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: \"\", err")
			return "", err
		}
		observe.GlobalTrace("return: control.ItemEvent, nil")
		return control.ItemEvent, nil
	}
	if hasBlocked && control.BlockedEvent != "" {
		blockedCursor := ChecklistItem{
			Status:      blockedStatus,
			BlockReason: "no pending checklist items remain; blocked checklist items are waiting on prerequisites",
		}
		if err := writeJSONFile(control.CursorPath, blockedCursor); err != nil {
			return "", err
		}
		return control.BlockedEvent, nil
	}
	if err := writeJSONFile(control.CursorPath, ChecklistItem{Status: doneStatus}); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: control.DoneEvent, nil")
	return control.DoneEvent, nil
}

func executeMarkCurrentItem(control MarkCurrentItemControl) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	checklist, err := readChecklist(control.ListPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	current, err := readChecklistItem(control.CursorPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	if current.ID == "" {
		observe.GlobalTrace("if: current.ID == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"current item %q has empty id\", control.CursorPath)")
		return "", fmt.Errorf("current item %q has empty id", control.CursorPath)
	}
	found := false
	for i := range checklist.Items {
		observe.GlobalTrace("range checklist.Items")
		if checklist.Items[i].ID == current.ID {
			observe.GlobalTrace("if: checklist.Items[i].ID == current.ID")
			checklist.Items[i].Status = control.Status
			checklist.Items[i].BlockedBy = nil
			checklist.Items[i].BlockReason = ""
			found = true
			break
		}
	}
	if !found {
		observe.GlobalTrace("if: !found")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"current item %q not found in checklist %q\", current.ID, contr...")
		return "", fmt.Errorf("current item %q not found in checklist %q", current.ID, control.ListPath)
	}
	unblockDependents(&checklist, current.ID)
	if err := writeJSONFile(control.ListPath, checklist); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	current.Status = control.Status
	if err := writeJSONFile(control.CursorPath, current); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: control.Event, nil")
	return control.Event, nil
}

func normalizeChecklistDependencies(checklist *Checklist, pendingStatus, blockedStatus string) bool {
	ids := make(map[string]bool, len(checklist.Items))
	for _, item := range checklist.Items {
		ids[item.ID] = true
	}
	changed := false
	for idx := range checklist.Items {
		item := &checklist.Items[idx]
		if len(item.BlockedBy) > 0 {
			normalized := normalizeBlockedBy(item.BlockedBy, item.ID, ids)
			if !sameStringSlice(item.BlockedBy, normalized) {
				item.BlockedBy = normalized
				changed = true
			}
			if len(item.BlockedBy) > 0 && item.Status != blockedStatus {
				item.Status = blockedStatus
				changed = true
			}
			continue
		}
		deferredUntil := checklistItemStringExtra(item, "validation_deferred_until")
		if ids[deferredUntil] && deferredUntil != item.ID {
			item.BlockedBy = []string{deferredUntil}
			item.Status = blockedStatus
			if item.BlockReason == "" {
				item.BlockReason = fmt.Sprintf("validation deferred until checklist item %q completes", deferredUntil)
			}
			changed = true
			continue
		}
		if item.Status == blockedStatus && item.BlockReason == "" {
			item.BlockReason = "blocked without a structured prerequisite"
			changed = true
		}
	}
	return changed
}

func unblockDependents(checklist *Checklist, completedID string) {
	for idx := range checklist.Items {
		item := &checklist.Items[idx]
		if len(item.BlockedBy) == 0 {
			continue
		}
		remaining := make([]string, 0, len(item.BlockedBy))
		for _, blockedBy := range item.BlockedBy {
			if blockedBy != completedID {
				remaining = append(remaining, blockedBy)
			}
		}
		if len(remaining) == len(item.BlockedBy) {
			continue
		}
		item.BlockedBy = remaining
		if len(item.BlockedBy) == 0 && item.Status == "blocked" {
			item.Status = "pending"
			item.BlockReason = ""
		}
	}
}

func normalizeBlockedBy(values []string, self string, ids map[string]bool) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || value == self || !ids[value] || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func checklistItemStringExtra(item *ChecklistItem, key string) string {
	if item == nil || item.Extra == nil {
		return ""
	}
	raw, ok := item.Extra[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx] != b[idx] {
			return false
		}
	}
	return true
}

func executeArtifactVerdict(control ArtifactVerdictControl) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := os.ReadFile(control.Path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	decision, err := parseDecision(string(raw))
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"parse verdict %q: %w\", control.Path, err)")
		return "", fmt.Errorf("parse verdict %q: %w", control.Path, err)
	}
	approve := control.Approve
	if approve == "" {
		observe.GlobalTrace("if: approve == \"\"")
		approve = "APPROVE"
	}
	block := control.Block
	if block == "" {
		observe.GlobalTrace("if: block == \"\"")
		block = "BLOCK"
	}
	switch decision {
	case approve:
		observe.GlobalTrace("case: approve")
		return control.ApproveEvent, nil
	case block:
		observe.GlobalTrace("case: block")
		return control.BlockEvent, nil
	default:
		observe.GlobalTrace("default")
		return "", fmt.Errorf("verdict %q has unsupported decision %q", control.Path, decision)
	}
}

func executeArtifactDecision(control ArtifactDecisionControl) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := os.ReadFile(control.Path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	decision, err := parseJSONDecision(raw, control.Field)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"parse decision %q: %w\", control.Path, err)")
		return "", fmt.Errorf("parse decision %q: %w", control.Path, err)
	}
	event, ok := control.Events[decision]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"decision %q has unsupported value %q\", control.Path, decision)")
		return "", fmt.Errorf("decision %q has unsupported value %q", control.Path, decision)
	}
	observe.GlobalTrace("return: event, nil")
	return event, nil
}

func parseJSONDecision(raw []byte, field string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(field) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(field) == \"\"")
		field = "decision"
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"parse json: %w\", err)")
		return "", fmt.Errorf("parse json: %w", err)
	}
	value, ok := doc[field]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"missing %q field\", field)")
		return "", fmt.Errorf("missing %q field", field)
	}
	var decision string
	if err := json.Unmarshal(value, &decision); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"%q field must be a string\", field)")
		return "", fmt.Errorf("%q field must be a string", field)
	}
	decision = strings.TrimSpace(decision)
	if decision == "" {
		observe.GlobalTrace("if: decision == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"%q field is empty\", field)")
		return "", fmt.Errorf("%q field is empty", field)
	}
	observe.GlobalTrace("return: decision, nil")
	return decision, nil
}

func parseDecision(text string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		observe.GlobalTrace("range lines")
		if strings.TrimSpace(line) != "Decision:" {
			observe.GlobalTrace("if: strings.TrimSpace(line) != \"Decision:\"")
			continue
		}
		for _, candidate := range lines[i+1:] {
			observe.GlobalTrace("range lines[i+1:]")
			decision := strings.TrimSpace(candidate)
			if decision != "" {
				observe.GlobalTrace("if: decision != \"\"")
				observe.GlobalTrace("return: decision, nil")
				return decision, nil
			}
		}
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"decision marker has no value\")")
		return "", fmt.Errorf("decision marker has no value")
	}
	observe.GlobalTrace("return: \"\", fmt.Errorf(\"missing Decision: marker\")")
	return "", fmt.Errorf("missing Decision: marker")
}

func readChecklist(path string) (Checklist, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Checklist{}, err")
		return Checklist{}, err
	}
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Checklist{}, fmt.Errorf(\"parse json %q: %w\", path, err)")
		return Checklist{}, fmt.Errorf("parse json %q: %w", path, err)
	}
	checklist := Checklist{Items: make([]ChecklistItem, 0, len(envelope.Items))}
	for idx, rawItem := range envelope.Items {
		observe.GlobalTrace("range envelope.Items")
		var item ChecklistItem
		if err := json.Unmarshal(rawItem, &item); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: Checklist{}, fmt.Errorf(\"parse json %q: items[%d]: %w\", path, idx, err)")
			return Checklist{}, fmt.Errorf("parse json %q: items[%d]: %w", path, idx, err)
		}
		if strings.TrimSpace(item.ID) == "" {
			observe.GlobalTrace("if: strings.TrimSpace(item.ID) == \"\"")
			observe.GlobalTrace("return: Checklist{}, fmt.Errorf(\"parse json %q: items[%d].id must be a non-empty stri...")
			return Checklist{}, fmt.Errorf("parse json %q: items[%d].id must be a non-empty string", path, idx)
		}
		checklist.Items = append(checklist.Items, item)
	}
	observe.GlobalTrace("return: checklist, nil")
	return checklist, nil
}

func readChecklistItem(path string) (ChecklistItem, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var item ChecklistItem
	if err := readJSONFile(path, &item); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: ChecklistItem{}, err")
		return ChecklistItem{}, err
	}
	observe.GlobalTrace("return: item, nil")
	return item, nil
}

func readJSONFile(path string, dst any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"parse json %q: %w\", path, err)")
		return fmt.Errorf("parse json %q: %w", path, err)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func jsonValueKind(text string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case text == "":
		observe.GlobalTrace("case: text == \"\"")
		return "empty"
	case strings.HasPrefix(text, "\""):
		observe.GlobalTrace("case: strings.HasPrefix(text, \"\\\"\")")
		return "string"
	case strings.HasPrefix(text, "{"):
		observe.GlobalTrace("case: strings.HasPrefix(text, \"{\")")
		return "object"
	case strings.HasPrefix(text, "["):
		observe.GlobalTrace("case: strings.HasPrefix(text, \"[\")")
		return "array"
	case text == "true" || text == "false":
		observe.GlobalTrace("case: text == \"true\" || text == \"false\"")
		return "boolean"
	case text == "null":
		observe.GlobalTrace("case: text == \"null\"")
		return "null"
	default:
		observe.GlobalTrace("default")
		return "number or token"
	}
}

func writeJSONFile(path string, value any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	observe.GlobalTrace("return: nil")
	return nil
}

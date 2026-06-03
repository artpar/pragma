package orchestration

import (
	"encoding/json"
	"fmt"
	"os"

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
	ID         string  `yaml:"id"`
	Terminal   bool    `yaml:"terminal,omitempty"`
	Persona    string  `yaml:"persona,omitempty"`
	TaskPrompt string  `yaml:"task_prompt,omitempty"`
	Control    Control `yaml:"control,omitempty"`
	Event      Event   `yaml:"event,omitempty"`
}

type Control struct {
	ForEachNext     *ForEachNextControl     `yaml:"foreach_next,omitempty"`
	MarkCurrentItem *MarkCurrentItemControl `yaml:"mark_current_item,omitempty"`
}

func (c Control) IsZero() bool {
	return c.ForEachNext == nil && c.MarkCurrentItem == nil
}

type ForEachNextControl struct {
	ListPath      string `yaml:"list_path"`
	CursorPath    string `yaml:"cursor_path"`
	PendingStatus string `yaml:"pending_status,omitempty"`
	DoneStatus    string `yaml:"done_status,omitempty"`
	ItemEvent     string `yaml:"item_event"`
	DoneEvent     string `yaml:"done_event"`
}

type MarkCurrentItemControl struct {
	ListPath   string `yaml:"list_path"`
	CursorPath string `yaml:"cursor_path"`
	Status     string `yaml:"status"`
	Event      string `yaml:"event"`
}

type Event struct {
	Default  string         `yaml:"default,omitempty"`
	FromFile *FileEventRule `yaml:"from_file,omitempty"`
}

type FileEventRule struct {
	Path  string      `yaml:"path"`
	Rules []TextEvent `yaml:"rules"`
}

type TextEvent struct {
	Contains string `yaml:"contains"`
	Event    string `yaml:"event"`
}

type Transition struct {
	Event string   `yaml:"event"`
	From  []string `yaml:"from"`
	To    string   `yaml:"to"`
}

type Definition struct {
	Name        string       `yaml:"name"`
	Initial     string       `yaml:"initial"`
	States      []State      `yaml:"states"`
	Transitions []Transition `yaml:"transitions"`
}

type Runtime struct {
	Definition Definition
	States     map[string]State
	FSM        *fsm.FSM
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
	Extra       map[string]json.RawMessage
}

func (i *ChecklistItem) UnmarshalJSON(data []byte) error {
	type checklistItem ChecklistItem
	var known checklistItem
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, key := range []string{"id", "title", "description", "acceptance", "status"} {
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
	raw, err := json.Marshal(i.Status)
	if err != nil {
		return nil, err
	}
	fields["status"] = raw
	return json.Marshal(fields)
}

func NewRuntime(def Definition) (*Runtime, error) {
	states, err := validate(def)
	if err != nil {
		return nil, err
	}

	events := make([]fsm.EventDesc, 0, len(def.Transitions))
	for _, tr := range def.Transitions {
		events = append(events, fsm.EventDesc{
			Name: tr.Event,
			Src:  append([]string(nil), tr.From...),
			Dst:  tr.To,
		})
	}

	return &Runtime{
		Definition: def,
		States:     states,
		FSM:        fsm.NewFSM(def.Initial, events, nil),
	}, nil
}

func LoadDefinitionFile(path string) (Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	var def Definition
	if err := yaml.Unmarshal(raw, &def); err != nil {
		return Definition{}, fmt.Errorf("parse orchestration definition %q: %w", path, err)
	}
	if _, err := validate(def); err != nil {
		return Definition{}, err
	}
	return def, nil
}

func validate(def Definition) (map[string]State, error) {
	if def.Name == "" {
		return nil, fmt.Errorf("orchestration definition requires a name")
	}
	if def.Initial == "" {
		return nil, fmt.Errorf("orchestration %q requires an initial state", def.Name)
	}

	states := make(map[string]State, len(def.States))
	for _, state := range def.States {
		if state.ID == "" {
			return nil, fmt.Errorf("orchestration %q has a state with empty id", def.Name)
		}
		if _, exists := states[state.ID]; exists {
			return nil, fmt.Errorf("orchestration %q has duplicate state %q", def.Name, state.ID)
		}
		states[state.ID] = state
	}

	if _, ok := states[def.Initial]; !ok {
		return nil, fmt.Errorf("orchestration %q initial state %q is not defined", def.Name, def.Initial)
	}

	for _, tr := range def.Transitions {
		if tr.Event == "" {
			return nil, fmt.Errorf("orchestration %q has a transition with empty event", def.Name)
		}
		if len(tr.From) == 0 {
			return nil, fmt.Errorf("orchestration %q transition %q has no source states", def.Name, tr.Event)
		}
		if _, ok := states[tr.To]; !ok {
			return nil, fmt.Errorf("orchestration %q transition %q targets unknown state %q", def.Name, tr.Event, tr.To)
		}
		for _, from := range tr.From {
			state, ok := states[from]
			if !ok {
				return nil, fmt.Errorf("orchestration %q transition %q references unknown source state %q", def.Name, tr.Event, from)
			}
			if state.Terminal {
				return nil, fmt.Errorf("orchestration %q terminal state %q cannot be a transition source", def.Name, from)
			}
		}
	}

	transitionsByStateEvent := make(map[string]map[string]bool)
	for _, tr := range def.Transitions {
		for _, from := range tr.From {
			if _, ok := transitionsByStateEvent[from]; !ok {
				transitionsByStateEvent[from] = make(map[string]bool)
			}
			transitionsByStateEvent[from][tr.Event] = true
		}
	}

	for _, state := range states {
		if state.Terminal {
			if !state.Control.IsZero() {
				return nil, fmt.Errorf("orchestration %q terminal state %q cannot have control", def.Name, state.ID)
			}
			continue
		}
		if err := validateStateExecution(def.Name, state); err != nil {
			return nil, err
		}
		for _, event := range emittedEvents(state) {
			if !transitionsByStateEvent[state.ID][event] {
				return nil, fmt.Errorf("orchestration %q state %q can emit event %q but has no matching transition", def.Name, state.ID, event)
			}
		}
		if state.Event.FromFile != nil {
			if state.Event.FromFile.Path == "" {
				return nil, fmt.Errorf("orchestration %q state %q file event requires a path", def.Name, state.ID)
			}
			if len(state.Event.FromFile.Rules) == 0 {
				return nil, fmt.Errorf("orchestration %q state %q file event requires at least one rule", def.Name, state.ID)
			}
			for _, rule := range state.Event.FromFile.Rules {
				if rule.Contains == "" {
					return nil, fmt.Errorf("orchestration %q state %q file event rule requires contains text", def.Name, state.ID)
				}
				if rule.Event == "" {
					return nil, fmt.Errorf("orchestration %q state %q file event rule requires an event", def.Name, state.ID)
				}
			}
		}
	}

	return states, nil
}

func validateStateExecution(defName string, state State) error {
	if state.TaskPrompt != "" && state.TaskPrompt != TaskPromptFull && state.TaskPrompt != TaskPromptNone {
		return fmt.Errorf("orchestration %q state %q has invalid task_prompt %q", defName, state.ID, state.TaskPrompt)
	}
	if !state.Control.IsZero() {
		if state.Persona != "" {
			return fmt.Errorf("orchestration %q state %q cannot have both persona and control", defName, state.ID)
		}
		if state.TaskPrompt != "" {
			return fmt.Errorf("orchestration %q control state %q cannot define task_prompt", defName, state.ID)
		}
		if state.Event.FromFile != nil || state.Event.Default != "" {
			return fmt.Errorf("orchestration %q control state %q cannot also define event", defName, state.ID)
		}
		controls := 0
		if state.Control.ForEachNext != nil {
			controls++
			if err := validateForEachNextControl(defName, state.ID, state.Control.ForEachNext); err != nil {
				return err
			}
		}
		if state.Control.MarkCurrentItem != nil {
			controls++
			if err := validateMarkCurrentItemControl(defName, state.ID, state.Control.MarkCurrentItem); err != nil {
				return err
			}
		}
		if controls != 1 {
			return fmt.Errorf("orchestration %q control state %q must define exactly one control", defName, state.ID)
		}
	}
	return nil
}

func validateForEachNextControl(defName, stateID string, control *ForEachNextControl) error {
	if control.ListPath == "" {
		return fmt.Errorf("orchestration %q state %q foreach_next requires list_path", defName, stateID)
	}
	if control.CursorPath == "" {
		return fmt.Errorf("orchestration %q state %q foreach_next requires cursor_path", defName, stateID)
	}
	if control.ItemEvent == "" {
		return fmt.Errorf("orchestration %q state %q foreach_next requires item_event", defName, stateID)
	}
	if control.DoneEvent == "" {
		return fmt.Errorf("orchestration %q state %q foreach_next requires done_event", defName, stateID)
	}
	return nil
}

func validateMarkCurrentItemControl(defName, stateID string, control *MarkCurrentItemControl) error {
	if control.ListPath == "" {
		return fmt.Errorf("orchestration %q state %q mark_current_item requires list_path", defName, stateID)
	}
	if control.CursorPath == "" {
		return fmt.Errorf("orchestration %q state %q mark_current_item requires cursor_path", defName, stateID)
	}
	if control.Status == "" {
		return fmt.Errorf("orchestration %q state %q mark_current_item requires status", defName, stateID)
	}
	if control.Event == "" {
		return fmt.Errorf("orchestration %q state %q mark_current_item requires event", defName, stateID)
	}
	return nil
}

func emittedEvents(state State) []string {
	events := make([]string, 0, 1+len(fileEventRules(state)))
	if state.Control.ForEachNext != nil {
		return []string{state.Control.ForEachNext.ItemEvent, state.Control.ForEachNext.DoneEvent}
	}
	if state.Control.MarkCurrentItem != nil {
		return []string{state.Control.MarkCurrentItem.Event}
	}
	if state.Event.Default != "" {
		events = append(events, state.Event.Default)
	} else if state.Event.FromFile == nil {
		events = append(events, EventComplete)
	}
	for _, rule := range fileEventRules(state) {
		events = append(events, rule.Event)
	}
	return events
}

func fileEventRules(state State) []TextEvent {
	if state.Event.FromFile == nil {
		return nil
	}
	return state.Event.FromFile.Rules
}

func ExecuteControl(state State) (string, error) {
	switch {
	case state.Control.ForEachNext != nil:
		return executeForEachNext(*state.Control.ForEachNext)
	case state.Control.MarkCurrentItem != nil:
		return executeMarkCurrentItem(*state.Control.MarkCurrentItem)
	default:
		return "", fmt.Errorf("state %q has no control", state.ID)
	}
}

func executeForEachNext(control ForEachNextControl) (string, error) {
	checklist, err := readChecklist(control.ListPath)
	if err != nil {
		return "", err
	}
	pendingStatus := control.PendingStatus
	if pendingStatus == "" {
		pendingStatus = "pending"
	}
	doneStatus := control.DoneStatus
	if doneStatus == "" {
		doneStatus = "approved"
	}
	for _, item := range checklist.Items {
		if item.Status != pendingStatus {
			continue
		}
		if err := writeJSONFile(control.CursorPath, item); err != nil {
			return "", err
		}
		return control.ItemEvent, nil
	}
	if err := writeJSONFile(control.CursorPath, ChecklistItem{Status: doneStatus}); err != nil {
		return "", err
	}
	return control.DoneEvent, nil
}

func executeMarkCurrentItem(control MarkCurrentItemControl) (string, error) {
	checklist, err := readChecklist(control.ListPath)
	if err != nil {
		return "", err
	}
	current, err := readChecklistItem(control.CursorPath)
	if err != nil {
		return "", err
	}
	if current.ID == "" {
		return "", fmt.Errorf("current item %q has empty id", control.CursorPath)
	}
	found := false
	for i := range checklist.Items {
		if checklist.Items[i].ID == current.ID {
			checklist.Items[i].Status = control.Status
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("current item %q not found in checklist %q", current.ID, control.ListPath)
	}
	if err := writeJSONFile(control.ListPath, checklist); err != nil {
		return "", err
	}
	current.Status = control.Status
	if err := writeJSONFile(control.CursorPath, current); err != nil {
		return "", err
	}
	return control.Event, nil
}

func readChecklist(path string) (Checklist, error) {
	var checklist Checklist
	if err := readJSONFile(path, &checklist); err != nil {
		return Checklist{}, err
	}
	return checklist, nil
}

func readChecklistItem(path string) (ChecklistItem, error) {
	var item ChecklistItem
	if err := readJSONFile(path, &item); err != nil {
		return ChecklistItem{}, err
	}
	return item, nil
}

func readJSONFile(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("parse json %q: %w", path, err)
	}
	return nil
}

func writeJSONFile(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return nil
}

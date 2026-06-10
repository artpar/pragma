package orchestration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	return c.ForEachNext == nil && c.MarkCurrentItem == nil && c.ArtifactVerdict == nil && c.ArtifactDecision == nil
}

type ForEachNextControl struct {
	ListPath      string `yaml:"list_path"`
	CursorPath    string `yaml:"cursor_path"`
	HandoffPath   string `yaml:"handoff_path,omitempty"`
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
		if err := validateArtifacts(def.Name, state.ID, state.Artifacts); err != nil {
			return nil, err
		}
		for _, event := range emittedEvents(state) {
			if !transitionsByStateEvent[state.ID][event] {
				return nil, fmt.Errorf("orchestration %q state %q can emit event %q but has no matching transition", def.Name, state.ID, event)
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
		if state.Event.Default != "" {
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
		if state.Control.ArtifactVerdict != nil {
			controls++
			if err := validateArtifactVerdictControl(defName, state.ID, state.Control.ArtifactVerdict); err != nil {
				return err
			}
		}
		if state.Control.ArtifactDecision != nil {
			controls++
			if err := validateArtifactDecisionControl(defName, state.ID, state.Control.ArtifactDecision); err != nil {
				return err
			}
		}
		if controls != 1 {
			return fmt.Errorf("orchestration %q control state %q must define exactly one control", defName, state.ID)
		}
	}
	return nil
}

func validateArtifacts(defName, stateID string, artifacts Artifacts) error {
	for _, artifact := range append(append([]Artifact(nil), artifacts.Inputs...), artifacts.Outputs...) {
		if artifact.ID == "" {
			return fmt.Errorf("orchestration %q state %q artifact requires id", defName, stateID)
		}
		if artifact.Path == "" {
			return fmt.Errorf("orchestration %q state %q artifact %q requires path", defName, stateID, artifact.ID)
		}
		for _, value := range artifact.AllowedValues {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("orchestration %q state %q artifact %q has empty allowed value", defName, stateID, artifact.ID)
			}
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

func validateArtifactVerdictControl(defName, stateID string, control *ArtifactVerdictControl) error {
	if control.Path == "" {
		return fmt.Errorf("orchestration %q state %q artifact_verdict requires path", defName, stateID)
	}
	if control.ApproveEvent == "" {
		return fmt.Errorf("orchestration %q state %q artifact_verdict requires approve_event", defName, stateID)
	}
	if control.BlockEvent == "" {
		return fmt.Errorf("orchestration %q state %q artifact_verdict requires block_event", defName, stateID)
	}
	return nil
}

func validateArtifactDecisionControl(defName, stateID string, control *ArtifactDecisionControl) error {
	if control.Path == "" {
		return fmt.Errorf("orchestration %q state %q artifact_decision requires path", defName, stateID)
	}
	if len(control.Events) == 0 {
		return fmt.Errorf("orchestration %q state %q artifact_decision requires events", defName, stateID)
	}
	for decision, event := range control.Events {
		if strings.TrimSpace(decision) == "" {
			return fmt.Errorf("orchestration %q state %q artifact_decision has empty decision", defName, stateID)
		}
		if strings.TrimSpace(event) == "" {
			return fmt.Errorf("orchestration %q state %q artifact_decision decision %q has empty event", defName, stateID, decision)
		}
	}
	return nil
}

func emittedEvents(state State) []string {
	if state.Control.ForEachNext != nil {
		return []string{state.Control.ForEachNext.ItemEvent, state.Control.ForEachNext.DoneEvent}
	}
	if state.Control.MarkCurrentItem != nil {
		return []string{state.Control.MarkCurrentItem.Event}
	}
	if state.Control.ArtifactVerdict != nil {
		return []string{state.Control.ArtifactVerdict.ApproveEvent, state.Control.ArtifactVerdict.BlockEvent}
	}
	if state.Control.ArtifactDecision != nil {
		events := make([]string, 0, len(state.Control.ArtifactDecision.Events))
		for _, event := range state.Control.ArtifactDecision.Events {
			events = append(events, event)
		}
		return events
	}
	if state.Event.Default != "" {
		return []string{state.Event.Default}
	}
	return []string{EventComplete}
}

func ExecuteControl(state State) (string, error) {
	switch {
	case state.Control.ForEachNext != nil:
		return executeForEachNext(*state.Control.ForEachNext)
	case state.Control.MarkCurrentItem != nil:
		return executeMarkCurrentItem(*state.Control.MarkCurrentItem)
	case state.Control.ArtifactVerdict != nil:
		return executeArtifactVerdict(*state.Control.ArtifactVerdict)
	case state.Control.ArtifactDecision != nil:
		return executeArtifactDecision(*state.Control.ArtifactDecision)
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
		if control.HandoffPath != "" {
			if err := writeForEachItemHandoff(control.HandoffPath, item); err != nil {
				return "", err
			}
		}
		return control.ItemEvent, nil
	}
	if err := writeJSONFile(control.CursorPath, ChecklistItem{Status: doneStatus}); err != nil {
		return "", err
	}
	if control.HandoffPath != "" {
		if err := writeForEachDoneHandoff(control.HandoffPath, doneStatus); err != nil {
			return "", err
		}
	}
	return control.DoneEvent, nil
}

func writeForEachItemHandoff(path string, item ChecklistItem) error {
	raw, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal selected item handoff: %w", err)
	}
	var b strings.Builder
	b.WriteString("# Current Item Handoff\n\n")
	b.WriteString("This handoff is for the immediate next checklist item only.\n")
	b.WriteString("Do not implement or inspect future checklist items from this handoff.\n\n")
	b.WriteString("## Selected Item\n\n")
	b.WriteString("```json\n")
	b.Write(raw)
	b.WriteString("\n```\n")
	return writeTextFile(path, b.String())
}

func writeForEachDoneHandoff(path string, doneStatus string) error {
	var b strings.Builder
	b.WriteString("# Checklist Exhausted Handoff\n\n")
	fmt.Fprintf(&b, "No pending checklist items remain. The cursor status is `%s`.\n", doneStatus)
	return writeTextFile(path, b.String())
}

func writeTextFile(path string, content string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
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

func executeArtifactVerdict(control ArtifactVerdictControl) (string, error) {
	raw, err := os.ReadFile(control.Path)
	if err != nil {
		return "", err
	}
	decision, err := parseDecision(string(raw))
	if err != nil {
		return "", fmt.Errorf("parse verdict %q: %w", control.Path, err)
	}
	approve := control.Approve
	if approve == "" {
		approve = "APPROVE"
	}
	block := control.Block
	if block == "" {
		block = "BLOCK"
	}
	switch decision {
	case approve:
		return control.ApproveEvent, nil
	case block:
		return control.BlockEvent, nil
	default:
		return "", fmt.Errorf("verdict %q has unsupported decision %q", control.Path, decision)
	}
}

func executeArtifactDecision(control ArtifactDecisionControl) (string, error) {
	raw, err := os.ReadFile(control.Path)
	if err != nil {
		return "", err
	}
	decision, err := parseJSONDecision(raw, control.Field)
	if err != nil {
		return "", fmt.Errorf("parse decision %q: %w", control.Path, err)
	}
	event, ok := control.Events[decision]
	if !ok {
		return "", fmt.Errorf("decision %q has unsupported value %q", control.Path, decision)
	}
	return event, nil
}

func parseJSONDecision(raw []byte, field string) (string, error) {
	if strings.TrimSpace(field) == "" {
		field = "decision"
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	value, ok := doc[field]
	if !ok {
		return "", fmt.Errorf("missing %q field", field)
	}
	var decision string
	if err := json.Unmarshal(value, &decision); err != nil {
		return "", fmt.Errorf("%q field must be a string", field)
	}
	decision = strings.TrimSpace(decision)
	if decision == "" {
		return "", fmt.Errorf("%q field is empty", field)
	}
	return decision, nil
}

func parseDecision(text string) (string, error) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "Decision:" {
			continue
		}
		for _, candidate := range lines[i+1:] {
			decision := strings.TrimSpace(candidate)
			if decision != "" {
				return decision, nil
			}
		}
		return "", fmt.Errorf("decision marker has no value")
	}
	return "", fmt.Errorf("missing Decision: marker")
}

func readChecklist(path string) (Checklist, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Checklist{}, err
	}
	var envelope struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Checklist{}, fmt.Errorf("parse json %q: %w", path, err)
	}
	checklist := Checklist{Items: make([]ChecklistItem, 0, len(envelope.Items))}
	for idx, rawItem := range envelope.Items {
		var item ChecklistItem
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return Checklist{}, fmt.Errorf("parse json %q: items[%d]: %w", path, idx, err)
		}
		if strings.TrimSpace(item.ID) == "" {
			return Checklist{}, fmt.Errorf("parse json %q: items[%d].id must be a non-empty string", path, idx)
		}
		checklist.Items = append(checklist.Items, item)
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

func jsonValueKind(text string) string {
	switch {
	case text == "":
		return "empty"
	case strings.HasPrefix(text, "\""):
		return "string"
	case strings.HasPrefix(text, "{"):
		return "object"
	case strings.HasPrefix(text, "["):
		return "array"
	case text == "true" || text == "false":
		return "boolean"
	case text == "null":
		return "null"
	default:
		return "number or token"
	}
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

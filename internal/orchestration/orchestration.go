package orchestration

import (
	"fmt"
	"os"

	"github.com/looplab/fsm"
	"gopkg.in/yaml.v3"
)

const (
	EventComplete = "complete"
)

// State is one orchestration node. Execution semantics live outside the graph.
type State struct {
	ID       string `yaml:"id"`
	Terminal bool   `yaml:"terminal,omitempty"`
	Persona  string `yaml:"persona,omitempty"`
	Event    Event  `yaml:"event,omitempty"`
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
			continue
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

func emittedEvents(state State) []string {
	events := make([]string, 0, 1+len(fileEventRules(state)))
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

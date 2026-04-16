package testing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"gopkg.in/yaml.v3"
)

// Scenario describes a multi-turn agentic interaction for testing.
type Scenario struct {
	Name        string         `yaml:"name"`
	Model       string         `yaml:"model"`
	MaxTurns    int            `yaml:"max_turns,omitempty"`
	DenyTools   []string       `yaml:"deny_tools,omitempty"`
	ExpectError bool           `yaml:"expect_error,omitempty"` // if true, engine must return error
	Turns       []ScenarioTurn `yaml:"turns"`
}

// ScenarioTurn describes one turn in a scenario.
type ScenarioTurn struct {
	User             string               `yaml:"user,omitempty"`
	ProviderResponse yaml.Node            `yaml:"provider_response"`
	ToolResults      []ScenarioToolResult `yaml:"tool_results,omitempty"`
	ToolErrors       []ScenarioToolError  `yaml:"tool_errors,omitempty"`
	ExpectEvents     []EventMatcher       `yaml:"expect_events,omitempty"`
	ExpectNoEvents   []string             `yaml:"expect_no_events,omitempty"`
}

// ScenarioToolResult defines the output a tool should return when invoked.
type ScenarioToolResult struct {
	Name   string `yaml:"name"`
	Output string `yaml:"output"`
}

// ScenarioToolError defines a tool that should return an error.
type ScenarioToolError struct {
	Name  string `yaml:"name"`
	Error string `yaml:"error"`
}

// EventMatcher matches events by kind and optional field values.
type EventMatcher struct {
	Kind   string            `yaml:"kind"`
	Fields map[string]string `yaml:"fields,omitempty"`
}

// parseProviderResponse converts a YAML node to model.Response by going through JSON.
// This reuses model.Response.UnmarshalJSON which already handles ContentPart discrimination.
func parseProviderResponse(node *yaml.Node) (model.Response, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var raw interface{}
	if err := node.Decode(&raw); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"decode provider_response YAML: %w\", err)")
		return model.Response{}, fmt.Errorf("decode provider_response YAML: %w", err)
	}

	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"marshal provider_response to JSON: %w\", err)")
		return model.Response{}, fmt.Errorf("marshal provider_response to JSON: %w", err)
	}

	var resp model.Response
	if err := json.Unmarshal(jsonBytes, &resp); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: model.Response{}, fmt.Errorf(\"unmarshal provider_response from JSON: %w\", err)")
		return model.Response{}, fmt.Errorf("unmarshal provider_response from JSON: %w", err)
	}
	observe.GlobalTrace("return: resp, nil")
	return resp, nil
}

// LoadScenario parses a YAML scenario file.
func LoadScenario(path string) (*Scenario, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"read scenario %s: %w\", path, err)")
		return nil, fmt.Errorf("read scenario %s: %w", path, err)
	}

	var scenario Scenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse scenario %s: %w\", path, err)")
		return nil, fmt.Errorf("parse scenario %s: %w", path, err)
	}

	if scenario.Name == "" {
		observe.GlobalTrace("if: scenario.Name == \"\"")
		scenario.Name = filepath.Base(path)
	}
	observe.GlobalTrace("return: &scenario, nil")
	return &scenario, nil
}

// RunScenario loads and executes a YAML scenario file.
func RunScenario(t *testing.T, path string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.Helper()

	scenario, err := LoadScenario(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		t.Fatalf("load scenario: %v", err)
	}

	// Collect all provider responses across turns.
	var responses []model.Response
	for i, turn := range scenario.Turns {
		observe.GlobalTrace("range scenario.Turns")
		resp, err := parseProviderResponse(&turn.ProviderResponse)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			t.Fatalf("turn %d: %v", i, err)
		}
		responses = append(responses, resp)
	}

	h := NewHarness().WithProviderResponses(responses...)

	toolOutputs := make(map[string]string)
	for _, turn := range scenario.Turns {
		observe.GlobalTrace("range scenario.Turns")
		for _, tr := range turn.ToolResults {
			observe.GlobalTrace("range turn.ToolResults")
			toolOutputs[tr.Name] = tr.Output
		}
	}
	for name, output := range toolOutputs {
		observe.GlobalTrace("range toolOutputs")
		out := output
		h.WithTool(name, func(_ json.RawMessage) (string, error) {
			return out, nil
		})
	}

	for _, turn := range scenario.Turns {
		observe.GlobalTrace("range scenario.Turns")
		for _, te := range turn.ToolErrors {
			observe.GlobalTrace("range turn.ToolErrors")
			h.WithToolError(te.Name, te.Error)
		}
	}

	if len(scenario.DenyTools) > 0 {
		observe.GlobalTrace("if: len(scenario.DenyTools) > 0")
		h.WithPermissionDeny(scenario.DenyTools...)
	}

	if scenario.MaxTurns > 0 {
		observe.GlobalTrace("if: scenario.MaxTurns > 0")
		h.WithMaxTurns(scenario.MaxTurns)
	}

	userMessage := "test"
	for _, turn := range scenario.Turns {
		observe.GlobalTrace("range scenario.Turns")
		if turn.User != "" {
			observe.GlobalTrace("if: turn.User != \"\"")
			userMessage = turn.User
			break
		}
	}

	_, runErr := h.Run(t.Context(), userMessage)

	for i, turn := range scenario.Turns {
		observe.GlobalTrace("range scenario.Turns")
		for j, matcher := range turn.ExpectEvents {
			observe.GlobalTrace("range turn.ExpectEvents")
			if len(matcher.Fields) == 0 {
				observe.GlobalTrace("if: len(matcher.Fields) == 0")
				if err := h.AssertEvent(matcher.Kind, nil); err != nil {
					observe.GlobalTrace("if: err != nil")
					t.Errorf("turn %d, expect_events[%d]: %v", i, j, err)
				}
			} else {
				observe.GlobalTrace("else: len(matcher.Fields) == 0")
				for field, expected := range matcher.Fields {
					observe.GlobalTrace("range matcher.Fields")
					if err := h.AssertEventField(matcher.Kind, field, expected); err != nil {
						observe.GlobalTrace("if: err != nil")
						t.Errorf("turn %d, expect_events[%d]: %v", i, j, err)
					}
				}
			}
		}

		for _, kind := range turn.ExpectNoEvents {
			observe.GlobalTrace("range turn.ExpectNoEvents")
			if err := h.AssertNoEvent(kind); err != nil {
				observe.GlobalTrace("if: err != nil")
				t.Errorf("turn %d, expect_no_events: %v", i, err)
			}
		}
	}

	if scenario.ExpectError && runErr == nil {
		observe.GlobalTrace("if: scenario.ExpectError && runErr == nil")
		t.Errorf("scenario expects an error but engine returned nil")
	}
	if !scenario.ExpectError && runErr != nil {
		observe.GlobalTrace("if: !scenario.ExpectError && runErr != nil")
		t.Errorf("unexpected engine error: %v", runErr)
	}
}

// RunAllScenarios discovers *.yaml files in dir and runs each as a subtest.
func RunAllScenarios(t *testing.T, dir string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		t.Fatalf("glob scenarios: %v", err)
	}
	if len(matches) == 0 {
		observe.GlobalTrace("if: len(matches) == 0")
		t.Fatalf("no scenario files found in %s", dir)
	}

	for _, path := range matches {
		observe.GlobalTrace("range matches")
		name := filepath.Base(path)
		name = name[:len(name)-len(filepath.Ext(name))]
		t.Run(name, func(t *testing.T) {
			RunScenario(t, path)
		})
	}
}

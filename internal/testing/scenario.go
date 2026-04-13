package testing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/gogent/internal/model"
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
	var raw interface{}
	if err := node.Decode(&raw); err != nil {
		return model.Response{}, fmt.Errorf("decode provider_response YAML: %w", err)
	}

	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return model.Response{}, fmt.Errorf("marshal provider_response to JSON: %w", err)
	}

	var resp model.Response
	if err := json.Unmarshal(jsonBytes, &resp); err != nil {
		return model.Response{}, fmt.Errorf("unmarshal provider_response from JSON: %w", err)
	}
	return resp, nil
}

// LoadScenario parses a YAML scenario file.
func LoadScenario(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario %s: %w", path, err)
	}

	var scenario Scenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		return nil, fmt.Errorf("parse scenario %s: %w", path, err)
	}

	if scenario.Name == "" {
		scenario.Name = filepath.Base(path)
	}
	return &scenario, nil
}

// RunScenario loads and executes a YAML scenario file.
func RunScenario(t *testing.T, path string) {
	t.Helper()

	scenario, err := LoadScenario(path)
	if err != nil {
		t.Fatalf("load scenario: %v", err)
	}

	// Collect all provider responses across turns.
	var responses []model.Response
	for i, turn := range scenario.Turns {
		resp, err := parseProviderResponse(&turn.ProviderResponse)
		if err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
		responses = append(responses, resp)
	}

	// Build harness.
	h := NewHarness().WithProviderResponses(responses...)

	// Register tools from ToolResults (tool returns the specified output).
	toolOutputs := make(map[string]string)
	for _, turn := range scenario.Turns {
		for _, tr := range turn.ToolResults {
			toolOutputs[tr.Name] = tr.Output
		}
	}
	for name, output := range toolOutputs {
		out := output // capture
		h.WithTool(name, func(_ json.RawMessage) (string, error) {
			return out, nil
		})
	}

	// Register error tools.
	for _, turn := range scenario.Turns {
		for _, te := range turn.ToolErrors {
			h.WithToolError(te.Name, te.Error)
		}
	}

	// Configure permission denial.
	if len(scenario.DenyTools) > 0 {
		h.WithPermissionDeny(scenario.DenyTools...)
	}

	// Configure max turns.
	if scenario.MaxTurns > 0 {
		h.WithMaxTurns(scenario.MaxTurns)
	}

	// Find the first user message.
	userMessage := "test"
	for _, turn := range scenario.Turns {
		if turn.User != "" {
			userMessage = turn.User
			break
		}
	}

	// Run the engine.
	_, runErr := h.Run(t.Context(), userMessage)

	// Assert events for each turn. Since the engine processes all turns in a single Run,
	// we check all expected events and no-events against the full event list.
	for i, turn := range scenario.Turns {
		for j, matcher := range turn.ExpectEvents {
			if len(matcher.Fields) == 0 {
				if err := h.AssertEvent(matcher.Kind, nil); err != nil {
					t.Errorf("turn %d, expect_events[%d]: %v", i, j, err)
				}
			} else {
				for field, expected := range matcher.Fields {
					if err := h.AssertEventField(matcher.Kind, field, expected); err != nil {
						t.Errorf("turn %d, expect_events[%d]: %v", i, j, err)
					}
				}
			}
		}

		for _, kind := range turn.ExpectNoEvents {
			if err := h.AssertNoEvent(kind); err != nil {
				t.Errorf("turn %d, expect_no_events: %v", i, err)
			}
		}
	}

	// Verify error expectation.
	if scenario.ExpectError && runErr == nil {
		t.Errorf("scenario expects an error but engine returned nil")
	}
	if !scenario.ExpectError && runErr != nil {
		t.Errorf("unexpected engine error: %v", runErr)
	}
}

// RunAllScenarios discovers *.yaml files in dir and runs each as a subtest.
func RunAllScenarios(t *testing.T, dir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		t.Fatalf("glob scenarios: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no scenario files found in %s", dir)
	}

	for _, path := range matches {
		name := filepath.Base(path)
		name = name[:len(name)-len(filepath.Ext(name))]
		t.Run(name, func(t *testing.T) {
			RunScenario(t, path)
		})
	}
}

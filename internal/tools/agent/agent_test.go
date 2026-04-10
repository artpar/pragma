package agent

import "testing"

func TestExcludeToolNilReturnsNil(t *testing.T) {
	result := excludeTool(nil, "Agent")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestExcludeToolFilters(t *testing.T) {
	names := []string{"Bash", "Agent", "Read", "Write"}
	result := excludeTool(names, "Agent")
	if len(result) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(result))
	}
	for _, n := range result {
		if n == "Agent" {
			t.Error("Agent should have been excluded")
		}
	}
}

func TestExcludeToolNotPresent(t *testing.T) {
	names := []string{"Bash", "Read"}
	result := excludeTool(names, "Agent")
	if len(result) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result))
	}
}

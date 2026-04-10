package search

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

// testTool is a minimal Descriptor for testing search.
type testTool struct {
	name string
	desc string
	ro   bool
}

func (t *testTool) Name() string                { return t.name }
func (t *testTool) Description() string          { return t.desc }
func (t *testTool) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *testTool) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	return tool.InvokeResult{}, nil
}
func (t *testTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), t.name, "")
}
func (t *testTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: t.ro}
}

func newTestRegistry(tools ...tool.Descriptor) *tool.Registry {
	bus := observe.NewEventBus(16)
	defer bus.Drain()
	reg := tool.NewRegistry(bus)
	for _, t := range tools {
		_ = reg.Register(t)
	}
	return reg
}

func TestToolSearch(t *testing.T) {
	t.Run("name and flags", func(t *testing.T) {
		ts := &Tool{Registry: newTestRegistry()}
		if ts.Name() != "ToolSearch" {
			t.Fatalf("expected ToolSearch, got %s", ts.Name())
		}
		if !ts.Flags().ReadOnly {
			t.Fatal("expected read-only")
		}
		if !ts.Flags().Concurrent {
			t.Fatal("expected concurrent")
		}
	})

	t.Run("schema is valid JSON", func(t *testing.T) {
		ts := &Tool{Registry: newTestRegistry()}
		var schema map[string]any
		if err := json.Unmarshal(ts.InputSchema(), &schema); err != nil {
			t.Fatalf("invalid schema JSON: %v", err)
		}
	})

	t.Run("empty query rejected", func(t *testing.T) {
		ts := &Tool{Registry: newTestRegistry()}
		_, err := ts.Invoke(context.Background(), json.RawMessage(`{"query": ""}`), nil)
		if err == nil {
			t.Fatal("expected error for empty query")
		}
	})

	t.Run("select mode returns matching tools", func(t *testing.T) {
		reg := newTestRegistry(
			&testTool{name: "FileRead", desc: "Read", ro: true},
			&testTool{name: "Bash", desc: "Run", ro: false},
		)
		ts := &Tool{Registry: reg}
		result, err := ts.Invoke(context.Background(), json.RawMessage(`{"query": "select:FileRead,Bash"}`), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var res struct {
			Matches []struct{ Name string } `json:"matches"`
		}
		if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
			t.Fatalf("invalid result JSON: %v", err)
		}
		if len(res.Matches) != 2 {
			t.Fatalf("expected 2 matches, got %d", len(res.Matches))
		}
	})

	t.Run("keyword search finds tools", func(t *testing.T) {
		reg := newTestRegistry(
			&testTool{name: "FileRead", desc: "Read a file", ro: true},
			&testTool{name: "Bash", desc: "Run shell commands", ro: false},
		)
		ts := &Tool{Registry: reg}
		result, err := ts.Invoke(context.Background(), json.RawMessage(`{"query": "file"}`), nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var res struct {
			Matches []struct{ Name string } `json:"matches"`
		}
		if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
			t.Fatalf("invalid result JSON: %v", err)
		}
		if len(res.Matches) == 0 {
			t.Fatal("expected at least 1 match for 'file'")
		}
		if res.Matches[0].Name != "FileRead" {
			t.Fatalf("expected FileRead, got %s", res.Matches[0].Name)
		}
	})
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"FileReadTool", []string{"File", "Read", "Tool"}},
		{"AskUserQuestion", []string{"Ask", "User", "Question"}},
		{"Bash", []string{"Bash"}},
		{"LSP", []string{"L", "S", "P"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := splitCamelCase(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitCamelCase(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCamelCase(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSplitToolName(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"mcp__slack__send", []string{"mcp", "slack", "send"}},
		{"FileRead", []string{"File", "Read"}},
	}
	for _, tt := range tests {
		got := splitToolName(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitToolName(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitToolName(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestKeywordSearch(t *testing.T) {
	tools := []tool.Descriptor{
		&testTool{name: "FileRead", desc: "Read a file from disk", ro: true},
		&testTool{name: "FileWrite", desc: "Write a file to disk", ro: false},
		&testTool{name: "FileEdit", desc: "Edit a file in place", ro: false},
		&testTool{name: "Bash", desc: "Run shell commands", ro: false},
		&testTool{name: "Grep", desc: "Search file contents with regex", ro: true},
		&testTool{name: "mcp__slack__send", desc: "Send a Slack message", ro: false},
	}

	t.Run("keyword file matches file tools", func(t *testing.T) {
		results := keywordSearch(tools, "file", 10)
		if len(results) < 3 {
			t.Fatalf("expected at least 3 file matches, got %d", len(results))
		}
		names := make(map[string]bool)
		for _, r := range results {
			names[r.name] = true
		}
		for _, want := range []string{"FileRead", "FileWrite", "FileEdit"} {
			if !names[want] {
				t.Errorf("expected %s in results", want)
			}
		}
	})

	t.Run("keyword shell matches Bash", func(t *testing.T) {
		results := keywordSearch(tools, "shell", 5)
		if len(results) == 0 {
			t.Fatal("expected at least 1 match for 'shell'")
		}
		if results[0].name != "Bash" {
			t.Fatalf("expected Bash as top result, got %s", results[0].name)
		}
	})

	t.Run("MCP tool search", func(t *testing.T) {
		results := keywordSearch(tools, "slack", 5)
		if len(results) == 0 {
			t.Fatal("expected at least 1 match for 'slack'")
		}
		if results[0].name != "mcp__slack__send" {
			t.Fatalf("expected mcp__slack__send, got %s", results[0].name)
		}
	})

	t.Run("required term filters", func(t *testing.T) {
		results := keywordSearch(tools, "+file read", 5)
		if len(results) == 0 {
			t.Fatal("expected results for '+file read'")
		}
		if results[0].name != "FileRead" {
			t.Fatalf("expected FileRead as top result, got %s", results[0].name)
		}
	})

	t.Run("max results respected", func(t *testing.T) {
		results := keywordSearch(tools, "file", 2)
		if len(results) > 2 {
			t.Fatalf("expected at most 2 results, got %d", len(results))
		}
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		results := keywordSearch(tools, "nonexistent", 5)
		if len(results) != 0 {
			t.Fatalf("expected 0 results, got %d", len(results))
		}
	})
}

func TestSelectByNames(t *testing.T) {
	tools := []tool.Descriptor{
		&testTool{name: "FileRead", desc: "Read", ro: true},
		&testTool{name: "Bash", desc: "Run", ro: false},
		&testTool{name: "Grep", desc: "Search", ro: true},
	}

	t.Run("selects matching names case-insensitive", func(t *testing.T) {
		results := selectByNames(tools, []string{"fileread", "bash"})
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("missing names silently skipped", func(t *testing.T) {
		results := selectByNames(tools, []string{"FileRead", "NonExistent"})
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
	})
}

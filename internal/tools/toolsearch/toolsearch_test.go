package toolsearch

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

// fakeDescriptor is a minimal real Descriptor for testing.
type fakeDescriptor struct {
	name string
	desc string
}

func (f *fakeDescriptor) Name() string                { return f.name }
func (f *fakeDescriptor) Description() string          { return f.desc }
func (f *fakeDescriptor) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (f *fakeDescriptor) Invoke(_ context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	return tool.InvokeResult{}, nil
}
func (f *fakeDescriptor) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), f.name, "")
}
func (f *fakeDescriptor) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func setupRegistry(t *testing.T) *tool.Registry {
	t.Helper()
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	reg := tool.NewRegistry(bus)

	tools := []struct{ name, desc string }{
		{"FileRead", "Read file contents from disk"},
		{"FileWrite", "Write content to a file on disk"},
		{"Bash", "Execute a bash command"},
		{"Grep", "Search file contents with regex"},
		{"GlobTool", "Find files matching glob patterns"},
		{"mcp__server1__get_data", "Get data from MCP server"},
		{"mcp__server1__list_items", "List items from MCP server"},
		{"mcp__server2__get_handoff", "Get handoff from server2"},
	}

	for _, td := range tools {
		if err := reg.Register(&fakeDescriptor{name: td.name, desc: td.desc}); err != nil {
			t.Fatalf("register %s: %v", td.name, err)
		}
	}
	return reg
}

func TestToolSearch_Name(t *testing.T) {
	tl := &Tool{}
	if tl.Name() != "ToolSearch" {
		t.Fatalf("expected ToolSearch, got %s", tl.Name())
	}
}

func TestToolSearch_Flags(t *testing.T) {
	tl := &Tool{}
	flags := tl.Flags()
	if !flags.ReadOnly {
		t.Fatal("expected ReadOnly")
	}
	if !flags.Concurrent {
		t.Fatal("expected Concurrent")
	}
}

func TestToolSearch_Invoke_EmptyQuery(t *testing.T) {
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	reg := tool.NewRegistry(bus)
	tl := &Tool{Registry: reg}
	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"query":""}`), nil)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestToolSearch_Invoke_InvalidJSON(t *testing.T) {
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	reg := tool.NewRegistry(bus)
	tl := &Tool{Registry: reg}
	_, err := tl.Invoke(context.Background(), json.RawMessage(`{bad`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestToolSearch_SelectMode(t *testing.T) {
	reg := setupRegistry(t)
	tl := &Tool{Registry: reg}

	tests := []struct {
		name     string
		query    string
		wantLen  int
		wantName string
	}{
		{"exact match single", `{"query":"select:Bash"}`, 1, "Bash"},
		{"exact match multiple", `{"query":"select:Bash,Grep"}`, 2, ""},
		{"suffix match", `{"query":"select:get_handoff"}`, 1, "mcp__server2__get_handoff"},
		{"no match", `{"query":"select:NonExistent"}`, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.query), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var out struct {
				Matches []string `json:"matches"`
			}
			if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if len(out.Matches) != tt.wantLen {
				t.Fatalf("expected %d matches, got %d: %v", tt.wantLen, len(out.Matches), out.Matches)
			}
			if tt.wantName != "" && len(out.Matches) > 0 && out.Matches[0] != tt.wantName {
				t.Fatalf("expected first match %q, got %q", tt.wantName, out.Matches[0])
			}
		})
	}
}

func TestToolSearch_ExactNameMode(t *testing.T) {
	reg := setupRegistry(t)
	tl := &Tool{Registry: reg}

	result, err := tl.Invoke(context.Background(), json.RawMessage(`{"query":"Bash"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Matches []string `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(out.Matches) != 1 || out.Matches[0] != "Bash" {
		t.Fatalf("expected [Bash], got %v", out.Matches)
	}
}

func TestToolSearch_MCPPrefixMode(t *testing.T) {
	reg := setupRegistry(t)
	tl := &Tool{Registry: reg}

	result, err := tl.Invoke(context.Background(), json.RawMessage(`{"query":"mcp__server1__"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out struct {
		Matches []string `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(out.Matches) != 2 {
		t.Fatalf("expected 2 MCP matches, got %d: %v", len(out.Matches), out.Matches)
	}
}

func TestToolSearch_KeywordMode(t *testing.T) {
	reg := setupRegistry(t)
	tl := &Tool{Registry: reg}

	tests := []struct {
		name    string
		query   string
		wantMin int
	}{
		{"keyword file", `{"query":"file"}`, 2},
		{"required term", `{"query":"+bash"}`, 1},
		{"required term missing", `{"query":"+nonexistent"}`, 0},
		{"max results", `{"query":"file","max_results":1}`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.query), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var out struct {
				Matches []string `json:"matches"`
			}
			if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if tt.name == "max results" {
				if len(out.Matches) > 1 {
					t.Fatalf("expected at most 1 result, got %d", len(out.Matches))
				}
			} else if len(out.Matches) < tt.wantMin {
				t.Fatalf("expected at least %d matches, got %d: %v", tt.wantMin, len(out.Matches), out.Matches)
			}
		})
	}
}

func TestParseToolName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{"camel case", "FileRead", []string{"file", "read"}},
		{"mcp name", "mcp__server__tool_name", []string{"mcp", "server", "tool", "name"}},
		{"single word", "Bash", []string{"bash"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := parseToolName(tt.input)
			if len(parts) != len(tt.expect) {
				t.Fatalf("expected %v, got %v", tt.expect, parts)
			}
			for i, p := range parts {
				if p != tt.expect[i] {
					t.Fatalf("part %d: expected %q, got %q", i, tt.expect[i], p)
				}
			}
		})
	}
}

func TestScoreTerm(t *testing.T) {
	tests := []struct {
		name     string
		term     string
		parts    []string
		nameL    string
		desc     string
		wantZero bool
	}{
		{"exact part match", "file", []string{"file", "read"}, "fileread", "read file contents", false},
		{"partial part match", "fil", []string{"file", "read"}, "fileread", "", false},
		{"description match", "contents", []string{"file", "read"}, "fileread", "read file contents", false},
		{"no match", "zzzz", []string{"file", "read"}, "fileread", "read file", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scoreTerm(tt.term, tt.parts, tt.nameL, tt.desc)
			if tt.wantZero && score != 0 {
				t.Fatalf("expected 0 score, got %d", score)
			}
			if !tt.wantZero && score == 0 {
				t.Fatal("expected non-zero score")
			}
		})
	}
}

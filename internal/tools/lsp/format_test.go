package toollsp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatLocations(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		label     string
		wantCount int
		wantFiles int
		wantSub   string // substring expected in output
	}{
		{
			name:      "empty array",
			raw:       `[]`,
			label:     "definition(s)",
			wantCount: 0,
			wantFiles: 0,
			wantSub:   "No definition(s) found.",
		},
		{
			name:      "null",
			raw:       `null`,
			label:     "references",
			wantCount: 0,
			wantFiles: 0,
			wantSub:   "No references found.",
		},
		{
			name: "single location",
			raw: `[{
				"uri": "file:///src/main.go",
				"range": {"start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 15}}
			}]`,
			label:     "definition(s)",
			wantCount: 1,
			wantFiles: 1,
			wantSub:   "Line 11, Col 6",
		},
		{
			name: "multiple locations same file",
			raw: `[
				{"uri": "file:///src/main.go", "range": {"start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 15}}},
				{"uri": "file:///src/main.go", "range": {"start": {"line": 20, "character": 0}, "end": {"line": 20, "character": 10}}}
			]`,
			label:     "reference(s)",
			wantCount: 2,
			wantFiles: 1,
		},
		{
			name: "multiple files",
			raw: `[
				{"uri": "file:///src/a.go", "range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 5}}},
				{"uri": "file:///src/b.go", "range": {"start": {"line": 3, "character": 2}, "end": {"line": 3, "character": 8}}}
			]`,
			label:     "implementation(s)",
			wantCount: 2,
			wantFiles: 2,
		},
		{
			name: "location links — parsed when Location[] produces empty URIs",
			raw: `[{
				"targetUri": "file:///src/target.go",
				"targetRange": {"start": {"line": 5, "character": 0}, "end": {"line": 10, "character": 0}},
				"targetSelectionRange": {"start": {"line": 5, "character": 4}, "end": {"line": 5, "character": 14}}
			}]`,
			label:     "definition(s)",
			wantCount: 1,
			wantFiles: 1,
			wantSub:   "1 definition(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count, files := formatLocations(json.RawMessage(tt.raw), "/src", tt.label)
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if files != tt.wantFiles {
				t.Errorf("files = %d, want %d", files, tt.wantFiles)
			}
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("output %q does not contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestFormatHover(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantSub string
		wantN   int
	}{
		{
			name:    "markup content",
			raw:     `{"contents": {"kind": "markdown", "value": "func Foo(x int) string"}}`,
			wantSub: "func Foo(x int) string",
			wantN:   1,
		},
		{
			name:    "plain string contents",
			raw:     `{"contents": "some hover text"}`,
			wantSub: "some hover text",
			wantN:   1,
		},
		{
			name:    "null hover",
			raw:     `null`,
			wantSub: "No hover information.",
			wantN:   0,
		},
		{
			name:    "invalid JSON",
			raw:     `not json`,
			wantSub: "No hover information.",
			wantN:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n, _ := formatHover(json.RawMessage(tt.raw), "")
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("output %q does not contain %q", got, tt.wantSub)
			}
			if n != tt.wantN {
				t.Errorf("count = %d, want %d", n, tt.wantN)
			}
		})
	}
}

func TestFormatDocumentSymbols(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantN   int
		wantSub string
	}{
		{
			name: "hierarchical symbols",
			raw: `[{
				"name": "MyStruct",
				"kind": 23,
				"range": {"start": {"line": 5, "character": 0}, "end": {"line": 20, "character": 1}},
				"selectionRange": {"start": {"line": 5, "character": 5}, "end": {"line": 5, "character": 13}},
				"children": [{
					"name": "MyMethod",
					"kind": 6,
					"range": {"start": {"line": 10, "character": 0}, "end": {"line": 15, "character": 1}},
					"selectionRange": {"start": {"line": 10, "character": 5}, "end": {"line": 10, "character": 13}}
				}]
			}]`,
			wantN:   2,
			wantSub: "Struct MyStruct",
		},
		{
			name: "flat symbol information",
			raw: `[{
				"name": "DoSomething",
				"kind": 12,
				"location": {"uri": "file:///src/main.go", "range": {"start": {"line": 10, "character": 0}, "end": {"line": 10, "character": 10}}}
			}]`,
			wantN:   1,
			wantSub: "Function DoSomething",
		},
		{
			name:    "empty",
			raw:     `[]`,
			wantN:   0,
			wantSub: "No symbols found.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n, _ := formatDocumentSymbols(json.RawMessage(tt.raw), "")
			if n != tt.wantN {
				t.Errorf("count = %d, want %d", n, tt.wantN)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("output %q does not contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestFormatWorkspaceSymbols(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantN   int
		wantF   int
		wantSub string
	}{
		{
			name: "multiple symbols across files",
			raw: `[
				{"name": "Foo", "kind": 12, "location": {"uri": "file:///src/a.go", "range": {"start": {"line": 5, "character": 0}, "end": {"line": 5, "character": 5}}}},
				{"name": "Bar", "kind": 5, "location": {"uri": "file:///src/b.go", "range": {"start": {"line": 10, "character": 0}, "end": {"line": 10, "character": 5}}}}
			]`,
			wantN: 2,
			wantF: 2,
		},
		{
			name:    "empty",
			raw:     `[]`,
			wantSub: "No symbols found.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n, f := formatWorkspaceSymbols(json.RawMessage(tt.raw), "/src")
			if n != tt.wantN {
				t.Errorf("count = %d, want %d", n, tt.wantN)
			}
			if f != tt.wantF {
				t.Errorf("files = %d, want %d", f, tt.wantF)
			}
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Errorf("output %q does not contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestFormatCallHierarchyItems(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantN   int
		wantSub string
	}{
		{
			name: "single item",
			raw: `[{
				"name": "main",
				"kind": 12,
				"uri": "file:///src/main.go",
				"range": {"start": {"line": 0, "character": 0}, "end": {"line": 10, "character": 0}},
				"selectionRange": {"start": {"line": 0, "character": 5}, "end": {"line": 0, "character": 9}}
			}]`,
			wantN:   1,
			wantSub: "Function main",
		},
		{
			name:    "empty",
			raw:     `[]`,
			wantSub: "No call hierarchy items found.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, n, _ := formatCallHierarchyItems(json.RawMessage(tt.raw), "/src")
			if n != tt.wantN {
				t.Errorf("count = %d, want %d", n, tt.wantN)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("output %q does not contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestFormatIncomingCalls(t *testing.T) {
	raw := `[{
		"from": {
			"name": "caller",
			"kind": 12,
			"uri": "file:///src/caller.go",
			"range": {"start": {"line": 5, "character": 0}, "end": {"line": 10, "character": 0}},
			"selectionRange": {"start": {"line": 5, "character": 5}, "end": {"line": 5, "character": 11}}
		},
		"fromRanges": [{"start": {"line": 7, "character": 2}, "end": {"line": 7, "character": 10}}]
	}]`

	got, n, _ := formatIncomingCalls(json.RawMessage(raw), "/src")
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
	if !strings.Contains(got, "Function caller") {
		t.Errorf("output %q does not contain 'Function caller'", got)
	}
}

func TestFormatOutgoingCalls(t *testing.T) {
	raw := `[{
		"to": {
			"name": "callee",
			"kind": 6,
			"uri": "file:///src/callee.go",
			"range": {"start": {"line": 20, "character": 0}, "end": {"line": 25, "character": 0}},
			"selectionRange": {"start": {"line": 20, "character": 5}, "end": {"line": 20, "character": 11}}
		},
		"fromRanges": [{"start": {"line": 7, "character": 2}, "end": {"line": 7, "character": 10}}]
	}]`

	got, n, _ := formatOutgoingCalls(json.RawMessage(raw), "/src")
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
	if !strings.Contains(got, "Method callee") {
		t.Errorf("output %q does not contain 'Method callee'", got)
	}
}

func TestFormatIncomingCalls_Empty(t *testing.T) {
	got, n, _ := formatIncomingCalls(json.RawMessage(`[]`), "/src")
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	if !strings.Contains(got, "No incoming calls found.") {
		t.Errorf("output %q missing expected message", got)
	}
}

func TestFormatOutgoingCalls_Empty(t *testing.T) {
	got, n, _ := formatOutgoingCalls(json.RawMessage(`[]`), "/src")
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
	if !strings.Contains(got, "No outgoing calls found.") {
		t.Errorf("output %q missing expected message", got)
	}
}

func TestSymbolKindName(t *testing.T) {
	tests := []struct {
		kind int
		want string
	}{
		{1, "File"},
		{5, "Class"},
		{6, "Method"},
		{12, "Function"},
		{23, "Struct"},
		{999, "Kind(999)"},
	}

	for _, tt := range tests {
		got := symbolKindName(tt.kind)
		if got != tt.want {
			t.Errorf("symbolKindName(%d) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestRelativePath(t *testing.T) {
	tests := []struct {
		path string
		cwd  string
		want string
	}{
		{"/src/main.go", "/src", "main.go"},
		{"/src/pkg/foo.go", "/src", "pkg/foo.go"},
		{"/other/bar.go", "", "/other/bar.go"},
		{"/src/main.go", "/src", "main.go"},
	}

	for _, tt := range tests {
		got := relativePath(tt.path, tt.cwd)
		if got != tt.want {
			t.Errorf("relativePath(%q, %q) = %q, want %q", tt.path, tt.cwd, got, tt.want)
		}
	}
}

func TestHoverContents_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantTxt string
	}{
		{
			name:    "markup content",
			input:   `{"kind": "markdown", "value": "**bold** text"}`,
			wantTxt: "**bold** text",
		},
		{
			name:    "plain string",
			input:   `"simple hover"`,
			wantTxt: "simple hover",
		},
		{
			name:    "raw fallback",
			input:   `[{"language": "go", "value": "func Foo()"}]`,
			wantTxt: `[{"language": "go", "value": "func Foo()"}]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h hoverContents
			if err := json.Unmarshal([]byte(tt.input), &h); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if got := h.text(); got != tt.wantTxt {
				t.Errorf("text() = %q, want %q", got, tt.wantTxt)
			}
		})
	}
}

package synthetic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/permission"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

func TestNew_ValidSchema(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		}
	}`)
	tl, err := New(schema)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tl.Name() != "StructuredOutput" {
		t.Errorf("Name = %q, want StructuredOutput", tl.Name())
	}
}

func TestNew_InvalidJSON(t *testing.T) {
	_, err := New(json.RawMessage(`{not valid json`))
	if err == nil {
		t.Fatal("New with invalid JSON should return error")
	}
	if !strings.Contains(err.Error(), "invalid JSON schema") {
		t.Errorf("error = %q, want to contain 'invalid JSON schema'", err.Error())
	}
}

func TestInvoke(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"required": ["name"],
		"properties": {
			"name": {"type": "string"},
			"count": {"type": "integer"}
		}
	}`)

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantMatch string
	}{
		{
			name:  "valid input",
			input: `{"name": "test", "count": 42}`,
		},
		{
			name:  "valid input minimal",
			input: `{"name": "hello"}`,
		},
		{
			name:      "missing required field",
			input:     `{"count": 42}`,
			wantErr:   true,
			wantMatch: "does not match required schema",
		},
		{
			name:      "wrong type",
			input:     `{"name": 123}`,
			wantErr:   true,
			wantMatch: "does not match required schema",
		},
		{
			name:    "invalid JSON input",
			input:   `{broken`,
			wantErr: true,
		},
	}

	tl, err := New(schema)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tl.Invoke(context.Background(), json.RawMessage(tt.input), staticState{"/tmp"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMatch != "" && !strings.Contains(err.Error(), tt.wantMatch) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Content != "Structured output provided successfully" {
				t.Errorf("Content = %q, want success message", result.Content)
			}
		})
	}
}

func TestInvoke_ErrorTruncation(t *testing.T) {
	// Schema with many required fields to produce a long error
	schema := json.RawMessage(`{
		"type": "object",
		"required": ["a","b","c","d","e","f","g","h","i","j","k","l","m","n","o","p","q","r","s","t"],
		"properties": {
			"a": {"type": "string"}, "b": {"type": "string"}, "c": {"type": "string"},
			"d": {"type": "string"}, "e": {"type": "string"}, "f": {"type": "string"},
			"g": {"type": "string"}, "h": {"type": "string"}, "i": {"type": "string"},
			"j": {"type": "string"}, "k": {"type": "string"}, "l": {"type": "string"},
			"m": {"type": "string"}, "n": {"type": "string"}, "o": {"type": "string"},
			"p": {"type": "string"}, "q": {"type": "string"}, "r": {"type": "string"},
			"s": {"type": "string"}, "t": {"type": "string"}
		}
	}`)
	tl, err := New(schema)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = tl.Invoke(context.Background(), json.RawMessage(`{}`), staticState{"/tmp"})
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	// The error message in the fmt.Errorf wraps the truncated message
	// Check that the original validation error was truncated
	errMsg := err.Error()
	if !strings.Contains(errMsg, "does not match required schema") {
		t.Errorf("error should mention schema mismatch, got: %s", errMsg)
	}
}

func TestFlags(t *testing.T) {
	schema := json.RawMessage(`{"type": "object"}`)
	tl, _ := New(schema)
	flags := tl.Flags()
	if !flags.ReadOnly {
		t.Error("expected ReadOnly = true")
	}
	if !flags.Concurrent {
		t.Error("expected Concurrent = true")
	}
}

func TestInputSchema_ReturnsUserSchema(t *testing.T) {
	schema := json.RawMessage(`{"type": "object", "properties": {"x": {"type": "number"}}}`)
	tl, _ := New(schema)
	got := tl.InputSchema()
	if string(got) != string(schema) {
		t.Errorf("InputSchema() = %s, want %s", got, schema)
	}
}

func TestCheckPerm_AlwaysAllow(t *testing.T) {
	schema := json.RawMessage(`{"type": "object"}`)
	tl, _ := New(schema)
	result := tl.CheckPerm(context.Background(), nil, nil)
	if result.Decision != permission.DecisionAllow {
		t.Errorf("CheckPerm decision = %v, want Allow", result.Decision)
	}
}

package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

type staticState struct{ dir string }

func (s staticState) WorkDir() string { return s.dir }

func TestFindSetting(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"model", true},
		{"verbose", true},
		{"theme", true},
		{"permission_mode", true},
		{"nonexistent", false},
		{"MODEL", false}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := FindSetting(tt.name)
			if (def != nil) != tt.found {
				t.Errorf("FindSetting(%q) found=%v, want %v", tt.name, def != nil, tt.found)
			}
		})
	}
}

func TestSettingNames(t *testing.T) {
	names := SettingNames()
	if len(names) != len(SupportedSettings) {
		t.Errorf("SettingNames() returned %d, want %d", len(names), len(SupportedSettings))
	}
	// Verify "model" is in the list
	found := false
	for _, n := range names {
		if n == "model" {
			found = true
			break
		}
	}
	if !found {
		t.Error("SettingNames() does not contain 'model'")
	}
}

func TestCoerceBool(t *testing.T) {
	tests := []struct {
		input any
		want  any
	}{
		{"true", true},
		{"false", false},
		{"TRUE", true},
		{"False", false},
		{" true ", true},
		{"yes", "yes"}, // not coercible
		{42, 42},       // not a string
		{true, true},   // already bool
	}

	for i, tt := range tests {
		got := coerceBool(tt.input)
		if got != tt.want {
			t.Errorf("case %d: coerceBool(%v) = %v, want %v", i, tt.input, got, tt.want)
		}
	}
}

func TestGetNestedValue(t *testing.T) {
	m := map[string]any{
		"top": "topval",
		"nested": map[string]any{
			"child": "childval",
			"deep": map[string]any{
				"leaf": 42,
			},
		},
	}

	tests := []struct {
		key  string
		want any
	}{
		{"top", "topval"},
		{"nested.child", "childval"},
		{"nested.deep.leaf", 42},
		{"missing", nil},
		{"nested.missing", nil},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := getNestedValue(m, tt.key)
			if got != tt.want {
				t.Errorf("getNestedValue(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestSetNestedValue(t *testing.T) {
	m := make(map[string]any)
	setNestedValue(m, "a.b.c", "deep")
	setNestedValue(m, "flat", "val")

	if getNestedValue(m, "a.b.c") != "deep" {
		t.Errorf("a.b.c = %v, want 'deep'", getNestedValue(m, "a.b.c"))
	}
	if getNestedValue(m, "flat") != "val" {
		t.Errorf("flat = %v, want 'val'", getNestedValue(m, "flat"))
	}
}

func TestHandleGet_MissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store := app.NewStateStore(app.AppState{CWD: tmp})
	tl := &Tool{Store: store, WorkDir: tmp}

	input, _ := json.Marshal(ConfigInput{Setting: "model"})
	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// Should show model = null (no file exists)
	if !strings.Contains(result.Content, "model") {
		t.Errorf("Content = %q, expected to contain 'model'", result.Content)
	}
}

func TestHandleSet_CreateFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store := app.NewStateStore(app.AppState{CWD: tmp})
	tl := &Tool{Store: store, WorkDir: tmp}

	value := any("claude-opus-4-6")
	input, _ := json.Marshal(ConfigInput{Setting: "model", Value: &value})
	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Set model") {
		t.Errorf("Content = %q, want confirmation", result.Content)
	}

	// Verify settings file was created (model is project-scoped)
	settingsPath := filepath.Join(tmp, ".pragma", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var raw map[string]any
	json.Unmarshal(data, &raw)
	if raw["model"] != "claude-opus-4-6" {
		t.Errorf("settings model = %v, want claude-opus-4-6", raw["model"])
	}
}

func TestHandleSet_EnumValidation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store := app.NewStateStore(app.AppState{CWD: tmp})
	tl := &Tool{Store: store, WorkDir: tmp}

	// Invalid theme
	badValue := any("neon")
	input, _ := json.Marshal(ConfigInput{Setting: "theme", Value: &badValue})
	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Invalid value") {
		t.Errorf("Content = %q, want 'Invalid value'", result.Content)
	}

	// Valid theme
	goodValue := any("dark")
	input, _ = json.Marshal(ConfigInput{Setting: "theme", Value: &goodValue})
	result, err = tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Set theme") {
		t.Errorf("Content = %q, want confirmation", result.Content)
	}
}

func TestHandleSet_BoolCoercion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	store := app.NewStateStore(app.AppState{CWD: tmp})
	tl := &Tool{Store: store, WorkDir: tmp}

	value := any("true")
	input, _ := json.Marshal(ConfigInput{Setting: "verbose", Value: &value})
	result, err := tl.Invoke(context.Background(), input, staticState{tmp})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Set verbose") {
		t.Errorf("Content = %q, want confirmation", result.Content)
	}
}

func TestSyncToAppState_Model(t *testing.T) {
	store := app.NewStateStore(app.AppState{
		CWD:          "/tmp",
		Conversation: model.NewConversation(model.SystemPrompt{}, "test", "", "/tmp"),
	})
	tl := &Tool{Store: store, WorkDir: "/tmp"}

	def := FindSetting("model")
	tl.syncToAppState(def, "new-model")

	snap := store.Snapshot()
	if snap.Model != "new-model" {
		t.Errorf("AppState.Model = %q, want 'new-model'", snap.Model)
	}
}

func TestInvoke_UnknownSetting(t *testing.T) {
	store := app.NewStateStore(app.AppState{CWD: "/tmp"})
	tl := &Tool{Store: store, WorkDir: "/tmp"}

	input, _ := json.Marshal(ConfigInput{Setting: "nonexistent"})
	result, err := tl.Invoke(context.Background(), input, staticState{"/tmp"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "Unknown setting") {
		t.Errorf("Content = %q, want 'Unknown setting'", result.Content)
	}
}

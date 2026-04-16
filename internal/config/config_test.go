package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMerge(t *testing.T) {
	temp := 0.7
	temp2 := 0.9

	tests := []struct {
		name    string
		base    Config
		overlay Config
		check   func(t *testing.T, got Config)
	}{
		{
			name:    "empty overlay preserves base",
			base:    Config{Model: "base-model", MaxTokens: 1000},
			overlay: Config{},
			check: func(t *testing.T, got Config) {
				if got.Model != "base-model" {
					t.Errorf("Model = %q, want %q", got.Model, "base-model")
				}
				if got.MaxTokens != 1000 {
					t.Errorf("MaxTokens = %d, want %d", got.MaxTokens, 1000)
				}
			},
		},
		{
			name:    "overlay string overrides base",
			base:    Config{Model: "base-model"},
			overlay: Config{Model: "overlay-model"},
			check: func(t *testing.T, got Config) {
				if got.Model != "overlay-model" {
					t.Errorf("Model = %q, want %q", got.Model, "overlay-model")
				}
			},
		},
		{
			name:    "overlay int overrides base",
			base:    Config{MaxTokens: 1000},
			overlay: Config{MaxTokens: 2000},
			check: func(t *testing.T, got Config) {
				if got.MaxTokens != 2000 {
					t.Errorf("MaxTokens = %d, want %d", got.MaxTokens, 2000)
				}
			},
		},
		{
			name:    "overlay nil pointer preserves base pointer",
			base:    Config{Temperature: &temp},
			overlay: Config{},
			check: func(t *testing.T, got Config) {
				if got.Temperature == nil || *got.Temperature != 0.7 {
					t.Errorf("Temperature = %v, want 0.7", got.Temperature)
				}
			},
		},
		{
			name:    "overlay non-nil pointer overrides base",
			base:    Config{Temperature: &temp},
			overlay: Config{Temperature: &temp2},
			check: func(t *testing.T, got Config) {
				if got.Temperature == nil || *got.Temperature != 0.9 {
					t.Errorf("Temperature = %v, want 0.9", got.Temperature)
				}
			},
		},
		{
			name:    "overlay thinking overrides base",
			base:    Config{},
			overlay: Config{Thinking: &ThinkingConfig{Enabled: true, BudgetTokens: 5000}},
			check: func(t *testing.T, got Config) {
				if got.Thinking == nil || !got.Thinking.Enabled || got.Thinking.BudgetTokens != 5000 {
					t.Errorf("Thinking = %+v, want enabled with 5000 budget", got.Thinking)
				}
			},
		},
		{
			name:    "overlay bool true overrides base false",
			base:    Config{Verbose: false},
			overlay: Config{Verbose: true},
			check: func(t *testing.T, got Config) {
				if !got.Verbose {
					t.Error("Verbose = false, want true")
				}
			},
		},
		{
			name:    "multiple fields mixed",
			base:    Config{Model: "base", Provider: "anthropic", MaxTokens: 1000},
			overlay: Config{Model: "overlay", MaxTokens: 2000},
			check: func(t *testing.T, got Config) {
				if got.Model != "overlay" {
					t.Errorf("Model = %q, want %q", got.Model, "overlay")
				}
				if got.Provider != "anthropic" {
					t.Errorf("Provider = %q, want %q", got.Provider, "anthropic")
				}
				if got.MaxTokens != 2000 {
					t.Errorf("MaxTokens = %d, want %d", got.MaxTokens, 2000)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := merge(tt.base, tt.overlay)
			tt.check(t, got)
		})
	}
}

func TestLoad_NoFiles(t *testing.T) {
	dir := t.TempDir()
	// Override global path for test by using Load with a workDir that has no .pragma/
	cfg, err := loadWithGlobal(filepath.Join(dir, "nonexistent-global.json"), dir)
	if err != nil {
		t.Fatalf("Load with no files: %v", err)
	}
	if cfg.Model != "" || cfg.Provider != "" || cfg.MaxTokens != 0 {
		t.Errorf("expected zero Config, got %+v", cfg)
	}
}

func TestLoad_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global", ".pragma")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalPath := filepath.Join(globalDir, "settings.json")
	writeJSON(t, globalPath, Config{Model: "global-model", MaxTokens: 4096})

	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadWithGlobal(globalPath, projectDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "global-model" {
		t.Errorf("Model = %q, want %q", cfg.Model, "global-model")
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want %d", cfg.MaxTokens, 4096)
	}
}

func TestLoad_ProjectOverride(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global", ".pragma")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	globalPath := filepath.Join(globalDir, "settings.json")
	writeJSON(t, globalPath, Config{Model: "global-model", MaxTokens: 4096, Provider: "anthropic"})

	projectDir := filepath.Join(dir, "project")
	projectClaudeDir := filepath.Join(projectDir, ".pragma")
	if err := os.MkdirAll(projectClaudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(projectClaudeDir, "settings.json"), Config{Model: "project-model", MaxTokens: 8192})

	cfg, err := loadWithGlobal(globalPath, projectDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "project-model" {
		t.Errorf("Model = %q, want %q", cfg.Model, "project-model")
	}
	if cfg.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want %d", cfg.MaxTokens, 8192)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q (preserved from global)", cfg.Provider, "anthropic")
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(gogentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gogentDir, "settings.json"), []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use the malformed file as the global config
	_, err := loadWithGlobal(filepath.Join(gogentDir, "settings.json"), t.TempDir())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	temp := 0.5
	original := Config{
		Model:        "test-model",
		Provider:     "anthropic",
		MaxTokens:    8192,
		Temperature:  &temp,
		Thinking:     &ThinkingConfig{Enabled: true, BudgetTokens: 10000},
		SystemPrompt: "You are helpful.",
		Verbose:      true,
		Record:       true,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var restored Config
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if restored.Model != original.Model {
		t.Errorf("Model = %q, want %q", restored.Model, original.Model)
	}
	if restored.Provider != original.Provider {
		t.Errorf("Provider = %q, want %q", restored.Provider, original.Provider)
	}
	if restored.MaxTokens != original.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d", restored.MaxTokens, original.MaxTokens)
	}
	if restored.Temperature == nil || *restored.Temperature != *original.Temperature {
		t.Errorf("Temperature = %v, want %v", restored.Temperature, *original.Temperature)
	}
	if restored.Thinking == nil || restored.Thinking.Enabled != original.Thinking.Enabled || restored.Thinking.BudgetTokens != original.Thinking.BudgetTokens {
		t.Errorf("Thinking = %+v, want %+v", restored.Thinking, original.Thinking)
	}
	if restored.SystemPrompt != original.SystemPrompt {
		t.Errorf("SystemPrompt = %q, want %q", restored.SystemPrompt, original.SystemPrompt)
	}
	if restored.Verbose != original.Verbose {
		t.Errorf("Verbose = %v, want %v", restored.Verbose, original.Verbose)
	}
	if restored.Record != original.Record {
		t.Errorf("Record = %v, want %v", restored.Record, original.Record)
	}
}

func TestLoad_RealFunction(t *testing.T) {
	// Use $HOME override to test the real Load function
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Create global config
	globalDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(globalDir, "settings.json"), Config{Model: "from-global", Verbose: true})

	// Create project config in a subdirectory
	projDir := filepath.Join(dir, "myproject")
	projGogentDir := filepath.Join(projDir, ".pragma")
	if err := os.MkdirAll(projGogentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(projGogentDir, "settings.json"), Config{Provider: "anthropic", Record: true})

	cfg, err := Load(projDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model != "from-global" {
		t.Errorf("Model = %q, want from-global", cfg.Model)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", cfg.Provider)
	}
	if !cfg.Verbose {
		t.Error("Verbose = false, want true (from global)")
	}
	if !cfg.Record {
		t.Error("Record = false, want true (from project)")
	}
}

func TestGlobalSettingsPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	path, err := GlobalSettingsPath()
	if err != nil {
		t.Fatalf("GlobalSettingsPath: %v", err)
	}
	expected := filepath.Join(dir, ".pragma", "settings.json")
	if path != expected {
		t.Errorf("GlobalSettingsPath = %q, want %q", path, expected)
	}
}

func TestMerge_AllFields(t *testing.T) {
	// Test that ALL fields in Config can be overridden
	temp := 0.5
	base := Config{}
	overlay := Config{
		Model:        "m",
		Provider:     "p",
		APIKey:       "k",
		MaxTokens:    100,
		Temperature:  &temp,
		Thinking:     &ThinkingConfig{Enabled: true, BudgetTokens: 5000},
		SystemPrompt: "sys",
		Verbose:      true,
		Record:       true,
	}

	got := merge(base, overlay)
	if got.Model != "m" {
		t.Errorf("Model = %q", got.Model)
	}
	if got.Provider != "p" {
		t.Errorf("Provider = %q", got.Provider)
	}
	if got.APIKey != "k" {
		t.Errorf("APIKey = %q", got.APIKey)
	}
	if got.MaxTokens != 100 {
		t.Errorf("MaxTokens = %d", got.MaxTokens)
	}
	if got.Temperature == nil || *got.Temperature != 0.5 {
		t.Errorf("Temperature = %v", got.Temperature)
	}
	if got.Thinking == nil || !got.Thinking.Enabled {
		t.Errorf("Thinking = %v", got.Thinking)
	}
	if got.SystemPrompt != "sys" {
		t.Errorf("SystemPrompt = %q", got.SystemPrompt)
	}
	if !got.Verbose {
		t.Error("Verbose = false")
	}
	if !got.Record {
		t.Error("Record = false")
	}
}

func TestReadFile_PermissionError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.json")
	if err := os.WriteFile(path, []byte(`{"model":"test"}`), 0o000); err != nil {
		t.Fatal(err)
	}

	_, err := readFile(path)
	if err == nil {
		t.Fatal("expected permission error, got nil")
	}
}

// loadWithGlobal is a test helper that loads config with an explicit global path
// instead of using the real home directory.
func loadWithGlobal(globalPath, workDir string) (Config, error) {
	global, err := readFile(globalPath)
	if err != nil {
		return Config{}, err
	}
	project, err := readFile(ProjectSettingsPath(workDir))
	if err != nil {
		return Config{}, err
	}
	return merge(global, project), nil
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal test config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

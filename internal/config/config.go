package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all configuration from settings files and CLI flags.
// No internal package dependencies (DAG rule: config has no internal deps).
type Config struct {
	Model        string          `json:"model,omitempty"`
	Provider     string          `json:"provider,omitempty"`
	APIKey       string          `json:"api_key,omitempty"`
	MaxTokens    int             `json:"max_tokens,omitempty"`
	Temperature  *float64        `json:"temperature,omitempty"`
	Thinking     *ThinkingConfig `json:"thinking,omitempty"`
	SystemPrompt string          `json:"system_prompt,omitempty"`
	Verbose      bool            `json:"verbose,omitempty"`
	Record       bool            `json:"record,omitempty"`
}

// ThinkingConfig controls extended thinking / reasoning.
// Same shape as provider.ThinkingConfig but independent — config package
// cannot import provider (DAG). main.go field-copies at wiring time.
type ThinkingConfig struct {
	Enabled      bool `json:"enabled"`
	BudgetTokens int  `json:"budget_tokens,omitempty"`
}

// Load reads global (~/.pragma/settings.json) and project (<workDir>/.pragma/settings.json)
// config files and merges them. Project settings override global settings.
// CLI flag overrides are applied by the caller after Load returns.
func Load(workDir string) (Config, error) {
	globalPath, err := GlobalPath()
	if err != nil {
		return Config{}, fmt.Errorf("resolve global config path: %w", err)
	}

	global, err := readFile(globalPath)
	if err != nil {
		return Config{}, fmt.Errorf("read global config: %w", err)
	}

	projectPath := ProjectPath(workDir)
	project, err := readFile(projectPath)
	if err != nil {
		return Config{}, fmt.Errorf("read project config: %w", err)
	}

	return merge(global, project), nil
}

// GlobalPath returns ~/.pragma/settings.json.
func GlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".pragma", "settings.json"), nil
}

// ProjectPath returns <workDir>/.pragma/settings.json.
func ProjectPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "settings.json")
}

// readFile reads a single JSON config file. Returns zero Config if file doesn't exist.
func readFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// merge applies non-zero fields from overlay onto base.
// String: non-empty wins. Int: non-zero wins. Pointer: non-nil wins.
// Bool: true wins — project config cannot override global true→false.
func merge(base, overlay Config) Config {
	result := base

	if overlay.Model != "" {
		result.Model = overlay.Model
	}
	if overlay.Provider != "" {
		result.Provider = overlay.Provider
	}
	if overlay.APIKey != "" {
		result.APIKey = overlay.APIKey
	}
	if overlay.MaxTokens != 0 {
		result.MaxTokens = overlay.MaxTokens
	}
	if overlay.Temperature != nil {
		result.Temperature = overlay.Temperature
	}
	if overlay.Thinking != nil {
		result.Thinking = overlay.Thinking
	}
	if overlay.SystemPrompt != "" {
		result.SystemPrompt = overlay.SystemPrompt
	}
	if overlay.Verbose {
		result.Verbose = true
	}
	if overlay.Record {
		result.Record = true
	}

	return result
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// RawPermission is a single permission entry from a settings file.
type RawPermission struct {
	Behavior string `json:"behavior"`
	Rule     string `json:"rule"`
}

// Config holds all configuration from settings files and CLI flags.
// No internal package dependencies (DAG rule: config has no internal deps).
type Config struct {
	Model             string          `json:"model,omitempty"`
	Provider          string          `json:"provider,omitempty"`
	APIKey            string          `json:"api_key,omitempty"`
	VertexProjectID   string          `json:"vertex_project_id,omitempty"`
	VertexLocation    string          `json:"vertex_location,omitempty"`
	VertexEndpointID  string          `json:"vertex_endpoint_id,omitempty"`
	VertexDomain      string          `json:"vertex_domain,omitempty"`
	MaxTokens         int             `json:"max_tokens,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	Thinking          *ThinkingConfig `json:"thinking,omitempty"`
	SystemPrompt      string          `json:"system_prompt,omitempty"`
	MaxTurns          int             `json:"max_turns,omitempty"`
	ContextMode       string          `json:"context_mode,omitempty"`
	HandoffSchema     string          `json:"handoff_schema,omitempty"`
	StopAfterToolExec bool            `json:"stop_after_tool_exec,omitempty"`
	Verbose           bool            `json:"verbose,omitempty"`
	Record            bool            `json:"record,omitempty"`
	Permissions       []RawPermission `json:"permissions,omitempty"`
	PermissionMode    string          `json:"permission_mode,omitempty"`
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
	globalPath, err := GlobalSettingsPath()
	if err != nil {
		return Config{}, fmt.Errorf("resolve global config path: %w", err)
	}

	global, err := readFile(globalPath)
	if err != nil {
		return Config{}, fmt.Errorf("read global config: %w", err)
	}

	projectPath := ProjectSettingsPath(workDir)
	project, err := readFile(projectPath)
	if err != nil {
		return Config{}, fmt.Errorf("read project config: %w", err)
	}

	return merge(global, project), nil
}

// PermissionWithSource is a permission entry tagged with its config source.
type PermissionWithSource struct {
	Behavior string
	Rule     string
	Source   string // "userSettings", "projectSettings", "localSettings"
}

// LoadPermissions reads permission entries from all config scopes and returns them
// ordered by priority (user first, local last). Also returns the permission mode.
func LoadPermissions(workDir string) ([]PermissionWithSource, string, error) {
	globalPath, err := GlobalSettingsPath()
	if err != nil {
		return nil, "", fmt.Errorf("resolve global config path: %w", err)
	}

	type scopeInfo struct {
		path   string
		source string
	}
	scopes := []scopeInfo{
		{globalPath, "userSettings"},
		{ProjectSettingsPath(workDir), "projectSettings"},
		{LocalSettingsPath(workDir), "localSettings"},
	}

	var result []PermissionWithSource
	var mode string

	for _, scope := range scopes {
		cfg, err := readFile(scope.path)
		if err != nil {
			continue // skip unreadable files
		}
		for _, perm := range cfg.Permissions {
			result = append(result, PermissionWithSource{
				Behavior: perm.Behavior,
				Rule:     perm.Rule,
				Source:   scope.source,
			})
		}
		// First non-empty mode wins (user > project > local)
		if mode == "" && cfg.PermissionMode != "" {
			mode = cfg.PermissionMode
		}
	}
	return result, mode, nil
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
	if overlay.MaxTurns != 0 {
		result.MaxTurns = overlay.MaxTurns
	}
	if overlay.ContextMode != "" {
		result.ContextMode = overlay.ContextMode
	}
	if overlay.HandoffSchema != "" {
		result.HandoffSchema = overlay.HandoffSchema
	}
	if overlay.StopAfterToolExec {
		result.StopAfterToolExec = true
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
	// Permissions concatenate (not override) — accumulate from all scopes, deduplicated.
	// GitHub #6991, #6874: identical rules from multiple scopes cause duplicate checks.
	result.Permissions = deduplicatePermissions(append(result.Permissions, overlay.Permissions...))
	if overlay.PermissionMode != "" {
		result.PermissionMode = overlay.PermissionMode
	}

	return result
}

// deduplicatePermissions removes duplicate permission entries by (Behavior, Rule) tuple.
// First occurrence wins (preserves priority order from global → project → local).
func deduplicatePermissions(perms []RawPermission) []RawPermission {
	type key struct{ behavior, rule string }
	seen := make(map[key]bool, len(perms))
	result := make([]RawPermission, 0, len(perms))
	for _, p := range perms {
		k := key{p.Behavior, p.Rule}
		if seen[k] {
			continue
		}
		seen[k] = true
		result = append(result, p)
	}
	return result
}

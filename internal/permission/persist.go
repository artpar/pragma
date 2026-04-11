package permission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/gogent/internal/config"
)

// PersistRule writes a permission rule to settings.local.json using read-modify-write.
// Avoids TS bug #9814 (overwriting existing rules) by reading first and appending.
// Uses atomic write (temp file + rename) matching session.Store pattern.
func PersistRule(workDir string, rule Rule) error {
	path := config.LocalSettingsPath(workDir)

	// Read existing settings
	existing, err := readLocalSettings(path)
	if err != nil {
		return fmt.Errorf("read settings.local.json: %w", err)
	}

	// Serialize the rule
	ruleStr := RuleToString(rule)
	behavior := string(rule.Decision)

	// Check for duplicate
	for _, p := range existing.Permissions {
		if p.Behavior == behavior && p.Rule == ruleStr {
			return nil // already exists, no-op
		}
	}

	// Append new rule
	existing.Permissions = append(existing.Permissions, config.RawPermission{
		Behavior: behavior,
		Rule:     ruleStr,
	})

	// Write atomically
	return writeLocalSettings(path, existing)
}

// LoadPersistedRules reads permission rules from settings.local.json.
// This is called at startup to merge persisted rules with config-loaded rules.
// Rules from this function have Source = SourceLocal.
func LoadPersistedRules(workDir string) ([]Rule, error) {
	path := config.LocalSettingsPath(workDir)
	settings, err := readLocalSettings(path)
	if err != nil {
		return nil, err
	}

	var rules []Rule
	for _, p := range settings.Permissions {
		decision := parseDecision(p.Behavior)
		if decision == "" {
			continue
		}
		rule := ParseRuleString(p.Rule, decision, SourceLocal)
		rules = append(rules, rule)
	}
	return rules, nil
}

// RuleToString serializes a Rule to "ToolName" or "ToolName(content)" format.
// This is the inverse of ParseRuleString.
func RuleToString(rule Rule) string {
	if rule.Content == "" {
		return rule.ToolName
	}
	return rule.ToolName + "(" + rule.Content + ")"
}

// readLocalSettings reads settings.local.json, returning empty Config if file doesn't exist.
func readLocalSettings(path string) (config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config.Config{}, nil
		}
		return config.Config{}, err
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return config.Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// writeLocalSettings writes settings to a file atomically (temp + rename).
func writeLocalSettings(path string, cfg config.Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp settings: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename settings: %w", err)
	}

	return nil
}

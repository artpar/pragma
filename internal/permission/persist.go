package permission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// PersistRule writes a permission rule to settings.local.json using read-modify-write.
// Avoids TS bug #9814 (overwriting existing rules) by reading first and appending.
// Uses atomic write (temp file + rename) matching session.Store pattern.
func PersistRule(workDir string, rule Rule) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	path := config.LocalSettingsPath(workDir)

	existing, err := readLocalSettings(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"read settings.local.json: %w\", err)")
		return fmt.Errorf("read settings.local.json: %w", err)
	}

	ruleStr := RuleToString(rule)
	behavior := string(rule.Decision)

	for _, p := range existing.Permissions {
		observe.GlobalTrace("range existing.Permissions")
		if p.Behavior == behavior && p.Rule == ruleStr {
			observe.GlobalTrace("if: p.Behavior == behavior && p.Rule == ruleStr")
			observe.GlobalTrace("return: nil")
			return nil
		}
	}

	existing.Permissions = append(existing.Permissions, config.RawPermission{
		Behavior: behavior,
		Rule:     ruleStr,
	})
	observe.GlobalTrace("return: writeLocalSettings(path, existing)")

	return writeLocalSettings(path, existing)
}

// LoadPersistedRules reads permission rules from settings.local.json.
// This is called at startup to merge persisted rules with config-loaded rules.
// Rules from this function have Source = SourceLocal.
func LoadPersistedRules(workDir string) ([]Rule, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	path := config.LocalSettingsPath(workDir)
	settings, err := readLocalSettings(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	var rules []Rule
	for _, p := range settings.Permissions {
		observe.GlobalTrace("range settings.Permissions")
		decision := parseDecision(p.Behavior)
		if decision == "" {
			observe.GlobalTrace("if: decision == \"\"")
			continue
		}
		rule := ParseRuleString(p.Rule, decision, SourceLocal)
		rules = append(rules, rule)
	}
	observe.GlobalTrace("return: rules, nil")
	return rules, nil
}

// RuleToString serializes a Rule to "ToolName" or "ToolName(content)" format.
// This is the inverse of ParseRuleString.
func RuleToString(rule Rule) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if rule.Content == "" {
		observe.GlobalTrace("if: rule.Content == \"\"")
		observe.GlobalTrace("return: rule.ToolName")
		return rule.ToolName
	}
	observe.GlobalTrace("return: rule.ToolName + \"(\" + rule.Content + \")\"")
	return rule.ToolName + "(" + rule.Content + ")"
}

// readLocalSettings reads settings.local.json, returning empty Config if file doesn't exist.
func readLocalSettings(path string) (config.Config, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if errors.Is(err, os.ErrNotExist) {
			observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
			observe.GlobalTrace("return: config.Config{}, nil")
			return config.Config{}, nil
		}
		observe.GlobalTrace("return: config.Config{}, err")
		return config.Config{}, err
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: config.Config{}, fmt.Errorf(\"parse %s: %w\", path, err)")
		return config.Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	observe.GlobalTrace("return: cfg, nil")
	return cfg, nil
}

// writeLocalSettings writes settings to a file atomically (temp + rename).
func writeLocalSettings(path string, cfg config.Config) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"create settings directory: %w\", err)")
		return fmt.Errorf("create settings directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"marshal settings: %w\", err)")
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"write temp settings: %w\", err)")
		return fmt.Errorf("write temp settings: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		observe.GlobalTrace("if: err != nil")
		os.Remove(tmpPath)
		observe.GlobalTrace("return: fmt.Errorf(\"rename settings: %w\", err)")
		return fmt.Errorf("rename settings: %w", err)
	}
	observe.GlobalTrace("return: nil")

	return nil
}

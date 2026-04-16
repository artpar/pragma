package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/app"
	goconfig "github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

// ConfigInput defines the parameters for the Config tool.
type ConfigInput struct {
	Setting string `json:"setting" desc:"The setting name (e.g., 'model', 'verbose', 'theme')"`
	Value   *any   `json:"value,omitempty" desc:"Value to set. Omit to read current value."`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["setting"],
	"properties": {
		"setting": {
			"type": "string",
			"description": "The setting name (e.g., 'model', 'verbose', 'theme', 'permissions.defaultMode')"
		},
		"value": {
			"description": "Value to set. Omit to read current value."
		}
	}
}`)

// Tool implements the Config tool for getting/setting configuration.
type Tool struct {
	Store   *app.StateStore
	WorkDir string
}

func (t *Tool) Name() string { return "Config" }

func (t *Tool) Description() string {
	return generateDescription()
}

func (t *Tool) InputSchema() json.RawMessage { return inputSchema }

func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "config", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "config", "Tool.CheckPerm", "exit")

	var in ConfigInput
	if err := json.Unmarshal(input, &in); err != nil {
		return checker.Check(ctx, "Config", "")
	}

	// GET operations are always safe
	if in.Value == nil {
		return permission.CheckResult{Decision: permission.DecisionAllow}
	}

	content := fmt.Sprintf("set:%s=%v", in.Setting, *in.Value)
	return checker.Check(ctx, "Config", content)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "config", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "config", "Tool.Invoke", "exit")

	var in ConfigInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Invalid input: %v", err)}, nil
	}

	in.Setting = strings.TrimSpace(in.Setting)
	if in.Setting == "" {
		return tool.InvokeResult{Content: "Setting name is required."}, nil
	}

	def := FindSetting(in.Setting)
	if def == nil {
		return tool.InvokeResult{
			Content: fmt.Sprintf("Unknown setting: %q. Available settings: %s", in.Setting, strings.Join(SettingNames(), ", ")),
		}, nil
	}

	if in.Value == nil {
		return t.handleGet(ctx, def)
	}
	return t.handleSet(ctx, def, *in.Value)
}

// handleGet reads a setting value from the merged config.
func (t *Tool) handleGet(ctx context.Context, def *SettingDef) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "config", "Tool.handleGet", "enter")
	defer observe.TraceCtx(ctx, "config", "Tool.handleGet", "exit")

	raw, err := readSettingFromFile(def, t.WorkDir)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Error reading %s: %v", def.Name, err)}, nil
	}

	valueJSON, _ := json.Marshal(raw)
	return tool.InvokeResult{Content: fmt.Sprintf("%s = %s", def.Name, string(valueJSON))}, nil
}

// handleSet validates and writes a setting value.
func (t *Tool) handleSet(ctx context.Context, def *SettingDef, value any) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "config", "Tool.handleSet", "enter")
	defer observe.TraceCtx(ctx, "config", "Tool.handleSet", "exit")

	// Type coercion: string "true"/"false" → bool
	if def.Type == "boolean" {
		value = coerceBool(value)
		if _, ok := value.(bool); !ok {
			return tool.InvokeResult{Content: fmt.Sprintf("%s requires true or false.", def.Name)}, nil
		}
	}

	// Validate against enum options
	if def.Options != nil {
		strVal := fmt.Sprintf("%v", value)
		valid := false
		for _, opt := range def.Options {
			if strings.EqualFold(strVal, opt) {
				value = opt // normalize casing
				valid = true
				break
			}
		}
		if !valid {
			return tool.InvokeResult{
				Content: fmt.Sprintf("Invalid value %q for %s. Options: %s", strVal, def.Name, strings.Join(def.Options, ", ")),
			}, nil
		}
	}

	// Determine target file
	var targetPath string
	if def.Source == "global" {
		p, err := goconfig.GlobalSettingsPath()
		if err != nil {
			return tool.InvokeResult{Content: fmt.Sprintf("Error: %v", err)}, nil
		}
		targetPath = p
	} else {
		targetPath = goconfig.ProjectSettingsPath(t.WorkDir)
	}

	// Atomic read-modify-write
	if err := updateSettingsFile(targetPath, def.Name, value); err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Error writing %s: %v", def.Name, err)}, nil
	}

	// Sync to AppState if applicable
	t.syncToAppState(def, value)

	valueJSON, _ := json.Marshal(value)
	return tool.InvokeResult{Content: fmt.Sprintf("Set %s to %s", def.Name, string(valueJSON))}, nil
}

// syncToAppState updates the live AppState for settings that have immediate effect.
func (t *Tool) syncToAppState(def *SettingDef, value any) {
	if def.AppStateKey == "" || t.Store == nil {
		return
	}
	t.Store.Update(func(s *app.AppState) {
		switch def.AppStateKey {
		case "Model":
			if v, ok := value.(string); ok {
				s.Model = v
			}
		}
	})
}

// coerceBool converts string "true"/"false" to bool. Returns value unchanged if not coercible.
func coerceBool(value any) any {
	if s, ok := value.(string); ok {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "true":
			return true
		case "false":
			return false
		}
	}
	return value
}

// readSettingFromFile reads a single setting value from the appropriate config file.
func readSettingFromFile(def *SettingDef, workDir string) (any, error) {
	var targetPath string
	if def.Source == "global" {
		p, err := goconfig.GlobalSettingsPath()
		if err != nil {
			return nil, err
		}
		targetPath = p
	} else {
		targetPath = goconfig.ProjectSettingsPath(workDir)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return getNestedValue(raw, def.Name), nil
}

// updateSettingsFile performs an atomic read-modify-write on a JSON settings file.
// Preserves all existing keys — only updates the specified key.
func updateSettingsFile(path, key string, value any) error {
	// Read existing file
	var raw map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		raw = make(map[string]any)
	} else {
		raw = make(map[string]any)
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("parse existing settings: %w", err)
		}
	}

	// Set the value (handle nested paths like "permissions.defaultMode")
	setNestedValue(raw, key, value)

	// Marshal with indentation
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	out = append(out, '\n')

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Atomic write: temp file + rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// getNestedValue reads a value from a nested map using a dotted key path.
func getNestedValue(m map[string]any, key string) any {
	parts := strings.Split(key, ".")
	current := any(m)
	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = cm[part]
	}
	return current
}

// setNestedValue sets a value in a nested map using a dotted key path.
func setNestedValue(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
}

// generateDescription builds the tool description with current settings list.
func generateDescription() string {
	var b strings.Builder
	b.WriteString("Get or set configuration settings.\n\n")
	b.WriteString("View or change settings. Use when the user requests configuration changes or asks about current settings.\n\n")
	b.WriteString("## Usage\n")
	b.WriteString("- **Get current value:** Omit the \"value\" parameter\n")
	b.WriteString("- **Set new value:** Include the \"value\" parameter\n\n")
	b.WriteString("## Available settings\n\n")

	for _, s := range SupportedSettings {
		line := fmt.Sprintf("- **%s**", s.Name)
		if s.Options != nil {
			line += fmt.Sprintf(": %s", strings.Join(s.Options, ", "))
		} else if s.Type == "boolean" {
			line += ": true/false"
		}
		line += " — " + s.Description + "\n"
		b.WriteString(line)
	}

	b.WriteString("\n## Examples\n")
	b.WriteString("- Get model: `{\"setting\": \"model\"}`\n")
	b.WriteString("- Set model: `{\"setting\": \"model\", \"value\": \"claude-opus-4-6\"}`\n")
	b.WriteString("- Set theme: `{\"setting\": \"theme\", \"value\": \"dark\"}`\n")

	return b.String()
}

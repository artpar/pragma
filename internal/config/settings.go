package config

import "sort"

// SettingDef describes a runtime-supported configuration setting.
type SettingDef struct {
	Name        string
	Type        string
	Source      string
	Options     []string
	Description string
	AppStateKey string
}

// SupportedSettings lists settings backed by Config fields and config.Load.
var SupportedSettings = []SettingDef{
	{
		Name:        "model",
		Type:        "string",
		Source:      "project",
		Description: "Override the default LLM model",
		AppStateKey: "Model",
	},
	{
		Name:        "provider",
		Type:        "string",
		Source:      "project",
		Description: "Override the default LLM provider",
	},
	{
		Name:        "verbose",
		Type:        "boolean",
		Source:      "global",
		Description: "Show detailed debug output",
	},
	{
		Name:        "record",
		Type:        "boolean",
		Source:      "global",
		Description: "Record session events",
	},
	{
		Name:        "permission_mode",
		Type:        "string",
		Source:      "project",
		Options:     []string{"default", "acceptEdits", "bypassPermissions", "dontAsk"},
		Description: "Default permission mode",
	},
	{
		Name:        "context_mode",
		Type:        "string",
		Source:      "project",
		Options:     []string{"chat", "state-handoff"},
		Description: "Conversation context mode",
	},
	{
		Name:        "handoff_schema",
		Type:        "string",
		Source:      "project",
		Options:     []string{"v1"},
		Description: "State handoff schema version",
	},
	{
		Name:        "stop_after_tool_exec",
		Type:        "boolean",
		Source:      "project",
		Description: "Stop after executing tool calls",
	},
	{
		Name:        "toolset",
		Type:        "string",
		Source:      "project",
		Description: "Restrict exposed tools to a named toolset",
	},
}

// FindSetting looks up a setting definition by name. Returns nil if not found.
func FindSetting(name string) *SettingDef {
	for i := range SupportedSettings {
		if SupportedSettings[i].Name == name {
			return &SupportedSettings[i]
		}
	}
	return nil
}

// SettingNames returns all supported setting names in stable order.
func SettingNames() []string {
	names := make([]string, len(SupportedSettings))
	for i, s := range SupportedSettings {
		names[i] = s.Name
	}
	sort.Strings(names)
	return names
}

package config

// SettingDef describes a supported configuration setting.
type SettingDef struct {
	Name        string   // e.g., "model", "verbose", "permissions.defaultMode"
	Type        string   // "string" or "boolean"
	Source      string   // "global" or "project"
	Options     []string // valid enum values; nil = freeform
	Description string   // human-readable help text
	AppStateKey string   // field name in AppState to sync on write; empty = no sync
}

// SupportedSettings lists all settings the ConfigTool can read/write.
var SupportedSettings = []SettingDef{
	{
		Name:        "model",
		Type:        "string",
		Source:      "project",
		Description: "Override the default LLM model",
		AppStateKey: "Model",
	},
	{
		Name:        "verbose",
		Type:        "boolean",
		Source:      "global",
		Description: "Show detailed debug output",
	},
	{
		Name:        "autoCompactEnabled",
		Type:        "boolean",
		Source:      "global",
		Description: "Auto-compact conversation when context is large",
	},
	{
		Name:        "theme",
		Type:        "string",
		Source:      "global",
		Options:     []string{"dark", "light"},
		Description: "Color theme for the UI",
	},
	{
		Name:        "permissions.defaultMode",
		Type:        "string",
		Source:      "project",
		Options:     []string{"default", "plan", "acceptEdits", "dontAsk"},
		Description: "Default permission mode",
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

// SettingNames returns all supported setting names.
func SettingNames() []string {
	names := make([]string, len(SupportedSettings))
	for i, s := range SupportedSettings {
		names[i] = s.Name
	}
	return names
}

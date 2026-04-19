package slash

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/observe"
)

func handleConfig(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	var b strings.Builder
	b.WriteString("pragma config\n")
	b.WriteString(strings.Repeat("─", 40) + "\n\n")

	cwd := deps.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Effective model + provider
	b.WriteString("Model & Provider\n")
	fmt.Fprintf(&b, "  model:    %s\n", valueOrDefault(deps.ModelName, "(not set)"))
	fmt.Fprintf(&b, "  provider: %s\n", valueOrDefault(deps.Provider, "(auto-detect)"))
	b.WriteString("\n")

	// Settings sources
	b.WriteString("Settings Sources\n")
	globalPath, _ := config.GlobalSettingsPath()
	projectPath := config.ProjectSettingsPath(cwd)
	localPath := config.LocalSettingsPath(cwd)

	writeSettingsSource(&b, "global", globalPath)
	writeSettingsSource(&b, "project", projectPath)
	writeSettingsSource(&b, "local", localPath)
	b.WriteString("\n")

	// Merged config values
	cfg, err := config.Load(cwd)
	if err != nil {
		fmt.Fprintf(&b, "  (config load error: %s)\n\n", err)
	} else {
		b.WriteString("Effective Config\n")
		if cfg.MaxTokens != 0 {
			fmt.Fprintf(&b, "  max_tokens:   %d\n", cfg.MaxTokens)
		}
		if cfg.MaxTurns != 0 {
			fmt.Fprintf(&b, "  max_turns:    %d\n", cfg.MaxTurns)
		}
		if cfg.Temperature != nil {
			fmt.Fprintf(&b, "  temperature:  %.1f\n", *cfg.Temperature)
		}
		if cfg.Thinking != nil {
			fmt.Fprintf(&b, "  thinking:     enabled=%t budget=%d\n", cfg.Thinking.Enabled, cfg.Thinking.BudgetTokens)
		}
		if cfg.PermissionMode != "" {
			fmt.Fprintf(&b, "  permissions:  %s\n", cfg.PermissionMode)
		}
		if cfg.Verbose {
			fmt.Fprintf(&b, "  verbose:      true\n")
		}
		if cfg.Record {
			fmt.Fprintf(&b, "  record:       true\n")
		}
		b.WriteString("\n")
	}

	// Credentials
	b.WriteString("Credentials\n")
	creds, credErr := config.LoadCredentials()
	if credErr != nil {
		fmt.Fprintf(&b, "  (error: %s)\n", credErr)
	} else if len(creds.Providers) == 0 {
		b.WriteString("  (none configured in credentials.yml)\n")
	} else {
		for name, pc := range creds.Providers {
			masked := maskKey(pc.APIKey)
			line := fmt.Sprintf("  %-12s %s", name+":", masked)
			if pc.BaseURL != "" {
				line += fmt.Sprintf("  base_url=%s", pc.BaseURL)
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")

	// Environment variables
	b.WriteString("Environment Variables\n")
	envVars := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "GROQ_API_KEY", "LILAC_API_KEY"}
	for _, env := range envVars {
		val := os.Getenv(env)
		if val != "" {
			fmt.Fprintf(&b, "  %-20s set (%s)\n", env, maskKey(val))
		} else {
			fmt.Fprintf(&b, "  %-20s not set\n", env)
		}
	}
	b.WriteString("\n")

	// Permissions
	perms, permMode, permErr := config.LoadPermissions(cwd)
	if permErr == nil && len(perms) > 0 {
		b.WriteString("Permission Rules\n")
		if permMode != "" {
			fmt.Fprintf(&b, "  mode: %s\n", permMode)
		}
		for _, p := range perms {
			fmt.Fprintf(&b, "  [%s] %-8s %s\n", sourceLabel(p.Source), p.Behavior, p.Rule)
		}
		b.WriteString("\n")
	}

	// MCP servers
	b.WriteString("MCP Servers\n")
	globalMCPPath, _ := config.GlobalMCPConfigPath()
	projectMCPPath := config.MCPConfigPath(cwd)
	localMCPPath := config.MCPLocalConfigPath(cwd)

	writeMCPSource(&b, "global", globalMCPPath)
	writeMCPSource(&b, "project", projectMCPPath)
	writeMCPSource(&b, "local", localMCPPath)

	return Result{DisplayText: b.String()}, nil
}

// writeSettingsSource shows whether a settings file exists and its key overrides.
func writeSettingsSource(b *strings.Builder, label, path string) {
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(b, "  %-8s (not found) %s\n", label+":", path)
		return
	}
	fmt.Fprintf(b, "  %-8s %s\n", label+":", path)
}

// writeMCPSource counts MCP servers in a config file.
func writeMCPSource(b *strings.Builder, label, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(b, "  %-8s (none)\n", label+":")
		return
	}
	var raw struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		fmt.Fprintf(b, "  %-8s (parse error)\n", label+":")
		return
	}
	count := len(raw.MCPServers)
	if count == 0 {
		fmt.Fprintf(b, "  %-8s (none)\n", label+":")
		return
	}
	names := make([]string, 0, count)
	for name := range raw.MCPServers {
		names = append(names, name)
	}
	fmt.Fprintf(b, "  %-8s %d servers (%s)\n", label+":", count, strings.Join(names, ", "))
}

// maskKey masks an API key showing first 4 and last 4 characters.
func maskKey(key string) string {
	if key == "" {
		return "(empty)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// valueOrDefault returns val if non-empty, else fallback.
func valueOrDefault(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// sourceLabel shortens source names for display.
func sourceLabel(source string) string {
	switch source {
	case "userSettings":
		return "global"
	case "projectSettings":
		return "project"
	case "localSettings":
		return "local"
	default:
		return source
	}
}

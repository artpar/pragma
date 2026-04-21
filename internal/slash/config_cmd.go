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
		observe.GlobalTrace("if: cwd == \"\"")
		cwd, _ = os.Getwd()
	}

	b.WriteString("Model & Provider\n")
	fmt.Fprintf(&b, "  model:    %s\n", valueOrDefault(deps.ModelName, "(not set)"))
	fmt.Fprintf(&b, "  provider: %s\n", valueOrDefault(deps.Provider, "(auto-detect)"))
	b.WriteString("\n")

	b.WriteString("Settings Sources\n")
	globalPath, _ := config.GlobalSettingsPath()
	projectPath := config.ProjectSettingsPath(cwd)
	localPath := config.LocalSettingsPath(cwd)

	writeSettingsSource(&b, "global", globalPath)
	writeSettingsSource(&b, "project", projectPath)
	writeSettingsSource(&b, "local", localPath)
	b.WriteString("\n")

	cfg, err := config.Load(cwd)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		fmt.Fprintf(&b, "  (config load error: %s)\n\n", err)
	} else {
		observe.GlobalTrace("else: err != nil")
		b.WriteString("Effective Config\n")
		if cfg.MaxTokens != 0 {
			observe.GlobalTrace("if: cfg.MaxTokens != 0")
			fmt.Fprintf(&b, "  max_tokens:   %d\n", cfg.MaxTokens)
		}
		if cfg.MaxTurns != 0 {
			observe.GlobalTrace("if: cfg.MaxTurns != 0")
			fmt.Fprintf(&b, "  max_turns:    %d\n", cfg.MaxTurns)
		}
		if cfg.Temperature != nil {
			observe.GlobalTrace("if: cfg.Temperature != nil")
			fmt.Fprintf(&b, "  temperature:  %.1f\n", *cfg.Temperature)
		}
		if cfg.Thinking != nil {
			observe.GlobalTrace("if: cfg.Thinking != nil")
			fmt.Fprintf(&b, "  thinking:     enabled=%t budget=%d\n", cfg.Thinking.Enabled, cfg.Thinking.BudgetTokens)
		}
		if cfg.PermissionMode != "" {
			observe.GlobalTrace("if: cfg.PermissionMode != \"\"")
			fmt.Fprintf(&b, "  permissions:  %s\n", cfg.PermissionMode)
		}
		if cfg.Verbose {
			observe.GlobalTrace("if: cfg.Verbose")
			fmt.Fprintf(&b, "  verbose:      true\n")
		}
		if cfg.Record {
			observe.GlobalTrace("if: cfg.Record")
			fmt.Fprintf(&b, "  record:       true\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("Credentials\n")
	creds, credErr := config.LoadCredentials()
	if credErr != nil {
		observe.GlobalTrace("if: credErr != nil")
		fmt.Fprintf(&b, "  (error: %s)\n", credErr)
	} else if len(creds.Providers) == 0 {
		observe.GlobalTrace("else-if: len(creds.Providers) == 0")
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

	b.WriteString("Environment Variables\n")
	envVars := []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "GROQ_API_KEY", "LILAC_API_KEY"}
	for _, env := range envVars {
		observe.GlobalTrace("range envVars")
		val := os.Getenv(env)
		if val != "" {
			observe.GlobalTrace("if: val != \"\"")
			fmt.Fprintf(&b, "  %-20s set (%s)\n", env, maskKey(val))
		} else {
			observe.GlobalTrace("else: val != \"\"")
			fmt.Fprintf(&b, "  %-20s not set\n", env)
		}
	}
	b.WriteString("\n")

	perms, permMode, permErr := config.LoadPermissions(cwd)
	if permErr == nil && len(perms) > 0 {
		observe.GlobalTrace("if: permErr == nil && len(perms) > 0")
		b.WriteString("Permission Rules\n")
		if permMode != "" {
			observe.GlobalTrace("if: permMode != \"\"")
			fmt.Fprintf(&b, "  mode: %s\n", permMode)
		}
		for _, p := range perms {
			observe.GlobalTrace("range perms")
			fmt.Fprintf(&b, "  [%s] %-8s %s\n", sourceLabel(p.Source), p.Behavior, p.Rule)
		}
		b.WriteString("\n")
	}

	b.WriteString("MCP Servers\n")
	globalMCPPath, _ := config.GlobalMCPConfigPath()
	projectMCPPath := config.MCPConfigPath(cwd)
	localMCPPath := config.MCPLocalConfigPath(cwd)

	writeMCPSource(&b, "global", globalMCPPath)
	writeMCPSource(&b, "project", projectMCPPath)
	writeMCPSource(&b, "local", localMCPPath)
	observe.GlobalTrace("return: Result{DisplayText: b.String()}, nil")

	return Result{DisplayText: b.String()}, nil
}

// writeSettingsSource shows whether a settings file exists and its key overrides.
func writeSettingsSource(b *strings.Builder, label, path string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if _, err := os.Stat(path); err != nil {
		observe.GlobalTrace("if: err != nil")
		fmt.Fprintf(b, "  %-8s (not found) %s\n", label+":", path)
		return
	}
	fmt.Fprintf(b, "  %-8s %s\n", label+":", path)
}

// writeMCPSource counts MCP servers in a config file.
func writeMCPSource(b *strings.Builder, label, path string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		fmt.Fprintf(b, "  %-8s (none)\n", label+":")
		return
	}
	var raw struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		observe.GlobalTrace("if: err != nil")
		fmt.Fprintf(b, "  %-8s (parse error)\n", label+":")
		return
	}
	count := len(raw.MCPServers)
	if count == 0 {
		observe.GlobalTrace("if: count == 0")
		fmt.Fprintf(b, "  %-8s (none)\n", label+":")
		return
	}
	names := make([]string, 0, count)
	for name := range raw.MCPServers {
		observe.GlobalTrace("range raw.MCPServers")
		names = append(names, name)
	}
	fmt.Fprintf(b, "  %-8s %d servers (%s)\n", label+":", count, strings.Join(names, ", "))
}

// maskKey masks an API key showing first 4 and last 4 characters.
func maskKey(key string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if key == "" {
		observe.GlobalTrace("if: key == \"\"")
		observe.GlobalTrace("return: \"(empty)\"")
		return "(empty)"
	}
	if len(key) <= 8 {
		observe.GlobalTrace("if: len(key) <= 8")
		observe.GlobalTrace("return: \"****\"")
		return "****"
	}
	observe.GlobalTrace("return: key[:4] + \"...\" + key[len(key)-4:]")
	return key[:4] + "..." + key[len(key)-4:]
}

// valueOrDefault returns val if non-empty, else fallback.
func valueOrDefault(val, fallback string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if val == "" {
		observe.GlobalTrace("if: val == \"\"")
		observe.GlobalTrace("return: fallback")
		return fallback
	}
	observe.GlobalTrace("return: val")
	return val
}

// sourceLabel shortens source names for display.
func sourceLabel(source string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch source {
	case "userSettings":
		observe.GlobalTrace("case: \"userSettings\"")
		return "global"
	case "projectSettings":
		observe.GlobalTrace("case: \"projectSettings\"")
		return "project"
	case "localSettings":
		observe.GlobalTrace("case: \"localSettings\"")
		return "local"
	default:
		observe.GlobalTrace("default")
		return source
	}
}

package slash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

func handleDoctor(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	var b strings.Builder
	b.WriteString("gogent doctor\n")
	b.WriteString(strings.Repeat("─", 40) + "\n\n")

	passes := 0
	warnings := 0
	failures := 0

	check := func(name string, ok bool, detail string) {
		if ok {
			passes++
			fmt.Fprintf(&b, "  [pass] %s\n", name)
		} else if detail != "" {
			warnings++
			fmt.Fprintf(&b, "  [warn] %s — %s\n", name, detail)
		} else {
			failures++
			fmt.Fprintf(&b, "  [FAIL] %s\n", name)
		}
	}

	providerName := deps.Provider
	if providerName == "" {
		observe.GlobalTrace("if: providerName == \"\"")
		providerName = deps.ModelName
	}
	check("Provider configured", providerName != "", "")

	hasKey := false
	for _, env := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "GROQ_API_KEY", "LILAC_API_KEY"} {
		observe.GlobalTrace("range env key check")
		if os.Getenv(env) != "" {
			observe.GlobalTrace("if: os.Getenv(env) != \"\"")
			hasKey = true
			break
		}
	}
	if !hasKey {
		creds, credErr := config.LoadCredentials()
		if credErr == nil {
			for _, pc := range creds.Providers {
				if pc.APIKey != "" {
					hasKey = true
					break
				}
			}
		}
	}
	check("API key found", hasKey, "set env var, --api-key, or add to ~/.gogent/credentials.yml")

	credPath, credPathErr := config.CredentialsPath()
	if credPathErr == nil {
		if _, err := os.Stat(credPath); err == nil {
			check("Credentials file", true, "")
		} else {
			check("Credentials file", false, "optional — ~/.gogent/credentials.yml not found")
		}
	}

	home, homeErr := config.GogentHome()
	if homeErr != nil {
		observe.GlobalTrace("if: homeErr != nil")
		check("Config directory (~/.gogent/)", false, homeErr.Error())
	} else {
		observe.GlobalTrace("else: homeErr != nil")
		_, err := os.Stat(home)
		check("Config directory (~/.gogent/)", err == nil, "run 'mkdir -p "+home+"'")
	}

	globalSettings, gsErr := config.GlobalSettingsPath()
	if gsErr != nil {
		observe.GlobalTrace("if: gsErr != nil")
		check("Global settings", false, gsErr.Error())
	} else {
		observe.GlobalTrace("else: gsErr != nil")
		_, err := os.Stat(globalSettings)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			check("Global settings", false, "optional — "+globalSettings+" not found")
		} else {
			observe.GlobalTrace("else: err != nil")
			check("Global settings", true, "")
		}
	}

	cwd := deps.Cwd
	if cwd == "" {
		observe.GlobalTrace("if: cwd == \"\"")
		cwd, _ = os.Getwd()
	}
	agentMD := filepath.Join(cwd, "AGENT.md")
	if _, err := os.Stat(agentMD); err != nil {
		observe.GlobalTrace("if: err != nil")
		check("AGENT.md", false, "run 'gogent init' to create")
	} else {
		observe.GlobalTrace("else: err != nil")
		check("AGENT.md", true, "")
	}

	gitDir := filepath.Join(cwd, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		observe.GlobalTrace("if: err == nil")
		check("Git repository", true, "")
	} else {
		observe.GlobalTrace("else: err == nil")
		check("Git repository", false, "not in a git repo")
	}

	_, cfgErr := config.Load(cwd)
	if cfgErr != nil {
		observe.GlobalTrace("if: cfgErr != nil")
		check("Config loads cleanly", false, cfgErr.Error())
	} else {
		observe.GlobalTrace("else: cfgErr != nil")
		check("Config loads cleanly", true, "")
	}

	mcpPath := filepath.Join(cwd, ".gogent", "mcp.json")
	if _, err := os.Stat(mcpPath); err == nil {
		observe.GlobalTrace("if: err == nil")
		check("MCP config exists", true, "")
	} else {
		observe.GlobalTrace("else: err == nil")
		check("MCP config", false, "no .gogent/mcp.json (optional)")
	}

	hookPaths := []string{
		filepath.Join(cwd, ".gogent", "settings.json"),
		filepath.Join(cwd, ".gogent", "settings.local.json"),
	}
	hookFound := false
	for _, hp := range hookPaths {
		observe.GlobalTrace("range hookPaths")
		if _, err := os.Stat(hp); err == nil {
			observe.GlobalTrace("if: err == nil")
			hookFound = true
			break
		}
	}
	if hookFound {
		observe.GlobalTrace("if: hookFound")
		check("Hook config", true, "")
	} else {
		observe.GlobalTrace("else: hookFound")
		check("Hook config", false, "no .gogent/settings.json (optional)")
	}

	fmt.Fprintf(&b, "\n%d passed, %d warnings, %d failures\n", passes, warnings, failures)
	observe.GlobalTrace("return: Result{DisplayText: b.String()}, nil")

	return Result{DisplayText: b.String()}, nil
}

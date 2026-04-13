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

	// 1. Provider configured
	providerName := deps.Provider
	if providerName == "" {
		providerName = deps.ModelName
	}
	check("Provider configured", providerName != "", "")

	// 2. API key present (check env vars for common providers)
	hasKey := false
	for _, env := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "GROQ_API_KEY"} {
		if os.Getenv(env) != "" {
			hasKey = true
			break
		}
	}
	check("API key found", hasKey, "set ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY, or GROQ_API_KEY")

	// 3. Global config directory
	home, homeErr := config.GogentHome()
	if homeErr != nil {
		check("Config directory (~/.gogent/)", false, homeErr.Error())
	} else {
		_, err := os.Stat(home)
		check("Config directory (~/.gogent/)", err == nil, "run 'mkdir -p "+home+"'")
	}

	// 4. Global settings file
	globalSettings, gsErr := config.GlobalSettingsPath()
	if gsErr != nil {
		check("Global settings", false, gsErr.Error())
	} else {
		_, err := os.Stat(globalSettings)
		if err != nil {
			check("Global settings", false, "optional — "+globalSettings+" not found")
		} else {
			check("Global settings", true, "")
		}
	}

	// 5. Project AGENT.md
	cwd := deps.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	agentMD := filepath.Join(cwd, "AGENT.md")
	if _, err := os.Stat(agentMD); err != nil {
		check("AGENT.md", false, "run 'gogent init' to create")
	} else {
		check("AGENT.md", true, "")
	}

	// 6. Git repo
	gitDir := filepath.Join(cwd, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		check("Git repository", true, "")
	} else {
		check("Git repository", false, "not in a git repo")
	}

	// 7. Project settings (merged — validates combined config, not per-file)
	_, cfgErr := config.Load(cwd)
	if cfgErr != nil {
		check("Config loads cleanly", false, cfgErr.Error())
	} else {
		check("Config loads cleanly", true, "")
	}

	// 8. MCP config
	mcpPath := filepath.Join(cwd, ".gogent", "mcp.json")
	if _, err := os.Stat(mcpPath); err == nil {
		check("MCP config exists", true, "")
	} else {
		check("MCP config", false, "no .gogent/mcp.json (optional)")
	}

	// 9. Hook config
	hookPaths := []string{
		filepath.Join(cwd, ".gogent", "settings.json"),
		filepath.Join(cwd, ".gogent", "settings.local.json"),
	}
	hookFound := false
	for _, hp := range hookPaths {
		if _, err := os.Stat(hp); err == nil {
			hookFound = true
			break
		}
	}
	if hookFound {
		check("Hook config", true, "")
	} else {
		check("Hook config", false, "no .gogent/settings.json (optional)")
	}

	// Summary
	fmt.Fprintf(&b, "\n%d passed, %d warnings, %d failures\n", passes, warnings, failures)

	return Result{DisplayText: b.String()}, nil
}

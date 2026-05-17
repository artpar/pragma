package mcp

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/observe"
)

// ServerConfig describes how to connect to one MCP server.
type ServerConfig struct {
	// Transport type: "stdio" (default if empty), "sse", "http".
	Type string `json:"type,omitempty"`

	// Stdio transport fields.
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// SSE / HTTP transport fields.
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// effectiveType returns the transport type, defaulting to "stdio".
func (sc ServerConfig) effectiveType() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sc.Type == "" {
		observe.GlobalTrace("if: sc.Type == \"\"")
		observe.GlobalTrace("return: \"stdio\"")
		return "stdio"
	}
	observe.GlobalTrace("return: sc.Type")
	return sc.Type
}

// validate checks that the config has the required fields for its transport type.
func (sc ServerConfig) validate() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch sc.effectiveType() {
	case "stdio":
		observe.GlobalTrace("case: \"stdio\"")
		if sc.Command == "" {
			observe.GlobalTrace("return: fmt.Errorf(\"%w: stdio server requires command\", ErrInvalidConfig)")
			return fmt.Errorf("%w: stdio server requires command", ErrInvalidConfig)
		}
	case "sse", "http":
		observe.GlobalTrace("case: \"sse\", \"http\"")
		if sc.URL == "" {
			observe.GlobalTrace("return: fmt.Errorf(\"%w: %s server requires url\", ErrInvalidConfig, sc.effectiveType())")
			return fmt.Errorf("%w: %s server requires url", ErrInvalidConfig, sc.effectiveType())
		}
	default:
		observe.GlobalTrace("default")
		return fmt.Errorf("%w: unknown transport type %q", ErrInvalidConfig, sc.Type)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// MCPConfig is the top-level .mcp.json file structure.
type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type jetBrainsMCPDiscovery struct {
	ProjectPath string    `json:"projectPath"`
	URL         string    `json:"url"`
	MCPConfig   MCPConfig `json:"mcpConfig"`
}

// LoadConfig loads and merges MCP server configs from multiple scopes.
// Scope priority: local > project > global (closer overrides farther).
// Invalid entries are skipped with warnings emitted to bus (per-entry validation).
func LoadConfig(workDir string, bus *observe.EventBus) (map[string]ServerConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	globalPath, err := config.GlobalMCPConfigPath()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve global mcp config path: %w\", err)")
		return nil, fmt.Errorf("resolve global mcp config path: %w", err)
	}

	paths := []string{
		globalPath,
		config.RootMCPConfigPath(workDir),
		config.MCPConfigPath(workDir),
		config.MCPLocalConfigPath(workDir),
	}

	merged := make(map[string]ServerConfig)

	for _, path := range paths {
		observe.GlobalTrace("range paths")
		servers, err := loadSingleConfig(path)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			if errors.Is(err, os.ErrNotExist) {
				observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
				continue
			}
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
				Severity:     "warn",
				Component:    "mcp",
				ErrorType:    "config_load_error",
				ErrorMessage: fmt.Sprintf("failed to load %s: %v", path, err),
			})
			continue
		}

		for name, sc := range servers {
			observe.GlobalTrace("range servers")
			if validErr := sc.validate(); validErr != nil {
				observe.GlobalTrace("if: validErr != nil")
				bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
					Severity:     "warn",
					Component:    "mcp",
					ErrorType:    "config_validation_error",
					ErrorMessage: fmt.Sprintf("skipping mcp server %q from %s: %v", name, path, validErr),
				})
				continue
			}
			merged[name] = sc
		}
	}

	jetBrainsServers, err := loadJetBrainsMCPDiscovery(workDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
			Severity:     "warn",
			Component:    "mcp",
			ErrorType:    "jetbrains_discovery_error",
			ErrorMessage: fmt.Sprintf("failed to load JetBrains MCP discovery: %v", err),
		})
	}
	for name, sc := range jetBrainsServers {
		observe.GlobalTrace("range jetBrainsServers")
		if _, exists := merged[name]; exists {
			observe.GlobalTrace("if: exists")
			continue
		}
		if validErr := sc.validate(); validErr != nil {
			observe.GlobalTrace("if: validErr != nil")
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
				Severity:     "warn",
				Component:    "mcp",
				ErrorType:    "config_validation_error",
				ErrorMessage: fmt.Sprintf("skipping JetBrains MCP server %q: %v", name, validErr),
			})
			continue
		}
		merged[name] = sc
	}
	observe.GlobalTrace("return: merged, nil")

	return merged, nil
}

// loadSingleConfig reads and parses one .mcp.json file.
func loadSingleConfig(path string) (map[string]ServerConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse %s: %w\", path, err)")
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.MCPServers == nil {
		observe.GlobalTrace("if: cfg.MCPServers == nil")
		observe.GlobalTrace("return: nil, nil")
		return nil, nil
	}
	observe.GlobalTrace("return: cfg.MCPServers, nil")

	return cfg.MCPServers, nil
}

func loadJetBrainsMCPDiscovery(workDir string) (map[string]ServerConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	pragmaHome, err := config.PragmaHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return nil, err
	}

	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		absWorkDir = workDir
	}

	dir := filepath.Join(pragmaHome, "jetbrains-mcp")
	paths := []string{
		filepath.Join(dir, jetBrainsProjectHash(absWorkDir)+".json"),
		filepath.Join(dir, "latest.json"),
	}

	for _, path := range paths {
		observe.GlobalTrace("range paths")
		servers, err := loadJetBrainsMCPDiscoveryFile(path, absWorkDir)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			if errors.Is(err, os.ErrNotExist) {
				observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
				continue
			}
			return nil, err
		}
		if len(servers) > 0 {
			observe.GlobalTrace("if: len(servers) > 0")
			return servers, nil
		}
	}
	observe.GlobalTrace("return: nil, nil")
	return nil, nil
}

func loadJetBrainsMCPDiscoveryFile(path, workDir string) (map[string]ServerConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return nil, err
	}

	var discovery jetBrainsMCPDiscovery
	if err := json.Unmarshal(data, &discovery); err != nil {
		observe.GlobalTrace("if: err != nil")
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	projectPath := discovery.ProjectPath
	if projectPath == "" {
		observe.GlobalTrace("if: projectPath == \"\"")
		return nil, nil
	}
	if !sameOrAncestor(projectPath, workDir) {
		observe.GlobalTrace("if: !sameOrAncestor(projectPath, workDir)")
		return nil, nil
	}

	servers := discovery.MCPConfig.MCPServers
	if len(servers) == 0 && discovery.URL != "" {
		observe.GlobalTrace("if: len(servers) == 0 && discovery.URL != \"\"")
		servers = map[string]ServerConfig{
			"jetbrains-" + jetBrainsProjectHash(projectPath): {
				Type: "http",
				URL:  discovery.URL,
			},
		}
	}
	observe.GlobalTrace("return: servers, nil")
	return servers, nil
}

func sameOrAncestor(projectPath, workDir string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	absProject, err := filepath.Abs(projectPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		absProject = projectPath
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		absWorkDir = workDir
	}
	rel, err := filepath.Rel(absProject, absWorkDir)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return false
	}
	observe.GlobalTrace("return: rel == \".\" || rel != \"..\" && !strings.HasPrefix(rel, \"..\"+string(filepath.Separator))")
	return rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func jetBrainsProjectHash(projectPath string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sum := sha256.Sum256([]byte(projectPath))
	observe.GlobalTrace("return: fmt.Sprintf(\"%x\", sum)[:16]")
	return fmt.Sprintf("%x", sum)[:16]
}

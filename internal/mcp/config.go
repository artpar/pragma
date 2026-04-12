package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
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
		observe.GlobalTrace("return: \"stdio\"")
		return "stdio"
	}
	observe.GlobalTrace("return: sc.Type")
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
			observe.GlobalTrace("return: fmt.Errorf(\"%w: stdio server requires command\", ErrInvalidConfig)")
			return fmt.Errorf("%w: stdio server requires command", ErrInvalidConfig)
		}
	case "sse", "http":
		observe.GlobalTrace("case: \"sse\", \"http\"")
		if sc.URL == "" {
			observe.GlobalTrace("return: fmt.Errorf(\"%w: %s server requires url\", ErrInvalidConfig, sc.effectiveType())")
			observe.GlobalTrace("return: fmt.Errorf(\"%w: %s server requires url\", ErrInvalidConfig, sc.effectiveType())")
			return fmt.Errorf("%w: %s server requires url", ErrInvalidConfig, sc.effectiveType())
		}
	default:
		observe.GlobalTrace("default")
		return fmt.Errorf("%w: unknown transport type %q", ErrInvalidConfig, sc.Type)
	}
	observe.GlobalTrace("return: nil")
	observe.GlobalTrace("return: nil")
	return nil
}

// MCPConfig is the top-level .mcp.json file structure.
type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
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
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve global mcp config path: %w\", err)")
		return nil, fmt.Errorf("resolve global mcp config path: %w", err)
	}

	paths := []string{
		globalPath,
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
	observe.GlobalTrace("return: merged, nil")
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
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse %s: %w\", path, err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse %s: %w\", path, err)")
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.MCPServers == nil {
		observe.GlobalTrace("if: cfg.MCPServers == nil")
		observe.GlobalTrace("return: nil, nil")
		observe.GlobalTrace("return: nil, nil")
		return nil, nil
	}
	observe.GlobalTrace("return: cfg.MCPServers, nil")
	observe.GlobalTrace("return: cfg.MCPServers, nil")

	return cfg.MCPServers, nil
}

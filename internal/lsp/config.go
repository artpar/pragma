package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// ServerConfig describes how to start and configure one LSP server.
type ServerConfig struct {
	Command               string            `json:"command"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	ExtensionToLanguage   map[string]string `json:"extensionToLanguage"`
	MaxRestarts           int               `json:"maxRestarts,omitempty"`
	StartupTimeoutMs      int               `json:"startupTimeoutMs,omitempty"`
	InitializationOptions json.RawMessage   `json:"initializationOptions,omitempty"`
	Settings              json.RawMessage   `json:"settings,omitempty"`
}

func (sc ServerConfig) validate() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sc.Command == "" {
		observe.GlobalTrace("if: sc.Command == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"%w: server requires command\", ErrInvalidConfig)")
		return fmt.Errorf("%w: server requires command", ErrInvalidConfig)
	}
	if len(sc.ExtensionToLanguage) == 0 {
		observe.GlobalTrace("if: len(sc.ExtensionToLanguage) == 0")
		observe.GlobalTrace("return: fmt.Errorf(\"%w: server requires extensionToLanguage\", ErrInvalidConfig)")
		return fmt.Errorf("%w: server requires extensionToLanguage", ErrInvalidConfig)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func (sc ServerConfig) maxRestartsOrDefault() int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sc.MaxRestarts > 0 {
		observe.GlobalTrace("if: sc.MaxRestarts > 0")
		observe.GlobalTrace("return: sc.MaxRestarts")
		return sc.MaxRestarts
	}
	observe.GlobalTrace("return: 3")
	return 3
}

func (sc ServerConfig) startupTimeoutMsOrDefault() int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if sc.StartupTimeoutMs > 0 {
		observe.GlobalTrace("if: sc.StartupTimeoutMs > 0")
		observe.GlobalTrace("return: sc.StartupTimeoutMs")
		return sc.StartupTimeoutMs
	}
	observe.GlobalTrace("return: 30000")
	return 30000
}

// LSPConfig is the top-level .gogent/lsp.json file structure.
type LSPConfig struct {
	LSPServers map[string]ServerConfig `json:"lspServers"`
}

// LoadConfig loads and merges LSP server configs from multiple scopes.
// Scope priority: local > project > global (closer overrides farther).
func LoadConfig(workDir string, bus *observe.EventBus) (map[string]ServerConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	globalPath, err := config.GlobalLSPConfigPath()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"resolve global lsp config path: %w\", err)")
		return nil, fmt.Errorf("resolve global lsp config path: %w", err)
	}

	paths := []string{
		globalPath,
		config.LSPConfigPath(workDir),
		config.LSPLocalConfigPath(workDir),
	}

	merged := make(map[string]ServerConfig)

	for _, path := range paths {
		observe.GlobalTrace("range paths")
		servers, err := loadSingleLSPConfig(path)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			if errors.Is(err, os.ErrNotExist) {
				observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
				continue
			}
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
				Severity:     "warn",
				Component:    "lsp",
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
					Component:    "lsp",
					ErrorType:    "config_validation_error",
					ErrorMessage: fmt.Sprintf("skipping lsp server %q from %s: %v", name, path, validErr),
				})
				continue
			}
			merged[name] = sc
		}
	}
	observe.GlobalTrace("return: merged, nil")

	return merged, nil
}

func loadSingleLSPConfig(path string) (map[string]ServerConfig, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}

	var cfg LSPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"parse %s: %w\", path, err)")
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if cfg.LSPServers == nil {
		observe.GlobalTrace("if: cfg.LSPServers == nil")
		observe.GlobalTrace("return: nil, nil")
		return nil, nil
	}
	observe.GlobalTrace("return: cfg.LSPServers, nil")

	return cfg.LSPServers, nil
}

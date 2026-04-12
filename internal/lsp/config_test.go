package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/gogent/internal/observe"
)

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: ServerConfig{
				Command:             "gopls",
				ExtensionToLanguage: map[string]string{".go": "go"},
			},
			wantErr: false,
		},
		{
			name: "missing command",
			config: ServerConfig{
				ExtensionToLanguage: map[string]string{".go": "go"},
			},
			wantErr: true,
		},
		{
			name: "missing extension mapping",
			config: ServerConfig{
				Command: "gopls",
			},
			wantErr: true,
		},
		{
			name:    "empty config",
			config:  ServerConfig{},
			wantErr: true,
		},
		{
			name: "empty extension mapping",
			config: ServerConfig{
				Command:             "gopls",
				ExtensionToLanguage: map[string]string{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerConfig_Defaults(t *testing.T) {
	sc := ServerConfig{Command: "gopls", ExtensionToLanguage: map[string]string{".go": "go"}}

	if got := sc.maxRestartsOrDefault(); got != 3 {
		t.Errorf("maxRestartsOrDefault() = %d, want 3", got)
	}
	if got := sc.startupTimeoutMsOrDefault(); got != 30000 {
		t.Errorf("startupTimeoutMsOrDefault() = %d, want 30000", got)
	}

	sc.MaxRestarts = 10
	sc.StartupTimeoutMs = 5000
	if got := sc.maxRestartsOrDefault(); got != 10 {
		t.Errorf("maxRestartsOrDefault() = %d, want 10", got)
	}
	if got := sc.startupTimeoutMsOrDefault(); got != 5000 {
		t.Errorf("startupTimeoutMsOrDefault() = %d, want 5000", got)
	}
}

func TestLoadSingleLSPConfig(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantCount  int
		wantErr    bool
		wantNilMap bool
	}{
		{
			name: "valid single server",
			content: `{"lspServers": {"gopls": {"command": "gopls", "extensionToLanguage": {".go": "go"}}}}`,
			wantCount: 1,
		},
		{
			name: "valid multiple servers",
			content: `{"lspServers": {
				"gopls": {"command": "gopls", "extensionToLanguage": {".go": "go"}},
				"tsserver": {"command": "typescript-language-server", "extensionToLanguage": {".ts": "typescript"}}
			}}`,
			wantCount: 2,
		},
		{
			name:       "empty lspServers",
			content:    `{"lspServers": null}`,
			wantNilMap: true,
		},
		{
			name:       "no lspServers key",
			content:    `{}`,
			wantNilMap: true,
		},
		{
			name:    "invalid JSON",
			content: `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "lsp.json")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			result, err := loadSingleLSPConfig(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadSingleLSPConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNilMap {
				if result != nil {
					t.Errorf("expected nil map, got %v", result)
				}
				return
			}
			if !tt.wantErr && len(result) != tt.wantCount {
				t.Errorf("got %d servers, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestLoadSingleLSPConfig_MissingFile(t *testing.T) {
	_, err := loadSingleLSPConfig("/nonexistent/path/lsp.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadConfig_MergesScopes(t *testing.T) {
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".gogent")
	if err := os.MkdirAll(gogentDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write project-level config
	projectCfg := LSPConfig{
		LSPServers: map[string]ServerConfig{
			"gopls": {
				Command:             "gopls",
				ExtensionToLanguage: map[string]string{".go": "go"},
				MaxRestarts:         3,
			},
			"tsserver": {
				Command:             "typescript-language-server",
				ExtensionToLanguage: map[string]string{".ts": "typescript"},
			},
		},
	}
	data, _ := json.Marshal(projectCfg)
	if err := os.WriteFile(filepath.Join(gogentDir, "lsp.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write local config that overrides gopls
	localCfg := LSPConfig{
		LSPServers: map[string]ServerConfig{
			"gopls": {
				Command:             "gopls",
				ExtensionToLanguage: map[string]string{".go": "go"},
				MaxRestarts:         10,
			},
		},
	}
	data, _ = json.Marshal(localCfg)
	if err := os.WriteFile(filepath.Join(gogentDir, "lsp.local.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	result, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Local should override project for gopls
	if gopls, ok := result["gopls"]; !ok {
		t.Error("gopls missing from merged config")
	} else if gopls.MaxRestarts != 10 {
		t.Errorf("gopls MaxRestarts = %d, want 10 (local override)", gopls.MaxRestarts)
	}

	// tsserver should come through from project
	if _, ok := result["tsserver"]; !ok {
		t.Error("tsserver missing — project config should have contributed it")
	}
}

func TestLoadConfig_SkipsInvalidServers(t *testing.T) {
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".gogent")
	if err := os.MkdirAll(gogentDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Config with one valid and one invalid server (no command)
	cfg := LSPConfig{
		LSPServers: map[string]ServerConfig{
			"valid": {
				Command:             "gopls",
				ExtensionToLanguage: map[string]string{".go": "go"},
			},
			"invalid": {
				ExtensionToLanguage: map[string]string{".py": "python"},
			},
		},
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(gogentDir, "lsp.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	result, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 valid server, got %d", len(result))
	}
	if _, ok := result["valid"]; !ok {
		t.Error("valid server missing from result")
	}
}

func TestLoadConfig_NoConfigs(t *testing.T) {
	dir := t.TempDir()

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	result, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d servers", len(result))
	}
}

func TestLSPConfig_JSONRoundTrip(t *testing.T) {
	cfg := LSPConfig{
		LSPServers: map[string]ServerConfig{
			"gopls": {
				Command:             "gopls",
				Args:                []string{"-remote=auto"},
				Env:                 map[string]string{"GOFLAGS": "-tags=test"},
				ExtensionToLanguage: map[string]string{".go": "go"},
				MaxRestarts:         5,
				StartupTimeoutMs:    10000,
			},
		},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var roundTripped LSPConfig
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	got := roundTripped.LSPServers["gopls"]
	want := cfg.LSPServers["gopls"]

	if got.Command != want.Command {
		t.Errorf("Command = %q, want %q", got.Command, want.Command)
	}
	if len(got.Args) != len(want.Args) {
		t.Errorf("Args len = %d, want %d", len(got.Args), len(want.Args))
	}
	if got.MaxRestarts != want.MaxRestarts {
		t.Errorf("MaxRestarts = %d, want %d", got.MaxRestarts, want.MaxRestarts)
	}
	if got.StartupTimeoutMs != want.StartupTimeoutMs {
		t.Errorf("StartupTimeoutMs = %d, want %d", got.StartupTimeoutMs, want.StartupTimeoutMs)
	}
}

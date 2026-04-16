package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/artpar/pragma/internal/observe"
)

func TestLoadSingleConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")

	content := `{
		"mcpServers": {
			"github": {
				"command": "github-mcp-server",
				"args": ["--token", "xxx"]
			},
			"db": {
				"type": "sse",
				"url": "http://localhost:8080/sse"
			}
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	servers, err := loadSingleConfig(path)
	if err != nil {
		t.Fatalf("loadSingleConfig: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	gh, ok := servers["github"]
	if !ok {
		t.Fatal("missing github server")
	}
	if gh.Command != "github-mcp-server" {
		t.Errorf("github command = %q, want %q", gh.Command, "github-mcp-server")
	}
	if len(gh.Args) != 2 || gh.Args[0] != "--token" {
		t.Errorf("github args = %v, want [--token xxx]", gh.Args)
	}
	if gh.effectiveType() != "stdio" {
		t.Errorf("github type = %q, want stdio", gh.effectiveType())
	}

	db, ok := servers["db"]
	if !ok {
		t.Fatal("missing db server")
	}
	if db.Type != "sse" {
		t.Errorf("db type = %q, want sse", db.Type)
	}
	if db.URL != "http://localhost:8080/sse" {
		t.Errorf("db url = %q", db.URL)
	}
}

func TestLoadSingleConfig_NotExist(t *testing.T) {
	_, err := loadSingleConfig("/nonexistent/mcp.json")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

func TestLoadSingleConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := loadSingleConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
	}{
		{"stdio valid", ServerConfig{Command: "cmd"}, false},
		{"stdio explicit", ServerConfig{Type: "stdio", Command: "cmd"}, false},
		{"stdio missing command", ServerConfig{Type: "stdio"}, true},
		{"stdio default missing command", ServerConfig{}, true},
		{"sse valid", ServerConfig{Type: "sse", URL: "http://localhost"}, false},
		{"sse missing url", ServerConfig{Type: "sse"}, true},
		{"http valid", ServerConfig{Type: "http", URL: "http://localhost"}, false},
		{"http missing url", ServerConfig{Type: "http"}, true},
		{"unknown type", ServerConfig{Type: "websocket"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadConfig_MergeScopes(t *testing.T) {
	// Create a temp directory structure with global and project configs
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(gogentDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Project config — defines "github" and "db"
	projectConfig := `{
		"mcpServers": {
			"github": {
				"command": "gh-server",
				"args": ["--mode", "project"]
			},
			"db": {
				"type": "sse",
				"url": "http://db.project:8080"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(gogentDir, "mcp.json"), []byte(projectConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Local config — overrides "github"
	localConfig := `{
		"mcpServers": {
			"github": {
				"command": "gh-server-local",
				"args": ["--mode", "local"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(gogentDir, "mcp.local.json"), []byte(localConfig), 0644); err != nil {
		t.Fatal(err)
	}

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Should have 2 servers: github (overridden by local) and db (from project)
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}

	gh := servers["github"]
	if gh.Command != "gh-server-local" {
		t.Errorf("github command = %q, want gh-server-local (local override)", gh.Command)
	}

	db := servers["db"]
	if db.URL != "http://db.project:8080" {
		t.Errorf("db url = %q, want http://db.project:8080", db.URL)
	}
}

func TestLoadConfig_InvalidEntrySkipped(t *testing.T) {
	dir := t.TempDir()
	gogentDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(gogentDir, 0755); err != nil {
		t.Fatal(err)
	}

	// One valid, one invalid (stdio missing command)
	content := `{
		"mcpServers": {
			"good": {"command": "good-server"},
			"bad": {"type": "stdio"}
		}
	}`
	if err := os.WriteFile(filepath.Join(gogentDir, "mcp.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server (bad skipped), got %d", len(servers))
	}
	if _, ok := servers["good"]; !ok {
		t.Error("expected 'good' server to be loaded")
	}
	if _, ok := servers["bad"]; ok {
		t.Error("expected 'bad' server to be skipped")
	}
}

func TestLoadConfig_NoConfigs(t *testing.T) {
	dir := t.TempDir()
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

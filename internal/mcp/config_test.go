package mcp

import (
	"encoding/json"
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

func TestLoadConfig_GlobalScope(t *testing.T) {
	// Verify global ~/.pragma/mcp.json is loaded when present
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	globalPragma := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(globalPragma, 0755); err != nil {
		t.Fatal(err)
	}

	globalConfig := `{
		"mcpServers": {
			"analytics": {
				"command": "analytics-mcp",
				"args": ["--global"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(globalPragma, "mcp.json"), []byte(globalConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Use a separate workDir with no project config
	workDir := t.TempDir()

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(workDir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(servers) != 1 {
		t.Fatalf("expected 1 server from global scope, got %d", len(servers))
	}
	if srv, ok := servers["analytics"]; !ok {
		t.Error("expected 'analytics' server from global config")
	} else if srv.Command != "analytics-mcp" {
		t.Errorf("analytics command = %q, want analytics-mcp", srv.Command)
	}
}

func TestLoadConfig_MergeScopes(t *testing.T) {
	// Create a temp directory structure with global and project configs
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	pragmaDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(pragmaDir, 0755); err != nil {
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
	if err := os.WriteFile(filepath.Join(pragmaDir, "mcp.json"), []byte(projectConfig), 0644); err != nil {
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
	if err := os.WriteFile(filepath.Join(pragmaDir, "mcp.local.json"), []byte(localConfig), 0644); err != nil {
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

func TestLoadConfig_RootMCPJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))

	content := `{
		"mcpServers": {
			"chrome-devtools": {
				"command": "npx",
				"args": ["-y", "chrome-devtools-mcp@latest"]
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(dir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	srv, ok := servers["chrome-devtools"]
	if !ok {
		t.Fatal("expected chrome-devtools server from .mcp.json")
	}
	if srv.Command != "npx" {
		t.Errorf("command = %q, want npx", srv.Command)
	}
}

func TestLoadConfig_JetBrainsDiscoveryExactProject(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		t.Fatal(err)
	}

	serverName := "jetbrains-" + jetBrainsProjectHash(absWorkDir)
	writeJetBrainsDiscovery(t, home, jetBrainsProjectHash(absWorkDir)+".json", absWorkDir, serverName, "http://127.0.0.1:49231")

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(workDir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	srv, ok := servers[serverName]
	if !ok {
		t.Fatalf("expected discovered JetBrains MCP server %q", serverName)
	}
	if srv.effectiveType() != "http" {
		t.Errorf("type = %q, want http", srv.effectiveType())
	}
	if srv.URL != "http://127.0.0.1:49231" {
		t.Errorf("url = %q", srv.URL)
	}
	if !srv.PreserveToolNames {
		t.Errorf("PreserveToolNames = false, want true")
	}
}

func TestLoadConfig_JetBrainsDiscoveryLatestForWorkspaceChild(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	projectDir := filepath.Join(dir, "project")
	workDir := filepath.Join(projectDir, "pkg")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		t.Fatal(err)
	}

	serverName := "jetbrains-" + jetBrainsProjectHash(absProjectDir)
	writeJetBrainsDiscovery(t, home, "latest.json", absProjectDir, serverName, "http://127.0.0.1:49232")

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(workDir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := servers[serverName]; !ok {
		t.Fatalf("expected latest JetBrains MCP discovery for ancestor project")
	}
}

func TestLoadConfig_JetBrainsDiscoverySkipsOtherProject(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workDir := filepath.Join(dir, "project")
	otherDir := filepath.Join(dir, "other")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatal(err)
	}
	absOtherDir, err := filepath.Abs(otherDir)
	if err != nil {
		t.Fatal(err)
	}

	serverName := "jetbrains-" + jetBrainsProjectHash(absOtherDir)
	writeJetBrainsDiscovery(t, home, "latest.json", absOtherDir, serverName, "http://127.0.0.1:49233")

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(workDir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if _, ok := servers[serverName]; ok {
		t.Fatalf("did not expect JetBrains MCP discovery for another project")
	}
}

func TestLoadConfig_JetBrainsDiscoveryDoesNotOverrideExplicitConfig(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		t.Fatal(err)
	}

	serverName := "jetbrains-" + jetBrainsProjectHash(absWorkDir)
	rootConfig := `{
		"mcpServers": {
			"` + serverName + `": {
				"type": "http",
				"url": "http://manual.example/mcp"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(workDir, ".mcp.json"), []byte(rootConfig), 0644); err != nil {
		t.Fatal(err)
	}
	writeJetBrainsDiscovery(t, home, jetBrainsProjectHash(absWorkDir)+".json", absWorkDir, serverName, "http://127.0.0.1:49234")

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	servers, err := LoadConfig(workDir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if servers[serverName].URL != "http://manual.example/mcp" {
		t.Fatalf("explicit config should win, got url %q", servers[serverName].URL)
	}
	if servers[serverName].PreserveToolNames {
		t.Fatalf("explicit config should keep its own preserveToolNames value")
	}
}

func TestLoadConfig_InvalidEntrySkipped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	pragmaDir := filepath.Join(dir, ".pragma")
	if err := os.MkdirAll(pragmaDir, 0755); err != nil {
		t.Fatal(err)
	}

	// One valid, one invalid (stdio missing command)
	content := `{
		"mcpServers": {
			"good": {"command": "good-server"},
			"bad": {"type": "stdio"}
		}
	}`
	if err := os.WriteFile(filepath.Join(pragmaDir, "mcp.json"), []byte(content), 0644); err != nil {
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
	t.Setenv("HOME", dir)
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

func writeJetBrainsDiscovery(t *testing.T, home, filename, projectPath, serverName, url string) {
	t.Helper()
	dir := filepath.Join(home, ".pragma", "jetbrains-mcp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `{
		"server": "pragma-jetbrains-reflective-mcp",
		"projectPath": ` + quoteJSON(projectPath) + `,
		"url": ` + quoteJSON(url) + `,
		"mcpConfig": {
			"mcpServers": {
				"` + serverName + `": {
					"type": "http",
					"url": ` + quoteJSON(url) + `
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func quoteJSON(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

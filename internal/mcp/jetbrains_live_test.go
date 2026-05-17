package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

const jetBrainsApplicationInfoTool = "com.intellij.openapi.application.ApplicationInfo.getInstance"

func TestLiveJetBrainsMCPDiscovery(t *testing.T) {
	if os.Getenv("PRAGMA_JETBRAINS_MCP_LIVE") != "1" {
		t.Skip("set PRAGMA_JETBRAINS_MCP_LIVE=1 to run against a live JetBrains MCP plugin")
	}

	workDir := os.Getenv("PRAGMA_JETBRAINS_MCP_WORKDIR")
	if workDir == "" {
		var err error
		workDir, err = repoRoot()
		if err != nil {
			t.Fatalf("resolve repo root: %v", err)
		}
	}

	servers, err := loadJetBrainsMCPDiscovery(workDir)
	if err != nil {
		t.Fatalf("load JetBrains MCP discovery: %v", err)
	}
	if len(servers) == 0 {
		t.Fatalf("no JetBrains MCP server discovered for %s", workDir)
	}

	serverName := ""
	for name := range servers {
		if strings.HasPrefix(name, "jetbrains-") {
			serverName = name
			break
		}
	}
	if serverName == "" {
		t.Fatalf("discovery did not include a jetbrains-* server: %v", servers)
	}

	bus := observe.NewEventBus(64)
	defer bus.Drain()
	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)
	defer mgr.DisconnectAll()

	if errs := mgr.ConnectAll(context.Background(), servers); len(errs) > 0 {
		t.Fatalf("connect discovered JetBrains MCP server: %v", errs)
	}
	if err := mgr.RegisterTools(context.Background()); err != nil {
		t.Fatalf("register JetBrains MCP tools: %v", err)
	}

	statuses := mgr.ServerStatuses()
	if len(statuses) != 1 || statuses[0].Name != serverName || statuses[0].Status != StatusConnected {
		t.Fatalf("unexpected server statuses: %+v", statuses)
	}

	registeredName := jetBrainsApplicationInfoTool
	desc, ok := registry.Get(registeredName)
	if !ok {
		t.Fatalf("expected registered tool %q", registeredName)
	}

	result, err := desc.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("invoke %s: %v", registeredName, err)
	}
	if !strings.Contains(result.Content, `"projectPath"`) || !strings.Contains(result.Content, `"ide"`) {
		t.Fatalf("unexpected ApplicationInfo result: %s", result.Content)
	}
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

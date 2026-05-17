package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

func TestManager_RegisterTools(t *testing.T) {
	echoTool := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: map[string]any{"msg": map[string]any{"type": "string"}},
			},
		},
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(req.GetString("msg", "")), nil
		},
	}

	srv, err := mcptest.NewServer(t, echoTool)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)

	mgr := NewManager(bus, registry)

	// Manually inject the connected client
	wrapper := &Client{
		name:      "test-srv",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}
	mgr.clients["test-srv"] = wrapper

	// Register tools from server
	if err := mgr.RegisterTools(context.Background()); err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	// Verify tool is registered in the registry
	fullName := BuildToolName("test-srv", "echo")
	desc, ok := registry.Get(fullName)
	if !ok {
		t.Fatalf("tool %q not found in registry", fullName)
	}

	if desc.Name() != fullName {
		t.Errorf("tool name = %q, want %q", desc.Name(), fullName)
	}

	// Verify the tool works via the registry
	input := json.RawMessage(`{"msg": "hello"}`)
	result, err := desc.Invoke(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if result.Content != "hello" {
		t.Errorf("result = %q, want %q", result.Content, "hello")
	}
}

func TestManager_ToolDefs(t *testing.T) {
	echoTool := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: map[string]any{"msg": map[string]any{"type": "string"}},
			},
		},
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	}

	srv, err := mcptest.NewServer(t, echoTool)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)

	wrapper := &Client{
		name:      "srv1",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}
	mgr.clients["srv1"] = wrapper
	mgr.RegisterTools(context.Background())

	// ToolDefs should include the MCP tool
	defs := registry.ToolDefs()
	found := false
	for _, def := range defs {
		if def.Name == BuildToolName("srv1", "echo") {
			found = true
			if def.Description != "Echoes input" {
				t.Errorf("description = %q", def.Description)
			}
		}
	}
	if !found {
		t.Error("MCP tool not found in ToolDefs")
	}
}

func TestManager_ConnectsDiscoveredJetBrainsHTTPServer(t *testing.T) {
	const reflectiveTool = "com.intellij.openapi.application.ApplicationInfo.getInstance"

	mcpServer := server.NewMCPServer(
		"pragma-jetbrains-reflective-mcp",
		"0.1.0",
		server.WithToolCapabilities(false),
	)
	mcpServer.AddTool(
		mcp.NewTool(reflectiveTool, mcp.WithDescription("Return IDE and project information")),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(`{"projectName":"pragma","ide":{"productName":"IntelliJ IDEA"}}`), nil
		},
	)

	httpServer := server.NewTestStreamableHTTPServer(mcpServer)
	defer httpServer.Close()

	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	t.Setenv("HOME", home)
	workDir := filepath.Join(dir, "pragma")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		t.Fatal(err)
	}
	serverName := "jetbrains-" + jetBrainsProjectHash(absWorkDir)
	writeJetBrainsDiscovery(t, home, jetBrainsProjectHash(absWorkDir)+".json", absWorkDir, serverName, httpServer.URL)

	bus := observe.NewEventBus(64)
	defer bus.Drain()
	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)

	servers, err := LoadConfig(workDir, bus)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if errs := mgr.ConnectAll(context.Background(), servers); len(errs) > 0 {
		t.Fatalf("ConnectAll errors: %v", errs)
	}
	if err := mgr.RegisterTools(context.Background()); err != nil {
		t.Fatalf("RegisterTools: %v", err)
	}

	fullName := BuildToolName(serverName, reflectiveTool)
	desc, ok := registry.Get(fullName)
	if !ok {
		t.Fatalf("expected registered JetBrains MCP tool %q", fullName)
	}
	result, err := desc.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, `"projectName":"pragma"`) {
		t.Fatalf("expected IDE project state, got %q", result.Content)
	}
}

func TestManager_DisconnectAll(t *testing.T) {
	echoTool := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "echo",
			Description: "Echoes input",
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: map[string]any{},
			},
		},
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("ok"), nil
		},
	}

	srv, err := mcptest.NewServer(t, echoTool)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)

	wrapper := &Client{
		name:      "srv1",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}
	mgr.clients["srv1"] = wrapper
	mgr.RegisterTools(context.Background())

	// Tool should be registered
	fullName := BuildToolName("srv1", "echo")
	if _, ok := registry.Get(fullName); !ok {
		t.Fatal("tool not registered before disconnect")
	}

	mgr.DisconnectAll()

	// Tool should be unregistered
	if _, ok := registry.Get(fullName); ok {
		t.Error("tool still registered after DisconnectAll")
	}

	if mgr.ConnectedCount() != 0 {
		t.Errorf("connected count = %d, want 0", mgr.ConnectedCount())
	}
}

func TestManager_ServerStatus(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)

	mgr.clients["alive"] = &Client{name: "alive", connected: true, bus: bus}
	mgr.clients["dead"] = &Client{name: "dead", connected: false, bus: bus}

	status := mgr.ServerStatus()
	if status["alive"] != "connected" {
		t.Errorf("alive status = %q", status["alive"])
	}
	if status["dead"] != "disconnected" {
		t.Errorf("dead status = %q", status["dead"])
	}
}

func TestManager_ServerStatusIncludesConfiguredDisconnectedServers(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)
	mgr.configs["configured"] = ServerConfig{Command: "missing-command"}

	status := mgr.ServerStatus()
	if status["configured"] != "disconnected" {
		t.Errorf("configured status = %q, want disconnected", status["configured"])
	}
}

func TestManager_ServerStatusesIncludesFailures(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)
	mgr.configs["broken"] = ServerConfig{Command: "missing-command"}
	mgr.statuses["broken"] = StatusFailed
	mgr.lastErrors["broken"] = "exec: missing-command: executable file not found"

	statuses := mgr.ServerStatuses()
	if len(statuses) != 1 {
		t.Fatalf("statuses len = %d, want 1", len(statuses))
	}
	got := statuses[0]
	if got.Name != "broken" {
		t.Errorf("name = %q, want broken", got.Name)
	}
	if got.Status != StatusFailed {
		t.Errorf("status = %q, want %q", got.Status, StatusFailed)
	}
	if got.Error == "" {
		t.Fatal("expected failure error to be preserved")
	}
	if got.Transport != "stdio" {
		t.Errorf("transport = %q, want stdio", got.Transport)
	}
}

func TestManager_PendingServerNames(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	registry := tool.NewRegistry(bus)
	mgr := NewManager(bus, registry)
	mgr.statuses["ready"] = StatusConnected
	mgr.statuses["waiting"] = StatusPending

	names := mgr.PendingServerNames()
	if len(names) != 1 || names[0] != "waiting" {
		t.Fatalf("pending names = %v, want [waiting]", names)
	}
}

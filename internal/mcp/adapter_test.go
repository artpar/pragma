package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"

	"github.com/artpar/pragma/internal/observe"
)

// MCPINJ-001 unit gates for the adapter conversion (ported from the
// worktree branch's MCPToolAdapter behavior).

func TestToolDefConversion(t *testing.T) {
	def := ToolDef("past-conversations", ToolInfo{
		Name:        "list_projects",
		Description: "List projects",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"limit":{"type":"number"}}}`),
	})
	if def.Name != "mcp__past-conversations__list_projects" {
		t.Fatalf("name = %q", def.Name)
	}
	if def.Description != "List projects" {
		t.Fatalf("description = %q", def.Description)
	}
	if string(def.InputSchema) != `{"type":"object","properties":{"limit":{"type":"number"}}}` {
		t.Fatalf("schema = %s", def.InputSchema)
	}

	long := strings.Repeat("x", maxToolDescriptionLen+10)
	def = ToolDef("srv", ToolInfo{Name: "t", Description: long})
	if len(def.Description) != maxToolDescriptionLen {
		t.Fatalf("description len = %d, want cap %d", len(def.Description), maxToolDescriptionLen)
	}

	def = ToolDef("srv", ToolInfo{Name: "t"})
	if string(def.InputSchema) != defaultToolSchema {
		t.Fatalf("default schema = %s, want %s", def.InputSchema, defaultToolSchema)
	}
}

func TestToolDefNormalizesToolNames(t *testing.T) {
	// JetBrains-style dotted tool names must be exposed with normalized
	// model-safe names.
	def := ToolDef("jetbrains-79dfb383eaefab75", ToolInfo{Name: "ide.capabilities", Description: "d"})
	if def.Name != "mcp__jetbrains-79dfb383eaefab75__ide_capabilities" {
		t.Fatalf("name = %q", def.Name)
	}
}

func TestClientToolDefsViaTestServer(t *testing.T) {
	srv := newTestServer(t)
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	client := &Client{
		name:      "test-server",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}
	defs, err := client.ToolDefs(context.Background())
	if err != nil {
		t.Fatalf("ToolDefs: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("defs = %d, want 2", len(defs))
	}
	names := map[string]bool{}
	for _, def := range defs {
		names[def.Name] = true
		if len(def.InputSchema) == 0 {
			t.Fatalf("def %q has empty schema", def.Name)
		}
	}
	if !names["mcp__test-server__echo"] || !names["mcp__test-server__greet"] {
		t.Fatalf("names = %v", names)
	}
}

func newManagerWithClients(t *testing.T, clients map[string]*Client) *Manager {
	t.Helper()
	bus := observe.NewEventBus(64)
	t.Cleanup(bus.Drain)
	m := NewManager(bus)
	for name, client := range clients {
		m.clients[name] = client
	}
	return m
}

func connectedTestClient(t *testing.T, name string, srv *mcptest.Server) *Client {
	t.Helper()
	return &Client{
		name:      name,
		config:    ServerConfig{Command: "test"},
		bus:       observe.NewEventBus(16),
		mcpCli:    srv.Client(),
		connected: true,
	}
}

func TestManagerToolDefsAggregatesConnectedServers(t *testing.T) {
	srv := newTestServer(t)
	connected := connectedTestClient(t, "alpha", srv)
	disconnected := &Client{name: "beta", config: ServerConfig{Command: "test"}, bus: observe.NewEventBus(16), connected: false}
	m := newManagerWithClients(t, map[string]*Client{
		"zulu":  connected,
		"beta":  disconnected,
		"alpha": connected, // same underlying server: duplicate tool names
	})

	defs := m.ToolDefs(context.Background())
	if len(defs) != 2 {
		t.Fatalf("defs = %d, want 2 (duplicate names deduped)", len(defs))
	}
	// Deterministic order: sorted by server name, then advertised order.
	if defs[0].Name != "mcp__alpha__echo" || defs[1].Name != "mcp__alpha__greet" {
		t.Fatalf("defs = %v, want deterministic [alpha.echo alpha.greet]", []string{defs[0].Name, defs[1].Name})
	}
}

func TestManagerCallMCPToolRoutesToServer(t *testing.T) {
	srv := newTestServer(t)
	m := newManagerWithClients(t, map[string]*Client{
		"test-server": connectedTestClient(t, "test-server", srv),
	})

	out, err := m.CallMCPTool(context.Background(), "mcp__test-server__echo", json.RawMessage(`{"message":"routing works"}`))
	if err != nil {
		t.Fatalf("CallMCPTool: %v", err)
	}
	if out != "routing works" {
		t.Fatalf("output = %q, want %q", out, "routing works")
	}
}

func TestManagerCallMCPToolRecoversOriginalToolName(t *testing.T) {
	// A tool whose advertised name is not model-safe must be callable under
	// its normalized injected name.
	dotted := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "ide.capabilities",
			Description: "Dotted tool",
			InputSchema: mcp.ToolInputSchema{Type: "object"},
		},
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText("capabilities ok"), nil
		},
	}
	srv, err := mcptest.NewServer(t, dotted)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	m := newManagerWithClients(t, map[string]*Client{
		"jetbrains-ab12cd34ef56": connectedTestClient(t, "jetbrains-ab12cd34ef56", srv),
	})

	defs := m.ToolDefs(context.Background())
	found := false
	for _, def := range defs {
		if def.Name == "mcp__jetbrains-ab12cd34ef56__ide_capabilities" {
			found = true
		}
	}
	if !found {
		t.Fatalf("normalized def missing: %v", defs)
	}

	out, err := m.CallMCPTool(context.Background(), "mcp__jetbrains-ab12cd34ef56__ide_capabilities", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallMCPTool: %v", err)
	}
	if out != "capabilities ok" {
		t.Fatalf("output = %q", out)
	}
}

func TestManagerCallMCPToolErrors(t *testing.T) {
	srv := newTestServer(t)
	m := newManagerWithClients(t, map[string]*Client{
		"test-server": connectedTestClient(t, "test-server", srv),
	})

	if _, err := m.CallMCPTool(context.Background(), "Bash", nil); err == nil {
		t.Fatal("non-mcp name must be rejected")
	}
	if _, err := m.CallMCPTool(context.Background(), "mcp__nosuchserver__echo", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown server must error")
	} else if !strings.Contains(err.Error(), "no connected MCP server") {
		t.Fatalf("unknown server error = %v", err)
	}
	if _, err := m.CallMCPTool(context.Background(), "mcp__test-server__nosuchtool", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown tool must error")
	} else if !strings.Contains(err.Error(), "unknown MCP tool") {
		t.Fatalf("unknown tool error = %v", err)
	}
}

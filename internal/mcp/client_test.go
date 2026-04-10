package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"

	"github.com/artpar/gogent/internal/observe"
)

func newTestServer(t *testing.T) *mcptest.Server {
	t.Helper()

	echoTool := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "echo",
			Description: "Echoes the input message",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "Message to echo",
					},
				},
				Required: []string{"message"},
			},
			Annotations: mcp.ToolAnnotation{
				ReadOnlyHint: boolPtr(true),
			},
		},
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msg := req.GetString("message", "")
			return mcp.NewToolResultText(msg), nil
		},
	}

	greetTool := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "greet",
			Description: "Greets a person",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"name": map[string]any{
						"type": "string",
					},
				},
			},
			Annotations: mcp.ToolAnnotation{
				DestructiveHint: boolPtr(true),
			},
		},
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name := req.GetString("name", "world")
			return mcp.NewToolResultText("Hello, " + name + "!"), nil
		},
	}

	srv, err := mcptest.NewServer(t, echoTool, greetTool)
	if err != nil {
		t.Fatalf("create test server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv
}

func boolPtr(b bool) *bool { return &b }

func TestClientListTools_ViaTestServer(t *testing.T) {
	srv := newTestServer(t)
	cli := srv.Client()

	result, err := cli.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}
	if !toolNames["echo"] || !toolNames["greet"] {
		t.Errorf("expected echo and greet tools, got %v", toolNames)
	}
}

func TestClientCallTool_ViaTestServer(t *testing.T) {
	srv := newTestServer(t)
	cli := srv.Client()

	result, err := cli.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "echo",
			Arguments: map[string]any{"message": "hello world"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	text := extractTextContent(result)
	if text != "hello world" {
		t.Errorf("got %q, want %q", text, "hello world")
	}
}

func TestWrappedClient_ListTools(t *testing.T) {
	srv := newTestServer(t)
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	// Create our wrapper Client that delegates to the test server's client
	wrapper := &Client{
		name:      "test-server",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}

	tools, err := wrapper.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Verify ToolInfo is populated correctly
	var echoInfo ToolInfo
	for _, ti := range tools {
		if ti.Name == "echo" {
			echoInfo = ti
			break
		}
	}
	if echoInfo.Name == "" {
		t.Fatal("echo tool not found")
	}
	if echoInfo.Description != "Echoes the input message" {
		t.Errorf("echo description = %q", echoInfo.Description)
	}
	if !echoInfo.ReadOnly {
		t.Error("echo should be ReadOnly (readOnlyHint=true)")
	}

	// Verify greet tool has destructive flag
	var greetInfo ToolInfo
	for _, ti := range tools {
		if ti.Name == "greet" {
			greetInfo = ti
			break
		}
	}
	if !greetInfo.Destructive {
		t.Error("greet should be Destructive (destructiveHint=true)")
	}
}

func TestWrappedClient_CallTool(t *testing.T) {
	srv := newTestServer(t)
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	wrapper := &Client{
		name:      "test-server",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}

	args := json.RawMessage(`{"message": "testing 123"}`)
	result, err := wrapper.CallTool(context.Background(), "echo", args)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	if result != "testing 123" {
		t.Errorf("got %q, want %q", result, "testing 123")
	}
}

func TestWrappedClient_CallTool_NotConnected(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	wrapper := &Client{
		name:      "dead-server",
		config:    ServerConfig{Command: "dead"},
		bus:       bus,
		connected: false,
	}

	_, err := wrapper.CallTool(context.Background(), "anything", nil)
	if err == nil {
		t.Fatal("expected error for disconnected server")
	}
}

func TestWrappedClient_ToolsCached(t *testing.T) {
	srv := newTestServer(t)
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	wrapper := &Client{
		name:      "test-server",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}

	// First call
	tools1, err := wrapper.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Second call should return cached result
	tools2, err := wrapper.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if len(tools1) != len(tools2) {
		t.Errorf("cached tools count mismatch: %d vs %d", len(tools1), len(tools2))
	}
}

func TestBuildEnv(t *testing.T) {
	c := &Client{
		config: ServerConfig{
			Env: map[string]string{
				"NEW_VAR":  "new_value",
				"HOME":     "/override/home",
			},
		},
	}

	env := c.buildEnv()
	if len(env) == 0 {
		t.Fatal("expected non-empty env")
	}

	envMap := make(map[string]string)
	for _, e := range env {
		k, v, _ := splitEnv(e)
		envMap[k] = v
	}

	if envMap["NEW_VAR"] != "new_value" {
		t.Errorf("NEW_VAR = %q, want new_value", envMap["NEW_VAR"])
	}
	if envMap["HOME"] != "/override/home" {
		t.Errorf("HOME = %q, want /override/home", envMap["HOME"])
	}
}

func splitEnv(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func TestBuildEnv_NoConfig(t *testing.T) {
	c := &Client{config: ServerConfig{}}
	env := c.buildEnv()
	if env != nil {
		t.Errorf("expected nil env when no config env, got %d entries", len(env))
	}
}

func TestToolCallTimeout(t *testing.T) {
	// Default should be 60s
	timeout := toolCallTimeout(nil)
	if timeout != defaultToolCallTimeout {
		t.Errorf("default timeout = %v, want %v", timeout, defaultToolCallTimeout)
	}
}

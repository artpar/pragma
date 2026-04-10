package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	"github.com/mark3labs/mcp-go/server"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

func newTestAdapter(t *testing.T) (*MCPToolAdapter, func()) {
	t.Helper()

	echoTool := server.ServerTool{
		Tool: mcp.Tool{
			Name:        "echo",
			Description: "Echoes the input message back",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"message": map[string]any{
						"type": "string",
					},
				},
				Required: []string{"message"},
			},
			Annotations: mcp.ToolAnnotation{
				ReadOnlyHint: boolPtr(true),
			},
		},
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(req.GetString("message", "")), nil
		},
	}

	srv, err := mcptest.NewServer(t, echoTool)
	if err != nil {
		t.Fatalf("create test server: %v", err)
	}

	bus := observe.NewEventBus(64)

	wrapper := &Client{
		name:      "test-srv",
		config:    ServerConfig{Command: "test"},
		bus:       bus,
		mcpCli:    srv.Client(),
		connected: true,
	}

	info := ToolInfo{
		Name:        "echo",
		Description: "Echoes the input message back",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`),
		ReadOnly:    true,
	}

	adapter := NewMCPToolAdapter(wrapper, info)

	cleanup := func() {
		srv.Close()
		bus.Drain()
	}

	return adapter, cleanup
}

func TestMCPToolAdapter_ImplementsDescriptor(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	var _ tool.Descriptor = adapter
}

func TestMCPToolAdapter_Name(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	want := "mcp__test-srv__echo"
	if got := adapter.Name(); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

func TestMCPToolAdapter_Description(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	if got := adapter.Description(); got != "Echoes the input message back" {
		t.Errorf("Description() = %q", got)
	}
}

func TestMCPToolAdapter_InputSchema(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	schema := adapter.InputSchema()
	if len(schema) == 0 {
		t.Fatal("InputSchema() returned empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("InputSchema() invalid JSON: %v", err)
	}
	if parsed["type"] != "object" {
		t.Errorf("schema type = %v, want object", parsed["type"])
	}
}

func TestMCPToolAdapter_Flags(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	flags := adapter.Flags()
	if !flags.ReadOnly {
		t.Error("expected ReadOnly=true")
	}
	if !flags.Concurrent {
		t.Error("expected Concurrent=true (derived from ReadOnly)")
	}
	if flags.Destructive {
		t.Error("expected Destructive=false")
	}
}

func TestMCPToolAdapter_Invoke(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	input := json.RawMessage(`{"message": "hello from test"}`)
	result, err := adapter.Invoke(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if result.Content != "hello from test" {
		t.Errorf("Content = %q, want %q", result.Content, "hello from test")
	}
	if len(result.Supplements) != 0 {
		t.Errorf("Supplements = %v, want empty", result.Supplements)
	}
}

func TestMCPToolAdapter_CheckPerm(t *testing.T) {
	adapter, cleanup := newTestAdapter(t)
	defer cleanup()

	checker := &testChecker{decision: permission.DecisionAllow}
	input := json.RawMessage(`{"message": "test"}`)

	result := adapter.CheckPerm(context.Background(), input, checker)
	if result.Decision != permission.DecisionAllow {
		t.Errorf("Decision = %q, want allow", result.Decision)
	}

	// Verify the checker was called with the correct tool name
	if checker.lastTool != "mcp__test-srv__echo" {
		t.Errorf("checker tool = %q, want mcp__test-srv__echo", checker.lastTool)
	}
	if checker.lastContent != `{"message": "test"}` {
		t.Errorf("checker content = %q", checker.lastContent)
	}
}

func TestMCPToolAdapter_DescriptionTruncation(t *testing.T) {
	bus := observe.NewEventBus(64)
	defer bus.Drain()

	// Create a long description
	longDesc := make([]byte, maxDescriptionLen+100)
	for i := range longDesc {
		longDesc[i] = 'a'
	}

	adapter := &MCPToolAdapter{
		toolInfo: ToolInfo{
			Name:        "long",
			Description: string(longDesc),
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		fullName: "mcp__srv__long",
	}

	desc := adapter.Description()
	if len(desc) != maxDescriptionLen {
		t.Errorf("description length = %d, want %d", len(desc), maxDescriptionLen)
	}
}

// testChecker is a simple permission.Checker for testing.
type testChecker struct {
	decision    permission.Decision
	lastTool    string
	lastContent string
}

func (c *testChecker) Check(_ context.Context, toolName string, content string) permission.CheckResult {
	c.lastTool = toolName
	c.lastContent = content
	return permission.CheckResult{
		Decision: c.decision,
		Content:  content,
	}
}

func (c *testChecker) AddSessionRule(_ permission.Rule) {}

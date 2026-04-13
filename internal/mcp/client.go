package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/artpar/gogent/internal/buildinfo"
	"github.com/artpar/gogent/internal/observe"
)

const (
	defaultConnectTimeout  = 30 * time.Second
	defaultToolCallTimeout = 60 * time.Second
)

// ToolInfo is the parsed tool definition from an MCP server.
type ToolInfo struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
	Destructive bool
}

// Client wraps a single mcp-go client connection to one MCP server.
type Client struct {
	name   string
	config ServerConfig
	bus    *observe.EventBus

	mu        sync.Mutex
	mcpCli    *mcpclient.Client
	connected bool
	tools     []ToolInfo
}

// NewClient creates a Client (does not connect yet).
func NewClient(name string, cfg ServerConfig, bus *observe.EventBus) *Client {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Client{\n\tname:\tname,\n\tconfig:\tcfg,\n\tbus:\tbus,\n}")
	return &Client{
		name:   name,
		config: cfg,
		bus:    bus,
	}
}

// Name returns the server name.
func (c *Client) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.name")
	return c.name
}

// Connected returns true if the client is connected.
func (c *Client) Connected() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	defer c.mu.Unlock()
	observe.GlobalTrace("return: c.connected")
	return c.connected
}

// Connect establishes the MCP connection with a startup timeout.
func (c *Client) Connect(ctx context.Context) error {
	observe.TraceCtx(ctx, "mcp", "Client.Connect", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.Connect", "exit")
	c.mu.Lock()
	defer c.mu.Unlock()

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	c.bus.Emit(observe.MCPServerConnecting{
		EventHeader: observe.NewEventHeader("MCPServerConnecting", traceID, spanID, ""),
		ServerName:  c.name,
		Transport:   c.config.effectiveType(),
	})

	start := time.Now()

	connectCtx, cancel := context.WithTimeout(ctx, defaultConnectTimeout)
	defer cancel()

	cli, err := c.createMCPClient()
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.Connect", "if: err != nil")
		c.bus.Emit(observe.MCPServerFailed{
			EventHeader:  observe.NewEventHeader("MCPServerFailed", traceID, spanID, ""),
			ServerName:   c.name,
			ErrorType:    "transport_create",
			ErrorMessage: err.Error(),
		})
		observe.TraceCtx(ctx, "mcp", "Client.Connect", "return: fmt.Errorf(\"create transport for %q: %w\", c.name, err)")
		return fmt.Errorf("create transport for %q: %w", c.name, err)
	}

	if c.config.effectiveType() != "stdio" {
		observe.TraceCtx(ctx, "mcp", "Client.Connect", "if: c.config.effectiveType() != \"stdio\"")
		if err := cli.Start(connectCtx); err != nil {
			observe.TraceCtx(ctx, "mcp", "Client.Connect", "if: err != nil")
			_ = cli.Close()
			c.bus.Emit(observe.MCPServerFailed{
				EventHeader:  observe.NewEventHeader("MCPServerFailed", traceID, spanID, ""),
				ServerName:   c.name,
				ErrorType:    "transport_start",
				ErrorMessage: err.Error(),
			})
			observe.TraceCtx(ctx, "mcp", "Client.Connect", "return: fmt.Errorf(\"start transport for %q: %w\", c.name, err)")
			return fmt.Errorf("start transport for %q: %w", c.name, err)
		}
	}

	_, err = cli.Initialize(connectCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "gogent",
				Version: buildinfo.Version,
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	})
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.Connect", "if: err != nil")
		_ = cli.Close()
		c.bus.Emit(observe.MCPServerFailed{
			EventHeader:  observe.NewEventHeader("MCPServerFailed", traceID, spanID, ""),
			ServerName:   c.name,
			ErrorType:    "initialize",
			ErrorMessage: err.Error(),
		})
		observe.TraceCtx(ctx, "mcp", "Client.Connect", "return: fmt.Errorf(\"initialize %q: %w\", c.name, err)")
		return fmt.Errorf("initialize %q: %w", c.name, err)
	}

	c.mcpCli = cli
	c.connected = true
	c.tools = nil

	tools, toolErr := c.listToolsLocked(connectCtx)
	toolCount := 0
	if toolErr == nil {
		observe.TraceCtx(ctx, "mcp", "Client.Connect", "if: toolErr == nil")
		toolCount = len(tools)
	}

	c.bus.Emit(observe.MCPServerConnected{
		EventHeader: observe.NewEventHeader("MCPServerConnected", traceID, spanID, ""),
		ServerName:  c.name,
		ToolCount:   toolCount,
		DurationMs:  time.Since(start).Milliseconds(),
	})
	observe.TraceCtx(ctx, "mcp", "Client.Connect", "return: nil")

	return nil
}

// createMCPClient creates the underlying mcp-go Client based on transport type.
func (c *Client) createMCPClient() (*mcpclient.Client, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch c.config.effectiveType() {
	case "stdio":
		observe.GlobalTrace("case: \"stdio\"")
		env := c.buildEnv()
		return mcpclient.NewStdioMCPClient(c.config.Command, env, c.config.Args...)

	case "sse":
		observe.GlobalTrace("case: \"sse\"")
		opts := make([]transport.ClientOption, 0, 1)
		if len(c.config.Headers) > 0 {
			opts = append(opts, transport.WithHeaders(c.config.Headers))
		}
		return mcpclient.NewSSEMCPClient(c.config.URL, opts...)

	case "http":
		observe.GlobalTrace("case: \"http\"")
		opts := make([]transport.StreamableHTTPCOption, 0, 1)
		if len(c.config.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(c.config.Headers))
		}
		return mcpclient.NewStreamableHttpClient(c.config.URL, opts...)

	default:
		observe.GlobalTrace("default")
		return nil, fmt.Errorf("unsupported transport: %s", c.config.effectiveType())
	}
}

// buildEnv merges config Env into the current process environment.
// Config entries override existing env vars with the same key.
func (c *Client) buildEnv() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(c.config.Env) == 0 {
		observe.GlobalTrace("if: len(c.config.Env) == 0")
		observe.GlobalTrace("return: nil")
		return nil
	}

	current := os.Environ()
	override := make(map[string]string, len(c.config.Env))
	for k, v := range c.config.Env {
		observe.GlobalTrace("range c.config.Env")
		override[k] = v
	}

	result := make([]string, 0, len(current)+len(override))
	seen := make(map[string]bool, len(current))

	for _, entry := range current {
		observe.GlobalTrace("range current")
		key, _, _ := strings.Cut(entry, "=")
		if v, ok := override[key]; ok {
			observe.GlobalTrace("if: ok")
			result = append(result, key+"="+v)
			seen[key] = true
		} else {
			observe.GlobalTrace("else: ok")
			result = append(result, entry)
			seen[key] = true
		}
	}

	for k, v := range override {
		observe.GlobalTrace("range override")
		if !seen[k] {
			observe.GlobalTrace("if: !seen[k]")
			result = append(result, k+"="+v)
		}
	}
	observe.GlobalTrace("return: result")

	return result
}

// ListTools returns the tools offered by this server (cached after first call).
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	observe.TraceCtx(ctx, "mcp", "Client.ListTools", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.ListTools", "exit")
	c.mu.Lock()
	defer c.mu.Unlock()
	observe.TraceCtx(ctx, "mcp", "Client.ListTools", "return: c.listToolsLocked(ctx)")
	return c.listToolsLocked(ctx)
}

func (c *Client) listToolsLocked(ctx context.Context) ([]ToolInfo, error) {
	observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "exit")
	if !c.connected || c.mcpCli == nil {
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: !c.connected || c.mcpCli == nil")
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		return nil, fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}

	if c.tools != nil {
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: c.tools != nil")
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "return: c.tools, nil")
		return c.tools, nil
	}

	result, err := c.mcpCli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: err != nil")
		c.tools = nil
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "return: nil, fmt.Errorf(\"list tools from %q: %w\", c.name, err)")
		return nil, fmt.Errorf("list tools from %q: %w", c.name, err)
	}

	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, t := range result.Tools {
		observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "range result.Tools")
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
			observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: err != nil")
			schema = []byte(`{"type":"object"}`)
		}

		if len(t.RawInputSchema) > 0 {
			observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: len(t.RawInputSchema) > 0")
			schema = t.RawInputSchema
		}

		info := ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		}

		if t.Annotations.ReadOnlyHint != nil {
			observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: t.Annotations.ReadOnlyHint != nil")
			info.ReadOnly = *t.Annotations.ReadOnlyHint
		}
		if t.Annotations.DestructiveHint != nil {
			observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "if: t.Annotations.DestructiveHint != nil")
			info.Destructive = *t.Annotations.DestructiveHint
		}

		tools = append(tools, info)
	}

	c.tools = tools
	observe.TraceCtx(ctx, "mcp", "Client.listToolsLocked", "return: tools, nil")
	return tools, nil
}

// CallTool invokes a tool on the MCP server with a deadline.
func (c *Client) CallTool(ctx context.Context, toolName string, args json.RawMessage) (string, error) {
	observe.TraceCtx(ctx, "mcp", "Client.CallTool", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.CallTool", "exit")
	c.mu.Lock()
	if !c.connected || c.mcpCli == nil {
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "if: !c.connected || c.mcpCli == nil")
		c.mu.Unlock()
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "return: \"\", fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		return "", fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}
	cli := c.mcpCli
	c.mu.Unlock()

	traceID := observe.NewTraceID()
	spanID := observe.NewSpanID()

	c.bus.Emit(observe.MCPToolCallStarted{
		EventHeader:    observe.NewEventHeader("MCPToolCallStarted", traceID, spanID, ""),
		ServerName:     c.name,
		ToolName:       toolName,
		InputSizeBytes: len(args),
	})

	start := time.Now()

	timeout := toolCallTimeout(c.bus)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Convert json.RawMessage to map[string]any for mcp-go
	var arguments map[string]any
	if len(args) > 0 {
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "if: len(args) > 0")
		if err := json.Unmarshal(args, &arguments); err != nil {
			observe.TraceCtx(ctx, "mcp", "Client.CallTool", "if: err != nil")
			observe.TraceCtx(ctx, "mcp", "Client.CallTool", "return: \"\", fmt.Errorf(\"parse tool arguments: %w\", err)")
			return "", fmt.Errorf("parse tool arguments: %w", err)
		}
	}

	result, err := cli.CallTool(callCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: arguments,
		},
	})
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "if: err != nil")
		if callCtx.Err() != nil && ctx.Err() == nil {
			observe.TraceCtx(ctx, "mcp", "Client.CallTool", "if: callCtx.Err() != nil && ctx.Err() == nil")
			observe.TraceCtx(ctx, "mcp", "Client.CallTool", "return: \"\", fmt.Errorf(\"%w: %s/%s after %s\", ErrToolCallTimeout, c.name, toolName, ti...")

			return "", fmt.Errorf("%w: %s/%s after %s", ErrToolCallTimeout, c.name, toolName, timeout)
		}
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "return: \"\", fmt.Errorf(\"%w: %s/%s: %v\", ErrToolCallFailed, c.name, toolName, err)")
		return "", fmt.Errorf("%w: %s/%s: %v", ErrToolCallFailed, c.name, toolName, err)
	}

	output := extractTextContent(result)

	if result.IsError {
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "if: result.IsError")
		c.bus.Emit(observe.MCPToolCallCompleted{
			EventHeader:     observe.NewEventHeader("MCPToolCallCompleted", traceID, spanID, ""),
			ServerName:      c.name,
			ToolName:        toolName,
			DurationMs:      time.Since(start).Milliseconds(),
			OutputSizeBytes: len(output),
		})
		observe.TraceCtx(ctx, "mcp", "Client.CallTool", "return: \"\", fmt.Errorf(\"%w: %s/%s: %s\", ErrToolCallFailed, c.name, toolName, output)")
		return "", fmt.Errorf("%w: %s/%s: %s", ErrToolCallFailed, c.name, toolName, output)
	}

	c.bus.Emit(observe.MCPToolCallCompleted{
		EventHeader:     observe.NewEventHeader("MCPToolCallCompleted", traceID, spanID, ""),
		ServerName:      c.name,
		ToolName:        toolName,
		DurationMs:      time.Since(start).Milliseconds(),
		OutputSizeBytes: len(output),
	})
	observe.TraceCtx(ctx, "mcp", "Client.CallTool", "return: output, nil")

	return output, nil
}

// Disconnect closes the connection and cleans up resources.
func (c *Client) Disconnect() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.mcpCli == nil {
		observe.GlobalTrace("if: !c.connected || c.mcpCli == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}

	traceID := observe.NewTraceID()

	err := c.mcpCli.Close()
	c.connected = false
	c.mcpCli = nil
	c.tools = nil

	c.bus.Emit(observe.MCPServerDisconnected{
		EventHeader: observe.NewEventHeader("MCPServerDisconnected", traceID, observe.NewSpanID(), ""),
		ServerName:  c.name,
		Reason:      "client_disconnect",
	})
	observe.GlobalTrace("return: err")

	return err
}

// Reconnect disconnects then connects again. Clears tool cache.
func (c *Client) Reconnect(ctx context.Context) error {
	observe.TraceCtx(ctx, "mcp", "Client.Reconnect", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.Reconnect", "exit")
	if err := c.Disconnect(); err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.Reconnect", "warn: Disconnect failed: "+err.Error())
	}
	observe.TraceCtx(ctx, "mcp", "Client.Reconnect", "return: c.Connect(ctx)")
	return c.Connect(ctx)
}

// toolCallTimeout returns the tool call timeout from MCP_TIMEOUT env or default.
// Emits a warning via bus if MCP_TIMEOUT is set but unparseable (GitHub #7575, #16837).
func toolCallTimeout(bus *observe.EventBus) time.Duration {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if v := os.Getenv("MCP_TIMEOUT"); v != "" {
		observe.GlobalTrace("if: v != \"\"")
		if ms, err := time.ParseDuration(v); err == nil {
			observe.GlobalTrace("if: err == nil")
			observe.GlobalTrace("return: ms")
			return ms
		}
		// Try parsing as milliseconds (common in TS world)
		var ms int64
		if _, err := fmt.Sscanf(v, "%d", &ms); err == nil && ms > 0 {
			observe.GlobalTrace("if: err == nil && ms > 0")
			observe.GlobalTrace("return: time.Duration(ms) * time.Millisecond")
			return time.Duration(ms) * time.Millisecond
		}

		if bus != nil {
			observe.GlobalTrace("if: bus != nil")
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warning",
				Component:    "mcp/client",
				ErrorType:    "invalid_timeout",
				ErrorMessage: fmt.Sprintf("MCP_TIMEOUT=%q is not a valid duration or millisecond value; using default %s", v, defaultToolCallTimeout),
			})
		}
	}
	observe.GlobalTrace("return: defaultToolCallTimeout")
	return defaultToolCallTimeout
}

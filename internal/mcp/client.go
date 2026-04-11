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

	"github.com/artpar/gogent/internal/observe"
)

// Version is the gogent version reported to MCP servers during initialization.
// Override at build time via -ldflags "-X github.com/artpar/gogent/internal/mcp.Version=x.y.z".
var Version = "0.1.0"

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
	return &Client{
		name:   name,
		config: cfg,
		bus:    bus,
	}
}

// Name returns the server name.
func (c *Client) Name() string { return c.name }

// Connected returns true if the client is connected.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// Connect establishes the MCP connection with a startup timeout.
func (c *Client) Connect(ctx context.Context) error {
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
		c.bus.Emit(observe.MCPServerFailed{
			EventHeader:  observe.NewEventHeader("MCPServerFailed", traceID, spanID, ""),
			ServerName:   c.name,
			ErrorType:    "transport_create",
			ErrorMessage: err.Error(),
		})
		return fmt.Errorf("create transport for %q: %w", c.name, err)
	}

	// For SSE/HTTP transports, Start must be called explicitly.
	// Stdio transport auto-starts in NewStdioMCPClient.
	if c.config.effectiveType() != "stdio" {
		if err := cli.Start(connectCtx); err != nil {
			_ = cli.Close()
			c.bus.Emit(observe.MCPServerFailed{
				EventHeader:  observe.NewEventHeader("MCPServerFailed", traceID, spanID, ""),
				ServerName:   c.name,
				ErrorType:    "transport_start",
				ErrorMessage: err.Error(),
			})
			return fmt.Errorf("start transport for %q: %w", c.name, err)
		}
	}

	// Initialize MCP handshake
	_, err = cli.Initialize(connectCtx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "gogent",
				Version: Version,
			},
			Capabilities: mcp.ClientCapabilities{},
		},
	})
	if err != nil {
		_ = cli.Close()
		c.bus.Emit(observe.MCPServerFailed{
			EventHeader:  observe.NewEventHeader("MCPServerFailed", traceID, spanID, ""),
			ServerName:   c.name,
			ErrorType:    "initialize",
			ErrorMessage: err.Error(),
		})
		return fmt.Errorf("initialize %q: %w", c.name, err)
	}

	c.mcpCli = cli
	c.connected = true
	c.tools = nil // clear cached tools

	// List tools immediately to populate cache and report count
	tools, toolErr := c.listToolsLocked(connectCtx)
	toolCount := 0
	if toolErr == nil {
		toolCount = len(tools)
	}

	c.bus.Emit(observe.MCPServerConnected{
		EventHeader: observe.NewEventHeader("MCPServerConnected", traceID, spanID, ""),
		ServerName:  c.name,
		ToolCount:   toolCount,
		DurationMs:  time.Since(start).Milliseconds(),
	})

	return nil
}

// createMCPClient creates the underlying mcp-go Client based on transport type.
func (c *Client) createMCPClient() (*mcpclient.Client, error) {
	switch c.config.effectiveType() {
	case "stdio":
		env := c.buildEnv()
		return mcpclient.NewStdioMCPClient(c.config.Command, env, c.config.Args...)

	case "sse":
		opts := make([]transport.ClientOption, 0, 1)
		if len(c.config.Headers) > 0 {
			opts = append(opts, transport.WithHeaders(c.config.Headers))
		}
		return mcpclient.NewSSEMCPClient(c.config.URL, opts...)

	case "http":
		opts := make([]transport.StreamableHTTPCOption, 0, 1)
		if len(c.config.Headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(c.config.Headers))
		}
		return mcpclient.NewStreamableHttpClient(c.config.URL, opts...)

	default:
		return nil, fmt.Errorf("unsupported transport: %s", c.config.effectiveType())
	}
}

// buildEnv merges config Env into the current process environment.
// Config entries override existing env vars with the same key.
func (c *Client) buildEnv() []string {
	if len(c.config.Env) == 0 {
		return nil // inherit current process env (default behavior)
	}

	// Start with current environment
	current := os.Environ()
	override := make(map[string]string, len(c.config.Env))
	for k, v := range c.config.Env {
		override[k] = v
	}

	result := make([]string, 0, len(current)+len(override))
	seen := make(map[string]bool, len(current))

	for _, entry := range current {
		key, _, _ := strings.Cut(entry, "=")
		if v, ok := override[key]; ok {
			result = append(result, key+"="+v)
			seen[key] = true
		} else {
			result = append(result, entry)
			seen[key] = true
		}
	}

	// Add any new env vars from config that weren't in current env
	for k, v := range override {
		if !seen[k] {
			result = append(result, k+"="+v)
		}
	}

	return result
}

// ListTools returns the tools offered by this server (cached after first call).
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listToolsLocked(ctx)
}

func (c *Client) listToolsLocked(ctx context.Context) ([]ToolInfo, error) {
	if !c.connected || c.mcpCli == nil {
		return nil, fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}

	if c.tools != nil {
		return c.tools, nil
	}

	result, err := c.mcpCli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		c.tools = nil // defense in depth: ensure stale cache is cleared on failure
		return nil, fmt.Errorf("list tools from %q: %w", c.name, err)
	}

	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, t := range result.Tools {
		schema, err := json.Marshal(t.InputSchema)
		if err != nil {
			schema = []byte(`{"type":"object"}`)
		}
		// Use RawInputSchema if available (arbitrary JSON Schema)
		if len(t.RawInputSchema) > 0 {
			schema = t.RawInputSchema
		}

		info := ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		}

		if t.Annotations.ReadOnlyHint != nil {
			info.ReadOnly = *t.Annotations.ReadOnlyHint
		}
		if t.Annotations.DestructiveHint != nil {
			info.Destructive = *t.Annotations.DestructiveHint
		}

		tools = append(tools, info)
	}

	c.tools = tools
	return tools, nil
}

// CallTool invokes a tool on the MCP server with a deadline.
func (c *Client) CallTool(ctx context.Context, toolName string, args json.RawMessage) (string, error) {
	c.mu.Lock()
	if !c.connected || c.mcpCli == nil {
		c.mu.Unlock()
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

	// Apply tool call timeout
	timeout := toolCallTimeout(c.bus)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Convert json.RawMessage to map[string]any for mcp-go
	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
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
		if callCtx.Err() != nil && ctx.Err() == nil {
			// Inner timeout fired but parent context still alive = tool call timeout
			return "", fmt.Errorf("%w: %s/%s after %s", ErrToolCallTimeout, c.name, toolName, timeout)
		}
		return "", fmt.Errorf("%w: %s/%s: %v", ErrToolCallFailed, c.name, toolName, err)
	}

	// Extract text from result content
	output := extractTextContent(result)

	if result.IsError {
		c.bus.Emit(observe.MCPToolCallCompleted{
			EventHeader:     observe.NewEventHeader("MCPToolCallCompleted", traceID, spanID, ""),
			ServerName:      c.name,
			ToolName:        toolName,
			DurationMs:      time.Since(start).Milliseconds(),
			OutputSizeBytes: len(output),
		})
		return "", fmt.Errorf("%w: %s/%s: %s", ErrToolCallFailed, c.name, toolName, output)
	}

	c.bus.Emit(observe.MCPToolCallCompleted{
		EventHeader:     observe.NewEventHeader("MCPToolCallCompleted", traceID, spanID, ""),
		ServerName:      c.name,
		ToolName:        toolName,
		DurationMs:      time.Since(start).Milliseconds(),
		OutputSizeBytes: len(output),
	})

	return output, nil
}

// extractTextContent concatenates text from CallToolResult content blocks.
func extractTextContent(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}

	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			parts = append(parts, c.Text)
		case *mcp.TextContent:
			parts = append(parts, c.Text)
		default:
			// For non-text content (images, resources), serialize as JSON
			data, err := json.Marshal(c)
			if err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// ResourceInfo describes an MCP resource exposed by a server.
type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	MimeType    string `json:"mimeType,omitempty"`
	Description string `json:"description,omitempty"`
}

// ResourceContent holds one piece of content returned by ReadResource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64-encoded binary
}

// ListResources returns the resources offered by this server.
func (c *Client) ListResources(ctx context.Context) ([]ResourceInfo, error) {
	c.mu.Lock()
	if !c.connected || c.mcpCli == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}
	cli := c.mcpCli
	c.mu.Unlock()

	result, err := cli.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("list resources from %q: %w", c.name, err)
	}

	resources := make([]ResourceInfo, 0, len(result.Resources))
	for _, r := range result.Resources {
		resources = append(resources, ResourceInfo{
			URI:         r.URI,
			Name:        r.Name,
			MimeType:    r.MIMEType,
			Description: r.Description,
		})
	}
	return resources, nil
}

// ReadResource reads a resource by URI from this server.
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	c.mu.Lock()
	if !c.connected || c.mcpCli == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}
	cli := c.mcpCli
	c.mu.Unlock()

	result, err := cli.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		return nil, fmt.Errorf("read resource %q from %q: %w", uri, c.name, err)
	}

	contents := make([]ResourceContent, 0, len(result.Contents))
	for _, content := range result.Contents {
		switch c := content.(type) {
		case mcp.TextResourceContents:
			contents = append(contents, ResourceContent{
				URI:      c.URI,
				MimeType: c.MIMEType,
				Text:     c.Text,
			})
		case mcp.BlobResourceContents:
			contents = append(contents, ResourceContent{
				URI:      c.URI,
				MimeType: c.MIMEType,
				Blob:     c.Blob,
			})
		}
	}
	return contents, nil
}

// Disconnect closes the connection and cleans up resources.
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.mcpCli == nil {
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

	return err
}

// Reconnect disconnects then connects again. Clears tool cache.
func (c *Client) Reconnect(ctx context.Context) error {
	_ = c.Disconnect()
	return c.Connect(ctx)
}

// toolCallTimeout returns the tool call timeout from MCP_TIMEOUT env or default.
// Emits a warning via bus if MCP_TIMEOUT is set but unparseable (GitHub #7575, #16837).
func toolCallTimeout(bus *observe.EventBus) time.Duration {
	if v := os.Getenv("MCP_TIMEOUT"); v != "" {
		if ms, err := time.ParseDuration(v); err == nil {
			return ms
		}
		// Try parsing as milliseconds (common in TS world)
		var ms int64
		if _, err := fmt.Sscanf(v, "%d", &ms); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
		// Both parse attempts failed — warn the user
		if bus != nil {
			bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warning",
				Component:    "mcp/client",
				ErrorType:    "invalid_timeout",
				ErrorMessage: fmt.Sprintf("MCP_TIMEOUT=%q is not a valid duration or millisecond value; using default %s", v, defaultToolCallTimeout),
			})
		}
	}
	return defaultToolCallTimeout
}

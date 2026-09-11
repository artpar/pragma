package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// maxToolDescriptionLen caps an MCP tool's description when it is exposed to
// the model. Ported from the worktree branch's MCPToolAdapter
// (maxDescriptionLen).
const maxToolDescriptionLen = 2048

// defaultToolSchema is used when a server advertises a tool without a JSON
// input schema.
const defaultToolSchema = `{"type":"object"}`

// ToolDef converts one advertised server tool into a model.ToolDef named
// "mcp__<server>__<tool>" (BuildToolName). This is the model-facing half of
// the MCPToolAdapter port: after conversion the tool is indistinguishable
// from a built-in tool definition for request-building purposes.
func ToolDef(clientName string, info ToolInfo) model.ToolDef {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	description := info.Description
	if len(description) > maxToolDescriptionLen {
		observe.GlobalTrace("if: len(description) > maxToolDescriptionLen")
		description = description[:maxToolDescriptionLen]
	}
	schema := info.InputSchema
	if len(schema) == 0 {
		observe.GlobalTrace("if: len(schema) == 0")
		schema = json.RawMessage(defaultToolSchema)
	}
	observe.GlobalTrace("return: model.ToolDef{\n\tName:\t\tBuildToolName(clientName, info.Name),\n\tDescription:\tde...")
	return model.ToolDef{
		Name:        BuildToolName(clientName, info.Name),
		Description: description,
		InputSchema: schema,
	}
}

// ToolDefs lists the client's advertised tools and converts them into model
// tool definitions. Tools are cached by ListTools after the first call.
func (c *Client) ToolDefs(ctx context.Context) ([]model.ToolDef, error) {
	observe.TraceCtx(ctx, "mcp", "Client.ToolDefs", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.ToolDefs", "exit")
	tools, err := c.ListTools(ctx)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.ToolDefs", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "Client.ToolDefs", "return: nil, err")
		return nil, err
	}
	defs := make([]model.ToolDef, 0, len(tools))
	for _, info := range tools {
		observe.TraceCtx(ctx, "mcp", "Client.ToolDefs", "range tools")
		defs = append(defs, ToolDef(c.Name(), info))
	}
	observe.TraceCtx(ctx, "mcp", "Client.ToolDefs", "return: defs, nil")
	return defs, nil
}

// ToolDefs aggregates tool definitions from all connected MCP servers, in
// deterministic server-name order. A server whose tools cannot be listed is
// skipped with a warn event (the branch's Manager.RegisterTools behavior);
// its connection status still reaches the model through the system prompt.
// Duplicate tool names are skipped (first definition wins).
func (m *Manager) ToolDefs(ctx context.Context) []model.ToolDef {
	observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "exit")
	clients := m.Clients()
	names := make([]string, 0, len(clients))
	for name := range clients {
		observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "range clients")
		names = append(names, name)
	}
	sort.Strings(names)

	seen := make(map[string]bool)
	var defs []model.ToolDef
	for _, name := range names {
		observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "range names")
		client := clients[name]
		if !client.Connected() {
			observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "if: !client.Connected()")
			continue
		}
		serverDefs, err := client.ToolDefs(ctx)
		if err != nil {
			observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "if: err != nil")
			m.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
				Severity:     "warn",
				Component:    "mcp",
				ErrorType:    "list_tools_error",
				ErrorMessage: fmt.Sprintf("failed to list tools from %q: %v", client.Name(), err),
			})
			continue
		}
		for _, def := range serverDefs {
			observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "range serverDefs")
			if seen[def.Name] {
				observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "if: seen[def.Name]")
				continue
			}
			seen[def.Name] = true
			defs = append(defs, def)
		}
	}
	observe.TraceCtx(ctx, "mcp", "Manager.ToolDefs", "return: defs")
	return defs
}

// CallMCPTool routes an "mcp__<server>__<tool>" invocation back to the owning
// server. The server is resolved by normalized name; the original
// (pre-normalization) tool name is recovered from the server's advertised
// list. On a lost connection, one reconnect is attempted before failing
// (the branch's MCPToolAdapter.Invoke retry).
func (m *Manager) CallMCPTool(ctx context.Context, fullName string, input json.RawMessage) (string, error) {
	observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "exit")
	serverName, _, ok := ParseToolName(fullName)
	if !ok {
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "if: !ok")
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "return: \"\", fmt.Errorf(\"invalid MCP tool name %q\", fullName)")
		return "", fmt.Errorf("invalid MCP tool name %q", fullName)
	}
	client := m.clientByNormalizedName(serverName)
	if client == nil {
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "if: client == nil")
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "return: \"\", fmt.Errorf(\"no connected MCP server %q for tool %q\", serverName, fullName)")
		return "", fmt.Errorf("no connected MCP server %q for tool %q", serverName, fullName)
	}
	toolName, err := m.originalToolName(ctx, client, fullName)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "return: \"\", err")
		return "", err
	}
	output, err := client.CallTool(ctx, toolName, input)
	if err != nil && errors.Is(err, ErrServerNotConnected) {
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "if: err != nil && errors.Is(err, ErrServerNotConnected)")
		if reconnErr := client.Reconnect(ctx); reconnErr != nil {
			observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "if: reconnErr != nil")
			observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "return: \"\", fmt.Errorf(\"reconnect failed for %s: %w\", fullName, reconnErr)")
			return "", fmt.Errorf("reconnect failed for %s: %w", fullName, reconnErr)
		}
		output, err = client.CallTool(ctx, toolName, input)
	}
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "return: \"\", err")
		return "", err
	}
	observe.TraceCtx(ctx, "mcp", "Manager.CallMCPTool", "return: output, nil")
	return output, nil
}

// clientByNormalizedName resolves a parsed (normalized) server name to its
// client. Server config names are compared through NormalizeName so a name
// with normalizable characters still routes.
func (m *Manager) clientByNormalizedName(normalized string) *Client {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for name, client := range m.Clients() {
		observe.GlobalTrace("range m.Clients()")
		if NormalizeName(name) == normalized {
			observe.GlobalTrace("if: NormalizeName(name) == normalized")
			observe.GlobalTrace("return: client")
			return client
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// originalToolName recovers the advertised tool name for a fully-qualified
// name, undoing BuildToolName's normalization.
func (m *Manager) originalToolName(ctx context.Context, client *Client, fullName string) (string, error) {
	observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "exit")
	tools, err := client.ListTools(ctx)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "return: \"\", err")
		return "", err
	}
	for _, info := range tools {
		observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "range tools")
		if BuildToolName(client.Name(), info.Name) == fullName {
			observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "if: BuildToolName(...) == fullName")
			observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "return: info.Name, nil")
			return info.Name, nil
		}
	}
	observe.TraceCtx(ctx, "mcp", "Manager.originalToolName", "return: \"\", fmt.Errorf(\"unknown MCP tool %q on server %q\", fullName, client.Name())")
	return "", fmt.Errorf("unknown MCP tool %q on server %q", fullName, client.Name())
}

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

const maxDescriptionLen = 2048

// MCPToolAdapter wraps an MCP server tool as a tool.Descriptor.
// After registration, it is indistinguishable from a built-in tool (ADR-005).
type MCPToolAdapter struct {
	client   *Client
	toolInfo ToolInfo
	fullName string
}

// NewMCPToolAdapter creates an adapter. fullName is "mcp__<server>__<tool>".
func NewMCPToolAdapter(client *Client, info ToolInfo) *MCPToolAdapter {
	return &MCPToolAdapter{
		client:   client,
		toolInfo: info,
		fullName: BuildToolName(client.Name(), info.Name),
	}
}

// Name returns the fully-qualified tool name (mcp__<server>__<tool>).
func (a *MCPToolAdapter) Name() string { return a.fullName }

// Description returns the MCP server's tool description, capped at 2048 chars.
func (a *MCPToolAdapter) Description() string {
	d := a.toolInfo.Description
	if len(d) > maxDescriptionLen {
		return d[:maxDescriptionLen]
	}
	return d
}

// InputSchema returns the MCP server's JSON Schema for the tool.
func (a *MCPToolAdapter) InputSchema() json.RawMessage {
	if len(a.toolInfo.InputSchema) == 0 {
		return json.RawMessage(`{"type":"object"}`)
	}
	return a.toolInfo.InputSchema
}

// Invoke calls the MCP server tool. Handles disconnection with 1 reconnect retry.
func (a *MCPToolAdapter) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	result, err := a.client.CallTool(ctx, a.toolInfo.Name, input)
	if err != nil {
		if errors.Is(err, ErrServerNotConnected) {
			// Server went away between calls — attempt reconnect + retry once
			if reconnErr := a.client.Reconnect(ctx); reconnErr != nil {
				return tool.InvokeResult{}, fmt.Errorf("reconnect failed for %s: %w", a.fullName, reconnErr)
			}
			result, err = a.client.CallTool(ctx, a.toolInfo.Name, input)
			if err != nil {
				return tool.InvokeResult{}, err
			}
		} else {
			return tool.InvokeResult{}, err
		}
	}

	return tool.InvokeResult{Content: result}, nil
}

// CheckPerm extracts a permission-checkable string from the input.
// The content is the stringified JSON args — permission rules match against this.
func (a *MCPToolAdapter) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	content := string(input)
	return checker.Check(ctx, a.fullName, content)
}

// Flags returns tool flags derived from MCP annotations.
func (a *MCPToolAdapter) Flags() tool.ToolFlags {
	return tool.ToolFlags{
		ReadOnly:    a.toolInfo.ReadOnly,
		Concurrent:  a.toolInfo.ReadOnly, // read-only tools are safe to run concurrently
		Destructive: a.toolInfo.Destructive,
	}
}

// Compile-time check that MCPToolAdapter implements tool.Descriptor.
var _ tool.Descriptor = (*MCPToolAdapter)(nil)

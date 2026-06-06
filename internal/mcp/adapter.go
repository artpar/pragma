package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

const maxDescriptionLen = 2048

// MCPToolAdapter wraps an MCP server tool as a tool.Descriptor.
// After registration, it is indistinguishable from a built-in tool (ADR-005).
type MCPToolAdapter struct {
	client    *Client
	toolInfo  ToolInfo
	fullName  string
	server    string
	reconnect func(context.Context, string, string) (*Client, error)
}

// NewMCPToolAdapter creates an adapter. By default fullName is
// "mcp__<server>__<tool>". Trusted reflective servers may opt into preserving
// the remote MCP tool name exactly.
func NewMCPToolAdapter(client *Client, info ToolInfo) *MCPToolAdapter {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fullName := BuildToolName(client.Name(), info.Name)
	if client.config.PreserveToolNames {
		observe.GlobalTrace("if: client.config.PreserveToolNames")
		fullName = info.Name
	}
	observe.GlobalTrace("return: &MCPToolAdapter{\n\tclient:\t\tclient,\n\ttoolInfo:\tinfo,\n\tfullName:\tBuildToolName(...")
	observe.GlobalTrace("return: &MCPToolAdapter{\n\tclient:\t\tclient,\n\ttoolInfo:\tinfo,\n\tfullName:\tfullName,\n}")
	return &MCPToolAdapter{
		client:   client,
		toolInfo: info,
		fullName: fullName,
		server:   client.Name(),
	}
}

// Name returns the registry-facing tool name.
func (a *MCPToolAdapter) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: a.fullName")
	return a.fullName
}

// Description returns the MCP server's tool description, capped at 2048 chars.
func (a *MCPToolAdapter) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d := a.toolInfo.Description
	if len(d) > maxDescriptionLen {
		observe.GlobalTrace("if: len(d) > maxDescriptionLen")
		observe.GlobalTrace("return: d[:maxDescriptionLen]")
		return d[:maxDescriptionLen]
	}
	observe.GlobalTrace("return: d")
	return d
}

// InputSchema returns the MCP server's JSON Schema for the tool.
func (a *MCPToolAdapter) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(a.toolInfo.InputSchema) == 0 {
		observe.GlobalTrace("if: len(a.toolInfo.InputSchema) == 0")
		observe.GlobalTrace("return: json.RawMessage(`{\"type\":\"object\"}`)")
		return json.RawMessage(`{"type":"object"}`)
	}
	observe.GlobalTrace("return: a.toolInfo.InputSchema")
	return a.toolInfo.InputSchema
}

// Invoke calls the MCP server tool. Handles disconnection with 1 reconnect retry.
func (a *MCPToolAdapter) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "enter")
	defer observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "exit")
	client := a.client
	result, err := client.CallTool(ctx, a.toolInfo.Name, input)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "if: err != nil")
		if errors.Is(err, ErrServerNotConnected) {
			observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "if: errors.Is(err, ErrServerNotConnected)")
			if a.reconnect == nil {
				observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "if: a.reconnect == nil")
				return tool.InvokeResult{}, err
			}
			reconnectedClient, reconnErr := a.reconnect(ctx, a.server, a.fullName)
			if reconnErr != nil {
				observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "if: reconnErr != nil")
				observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"reconnect failed for %s: %w\", a.fullName, re...")
				return tool.InvokeResult{}, fmt.Errorf("reconnect failed for %s: %w", a.fullName, reconnErr)
			}
			result, err = reconnectedClient.CallTool(ctx, a.toolInfo.Name, input)
			if err != nil {
				observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "if: err != nil")
				observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "return: tool.InvokeResult{}, err")
				return tool.InvokeResult{}, err
			}
		} else {
			observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "else: errors.Is(err, ErrServerNotConnected)")
			observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "return: tool.InvokeResult{}, err")
			return tool.InvokeResult{}, err
		}
	}
	observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.Invoke", "return: tool.InvokeResult{Content: result}, nil")

	return tool.InvokeResult{Content: result}, nil
}

// CheckPerm extracts a permission-checkable string from the input.
// The content is the stringified JSON args — permission rules match against this.
func (a *MCPToolAdapter) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.CheckPerm", "exit")
	content := string(input)
	observe.TraceCtx(ctx, "mcp", "MCPToolAdapter.CheckPerm", "return: checker.Check(ctx, a.fullName, content)")
	return checker.Check(ctx, a.fullName, content)
}

// Flags returns tool flags derived from MCP annotations.
func (a *MCPToolAdapter) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{\n\tReadOnly:\ta.toolInfo.ReadOnly,\n\tConcurrent:\ta.toolInfo.ReadO...")
	return tool.ToolFlags{
		ReadOnly:    a.toolInfo.ReadOnly,
		Concurrent:  a.toolInfo.ReadOnly,
		Destructive: a.toolInfo.Destructive,
	}
}

// Compile-time check that MCPToolAdapter implements tool.Descriptor.
var _ tool.Descriptor = (*MCPToolAdapter)(nil)

package mcpauth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

var inputSchema = json.RawMessage(`{"type": "object",
	"additionalProperties": false, "properties": {}}`)

// Tool is a pseudo-tool injected when an MCP server requires OAuth authentication.
// When invoked, it starts an OAuth PKCE flow and returns the authorization URL.
// After the user completes the flow, the background goroutine reconnects the
// server and swaps this pseudo-tool for the real server tools.
type Tool struct {
	ServerName string
	Transport  string // "sse", "http", or "stdio"
	Auth       mcp.AuthConfig
	Manager    *mcp.Manager
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"mcp__\" + mcp.NormalizeName(t.ServerName) + \"__authenticate\"")
	return "mcp__" + mcp.NormalizeName(t.ServerName) + "__authenticate"
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: fmt.Sprintf(\n\t\"Authenticate with the %q MCP server. This server requires OAut...")
	return fmt.Sprintf(
		"Authenticate with the %q MCP server. This server requires OAuth authentication before its tools can be used. "+
			"Call this tool to start the authentication flow — it will return a URL for the user to open in their browser.",
		t.ServerName,
	)
}

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "mcpauth", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "mcpauth", "Tool.CheckPerm", "exit")
	content := t.permissionSubject()
	if checker == nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.CheckPerm", "if: checker == nil")
		observe.TraceCtx(ctx, "mcpauth", "Tool.CheckPerm", "return: permission.CheckResult{\n\tDecision:\tpermission.DecisionDeny,\n\tReason:\t\t\"permis...")
		return permission.CheckResult{
			Decision: permission.DecisionDeny,
			Reason:   "permission checker unavailable",
			Content:  content,
		}
	}
	observe.TraceCtx(ctx, "mcpauth", "Tool.CheckPerm", "return: checker.Check(ctx, t.Name(), content)")
	return checker.Check(ctx, t.Name(), content)
}

func (t *Tool) permissionSubject() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: fmt.Sprintf(\"server:%s action:oauth_authenticate effects:credential_persisten...")
	return fmt.Sprintf("server:%s action:oauth_authenticate effects:credential_persistence,capability_change", t.ServerName)
}

func (t *Tool) Invoke(ctx context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "exit")

	if t.Transport != "" && t.Transport != "sse" && t.Transport != "http" {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: t.Transport != \"\" && t.Transport != \"sse\" && t.Transport != \"http\"")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Server %q uses %s transport which d...")
		return tool.InvokeResult{
			Content: fmt.Sprintf("Server %q uses %s transport which does not support OAuth. Ask the user to authenticate manually.", t.ServerName, t.Transport),
		}, nil
	}

	if t.Auth.AuthURL == "" || t.Auth.TokenURL == "" {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: t.Auth.AuthURL == \"\" || t.Auth.TokenURL == \"\"")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"Server %q does not have OAuth endpo...")
		return tool.InvokeResult{
			Content: fmt.Sprintf("Server %q does not have OAuth endpoints configured. Ask the user to configure auth_url and token_url in the MCP server config.", t.ServerName),
		}, nil
	}

	if t.Manager == nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: t.Manager == nil")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\"MCP manager is not available. Ask t...")
		return tool.InvokeResult{
			Content: fmt.Sprintf("MCP manager is not available. Ask the user to restart Pragma and try authenticating %q again.", t.ServerName),
		}, nil
	}

	authURL, err := t.Manager.StartOAuthFlow(t.ServerName, t.Auth)
	if err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to start OAuth flow: %v\", err)...")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to start OAuth flow: %v", err)}, nil
	}
	observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\n\t\t\"Ask the user to open this URL in...")

	return tool.InvokeResult{
		Content: fmt.Sprintf(
			"Ask the user to open this URL in their browser to authorize the %q MCP server:\n\n%s\n\n"+
				"Once they complete the flow, the server's tools will become available automatically.",
			t.ServerName, authURL,
		),
	}, nil
}

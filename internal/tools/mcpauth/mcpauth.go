package mcpauth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/artpar/gogent/internal/mcp"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

var inputSchema = json.RawMessage(`{"type": "object", "properties": {}}`)

// Tool is a pseudo-tool injected when an MCP server requires OAuth authentication.
// When invoked, it starts an OAuth PKCE flow and returns the authorization URL.
// After the user completes the flow, the background goroutine reconnects the
// server and swaps this pseudo-tool for the real server tools.
type Tool struct {
	ServerName string
	Transport  string // "sse", "http", or "stdio"
	Auth       mcp.AuthConfig
	Manager    *mcp.Manager
	Registry   *tool.Registry
	Bus        *observe.EventBus
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

func (t *Tool) CheckPerm(_ context.Context, _ json.RawMessage, _ permission.Checker) permission.CheckResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: permission.CheckResult{Decision: permission.DecisionAllow}")
	return permission.CheckResult{Decision: permission.DecisionAllow}
}

func (t *Tool) Invoke(ctx context.Context, _ json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "exit")

	// Only SSE and HTTP transports support OAuth (stdio is local process, no auth needed)
	if t.Transport != "" && t.Transport != "sse" && t.Transport != "http" {
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

	verifier, challenge, err := mcp.GeneratePKCE()
	if err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to generate PKCE parameters: %...")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to generate PKCE parameters: %v", err)}, nil
	}

	state, err := mcp.GenerateState()
	if err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to generate state: %v\", err)},...")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to generate state: %v", err)}, nil
	}

	port, err := mcp.FindCallbackPort()
	if err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Failed to find callback port: %v\", er...")
		return tool.InvokeResult{Content: fmt.Sprintf("Failed to find callback port: %v", err)}, nil
	}

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	authURL := mcp.BuildAuthURL(t.Auth.AuthURL, t.Auth.ClientID, redirectURI, challenge, state, t.Auth.Scopes)

	if t.Bus != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "if: t.Bus != nil")
		t.Bus.Emit(observe.McpOAuthStarted{
			EventHeader: observe.NewEventHeader("McpOAuthStarted", "", observe.NewSpanID(), ""),
			ServerName:  t.ServerName,
			AuthURL:     authURL,
		})
	}

	go t.handleOAuthCallback(context.Background(), port, state, verifier, redirectURI)
	observe.TraceCtx(ctx, "mcpauth", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent: fmt.Sprintf(\n\t\t\"Ask the user to open this URL in...")

	return tool.InvokeResult{
		Content: fmt.Sprintf(
			"Ask the user to open this URL in their browser to authorize the %q MCP server:\n\n%s\n\n"+
				"Once they complete the flow, the server's tools will become available automatically.",
			t.ServerName, authURL,
		),
	}, nil
}

// handleOAuthCallback waits for the OAuth callback, exchanges the code, stores the token,
// and reconnects the MCP server.
func (t *Tool) handleOAuthCallback(ctx context.Context, port int, expectedState, verifier, redirectURI string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	code, err := mcp.StartCallbackServer(ctx, port, expectedState)
	if err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.handleOAuthCallback", "if: err != nil")
		t.emitCompletion(false, err.Error())
		return
	}

	token, err := mcp.ExchangeCode(ctx, t.Auth.TokenURL, code, verifier, redirectURI, t.Auth.ClientID, t.Auth.ClientSecret)
	if err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.handleOAuthCallback", "if: err != nil")
		t.emitCompletion(false, "token exchange failed: "+err.Error())
		return
	}

	if err := mcp.SaveToken(t.ServerName, token); err != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.handleOAuthCallback", "if: err != nil")
		t.emitCompletion(false, "failed to save token: "+err.Error())
		return
	}

	if t.Manager != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.handleOAuthCallback", "if: t.Manager != nil")
		if err := t.Manager.ReconnectServer(ctx, t.ServerName); err != nil {
			observe.TraceCtx(ctx, "mcpauth", "Tool.handleOAuthCallback", "if: err != nil")
			t.emitCompletion(false, "reconnect failed: "+err.Error())
			return
		}
	}

	if t.Registry != nil {
		observe.TraceCtx(ctx, "mcpauth", "Tool.handleOAuthCallback", "if: t.Registry != nil")
		t.Registry.Unregister(t.Name())
	}

	t.emitCompletion(true, "")
}

func (t *Tool) emitCompletion(success bool, errMsg string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if t.Bus == nil {
		observe.GlobalTrace("if: t.Bus == nil")
		return
	}
	t.Bus.Emit(observe.McpOAuthCompleted{
		EventHeader: observe.NewEventHeader("McpOAuthCompleted", "", observe.NewSpanID(), ""),
		ServerName:  t.ServerName,
		Success:     success,
		Error:       errMsg,
	})
}

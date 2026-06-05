package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/artpar/pragma/internal/observe"
)

// StartOAuthFlow starts a manager-owned OAuth PKCE callback flow and returns
// the authorization URL the user should open.
func (m *Manager) StartOAuthFlow(serverName string, auth AuthConfig) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if auth.AuthURL == "" || auth.TokenURL == "" {
		observe.GlobalTrace("if: auth.AuthURL == \"\" || auth.TokenURL == \"\"")
		return "", fmt.Errorf("server %q does not have OAuth endpoints configured", serverName)
	}

	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return "", fmt.Errorf("generate PKCE parameters: %w", err)
	}
	state, err := GenerateState()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return "", fmt.Errorf("generate state: %w", err)
	}
	port, err := FindCallbackPort()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		return "", fmt.Errorf("find callback port: %w", err)
	}

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)
	authURL := BuildAuthURL(auth.AuthURL, auth.ClientID, redirectURI, challenge, state, auth.Scopes)
	m.emitOAuthStarted(serverName, authURL)

	go m.handleOAuthCallback(m.LifecycleContext(), serverName, auth, port, state, verifier, redirectURI)
	return authURL, nil
}

func (m *Manager) handleOAuthCallback(ctx context.Context, serverName string, auth AuthConfig, port int, expectedState, verifier, redirectURI string) {
	observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "exit")
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	code, err := StartCallbackServer(ctx, port, expectedState)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "if: err != nil")
		m.emitOAuthCompleted(serverName, false, err.Error())
		return
	}

	token, err := ExchangeCode(ctx, auth.TokenURL, code, verifier, redirectURI, auth.ClientID, auth.ClientSecret)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "if: err != nil")
		m.emitOAuthCompleted(serverName, false, "token exchange failed: "+err.Error())
		return
	}
	if err := SaveToken(serverName, token); err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "if: err != nil")
		m.emitOAuthCompleted(serverName, false, "failed to save token: "+err.Error())
		return
	}
	if err := m.ReconnectServer(ctx, serverName); err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "if: err != nil")
		m.emitOAuthCompleted(serverName, false, "reconnect failed: "+err.Error())
		return
	}
	if m.registry != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.handleOAuthCallback", "if: m.registry != nil")
		m.registry.Unregister(authToolName(serverName))
	}
	m.emitOAuthCompleted(serverName, true, "")
}

func authToolName(serverName string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return "mcp__" + NormalizeName(serverName) + "__authenticate"
}

func (m *Manager) emitOAuthStarted(serverName, authURL string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.bus == nil {
		observe.GlobalTrace("if: m.bus == nil")
		return
	}
	m.bus.Emit(observe.McpOAuthStarted{
		EventHeader: observe.NewEventHeader("McpOAuthStarted", "", observe.NewSpanID(), ""),
		ServerName:  serverName,
		AuthURL:     authURL,
	})
}

func (m *Manager) emitOAuthCompleted(serverName string, success bool, errMsg string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if m.bus == nil {
		observe.GlobalTrace("if: m.bus == nil")
		return
	}
	m.bus.Emit(observe.McpOAuthCompleted{
		EventHeader: observe.NewEventHeader("McpOAuthCompleted", "", observe.NewSpanID(), ""),
		ServerName:  serverName,
		Success:     success,
		Error:       errMsg,
	})
}

package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/config"
	"github.com/artpar/gogent/internal/observe"
)

// OAuthToken holds OAuth credentials for an MCP server.
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes,omitempty"`
}

// IsValid returns true if the token exists and has not expired.
func (t OAuthToken) IsValid() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: t.AccessToken != \"\" && time.Now().Before(t.ExpiresAt)")
	return t.AccessToken != "" && time.Now().Before(t.ExpiresAt)
}

// AuthConfig holds OAuth configuration for an MCP server.
type AuthConfig struct {
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret string   `json:"client_secret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	AuthURL      string   `json:"auth_url,omitempty"`  // authorization endpoint
	TokenURL     string   `json:"token_url,omitempty"` // token endpoint
}

// GeneratePKCE generates a PKCE code verifier and code challenge (S256).
func GeneratePKCE() (verifier, challenge string, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	buf := make([]byte, 96)
	if _, err := rand.Read(buf); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", \"\", fmt.Errorf(\"generate random bytes: %w\", err)")
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buf)

	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	observe.GlobalTrace("return: verifier, challenge, nil")
	return verifier, challenge, nil
}

// GenerateState generates a random state parameter for CSRF protection.
func GenerateState() (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: base64.RawURLEncoding.EncodeToString(buf), nil")
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// FindCallbackPort finds an available port for the OAuth callback server.
// If MCP_OAUTH_CALLBACK_PORT is set, uses that. Otherwise finds a free port
// in the ephemeral range (49152-65535).
func FindCallbackPort() (int, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if portStr := os.Getenv("MCP_OAUTH_CALLBACK_PORT"); portStr != "" {
		observe.GlobalTrace("if: portStr != \"\"")
		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err == nil && port > 0 {
			observe.GlobalTrace("if: err == nil && port > 0")
			observe.GlobalTrace("return: port, nil")
			return port, nil
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: 0, fmt.Errorf(\"find free port: %w\", err)")
		return 0, fmt.Errorf("find free port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	observe.GlobalTrace("return: port, nil")
	return port, nil
}

// StartCallbackServer starts a local HTTP server that waits for the OAuth callback.
// It blocks until a callback is received or the context is cancelled.
// Returns the authorization code from the callback.
func StartCallbackServer(ctx context.Context, port int, expectedState string) (string, error) {
	observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "enter")
	defer observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "exit")
	type result struct {
		code string
		err  error
	}
	ch := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		errParam := r.URL.Query().Get("error")

		if state != expectedState {
			http.Error(w, "State mismatch — possible CSRF attack.", http.StatusBadRequest)
			select {
			case ch <- result{err: fmt.Errorf("state mismatch: expected %q, got %q", expectedState, state)}:
			default:
			}
			return
		}

		if errParam != "" {
			desc := r.URL.Query().Get("error_description")
			http.Error(w, "Authorization denied: "+desc, http.StatusForbidden)
			select {
			case ch <- result{err: fmt.Errorf("provider denied: %s — %s", errParam, desc)}:
			default:
			}
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "<html><body><h2>Authorization successful!</h2><p>You can close this window.</p></body></html>")
		select {
		case ch <- result{code: code}:
		default:
		}
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "if: err != nil && err != http.ErrServerClosed")
			select {
			case ch <- result{err: fmt.Errorf("callback server: %w", err)}:
				observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "select: ch <- result{err: fmt.Errorf(\"callback server: %w\", err)}")
			default:
				observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "select: default")
			}
		}
	}()

	defer server.Shutdown(context.Background())

	select {
	case res := <-ch:
		observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "select: res := <-ch")
		return res.code, res.err
	case <-ctx.Done():
		observe.TraceCtx(ctx, "mcp", "StartCallbackServer", "select: <-ctx.Done()")
		return "", ctx.Err()
	}
}

// ExchangeCode exchanges an authorization code for tokens.
func ExchangeCode(ctx context.Context, tokenURL, code, codeVerifier, redirectURI, clientID, clientSecret string) (OAuthToken, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
	}
	if clientSecret != "" {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: clientSecret != \"\"")
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "return: OAuthToken{}, fmt.Errorf(\"build token request: %w\", err)")
		return OAuthToken{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "return: OAuthToken{}, fmt.Errorf(\"token request: %w\", err)")
		return OAuthToken{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "return: OAuthToken{}, fmt.Errorf(\"read token response: %w\", err)")
		return OAuthToken{}, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: resp.StatusCode != http.StatusOK")
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "return: OAuthToken{}, fmt.Errorf(\"token endpoint returned %d: %s\", resp.StatusCode, s...")
		return OAuthToken{}, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"` // seconds
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "return: OAuthToken{}, fmt.Errorf(\"decode token response: %w\", err)")
		return OAuthToken{}, fmt.Errorf("decode token response: %w", err)
	}

	expiresIn := tokenResp.ExpiresIn
	if expiresIn <= 0 {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: expiresIn <= 0")
		expiresIn = 3600
	}

	var scopes []string
	if tokenResp.Scope != "" {
		observe.TraceCtx(ctx, "mcp", "ExchangeCode", "if: tokenResp.Scope != \"\"")
		scopes = strings.Split(tokenResp.Scope, " ")
	}
	observe.TraceCtx(ctx, "mcp", "ExchangeCode", "return: OAuthToken{\n\tAccessToken:\ttokenResp.AccessToken,\n\tRefreshToken:\ttokenResp.Ref...")

	return OAuthToken{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
		Scopes:       scopes,
	}, nil
}

// SaveToken writes an OAuth token to disk atomically.
// Tokens are stored at ~/.gogent/mcp-tokens/<server>.json.
func SaveToken(serverName string, token OAuthToken) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dir, err := tokenDir()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"create token directory: %w\", err)")
		return fmt.Errorf("create token directory: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	data = append(data, '\n')

	path := filepath.Join(dir, NormalizeName(serverName)+".json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"write temp token file: %w\", err)")
		return fmt.Errorf("write temp token file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		observe.GlobalTrace("if: err != nil")
		os.Remove(tmpPath)
		observe.GlobalTrace("return: fmt.Errorf(\"rename token file: %w\", err)")
		return fmt.Errorf("rename token file: %w", err)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// LoadToken reads an OAuth token from disk.
func LoadToken(serverName string) (OAuthToken, bool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dir, err := tokenDir()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: OAuthToken{}, false, err")
		return OAuthToken{}, false, err
	}

	path := filepath.Join(dir, NormalizeName(serverName)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		if os.IsNotExist(err) {
			observe.GlobalTrace("if: os.IsNotExist(err)")
			observe.GlobalTrace("return: OAuthToken{}, false, nil")
			return OAuthToken{}, false, nil
		}
		observe.GlobalTrace("return: OAuthToken{}, false, err")
		return OAuthToken{}, false, err
	}

	var token OAuthToken
	if err := json.Unmarshal(data, &token); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: OAuthToken{}, false, fmt.Errorf(\"decode token: %w\", err)")
		return OAuthToken{}, false, fmt.Errorf("decode token: %w", err)
	}
	observe.GlobalTrace("return: token, true, nil")
	return token, true, nil
}

func tokenDir() (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	home, err := config.GogentHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", err")
		return "", err
	}
	observe.GlobalTrace("return: filepath.Join(home, \"mcp-tokens\"), nil")
	return filepath.Join(home, "mcp-tokens"), nil
}

// BuildAuthURL constructs an OAuth authorization URL with PKCE parameters.
func BuildAuthURL(authEndpoint, clientID, redirectURI, codeChallenge, state string, scopes []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	params := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	if len(scopes) > 0 {
		observe.GlobalTrace("if: len(scopes) > 0")
		params.Set("scope", strings.Join(scopes, " "))
	}
	observe.GlobalTrace("return: authEndpoint + \"?\" + params.Encode()")
	return authEndpoint + "?" + params.Encode()
}

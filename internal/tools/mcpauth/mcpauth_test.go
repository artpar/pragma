package mcpauth

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/gogent/internal/mcp"
)

func TestInvoke_UnsupportedTransport(t *testing.T) {
	tl := &Tool{
		ServerName: "test-server",
		Transport:  "stdio",
	}
	result, err := tl.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "does not support OAuth") {
		t.Errorf("Content = %q, want mention of OAuth unsupported", result.Content)
	}
	if !strings.Contains(result.Content, "manually") {
		t.Errorf("Content = %q, want mention of manual auth", result.Content)
	}
}

func TestInvoke_MissingAuthURLs(t *testing.T) {
	tl := &Tool{
		ServerName: "test-server",
		Transport:  "sse",
		Auth:       mcp.AuthConfig{},
	}
	result, err := tl.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(result.Content, "does not have OAuth endpoints") {
		t.Errorf("Content = %q, want mention of missing endpoints", result.Content)
	}
}

func TestName(t *testing.T) {
	tl := &Tool{ServerName: "my-server"}
	name := tl.Name()
	if !strings.HasPrefix(name, "mcp__") {
		t.Errorf("Name = %q, want mcp__ prefix", name)
	}
	if !strings.HasSuffix(name, "__authenticate") {
		t.Errorf("Name = %q, want __authenticate suffix", name)
	}
}

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := mcp.GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE: %v", err)
	}
	if verifier == "" {
		t.Error("verifier is empty")
	}
	if challenge == "" {
		t.Error("challenge is empty")
	}
	if verifier == challenge {
		t.Error("verifier and challenge should differ")
	}
}

func TestGenerateState(t *testing.T) {
	state, err := mcp.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	if state == "" {
		t.Error("state is empty")
	}
	if len(state) < 32 {
		t.Errorf("state length = %d, want >= 32", len(state))
	}

	// Two calls should produce different values
	state2, _ := mcp.GenerateState()
	if state == state2 {
		t.Error("GenerateState returned same value twice")
	}
}

package cli

import (
	"os"
	"testing"

	"github.com/artpar/pragma/internal/config"
)

// WEB-001 wiring gates: key resolution order and base URL override.

func TestWebSearchKeyEnvWinsOverCredentials(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "env-key")
	d := &Deps{Creds: config.Credentials{Providers: map[string]config.ProviderCredential{
		"brave": {APIKey: "file-key"},
	}}}
	if got := webSearchKey(d); got != "env-key" {
		t.Fatalf("webSearchKey = %q, want env-key", got)
	}
}

func TestWebSearchKeyFromCredentials(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	d := &Deps{Creds: config.Credentials{Providers: map[string]config.ProviderCredential{
		"brave": {APIKey: "file-key"},
	}}}
	if got := webSearchKey(d); got != "file-key" {
		t.Fatalf("webSearchKey = %q, want file-key", got)
	}
}

func TestWebSearchKeyEmptyWhenUnresolvable(t *testing.T) {
	t.Setenv("BRAVE_SEARCH_API_KEY", "")
	d := &Deps{}
	if got := webSearchKey(d); got != "" {
		t.Fatalf("webSearchKey = %q, want empty", got)
	}
}

func TestResolveWebSearchBaseURL(t *testing.T) {
	os.Unsetenv("BRAVE_SEARCH_BASE_URL")
	if got := resolveWebSearchBaseURL(); got != "" {
		t.Fatalf("default base URL override = %q, want empty", got)
	}
	t.Setenv("BRAVE_SEARCH_BASE_URL", " http://127.0.0.1:9/v1 ")
	if got := resolveWebSearchBaseURL(); got != "http://127.0.0.1:9/v1" {
		t.Fatalf("base URL override = %q", got)
	}
}

package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadCredentials_NoFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	creds, err := LoadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Providers != nil {
		t.Errorf("expected nil providers, got %v", creds.Providers)
	}
}

func TestLoadCredentials_ValidFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".pragma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `providers:
  anthropic:
    api_key: sk-ant-test123
  openai:
    api_key: sk-test456
    base_url: https://custom.example.com/v1
  google:
    api_key: AIzaTest789
`
	if err := os.WriteFile(filepath.Join(dir, "credentials.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	creds, err := LoadCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		provider string
		wantKey  string
		wantURL  string
	}{
		{"anthropic", "sk-ant-test123", ""},
		{"openai", "sk-test456", "https://custom.example.com/v1"},
		{"google", "AIzaTest789", ""},
		{"groq", "", ""},
	}

	for _, tt := range tests {
		pc := creds.CredentialFor(tt.provider)
		if pc.APIKey != tt.wantKey {
			t.Errorf("CredentialFor(%q).APIKey = %q, want %q", tt.provider, pc.APIKey, tt.wantKey)
		}
		if pc.BaseURL != tt.wantURL {
			t.Errorf("CredentialFor(%q).BaseURL = %q, want %q", tt.provider, pc.BaseURL, tt.wantURL)
		}
	}
}

func TestLoadCredentials_MalformedYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".pragma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.yml"), []byte("{{invalid yaml"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCredentials()
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestSaveCredentials_Permissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	creds := Credentials{
		Providers: map[string]ProviderCredential{
			"anthropic": {APIKey: "sk-ant-secret"},
		},
	}

	if err := SaveCredentials(creds); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	path := filepath.Join(home, ".pragma", "credentials.yml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestSaveCredentials_RoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	original := Credentials{
		Providers: map[string]ProviderCredential{
			"anthropic":  {APIKey: "sk-ant-key1"},
			"openai":     {APIKey: "sk-key2", BaseURL: "https://custom.example.com/v1"},
			"google":     {APIKey: "AIza-key3"},
			"groq":       {APIKey: "gsk-key4"},
			"openrouter": {APIKey: "sk-or-key5", BaseURL: "https://openrouter.ai/api/v1"},
			"morphllm":   {APIKey: "morph-key6", BaseURL: "https://api.morphllm.com/v1"},
		},
	}

	if err := SaveCredentials(original); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}

	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}

	for provider, want := range original.Providers {
		got := loaded.CredentialFor(provider)
		if got.APIKey != want.APIKey {
			t.Errorf("provider %q: APIKey = %q, want %q", provider, got.APIKey, want.APIKey)
		}
		if got.BaseURL != want.BaseURL {
			t.Errorf("provider %q: BaseURL = %q, want %q", provider, got.BaseURL, want.BaseURL)
		}
	}
}

func TestCredentialFor_NilProviders(t *testing.T) {
	creds := Credentials{}
	pc := creds.CredentialFor("anthropic")
	if pc.APIKey != "" || pc.BaseURL != "" {
		t.Errorf("expected zero value, got %+v", pc)
	}
}

func TestYAMLRoundTrip(t *testing.T) {
	original := Credentials{
		Providers: map[string]ProviderCredential{
			"anthropic": {APIKey: "key1"},
			"openai":    {APIKey: "key2", BaseURL: "https://example.com"},
		},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Credentials
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for provider, want := range original.Providers {
		got := decoded.CredentialFor(provider)
		if got != want {
			t.Errorf("provider %q: got %+v, want %+v", provider, got, want)
		}
	}
}

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/config"
)

func TestAutoDetectProvider(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		creds   config.Credentials
		want    string
	}{
		{
			name: "no credentials at all",
			want: "",
		},
		{
			name:    "ANTHROPIC_API_KEY env var",
			envVars: map[string]string{"ANTHROPIC_API_KEY": "sk-ant-xxx"},
			want:    "anthropic",
		},
		{
			name:    "GOOGLE_API_KEY env var only",
			envVars: map[string]string{"GOOGLE_API_KEY": "AIza-xxx"},
			want:    "google",
		},
		{
			name:    "OPENAI_API_KEY env var only",
			envVars: map[string]string{"OPENAI_API_KEY": "sk-xxx"},
			want:    "openai",
		},
		{
			name:    "OPENROUTER_API_KEY env var only",
			envVars: map[string]string{"OPENROUTER_API_KEY": "sk-or-xxx"},
			want:    "openrouter",
		},
		{
			name:    "GROQ_API_KEY env var only",
			envVars: map[string]string{"GROQ_API_KEY": "gsk-xxx"},
			want:    "groq",
		},
		{
			name:    "anthropic wins over google by priority",
			envVars: map[string]string{"ANTHROPIC_API_KEY": "sk-ant-xxx", "GOOGLE_API_KEY": "AIza-xxx"},
			want:    "anthropic",
		},
		{
			name:    "google wins over openai by priority",
			envVars: map[string]string{"GOOGLE_API_KEY": "AIza-xxx", "OPENAI_API_KEY": "sk-xxx"},
			want:    "google",
		},
		{
			name: "credentials file anthropic only",
			creds: config.Credentials{
				Providers: map[string]config.ProviderCredential{
					"anthropic": {APIKey: "sk-ant-creds"},
				},
			},
			want: "anthropic",
		},
		{
			name: "credentials file groq only",
			creds: config.Credentials{
				Providers: map[string]config.ProviderCredential{
					"groq": {APIKey: "gsk-creds"},
				},
			},
			want: "groq",
		},
		{
			name:    "credentials anthropic beats env groq (priority order)",
			envVars: map[string]string{"GROQ_API_KEY": "gsk-xxx"},
			creds: config.Credentials{
				Providers: map[string]config.ProviderCredential{
					"anthropic": {APIKey: "sk-ant-creds"},
				},
			},
			want: "anthropic",
		},
		{
			name:    "env anthropic beats credentials google",
			envVars: map[string]string{"ANTHROPIC_API_KEY": "sk-ant-xxx"},
			creds: config.Credentials{
				Providers: map[string]config.ProviderCredential{
					"google": {APIKey: "AIza-creds"},
				},
			},
			want: "anthropic",
		},
	}

	// All env vars that autoDetectProvider checks
	allEnvVars := []string{"ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "OPENROUTER_API_KEY", "GROQ_API_KEY"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars
			for _, ev := range allEnvVars {
				t.Setenv(ev, "")
			}
			// Set test-specific env vars
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			got := autoDetectProvider(tt.creds)
			if got != tt.want {
				t.Errorf("autoDetectProvider() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveProviderConfigExplicitFlagsWin(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENROUTER_API_KEY", "env-key")
	t.Chdir(work)
	writeTestCredentials(t, home, `providers:
  openrouter:
    api_key: creds-key
    base_url: https://creds.example/v1
`)

	cmd := newFlagCommand(t, "--provider", "openrouter", "--model", "z-ai/glm-5.3", "--api-key", "flag-key")
	resolved, err := ResolveProviderConfig(cmd, ProviderResolutionOptions{})
	if err != nil {
		t.Fatalf("ResolveProviderConfig: %v", err)
	}
	if resolved.Provider != "openrouter" {
		t.Fatalf("Provider = %q, want openrouter", resolved.Provider)
	}
	if resolved.Model != "z-ai/glm-5.3" {
		t.Fatalf("Model = %q, want z-ai/glm-5.3", resolved.Model)
	}
	if resolved.APIKey != "flag-key" {
		t.Fatalf("APIKey = %q, want flag-key", resolved.APIKey)
	}
	if !resolved.ProviderExplicit || !resolved.ModelExplicit {
		t.Fatalf("explicit flags not tracked: %#v", resolved)
	}
	if resolved.BaseURL != "https://creds.example/v1" {
		t.Fatalf("BaseURL = %q, want credentials URL", resolved.BaseURL)
	}
}

func TestRegisterFlagsIncludesSystemPrompt(t *testing.T) {
	cmd := newFlagCommand(t, "--system-prompt", "custom rules")
	got, err := cmd.Flags().GetString("system-prompt")
	if err != nil {
		t.Fatalf("GetString(system-prompt): %v", err)
	}
	if got != "custom rules" {
		t.Fatalf("system-prompt = %q, want custom rules", got)
	}
}

func TestRegisterFlagsIncludesLoop(t *testing.T) {
	cmd := newFlagCommand(t, "--loop", "provider-tools")
	got, err := cmd.Flags().GetString("loop")
	if err != nil {
		t.Fatalf("GetString(loop): %v", err)
	}
	if got != "provider-tools" {
		t.Fatalf("loop = %q, want provider-tools", got)
	}
}

func TestResolveProviderConfigRejectsInvalidLoop(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENROUTER_API_KEY", "env-key")
	t.Chdir(work)

	cmd := newFlagCommand(t, "--loop", "bogus")
	_, err := SetupDepsWithOptions(cmd, SetupDepsOptions{})
	if err == nil {
		t.Fatal("SetupDepsWithOptions error = nil, want invalid loop mode")
	}
	if got := err.Error(); got != `invalid loop mode "bogus"; expected pragma or provider-tools` {
		t.Fatalf("error = %q", got)
	}
}

func TestResolveProviderConfigCredentialsThenEnv(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENROUTER_API_KEY", "env-key")
	t.Chdir(work)
	writeTestCredentials(t, home, `providers:
  openrouter:
    api_key: creds-key
`)

	cmd := newFlagCommand(t)
	resolved, err := ResolveProviderConfig(cmd, ProviderResolutionOptions{DefaultProvider: "openrouter", DefaultModel: "captured-model"})
	if err != nil {
		t.Fatalf("ResolveProviderConfig: %v", err)
	}
	if resolved.Provider != "openrouter" {
		t.Fatalf("Provider = %q, want openrouter", resolved.Provider)
	}
	if resolved.Model != "captured-model" {
		t.Fatalf("Model = %q, want captured-model", resolved.Model)
	}
	if resolved.APIKey != "creds-key" {
		t.Fatalf("APIKey = %q, want credentials before env", resolved.APIKey)
	}
	if resolved.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("BaseURL = %q, want default OpenRouter URL", resolved.BaseURL)
	}
}

func TestResolveProviderConfigEnvFallback(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENAI_API_KEY", "env-openai")
	t.Chdir(work)

	cmd := newFlagCommand(t)
	resolved, err := ResolveProviderConfig(cmd, ProviderResolutionOptions{DefaultProvider: "openai"})
	if err != nil {
		t.Fatalf("ResolveProviderConfig: %v", err)
	}
	if resolved.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", resolved.Provider)
	}
	if resolved.APIKey != "env-openai" {
		t.Fatalf("APIKey = %q, want env-openai", resolved.APIKey)
	}
	if resolved.Model != DefaultModelFor("openai") {
		t.Fatalf("Model = %q, want OpenAI default", resolved.Model)
	}
}

func TestSecondaryModelForOpenRouterPreservesActiveModel(t *testing.T) {
	const active = "z-ai/glm-5.3"
	if got := SecondaryModelFor("openrouter", active); got != active {
		t.Fatalf("SecondaryModelFor(openrouter, %q) = %q, want active model", active, got)
	}
}

func TestOpenRouterDefaultsToGLM53(t *testing.T) {
	if got := DefaultModelFor("openrouter"); got != "z-ai/glm-5.3" {
		t.Fatalf("DefaultModelFor(openrouter) = %q, want z-ai/glm-5.3", got)
	}
	if got := DefaultBaseURLFor("openrouter"); got != "https://openrouter.ai/api/v1" {
		t.Fatalf("DefaultBaseURLFor(openrouter) = %q", got)
	}
	if got := envVarForProvider("openrouter"); got != "OPENROUTER_API_KEY" {
		t.Fatalf("envVarForProvider(openrouter) = %q", got)
	}
}

func newFlagCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "pragma"}
	RegisterFlags(cmd)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	return cmd
}

func writeTestCredentials(t *testing.T, home, content string) {
	t.Helper()
	dir := filepath.Join(home, ".pragma")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "credentials.yml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

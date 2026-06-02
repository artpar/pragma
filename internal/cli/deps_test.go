package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/model"
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
			name:    "LILAC_API_KEY env var only",
			envVars: map[string]string{"LILAC_API_KEY": "lilac-xxx"},
			want:    "lilac",
		},
		{
			name:    "OPENAI_API_KEY env var only",
			envVars: map[string]string{"OPENAI_API_KEY": "sk-xxx"},
			want:    "openai",
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
	allEnvVars := []string{"ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "LILAC_API_KEY", "OPENAI_API_KEY", "GROQ_API_KEY"}

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

func TestApplyFlagOverridesStopAfterToolExec(t *testing.T) {
	cmd := &cobra.Command{Use: "pragma"}
	RegisterFlags(cmd)
	if err := cmd.ParseFlags([]string{
		"--stop-after-tool-exec",
		"--context-mode", model.ContextModeStateHandoff,
		"--handoff-schema", model.HandoffSchemaV1,
		"--toolset", "idea",
	}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	var cfg config.Config
	ApplyFlagOverrides(cmd, &cfg)

	if !cfg.StopAfterToolExec {
		t.Fatal("StopAfterToolExec = false, want true")
	}
	if cfg.ContextMode != model.ContextModeStateHandoff {
		t.Fatalf("ContextMode = %q, want %q", cfg.ContextMode, model.ContextModeStateHandoff)
	}
	if cfg.HandoffSchema != model.HandoffSchemaV1 {
		t.Fatalf("HandoffSchema = %q, want %q", cfg.HandoffSchema, model.HandoffSchemaV1)
	}
	if cfg.Toolset != "idea" {
		t.Fatalf("Toolset = %q, want idea", cfg.Toolset)
	}
}

func TestApplyFlagOverridesStateHandoffDoesNotImplyStopAfterToolExec(t *testing.T) {
	cmd := &cobra.Command{Use: "pragma"}
	RegisterFlags(cmd)
	if err := cmd.ParseFlags([]string{
		"--context-mode", model.ContextModeStateHandoff,
		"--handoff-schema", model.HandoffSchemaV1,
	}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}

	var cfg config.Config
	ApplyFlagOverrides(cmd, &cfg)

	if cfg.StopAfterToolExec {
		t.Fatal("StopAfterToolExec = true, want false unless explicitly requested")
	}
}

func TestApplyContextDefaultsUsesStateHandoff(t *testing.T) {
	var cfg config.Config
	applyContextDefaults(&cfg)

	if cfg.ContextMode != model.ContextModeStateHandoff {
		t.Fatalf("ContextMode = %q, want %q", cfg.ContextMode, model.ContextModeStateHandoff)
	}
	if cfg.HandoffSchema != model.HandoffSchemaV1 {
		t.Fatalf("HandoffSchema = %q, want %q", cfg.HandoffSchema, model.HandoffSchemaV1)
	}
	if cfg.StopAfterToolExec {
		t.Fatal("StopAfterToolExec = true, want false by default")
	}
}

func TestApplyContextDefaultsPreservesExplicitChatMode(t *testing.T) {
	cfg := config.Config{ContextMode: model.ContextModeChat}
	applyContextDefaults(&cfg)

	if cfg.ContextMode != model.ContextModeChat {
		t.Fatalf("ContextMode = %q, want %q", cfg.ContextMode, model.ContextModeChat)
	}
	if cfg.StopAfterToolExec {
		t.Fatal("StopAfterToolExec = true, want false for explicit chat mode")
	}
}

func TestResolveProviderConfigExplicitFlagsWin(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LILAC_API_KEY", "env-key")
	t.Chdir(work)
	writeTestCredentials(t, home, `providers:
  lilac:
    api_key: creds-key
    base_url: https://creds.example/v1
`)

	cmd := newFlagCommand(t, "--provider", "lilac", "--model", "glm", "--api-key", "flag-key")
	resolved, err := ResolveProviderConfig(cmd, ProviderResolutionOptions{})
	if err != nil {
		t.Fatalf("ResolveProviderConfig: %v", err)
	}
	if resolved.Provider != "lilac" {
		t.Fatalf("Provider = %q, want lilac", resolved.Provider)
	}
	if resolved.Model != "zai-org/glm-5.1" {
		t.Fatalf("Model = %q, want alias-resolved glm", resolved.Model)
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

func TestResolveProviderConfigCredentialsThenEnv(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LILAC_API_KEY", "env-key")
	t.Chdir(work)
	writeTestCredentials(t, home, `providers:
  lilac:
    api_key: creds-key
`)

	cmd := newFlagCommand(t)
	resolved, err := ResolveProviderConfig(cmd, ProviderResolutionOptions{DefaultProvider: "lilac", DefaultModel: "captured-model"})
	if err != nil {
		t.Fatalf("ResolveProviderConfig: %v", err)
	}
	if resolved.Provider != "lilac" {
		t.Fatalf("Provider = %q, want lilac", resolved.Provider)
	}
	if resolved.Model != "captured-model" {
		t.Fatalf("Model = %q, want captured-model", resolved.Model)
	}
	if resolved.APIKey != "creds-key" {
		t.Fatalf("APIKey = %q, want credentials before env", resolved.APIKey)
	}
	if resolved.BaseURL != "https://api.getlilac.com/v1" {
		t.Fatalf("BaseURL = %q, want default Lilac URL", resolved.BaseURL)
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

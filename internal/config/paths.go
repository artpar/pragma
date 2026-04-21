package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// PragmaHome returns the pragma home directory (~/.pragma/).
func PragmaHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".pragma"), nil
}

// CredentialsPath returns ~/.pragma/credentials.yml.
func CredentialsPath() (string, error) {
	dir, err := PragmaHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yml"), nil
}

// GlobalSettingsPath returns ~/.pragma/settings.json.
func GlobalSettingsPath() (string, error) {
	dir, err := PragmaHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// ProjectSettingsPath returns <workDir>/.pragma/settings.json.
func ProjectSettingsPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "settings.json")
}

// LocalSettingsPath returns <workDir>/.pragma/settings.local.json.
func LocalSettingsPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "settings.local.json")
}

// SessionsDir returns ~/.pragma/sessions/.
func SessionsDir() (string, error) {
	dir, err := PragmaHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// GlobalAgentMDPath returns ~/.pragma/AGENT.md.
func GlobalAgentMDPath() (string, error) {
	dir, err := PragmaHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "AGENT.md"), nil
}

// ProjectAgentMDPath returns <workDir>/.pragma/AGENT.md.
func ProjectAgentMDPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "AGENT.md")
}

// LocalAgentMDPath returns <workDir>/.pragma/AGENT.local.md.
func LocalAgentMDPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "AGENT.local.md")
}

// MCPConfigPath returns <workDir>/.pragma/mcp.json.
func MCPConfigPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "mcp.json")
}

// MCPLocalConfigPath returns <workDir>/.pragma/mcp.local.json.
func MCPLocalConfigPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "mcp.local.json")
}

// GlobalMCPConfigPath returns ~/.pragma/mcp.json.
func GlobalMCPConfigPath() (string, error) {
	dir, err := PragmaHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp.json"), nil
}

// LSPConfigPath returns <workDir>/.pragma/lsp.json.
func LSPConfigPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "lsp.json")
}

// LSPLocalConfigPath returns <workDir>/.pragma/lsp.local.json.
func LSPLocalConfigPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "lsp.local.json")
}

// GlobalLSPConfigPath returns ~/.pragma/lsp.json.
func GlobalLSPConfigPath() (string, error) {
	dir, err := PragmaHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lsp.json"), nil
}

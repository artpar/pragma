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

// MCPConfigPath returns <workDir>/.pragma/mcp.json.
func MCPConfigPath(workDir string) string {
	return filepath.Join(workDir, ".pragma", "mcp.json")
}

// RootMCPConfigPath returns <workDir>/.mcp.json.
func RootMCPConfigPath(workDir string) string {
	return filepath.Join(workDir, ".mcp.json")
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

package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// GogentHome returns the gogent home directory (~/.gogent/).
func GogentHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".gogent"), nil
}

// CredentialsPath returns ~/.gogent/credentials.yml.
func CredentialsPath() (string, error) {
	dir, err := GogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.yml"), nil
}

// GlobalSettingsPath returns ~/.gogent/settings.json.
func GlobalSettingsPath() (string, error) {
	dir, err := GogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// ProjectSettingsPath returns <workDir>/.gogent/settings.json.
func ProjectSettingsPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "settings.json")
}

// LocalSettingsPath returns <workDir>/.gogent/settings.local.json.
func LocalSettingsPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "settings.local.json")
}

// SessionsDir returns ~/.gogent/sessions/.
func SessionsDir() (string, error) {
	dir, err := GogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// GlobalAgentMDPath returns ~/.gogent/AGENT.md.
func GlobalAgentMDPath() (string, error) {
	dir, err := GogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "AGENT.md"), nil
}

// ProjectAgentMDPath returns <workDir>/.gogent/AGENT.md.
func ProjectAgentMDPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "AGENT.md")
}

// LocalAgentMDPath returns <workDir>/.gogent/AGENT.local.md.
func LocalAgentMDPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "AGENT.local.md")
}

// MCPConfigPath returns <workDir>/.gogent/mcp.json.
func MCPConfigPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "mcp.json")
}

// MCPLocalConfigPath returns <workDir>/.gogent/mcp.local.json.
func MCPLocalConfigPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "mcp.local.json")
}

// GlobalMCPConfigPath returns ~/.gogent/mcp.json.
func GlobalMCPConfigPath() (string, error) {
	dir, err := GogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp.json"), nil
}

// LSPConfigPath returns <workDir>/.gogent/lsp.json.
func LSPConfigPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "lsp.json")
}

// LSPLocalConfigPath returns <workDir>/.gogent/lsp.local.json.
func LSPLocalConfigPath(workDir string) string {
	return filepath.Join(workDir, ".gogent", "lsp.local.json")
}

// GlobalLSPConfigPath returns ~/.gogent/lsp.json.
func GlobalLSPConfigPath() (string, error) {
	dir, err := GogentHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lsp.json"), nil
}

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

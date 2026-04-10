package config

import (
	"path/filepath"
	"testing"
)

func TestGogentHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	home, err := GogentHome()
	if err != nil {
		t.Fatalf("GogentHome: %v", err)
	}
	if home != filepath.Join(dir, ".gogent") {
		t.Errorf("GogentHome = %q, want %q", home, filepath.Join(dir, ".gogent"))
	}
}

func TestSessionsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sessDir, err := SessionsDir()
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	expected := filepath.Join(dir, ".gogent", "sessions")
	if sessDir != expected {
		t.Errorf("SessionsDir = %q, want %q", sessDir, expected)
	}
}

func TestGlobalAgentMDPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	path, err := GlobalAgentMDPath()
	if err != nil {
		t.Fatalf("GlobalAgentMDPath: %v", err)
	}
	expected := filepath.Join(dir, ".gogent", "AGENT.md")
	if path != expected {
		t.Errorf("GlobalAgentMDPath = %q, want %q", path, expected)
	}
}

func TestProjectPaths(t *testing.T) {
	workDir := "/tmp/myproject"

	tests := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{"ProjectSettingsPath", ProjectSettingsPath, filepath.Join(workDir, ".gogent", "settings.json")},
		{"LocalSettingsPath", LocalSettingsPath, filepath.Join(workDir, ".gogent", "settings.local.json")},
		{"ProjectAgentMDPath", ProjectAgentMDPath, filepath.Join(workDir, ".gogent", "AGENT.md")},
		{"LocalAgentMDPath", LocalAgentMDPath, filepath.Join(workDir, ".gogent", "AGENT.local.md")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(workDir)
			if got != tt.want {
				t.Errorf("%s(%q) = %q, want %q", tt.name, workDir, got, tt.want)
			}
		})
	}
}

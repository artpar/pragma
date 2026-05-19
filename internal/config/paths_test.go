package config

import (
	"path/filepath"
	"testing"
)

func TestPragmaHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	home, err := PragmaHome()
	if err != nil {
		t.Fatalf("PragmaHome: %v", err)
	}
	if home != filepath.Join(dir, ".pragma") {
		t.Errorf("PragmaHome = %q, want %q", home, filepath.Join(dir, ".pragma"))
	}
}

func TestSessionsDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	sessDir, err := SessionsDir()
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}
	expected := filepath.Join(dir, ".pragma", "sessions")
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
	expected := filepath.Join(dir, ".pragma", "AGENT.md")
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
		{"ProjectSettingsPath", ProjectSettingsPath, filepath.Join(workDir, ".pragma", "settings.json")},
		{"LocalSettingsPath", LocalSettingsPath, filepath.Join(workDir, ".pragma", "settings.local.json")},
		{"ProjectAgentMDPath", ProjectAgentMDPath, filepath.Join(workDir, ".pragma", "AGENT.md")},
		{"LocalAgentMDPath", LocalAgentMDPath, filepath.Join(workDir, ".pragma", "AGENT.local.md")},
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

func TestToolsetsPaths(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	workDir := "/tmp/myproject"

	global, err := GlobalToolsetsPath("json")
	if err != nil {
		t.Fatalf("GlobalToolsetsPath: %v", err)
	}
	if global != filepath.Join(dir, ".pragma", "toolsets.json") {
		t.Errorf("GlobalToolsetsPath = %q", global)
	}
	if got := ProjectToolsetsPath(workDir, "yaml"); got != filepath.Join(workDir, ".pragma", "toolsets.yaml") {
		t.Errorf("ProjectToolsetsPath = %q", got)
	}
	if got := LocalToolsetsPath(workDir, "json"); got != filepath.Join(workDir, ".pragma", "toolsets.local.json") {
		t.Errorf("LocalToolsetsPath = %q", got)
	}
}

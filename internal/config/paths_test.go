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

func TestProjectPaths(t *testing.T) {
	workDir := "/tmp/myproject"

	tests := []struct {
		name string
		fn   func(string) string
		want string
	}{
		{"ProjectSettingsPath", ProjectSettingsPath, filepath.Join(workDir, ".pragma", "settings.json")},
		{"LocalSettingsPath", LocalSettingsPath, filepath.Join(workDir, ".pragma", "settings.local.json")},
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

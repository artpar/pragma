package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		baseDir string
		want    string
	}{
		{"relative file", "foo.go", "/project", "/project/foo.go"},
		{"relative subdir", "src/main.go", "/project", "/project/src/main.go"},
		{"dot-relative", "./src/main.go", "/project", "/project/src/main.go"},
		{"parent-relative", "../other/file.go", "/project/sub", "/project/other/file.go"},
		{"absolute unchanged", "/absolute/path.go", "/project", "/absolute/path.go"},
		{"absolute cleaned", "/absolute//path.go", "/project", "/absolute/path.go"},
		{"tilde home", "~", "/project", home},
		{"tilde subpath", "~/docs/file.go", "/project", filepath.Join(home, "docs/file.go")},
		{"empty returns baseDir", "", "/project", "/project"},
		{"whitespace returns baseDir", "  ", "/project", "/project"},
		{"whitespace trimmed", "  foo.go  ", "/project", "/project/foo.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.path, tt.baseDir)
			if got != tt.want {
				t.Errorf("ExpandPath(%q, %q) = %q, want %q", tt.path, tt.baseDir, got, tt.want)
			}
		})
	}
}

func TestToRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		absPath string
		cwd     string
		want    string
	}{
		{"under cwd", "/project/src/main.go", "/project", "src/main.go"},
		{"same as cwd", "/project", "/project", "."},
		{"outside cwd", "/other/file.go", "/project", "/other/file.go"},
		{"deeply nested", "/project/a/b/c.go", "/project", "a/b/c.go"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToRelativePath(tt.absPath, tt.cwd)
			if got != tt.want {
				t.Errorf("ToRelativePath(%q, %q) = %q, want %q", tt.absPath, tt.cwd, got, tt.want)
			}
		})
	}
}

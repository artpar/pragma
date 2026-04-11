package permission

import (
	"github.com/artpar/gogent/internal/observe"
	"os"
	"path/filepath"
	"strings"
)

// DangerousFiles lists file names that require explicit permission to edit.
// These are sensitive configuration files that can affect system behavior.
var DangerousFiles = []string{
	".gitconfig",
	".bashrc",
	".zshrc",
	".profile",
	".bash_profile",
	".mcp.json",
	".pragma/settings.json",
	".env",
}

// DangerousFilePatterns lists glob patterns for dangerous files.
var DangerousFilePatterns = []string{
	".env.*",
}

// DangerousDirs lists directory names that require explicit permission.
var DangerousDirs = []string{
	".git",
	".vscode",
	".idea",
	".pragma",
	".gogent",
}

// IsDangerousPath checks if a file path is in a dangerous location.
// Returns true if the path matches a dangerous file, pattern, or directory.
func IsDangerousPath(absPath, workDir string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cleanPath := filepath.Clean(absPath)
	baseName := filepath.Base(cleanPath)

	for _, df := range DangerousFiles {
		observe.GlobalTrace("range DangerousFiles")
		if strings.EqualFold(baseName, df) {
			observe.GlobalTrace("if: strings.EqualFold(baseName, df)")
			observe.GlobalTrace("return: true")
			return true
		}
	}

	for _, pat := range DangerousFilePatterns {
		observe.GlobalTrace("range DangerousFilePatterns")
		if matched, _ := filepath.Match(pat, baseName); matched {
			observe.GlobalTrace("if: matched")
			observe.GlobalTrace("return: true")
			return true
		}
	}

	relPath := cleanPath
	if workDir != "" {
		observe.GlobalTrace("if: workDir != \"\"")
		if rel, err := filepath.Rel(workDir, cleanPath); err == nil {
			observe.GlobalTrace("if: err == nil")
			relPath = rel
		}
	}
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, part := range parts {
		observe.GlobalTrace("range parts")
		for _, dd := range DangerousDirs {
			observe.GlobalTrace("range DangerousDirs")
			if strings.EqualFold(part, dd) {
				observe.GlobalTrace("if: strings.EqualFold(part, dd)")
				observe.GlobalTrace("return: true")
				return true
			}
		}
	}
	observe.GlobalTrace("return: false")

	return false
}

// isFilePath returns true if a permission content string looks like a file path
// rather than a shell command or domain. File tools extract paths that start
// with "/" (absolute) or "~" (home-relative).
func isFilePath(content string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.HasPrefix(content, \"/\") || strings.HasPrefix(content, \"~\")")
	return strings.HasPrefix(content, "/") || strings.HasPrefix(content, "~")
}

// resolvePathsForCheck returns all paths that should be checked for permissions:
// the cleaned original path and, if it's a symlink, the resolved target.
// This prevents symlink-based permission bypasses (GitHub issues #5938, #23960, #10252).
func resolvePathsForCheck(content, workDir string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	absPath := content
	if !filepath.IsAbs(absPath) {
		observe.GlobalTrace("if: !filepath.IsAbs(absPath)")
		absPath = filepath.Join(workDir, absPath)
	}
	absPath = filepath.Clean(absPath)

	paths := []string{absPath}

	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil && resolved != absPath {
		observe.GlobalTrace("if: err == nil && resolved != absPath")
		paths = append(paths, resolved)
	}

	if os.IsNotExist(ignoreStat(absPath)) {
		observe.GlobalTrace("if: os.IsNotExist(ignoreStat(absPath))")
		parentResolved, err := filepath.EvalSymlinks(filepath.Dir(absPath))
		if err == nil {
			observe.GlobalTrace("if: err == nil")
			resolvedViaParent := filepath.Join(parentResolved, filepath.Base(absPath))
			if resolvedViaParent != absPath && resolvedViaParent != resolved {
				observe.GlobalTrace("if: resolvedViaParent != absPath && resolvedViaParent != resolved")
				paths = append(paths, resolvedViaParent)
			}
		}
	}
	observe.GlobalTrace("return: paths")

	return paths
}

// ignoreStat returns the error from os.Stat (for IsNotExist checks).
func ignoreStat(path string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	_, err := os.Stat(path)
	observe.GlobalTrace("return: err")
	return err
}

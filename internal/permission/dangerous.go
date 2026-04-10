package permission

import (
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
	cleanPath := filepath.Clean(absPath)
	baseName := filepath.Base(cleanPath)

	// Check dangerous files by exact name
	for _, df := range DangerousFiles {
		if strings.EqualFold(baseName, df) {
			return true
		}
	}

	// Check dangerous file patterns
	for _, pat := range DangerousFilePatterns {
		if matched, _ := filepath.Match(pat, baseName); matched {
			return true
		}
	}

	// Check if any path component is a dangerous directory
	relPath := cleanPath
	if workDir != "" {
		if rel, err := filepath.Rel(workDir, cleanPath); err == nil {
			relPath = rel
		}
	}
	parts := strings.Split(relPath, string(filepath.Separator))
	for _, part := range parts {
		for _, dd := range DangerousDirs {
			if strings.EqualFold(part, dd) {
				return true
			}
		}
	}

	return false
}

// isFilePath returns true if a permission content string looks like a file path
// rather than a shell command or domain. File tools extract paths that start
// with "/" (absolute) or "~" (home-relative).
func isFilePath(content string) bool {
	return strings.HasPrefix(content, "/") || strings.HasPrefix(content, "~")
}

// resolvePathsForCheck returns all paths that should be checked for permissions:
// the cleaned original path and, if it's a symlink, the resolved target.
// This prevents symlink-based permission bypasses (GitHub issues #5938, #23960, #10252).
func resolvePathsForCheck(content, workDir string) []string {
	absPath := content
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workDir, absPath)
	}
	absPath = filepath.Clean(absPath)

	paths := []string{absPath}

	// Resolve symlinks — EvalSymlinks follows the full chain.
	// Errors (file doesn't exist yet, permission denied) are ignored;
	// the original path is still checked.
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil && resolved != absPath {
		paths = append(paths, resolved)
	}

	// Also check if any parent directory is a symlink pointing elsewhere.
	// filepath.EvalSymlinks on the full path already handles this, but
	// for files that don't exist yet, check the parent.
	if os.IsNotExist(ignoreStat(absPath)) {
		parentResolved, err := filepath.EvalSymlinks(filepath.Dir(absPath))
		if err == nil {
			resolvedViaParent := filepath.Join(parentResolved, filepath.Base(absPath))
			if resolvedViaParent != absPath && resolvedViaParent != resolved {
				paths = append(paths, resolvedViaParent)
			}
		}
	}

	return paths
}

// ignoreStat returns the error from os.Stat (for IsNotExist checks).
func ignoreStat(path string) error {
	_, err := os.Stat(path)
	return err
}

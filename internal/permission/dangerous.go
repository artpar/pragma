package permission

import (
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

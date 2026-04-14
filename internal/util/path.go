package util

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath resolves a path to absolute. Handles:
//   - Empty/whitespace → returns baseDir
//   - "~" or "~/..." → expands to home directory
//   - Absolute paths → cleaned and returned
//   - Relative paths → joined with baseDir
//
// Mirrors the TS reference expandPath() in utils/path.ts.
func ExpandPath(path, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return filepath.Clean(baseDir)
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return filepath.Clean(baseDir)
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(baseDir, path)
}

// ToRelativePath converts an absolute path to relative from cwd.
// If the path is outside cwd (would start with ..), returns absolute unchanged.
// Mirrors TS toRelativePath() in utils/path.ts.
func ToRelativePath(absPath, cwd string) string {
	rel, err := filepath.Rel(cwd, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return absPath
	}
	return rel
}

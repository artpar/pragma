package permission

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/gogent/internal/observe"
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
// the cleaned original path, all intermediate symlink targets in the chain,
// and the final resolved path. This prevents symlink-based permission bypasses.
// Matches the TS reference getPathsForPermissionCheck() (fsOperations.ts:288-382).
func resolvePathsForCheck(content, workDir string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	absPath := content
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workDir, absPath)
	}
	absPath = filepath.Clean(absPath)

	seen := make(map[string]bool)
	var paths []string
	addPath := func(p string) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	// 1. Always check the original path
	addPath(absPath)

	// 2. Follow symlink chain for existing paths (collect intermediates)
	current := absPath
	visited := make(map[string]bool)
	for i := 0; i < 40; i++ { // max depth matches SYMLOOP_MAX
		if visited[current] {
			break // circular symlink
		}
		visited[current] = true

		info, err := os.Lstat(current)
		if err != nil {
			break // path doesn't exist — handled below
		}
		if info.Mode()&os.ModeSymlink == 0 {
			break // not a symlink
		}
		target, err := os.Readlink(current)
		if err != nil {
			break
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		target = filepath.Clean(target)
		addPath(target)
		current = target
	}

	// 3. For non-existent files: walk up ancestors to find symlinks
	if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
		if resolved := resolveDeepestExistingAncestor(absPath); resolved != "" {
			addPath(resolved)
		}
	}

	// 4. Final resolve via EvalSymlinks (catches directory-component symlinks)
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil && resolved != absPath {
		addPath(resolved)
	}

	return paths
}

// resolveDeepestExistingAncestor walks up from absPath using Lstat until it
// finds an existing component, then resolves symlinks there. This handles the
// case where a file doesn't exist but an ancestor directory is a symlink.
// Matches the TS resolveDeepestExistingAncestorSync() (fsOperations.ts:215-270).
func resolveDeepestExistingAncestor(absPath string) string {
	dir := absPath
	var segments []string
	for dir != filepath.Dir(dir) { // stop at root
		info, err := os.Lstat(dir)
		if err != nil {
			// Doesn't exist — accumulate segment, walk up
			segments = append([]string{filepath.Base(dir)}, segments...)
			dir = filepath.Dir(dir)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Found a symlink — resolve it
			resolved, err := filepath.EvalSymlinks(dir)
			if err != nil {
				// Dangling symlink — try readlink
				target, err := os.Readlink(dir)
				if err != nil {
					return ""
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(dir), target)
				}
				if len(segments) > 0 {
					return filepath.Join(append([]string{target}, segments...)...)
				}
				return target
			}
			if len(segments) > 0 {
				return filepath.Join(append([]string{resolved}, segments...)...)
			}
			return resolved
		}
		// Non-symlink exists — check if ancestors have symlinks via EvalSymlinks
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil && resolved != dir {
			if len(segments) > 0 {
				return filepath.Join(append([]string{resolved}, segments...)...)
			}
			return resolved
		}
		return "" // no symlinks found in ancestor chain
	}
	return ""
}

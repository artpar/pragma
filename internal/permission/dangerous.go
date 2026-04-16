package permission

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/observe"
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
	".pragma",
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
		observe.GlobalTrace("if: !filepath.IsAbs(absPath)")
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

	addPath(absPath)

	current := absPath
	visited := make(map[string]bool)
	for i := 0; i < 40; i++ {
		observe.GlobalTrace("for: i < 40")
		if visited[current] {
			observe.GlobalTrace("if: visited[current]")
			break
		}
		visited[current] = true

		info, err := os.Lstat(current)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			break
		}
		if info.Mode()&os.ModeSymlink == 0 {
			observe.GlobalTrace("if: info.Mode()&os.ModeSymlink == 0")
			break
		}
		target, err := os.Readlink(current)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			break
		}
		if !filepath.IsAbs(target) {
			observe.GlobalTrace("if: !filepath.IsAbs(target)")
			target = filepath.Join(filepath.Dir(current), target)
		}
		target = filepath.Clean(target)
		addPath(target)
		current = target
	}

	if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
		observe.GlobalTrace("if: errors.Is(err, os.ErrNotExist)")
		if resolved := resolveDeepestExistingAncestor(absPath); resolved != "" {
			observe.GlobalTrace("if: resolved != \"\"")
			addPath(resolved)
		}
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil && resolved != absPath {
		observe.GlobalTrace("if: err == nil && resolved != absPath")
		addPath(resolved)
	}
	observe.GlobalTrace("return: paths")

	return paths
}

// resolveDeepestExistingAncestor walks up from absPath using Lstat until it
// finds an existing component, then resolves symlinks there. This handles the
// case where a file doesn't exist but an ancestor directory is a symlink.
// Matches the TS resolveDeepestExistingAncestorSync() (fsOperations.ts:215-270).
func resolveDeepestExistingAncestor(absPath string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	dir := absPath
	var segments []string
	for dir != filepath.Dir(dir) {
		observe.GlobalTrace("for: dir != filepath.Dir(dir)")
		info, err := os.Lstat(dir)
		if err != nil {
			observe.GlobalTrace("if: err != nil")

			segments = append([]string{filepath.Base(dir)}, segments...)
			dir = filepath.Dir(dir)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			observe.GlobalTrace("if: info.Mode()&os.ModeSymlink != 0")

			resolved, err := filepath.EvalSymlinks(dir)
			if err != nil {
				observe.GlobalTrace("if: err != nil")

				target, err := os.Readlink(dir)
				if err != nil {
					observe.GlobalTrace("if: err != nil")
					observe.GlobalTrace("return: \"\"")
					return ""
				}
				if !filepath.IsAbs(target) {
					observe.GlobalTrace("if: !filepath.IsAbs(target)")
					target = filepath.Join(filepath.Dir(dir), target)
				}
				if len(segments) > 0 {
					observe.GlobalTrace("if: len(segments) > 0")
					observe.GlobalTrace("return: filepath.Join(append([]string{target}, segments...)...)")
					return filepath.Join(append([]string{target}, segments...)...)
				}
				observe.GlobalTrace("return: target")
				return target
			}
			if len(segments) > 0 {
				observe.GlobalTrace("if: len(segments) > 0")
				observe.GlobalTrace("return: filepath.Join(append([]string{resolved}, segments...)...)")
				return filepath.Join(append([]string{resolved}, segments...)...)
			}
			observe.GlobalTrace("return: resolved")
			return resolved
		}

		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil && resolved != dir {
			observe.GlobalTrace("if: err == nil && resolved != dir")
			if len(segments) > 0 {
				observe.GlobalTrace("if: len(segments) > 0")
				observe.GlobalTrace("return: filepath.Join(append([]string{resolved}, segments...)...)")
				return filepath.Join(append([]string{resolved}, segments...)...)
			}
			observe.GlobalTrace("return: resolved")
			return resolved
		}
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

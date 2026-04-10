package glob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

const maxResults = 100

// errMaxResults is a sentinel to stop GlobWalk early when we hit the cap.
var errMaxResults = errors.New("max results reached")

// GlobInput defines the parameters for the Glob tool.
type GlobInput struct {
	Pattern string `json:"pattern" desc:"Glob pattern to match files (e.g. **/*.go, src/**/*.ts)"`
	Path    string `json:"path,omitempty" desc:"Directory to search in. Defaults to working directory if omitted."`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["pattern"],
	"properties": {
		"pattern": {
			"type": "string",
			"description": "Glob pattern to match files (e.g. **/*.go, src/**/*.ts)"
		},
		"path": {
			"type": "string",
			"description": "Directory to search in. Defaults to working directory if omitted."
		}
	}
}`)

// Tool implements the Glob tool for file pattern matching.
type Tool struct{}

func (t *Tool) Name() string             { return "Glob" }
func (t *Tool) Description() string       { return "Fast file pattern matching tool that returns matching file paths sorted by modification time." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path,omitempty"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Pattern == "" {
		return checker.Check(ctx, "Glob", "")
	}
	// If an explicit absolute path is given, check permission against that path
	// so the permission system can enforce directory restrictions.
	if in.Path != "" && filepath.IsAbs(in.Path) {
		return checker.Check(ctx, "Glob", in.Path)
	}
	return checker.Check(ctx, "Glob", in.Pattern)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in GlobInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" {
		return tool.InvokeResult{}, fmt.Errorf("pattern is required")
	}

	baseDir := state.WorkDir()
	if in.Path != "" {
		if filepath.IsAbs(in.Path) {
			baseDir = in.Path
		} else {
			baseDir = filepath.Join(state.WorkDir(), in.Path)
		}
	}

	// Verify directory exists
	info, err := os.Stat(baseDir)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("directory not found: %s", baseDir)
	}
	if !info.IsDir() {
		return tool.InvokeResult{}, fmt.Errorf("not a directory: %s", baseDir)
	}

	// Walk the filesystem using doublestar
	type fileEntry struct {
		path    string
		modTime time.Time
	}
	var matches []fileEntry
	truncated := false

	fsys := os.DirFS(baseDir)
	err = doublestar.GlobWalk(fsys, in.Pattern, func(path string, d os.DirEntry) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Skip directories — only return files
		if d.IsDir() {
			return nil
		}
		// Skip hidden VCS dirs (doublestar uses "/" as separator)
		for _, seg := range strings.Split(path, "/") {
			switch seg {
			case ".git", ".svn", ".hg", ".bzr", ".jj", ".sl":
				return nil
			}
		}

		info, err := d.Info()
		var mt time.Time
		if err == nil {
			mt = info.ModTime()
		}
		matches = append(matches, fileEntry{path: path, modTime: mt})

		// Cap to avoid unbounded results
		if len(matches) >= maxResults {
			truncated = true
			return errMaxResults
		}
		return nil
	})
	if err != nil && !errors.Is(err, errMaxResults) && ctx.Err() == nil {
		return tool.InvokeResult{}, fmt.Errorf("glob error: %w", err)
	}

	// Sort by modification time (newest first), filename tiebreaker
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].modTime.Equal(matches[j].modTime) {
			return matches[i].path < matches[j].path
		}
		return matches[i].modTime.After(matches[j].modTime)
	})

	if len(matches) == 0 {
		return tool.InvokeResult{Content: "No files found"}, nil
	}

	// Build plain text output (filenames joined by newline, matching TS mapToolResultToToolResultBlockParam)
	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m.path)
		sb.WriteByte('\n')
	}
	if truncated {
		sb.WriteString("(Results are truncated. Consider using a more specific path or pattern.)\n")
	}

	return tool.InvokeResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

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

	"github.com/artpar/pragma/internal/util"
	"github.com/bmatcuk/doublestar/v4"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
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

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Glob\"")
	return "Glob"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Fast file pattern matching tool that works with any codebase size...\"")
	observe.GlobalTrace("return: globDescription")
	return globDescription
}

const globDescription = `- Fast file pattern matching tool that works with any codebase size
- Supports glob patterns like "**/*.js" or "src/**/*.ts"
- Returns matching file paths sorted by modification time
- Use this tool when you need to find files by name patterns
- When you are doing an open ended search that may require multiple rounds of globbing and grepping, use the Agent tool instead
- If no files match, try broader patterns: widen from a specific directory to **, try alternative extensions, or use Grep to search file content instead of filenames
- You can call multiple tools in a single response. It is always better to speculatively perform multiple searches in parallel if they are potentially useful.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "exit")
	var in struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path,omitempty"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.Pattern == "" {
		observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "if: err != nil || in.Pattern == \"\"")
		observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "return: checker.Check(ctx, \"Glob\", \"\")")
		return checker.Check(ctx, "Glob", "")
	}

	if in.Path != "" && filepath.IsAbs(in.Path) {
		observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "if: in.Path != \"\" && filepath.IsAbs(in.Path)")
		observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "return: checker.Check(ctx, \"Glob\", in.Path)")
		return checker.Check(ctx, "Glob", in.Path)
	}
	observe.TraceCtx(ctx, "glob", "Tool.CheckPerm", "return: checker.Check(ctx, \"Glob\", in.Pattern)")
	return checker.Check(ctx, "Glob", in.Pattern)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "glob", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "glob", "Tool.Invoke", "exit")
	var in GlobInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: in.Pattern == \"\"")
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"pattern is required\")")
		return tool.InvokeResult{}, fmt.Errorf("pattern is required")
	}

	baseDir := state.WorkDir()
	if in.Path != "" {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: in.Path != \"\"")
		baseDir = util.ExpandPath(in.Path, state.WorkDir())
	}

	info, err := os.Stat(baseDir)
	if err != nil {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"directory not found: %s\", baseDir)")
		return tool.InvokeResult{}, fmt.Errorf("directory not found: %s", baseDir)
	}
	if !info.IsDir() {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: !info.IsDir()")
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"not a directory: %s\", baseDir)")
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

		if d.IsDir() {
			return nil
		}

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

		if len(matches) >= maxResults {
			truncated = true
			return errMaxResults
		}
		return nil
	})
	if err != nil && !errors.Is(err, errMaxResults) && ctx.Err() == nil {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: err != nil && !errors.Is(err, errMaxResults) && ctx.Err() == nil")
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"glob error: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("glob error: %w", err)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].modTime.Equal(matches[j].modTime) {
			return matches[i].path < matches[j].path
		}
		return matches[i].modTime.After(matches[j].modTime)
	})

	if len(matches) == 0 {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: len(matches) == 0")
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{Content: \"No files found\"}, nil")
		return tool.InvokeResult{Content: "No files found"}, nil
	}

	// Build plain text output (filenames joined by newline, matching TS mapToolResultToToolResultBlockParam)
	var sb strings.Builder
	for _, m := range matches {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "range matches")
		sb.WriteString(m.path)
		sb.WriteByte('\n')
	}
	if truncated {
		observe.TraceCtx(ctx, "glob", "Tool.Invoke", "if: truncated")
		sb.WriteString("(Results are truncated. Consider using a more specific path or pattern.)\n")
	}
	observe.TraceCtx(ctx, "glob", "Tool.Invoke", "return: tool.InvokeResult{Content: strings.TrimRight(sb.String(), \"\\n\")}, nil")

	return tool.InvokeResult{Content: strings.TrimRight(sb.String(), "\n")}, nil
}

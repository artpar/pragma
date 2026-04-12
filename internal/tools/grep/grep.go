package grep

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

const defaultHeadLimit = 250

// VCS directories excluded from searches.
var vcsDirsToExclude = []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}

// GrepInput defines the parameters for the Grep tool.
type GrepInput struct {
	Pattern    string `json:"pattern" desc:"Regular expression pattern to search for in file contents"`
	Path       string `json:"path,omitempty" desc:"File or directory to search in. Defaults to current working directory."`
	Glob       string `json:"glob,omitempty" desc:"Glob pattern to filter files (e.g. *.js, *.{ts,tsx})"`
	OutputMode string `json:"output_mode,omitempty" desc:"Output mode: content, files_with_matches (default), or count"`
	Before     *int   `json:"-B,omitempty" desc:"Lines to show before each match (content mode only)"`
	After      *int   `json:"-A,omitempty" desc:"Lines to show after each match (content mode only)"`
	ContextC   *int   `json:"-C,omitempty" desc:"Alias for context lines"`
	Context    *int   `json:"context,omitempty" desc:"Lines to show before and after each match (content mode only)"`
	LineNums   *bool  `json:"-n,omitempty" desc:"Show line numbers (content mode, default true)"`
	CaseInsens *bool  `json:"-i,omitempty" desc:"Case insensitive search"`
	FileType   string `json:"type,omitempty" desc:"File type to search (e.g. js, py, go)"`
	HeadLimit  *int   `json:"head_limit,omitempty" desc:"Limit output to first N entries. Default 250, pass 0 for unlimited."`
	Offset     *int   `json:"offset,omitempty" desc:"Skip first N entries before applying head_limit"`
	Multiline  *bool  `json:"multiline,omitempty" desc:"Enable multiline matching where . matches newlines"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["pattern"],
	"properties": {
		"pattern": {"type": "string", "description": "The regular expression pattern to search for in file contents"},
		"path": {"type": "string", "description": "File or directory to search in. Defaults to current working directory."},
		"glob": {"type": "string", "description": "Glob pattern to filter files (e.g. \"*.js\", \"*.{ts,tsx}\")"},
		"output_mode": {"type": "string", "enum": ["content", "files_with_matches", "count"], "description": "Output mode. Defaults to files_with_matches."},
		"-B": {"type": "number", "description": "Lines before each match (content mode only)"},
		"-A": {"type": "number", "description": "Lines after each match (content mode only)"},
		"-C": {"type": "number", "description": "Alias for context"},
		"context": {"type": "number", "description": "Lines before and after each match (content mode only)"},
		"-n": {"type": "boolean", "description": "Show line numbers (content mode, default true)"},
		"-i": {"type": "boolean", "description": "Case insensitive search"},
		"type": {"type": "string", "description": "File type to search (e.g. js, py, go)"},
		"head_limit": {"type": "number", "description": "Limit output to first N entries. Default 250, 0 for unlimited."},
		"offset": {"type": "number", "description": "Skip first N entries before applying head_limit"},
		"multiline": {"type": "boolean", "description": "Enable multiline mode"}
	}
}`)

// Tool implements the Grep tool for content search.
type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Grep\"")
	observe.GlobalTrace("return: \"Grep\"")
	observe.GlobalTrace("return: \"Grep\"")
	return "Grep"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"A powerful search tool built on ripgrep...\"")
	observe.GlobalTrace("return: grepDescription")
	observe.GlobalTrace("return: grepDescription")
	return grepDescription
}

const grepDescription = `A powerful search tool built on ripgrep

  Usage:
  - ALWAYS use Grep for search tasks. NEVER invoke ` + "`grep`" + ` or ` + "`rg`" + ` as a Bash command. The Grep tool has been optimized for correct permissions and access.
  - Supports full regex syntax (e.g., "log.*Error", "function\\s+\\w+")
  - Filter files with glob parameter (e.g., "*.js", "**/*.tsx") or type parameter (e.g., "js", "py", "rust")
  - Output modes: "content" shows matching lines, "files_with_matches" shows only file paths (default), "count" shows match counts
  - Use Agent tool for open-ended searches requiring multiple rounds
  - Pattern syntax: Uses ripgrep (not grep) - literal braces need escaping (use ` + "`interface\\{\\}`" + ` to find ` + "`interface{}`" + ` in Go code)
  - Multiline matching: By default patterns match within single lines only. For cross-line patterns like ` + "`struct \\{[\\s\\S]*?field`" + `, use ` + "`multiline: true`" + `
`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "exit")
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Grep\", \"\")")
		observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Grep\", \"\")")
		observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Grep\", \"\")")
		return checker.Check(ctx, "Grep", "")
	}
	observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Grep\", in.Path)")
	observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Grep\", in.Path)")
	observe.TraceCtx(ctx, "grep", "Tool.CheckPerm", "return: checker.Check(ctx, \"Grep\", in.Path)")
	return checker.Check(ctx, "Grep", in.Path)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "grep", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "grep", "Tool.Invoke", "exit")
	var in GrepInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.Pattern == \"\"")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"pattern is required\")")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"pattern is required\")")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"pattern is required\")")
		return tool.InvokeResult{}, fmt.Errorf("pattern is required")
	}

	outputMode := in.OutputMode
	if outputMode == "" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: outputMode == \"\"")
		outputMode = "files_with_matches"
	}

	searchPath := state.WorkDir()
	if in.Path != "" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.Path != \"\"")
		if filepath.IsAbs(in.Path) {
			observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: filepath.IsAbs(in.Path)")
			searchPath = in.Path
		} else {
			observe.TraceCtx(ctx, "grep", "Tool.Invoke", "else: filepath.IsAbs(in.Path)")
			searchPath = filepath.Join(state.WorkDir(), in.Path)
		}
	}

	if _, err := os.Stat(searchPath); err != nil {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"path not found: %s\", searchPath)")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"path not found: %s\", searchPath)")
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"path not found: %s\", searchPath)")
		return tool.InvokeResult{}, fmt.Errorf("path not found: %s", searchPath)
	}

	args := []string{"--hidden"}

	for _, dir := range vcsDirsToExclude {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "range vcsDirsToExclude")
		args = append(args, "--glob", "!"+dir)
	}

	args = append(args, "--max-columns", "500")

	if in.Multiline != nil && *in.Multiline {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.Multiline != nil && *in.Multiline")
		args = append(args, "-U", "--multiline-dotall")
	}

	if in.CaseInsens != nil && *in.CaseInsens {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.CaseInsens != nil && *in.CaseInsens")
		args = append(args, "-i")
	}

	switch outputMode {
	case "files_with_matches":
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "case: \"files_with_matches\"")
		args = append(args, "-l")
	case "count":
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "case: \"count\"")
		args = append(args, "-c")
	}

	showLineNums := true
	if in.LineNums != nil {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.LineNums != nil")
		showLineNums = *in.LineNums
	}
	if showLineNums && outputMode == "content" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: showLineNums && outputMode == \"content\"")
		args = append(args, "-n")
	}

	if outputMode == "content" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: outputMode == \"content\"")
		if in.Context != nil {
			observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.Context != nil")
			args = append(args, "-C", strconv.Itoa(*in.Context))
		} else if in.ContextC != nil {
			observe.TraceCtx(ctx, "grep", "Tool.Invoke", "else-if: in.ContextC != nil")
			args = append(args, "-C", strconv.Itoa(*in.ContextC))
		} else {
			if in.Before != nil {
				args = append(args, "-B", strconv.Itoa(*in.Before))
			}
			if in.After != nil {
				args = append(args, "-A", strconv.Itoa(*in.After))
			}
		}
	}

	if strings.HasPrefix(in.Pattern, "-") {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: strings.HasPrefix(in.Pattern, \"-\")")
		args = append(args, "-e", in.Pattern)
	} else {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "else: strings.HasPrefix(in.Pattern, \"-\")")
		args = append(args, in.Pattern)
	}

	if in.FileType != "" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.FileType != \"\"")
		args = append(args, "--type", in.FileType)
	}

	if in.Glob != "" {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.Glob != \"\"")
		args = append(args, "--glob", in.Glob)
	}

	args = append(args, searchPath)

	cmd := exec.CommandContext(ctx, "rg", args...)
	out, err := cmd.Output()

	if err != nil {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: err != nil")
		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: ok")
			if exitErr.ExitCode() == 1 {
				observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: exitErr.ExitCode() == 1")

				out = nil
			} else if exitErr.ExitCode() == 2 {
				observe.TraceCtx(ctx, "grep", "Tool.Invoke", "else-if: exitErr.ExitCode() == 2")
				return tool.InvokeResult{}, fmt.Errorf("ripgrep error: %s", string(exitErr.Stderr))
			}
		} else if ctx.Err() != nil {
			observe.TraceCtx(ctx, "grep", "Tool.Invoke", "else-if: ctx.Err() != nil")
			return tool.InvokeResult{}, ctx.Err()
		} else {
			return tool.InvokeResult{}, fmt.Errorf("ripgrep: %w", err)
		}
	}

	// Parse results
	var lines []string
	if len(out) > 0 {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: len(out) > 0")
		raw := strings.TrimRight(string(out), "\n")
		lines = strings.Split(raw, "\n")
	}

	headLimit := defaultHeadLimit
	if in.HeadLimit != nil {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.HeadLimit != nil")
		headLimit = *in.HeadLimit
	}
	offset := 0
	if in.Offset != nil {
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "if: in.Offset != nil")
		offset = *in.Offset
	}

	workDir := state.WorkDir()

	switch outputMode {
	case "content":
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "case: \"content\"")
		return tool.InvokeResult{Content: buildContentResult(lines, headLimit, offset, workDir)}, nil
	case "count":
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "case: \"count\"")
		return tool.InvokeResult{Content: buildCountResult(lines, headLimit, offset, workDir)}, nil
	default:
		observe.TraceCtx(ctx, "grep", "Tool.Invoke", "default")
		return tool.InvokeResult{Content: buildFilesResult(lines, headLimit, offset, workDir)}, nil
	}
}

// applyHeadLimit applies offset+limit pagination. headLimit=0 means unlimited.
func applyHeadLimit(items []string, headLimit, offset int) (result []string, appliedLimit *int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if offset > 0 {
		observe.GlobalTrace("if: offset > 0")
		if offset >= len(items) {
			observe.GlobalTrace("if: offset >= len(items)")
			observe.GlobalTrace("return: nil, nil")
			observe.GlobalTrace("return: nil, nil")
			observe.GlobalTrace("return: nil, nil")
			return nil, nil
		}
		items = items[offset:]
	}
	if headLimit == 0 {
		observe.GlobalTrace("if: headLimit == 0")
		observe.GlobalTrace("return: items, nil")
		observe.GlobalTrace("return: items, nil")
		observe.GlobalTrace("return: items, nil")
		return items, nil
	}
	if len(items) > headLimit {
		observe.GlobalTrace("if: len(items) > headLimit")
		truncated := headLimit
		observe.GlobalTrace("return: items[:headLimit], &truncated")
		observe.GlobalTrace("return: items[:headLimit], &truncated")
		observe.GlobalTrace("return: items[:headLimit], &truncated")
		return items[:headLimit], &truncated
	}
	observe.GlobalTrace("return: items, nil")
	observe.GlobalTrace("return: items, nil")
	observe.GlobalTrace("return: items, nil")
	return items, nil
}

func relativizePath(absPath, workDir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	rel, err := filepath.Rel(workDir, absPath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: absPath")
		observe.GlobalTrace("return: absPath")
		observe.GlobalTrace("return: absPath")
		return absPath
	}
	observe.GlobalTrace("return: rel")
	observe.GlobalTrace("return: rel")
	observe.GlobalTrace("return: rel")
	return rel
}

func buildContentResult(lines []string, headLimit, offset int, workDir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	limited, appliedLimit := applyHeadLimit(lines, headLimit, offset)

	// Relativize paths in each line
	var result []string
	for _, line := range limited {
		observe.GlobalTrace("range limited")
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			observe.GlobalTrace("if: colonIdx > 0")
			path := line[:colonIdx]
			rest := line[colonIdx:]
			result = append(result, relativizePath(path, workDir)+rest)
		} else {
			observe.GlobalTrace("else: colonIdx > 0")
			result = append(result, line)
		}
	}

	out := strings.Join(result, "\n")
	if out == "" {
		observe.GlobalTrace("if: out == \"\"")
		out = "No matches found"
	}

	if appliedLimit != nil {
		observe.GlobalTrace("if: appliedLimit != nil")
		out += fmt.Sprintf("\n\n[Showing results with pagination = limit: %d", *appliedLimit)
		if offset > 0 {
			observe.GlobalTrace("if: offset > 0")
			out += fmt.Sprintf(", offset: %d", offset)
		}
		out += "]"
	} else if offset > 0 {
		observe.GlobalTrace("else-if: offset > 0")
		out += fmt.Sprintf("\n\n[Showing results with pagination = offset: %d]", offset)
	}
	observe.GlobalTrace("return: out")
	observe.GlobalTrace("return: out")
	observe.GlobalTrace("return: out")

	return out
}

func buildCountResult(lines []string, headLimit, offset int, workDir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	limited, appliedLimit := applyHeadLimit(lines, headLimit, offset)

	var totalMatches, fileCount int
	var countLines []string
	for _, line := range limited {
		observe.GlobalTrace("range limited")
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx > 0 {
			observe.GlobalTrace("if: colonIdx > 0")
			path := line[:colonIdx]
			countStr := line[colonIdx+1:]
			count, err := strconv.Atoi(strings.TrimSpace(countStr))
			if err == nil {
				observe.GlobalTrace("if: err == nil")
				totalMatches += count
				fileCount++
			}
			countLines = append(countLines, relativizePath(path, workDir)+":"+countStr)
		} else {
			observe.GlobalTrace("else: colonIdx > 0")
			countLines = append(countLines, line)
		}
	}

	out := strings.Join(countLines, "\n")
	if out == "" {
		observe.GlobalTrace("if: out == \"\"")
		out = "No matches found"
	}

	occurrences := "occurrences"
	if totalMatches == 1 {
		observe.GlobalTrace("if: totalMatches == 1")
		occurrences = "occurrence"
	}
	files := "files"
	if fileCount == 1 {
		observe.GlobalTrace("if: fileCount == 1")
		files = "file"
	}
	summary := fmt.Sprintf("\n\nFound %d total %s across %d %s.", totalMatches, occurrences, fileCount, files)
	if appliedLimit != nil {
		observe.GlobalTrace("if: appliedLimit != nil")
		summary += fmt.Sprintf(" with pagination = limit: %d", *appliedLimit)
		if offset > 0 {
			observe.GlobalTrace("if: offset > 0")
			summary += fmt.Sprintf(", offset: %d", offset)
		}
	} else if offset > 0 {
		observe.GlobalTrace("else-if: offset > 0")
		summary += fmt.Sprintf(" with pagination = offset: %d", offset)
	}
	observe.GlobalTrace("return: out + summary")
	observe.GlobalTrace("return: out + summary")
	observe.GlobalTrace("return: out + summary")

	return out + summary
}

func buildFilesResult(lines []string, headLimit, offset int, workDir string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(lines) == 0 {
		observe.GlobalTrace("if: len(lines) == 0")
		observe.GlobalTrace("return: \"No files found\"")
		observe.GlobalTrace("return: \"No files found\"")
		observe.GlobalTrace("return: \"No files found\"")
		return "No files found"
	}

	// Stat each file for mtime, sort newest first
	type fileWithMtime struct {
		path    string
		mtimeMs int64
	}
	var files []fileWithMtime
	for _, line := range lines {
		observe.GlobalTrace("range lines")
		line = strings.TrimSpace(line)
		if line == "" {
			observe.GlobalTrace("if: line == \"\"")
			continue
		}
		var mtimeMs int64
		if info, err := os.Stat(line); err == nil {
			observe.GlobalTrace("if: err == nil")
			mtimeMs = info.ModTime().UnixMilli()
		}
		files = append(files, fileWithMtime{path: line, mtimeMs: mtimeMs})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].mtimeMs == files[j].mtimeMs {
			return files[i].path < files[j].path
		}
		return files[i].mtimeMs > files[j].mtimeMs
	})

	sorted := make([]string, len(files))
	for i, f := range files {
		observe.GlobalTrace("range files")
		sorted[i] = f.path
	}

	limited, appliedLimit := applyHeadLimit(sorted, headLimit, offset)

	relative := make([]string, len(limited))
	for i, p := range limited {
		observe.GlobalTrace("range limited")
		relative[i] = relativizePath(p, workDir)
	}

	numFiles := len(relative)
	filesWord := "files"
	if numFiles == 1 {
		observe.GlobalTrace("if: numFiles == 1")
		filesWord = "file"
	}

	header := fmt.Sprintf("Found %d %s", numFiles, filesWord)
	if appliedLimit != nil {
		observe.GlobalTrace("if: appliedLimit != nil")
		header += fmt.Sprintf(" limit: %d", *appliedLimit)
		if offset > 0 {
			observe.GlobalTrace("if: offset > 0")
			header += fmt.Sprintf(" offset: %d", offset)
		}
	} else if offset > 0 {
		observe.GlobalTrace("else-if: offset > 0")
		header += fmt.Sprintf(" offset: %d", offset)
	}
	observe.GlobalTrace("return: header + \"\\n\" + strings.Join(relative, \"\\n\")")
	observe.GlobalTrace("return: header + \"\\n\" + strings.Join(relative, \"\\n\")")
	observe.GlobalTrace("return: header + \"\\n\" + strings.Join(relative, \"\\n\")")

	return header + "\n" + strings.Join(relative, "\n")
}

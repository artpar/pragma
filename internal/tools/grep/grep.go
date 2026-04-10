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

func (t *Tool) Name() string                { return "Grep" }
func (t *Tool) Description() string          { return "Search file contents using regular expressions (powered by ripgrep)." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return checker.Check(ctx, "Grep", "")
	}
	return checker.Check(ctx, "Grep", in.Path)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in GrepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Pattern == "" {
		return tool.InvokeResult{}, fmt.Errorf("pattern is required")
	}

	outputMode := in.OutputMode
	if outputMode == "" {
		outputMode = "files_with_matches"
	}

	searchPath := state.WorkDir()
	if in.Path != "" {
		if filepath.IsAbs(in.Path) {
			searchPath = in.Path
		} else {
			searchPath = filepath.Join(state.WorkDir(), in.Path)
		}
	}

	// Verify path exists
	if _, err := os.Stat(searchPath); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("path not found: %s", searchPath)
	}

	// Build ripgrep args
	args := []string{"--hidden"}

	// Exclude VCS directories
	for _, dir := range vcsDirsToExclude {
		args = append(args, "--glob", "!"+dir)
	}

	// Max columns to prevent base64/minified content clutter
	args = append(args, "--max-columns", "500")

	// Multiline
	if in.Multiline != nil && *in.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}

	// Case insensitive
	if in.CaseInsens != nil && *in.CaseInsens {
		args = append(args, "-i")
	}

	// Output mode flags
	switch outputMode {
	case "files_with_matches":
		args = append(args, "-l")
	case "count":
		args = append(args, "-c")
	}

	// Line numbers in content mode
	showLineNums := true
	if in.LineNums != nil {
		showLineNums = *in.LineNums
	}
	if showLineNums && outputMode == "content" {
		args = append(args, "-n")
	}

	// Context flags (context/-C takes precedence)
	if outputMode == "content" {
		if in.Context != nil {
			args = append(args, "-C", strconv.Itoa(*in.Context))
		} else if in.ContextC != nil {
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

	// Pattern (use -e if starts with -)
	if strings.HasPrefix(in.Pattern, "-") {
		args = append(args, "-e", in.Pattern)
	} else {
		args = append(args, in.Pattern)
	}

	// File type filter
	if in.FileType != "" {
		args = append(args, "--type", in.FileType)
	}

	// Glob filter
	if in.Glob != "" {
		args = append(args, "--glob", in.Glob)
	}

	// Search path
	args = append(args, searchPath)

	// Execute ripgrep
	cmd := exec.CommandContext(ctx, "rg", args...)
	out, err := cmd.Output()
	// rg exits 1 for "no matches" — that's not an error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				// No matches
				out = nil
			} else if exitErr.ExitCode() == 2 {
				return tool.InvokeResult{}, fmt.Errorf("ripgrep error: %s", string(exitErr.Stderr))
			}
		} else if ctx.Err() != nil {
			return tool.InvokeResult{}, ctx.Err()
		} else {
			return tool.InvokeResult{}, fmt.Errorf("ripgrep: %w", err)
		}
	}

	// Parse results
	var lines []string
	if len(out) > 0 {
		raw := strings.TrimRight(string(out), "\n")
		lines = strings.Split(raw, "\n")
	}

	// Compute head_limit and offset
	headLimit := defaultHeadLimit
	if in.HeadLimit != nil {
		headLimit = *in.HeadLimit
	}
	offset := 0
	if in.Offset != nil {
		offset = *in.Offset
	}

	workDir := state.WorkDir()

	switch outputMode {
	case "content":
		return tool.InvokeResult{Content: buildContentResult(lines, headLimit, offset, workDir)}, nil
	case "count":
		return tool.InvokeResult{Content: buildCountResult(lines, headLimit, offset, workDir)}, nil
	default:
		return tool.InvokeResult{Content: buildFilesResult(lines, headLimit, offset, workDir)}, nil
	}
}

// applyHeadLimit applies offset+limit pagination. headLimit=0 means unlimited.
func applyHeadLimit(items []string, headLimit, offset int) (result []string, appliedLimit *int) {
	if offset > 0 {
		if offset >= len(items) {
			return nil, nil
		}
		items = items[offset:]
	}
	if headLimit == 0 {
		return items, nil
	}
	if len(items) > headLimit {
		truncated := headLimit
		return items[:headLimit], &truncated
	}
	return items, nil
}

func relativizePath(absPath, workDir string) string {
	rel, err := filepath.Rel(workDir, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

func buildContentResult(lines []string, headLimit, offset int, workDir string) string {
	limited, appliedLimit := applyHeadLimit(lines, headLimit, offset)

	// Relativize paths in each line
	var result []string
	for _, line := range limited {
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			path := line[:colonIdx]
			rest := line[colonIdx:]
			result = append(result, relativizePath(path, workDir)+rest)
		} else {
			result = append(result, line)
		}
	}

	out := strings.Join(result, "\n")
	if out == "" {
		out = "No matches found"
	}

	if appliedLimit != nil {
		out += fmt.Sprintf("\n\n[Showing results with pagination = limit: %d", *appliedLimit)
		if offset > 0 {
			out += fmt.Sprintf(", offset: %d", offset)
		}
		out += "]"
	} else if offset > 0 {
		out += fmt.Sprintf("\n\n[Showing results with pagination = offset: %d]", offset)
	}

	return out
}

func buildCountResult(lines []string, headLimit, offset int, workDir string) string {
	limited, appliedLimit := applyHeadLimit(lines, headLimit, offset)

	var totalMatches, fileCount int
	var countLines []string
	for _, line := range limited {
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx > 0 {
			path := line[:colonIdx]
			countStr := line[colonIdx+1:]
			count, err := strconv.Atoi(strings.TrimSpace(countStr))
			if err == nil {
				totalMatches += count
				fileCount++
			}
			countLines = append(countLines, relativizePath(path, workDir)+":"+countStr)
		} else {
			countLines = append(countLines, line)
		}
	}

	out := strings.Join(countLines, "\n")
	if out == "" {
		out = "No matches found"
	}

	occurrences := "occurrences"
	if totalMatches == 1 {
		occurrences = "occurrence"
	}
	files := "files"
	if fileCount == 1 {
		files = "file"
	}
	summary := fmt.Sprintf("\n\nFound %d total %s across %d %s.", totalMatches, occurrences, fileCount, files)
	if appliedLimit != nil {
		summary += fmt.Sprintf(" with pagination = limit: %d", *appliedLimit)
		if offset > 0 {
			summary += fmt.Sprintf(", offset: %d", offset)
		}
	} else if offset > 0 {
		summary += fmt.Sprintf(" with pagination = offset: %d", offset)
	}

	return out + summary
}

func buildFilesResult(lines []string, headLimit, offset int, workDir string) string {
	if len(lines) == 0 {
		return "No files found"
	}

	// Stat each file for mtime, sort newest first
	type fileWithMtime struct {
		path    string
		mtimeMs int64
	}
	var files []fileWithMtime
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var mtimeMs int64
		if info, err := os.Stat(line); err == nil {
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

	// Convert to string slice for pagination
	sorted := make([]string, len(files))
	for i, f := range files {
		sorted[i] = f.path
	}

	limited, appliedLimit := applyHeadLimit(sorted, headLimit, offset)

	// Relativize paths
	relative := make([]string, len(limited))
	for i, p := range limited {
		relative[i] = relativizePath(p, workDir)
	}

	numFiles := len(relative)
	filesWord := "files"
	if numFiles == 1 {
		filesWord = "file"
	}

	header := fmt.Sprintf("Found %d %s", numFiles, filesWord)
	if appliedLimit != nil {
		header += fmt.Sprintf(" limit: %d", *appliedLimit)
		if offset > 0 {
			header += fmt.Sprintf(" offset: %d", offset)
		}
	} else if offset > 0 {
		header += fmt.Sprintf(" offset: %d", offset)
	}

	return header + "\n" + strings.Join(relative, "\n")
}

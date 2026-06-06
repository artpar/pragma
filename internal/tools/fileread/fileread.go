package fileread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/util"
)

const maxFileSize = 1024 * 1024 * 1024 // 1 GiB
const defaultLimit = 2000

// PDF limits matching TS reference (src/constants/apiLimits.ts)
const (
	pdfMaxRawSize      = 20 * 1024 * 1024 // 20 MB — after base64 encoding (~33% larger), must fit API limit
	pdfMaxPagesPerRead = 20
)

// Blocked device paths that would hang or produce infinite output.
var blockedDevicePaths = map[string]bool{
	"/dev/zero":    true,
	"/dev/random":  true,
	"/dev/urandom": true,
	"/dev/full":    true,
	"/dev/stdin":   true,
	"/dev/tty":     true,
	"/dev/console": true,
	"/dev/stdout":  true,
	"/dev/stderr":  true,
	"/dev/fd/0":    true,
	"/dev/fd/1":    true,
	"/dev/fd/2":    true,
}

// Image extensions returned as native ImagePart supplements.
var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// Common binary extensions (non-image, non-PDF).
var binaryExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".a": true,
	".o": true, ".obj": true, ".bin": true, ".class": true, ".jar": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
	".7z": true, ".rar": true, ".iso": true, ".dmg": true, ".deb": true,
	".rpm": true, ".msi": true, ".whl": true, ".pyc": true, ".pyo": true,
	".wasm": true, ".ttf": true, ".otf": true, ".woff": true, ".woff2": true,
	".eot": true, ".ico": true, ".mp3": true, ".mp4": true, ".avi": true,
	".mov": true, ".mkv": true, ".wav": true, ".flac": true, ".ogg": true,
	".db": true, ".sqlite": true, ".sqlite3": true,
}

// FileReadInput defines the parameters for the FileRead tool.
type FileReadInput struct {
	FilePath string  `json:"file_path" desc:"The absolute path to the file to read"`
	Offset   *int    `json:"offset,omitempty" desc:"Line number to start reading from (1-indexed)"`
	Limit    *int    `json:"limit,omitempty" desc:"Number of lines to read"`
	Pages    *string `json:"pages,omitempty" desc:"Page range for PDF files (e.g. 1-5, 3, 10-20). Maximum 20 pages per request."`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["file_path"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The absolute path to the file to read"
		},
		"offset": {
			"type": "number",
			"description": "The line number to start reading from (1-indexed). Only provide if the file is too large to read at once."
		},
		"limit": {
			"type": "number",
			"description": "The number of lines to read. Only provide if the file is too large to read at once."
		},
		"pages": {
			"type": "string",
			"description": "Page range for PDF files (e.g. \"1-5\", \"3\", \"10-20\"). Maximum 20 pages per request."
		}
	}
}`)

// Tool implements the FileRead tool.
type Tool struct {
	PreserveRepeatedContent bool
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Read\"")
	return "Read"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Reads a file from the local filesystem. You can access any file directly...\"")
	observe.GlobalTrace("return: readDescription")
	return readDescription
}

const readDescription = `Reads a file from the local filesystem.
The file_path parameter must be an absolute path.
By default, it reads up to 2000 lines starting from the beginning of the file.
Use offset and limit to read a portion of a text file.
Results are returned with line numbers starting at 1, formatted as: line_number→content.
Supports images, PDF page ranges, and Jupyter notebooks.`

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
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true, MaxResultSizeChars: -1}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true, MaxResultSizeChars: -1}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "exit")
	var in struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.FilePath == "" {
		observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "if: err != nil || in.FilePath == \"\"")
		observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "return: checker.Check(ctx, \"Read\", \"\")")
		return checker.Check(ctx, "Read", "")
	}
	observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "return: checker.Check(ctx, \"Read\", in.FilePath)")
	return checker.Check(ctx, "Read", in.FilePath)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "exit")
	var in FileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: in.FilePath == \"\"")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file_path is required\")")
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}

	filePath := util.ExpandPath(in.FilePath, state.WorkDir())

	if blockedDevicePaths[filePath] {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: blockedDevicePaths[filePath]")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"cannot read '%s': this device file would blo...")
		return tool.InvokeResult{}, fmt.Errorf("cannot read '%s': this device file would block or produce infinite output", in.FilePath)
	}
	if strings.HasPrefix(filePath, "/proc/") {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: strings.HasPrefix(filePath, \"/proc/\")")
		for _, suffix := range []string{"/fd/0", "/fd/1", "/fd/2"} {
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "range []string{\"/fd/0\", \"/fd/1\", \"/fd/2\"}")
			if strings.HasSuffix(filePath, suffix) {
				observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: strings.HasSuffix(filePath, suffix)")
				observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"cannot read '%s': this device file would blo...")
				return tool.InvokeResult{}, fmt.Errorf("cannot read '%s': this device file would block or produce infinite output", in.FilePath)
			}
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if _, isImage := imageExtensions[ext]; !isImage && ext != ".pdf" {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: !isImage && ext != \".pdf\"")
		if binaryExtensions[ext] {
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: binaryExtensions[ext]")
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"this tool cannot read binary files. The file...")
			return tool.InvokeResult{}, fmt.Errorf("this tool cannot read binary files. The file appears to be a binary %s file", ext)
		}
	}

	info, err := os.Stat(filePath)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: err != nil")
		if os.IsNotExist(err) {
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: os.IsNotExist(err)")
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file not found: %s. Make sure the path is co...")
			return tool.InvokeResult{}, fmt.Errorf("file not found: %s. Make sure the path is correct and the file exists.", in.FilePath)
		}
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"stat error: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("stat error: %w", err)
	}
	if info.IsDir() {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: info.IsDir()")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"path is a directory, not a file: %s. Use the...")
		return tool.InvokeResult{}, fmt.Errorf("path is a directory, not a file: %s. Use the Bash tool with ls or the Glob tool to list directory contents.", in.FilePath)
	}
	if info.Size() > maxFileSize {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: info.Size() > maxFileSize")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file is too large (%d bytes). Use offset and...")
		return tool.InvokeResult{}, fmt.Errorf("file is too large (%d bytes). Use offset and limit to read specific portions", info.Size())
	}

	if mimeType, isImage := imageExtensions[ext]; isImage {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: isImage")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: readImage(filePath, mimeType, info.Size())")
		return readImage(filePath, mimeType, info.Size())
	}

	if ext == ".pdf" {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: ext == \".pdf\"")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: readPDF(ctx, filePath, in.FilePath, info.Size(), in.Pages)")
		return readPDF(ctx, filePath, in.FilePath, info.Size(), in.Pages)
	}

	result, err := readTextFile(filePath, in.Offset, in.Limit)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	if cache, ok := tool.FileStateCacheFrom(state); ok {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: ok")
		if !t.PreserveRepeatedContent && previousReadUnchanged(cache, filePath, result, info.ModTime().UnixMilli()) {
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: found && previous.Offset != nil && sameOptionalInt(previous.Offset, result.Of...")
			observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{Content: \"File unchanged since last read. The content from ...")
			return tool.InvokeResult{Content: "File unchanged since last read."}, nil
		}
		cache.Set(filePath, tool.FileState{
			Content:   result.Content,
			Timestamp: info.ModTime().UnixMilli(),
			Offset:    result.Offset,
			Limit:     result.Limit,
		})
	}
	observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{Content: result.Output}, nil")
	return tool.InvokeResult{Content: result.Output}, nil
}

func previousReadUnchanged(cache *tool.FileStateCache, filePath string, result textReadResult, timestamp int64) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	previous, found := cache.Get(filePath)
	observe.GlobalTrace("return: found && previous.Offset != nil && sameOptionalInt(previous.Offset, result.Of...")
	return found && previous.Offset != nil && sameOptionalInt(previous.Offset, result.Offset) && sameOptionalInt(previous.Limit, result.Limit) && previous.Timestamp == timestamp
}

func readImage(filePath, mimeType string, size int64) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(filePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, fmt.Errorf(\"read image: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read image: %w", err)
	}
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"Image file: %s (%d bytes)\", filepat...")
	return tool.InvokeResult{
		Content: fmt.Sprintf("Image file: %s (%d bytes)", filepath.Base(filePath), size),
		Supplements: []model.ContentPart{
			model.ImagePart{
				MimeType: mimeType,
				Data:     data,
			},
		},
	}, nil
}

type textReadResult struct {
	Output  string
	Content string
	Offset  *int
	Limit   *int
}

func readTextFile(filePath string, offset, limit *int) (textReadResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(filePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"read file: %w\", err)")
		observe.GlobalTrace("return: textReadResult{}, fmt.Errorf(\"read file: %w\", err)")
		return textReadResult{}, fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	content = tool.NormalizeTextContent(content)

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if totalLines > 0 && lines[totalLines-1] == "" {
		observe.GlobalTrace("if: totalLines > 0 && lines[totalLines-1] == \"\"")
		lines = lines[:totalLines-1]
		totalLines = len(lines)
	}

	if totalLines == 0 {
		observe.GlobalTrace("if: totalLines == 0")
		observe.GlobalTrace("return: \"<system-reminder>Warning: the file exists but the contents are empty.</syste...")
		startLine := 1
		observe.GlobalTrace("return: textReadResult{\n\tOutput:\t\"<system-reminder>Warning: the file exists but the ...")
		return textReadResult{
			Output:  "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>",
			Content: content,
			Offset:  &startLine,
			Limit:   cloneIntPtr(limit),
		}, nil
	}

	startLine := 1
	if offset != nil {
		observe.GlobalTrace("if: offset != nil")
		if *offset < 1 {
			observe.GlobalTrace("if: *offset < 1")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"offset must be >= 1 (1-indexed), got %d\", *offset)")
			observe.GlobalTrace("return: textReadResult{}, fmt.Errorf(\"offset must be >= 1 (1-indexed), got %d\", *offset)")
			return textReadResult{}, fmt.Errorf("offset must be >= 1 (1-indexed), got %d", *offset)
		}
		startLine = *offset
	}

	if startLine > totalLines {
		observe.GlobalTrace("if: startLine > totalLines")
		observe.GlobalTrace("return: fmt.Sprintf(\"<system-reminder>Warning: the file exists but is shorter than th...")
		observe.GlobalTrace("return: textReadResult{\n\tOutput:\tfmt.Sprintf(\"<system-reminder>Warning: the file exi...")
		return textReadResult{
			Output:  fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", startLine, totalLines),
			Content: content,
			Offset:  &startLine,
			Limit:   cloneIntPtr(limit),
		}, nil
	}

	endLine := totalLines
	effectiveLimit := defaultLimit
	if limit != nil && *limit > 0 {
		observe.GlobalTrace("if: limit != nil && *limit > 0")
		effectiveLimit = *limit
	}
	if startLine-1+effectiveLimit < endLine {
		observe.GlobalTrace("if: startLine-1+effectiveLimit < endLine")
		endLine = startLine - 1 + effectiveLimit
	}

	selectedLines := lines[startLine-1 : endLine]
	numLines := len(selectedLines)
	viewContent := strings.Join(selectedLines, "\n")
	if startLine == 1 && endLine == totalLines {
		observe.GlobalTrace("if: startLine == 1 && endLine == totalLines")
		viewContent = content
	}

	// Add line numbers (number→content format)
	var sb strings.Builder
	maxLineNumWidth := len(fmt.Sprintf("%d", endLine))
	for i, line := range selectedLines {
		observe.GlobalTrace("range selectedLines")
		lineNum := startLine + i
		sb.WriteString(fmt.Sprintf("%*d→%s\n", maxLineNumWidth, lineNum, line))
	}

	result := sb.String()
	if numLines < totalLines {
		observe.GlobalTrace("if: numLines < totalLines")
		result += fmt.Sprintf("\n(%d lines total, showing lines %d-%d)", totalLines, startLine, endLine)
	}
	observe.GlobalTrace("return: result, nil")
	observe.GlobalTrace("return: textReadResult{\n\tOutput:\tresult,\n\tContent:\tviewContent,\n\tOffset:\t\t&startLine...")

	return textReadResult{
		Output:  result,
		Content: viewContent,
		Offset:  &startLine,
		Limit:   cloneIntPtr(limit),
	}, nil
}

func sameOptionalInt(a, b *int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if a == nil || b == nil {
		observe.GlobalTrace("if: a == nil || b == nil")
		observe.GlobalTrace("return: a == nil && b == nil")
		return a == nil && b == nil
	}
	observe.GlobalTrace("return: *a == *b")
	return *a == *b
}

func cloneIntPtr(v *int) *int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if v == nil {
		observe.GlobalTrace("if: v == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	out := *v
	observe.GlobalTrace("return: &out")
	return &out
}

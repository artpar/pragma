package fileread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
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
type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Read\"")
	return "Read"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Read a file from the local filesystem.\"")
	return "Read a file from the local filesystem."
}
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
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: in.FilePath == \"\"")
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}

	filePath := in.FilePath
	if !filepath.IsAbs(filePath) {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: !filepath.IsAbs(filePath)")
		filePath = filepath.Join(state.WorkDir(), filePath)
	}

	if blockedDevicePaths[filePath] {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: blockedDevicePaths[filePath]")
		return tool.InvokeResult{}, fmt.Errorf("cannot read '%s': this device file would block or produce infinite output", in.FilePath)
	}
	if strings.HasPrefix(filePath, "/proc/") {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: strings.HasPrefix(filePath, \"/proc/\")")
		for _, suffix := range []string{"/fd/0", "/fd/1", "/fd/2"} {
			if strings.HasSuffix(filePath, suffix) {
				return tool.InvokeResult{}, fmt.Errorf("cannot read '%s': this device file would block or produce infinite output", in.FilePath)
			}
		}
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if _, isImage := imageExtensions[ext]; !isImage && ext != ".pdf" {
		if binaryExtensions[ext] {
			return tool.InvokeResult{}, fmt.Errorf("this tool cannot read binary files. The file appears to be a binary %s file", ext)
		}
	}

	info, err := os.Stat(filePath)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: err != nil")
		if os.IsNotExist(err) {
			return tool.InvokeResult{}, fmt.Errorf("file not found: %s. Make sure the path is correct and the file exists.", in.FilePath)
		}
		return tool.InvokeResult{}, fmt.Errorf("stat error: %w", err)
	}
	if info.IsDir() {
		return tool.InvokeResult{}, fmt.Errorf("path is a directory, not a file: %s. Use the Bash tool with ls or the Glob tool to list directory contents.", in.FilePath)
	}
	if info.Size() > maxFileSize {
		return tool.InvokeResult{}, fmt.Errorf("file is too large (%d bytes). Use offset and limit to read specific portions", info.Size())
	}

	if mimeType, isImage := imageExtensions[ext]; isImage {
		return readImage(filePath, mimeType, info.Size())
	}

	if ext == ".pdf" {
		return readPDF(ctx, filePath, in.FilePath, info.Size(), in.Pages)
	}

	result, err := readTextFile(filePath, in.Offset, in.Limit)
	if err != nil {
		return tool.InvokeResult{}, err
	}
	return tool.InvokeResult{Content: result}, nil
}

func readImage(filePath, mimeType string, size int64) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("read image: %w", err)
	}
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

func readTextFile(filePath string, offset, limit *int) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	content = strings.ReplaceAll(content, "\r\n", "\n")

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if totalLines > 0 && lines[totalLines-1] == "" {
		lines = lines[:totalLines-1]
		totalLines = len(lines)
	}

	if totalLines == 0 {
		return "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", nil
	}

	startLine := 1
	if offset != nil {
		if *offset < 1 {
			return "", fmt.Errorf("offset must be >= 1 (1-indexed), got %d", *offset)
		}
		startLine = *offset
	}

	if startLine > totalLines {
		return fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", startLine, totalLines), nil
	}

	endLine := totalLines
	effectiveLimit := defaultLimit
	if limit != nil && *limit > 0 {
		effectiveLimit = *limit
	}
	if startLine-1+effectiveLimit < endLine {
		endLine = startLine - 1 + effectiveLimit
	}

	selectedLines := lines[startLine-1 : endLine]
	numLines := len(selectedLines)

	// Add line numbers (cat -n format)
	var sb strings.Builder
	maxLineNumWidth := len(fmt.Sprintf("%d", endLine))
	for i, line := range selectedLines {
		lineNum := startLine + i
		sb.WriteString(fmt.Sprintf("%*d\t%s\n", maxLineNumWidth, lineNum, line))
	}

	result := sb.String()
	if numLines < totalLines {
		result += fmt.Sprintf("\n(%d lines total, showing lines %d-%d)", totalLines, startLine, endLine)
	}

	return result, nil
}

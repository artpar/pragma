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

const readDescription = `Reads a file from the local filesystem. You can access any file directly by using this tool.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- When you already know which part of the file you need, only read that part. This can be important for larger files.
- Results are returned using cat -n format, with line numbers starting at 1
- This tool allows gogent to read images (eg PNG, JPG, etc). When reading an image file the contents are presented visually as gogent is a multimodal LLM.
- This tool can read PDF files (.pdf). For large PDFs (more than 10 pages), you MUST provide the pages parameter to read specific page ranges (e.g., pages: "1-5"). Reading a large PDF without the pages parameter will fail. Maximum 20 pages per request.
- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.
- This tool can only read files, not directories. To read a directory, use an ls command via the Bash tool.
- You can call multiple tools in a single response. It is always better to speculatively read multiple potentially useful files in parallel.
- You will regularly be asked to read screenshots. If the user provides a path to a screenshot, ALWAYS use this tool to view the file at the path. This tool will work with all temporary file paths.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.`

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}
func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
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
		observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "return: checker.Check(ctx, \"Read\", \"\")")
		return checker.Check(ctx, "Read", "")
	}
	observe.TraceCtx(ctx, "fileread", "Tool.CheckPerm", "return: checker.Check(ctx, \"Read\", in.FilePath)")
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

	filePath := in.FilePath
	if !filepath.IsAbs(filePath) {
		observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "if: !filepath.IsAbs(filePath)")
		filePath = filepath.Join(state.WorkDir(), filePath)
	}

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
	observe.TraceCtx(ctx, "fileread", "Tool.Invoke", "return: tool.InvokeResult{Content: result}, nil")
	return tool.InvokeResult{Content: result}, nil
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

func readTextFile(filePath string, offset, limit *int) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := os.ReadFile(filePath)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"read file: %w\", err)")
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	content = strings.ReplaceAll(content, "\r\n", "\n")

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
		return "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", nil
	}

	startLine := 1
	if offset != nil {
		observe.GlobalTrace("if: offset != nil")
		if *offset < 1 {
			observe.GlobalTrace("if: *offset < 1")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"offset must be >= 1 (1-indexed), got %d\", *offset)")
			return "", fmt.Errorf("offset must be >= 1 (1-indexed), got %d", *offset)
		}
		startLine = *offset
	}

	if startLine > totalLines {
		observe.GlobalTrace("if: startLine > totalLines")
		observe.GlobalTrace("return: fmt.Sprintf(\"<system-reminder>Warning: the file exists but is shorter than th...")
		return fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", startLine, totalLines), nil
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

	// Add line numbers (cat -n format)
	var sb strings.Builder
	maxLineNumWidth := len(fmt.Sprintf("%d", endLine))
	for i, line := range selectedLines {
		observe.GlobalTrace("range selectedLines")
		lineNum := startLine + i
		sb.WriteString(fmt.Sprintf("%*d\t%s\n", maxLineNumWidth, lineNum, line))
	}

	result := sb.String()
	if numLines < totalLines {
		observe.GlobalTrace("if: numLines < totalLines")
		result += fmt.Sprintf("\n(%d lines total, showing lines %d-%d)", totalLines, startLine, endLine)
	}
	observe.GlobalTrace("return: result, nil")

	return result, nil
}

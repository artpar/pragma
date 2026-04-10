package fileread

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

const maxFileSize = 1024 * 1024 * 1024 // 1 GiB
const defaultLimit = 2000

// PDF limits matching TS reference (src/constants/apiLimits.ts)
const (
	pdfMaxRawSize     = 20 * 1024 * 1024 // 20 MB — after base64 encoding (~33% larger), must fit API limit
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

// Image extensions that can be returned as base64.
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

func (t *Tool) Name() string                { return "Read" }
func (t *Tool) Description() string          { return "Read a file from the local filesystem." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.FilePath == "" {
		return checker.Check(ctx, "Read", "")
	}
	return checker.Check(ctx, "Read", in.FilePath)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in FileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}

	filePath := in.FilePath
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(state.WorkDir(), filePath)
	}

	// Block dangerous device paths
	if blockedDevicePaths[filePath] {
		return tool.InvokeResult{}, fmt.Errorf("cannot read '%s': this device file would block or produce infinite output", in.FilePath)
	}
	if strings.HasPrefix(filePath, "/proc/") {
		for _, suffix := range []string{"/fd/0", "/fd/1", "/fd/2"} {
			if strings.HasSuffix(filePath, suffix) {
				return tool.InvokeResult{}, fmt.Errorf("cannot read '%s': this device file would block or produce infinite output", in.FilePath)
			}
		}
	}

	// Check extension for binary files (images and PDFs are excluded from binary check)
	ext := strings.ToLower(filepath.Ext(filePath))
	if _, isImage := imageExtensions[ext]; !isImage && ext != ".pdf" {
		if binaryExtensions[ext] {
			return tool.InvokeResult{}, fmt.Errorf("this tool cannot read binary files. The file appears to be a binary %s file", ext)
		}
	}

	// Stat to check existence and size
	info, err := os.Stat(filePath)
	if err != nil {
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

	// Image handling
	if mimeType, isImage := imageExtensions[ext]; isImage {
		result, err := readImage(filePath, mimeType, info.Size())
		if err != nil {
			return tool.InvokeResult{}, err
		}
		return tool.InvokeResult{Content: result}, nil
	}

	// PDF handling
	if ext == ".pdf" {
		return readPDF(ctx, filePath, in.FilePath, info.Size(), in.Pages)
	}

	// Text file handling
	result, err := readTextFile(filePath, in.Offset, in.Limit)
	if err != nil {
		return tool.InvokeResult{}, err
	}
	return tool.InvokeResult{Content: result}, nil
}

func readImage(filePath, mimeType string, size int64) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("Image file: %s (%d bytes)\nbase64:%s:%s", filepath.Base(filePath), size, mimeType, encoded), nil
}

// readPDF reads a PDF file. For small PDFs, it sends the full PDF as a DocumentPart
// supplement so the provider can deliver it as a native document block. For specific
// page ranges, it uses pdftoppm to extract pages as images. Falls back to pdftotext
// for text extraction when native PDF support tools are unavailable.
func readPDF(ctx context.Context, filePath, displayPath string, size int64, pages *string) (tool.InvokeResult, error) {
	// Validate PDF header
	header := make([]byte, 5)
	f, err := os.Open(filePath)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("open PDF: %w", err)
	}
	n, err := f.Read(header)
	f.Close()
	if err != nil || n < 5 || string(header[:5]) != "%PDF-" {
		return tool.InvokeResult{}, fmt.Errorf("file is not a valid PDF (missing %%PDF- header): %s", displayPath)
	}

	// If pages parameter is specified, extract those pages
	if pages != nil && *pages != "" {
		return readPDFPages(ctx, filePath, displayPath, *pages)
	}

	// Size check for full PDF
	if size > pdfMaxRawSize {
		return tool.InvokeResult{}, fmt.Errorf(
			"PDF file is too large (%s). Maximum size for full PDF reading is %s. "+
				"Use the pages parameter to read specific page ranges (e.g., pages: \"1-5\"), "+
				"maximum %d pages per request.",
			formatSize(size), formatSize(pdfMaxRawSize), pdfMaxPagesPerRead)
	}

	// Read full PDF as base64, send as document supplement
	data, err := os.ReadFile(filePath)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("read PDF: %w", err)
	}

	return tool.InvokeResult{
		Content: fmt.Sprintf("PDF file read: %s (%s)", displayPath, formatSize(size)),
		Supplements: []model.ContentPart{
			model.DocumentPart{
				MimeType: "application/pdf",
				Data:     data,
			},
		},
	}, nil
}

// readPDFPages extracts specific pages from a PDF using pdftoppm (poppler-utils).
// Falls back to pdftotext if pdftoppm is unavailable.
func readPDFPages(ctx context.Context, filePath, displayPath string, pages string) (tool.InvokeResult, error) {
	first, last, err := parsePDFPageRange(pages)
	if err != nil {
		return tool.InvokeResult{}, err
	}

	pageCount := last - first + 1
	if last == -1 {
		pageCount = pdfMaxPagesPerRead + 1 // open-ended: will be validated after pdfinfo
	}
	if pageCount > pdfMaxPagesPerRead {
		return tool.InvokeResult{}, fmt.Errorf(
			"page range \"%s\" exceeds maximum of %d pages per request. Please use a smaller range.",
			pages, pdfMaxPagesPerRead)
	}

	// Try pdftoppm for page extraction as images
	if pdftoppmAvailable(ctx) {
		return extractPagesAsImages(ctx, filePath, displayPath, first, last)
	}

	// Fall back to pdftotext for the page range
	if pdftotextAvailable(ctx) {
		return extractPagesAsText(ctx, filePath, displayPath, first, last)
	}

	return tool.InvokeResult{}, fmt.Errorf(
		"PDF page extraction requires poppler-utils. Install with: "+
			"brew install poppler (macOS) or apt-get install poppler-utils (Debian/Ubuntu)")
}

func extractPagesAsImages(ctx context.Context, filePath, displayPath string, first, last int) (tool.InvokeResult, error) {
	dir, err := os.MkdirTemp("", "gogent-pdf-*")
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	args := []string{"-jpeg", "-r", "150"}
	if first > 0 {
		args = append(args, "-f", strconv.Itoa(first))
	}
	if last > 0 {
		args = append(args, "-l", strconv.Itoa(last))
	}
	args = append(args, filePath, filepath.Join(dir, "page"))

	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("pdftoppm failed: %s", strings.TrimSpace(string(out)))
	}

	// Read extracted JPEG files as image supplements
	entries, err := os.ReadDir(dir)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("read extracted pages: %w", err)
	}

	var supplements []model.ContentPart
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jpg") {
			continue
		}
		imgData, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		supplements = append(supplements, model.ImagePart{
			MimeType: "image/jpeg",
			Data:     imgData,
		})
	}

	if len(supplements) == 0 {
		return tool.InvokeResult{}, fmt.Errorf("no pages extracted from PDF")
	}

	return tool.InvokeResult{
		Content:     fmt.Sprintf("PDF pages extracted: %d page(s) from %s", len(supplements), displayPath),
		Supplements: supplements,
	}, nil
}

func extractPagesAsText(ctx context.Context, filePath, displayPath string, first, last int) (tool.InvokeResult, error) {
	args := []string{"-layout"}
	if first > 0 {
		args = append(args, "-f", strconv.Itoa(first))
	}
	if last > 0 {
		args = append(args, "-l", strconv.Itoa(last))
	}
	args = append(args, filePath, "-")

	cmd := exec.CommandContext(ctx, "pdftotext", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return tool.InvokeResult{}, fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return tool.InvokeResult{}, fmt.Errorf("pdftotext: %w", err)
	}

	content := strings.TrimRight(string(out), "\n")
	if content == "" {
		return tool.InvokeResult{Content: fmt.Sprintf("PDF %s: no text content found in pages %d-%d", displayPath, first, last)}, nil
	}

	return tool.InvokeResult{Content: content}, nil
}

// parsePDFPageRange parses "5", "1-10", or "3-" into first/last page numbers.
// Returns -1 for last if open-ended. Pages are 1-indexed.
func parsePDFPageRange(pages string) (first, last int, err error) {
	pages = strings.TrimSpace(pages)
	if pages == "" {
		return 0, 0, fmt.Errorf("empty pages parameter")
	}

	// Open-ended: "3-"
	if strings.HasSuffix(pages, "-") {
		f, err := strconv.Atoi(pages[:len(pages)-1])
		if err != nil || f < 1 {
			return 0, 0, fmt.Errorf("invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\"", pages)
		}
		return f, -1, nil
	}

	dashIdx := strings.Index(pages, "-")
	if dashIdx == -1 {
		// Single page: "5"
		p, err := strconv.Atoi(pages)
		if err != nil || p < 1 {
			return 0, 0, fmt.Errorf("invalid pages parameter: %q. Pages are 1-indexed", pages)
		}
		return p, p, nil
	}

	// Range: "1-10"
	f, err1 := strconv.Atoi(pages[:dashIdx])
	l, err2 := strconv.Atoi(pages[dashIdx+1:])
	if err1 != nil || err2 != nil || f < 1 || l < 1 || l < f {
		return 0, 0, fmt.Errorf("invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\"", pages)
	}
	return f, l, nil
}

func pdftoppmAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "pdftoppm", "-v")
	err := cmd.Run()
	return err == nil
}

func pdftotextAvailable(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "pdftotext", "-v")
	// pdftotext -v prints to stderr and exits 0 (or sometimes 99)
	out, err := cmd.CombinedOutput()
	return err == nil || len(out) > 0
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}

func readTextFile(filePath string, offset, limit *int) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(data)
	// Normalize CRLF to LF
	content = strings.ReplaceAll(content, "\r\n", "\n")

	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	// Remove trailing empty line from final newline
	if totalLines > 0 && lines[totalLines-1] == "" {
		lines = lines[:totalLines-1]
		totalLines = len(lines)
	}

	// Empty file warning
	if totalLines == 0 {
		return "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", nil
	}

	// Apply offset (1-indexed)
	startLine := 1
	if offset != nil && *offset > 0 {
		startLine = *offset
	}

	if startLine > totalLines {
		return fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", startLine, totalLines), nil
	}

	// Apply limit
	endLine := totalLines
	effectiveLimit := defaultLimit
	if limit != nil && *limit > 0 {
		effectiveLimit = *limit
	}
	if startLine-1+effectiveLimit < endLine {
		endLine = startLine - 1 + effectiveLimit
	}

	// Slice to the requested range
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

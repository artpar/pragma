package fileread

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// readPDF reads a PDF file. For small PDFs, it sends the full PDF as a DocumentPart
// supplement so the provider can deliver it as a native document block. For specific
// page ranges, it uses pdftoppm to extract pages as images. Falls back to pdftotext
// for text extraction when native PDF support tools are unavailable.
func readPDF(ctx context.Context, filePath, displayPath string, size int64, pages *string) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "fileread", "readPDF", "enter")
	defer observe.TraceCtx(ctx, "fileread", "readPDF", "exit")

	header := make([]byte, 5)
	f, err := os.Open(filePath)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"open PDF: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("open PDF: %w", err)
	}
	n, err := f.Read(header)
	f.Close()
	if err != nil || n < 5 || string(header[:5]) != "%PDF-" {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: err != nil || n < 5 || string(header[:5]) != \"%PDF-\"")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"file is not a valid PDF (missing %%PDF- head...")
		return tool.InvokeResult{}, fmt.Errorf("file is not a valid PDF (missing %%PDF- header): %s", displayPath)
	}

	if pages != nil && *pages != "" {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: pages != nil && *pages != \"\"")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: readPDFPages(ctx, filePath, displayPath, *pages)")
		return readPDFPages(ctx, filePath, displayPath, *pages)
	}

	if size > pdfMaxRawSize {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: size > pdfMaxRawSize")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"PDF file is too large (%s). Maximum size f...")
		return tool.InvokeResult{}, fmt.Errorf(
			"PDF file is too large (%s). Maximum size for full PDF reading is %s. "+
				"Use the pages parameter to read specific page ranges (e.g., pages: \"1-5\"), "+
				"maximum %d pages per request.",
			formatSize(size), formatSize(pdfMaxRawSize), pdfMaxPagesPerRead)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"read PDF: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read PDF: %w", err)
	}
	observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"PDF file read: %s (%s)\", displayPat...")

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
	observe.TraceCtx(ctx, "fileread", "readPDFPages", "enter")
	defer observe.TraceCtx(ctx, "fileread", "readPDFPages", "exit")
	first, last, err := parsePDFPageRange(pages)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	if last == -1 {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: last == -1")
		last = first + pdfMaxPagesPerRead - 1
	} else {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "else: last == -1")
		pageCount := last - first + 1
		if pageCount > pdfMaxPagesPerRead {
			observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: pageCount > pdfMaxPagesPerRead")
			observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"page range \\\"%s\\\" exceeds maximum of %d pa...")
			return tool.InvokeResult{}, fmt.Errorf(
				"page range \"%s\" exceeds maximum of %d pages per request. Please use a smaller range.",
				pages, pdfMaxPagesPerRead)
		}
	}

	if pdftoppmAvailable(ctx) {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: pdftoppmAvailable(ctx)")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: extractPagesAsImages(ctx, filePath, displayPath, first, last)")
		return extractPagesAsImages(ctx, filePath, displayPath, first, last)
	}

	if pdftotextAvailable(ctx) {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: pdftotextAvailable(ctx)")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: extractPagesAsText(ctx, filePath, displayPath, first, last)")
		return extractPagesAsText(ctx, filePath, displayPath, first, last)
	}
	observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"PDF page extraction requires poppler-utils...")

	return tool.InvokeResult{}, fmt.Errorf(
		"PDF page extraction requires poppler-utils. Install with: " +
			"brew install poppler (macOS) or apt-get install poppler-utils (Debian/Ubuntu)")
}

func extractPagesAsImages(ctx context.Context, filePath, displayPath string, first, last int) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "enter")
	defer observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "exit")
	dir, err := os.MkdirTemp("", "gogent-pdf-*")
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"create temp dir: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	args := []string{"-jpeg", "-r", "150"}
	if first > 0 {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: first > 0")
		args = append(args, "-f", strconv.Itoa(first))
	}
	if last > 0 {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: last > 0")
		args = append(args, "-l", strconv.Itoa(last))
	}
	args = append(args, filePath, filepath.Join(dir, "page"))

	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftoppm failed: %s\", strings.TrimSpace(stri...")
		return tool.InvokeResult{}, fmt.Errorf("pdftoppm failed: %s", strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"read extracted pages: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read extracted pages: %w", err)
	}

	var supplements []model.ContentPart
	for _, entry := range entries {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "range entries")
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jpg") {
			observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: entry.IsDir() || !strings.HasSuffix(entry.Name(), \".jpg\")")
			continue
		}
		imgData, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: err != nil")
			continue
		}
		supplements = append(supplements, model.ImagePart{
			MimeType: "image/jpeg",
			Data:     imgData,
		})
	}

	if len(supplements) == 0 {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: len(supplements) == 0")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"no pages extracted from PDF\")")
		return tool.InvokeResult{}, fmt.Errorf("no pages extracted from PDF")
	}
	observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"PDF pages extracted: %d page(s) fro...")

	return tool.InvokeResult{
		Content:     fmt.Sprintf("PDF pages extracted: %d page(s) from %s", len(supplements), displayPath),
		Supplements: supplements,
	}, nil
}

func extractPagesAsText(ctx context.Context, filePath, displayPath string, first, last int) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "enter")
	defer observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "exit")
	args := []string{"-layout"}
	if first > 0 {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "if: first > 0")
		args = append(args, "-f", strconv.Itoa(first))
	}
	if last > 0 {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "if: last > 0")
		args = append(args, "-l", strconv.Itoa(last))
	}
	args = append(args, filePath, "-")

	cmd := exec.CommandContext(ctx, "pdftotext", args...)
	out, err := cmd.Output()
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "if: err != nil")
		if exitErr, ok := err.(*exec.ExitError); ok {
			observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "if: ok")
			observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftotext failed: %s\", strings.TrimSpace(str...")
			return tool.InvokeResult{}, fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftotext: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("pdftotext: %w", err)
	}

	content := strings.TrimRight(string(out), "\n")
	if content == "" {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "if: content == \"\"")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{Content: fmt.Sprintf(\"PDF %s: no text content found in page...")
		return tool.InvokeResult{Content: fmt.Sprintf("PDF %s: no text content found in pages %d-%d", displayPath, first, last)}, nil
	}
	observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{Content: content}, nil")

	return tool.InvokeResult{Content: content}, nil
}

// parsePDFPageRange parses "5", "1-10", or "3-" into first/last page numbers.
// Returns -1 for last if open-ended. Pages are 1-indexed.
func parsePDFPageRange(pages string) (first, last int, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	pages = strings.TrimSpace(pages)
	if pages == "" {
		observe.GlobalTrace("if: pages == \"\"")
		observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"empty pages parameter\")")
		return 0, 0, fmt.Errorf("empty pages parameter")
	}

	if strings.HasSuffix(pages, "-") {
		observe.GlobalTrace("if: strings.HasSuffix(pages, \"-\")")
		f, err := strconv.Atoi(pages[:len(pages)-1])
		if err != nil || f < 1 {
			observe.GlobalTrace("if: err != nil || f < 1")
			observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Use formats like \\\"1-5\\\", \\\"3\\...")
			return 0, 0, fmt.Errorf("invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\"", pages)
		}
		observe.GlobalTrace("return: f, -1, nil")
		return f, -1, nil
	}

	dashIdx := strings.Index(pages, "-")
	if dashIdx == -1 {
		observe.GlobalTrace("if: dashIdx == -1")

		p, err := strconv.Atoi(pages)
		if err != nil || p < 1 {
			observe.GlobalTrace("if: err != nil || p < 1")
			observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Pages are 1-indexed\", pages)")
			return 0, 0, fmt.Errorf("invalid pages parameter: %q. Pages are 1-indexed", pages)
		}
		observe.GlobalTrace("return: p, p, nil")
		return p, p, nil
	}

	f, err1 := strconv.Atoi(pages[:dashIdx])
	l, err2 := strconv.Atoi(pages[dashIdx+1:])
	if err1 != nil || err2 != nil || f < 1 || l < 1 || l < f {
		observe.GlobalTrace("if: err1 != nil || err2 != nil || f < 1 || l < 1 || l < f")
		observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Use formats like \\\"1-5\\\", \\\"3\\...")
		return 0, 0, fmt.Errorf("invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\"", pages)
	}
	observe.GlobalTrace("return: f, l, nil")
	return f, l, nil
}

func pdftoppmAvailable(ctx context.Context) bool {
	observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "enter")
	defer observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "exit")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-v")
	err := cmd.Run()
	observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "return: err == nil")
	return err == nil
}

func pdftotextAvailable(ctx context.Context) bool {
	observe.TraceCtx(ctx, "fileread", "pdftotextAvailable", "enter")
	defer observe.TraceCtx(ctx, "fileread", "pdftotextAvailable", "exit")
	cmd := exec.CommandContext(ctx, "pdftotext", "-v")

	out, err := cmd.CombinedOutput()
	observe.TraceCtx(ctx, "fileread", "pdftotextAvailable", "return: err == nil || len(out) > 0")
	return err == nil || len(out) > 0
}

func formatSize(bytes int64) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch {
	case bytes >= 1024*1024*1024:
		observe.GlobalTrace("case: bytes >= 1024*1024*1024")
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	case bytes >= 1024*1024:
		observe.GlobalTrace("case: bytes >= 1024*1024")
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		observe.GlobalTrace("case: bytes >= 1024")
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		observe.GlobalTrace("default")
		return fmt.Sprintf("%d bytes", bytes)
	}
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

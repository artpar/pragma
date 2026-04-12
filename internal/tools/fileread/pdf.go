package fileread

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/tool"
)

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
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"open PDF: %w\", err)")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"open PDF: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("open PDF: %w", err)
	}
	n, err := f.Read(header)
	_ = f.Close()
	if err != nil || n < 5 || string(header[:5]) != "%PDF-" {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: err != nil || n < 5 || string(header[:5]) != \"%PDF-\"")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"file is not a valid PDF (missing %%PDF- head...\")")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"file is not a valid PDF (missing %%PDF- head...")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"file is not a valid PDF (missing %%PDF- head...")
		return tool.InvokeResult{}, fmt.Errorf("file is not a valid PDF (missing %%PDF- header): %s", displayPath)
	}

	if pages != nil && *pages != "" {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: pages != nil && *pages != \"\"")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: readPDFPages(ctx, filePath, displayPath, *pages)")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: readPDFPages(ctx, filePath, displayPath, *pages)")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: readPDFPages(ctx, filePath, displayPath, *pages)")
		return readPDFPages(ctx, filePath, displayPath, *pages)
	}

	if size > pdfMaxRawSize {
		observe.TraceCtx(ctx, "fileread", "readPDF", "if: size > pdfMaxRawSize")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"PDF file is too large (%s). Maximum size f...")
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
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"read PDF: %w\", err)")
		observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{}, fmt.Errorf(\"read PDF: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read PDF: %w", err)
	}
	observe.TraceCtx(ctx, "fileread", "readPDF", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"PDF file read: %s (%s)\", displayPat...")
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
			observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"page range \\\"%s\\\" exceeds maximum of %d pa...")
			return tool.InvokeResult{}, fmt.Errorf(
				"page range \"%s\" exceeds maximum of %d pages per request. Please use a smaller range.",
				pages, pdfMaxPagesPerRead)
		}
	}

	if pdftoppmAvailable(ctx) {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: pdftoppmAvailable(ctx)")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: extractPagesAsImages(ctx, filePath, displayPath, first, last)")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: extractPagesAsImages(ctx, filePath, displayPath, first, last)")
		return extractPagesAsImages(ctx, filePath, displayPath, first, last)
	}

	if pdftotextAvailable(ctx) {
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "if: pdftotextAvailable(ctx)")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: extractPagesAsText(ctx, filePath, displayPath, first, last)")
		observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: extractPagesAsText(ctx, filePath, displayPath, first, last)")
		return extractPagesAsText(ctx, filePath, displayPath, first, last)
	}
	observe.TraceCtx(ctx, "fileread", "readPDFPages", "return: tool.InvokeResult{}, fmt.Errorf(\n\t\"PDF page extraction requires poppler-utils...")
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
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftoppm failed: %s\", strings.TrimSpace(stri...")
		return tool.InvokeResult{}, fmt.Errorf("pdftoppm failed: %s", strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "if: err != nil")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"read extracted pages: %w\", err)")
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
		observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{}, fmt.Errorf(\"no pages extracted from PDF\")")
		return tool.InvokeResult{}, fmt.Errorf("no pages extracted from PDF")
	}
	observe.TraceCtx(ctx, "fileread", "extractPagesAsImages", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"PDF pages extracted: %d page(s) fro...")
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
			observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftotext failed: %s\", strings.TrimSpace(str...")
			return tool.InvokeResult{}, fmt.Errorf("pdftotext failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftotext: %w\", err)")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{}, fmt.Errorf(\"pdftotext: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("pdftotext: %w", err)
	}

	content := strings.TrimRight(string(out), "\n")
	if content == "" {
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "if: content == \"\"")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{Content: fmt.Sprintf(\"PDF %s: no text content found in page...")
		observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{Content: fmt.Sprintf(\"PDF %s: no text content found in page...")
		return tool.InvokeResult{Content: fmt.Sprintf("PDF %s: no text content found in pages %d-%d", displayPath, first, last)}, nil
	}
	observe.TraceCtx(ctx, "fileread", "extractPagesAsText", "return: tool.InvokeResult{Content: content}, nil")
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
		observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"empty pages parameter\")")
		return 0, 0, fmt.Errorf("empty pages parameter")
	}

	if strings.HasSuffix(pages, "-") {
		observe.GlobalTrace("if: strings.HasSuffix(pages, \"-\")")
		f, err := strconv.Atoi(pages[:len(pages)-1])
		if err != nil || f < 1 {
			observe.GlobalTrace("if: err != nil || f < 1")
			observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Use formats like \\\"1-5\\\", \\\"3\\...")
			observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Use formats like \\\"1-5\\\", \\\"3\\...")
			return 0, 0, fmt.Errorf("invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\"", pages)
		}
		observe.GlobalTrace("return: f, -1, nil")
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
			observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Pages are 1-indexed\", pages)")
			return 0, 0, fmt.Errorf("invalid pages parameter: %q. Pages are 1-indexed", pages)
		}
		observe.GlobalTrace("return: p, p, nil")
		observe.GlobalTrace("return: p, p, nil")
		return p, p, nil
	}

	f, err1 := strconv.Atoi(pages[:dashIdx])
	l, err2 := strconv.Atoi(pages[dashIdx+1:])
	if err1 != nil || err2 != nil || f < 1 || l < 1 || l < f {
		observe.GlobalTrace("if: err1 != nil || err2 != nil || f < 1 || l < 1 || l < f")
		observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Use formats like \\\"1-5\\\", \\\"3\\...")
		observe.GlobalTrace("return: 0, 0, fmt.Errorf(\"invalid pages parameter: %q. Use formats like \\\"1-5\\\", \\\"3\\...")
		return 0, 0, fmt.Errorf("invalid pages parameter: %q. Use formats like \"1-5\", \"3\", or \"10-20\"", pages)
	}
	observe.GlobalTrace("return: f, l, nil")
	observe.GlobalTrace("return: f, l, nil")
	return f, l, nil
}

func pdftoppmAvailable(ctx context.Context) bool {
	observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "enter")
	defer observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "exit")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-v")
	err := cmd.Run()
	observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "return: err == nil")
	observe.TraceCtx(ctx, "fileread", "pdftoppmAvailable", "return: err == nil")
	return err == nil
}

func pdftotextAvailable(ctx context.Context) bool {
	observe.TraceCtx(ctx, "fileread", "pdftotextAvailable", "enter")
	defer observe.TraceCtx(ctx, "fileread", "pdftotextAvailable", "exit")
	cmd := exec.CommandContext(ctx, "pdftotext", "-v")
	out, err := cmd.CombinedOutput()
	observe.TraceCtx(ctx, "fileread", "pdftotextAvailable", "return: err == nil || len(out) > 0")
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

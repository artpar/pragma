package fileread

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
	toolpkg "github.com/artpar/pragma/internal/tool"
)

type testState struct{ dir string }

func (s testState) WorkDir() string { return s.dir }

type cachedTestState struct {
	dir   string
	cache *toolpkg.FileStateCache
}

func (s cachedTestState) WorkDir() string { return s.dir }
func (s cachedTestState) ReadFileState() *toolpkg.FileStateCache {
	return s.cache
}

func TestFileReadTool_BasicRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line one\nline two\nline three\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have line numbers
	if !strings.Contains(result.Content, "1→line one") {
		t.Errorf("expected line-numbered content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "3→line three") {
		t.Errorf("expected line 3, got: %s", result.Content)
	}
}

func TestFileReadToolRepeatedReadCanPreserveContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("line one\nline two\n"), 0644)
	state := cachedTestState{dir: dir, cache: toolpkg.NewFileStateCache()}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	if _, err := (&Tool{}).Invoke(context.Background(), input, state); err != nil {
		t.Fatalf("initial read: %v", err)
	}
	elided, err := (&Tool{}).Invoke(context.Background(), input, state)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if elided.Content != "File unchanged since last read." {
		t.Fatalf("expected repeated read to be elided, got: %s", elided.Content)
	}

	preserved, err := (&Tool{PreserveRepeatedContent: true}).Invoke(context.Background(), input, state)
	if err != nil {
		t.Fatalf("preserved read: %v", err)
	}
	if !strings.Contains(preserved.Content, "1→line one") {
		t.Fatalf("expected repeated content in preserve mode, got: %s", preserved.Content)
	}
}

func TestFileReadToolDescriptionIsMechanical(t *testing.T) {
	description := (&Tool{}).Description()
	for _, forbidden := range []string{
		"speculatively",
		"parallel",
		"ALWAYS",
		"Assume this tool",
		"Usage:",
	} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("description contains behavioral guidance %q: %s", forbidden, description)
		}
	}
}

func TestFileReadTool_OffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "line "+strings.Repeat("x", i))
	}
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)

	tool := &Tool{}
	offset := 5
	limit := 3
	input, _ := json.Marshal(FileReadInput{FilePath: path, Offset: &offset, Limit: &limit})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "5→") {
		t.Errorf("expected line 5, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "7→") {
		t.Errorf("expected line 7, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "8→") {
		t.Errorf("should not contain line 8, got: %s", result.Content)
	}
}

func TestFileReadTool_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte(""), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "empty") {
		t.Errorf("expected empty file warning, got: %s", result.Content)
	}
}

func TestFileReadTool_OffsetBeyondFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "short.txt")
	os.WriteFile(path, []byte("one\ntwo\n"), 0644)

	tool := &Tool{}
	offset := 100
	input, _ := json.Marshal(FileReadInput{FilePath: path, Offset: &offset})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "shorter than the provided offset") {
		t.Errorf("expected offset warning, got: %s", result.Content)
	}
}

func TestFileReadTool_FileNotFound(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: "/nonexistent/file.txt"})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "file not found") {
		t.Errorf("expected 'file not found' error, got: %v", err)
	}
}

func TestFileReadTool_BlockedDevice(t *testing.T) {
	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: "/dev/zero"})

	_, err := tool.Invoke(context.Background(), input, testState{t.TempDir()})
	if err == nil {
		t.Fatal("expected error for blocked device")
	}
	if !strings.Contains(err.Error(), "block") {
		t.Errorf("expected blocking error, got: %v", err)
	}
}

func TestFileReadTool_BinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.exe")
	os.WriteFile(path, []byte{0x00, 0x01, 0x02}, 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for binary file")
	}
	if !strings.Contains(err.Error(), "binary") {
		t.Errorf("expected binary error, got: %v", err)
	}
}

func TestFileReadTool_Directory(t *testing.T) {
	dir := t.TempDir()
	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: dir})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got: %v", err)
	}
}

func TestFileReadTool_ImageFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	// Write a minimal PNG header
	os.WriteFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "Image file: test.png") {
		t.Errorf("expected image description, got: %s", result.Content)
	}
	if len(result.Supplements) != 1 {
		t.Fatalf("expected 1 ImagePart supplement, got %d", len(result.Supplements))
	}
}

func TestFileReadTool_PDFValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	// Write a minimal PDF
	pdfContent := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
	os.WriteFile(path, pdfContent, 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "PDF file read") {
		t.Errorf("expected PDF file read message, got: %s", result.Content)
	}
	// Should have a DocumentPart supplement
	if len(result.Supplements) != 1 {
		t.Fatalf("expected 1 supplement (DocumentPart), got %d", len(result.Supplements))
	}
	doc, ok := result.Supplements[0].(model.DocumentPart)
	if !ok {
		t.Fatalf("expected DocumentPart supplement, got %T", result.Supplements[0])
	}
	if doc.MimeType != "application/pdf" {
		t.Errorf("expected application/pdf, got %s", doc.MimeType)
	}
	if string(doc.Data) != string(pdfContent) {
		t.Errorf("expected PDF data to match original")
	}
}

func TestFileReadTool_PDFInvalidHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.pdf")
	os.WriteFile(path, []byte("<html>not a pdf</html>"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	_, err := tool.Invoke(context.Background(), input, testState{dir})
	if err == nil {
		t.Fatal("expected error for invalid PDF header")
	}
	if !strings.Contains(err.Error(), "not a valid PDF") {
		t.Errorf("expected 'not a valid PDF' error, got: %v", err)
	}
}

func TestFileReadTool_PDFPageRangeParsing(t *testing.T) {
	tests := []struct {
		input     string
		wantFirst int
		wantLast  int
		wantErr   bool
	}{
		{"5", 5, 5, false},
		{"1-10", 1, 10, false},
		{"3-", 3, -1, false},
		{"0", 0, 0, true},
		{"abc", 0, 0, true},
		{"10-5", 0, 0, true},
		{"", 0, 0, true},
	}
	for _, tt := range tests {
		first, last, err := parsePDFPageRange(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parsePDFPageRange(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if err == nil && (first != tt.wantFirst || last != tt.wantLast) {
			t.Errorf("parsePDFPageRange(%q): got (%d,%d), want (%d,%d)", tt.input, first, last, tt.wantFirst, tt.wantLast)
		}
	}
}

func TestFileReadTool_CRLFNormalization(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crlf.txt")
	os.WriteFile(path, []byte("line1\r\nline2\r\n"), 0644)

	tool := &Tool{}
	input, _ := json.Marshal(FileReadInput{FilePath: path})

	result, err := tool.Invoke(context.Background(), input, testState{dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result.Content, "\r") {
		t.Errorf("expected CRLF normalized, got result with \\r")
	}
	if !strings.Contains(result.Content, "line1") && !strings.Contains(result.Content, "line2") {
		t.Errorf("expected both lines, got: %s", result.Content)
	}
}

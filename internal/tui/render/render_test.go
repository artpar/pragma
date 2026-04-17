package render

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/model"
)

// stripANSI removes ANSI escape sequences for test assertions.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestMarkdownRenderer(t *testing.T) {
	md := NewMarkdownRenderer(80)

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"plain text", "hello world", "hello world"},
		{"heading", "# Title", "Title"},
		{"bold", "**bold text**", "bold text"},
		{"code block", "```go\nfmt.Println()\n```", "fmt.Println"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := md.Render(tt.input)
			plain := stripANSI(result)
			if tt.contains != "" && !strings.Contains(plain, tt.contains) {
				t.Errorf("expected %q in rendered output (stripped):\n%s", tt.contains, plain)
			}
		})
	}
}

func TestMarkdownRendererUpdateWidth(t *testing.T) {
	md := NewMarkdownRenderer(40)
	md.UpdateWidth(120)
	result := md.Render("test text")
	plain := stripANSI(result)
	if !strings.Contains(plain, "test text") {
		t.Errorf("expected text after width update, got (stripped): %q", plain)
	}
}

func TestRenderToolCallWithPrimaryParam(t *testing.T) {
	tc := model.ToolCallPart{
		ID:    "t1",
		Name:  "Bash",
		Input: json.RawMessage(`{"command":"ls -la /tmp"}`),
	}
	result := RenderToolCall(tc, 80)
	if !strings.Contains(result, BlackCircle) {
		t.Errorf("expected ⏺ glyph in %q", result)
	}
	if !strings.Contains(result, "Bash") {
		t.Errorf("expected tool name in %q", result)
	}
	if !strings.Contains(result, "ls -la /tmp") {
		t.Errorf("expected command in %q", result)
	}
}

func TestRenderToolCallWithoutInput(t *testing.T) {
	tc := model.ToolCallPart{ID: "t1", Name: "SomeTool"}
	result := RenderToolCall(tc, 80)
	if !strings.Contains(result, "SomeTool") {
		t.Errorf("expected tool name in %q", result)
	}
}

func TestWrapWithBracket(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		isError  bool
		contains string
	}{
		{"empty normal", "", false, "no output"},
		{"empty error", "", true, "error"},
		{"single line", "hello", false, Bracket},
		{"multi line", "line1\nline2\nline3", false, "line2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapWithBracket(tt.content, tt.isError, 80, false)
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected %q in %q", tt.contains, result)
			}
		})
	}
}

func TestWrapWithBracketTruncation(t *testing.T) {
	// Generate content with 20 lines (exceeds maxLines=15)
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line content here")
	}
	content := strings.Join(lines, "\n")

	result := WrapWithBracket(content, false, 80, false)
	if !strings.Contains(result, "+5 more lines") {
		t.Errorf("expected truncation indicator, got %q", result)
	}
}

func TestRenderThinkingWithGlyph(t *testing.T) {
	tp := model.ThinkingPart{Text: "analyzing..."}
	result := RenderThinking(tp, true)
	if !strings.Contains(result, ThinkGlyph) {
		t.Errorf("expected think glyph in %q", result)
	}
	if !strings.Contains(result, "analyzing") {
		t.Errorf("expected text in %q", result)
	}
}

func TestRenderThinkingCollapsed(t *testing.T) {
	tp := model.ThinkingPart{Text: "analyzing..."}
	result := RenderThinking(tp, false)
	if !strings.Contains(result, "Thinking") {
		t.Errorf("expected 'Thinking' label in collapsed result %q", result)
	}
	if strings.Contains(result, "analyzing") {
		t.Errorf("collapsed thinking should not contain content, got %q", result)
	}
	if !strings.Contains(result, "ctrl+o") {
		t.Errorf("expected Ctrl+O hint in collapsed result %q", result)
	}
}

func TestRenderThinkingRedacted(t *testing.T) {
	tp := model.ThinkingPart{Redacted: true}
	result := RenderThinking(tp, false)
	if !strings.Contains(result, "redacted") {
		t.Errorf("expected redacted in %q", result)
	}
}

func TestRenderToolOutputBash(t *testing.T) {
	input := json.RawMessage(`{"command":"echo hello"}`)
	result := RenderToolOutput("Bash", input, "hello\n", false, 80, "", false)
	// Command is shown in tool call line, not repeated in result
	if !strings.Contains(result, "hello") {
		t.Errorf("expected output in %q", result)
	}
}

func TestRenderToolOutputBashNoOutput(t *testing.T) {
	input := json.RawMessage(`{"command":"true"}`)
	result := RenderToolOutput("Bash", input, "", false, 80, "", false)
	if !strings.Contains(result, "no output") {
		t.Errorf("expected no output indicator in %q", result)
	}
}

func TestRenderToolOutputEdit(t *testing.T) {
	input := json.RawMessage(`{"file_path":"test.go","old_string":"foo","new_string":"bar"}`)
	result := RenderToolOutput("Edit", input, "ok", false, 80, "", false)
	if !strings.Contains(result, "test.go") {
		t.Errorf("expected file path in %q", result)
	}
	if !strings.Contains(result, "- foo") {
		t.Errorf("expected removed line in %q", result)
	}
	if !strings.Contains(result, "+ bar") {
		t.Errorf("expected added line in %q", result)
	}
}

func TestRenderToolOutputEditWithUnifiedDiff(t *testing.T) {
	input := json.RawMessage(`{"file_path":"test.go","old_string":"foo","new_string":"bar"}`)
	display := "@@ -2,5 +2,5 @@\n ctx1\n ctx2\n ctx3\n-foo\n+bar\n ctx4\n ctx5"
	result := RenderToolOutput("Edit", input, "ok", false, 80, display, false)
	if !strings.Contains(result, "test.go") {
		t.Errorf("expected file path in %q", result)
	}
	// Should have line numbers in gutter
	if !strings.Contains(result, "4") {
		t.Errorf("expected line numbers in %q", result)
	}
	// Should not have old-style "- foo" (unified diff uses different format)
	if strings.Contains(result, "- foo") {
		t.Errorf("should use unified diff format, not old-style, in %q", result)
	}
}

func TestRenderToolOutputGrep(t *testing.T) {
	input := json.RawMessage(`{"pattern":"TODO"}`)
	content := "file1.go\nfile2.go\nfile3.go\n"
	result := RenderToolOutput("Grep", input, content, false, 80, "", false)
	if !strings.Contains(result, "3 files") {
		t.Errorf("expected file count in %q", result)
	}
}

func TestRenderToolOutputGlob(t *testing.T) {
	input := json.RawMessage(`{"pattern":"*.go"}`)
	content := "a.go\nb.go\n"
	result := RenderToolOutput("Glob", input, content, false, 80, "", false)
	if !strings.Contains(result, "2 files") {
		t.Errorf("expected file count in %q", result)
	}
}

func TestRenderToolOutputUnknownTool(t *testing.T) {
	input := json.RawMessage(`{"key":"value"}`)
	result := RenderToolOutput("UnknownTool", input, "some output", false, 80, "", false)
	if !strings.Contains(result, Bracket) {
		t.Errorf("expected bracket wrapper for unknown tool in %q", result)
	}
	if !strings.Contains(result, "some output") {
		t.Errorf("expected content in %q", result)
	}
}

func TestRenderToolOutputRead(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/test.go","offset":0,"limit":5}`)
	content := "line1\nline2\nline3\nline4\nline5"
	result := RenderToolOutput("Read", input, content, false, 80, "", false)
	// Compact summary: "Read N lines" — path is shown in the tool call line
	if !strings.Contains(result, "Read 5 lines") {
		t.Errorf("expected compact summary in %q", result)
	}
}

func TestRenderToolOutputReadError(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/missing.go"}`)
	result := RenderToolOutput("Read", input, "file not found", true, 80, "", false)
	if !strings.Contains(result, "file not found") {
		t.Errorf("expected error content in %q", result)
	}
}

func TestRenderToolOutputWrite(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/out.go"}`)
	result := RenderToolOutput("Write", input, "package main", false, 80, "", false)
	if !strings.Contains(result, "/tmp/out.go") {
		t.Errorf("expected file path in %q", result)
	}
	if !strings.Contains(result, "Wrote") {
		t.Errorf("expected 'Wrote' in %q", result)
	}
}

func TestRenderToolOutputAgent(t *testing.T) {
	input := json.RawMessage(`{"prompt":"search for patterns in the codebase"}`)
	content := "Found 3 patterns\nin file1.go\nin file2.go"
	result := RenderToolOutput("Agent", input, content, false, 80, "", false)
	if !strings.Contains(result, DiamondFilled) {
		t.Errorf("expected filled diamond for completed agent in %q", result)
	}
	if !strings.Contains(result, "search for patterns") {
		t.Errorf("expected prompt summary in %q", result)
	}
}

func TestRenderToolOutputAgentError(t *testing.T) {
	input := json.RawMessage(`{"prompt":"do something"}`)
	result := RenderToolOutput("Agent", input, "timeout", true, 80, "", false)
	if !strings.Contains(result, DiamondOpen) {
		t.Errorf("expected open diamond for error agent in %q", result)
	}
}

func TestRenderToolOutputBashVerbose(t *testing.T) {
	input := json.RawMessage(`{"command":"seq 10"}`)
	// 10 lines: verbose should show all, non-verbose only last 5
	content := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10"
	nonVerbose := RenderToolOutput("Bash", input, content, false, 80, "", false)
	verbose := RenderToolOutput("Bash", input, content, false, 80, "", true)

	// Non-verbose: should hide first 5 lines and show hint
	if !strings.Contains(nonVerbose, "5 lines hidden") {
		t.Errorf("non-verbose should show hidden count, got %q", nonVerbose)
	}
	if !strings.Contains(nonVerbose, "ctrl+o to expand") {
		t.Errorf("non-verbose should show expand hint, got %q", nonVerbose)
	}

	// Verbose: should show all lines, no hint
	plain := stripANSI(verbose)
	if strings.Contains(plain, "lines hidden") {
		t.Errorf("verbose should not hide lines, got %q", plain)
	}
	if strings.Contains(plain, "ctrl+o") {
		t.Errorf("verbose should not show expand hint, got %q", plain)
	}
	if !strings.Contains(plain, "1") || !strings.Contains(plain, "10") {
		t.Errorf("verbose should show all lines, got %q", plain)
	}
}

func TestRenderToolOutputReadVerbose(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/test.go","offset":0,"limit":100}`)
	content := "line1\nline2\nline3"
	nonVerbose := RenderToolOutput("Read", input, content, false, 80, "", false)
	verbose := RenderToolOutput("Read", input, content, false, 80, "", true)

	// Non-verbose: summary only + hint
	plain := stripANSI(nonVerbose)
	if !strings.Contains(plain, "Read 3 lines") {
		t.Errorf("non-verbose should show summary, got %q", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Errorf("non-verbose should show expand hint, got %q", plain)
	}
	if strings.Contains(plain, "line1") {
		t.Errorf("non-verbose should not show file content, got %q", plain)
	}

	// Verbose: should show numbered content
	vPlain := stripANSI(verbose)
	if !strings.Contains(vPlain, "line1") {
		t.Errorf("verbose should show file content, got %q", vPlain)
	}
	if !strings.Contains(vPlain, "1 ") {
		t.Errorf("verbose should show line numbers, got %q", vPlain)
	}
	if strings.Contains(vPlain, "ctrl+o") {
		t.Errorf("verbose should not show expand hint, got %q", vPlain)
	}
}

func TestRenderToolOutputGrepVerbose(t *testing.T) {
	input := json.RawMessage(`{"pattern":"TODO"}`)
	// 12 files: non-verbose shows 10, verbose shows all
	var files []string
	for i := 1; i <= 12; i++ {
		files = append(files, fmt.Sprintf("file%d.go", i))
	}
	content := strings.Join(files, "\n")

	nonVerbose := RenderToolOutput("Grep", input, content, false, 80, "", false)
	verbose := RenderToolOutput("Grep", input, content, false, 80, "", true)

	// Non-verbose: should truncate + hint
	plain := stripANSI(nonVerbose)
	if !strings.Contains(plain, "+2 more files") {
		t.Errorf("non-verbose should show remaining count, got %q", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Errorf("non-verbose should show expand hint, got %q", plain)
	}

	// Verbose: should show all 12
	vPlain := stripANSI(verbose)
	if !strings.Contains(vPlain, "file12.go") {
		t.Errorf("verbose should show all files, got %q", vPlain)
	}
	if strings.Contains(vPlain, "more files") {
		t.Errorf("verbose should not truncate, got %q", vPlain)
	}
}

func TestRenderToolOutputAgentVerbose(t *testing.T) {
	input := json.RawMessage(`{"prompt":"search patterns"}`)
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8"

	nonVerbose := RenderToolOutput("Agent", input, content, false, 80, "", false)
	verbose := RenderToolOutput("Agent", input, content, false, 80, "", true)

	// Non-verbose: truncated to 5 lines + hint
	plain := stripANSI(nonVerbose)
	if !strings.Contains(plain, "+3 more lines") {
		t.Errorf("non-verbose should show truncation count, got %q", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Errorf("non-verbose should show expand hint, got %q", plain)
	}

	// Verbose: all lines
	vPlain := stripANSI(verbose)
	if !strings.Contains(vPlain, "line8") {
		t.Errorf("verbose should show all lines, got %q", vPlain)
	}
	if strings.Contains(vPlain, "more lines") {
		t.Errorf("verbose should not truncate, got %q", vPlain)
	}
}

func TestWrapWithBracketVerbose(t *testing.T) {
	// 20 lines: non-verbose truncates to 15, verbose shows up to 200
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, "line content here")
	}
	content := strings.Join(lines, "\n")

	nonVerbose := WrapWithBracket(content, false, 80, false)
	verbose := WrapWithBracket(content, false, 80, true)

	if !strings.Contains(nonVerbose, "+5 more lines") {
		t.Errorf("non-verbose should truncate, got %q", nonVerbose)
	}
	if !strings.Contains(nonVerbose, "ctrl+o to expand") {
		t.Errorf("non-verbose should show expand hint, got %q", nonVerbose)
	}
	if strings.Contains(verbose, "more lines") {
		t.Errorf("verbose should not truncate 20 lines, got %q", verbose)
	}
}

func TestRenderConversationNoSeparators(t *testing.T) {
	md := NewMarkdownRenderer(80)
	msgs := []model.Message{
		{ID: "1", Role: model.RoleUser, Content: []model.ContentPart{model.TextPart{Text: "Hi"}}},
		{ID: "2", Role: model.RoleAssistant, Content: []model.ContentPart{model.TextPart{Text: "Hello"}}},
	}
	result := RenderConversation(msgs, md)
	// No turn separators — clean flow like Pragma
	if strings.Contains(result, "━") {
		t.Errorf("expected no turn separator in conversation output, got %q", result)
	}
	if !strings.Contains(result, "Hi") || !strings.Contains(result, "Hello") {
		t.Errorf("expected both messages in conversation output")
	}
}

func TestGenerateGroupSummary(t *testing.T) {
	tests := []struct {
		name     string
		search   int
		read     int
		active   bool
		expected string
	}{
		{"empty", 0, 0, false, ""},
		{"single read completed", 0, 1, false, "Read 1 file"},
		{"multiple reads completed", 0, 5, false, "Read 5 files"},
		{"single search completed", 1, 0, false, "Searched for 1 pattern"},
		{"multiple searches completed", 3, 0, false, "Searched for 3 patterns"},
		{"search and read completed", 2, 3, false, "Searched for 2 patterns, read 3 files"},
		{"single read active", 0, 1, true, "Reading 1 file\u2026"},
		{"search and read active", 2, 3, true, "Searching for 2 patterns, reading 3 files\u2026"},
		{"search active first capital", 1, 0, true, "Searching for 1 pattern\u2026"},
		{"glob counts as search", 4, 0, false, "Searched for 4 patterns"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateGroupSummary(tt.search, tt.read, tt.active)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRenderToolGroupCollapsed(t *testing.T) {
	g := GroupData{
		Entries: []GroupEntry{
			{CallHeader: "⏺ Grep(pattern)\n", Name: "Grep", HasResult: true},
			{CallHeader: "⏺ Read(file.go)\n", Name: "Read", HasResult: true},
		},
		SearchCount: 1,
		ReadCount:   1,
		Active:      false,
	}
	result := RenderToolGroup(g, false, 80)
	plain := stripANSI(result)
	if !strings.Contains(plain, "Searched for 1 pattern") {
		t.Errorf("expected search summary in %q", plain)
	}
	if !strings.Contains(plain, "read 1 file") {
		t.Errorf("expected read summary in %q", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Errorf("expected expand hint in %q", plain)
	}
}

func TestRenderToolGroupVerbose(t *testing.T) {
	readInput := json.RawMessage(`{"file_path":"/tmp/test.go"}`)
	g := GroupData{
		Entries: []GroupEntry{
			{
				CallHeader: "⏺ Read(test.go)\n",
				Name:       "Read",
				Input:      readInput,
				Content:    "line1\nline2\nline3",
				HasResult:  true,
			},
		},
		ReadCount: 1,
	}
	result := RenderToolGroup(g, true, 80)
	plain := stripANSI(result)
	if !strings.Contains(plain, "Read(test.go)") {
		t.Errorf("verbose should show tool call header, got %q", plain)
	}
	if !strings.Contains(plain, "Read 3 lines") {
		t.Errorf("verbose should show tool result, got %q", plain)
	}
}

func TestRenderToolGroupActiveWithHint(t *testing.T) {
	g := GroupData{
		Entries: []GroupEntry{
			{CallHeader: "⏺ Read(main.go)\n", Name: "Read", HasResult: true},
		},
		ReadCount:  1,
		Active:     true,
		LatestHint: "main.go",
	}
	result := RenderToolGroup(g, false, 80)
	plain := stripANSI(result)
	if !strings.Contains(plain, "Reading 1 file\u2026") {
		t.Errorf("active group should use present tense, got %q", plain)
	}
	if !strings.Contains(plain, "main.go") {
		t.Errorf("active group should show hint, got %q", plain)
	}
	if !strings.Contains(plain, "ctrl+o to expand") {
		t.Errorf("active group should also show expand hint, got %q", plain)
	}
}

func TestRenderToolGroupEmptySilent(t *testing.T) {
	// Group with only "silent" entries (ToolSearch) — should produce empty output
	g := GroupData{
		Entries: []GroupEntry{
			{CallHeader: "⏺ ToolSearch(query)\n", Name: "ToolSearch", HasResult: true},
		},
		SearchCount: 0,
		ReadCount:   0,
		Active:      false,
	}
	result := RenderToolGroup(g, false, 80)
	if result != "" {
		t.Errorf("silent-only group should produce empty output, got %q", result)
	}
}

func TestRenderContentPartImageAndDocument(t *testing.T) {
	md := NewMarkdownRenderer(80)

	imgResult := RenderContentPart(model.ImagePart{MimeType: "image/png"}, md, 80)
	if !strings.Contains(imgResult, "image/png") {
		t.Errorf("expected mime type in image render: %q", imgResult)
	}

	docResult := RenderContentPart(model.DocumentPart{MimeType: "application/pdf"}, md, 80)
	if !strings.Contains(docResult, "application/pdf") {
		t.Errorf("expected mime type in document render: %q", docResult)
	}
}

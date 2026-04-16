package render

import (
	"encoding/json"
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
			result := WrapWithBracket(tt.content, tt.isError, 80)
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

	result := WrapWithBracket(content, false, 80)
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
	result := RenderToolOutput("Bash", input, "hello\n", false, 80)
	// Command is shown in tool call line, not repeated in result
	if !strings.Contains(result, "hello") {
		t.Errorf("expected output in %q", result)
	}
}

func TestRenderToolOutputBashNoOutput(t *testing.T) {
	input := json.RawMessage(`{"command":"true"}`)
	result := RenderToolOutput("Bash", input, "", false, 80)
	if !strings.Contains(result, "no output") {
		t.Errorf("expected no output indicator in %q", result)
	}
}

func TestRenderToolOutputEdit(t *testing.T) {
	input := json.RawMessage(`{"file_path":"test.go","old_string":"foo","new_string":"bar"}`)
	result := RenderToolOutput("Edit", input, "ok", false, 80)
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

func TestRenderToolOutputGrep(t *testing.T) {
	input := json.RawMessage(`{"pattern":"TODO"}`)
	content := "file1.go\nfile2.go\nfile3.go\n"
	result := RenderToolOutput("Grep", input, content, false, 80)
	if !strings.Contains(result, "3 files") {
		t.Errorf("expected file count in %q", result)
	}
}

func TestRenderToolOutputGlob(t *testing.T) {
	input := json.RawMessage(`{"pattern":"*.go"}`)
	content := "a.go\nb.go\n"
	result := RenderToolOutput("Glob", input, content, false, 80)
	if !strings.Contains(result, "2 files") {
		t.Errorf("expected file count in %q", result)
	}
}

func TestRenderToolOutputUnknownTool(t *testing.T) {
	input := json.RawMessage(`{"key":"value"}`)
	result := RenderToolOutput("UnknownTool", input, "some output", false, 80)
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
	result := RenderToolOutput("Read", input, content, false, 80)
	// Compact summary: "Read N lines" — path is shown in the tool call line
	if !strings.Contains(result, "Read 5 lines") {
		t.Errorf("expected compact summary in %q", result)
	}
}

func TestRenderToolOutputReadError(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/missing.go"}`)
	result := RenderToolOutput("Read", input, "file not found", true, 80)
	if !strings.Contains(result, "file not found") {
		t.Errorf("expected error content in %q", result)
	}
}

func TestRenderToolOutputWrite(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/tmp/out.go"}`)
	result := RenderToolOutput("Write", input, "package main", false, 80)
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
	result := RenderToolOutput("Agent", input, content, false, 80)
	if !strings.Contains(result, DiamondFilled) {
		t.Errorf("expected filled diamond for completed agent in %q", result)
	}
	if !strings.Contains(result, "search for patterns") {
		t.Errorf("expected prompt summary in %q", result)
	}
}

func TestRenderToolOutputAgentError(t *testing.T) {
	input := json.RawMessage(`{"prompt":"do something"}`)
	result := RenderToolOutput("Agent", input, "timeout", true, 80)
	if !strings.Contains(result, DiamondOpen) {
		t.Errorf("expected open diamond for error agent in %q", result)
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

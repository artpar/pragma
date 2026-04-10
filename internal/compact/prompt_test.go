package compact

import (
	"strings"
	"testing"
)

func TestCompactPrompt(t *testing.T) {
	t.Run("without custom instructions", func(t *testing.T) {
		prompt := CompactPrompt("")
		if !strings.HasPrefix(prompt, "CRITICAL:") {
			t.Error("should start with NO_TOOLS_PREAMBLE")
		}
		if !strings.Contains(prompt, "Primary Request and Intent") {
			t.Error("should contain section 1")
		}
		if !strings.Contains(prompt, "Optional Next Step") {
			t.Error("should contain section 9")
		}
		if !strings.HasSuffix(prompt, "Tool calls will be rejected and you will fail the task.") {
			t.Error("should end with NO_TOOLS_TRAILER")
		}
		if strings.Contains(prompt, "Additional Instructions") {
			t.Error("should not contain Additional Instructions without custom input")
		}
	})

	t.Run("with custom instructions", func(t *testing.T) {
		prompt := CompactPrompt("Focus on test results")
		if !strings.Contains(prompt, "Additional Instructions:\nFocus on test results") {
			t.Error("should include custom instructions")
		}
	})

	t.Run("whitespace-only custom instructions ignored", func(t *testing.T) {
		prompt := CompactPrompt("   ")
		if strings.Contains(prompt, "Additional Instructions") {
			t.Error("should not include empty custom instructions")
		}
	})
}

func TestFormatCompactSummary(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "strips analysis and formats summary",
			raw:  "<analysis>\nThinking about things...\n</analysis>\n\n<summary>\n1. Primary Request:\nDo stuff\n</summary>",
			want: "Summary:\n1. Primary Request:\nDo stuff",
		},
		{
			name: "no analysis tag",
			raw:  "<summary>\nJust the summary\n</summary>",
			want: "Summary:\nJust the summary",
		},
		{
			name: "no summary tag",
			raw:  "Plain text response without tags",
			want: "Plain text response without tags",
		},
		{
			name: "empty input",
			raw:  "",
			want: "",
		},
		{
			name: "analysis only, no summary",
			raw:  "<analysis>\nSome analysis\n</analysis>\n\nRemaining text",
			want: "Remaining text",
		},
		{
			name: "collapses multiple blank lines",
			raw:  "<analysis>\nx\n</analysis>\n\n\n\n<summary>\ny\n</summary>",
			want: "Summary:\ny",
		},
		{
			name: "multiline analysis and summary",
			raw: `<analysis>
Line 1
Line 2
Line 3
</analysis>

<summary>
1. Primary Request:
   Build a Go CLI tool

2. Key Technical Concepts:
   - Go modules
   - Cobra CLI
</summary>`,
			want: `Summary:
1. Primary Request:
   Build a Go CLI tool

2. Key Technical Concepts:
   - Go modules
   - Cobra CLI`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatCompactSummary(tt.raw)
			if got != tt.want {
				t.Errorf("FormatCompactSummary():\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestCompactUserMessage(t *testing.T) {
	t.Run("without suppress", func(t *testing.T) {
		msg := CompactUserMessage("<summary>\nTest summary\n</summary>", false)
		if !strings.Contains(msg, "being continued from a previous conversation") {
			t.Error("should contain continuation header")
		}
		if !strings.Contains(msg, "Summary:\nTest summary") {
			t.Error("should contain formatted summary")
		}
		if strings.Contains(msg, "without asking the user") {
			t.Error("should not contain suppress text")
		}
	})

	t.Run("with suppress", func(t *testing.T) {
		msg := CompactUserMessage("<summary>\nTest summary\n</summary>", true)
		if !strings.Contains(msg, "without asking the user any further questions") {
			t.Error("should contain suppress text")
		}
	})
}

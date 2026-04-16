package util

import (
	"strings"
	"testing"
)

func TestGenerateEditDiff(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		oldStr      string
		newStr      string
		filePath    string
		replaceAll  bool
		context     int
		wantHunks   int    // number of @@ headers
		wantContain string // substring that must be present
		wantEmpty   bool
	}{
		{
			name:        "single line change with context",
			fileContent: "line1\nline2\nline3\nfoo\nline5\nline6\nline7\n",
			oldStr:      "foo",
			newStr:      "bar",
			filePath:    "test.go",
			context:     3,
			wantHunks:   1,
			wantContain: "-foo",
		},
		{
			name:        "multi-line insertion",
			fileContent: "a\nb\nc\nd\ne\nf\ng\n",
			oldStr:      "c\nd",
			newStr:      "c\nX\nY\nd",
			filePath:    "test.go",
			context:     2,
			wantHunks:   1,
			wantContain: "+X",
		},
		{
			name:        "multi-line deletion",
			fileContent: "a\nb\nc\nd\ne\nf\ng\n",
			oldStr:      "c\nd\ne",
			newStr:      "c",
			filePath:    "test.go",
			context:     2,
			wantHunks:   1,
			wantContain: "-d",
		},
		{
			name:        "replace_all two occurrences",
			fileContent: "aaa\nfoo\nbbb\nccc\nddd\neee\nfff\nfoo\nggg\n",
			oldStr:      "foo",
			newStr:      "bar",
			filePath:    "test.go",
			replaceAll:  true,
			context:     1,
			wantHunks:   2,
			wantContain: "+bar",
		},
		{
			name:        "replace_all adjacent — merged hunks",
			fileContent: "a\nfoo\nfoo\nb\n",
			oldStr:      "foo",
			newStr:      "bar",
			filePath:    "test.go",
			replaceAll:  true,
			context:     3,
			wantHunks:   1,
			wantContain: "+bar",
		},
		{
			name:        "change at line 1",
			fileContent: "first\nsecond\nthird\n",
			oldStr:      "first",
			newStr:      "FIRST",
			filePath:    "test.go",
			context:     3,
			wantHunks:   1,
			wantContain: "+FIRST",
		},
		{
			name:        "change at last line",
			fileContent: "a\nb\nlast",
			oldStr:      "last",
			newStr:      "LAST",
			filePath:    "test.go",
			context:     3,
			wantHunks:   1,
			wantContain: "+LAST",
		},
		{
			name:        "new file creation",
			fileContent: "",
			oldStr:      "",
			newStr:      "package main\n\nfunc main() {}",
			filePath:    "main.go",
			context:     3,
			wantHunks:   1,
			wantContain: "+package main",
		},
		{
			name:        "deletion to empty",
			fileContent: "a\nb\nfoo\nc\nd\n",
			oldStr:      "foo",
			newStr:      "",
			filePath:    "test.go",
			context:     2,
			wantHunks:   1,
			wantContain: "-foo",
		},
		{
			name:      "no match found",
			fileContent: "a\nb\nc\n",
			oldStr:      "notfound",
			newStr:      "x",
			filePath:    "test.go",
			context:     3,
			wantEmpty:   true,
		},
		{
			name:      "empty old and new with content",
			fileContent: "something",
			oldStr:      "",
			newStr:      "",
			filePath:    "test.go",
			context:     3,
			wantEmpty:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateEditDiff(tt.fileContent, tt.oldStr, tt.newStr, tt.filePath, tt.replaceAll, tt.context)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("expected empty result, got:\n%s", result)
				}
				return
			}

			if result == "" {
				t.Fatal("expected non-empty result")
			}

			// Count hunks
			hunkCount := strings.Count(result, "@@ ")
			if hunkCount != tt.wantHunks {
				t.Errorf("expected %d hunks, got %d.\nResult:\n%s", tt.wantHunks, hunkCount, result)
			}

			if tt.wantContain != "" && !strings.Contains(result, tt.wantContain) {
				t.Errorf("expected result to contain %q.\nResult:\n%s", tt.wantContain, result)
			}
		})
	}
}

func TestGenerateEditDiffContextLines(t *testing.T) {
	// Verify context lines are included
	content := "L1\nL2\nL3\nL4\nOLD\nL6\nL7\nL8\nL9\n"
	result := GenerateEditDiff(content, "OLD", "NEW", "f.go", false, 3)

	if !strings.Contains(result, " L2") {
		t.Errorf("expected context line L2, got:\n%s", result)
	}
	if !strings.Contains(result, " L3") {
		t.Errorf("expected context line L3, got:\n%s", result)
	}
	if !strings.Contains(result, " L4") {
		t.Errorf("expected context line L4, got:\n%s", result)
	}
	if !strings.Contains(result, "-OLD") {
		t.Errorf("expected removed line, got:\n%s", result)
	}
	if !strings.Contains(result, "+NEW") {
		t.Errorf("expected added line, got:\n%s", result)
	}
	if !strings.Contains(result, " L6") {
		t.Errorf("expected context line L6, got:\n%s", result)
	}
	if !strings.Contains(result, " L7") {
		t.Errorf("expected context line L7, got:\n%s", result)
	}
	if !strings.Contains(result, " L8") {
		t.Errorf("expected context line L8, got:\n%s", result)
	}
}

func TestGenerateEditDiffHunkHeader(t *testing.T) {
	// Verify hunk header has correct line numbers
	content := "L1\nL2\nL3\nL4\nOLD\nL6\nL7\nL8\nL9\n"
	result := GenerateEditDiff(content, "OLD", "NEW", "f.go", false, 3)

	// OLD is on line 5 (0-indexed: 4), context starts at line 2 (0-indexed: 1)
	// @@ -2,8 +2,8 @@ (3 before + OLD + 3 after = 7, but start is 1-based line 2)
	if !strings.Contains(result, "@@ -2,") {
		t.Errorf("expected hunk starting at old line 2, got:\n%s", result)
	}
}

func TestGenerateEditDiffLargeFile(t *testing.T) {
	// Files over 50K lines should return empty
	var sb strings.Builder
	for i := 0; i < 50001; i++ {
		sb.WriteString("line\n")
	}
	result := GenerateEditDiff(sb.String(), "line", "changed", "big.go", false, 3)
	if result != "" {
		t.Error("expected empty result for large file")
	}
}

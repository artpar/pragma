package applypatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatchUpdateAddDelete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("remove me\n"), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: main.go
@@
 func main() {
-	println("old")
+	println("new")
 }
*** Add File: added.txt
+created
*** Delete File: old.txt
*** End Patch`

	result, err := ApplyPatchText(context.Background(), patch, dir)
	if err != nil {
		t.Fatalf("ApplyPatchText: %v", err)
	}
	if !strings.Contains(result.Content, "main.go") || !strings.Contains(result.Content, "added.txt") {
		t.Fatalf("result content did not list changed files: %s", result.Content)
	}
	mainData, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mainData); !strings.Contains(got, `println("new")`) {
		t.Fatalf("main.go not updated:\n%s", got)
	}
	addedData, err := os.ReadFile(filepath.Join(dir, "added.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(addedData); got != "created\n" {
		t.Fatalf("added.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt still exists or stat failed with unexpected error: %v", err)
	}
}

func TestToolNameIsLowercaseApplyPatch(t *testing.T) {
	if ToolName != "apply_patch" {
		t.Fatalf("ToolName = %q, want apply_patch", ToolName)
	}
	if LegacyToolName != "ApplyPatch" {
		t.Fatalf("LegacyToolName = %q, want ApplyPatch", LegacyToolName)
	}
}

func TestApplyPatchRejectsStaleUpdateWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	original := "package main\n\nfunc main() {\n\tprintln(\"current\")\n}\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	patch := `*** Begin Patch
*** Update File: main.go
@@
 func main() {
-	println("old")
+	println("new")
 }
*** End Patch`

	_, err := ApplyPatchText(context.Background(), patch, dir)
	if err == nil {
		t.Fatal("expected stale update to fail")
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := string(data); got != original {
		t.Fatalf("file mutated after failed patch:\n%s", got)
	}
}

func TestApplyPatchMalformedUpdateErrorIsActionable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	patch := `*** Begin Patch
*** Update File: main.go
@@
package main
*** End Patch`

	_, err := ApplyPatchText(context.Background(), patch, dir)
	if err == nil {
		t.Fatal("expected malformed patch to fail")
	}
	msg := err.Error()
	for _, want := range []string{
		"is malformed",
		"package main",
		"Unchanged context lines need a leading space",
		"reread the target range",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

func TestApplyPatchAcceptsBlankContextLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc main() {\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	patch := `*** Begin Patch
*** Update File: main.go
@@
 package main

 func main() {
-}
+	println("ok")
+}
*** End Patch`

	if _, err := ApplyPatchText(context.Background(), patch, dir); err != nil {
		t.Fatalf("ApplyPatchText: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `println("ok")`) {
		t.Fatalf("patched content = %q", string(data))
	}
}

func TestExtractShellApplyPatchRejectsMixedCommands(t *testing.T) {
	_, _, ok, err := ExtractShellApplyPatch("echo before; apply_patch <<'EOF'\n*** Begin Patch\n*** End Patch\nEOF")
	if err == nil {
		t.Fatal("expected mixed command to fail")
	}
	if ok {
		t.Fatal("mixed command should not be ok")
	}
}

func TestExtractShellApplyPatchIgnoresApplyPatchInNonPatchHeredoc(t *testing.T) {
	tests := []string{
		"cat > /tmp/pragma/implementer-report.md <<'EOF'\nBlocker:\napply_patch failed earlier\nEOF",
		"cat > /tmp/pragma/implementer-report.md <<'EOF'\nBlocker:\napplypatch failed earlier\nEOF",
		"cd /tmp && cat > /tmp/pragma/implementer-report.md <<'EOF'\nBlocker:\napply_patch failed earlier\nEOF",
	}

	for _, command := range tests {
		patch, workDir, ok, err := ExtractShellApplyPatch(command)
		if err != nil {
			t.Fatalf("ExtractShellApplyPatch(%q) error = %v", command, err)
		}
		if ok {
			t.Fatalf("ExtractShellApplyPatch(%q) ok = true, patch=%q workDir=%q", command, patch, workDir)
		}
	}
}

func TestExtractShellApplyPatchRejectsTrailingExecutableCommand(t *testing.T) {
	_, _, ok, err := ExtractShellApplyPatch("apply_patch <<'EOF'\n*** Begin Patch\n*** End Patch\nEOF\necho after")
	if err == nil {
		t.Fatal("expected trailing executable command to fail")
	}
	if ok {
		t.Fatal("trailing command should not be ok")
	}
}

func TestExtractShellApplyPatch(t *testing.T) {
	patch, workDir, ok, err := ExtractShellApplyPatch("cd subdir && apply_patch <<'EOF'\n*** Begin Patch\n*** Add File: x.txt\n+x\n*** End Patch\nEOF")
	if err != nil {
		t.Fatalf("ExtractShellApplyPatch: %v", err)
	}
	if !ok {
		t.Fatal("expected ok")
	}
	if workDir != "subdir" {
		t.Fatalf("workDir = %q", workDir)
	}
	if !strings.Contains(patch, "*** Add File: x.txt") {
		t.Fatalf("patch = %q", patch)
	}
}

// PACT-001: count-bearing hunk headers (@@ -29,6 +29,18 @@) were silently
// ignored by the parser. A header/body count disagreement surfaced as a
// misleading "did not match current file content" error (recorded failing
// input: session 729e2564, 2026-09-24, calc.go) or silently applied when the
// body happened to match, instead of a preflight patch-format error.
func TestApplyPatchPreflightRejectsHunkHeaderCountMismatch(t *testing.T) {
	// Wire-recorded failing input: session 729e2564, apply_patch call 17
	// (2026-09-24), calc.go. The hunk header states 6 old / 18 new lines
	// while the body carries 4 old lines (context: return, }, blank, func
	// main) and 19 new lines (4 context + 15 added).
	recordedPatch := "*** Begin Patch\n" +
		"*** Update File: calc.go\n" +
		"@@\n" +
		"@@ -29,6 +29,18 @@\n" +
		" \treturn total / float64(len(scores))\n" +
		" }\n" +
		"\n" +
		"+func NormalizeName(name string) string {\n" +
		"+\tname = strings.TrimSpace(name)\n" +
		"+\tname = strings.ToLower(name)\n" +
		"+\tname = regexp.MustCompile(`\\s+`).ReplaceAllString(name, \"-\")\n" +
		"+\treturn name\n" +
		"+}\n" +
		"+\n" +
		"+func Score(name string) int {\n" +
		"+\tnormalized := NormalizeName(name)\n" +
		"+\tif strings.HasPrefix(normalized, \"bad\") || len(normalized) < 3 {\n" +
		"+\t\treturn 0\n" +
		"+\t}\n" +
		"+\treturn len(normalized)\n" +
		"+}\n" +
		"+\n" +
		" func main() {\n" +
		"*** End Patch"

	t.Run("misleading content error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "calc.go")
		// No blank line between } and func main: the recorded hunk's
		// context cannot match the file, reproducing the recorded
		// misleading "did not match current file content" failure.
		original := "package main\n\nfunc average(scores []float64) float64 {\n\ttotal := 0.0\n\tfor _, s := range scores {\n\t\ttotal += s\n\t}\n\treturn total / float64(len(scores))\n}\nfunc main() {\n}\n"
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := ApplyPatchText(context.Background(), recordedPatch, dir)
		if err == nil {
			t.Fatal("expected preflight hunk-header count mismatch error, got success")
		}
		msg := err.Error()
		if !strings.Contains(msg, "hunk header") {
			t.Fatalf("error does not diagnose the hunk header: %q", msg)
		}
		for _, want := range []string{"-29,6 +29,18", "6 old and 18 new", "4 old and 19 new"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("error %q does not contain %q", msg, want)
			}
		}
		if strings.Contains(msg, "did not match current file content") {
			t.Fatalf("error blames file content instead of the patch's own header/body mismatch: %q", msg)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if got := string(data); got != original {
			t.Fatalf("file mutated by rejected patch:\n%s", got)
		}
	})

	t.Run("silent acceptance when body matches", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "calc.go")
		// Blank line between } and func main: the recorded hunk's
		// context matches, so the unchanged harness silently applied
		// the patch despite the disagreeing header counts.
		original := "package main\n\nfunc average(scores []float64) float64 {\n\ttotal := 0.0\n\tfor _, s := range scores {\n\t\ttotal += s\n\t}\n\treturn total / float64(len(scores))\n}\n\nfunc main() {\n}\n"
		if err := os.WriteFile(path, []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := ApplyPatchText(context.Background(), recordedPatch, dir)
		if err == nil {
			t.Fatal("expected preflight hunk-header count mismatch error even when body matches file, got success")
		}
		if !strings.Contains(err.Error(), "hunk header") {
			t.Fatalf("error does not diagnose the hunk header: %q", err.Error())
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if got := string(data); got != original {
			t.Fatalf("file mutated by rejected patch:\n%s", got)
		}
	})
}

func TestApplyPatchPreflightAcceptsMatchingHunkHeaderCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "calc.go")
	original := "package main\n\nimport \"fmt\"\n\nfunc average(scores []float64) float64 {\n\ttotal := 0.0\n\tfor _, s := range scores {\n\t\ttotal += s\n\t}\n\treturn total / float64(len(scores))\n}\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	patch := `*** Begin Patch
*** Update File: calc.go
@@
@@ -8,3 +8,4 @@
 func average(scores []float64) float64 {
 	total := 0.0
+	if len(scores) == 0 {
 	for _, s := range scores {
*** End Patch`
	if _, err := ApplyPatchText(context.Background(), patch, dir); err != nil {
		t.Fatalf("matching header counts must still apply: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "if len(scores) == 0 {") {
		t.Fatalf("patched content = %q", string(data))
	}
}

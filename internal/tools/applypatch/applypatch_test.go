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
